package tui

import (
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

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

func TestGlyphsOccupyOneTile(t *testing.T) {
	terrain := []sim.Terrain{sim.Floor, sim.Rock, sim.Wall, sim.NutrientPod, sim.Toilet, sim.Bed}
	for _, tile := range terrain {
		if got := cells.Width(terrainGlyph(tile)); got != tileWidth {
			t.Errorf("terrain %v occupies %d cells, want %d", tile, got, tileWidth)
		}
	}

	entities := []sim.EntityView{
		{Kind: sim.Colonist, State: sim.Mining},
		{Kind: sim.Colonist, State: sim.Fleeing},
		{Kind: sim.Colonist, State: sim.Talking},
		{Kind: sim.Colonist, State: sim.Stomping},
		{Kind: sim.Alien},
		{Kind: sim.Cat},
		{Kind: sim.Mouse},
	}
	for _, entity := range entities {
		if got := cells.Width(entityGlyph(entity)); got != tileWidth {
			t.Errorf("entity %v occupies %d cells, want %d", entity.Kind, got, tileWidth)
		}
	}
}

// Pressing tab opens the roster, which shows the selected colonist's name and
// traits.
func TestRosterShowsColonistDetail(t *testing.T) {
	snap := makeSnapshot()
	snap.Entities[0].Profile = &sim.Profile{
		Name: "Zoe Vargas", Age: 32, Gender: sim.GenderWoman,
		Orientation: sim.Bisexual, HeightCM: 168, WeightKG: 61,
		Traits: []sim.Trait{sim.TraitBigEater},
	}
	snap.Entities[0].Inventory[0] = sim.ItemStack{Kind: sim.RawRock, Count: 12}
	snap.NeedsMeta[0] = sim.NeedMeta{Name: "food", Max: 1000, Fatal: true}

	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: snap})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	out := m.View()
	if !strings.Contains(out, "Zoe Vargas") {
		t.Error("roster should show the colonist's name")
	}
	if !strings.Contains(out, "age 32") {
		t.Error("roster should show the colonist's age")
	}
	if !strings.Contains(out, "Big Eater") {
		t.Error("roster should show the colonist's trait")
	}
	if !strings.Contains(out, "raw rock ×12") {
		t.Error("roster should show the colonist's inventory")
	}
	if !strings.Contains(out, "COLONISTS") {
		t.Error("roster should show the colonist list heading")
	}
}

// The roster inspector shows a colonist's family ties and affinities, naming the
// related colonists.
func TestRosterShowsFamilyAndAffinity(t *testing.T) {
	snap := makeSnapshot()
	snap.AffinityMax, snap.MoodMax = 100, 100
	snap.Entities[0].Profile = &sim.Profile{Name: "Zoe Vargas", Gender: sim.GenderWoman}
	snap.Entities[0].Mood = 42
	// Colonists Zoe is related to: one she likes, one she has come to dislike.
	snap.Entities = append(snap.Entities,
		sim.EntityView{
			ID: 3, Kind: sim.Colonist, Pos: sim.Point{X: 1, Y: 2}, HP: 40, MaxHP: 40,
			Profile: &sim.Profile{Name: "Ravi Boone", Gender: sim.GenderMan},
		},
		sim.EntityView{
			ID: 4, Kind: sim.Colonist, Pos: sim.Point{X: 2, Y: 2}, HP: 40, MaxHP: 40,
			Profile: &sim.Profile{Name: "Omar Petrov", Gender: sim.GenderMan},
		},
	)
	snap.Entities[0].Relations = []sim.Relation{{Other: 3, Kind: sim.RelSibling}}
	snap.Entities[0].Affinities = []sim.Affinity{{Other: 3, Value: 40}, {Other: 4, Value: -30}}

	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = m.Update(snapshotMsg{snap: snap})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	out := m.View()
	for _, want := range []string{"FAMILY", "sibling", "Ravi Boone", "AFFINITIES", "Omar Petrov", "-30", "mood", "+42"} {
		if !strings.Contains(out, want) {
			t.Errorf("roster detail missing %q", want)
		}
	}
}

// Pressing tab twice opens the job board, which lists a queued project, its
// progress, and the colonist assigned to one of its tasks.
func TestJobBoardShowsProjectAndAssignee(t *testing.T) {
	snap := makeSnapshot()
	snap.Entities[0].Profile = &sim.Profile{Name: "Zoe Vargas", Gender: sim.GenderWoman}
	snap.Projects = []sim.ProjectView{
		{
			ID:         1,
			Name:       "facility room",
			QueuedTick: 2,
			Phase:      0,
			Tasks: []sim.TaskView{
				{Pos: sim.Point{X: 1, Y: 1}, Terrain: sim.Wall, Done: true},
				{Pos: sim.Point{X: 2, Y: 1}, Terrain: sim.Wall, Owner: 1},
			},
		},
	}

	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: snap})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	out := m.View()
	for _, want := range []string{
		"JOB BOARD", "facility room", "1/2 tasks", "1 assigned",
		"queued tick 2", "1 action(s) remaining", "Zoe Vargas", "building: Zoe Vargas",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("job board missing %q in:\n%s", want, out)
		}
	}
}

// With no projects queued, the job board reports any manual orders still
// waiting for a build site instead of an empty screen.
func TestJobBoardShowsPendingOrdersWhenEmpty(t *testing.T) {
	snap := makeSnapshot()
	snap.PendingFacilityRooms = 1

	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: snap})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	out := m.View()
	if !strings.Contains(out, "No jobs queued") {
		t.Error("job board should report nothing queued")
	}
	if !strings.Contains(out, "1 facility room order(s) waiting") {
		t.Error("job board should surface the pending facility room order")
	}
}

// Pressing s opens the spawn menu, whose prompt replaces the footer; esc
// cancels it without sending a command.
func TestSpawnMenuOpensAndCancels(t *testing.T) {
	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: makeSnapshot()})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	out := m.View()
	if !strings.Contains(out, "spawn:") || !strings.Contains(out, "colonist") {
		t.Errorf("spawn menu prompt not shown:\n%s", out)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	out = m.View()
	if strings.Contains(out, "spawn:") {
		t.Error("esc should close the spawn menu")
	}
}

// Pressing b opens the build menu; selecting a room kind sends the matching
// command and closes the menu, using a real engine so Send does not panic.
func TestBuildMenuSelectsRoomKind(t *testing.T) {
	eng := sim.NewEngine(sim.DefaultConfig())
	var m tea.Model = New(eng, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: makeSnapshot()})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	out := m.View()
	if !strings.Contains(out, "build:") || !strings.Contains(out, "dormitory") {
		t.Errorf("build menu prompt not shown:\n%s", out)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	out = m.View()
	if strings.Contains(out, "build:") {
		t.Error("selecting a room kind should close the build menu")
	}
}

// Arrow keys move the highlighted option in an open menu instead of
// submitting immediately, and enter confirms whatever is highlighted.
func TestMenuArrowsMoveSelectionAndEnterConfirms(t *testing.T) {
	eng := sim.NewEngine(sim.DefaultConfig())
	var m tea.Model = New(eng, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: makeSnapshot()})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	out := m.View()
	if !strings.Contains(out, "[c colonist]") {
		t.Errorf("expected colonist highlighted by default:\n%s", out)
	}

	// colonist -> alien -> cat -> mouse.
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	out = m.View()
	if !strings.Contains(out, "[m mouse]") {
		t.Errorf("expected mouse highlighted after three down presses:\n%s", out)
	}
	if strings.Contains(out, "spawn:") == false {
		t.Error("menu should still be open before enter is pressed")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	out = m.View()
	if strings.Contains(out, "spawn:") {
		t.Error("enter should submit the highlighted option and close the menu")
	}
}

// The highlighted option in each menu is remembered across opens, so
// repeating a choice is just reopen-and-confirm: s -> navigate to mouse ->
// enter, then s -> enter, s -> enter for three mice.
func TestMenuRemembersLastSelection(t *testing.T) {
	eng := sim.NewEngine(sim.DefaultConfig())
	var m tea.Model = New(eng, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: makeSnapshot()})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	for i := 0; i < 2; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
		out := m.View()
		if !strings.Contains(out, "[m mouse]") {
			t.Fatalf("round %d: expected the menu to reopen with mouse still highlighted:\n%s", i, out)
		}
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	}
}

// A shortcut key (e.g. f for facility room) still jumps to and submits that
// option immediately, and also updates the remembered highlight.
func TestMenuShortcutKeyUpdatesRememberedSelection(t *testing.T) {
	eng := sim.NewEngine(sim.DefaultConfig())
	var m tea.Model = New(eng, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: makeSnapshot()})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}) // dormitory, direct shortcut

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	out := m.View()
	if !strings.Contains(out, "[d dormitory]") {
		t.Errorf("expected the build menu to reopen with dormitory remembered:\n%s", out)
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
