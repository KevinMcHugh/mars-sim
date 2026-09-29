// The engine's host loop (docs/browser-frontend.md, "The engine in a worker").
//
// Go's WASM runtime only returns to the event loop when every goroutine is
// blocked, so the engine cannot run its own loop here: the worker calls
// marssim.advance in slices and yields between them.
//
// A slice that ended with a tick still due (wait 0: flat out, or out of
// budget) is re-queued through a MessageChannel rather than setTimeout(0),
// which browsers clamp to at least 4 ms once nested, capping the sim near
// 250 tps. Any positive wait goes to setTimeout, clamp and all: the engine's
// schedule is fixed deadlines, so a late slice runs the ticks it missed, and a
// 1000 tps game holds 1000 tps in 4 ms batches. Pinging for a sub-millisecond
// wait instead would spin the worker (tens of thousands of empty slices a
// second, measured) to save nothing.
importScripts('wasm_exec.js');

let budgetMs = 8;
let timer = null;      // pending setTimeout, when the next tick is in the future
let queued = false;    // a MessageChannel ping is in flight
let slices = 0;

const ping = new MessageChannel();
ping.port1.onmessage = () => { queued = false; slice(); };

function schedule(waitMs) {
  if (timer !== null) { clearTimeout(timer); timer = null; }
  if (waitMs < 0) return; // paused: only a command resumes
  if (waitMs === 0) {
    if (!queued) { queued = true; ping.port2.postMessage(0); }
  } else {
    timer = setTimeout(() => { timer = null; slice(); }, waitMs);
  }
}

function slice() {
  slices++;
  const r = JSON.parse(marssim.advance(budgetMs));
  if (r.error) { postMessage({ type: 'error', error: r.error }); return; }
  if (r.frame) postMessage({ type: 'frame', frame: r.frame, at: performance.now() });
  schedule(r.wait);
}

const ready = (async () => {
  const go = new Go();
  const t0 = performance.now();
  const { instance } = await WebAssembly.instantiateStreaming(fetch('mars-sim.wasm'), go.importObject);
  go.run(instance); // returns once main blocks; the exports are set by then
  return performance.now() - t0;
})();

setInterval(() => {
  if (typeof marssim === 'undefined') return;
  postMessage({ type: 'memory', memory: JSON.parse(marssim.memory()), slices });
  slices = 0;
}, 1000);

onmessage = async (e) => {
  const msg = e.data;
  switch (msg.type) {
    case 'start': {
      const loadMs = await ready;
      budgetMs = msg.budgetMs ?? budgetMs;
      const r = JSON.parse(marssim.start(JSON.stringify(msg.settings)));
      postMessage({ type: 'started', result: r, loadMs });
      if (!r.error) schedule(0);
      break;
    }
    case 'command': {
      const r = JSON.parse(marssim.send(JSON.stringify(msg.command)));
      if (r.error) postMessage({ type: 'error', error: r.error });
      schedule(0); // apply it now, even if paused or waiting on a slow tick rate
      break;
    }
    case 'budget':
      budgetMs = msg.budgetMs;
      break;
  }
};
