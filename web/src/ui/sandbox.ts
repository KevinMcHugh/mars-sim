// The Sandbox tab's species search (docs/sandbox.md). Kept out of
// SandboxPanel so node --test can run it.

/** The lore topic's species fields the search reads (internal/wire/topics.go). */
export interface SpeciesLine {
  label: string;
  singular: string;
  plural: string;
  scientificName: string;
  temperament: string;
}

/** The temperaments a species can have, mildest first (sim.Temperament). */
export const TEMPERAMENTS = ['friendly', 'cautious', 'hostile'] as const;

/**
 * The roster indexes of the species matching a query, in roster order. Every
 * word of the query must appear, case-insensitively, somewhere in a species'
 * label, names, binomial or temperament, so "hostile sting" finds a hostile
 * stingtail. An empty query matches every species. With any temperaments
 * given, a species must also have one of them (the tab's filter chips).
 */
export function matchSpecies(species: readonly SpeciesLine[], query: string, temperaments: readonly string[] = []): number[] {
  const words = query.toLowerCase().split(/\s+/).filter((w) => w !== '');
  const out: number[] = [];
  species.forEach((s, i) => {
    if (temperaments.length > 0 && !temperaments.includes(s.temperament)) return;
    const text = [s.label, s.singular, s.plural, s.scientificName, s.temperament].join(' ').toLowerCase();
    if (words.every((w) => text.includes(w))) out.push(i);
  });
  return out;
}
