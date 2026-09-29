package tui

import (
	"os"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"
)

// activitySnapshot is populationSnapshot with activity tallies: four
// colonists sleeping a quarter of the time, then an alien raid in the second
// half that has the two miners fleeing instead. The cook spends two ticks in
// five walking to the scumhouse.
func activitySnapshot(n int) *sim.Snapshot {
	snap := populationSnapshot(n)
	for i := range snap.Population {
		a := &snap.Population[i].Activity
		a[sim.ActSleeping] = 50
		a[sim.ActMining] = 100
		a[sim.ActCooking] = 50
		snap.Population[i].Walking[sim.ActCooking] = 20
		if i >= n/2 {
			a[sim.ActMining] = 0
			a[sim.ActFleeing] = 100
		}
	}
	return snap
}

// The Activity tab draws the stacked chart and a legend with each activity's
// share now and over the game, and `c` switches to average colonists.
func TestActivityTabShowsSharesAndCounts(t *testing.T) {
	m := New(nil, nil)
	m.termW, m.termH = 120, 36
	m.latest = activitySnapshot(200)
	m.mode = modeActivity
	out := m.View()
	for _, want := range []string{"COLONIST ACTIVITY", "share of colonist time", "LEGEND",
		"sleeping", "cooking", "fleeing", "100%", "t50", "t10000", "█"} {
		if !strings.Contains(out, want) {
			t.Fatalf("activity tab missing %q:\n%s", want, out)
		}
	}
	// Fleeing is half the time now and a quarter over the game.
	if !legendRow(out, "fleeing", "50%", "25%", "0%") {
		t.Fatalf("fleeing's legend row is wrong:\n%s", out)
	}
	if !legendRow(out, "cooking", "25%", "25%", "40%") {
		t.Fatalf("cooking's legend row does not show its walking:\n%s", out)
	}
	if !legendRow(out, "building", "0%", "0%", "-") {
		t.Fatalf("an activity nobody did should show no walking share:\n%s", out)
	}
	for i, line := range strings.Split(out, "\n") {
		if w := cells.Width(line); w > m.termW {
			t.Fatalf("line %d is %d cells wide on a %d-cell terminal", i, w, m.termW)
		}
	}
	if os.Getenv("SHOW_ACTIVITY") != "" {
		t.Log("\n" + out)
	}

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	out = m.View()
	if !strings.Contains(out, "average colonists") || !legendRow(out, "fleeing", "2.0", "1.0") {
		t.Fatalf("counting mode did not show average colonists:\n%s", out)
	}
}

// legendRow reports whether a line names activity followed by values.
func legendRow(out, activity string, values ...string) bool {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == activity && i+len(values) < len(fields) &&
				slices.Equal(fields[i+1:i+1+len(values)], values) {
				return true
			}
		}
	}
	return false
}

// With no history yet, the tab says so rather than drawing empty axes.
func TestActivityTabBeforeAnySample(t *testing.T) {
	m := New(nil, nil)
	m.termW, m.termH = 100, 30
	m.latest = makeSnapshot()
	m.mode = modeActivity
	if out := m.View(); !strings.Contains(out, "no samples yet") {
		t.Fatalf("an empty history did not say so:\n%s", out)
	}
}

// Columns sum the samples they cover, so each column's ticks and tallies are
// the whole history's between them.
func TestActivityColumnsCoverTheHistory(t *testing.T) {
	samples := activitySnapshot(300).Population
	cols := activityColumns(samples, 40)
	ticks, fled := 0, 0
	for _, c := range cols {
		ticks += c.ticks
		fled += c.n[sim.ActFleeing]
	}
	if ticks != samples[len(samples)-1].Tick || fled != 150*100 {
		t.Fatalf("columns cover %d ticks and %d fleeing colonist-ticks, want %d and %d",
			ticks, fled, samples[len(samples)-1].Tick, 150*100)
	}
}
