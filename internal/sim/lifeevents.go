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

// repeatMoodDecay is the diminishing-returns curve: the percentage of a
// LifeEventKind's table mood effect that still lands, indexed by how many
// times in a row that kind has now happened (entry 0 is the first time). The
// last entry holds for every occurrence past the end of the table, so a long
// enough run stops moving mood at all.
//
// The first completed dig of a shift is an accomplishment; the twelfth is just
// work. Without this, mood is a ratchet — it has no time decay, so a colonist
// left on the mining frontier climbs to +MoodMax and stays pinned there, and
// "how is this colonist doing" stops meaning anything. The curve applies to
// negative effects too, as habituation: a run of nothing but bad news lands
// softer each time, the same way a run of nothing but good news does.
//
// It is a package-level table rather than a Config knob for the same reason
// lifeEventMoodEffects is: it is the shape of the mood model, edited with the
// deltas it scales, not a per-run tunable like MineTicks.
var repeatMoodDecay = []int{100, 60, 35, 20, 10, 0}

// noteRepeat records that kind just happened to this colonist and returns how
// many times in a row it now has — 1 for a fresh kind, n for the nth
// consecutive one. Anything else happening in between resets the streak, so
// "over and over" means uninterrupted, the same sense of a run that memory
// collapsing uses (see collapseRepeat in world.go).
func (e *Entity) noteRepeat(kind LifeEventKind) int {
	if e.repeatRun > 0 && e.repeatKind == kind {
		e.repeatRun++
	} else {
		e.repeatKind = kind
		e.repeatRun = 1
	}
	return e.repeatRun
}

// repeatMoodPercent reads the decay curve for the repeat-th consecutive
// occurrence (1-based), holding the table's last entry for anything past its
// end.
func repeatMoodPercent(repeat int) int {
	if repeat < 1 {
		repeat = 1
	}
	if repeat > len(repeatMoodDecay) {
		repeat = len(repeatMoodDecay)
	}
	return repeatMoodDecay[repeat-1]
}

// scaleMood takes percent of delta, rounding half away from zero so a small
// effect fades gradually to nothing instead of truncating to 0 the moment the
// curve dips below 100% — at the +2 a finished job is worth, plain truncation
// would make the second one free.
func scaleMood(delta, percent int) int {
	scaled := (abs(delta)*percent + 50) / 100
	if delta < 0 {
		return -scaled
	}
	return scaled
}

// applyMoodEffects folds a LifeEvent's mood impact onto a colonist: every
// unconditional table effect always applies, a conditional one only if the
// colonist's profile carries the named trait, and evt.Mood always applies on
// top. Everything is summed into a single adjustMood call — one clamp, one
// source of truth for "how did this affect mood," whether the number came
// from the table or was computed by the caller.
//
// repeat is how many times in a row this kind has happened (from noteRepeat),
// and it discounts the *table* effects only. A caller-computed evt.Mood is
// left at full strength because a caller that computes a delta per occurrence
// already owns whatever fatigue it should have — the one today, a
// conversation, has social fatigue with its own window and per-colonist
// capacity (noteConversation), and stacking a second curve on top would
// double-count the same "you have been doing a lot of this" idea with two
// unrelated shapes.
func (w *World) applyMoodEffects(e *Entity, evt LifeEvent, repeat int) {
	table := 0
	for _, eff := range lifeEventMoodEffects[evt.Kind] {
		if eff.Conditional && (e.Profile == nil || !e.Profile.HasTrait(eff.Trait)) {
			continue
		}
		table += eff.Delta
	}
	delta := evt.Mood + scaleMood(table, repeatMoodPercent(repeat))
	if delta != 0 {
		w.adjustMood(e, delta)
	}
}

// lifeEventCollapseText is the collapse table: a non-empty entry marks a kind
// as *minor and repetitive* — routine work and bodily upkeep, the things a
// colonist does dozens of times in a row — and supplies the text a run of them
// collapses to. The entry is deliberately less specific than the per-
// occurrence text: once a memory stands for twelve digs, which tile each one
// was at is not what the line is about any more, so "Finished mining at
// (514, 501)." becomes "Finished mining." A kind with no entry (the zero
// value, "") never collapses — every occurrence stays its own memory, which is
// the right default for anything a player would read as a distinct beat in the
// colonist's story (a kill, a bite, a mutation, a conversation with a
// particular person).
//
// Making an existing kind collapsible, or changing what a run reads as, is an
// edit here; nothing else changes. See docs/memories.md.
var lifeEventCollapseText = [numLifeEventKinds]string{
	EvtFinishedMining:       "Finished mining.",
	EvtClearedRock:          "Cleared rock for a room.",
	EvtFinishedConstruction: "Finished construction.",
	EvtCleanedRefuse:        "Cleaned up refuse.",
	EvtIncineratedRefuse:    "Burned refuse in the incinerator.",
	EvtAte:                  "Had a meal.",
	EvtUsedToilet:           "Used the toilet.",
	EvtSlept:                "Slept in a bed.",
	EvtNeedSatisfied:        "Satisfied a need.",
}
