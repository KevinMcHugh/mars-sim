package sim

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func TestRememberKeepsRecentMemories(t *testing.T) {
	cfg := DefaultConfig()
	w := newWorld(cfg, rand.New(rand.NewSource(1)))
	col := newEntity(1, Colonist, Point{}, cfg)
	w.entities[col.ID] = col

	for i := 0; i < maxColonistMemories+3; i++ {
		w.tick = i
		w.remember(col, event(EvtNeedSatisfied, "event"))
	}
	if got, want := len(col.Memories), maxColonistMemories; got != want {
		t.Fatalf("memory count = %d, want %d", got, want)
	}
	if got, want := col.Memories[0].Tick, 3; got != want {
		t.Fatalf("oldest retained tick = %d, want %d", got, want)
	}
}

// A colonist close enough to notice an alien should remember watching it
// attack another colonist, whether or not the victim survives the bite.
func TestBystanderRemembersAlienAttack(t *testing.T) {
	cfg := DefaultConfig()
	w := newWorld(cfg, rand.New(rand.NewSource(1)))

	alien := w.spawn(Alien, Point{0, 0})
	victim := w.spawn(Colonist, Point{1, 0})
	bystander := w.spawn(Colonist, Point{1, 1})

	victim.HP = cfg.AlienDamage + 1 // survives this bite
	w.bite(alien, victim)

	// The exact body part hit is an RNG detail (see rollHit); only the shape
	// of the message is pinned here.
	if got := lastMemory(victim); !strings.HasPrefix(got, "Bitten in the ") || !strings.HasSuffix(got, " by an alien!") {
		t.Fatalf("victim memory = %q, want a %q..%q message", got, "Bitten in the ", " by an alien!")
	}
	wantWitness := "Watched an alien attack " + victim.displayName() + "."
	if got := lastMemory(bystander); got != wantWitness {
		t.Fatalf("bystander memory = %q, want %q", got, wantWitness)
	}

	// A second, fatal bite: the victim is removed and cannot hold a memory,
	// but the bystander should remember watching the kill.
	victim2 := w.spawn(Colonist, Point{1, 0})
	victim2.HP = cfg.AlienDamage
	name := victim2.displayName()
	w.bite(alien, victim2)

	if w.entities[victim2.ID] != nil {
		t.Fatal("fatally bitten colonist should have been removed")
	}
	wantKillWitness := "Watched an alien kill " + name + "."
	if got := lastMemory(bystander); got != wantKillWitness {
		t.Fatalf("bystander memory after kill = %q, want %q", got, wantKillWitness)
	}
}

// A colonist too far away to have noticed the alien should not gain a
// memory of the attack.
func TestDistantColonistDoesNotWitnessAlienAttack(t *testing.T) {
	cfg := DefaultConfig()
	w := newWorld(cfg, rand.New(rand.NewSource(1)))

	alien := w.spawn(Alien, Point{0, 0})
	victim := w.spawn(Colonist, Point{1, 0})
	far := w.spawn(Colonist, Point{0, cfg.FleeRadius + 5})

	victim.HP = cfg.AlienDamage + 1
	w.bite(alien, victim)

	if len(far.Memories) != 0 {
		t.Fatalf("distant colonist should not witness the attack, got memories %v", far.Memories)
	}
}

// A bystander close enough to have noticed a mouse should remember watching
// it get crushed underfoot or caught by a cat.
func TestBystanderRemembersMouseKilled(t *testing.T) {
	cfg := DefaultConfig()
	w := newWorld(cfg, rand.New(rand.NewSource(1)))

	colonist := w.spawn(Colonist, Point{0, 0})
	bystander := w.spawn(Colonist, Point{0, 1})
	mouse := w.spawn(Mouse, Point{1, 0})
	w.stomp(colonist, mouse)

	want := fmt.Sprintf("Watched a colonist crush mouse #%d.", mouse.ID)
	if got := lastMemory(bystander); got != want {
		t.Fatalf("bystander memory after stomp = %q, want %q", got, want)
	}

	cat := w.spawn(Cat, Point{0, 0})
	mouse2 := w.spawn(Mouse, Point{1, 0})
	w.pounce(cat, mouse2)

	want2 := fmt.Sprintf("Watched a cat catch mouse #%d.", mouse2.ID)
	if got := lastMemory(bystander); got != want2 {
		t.Fatalf("bystander memory after pounce = %q, want %q", got, want2)
	}
}

func lastMemory(e *Entity) string {
	if len(e.Memories) == 0 {
		return ""
	}
	return e.Memories[len(e.Memories)-1].Text
}

func TestSnapshotCopiesMemories(t *testing.T) {
	cfg := DefaultConfig()
	w := newWorld(cfg, rand.New(rand.NewSource(1)))
	col := newEntity(1, Colonist, Point{}, cfg)
	w.entities[col.ID] = col
	w.remember(col, event(EvtAte, "a meal"))

	snap := w.snapshot(false, 1)
	snap.Entities[0].Memories[0].Text = "mutated"
	if got := col.Memories[0].Text; got != "a meal" {
		t.Fatalf("snapshot mutation changed live memory to %q", got)
	}
}
