// The inspector's topic payloads (internal/wire/inspect.go: EntityTopic and
// TileTopic), as the page reads them.

export interface PartHP { name: string; hp: number; max: number }
export interface Stack { slot: number; item: string; count: number }

export interface Colonist {
  pronouns: string; orientation: string; age: number;
  height: string; heightCm: number; weightKg: number; skin: string; hair: string;
  wallet: number;
  mood: { charge: number; grip: number; valence: number; label: string; max: number };
  /** consequence is what a full bar does: "none", "death", "loneliness", "passing out" or "soiling" (sim.Consequence). */
  drives: { name: string; value: number; max: number; consequence: string }[];
  inventory: Stack[];
  slots: number;
  traits: { name: string; desc: string }[];
  /** Skills it has a rank in, in skill order (docs/skills.md). */
  skills: { name: string; label: string; rank: number; maxRank: number; practice: number }[];
  /** The skill it's known for, and its title in it; absent until it has one. */
  profession?: string; professionLabel?: string;
  family: { relation: string; id: number; name: string }[];
  affinities: { id: number; name: string; value: number }[];
  affinityMax: number;
  /** Newest first. count > 1 is a run of the same event, tick to lastTick. */
  memories: { tick: number; lastTick: number; count: number; text: string }[];
}

export interface EntityInfo {
  /** False once the creature is gone with no record left (a rat aged out of the graveyard). */
  found: boolean;
  id: number; kind: string; glyph: string; name: string;
  /** The glyph in the colonist's own skin and hair: pick with pickGlyph. */
  look?: string[];
  x: number; y: number; state: string; focus: string;
  hp: number; maxHp: number; parts: PartHP[];
  dead: boolean; diedTick?: number; cause?: string;
  species?: string;
  colonist?: Colonist;
}

export interface TileInfo {
  x: number; y: number;
  /** False under the fog: nothing else is set. */
  explored: boolean;
  terrain?: string; glyph?: string;
  fixture?: { owner: string; ownerId?: number; access: string; price?: number };
  storage?: {
    label: string; used: number; slots: number; items: number; capacity: number;
    contents: Stack[];
    ledger: { owner: string; item: string; count: number }[];
  };
  creatures: { id: number; glyph: string; look?: string[]; name: string; state: string }[];
}
