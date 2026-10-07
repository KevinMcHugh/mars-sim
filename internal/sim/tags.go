package sim

import "strings"

// Tags are what a species is, as far as other creatures' behaviors care:
// what a hunter looks for and what a prey animal runs from. Behaviors match
// on tags rather than on Kind, so "cats ignore chickens" is not a rule the
// code has to remember but simply the absence of a hunt that matches a
// chicken, and a new species fits into the food web by declaring what it is.
// See docs/species-and-behaviors.md.
type Tags uint32

const (
	// TagColonist is a person of the colony.
	TagColonist Tags = 1 << iota
	// TagAlien is any alien, in any form of its life.
	TagAlien
	// TagVermin is what a mouser hunts and a Hostile alien eats besides
	// colonists: rats.
	TagVermin
	// TagMouser is what vermin run from: cats. (Aliens eat rats too, but a
	// rat does not know to fear one.)
	TagMouser
	// TagPet is a creature a colonist can bring down in its ship: cats and
	// chickens.
	TagPet

	numTags = iota
)

var tagNames = [numTags]string{"colonist", "alien", "vermin", "mouser", "pet"}

// Has reports whether t shares any tag with o.
func (t Tags) Has(o Tags) bool { return t&o != 0 }

// String lists the tags, "vermin|pet", for logs and test failures.
func (t Tags) String() string {
	var names []string
	for i := 0; i < numTags; i++ {
		if t&(1<<i) != 0 {
			names = append(names, tagNames[i])
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, "|")
}

// tagsOf is what an entity is, by its species (in its current form).
func (w *World) tagsOf(e *Entity) Tags { return w.speciesOf(e).Tags }

// nearestTagged is the nearest living entity within range carrying any of
// tags, ties toward the lower ID (nearestMatch's chunk-ring scan).
func (w *World) nearestTagged(from Point, tags Tags, within int) (*Entity, bool) {
	return w.nearestMatch(from, within, func(e *Entity) bool { return w.tagsOf(e).Has(tags) })
}

// nearestTaggedWhere is the nearest living entity anywhere on the map
// carrying any of tags and passing keep (nil keeps all), ties toward the
// lower ID. Rather than nearestMatch's chunk rings, which an unbounded search
// would drive across every chunk on the map, it scans World.kindEntities for
// each kind whose species can carry the tags: creatures of a kind are few,
// so a direct scan is far cheaper. The distance-then-ID order makes the
// answer independent of both map order and the order kinds are scanned in.
func (w *World) nearestTaggedWhere(from Point, tags Tags, keep func(*Entity) bool) (*Entity, bool) {
	var best *Entity
	bestDist := 0
	for k := Kind(0); k < numKinds; k++ {
		if !w.species[k].Tags.Has(tags) {
			continue
		}
		for id := range w.kindEntities[k] {
			e := w.entities[id]
			if e == nil || !e.Alive() || !w.tagsOf(e).Has(tags) || (keep != nil && !keep(e)) {
				continue
			}
			d := from.Chebyshev(e.Pos)
			if best == nil || d < bestDist || (d == bestDist && e.ID < best.ID) {
				best, bestDist = e, d
			}
		}
	}
	return best, best != nil
}

// sameSpecies reports whether two entities are of one species: one kind,
// and for aliens one rolled species (every other kind has one species). A
// hunter never takes its own species, so a nest does not eat itself.
func sameSpecies(a, b *Entity) bool { return a.Kind == b.Kind && a.Species == b.Species }
