package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	tea "github.com/charmbracelet/bubbletea"
)

// This is the test the grid-shearing bug got past. Every earlier check asked
// "is this glyph two cells wide?"; none asked "is the finished frame still a
// rectangle?" — which is the property that actually breaks when a glyph is
// mismeasured, because one wrong row pushes the sidebar off the right edge and
// the terminal wraps it.
//
// It runs every screen at a spread of terminal sizes, including sizes far
// smaller than the layout's minimums, since a cramped terminal is where the
// clamps get exercised.
func TestFrameNeverExceedsTerminalWidth(t *testing.T) {
	sizes := []struct{ w, h int }{
		{20, 8}, {40, 12}, {60, 20}, {80, 24}, {100, 30}, {120, 40}, {200, 50},
		{81, 25}, {99, 31}, // odd widths: the map's cols*2 cannot divide them evenly
	}
	screens := []struct {
		name string
		keys []tea.KeyMsg
	}{
		{"map", nil},
		{"roster", []tea.KeyMsg{{Type: tea.KeyTab}}},
		{"jobs", []tea.KeyMsg{{Type: tea.KeyTab}, {Type: tea.KeyTab}}},
		{"perf", []tea.KeyMsg{{Type: tea.KeyTab}, {Type: tea.KeyTab}, {Type: tea.KeyTab}, {Type: tea.KeyTab}, {Type: tea.KeyTab}}},
		{"spawn menu", []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("s")}}},
		{"build menu", []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("b")}}},
	}

	for _, ascii := range []bool{false, true} {
		restoreGlyphs(t, ascii)
		for _, size := range sizes {
			for _, screen := range screens {
				t.Run(fmt.Sprintf("ascii=%v/%s/%dx%d", ascii, screen.name, size.w, size.h), func(t *testing.T) {
					var m tea.Model = New(nil, nil)
					m, _ = m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
					m, _ = m.Update(snapshotMsg{snap: busySnapshot()})
					for _, k := range screen.keys {
						m, _ = m.Update(k)
					}
					// Check the frame the renderers produced, not the one
					// clampFrame rescued: the net must never have anything to
					// catch, or it is hiding the bug from this test.
					model := m.(Model)
					frame := model.renderFrame()
					for i, line := range strings.Split(frame, "\n") {
						if w := cells.Width(line); w > size.w {
							t.Errorf("line %d is %d cells in a %d-cell terminal; it will wrap and shift every row below it:\n%q",
								i, w, size.w, line)
						}
					}
					if clamped := model.clampFrame(frame); clamped != frame {
						t.Error("clampFrame changed the frame: a renderer produced an over-wide line and the safety net covered for it")
					}
				})
			}
		}
	}
}

// The map is a grid, so its rows must all be the same width — that is the
// property the brick wall going jagged was a symptom of losing.
func TestMapRowsAreAllTheSameWidth(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		restoreGlyphs(t, ascii)
		for _, size := range []struct{ w, h int }{{40, 12}, {80, 24}, {120, 40}, {99, 31}} {
			m := New(nil, nil)
			m.termW, m.termH = size.w, size.h
			m.latest = busySnapshot()

			cols, rows := m.viewportTiles()
			lines := strings.Split(m.renderMap(), "\n")
			if len(lines) != rows {
				t.Errorf("ascii=%v %dx%d: map has %d rows, want %d", ascii, size.w, size.h, len(lines), rows)
			}
			for i, line := range lines {
				if w := cells.Width(line); w != cols*tileWidth {
					t.Errorf("ascii=%v %dx%d: map row %d is %d cells, want %d (%d tiles x %d)",
						ascii, size.w, size.h, i, w, cols*tileWidth, cols, tileWidth)
				}
			}
		}
	}
}

// The panel width constants are totals including the border. lipgloss's
// Style.Width sets the content box instead, so the renderers subtract
// borderCells — if that convention ever slips, the panels silently grow by two
// cells and run off the edge.
func TestPanelsRenderAtTheirDeclaredWidth(t *testing.T) {
	cases := []struct {
		name  string
		width int
	}{
		{"sidebar", sidebarWidth},
		{"roster list", rosterListWidth},
		{"job list", jobListWidth},
		{"storage list", storageListWidth},
		{"lore list", loreListWidth},
	}
	for _, tc := range cases {
		got := sidebarStyle.Width(tc.width - borderCells).Height(4).Render("content")
		if w := cells.Width(got); w != tc.width {
			t.Errorf("%s renders at %d cells, but its constant declares %d", tc.name, w, tc.width)
		}
	}
}

// The map, the gap, and the sidebar have to add up to no more than the
// terminal, or the sidebar is pushed off the right edge — the clipped border in
// the bug report. Below the width where both fit, the sidebar is dropped
// instead, and the map alone must still fit.
func TestMapAndSidebarFitSideBySide(t *testing.T) {
	sawBoth, sawMapOnly := false, false
	for w := 1; w <= 200; w++ {
		m := New(nil, nil)
		m.termW, m.termH = w, 30
		m.latest = busySnapshot()

		cols, _ := m.viewportTiles()
		total := cols * tileWidth
		if m.sidebarFits() {
			sawBoth = true
			total += panelGap + sidebarWidth
		} else {
			sawMapOnly = true
		}
		// One tile is the floor even in a terminal too narrow for it;
		// clampFrame trims the overflow in that degenerate case.
		if total > w && cols > 1 {
			t.Errorf("termW=%d: map (%d cells, sidebar shown=%v) totals %d cells, over by %d",
				w, cols*tileWidth, m.sidebarFits(), total, total-w)
		}
	}
	if !sawBoth || !sawMapOnly {
		t.Errorf("expected the sweep to cover both layouts; saw both=%v, map only=%v", sawBoth, sawMapOnly)
	}
}

// busySnapshot is a frame with every terrain kind, refuse on the floor, every
// entity state, and colonists of each age/gender bracket, so a layout test
// exercises every glyph the renderer can emit rather than the two the minimal
// fixture has.
func busySnapshot() *sim.Snapshot {
	w, h := 60, 40
	tiles := make([]sim.Tile, w*h)
	terrains := []sim.Terrain{
		sim.Rock, sim.Floor, sim.Wall, sim.NutrientPod, sim.Toilet, sim.Bed, sim.Incinerator, sim.Storage,
	}
	for i := range tiles {
		tiles[i].Terrain = terrains[i%len(terrains)]
		// Scatter refuse so the sweep covers the gore and corpse glyphs that
		// tileGlyph draws in place of bare terrain.
		switch {
		case i%37 == 0:
			tiles[i].Gore = 1
		case i%53 == 0:
			tiles[i].Corpses = 1
		}
	}

	profiles := []*sim.Profile{
		nil,
		{Name: "Zoe Vargas", Age: 32, Gender: sim.GenderWoman},
		{Name: "Ravi Boone", Age: 71, Gender: sim.GenderMan},
		{Name: "Sam Okonkwo-Lindqvist", Age: 45, Gender: sim.GenderNonbinary},
		{Name: "Ada Fields", Age: 64, Gender: sim.GenderWoman},
	}
	states := []sim.State{sim.Idle, sim.Mining, sim.Fleeing, sim.Talking, sim.Cleaning}

	var entities []sim.EntityView
	for i, p := range profiles {
		entities = append(entities, sim.EntityView{
			ID: sim.EntityID(i + 1), Kind: sim.Colonist, Pos: sim.Point{X: i, Y: 0},
			HP: 40, MaxHP: 40, Charge: 20, Grip: 10, MoodLabel: "driven", State: states[i], Profile: p,
		})
	}
	for i, kind := range []sim.Kind{sim.Alien, sim.Cat, sim.Mouse} {
		entities = append(entities, sim.EntityView{
			ID: sim.EntityID(100 + i), Kind: kind, Pos: sim.Point{X: i, Y: 1}, HP: 30, MaxHP: 30,
		})
	}

	return &sim.Snapshot{
		Tick: 1234, Width: w, Height: h, Seed: 42, Tiles: sim.NewTileGrid(w, h, tiles), TicksPerSecond: 8,
		MoodMax: 100, AffinityMax: 100, Entities: entities,
		AlienSpecies: []sim.AlienSpecies{
			{
				Singular: "xeno", Plural: "xenos",
				HeightMinCM: 180, HeightMaxCM: 220, WeightMinKG: 70, WeightMaxKG: 95,
				Eyes: 4, Limbs: 6, Arms: 2, Tail: true,
				Skin: sim.SkinScaly, Color: "green",
				Temperament: sim.TemperamentHostile,
				BiteDamage:  12, BiteRest: 2, Slowness: 1,
			},
			{
				Singular: "gremlin", Plural: "gremlins",
				HeightMinCM: 90, HeightMaxCM: 130, WeightMinKG: 25, WeightMaxKG: 40,
				Eyes: 2, Limbs: 4, Arms: 2, Tail: false,
				Skin: sim.SkinFurry, Color: "gray",
				Temperament: sim.TemperamentCautious,
				BiteDamage:  4, BiteRest: 3, Slowness: 2,
			},
		},
		Projects: []sim.ProjectView{{
			ID: 1, Name: "a dormitory with a deliberately long name", QueuedTick: 2,
			Tasks: []sim.TaskView{
				{Pos: sim.Point{X: 1, Y: 1}, Terrain: sim.Wall, Done: true},
				{Pos: sim.Point{X: 2, Y: 1}, Terrain: sim.Wall, Owner: 2},
			},
		}},
		Perf: busyPerf(),
		Log: []string{
			"The colony ship settles onto the Martian crust.",
			"A bunk is bolted into the dormitory floor.",
			"A dormitory is complete.",
			"Zoe Vargas stomps a mouse flat against the regolith.",
		},
	}
}

// busyPerf is a timing history with a pause in it (zero-tick buckets) and a
// stall, so the Perf screen draws gaps and a spike.
func busyPerf() []sim.PerfSample {
	start := time.Date(2026, 9, 25, 19, 53, 13, 0, time.UTC)
	var out []sim.PerfSample
	for i := 0; i < 300; i++ {
		s := sim.PerfSample{Start: start.Add(time.Duration(i) * sim.PerfBucket)}
		if i < 120 || i > 140 {
			s.Ticks = 2 + i%3
			s.Step = time.Duration(s.Ticks) * time.Duration(900+i%7*40) * time.Microsecond
			s.Publish = time.Duration(s.Ticks) * 150 * time.Microsecond
			s.MaxTick = 2 * time.Millisecond
		}
		if i == 200 {
			s.MaxTick = 40 * time.Millisecond
			s.Step += 38 * time.Millisecond
		}
		out = append(out, s)
	}
	return out
}

// restoreGlyphs switches the glyph set for one test and puts it back
// afterwards, so a test that exercises the ASCII fallback does not leak that
// choice into the tests that run after it.
func restoreGlyphs(t *testing.T, ascii bool) {
	t.Helper()
	prevRendered, prevASCII := renderedGlyphs.Load(), asciiGlyphs.Load()
	t.Cleanup(func() {
		renderedGlyphs.Store(prevRendered)
		asciiGlyphs.Store(prevASCII)
	})
	renderedGlyphs.Store(buildRenderedGlyphs(ascii))
	asciiGlyphs.Store(ascii)
}

// The roster and job board fill the terminal exactly: header, two panels,
// footer. They used
// to come up two rows short, because their panels passed the content
// height to MaxHeight — which trims the finished block, border included — and
// lost their last row and bottom border to it. The detail panel's scroll
// position line lives on that last row, so a short panel would hide it.
func TestListScreensFillTerminalHeight(t *testing.T) {
	for _, mode := range []struct {
		name string
		mode viewMode
	}{{"roster", modeRoster}, {"jobs", modeJobs}, {"storage", modeStorage}, {"lore", modeLore}, {"perf", modePerf}} {
		for _, size := range []struct{ w, h int }{{100, 30}, {120, 40}, {200, 50}, {80, 24}} {
			m := New(nil, nil)
			m.termW, m.termH = size.w, size.h
			m.latest = busySnapshot()
			m.mode = mode.mode

			lines := strings.Split(m.renderFrame(), "\n")
			if len(lines) != size.h {
				t.Errorf("%s %dx%d: frame is %d rows, want %d", mode.name, size.w, size.h, len(lines), size.h)
			}
			if body := lines[len(lines)-2]; !strings.Contains(body, "╰") {
				t.Errorf("%s %dx%d: expected the panels' bottom border above the footer, got %q", mode.name, size.w, size.h, body)
			}
		}
	}
}
