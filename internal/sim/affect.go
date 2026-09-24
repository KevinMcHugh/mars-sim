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

// moodAppraisal is how one life event kind reads to a colonist: where it wants
// to leave them, and how hard it insists. Impact alone chooses between the two
// operations in blendAffect, which is why a run of good meals cannot cancel a
// killing -- the killing moved the colonist rather than adding to them.
//
// Fresh is where the event leaves someone it is new to; Worn is where it
// leaves someone it has stopped being new to. Note which way round the pair
// runs for the traumatic kinds: Fresh is the *stronger* reading, not the
// weaker one. The first death is a rallying cry and the tenth is catatonia, so
// wear turns a reaction rather than just quieting it.
type moodAppraisal struct {
	Impact      int
	Fresh, Worn MoodVector
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

// lifeEventAppraisals is the complete semantic event appraisal table. Unlike
// decay, hysteresis and the push/pull endpoints, these meanings are
// intentionally not balance knobs.
//
// Reading a row: routine life sits under MoodPushImpact, where the target is a
// small displacement that accumulates. The handful above it are targets in the
// literal sense -- a colonist who watches someone die ends up near `anxious`
// no matter how good their day had been -- so those rows are written at the
// scale of the plane rather than the scale of a nudge, along the same
// direction the smaller version pointed.
//
// Valence is mostly zero here on purpose. Charge and grip shed points every
// turn, so a steady drip of small numbers goes nowhere, but valence settles a
// point at a time and a colonist digs far more often than that: paying it for
// routine upkeep pegged every colonist at the maximum within a few hundred
// ticks, which is the same "everyone always reads as fine" failure the axis
// exists to fix. So only what a colonist would actually count as a good or bad
// day moves it, and an uneventful shift leaves it where it was. The worn
// column holds that line for the same reason: monotony takes away the lift a
// job used to give rather than making the job itself a misfortune, so it shows
// up in grip rather than as a slow bleed of valence no colonist could explain.
var lifeEventAppraisals = [numLifeEventKinds]moodAppraisal{
	EvtSawAlien:                  {45, MoodVector{8, -10, -15}, MoodVector{5, -22, -22}},
	EvtSawRat:                    {8, MoodVector{2, -3, 0}, MoodVector{1, -1, 0}},
	EvtSawGore:                   {30, MoodVector{-3, -7, -6}, MoodVector{-6, -14, -10}},
	EvtBitten:                    {70, MoodVector{55, -44, -30}, MoodVector{35, -80, -55}},
	EvtWitnessedColonistKilled:   {95, MoodVector{70, 40, -60}, MoodVector{20, -85, -85}},
	EvtWitnessedColonistAttacked: {75, MoodVector{45, 25, -40}, MoodVector{25, -70, -60}},
	EvtCrushedRat:                {6, MoodVector{-1, 2, 0}, MoodVector{-2, 0, 0}},
	EvtWitnessedRatCrushed:       {8, MoodVector{-1, -2, -1}, MoodVector{-1, -1, 0}},
	EvtWitnessedCatCatch:         {5, MoodVector{1, 1, 0}, MoodVector{0, 0, 0}},
	EvtKilledAlien:               {55, MoodVector{45, 52, 35}, MoodVector{25, 20, 8}},
	EvtWitnessedAlienKilled:      {35, MoodVector{5, 6, 4}, MoodVector{2, 2, 0}},
	EvtWoundedAlien:              {25, MoodVector{4, 5, 2}, MoodVector{2, 2, 0}},
	EvtWitnessedGunfight:         {50, MoodVector{35, -25, -20}, MoodVector{18, -45, -35}},
	// A chat's target is the one thing here that cannot be a table lookup: it
	// comes from how that particular conversation went. Only the impact is
	// declared; conversationMoodVector supplies the rest per occurrence, and
	// wear leaves it alone (see applyAffect).
	EvtConversation:         {Impact: 20},
	EvtAte:                  {10, MoodVector{4, 2, 1}, MoodVector{2, -1, 0}},
	EvtUsedToilet:           {4, MoodVector{1, 2, 0}, MoodVector{0, 0, 0}},
	EvtSlept:                {25, MoodVector{15, 2, 2}, MoodVector{12, -2, 0}},
	EvtNeedSatisfied:        {8, MoodVector{2, 2, 0}, MoodVector{1, 0, 0}},
	EvtFinishedMining:       {15, MoodVector{-1, 5, 0}, MoodVector{-5, -3, 0}},
	EvtClearedRock:          {15, MoodVector{-1, 5, 0}, MoodVector{-5, -3, 0}},
	EvtFinishedConstruction: {18, MoodVector{-1, 6, 3}, MoodVector{-4, 0, 0}},
	EvtCleanedRefuse:        {14, MoodVector{-1, 5, 0}, MoodVector{-5, -4, 0}},
	EvtIncineratedRefuse:    {16, MoodVector{-1, 7, 1}, MoodVector{-4, 0, 0}},
	EvtMutated:              {60, MoodVector{18, -70, -35}, MoodVector{10, -90, -70}},
	EvtWitnessedMutation:    {35, MoodVector{2, -8, -10}, MoodVector{1, -16, -18}},
	// Gruel fills a stomach and nothing else: the safety net is meant to be
	// the worst way to eat, so it reads as a small, dispiriting non-event next
	// to a real meal's lift, and wears into a mild grievance.
	EvtAteGruel: {10, MoodVector{0, 0, -1}, MoodVector{-1, -2, -1}},
	// Food work reads like the other chores: a little satisfying, and dull
	// once it is routine.
	EvtMadeSlurry:   {15, MoodVector{-1, 5, 1}, MoodVector{-5, -2, 0}},
	EvtScrapedScum:  {12, MoodVector{-1, 4, 0}, MoodVector{-5, -3, 0}},
	EvtFedScumhouse: {10, MoodVector{-1, 4, 0}, MoodVector{-4, -2, 0}},
	// Trading is a small errand; buying food you could not otherwise have is
	// a relief with a little sting in it.
	EvtWentToMarket: {8, MoodVector{0, 3, 0}, MoodVector{-2, 0, 0}},
	EvtBoughtMeal:   {12, MoodVector{2, 3, 0}, MoodVector{1, 0, 0}},
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
				v.Valence *= 2
			}
		case TraitIntrovert:
			// Charge only: the conversation still did them good, it just cost
			// them something to have it.
			if kind == EvtConversation {
				v.Charge = -v.Charge
			}
		case TraitTidy:
			switch kind {
			case EvtSawGore:
				v.Charge = v.Charge * 22 / 10
				v.Grip = v.Grip * 22 / 10
				v.Valence = v.Valence * 22 / 10
			case EvtIncineratedRefuse:
				v.Grip *= 2
			}
		case TraitMutantLover:
			// Grip and valence both: to them the change is mastery, and a good
			// thing to have happened.
			if kind == EvtMutated || kind == EvtWitnessedMutation {
				v.Grip = -v.Grip
				v.Valence = -v.Valence
			}
		}
	}
	return v
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

func (w *World) refreshMoodAttractor(e *Entity) {
	challenger, challengerClaim := bestMoodAttractor(e.affect.Charge, e.affect.Grip)
	if challenger == e.affect.Label {
		return
	}
	incumbentClaim := moodClaim(e.affect.Label, e.affect.Charge, e.affect.Grip)
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

// moodWear reports how used to a kind of thing a colonist is, in hundredths.
//
// It counts remembered *occasions* rather than occurrences: a run of digs
// folds into one memory, and collapsing has already decided that run was one
// memorable thing. Counting each occurrence instead would let a single long
// shift peg a colonist's wear permanently, which is the ratchet this has to
// avoid. Entries roll off the bounded memory log, so wear falls again once
// something stops happening -- recovery is most of what keeps this feeling
// like a person rather than a counter.
func (w *World) moodWear(e *Entity, kind LifeEventKind) int {
	occasions := 0
	for i := range e.Memories {
		if e.Memories[i].Kind == kind {
			occasions++
		}
	}
	return min(100, occasions*w.cfg.MoodWearPerOccasion)
}

// wearTarget moves an appraisal from its fresh reading toward its worn one.
func wearTarget(a moodAppraisal, wear int) MoodVector {
	if wear <= 0 {
		return a.Fresh
	}
	return MoodVector{
		Charge:  a.Fresh.Charge + roundedDiv((a.Worn.Charge-a.Fresh.Charge)*wear, 100),
		Grip:    a.Fresh.Grip + roundedDiv((a.Worn.Grip-a.Fresh.Grip)*wear, 100),
		Valence: a.Fresh.Valence + roundedDiv((a.Worn.Valence-a.Fresh.Valence)*wear, 100),
	}
}

func (w *World) applyAffect(e *Entity, evt LifeEvent) {
	appraisal := lifeEventAppraisals[evt.Kind]
	target := wearTarget(appraisal, w.moodWear(e, evt.Kind))
	if evt.Outcome != 0 || evt.Kind == EvtConversation {
		// A conversation is exempt: its vector is already computed per
		// occurrence, and noteConversation's social fatigue window is wearing
		// down repetition by the time it gets here. Wearing it again would
		// charge a talkative colonist twice for the same talkativeness.
		target = conversationMoodVector(evt.Outcome)
	}
	target = transformMoodVector(e, evt.Kind, target)
	w.blendAffect(e, target, appraisal.Impact)
}
