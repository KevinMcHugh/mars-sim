package sim

import (
	"fmt"
	"testing"
)

func TestConfigurableOccurrencePerceptionAndReaction(t *testing.T) {
	cfg := testConfig()
	if err := ApplyCognitionYAML(&cfg.Cognition, []byte(`
vocabulary:
  nouns: [toy]
  actions: [play]
perceptions:
  - id: hear-toy-play
    match: { actor_noun: colonist, action: play, object_noun: toy }
    sense: { channel: hearing, role: witness, distance: 5, cadence: instant }
reactions:
  - id: heard-toy-play
    match: { actor_noun: colonist, action: play, object_noun: toy, channel: hearing, role: witness, phase: instant }
    memory: { record: true, text: "Heard {actor} play with {object}." }
    affect: { impact: 10, fresh: { charge: 1, grip: 2, valence: 1 }, worn: { charge: 0, grip: 1, valence: 0 } }
    wear_policy: memory-occasions
`)); err != nil {
		t.Fatalf("apply custom cognition: %v", err)
	}
	cfg.SyncWithCognition()
	w := newTestWorld(t, cfg)
	actor := w.spawn(Colonist, Point{5, 5})
	near := w.spawn(Colonist, Point{9, 5})
	far := w.spawn(Colonist, Point{20, 20})

	o := occurrence(actor, "play", nil, actor.Pos, "")
	o.Object = FactRef{Noun: "toy", Label: "a toy"}
	w.emitOccurrence(o)

	if got := lastMemory(near); got != "Heard "+actor.displayName()+" play with a toy." {
		t.Fatalf("near witness memory = %q", got)
	}
	if near.affect.Grip != 2 || near.affect.Valence != 1 {
		t.Fatalf("near witness affect = %+v", near.affect)
	}
	if len(far.Memories) != 0 {
		t.Fatalf("far colonist perceived out-of-range occurrence: %+v", far.Memories)
	}
	if len(actor.Memories) != 0 {
		t.Fatalf("actor matched witness-only reaction: %+v", actor.Memories)
	}
}

func TestConfiguredLineOfSightBlocksPersistentPerception(t *testing.T) {
	cfg := testConfig()
	if err := ApplyCognitionYAML(&cfg.Cognition, []byte(`
perceptions:
  - id: visible-alien
    match: { actor_noun: alien, action: present }
    sense: { channel: sight, role: witness, distance: 5, line_of_sight: true, cadence: enter-and-ongoing }
`)); err != nil {
		t.Fatalf("enable line of sight: %v", err)
	}
	cfg.SyncWithCognition()
	w := newTestWorld(t, cfg)
	observer := w.spawn(Colonist, Point{2, 2})
	w.spawn(Alien, Point{4, 2})
	w.SetTerrain(Point{3, 2}, Wall)

	w.observeNearby(observer)
	if len(observer.Memories) != 0 {
		t.Fatalf("wall did not occlude alien: %+v", observer.Memories)
	}

	w.SetTerrain(Point{3, 2}, Floor)
	w.observeNearby(observer)
	if got := lastMemory(observer); got == "" {
		t.Fatal("clearing line of sight did not create enter percept")
	}
}

func TestPersistentPerceptionEmitsExitPhase(t *testing.T) {
	cfg := testConfig()
	if err := ApplyCognitionYAML(&cfg.Cognition, []byte(`
reactions:
  - id: lost-sight-alien
    match: { actor_noun: alien, action: present, channel: sight, role: witness, phase: exit }
    memory: { record: true, text: "Lost sight of {actor}." }
    affect: { impact: 0, fresh: { charge: 0, grip: 0, valence: 0 }, worn: { charge: 0, grip: 0, valence: 0 } }
    wear_policy: memory-occasions
`)); err != nil {
		t.Fatalf("add exit reaction: %v", err)
	}
	cfg.SyncWithCognition()
	w := newTestWorld(t, cfg)
	observer := w.spawn(Colonist, Point{2, 2})
	alien := w.spawn(Alien, Point{3, 2})

	w.observeNearby(observer)
	alien.Pos = Point{20, 20}
	w.observeNearby(observer)

	if got := observer.Memories[len(observer.Memories)-1].Rule; got != "lost-sight-alien" {
		t.Fatalf("exit memory rule = %q, want lost-sight-alien", got)
	}
	if observer.seesThreat {
		t.Fatal("exit left threat-presence cache active")
	}
}

func TestTraitRulesMatchPerceptGrammar(t *testing.T) {
	w, colonist := focusTestColonist(t)
	colonist.Profile = &Profile{Traits: []Trait{TraitTidy}}
	reaction := testReaction(w, "saw-mouse")
	w.cognition.TraitRules = append(w.cognition.TraitRules, TraitRule{
		ID: "tidy-saw-mouse-test", Trait: TraitTidy, Match: reaction.Match,
		Impact: 100, Charge: 220, Grip: 220, Valence: 220, WearRate: 100,
	})
	o := Occurrence{
		Actor:  FactRef{Noun: NounMouse, Entity: 99, Label: "mouse #99"},
		Action: ActionPresent,
		Text:   "Saw a bloody mouse.",
	}
	w.rememberPercept(colonist, Percept{
		Observer: colonist.ID, Channel: ChannelSight, Role: RoleWitness,
		Phase: PhaseEnter, Occurrence: o,
	})

	if colonist.affect.Charge != reaction.Fresh.Charge*220/100 ||
		colonist.affect.Grip != reaction.Fresh.Grip*220/100 {
		t.Fatalf("grammar trait rule did not apply: %+v", colonist.affect)
	}
}

func TestFatalBiteComputesFriendRelationBeforeRemoval(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)
	alien := w.spawn(Alien, Point{5, 5})
	victim := w.spawn(Colonist, Point{6, 5})
	witness := w.spawn(Colonist, Point{6, 6})
	witness.Profile = &Profile{Traits: []Trait{TraitExtrovert}}
	w.addAffinity(witness.ID, victim.ID, cfg.MoodFriendAffinity+5)
	victim.HP = 1
	for part := BodyPart(0); part < numBodyParts; part++ {
		victim.Parts[part] = 1
	}

	w.bite(alien, victim)

	if w.entities[victim.ID] != nil {
		t.Fatal("fatal bite did not remove victim")
	}
	if witness.affect.Charge != 91 || witness.affect.Valence != -90 {
		t.Fatalf("friend-loss trait did not see relation before removal: affect %+v", witness.affect)
	}
	if got := witness.Memories[len(witness.Memories)-1].Rule; got != "witnessed-colonist-killed" {
		t.Fatalf("fatal bite memory rule = %q", got)
	}
}

func TestPersistentPerceptionUsesDeterministicEntityOrder(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)
	observer := w.spawn(Colonist, Point{10, 10})
	first := w.spawn(Mouse, Point{12, 10})
	second := w.spawn(Mouse, Point{8, 10})

	w.observeNearby(observer)

	if len(observer.Memories) != 2 {
		t.Fatalf("mouse memories = %d, want 2", len(observer.Memories))
	}
	if observer.Memories[0].Text != "Saw mouse #"+fmt.Sprint(first.ID)+"." ||
		observer.Memories[1].Text != "Saw mouse #"+fmt.Sprint(second.ID)+"." {
		t.Fatalf("persistent order = %+v, want entity ID order", observer.Memories)
	}
}

func TestRestingFastPathDoesNotSkipCustomPersistentRule(t *testing.T) {
	cfg := testConfig()
	if err := ApplyCognitionYAML(&cfg.Cognition, []byte(`
perceptions:
  - id: visible-cat
    match: { actor_noun: cat, action: present }
    sense: { channel: sight, role: witness, distance: 4, cadence: enter }
reactions:
  - id: saw-cat
    match: { actor_noun: cat, action: present, channel: sight, role: witness, phase: enter }
    memory: { record: true }
    affect: { impact: 1, fresh: { charge: 0, grip: 1, valence: 0 }, worn: { charge: 0, grip: 0, valence: 0 } }
    wear_policy: memory-occasions
`)); err != nil {
		t.Fatalf("custom persistent config: %v", err)
	}
	cfg.SyncWithCognition()
	w := newTestWorld(t, cfg)
	observer := w.spawn(Colonist, Point{5, 5})
	w.spawn(Cat, Point{6, 5})
	observer.focus, observer.Job = FocusIdle, JobNone
	observer.resting, observer.wakeTick = true, w.tick+100
	observer.mindDirty, observer.nextThinkTick = false, w.tick+100

	if w.tryCognitionFastPath(observer) {
		t.Fatal("resting fast path skipped custom persistent perception")
	}
	w.observeNearby(observer)
	if got := observer.Memories[len(observer.Memories)-1].Rule; got != "saw-cat" {
		t.Fatalf("custom persistent memory = %q", got)
	}
}
