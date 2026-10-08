package wire

import (
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/glyphs"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// A species pack's sprites ride in the Hello, and an alien drawing one
// carries a glyph past the looks that indexes them; every other entity keeps
// its glyph.
func TestSpriteGlyphs(t *testing.T) {
	snap := &sim.Snapshot{Sprites: []string{"<svg a/>", "<svg b/>"}}
	h := NewHello(snap)
	if len(h.Glyphs.Sprites) != 2 || h.Glyphs.Sprites[1] != "<svg b/>" {
		t.Fatalf("hello sprites %v", h.Glyphs.Sprites)
	}
	if h.Glyphs.Symbols[h.Glyphs.SpriteFallback] != glyphs.ForKind(sim.Alien) {
		t.Fatal("a sprite's stand-in is not the plain alien")
	}

	alien := sim.EntityView{Kind: sim.Alien, Sprite: 2}
	if got, want := int(entityGlyph(alien)), len(h.Glyphs.Symbols)+len(h.Glyphs.Looks)+1; got != want {
		t.Fatalf("sprite glyph %d, want %d", got, want)
	}
	alien.Sprite = 0
	if got := int(entityGlyph(alien)); got >= len(h.Glyphs.Symbols) {
		t.Fatalf("an alien without a sprite got glyph %d, past the symbols", got)
	}
	if len(NewHello(&sim.Snapshot{}).Glyphs.Sprites) != 0 {
		t.Fatal("a world without a pack sent sprites")
	}
}

// The lore topic carries a packed species' lab text: the rewrite in place of
// the generated field notes, and the notes beside them.
func TestLoreCarriesLabText(t *testing.T) {
	sp := sim.RollLabSpecies(3)
	snap := &sim.Snapshot{
		AlienSpecies: []sim.AlienSpecies{sp, sp},
		AlienLore:    []sim.SpeciesLore{{FieldNotes: "Rewritten.", LabNotes: "Notes."}, {}},
	}
	lore := loreTopic(snap).(LoreTopic)
	if lore.Species[0].Description != "Rewritten." || lore.Species[0].Notes != "Notes." {
		t.Fatalf("packed lore %+v", lore.Species[0])
	}
	if lore.Species[1].Description != sp.Description() || lore.Species[1].Notes != "" {
		t.Fatalf("unedited lore %q / %q", lore.Species[1].Description, lore.Species[1].Notes)
	}
}

// The lore topic lists a packed species' forms with their sprites and picks
// the first form of the last stage to show beside the name; a rolled
// species has neither, and its title is its label without the emoji.
func TestLoreForms(t *testing.T) {
	var sp sim.AlienSpecies
	for seed := int64(1); ; seed++ {
		if sp = sim.RollLabSpecies(seed); sp.FormCount >= 2 {
			break
		}
	}
	var of [7]int
	for f := 0; f < sp.FormCount; f++ {
		of[f] = f + 1
	}
	snap := &sim.Snapshot{
		AlienSpecies: []sim.AlienSpecies{sp, sim.RollLabSpecies(1)},
		Sprites:      make([]string, sp.FormCount),
		SpriteOf:     [][7]int{of, {}},
	}
	lore := loreTopic(snap).(LoreTopic)
	packed, rolled := lore.Species[0], lore.Species[1]
	if len(packed.Forms) != sp.FormCount || packed.Forms[0].Sprite != 0 || packed.Forms[0].Name != sp.Forms[0].Name {
		t.Fatalf("packed forms %+v", packed.Forms)
	}
	last := sp.Forms[sp.FormCount-1].Stage
	if p := packed.Portrait; p < 0 || sp.Forms[p].Stage != last || (p > 0 && sp.Forms[p-1].Stage == last) {
		t.Fatalf("portrait is form %d, want the first form of stage %d", p, last)
	}
	if rolled.Forms != nil || rolled.Portrait != -1 {
		t.Fatalf("rolled species has forms %+v, portrait %d", rolled.Forms, rolled.Portrait)
	}
	if rolled.Title == "" || strings.Contains(rolled.Title, rolled.Glyph) {
		t.Fatalf("title %q", rolled.Title)
	}
}
