package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"
)

// populationSnapshot is makeSnapshot with a population history: colonists
// arriving and dying, meals running down and back up, the colony growing.
func populationSnapshot(n int) *sim.Snapshot {
	snap := makeSnapshot()
	for i := 0; i < n; i++ {
		snap.Population = append(snap.Population, sim.PopulationSample{
			Tick:       (i + 1) * 50,
			Colonists:  6 + i/40 - i/90,
			Meals:      60 - i%50,
			ColonySize: 300 + 7*i,
			Fixtures:   18 + i/15,
		})
	}
	return snap
}

// The Population tab draws all four charts, each with its latest value, and
// labels the x axis in ticks from the first sample to the last.
func TestPopulationTabShowsTheFourCharts(t *testing.T) {
	m := New(nil, nil)
	m.termW, m.termH = 120, 36
	m.latest = populationSnapshot(200)
	m.mode = modePopulation
	out := m.View()
	for _, want := range []string{"COLONISTS", "now 8", "MEALS IN STORAGE", "now 11",
		"COLONY SIZE (floor tiles)", "now 1693", "FIXTURES", "now 31", "t50", "t10000"} {
		if !strings.Contains(out, want) {
			t.Fatalf("population tab missing %q:\n%s", want, out)
		}
	}
	for i, line := range strings.Split(out, "\n") {
		if w := cells.Width(line); w > m.termW {
			t.Fatalf("line %d is %d cells wide on a %d-cell terminal", i, w, m.termW)
		}
	}
	if os.Getenv("SHOW_POPULATION") != "" {
		t.Log("\n" + out)
	}
}

// With no history yet (the first ticks of a game), the tab says so rather
// than drawing empty axes.
func TestPopulationTabBeforeAnySample(t *testing.T) {
	m := New(nil, nil)
	m.termW, m.termH = 100, 30
	m.latest = makeSnapshot()
	m.mode = modePopulation
	if out := m.View(); !strings.Contains(out, "no samples yet") {
		t.Fatalf("an empty history did not say so:\n%s", out)
	}
}

// A history shorter than the plot is stretched across it, and a longer one is
// thinned, so the chart always spans the whole game.
func TestStretchSpansTheWholeHistory(t *testing.T) {
	samples := populationSnapshot(10).Population
	v := func(s sim.PopulationSample) int { return s.Tick }
	short := stretch(samples, 40, v)
	if short[0] != 50 || short[39] != 500 {
		t.Fatalf("stretched: first %v last %v, want 50 and 500", short[0], short[39])
	}
	long := stretch(populationSnapshot(1000).Population, 40, v)
	if long[0] != 50 || long[39] < 48000 {
		t.Fatalf("thinned: first %v last %v, want 50 and near 50000", long[0], long[39])
	}
}
