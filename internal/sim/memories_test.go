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

	// EvtConversation, not one of the routine kinds: a collapsible kind would
	// fold these into a single memory and never reach the cap at all (see
	// TestRepeatedMinorEventsCollapse).
	for i := 0; i < maxColonistMemories+3; i++ {
		w.tick = i
		w.remember(col, event(EvtConversation, "event"))
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

// A colonist grinding through a mining shift should end up with one memory
// standing for the whole run — spanning first dig to last, counting them —
// rather than a dozen near-identical lines.
func TestRepeatedMinorEventsCollapse(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	col := w.spawn(Colonist, Point{0, 0})
	for i := 0; i < 12; i++ {
		w.tick = 100 + i*7
		w.remember(col, event(EvtFinishedMining, "Finished mining at (%d, %d).", i, i))
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
	if m.Kind != EvtFinishedMining {
		t.Errorf("collapsed kind = %v, want EvtFinishedMining", m.Kind)
	}
}

// A single occurrence of a collapsible kind keeps its own specific text — the
// generic wording is what a *run* reads as, not a tax on every routine event.
func TestSingleMinorEventKeepsItsOwnText(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	col := w.spawn(Colonist, Point{0, 0})
	w.remember(col, event(EvtFinishedMining, "Finished mining at (514, 501)."))

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
	w.remember(col, event(EvtFinishedMining, "Finished mining at (1, 1)."))
	w.tick = 20
	w.remember(col, event(EvtFinishedMining, "Finished mining at (2, 2)."))
	w.tick = 30
	w.remember(col, event(EvtAte, "Had a meal."))
	w.tick = 40
	w.remember(col, event(EvtFinishedMining, "Finished mining at (3, 3)."))

	if got, want := len(col.Memories), 3; got != want {
		t.Fatalf("memory count = %d, want %d", got, want)
	}
	if col.Memories[0].Count != 2 || col.Memories[0].Kind != EvtFinishedMining {
		t.Errorf("first memory = {Kind: %v, Count: %d}, want the two-dig run", col.Memories[0].Kind, col.Memories[0].Count)
	}
	if col.Memories[1].Kind != EvtAte || col.Memories[1].Count != 1 {
		t.Errorf("second memory = {Kind: %v, Count: %d}, want a single meal", col.Memories[1].Kind, col.Memories[1].Count)
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
	w.remember(col, event(EvtConversation, "Had a conversation with Ada."))
	w.remember(col, event(EvtConversation, "Had a conversation with Bo."))

	if got, want := len(col.Memories), 2; got != want {
		t.Fatalf("memory count = %d, want %d", got, want)
	}
	if col.Memories[0].Text == col.Memories[1].Text {
		t.Error("collapsing overwrote a conversation's text; each names its own partner")
	}
}

// A collapsed run still applies mood per occurrence rather than once — but
// each repeat is worth less than the one before, so a run lifts mood by more
// than a single job and by less than the naive multiple.
func TestCollapsedRunAppliesDiminishingMood(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	once := w.spawn(Colonist, Point{0, 0})
	once.Profile = &Profile{}
	thrice := w.spawn(Colonist, Point{10, 10})
	thrice.Profile = &Profile{}

	w.remember(once, event(EvtFinishedMining, "Finished mining at (1, 1)."))
	for i := 0; i < 3; i++ {
		w.remember(thrice, event(EvtFinishedMining, "Finished mining at (%d, %d).", i, i))
	}

	if once.mood <= 0 {
		t.Fatalf("mood after one job = %d, want positive", once.mood)
	}
	if thrice.mood <= once.mood {
		t.Errorf("mood after three jobs = %d, want more than one job's %d", thrice.mood, once.mood)
	}
	if thrice.mood >= 3*once.mood {
		t.Errorf("mood after three jobs = %d, want less than three times one job's %d", thrice.mood, once.mood)
	}
}

// Each consecutive repeat of a kind should move mood less than the one before
// it, and a long enough run should stop moving mood at all — otherwise a
// colonist parked on the mining frontier ratchets to MoodMax and stays there,
// since mood has no time decay of its own.
func TestRepeatedEventMoodDecaysToNothing(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	col := w.spawn(Colonist, Point{0, 0})
	col.Profile = &Profile{}

	var steps []int
	prev := col.mood
	for i := 0; i < len(repeatMoodDecay)+2; i++ {
		w.remember(col, event(EvtFinishedMining, "Finished mining at (%d, %d).", i, i))
		steps = append(steps, col.mood-prev)
		prev = col.mood
	}

	if steps[0] <= 0 {
		t.Fatalf("first job moved mood by %d, want positive", steps[0])
	}
	for i := 1; i < len(steps); i++ {
		if steps[i] > steps[i-1] {
			t.Errorf("job %d moved mood by %d, more than job %d's %d; the curve should never rise", i+1, steps[i], i, steps[i-1])
		}
	}
	if last := steps[len(steps)-1]; last != 0 {
		t.Errorf("job past the end of the decay curve moved mood by %d, want 0", last)
	}
}

// Doing something else resets the streak: coming back to a job after a break
// is worth full value again.
func TestBreakingAStreakRestoresFullMood(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	col := w.spawn(Colonist, Point{0, 0})
	col.Profile = &Profile{}

	w.remember(col, event(EvtFinishedMining, "Finished mining at (1, 1)."))
	first := col.mood
	for i := 0; i < len(repeatMoodDecay); i++ {
		w.remember(col, event(EvtFinishedMining, "Finished mining at (%d, %d).", i, i))
	}
	worn := col.mood
	w.remember(col, event(EvtFinishedMining, "Finished mining at (9, 9)."))
	if col.mood != worn {
		t.Fatalf("mood still moving at the end of a long run: %d -> %d", worn, col.mood)
	}

	// EvtSlept has no mood effect of its own, so any change after it is the
	// next dig being worth full value again rather than the nap itself.
	w.remember(col, event(EvtSlept, "Slept in a bed."))
	w.remember(col, event(EvtFinishedMining, "Finished mining at (5, 5)."))

	if got := col.mood - worn; got != first {
		t.Errorf("first job after a break moved mood by %d, want a full %d", got, first)
	}
}

// Diminishing returns discount what the mood table declares, not a delta the
// caller computed for this occasion: conversations already model repetition
// with their own social-fatigue window, and shouldn't be taxed twice.
func TestComputedMoodIsNotDiscountedByRepeats(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	col := w.spawn(Colonist, Point{0, 0})
	col.Profile = &Profile{}

	const perTalk = 5
	for i := 0; i < len(repeatMoodDecay)+2; i++ {
		w.remember(col, eventMood(EvtConversation, perTalk, "Had a conversation."))
	}

	if want := perTalk * (len(repeatMoodDecay) + 2); col.mood != want {
		t.Errorf("mood after %d conversations = %d, want %d undiscounted", len(repeatMoodDecay)+2, col.mood, want)
	}
}

// The decay curve is a discount, not a sign flip: habituation to a run of bad
// news softens it toward zero, it never turns a penalty into a reward.
func TestRepeatDecayNeverFlipsSign(t *testing.T) {
	for _, delta := range []int{-14, -6, -5, -1, 1, 2, 6, 15} {
		for repeat := 1; repeat <= len(repeatMoodDecay)+2; repeat++ {
			got := scaleMood(delta, repeatMoodPercent(repeat))
			if delta > 0 && (got < 0 || got > delta) {
				t.Errorf("scaleMood(%d, repeat %d) = %d, want within [0, %d]", delta, repeat, got, delta)
			}
			if delta < 0 && (got > 0 || got < delta) {
				t.Errorf("scaleMood(%d, repeat %d) = %d, want within [%d, 0]", delta, repeat, got, delta)
			}
		}
	}
}
