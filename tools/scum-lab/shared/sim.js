// The browser side of the lab WASM build. Scoring and colonist rolls happen
// in the sim package; this module only carries the open file and the story
// across that boundary.

let catalogCache = null;

export async function bootSim() {
  const go = new Go();
  const response = await fetch(new URL("../scumlab.wasm", import.meta.url));
  if (!response.ok) {
    throw new Error("The lab's sim module did not load. Build it with go build -o tools/scum-lab/scumlab.wasm from tools/scum-lab/wasm.");
  }
  const bytes = await response.arrayBuffer();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  go.run(instance);
  catalogCache = read("catalog");
}

export function catalog() {
  return catalogCache;
}

export function rollColonist(seed) {
  return read("roll", seed);
}

export function traitById(id) {
  return catalogCache.traits.find(trait => trait.id === id);
}

export function evaluate(config, bench) {
  return read("evaluate", JSON.stringify(cognitionPayload(config, bench)), JSON.stringify(situationPayload(bench)));
}

function read(name, ...args) {
  const data = JSON.parse(globalThis.scumlab[name](...args));
  if (data.error) throw new Error(data.error);
  return data;
}

function cognitionPayload(config, bench) {
  const focuses = {};
  for (const [id, focus] of Object.entries(config.focuses || {})) {
    focuses[id] = {
      base: focus.base ?? 0,
      driveWeight: focus.drive_weight ?? 0,
      chargeWeight: focus.charge_weight ?? 0,
      gripWeight: focus.grip_weight ?? 0,
      distanceWeight: focus.distance_weight ?? 0,
    };
  }
  const arbitration = config.arbitration || {};
  return {
    focuses,
    arbitration: {
      currentBonus: arbitration.current_bonus ?? 0,
      switchMargin: arbitration.switch_margin ?? 0,
      criticalBonus: arbitration.critical_bonus ?? 0,
      fatalBonus: arbitration.fatal_bonus ?? 0,
    },
    attractors: Object.entries(config.attractors || {}).map(([kind, attractor]) => ({
      kind,
      good: attractor.good,
      bad: attractor.bad,
      charge: attractor.charge,
      grip: attractor.grip,
      radius: attractor.radius,
    })),
    stimuli: (bench.stimuli || []).map(stim => ({
      kind: stim.kind,
      salience: stim.salience,
      contributions: config.reactions?.[stim.kind]?.stimulus?.contributions || {},
    })),
  };
}

function situationPayload(bench) {
  return {
    charge: bench.charge,
    grip: bench.grip,
    valence: bench.valence,
    moodLabel: bench.moodLabel || "",
    drives: bench.drives,
    traits: bench.person?.traits || [],
    current: bench.currentFocus,
    canWork: !!bench.around?.canWork,
    pod: !!bench.around?.pod,
    toilet: !!bench.around?.toilet,
    bed: !!bench.around?.bed,
    company: !!bench.around?.company,
    alien: !!bench.around?.alien,
    armed: !!bench.around?.weapon && bench.around.weapon !== "none",
    sealed: !!bench.around?.sealed,
  };
}
