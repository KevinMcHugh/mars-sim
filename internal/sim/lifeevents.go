package sim

import "fmt"

// LifeEventKind identifies a notable, memory-worthy thing that happened to or
// near a colonist. It is the single point where "what happened" (the memory
// text, supplied per occurrence — see LifeEvent) meets "how it feels" (the
// mood effects below, declared once per kind). See docs/memories.md.
//
// Named LifeEvent/LifeEventKind rather than Event: this package already has
// a WorldEvent interface (events.go) for the terrain-change pub/sub bus, an
// unrelated concept — a bare Event would be ambiguous between the two.
type LifeEventKind uint8

const (
	EvtSawAlien LifeEventKind = iota
	EvtSawMouse
	EvtSawGore
	EvtBitten
	EvtWitnessedColonistKilled
	EvtWitnessedColonistAttacked
	EvtCrushedMouse
	EvtWitnessedMouseCrushed
	EvtWitnessedCatCatch
	EvtKilledAlien
	EvtWitnessedAlienKilled
	EvtWoundedAlien
	EvtWitnessedGunfight
	EvtConversation
	EvtAte
	EvtUsedToilet
	EvtSlept
	EvtNeedSatisfied
	EvtFinishedMining
	EvtClearedRock
	EvtFinishedConstruction
	EvtCleanedRefuse
	EvtIncineratedRefuse
	EvtMutated
	EvtWitnessedMutation

	numLifeEventKinds // keep last
)

// MoodEffect is one mood delta a LifeEvent applies. An unconditional effect
// (Conditional false) applies to every colonist; a conditional one applies
// only to a colonist whose Profile carries Trait. A LifeEvent lists as many
// as it needs — e.g. gore is a small universal dip plus an extra one that
// only stacks on for the squeamish (TraitTidy) — so "which traits does this
// affect, or is it universal" is answered as data right next to the base
// effect, not as a special case in code.
type MoodEffect struct {
	Conditional bool
	Trait       Trait // meaningful only when Conditional
	Delta       int
}

// jobFinishedMood is shared by every "finished a work job" LifeEventKind: a
// minor universal sense of accomplishment, doubled for the Industrious, who
// take more satisfaction in a completed task.
var jobFinishedMood = []MoodEffect{
	{Delta: 2},
	{Conditional: true, Trait: TraitIndustrious, Delta: 2},
}

// lifeEventMoodEffects is the mood table: one slice of MoodEffect per
// LifeEventKind, indexed by the constant. A kind with no entry (the zero
// value, a nil slice) is memory-worthy but mood-neutral — most kinds are,
// today. Giving an existing event a mood effect, or adding a new mood-bearing
// event, is a table edit here; nothing else needs to change. This only covers
// a *fixed* (optionally trait-gated) delta; an event whose delta must be
// computed per occurrence (there is exactly one today — EvtConversation, see
// finishTalk) carries it on the LifeEvent itself instead (see Mood below).
var lifeEventMoodEffects = [numLifeEventKinds][]MoodEffect{
	EvtSawAlien: {{Delta: -6}},
	EvtSawMouse: {{Delta: -2}},
	EvtSawGore: {
		{Delta: -4},
		{Conditional: true, Trait: TraitTidy, Delta: -6},
	},
	EvtBitten:               {{Delta: -5}},
	EvtKilledAlien:          {{Delta: 15}},
	EvtWitnessedAlienKilled: {{Delta: 6}},
	// Growing a part you were not born with is body horror — unless being
	// changed is the thing you already admired in other people, in which case
	// it is the best day of your life. Same event, opposite sign, declared as
	// data: the conditional effect adds to the base one, so a mutant-lover
	// nets +14 where everyone else nets -14.
	EvtMutated: {
		{Delta: -14},
		{Conditional: true, Trait: TraitMutantLover, Delta: 28},
	},
	EvtWitnessedMutation: {
		{Delta: -6},
		{Conditional: true, Trait: TraitMutantLover, Delta: 12},
	},
	EvtFinishedMining:       jobFinishedMood,
	EvtClearedRock:          jobFinishedMood,
	EvtFinishedConstruction: jobFinishedMood,
	EvtCleanedRefuse:        jobFinishedMood,
	// Burning the mess is the moment the colony is clean again, so it carries
	// the ordinary job satisfaction plus an extra lift for the Tidy — the same
	// colonist EvtSawGore hits hardest, relieved in the same terms.
	EvtIncineratedRefuse: {
		{Delta: 2},
		{Conditional: true, Trait: TraitIndustrious, Delta: 2},
		{Conditional: true, Trait: TraitTidy, Delta: 6},
	},
}

// LifeEvent pairs what happened (Kind, which looks up the fixed mood effects
// above and could back future filtering/UI) with the player-facing
// description of it (Text, what actually lands in the memory log). Build one
// with event(), or eventMood() when the delta can't be a fixed table lookup
// (it depends on something rolled or accumulated per occurrence, like a
// conversation's quality).
type LifeEvent struct {
	Kind LifeEventKind
	Text string
	Mood int // extra delta on top of Kind's table effects; 0 for a table-only event
}

// event builds a LifeEvent with no per-occurrence mood component, formatting
// Text like fmt.Sprintf — a plain string with no verbs works fine too, so
// every table-driven remember() call site uses this rather than switching
// between a raw string and a formatted one.
func event(kind LifeEventKind, format string, args ...any) LifeEvent {
	return LifeEvent{Kind: kind, Text: fmt.Sprintf(format, args...)}
}

// eventMood is event() plus a caller-computed mood delta, for the rare event
// whose mood impact isn't a fixed (optionally trait-gated) constant — it adds
// to, rather than replaces, whatever Kind's table declares (today, that's
// only ever nothing, but a future event could combine both).
func eventMood(kind LifeEventKind, mood int, format string, args ...any) LifeEvent {
	return LifeEvent{Kind: kind, Text: fmt.Sprintf(format, args...), Mood: mood}
}

// applyMoodEffects folds a LifeEvent's mood impact onto a colonist: every
// unconditional table effect always applies, a conditional one only if the
// colonist's profile carries the named trait, and evt.Mood always applies on
// top. Everything is summed into a single adjustMood call — one clamp, one
// source of truth for "how did this affect mood," whether the number came
// from the table or was computed by the caller.
func (w *World) applyMoodEffects(e *Entity, evt LifeEvent) {
	delta := evt.Mood
	for _, eff := range lifeEventMoodEffects[evt.Kind] {
		if eff.Conditional && (e.Profile == nil || !e.Profile.HasTrait(eff.Trait)) {
			continue
		}
		delta += eff.Delta
	}
	if delta != 0 {
		w.adjustMood(e, delta)
	}
}
