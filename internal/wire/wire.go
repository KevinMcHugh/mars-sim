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
const Version = 3

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
}

// glyphIndex is each glyph's position in glyphs.All, the index frames carry.
var glyphIndex = func() map[string]uint16 {
	m := make(map[string]uint16, len(glyphs.All))
	for i, s := range glyphs.All {
		m[s] = uint16(i)
	}
	return m
}()

func helloGlyphs() HelloGlyphs {
	enums := sim.EnumNames()
	terrains := enums.Terrains
	h := HelloGlyphs{
		Symbols: glyphs.All,
		Terrain: make([]int, len(terrains)),
		Kinds:   make([]int, len(enums.Kinds)),
		Gore:    int(glyphIndex[glyphs.Gore]),
		Corpse:  int(glyphIndex[glyphs.Corpse]),
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
		Glyphs:   helloGlyphs(),
		GoreMax:  sim.MaxGore,
		ScumMax:  snap.ScumMax,
	}
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
