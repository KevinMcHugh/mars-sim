package tui

import (
	"fmt"
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
		Tiles:          sim.NewTileGrid(w, h, tiles),
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
	terrain := []sim.Terrain{
		sim.Floor, sim.Rock, sim.Wall, sim.NutrientPod, sim.Toilet, sim.Bed, sim.Incinerator, sim.Storage,
	}
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
		{Kind: sim.Colonist, State: sim.Cleaning},
		{Kind: sim.Colonist, State: sim.Hauling},
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
	if !strings.Contains(out, "ROSTER") {
		t.Error("roster should show the entity list heading")
	}
}

// The roster inspector shows a colonist's family ties and affinities, naming the
// related colonists.
func TestRosterShowsFamilyAndAffinity(t *testing.T) {
	snap := makeSnapshot()
	snap.AffinityMax, snap.MoodMax = 100, 100
	snap.Entities[0].Profile = &sim.Profile{Name: "Zoe Vargas", Gender: sim.GenderWoman}
	snap.Entities[0].Charge, snap.Entities[0].Grip, snap.Entities[0].MoodLabel = 42, -7, "anxious"
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
	for _, want := range []string{"FAMILY", "sibling", "Ravi Boone", "AFFINITIES", "Omar Petrov", "-30", "affect", "C+42", "G-7", "anxious"} {
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

func TestStorageDetailsTabNavigatesContainerContents(t *testing.T) {
	snap := makeSnapshot()
	var first, second sim.StorageInventory
	first[0] = sim.ItemStack{Kind: sim.IronOre, Count: 12}
	second[0] = sim.ItemStack{Kind: sim.WaterIce, Count: 7}
	snap.Storages = []sim.StorageView{
		{Pos: sim.Point{X: 1, Y: 2}, Inventory: first},
		{Pos: sim.Point{X: 4, Y: 2}, Inventory: second},
	}

	var model tea.Model = New(nil, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(snapshotMsg{snap: snap})
	for range 3 {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	}

	if out := model.View(); !strings.Contains(out, "DETAILS · STORAGE") ||
		!strings.Contains(out, "iron ore ×12") {
		t.Fatalf("storage details did not show the first chest:\n%s", out)
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if out := model.View(); !strings.Contains(out, "Storage chest (4,2)") ||
		!strings.Contains(out, "water ice ×7") {
		t.Fatalf("down did not select the second chest:\n%s", out)
	}
}

func TestMapCursorInspectsAndOpensStorage(t *testing.T) {
	snap := makeSnapshot()
	tiles := make([]sim.Tile, snap.Width*snap.Height)
	for i := range tiles {
		tiles[i].Terrain = sim.Rock
	}
	p := sim.Point{X: snap.Width - 1, Y: snap.Height - 1}
	tiles[p.Y*snap.Width+p.X].Terrain = sim.Storage
	snap.Tiles = sim.NewTileGrid(snap.Width, snap.Height, tiles)
	var inventory sim.StorageInventory
	inventory[0] = sim.ItemStack{Kind: sim.RawRock, Count: 9}
	snap.Storages = []sim.StorageView{{Pos: p, Inventory: inventory}}

	var model tea.Model = New(nil, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(snapshotMsg{snap: snap})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})

	if out := model.View(); !strings.Contains(out, "INSPECT") ||
		!strings.Contains(out, "raw rock ×9") {
		t.Fatalf("map cursor did not inspect storage:\n%s", out)
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if out := model.View(); !strings.Contains(out, "DETAILS · STORAGE") {
		t.Fatalf("enter did not open storage details:\n%s", out)
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

// By default the roster shows only living colonists — the alien in the
// snapshot, and a dead mouse in the graveyard, should both be hidden until
// their filters are turned on.
func TestRosterHidesNonHumanAndDeadByDefault(t *testing.T) {
	snap := makeSnapshot()
	snap.Graveyard = []sim.EntityView{
		{ID: 9, Kind: sim.Mouse, Dead: true, DiedTick: 3, Cause: "crushed by a colonist"},
	}

	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: snap})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	out := m.View()
	if strings.Contains(out, "alien #2") {
		t.Error("roster should not show the alien until the non-human filter is on")
	}
	if strings.Contains(out, "crushed by a colonist") {
		t.Error("roster should not show the dead mouse until the dead filter is on")
	}
	if !strings.Contains(out, "ROSTER (1)") {
		t.Errorf("roster should count only the one living colonist:\n%s", out)
	}
}

// Opening the filter menu (f) and toggling non-human (n) reveals the alien in
// the roster; toggling it again hides it. The menu stays open across the
// toggle, since setting more than one filter per visit is the normal case.
func TestFilterMenuTogglesNonHuman(t *testing.T) {
	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: makeSnapshot()})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // open roster

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	out := m.View()
	if !strings.Contains(out, "filter:") || !strings.Contains(out, "n non-human: off") {
		t.Errorf("filter menu prompt not shown:\n%s", out)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	out = m.View()
	if !strings.Contains(out, "filter:") {
		t.Error("toggling a filter should not close the menu")
	}
	if !strings.Contains(out, "n non-human: on") {
		t.Errorf("filter prompt should show non-human on:\n%s", out)
	}
	if !strings.Contains(out, "alien #2") {
		t.Error("roster should show the alien once the non-human filter is on")
	}
	if !strings.Contains(out, "ROSTER (2) +non-human") {
		t.Errorf("roster title should show the count and active filter:\n%s", out)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	out = m.View()
	if strings.Contains(out, "alien #2") {
		t.Error("toggling non-human off again should hide the alien")
	}
}

// Toggling the dead filter reveals a graveyard entry in the roster, including
// its cause of death.
func TestFilterMenuTogglesDead(t *testing.T) {
	snap := makeSnapshot()
	snap.Graveyard = []sim.EntityView{
		{ID: 9, Kind: sim.Colonist, Dead: true, DiedTick: 3, Cause: "starved",
			Profile: &sim.Profile{Name: "Ada Okafor", Gender: sim.GenderWoman}},
	}

	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: snap})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	out := m.View()
	if !strings.Contains(out, "Ada Okafor") {
		t.Errorf("roster should show the dead colonist once the dead filter is on:\n%s", out)
	}
	if !strings.Contains(out, "dead — starved") {
		t.Errorf("roster list should show the cause of death:\n%s", out)
	}
}

// esc closes the filter menu without losing whatever was toggled.
func TestFilterMenuEscClosesAndKeepsFilters(t *testing.T) {
	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: makeSnapshot()})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	out := m.View()
	if strings.Contains(out, "filter:") {
		t.Error("esc should close the filter menu")
	}
	if !strings.Contains(out, "alien #2") {
		t.Error("closing the filter menu should not undo an already-toggled filter")
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

// The inspector scrolls. A colonist with a long history has memories below
// the fold; pgdn brings them into view, the panel says where in the text the
// reader is, and moving the roster selection puts it back at the top.
func TestRosterDetailScrolls(t *testing.T) {
	snap := makeSnapshot()
	snap.Entities[0].Profile = &sim.Profile{Name: "Zoe Vargas", Gender: sim.GenderWoman}
	for i := 0; i < 30; i++ {
		snap.Entities[0].Memories = append(snap.Entities[0].Memories,
			sim.Memory{Tick: i, Text: fmt.Sprintf("Remembered thing %d.", i)})
	}

	var m tea.Model = New(nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: snap})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	out := m.View()
	if !strings.Contains(out, "STATUS") {
		t.Fatal("the inspector should open at the top of the colonist")
	}
	if strings.Contains(out, "Remembered thing 29.") {
		t.Fatal("the newest memory should start below the fold, or this test proves nothing")
	}
	if !strings.Contains(out, "1-24 of 59") {
		t.Errorf("an overflowing inspector should say how much content there is:\n%s", out)
	}

	// Two screenfuls is past the end of this colonist; the panel stops at the
	// last line rather than scrolling into blank space.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	out = m.View()
	if !strings.Contains(out, "Remembered thing 29.") {
		t.Errorf("scrolling down should reach the newest memory:\n%s", out)
	}
	if strings.Contains(out, "STATUS") {
		t.Error("scrolled to the bottom, the identity block should be off the panel")
	}
	if !strings.Contains(out, "36-59 of 59") {
		t.Errorf("the bottom of the content should be the last line shown:\n%s", out)
	}

	// One line back up, then reselecting a colonist returns to the top.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftUp})
	if out = m.View(); !strings.Contains(out, "35-58 of 59") {
		t.Errorf("shift+up should move the window one line:\n%s", out)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if out = m.View(); !strings.Contains(out, "STATUS") {
		t.Error("changing the roster selection should reset the inspector to the top")
	}
}

// scrollDetail always fills exactly the rows it is given and keeps the last
// one for the position line, whatever offset it is handed — the property the
// roster's fixed-height layout depends on.
func TestScrollDetailFillsItsPanel(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	const height = 10
	for _, off := range []int{-5, 0, 3, 31, 39, 500} {
		got := scrollDetail(lines, off, height, 20)
		if len(got) != height {
			t.Fatalf("offset %d: got %d lines, want %d", off, len(got), height)
		}
		if !strings.Contains(got[height-1], "of 40") {
			t.Errorf("offset %d: last row should be the position line, got %q", off, got[height-1])
		}
	}
	// Content that fits is left alone: no position line, no padding rows.
	if got := scrollDetail(lines[:4], 0, height, 20); len(got) != 4 {
		t.Errorf("content that fits should be returned as-is, got %d lines", len(got))
	}
}

// A memory standing for a run of the same minor event should read as one line
// that names the span and the number of occurrences; an ordinary single memory
// should still read as a plain tick and text.
func TestRosterDetailShowsCollapsedMemoryRun(t *testing.T) {
	snap := makeSnapshot()
	snap.Entities[0].Profile = &sim.Profile{Name: "Zoe Vargas", Gender: sim.GenderWoman}
	snap.Entities[0].Memories = []sim.Memory{
		{Tick: 1586, LastTick: 1586, Count: 1, Text: "Had a meal.", Kind: sim.EvtAte},
		{Tick: 1607, LastTick: 1630, Count: 12, Text: "Finished mining.", Kind: sim.EvtFinishedMining},
	}

	var m tea.Model = New(nil, nil)
	// Tall enough that the whole inspector fits without scrolling.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
	m, _ = m.Update(snapshotMsg{snap: snap})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	out := m.View()
	if !strings.Contains(out, "t1607-1630: Finished mining. (x12)") {
		t.Errorf("collapsed memory line missing from inspector:\n%s", out)
	}
	if !strings.Contains(out, "t1586: Had a meal.") {
		t.Errorf("single memory line missing from inspector:\n%s", out)
	}
}
