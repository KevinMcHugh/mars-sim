package sim

// MoodVector is a cartesian affect displacement. Charge describes activation;
// Grip describes felt control.
type MoodVector struct {
	Charge int
	Grip   int
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

// AffectState is the colonist's bounded cartesian mood and cached display
// projection. labelBad caches contextual valence's word choice separately from
// attractor identity so context cannot feed behavior back through MoodKind.
type AffectState struct {
	Charge int
	Grip   int
	Label  MoodKind

	labelBad  bool
	labelName string
}

type moodAttractor struct {
	Kind         MoodKind
	GoodName     string
	BadName      string
	Charge, Grip int
	Radius       int
}

// Declaration order is significant: equal claims choose the first attractor.
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

// lifeEventMoodVectors is the complete semantic event appraisal table. Unlike
// decay and hysteresis, these meanings are intentionally not balance knobs.
var lifeEventMoodVectors = [numLifeEventKinds]MoodVector{
	EvtSawAlien:                  {8, -10},
	EvtSawMouse:                  {2, -3},
	EvtSawGore:                   {-3, -7},
	EvtBitten:                    {10, -8},
	EvtWitnessedColonistKilled:   {6, -16},
	EvtWitnessedColonistAttacked: {5, -9},
	EvtCrushedMouse:              {-1, 2},
	EvtWitnessedMouseCrushed:     {-1, -2},
	EvtWitnessedCatCatch:         {1, 1},
	EvtKilledAlien:               {12, 14},
	EvtWitnessedAlienKilled:      {5, 6},
	EvtWoundedAlien:              {4, 5},
	EvtWitnessedGunfight:         {7, -5},
	EvtConversation:              {3, 7},
	EvtAte:                       {4, 2},
	EvtUsedToilet:                {1, 2},
	EvtSlept:                     {15, 2},
	EvtNeedSatisfied:             {2, 2},
	EvtFinishedMining:            {-1, 5},
	EvtClearedRock:               {-1, 5},
	EvtFinishedConstruction:      {-1, 6},
	EvtCleanedRefuse:             {-1, 5},
	EvtIncineratedRefuse:         {-1, 7},
	EvtMutated:                   {4, -18},
	EvtWitnessedMutation:         {2, -8},
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
// conversation outcome without retaining that scalar as mood state.
func conversationMoodVector(outcome int) MoodVector {
	switch {
	case outcome > 0:
		return MoodVector{Charge: max(1, roundedDiv(outcome*3, 7)), Grip: outcome}
	case outcome < 0:
		return MoodVector{Charge: max(1, roundedDiv(-outcome*4, 6)), Grip: outcome}
	default:
		return MoodVector{}
	}
}

func finishedWorkEvent(kind LifeEventKind) bool {
	switch kind {
	case EvtFinishedMining, EvtClearedRock, EvtFinishedConstruction,
		EvtCleanedRefuse, EvtIncineratedRefuse:
		return true
	default:
		return false
	}
}

// transformMoodVector applies event-only traits in Trait declaration order,
// independent of the order traits happen to be stored on a Profile.
func transformMoodVector(e *Entity, kind LifeEventKind, v MoodVector) MoodVector {
	if e.Profile == nil {
		return v
	}
	for trait := Trait(0); trait < numTraits; trait++ {
		if !e.Profile.HasTrait(trait) {
			continue
		}
		switch trait {
		case TraitIndustrious:
			if finishedWorkEvent(kind) {
				v.Charge *= 2
				v.Grip *= 2
			}
		case TraitIntrovert:
			if kind == EvtConversation {
				v.Charge = -v.Charge
			}
		case TraitTidy:
			switch kind {
			case EvtSawGore:
				v.Charge = v.Charge * 22 / 10
				v.Grip = v.Grip * 22 / 10
			case EvtIncineratedRefuse:
				v.Grip *= 2
			}
		case TraitMutantLover:
			if kind == EvtMutated || kind == EvtWitnessedMutation {
				v.Grip = -v.Grip
			}
		}
	}
	return v
}

func (w *World) addAffect(e *Entity, v MoodVector) {
	e.affect.Charge = clampInt(e.affect.Charge+v.Charge, -w.cfg.MoodMax, w.cfg.MoodMax)
	e.affect.Grip = clampInt(e.affect.Grip+v.Grip, -w.cfg.MoodMax, w.cfg.MoodMax)
	w.refreshMoodAttractor(e)
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
	charge := approach(e.affect.Charge, 0, w.cfg.MoodChargeDecayPerTick)
	grip := approach(e.affect.Grip, 0, w.cfg.MoodGripDecayPerTick)
	if charge == e.affect.Charge && grip == e.affect.Grip {
		return
	}
	e.affect.Charge, e.affect.Grip = charge, grip
	w.refreshMoodAttractor(e)
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

func (w *World) contextualValence(e *Entity) int {
	badness := 0
	for n := NeedKind(0); n < numNeeds; n++ {
		badness = max(badness, needPressure(w.needLevel(e, n), w.cfg.Needs[n]))
	}
	badness = max(badness, 100*(e.MaxHP-e.HP)/atLeast1(e.MaxHP))
	if _, ok := w.nearestAlien(e.Pos, w.cfg.FleeRadius); ok {
		badness = 100
	}
	return 100 - 2*badness
}

func (w *World) refreshMoodAttractor(e *Entity) {
	challenger, challengerClaim := bestMoodAttractor(e.affect.Charge, e.affect.Grip)
	if challenger != e.affect.Label {
		incumbentClaim := moodClaim(e.affect.Label, e.affect.Charge, e.affect.Grip)
		if challengerClaim > incumbentClaim+w.cfg.MoodLabelSwitchMargin {
			e.affect.Label = challenger
		}
	}
	e.affect.labelName = moodName(e.affect.Label, e.affect.labelBad)
}

func moodName(kind MoodKind, bad bool) string {
	if kind == MoodSettling {
		return "settling"
	}
	for _, spec := range moodAttractors {
		if spec.Kind == kind {
			if bad {
				return spec.BadName
			}
			return spec.GoodName
		}
	}
	return "settling"
}

func (w *World) refreshMoodContext(e *Entity, needBad int, threatVisible bool) {
	badness := max(needBad, 100*(e.MaxHP-e.HP)/atLeast1(e.MaxHP))
	if threatVisible {
		badness = 100
	}
	bad := 100-2*badness < 0
	if bad == e.affect.labelBad && e.affect.labelName != "" {
		return
	}
	e.affect.labelBad = bad
	e.affect.labelName = moodName(e.affect.Label, bad)
}

// refreshMoodLabel is the combined, non-hot-path refresh used after direct
// context changes and by focused tests. Normal turns share need/threat facts
// already computed by focusCandidates.
func (w *World) refreshMoodLabel(e *Entity) {
	w.refreshMoodAttractor(e)
	w.refreshMoodContext(e, (100-w.contextualValence(e))/2, false)
}

func (a AffectState) MoodName() string {
	if a.labelName == "" {
		return moodName(a.Label, a.labelBad)
	}
	return a.labelName
}

func (w *World) applyAffect(e *Entity, evt LifeEvent) {
	v := lifeEventMoodVectors[evt.Kind]
	if evt.Outcome != 0 || evt.Kind == EvtConversation {
		v = conversationMoodVector(evt.Outcome)
	}
	v = transformMoodVector(e, evt.Kind, v)
	if v.Charge != 0 || v.Grip != 0 {
		w.addAffect(e, v)
	}
}
