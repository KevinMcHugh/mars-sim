// The workbench's own YAML reader and writer. It is intentionally small: it
// understands the cognition.yaml shape the lab emits, not arbitrary YAML.
// Go still validates the file at load time.

import { DEFAULT_VOCABULARY } from "./defaults.js";

const KEBAB = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

function yamlScalar(value) {
  if (typeof value === "boolean" || typeof value === "number") return String(value);
  const text = String(value ?? "");
  return /^[a-z0-9-]+$/.test(text) ? text : JSON.stringify(text);
}

function flowMap(object) {
  return `{ ${Object.entries(object || {})
    .filter(([, value]) => value !== "" && value !== undefined && value !== null)
    .map(([key, value]) => `${key}: ${yamlScalar(value)}`).join(", ")} }`;
}

export function dumpYAML(cfg) {
  let out = `# mars-sim compositional perception and cognition settings\nschema_version: 1\n\n`;
  const vocabulary = cfg.vocabulary || DEFAULT_VOCABULARY;
  out += `vocabulary:\n`;
  out += `  nouns: [${(vocabulary.nouns || []).join(", ")}]\n`;
  out += `  actions: [${(vocabulary.actions || []).join(", ")}]\n\n`;
  out += `attractors:\n`;
  for (const [key, attractor] of Object.entries(cfg.attractors)) {
    out += `  ${key.padEnd(10)}: { good: "${attractor.good}", bad: "${attractor.bad}", charge: ${attractor.charge}, grip: ${attractor.grip}, radius: ${attractor.radius} }\n`;
  }
  out += `\nfocuses:\n`;
  for (const [key, focus] of Object.entries(cfg.focuses)) {
    out += `  ${key.padEnd(10)}: { base: ${focus.base}, need_weight: ${focus.need_weight}, charge_weight: ${focus.charge_weight}, grip_weight: ${focus.grip_weight}, distance_weight: ${focus.distance_weight} }\n`;
  }
  out += `\narbitration:\n`;
  out += `  current_bonus: ${cfg.arbitration.current_bonus}\n`;
  out += `  switch_margin: ${cfg.arbitration.switch_margin}\n`;
  out += `  critical_bonus: ${cfg.arbitration.critical_bonus}\n`;
  out += `  fatal_bonus: ${cfg.arbitration.fatal_bonus}\n`;
  out += `  active_stimulus_limit: ${cfg.arbitration.active_stimulus_limit}\n`;

  out += `\nperceptions:\n`;
  for (const perception of (cfg.perceptions || [])) {
    out += `  - id: ${perception.id}\n`;
    if (Object.keys(perception.match || {}).length) out += `    match: ${flowMap(perception.match)}\n`;
    out += `    sense: ${flowMap(perception.sense)}\n`;
  }

  out += `\nreactions:\n`;
  for (const [id, reaction] of Object.entries(cfg.reactions)) {
    out += `  - id: ${id}\n`;
    if (reaction.priority) out += `    priority: ${reaction.priority}\n`;
    out += `    match: ${flowMap(reaction.match)}\n`;
    if (reaction.memory?.record === false) {
      out += `    memory: { record: false }\n`;
    } else {
      const memory = { record: true };
      if (reaction.memory?.text) memory.text = reaction.memory.text;
      if (reaction.memory?.collapse) memory.collapse = reaction.memory.collapse;
      out += `    memory: ${flowMap(memory)}\n`;
    }
    const fresh = reaction.fresh || { charge: 0, grip: 0, valence: 0 };
    const worn = reaction.worn || fresh;
    out += `    affect: { impact: ${reaction.impact}, fresh: ${flowMap(fresh)}, worn: ${flowMap(worn)} }\n`;
    out += `    wear_policy: ${reaction.wear_policy || "memory-occasions"}\n`;
    if (reaction.stimulus) {
      const contributions = flowMap(reaction.stimulus.contributions || {});
      out += `    stimulus: { salience: ${reaction.stimulus.salience}, lifetime: ${reaction.stimulus.lifetime}, source: ${reaction.stimulus.source || "actor"}, contributions: ${contributions} }\n`;
    }
  }

  out += `\ntrait_rules:\n`;
  for (const rule of (cfg.trait_rules || [])) {
    out += `  - id: ${rule.id}\n`;
    out += `    trait: ${rule.trait}\n`;
    if (Object.values(rule.match || {}).some(Boolean)) out += `    match: ${flowMap(rule.match)}\n`;
    out += `    scales: ${flowMap(rule.scales)}\n`;
  }
  return out;
}

function splitTopLevel(text, delimiter) {
  const parts = [];
  let start = 0;
  let depth = 0;
  let quote = null;
  for (let i = 0; i < text.length; i++) {
    const char = text[i];
    if (quote) {
      if (char === quote && text[i - 1] !== "\\") quote = null;
      continue;
    }
    if (char === '"' || char === "'") quote = char;
    else if (char === "{" || char === "[") depth++;
    else if (char === "}" || char === "]") depth--;
    else if (char === delimiter && depth === 0) {
      parts.push(text.slice(start, i).trim());
      start = i + 1;
    }
  }
  parts.push(text.slice(start).trim());
  return parts.filter(Boolean);
}

function parseYAMLValue(raw) {
  const value = raw.trim();
  if (value.startsWith("{") && value.endsWith("}")) {
    const result = {};
    for (const part of splitTopLevel(value.slice(1, -1), ",")) {
      const pair = splitTopLevel(part, ":");
      if (pair.length < 2) throw new Error(`Invalid flow map entry: ${part}`);
      result[pair[0].trim()] = parseYAMLValue(pair.slice(1).join(":"));
    }
    return result;
  }
  if (value.startsWith("[") && value.endsWith("]")) {
    return splitTopLevel(value.slice(1, -1), ",").map(parseYAMLValue);
  }
  if (value === "true" || value === "false") return value === "true";
  if (/^-?\d+$/.test(value)) return Number(value);
  if (value.startsWith('"')) return JSON.parse(value);
  if (value.startsWith("'") && value.endsWith("'")) return value.slice(1, -1);
  return value;
}

function linePair(trimmed) {
  const colon = trimmed.indexOf(":");
  if (colon < 0) throw new Error(`Expected key: value, got "${trimmed}"`);
  return [trimmed.slice(0, colon).trim(), trimmed.slice(colon + 1).trim()];
}

function validateConfig(candidate) {
  const vocab = candidate.vocabulary || DEFAULT_VOCABULARY;
  const sets = {
    actor_noun: new Set(vocab.nouns || []),
    object_noun: new Set(vocab.nouns || []),
    action: new Set(vocab.actions || []),
    channel: new Set(vocab.channels || DEFAULT_VOCABULARY.channels),
    role: new Set(vocab.roles || DEFAULT_VOCABULARY.roles),
    phase: new Set(vocab.phases || DEFAULT_VOCABULARY.phases),
    object_relation: new Set(vocab.object_relations || DEFAULT_VOCABULARY.object_relations),
  };
  const seen = new Set();
  for (const [id, reaction] of Object.entries(candidate.reactions || {})) {
    if (!KEBAB.test(id) || seen.has(id)) throw new Error(`Invalid or duplicate reaction ID "${id}"`);
    seen.add(id);
    for (const [field, allowed] of Object.entries(sets)) {
      const value = reaction.match?.[field];
      if (value && !allowed.has(value)) throw new Error(`Reaction ${id} uses unknown ${field} "${value}"`);
    }
    if (!Number.isInteger(reaction.impact) || reaction.impact < 0 || reaction.impact > 100) {
      throw new Error(`Reaction ${id} impact must be 0..100`);
    }
  }
  const perceptionIDs = new Set();
  for (const perception of (candidate.perceptions || [])) {
    if (!KEBAB.test(perception.id) || perceptionIDs.has(perception.id)) {
      throw new Error(`Invalid or duplicate perception ID "${perception.id}"`);
    }
    perceptionIDs.add(perception.id);
    for (const field of ["actor_noun", "object_noun", "action"]) {
      const value = perception.match?.[field];
      if (value && !sets[field].has(value)) throw new Error(`Perception ${perception.id} uses unknown ${field} "${value}"`);
    }
    if (!sets.channel.has(perception.sense?.channel) || !sets.role.has(perception.sense?.role)) {
      throw new Error(`Perception ${perception.id} has an unknown channel or role`);
    }
  }
}

export function parseYAML(text, base) {
  const candidate = JSON.parse(JSON.stringify(base));
  let section = null;
  let current = null;
  const allowed = new Set(["vocabulary", "attractors", "focuses", "arbitration", "perceptions", "reactions", "trait_rules"]);
  for (const rawLine of text.split("\n")) {
    const trimmed = rawLine.trim();
    if (!trimmed || trimmed.startsWith("#") || trimmed.startsWith("schema_version:")) continue;
    if (!rawLine.startsWith(" ") && trimmed.endsWith(":")) {
      section = trimmed.slice(0, -1);
      if (!allowed.has(section)) throw new Error(`Unknown section "${section}"`);
      current = null;
      if (section === "perceptions") candidate.perceptions = [];
      if (section === "reactions") candidate.reactions = {};
      if (section === "trait_rules") candidate.trait_rules = [];
      continue;
    }
    if (!section) throw new Error(`Value outside a known section: ${trimmed}`);

    if (section === "vocabulary") {
      const [key, value] = linePair(trimmed);
      candidate.vocabulary[key] = parseYAMLValue(value);
    } else if (section === "attractors" || section === "focuses") {
      const [key, value] = linePair(trimmed);
      candidate[section][key] = { ...(candidate[section][key] || {}), ...parseYAMLValue(value) };
    } else if (section === "arbitration") {
      const [key, value] = linePair(trimmed);
      candidate.arbitration[key] = parseYAMLValue(value);
    } else if (trimmed.startsWith("- id:")) {
      const id = parseYAMLValue(trimmed.slice(trimmed.indexOf(":") + 1));
      if (section === "reactions") {
        current = {
          id, match: {}, memory: { record: true }, impact: 0,
          fresh: { charge: 0, grip: 0, valence: 0 },
          worn: { charge: 0, grip: 0, valence: 0 },
          wear_policy: "memory-occasions",
        };
        candidate.reactions[id] = current;
      } else {
        current = { id, match: {}, scales: {} };
        candidate[section].push(current);
      }
    } else {
      if (!current) throw new Error(`${section} field appeared before an id`);
      const [key, value] = linePair(trimmed);
      const parsed = parseYAMLValue(value);
      if (section === "reactions" && key === "affect") {
        current.impact = parsed.impact;
        current.fresh = parsed.fresh || parsed.target || current.fresh;
        current.worn = parsed.worn || current.fresh;
      } else {
        current[key] = parsed;
      }
    }
  }
  validateConfig(candidate);
  return candidate;
}

export function parseVocabulary(raw, current) {
  const imported = JSON.parse(raw);
  const vocabulary = imported.vocabulary || imported;
  if (!Array.isArray(vocabulary.nouns) || !Array.isArray(vocabulary.actions)) {
    throw new Error("Vocabulary JSON needs nouns and actions arrays.");
  }
  return { ...current, vocabulary: { ...DEFAULT_VOCABULARY, ...vocabulary } };
}
