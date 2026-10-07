package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// twoLevelSnapshot is makeSnapshot with a second level: a stair down at (2, 2)
// on the landing level, its foot below, an alien on level 2, and nothing of
// level 2's on the landing level.
func twoLevelSnapshot() *sim.Snapshot {
	snap := makeSnapshot()
	floor := func() []sim.Tile {
		tiles := make([]sim.Tile, snap.Width*snap.Height)
		for i := range tiles {
			tiles[i].Terrain = sim.Floor
		}
		return tiles
	}
	top, bottom := floor(), floor()
	top[2*snap.Width+2].Terrain = sim.StairDown
	bottom[2*snap.Width+2].Terrain = sim.StairUp
	snap.Tiles = sim.NewTileGrid(snap.Width, snap.Height, top)
	snap.LevelTiles = []*sim.TileGrid{nil, snap.Tiles, sim.NewTileGridOn(sim.LandingLevel+1, snap.Width, snap.Height, bottom)}
	snap.Entities = []sim.EntityView{
		{ID: 9, Kind: sim.Alien, Pos: sim.Point{X: 4, Y: 2, Level: sim.LandingLevel + 1}, HP: 5, MaxHP: 5},
	}
	return snap
}

// The map starts on the landing level, > goes down a level and < back up,
// and each level shows its own end of the stair and its own creatures.
func TestMapSwitchesLevels(t *testing.T) {
	restoreGlyphs(t, false)
	snap := twoLevelSnapshot()
	var model tea.Model = New(nil, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(snapshotMsg{snap: snap})
	view := model.View()
	if !strings.Contains(view, "landing level") {
		t.Fatalf("header does not name the landing level:\n%s", view)
	}
	// The legend shows each stair glyph once; the map adds its own end.
	if strings.Count(view, glyphStairDown) != 2 || strings.Count(view, glyphStairUp) != 1 {
		t.Fatalf("landing level should draw the top of the stair, not its foot:\n%s", view)
	}
	aliensUp := strings.Count(view, glyphAlien) // the header's count and the legend

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'>'}})
	view = model.View()
	if !strings.Contains(view, "level 2") {
		t.Fatalf("> did not go down to level 2:\n%s", view)
	}
	if strings.Count(view, glyphStairUp) != 2 || strings.Count(view, glyphStairDown) != 1 {
		t.Fatalf("level 2 should draw the foot of the stair:\n%s", view)
	}
	if strings.Count(view, glyphAlien) != aliensUp+1 {
		t.Fatalf("the alien on level 2 should be drawn there, and only there:\n%s", view)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'>'}})
	if !strings.Contains(model.View(), "level 2") {
		t.Fatal("> went past the deepest level the colony has")
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'<'}})
	if !strings.Contains(model.View(), "landing level") {
		t.Fatal("< did not come back up")
	}
}

// A one-level game shows no level in the header and no stair legend.
func TestOneLevelShowsNoLevel(t *testing.T) {
	restoreGlyphs(t, false)
	var model tea.Model = New(nil, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(snapshotMsg{snap: makeSnapshot()})
	view := model.View()
	if strings.Contains(view, "landing level") || strings.Contains(view, glyphStairDown) {
		t.Fatalf("a one-level game mentions levels:\n%s", view)
	}
}
