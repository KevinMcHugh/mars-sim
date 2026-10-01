package wire

import (
	"encoding/binary"
	"math"
	"reflect"
	"slices"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The frame layout. Every section starts on a 4-byte boundary, so the page
// can view it as an Int32Array or Uint8Array in place. docs/wire-format.md is
// the reference; this is the code.
const (
	headerLen = 72
	pageTiles = sim.TilePageSide * sim.TilePageSide
	// tileBytes is one tile on the wire: terrain, then flags.
	tileBytes = 2

	flagPaused      = 1 << 0
	flagFogOfWar    = 1 << 1
	flagTilesReset  = 1 << 2 // drop every page held; the ones in this frame start over
	flagRefuseFrame = 1 << 3 // the refuse section is the whole list; without it, keep the last one
	flagScumFrame   = 1 << 4 // the scum section is the whole list; without it, keep the last one
	flagSaltFrame   = 1 << 5 // the salt section is the whole list; without it, keep the last one
	flagFlowFrame   = 1 << 6 // the flow section is the shown field in view; without it, keep the last one

	// The tile flags byte: the rock composition in the low bits, and whether
	// the tile may be drawn (explored, or the fog is off).
	tileCompositionMask = 0x0F
	tileVisible         = 1 << 4
)

var magic = [4]byte{'M', 'S', 'F', 'R'}

// DefaultMaxPages is how many tile pages one frame carries at most, about
// 512 KB of tiles. A viewport that needs more (a first frame, a zoom out, a
// fast pan) is filled over the next frames, nearest its middle first.
const DefaultMaxPages = 64

// Rect is a region of the map in tiles, half-open: X0 <= x < X1.
type Rect struct{ X0, Y0, X1, Y1 int }

// Encoder turns snapshots into frames for one page. It remembers which tile
// pages the page holds, so each frame carries only the pages that are in view
// and either new to it or changed since it got them.
//
// It relies on seeing every snapshot the engine publishes, because a
// snapshot's TileChanges is a delta against the one before: that is what
// Engine.Advance guarantees. A gap in TileChanges.Frame is handled anyway, by
// starting the page's tiles over (flagTilesReset).
type Encoder struct {
	// MaxPages caps the tile pages in one frame; 0 means DefaultMaxPages.
	MaxPages int

	interest    Rect
	hasInterest bool

	held      map[int]bool // page table indexes whose current tiles the page holds
	lastFrame uint64       // TileChanges.Frame of the last snapshot applied
	// lastScum is the Snapshot.Scum map last sent. The engine hands out the
	// same map until the scum changes (see sim.World.publishedScum), so a
	// different map is the signal to send the list again.
	lastScum uintptr
	lastSalt uintptr // likewise for Snapshot.Salt
	// lastFlow is the Snapshot.FlowField last sent. The engine hands out the
	// same view until the field changes (see sim.Engine.publish), so, like
	// scum, a different pointer is the signal to send it again. flowMoved
	// says the interest changed since then: the section only carries the
	// tiles in view, so a new view needs them again.
	lastFlow  *sim.FlowFieldView
	flowMoved bool
	flow      []flowTile
	owed      int // pages in view still to send after the last frame

	buf   []byte
	tiles []sim.Tile
	order []pageRef
}

// NewEncoder returns an encoder with nothing in view: frames carry no tiles
// until SetInterest.
func NewEncoder() *Encoder {
	return &Encoder{held: map[int]bool{}, tiles: make([]sim.Tile, pageTiles)}
}

// SetInterest sets the map region the page is showing. Pages that intersect
// it are sent as they become known or change; pages outside it are not, and
// any that change while out of view are sent again when they come back.
func (e *Encoder) SetInterest(r Rect) {
	e.interest, e.hasInterest = r, true
	e.flowMoved = true
}

// Owed is how many pages in view the last frame left out for the MaxPages
// cap. While it is above zero the host should encode again (the same
// snapshot will do) rather than wait for the next one; so should it after
// SetInterest.
func (e *Encoder) Owed() int { return e.owed }

type pageRef struct {
	pi     int
	origin sim.Point
	dist   int
}

// Encode returns the frame for snap. The slice is reused by the next call.
//
// Encoding the same snapshot again (it has the same TileChanges.Frame) is how
// the host sends pages owed from the last call, or pages a new interest
// brought into view, while the engine is paused or between publishes. With
// live tiles the snapshot's terrain is the live map, which is only valid until
// the next tick; the host encodes between ticks, on the engine's goroutine.
func (e *Encoder) Encode(snap *sim.Snapshot) []byte {
	var flags uint16
	if snap.Paused {
		flags |= flagPaused
	}
	if snap.FogOfWar {
		flags |= flagFogOfWar
	}
	tc := snap.TileChanges
	if tc.Frame != e.lastFrame {
		if tc.All || tc.Frame != e.lastFrame+1 {
			clear(e.held)
			flags |= flagTilesReset | flagRefuseFrame
		} else {
			for _, pi := range tc.Pages {
				delete(e.held, pi)
			}
			if tc.Refuse {
				flags |= flagRefuseFrame
			}
		}
		e.lastFrame = tc.Frame
	}

	pages := e.pagesToSend(snap)
	var refuse []sim.RefuseTile
	if flags&flagRefuseFrame != 0 {
		refuse = snap.Tiles.RefuseTiles()
	}
	var scum []scumTile
	if id := reflect.ValueOf(snap.Scum).Pointer(); id != e.lastScum || flags&flagTilesReset != 0 {
		flags |= flagScumFrame
		scum = scumTiles(snap.Scum)
		e.lastScum = id
	}
	var salt []sim.Point
	if id := reflect.ValueOf(snap.Salt).Pointer(); id != e.lastSalt || flags&flagTilesReset != 0 {
		flags |= flagSaltFrame
		salt = saltTiles(snap.Salt)
		e.lastSalt = id
	}

	flowField, flowMax, flowGoals := int32(-1), int32(0), int32(0)
	e.flow = e.flow[:0]
	if v := snap.FlowField; v != e.lastFlow || (v != nil && (e.flowMoved || flags&flagTilesReset != 0)) {
		flags |= flagFlowFrame
		e.lastFlow, e.flowMoved = v, false
		if v != nil {
			flowField = int32(slices.Index(snap.FlowFields, v.Field))
			flowMax, flowGoals = v.Max, int32(v.Goals)
			e.flow = e.flowTiles(v)
		}
	}

	n := len(snap.Entities)
	size := headerLen +
		4*len(statFields) +
		n*(4+4+4) + align4(n*2) + align4(n*3) +
		len(pages)*(4+4) + len(pages)*pageTiles*tileBytes +
		len(refuse)*(4+4) + align4(len(refuse)*2) + align4(len(refuse)) +
		len(scum)*(4+4) + align4(len(scum)) +
		len(salt)*(4+4) +
		len(e.flow)*(4+4) + align4(len(e.flow)*2)
	e.buf = slices.Grow(e.buf[:0], size)[:size]
	clear(e.buf)
	b := e.buf
	le := binary.LittleEndian

	copy(b[0:4], magic[:])
	le.PutUint16(b[4:], Version)
	le.PutUint16(b[6:], flags)
	le.PutUint64(b[8:], uint64(snap.Tick))
	le.PutUint64(b[16:], tc.Frame)
	le.PutUint32(b[24:], uint32(snap.TicksPerSecond))
	le.PutUint32(b[28:], uint32(len(statFields)))
	le.PutUint32(b[32:], uint32(n))
	le.PutUint32(b[36:], uint32(len(pages)))
	le.PutUint32(b[40:], uint32(len(refuse)))
	le.PutUint32(b[44:], uint32(e.owed))
	le.PutUint32(b[48:], uint32(len(scum)))
	le.PutUint32(b[52:], uint32(len(salt)))
	le.PutUint32(b[56:], uint32(len(e.flow)))
	le.PutUint32(b[60:], uint32(flowField))
	le.PutUint32(b[64:], uint32(flowMax))
	le.PutUint32(b[68:], uint32(flowGoals))
	at := headerLen

	for _, v := range statValues(snap.Stats) {
		le.PutUint32(b[at:], uint32(clampInt32(v)))
		at += 4
	}

	// Entities, as struct-of-arrays: ids, xs, ys, glyphs (uint16, an index
	// into Hello.Glyphs.Symbols, or past its end into Looks), then a byte each of kind, state and focus.
	for i := range snap.Entities {
		le.PutUint32(b[at+4*i:], uint32(snap.Entities[i].ID))
	}
	at += 4 * n
	for i := range snap.Entities {
		le.PutUint32(b[at+4*i:], uint32(int32(snap.Entities[i].Pos.X)))
	}
	at += 4 * n
	for i := range snap.Entities {
		le.PutUint32(b[at+4*i:], uint32(int32(snap.Entities[i].Pos.Y)))
	}
	at += 4 * n
	for i := range snap.Entities {
		le.PutUint16(b[at+2*i:], entityGlyph(snap.Entities[i]))
	}
	at += align4(2 * n)
	for i := range snap.Entities {
		ev := &snap.Entities[i]
		b[at+i] = byte(ev.Kind)
		b[at+n+i] = byte(ev.State)
		b[at+2*n+i] = byte(ev.Focus)
	}
	at += align4(3 * n)

	// Pages: page x and y (in pages, not tiles), then each page's tiles.
	for i, p := range pages {
		le.PutUint32(b[at+4*i:], uint32(p.origin.X/sim.TilePageSide))
		le.PutUint32(b[at+4*(len(pages)+i):], uint32(p.origin.Y/sim.TilePageSide))
	}
	at += 8 * len(pages)
	visibleAll := !snap.FogOfWar
	for _, p := range pages {
		snap.ReadPage(p.pi, e.tiles) // pagesToSend checked PageKnown
		for _, t := range e.tiles {
			b[at] = byte(t.Terrain)
			f := byte(t.Composition) & tileCompositionMask
			if visibleAll || t.Explored {
				f |= tileVisible
			}
			b[at+1] = f
			at += tileBytes
		}
	}

	// Refuse: xs, ys, corpse counts (uint16), gore (uint8).
	r := len(refuse)
	for i, t := range refuse {
		le.PutUint32(b[at+4*i:], uint32(int32(t.Pos.X)))
		le.PutUint32(b[at+4*(r+i):], uint32(int32(t.Pos.Y)))
	}
	at += 8 * r
	for i, t := range refuse {
		le.PutUint16(b[at+2*i:], t.Corpses)
	}
	at += align4(2 * r)
	for i, t := range refuse {
		b[at+i] = t.Gore
	}
	at += align4(r)

	// Scum: xs, ys, amount (uint8, 1..Hello.scumMax).
	c := len(scum)
	for i, t := range scum {
		le.PutUint32(b[at+4*i:], uint32(int32(t.pos.X)))
		le.PutUint32(b[at+4*(c+i):], uint32(int32(t.pos.Y)))
	}
	at += 8 * c
	for i, t := range scum {
		b[at+i] = t.amount
	}
	at += align4(c)

	// Salt: xs, ys.
	s := len(salt)
	for i, p := range salt {
		le.PutUint32(b[at+4*i:], uint32(int32(p.X)))
		le.PutUint32(b[at+4*(s+i):], uint32(int32(p.Y)))
	}
	at += 8 * s

	// Flow: xs, ys, distances (uint16, saturating).
	fl := len(e.flow)
	for i, t := range e.flow {
		le.PutUint32(b[at+4*i:], uint32(int32(t.pos.X)))
		le.PutUint32(b[at+4*(fl+i):], uint32(int32(t.pos.Y)))
	}
	at += 8 * fl
	for i, t := range e.flow {
		le.PutUint16(b[at+2*i:], t.dist)
	}
	at += align4(2 * fl)

	if at != size {
		panic("wire: frame size miscounted")
	}
	return b
}

// pagesToSend picks the pages in view the page lacks, nearest the middle of
// the view first, up to MaxPages, marks them held, and records how many were
// left over in owed. A page with nothing to show (not generated, fog on) is
// skipped without being held: when it is generated it arrives in
// TileChanges, and is sent then if it is still in view.
func (e *Encoder) pagesToSend(snap *sim.Snapshot) []pageRef {
	e.order = e.order[:0]
	e.owed = 0
	if !e.hasInterest {
		return nil
	}
	side := sim.TilePageSide
	r := e.interest
	x0, y0 := max(r.X0, 0), max(r.Y0, 0)
	x1, y1 := min(r.X1, snap.Width), min(r.Y1, snap.Height)
	if x0 >= x1 || y0 >= y1 {
		return nil
	}
	cx, cy := (x0+x1)/2/side, (y0+y1)/2/side
	for py := y0 / side; py <= (y1-1)/side; py++ {
		for px := x0 / side; px <= (x1-1)/side; px++ {
			origin := sim.Point{X: px * side, Y: py * side}
			pi := snap.Tiles.PageIndex(origin)
			if e.held[pi] {
				continue
			}
			dx, dy := px-cx, py-cy
			e.order = append(e.order, pageRef{pi: pi, origin: origin, dist: dx*dx + dy*dy})
		}
	}
	slices.SortStableFunc(e.order, func(a, b pageRef) int { return a.dist - b.dist })

	limit := e.MaxPages
	if limit <= 0 {
		limit = DefaultMaxPages
	}
	out := e.order[:0]
	for i, p := range e.order {
		if len(out) == limit {
			e.owed = e.countShowable(snap, e.order[i:])
			break
		}
		if !snap.PageKnown(p.pi) {
			continue
		}
		e.held[p.pi] = true
		out = append(out, p)
	}
	return out
}

// countShowable counts the pages that have something to show.
func (e *Encoder) countShowable(snap *sim.Snapshot, pages []pageRef) int {
	n := 0
	for _, p := range pages {
		if snap.PageKnown(p.pi) {
			n++
		}
	}
	return n
}

func align4(n int) int { return (n + 3) &^ 3 }

func clampInt32(v int) int32 {
	return int32(max(min(v, math.MaxInt32), math.MinInt32))
}

type scumTile struct {
	pos    sim.Point
	amount uint8
}

// scumTiles lists the scum map in row order: map order is random, and the
// frame's bytes should not be (the golden tests compare them).
func scumTiles(m map[sim.Point]uint8) []scumTile {
	out := make([]scumTile, 0, len(m))
	for p, n := range m {
		out = append(out, scumTile{p, n})
	}
	slices.SortFunc(out, func(a, b scumTile) int {
		if a.pos.Y != b.pos.Y {
			return a.pos.Y - b.pos.Y
		}
		return a.pos.X - b.pos.X
	})
	return out
}

// saltTiles lists the salt set in row order, for the same reason as scumTiles.
func saltTiles(m map[sim.Point]struct{}) []sim.Point {
	out := make([]sim.Point, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b sim.Point) int {
		if a.Y != b.Y {
			return a.Y - b.Y
		}
		return a.X - b.X
	})
	return out
}

type flowTile struct {
	pos  sim.Point
	dist uint16
}

// flowTiles lists the tiles in view the field reaches, in row order, with
// their distances. Only the view: a field covers the whole colony, and the
// page draws only what is on screen (plus the interest's margin).
func (e *Encoder) flowTiles(v *sim.FlowFieldView) []flowTile {
	out := e.flow[:0]
	if !e.hasInterest {
		return out
	}
	r := e.interest
	v.Range(r.X0, r.Y0, r.X1, r.Y1, func(p sim.Point, d int32) {
		out = append(out, flowTile{p, uint16(min(d, math.MaxUint16))})
	})
	return out
}
