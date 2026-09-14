package tui

import (
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		if got := lipgloss.Width(terrainGlyph(tile)); got != tileWidth {
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
		if got := lipgloss.Width(entityGlyph(entity)); got != tileWidth {
			t.Errorf("entity %v occupies %d cells, want %d", entity.Kind, got, tileWidth)
		}
	}
}

// Pressing tab opens the roster, which shows the selected colonist's name and
// traits.
func TestRosterShowsColonistDetail(t *testing.T) {
	snap := makeSnapshot()
	snap.Entities[0].Profile = &sim.Profile{
		Name: "Zoe Vargas", Sex: sim.SexFemale, Gender: sim.GenderWoman,
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

// Before the first frame arrives the view should show a booting message, not
// crash on nil state.
func TestViewBeforeFirstFrame(t *testing.T) {
	m := New(nil, nil)
	if got := m.View(); !strings.Contains(got, "Booting") && !strings.Contains(got, "colony") {
		t.Errorf("unexpected pre-frame view: %q", got)
	}
}
