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

// lifeEventKindNames is how each kind is spelled in events.yaml. The enum
// stays in Go because emit sites name kinds at compile time; the file supplies
// what each one means, not which ones exist.
var lifeEventKindNames = [numLifeEventKinds]string{
	EvtSawAlien:                  "saw-alien",
	EvtSawMouse:                  "saw-mouse",
	EvtSawGore:                   "saw-gore",
	EvtBitten:                    "bitten",
	EvtWitnessedColonistKilled:   "witnessed-colonist-killed",
	EvtWitnessedColonistAttacked: "witnessed-colonist-attacked",
	EvtCrushedMouse:              "crushed-mouse",
	EvtWitnessedMouseCrushed:     "witnessed-mouse-crushed",
	EvtWitnessedCatCatch:         "witnessed-cat-catch",
	EvtKilledAlien:               "killed-alien",
	EvtWitnessedAlienKilled:      "witnessed-alien-killed",
	EvtWoundedAlien:              "wounded-alien",
	EvtWitnessedGunfight:         "witnessed-gunfight",
	EvtConversation:              "conversation",
	EvtAte:                       "ate",
	EvtUsedToilet:                "used-toilet",
	EvtSlept:                     "slept",
	EvtNeedSatisfied:             "need-satisfied",
	EvtFinishedMining:            "finished-mining",
	EvtClearedRock:               "cleared-rock",
	EvtFinishedConstruction:      "finished-construction",
	EvtCleanedRefuse:             "cleaned-refuse",
	EvtIncineratedRefuse:         "incinerated-refuse",
	EvtMutated:                   "mutated",
	EvtWitnessedMutation:         "witnessed-mutation",
}

func (k LifeEventKind) String() string {
	if int(k) >= len(lifeEventKindNames) {
		return "unknown"
	}
	return lifeEventKindNames[k]
}

// LifeEvent pairs what happened with its player-facing description. Outcome is
// the temporary signed result of a conversation; it is converted to a vector
// during ingestion and is never stored as mood state.
type LifeEvent struct {
	Kind LifeEventKind
	// Source is what caused the occurrence -- the alien that bit someone --
	// and is what stimulus tracking keys on. Subject is who it happened to,
	// which is a different entity and a different question: whether the
	// colonist remembering this cared about them. Either may be zero.
	Subject EntityID
	Source  EntityID
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

// eventAbout is eventFrom for an occurrence that happened *to* someone: the
// source caused it, the subject suffered it. Ingestion reads the subject to
// decide whether this happened to a stranger or to someone the colonist is
// close to, which is not a fact any table could hold.
func eventAbout(kind LifeEventKind, source, subject EntityID, format string, args ...any) LifeEvent {
	return LifeEvent{Kind: kind, Source: source, Subject: subject, Text: fmt.Sprintf(format, args...)}
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
var lifeEventCollapseText [numLifeEventKinds]string
