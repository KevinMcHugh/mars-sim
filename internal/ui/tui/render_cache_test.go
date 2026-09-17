package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"
)

// renderMap no longer re-measures each row, on the strength of this: a row is
// a run of fitted glyphs, so its width is the sum of theirs unless two adjacent
// glyphs fuse into one grapheme cluster. Check every ordered pair, in both
// glyph sets, so a new glyph that would fuse with a neighbour fails here
// instead of shearing a map row.
func TestAdjacentGlyphsNeverMerge(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		rendered := *buildRenderedGlyphs(ascii)
		for a, left := range rendered {
			for b, right := range rendered {
				if w := cells.Width(left + right); w != 2*tileWidth {
					t.Errorf("ascii=%v: %+q then %+q measures %d cells, want %d",
						ascii, a, b, w, 2*tileWidth)
				}
			}
		}
	}
}

// joinColumns replaces lipgloss.JoinHorizontal on the map screen and must
// produce the same bytes, including when either column is the shorter one.
func TestJoinColumnsMatchesLipgloss(t *testing.T) {
	gap := strings.Repeat(" ", panelGap)
	check := func(name, left string, leftWidth int, right string, rightWidth int) {
		t.Helper()
		want := lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
		if got := joinColumns(left, leftWidth, right, rightWidth); got != want {
			t.Errorf("%s: joinColumns differs from lipgloss.JoinHorizontal\n got: %q\nwant: %q", name, got, want)
		}
	}

	for _, ascii := range []bool{false, true} {
		restoreGlyphs(t, ascii)
		for _, size := range [][2]int{{100, 30}, {140, 12}, {200, 50}} {
			var tm tea.Model = New(nil, nil)
			tm, _ = tm.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			tm, _ = tm.Update(snapshotMsg{snap: busySnapshot()})
			m := tm.(Model)
			cols, _ := m.viewportTiles()
			mapBlock, sidebar := m.renderMap(), m.renderSidebar()
			check("real map and sidebar", mapBlock, cols*tileWidth, sidebar, sidebarWidth)

			mapLines := strings.Split(mapBlock, "\n")
			sideLines := strings.Split(sidebar, "\n")
			check("left column shorter", strings.Join(mapLines[:len(mapLines)/2], "\n"), cols*tileWidth, sidebar, sidebarWidth)
			check("right column shorter", mapBlock, cols*tileWidth, strings.Join(sideLines[:len(sideLines)/2], "\n"), sidebarWidth)
		}
	}
}

func cacheTestModel(w, h int) tea.Model {
	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m, _ = m.Update(snapshotMsg{snap: busySnapshot()})
	return m
}

// The sidebar is memoized. A stale sidebar is the failure mode, so pin each
// input that must invalidate it: a new log line, a panel height change, a
// switch of glyph set, and fog of war going on or off.
func TestSidebarCacheInvalidates(t *testing.T) {
	t.Run("new log line", func(t *testing.T) {
		m := cacheTestModel(120, 30)
		_ = m.View()
		next := busySnapshot()
		next.Log = append(next.Log, "An alien surfaces nearby.")
		m, _ = m.Update(snapshotMsg{snap: next})
		if !strings.Contains(m.View(), "An alien surfaces nearby.") {
			t.Error("sidebar did not show the new log line; cache is stale")
		}
	})

	t.Run("resize", func(t *testing.T) {
		m := cacheTestModel(120, 30)
		tall := m.(Model).renderSidebar()
		m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 22})
		short := m.(Model).renderSidebar()
		if strings.Count(tall, "\n") == strings.Count(short, "\n") {
			t.Error("sidebar height did not change after a resize; cache is stale")
		}
	})

	t.Run("glyph set", func(t *testing.T) {
		restoreGlyphs(t, false)
		m := cacheTestModel(120, 30)
		emoji := m.(Model).renderSidebar()
		restoreGlyphs(t, true)
		ascii := m.(Model).renderSidebar()
		if emoji == ascii {
			t.Error("sidebar did not change after switching to ASCII glyphs; cache is stale")
		}
	})

	// The legend explains the fog only when there is fog, so the row has to
	// come and go with the setting rather than stick at the first frame's.
	t.Run("fog of war", func(t *testing.T) {
		restoreGlyphs(t, false)
		m := cacheTestModel(120, 30)
		clear := m.(Model).renderSidebar()
		foggy := busySnapshot()
		foggy.FogOfWar = true
		m, _ = m.Update(snapshotMsg{snap: foggy})
		if fogged := m.(Model).renderSidebar(); fogged == clear {
			t.Error("sidebar did not change after fog of war came on; cache is stale")
		} else if !strings.Contains(fogged, "unexplored") {
			t.Errorf("the legend does not explain the fog: %q", fogged)
		}
	})
}

// Caching must never change what is drawn: a Model with no cache renders the
// same frames as one with a warm cache, across ticks and resizes.
func TestCachedAndUncachedFramesMatch(t *testing.T) {
	cached := cacheTestModel(120, 30)
	plain := cached.(Model)
	plain.cache = nil
	var uncached tea.Model = plain

	steps := []tea.Msg{
		snapshotMsg{snap: busySnapshot()},
		tea.WindowSizeMsg{Width: 90, Height: 24},
		tea.KeyMsg{Type: tea.KeyRight},
		snapshotMsg{snap: busySnapshot()},
		tea.WindowSizeMsg{Width: 40, Height: 10},
		tea.WindowSizeMsg{Width: 200, Height: 50},
	}
	for i, msg := range steps {
		cached, _ = cached.Update(msg)
		uncached, _ = uncached.Update(msg)
		if cached.View() != uncached.View() {
			t.Fatalf("step %d (%T): cached and uncached frames differ", i, msg)
		}
	}
}

// clampFrame reuses line widths from the previous frame. A reused width must
// still be checked against the current terminal width, so a line that fit
// before a resize is trimmed after it.
func TestClampFrameRechecksCachedLinesAfterResize(t *testing.T) {
	m := New(nil, nil)
	line := strings.Repeat("x", 50)

	m.termW = 80
	if got := m.clampFrame(line); got != line {
		t.Fatalf("a 50-cell line was changed in an 80-cell terminal: %q", got)
	}
	m.termW = 20
	if w := cells.Width(m.clampFrame(line)); w > 20 {
		t.Errorf("after shrinking to 20 cells the cached line is still %d cells", w)
	}
}

// When entities share a tile the map draws the alien, whichever order they
// arrive in: it is the one the player most needs to see.
func TestMapDrawsAlienOnSharedTile(t *testing.T) {
	restoreGlyphs(t, false)
	for _, alienFirst := range []bool{true, false} {
		snap := busySnapshot()
		shared := snap.Entities[len(snap.Entities)-1].Pos // the mouse's tile
		alien := snap.Entities[len(snap.Entities)-3]      // the alien
		alien.ID, alien.Pos = 999, shared
		if alienFirst {
			snap.Entities = append([]sim.EntityView{alien}, snap.Entities...)
		} else {
			snap.Entities = append(snap.Entities, alien)
		}

		var m tea.Model = New(nil, nil)
		m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
		m, _ = m.Update(snapshotMsg{snap: snap})
		model := m.(Model)
		row := strings.Split(model.renderMap(), "\n")[shared.Y-model.cam.Y]
		if !strings.Contains(row, fitGlyph(glyphAlien)) {
			t.Fatalf("alienFirst=%v: shared tile row has no alien: %q", alienFirst, row)
		}
		if strings.Contains(row, fitGlyph(glyphMouse)) {
			t.Errorf("alienFirst=%v: the mouse was drawn over the alien on a shared tile", alienFirst)
		}
	}
}
