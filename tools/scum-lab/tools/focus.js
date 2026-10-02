// Focus bench: one colonist, top to bottom, and whether their mind changes.
// Situation state lives on this module so a trip to the grammar tool and back
// doesn't wipe the person you were tuning.

import { store } from "../shared/store.js";
import { catalog, evaluate, rollColonist, traitById } from "../shared/sim.js";

const GROUP_LABELS = {
  appetite: "Appetite",
  work: "Work ethic",
  social: "Company",
  temperament: "Temperament",
  mutation: "Mutation",
  "mutant-attitude": "Mutants",
  nerve: "Nerve",
  outlook: "Outlook",
};

function formatHeight(cm) {
  const inches = Math.trunc((cm * 200 + 254) / 508);
  return `${Math.trunc(inches / 12)}'${inches % 12}"`;
}

const FOCI = [
  { id: "idle", title: "Linger" },
  { id: "work", title: "Work" },
  { id: "eat", title: "Eat" },
  { id: "relieve", title: "Use the toilet" },
  { id: "socialize", title: "Find someone" },
  { id: "sleep", title: "Sleep" },
  { id: "flee", title: "Run" },
  { id: "fight", title: "Fight" },
  { id: "escape", title: "Dig out" },
  { id: "wash", title: "Wash" },
];

const NEEDS = [
  { id: "food", title: "Hunger", hint: "A nutrient pod is how this gets answered." },
  { id: "bladder", title: "Bladder", hint: "This is the one that wants a toilet." },
  { id: "social", title: "Company", hint: "Someone free to talk, not a building." },
  { id: "sleep", title: "Sleep", hint: "A bed, once they're tired enough." },
  { id: "hygiene", title: "Hygiene", hint: "A shower; dirty work makes it jump." },
];

const AROUND = [
  { id: "pod", title: "Nutrient pod nearby", blocks: "Eat" },
  { id: "toilet", title: "Toilet nearby", blocks: "Use the toilet" },
  { id: "bed", title: "Bed nearby", blocks: "Sleep" },
  { id: "shower", title: "Shower nearby", blocks: "Wash" },
  { id: "company", title: "Someone nearby to talk to", blocks: "Find someone" },
  { id: "canWork", title: "Awake, or already on a job", blocks: "Work" },
  { id: "sealed", title: "Sealed off from the colony", blocks: "Dig out" },
];

const PHASE_WORD = {
  satisfied: "Fine",
  growing: "Building",
  pressing: "Pressing",
  critical: "Critical",
};

let bench = null;
let nextSeed = 1;

function titleOf(id) {
  return FOCI.find(focus => focus.id === id)?.title || id;
}

function fresh(seed) {
  const person = rollColonist(seed);
  const bench = {
    person,
    charge: 0,
    grip: 0,
    valence: 0,
    moodLabel: "",
    needs: { food: 200, bladder: 150, social: 100, sleep: 250, hygiene: 200 },
    currentFocus: "work",
    around: {
      pod: true,
      toilet: true,
      bed: true,
      shower: true,
      company: true,
      canWork: true,
      alien: false,
      weapon: "none",
      sealed: false,
    },
    stimuli: [],
  };
  const home = evaluate(store.config, bench).home;
  bench.charge = home.charge;
  bench.grip = home.grip;
  bench.valence = home.valence;
  return bench;
}

function groups() {
  const order = [];
  for (const trait of catalog().traits) {
    if (!order.includes(trait.group)) order.push(trait.group);
  }
  return order.map(id => ({
    id,
    label: GROUP_LABELS[id] || id,
    traits: catalog().traits.filter(trait => trait.group === id),
  }));
}

export const focusTool = {
  id: "focus",
  title: "Focus Tester",
  summary: "A colonist, their mood, their needs, and whether any of it is enough to change their mind.",
  mount(host) {
    if (!bench) bench = fresh(nextSeed);
    const ac = new AbortController();
    host.innerHTML = render();
    wire(host, ac.signal);
    paint(host);
    return () => ac.abort();
  },
};

function reroll(host, seed) {
  nextSeed = seed;
  const kept = {
    needs: { ...bench.needs },
    currentFocus: bench.currentFocus,
    around: { ...bench.around, alien: false },
  };
  bench = fresh(seed);
  bench.needs = kept.needs;
  bench.currentFocus = kept.currentFocus;
  bench.around = kept.around;
  bench.stimuli = [];
  host.innerHTML = render();
  paint(host);
}

function render() {
  const person = bench.person;
  const traitRows = list => list.map(group => {
    const chips = group.traits.map(trait => `
      <button type="button" class="chip" data-trait="${trait.id}" title="${trait.desc}">${trait.name}</button>`).join("");
    return `
      <div class="trait-group">
        <div class="trait-label">${group.label}</div>
        <div class="chips">${chips}</div>
      </div>`;
  }).join("");
  // A group is acquired only when every trait in it is: those are gained in
  // play, never rolled at spawn, so they get their own section.
  const all = groups();
  const acquired = all.filter(group => group.traits.every(trait => trait.acquired));
  const native = all.filter(group => !acquired.includes(group));
  const traitBlocks = [
    { title: "Native", hint: "rolled at birth", list: native },
    { title: "Acquired", hint: "gained on Mars", list: acquired },
  ].filter(kind => kind.list.length).map(kind => `
      <div class="trait-kind">
        <h3 class="trait-kind-title">${kind.title}<em>${kind.hint}</em></h3>
        ${traitRows(kind.list)}
      </div>`).join("");

  const needBlocks = NEEDS.map(need => `
    <label class="slider-row">
      <span class="slider-name">${need.title}<small>${need.hint}</small></span>
      <input type="range" min="0" max="1000" value="${bench.needs[need.id]}" data-need="${need.id}">
      <span class="slider-read"><b data-need-value="${need.id}"></b><i data-need-phase="${need.id}" class="pill"></i></span>
    </label>`).join("");

  const focusChips = FOCI.map(focus => `
    <button type="button" class="choice" data-focus="${focus.id}">${focus.title}</button>`).join("");

  const around = AROUND.map(item => `
    <label class="check">
      <input type="checkbox" data-around="${item.id}" ${bench.around[item.id] ? "checked" : ""}>
      <span>${item.title}</span>
    </label>`).join("");

  return `
  <div class="story">
    <section class="tile bio-tile">
      <p class="step">01 · Personnel file</p>
      <div class="bio-top">
        <div>
          <h2 class="bio-name">${person.name}</h2>
          <p class="bio-line">${person.name} is ${person.age}, ${person.pronouns}, ${person.orientation}.</p>
          <p class="bio-meta">${formatHeight(person.heightCM)} · ${person.weightKG} kg · ${person.hair} hair · <span class="swatch skin-${person.skin}"></span> ${person.skin} skin</p>
        </div>
        <div class="bio-actions">
          <button type="button" class="ghost" id="another">Another colonist</button>
          <label class="seed">Roll <input id="seed" type="number" min="1" value="${person.seed}"></label>
        </div>
      </div>
      <p class="hint">One trait per row. Tap again to clear.</p>
      <div class="traits">${traitBlocks}</div>
    </section>

    <section class="tile">
      <p class="step">02 · How they feel</p>
      <h2>Charge, grip, valence</h2>
      <p class="mood-word">They feel <strong data-mood>steady</strong>.</p>
      <p class="hint" data-home></p>
      ${axis("charge", "Charge", "How keyed up they are", -100, 100)}
      ${axis("grip", "Grip", "How in control they feel", -100, 100)}
      ${axis("valence", "Valence", "Whether it feels good or bad", -100, 100)}
      <button type="button" class="ghost" id="settle">Let them settle to their baseline</button>
    </section>

    <section class="tile">
      <p class="step">03 · What their body wants</p>
      <h2>Needs</h2>
      <p class="lede">Fine and building don't make them get up. Pressing and critical do.</p>
      ${needBlocks}
    </section>

    <section class="tile">
      <p class="step">04 · Focus</p>
      <div class="focus-block">
        <h3>Current</h3>
        <div class="choices" id="focus-choices">${focusChips}</div>
      </div>
      <div class="focus-block">
        <h3>Prerequisites</h3>
        <div class="around">${around}</div>
      </div>
      <div class="focus-block">
        <h3>Possible</h3>
        <ul class="eligibility" id="eligibility"></ul>
      </div>
    </section>

    <section class="tile">
      <p class="step">05 · What's in front of them</p>
      <h2>Stimuli</h2>
      <p class="lede">Seeing an alien is both a scare and a fact about the room. A weapon is what decides whether they can fight.</p>
      <div class="alien-card">
        <label class="check">
          <input type="checkbox" id="alien" ${bench.around.alien ? "checked" : ""}>
          <span>An alien is in sight</span>
        </label>
        <label class="field compact">
          <span>They're carrying</span>
          <select id="weapon">
            <option value="none"${bench.around.weapon === "none" ? " selected" : ""}>nothing</option>
            <option value="pistol"${bench.around.weapon === "pistol" ? " selected" : ""}>a pistol</option>
            <option value="shotgun"${bench.around.weapon === "shotgun" ? " selected" : ""}>a shotgun</option>
          </select>
        </label>
      </div>
      <p class="hint">Also on their mind. These come from reactions that carry a stimulus. Edit the weights under Grammar.</p>
      <div class="stim-grid" id="stim-grid"></div>
    </section>

    <section class="tile verdict-tile">
      <p class="step">06 · Focus result</p>
      <h2 data-verdict-title>Unchanged</h2>
      <p class="lede" data-verdict-sub></p>
      <div class="legend">
        <span><i class="seg base"></i> baseline</span>
        <span><i class="seg need"></i> need</span>
        <span><i class="seg affect"></i> mood</span>
        <span><i class="seg stimulus"></i> stimulus</span>
        <span><i class="seg stay"></i> staying put</span>
      </div>
      <div id="diagnostic"></div>
    </section>
  </div>`;
}

function axis(id, title, hint, min, max) {
  return `
    <label class="slider-row">
      <span class="slider-name">${title}<small>${hint}</small></span>
      <input type="range" min="${min}" max="${max}" value="${bench[id]}" data-axis="${id}">
      <span class="slider-read"><b data-axis-value="${id}"></b></span>
    </label>`;
}

function wire(host, signal) {
  const listen = (type, fn) => host.addEventListener(type, fn, { signal });
  listen("click", (event) => {
    if (event.target.closest("#another")) {
      const typed = Number(host.querySelector("#seed").value);
      const seed = Number.isInteger(typed) && typed > 0 ? typed + 1 : nextSeed + 1;
      reroll(host, seed);
      return;
    }
    if (event.target.closest("#settle")) {
      const home = evaluate(store.config, bench).home;
      bench.charge = home.charge;
      bench.grip = home.grip;
      bench.valence = home.valence;
      bench.moodLabel = "";
      for (const input of host.querySelectorAll("[data-axis]")) input.value = bench[input.dataset.axis];
      paint(host);
      return;
    }
    const trait = event.target.closest("[data-trait]");
    if (trait) {
      toggleTrait(trait.dataset.trait);
      paint(host);
      return;
    }
    const focus = event.target.closest("[data-focus]");
    if (focus && !focus.disabled) {
      bench.currentFocus = focus.dataset.focus;
      paint(host);
    }
  });
  listen("focusout", (event) => {
    if (event.target.id !== "seed") return;
    if (event.relatedTarget?.closest?.("#another")) return;
    const seed = Math.max(1, Number(event.target.value) || 1);
    if (seed !== bench.person.seed) reroll(host, seed);
  });
  listen("change", (event) => {
    if (event.target.id === "alien") {
      bench.around.alien = event.target.checked;
      syncAlien();
      paint(host);
      return;
    }
    if (event.target.id === "weapon") {
      bench.around.weapon = event.target.value;
      paint(host);
    }
  });
  listen("input", (event) => {
    const axisInput = event.target.dataset.axis;
    if (axisInput) {
      bench[axisInput] = Number(event.target.value);
      paint(host);
      return;
    }
    const need = event.target.dataset.need;
    if (need) {
      bench.needs[need] = Number(event.target.value);
      paint(host);
      return;
    }
    if (event.target.dataset.around) {
      bench.around[event.target.dataset.around] = event.target.checked;
      paint(host);
    }
  });
}

function toggleTrait(id) {
  const trait = traitById(id);
  if (!trait) return;
  const has = bench.person.traits.includes(id);
  bench.person.traits = bench.person.traits.filter(other => traitById(other)?.group !== trait.group);
  if (!has) bench.person.traits.push(id);
}

function syncAlien() {
  bench.stimuli = bench.stimuli.filter(stim => stim.kind !== "saw-alien");
  if (!bench.around.alien) return;
  const reaction = store.config.reactions["saw-alien"];
  if (!reaction?.stimulus) return;
  bench.stimuli.unshift({
    kind: "saw-alien",
    salience: reaction.stimulus.salience,
    lifetime: reaction.stimulus.lifetime,
  });
}

function toggleStimulus(kind, on) {
  bench.stimuli = bench.stimuli.filter(stim => stim.kind !== kind);
  if (!on) return;
  const limit = store.config.arbitration?.active_stimulus_limit ?? 8;
  if (bench.stimuli.length >= limit) return false;
  const reaction = store.config.reactions[kind];
  bench.stimuli.push({
    kind,
    salience: reaction.stimulus.salience,
    lifetime: reaction.stimulus.lifetime,
  });
  return true;
}

function paint(host) {
  const result = evaluate(store.config, bench);
  bench.moodLabel = result.mood.label;
  host.querySelector("[data-mood]").textContent = result.mood.word;
  host.querySelector("[data-home]").textContent =
    `Baseline: charge ${signed(result.home.charge)}, grip ${signed(result.home.grip)}, valence ${signed(result.home.valence)}.`;

  for (const id of ["charge", "grip", "valence"]) {
    host.querySelector(`[data-axis-value="${id}"]`).textContent = signed(bench[id]);
  }
  for (const need of NEEDS) {
    const phase = result.phases[need.id];
    host.querySelector(`[data-need-value="${need.id}"]`).textContent = String(bench.needs[need.id]);
    const pill = host.querySelector(`[data-need-phase="${need.id}"]`);
    pill.textContent = PHASE_WORD[phase];
    pill.className = `pill phase-${phase}`;
  }
  for (const button of host.querySelectorAll("[data-trait]")) {
    button.classList.toggle("on", bench.person.traits.includes(button.dataset.trait));
  }

  for (const button of host.querySelectorAll("[data-focus]")) {
    const candidate = result.candidates.find(item => item.id === button.dataset.focus);
    const current = button.dataset.focus === bench.currentFocus;
    button.disabled = !candidate?.eligible;
    button.classList.toggle("on", current);
    button.title = candidate?.eligible ? "" : (candidate?.reasons[0] || "");
  }
  const list = host.querySelector("#eligibility");
  list.replaceChildren();
  for (const candidate of result.candidates) {
    const item = document.createElement("li");
    item.className = candidate.eligible ? "ok" : "no";
    if (candidate.id === bench.currentFocus) item.classList.add("current");
    const name = document.createElement("strong");
    name.textContent = titleOf(candidate.id);
    const why = document.createElement("span");
    why.textContent = candidate.reasons.join(" ");
    item.append(name, why);
    list.append(item);
  }

  paintStimuli(host);
  paintVerdict(host, result);
}

function paintStimuli(host) {
  const grid = host.querySelector("#stim-grid");
  const selected = new Set(bench.stimuli.map(stim => stim.kind));
  const entries = Object.entries(store.config.reactions)
    .filter(([id, reaction]) => id !== "saw-alien" && reaction.stimulus && reaction.stimulus.salience > 0);
  grid.replaceChildren();
  if (entries.length === 0) {
    const empty = document.createElement("p");
    empty.className = "hint";
    empty.textContent = "No other stimuli in this config.";
    grid.append(empty);
    return;
  }
  for (const [id, reaction] of entries) {
    const label = document.createElement("label");
    label.className = "check stim";
    const box = document.createElement("input");
    box.type = "checkbox";
    box.checked = selected.has(id);
    box.onchange = () => {
      const added = toggleStimulus(id, box.checked);
      if (box.checked && added === false) {
        box.checked = false;
        const limit = store.config.arbitration?.active_stimulus_limit ?? 8;
        label.title = `Attention holds ${limit}. Clear one first.`;
      }
      paint(host);
    };
    const text = document.createElement("span");
    const focuses = Object.entries(reaction.stimulus.contributions || {})
      .map(([focus, value]) => `${titleOf(focus)} ${signed(value)}`)
      .join(", ");
    const strong = document.createElement("strong");
    strong.textContent = id.replaceAll("-", " ");
    const small = document.createElement("small");
    small.textContent = focuses;
    text.append(strong, small);
    label.append(box, text);
    grid.append(label);
  }
}

function paintVerdict(host, result) {
  const currentName = titleOf(result.current.id);
  const winnerName = titleOf(result.winner.id);
  const title = host.querySelector("[data-verdict-title]");
  const sub = host.querySelector("[data-verdict-sub]");
  // The verdict's current is only a ref; eligibility and reasons live on its candidate.
  const current = result.candidates.find(candidate => candidate.id === result.current.id);
  if (!result.switched) {
    title.textContent = `Unchanged: ${currentName}`;
    sub.textContent = result.held
      ? `${titleOf(result.lead.id)} is short of the ${result.margin}-point margin.`
      : "";
  } else {
    title.textContent = `Changed: ${winnerName}`;
    sub.textContent = current && !current.eligible ? (current.reasons?.[0] || "") : "";
  }
  sub.hidden = sub.textContent === "";

  const diagnostic = host.querySelector("#diagnostic");
  diagnostic.replaceChildren();
  const span = (candidate) => Math.max(0, candidate.base) + Math.max(0, candidate.need) + Math.max(0, candidate.affect) + Math.max(0, candidate.stimulus) + Math.max(0, candidate.commitment);
  // Scale to the race, so a gated focus with a huge base (dig out) doesn't
  // flatten the bars you can actually compare. That overflow is marked.
  const racing = result.candidates.filter(candidate => candidate.eligible);
  const max = Math.max(1, ...(racing.length ? racing : result.candidates).map(span));
  for (const candidate of result.candidates) {
    const row = document.createElement("div");
    row.className = "diag";
    if (!candidate.eligible) row.classList.add("out");
    if (candidate.id === result.winner.id) row.classList.add("winner");
    const head = document.createElement("div");
    head.className = "diag-head";
    const name = document.createElement("strong");
    name.textContent = titleOf(candidate.id);
    const score = document.createElement("span");
    score.textContent = String(candidate.total);
    if (!candidate.eligible) {
      const tag = document.createElement("em");
      tag.textContent = "out";
      score.append(" ", tag);
    }
    head.append(name, score);
    const track = document.createElement("div");
    track.className = "bar";
    if (!candidate.eligible && span(candidate) > max) track.classList.add("overflow");
    let used = 0;
    for (const [kind, value] of [["base", candidate.base], ["need", candidate.need], ["affect", candidate.affect], ["stimulus", candidate.stimulus], ["stay", candidate.commitment]]) {
      if (value <= 0 || used >= 100) continue;
      const seg = document.createElement("i");
      seg.className = `seg ${kind}`;
      const width = Math.min(100 - used, (value / max) * 100);
      used += width;
      seg.style.width = `${width}%`;
      track.append(seg);
    }
    const bits = document.createElement("p");
    bits.className = "weights";
    const parts = [
      `baseline ${signed(candidate.base)}`,
      `need ${signed(candidate.need)}`,
      `mood ${signed(candidate.affect)}`,
      `stimulus ${signed(candidate.stimulus)}`,
      `staying ${signed(candidate.commitment)}`,
    ];
    if (candidate.distance) parts.push(`a step to run ${signed(candidate.distance)}`);
    bits.textContent = parts.join(" · ");
    row.append(head, track, bits);
    if (!candidate.eligible && candidate.reasons.length) {
      const why = document.createElement("p");
      why.className = "why";
      why.textContent = candidate.reasons.join(" ");
      row.append(why);
    }
    diagnostic.append(row);
  }
}

function signed(value) {
  if (value > 0) return `+${value}`;
  return String(value);
}
