package sim

import (
	"math/rand"
	"strings"
	"testing"
)

func TestLoadSchedulesValid(t *testing.T) {
	data := []byte(`
schedules:
  - name: "first winter"
    earliest_tick: 100
    latest_tick: 200
    occurrences:
      - kind: mouse-plague
        count: 25
      - kind: alien-swarm
        count: 4
      - kind: supply-drop
        pistols: 2
        shotguns: 1
`)
	schedules, err := LoadSchedules(data, "test.yaml")
	if err != nil {
		t.Fatalf("LoadSchedules: %v", err)
	}
	if len(schedules) != 1 {
		t.Fatalf("got %d schedules, want 1", len(schedules))
	}
	s := schedules[0]
	if s.Name != "first winter" || s.EarliestTick != 100 || s.LatestTick != 200 {
		t.Fatalf("schedule = %+v, want name/window 100..200", s)
	}
	if len(s.Occurrences) != 3 {
		t.Fatalf("got %d occurrences, want 3", len(s.Occurrences))
	}
	want := []Occurrence{
		{Kind: OccMousePlague, Count: 25},
		{Kind: OccAlienSwarm, Count: 4},
		{Kind: OccSupplyDrop, Pistols: 2, Shotguns: 1},
	}
	for i, w := range want {
		if s.Occurrences[i] != w {
			t.Errorf("occurrence %d = %+v, want %+v", i, s.Occurrences[i], w)
		}
	}
}

func TestLoadSchedulesEmpty(t *testing.T) {
	schedules, err := LoadSchedules(nil, "test.yaml")
	if err != nil {
		t.Fatalf("LoadSchedules(nil): %v", err)
	}
	if schedules != nil {
		t.Fatalf("got %v, want nil", schedules)
	}
}

func TestLoadSchedulesErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			"missing name",
			`schedules: [{earliest_tick: 0, latest_tick: 10, occurrences: [{kind: mouse-plague, count: 1}]}]`,
			"name is required",
		},
		{
			"latest before earliest",
			`schedules: [{name: x, earliest_tick: 100, latest_tick: 50, occurrences: [{kind: mouse-plague, count: 1}]}]`,
			"cannot be before earliest_tick",
		},
		{
			"negative earliest",
			`schedules: [{name: x, earliest_tick: -1, latest_tick: 10, occurrences: [{kind: mouse-plague, count: 1}]}]`,
			"cannot be negative",
		},
		{
			"no occurrences",
			`schedules: [{name: x, earliest_tick: 0, latest_tick: 10, occurrences: []}]`,
			"at least one occurrence",
		},
		{
			"unknown kind",
			`schedules: [{name: x, earliest_tick: 0, latest_tick: 10, occurrences: [{kind: zombie-outbreak, count: 1}]}]`,
			`unknown kind "zombie-outbreak"`,
		},
		{
			"zero count",
			`schedules: [{name: x, earliest_tick: 0, latest_tick: 10, occurrences: [{kind: alien-swarm, count: 0}]}]`,
			"needs count >= 1",
		},
		{
			"empty supply drop",
			`schedules: [{name: x, earliest_tick: 0, latest_tick: 10, occurrences: [{kind: supply-drop}]}]`,
			"at least one pistol or shotgun",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadSchedules([]byte(tc.yaml), "test.yaml")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// resolveSchedules must draw from the simulation stream deterministically: the
// same seed and the same schedule list produce the same firings every time.
func TestResolveSchedulesDeterministic(t *testing.T) {
	schedules := []Schedule{
		{
			Name: "a", EarliestTick: 100, LatestTick: 500,
			Occurrences: []Occurrence{
				{Kind: OccMousePlague, Count: 10},
				{Kind: OccAlienSwarm, Count: 5},
				{Kind: OccSupplyDrop, Pistols: 1},
			},
		},
		{
			Name: "b", EarliestTick: 1000, LatestTick: 1000, // a zero-width window
			Occurrences: []Occurrence{{Kind: OccMousePlague, Count: 3}},
		},
	}

	first := resolveSchedules(schedules, rand.New(rand.NewSource(7)))
	second := resolveSchedules(schedules, rand.New(rand.NewSource(7)))
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("got %d/%d events, want 2/2", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("event %d differs across identical seeds: %+v vs %+v", i, first[i], second[i])
		}
	}

	ev := first[0]
	if ev.Tick < 100 || ev.Tick > 500 {
		t.Errorf("tick %d outside window [100, 500]", ev.Tick)
	}
	if first[1].Tick != 1000 {
		t.Errorf("zero-width window resolved to tick %d, want 1000", first[1].Tick)
	}
	// Sorted ascending by tick.
	if first[0].Tick > first[1].Tick {
		t.Errorf("events not sorted by tick: %+v", first)
	}
}

func schedulesTestConfig(schedules []Schedule) Config {
	c := testConfig()
	c.Schedules = schedules
	return c
}

func TestDirectorFiresMousePlague(t *testing.T) {
	cfg := schedulesTestConfig([]Schedule{
		{
			Name: "plague", EarliestTick: 1, LatestTick: 1,
			Occurrences: []Occurrence{{Kind: OccMousePlague, Count: 15}},
		},
	})
	w := newTestWorld(t, cfg)
	before := w.countKind(Mouse)

	w.step()

	after := w.countKind(Mouse)
	if after != before+15 {
		t.Fatalf("mice after plague = %d, want %d (%d before + 15)", after, before+15, before)
	}
}

func TestDirectorFiresAlienSwarm(t *testing.T) {
	cfg := schedulesTestConfig([]Schedule{
		{
			Name: "swarm", EarliestTick: 1, LatestTick: 1,
			Occurrences: []Occurrence{{Kind: OccAlienSwarm, Count: 6}},
		},
	})
	// Big enough for hidden caves, so the swarm lands dormant in them rather
	// than on colony floor, where an armed colonist could shoot one the same
	// tick and throw the count off.
	cfg.Width, cfg.Height = 120, 60
	w := newTestWorld(t, cfg)
	before := w.countKind(Alien)

	w.step()

	after := w.countKind(Alien)
	if after != before+6 {
		t.Fatalf("aliens after swarm = %d, want %d (%d before + 6)", after, before+6, before)
	}
}

func TestDirectorFiresSupplyDrop(t *testing.T) {
	cfg := schedulesTestConfig([]Schedule{
		{
			Name: "airdrop", EarliestTick: 1, LatestTick: 1,
			Occurrences: []Occurrence{{Kind: OccSupplyDrop, Pistols: 3, Shotguns: 2}},
		},
	})
	w := newTestWorld(t, cfg)

	countWeapons := func(kind ItemKind) int {
		n := 0
		for _, e := range w.entities {
			if e.Kind == Colonist {
				n += e.Inventory.Count(kind)
			}
		}
		return n
	}
	beforePistols, beforeShotguns := countWeapons(Pistol), countWeapons(Shotgun)

	w.step()

	if got := countWeapons(Pistol); got != beforePistols+3 {
		t.Errorf("pistols among colonists = %d, want %d", got, beforePistols+3)
	}
	if got := countWeapons(Shotgun); got != beforeShotguns+2 {
		t.Errorf("shotguns among colonists = %d, want %d", got, beforeShotguns+2)
	}
}

// A schedule's window can span many ticks; the event must fire exactly once,
// on the tick it resolved to, not before and not again after. Population
// counts alone are not a reliable signal here (mice breed and starve on
// their own), so this checks the director's own progress through its queue
// instead.
func TestDirectorFiresExactlyOnceAtResolvedTick(t *testing.T) {
	cfg := schedulesTestConfig([]Schedule{
		{
			Name: "someday", EarliestTick: 5, LatestTick: 5,
			Occurrences: []Occurrence{{Kind: OccMousePlague, Count: 4}},
		},
	})
	w := newTestWorld(t, cfg)

	for i := 0; i < 4; i++ { // ticks 1-4: too early
		w.step()
		if w.directorNext != 0 {
			t.Fatalf("tick %d: directorNext = %d, want 0 (fired early)", w.tick, w.directorNext)
		}
	}
	w.step() // tick 5: fires
	if w.directorNext != 1 {
		t.Fatalf("tick 5: directorNext = %d, want 1 (should have fired)", w.directorNext)
	}
	for i := 0; i < 5; i++ { // later ticks: must not fire again
		w.step()
		if w.directorNext != 1 {
			t.Fatalf("tick %d: directorNext = %d, want 1 (fired again)", w.tick, w.directorNext)
		}
	}
}

// One of several candidates in a schedule fires, never more than one.
func TestDirectorFiresOneOfSeveralCandidates(t *testing.T) {
	cfg := schedulesTestConfig([]Schedule{
		{
			Name: "one-of", EarliestTick: 1, LatestTick: 1,
			Occurrences: []Occurrence{
				{Kind: OccMousePlague, Count: 100},
				{Kind: OccAlienSwarm, Count: 100},
			},
		},
	})
	w := newTestWorld(t, cfg)
	beforeMice, beforeAliens := w.countKind(Mouse), w.countKind(Alien)

	w.step()

	miceFired := w.countKind(Mouse) > beforeMice
	aliensFired := w.countKind(Alien) > beforeAliens
	if miceFired == aliensFired {
		t.Fatalf("exactly one candidate should fire: mice fired=%v, aliens fired=%v", miceFired, aliensFired)
	}
}
