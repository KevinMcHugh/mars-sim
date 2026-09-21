package sim

// MoodVector is a point in affect space. Charge describes activation, Grip
// felt control, and Valence how well the colonist's life is going.
//
// A vector is both a displacement and a destination: blendAffect adds it for a
// small event and moves to it for a large one, so one table row serves both.
type MoodVector struct {
	Charge  int
	Grip    int
	Valence int
}

// MoodKind identifies a display attractor. It is never behavioral state: focus
// scoring reads AffectState.Charge and AffectState.Grip directly.
type MoodKind uint8

const (
	MoodDriven MoodKind = iota
	MoodElated
	MoodGiddy
	MoodAdrift
	MoodListless
	MoodSpent
	MoodContent
	MoodComposed
	MoodSteady
	MoodSettling
)

// AffectState is the colonist's bounded mood. Focus scoring reads Charge and
// Grip; Valence is display-only, and deliberately so -- needs, HP and visible
// threats already feed scoring directly, so letting the axis that summarizes
// them back in would count them twice.
type AffectState struct {
	Charge  int
	Grip    int
	Valence int
	Label   MoodKind
}

type moodAttractor struct {
	Kind         MoodKind
	GoodName     string
	BadName      string
	Charge, Grip int
	Radius       int
}

// Declaration order is significant twice over: equal claims choose the first
// attractor, and moodName indexes this table by MoodKind (see
// TestMoodAttractorsIndexedByKind).
var moodAttractors = [...]moodAttractor{
	{MoodDriven, "driven", "furious", 70, 60, 46},
	{MoodElated, "elated", "frantic", 88, 0, 42},
	{MoodGiddy, "giddy", "panicked", 65, -62, 46},
	{MoodAdrift, "adrift", "anxious", 0, -85, 42},
	{MoodListless, "listless", "despairing", -62, -62, 46},
	{MoodSpent, "spent", "numb", -85, 0, 42},
	{MoodContent, "content", "grim", -60, 60, 46},
	{MoodComposed, "composed", "hardened", 0, 85, 42},
	{MoodSteady, "steady", "flat", 0, 0, 28},
}

func roundedDiv(n, d int) int {
	if d < 0 {
		n, d = -n, -d
	}
	if n < 0 {
		return -((-n + d/2) / d)
	}
	return (n + d/2) / d
}

// conversationMoodVector converts the existing signed, per-occurrence
// conversation outcome without retaining that scalar as mood state. Valence
// follows the outcome's sign at a quarter strength: how the people around you
// treat you is real evidence about how life is going, but colonists talk often
// enough that a fuller share of each chat would drown out everything else.
func conversationMoodVector(outcome int) MoodVector {
	switch {
	case outcome > 0:
		return MoodVector{Charge: max(1, roundedDiv(outcome*3, 7)), Grip: outcome, Valence: roundedDiv(outcome, 4)}
	case outcome < 0:
		return MoodVector{Charge: max(1, roundedDiv(-outcome*4, 6)), Grip: outcome, Valence: roundedDiv(outcome, 4)}
	default:
		return MoodVector{}
	}
}

// moodPull reports how much of an appraisal relocates rather than nudges, in
// hundredths. Integer throughout: appraisal has to be reproducible for a seed.
func (w *World) moodPull(impact int) int {
	if impact <= w.cfg.MoodPushImpact {
		return 0
	}
	if impact >= w.cfg.MoodPullImpact {
		return 100
	}
	return 100 * (impact - w.cfg.MoodPushImpact) / (w.cfg.MoodPullImpact - w.cfg.MoodPushImpact)
}

// blendAffect applies one appraisal. At pull 0 this is state + target, plain
// addition; at pull 100 it is target exactly, wherever the colonist had been.
// In between they keep a shrinking share of where they were, so a strong event
// overwrites a mood rather than being averaged into it.
func (w *World) blendAffect(e *Entity, target MoodVector, impact int) {
	keep := 100 - w.moodPull(impact)
	lim := w.cfg.MoodMax
	w.setAffect(e,
		clampInt(roundedDiv(e.affect.Charge*keep, 100)+target.Charge, -lim, lim),
		clampInt(roundedDiv(e.affect.Grip*keep, 100)+target.Grip, -lim, lim),
		clampInt(roundedDiv(e.affect.Valence*keep, 100)+target.Valence, -lim, lim))
}

// setAffect is the one place affect changes. Only charge and grip reach focus
// scoring, so a valence-only change updates what the roster says about a
// colonist without making them reconsider what they are doing.
func (w *World) setAffect(e *Entity, charge, grip, valence int) {
	moved := charge != e.affect.Charge || grip != e.affect.Grip
	if !moved && valence == e.affect.Valence {
		return
	}
	e.affect.Charge, e.affect.Grip, e.affect.Valence = charge, grip, valence
	if moved {
		w.refreshMoodAttractor(e)
		w.markMindDirty(e)
	}
}

func approach(value, home, amount int) int {
	if amount < 0 {
		amount = -amount
	}
	if value < home {
		return min(value+amount, home)
	}
	if value > home {
		return max(value-amount, home)
	}
	return value
}

func (w *World) decayAffect(e *Entity) {
	// Valence describes how life has been going over hours and days, not the
	// minutes charge and grip answer to, so it gives up a point every few turns
	// rather than points every turn. The period counts world ticks rather than
	// per-colonist turns so a seeded run stays reproducible.
	valence := e.affect.Valence
	if n := w.cfg.MoodValenceDecayTicks; n > 0 && w.tick%n == 0 {
		valence = approach(valence, 0, 1)
	}
	w.setAffect(e,
		approach(e.affect.Charge, 0, w.cfg.MoodChargeDecayPerTick),
		approach(e.affect.Grip, 0, w.cfg.MoodGripDecayPerTick),
		valence)
}

func attractorClaim(a moodAttractor, charge, grip int) int {
	dc, dg := charge-a.Charge, grip-a.Grip
	return 100 - 100*(dc*dc+dg*dg)/(a.Radius*a.Radius)
}

func bestMoodAttractor(charge, grip int) (MoodKind, int) {
	return bestMoodAttractorIn(moodAttractors[:], charge, grip)
}

func bestMoodAttractorIn(attractors []moodAttractor, charge, grip int) (MoodKind, int) {
	kind, claim := MoodSettling, 0
	found := false
	for _, a := range attractors {
		c := attractorClaim(a, charge, grip)
		if c < 0 || found && c <= claim {
			continue
		}
		kind, claim, found = a.Kind, c, true
	}
	return kind, claim
}

func moodClaim(kind MoodKind, charge, grip int) int {
	if kind == MoodSettling {
		return 0
	}
	for _, a := range moodAttractors {
		if a.Kind == kind {
			return attractorClaim(a, charge, grip)
		}
	}
	return 0
}

func (w *World) bestMoodAttractor(charge, grip int) (MoodKind, int) {
	kind, claim := MoodSettling, 0
	found := false
	for _, a := range w.cognition.Attractors {
		c := attractorClaim(moodAttractor{Kind: a.Kind, Radius: a.Radius, Charge: a.Charge, Grip: a.Grip}, charge, grip)
		if c < 0 || found && c <= claim {
			continue
		}
		kind, claim, found = a.Kind, c, true
	}
	return kind, claim
}

func (w *World) moodClaim(kind MoodKind, charge, grip int) int {
	if kind == MoodSettling {
		return 0
	}
	for _, a := range w.cognition.Attractors {
		if a.Kind == kind {
			return attractorClaim(moodAttractor{Kind: a.Kind, Radius: a.Radius, Charge: a.Charge, Grip: a.Grip}, charge, grip)
		}
	}
	return 0
}

func (w *World) refreshMoodAttractor(e *Entity) {
	challenger, challengerClaim := w.bestMoodAttractor(e.affect.Charge, e.affect.Grip)
	if challenger == e.affect.Label {
		return
	}
	incumbentClaim := w.moodClaim(e.affect.Label, e.affect.Charge, e.affect.Grip)
	if challengerClaim > incumbentClaim+w.cfg.MoodLabelSwitchMargin {
		e.affect.Label = challenger
	}
}

func moodName(kind MoodKind, bad bool) string {
	if int(kind) >= len(moodAttractors) {
		return "settling"
	}
	if bad {
		return moodAttractors[kind].BadName
	}
	return moodAttractors[kind].GoodName
}

// MoodName is the word the roster shows. The attractor names a shape of
// feeling and valence picks which of its two readings applies, so the same
// charge and grip read as "driven" or "furious" depending on how the
// colonist's life has actually been going -- not, as this once did, on
// whether they happen to be hungry or standing near an alien right now.
func (a AffectState) MoodName() string {
	return moodName(a.Label, a.Valence < 0)
}

func (w *World) affectName(a AffectState) string {
	if a.Label == MoodSettling {
		return "settling"
	}
	for _, attractor := range w.cognition.Attractors {
		if attractor.Kind != a.Label {
			continue
		}
		if a.Valence < 0 {
			return attractor.BadName
		}
		return attractor.GoodName
	}
	return "settling"
}

func (w *World) applyAffect(e *Entity, reaction *ReactionSpec, percept Percept) {
	target := reaction.Target
	if contextual, ok := percept.Occurrence.appraisalFor(e.ID); ok {
		target = contextual
	}
	impact := reaction.Impact
	target, impact = w.transformAppraisal(e, reaction, percept.Occurrence.Tags, target, impact)
	w.blendAffect(e, target, impact)
}

// transformAppraisal applies configured trait/tag rules in Trait declaration
// order, independent of profile storage order and YAML map order.
func (w *World) transformAppraisal(e *Entity, reaction *ReactionSpec, dynamic []TagID, target MoodVector, impact int) (MoodVector, int) {
	if e.Profile == nil {
		return target, impact
	}
	for trait := Trait(0); trait < numTraits; trait++ {
		if !e.Profile.HasTrait(trait) {
			continue
		}
		for i := range w.cognition.Modifiers {
			modifier := &w.cognition.Modifiers[i]
			if modifier.Trait != trait || !modifierMatches(*modifier, reaction.Tags, dynamic) {
				continue
			}
			impact = impact * modifier.Impact / 100
			target.Charge = target.Charge * modifier.Charge / 100
			target.Grip = target.Grip * modifier.Grip / 100
			target.Valence = target.Valence * modifier.Valence / 100
		}
	}
	return target, impact
}

func modifierMatches(rule TraitModifier, static, dynamic []TagID) bool {
	has := func(tag TagID) bool { return hasTag(static, tag) || hasTag(dynamic, tag) }
	if len(rule.AnyTags) > 0 {
		found := false
		for _, tag := range rule.AnyTags {
			if has(tag) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, tag := range rule.AllTags {
		if !has(tag) {
			return false
		}
	}
	for _, tag := range rule.NotTags {
		if has(tag) {
			return false
		}
	}
	return true
}
