package wire

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/glyphs"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// decoded is a frame read back by decode, the Go twin of web/wire/decode.js.
// The tests check the encoder through it, and the golden files record it as
// JSON so the JS decoder is checked against the same answers.
type decoded struct {
	Version   int            `json:"version"`
	Paused    bool           `json:"paused"`
	FogOfWar  bool           `json:"fogOfWar"`
	Reset     bool           `json:"tilesReset"`
	HasRefuse bool           `json:"refuseFrame"`
	Tick      uint64         `json:"tick"`
	TileFrame uint64         `json:"tileFrame"`
	TPS       uint32         `json:"tps"`
	Owed      uint32         `json:"pagesOwed"`
	Stats     []int32        `json:"stats"`
	Entities  []decodedEnt   `json:"entities"`
	Pages     []decodedPage  `json:"pages"`
	Refuse    []decodedWaste `json:"refuse"`
	HasScum   bool           `json:"scumFrame"`
	Scum      []decodedScum  `json:"scum"`
	HasSalt   bool           `json:"saltFrame"`
	Salt      []decodedSalt  `json:"salt"`
}

type decodedSalt struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

type decodedScum struct {
	X      int32 `json:"x"`
	Y      int32 `json:"y"`
	Amount uint8 `json:"amount"`
}

type decodedEnt struct {
	ID    uint32 `json:"id"`
	X     int32  `json:"x"`
	Y     int32  `json:"y"`
	Glyph uint16 `json:"glyph"`
	Kind  uint8  `json:"kind"`
	State uint8  `json:"state"`
	Focus uint8  `json:"focus"`
}

type decodedPage struct {
	PX int32 `json:"px"`
	PY int32 `json:"py"`
	// Tiles is the page's 2-byte tiles; the golden JSON keeps a digest of
	// them rather than 8 KB of numbers (see tileSum).
	Tiles []byte `json:"-"`
	Sum   uint32 `json:"tileSum"`
}

type decodedWaste struct {
	X       int32  `json:"x"`
	Y       int32  `json:"y"`
	Corpses uint16 `json:"corpses"`
	Gore    uint8  `json:"gore"`
}

func decode(t *testing.T, b []byte) decoded {
	t.Helper()
	le := binary.LittleEndian
	if string(b[:4]) != "MSFR" {
		t.Fatalf("bad magic %q", b[:4])
	}
	flags := le.Uint16(b[6:])
	d := decoded{
		Version:   int(le.Uint16(b[4:])),
		Paused:    flags&flagPaused != 0,
		FogOfWar:  flags&flagFogOfWar != 0,
		Reset:     flags&flagTilesReset != 0,
		HasRefuse: flags&flagRefuseFrame != 0,
		HasScum:   flags&flagScumFrame != 0,
		HasSalt:   flags&flagSaltFrame != 0,
		Tick:      le.Uint64(b[8:]),
		TileFrame: le.Uint64(b[16:]),
		TPS:       le.Uint32(b[24:]),
		Owed:      le.Uint32(b[44:]),
	}
	nStats, n := int(le.Uint32(b[28:])), int(le.Uint32(b[32:]))
	nPages, nRefuse := int(le.Uint32(b[36:])), int(le.Uint32(b[40:]))
	nScum, nSalt := int(le.Uint32(b[48:])), int(le.Uint32(b[52:]))
	at := headerLen
	for i := 0; i < nStats; i++ {
		d.Stats = append(d.Stats, int32(le.Uint32(b[at:])))
		at += 4
	}
	d.Entities = make([]decodedEnt, n)
	for i := range d.Entities {
		d.Entities[i] = decodedEnt{
			ID:    le.Uint32(b[at+4*i:]),
			X:     int32(le.Uint32(b[at+4*(n+i):])),
			Y:     int32(le.Uint32(b[at+4*(2*n+i):])),
			Glyph: le.Uint16(b[at+12*n+2*i:]),
			Kind:  b[at+12*n+align4(2*n)+i],
			State: b[at+12*n+align4(2*n)+n+i],
			Focus: b[at+12*n+align4(2*n)+2*n+i],
		}
	}
	at += 12*n + align4(2*n) + align4(3*n)
	d.Pages = make([]decodedPage, nPages)
	for i := range d.Pages {
		d.Pages[i].PX = int32(le.Uint32(b[at+4*i:]))
		d.Pages[i].PY = int32(le.Uint32(b[at+4*(nPages+i):]))
	}
	at += 8 * nPages
	for i := range d.Pages {
		d.Pages[i].Tiles = b[at : at+pageTiles*tileBytes]
		d.Pages[i].Sum = tileSum(d.Pages[i].Tiles)
		at += pageTiles * tileBytes
	}
	d.Refuse = make([]decodedWaste, nRefuse)
	for i := range d.Refuse {
		d.Refuse[i].X = int32(le.Uint32(b[at+4*i:]))
		d.Refuse[i].Y = int32(le.Uint32(b[at+4*(nRefuse+i):]))
		d.Refuse[i].Corpses = le.Uint16(b[at+8*nRefuse+2*i:])
		d.Refuse[i].Gore = b[at+8*nRefuse+align4(2*nRefuse)+i]
	}
	at += 8*nRefuse + align4(2*nRefuse) + align4(nRefuse)
	d.Scum = make([]decodedScum, nScum)
	for i := range d.Scum {
		d.Scum[i].X = int32(le.Uint32(b[at+4*i:]))
		d.Scum[i].Y = int32(le.Uint32(b[at+4*(nScum+i):]))
		d.Scum[i].Amount = b[at+8*nScum+i]
	}
	at += 8*nScum + align4(nScum)
	d.Salt = make([]decodedSalt, nSalt)
	for i := range d.Salt {
		d.Salt[i].X = int32(le.Uint32(b[at+4*i:]))
		d.Salt[i].Y = int32(le.Uint32(b[at+4*(nSalt+i):]))
	}
	at += 8 * nSalt
	if at != len(b) {
		t.Fatalf("frame is %d bytes, sections add up to %d", len(b), at)
	}
	return d
}

// tileSum is a position-weighted digest of a page's tile bytes (FNV-1a), so
// the golden JSON can pin a page's contents in one number.
func tileSum(tiles []byte) uint32 {
	h := uint32(2166136261)
	for _, c := range tiles {
		h = (h ^ uint32(c)) * 16777619
	}
	return h
}

// fixture is a small hand-built world: 150x70 tiles, so 3x2 pages with the
// right column and bottom row clipped, and one page (1,1) never written, which
// with fog on means not generated.
func fixture(fog bool) *sim.Snapshot {
	const w, h = 150, 70
	tiles := make([]sim.Tile, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := &tiles[y*w+x]
			switch {
			case x >= 64 && x < 128 && y >= 64:
				// page (1,1): left as zero, i.e. never written
			case (x+y)%7 == 0:
				t.Terrain = sim.Floor
				t.Explored = true
			case x%5 == 0:
				t.Composition = sim.IronBearingRock
			case y%11 == 0:
				t.Terrain = sim.Wall
				t.Explored = x < 40
			}
		}
	}
	tiles[3*w+10].Gore = 2
	tiles[3*w+10].Terrain = sim.Floor
	tiles[40*w+130].Corpses = 3
	return &sim.Snapshot{
		Tick: 1234, Width: w, Height: h, Seed: 7,
		Tiles:       sim.NewTileGrid(w, h, tiles),
		TileChanges: sim.TileChanges{Frame: 1, All: true, Refuse: true},
		Entities: []sim.EntityView{
			{ID: 1, Kind: sim.Colonist, Pos: sim.Point{X: 10, Y: 3}, State: sim.Mining, Focus: sim.FocusWork},
			{ID: 2, Kind: sim.Cat, Pos: sim.Point{X: 70, Y: 20}, State: sim.Hunting},
			{ID: 9, Kind: sim.Alien, Pos: sim.Point{X: 149, Y: 69}, State: sim.Sleeping,
				AlienSpecies: sim.AlienSpecies{Emoji: glyphs.Beetle}},
		},
		Stats:          sim.Stats{Colonists: 1, Cats: 1, Aliens: 1, FloorDug: 1500},
		Scum:           map[sim.Point]uint8{{X: 12, Y: 4}: 3, {X: 2, Y: 4}: 1, {X: 130, Y: 1}: 2},
		ScumMax:        3,
		Salt:           map[sim.Point]struct{}{{X: 7, Y: 9}: {}, {X: 1, Y: 9}: {}, {X: 129, Y: 0}: {}},
		TicksPerSecond: 8,
		FogOfWar:       fog,
	}
}

func everything() Rect { return Rect{0, 0, 1 << 20, 1 << 20} }

func TestEncodeFirstFrame(t *testing.T) {
	snap := fixture(true)
	e := NewEncoder()
	e.SetInterest(everything())
	d := decode(t, e.Encode(snap))

	if d.Version != Version || d.Tick != 1234 || d.TileFrame != 1 || d.TPS != 8 || !d.FogOfWar || d.Paused {
		t.Errorf("header = %+v", d)
	}
	if !d.Reset || !d.HasRefuse {
		t.Errorf("first frame should reset tiles and carry refuse: %+v", d)
	}
	if got := d.Stats[slices.Index(statNames, "FloorDug")]; got != 1500 {
		t.Errorf("FloorDug stat = %d", got)
	}
	wantEnts := []decodedEnt{
		{ID: 1, X: 10, Y: 3, Glyph: glyphIndex[glyphs.Colonist], Kind: uint8(sim.Colonist), State: uint8(sim.Mining), Focus: uint8(sim.FocusWork)},
		{ID: 2, X: 70, Y: 20, Glyph: glyphIndex[glyphs.Cat], Kind: uint8(sim.Cat), State: uint8(sim.Hunting)},
		{ID: 9, X: 149, Y: 69, Glyph: glyphIndex[glyphs.Beetle], Kind: uint8(sim.Alien), State: uint8(sim.Sleeping)},
	}
	if !slices.Equal(d.Entities, wantEnts) {
		t.Errorf("entities = %+v", d.Entities)
	}
	// Five pages: (1,1) was never written, and with fog on it has nothing to
	// show, so it is skipped. Nearest the middle of the view first.
	var got [][2]int32
	for _, p := range d.Pages {
		got = append(got, [2]int32{p.PX, p.PY})
	}
	if len(got) != 5 || slices.Contains(got, [2]int32{1, 1}) {
		t.Errorf("pages = %v, want the five written ones", got)
	}
	wantRefuse := []decodedWaste{{X: 10, Y: 3, Gore: 2}, {X: 130, Y: 40, Corpses: 3}}
	if !slices.Equal(d.Refuse, wantRefuse) {
		t.Errorf("refuse = %+v", d.Refuse)
	}
	if d.Owed != 0 {
		t.Errorf("owed = %d", d.Owed)
	}
}

// Every tile of every page decodes to what TileAt reads: terrain, composition,
// and visible = explored (fog on) or always (fog off). Past the map edge a
// page reads as rock.
func TestEncodedTilesMatchTileAt(t *testing.T) {
	for _, fog := range []bool{true, false} {
		snap := fixture(fog)
		e := NewEncoder()
		e.SetInterest(everything())
		d := decode(t, e.Encode(snap))
		wantPages := 5
		if !fog {
			wantPages = 6 // fog off: the unwritten page is shown (as rock)
		}
		if len(d.Pages) != wantPages {
			t.Fatalf("fog %v: %d pages, want %d", fog, len(d.Pages), wantPages)
		}
		for _, p := range d.Pages {
			for off := 0; off < pageTiles; off++ {
				pos := sim.Point{X: int(p.PX)*sim.TilePageSide + off%sim.TilePageSide, Y: int(p.PY)*sim.TilePageSide + off/sim.TilePageSide}
				want := sim.Tile{Terrain: sim.Rock}
				visible := !fog
				if pos.X < snap.Width && pos.Y < snap.Height {
					want = snap.TileAt(pos)
					visible = snap.ExploredAt(pos)
				}
				terrain, flags := p.Tiles[2*off], p.Tiles[2*off+1]
				if sim.Terrain(terrain) != want.Terrain ||
					sim.RockComposition(flags&tileCompositionMask) != want.Composition ||
					(flags&tileVisible != 0) != visible {
					t.Fatalf("fog %v: tile %v = terrain %d flags %#x, want %+v visible %v", fog, pos, terrain, flags, want, visible)
				}
			}
		}
	}
}

// After the first frame, only pages the page lacks are sent: a quiet frame
// sends none, a changed page is sent again, and refuse only when it changed.
func TestEncodeSendsOnlyChangedPages(t *testing.T) {
	snap := fixture(true)
	e := NewEncoder()
	e.SetInterest(everything())
	e.Encode(snap)

	quiet := *snap
	quiet.TileChanges = sim.TileChanges{Frame: 2}
	if d := decode(t, e.Encode(&quiet)); len(d.Pages) != 0 || d.Reset || d.HasRefuse {
		t.Errorf("quiet frame: %d pages, reset %v, refuse %v", len(d.Pages), d.Reset, d.HasRefuse)
	}

	changed := *snap
	pi := snap.Tiles.PageIndex(sim.Point{X: 70, Y: 10}) // page (1,0)
	changed.TileChanges = sim.TileChanges{Frame: 3, Pages: []int{pi}, Refuse: true}
	d := decode(t, e.Encode(&changed))
	if len(d.Pages) != 1 || d.Pages[0].PX != 1 || d.Pages[0].PY != 0 || !d.HasRefuse || d.Reset {
		t.Errorf("changed frame: pages %+v, refuse %v, reset %v", d.Pages, d.HasRefuse, d.Reset)
	}

	// Encoding the same snapshot again applies nothing twice.
	if d := decode(t, e.Encode(&changed)); len(d.Pages) != 0 || d.HasRefuse {
		t.Errorf("re-encode: %d pages, refuse %v", len(d.Pages), d.HasRefuse)
	}
}

// A skipped frame number means deltas were lost: start the page's tiles over.
func TestEncodeResetsAfterAGap(t *testing.T) {
	snap := fixture(true)
	e := NewEncoder()
	e.SetInterest(everything())
	e.Encode(snap)
	later := *snap
	later.TileChanges = sim.TileChanges{Frame: 5}
	d := decode(t, e.Encode(&later))
	if !d.Reset || !d.HasRefuse || len(d.Pages) != 5 {
		t.Errorf("after a gap: reset %v, refuse %v, %d pages; want a full resend", d.Reset, d.HasRefuse, len(d.Pages))
	}
}

// Pages outside the view are not sent. One that changes while out of view is
// sent again when the view comes back to it, and not before.
func TestEncodeFollowsTheView(t *testing.T) {
	snap := fixture(true)
	e := NewEncoder()
	e.SetInterest(Rect{0, 0, 64, 64}) // page (0,0) only
	d := decode(t, e.Encode(snap))
	if len(d.Pages) != 1 || d.Pages[0].PX != 0 || d.Pages[0].PY != 0 {
		t.Fatalf("pages = %+v, want (0,0) only", d.Pages)
	}

	changed := *snap
	changed.TileChanges = sim.TileChanges{Frame: 2, Pages: []int{snap.Tiles.PageIndex(sim.Point{})}}
	e.SetInterest(Rect{64, 0, 128, 64}) // move to page (1,0)
	d = decode(t, e.Encode(&changed))
	if len(d.Pages) != 1 || d.Pages[0].PX != 1 {
		t.Fatalf("after the pan: pages %+v, want (1,0)", d.Pages)
	}

	e.SetInterest(Rect{0, 0, 64, 64}) // back: (0,0) changed while away
	d = decode(t, e.Encode(&changed))
	if len(d.Pages) != 1 || d.Pages[0].PX != 0 {
		t.Errorf("back again: pages %+v, want (0,0) resent", d.Pages)
	}
}

// A view bigger than MaxPages fills over several frames, nearest the middle
// first, with Owed counting down what is left.
func TestEncodeCapsPagesPerFrame(t *testing.T) {
	snap := fixture(false) // six showable pages
	e := NewEncoder()
	e.MaxPages = 4
	e.SetInterest(everything())
	d := decode(t, e.Encode(snap))
	if len(d.Pages) != 4 || d.Owed != 2 || e.Owed() != 2 {
		t.Fatalf("first: %d pages, owed %d", len(d.Pages), d.Owed)
	}
	// The view's middle is page (1,0) or (1,1); the far corners come last.
	if p := d.Pages[0]; p.PX != 1 {
		t.Errorf("first page sent is (%d,%d), want a middle column", p.PX, p.PY)
	}
	d = decode(t, e.Encode(snap))
	if len(d.Pages) != 2 || d.Owed != 0 {
		t.Errorf("second: %d pages, owed %d", len(d.Pages), d.Owed)
	}
}

// With a real engine and fog on, only generated chunks are sent: the rest of
// a 2000x2000 map has nothing to show yet.
func TestEncodeRealWorldSendsGeneratedChunks(t *testing.T) {
	cfg := sim.DefaultConfig()
	cfg.Seed = 7
	cfg.Width, cfg.Height = 2000, 2000
	eng := sim.NewEngine(cfg)
	eng.ShareLiveTiles()
	snap, _ := eng.Advance(time.Millisecond)
	if snap == nil {
		t.Fatal("no initial frame")
	}
	e := NewEncoder()
	e.MaxPages = 1 << 20
	e.SetInterest(everything())
	d := decode(t, e.Encode(snap))
	if want := snap.Stats.ChunksGenerated; len(d.Pages) != want || want == 0 {
		t.Errorf("%d pages sent, %d chunks generated", len(d.Pages), want)
	}
	if len(d.Entities) != len(snap.Entities) || d.Stats[slices.Index(statNames, "Colonists")] != int32(snap.Stats.Colonists) {
		t.Errorf("entities %d / %d", len(d.Entities), len(snap.Entities))
	}
}

func TestHelloNamesEverythingAFrameIndexes(t *testing.T) {
	h := NewHello(fixture(true))
	if h.Version != Version || h.PageSide != sim.TilePageSide || h.Width != 150 || !h.FogOfWar {
		t.Errorf("hello = %+v", h)
	}
	if len(h.Stats) != len(statFields) || h.Stats[0] != "Colonists" {
		t.Errorf("stats = %v", h.Stats)
	}
	g := h.Glyphs
	if len(g.Symbols) != len(glyphs.All) || len(g.Terrain) != len(h.Enums.Terrains) {
		t.Fatalf("glyphs: %d symbols, %d terrains", len(g.Symbols), len(g.Terrain))
	}
	if g.Terrain[sim.Floor] != -1 || g.Terrain[sim.Rock] != -1 || g.Terrain[sim.Hull] != -1 {
		t.Errorf("swatch terrains should have no glyph: %v", g.Terrain)
	}
	if i := g.Terrain[sim.Bed]; i < 0 || g.Symbols[i] != glyphs.Bed {
		t.Errorf("bed draws as glyph %d", i)
	}
	if len(g.Kinds) != len(h.Enums.Kinds) || g.Symbols[g.Kinds[sim.Colonist]] != glyphs.Colonist || g.Symbols[g.Kinds[sim.Alien]] != glyphs.Alien {
		t.Errorf("kind glyphs: %v", g.Kinds)
	}
	if g.Symbols[g.Gore] != glyphs.Gore || g.Symbols[g.Corpse] != glyphs.Corpse {
		t.Error("refuse glyphs point at the wrong symbols")
	}
	if h.GoreMax != sim.MaxGore || h.ScumMax != 3 {
		t.Errorf("goreMax %d scumMax %d", h.GoreMax, h.ScumMax)
	}
	if len(glyphs.All)+len(g.Looks) > 0xFFFF || len(g.Looks) != len(glyphs.Looks) {
		t.Errorf("%d symbols and %d looks: glyph indexes no longer fit a frame's uint16", len(glyphs.All), len(g.Looks))
	}
	// A colonist at rest indexes past the symbols into its look.
	p := &sim.Profile{Gender: sim.GenderWoman, Age: 40, SkinTone: sim.SkinDark, HairColor: sim.HairRed}
	i := int(entityGlyph(sim.EntityView{Kind: sim.Colonist, Profile: p})) - len(g.Symbols)
	if i < 0 || i >= len(g.Looks) || g.Looks[i][0] != "\U0001F469\U0001F3FF\u200D\U0001F9B0" {
		t.Errorf("a dark-skinned red-haired woman draws as look %d", i)
	}
	if n := len(h.Enums.Compositions); n > tileCompositionMask+1 {
		t.Errorf("%d rock compositions do not fit the tile flags' %d bits", n, 4)
	}
	if len(h.Enums.Terrains) > 256 || len(h.Enums.States) > 256 || len(h.Enums.Kinds) > 256 || len(h.Enums.Focuses) > 256 {
		t.Error("an enum no longer fits its byte on the wire")
	}
}

// Scum goes whole, in row order, on the first frame, whenever the engine
// publishes a different scum map, and on a tiles reset; a frame that shares the
// last map sends none.
func TestEncodeSendsScumWhenItChanges(t *testing.T) {
	snap := fixture(true)
	e := NewEncoder()
	d := decode(t, e.Encode(snap))
	want := []decodedScum{{X: 130, Y: 1, Amount: 2}, {X: 2, Y: 4, Amount: 1}, {X: 12, Y: 4, Amount: 3}}
	if !d.HasScum || !slices.Equal(d.Scum, want) {
		t.Fatalf("first frame scum %v %+v, want %+v", d.HasScum, d.Scum, want)
	}

	same := *snap
	same.TileChanges = sim.TileChanges{Frame: 2}
	if d := decode(t, e.Encode(&same)); d.HasScum || len(d.Scum) != 0 {
		t.Errorf("unchanged scum was sent again: %+v", d.Scum)
	}

	// A tiles reset drops everything the page holds, so the same map goes
	// again.
	reset := same
	reset.TileChanges = sim.TileChanges{Frame: 3, All: true}
	if d := decode(t, e.Encode(&reset)); !d.HasScum || !slices.Equal(d.Scum, want) {
		t.Errorf("scum after a tiles reset: %v %+v, want %+v", d.HasScum, d.Scum, want)
	}

	scraped := same
	scraped.TileChanges = sim.TileChanges{Frame: 4}
	scraped.Scum = map[sim.Point]uint8{{X: 12, Y: 4}: 2}
	if d := decode(t, e.Encode(&scraped)); !d.HasScum || !slices.Equal(d.Scum, []decodedScum{{X: 12, Y: 4, Amount: 2}}) {
		t.Errorf("changed scum: %v %+v", d.HasScum, d.Scum)
	}

	gone := scraped
	gone.TileChanges = sim.TileChanges{Frame: 5}
	gone.Scum = map[sim.Point]uint8{}
	if d := decode(t, e.Encode(&gone)); !d.HasScum || len(d.Scum) != 0 {
		t.Errorf("all scum scraped: want an empty scum frame, got %v %+v", d.HasScum, d.Scum)
	}
}

// Salt goes whole, in row order, on the first frame and whenever the engine
// publishes a different salt map or the tiles start over, exactly as scum does.
func TestEncodeSendsSaltWhenItChanges(t *testing.T) {
	snap := fixture(true)
	e := NewEncoder()
	d := decode(t, e.Encode(snap))
	want := []decodedSalt{{X: 129, Y: 0}, {X: 1, Y: 9}, {X: 7, Y: 9}}
	if !d.HasSalt || !slices.Equal(d.Salt, want) {
		t.Fatalf("first frame salt %v %+v, want %+v", d.HasSalt, d.Salt, want)
	}

	same := *snap
	same.TileChanges = sim.TileChanges{Frame: 2}
	if d := decode(t, e.Encode(&same)); d.HasSalt || len(d.Salt) != 0 {
		t.Errorf("unchanged salt was sent again: %+v", d.Salt)
	}

	reset := same
	reset.TileChanges = sim.TileChanges{Frame: 3, All: true}
	if d := decode(t, e.Encode(&reset)); !d.HasSalt || !slices.Equal(d.Salt, want) {
		t.Errorf("salt after a tiles reset: %v %+v, want %+v", d.HasSalt, d.Salt, want)
	}

	built := same
	built.TileChanges = sim.TileChanges{Frame: 4}
	built.Salt = map[sim.Point]struct{}{{X: 1, Y: 9}: {}}
	if d := decode(t, e.Encode(&built)); !d.HasSalt || !slices.Equal(d.Salt, []decodedSalt{{X: 1, Y: 9}}) {
		t.Errorf("changed salt: %v %+v", d.HasSalt, d.Salt)
	}

	gone := built
	gone.TileChanges = sim.TileChanges{Frame: 5}
	gone.Salt = map[sim.Point]struct{}{}
	if d := decode(t, e.Encode(&gone)); !d.HasSalt || len(d.Salt) != 0 {
		t.Errorf("all salt built over: want an empty salt frame, got %v %+v", d.HasSalt, d.Salt)
	}
}
