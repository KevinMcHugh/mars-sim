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
// Whether a game has started. The page opens on the New game form, so topics
// are subscribed long before there is an engine to advance: until then a
// subscription is only recorded, and nothing runs.
let started = false;

const ping = new MessageChannel();
ping.port1.onmessage = () => { queued = false; slice(); };

function schedule(waitMs) {
  if (timer !== null) { clearTimeout(timer); timer = null; }
  if (!started) return; // no game yet: start schedules the first slice
  if (waitMs < 0) return; // paused: only a command resumes
  if (waitMs === 0) {
    if (!queued) { queued = true; ping.port2.postMessage(0); }
  } else {
    timer = setTimeout(() => { timer = null; slice(); }, waitMs);
  }
}

function slice() {
  slices++;
  const r = marssim.advance(budgetMs);
  if (typeof r === 'string') { postMessage({ type: 'error', error: JSON.parse(r).error }); return; }
  if (r.frame) {
    // The frame is a fresh, exactly sized buffer: transfer it, don't copy it.
    const buffer = r.frame.buffer;
    postMessage({ type: 'frame', buffer, perf: r.perf && JSON.parse(r.perf) }, [buffer]);
  }
  if (r.topics) postMessage({ type: 'topics', topics: JSON.parse(r.topics) });
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

// The handler is async (start awaits the WASM), so an exception in it would
// become an unhandled rejection that the page never hears about: a panel
// waiting on a topic would just sit on "Loading…". Report every failure.
onmessage = async (e) => {
  try {
    await handle(e.data);
  } catch (err) {
    postMessage({ type: 'error', error: String(err && err.message ? err.message : err) });
  }
};

async function handle(msg) {
  // Every message waits for the engine to load, not just start: a panel
  // opened while the page is still loading subscribes before marssim
  // exists, and that subscription must not be lost. Awaiting one promise
  // resumes in arrival order, so messages keep their order.
  const loadMs = await ready;
  switch (msg.type) {
    case 'start': {
      budgetMs = msg.budgetMs ?? budgetMs;
      // Builds before the API was versioned have no marssim.api: call them 1.
      const api = marssim.api ?? 1;
      const r = JSON.parse(marssim.start(JSON.stringify(msg.settings)));
      postMessage({ type: 'started', result: r, loadMs, api });
      if (!r.error) { started = true; schedule(0); }
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
    case 'subscribe':
    case 'unsubscribe': {
      const r = JSON.parse(marssim[msg.type](msg.topic));
      if (r.error) postMessage({ type: 'error', error: r.error });
      schedule(0); // a new subscription is sent on the next advance, even paused
      break;
    }
    case 'interest': {
      const { x0, y0, x1, y1 } = msg.rect;
      marssim.interest(x0, y0, x1, y1);
      schedule(0); // send the newly visible pages now, even if paused
      break;
    }
  }
}
