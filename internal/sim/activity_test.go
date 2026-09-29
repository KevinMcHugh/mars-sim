package sim

import "testing"

// A colonist walking somewhere is credited to what it is walking there to do
// (its job when the job names the work, else its focus) and marked as
// walking there; one visibly at the work is doing it.
func TestActivityCreditsTheWalkToItsPurpose(t *testing.T) {
	cases := []struct {
		state State
		job   JobKind
		focus FocusKind
		want  Activity
		walk  bool
	}{
		{Crafting, JobCraft, FocusWork, ActCooking, false},
		{Storing, JobStore, FocusWork, ActHauling, false},
		{Stomping, JobNone, FocusIdle, ActFighting, false},
		{Fleeing, JobNone, FocusFlee, ActFleeing, false},   // running is the fleeing
		{Building, JobBuild, FocusEat, ActBuilding, false}, // a hungry colonist building a pod is building
		{Moving, JobMine, FocusWork, ActMining, true},
		{Moving, JobCraft, FocusWork, ActCooking, true}, // on the way to cook
		{Moving, JobUse, FocusSleep, ActSleeping, true},
		{Moving, JobUse, FocusRelieve, ActRelieving, true},
		{Idle, JobMine, FocusWork, ActMining, false}, // waiting at the face is not walking
		{Idle, JobNone, FocusIdle, ActIdle, false},
	}
	for _, c := range cases {
		e := &Entity{Kind: Colonist, State: c.state, Job: c.job, focus: c.focus}
		if got, walk := activityOf(e); got != c.want || walk != c.walk {
			t.Errorf("%v/%v/%v: got %v (walking %v), want %v (walking %v)",
				c.state, c.job, c.focus, got, walk, c.want, c.walk)
		}
	}
}

// Every colonist-tick lands in exactly one sample's tally, including across
// the halvings that drop samples: the tallies over the history sum to the
// colonist-ticks up to its last sample.
func TestActivityTallyCoversEveryTick(t *testing.T) {
	w := propertyWorld(t)
	a := w.spawn(Colonist, Point{8, 8})
	b := w.spawn(Colonist, Point{10, 8})
	for tick := 1; tick <= 60000; tick++ {
		w.tick = tick
		a.State, a.Job = Mining, JobMine
		if tick%3 == 0 {
			a.State, a.Job = Moving, JobEat // fetching a meal
		}
		b.State = Sleeping
		w.tallyActivity(a)
		w.tallyActivity(b)
		w.samplePopulation()
	}
	if w.popEvery <= popFirstEvery {
		t.Fatal("the history never halved")
	}
	var total, walked [NumActivities]int
	prev := 0
	for _, s := range w.popHist {
		sum := 0
		for i, n := range s.Activity {
			total[i] += n
			walked[i] += s.Walking[i]
			sum += n
		}
		if want := 2 * (s.Tick - prev); sum != want {
			t.Fatalf("sample at tick %d tallies %d colonist-ticks, want %d", s.Tick, sum, want)
		}
		prev = s.Tick
	}
	last := w.popHist[len(w.popHist)-1].Tick
	if total[ActSleeping] != last || total[ActEating] != last/3 || total[ActMining] != last-last/3 {
		t.Fatalf("tallies %v over %d ticks: mining %d, eating %d, sleeping %d expected",
			total, last, last-last/3, last/3, last)
	}
	if walked != [NumActivities]int{ActEating: last / 3} {
		t.Fatalf("walking tallies %v, want all %d eating ticks and nothing else", walked, last/3)
	}
}

// In a real game the tally is taken during the tick, and colonists get up to
// more than one thing.
func TestActivityTalliedInPlay(t *testing.T) {
	w := newTestWorld(t, testConfig())
	for range 2 * popFirstEvery {
		w.step()
	}
	kinds, sum := 0, 0
	for _, n := range w.popHist[0].Activity {
		if n > 0 {
			kinds++
		}
		sum += n
	}
	if sum == 0 || kinds < 2 {
		t.Fatalf("first sample's activity %v: want colonist-ticks spread over several activities", w.popHist[0].Activity)
	}
}
