// Package wire turns engine snapshots into the messages the browser frontend
// reads (see docs/browser-frontend.md and docs/wire-format.md).
//
// Two messages go from the engine to the page:
//
//   - Hello, once per game: the map's size, the enum names every other
//     message indexes into, and the Stats field names. JSON, because it is
//     sent once and is easier to read in a debugger that way.
//   - A frame, whenever a snapshot is published or the page is owed tiles:
//     a binary buffer (see Encoder and the layout in docs/wire-format.md),
//     laid out so the page can view each section as a typed array without
//     parsing it.
//
// The package is plain Go with no syscall/js, so it is tested natively; the
// WASM entry point (cmd/mars-sim-wasm) only copies its bytes across.
package wire

import (
	"reflect"

	"github.com/kevinmchugh/mars-sim/internal/glyphs"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// Version is the frame layout's version, carried in every frame header and in
// Hello. Bump it on any change to the layout, and update the decoder
// (web/wire/decode.js) and the golden files in the same change.
const Version = 7

// Hello is the once-per-game message.
type Hello struct {
	Version  int       `json:"version"`
	Width    int       `json:"width"`
	Height   int       `json:"height"`
	Seed     int64     `json:"seed"`
	PageSide int       `json:"pageSide"` // tiles per side of one frame page
	FogOfWar bool      `json:"fogOfWar"`
	Enums    sim.Enums `json:"enums"`
	// Stats names the frame's stats section, in order: frame stat i is
	// the Stats field Stats[i].
	Stats []string `json:"stats"`
	// Glyphs is the emoji set, for a frontend that draws them.
	Glyphs HelloGlyphs `json:"glyphs"`
	// GoreMax and ScumMax are the most gore and scum one tile holds, so the
	// page can shade a tile by how much is on it.
	GoreMax int `json:"goreMax"`
	ScumMax int `json:"scumMax"`
	// FlowFields names the shared flow fields, in the engine's order: the
	// flow section's field index, and the page's flow command, index this.
	FlowFields []string `json:"flowFields"`
	// LandingLevel is the level ships land on, the one the page shows
	// first; DeepestLevel the deepest the colony may dig to. A frame says
	// which level it shows and which levels exist so far. See
	// docs/z-levels.md.
	LandingLevel int `json:"landingLevel"`
	DeepestLevel int `json:"deepestLevel"`
	// SitingAuto is the siting-auto setting: whether the colony may pick
	// where to dig down itself (docs/siting.md), so the page can offer
	// orders without a tile.
	SitingAuto bool `json:"sitingAuto"`
	// Sandbox is the sandbox setting: an empty world the player fills by
	// hand with the place command, so the page shows its Sandbox tab
	// (docs/sandbox.md).
	Sandbox bool `json:"sandbox"`
}

// HelloGlyphs is the map's emoji (internal/glyphs) as the page needs them: the
// symbols, which a frame's entity glyphs index; which glyph each terrain draws
// as; and the refuse glyphs. A terrain whose glyph is only a patch of color
// (glyphs.Swatch: rock, floor, hull) has -1: the page draws its color instead.
type HelloGlyphs struct {
	Symbols []string `json:"symbols"`
	Terrain []int    `json:"terrain"`
	// Kinds is each creature kind's generic glyph (glyphs.ForKind), for
	// counts and legends.
	Kinds  []int `json:"kinds"`
	Gore   int   `json:"gore"`
	Corpse int   `json:"corpse"`
	// Looks are colonist figures in their own skin and hair (glyphs.Looks):
	// a frame's glyph len(Symbols)+i is Looks[i]. Each is a candidate list,
	// best first; the page draws the first its emoji font fuses into one
	// glyph, and the last is always a plain symbol. See
	// docs/colonist-looks.md.
	Looks []glyphs.Look `json:"looks"`
	// Sprites are the SVG sources of a species pack's aliens
	// (sim.Snapshot.Sprites): a frame's glyph len(Symbols)+len(Looks)+i is
	// Sprites[i]. Empty for a rolled roster. A page that cannot draw one
	// shows SpriteFallback in its place. See docs/species-pack.md.
	Sprites        []string `json:"sprites"`
	SpriteFallback int      `json:"spriteFallback"`
	// Dolls says how to read a glyph index past the sprites as a paper doll
	// (glyphs.DollKey). See docs/body-sprites.md.
	Dolls HelloDolls `json:"dolls"`
}

// HelloDolls is the doll key's layout. A frame's glyph Base+k is doll k, for
// k below Count. k is mixed-radix, least significant first: a bit per part in
// Parts, then the hair (an index into Hairs), the skin tone (0 to Tones-1),
// and the figure (an index into Figures). The page composes the doll's art
// from those digits, and until it has, draws a doll with any part grown as
// the symbol Fallback (the mutant) and any other as glyph Fallbacks[k >>
// len(Parts)], its look.
type HelloDolls struct {
	Base      int      `json:"base"`
	Count     int      `json:"count"`
	Figures   []string `json:"figures"`
	Tones     int      `json:"tones"`
	Hairs     []string `json:"hairs"`
	Parts     []string `json:"parts"`
	Fallback  int      `json:"fallback"`
	Fallbacks []int    `json:"fallbacks"`
}

// dollFigureNames name glyphs.DollFigures for the page's part library.
var dollFigureNames = map[string]string{
	glyphs.ManAdult: "man", glyphs.WomanAdult: "woman", glyphs.PersonAdult: "person",
	glyphs.ManSenior: "old man", glyphs.WomanSenior: "old woman", glyphs.PersonSenior: "older person",
}

func helloDolls(sprites int) HelloDolls {
	d := HelloDolls{
		Base:      dollBase(sprites),
		Count:     glyphs.NumDolls,
		Tones:     glyphs.DollTones,
		Fallback:  int(glyphIndex[glyphs.Mutant]),
		Fallbacks: glyphs.DollFallbacks(),
	}
	for _, f := range glyphs.DollFigures {
		d.Figures = append(d.Figures, dollFigureNames[f])
	}
	for h := range glyphs.DollHairs {
		d.Hairs = append(d.Hairs, sim.HairColor(h).String())
	}
	for _, p := range glyphs.DollParts {
		d.Parts = append(d.Parts, p.String())
	}
	return d
}

// dollBase is the glyph index of doll 0: past the symbols, the looks and a
// game's sprites.
func dollBase(sprites int) int { return len(glyphs.All) + len(glyphs.Looks) + sprites }

// glyphIndex is each glyph's position in glyphs.All, the index frames carry
// (beside looks: see entityGlyph).
var glyphIndex = func() map[string]uint16 {
	m := make(map[string]uint16, len(glyphs.All))
	for i, s := range glyphs.All {
		m[s] = uint16(i)
	}
	return m
}()

// entityHeight is the height a frame carries for e, in centimetres, clamped
// to a uint16; 0 means the kind has none. See docs/body-sprites.md.
func entityHeight(e sim.EntityView) uint16 {
	return uint16(min(max(e.HeightCM, 0), 0xFFFF))
}

// entityGlyph is the glyph index a frame carries for e in a game with that many
// species-pack sprites: its sprite past the looks when it has one, its doll past the
// sprites when it has one, its look past the end of glyphs.All when it has
// one, else its glyph in it.
func entityGlyph(e sim.EntityView, sprites int) uint16 {
	if e.Sprite > 0 {
		return uint16(len(glyphs.All) + len(glyphs.Looks) + e.Sprite - 1)
	}
	if k, ok := glyphs.DollKey(e); ok {
		return uint16(dollBase(sprites) + k)
	}
	if i, ok := glyphs.LookIndex(e); ok {
		return uint16(i)
	}
	return glyphIndex[glyphs.ForEntity(e)]
}

func helloGlyphs(sprites []string) HelloGlyphs {
	enums := sim.EnumNames()
	terrains := enums.Terrains
	h := HelloGlyphs{
		Symbols: glyphs.All,
		Terrain: make([]int, len(terrains)),
		Kinds:   make([]int, len(enums.Kinds)),
		Gore:    int(glyphIndex[glyphs.Gore]),
		Corpse:  int(glyphIndex[glyphs.Corpse]),
		Looks:   glyphs.Looks,
		Sprites: append([]string{}, sprites...),
		// The plain alien: what a hover line says until the image is drawn.
		SpriteFallback: int(glyphIndex[glyphs.ForKind(sim.Alien)]),
		Dolls:          helloDolls(len(sprites)),
	}
	for k := range enums.Kinds {
		h.Kinds[k] = int(glyphIndex[glyphs.ForKind(sim.Kind(k))])
	}
	for t := range terrains {
		g := glyphs.ForTerrain(sim.Terrain(t))
		if glyphs.Swatch(g) {
			h.Terrain[t] = -1
		} else {
			h.Terrain[t] = int(glyphIndex[g])
		}
	}
	return h
}

// NewHello builds the Hello for the game snap belongs to.
func NewHello(snap *sim.Snapshot) Hello {
	return Hello{
		Version:  Version,
		Width:    snap.Width,
		Height:   snap.Height,
		Seed:     snap.Seed,
		PageSide: sim.TilePageSide,
		FogOfWar: snap.FogOfWar,
		Enums:    sim.EnumNames(),
		Stats:    statNames,
		Glyphs:   helloGlyphs(snap.Sprites),
		GoreMax:  sim.MaxGore,
		ScumMax:  snap.ScumMax,

		FlowFields: flowFieldNames(snap.FlowFields),

		LandingLevel: int(sim.LandingLevel),
		DeepestLevel: deepestLevel(snap),
		SitingAuto:   snap.Config != nil && snap.Config.SitingAuto,
		Sandbox:      snap.Config != nil && snap.Config.Sandbox,
	}
}

// deepestLevel is Config.DeepestLevel, or the landing level for a hand-built
// Snapshot with no Config.
func deepestLevel(snap *sim.Snapshot) int {
	if snap.Config == nil {
		return int(sim.LandingLevel)
	}
	return max(snap.Config.DeepestLevel, int(sim.LandingLevel))
}

func flowFieldNames(refs []sim.FlowFieldRef) []string {
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name()
	}
	return names
}

// statFields are the int fields of sim.Stats, in declaration order: a new
// field reaches the page without a change here, named in Hello.Stats.
var statFields, statNames = func() ([]int, []string) {
	t := reflect.TypeOf(sim.Stats{})
	var idx []int
	var names []string
	for i := 0; i < t.NumField(); i++ {
		if f := t.Field(i); f.IsExported() && f.Type.Kind() == reflect.Int {
			idx = append(idx, i)
			names = append(names, f.Name)
		}
	}
	return idx, names
}()

// statValues reads the fields statFields names.
func statValues(s sim.Stats) []int {
	v := reflect.ValueOf(s)
	out := make([]int, len(statFields))
	for i, f := range statFields {
		out[i] = int(v.Field(f).Int())
	}
	return out
}
