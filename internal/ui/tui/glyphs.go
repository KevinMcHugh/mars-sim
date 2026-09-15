package tui

import (
	"sync/atomic"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"
)

// tileWidth is how many terminal cells one map tile occupies. Every glyph is
// rendered into exactly this many cells, so a map row is always cols*tileWidth
// cells wide no matter what it contains.
const tileWidth = 2

// How wide a terminal paints an emoji is a property of the terminal, not of
// Unicode: it depends on which Unicode version the terminal's width table came
// from, how it treats emoji presentation, and whether it fuses multi-code-point
// sequences. Nothing we can compute from the string is authoritative.
//
// Glyphs come in two tiers, and the difference is how we establish their width.
//
// An ATOMIC glyph is a single code point with Emoji_Presentation=Yes. Every
// terminal agrees these are two cells, so the registry's declared width is
// trustworthy without asking. Atomic glyphs must not contain a variation
// selector, a ZWJ or a skin tone modifier — the constructs terminals disagree
// about (TestAtomicGlyphsAreUnambiguous enforces this). Our own dependencies
// disagree about VS16 today: go-runewidth calls "\U0001F6CF️" one cell
// while x/ansi and uniseg call it two, which is exactly the shear that started
// all this.
//
// A COMPOSED glyph is a base figure plus a skin tone modifier and optionally a
// ZWJ hair component: "\U0001F468\U0001F3FF‍\U0001F9B0" is 👨🏿‍🦰. These are
// two cells *if the terminal fuses the sequence*, and there is no static way to
// find out. Every width table we have says two cells — x/ansi, uniseg and
// go-runewidth all agree — because two cells is the spec answer. A terminal
// that does not fuse paints the parts separately and takes four or six cells
// instead, and no library will warn us. Only measurement settles it, which is
// why a composed glyph must be probed (see probe.go) and must declare a
// `reduce` target: the rung below it on the ladder.
const (
	glyphRock   = "\U0001F7EB" // 🟫 unexcavated regolith
	glyphFloor  = "  "         // open, walkable space
	glyphWall   = "\U0001F9F1" // 🧱 built wall
	glyphPod    = "\U0001F96B" // 🥫 nutrient pod (food)
	glyphToilet = "\U0001F6BD" // 🚽 toilet (bladder)
	glyphBed    = "\U0001F6CC" // 🛌 dormitory bunk (sleep)

	glyphColonist = "\U0001F477" // 👷 colonist of unknown age/gender (no profile)
	glyphFleeing  = "\U0001F631" // 😱 colonist running from an alien
	glyphTalking  = "\U0001F4AC" // 💬 colonist chatting with another
	glyphAlien    = "\U0001F47D" // 👽 subterranean mutant
	glyphCat      = "\U0001F408" // 🐈 floor predator hunting mice
	glyphMouse    = "\U0001F401" // 🐁 pest that raids the food pods
	glyphStomp    = "\U0001F97E" // 🥾 colonist chasing down a mouse to stomp it

	glyphManAdult     = "\U0001F468" // 👨 adult man colonist
	glyphWomanAdult   = "\U0001F469" // 👩 adult woman colonist
	glyphPersonAdult  = "\U0001F9D1" // 🧑 adult non-binary colonist
	glyphManSenior    = "\U0001F474" // 👴 senior man colonist
	glyphWomanSenior  = "\U0001F475" // 👵 senior woman colonist
	glyphPersonSenior = "\U0001F9D3" // 🧓 senior non-binary colonist

	// glyphMars is the header's planet mark. It is drawn inline rather than on
	// the grid, but it goes through the registry like everything else so the
	// startup probe covers it too.
	glyphMars = "\U0001F534" // 🔴
)

// zwj joins a base figure to a hair component.
const zwj = "‍"

// seniorAge is the age at which a colonist's default glyph switches from an
// adult to a senior variant.
const seniorAge = 60

// skinToneModifiers are the five Fitzpatrick-scale modifiers, indexed by
// sim.SkinTone — which is defined on the same five points precisely so this
// mapping is an index rather than a translation.
var skinToneModifiers = [...]string{
	sim.SkinLight:       "\U0001F3FB",
	sim.SkinMediumLight: "\U0001F3FC",
	sim.SkinMedium:      "\U0001F3FD",
	sim.SkinMediumDark:  "\U0001F3FE",
	sim.SkinDark:        "\U0001F3FF",
}

// hairComponents are the hair colours emoji has a component for. Black, brown
// and blonde have none, so those colonists draw the unmodified figure and their
// hair colour lives in the inspector's flavour text (see renderColonistDetail).
var hairComponents = []struct {
	colour    sim.HairColor
	component string
}{
	{sim.HairRed, "\U0001F9B0"},   // 🦰
	{sim.HairWhite, "\U0001F9B3"}, // 🦳
	{sim.HairBald, "\U0001F9B2"},  // 🦲
}

// colonistBases are the six figures a colonist's glyph is built from.
//
// Only the adult figures take hair components. The RGI sequences pair hair with
// 👨 👩 🧑 and not with 👴 👵 🧓, so a senior-plus-hair sequence is one no font
// is obliged to carry and most would paint decomposed. Seniors get a skin tone
// and stop there — they already read as grey-haired. This is the one claim here
// we could not check against the Unicode data files directly, and it is also
// the one that costs least if it is wrong: the probe would reject the sequence
// and the ladder would drop back to the skin-tone rung on its own.
var colonistBases = []struct {
	symbol string
	ascii  string
	gender sim.Gender
	senior bool
}{
	{glyphManAdult, "M ", sim.GenderMan, false},
	{glyphWomanAdult, "W ", sim.GenderWoman, false},
	{glyphPersonAdult, "P ", sim.GenderNonbinary, false},
	{glyphManSenior, "m ", sim.GenderMan, true},
	{glyphWomanSenior, "w ", sim.GenderWoman, true},
	{glyphPersonSenior, "p ", sim.GenderNonbinary, true},
}

// glyph is one drawable symbol and what to do when the terminal will not paint
// it at the width we expect.
//
// Exactly one of reduce and fallback is set. A composed glyph has a reduce: the
// next rung down, a simpler sequence meaning the same thing. An atomic glyph
// has a fallback: pure ASCII, whose width no terminal has ever disputed, and
// which is where every ladder ends.
type glyph struct {
	symbol   string
	cells    int    // how many terminal cells we claim symbol occupies
	reduce   string // composed: the next rung down
	fallback string // atomic: ASCII stand-in, exactly tileWidth cells
}

func (g glyph) composed() bool { return g.reduce != "" }

// glyphRegistry is every symbol the UI may draw. Rendering goes through it, so
// a glyph used without an entry here fails a test rather than quietly
// corrupting a frame at runtime.
var glyphRegistry = buildGlyphRegistry()

func buildGlyphRegistry() map[string]glyph {
	reg := map[string]glyph{
		glyphRock:   {symbol: glyphRock, cells: 2, fallback: "##"},
		glyphFloor:  {symbol: glyphFloor, cells: 2, fallback: "  "},
		glyphWall:   {symbol: glyphWall, cells: 2, fallback: "[]"},
		glyphPod:    {symbol: glyphPod, cells: 2, fallback: "%%"},
		glyphToilet: {symbol: glyphToilet, cells: 2, fallback: "WC"},
		glyphBed:    {symbol: glyphBed, cells: 2, fallback: "=="},

		glyphColonist: {symbol: glyphColonist, cells: 2, fallback: "@ "},
		glyphFleeing:  {symbol: glyphFleeing, cells: 2, fallback: "@!"},
		glyphTalking:  {symbol: glyphTalking, cells: 2, fallback: "@?"},
		glyphAlien:    {symbol: glyphAlien, cells: 2, fallback: "A "},
		glyphCat:      {symbol: glyphCat, cells: 2, fallback: "f "},
		glyphMouse:    {symbol: glyphMouse, cells: 2, fallback: "r "},
		glyphStomp:    {symbol: glyphStomp, cells: 2, fallback: "@*"},

		glyphMars: {symbol: glyphMars, cells: 2, fallback: "()"},
	}

	// Each colonist figure seeds a ladder: figure+skin+hair reduces to
	// figure+skin, which reduces to the bare figure, which falls back to ASCII.
	for _, base := range colonistBases {
		reg[base.symbol] = glyph{symbol: base.symbol, cells: 2, fallback: base.ascii}
		for _, modifier := range skinToneModifiers {
			skin := base.symbol + modifier
			reg[skin] = glyph{symbol: skin, cells: 2, reduce: base.symbol}
			if base.senior {
				continue
			}
			for _, hair := range hairComponents {
				haired := skin + zwj + hair.component
				reg[haired] = glyph{symbol: haired, cells: 2, reduce: skin}
			}
		}
	}
	return reg
}

// renderedGlyphs maps each registered symbol to the string actually written to
// the terminal, pre-fitted to tileWidth cells and already reduced to whatever
// rung this terminal accepts. It is replaced wholesale when the probe reports
// back, so reads must be atomic: the probe runs on the main goroutine before
// the program starts, but View runs on Bubble Tea's.
var renderedGlyphs atomic.Pointer[map[string]string]

// asciiGlyphs tracks whether the whole UI has been downgraded to ASCII, so the
// footer can mention it instead of leaving the player wondering where the emoji
// went.
var asciiGlyphs atomic.Bool

func init() { renderedGlyphs.Store(buildRenderedGlyphs(nil, false)) }

// buildRenderedGlyphs resolves every registered symbol down its ladder, past
// any rung in rejected, to the first one this terminal will paint correctly.
func buildRenderedGlyphs(rejected map[string]bool, ascii bool) *map[string]string {
	out := make(map[string]string, len(glyphRegistry))
	for symbol := range glyphRegistry {
		out[symbol] = cells.Fit(resolveGlyph(symbol, rejected, ascii), tileWidth)
	}
	return &out
}

// resolveGlyph walks a ladder from symbol downwards and returns the first rung
// the terminal is willing to paint at the declared width.
//
// In ASCII mode it walks straight to the bottom. Otherwise it steps past each
// rejected rung — 👨🏿‍🦰 to 👨🏿 to 👨 — so a terminal that cannot fuse hair still
// shows a colonist with the right skin tone, rather than dropping the entire
// map to ASCII over one sequence.
func resolveGlyph(symbol string, rejected map[string]bool, ascii bool) string {
	// A ladder is at most three rungs; the bound is paranoia about a cycle
	// slipping into the registry, which TestLaddersTerminate also rules out.
	for i := 0; i <= len(glyphRegistry); i++ {
		g, ok := glyphRegistry[symbol]
		if !ok {
			// Unregistered: fit it and hope. A wrong glyph beats a broken grid,
			// and TestGlyphRegistryCoversEveryGlyph stops us shipping one.
			return symbol
		}
		if !g.composed() {
			if ascii {
				return g.fallback
			}
			if rejected[symbol] {
				return g.fallback
			}
			return g.symbol
		}
		if !ascii && !rejected[symbol] {
			return g.symbol
		}
		symbol = g.reduce
	}
	return "??"
}

// useASCIIGlyphs switches the whole UI to the ASCII fallback set.
//
// This is reserved for a terminal that gets an *atomic* glyph wrong. Those are
// single code points every width table agrees on, so a terminal that disagrees
// is one whose emoji handling cannot be trusted at all, and a half-emoji map is
// harder to read than an honest ASCII one. A composed glyph failing means only
// that this terminal cannot fuse that sequence, which the ladder handles
// per-glyph.
func useASCIIGlyphs() {
	renderedGlyphs.Store(buildRenderedGlyphs(nil, true))
	asciiGlyphs.Store(true)
}

// reduceRejectedGlyphs applies per-glyph ladder reductions for the composed
// sequences this terminal will not fuse, leaving everything else as emoji.
func reduceRejectedGlyphs(rejected map[string]bool) {
	renderedGlyphs.Store(buildRenderedGlyphs(rejected, false))
}

// usingASCIIGlyphs reports whether the fallback set is active.
func usingASCIIGlyphs() bool { return asciiGlyphs.Load() }

// fitGlyph returns the drawable form of a symbol: exactly tileWidth cells, and
// already reduced to a rung this terminal paints correctly.
func fitGlyph(symbol string) string {
	if rendered, ok := (*renderedGlyphs.Load())[symbol]; ok {
		return rendered
	}
	return cells.Fit(symbol, tileWidth)
}

// colonistGlyph builds the map glyph for a colonist at rest: a figure for their
// gender identity and age bracket, their skin tone, and a hair component when
// emoji has one for their hair colour.
//
// It always returns the *most specific* sequence. Reducing it to something this
// terminal can actually paint is fitGlyph's job, so the caller never has to
// know what the terminal supports.
func colonistGlyph(p *sim.Profile) string {
	if p == nil {
		return glyphColonist
	}
	senior := p.Age >= seniorAge
	base := glyphPersonAdult
	for _, b := range colonistBases {
		if b.gender == p.Gender && b.senior == senior {
			base = b.symbol
			break
		}
	}

	if int(p.SkinTone) >= len(skinToneModifiers) {
		return base // an unknown tone is no tone, not a corrupt sequence
	}
	symbol := base + skinToneModifiers[p.SkinTone]
	if senior {
		return symbol
	}
	for _, hair := range hairComponents {
		if hair.colour == p.HairColor {
			return symbol + zwj + hair.component
		}
	}
	return symbol
}

func terrainGlyph(t sim.Terrain) string {
	var symbol string
	switch t {
	case sim.Floor:
		symbol = glyphFloor
	case sim.Wall:
		symbol = glyphWall
	case sim.NutrientPod:
		symbol = glyphPod
	case sim.Toilet:
		symbol = glyphToilet
	case sim.Bed:
		symbol = glyphBed
	default:
		symbol = glyphRock
	}
	return fitGlyph(symbol)
}

func entityGlyph(e sim.EntityView) string {
	var symbol string
	switch e.Kind {
	case sim.Alien:
		symbol = glyphAlien
	case sim.Cat:
		symbol = glyphCat
	case sim.Mouse:
		symbol = glyphMouse
	case sim.Colonist:
		switch e.State {
		case sim.Fleeing:
			symbol = glyphFleeing
		case sim.Talking:
			symbol = glyphTalking
		case sim.Stomping:
			symbol = glyphStomp
		default:
			symbol = colonistGlyph(e.Profile)
		}
	default:
		symbol = glyphColonist
	}
	return fitGlyph(symbol)
}
