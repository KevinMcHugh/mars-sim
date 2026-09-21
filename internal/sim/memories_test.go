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

	// conversation, not one of the routine reactions: a collapsible reaction would
	// fold these into a single memory and never reach the cap at all (see
	// TestRepeatedMinorEventsCollapse).
	for i := 0; i < maxColonistMemories+3; i++ {
		w.tick = i
		rememberTest(w, col, "conversation", "event")
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
	rememberTest(w, col, "ate", "a meal")

	snap := w.snapshot(false, 1)
	snap.Entities[0].Memories[0].Text = "mutated"
	if got := col.Memories[0].Text; got != "a meal" {
		t.Fatalf("snapshot mutation changed live memory to %q", got)
	}
}

// A colonist grinding through a mining shift should end up with one memory
// standing for the whole run — spanning first dig to last, counting them —
// rather than a dozen near-identical lines.
func TestRepeatedMinorEventsCollapse(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	col := w.spawn(Colonist, Point{0, 0})
	for i := 0; i < 12; i++ {
		w.tick = 100 + i*7
		rememberTest(w, col, "finished-mining", fmt.Sprintf("Finished mining at (%d, %d).", i, i))
	}

	if got, want := len(col.Memories), 1; got != want {
		t.Fatalf("memory count = %d, want %d", got, want)
	}
	m := col.Memories[0]
	if m.Count != 12 {
		t.Errorf("collapsed count = %d, want 12", m.Count)
	}
	if m.Tick != 100 || m.LastTick != 100+11*7 {
		t.Errorf("collapsed span = t%d-%d, want t%d-%d", m.Tick, m.LastTick, 100, 100+11*7)
	}
	// The per-occurrence coordinates are gone: a run is about the repetition,
	// not about which tile the eleventh dig was on.
	if want := "Finished mining."; m.Text != want {
		t.Errorf("collapsed text = %q, want %q", m.Text, want)
	}
	if m.Rule != "finished-mining" {
		t.Errorf("collapsed rule = %v, want finished-mining", m.Rule)
	}
}

// A single occurrence of a collapsible kind keeps its own specific text — the
// generic wording is what a *run* reads as, not a tax on every routine event.
func TestSingleMinorEventKeepsItsOwnText(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	col := w.spawn(Colonist, Point{0, 0})
	rememberTest(w, col, "finished-mining", "Finished mining at (514, 501).")

	m := col.Memories[0]
	if want := "Finished mining at (514, 501)."; m.Text != want {
		t.Errorf("text = %q, want %q", m.Text, want)
	}
	if m.Count != 1 || m.LastTick != m.Tick {
		t.Errorf("uncollapsed memory = {Count: %d, Tick: %d, LastTick: %d}, want Count 1 and LastTick == Tick", m.Count, m.Tick, m.LastTick)
	}
}

// Runs are consecutive: doing something else in the middle of a mining shift
// breaks the run, so the log still shows the order things happened in.
func TestDifferentEventBreaksACollapsedRun(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	col := w.spawn(Colonist, Point{0, 0})
	w.tick = 10
	rememberTest(w, col, "finished-mining", "Finished mining at (1, 1).")
	w.tick = 20
	rememberTest(w, col, "finished-mining", "Finished mining at (2, 2).")
	w.tick = 30
	rememberTest(w, col, "ate", "Had a meal.")
	w.tick = 40
	rememberTest(w, col, "finished-mining", "Finished mining at (3, 3).")

	if got, want := len(col.Memories), 3; got != want {
		t.Fatalf("memory count = %d, want %d", got, want)
	}
	if col.Memories[0].Count != 2 || col.Memories[0].Rule != "finished-mining" {
		t.Errorf("first memory = {Rule: %v, Count: %d}, want the two-dig run", col.Memories[0].Rule, col.Memories[0].Count)
	}
	if col.Memories[1].Rule != "ate" || col.Memories[1].Count != 1 {
		t.Errorf("second memory = {Rule: %v, Count: %d}, want a single meal", col.Memories[1].Rule, col.Memories[1].Count)
	}
	if col.Memories[2].Count != 1 || col.Memories[2].Tick != 40 {
		t.Errorf("third memory = {Count: %d, Tick: %d}, want a fresh run starting at t40", col.Memories[2].Count, col.Memories[2].Tick)
	}
}

// Notable events are never collapsed, even back to back: two conversations,
// two kills, or two bites are two distinct beats in a colonist's story.
func TestNotableEventsDoNotCollapse(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	col := w.spawn(Colonist, Point{0, 0})
	rememberTest(w, col, "conversation", "Had a conversation with Ada.")
	rememberTest(w, col, "conversation", "Had a conversation with Bo.")

	if got, want := len(col.Memories), 2; got != want {
		t.Fatalf("memory count = %d, want %d", got, want)
	}
	if col.Memories[0].Text == col.Memories[1].Text {
		t.Error("collapsing overwrote a conversation's text; each names its own partner")
	}
}

// Collapsing is a display decision, not an affect one: every completed job
// still applies its vector.
func TestCollapsedRunStillAppliesAffectPerOccurrence(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	once := w.spawn(Colonist, Point{0, 0})
	once.Profile = &Profile{}
	thrice := w.spawn(Colonist, Point{10, 10})
	thrice.Profile = &Profile{}

	rememberTest(w, once, "finished-mining", "Finished mining at (1, 1).")
	for i := 0; i < 3; i++ {
		rememberTest(w, thrice, "finished-mining", fmt.Sprintf("Finished mining at (%d, %d).", i, i))
	}

	if once.affect.Grip <= 0 {
		t.Fatalf("affect after one job = %+v, want positive grip", once.affect)
	}
	if thrice.affect.Charge != 3*once.affect.Charge || thrice.affect.Grip != 3*once.affect.Grip {
		t.Errorf("affect after three collapsed jobs = %+v, want three times %+v", thrice.affect, once.affect)
	}
}
