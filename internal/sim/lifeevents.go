package sim

import "fmt"

// LifeEventKind identifies a notable, memory-worthy thing that happened to or
// near a colonist. It is the single point where "what happened" (the memory
// text, supplied per occurrence — see LifeEvent) meets "how it feels" (the
// mood effects below, declared once per kind). See docs/memories.md.
//
// Named LifeEvent/LifeEventKind rather than Event: this package already has
// an Event interface (events.go) for the terrain-change pub/sub bus, an
// unrelated concept — reusing the name would collide.
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

// lifeEventMoodEffects is the mood table: one slice of MoodEffect per
// LifeEventKind, indexed by the constant. A kind with no entry (the zero
// value, a nil slice) is memory-worthy but mood-neutral — most kinds are,
// today. Giving an existing event a mood effect, or adding a new mood-bearing
// event, is a table edit here; nothing else needs to change.
var lifeEventMoodEffects = [numLifeEventKinds][]MoodEffect{
	EvtSawAlien: {{Delta: -6}},
	EvtSawMouse: {{Delta: -2}},
	EvtSawGore: {
		{Delta: -4},
		{Conditional: true, Trait: TraitTidy, Delta: -6},
	},
	EvtKilledAlien:          {{Delta: 15}},
	EvtWitnessedAlienKilled: {{Delta: 6}},
}

// LifeEvent pairs what happened (Kind, which looks up mood effects and could
// back future filtering/UI) with the player-facing description of it (Text,
// what actually lands in the memory log). Build one with event().
type LifeEvent struct {
	Kind LifeEventKind
	Text string
}

// event builds a LifeEvent, formatting Text like fmt.Sprintf — a plain string
// with no verbs works fine too, so every remember() call site uses this
// rather than switching between a raw string and a formatted one.
func event(kind LifeEventKind, format string, args ...any) LifeEvent {
	return LifeEvent{Kind: kind, Text: fmt.Sprintf(format, args...)}
}

// applyMoodEffects folds a LifeEvent's mood table onto a colonist: every
// unconditional effect always applies, and a conditional one only if the
// colonist's profile carries the named trait. Effects stack (see EvtSawGore).
func (w *World) applyMoodEffects(e *Entity, kind LifeEventKind) {
	for _, eff := range lifeEventMoodEffects[kind] {
		if eff.Conditional && (e.Profile == nil || !e.Profile.HasTrait(eff.Trait)) {
			continue
		}
		w.adjustMood(e, eff.Delta)
	}
}
