package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	tea "github.com/charmbracelet/bubbletea"
)

// fogExplored is the explored window of fogSnapshot: the top-left corner of the
// map, leaving the rest of it dark.
func fogExplored(p sim.Point) bool { return p.X <= 4 && p.Y <= 4 }

// fogSnapshot is a frame with fog of war on: a dug-out pocket in the top-left
// with a colonist in it, and an alien still out in the unexplored rock.
func fogSnapshot() *sim.Snapshot {
	w, h := 12, 8
	tiles := make([]sim.Tile, w*h)
	for i := range tiles {
		p := sim.Point{X: i % w, Y: i / w}
		tiles[i].Terrain = sim.Rock
		tiles[i].Explored = fogExplored(p)
	}
	tiles[2*w+2].Terrain = sim.Floor

	return &sim.Snapshot{
		Tick: 3, Width: w, Height: h, Tiles: sim.NewTileGrid(w, h, tiles),
		TicksPerSecond: 8, FogOfWar: true, MoodMax: 100, AffinityMax: 100,
		Entities: []sim.EntityView{
			{ID: 1, Kind: sim.Colonist, Pos: sim.Point{X: 2, Y: 2}, HP: 40, MaxHP: 40},
			{ID: 2, Kind: sim.Alien, Pos: sim.Point{X: 9, Y: 6}, HP: 30, MaxHP: 30},
		},
		Log: []string{"The colony ship settles onto the Martian crust."},
	}
}

func fogModel(t *testing.T, snap *sim.Snapshot) Model {
	t.Helper()
	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m, _ = m.Update(snapshotMsg{snap: snap})
	return m.(Model)
}

// Rock the colony has not dug up to yet is not drawn at all: the map shows one
// rock glyph per *explored* rock tile in the viewport and no more.
func TestMapDrawsOnlyExploredRock(t *testing.T) {
	restoreGlyphs(t, false)
	m := fogModel(t, fogSnapshot())
	cols, rows := m.viewportTiles()

	want := 0
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			p := m.cam.Add(x, y)
			if fogExplored(p) && m.latest.TerrainAt(p) == sim.Rock {
				want++
			}
		}
	}
	if want == 0 {
		t.Fatal("the fixture has no visible rock; the test would prove nothing")
	}
	if got := strings.Count(m.renderMap(), fitGlyph(glyphRock)); got != want {
		t.Errorf("map drew %d rock glyphs, want %d (one per explored rock tile)", got, want)
	}
}

// An alien lurking in the unknown is exactly what the fog hides, so it is
// not drawn until the colony has dug up to it.
func TestMapHidesEntitiesInTheFog(t *testing.T) {
	restoreGlyphs(t, false)
	snap := fogSnapshot()
	m := fogModel(t, snap)

	body := m.renderMap()
	if !strings.Contains(body, fitGlyph(glyphColonist)) {
		t.Error("the colonist standing in the lit pocket was not drawn")
	}
	if strings.Contains(body, fitGlyph(glyphAlien)) {
		t.Error("an alien out in the unexplored rock was drawn through the fog")
	}

	// Dig up to it and it appears, without anything else about the frame
	// changing.
	alien := snap.Entities[1].Pos
	tiles := make([]sim.Tile, snap.Width*snap.Height)
	for i := range tiles {
		p := sim.Point{X: i % snap.Width, Y: i / snap.Width}
		tiles[i] = snap.TileAt(p)
		if p == alien {
			tiles[i].Explored = true
		}
	}
	snap.Tiles = sim.NewTileGrid(snap.Width, snap.Height, tiles)
	if !strings.Contains(fogModel(t, snap).renderMap(), fitGlyph(glyphAlien)) {
		t.Error("the alien stayed hidden on a tile the colony has now seen")
	}
}

// Fog is painted with a background colour rather than a character, so a fogged
// row still measures exactly one tile per column — the invariant every other
// map row depends on (see docs/terminal-cell-widths.md).
func TestFoggedMapRowsAreExactlyOneTilePerColumn(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		restoreGlyphs(t, ascii)
		for _, size := range []struct{ w, h int }{{20, 8}, {60, 20}, {81, 25}, {120, 40}} {
			t.Run(fmt.Sprintf("ascii=%v/%dx%d", ascii, size.w, size.h), func(t *testing.T) {
				var m tea.Model = New(nil, nil)
				m, _ = m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
				m, _ = m.Update(snapshotMsg{snap: fogSnapshot()})
				model := m.(Model)
				cols, _ := model.viewportTiles()

				for i, row := range strings.Split(model.renderMap(), "\n") {
					if got := cells.Width(row); got != cols*tileWidth {
						t.Fatalf("row %d is %d cells, want %d", i, got, cols*tileWidth)
					}
				}
				if frame := model.renderFrame(); frame != model.clampFrame(frame) {
					t.Error("a fogged frame had to be rescued by clampFrame")
				}
			})
		}
	}
}

// The inspector reports an unexplored tile as unexplored. Handing back the rock
// the snapshot knows is there would give away the map the fog is hiding.
func TestInspectorHidesUnexploredTerrain(t *testing.T) {
	restoreGlyphs(t, false)
	m := fogModel(t, fogSnapshot())
	m.inspecting = true

	m.cursor = sim.Point{X: 9, Y: 6}
	for name, out := range map[string]string{"sidebar": m.renderMapSidebar(), "footer": m.renderFooter()} {
		if !strings.Contains(out, "unexplored") {
			t.Errorf("%s does not call an unexplored tile unexplored: %q", name, out)
		}
		if strings.Contains(out, "rock") {
			t.Errorf("%s names the rock hidden under the fog: %q", name, out)
		}
	}

	m.cursor = sim.Point{X: 2, Y: 2}
	if out := m.renderMapSidebar(); !strings.Contains(out, "floor") {
		t.Errorf("sidebar does not report the explored tile's terrain: %q", out)
	}
}

// The legend gains an "unexplored" swatch only when there is fog to explain.
// TestSidebarCacheInvalidates pins that the memoized sidebar notices it going
// on and off.
func TestLegendExplainsFogOnlyWhenItIsOn(t *testing.T) {
	restoreGlyphs(t, false)
	if out := fogModel(t, fogSnapshot()).renderSidebar(); !strings.Contains(out, "unexplored") {
		t.Errorf("the legend does not mention the fog: %q", out)
	}

	clear := fogSnapshot()
	clear.FogOfWar = false
	if strings.Contains(fogModel(t, clear).renderSidebar(), "unexplored") {
		t.Error("the legend still mentions fog with fog of war off")
	}
}
