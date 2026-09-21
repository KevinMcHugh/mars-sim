package tui

import (
	"sync/atomic"

	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// tileWidth is how many terminal cells one map tile occupies. Every glyph is
// rendered into exactly this many cells, so a map row is always cols*tileWidth
// cells wide no matter what it contains.
const tileWidth = 2

// How wide a terminal paints an emoji is a property of the terminal, not of
// Unicode: it depends on which Unicode version the terminal's width table came
// from and how it treats emoji presentation. Nothing we can compute from the
// string is authoritative. What we can do is refuse to draw anything whose
// width is *contested*, and then check our assumption against the real terminal
// at startup.
//
// The contested constructs, all of which this package bans (enforced by
// TestGlyphRegistryIsUnambiguous):
//
//   - U+FE0F VARIATION SELECTOR-16. "🛏️" is U+1F6CF + VS16: a terminal that
//     honours the selector paints two cells, one that ignores it paints the
//     text-presentation form in one. Our own dependencies disagree about this
//     today — go-runewidth says one cell, x/ansi and uniseg say two — which is
//     exactly the class of bug that shears the grid.
//   - U+200D ZERO WIDTH JOINER, for multi-person and profession sequences.
//   - U+1F3FB..U+1F3FF skin tone modifiers, which a terminal that does not fuse
//     them paints as a separate coloured square.
//
// Every glyph below is therefore a single code point with
// Emoji_Presentation=Yes, the one case terminals agree on: two cells, always.
// Skin tone and hair colour live in the colonist's flavour text instead (see
// renderColonistDetail); they never enter a glyph.
const (
	glyphRock        = "\U0001F7E5" // 🟥 unexcavated regolith: Mars is the red planet
	glyphIronRock    = "\U00002B1B" // ⬛ iron-bearing rock
	glyphIceRock     = "\U0001F7E6" // 🟦 water ice-bearing rock
	glyphUranium     = "\U0001F7E9" // 🟩 uranium-bearing rock (the glow is the warning)
	glyphClayRock    = "\U0001F7E7" // 🟧 clay-bearing rock
	glyphFloor       = "  "         // open, walkable space
	glyphWall        = "\U0001F9F1" // 🧱 built wall
	glyphPod         = "\U0001F96B" // 🥫 nutrient pod (food)
	glyphToilet      = "\U0001F6BD" // 🚽 toilet (bladder)
	glyphBed         = "\U0001F6CC" // 🛌 dormitory bunk (sleep)
	glyphIncinerator = "\U0001F525" // 🔥 incinerator: burns refuse hauled to the trash room
	glyphStorage     = "\U0001F9F0" // 🧰 storage container: six colonist inventories

	glyphColonist = "\U0001F477" // 👷 colonist of unknown age/gender (no profile)
	glyphFleeing  = "\U0001F631" // 😱 colonist running from an alien
	glyphTalking  = "\U0001F4AC" // 💬 colonist chatting with another
	glyphAlien    = "\U0001F47D" // 👽 subterranean mutant
	glyphCat      = "\U0001F408" // 🐈 floor predator hunting mice
	glyphMouse    = "\U0001F401" // 🐁 pest that raids the food pods
	glyphStomp    = "\U0001F97E" // 🥾 colonist chasing down a mouse to stomp it
	glyphFighting = "\U0001F52B" // 🔫 armed colonist standing its ground against an alien
	glyphGore     = "\U0001FA78" // 🩸 a violent death's residue on a tile
	glyphCorpse   = "\U0001F9B4" // 🦴 a body left where something died, waiting to be hauled
	glyphCleaning = "\U0001F9F9" // 🧹 colonist scrubbing refuse up or feeding the incinerator
	glyphHauling  = "\U0001F4E6" // 📦 colonist carrying refuse to the incinerator
	glyphMutant   = "\U0001F9DF" // 🧟 colonist changed by uranium (see docs/mutation.md)

	// Alien species flavor glyphs: the curated set an AlienSpecies.Emoji (see
	// docs/lore.md and internal/sim/alien-names.yaml) can draw from and still
	// reach the map, via alienGlyph. A species' rolled Emoji is arbitrary
	// runtime data — sim knows nothing about glyphs or widths — so only a
	// string that matches one of these vetted, registered symbols is ever
	// drawn on the map; anything else (a custom -alien-names file's own
	// choice, say) falls back to glyphAlien rather than reaching fitGlyph
	// unvetted.
	glyphLizard       = "\U0001F98E" // 🦎
	glyphSnake        = "\U0001F40D" // 🐍
	glyphTurtle       = "\U0001F422" // 🐢
	glyphTRex         = "\U0001F996" // 🦖
	glyphSauropod     = "\U0001F995" // 🦕
	glyphCaterpillar  = "\U0001F41B" // 🐛
	glyphBeetle       = "\U0001FAB2" // 🪲
	glyphAnt          = "\U0001F41C" // 🐜
	glyphCricket      = "\U0001F997" // 🦗
	glyphScorpion     = "\U0001F982" // 🦂
	glyphWorm         = "\U0001FAB1" // 🪱
	glyphSaucer       = "\U0001F6F8" // 🛸
	glyphMicrobe      = "\U0001F9A0" // 🦠
	glyphSpaceInvader = "\U0001F47E" // 👾

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

// seniorAge is the age at which a colonist's default glyph switches from an
// adult to a senior variant.
const seniorAge = 60

// glyph is one drawable symbol: what we want to draw, how many cells we claim
// it takes, and what to draw instead on a terminal that disagrees.
//
// The fallback is not decoration. A glyph whose real width differs from cells
// by even one column desyncs the line it is on, and the fallback is pure ASCII,
// whose width no terminal has ever disputed.
type glyph struct {
	symbol   string // the emoji we would like to draw
	cells    int    // how many terminal cells we claim symbol occupies
	fallback string // ASCII stand-in, exactly tileWidth cells
}

// glyphRegistry is every symbol the UI may draw. Rendering goes through it, so
// a glyph added to the code without an entry here fails a test rather than
// quietly corrupting a frame at runtime.
var glyphRegistry = map[string]glyph{
	glyphRock:        {glyphRock, 2, "##"},
	glyphIronRock:    {glyphIronRock, 2, "Fe"},
	glyphIceRock:     {glyphIceRock, 2, "H2"},
	glyphUranium:     {glyphUranium, 2, "U "},
	glyphClayRock:    {glyphClayRock, 2, "Cl"},
	glyphFloor:       {glyphFloor, 2, "  "},
	glyphWall:        {glyphWall, 2, "[]"},
	glyphPod:         {glyphPod, 2, "%%"},
	glyphToilet:      {glyphToilet, 2, "WC"},
	glyphBed:         {glyphBed, 2, "=="},
	glyphIncinerator: {glyphIncinerator, 2, "&&"},
	glyphStorage:     {glyphStorage, 2, "[]"},

	glyphColonist: {glyphColonist, 2, "@ "},
	glyphFleeing:  {glyphFleeing, 2, "@!"},
	glyphTalking:  {glyphTalking, 2, "@?"},
	glyphAlien:    {glyphAlien, 2, "A "},
	glyphCat:      {glyphCat, 2, "f "},
	glyphMouse:    {glyphMouse, 2, "r "},
	glyphStomp:    {glyphStomp, 2, "@*"},
	glyphFighting: {glyphFighting, 2, "@="},
	glyphGore:     {glyphGore, 2, "~~"},
	glyphCorpse:   {glyphCorpse, 2, "%!"},
	glyphCleaning: {glyphCleaning, 2, "@/"},
	glyphHauling:  {glyphHauling, 2, "@+"},
	glyphMutant:   {glyphMutant, 2, "@%"},

	glyphLizard:       {glyphLizard, 2, "Lz"},
	glyphSnake:        {glyphSnake, 2, "Sn"},
	glyphTurtle:       {glyphTurtle, 2, "Tu"},
	glyphTRex:         {glyphTRex, 2, "Rx"},
	glyphSauropod:     {glyphSauropod, 2, "Sp"},
	glyphCaterpillar:  {glyphCaterpillar, 2, "Bg"},
	glyphBeetle:       {glyphBeetle, 2, "Bt"},
	glyphAnt:          {glyphAnt, 2, "An"},
	glyphCricket:      {glyphCricket, 2, "Cr"},
	glyphScorpion:     {glyphScorpion, 2, "Sc"},
	glyphWorm:         {glyphWorm, 2, "Wm"},
	glyphSaucer:       {glyphSaucer, 2, "UF"},
	glyphMicrobe:      {glyphMicrobe, 2, "Mb"},
	glyphSpaceInvader: {glyphSpaceInvader, 2, "SI"},

	glyphManAdult:     {glyphManAdult, 2, "M "},
	glyphWomanAdult:   {glyphWomanAdult, 2, "W "},
	glyphPersonAdult:  {glyphPersonAdult, 2, "P "},
	glyphManSenior:    {glyphManSenior, 2, "m "},
	glyphWomanSenior:  {glyphWomanSenior, 2, "w "},
	glyphPersonSenior: {glyphPersonSenior, 2, "p "},

	glyphMars: {glyphMars, 2, "()"},
}

// renderedGlyphs maps each registered symbol to the string actually written to
// the terminal, pre-fitted to tileWidth cells. It is replaced wholesale by
// useASCIIGlyphs when the startup probe catches the terminal disagreeing with
// the registry, so reads must be atomic: the probe runs on the main goroutine
// before the program starts, but View runs on Bubble Tea's.
var renderedGlyphs atomic.Pointer[map[string]string]

// asciiGlyphs tracks which set renderedGlyphs currently holds, so the UI can
// mention the downgrade instead of leaving the player wondering where the
// emoji went.
var asciiGlyphs atomic.Bool

func init() { renderedGlyphs.Store(buildRenderedGlyphs(false)) }

func buildRenderedGlyphs(ascii bool) *map[string]string {
	out := make(map[string]string, len(glyphRegistry))
	for symbol, g := range glyphRegistry {
		if ascii {
			out[symbol] = cells.Fit(g.fallback, tileWidth)
			continue
		}
		out[symbol] = cells.Fit(g.symbol, tileWidth)
	}
	return &out
}

// useASCIIGlyphs switches the whole UI to the ASCII fallback set. It is all or
// nothing on purpose: a half-emoji, half-ASCII map is harder to read than
// either, and if the terminal got one glyph's width wrong there is no reason to
// trust it about the rest.
func useASCIIGlyphs() {
	renderedGlyphs.Store(buildRenderedGlyphs(true))
	asciiGlyphs.Store(true)
}

// usingASCIIGlyphs reports whether the fallback set is active.
func usingASCIIGlyphs() bool { return asciiGlyphs.Load() }

// fitGlyph returns the drawable form of a symbol: exactly tileWidth cells,
// ASCII if the terminal has been caught disagreeing about emoji widths.
//
// An unregistered symbol still gets fitted rather than dropped — a wrong glyph
// beats a broken grid — but TestGlyphRegistryCoversEveryGlyph makes sure we do
// not ship one.
func fitGlyph(symbol string) string {
	if rendered, ok := (*renderedGlyphs.Load())[symbol]; ok {
		return rendered
	}
	return cells.Fit(symbol, tileWidth)
}

// colonistGlyph picks the default map glyph for a colonist at rest: a base
// figure for their gender identity and age bracket. A colonist without a
// profile falls back to glyphColonist.
//
// A mutant overrides all of that. What uranium did to them is the most
// important thing about that figure on the map — it is why the colony treats
// them differently — and it is not something a gender/age figure can show.
func colonistGlyph(p *sim.Profile) string {
	if p == nil {
		return glyphColonist
	}
	if p.HasTrait(sim.TraitMutant) {
		return glyphMutant
	}
	senior := p.Age >= seniorAge
	switch p.Gender {
	case sim.GenderMan:
		if senior {
			return glyphManSenior
		}
		return glyphManAdult
	case sim.GenderWoman:
		if senior {
			return glyphWomanSenior
		}
		return glyphWomanAdult
	default:
		if senior {
			return glyphPersonSenior
		}
		return glyphPersonAdult
	}
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
	case sim.Incinerator:
		symbol = glyphIncinerator
	case sim.Storage:
		symbol = glyphStorage
	default:
		symbol = glyphRock
	}
	return fitGlyph(symbol)
}

// tileGlyph draws an empty tile: refuse takes priority over bare terrain (and
// over the ore in it), since it is the more notable thing to see there, and a
// body outranks the stains around it — it is what a colonist is coming to haul
// away. Any of a stomp, a bite, or a gunshot can leave gore (see
// docs/combat.md); a body is left by a death nothing ate (see
// docs/sanitation.md).
func tileGlyph(tile sim.Tile) string {
	if tile.Corpses > 0 {
		return fitGlyph(glyphCorpse)
	}
	if tile.Gore > 0 {
		return fitGlyph(glyphGore)
	}
	if tile.Terrain != sim.Rock {
		return terrainGlyph(tile.Terrain)
	}
	switch tile.Composition {
	case sim.IronBearingRock:
		return fitGlyph(glyphIronRock)
	case sim.WaterIceBearingRock:
		return fitGlyph(glyphIceRock)
	case sim.UraniumBearingRock:
		return fitGlyph(glyphUranium)
	case sim.ClayBearingRock:
		return fitGlyph(glyphClayRock)
	default:
		return fitGlyph(glyphRock)
	}
}

// alienGlyph picks a specific alien's map glyph: its species' rolled emoji
// (see sim.AlienSpecies.Emoji), if it names one of this package's registered
// glyphs, or the generic glyphAlien otherwise. sim carries Emoji as opaque
// data — it could be any string a -alien-names file supplied — so this is
// the one place that decides whether to trust it for the map's fixed
// two-cell tile: a registered symbol has a declared width and an ASCII
// fallback and reaches fitGlyph the ordinary way, exactly like every other
// glyph; anything unrecognized draws the same generic alien every species
// used to. See docs/lore.md.
func alienGlyph(sp sim.AlienSpecies) string {
	if sp.Emoji != "" {
		if _, ok := glyphRegistry[sp.Emoji]; ok {
			return sp.Emoji
		}
	}
	return glyphAlien
}

func entityGlyph(e sim.EntityView) string {
	var symbol string
	switch e.Kind {
	case sim.Alien:
		symbol = alienGlyph(e.AlienSpecies)
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
		case sim.Fighting:
			symbol = glyphFighting
		case sim.Cleaning:
			symbol = glyphCleaning
		case sim.Hauling:
			symbol = glyphHauling
		case sim.Storing:
			symbol = glyphHauling
		default:
			symbol = colonistGlyph(e.Profile)
		}
	default:
		symbol = glyphColonist
	}
	return fitGlyph(symbol)
}
