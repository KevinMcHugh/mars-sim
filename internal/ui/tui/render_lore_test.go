package tui

import (
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	tea "github.com/charmbracelet/bubbletea"
)

func loreSnapshot() *sim.Snapshot {
	snap := makeSnapshot()
	snap.Seed = 12345
	snap.FogOfWar = true
	snap.Stats.ExploredTiles = 6 // of the 6x4 = 24 tiles makeSnapshot lays out
	snap.AlienSpecies = []sim.AlienSpecies{
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
	}
	return snap
}

// toLore advances a fresh model through the tab rotation to the lore screen
// (map -> roster -> jobs -> storage -> lore).
func toLore(t *testing.T, snap *sim.Snapshot) tea.Model {
	t.Helper()
	var model tea.Model = New(nil, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(snapshotMsg{snap: snap})
	for range 4 {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	return model
}

// The lore tab should show world facts (size, how much has been explored,
// the seed) and the first rolled species' full detail by default.
func TestLoreTabShowsWorldFactsAndFirstSpecies(t *testing.T) {
	model := toLore(t, loreSnapshot())

	out := model.View()
	for _, want := range []string{
		"LORE · WORLD",
		"Size: 6 x 4 (24 tiles)",
		"Explored: 25% (6/24 tiles)",
		"Seed: 12345",
		"ALIEN SPECIES (2)",
		"Xeno · hostile",
		"Height:        180-220 cm",
		"Bite damage:   12",
		"FIELD NOTES",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("lore tab missing %q:\n%s", want, out)
		}
	}
}

// Down should move the selection to the second species and swap the detail
// panel to it.
func TestLoreTabNavigatesSpeciesSelection(t *testing.T) {
	model := toLore(t, loreSnapshot())
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})

	out := model.View()
	if !strings.Contains(out, "Gremlin · cautious") {
		t.Fatalf("down did not select the second species:\n%s", out)
	}
	if strings.Contains(out, "FIELD NOTES\nGremlins stand 180") {
		t.Fatal("detail panel still shows the first species' build")
	}
}

// esc should return to the map from the lore tab, like every other details
// panel.
func TestLoreTabEscReturnsToMap(t *testing.T) {
	model := toLore(t, loreSnapshot())
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEscape})

	if m := model.(Model); m.mode != modeMap {
		t.Fatalf("mode after esc = %v, want modeMap", m.mode)
	}
}

// With fog of war off, the explored line should say the map is fully
// visible instead of reporting a stuck 0% (World.reveal is never called
// while fog is off, so Stats.ExploredTiles never moves).
func TestLoreTabReportsFullyVisibleWhenFogOff(t *testing.T) {
	snap := loreSnapshot()
	snap.FogOfWar = false
	snap.Stats.ExploredTiles = 0

	out := toLore(t, snap).View()
	if !strings.Contains(out, "Explored: 100% (fog off)") {
		t.Fatalf("fog-off snapshot did not report full visibility:\n%s", out)
	}
	if strings.Contains(out, "Explored: 0%") {
		t.Fatalf("fog-off snapshot reported a stuck 0%%:\n%s", out)
	}
}

// A world that has rolled no alien species (a hand-built Snapshot, or a
// misconfigured 0-species world) must render an empty state, not panic or
// draw a blank detail panel.
func TestLoreTabHandlesNoSpecies(t *testing.T) {
	snap := loreSnapshot()
	snap.AlienSpecies = nil

	out := toLore(t, snap).View()
	if !strings.Contains(out, "ALIEN SPECIES (0)") || !strings.Contains(out, "None rolled.") {
		t.Fatalf("empty species roster did not render its empty state:\n%s", out)
	}
}

func TestWrapWordsBreaksOnlyAtSpaces(t *testing.T) {
	got := wrapWords("the quick brown fox jumps", 10)
	want := []string{"the quick", "brown fox", "jumps"}
	if len(got) != len(want) {
		t.Fatalf("wrapWords lines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wrapWords line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWrapWordsEmptyInput(t *testing.T) {
	if got := wrapWords("", 10); got != nil {
		t.Fatalf("wrapWords(\"\", 10) = %v, want nil", got)
	}
}
