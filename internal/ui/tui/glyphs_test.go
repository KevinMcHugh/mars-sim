package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// Contested code points: the constructs whose painted width terminals do not
// agree on. See the commentary in glyphs.go for why each one breaks the grid.
const (
	variationSelector16 = '️'
	zeroWidthJoiner     = '‍'
	skinToneFirst       = '\U0001F3FB'
	skinToneLast        = '\U0001F3FF'
)

// This is the test that has to stop the bug recurring, so it checks the
// *structure* of each glyph rather than just measuring it. Measuring only tells
// us what our width tables think today; the structural rules are what make the
// terminal agree with them.
func TestGlyphRegistryIsUnambiguous(t *testing.T) {
	for symbol, g := range glyphRegistry {
		if symbol != g.symbol {
			t.Errorf("registry key %q does not match its glyph symbol %q", symbol, g.symbol)
		}

		// A glyph must be one grapheme cluster, so the terminal makes exactly
		// one width decision about it. Open floor is the deliberate exception:
		// it is plain spaces, not a symbol at all.
		if symbol != glyphFloor {
			if n := uniseg.GraphemeClusterCount(symbol); n != 1 {
				t.Errorf("glyph %+q is %d grapheme clusters, want 1", symbol, n)
			}
		}

		for _, r := range symbol {
			switch {
			case r == variationSelector16:
				t.Errorf("glyph %+q contains U+FE0F: terminals disagree whether it paints one cell or two", symbol)
			case r == zeroWidthJoiner:
				t.Errorf("glyph %+q contains U+200D: terminals that do not fuse the sequence paint each part separately", symbol)
			case r >= skinToneFirst && r <= skinToneLast:
				t.Errorf("glyph %+q contains a skin tone modifier: an unfused modifier paints as its own coloured square", symbol)
			}
		}

		// Every width table we can reach must give the same answer as the
		// registry. These libraries are built from different Unicode versions,
		// so agreement between them is decent evidence that terminals — also
		// built from assorted Unicode versions — will agree too. This is the
		// check that fails on a VS16 glyph even if someone deletes the rule
		// above: go-runewidth calls "\U0001F6CF️" one cell.
		measures := map[string]int{
			"x/ansi (what bubbletea and lipgloss use)": ansi.StringWidth(symbol),
			"uniseg":       uniseg.StringWidth(symbol),
			"go-runewidth": runewidth.StringWidth(symbol),
			"cells.Width":  cells.Width(symbol),
		}
		for lib, got := range measures {
			if got != g.cells {
				t.Errorf("glyph %+q: %s measures %d cells, registry declares %d", symbol, lib, got, g.cells)
			}
		}

		if g.cells != tileWidth {
			t.Errorf("glyph %+q declares %d cells, want %d — one tile is one glyph", symbol, g.cells, tileWidth)
		}

		// The fallback has to be a drop-in replacement, so it must already be
		// the right width without padding, and must be ASCII — the only thing
		// no terminal has ever measured differently.
		if w := cells.Width(g.fallback); w != tileWidth {
			t.Errorf("glyph %+q fallback %q is %d cells, want %d", symbol, g.fallback, w, tileWidth)
		}
		for _, r := range g.fallback {
			if r >= utf8.RuneSelf {
				t.Errorf("glyph %+q fallback %q contains non-ASCII %+q", symbol, g.fallback, r)
			}
		}
	}
}

// Both glyph sets must render every tile in exactly tileWidth cells, since the
// map's column arithmetic assumes it unconditionally.
func TestFitGlyphAlwaysFillsOneTile(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		rendered := *buildRenderedGlyphs(ascii)
		if len(rendered) != len(glyphRegistry) {
			t.Fatalf("ascii=%v: rendered %d glyphs, registry has %d", ascii, len(rendered), len(glyphRegistry))
		}
		for symbol, drawn := range rendered {
			if w := cells.Width(drawn); w != tileWidth {
				t.Errorf("ascii=%v: glyph %+q renders as %q (%d cells), want %d", ascii, symbol, drawn, w, tileWidth)
			}
		}
	}
}

// Every glyph the renderer can produce must be registered, or the probe cannot
// vet it and the fallback set has nothing to swap in. Rather than list the
// glyphs again (a list that would drift), this parses glyphs.go and requires
// every glyph* constant declared there to have an entry.
func TestGlyphRegistryCoversEveryGlyph(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "glyphs.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing glyphs.go: %v", err)
	}

	found := 0
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		name := spec.Names[0].Name
		if !strings.HasPrefix(name, "glyph") || name == "glyphRegistry" {
			return true
		}
		lit, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		found++
		if _, ok := glyphRegistry[value]; !ok {
			t.Errorf("%s = %+q is not in glyphRegistry: add it (with a declared width and an ASCII fallback) so the startup probe can vet it", name, value)
		}
		return true
	})

	if found != len(glyphRegistry) {
		t.Errorf("found %d glyph constants but the registry has %d entries; a registry entry with no constant is dead weight", found, len(glyphRegistry))
	}
	if found == 0 {
		t.Error("found no glyph constants in glyphs.go — this test is no longer checking anything")
	}
}

// A glyph is only safe if it reaches the terminal through fitGlyph. This scans
// the package's own rendering code for raw emoji in string literals, which is
// how the header's counts line drifted out of the registry before.
func TestNoRawEmojiOutsideTheRegistry(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing package: %v", err)
	}

	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			if strings.HasSuffix(path, "glyphs.go") {
				continue // the registry is where the literals are supposed to live
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				for _, r := range value {
					if isEmoji(r) || r == variationSelector16 || r == zeroWidthJoiner {
						t.Errorf("%s: literal %s contains %+q; use a glyph constant through fitGlyph instead",
							fset.Position(lit.Pos()), lit.Value, r)
						return false
					}
				}
				return true
			})
		}
	}
}

// isEmoji is a deliberately loose test for "a code point whose terminal width
// we should not be guessing at in an ad-hoc string literal".
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1FAFF: // pictographs, symbols, emoticons, transport
		return true
	case r >= 0x2600 && r <= 0x27BF: // misc symbols and dingbats
		return true
	default:
		return false
	}
}
