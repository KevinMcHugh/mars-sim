package sim

import (
	"math"
	"math/rand"
)

// Heredity is what family membership does to a newly generated colonist beyond
// putting them in the tree: they take the family's surname, they look like their
// closest relatives, and they already know them.
//
// It runs as a pass after kin assignment (see relationships.go) rather than
// inside personality generation, because none of it can be decided until the
// colonist's place in the tree is known — and the tree itself needs a finished
// profile to validate against (a parent must be old enough, a spouse must be a
// plausible match). So generation stays three ordered steps, in spawn():
//
//  1. assignPersonality — roll a whole, standalone person.
//  2. assignKin         — find them a family, using that person's age/gender.
//  3. inheritFamily     — rewrite the parts of them their family decides.
//
// Everything here draws from the personality RNG (World.prng), the same stream
// kin assignment uses, so heredity never perturbs the simulation stream.

// kinBond is what a derived family tie passes along: how much appearance
// carries across it, and how warmly the two colonists start out. Both are
// percentages — genes of cfg.AppearanceInheritChance, warmth of the
// cfg.FamilyAffinity baseline — so the config tunes the overall strength while
// this table keeps the relative shape (you look more like a sibling than a
// cousin's parent; you are closer to a spouse than to a nibling).
//
// A spouse passes on no genes. That is also what marks them as married in
// rather than born in, which is what keeps a household's two surnames straight.
type kinBond struct {
	genes  int
	warmth int
}

var kinBonds = [numRelationKinds]kinBond{
	RelSpouse:      {genes: 0, warmth: 100},
	RelParent:      {genes: 100, warmth: 90},
	RelChild:       {genes: 100, warmth: 90},
	RelSibling:     {genes: 100, warmth: 80},
	RelGrandparent: {genes: 50, warmth: 65},
	RelGrandchild:  {genes: 50, warmth: 65},
	RelAuntUncle:   {genes: 50, warmth: 55},
	RelNibling:     {genes: 50, warmth: 55},
}

// heightHeritability is how much of a relative's height z-score carries to the
// colonist inheriting it; the rest is fresh variation. The two are combined as
// z*h + noise*sqrt(1-h^2) so the population's height distribution keeps its
// shape — families vary less internally than the colony does overall, without
// the colony as a whole drifting toward the mean.
const heightHeritability = 0.6

// inheritFamily settles the parts of a new colonist's identity that only make
// sense once their family is known. Call it only for a colonist assignKin
// actually tied to someone.
func (w *World) inheritFamily(e *Entity) {
	if e.Profile == nil {
		return
	}
	rels := w.relativesOf(e, w.cachedKinChildren())
	if len(rels) == 0 {
		return
	}
	w.adoptSurname(e, rels)
	donors, genes := w.closestKin(rels)
	w.inheritAppearance(e.Profile, donors, genes)
	w.seedFamilyAffinity(e, rels)
}

// adoptSurname moves a new colonist onto their family's name.
//
// Every blood relative in the colony already shares the line's surname (each
// new member took it on the way in), so any of them would do — but taking it
// from the lowest entity ID, the first of the line to land on Mars, keeps the
// whole line converged on one name even in the one case where a colonist's
// relatives disagree: the children of a couple where one spouse kept their own
// name are blood relatives of both parents.
//
// A colonist whose only tie is a spouse has married into the colony rather than
// been born into it, and takes the spouse's name only SpouseSurnameChance of the
// time; otherwise they keep the name they were generated with and their children
// resolve the tie as above.
func (w *World) adoptSurname(e *Entity, rels []Relation) {
	var line *Entity
	var spouse *Entity
	for _, r := range rels {
		other := w.entities[r.Other]
		if other == nil || other.Profile == nil {
			continue
		}
		if kinBonds[r.Kind].genes == 0 { // married in, not descended
			spouse = other
			continue
		}
		if line == nil || other.ID < line.ID {
			line = other
		}
	}
	switch {
	case line != nil:
		w.takeSurname(e, line.Profile.surname)
	case spouse != nil && w.prng.Intn(100) < w.cfg.SpouseSurnameChance:
		w.takeSurname(e, spouse.Profile.surname)
	}
}

// takeSurname moves a colonist onto a family's name. Adopting a surname can
// collide with a relative who already has that given name, so the name is
// re-checked afterwards (see uniquifyName).
func (w *World) takeSurname(e *Entity, surname string) {
	e.Profile.setSurname(surname)
	w.uniquifyName(e)
}

// closestKin returns the relatives a new colonist takes after — the tier of
// blood relations with the most heredity behind it, so parents and siblings if
// there are any and the grandparent/cousin tier otherwise — along with that
// tier's genes percentage.
//
// It returns the whole tier rather than one relative on purpose: each feature
// then picks its own donor from it, so a child of two colonists can have one
// parent's hair and the other's skin instead of being a copy of whichever
// parent happened to be chosen first.
func (w *World) closestKin(rels []Relation) ([]*Entity, int) {
	best := 0
	for _, r := range rels {
		if g := kinBonds[r.Kind].genes; g > best {
			best = g
		}
	}
	if best == 0 {
		return nil, 0
	}
	var out []*Entity
	for _, r := range rels {
		if kinBonds[r.Kind].genes != best {
			continue
		}
		if other := w.entities[r.Other]; other != nil && other.Profile != nil {
			out = append(out, other)
		}
	}
	return out, best
}

// inheritAppearance replaces each heritable feature of a freshly rolled profile
// with one taken from a relative, independently and probabilistically, so a
// family resembles each other without anybody being a duplicate. Features left
// un-inherited keep the value the colonist was rolled with, which is why
// generation rolls a complete person first: a partial inheritance always has
// something plausible to fall back on.
func (w *World) inheritAppearance(p *Profile, donors []*Entity, genes int) {
	if len(donors) == 0 {
		return
	}
	chance := w.cfg.AppearanceInheritChance * genes / 100
	// donor draws a relative for one feature, reporting whether that feature is
	// inherited at all. Drawing per feature is what mixes two parents together.
	donor := func() (*Profile, bool) {
		d := donors[w.prng.Intn(len(donors))].Profile
		return d, w.prng.Intn(100) < chance
	}

	if d, ok := donor(); ok {
		p.SkinTone = inheritSkinTone(w.prng, d.SkinTone)
	}
	if d, ok := donor(); ok {
		p.hairBase = d.hairBase
		// Greying and balding are this colonist's own age showing, not something a
		// relative handed them, so an inherited color only replaces a natural one.
		if p.HairColor != HairWhite && p.HairColor != HairBald {
			p.HairColor = p.hairBase
		}
	}
	if d, ok := donor(); ok {
		setHeightZ(p, inheritHeightZ(w.prng, d.heightZ))
	}
}

// inheritSkinTone passes a relative's tone down as an ordinal nudge rather than
// a copy: usually the same point on the five-point scale, otherwise one step
// lighter or darker. A family ends up sharing a range instead of a single value,
// which is both truer and more legible in the roster than exact matches.
func inheritSkinTone(rng *rand.Rand, t SkinTone) SkinTone {
	step := 0
	switch rng.Intn(4) {
	case 0:
		step = -1
	case 1:
		step = 1
	}
	return SkinTone(clampInt(int(t)+step, int(SkinLight), int(SkinDark)))
}

// inheritHeightZ blends a relative's height z-score with fresh variation. See
// heightHeritability for why the two are weighted the way they are.
func inheritHeightZ(rng *rand.Rand, z float64) float64 {
	return z*heightHeritability + rng.NormFloat64()*math.Sqrt(1-heightHeritability*heightHeritability)
}

// seedFamilyAffinity starts a new colonist warm with everyone they are related
// to. Family arriving on the same ship have known each other for years, so
// beginning at a stranger's zero and having to talk their way up would be
// plainly wrong; the spread keeps it from being a flat number per relation kind,
// because not every cousin is equally close.
//
// This writes simulation-visible state from the personality stream, as family
// generation itself already does: the draws stay off World.rng, so a run with
// FamilyChance = 0 is unaffected.
func (w *World) seedFamilyAffinity(e *Entity, rels []Relation) {
	base := w.cfg.FamilyAffinity * w.cfg.AffinityMax / 100
	if base <= 0 {
		return
	}
	for _, r := range rels {
		if w.entities[r.Other] == nil {
			continue
		}
		v := base * kinBonds[r.Kind].warmth / 100
		if s := w.cfg.FamilyAffinitySpread; s > 0 {
			v += w.prng.Intn(2*s+1) - s
		}
		if v > 0 {
			w.addAffinity(e.ID, r.Other, v)
		}
	}
}
