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
	EvtSawRat
	EvtSawGore
	EvtBitten
	EvtWitnessedColonistKilled
	EvtWitnessedColonistAttacked
	EvtCrushedRat
	EvtWitnessedRatCrushed
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
	EvtAteGruel // ate the safety net's free pod gruel rather than a real meal
	EvtMadeSlurry
	EvtScrapedScum
	EvtFedScumhouse
	EvtWentToMarket
	EvtBoughtMeal

	numLifeEventKinds // keep last
)

// LifeEvent pairs what happened with its player-facing description. Outcome is
// the temporary signed result of a conversation; it is converted to a vector
// during ingestion and is never stored as mood state.
type LifeEvent struct {
	Kind    LifeEventKind
	Source  EntityID // zero when the occurrence is not tied to an entity
	Text    string
	Outcome int // meaningful only for EvtConversation
}

// event builds a LifeEvent with no per-occurrence mood component, formatting
// Text like fmt.Sprintf — a plain string with no verbs works fine too, so
// every table-driven remember() call site uses this rather than switching
// between a raw string and a formatted one.
func event(kind LifeEventKind, format string, args ...any) LifeEvent {
	return LifeEvent{Kind: kind, Text: fmt.Sprintf(format, args...)}
}

// eventFrom is event with the entity responsible for or represented by the
// occurrence. Keeping source on the event lets stimulus ingestion remain in the
// same funnel as mood and memory.
func eventFrom(kind LifeEventKind, source EntityID, format string, args ...any) LifeEvent {
	return LifeEvent{Kind: kind, Source: source, Text: fmt.Sprintf(format, args...)}
}

// eventOutcome carries the one per-occurrence appraisal input that cannot be a
// semantic table lookup: conversation quality plus company and fatigue.
func eventOutcome(kind LifeEventKind, outcome int, format string, args ...any) LifeEvent {
	return LifeEvent{Kind: kind, Text: fmt.Sprintf(format, args...), Outcome: outcome}
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
	EvtMadeSlurry:           "Worked the scumhouse.",
	EvtScrapedScum:          "Scraped cave scum.",
	EvtFedScumhouse:         "Fed the scumhouse.",
	EvtWentToMarket:         "Went to market.",
	EvtAteGruel:             "Ate nutrient-pod gruel.",
	EvtUsedToilet:           "Used the toilet.",
	EvtSlept:                "Slept in a bed.",
	EvtNeedSatisfied:        "Satisfied a need.",
}
