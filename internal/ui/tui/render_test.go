package tui

import (
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	tea "github.com/charmbracelet/bubbletea"
)

// makeSnapshot builds a tiny hand-authored frame for view tests.
func makeSnapshot() *sim.Snapshot {
	w, h := 6, 4
	tiles := make([]sim.Tile, w*h)
	for i := range tiles {
		tiles[i].Terrain = sim.Rock
	}
	// Carve a floor cell at (1,1).
	tiles[1*w+1].Terrain = sim.Floor

	return &sim.Snapshot{
		Tick:           7,
		Width:          w,
		Height:         h,
		Tiles:          tiles,
		TicksPerSecond: 8,
		Entities: []sim.EntityView{
			{ID: 1, Kind: sim.Colonist, Pos: sim.Point{X: 1, Y: 1}, HP: 40, MaxHP: 40, State: sim.Mining},
			{ID: 2, Kind: sim.Alien, Pos: sim.Point{X: 4, Y: 2}, HP: 30, MaxHP: 30, State: sim.Hunting},
		},
		Log: []string{"The colony ship settles onto the Martian crust."},
	}
}

// The view should render without panicking and place entity glyphs on the map.
func TestViewRendersEntities(t *testing.T) {
	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: makeSnapshot()})

	out := m.View()
	if !strings.Contains(out, glyphColonist) {
		t.Error("expected a colonist glyph in the rendered view")
	}
	if !strings.Contains(out, glyphAlien) {
		t.Error("expected an alien glyph in the rendered view")
	}
	if !strings.Contains(out, "MARS-SIM") {
		t.Error("expected the title in the header")
	}
}

// Before the first frame arrives the view should show a booting message, not
// crash on nil state.
func TestViewBeforeFirstFrame(t *testing.T) {
	m := New(nil, nil)
	if got := m.View(); !strings.Contains(got, "Booting") && !strings.Contains(got, "colony") {
		t.Errorf("unexpected pre-frame view: %q", got)
	}
}
