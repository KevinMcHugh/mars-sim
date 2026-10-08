package wire

import (
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
