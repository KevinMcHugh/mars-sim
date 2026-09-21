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
	Tags        EventTag
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

// EventTag is a set of flavors an occurrence has, one bit each. A trait reacts
// to tags rather than to kinds, which is what keeps adding a trait from being a
// decision against every event and vice versa: a new event picks its tags, a
// new trait picks the tags it cares about, and neither has to know the other
// exists.
//
// A set rather than a single value, because occurrences genuinely have several
// flavors at once -- watching an alien eat someone is a death, an act of
// violence, the loss of a colleague, and a mess, and different colonists react
// to different parts of that.
type EventTag uint32

const (
	TagThreat EventTag = 1 << iota
	TagViolence
	TagDeath
	TagSocialLoss
	TagGore
	TagRodent
	TagWork
	TagFinishedWork
	TagIncineration
	TagAchievement
	TagUpkeep
	TagRest
	TagSocial
	TagMutation
	// TagFriend is the one tag no table can declare: ingestion stamps it per
	// occurrence when the colonist was close to whoever this happened to.
	TagFriend
)

// eventTagNames is how each tag is spelled in events.yaml, in bit order.
var eventTagNames = []struct {
	Tag  EventTag
	Name string
}{
	{TagThreat, "threat"},
	{TagViolence, "violence"},
	{TagDeath, "death"},
	{TagSocialLoss, "social-loss"},
	{TagGore, "gore"},
	{TagRodent, "rodent"},
	{TagWork, "work"},
	{TagFinishedWork, "finished-work"},
	{TagIncineration, "incineration"},
	{TagAchievement, "achievement"},
	{TagUpkeep, "upkeep"},
	{TagRest, "rest"},
	{TagSocial, "social"},
	{TagMutation, "mutation"},
	{TagFriend, "friend"},
}

func (t EventTag) any(of EventTag) bool  { return of == 0 || t&of != 0 }
func (t EventTag) all(of EventTag) bool  { return t&of == of }
func (t EventTag) none(of EventTag) bool { return t&of == 0 }

// lifeEventAppraisals is what each kind of occurrence means to a colonist,
// loaded from events.yaml at init. It was a Go literal until the tag phase
// left nothing in the engine branching on a specific kind, at which point
// keeping 25 rows of pure data in source stopped paying for itself: the
// sandbox that tunes these numbers can now emit the file the sim reads.
var lifeEventAppraisals [numLifeEventKinds]moodAppraisal

// traitRules is what each trait does when something happens, loaded from
// traits.yaml at init. Order is application order.
var traitRules []traitRule

// traitRule is one trait's reaction to a flavor of occurrence. Any empty
// matches every event; All must all be present; None must all be absent.
type traitRule struct {
	Trait          Trait
	Any, All, None EventTag

	Impact   int
	Charge   int
	Grip     int
	Valence  int
	WearRate int
}

func (r traitRule) matches(tags EventTag) bool {
	return tags.any(r.Any) && tags.all(r.All) && tags.none(r.None)
}

// pct scales by a percentage, truncating toward zero exactly as the per-trait
// arithmetic it replaced did, so the transforms that predate the rule table
// still land on the same numbers.
func pct(v, scale int) int { return v * orPct(scale) / 100 }

// orPct reads traitSpecs' convention: an unset factor means no change.
func orPct(scale int) int {
	if scale == 0 {
		return 100
	}
	return scale
}

// colonistRules yields the rules that apply to this colonist and occurrence, in
// declaration order.
func (w *World) forEachRule(e *Entity, tags EventTag, fn func(traitRule)) {
	if e.Profile == nil {
		return
	}
	for _, r := range traitRules {
		if r.matches(tags) && e.Profile.HasTrait(r.Trait) {
			fn(r)
		}
	}
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

// transformMoodVector applies every trait rule matching this occurrence, in
// rule declaration order, independent of the order traits happen to be stored
// on a Profile.
func (w *World) transformMoodVector(e *Entity, tags EventTag, v MoodVector) MoodVector {
	w.forEachRule(e, tags, func(r traitRule) {
		v.Charge = pct(v.Charge, r.Charge)
		v.Grip = pct(v.Grip, r.Grip)
		v.Valence = pct(v.Valence, r.Valence)
	})
	return v
}

// eventTags is the occurrence's declared flavors plus whatever only this
// occurrence knows. Today that is one thing: whether it happened to someone
// this colonist is close to, which is why remember passes the whole event here
// rather than just its kind.
func (w *World) eventTags(e *Entity, evt LifeEvent) EventTag {
	tags := lifeEventAppraisals[evt.Kind].Tags
	if evt.Subject != 0 && evt.Subject != e.ID &&
		w.affinityBetween(e.ID, evt.Subject) >= w.cfg.MoodFriendAffinity {
		tags |= TagFriend
	}
	return tags
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

// decayAffect settles a colonist back toward their own resting point, which is
// the origin for most of them and is not for anyone with a temperament.
func (w *World) decayAffect(e *Entity) {
	// Valence describes how life has been going over hours and days, not the
	// minutes charge and grip answer to, so it gives up a point every few turns
	// rather than points every turn. The period counts world ticks rather than
	// per-colonist turns so a seeded run stays reproducible.
	valence := e.affect.Valence
	if n := w.cfg.MoodValenceDecayTicks; n > 0 && w.tick%n == 0 {
		valence = approach(valence, e.affectHome.Valence, 1)
	}
	w.setAffect(e,
		approach(e.affect.Charge, e.affectHome.Charge, w.cfg.MoodChargeDecayPerTick),
		approach(e.affect.Grip, e.affectHome.Grip, w.cfg.MoodGripDecayPerTick),
		valence)
}

// affectSettled reports whether a colonist's mood has finished moving. Charge
// and grip only: nothing scored reads valence, so its drift is no reason to
// think again.
func (e *Entity) affectSettled() bool {
	return e.affect.Charge == e.affectHome.Charge && e.affect.Grip == e.affectHome.Grip
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
	rate := w.cfg.MoodWearPerOccasion
	// Declared tags only: how fast someone gets used to a kind of thing is
	// about the kind, not about who this particular one happened to.
	w.forEachRule(e, lifeEventAppraisals[kind].Tags, func(r traitRule) {
		rate = pct(rate, r.WearRate)
	})
	return min(100, occasions*rate)
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
	tags := w.eventTags(e, evt)
	target := wearTarget(appraisal, w.moodWear(e, evt.Kind))
	if evt.Outcome != 0 || evt.Kind == EvtConversation {
		// A conversation is exempt: its vector is already computed per
		// occurrence, and noteConversation's social fatigue window is wearing
		// down repetition by the time it gets here. Wearing it again would
		// charge a talkative colonist twice for the same talkativeness.
		target = conversationMoodVector(evt.Outcome)
	}
	target = w.transformMoodVector(e, tags, target)
	impact := appraisal.Impact
	w.forEachRule(e, tags, func(r traitRule) { impact = pct(impact, r.Impact) })
	w.blendAffect(e, target, impact)
}
