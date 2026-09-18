package sim

import "fmt"

// ---- Uranium exposure and mutation -----------------------------------------
//
// Uranium is the one rock composition that acts back on the colonist who digs
// it. A colonist standing next to an unexcavated uranium deposit, or carrying
// uranium ore in their pack, is *exposed*: every exposed tick adds to a
// cumulative dose, and every UraniumExposureTicks of dose is one roll at
// MutationChance to grow an extra body part and pick up the Mutant trait. The
// defaults are tuned so that lands on about 1% of colonists in a typical run.
//
// Everything here runs on the simulation RNG (w.rng), not the personality
// stream: a mutation changes a colonist's body and how combat resolves against
// it, so it is gameplay, not flavor (see AGENTS.md). See docs/mutation.md.

// mutantParts lists the parts a mutation can grow, in a fixed order so the
// draw is deterministic. Adding one is a table edit here plus its enum entry,
// name, and hit weight in entity.go.
var mutantParts = [...]BodyPart{ThirdArm, ExtraEye, Tail, VestigialTwin}

// uraniumExposed reports whether a colonist is under a dose this tick: either
// carrying uranium ore, or standing within a step of unexcavated
// uranium-bearing rock. Both are bounded, allocation-free checks — eight
// inventory slots and eight neighbors — because this runs for every colonist,
// every tick.
//
// Carrying the ore is the dominant path in practice: a miner picks a lump up
// and stays dosed until a full load sends it to a chest (jobStore). Proximity
// matters anyway, so that a colonist digging alongside a vein they never manage
// to break into is still taking a dose.
func (w *World) uraniumExposed(e *Entity) bool {
	if e.Inventory.Has(UraniumOre) {
		return true
	}
	for _, d := range neighbors8 {
		t := w.TileAt(e.Pos.Add(d.X, d.Y))
		if t.Terrain == Rock && t.Composition == UraniumBearingRock {
			return true
		}
	}
	return false
}

// applyUraniumExposure accumulates this tick's dose and rolls for a mutation
// each time the dose crosses UraniumExposureTicks. The counter resets on every
// roll rather than latching, so exposure that does not mutate a colonist is
// not wasted: keep working the vein and the rolls keep coming, and a colonist
// can mutate more than once over a long enough career.
//
// The dose never decays. "Prolonged exposure" is meant to be a record of how
// much uranium this colonist has handled in total, not a level they can cool
// off from by taking a shift away from the vein.
//
// That is also why UraniumExposureTicks, not MutationChance, is the knob that
// decides how many colonists end up mutants: rolls keep coming for as long as
// a colonist works uranium, so a low per-roll chance on its own barely changes
// the outcome. See "Tuning the rate" in docs/mutation.md.
func (w *World) applyUraniumExposure(e *Entity) {
	if w.cfg.UraniumExposureTicks < 1 || !w.uraniumExposed(e) {
		return
	}
	e.uraniumExposure++
	if e.uraniumExposure < w.cfg.UraniumExposureTicks {
		return
	}
	e.uraniumExposure = 0
	if w.cfg.MutationChance > 0 && w.rng.Intn(100) < w.cfg.MutationChance {
		w.mutate(e)
	}
}

// mutate grows one new body part on a colonist and marks them a Mutant. A
// colonist who already has every mutant part keeps the trait and simply has
// nothing left to grow.
func (w *World) mutate(e *Entity) {
	part, ok := w.rollMutantPart(e)
	if !ok {
		return
	}
	w.growPart(e, part)
	w.giveTrait(e, TraitMutant)

	named := withArticle(part.String()) // "a tail", "an extra eye"
	w.remember(e, event(EvtMutated, "The uranium changed them: grew %s.", named))
	w.log.add(fmt.Sprintf("%s has mutated — %s.", e.displayName(), named))
	for _, wit := range w.colonistsWithin(e.Pos, w.cfg.FleeRadius, e.ID) {
		w.remember(wit, event(EvtWitnessedMutation, "Watched %s grow %s.", e.displayName(), named))
	}
}

// withArticle prefixes a noun phrase with the right indefinite article. Body
// part names are a closed, plain-English set ("tail", "extra eye"), so the
// vowel rule is enough — this is not trying to be a general English article
// function.
func withArticle(noun string) string {
	if noun == "" {
		return noun
	}
	switch noun[0] {
	case 'a', 'e', 'i', 'o', 'u':
		return "an " + noun
	}
	return "a " + noun
}

// rollMutantPart picks uniformly among the mutant parts this colonist has not
// grown yet, reporting false once there are none left.
func (w *World) rollMutantPart(e *Entity) (BodyPart, bool) {
	var candidates []BodyPart
	for _, part := range mutantParts {
		if !e.hasPart(part) {
			candidates = append(candidates, part)
		}
	}
	if len(candidates) == 0 {
		return 0, false
	}
	return candidates[w.rng.Intn(len(candidates))], true
}

// growPart adds a body part to an entity at full health. Its pool is sized
// from the body the colonist already has, and adds to MaxHP rather than being
// carved out of it: a mutation is extra flesh, not redistributed flesh. The
// upshot is that a mutant is a little tougher and has one more place to be
// wounded, which also thins the odds that any single hit finds a vital part —
// the compensation for being treated as a mutant by everyone else.
func (w *World) growPart(e *Entity, part BodyPart) {
	if !e.hasParts() || e.hasPart(part) {
		return
	}
	hp := atLeast1(baseBodyHP(e) * bodyPartWeight[part] / 100)
	e.MaxParts[part] = hp
	e.Parts[part] = hp
	e.MaxHP += hp
	e.HP += hp
}

// baseBodyHP is what the entity's body was worth before any mutation, so a
// second mutation is sized like the first instead of compounding off a body
// that mutation itself inflated.
func baseBodyHP(e *Entity) int {
	total := 0
	for part := BodyPart(0); part < numBaseBodyParts; part++ {
		total += e.MaxParts[part]
	}
	return total
}

// giveTrait adds a trait a colonist acquired in play and re-resolves the
// effective parameters traits feed (need rise, rest, work speed), so an
// acquired trait behaves exactly like one rolled at spawn. Adding a trait a
// colonist already has is a no-op.
func (w *World) giveTrait(e *Entity, t Trait) {
	if e.Profile == nil || e.Profile.HasTrait(t) {
		return
	}
	e.Profile.Traits = append(e.Profile.Traits, t)
	w.resolveTraitEffects(e)
}

// mutantAffinityBonus is the extra affinity one colonist gains toward another
// per conversation because of what they are: a Mutant-Lover warms to a mutant
// far faster than to anyone else. It is directional — it is a fact about the
// admirer, not about the pair — which is why finishTalk applies affinity per
// direction rather than through addAffinity.
//
// The bonus rides on top of the ordinary talk step and is not subject to its
// half-of-AffinityMax saturation, so a mutant-lover's regard for a mutant can
// climb into the outer range that plain conversation alone can never reach.
func (w *World) mutantAffinityBonus(from, to *Entity) int {
	if from.Profile == nil || to.Profile == nil {
		return 0
	}
	if from.Profile.HasTrait(TraitMutantLover) && to.Profile.HasTrait(TraitMutant) {
		return w.cfg.MutantLoverAffinityBonus
	}
	return 0
}
