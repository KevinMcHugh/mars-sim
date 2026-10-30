package wire

import (
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/glyphs"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// A mutant at rest carries a glyph past the sprites that names their doll,
// and the Hello says where the dolls start and how to read a key
// (docs/body-sprites.md).
func TestDollGlyphs(t *testing.T) {
	snap := &sim.Snapshot{Sprites: []string{"<svg a/>", "<svg b/>"}}
	h := NewHello(snap)
	d := h.Glyphs.Dolls
	if d.Base != len(h.Glyphs.Symbols)+len(h.Glyphs.Looks)+2 || d.Count != glyphs.NumDolls {
		t.Fatalf("dolls at %d, %d of them", d.Base, d.Count)
	}
	if len(d.Figures) != len(glyphs.DollFigures) || len(d.Hairs) != glyphs.DollHairs || len(d.Parts) != len(glyphs.DollParts) || d.Tones != glyphs.DollTones {
		t.Fatalf("doll digits %+v", d)
	}
	if h.Glyphs.Symbols[d.Fallback] != glyphs.Mutant {
		t.Fatal("a doll's stand-in is not the mutant")
	}
	if d.Base+d.Count > 0xFFFF {
		t.Fatal("doll glyphs do not fit a frame's uint16")
	}

	p := &sim.Profile{Gender: sim.GenderMan, Age: 30, Traits: []sim.Trait{sim.TraitMutant}}
	e := sim.EntityView{Kind: sim.Colonist, Profile: p}
	k, ok := glyphs.DollKey(e)
	if !ok {
		t.Fatal("mutant has no doll")
	}
	if got := int(entityGlyph(e, 2)); got != d.Base+k {
		t.Fatalf("mutant glyph %d, want %d", got, d.Base+k)
	}
	e.State = sim.Fleeing
	if got := int(entityGlyph(e, 2)); got >= len(h.Glyphs.Symbols) {
		t.Fatalf("a fleeing mutant got glyph %d, past the symbols", got)
	}
	if row := rosterRow(e); row.Doll != k {
		t.Fatalf("roster doll %d, want %d", row.Doll, k)
	}
}
