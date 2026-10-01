package tui

import (
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	tea "github.com/charmbracelet/bubbletea"
)

func flowSnapshot() *sim.Snapshot {
	snap := makeSnapshot()
	tiles := make([]sim.Tile, snap.Width*snap.Height)
	for i := range tiles {
		tiles[i].Terrain = sim.Floor
	}
	snap.Tiles = sim.NewTileGrid(snap.Width, snap.Height, tiles)
	snap.Entities = nil
	snap.FlowFields = []sim.FlowFieldRef{{Facility: sim.Toilet}, {Frontier: true}}
	return snap
}

// f cycles the map through each field the engine offers and then off, the
// overlay writes distances on bare floor, and the sidebar names the field.
func TestFlowFieldOverlayCycles(t *testing.T) {
	restoreGlyphs(t, false)
	snap := flowSnapshot()
	press := func(m tea.Model, r rune) tea.Model {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return m
	}
	var model tea.Model = New(nil, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(snapshotMsg{snap: snap})

	model = press(model, 'f')
	if m := model.(Model); !m.flowOn || m.flowRef != snap.FlowFields[0] {
		t.Fatalf("first f should show the toilet field, got on=%v %+v", m.flowOn, m.flowRef)
	}
	// Until the engine publishes the field, nothing is drawn for it.
	if out := model.View(); !strings.Contains(out, "waiting for the engine") {
		t.Fatalf("expected the sidebar to wait for the field:\n%s", out)
	}

	snap.FlowField = sim.NewFlowFieldView(snap.FlowFields[0], snap.Width, snap.Height,
		map[sim.Point]int32{{X: 0, Y: 0}: 0, {X: 1, Y: 0}: 1, {X: 2, Y: 0}: 17})
	model, _ = model.Update(snapshotMsg{snap: snap})
	out := model.View()
	for _, want := range []string{"FLOW FIELD", "toilet", "goal tiles   1", "farthest     17 steps", " 0 117", "flow field: toilet"} {
		if !strings.Contains(out, want) {
			t.Fatalf("overlay missing %q:\n%s", want, out)
		}
	}

	// A frame still carrying the old field draws nothing for the next one.
	model = press(model, 'f')
	if m := model.(Model); !m.flowRef.Frontier || m.shownFlowField() != nil {
		t.Fatalf("second f should move to the frontier field and drop the stale toilet view")
	}
	model = press(model, 'f')
	if model.(Model).flowOn {
		t.Fatal("f past the last field should turn the overlay off")
	}
	if out := model.View(); strings.Contains(out, "FLOW FIELD") || !strings.Contains(out, "LEGEND") {
		t.Fatalf("overlay off should restore the legend:\n%s", out)
	}

	model = press(model, 'f')
	model = press(model, 'F')
	if model.(Model).flowOn {
		t.Fatal("F should turn the overlay off")
	}
}

// The inspect cursor reports the shown field's distance in the footer.
func TestFlowFieldInspectShowsDistance(t *testing.T) {
	snap := flowSnapshot()
	snap.FlowField = sim.NewFlowFieldView(snap.FlowFields[0], snap.Width, snap.Height,
		map[sim.Point]int32{{X: 3, Y: 2}: 4})
	var model tea.Model = New(nil, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(snapshotMsg{snap: snap})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m := model.(Model)
	m.cursor = sim.Point{X: 3, Y: 2}
	if out := m.View(); !strings.Contains(out, "toilet 4 steps") {
		t.Fatalf("footer should give the cursor's distance:\n%s", out)
	}
	m.cursor = sim.Point{X: 0, Y: 0}
	if out := m.View(); !strings.Contains(out, "toilet unreachable") {
		t.Fatalf("footer should call an unreached tile unreachable:\n%s", out)
	}
}
