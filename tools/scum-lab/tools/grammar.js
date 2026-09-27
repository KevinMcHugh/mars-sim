// Grammar tool: perception rules, reactions, and trait appraisals.
// The focus bench reads the same config object, so a reaction's stimulus shows up there.

import { DEFAULT_VOCABULARY } from "../shared/defaults.js";
import { store } from "../shared/store.js";

const KEBAB = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

function q(root, sel) {
  return root.querySelector(sel);
}

function fillSelect(select, values, wildcard = true) {
  const previous = select.value;
  select.replaceChildren();
  if (wildcard) select.add(new Option("any", ""));
  for (const value of values) select.add(new Option(value, value));
  if ([...select.options].some(option => option.value === previous)) select.value = previous;
}

function vocab() {
  return store.config.vocabulary || DEFAULT_VOCABULARY;
}

export const grammarTool = {
  id: "grammar",
  title: "Grammar Builder",
  summary: "Perception rules, reactions, and the trait appraisals that scale them. This is the file the simulation loads.",
  mount(host) {
    host.innerHTML = TEMPLATE;
    const root = q(host, ".grammar");
    const editor = bind(root);
    editor.populate();
    editor.resetEditors();
    editor.renderAll();
    return () => { host.replaceChildren(); };
  },
};

function bind(root) {
  let editingRule = null;
  let editingPerception = null;
  let editingTrait = null;
  const lists = {
    rules: { query: "", open: null },
    perceptions: { query: "", open: null },
    traits: { query: "", open: null },
  };

  function scrollTo(id) {
    q(root, id)?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function populate() {
    const words = vocab();
    fillSelect(q(root, "#rule-actor"), words.nouns || []);
    fillSelect(q(root, "#rule-action"), words.actions || []);
    fillSelect(q(root, "#rule-object"), words.nouns || []);
    fillSelect(q(root, "#rule-channel"), words.channels || DEFAULT_VOCABULARY.channels);
    fillSelect(q(root, "#rule-role"), words.roles || DEFAULT_VOCABULARY.roles);
    fillSelect(q(root, "#rule-phase"), words.phases || DEFAULT_VOCABULARY.phases);
    fillSelect(q(root, "#rule-relation"), words.object_relations || DEFAULT_VOCABULARY.object_relations);
    fillSelect(q(root, "#rule-wear-policy"), words.wear_policies || DEFAULT_VOCABULARY.wear_policies, false);
    fillSelect(q(root, "#perception-actor"), words.nouns || []);
    fillSelect(q(root, "#perception-action"), words.actions || []);
    fillSelect(q(root, "#perception-object"), words.nouns || []);
    fillSelect(q(root, "#perception-channel"), words.channels || DEFAULT_VOCABULARY.channels, false);
    fillSelect(q(root, "#perception-role"), words.roles || DEFAULT_VOCABULARY.roles, false);
    fillSelect(q(root, "#perception-cadence"), words.cadences || DEFAULT_VOCABULARY.cadences, false);
    fillSelect(q(root, "#perception-radius"), words.radii || DEFAULT_VOCABULARY.radii);
    fillSelect(q(root, "#trait-name"), words.traits || DEFAULT_VOCABULARY.traits, false);
    fillSelect(q(root, "#trait-actor"), words.nouns || []);
    fillSelect(q(root, "#trait-action"), words.actions || []);
    fillSelect(q(root, "#trait-object"), words.nouns || []);
    fillSelect(q(root, "#trait-channel"), words.channels || DEFAULT_VOCABULARY.channels);
    fillSelect(q(root, "#trait-role"), words.roles || DEFAULT_VOCABULARY.roles);
    fillSelect(q(root, "#trait-phase"), words.phases || DEFAULT_VOCABULARY.phases);
    fillSelect(q(root, "#trait-relation"), words.object_relations || DEFAULT_VOCABULARY.object_relations);
  }

  function note(id, message, ok) {
    const el = q(root, id);
    el.textContent = message;
    el.className = ok ? "note ok" : "note bad";
  }

  function clearRule() {
    editingRule = null;
    const id = q(root, "#rule-id");
    id.disabled = false;
    id.value = "";
    for (const field of ["#rule-actor", "#rule-action", "#rule-object", "#rule-channel", "#rule-role", "#rule-phase", "#rule-relation"]) {
      q(root, field).value = "";
    }
    q(root, "#rule-wear-policy").value = "memory-occasions";
    q(root, "#rule-impact").value = 10;
    for (const field of ["charge", "grip", "valence"]) {
      q(root, `#rule-fresh-${field}`).value = 0;
      q(root, `#rule-worn-${field}`).value = 0;
    }
    q(root, "#rule-memory").value = "";
    note("#rule-validation", "", true);
  }

  function editRule(id) {
    const rule = store.config.reactions[id];
    if (!rule) return;
    editingRule = id;
    const input = q(root, "#rule-id");
    input.value = id;
    input.disabled = true;
    q(root, "#rule-actor").value = rule.match?.actor_noun || "";
    q(root, "#rule-action").value = rule.match?.action || "";
    q(root, "#rule-object").value = rule.match?.object_noun || "";
    q(root, "#rule-channel").value = rule.match?.channel || "";
    q(root, "#rule-role").value = rule.match?.role || "";
    q(root, "#rule-phase").value = rule.match?.phase || "";
    q(root, "#rule-relation").value = rule.match?.object_relation || "";
    q(root, "#rule-wear-policy").value = rule.wear_policy || "memory-occasions";
    q(root, "#rule-impact").value = rule.impact ?? 0;
    q(root, "#rule-fresh-charge").value = rule.fresh?.charge ?? 0;
    q(root, "#rule-fresh-grip").value = rule.fresh?.grip ?? 0;
    q(root, "#rule-fresh-valence").value = rule.fresh?.valence ?? 0;
    q(root, "#rule-worn-charge").value = rule.worn?.charge ?? 0;
    q(root, "#rule-worn-grip").value = rule.worn?.grip ?? 0;
    q(root, "#rule-worn-valence").value = rule.worn?.valence ?? 0;
    q(root, "#rule-memory").value = rule.memory?.text || "";
    note("#rule-validation", "", true);
    scrollTo("#rule-form");
  }

  function readRule() {
    const id = q(root, "#rule-id").value.trim().toLowerCase();
    if (!KEBAB.test(id)) throw new Error("Rule ID must be lowercase kebab-case.");
    if (!editingRule && store.config.reactions[id]) throw new Error(`Rule ID "${id}" already exists.`);
    const impact = Number(q(root, "#rule-impact").value);
    if (!Number.isInteger(impact) || impact < 0 || impact > 100) throw new Error("Impact must be an integer from 0 to 100.");
    const match = {
      actor_noun: q(root, "#rule-actor").value,
      action: q(root, "#rule-action").value,
      object_noun: q(root, "#rule-object").value,
      channel: q(root, "#rule-channel").value,
      role: q(root, "#rule-role").value,
      phase: q(root, "#rule-phase").value,
      object_relation: q(root, "#rule-relation").value,
    };
    const signature = JSON.stringify(match);
    for (const [otherID, other] of Object.entries(store.config.reactions)) {
      if (otherID !== editingRule && JSON.stringify(other.match || {}) === signature) {
        throw new Error(`Match duplicates "${otherID}". Give one rule a more specific match.`);
      }
    }
    return {
      id,
      rule: {
        match,
        memory: { record: true, text: q(root, "#rule-memory").value },
        impact,
        wear_policy: q(root, "#rule-wear-policy").value || "memory-occasions",
        fresh: {
          charge: Number(q(root, "#rule-fresh-charge").value) || 0,
          grip: Number(q(root, "#rule-fresh-grip").value) || 0,
          valence: Number(q(root, "#rule-fresh-valence").value) || 0,
        },
        worn: {
          charge: Number(q(root, "#rule-worn-charge").value) || 0,
          grip: Number(q(root, "#rule-worn-grip").value) || 0,
          valence: Number(q(root, "#rule-worn-valence").value) || 0,
        },
      },
    };
  }

  function renderRules() {
    paintList(lists.rules, "rule", Object.entries(store.config.reactions).map(([id, rule]) => {
      const summary = `${rule.match?.channel || "any"} / ${rule.match?.role || "any"} · ${rule.match?.actor_noun || "any"} ${rule.match?.action || "any"} ${rule.match?.object_noun || ""}`.trim();
      return item(id, summary, reactionFacts(rule), () => editRule(id), () => {
        delete store.config.reactions[id];
        if (editingRule === id) clearRule();
        if (lists.rules.open === id) lists.rules.open = null;
        renderRules();
      });
    }));
  }

  function clearPerception() {
    editingPerception = null;
    const id = q(root, "#perception-id");
    id.disabled = false;
    id.value = "";
    for (const field of ["#perception-actor", "#perception-action", "#perception-object", "#perception-radius"]) {
      q(root, field).value = "";
    }
    q(root, "#perception-channel").value = "sight";
    q(root, "#perception-role").value = "witness";
    q(root, "#perception-cadence").value = "instant";
    q(root, "#perception-distance").value = 0;
    q(root, "#perception-los").value = "false";
    q(root, "#perception-interrupt-rest").value = "false";
    note("#perception-validation", "", true);
  }

  function editPerception(id) {
    const perception = (store.config.perceptions || []).find(rule => rule.id === id);
    if (!perception) return;
    editingPerception = id;
    const input = q(root, "#perception-id");
    input.value = id;
    input.disabled = true;
    q(root, "#perception-actor").value = perception.match?.actor_noun || "";
    q(root, "#perception-action").value = perception.match?.action || "";
    q(root, "#perception-object").value = perception.match?.object_noun || "";
    q(root, "#perception-channel").value = perception.sense?.channel || "sight";
    q(root, "#perception-role").value = perception.sense?.role || "witness";
    q(root, "#perception-cadence").value = perception.sense?.cadence || "instant";
    q(root, "#perception-radius").value = perception.sense?.radius || "";
    q(root, "#perception-distance").value = perception.sense?.distance || 0;
    q(root, "#perception-los").value = String(Boolean(perception.sense?.line_of_sight));
    q(root, "#perception-interrupt-rest").value = String(Boolean(perception.sense?.interrupt_rest));
    note("#perception-validation", "", true);
    scrollTo("#perception-form");
  }

  function readPerception() {
    const id = q(root, "#perception-id").value.trim().toLowerCase();
    if (!KEBAB.test(id)) throw new Error("Rule ID must be lowercase kebab-case.");
    if (!editingPerception && store.config.perceptions.some(rule => rule.id === id)) throw new Error(`Rule ID "${id}" already exists.`);
    const distance = Number(q(root, "#perception-distance").value) || 0;
    const channel = q(root, "#perception-channel").value;
    const radius = q(root, "#perception-radius").value;
    const cadence = q(root, "#perception-cadence").value;
    const interruptRest = q(root, "#perception-interrupt-rest").value === "true";
    if (channel !== "direct" && !radius && distance <= 0) throw new Error("Spatial perception needs a named radius or a positive distance.");
    if (channel === "direct" && (radius || distance || cadence !== "instant")) throw new Error("Direct perception must be instant and have no radius.");
    if (interruptRest && cadence === "instant") throw new Error("interrupt_rest is only valid on persistent perception.");
    const sense = {
      channel,
      role: q(root, "#perception-role").value,
      radius,
      distance,
      line_of_sight: q(root, "#perception-los").value === "true",
      cadence,
    };
    if (interruptRest) sense.interrupt_rest = true;
    return {
      id,
      match: {
        actor_noun: q(root, "#perception-actor").value,
        action: q(root, "#perception-action").value,
        object_noun: q(root, "#perception-object").value,
      },
      sense,
    };
  }

  function renderPerceptions() {
    paintList(lists.perceptions, "perception", (store.config.perceptions || []).map(perception => {
      const summary = `${perception.sense.channel} / ${perception.sense.role} · ${perception.match?.actor_noun || "any"} ${perception.match?.action || "any"} ${perception.match?.object_noun || ""}`.trim();
      return item(perception.id, summary, perceptionFacts(perception), () => editPerception(perception.id), () => {
        store.config.perceptions = store.config.perceptions.filter(rule => rule.id !== perception.id);
        if (editingPerception === perception.id) clearPerception();
        if (lists.perceptions.open === perception.id) lists.perceptions.open = null;
        renderPerceptions();
      });
    }));
  }

  function clearTrait() {
    editingTrait = null;
    const id = q(root, "#trait-id");
    id.disabled = false;
    id.value = "";
    for (const field of ["#trait-actor", "#trait-action", "#trait-object", "#trait-channel", "#trait-role", "#trait-phase", "#trait-relation"]) {
      q(root, field).value = "";
    }
    q(root, "#trait-impact").value = 100;
    q(root, "#trait-charge").value = 100;
    q(root, "#trait-grip").value = 100;
    q(root, "#trait-valence").value = 100;
    q(root, "#trait-wear").value = 100;
    note("#trait-validation", "", true);
  }

  function editTrait(id) {
    const rule = (store.config.trait_rules || []).find(item => item.id === id);
    if (!rule) return;
    editingTrait = id;
    const input = q(root, "#trait-id");
    input.value = id;
    input.disabled = true;
    q(root, "#trait-name").value = rule.trait || "";
    q(root, "#trait-actor").value = rule.match?.actor_noun || "";
    q(root, "#trait-action").value = rule.match?.action || "";
    q(root, "#trait-object").value = rule.match?.object_noun || "";
    q(root, "#trait-channel").value = rule.match?.channel || "";
    q(root, "#trait-role").value = rule.match?.role || "";
    q(root, "#trait-phase").value = rule.match?.phase || "";
    q(root, "#trait-relation").value = rule.match?.object_relation || "";
    q(root, "#trait-impact").value = rule.scales?.impact ?? 100;
    q(root, "#trait-charge").value = rule.scales?.charge ?? 100;
    q(root, "#trait-grip").value = rule.scales?.grip ?? 100;
    q(root, "#trait-valence").value = rule.scales?.valence ?? 100;
    q(root, "#trait-wear").value = rule.scales?.wear_rate ?? 100;
    note("#trait-validation", "", true);
    scrollTo("#trait-form");
  }

  function readTrait() {
    const id = q(root, "#trait-id").value.trim().toLowerCase();
    if (!KEBAB.test(id)) throw new Error("Rule ID must be lowercase kebab-case.");
    if (!editingTrait && (store.config.trait_rules || []).some(rule => rule.id === id)) throw new Error(`Rule ID "${id}" already exists.`);
    const trait = q(root, "#trait-name").value;
    if (!trait) throw new Error("Pick a trait.");
    return {
      id,
      trait,
      match: {
        actor_noun: q(root, "#trait-actor").value,
        action: q(root, "#trait-action").value,
        object_noun: q(root, "#trait-object").value,
        channel: q(root, "#trait-channel").value,
        role: q(root, "#trait-role").value,
        phase: q(root, "#trait-phase").value,
        object_relation: q(root, "#trait-relation").value,
      },
      scales: {
        impact: Number(q(root, "#trait-impact").value) || 100,
        charge: Number(q(root, "#trait-charge").value) || 100,
        grip: Number(q(root, "#trait-grip").value) || 100,
        valence: Number(q(root, "#trait-valence").value) || 100,
        wear_rate: Number(q(root, "#trait-wear").value) || 100,
      },
    };
  }

  function renderTraits() {
    paintList(lists.traits, "trait", (store.config.trait_rules || []).map(rule => {
      const match = rule.match || {};
      const summary = `${rule.trait} · ${match.actor_noun || "any"} ${match.action || "any"} ${match.object_relation || ""}`.trim();
      return item(rule.id, summary, traitFacts(rule), () => editTrait(rule.id), () => {
        store.config.trait_rules = store.config.trait_rules.filter(entry => entry.id !== rule.id);
        if (editingTrait === rule.id) clearTrait();
        if (lists.traits.open === rule.id) lists.traits.open = null;
        renderTraits();
      });
    }));
  }

  function paintList(state, prefix, items) {
    const query = state.query.trim().toLowerCase();
    const shown = query ? items.filter(entry => entry.hay.includes(query)) : items;
    q(root, `#${prefix}-count`).textContent = query ? `${shown.length} / ${items.length}` : String(items.length);
    const list = q(root, `#${prefix}-list`);
    list.replaceChildren();
    if (shown.length === 0) {
      const empty = document.createElement("p");
      empty.className = "hint";
      empty.textContent = items.length === 0 ? "None yet." : "No matches.";
      list.append(empty);
      return;
    }
    for (const entry of shown) {
      list.append(ruleEntry(entry, state.open === entry.id, () => {
        state.open = state.open === entry.id ? null : entry.id;
        paintList(state, prefix, items);
      }));
    }
  }

  function renderAll() {
    renderRules();
    renderPerceptions();
    renderTraits();
  }

  q(root, "#rule-search").oninput = (event) => {
    lists.rules.query = event.target.value;
    renderRules();
  };
  q(root, "#perception-search").oninput = (event) => {
    lists.perceptions.query = event.target.value;
    renderPerceptions();
  };
  q(root, "#trait-search").oninput = (event) => {
    lists.traits.query = event.target.value;
    renderTraits();
  };

  q(root, "#rule-new").onclick = clearRule;
  q(root, "#rule-save").onclick = () => {
    try {
      const { id, rule } = readRule();
      store.config.reactions[id] = { ...(store.config.reactions[id] || {}), ...rule };
      editingRule = id;
      q(root, "#rule-id").disabled = true;
      note("#rule-validation", `Saved ${id}. Go checks the exported file again when the sim loads it.`, true);
      renderRules();
    } catch (error) {
      note("#rule-validation", error.message, false);
    }
  };
  q(root, "#perception-new").onclick = clearPerception;
  q(root, "#perception-save").onclick = () => {
    try {
      const rule = readPerception();
      const index = store.config.perceptions.findIndex(item => item.id === (editingPerception || rule.id));
      if (index >= 0) store.config.perceptions[index] = rule;
      else store.config.perceptions.push(rule);
      editingPerception = rule.id;
      q(root, "#perception-id").disabled = true;
      note("#perception-validation", `Saved ${rule.id}.`, true);
      renderPerceptions();
    } catch (error) {
      note("#perception-validation", error.message, false);
    }
  };
  q(root, "#trait-new").onclick = clearTrait;
  q(root, "#trait-save").onclick = () => {
    try {
      const rule = readTrait();
      if (!store.config.trait_rules) store.config.trait_rules = [];
      const index = store.config.trait_rules.findIndex(item => item.id === (editingTrait || rule.id));
      if (index >= 0) store.config.trait_rules[index] = rule;
      else store.config.trait_rules.push(rule);
      editingTrait = rule.id;
      q(root, "#trait-id").disabled = true;
      note("#trait-validation", `Saved ${rule.id}.`, true);
      renderTraits();
    } catch (error) {
      note("#trait-validation", error.message, false);
    }
  };

  return {
    populate,
    renderAll,
    resetEditors() {
      clearRule();
      clearPerception();
      clearTrait();
    },
  };
}

function item(id, summary, detail, onEdit, onDelete) {
  return {
    id,
    summary,
    detail,
    hay: [id, summary, ...detail.map(pair => pair.join(" "))].join(" ").toLowerCase(),
    onEdit,
    onDelete,
  };
}

function ruleEntry(entry, open, onToggle) {
  const wrap = document.createElement("div");
  wrap.className = open ? "rule-entry open" : "rule-entry";
  const line = document.createElement("div");
  line.className = "rule-row";
  const main = document.createElement("button");
  main.type = "button";
  main.className = "rule-main";
  main.setAttribute("aria-expanded", open ? "true" : "false");
  const code = document.createElement("code");
  code.textContent = entry.id;
  const text = document.createElement("span");
  text.textContent = entry.summary;
  main.append(code, text);
  main.onclick = onToggle;
  const actions = document.createElement("span");
  actions.className = "row-actions";
  const edit = document.createElement("button");
  edit.type = "button";
  edit.className = "ghost";
  edit.textContent = "Edit";
  edit.onclick = entry.onEdit;
  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "ghost";
  remove.textContent = "Delete";
  remove.onclick = entry.onDelete;
  actions.append(edit, remove);
  line.append(main, actions);
  wrap.append(line);
  if (open) wrap.append(facts(entry.detail));
  return wrap;
}

function facts(pairs) {
  const dl = document.createElement("dl");
  dl.className = "rule-facts";
  for (const [label, value] of pairs) {
    const bit = document.createElement("div");
    const dt = document.createElement("dt");
    dt.textContent = label;
    const dd = document.createElement("dd");
    dd.textContent = String(value);
    bit.append(dt, dd);
    dl.append(bit);
  }
  return dl;
}

function kept(entries) {
  return entries.filter(([, value]) => value !== "" && value !== undefined && value !== null);
}

function reactionFacts(rule) {
  const match = rule.match || {};
  const fresh = rule.fresh || {};
  const worn = rule.worn || {};
  const contributions = rule.stimulus?.contributions;
  const stimulus = contributions
    ? Object.entries(contributions).map(([focus, value]) => `${focus} ${value > 0 ? "+" : ""}${value}`).join(", ")
    : "";
  return kept([
    ["actor", match.actor_noun || "any"],
    ["action", match.action || "any"],
    ["object", match.object_noun || "any"],
    ["channel", match.channel || "any"],
    ["role", match.role || "any"],
    ["phase", match.phase || "any"],
    ["relation", match.object_relation || ""],
    ["wear", rule.wear_policy || ""],
    ["impact", rule.impact ?? ""],
    ["fresh", `${fresh.charge ?? 0} / ${fresh.grip ?? 0} / ${fresh.valence ?? 0}`],
    ["worn", `${worn.charge ?? 0} / ${worn.grip ?? 0} / ${worn.valence ?? 0}`],
    ["memory", rule.memory?.text || rule.memory?.collapse || ""],
    ["stimulus", stimulus],
  ]);
}

function perceptionFacts(perception) {
  const match = perception.match || {};
  const sense = perception.sense || {};
  return kept([
    ["actor", match.actor_noun || "any"],
    ["action", match.action || "any"],
    ["object", match.object_noun || "any"],
    ["channel", sense.channel || ""],
    ["role", sense.role || ""],
    ["cadence", sense.cadence || ""],
    ["radius", sense.radius || ""],
    ["distance", sense.distance || ""],
    ["line of sight", sense.line_of_sight ? "yes" : "no"],
    ["wakes from rest", sense.interrupt_rest ? "yes" : "no"],
  ]);
}

function traitFacts(rule) {
  const match = rule.match || {};
  const scales = rule.scales || {};
  return kept([
    ["trait", rule.trait || ""],
    ["actor", match.actor_noun || "any"],
    ["action", match.action || "any"],
    ["object", match.object_noun || "any"],
    ["channel", match.channel || "any"],
    ["role", match.role || "any"],
    ["phase", match.phase || "any"],
    ["relation", match.object_relation || ""],
    ["impact", `${scales.impact ?? 100}%`],
    ["charge", `${scales.charge ?? 100}%`],
    ["grip", `${scales.grip ?? 100}%`],
    ["valence", `${scales.valence ?? 100}%`],
    ["wear", `${scales.wear_rate ?? 100}%`],
  ]);
}

function field(label, control) {
  return `<label class="field"><span>${label}</span>${control}</label>`;
}

function select(id) {
  return `<select id="${id}"></select>`;
}

function num(id, value, min, max) {
  const bounds = `${min !== undefined ? ` min="${min}"` : ""}${max !== undefined ? ` max="${max}"` : ""}`;
  return `<input id="${id}" type="number" value="${value}"${bounds}>`;
}

function drawer(prefix, title) {
  return `
    <details class="rule-drawer">
      <summary>${title} <span class="count" id="${prefix}-count"></span></summary>
      <input id="${prefix}-search" class="rule-search" type="search" placeholder="Search" autocomplete="off">
      <div id="${prefix}-list" class="rule-list"></div>
    </details>`;
}

const TEMPLATE = `
<div class="grammar">
  <section class="tile subtool">
    <details open>
      <summary>Reactions</summary>
      <div class="subtool-body">
    <p class="lede">One percept, one reaction. Fresh is the first time. Worn is after it has happened enough to go dull.</p>
    <div class="form-grid" id="rule-form">
      ${field("Rule id", `<input id="rule-id" type="text" placeholder="saw-cat-play" autocomplete="off">`)}
      ${field("Actor", select("rule-actor"))}
      ${field("Action", select("rule-action"))}
      ${field("Object", select("rule-object"))}
      ${field("Channel", select("rule-channel"))}
      ${field("Role", select("rule-role"))}
      ${field("Phase", select("rule-phase"))}
      ${field("Relation", select("rule-relation"))}
      ${field("Wear", select("rule-wear-policy"))}
      ${field("Impact", num("rule-impact", 10, 0, 100))}
      ${field("Fresh charge", num("rule-fresh-charge", 0, -100, 100))}
      ${field("Fresh grip", num("rule-fresh-grip", 0, -100, 100))}
      ${field("Fresh valence", num("rule-fresh-valence", 0, -100, 100))}
      ${field("Worn charge", num("rule-worn-charge", 0, -100, 100))}
      ${field("Worn grip", num("rule-worn-grip", 0, -100, 100))}
      ${field("Worn valence", num("rule-worn-valence", 0, -100, 100))}
      <label class="field wide"><span>Memory line</span><input id="rule-memory" type="text" placeholder="Watched {actor} play with {object}."></label>
    </div>
    <div class="form-actions">
      <button type="button" id="rule-new" class="ghost">Clear</button>
      <button type="button" id="rule-save" class="primary">Save</button>
    </div>
    <p id="rule-validation" class="note"></p>
    ${drawer("rule", "Rules")}
      </div>
    </details>
  </section>

  <section class="tile subtool">
    <details>
      <summary>Perceptions</summary>
      <div class="subtool-body">
    <p class="lede">A perception rule says which channel picks the fact up, and from how far.</p>
    <div class="form-grid" id="perception-form">
      ${field("Rule id", `<input id="perception-id" type="text" placeholder="see-cat-play" autocomplete="off">`)}
      ${field("Actor", select("perception-actor"))}
      ${field("Action", select("perception-action"))}
      ${field("Object", select("perception-object"))}
      ${field("Channel", select("perception-channel"))}
      ${field("Role", select("perception-role"))}
      ${field("Cadence", select("perception-cadence"))}
      ${field("Named radius", select("perception-radius"))}
      ${field("Fixed distance", num("perception-distance", 0, 0))}
      ${field("Line of sight", `<select id="perception-los"><option value="false">No</option><option value="true">Yes</option></select>`)}
      ${field("Wakes them from rest", `<select id="perception-interrupt-rest"><option value="false">No</option><option value="true">Yes</option></select>`)}
    </div>
    <div class="form-actions">
      <button type="button" id="perception-new" class="ghost">Clear</button>
      <button type="button" id="perception-save" class="primary">Save</button>
    </div>
    <p id="perception-validation" class="note"></p>
    ${drawer("perception", "Rules")}
      </div>
    </details>
  </section>

  <section class="tile subtool">
    <details>
      <summary>Trait Appraisals</summary>
      <div class="subtool-body">
    <p class="lede">Scales are percents of the reaction. 100 leaves it alone. Empty match fields mean "anything."</p>
    <div class="form-grid" id="trait-form">
      ${field("Rule id", `<input id="trait-id" type="text" placeholder="cowardly-visible-alien" autocomplete="off">`)}
      ${field("Trait", select("trait-name"))}
      ${field("Actor", select("trait-actor"))}
      ${field("Action", select("trait-action"))}
      ${field("Object", select("trait-object"))}
      ${field("Channel", select("trait-channel"))}
      ${field("Role", select("trait-role"))}
      ${field("Phase", select("trait-phase"))}
      ${field("Relation", select("trait-relation"))}
      ${field("Impact %", num("trait-impact", 100))}
      ${field("Charge %", num("trait-charge", 100))}
      ${field("Grip %", num("trait-grip", 100))}
      ${field("Valence %", num("trait-valence", 100))}
      ${field("Wear rate %", num("trait-wear", 100))}
    </div>
    <div class="form-actions">
      <button type="button" id="trait-new" class="ghost">Clear</button>
      <button type="button" id="trait-save" class="primary">Save</button>
    </div>
    <p id="trait-validation" class="note"></p>
    ${drawer("trait", "Rules")}
      </div>
    </details>
  </section>
</div>
`;
