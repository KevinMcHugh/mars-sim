package tui

import (
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"
)

// A dark-skinned, red-haired man: the sequence that started this, and the one
// with the most rungs to fall through.
const (
	manDarkRed  = glyphManAdult + "\U0001F3FF" + zwj + "\U0001F9B0" // 👨🏿‍🦰
	manDarkSkin = glyphManAdult + "\U0001F3FF"                      // 👨🏿
)

// The ladder is the whole point of the composed tier: each rung the terminal
// refuses drops to a simpler figure that still means the same colonist, and
// only the bottom falls all the way to ASCII.
func TestLadderStepsDownOneRungAtATime(t *testing.T) {
	cases := []struct {
		name     string
		rejected []string
		ascii    bool
		want     string
	}{
		{
			name: "terminal fuses everything: full detail",
			want: manDarkRed,
		},
		{
			name:     "no hair fusion: keeps the skin tone",
			rejected: []string{manDarkRed},
			want:     manDarkSkin,
		},
		{
			name:     "no modifiers at all: plain figure",
			rejected: []string{manDarkRed, manDarkSkin},
			want:     glyphManAdult,
		},
		{
			name:     "even the figure is wrong: ASCII",
			rejected: []string{manDarkRed, manDarkSkin, glyphManAdult},
			want:     "M ",
		},
		{
			name:  "forced ASCII skips every rung",
			ascii: true,
			want:  "M ",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rejected := make(map[string]bool, len(tc.rejected))
			for _, s := range tc.rejected {
				rejected[s] = true
			}
			got := resolveGlyph(manDarkRed, rejected, tc.ascii)
			if got != tc.want {
				t.Errorf("resolveGlyph(%+q) = %+q, want %+q", manDarkRed, got, tc.want)
			}
			if w := cells.Width(cells.Fit(got, tileWidth)); w != tileWidth {
				t.Errorf("rung %+q does not fit one tile", got)
			}
		})
	}
}

// Rejecting one sequence must not touch the others. The old all-or-nothing
// behaviour would have dropped the whole map to ASCII over a single
// unfusable glyph, which is what the ladder exists to avoid.
func TestOneUnfusableGlyphDoesNotDowngradeTheRest(t *testing.T) {
	rejected := map[string]bool{manDarkRed: true}
	rendered := *buildRenderedGlyphs(rejected, false)

	if got := rendered[manDarkRed]; got != cells.Fit(manDarkSkin, tileWidth) {
		t.Errorf("the rejected glyph rendered as %+q, want the skin-tone rung %+q", got, manDarkSkin)
	}
	for _, unaffected := range []string{glyphRock, glyphAlien, glyphManAdult, manDarkSkin} {
		if got := rendered[unaffected]; got != cells.Fit(unaffected, tileWidth) {
			t.Errorf("glyph %+q was reduced to %+q, but nothing was wrong with it", unaffected, got)
		}
	}
	if usingASCIIGlyphs() {
		t.Error("a single unfusable composed glyph should not put the whole UI in ASCII mode")
	}
}

// Every colonist the simulation can generate must map to a registered glyph.
// An unregistered one would skip the probe, which is exactly how an unvetted
// sequence would reach the terminal.
func TestEveryColonistProfileMapsToARegisteredGlyph(t *testing.T) {
	genders := []sim.Gender{sim.GenderMan, sim.GenderWoman, sim.GenderNonbinary}
	tones := []sim.SkinTone{sim.SkinLight, sim.SkinMediumLight, sim.SkinMedium, sim.SkinMediumDark, sim.SkinDark}
	hairs := []sim.HairColor{sim.HairBlack, sim.HairBrown, sim.HairBlonde, sim.HairRed, sim.HairWhite, sim.HairBald}
	ages := []int{18, 30, seniorAge - 1, seniorAge, 80}

	seen := make(map[string]bool)
	for _, g := range genders {
		for _, tone := range tones {
			for _, hair := range hairs {
				for _, age := range ages {
					p := &sim.Profile{Gender: g, SkinTone: tone, HairColor: hair, Age: age}
					symbol := colonistGlyph(p)
					seen[symbol] = true
					if _, ok := glyphRegistry[symbol]; !ok {
						t.Errorf("profile (%v, %v hair, %v skin, age %d) produced unregistered glyph %+q",
							g, hair, tone, age, symbol)
					}
					if w := cells.Width(fitGlyph(symbol)); w != tileWidth {
						t.Errorf("profile glyph %+q renders %d cells, want %d", symbol, w, tileWidth)
					}
				}
			}
		}
	}

	// Sanity: the combinations should be producing genuinely distinct glyphs,
	// not collapsing to one figure because a lookup silently fell through.
	if len(seen) < 40 {
		t.Errorf("only %d distinct colonist glyphs across every profile; the mapping is collapsing", len(seen))
	}
}

// Seniors deliberately stop at a skin tone: emoji pairs hair components with
// the adult figures only, so a senior-plus-hair sequence is one no font is
// obliged to carry.
func TestSeniorsTakeASkinToneButNoHairComponent(t *testing.T) {
	senior := &sim.Profile{Gender: sim.GenderMan, SkinTone: sim.SkinDark, HairColor: sim.HairRed, Age: seniorAge}
	got := colonistGlyph(senior)
	want := glyphManSenior + "\U0001F3FF"
	if got != want {
		t.Errorf("senior glyph = %+q, want %+q (skin tone, no hair component)", got, want)
	}

	adult := &sim.Profile{Gender: sim.GenderMan, SkinTone: sim.SkinDark, HairColor: sim.HairRed, Age: seniorAge - 1}
	if colonistGlyph(adult) != manDarkRed {
		t.Errorf("adult glyph = %+q, want %+q", colonistGlyph(adult), manDarkRed)
	}
}

// Hair colours emoji has no component for must not silently produce a
// malformed sequence — they simply stop at the skin tone.
func TestHairColoursWithoutAComponentStopAtSkinTone(t *testing.T) {
	for _, hair := range []sim.HairColor{sim.HairBlack, sim.HairBrown, sim.HairBlonde} {
		p := &sim.Profile{Gender: sim.GenderMan, SkinTone: sim.SkinDark, HairColor: hair, Age: 30}
		if got := colonistGlyph(p); got != manDarkSkin {
			t.Errorf("%v hair produced %+q, want the plain skin-tone glyph %+q", hair, got, manDarkSkin)
		}
	}
}

// A profile carrying a skin tone outside the five-point scale must degrade to
// the bare figure rather than build a sequence out of a bogus modifier.
func TestOutOfRangeSkinToneFallsBackToTheBareFigure(t *testing.T) {
	p := &sim.Profile{Gender: sim.GenderWoman, SkinTone: sim.SkinTone(99), HairColor: sim.HairRed, Age: 30}
	if got := colonistGlyph(p); got != glyphWomanAdult {
		t.Errorf("out-of-range skin tone produced %+q, want %+q", got, glyphWomanAdult)
	}
}
