package sim

import (
	"fmt"
	"strings"
)

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

// mutate changes a colonist's body in the two ways uranium can change it —
// growing a part nobody is born with, and resizing them — and marks them a
// Mutant. Both are attempted on every mutation, and each can decline: a
// colonist who has every mutant part has nothing left to grow, and one pinned
// at a stature limit cannot go further that way. A mutation that finds nothing
// at all to change is not a mutation: no trait, no memory, no log line.
func (w *World) mutate(e *Entity) {
	var changes []string
	if part, ok := w.rollMutantPart(e); ok {
		w.growPart(e, part)
		changes = append(changes, "grew "+withArticle(part.String())) // "grew a tail"
	}
	if change, ok := w.resize(e); ok {
		changes = append(changes, change)
	}
	if len(changes) == 0 {
		return
	}
	w.giveTrait(e, TraitMutant)

	what := joinAnd(changes)
	w.remember(e, event(EvtMutated, "The uranium changed them: %s.", what))
	w.log.add(fmt.Sprintf("%s has mutated — %s.", e.displayName(), what))
	for _, wit := range w.colonistsWithin(e.Pos, w.cfg.FleeRadius, e.ID) {
		w.remember(wit, event(EvtWitnessedMutation, "Watched %s mutate: %s.", e.displayName(), what))
	}
}

// resize grows or shrinks a mutating colonist by MutationStaturePercent of the
// height they are now, and reports the change as a past-tense phrase for the
// memory and log lines. It returns false when there is nothing to do: an
// entity with no profile (nothing has a height but a colonist), resizing
// switched off, or a step too small to move a rounded centimetre.
//
// The step is a fixed percentage and only the *direction* is drawn, which
// makes the walk legible: a colonist's height is their starting height times
// 1.15 raised to (lucky doses minus unlucky ones). The extremes are therefore
// a run of one-sided luck rather than a single jackpot roll, which is what
// makes the ten-foot colonist in the dormitory a story instead of a number.
//
// Direction is drawn from the simulation RNG (w.rng), not the personality
// stream: stature scales the body, and the body is combat (see scaleBody).
func (w *World) resize(e *Entity) (string, bool) {
	if e.Profile == nil || w.cfg.MutationStaturePercent <= 0 {
		return "", false
	}
	before := e.Profile.HeightCM
	after := w.rollStature(before)
	if after == before {
		return "", false
	}
	w.setStature(e, after)

	verb := "shrank"
	if after > before {
		verb = "stretched"
	}
	return fmt.Sprintf("%s from %s to %s", verb, FormatHeight(before), FormatHeight(after)), true
}

// rollStature picks the height a mutation moves a colonist to. Both directions
// are one MutationStaturePercent step from where they are now, held inside the
// configured limits; a direction that no longer moves the colonist at all is
// not a candidate, so one already pinned at a limit always goes the other way
// instead of wasting the mutation. With both ends open it is an even coin flip
// — uranium has no opinion about which way a body should go.
//
// Clamping each candidate (rather than refusing a step that would overshoot)
// is also what walks a colonist back into a range they start outside of, which
// a colony configured with a stature range narrower than ordinary human height
// generates on purpose.
func (w *World) rollStature(cm int) int {
	lo, hi := w.statureLimits()
	var candidates []int
	if grown := clampInt(w.grownStature(cm), lo, hi); grown > cm {
		candidates = append(candidates, grown)
	}
	if shrunk := clampInt(w.shrunkStature(cm), lo, hi); shrunk < cm {
		candidates = append(candidates, shrunk)
	}
	if len(candidates) == 0 {
		return cm
	}
	return candidates[w.rng.Intn(len(candidates))]
}

// grownStature and shrunkStature are the two ends of one mutation's step, and
// are each other's inverse: growing multiplies by (100+pct)/100 and shrinking
// divides by the same ratio, rather than subtracting the percentage. Taking
// pct off and putting pct back on would not land where it started — 0.85 ×
// 1.15 is 0.98 — and that missing two percent, compounded over a career of
// doses, is a steady downward drift in a walk that is supposed to be a fair
// coin flip.
func (w *World) grownStature(cm int) int {
	return scaleRound(cm, 100+w.cfg.MutationStaturePercent, 100)
}

func (w *World) shrunkStature(cm int) int {
	return scaleRound(cm, 100, 100+w.cfg.MutationStaturePercent)
}

// setStature moves a colonist to a new height and brings the rest of the body
// along with it: weight at their own unchanged build, and HP scaled so a
// ten-foot colonist is genuinely harder to put down than a two-foot one.
func (w *World) setStature(e *Entity, cm int) {
	before := e.Profile.HeightCM
	if before <= 0 || cm == before {
		return
	}
	e.Profile.HeightCM = cm
	// Weight follows the square of the height rather than the cube, which
	// keeps the colonist's BMI — the build rollBody gave them — exactly as it
	// was. A true volume scaling would be the physical answer for a statue
	// scaled up, but these are people: the two-foot one should read as a small
	// person, not as something that could blow away.
	//
	// It is computed from the body they were born with, never from the weight
	// they are now, so a colonist stretched and shrunk back over a career lands
	// on the weight they started at instead of on whatever a long chain of
	// roundings left behind.
	bornH, bornKG := e.Profile.BornHeightCM, e.Profile.BornWeightKG
	if bornH <= 0 || bornKG <= 0 { // a profile built without a birth body
		bornH, bornKG = before, e.Profile.WeightKG
	}
	e.Profile.WeightKG = atLeast1(scaleRound(bornKG*cm, cm, bornH*bornH))
	scaleBody(e, cm, before)
}

// scaleBody scales an entity's HP pool, and every part of it, by num/den —
// the ratio its height just changed by. HP tracks height linearly rather than
// mass: mass grows as the square here, and a colonist who came out of the
// uranium four times as hard to kill would end the alien problem by standing
// in the wrong tunnel often enough.
//
// Current values scale alongside the maxima, so a resize neither heals a wound
// nor opens one: a colonist half dead before is half dead after. A part that
// is merely small never rounds away to nothing — a shrunken part is still a
// part the colonist has, and MaxParts at zero is the test for an anatomy that
// never had it (see Entity.hasPart) — but a destroyed part (current zero)
// stays destroyed.
func scaleBody(e *Entity, num, den int) {
	if den <= 0 || num <= 0 || num == den {
		return
	}
	e.MaxHP = atLeast1(scaleRound(e.MaxHP, num, den))
	e.HP = min(scaleRound(e.HP, num, den), e.MaxHP)
	for part := BodyPart(0); part < numBodyParts; part++ {
		if e.MaxParts[part] <= 0 {
			continue
		}
		e.MaxParts[part] = atLeast1(scaleRound(e.MaxParts[part], num, den))
		e.Parts[part] = min(scaleRound(e.Parts[part], num, den), e.MaxParts[part])
	}
}

// statureLimits is the configured height range, with the ordering defended so
// a bad pair of limits pins colonists at one height instead of inverting the
// clamp.
func (w *World) statureLimits() (lo, hi int) {
	lo, hi = w.cfg.StatureMinCM, w.cfg.StatureMaxCM
	if lo < 1 {
		lo = 1
	}
	if hi < lo {
		hi = lo
	}
	return lo, hi
}

// scaleRound multiplies v by num/den, rounding to nearest instead of
// truncating — over a career of resizes, truncation alone would walk a
// colonist steadily downward.
func scaleRound(v, num, den int) int {
	if den == 0 {
		return v
	}
	return (v*num*2 + den) / (den * 2)
}

// FormatHeight renders a height in centimetres the way the colony talks about
// it: feet and inches, which is the only unit in which "ten foot tall" is a
// thing to say. The rounding happens in inches, before the split into feet, so
// a height just shy of six feet reads 6'0" rather than 5'12".
func FormatHeight(cm int) string {
	inches := scaleRound(cm, 100, 254)
	return fmt.Sprintf("%d'%d\"", inches/12, inches%12)
}

// joinAnd renders a short list of phrases as plain English. Mutation produces
// at most two, so this does not try to be a general Oxford-comma joiner.
func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
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
