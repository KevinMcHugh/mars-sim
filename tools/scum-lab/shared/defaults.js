// Baked-in copy of the workbench defaults, aligned with cognition.yaml.
// Importing a file replaces this at runtime. When cognition.yaml's defaults
// change, update this module in the same change — the lab does not regenerate it.

const DEFAULT_CONFIG = {
  attractors: {
    driven:    { good: "driven",    bad: "furious",    charge: 70,  grip: 60,  radius: 46 },
    elated:    { good: "elated",    bad: "frantic",    charge: 88,  grip: 0,   radius: 42 },
    giddy:     { good: "giddy",     bad: "panicked",   charge: 65,  grip: -62, radius: 46 },
    adrift:    { good: "adrift",    bad: "anxious",    charge: 0,   grip: -85, radius: 42 },
    listless:  { good: "listless",  bad: "despairing", charge: -62, grip: -62, radius: 46 },
    spent:     { good: "spent",     bad: "numb",       charge: -85, grip: 0,   radius: 42 },
    content:   { good: "content",   bad: "grim",       charge: -60, grip: 60,  radius: 46 },
    composed:  { good: "composed",  bad: "hardened",   charge: 0,   grip: 85,  radius: 42 },
    steady:    { good: "steady",    bad: "flat",       charge: 0,   grip: 0,   radius: 28 }
  },
  focuses: {
    idle:      { base: 0,  need_weight: 0,   charge_weight: -10, grip_weight: 0,   distance_weight: 0 },
    work:      { base: 25, need_weight: 0,   charge_weight: 20,  grip_weight: 10,  distance_weight: 1 },
    eat:       { base: 40, need_weight: 100, charge_weight: 0,   grip_weight: 5,   distance_weight: 1 },
    relieve:   { base: 40, need_weight: 100, charge_weight: 0,   grip_weight: 0,   distance_weight: 1 },
    socialize: { base: 40, need_weight: 100, charge_weight: 10,  grip_weight: 5,   distance_weight: 1 },
    sleep:     { base: 40, need_weight: 100, charge_weight: -30, grip_weight: 0,   distance_weight: 1 },
    flee:      { base: 0,  need_weight: 0,   charge_weight: 20,  grip_weight: -40, distance_weight: 1 },
    fight:     { base: 15, need_weight: 0,   charge_weight: 20,  grip_weight: 40,  distance_weight: 1 },
    escape:    { base: 400, need_weight: 0,   charge_weight: 0,   grip_weight: 0,   distance_weight: 0 }
  },
  arbitration: {
    current_bonus: 25,
    switch_margin: 10,
    critical_bonus: 100,
    fatal_bonus: 150,
    active_stimulus_limit: 8
  },
  reactions: {
    "saw-alien": { impact: 45, fresh: { charge: 8, grip: -10, valence: -15 }, worn: { charge: 5, grip: -22, valence: -22 }, wear_policy: "memory-occasions", stimulus: { salience: 100, lifetime: 12, contributions: { flee: 500, fight: 500 } } },
    "saw-rat": { impact: 8, fresh: { charge: 2, grip: -3, valence: 0 }, worn: { charge: 1, grip: -1, valence: 0 }, wear_policy: "memory-occasions" },
    "saw-gore": { impact: 30, fresh: { charge: -3, grip: -7, valence: -6 }, worn: { charge: -6, grip: -14, valence: -10 }, wear_policy: "memory-occasions", stimulus: { salience: 35, lifetime: 30, contributions: { work: 20 } } },
    "bitten": { impact: 70, fresh: { charge: 55, grip: -44, valence: -30 }, worn: { charge: 35, grip: -80, valence: -55 }, wear_policy: "memory-occasions", stimulus: { salience: 100, lifetime: 20, contributions: { flee: 300, fight: 150 } } },
    "witnessed-colonist-killed": { impact: 95, fresh: { charge: 70, grip: 40, valence: -60 }, worn: { charge: 20, grip: -85, valence: -85 }, wear_policy: "memory-occasions", stimulus: { salience: 90, lifetime: 20, contributions: { flee: 250, fight: 100 } } },
    "witnessed-colonist-attacked": { impact: 75, fresh: { charge: 45, grip: 25, valence: -40 }, worn: { charge: 25, grip: -70, valence: -60 }, wear_policy: "memory-occasions", stimulus: { salience: 70, lifetime: 12, contributions: { flee: 200, fight: 100 } } },
    "crushed-rat": { impact: 6, fresh: { charge: -1, grip: 2, valence: 0 }, worn: { charge: -2, grip: 0, valence: 0 }, wear_policy: "memory-occasions" },
    "witnessed-rat-crushed": { impact: 8, fresh: { charge: -1, grip: -2, valence: -1 }, worn: { charge: -1, grip: -1, valence: 0 }, wear_policy: "memory-occasions" },
    "witnessed-cat-catch": { impact: 5, fresh: { charge: 1, grip: 1, valence: 0 }, worn: { charge: 0, grip: 0, valence: 0 }, wear_policy: "memory-occasions" },
    "killed-alien": { impact: 55, fresh: { charge: 45, grip: 52, valence: 35 }, worn: { charge: 25, grip: 20, valence: 8 }, wear_policy: "memory-occasions" },
    "witnessed-alien-killed": { impact: 35, fresh: { charge: 5, grip: 6, valence: 4 }, worn: { charge: 2, grip: 2, valence: 0 }, wear_policy: "memory-occasions" },
    "wounded-alien": { impact: 25, fresh: { charge: 4, grip: 5, valence: 2 }, worn: { charge: 2, grip: 2, valence: 0 }, wear_policy: "memory-occasions" },
    "witnessed-gunfight": { impact: 50, fresh: { charge: 35, grip: -25, valence: -20 }, worn: { charge: 18, grip: -45, valence: -35 }, wear_policy: "memory-occasions" },
    "conversation": { impact: 20, fresh: { charge: 0, grip: 0, valence: 0 }, worn: { charge: 0, grip: 0, valence: 0 }, wear_policy: "none" },
    "ate": { impact: 10, fresh: { charge: 4, grip: 2, valence: 1 }, worn: { charge: 2, grip: -1, valence: 0 }, wear_policy: "memory-occasions" },
    "used-toilet": { impact: 4, fresh: { charge: 1, grip: 2, valence: 0 }, worn: { charge: 0, grip: 0, valence: 0 }, wear_policy: "memory-occasions" },
    "slept": { impact: 25, fresh: { charge: 15, grip: 2, valence: 2 }, worn: { charge: 12, grip: -2, valence: 0 }, wear_policy: "memory-occasions" },
    "need-satisfied": { impact: 8, fresh: { charge: 2, grip: 2, valence: 0 }, worn: { charge: 1, grip: 0, valence: 0 }, wear_policy: "memory-occasions" },
    "finished-mining": { impact: 15, fresh: { charge: -1, grip: 5, valence: 0 }, worn: { charge: -5, grip: -3, valence: 0 }, wear_policy: "memory-occasions", stimulus: { salience: 25, lifetime: 10, contributions: { work: 15 } } },
    "cleared-rock": { impact: 15, fresh: { charge: -1, grip: 5, valence: 0 }, worn: { charge: -5, grip: -3, valence: 0 }, wear_policy: "memory-occasions", stimulus: { salience: 25, lifetime: 10, contributions: { work: 15 } } },
    "finished-construction": { impact: 18, fresh: { charge: -1, grip: 6, valence: 3 }, worn: { charge: -4, grip: 0, valence: 0 }, wear_policy: "memory-occasions", stimulus: { salience: 25, lifetime: 10, contributions: { work: 15 } } },
    "cleaned-refuse": { impact: 14, fresh: { charge: -1, grip: 5, valence: 0 }, worn: { charge: -5, grip: -4, valence: 0 }, wear_policy: "memory-occasions", stimulus: { salience: 25, lifetime: 10, contributions: { work: 15 } } },
    "incinerated-refuse": { impact: 16, fresh: { charge: -1, grip: 7, valence: 1 }, worn: { charge: -4, grip: 0, valence: 0 }, wear_policy: "memory-occasions", stimulus: { salience: 25, lifetime: 10, contributions: { work: 15 } } },
    "mutated": { impact: 60, fresh: { charge: 18, grip: -70, valence: -35 }, worn: { charge: 10, grip: -90, valence: -70 }, wear_policy: "memory-occasions" },
    "witnessed-mutation": { impact: 35, fresh: { charge: 2, grip: -8, valence: -10 }, worn: { charge: 1, grip: -16, valence: -18 }, wear_policy: "memory-occasions" },
    "ate-gruel": { impact: 10, fresh: { charge: 0, grip: 0, valence: -1 }, worn: { charge: -1, grip: -2, valence: -1 }, wear_policy: "memory-occasions" },
    "cooked": { impact: 15, fresh: { charge: -1, grip: 5, valence: 1 }, worn: { charge: -5, grip: -2, valence: 0 }, wear_policy: "memory-occasions" },
    "scraped-scum": { impact: 12, fresh: { charge: -1, grip: 4, valence: 0 }, worn: { charge: -5, grip: -3, valence: 0 }, wear_policy: "memory-occasions" },
    "fed-scumhouse": { impact: 10, fresh: { charge: -1, grip: 4, valence: 0 }, worn: { charge: -4, grip: -2, valence: 0 }, wear_policy: "memory-occasions" },
    "went-to-market": { impact: 8, fresh: { charge: 0, grip: 3, valence: 0 }, worn: { charge: -2, grip: 0, valence: 0 }, wear_policy: "memory-occasions" },
    "hauled": { impact: 8, fresh: { charge: 0, grip: 2, valence: 0 }, worn: { charge: -2, grip: 0, valence: 0 }, wear_policy: "memory-occasions" },
    "bought-meal": { impact: 12, fresh: { charge: 2, grip: 3, valence: 0 }, worn: { charge: 1, grip: 0, valence: 0 }, wear_policy: "memory-occasions" }
  }
};

const DEFAULT_VOCABULARY = {
  nouns: ["colonist", "alien", "cat", "rat", "gore", "meal", "toilet", "bed", "need", "rock", "structure", "refuse", "gruel", "scum", "scumhouse", "goods"],
  actions: ["present", "bite", "attack", "kill", "crush", "catch", "wound", "converse", "eat", "use", "sleep", "satisfy", "mine", "clear", "construct", "clean", "incinerate", "mutate", "cook", "scrape", "deliver", "trade", "buy", "haul"],
  channels: ["direct", "sight", "hearing", "proximity"],
  roles: ["actor", "target", "witness"],
  phases: ["instant", "enter", "ongoing", "exit"],
  cadences: ["instant", "enter", "enter-and-ongoing"],
  radii: ["flee-radius", "stomp-radius", "gore-sight-radius"],
  traits: ["big-eater", "light-eater", "industrious", "lazy", "asocial", "introvert", "extrovert", "tidy", "mutant", "mutant-lover", "resilient", "cowardly", "optimist", "pessimist"],
  object_relations: ["friend"],
  wear_policies: ["memory-occasions", "none"],
  focuses: ["idle", "work", "eat", "relieve", "socialize", "sleep", "flee", "fight", "escape"]
};

const DEFAULT_MATCHES = {
  "saw-alien":                  { actor_noun:"alien", action:"present", channel:"sight", role:"witness", phase:"enter" },
  "saw-rat":                    { actor_noun:"rat", action:"present", channel:"sight", role:"witness", phase:"enter" },
  "saw-gore":                   { actor_noun:"gore", action:"present", channel:"sight", role:"witness", phase:"enter" },
  "bitten":                     { actor_noun:"alien", action:"bite", object_noun:"colonist", channel:"direct", role:"target", phase:"instant" },
  "witnessed-colonist-killed":  { actor_noun:"alien", action:"kill", object_noun:"colonist", channel:"sight", role:"witness", phase:"instant" },
  "witnessed-colonist-attacked":{ actor_noun:"alien", action:"bite", object_noun:"colonist", channel:"sight", role:"witness", phase:"instant" },
  "crushed-rat":                { actor_noun:"colonist", action:"crush", object_noun:"rat", channel:"direct", role:"actor", phase:"instant" },
  "witnessed-rat-crushed":      { actor_noun:"colonist", action:"crush", object_noun:"rat", channel:"sight", role:"witness", phase:"instant" },
  "witnessed-cat-catch":        { actor_noun:"cat", action:"catch", object_noun:"rat", channel:"sight", role:"witness", phase:"instant" },
  "killed-alien":               { actor_noun:"colonist", action:"kill", object_noun:"alien", channel:"direct", role:"actor", phase:"instant" },
  "witnessed-alien-killed":     { actor_noun:"colonist", action:"kill", object_noun:"alien", channel:"sight", role:"witness", phase:"instant" },
  "wounded-alien":              { actor_noun:"colonist", action:"wound", object_noun:"alien", channel:"direct", role:"actor", phase:"instant" },
  "witnessed-gunfight":         { actor_noun:"colonist", action:"wound", object_noun:"alien", channel:"sight", role:"witness", phase:"instant" },
  "conversation":               { actor_noun:"colonist", action:"converse", object_noun:"colonist", channel:"direct", role:"", phase:"instant" },
  "ate":                        { actor_noun:"colonist", action:"eat", object_noun:"meal", channel:"direct", role:"actor", phase:"instant" },
  "used-toilet":                { actor_noun:"colonist", action:"use", object_noun:"toilet", channel:"direct", role:"actor", phase:"instant" },
  "slept":                      { actor_noun:"colonist", action:"sleep", object_noun:"bed", channel:"direct", role:"actor", phase:"instant" },
  "need-satisfied":             { actor_noun:"colonist", action:"satisfy", object_noun:"need", channel:"direct", role:"actor", phase:"instant" },
  "finished-mining":            { actor_noun:"colonist", action:"mine", object_noun:"rock", channel:"direct", role:"actor", phase:"instant" },
  "cleared-rock":               { actor_noun:"colonist", action:"clear", object_noun:"rock", channel:"direct", role:"actor", phase:"instant" },
  "finished-construction":      { actor_noun:"colonist", action:"construct", object_noun:"structure", channel:"direct", role:"actor", phase:"instant" },
  "cleaned-refuse":             { actor_noun:"colonist", action:"clean", object_noun:"refuse", channel:"direct", role:"actor", phase:"instant" },
  "incinerated-refuse":         { actor_noun:"colonist", action:"incinerate", object_noun:"refuse", channel:"direct", role:"actor", phase:"instant" },
  "mutated":                    { actor_noun:"colonist", action:"mutate", channel:"direct", role:"actor", phase:"instant" },
  "witnessed-mutation":         { actor_noun:"colonist", action:"mutate", channel:"sight", role:"witness", phase:"instant" },
  "ate-gruel":                  { actor_noun:"colonist", action:"eat", object_noun:"gruel", channel:"direct", role:"actor", phase:"instant" },
  "cooked":                     { actor_noun:"colonist", action:"cook", object_noun:"meal", channel:"direct", role:"actor", phase:"instant" },
  "scraped-scum":               { actor_noun:"colonist", action:"scrape", object_noun:"scum", channel:"direct", role:"actor", phase:"instant" },
  "fed-scumhouse":              { actor_noun:"colonist", action:"deliver", object_noun:"scumhouse", channel:"direct", role:"actor", phase:"instant" },
  "went-to-market":             { actor_noun:"colonist", action:"trade", object_noun:"goods", channel:"direct", role:"actor", phase:"instant" },
  "hauled":                     { actor_noun:"colonist", action:"haul", object_noun:"goods", channel:"direct", role:"actor", phase:"instant" },
  "bought-meal":                { actor_noun:"colonist", action:"buy", object_noun:"meal", channel:"direct", role:"actor", phase:"instant" }
};

const DEFAULT_PERCEPTIONS = [
  { id:"direct-actor", match:{}, sense:{ channel:"direct", role:"actor", cadence:"instant" } },
  { id:"direct-target", match:{}, sense:{ channel:"direct", role:"target", cadence:"instant" } },
  { id:"visible-alien", match:{ actor_noun:"alien", action:"present" }, sense:{ channel:"sight", role:"witness", radius:"flee-radius", cadence:"enter-and-ongoing" } },
  { id:"visible-rat", match:{ actor_noun:"rat", action:"present" }, sense:{ channel:"sight", role:"witness", radius:"stomp-radius", cadence:"enter" } },
  { id:"visible-gore", match:{ actor_noun:"gore", action:"present" }, sense:{ channel:"sight", role:"witness", radius:"gore-sight-radius", cadence:"enter" } },
  { id:"witness-alien-kill", match:{ actor_noun:"alien", action:"kill", object_noun:"colonist" }, sense:{ channel:"sight", role:"witness", radius:"flee-radius", cadence:"instant" } },
  { id:"witness-alien-attack", match:{ actor_noun:"alien", action:"bite", object_noun:"colonist" }, sense:{ channel:"sight", role:"witness", radius:"flee-radius", cadence:"instant" } },
  { id:"witness-rat-crushed", match:{ actor_noun:"colonist", action:"crush", object_noun:"rat" }, sense:{ channel:"sight", role:"witness", radius:"stomp-radius", cadence:"instant" } },
  { id:"witness-cat-catch", match:{ actor_noun:"cat", action:"catch", object_noun:"rat" }, sense:{ channel:"sight", role:"witness", radius:"stomp-radius", cadence:"instant" } },
  { id:"witness-alien-killed", match:{ actor_noun:"colonist", action:"kill", object_noun:"alien" }, sense:{ channel:"sight", role:"witness", radius:"flee-radius", cadence:"instant" } },
  { id:"witness-gunfight", match:{ actor_noun:"colonist", action:"wound", object_noun:"alien" }, sense:{ channel:"sight", role:"witness", radius:"flee-radius", cadence:"instant" } },
  { id:"witness-mutation", match:{ actor_noun:"colonist", action:"mutate" }, sense:{ channel:"sight", role:"witness", radius:"flee-radius", cadence:"instant" } }
];

const COLLAPSE_TEXT = {
  "finished-mining":"Finished mining.", "cleared-rock":"Cleared rock for a room.",
  "finished-construction":"Finished construction.", "cleaned-refuse":"Cleaned up refuse.",
  "incinerated-refuse":"Burned refuse in the incinerator.", "ate":"Had a meal.",
  "used-toilet":"Used the toilet.", "slept":"Slept in a bed.", "need-satisfied":"Satisfied a need.",
  "ate-gruel":"Ate nutrient-pod gruel.", "cooked":"Worked the scumhouse.", "scraped-scum":"Scraped cave scum.",
  "fed-scumhouse":"Fed the scumhouse.", "went-to-market":"Went to market.", "hauled":"Hauled goods for hire."
};
const ACTOR_SOURCED_STIMULI = new Set([
  "saw-alien", "bitten", "witnessed-colonist-killed", "witnessed-colonist-attacked"
]);

for (const [id, reaction] of Object.entries(DEFAULT_CONFIG.reactions)) {
  reaction.match = DEFAULT_MATCHES[id] || {};
  reaction.memory = { record:true };
  if (COLLAPSE_TEXT[id]) reaction.memory.collapse = COLLAPSE_TEXT[id];
  if (reaction.stimulus) reaction.stimulus.source = ACTOR_SOURCED_STIMULI.has(id) ? "actor" : "none";
}
DEFAULT_CONFIG.schema_version = 1;
DEFAULT_CONFIG.vocabulary = JSON.parse(JSON.stringify(DEFAULT_VOCABULARY));
DEFAULT_CONFIG.perceptions = JSON.parse(JSON.stringify(DEFAULT_PERCEPTIONS));
DEFAULT_CONFIG.trait_rules = [
  { id:"tidy-saw-gore", trait:"tidy", match:{ actor_noun:"gore", action:"present", channel:"sight", role:"witness", phase:"enter" }, scales:{ impact:100, charge:220, grip:220, valence:220, wear_rate:100 } },
  { id:"tidy-witnessed-colonist-killed", trait:"tidy", match:{ actor_noun:"alien", action:"kill", object_noun:"colonist", channel:"sight", role:"witness", phase:"instant" }, scales:{ impact:100, charge:220, grip:220, valence:220, wear_rate:100 } },
  { id:"tidy-witnessed-rat-crushed", trait:"tidy", match:{ actor_noun:"colonist", action:"crush", object_noun:"rat", channel:"sight", role:"witness", phase:"instant" }, scales:{ impact:100, charge:220, grip:220, valence:220, wear_rate:100 } },
  { id:"tidy-cleaned-refuse", trait:"tidy", match:{ actor_noun:"colonist", action:"clean", object_noun:"refuse", channel:"direct", role:"actor", phase:"instant" }, scales:{ impact:100, charge:220, grip:220, valence:220, wear_rate:100 } },
  { id:"tidy-incinerated-refuse", trait:"tidy", match:{ actor_noun:"colonist", action:"incinerate", object_noun:"refuse", channel:"direct", role:"actor", phase:"instant" }, scales:{ impact:100, charge:100, grip:200, valence:100, wear_rate:100 } },
  { id:"industrious-mining", trait:"industrious", match:{ actor_noun:"colonist", action:"mine", object_noun:"rock", channel:"direct", role:"actor", phase:"instant" }, scales:{ impact:100, charge:200, grip:200, valence:200, wear_rate:100 } },
  { id:"industrious-clearing", trait:"industrious", match:{ actor_noun:"colonist", action:"clear", object_noun:"rock", channel:"direct", role:"actor", phase:"instant" }, scales:{ impact:100, charge:200, grip:200, valence:200, wear_rate:100 } },
  { id:"industrious-construction", trait:"industrious", match:{ actor_noun:"colonist", action:"construct", object_noun:"structure", channel:"direct", role:"actor", phase:"instant" }, scales:{ impact:100, charge:200, grip:200, valence:200, wear_rate:100 } },
  { id:"industrious-cleaning", trait:"industrious", match:{ actor_noun:"colonist", action:"clean", object_noun:"refuse", channel:"direct", role:"actor", phase:"instant" }, scales:{ impact:100, charge:200, grip:200, valence:200, wear_rate:100 } },
  { id:"industrious-incinerating", trait:"industrious", match:{ actor_noun:"colonist", action:"incinerate", object_noun:"refuse", channel:"direct", role:"actor", phase:"instant" }, scales:{ impact:100, charge:200, grip:200, valence:200, wear_rate:100 } },
  { id:"introvert-conversation", trait:"introvert", match:{ actor_noun:"colonist", action:"converse", object_noun:"colonist", channel:"direct", phase:"instant" }, scales:{ impact:100, charge:-100, grip:100, valence:100, wear_rate:100 } },
  { id:"mutant-lover-mutated", trait:"mutant-lover", match:{ actor_noun:"colonist", action:"mutate", channel:"direct", role:"actor", phase:"instant" }, scales:{ impact:100, charge:100, grip:-100, valence:-100, wear_rate:100 } },
  { id:"mutant-lover-witnessed-mutation", trait:"mutant-lover", match:{ actor_noun:"colonist", action:"mutate", channel:"sight", role:"witness", phase:"instant" }, scales:{ impact:100, charge:100, grip:-100, valence:-100, wear_rate:100 } },
  { id:"extrovert-friend-killed", trait:"extrovert", match:{ actor_noun:"alien", action:"kill", object_noun:"colonist", channel:"sight", role:"witness", phase:"instant", object_relation:"friend" }, scales:{ impact:100, charge:130, grip:100, valence:150, wear_rate:100 } },
  { id:"extrovert-friend-attacked", trait:"extrovert", match:{ actor_noun:"alien", action:"bite", object_noun:"colonist", channel:"sight", role:"witness", phase:"instant", object_relation:"friend" }, scales:{ impact:100, charge:130, grip:100, valence:150, wear_rate:100 } },
  { id:"resilient-global-wear", trait:"resilient", match:{}, scales:{ impact:100, charge:100, grip:100, valence:100, wear_rate:40 } },
  { id:"cowardly-global-wear", trait:"cowardly", match:{}, scales:{ impact:100, charge:100, grip:100, valence:100, wear_rate:180 } },
  { id:"cowardly-visible-alien", trait:"cowardly", match:{ actor_noun:"alien", action:"present", channel:"sight", role:"witness", phase:"enter" }, scales:{ impact:150, charge:100, grip:100, valence:100, wear_rate:100 } },
  { id:"cowardly-bitten", trait:"cowardly", match:{ actor_noun:"alien", action:"bite", object_noun:"colonist", channel:"direct", role:"target", phase:"instant" }, scales:{ impact:150, charge:100, grip:100, valence:100, wear_rate:100 } },
  { id:"cowardly-witnessed-gunfight", trait:"cowardly", match:{ actor_noun:"colonist", action:"wound", object_noun:"alien", channel:"sight", role:"witness", phase:"instant" }, scales:{ impact:150, charge:100, grip:100, valence:100, wear_rate:100 } }
];
const TRAIT_HOMES = {
  optimist: { charge: 0, grip: 8, valence: 25 },
  pessimist: { charge: 0, grip: -8, valence: -25 }
};


export {
  DEFAULT_CONFIG,
  DEFAULT_VOCABULARY,
  DEFAULT_PERCEPTIONS,
  TRAIT_HOMES,
};
