package sim

import "testing"

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
    affect: { impact: 10, target: { charge: 1, grip: 2, valence: 1 } }
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
    affect: { impact: 0, target: { charge: 0, grip: 0, valence: 0 } }
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

func TestDynamicOccurrenceTagsComposeWithTraitModifiers(t *testing.T) {
	w, colonist := focusTestColonist(t)
	colonist.Profile = &Profile{Traits: []Trait{TraitTidy}}
	reaction := testReaction(w, "saw-mouse")
	o := Occurrence{
		Actor:  FactRef{Noun: NounMouse, Entity: 99, Label: "mouse #99"},
		Action: ActionPresent,
		Tags:   []TagID{TagGore},
		Text:   "Saw a bloody mouse.",
	}
	w.rememberPercept(colonist, Percept{
		Observer: colonist.ID, Channel: ChannelSight, Role: RoleWitness,
		Phase: PhaseEnter, Occurrence: o,
	})

	if colonist.affect.Charge != reaction.Target.Charge*220/100 ||
		colonist.affect.Grip != reaction.Target.Grip*220/100 {
		t.Fatalf("dynamic gore tag did not apply Tidy modifier: %+v", colonist.affect)
	}
}
