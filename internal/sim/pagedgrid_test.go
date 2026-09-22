package sim

import (
	"testing"
	"unsafe"
)

// TestPagedGridAddressing pins the property everything else rests on: distinct
// in-bounds coordinates address distinct slots, across page boundaries and on
// a map whose width is not a multiple of the page side.
func TestPagedGridAddressing(t *testing.T) {
	const w, h = 150, 70 // deliberately not multiples of gridPageSide
	g := newPagedGrid[int32](w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g.set(x, y, int32(y*w+x+1))
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if got, want := g.at(x, y), int32(y*w+x+1); got != want {
				t.Fatalf("at(%d,%d) = %d, want %d", x, y, got, want)
			}
		}
	}
}

// TestPagedGridUnwrittenReadsZero is the property that lets an untouched page
// stand in for "nothing known" without being allocated.
func TestPagedGridUnwrittenReadsZero(t *testing.T) {
	g := newPagedGrid[flowCell](1000, 1000)
	if g.pagesAllocated() != 0 {
		t.Fatalf("fresh grid allocated %d pages, want 0", g.pagesAllocated())
	}
	if got := g.at(500, 500); got != (flowCell{}) {
		t.Fatalf("unwritten cell = %+v, want zero", got)
	}
	if page := g.pageAt(500, 500); page != nil {
		t.Fatal("pageAt allocated a page; it must not")
	}
	if g.pagesAllocated() != 0 {
		t.Fatalf("reads allocated %d pages, want 0", g.pagesAllocated())
	}
	g.set(500, 500, flowCell{gen: 7, dist: 3})
	if g.pagesAllocated() != 1 {
		t.Fatalf("one write allocated %d pages, want 1", g.pagesAllocated())
	}
}

// TestPagedGridInteriorPage checks the 8-connected fast path: a node away from
// a page edge reaches all its neighbours through the page it returns, and a
// node on an edge is refused rather than handed a page its neighbours are not in.
func TestPagedGridInteriorPage(t *testing.T) {
	g := newPagedGrid[int32](256, 256)
	for _, p := range []Point{{0, 10}, {gridPageMask, 10}, {10, 0}, {10, gridPageMask}, {gridPageSide, 10}} {
		g.set(p.X, p.Y, 1)
		if page := g.interiorPage(p.X, p.Y); page != nil && (p.X%gridPageSide == 0 || p.X%gridPageSide == gridPageMask || p.Y%gridPageSide == 0 || p.Y%gridPageSide == gridPageMask) {
			t.Fatalf("interiorPage(%v) returned a page for an edge cell", p)
		}
	}
	// An interior cell's whole neighbourhood must live in the page it returns.
	cx, cy := 100, 100
	g.set(cx, cy, 1)
	page := g.interiorPage(cx, cy)
	if page == nil {
		t.Fatal("interiorPage returned nil for an interior cell")
	}
	for _, d := range neighbors8 {
		nx, ny := cx+d.X, cy+d.Y
		page[offset(nx, ny)] = int32(nx*1000 + ny)
		if got, want := g.at(nx, ny), int32(nx*1000+ny); got != want {
			t.Fatalf("writing (%d,%d) through the interior page landed elsewhere: at = %d, want %d", nx, ny, got, want)
		}
	}
}

// TestTileRecordStaysNarrow guards the width of the one structure that is
// genuinely dense. Every byte here is 95 MB on a 10000x10000 map, and twice
// that once the published grid mirrors it — a word-sized field slipped into
// tileCell is how this cost 16 GB the first time. See docs/sparse-grids.md.
func TestTileRecordStaysNarrow(t *testing.T) {
	if got := unsafe.Sizeof(tileCell{}); got > 4 {
		t.Errorf("tileCell is %d bytes; it holds terrain, composition and a flag, so it should be 3", got)
	}
}

// TestPagedGridsStaySparse is the regression guard for the whole point of
// pagedgrid.go: on a huge map with a small colony, every paged grid must hold
// pages for the colony, not for the world. A change that walks or stamps one of
// these grids across the full map trips this rather than quietly costing
// gigabytes on a map nobody benchmarks.
func TestPagedGridsStaySparse(t *testing.T) {
	w := benchWorldSmallColony(4096, 60, 20)
	for i := 0; i < 200; i++ {
		w.step()
		w.snapshot(false, 8)
	}
	// The colony is a 60x60 chamber plus whatever it has dug, well inside a
	// 4x4 block of 64x64 pages. Aliens roam through rock and drag the
	// occupancy index wider than the rest, so it gets more room.
	full := len(w.regionOf.pages)
	for _, c := range []struct {
		name  string
		pages int
		max   int
	}{
		{"regionOf", w.regionOf.pagesAllocated(), 16},
		{"occ", w.occ.pagesAllocated(), 128},
		{"facilityCells", w.facilityCells.pagesAllocated(), 16},
		{"transitSeen", w.transitSeen.pagesAllocated(), 16},
		{"pathfinder cells", w.pf.cells.pagesAllocated(), 16},
		{"corridorSeen", w.pf.corridorSeen.pagesAllocated(), 16},
		{"frontier field", w.frontier.cells.pagesAllocated(), 16},
	} {
		if c.pages > c.max {
			t.Errorf("%s allocated %d of %d pages, want <= %d: it is tracking the map, not the colony",
				c.name, c.pages, full, c.max)
		}
	}
	for tr := Terrain(0); tr < numTerrains; tr++ {
		if f := w.fields[tr]; f != nil {
			if n := f.cells.pagesAllocated(); n > 16 {
				t.Errorf("%s flow field allocated %d of %d pages, want <= 16", tr, n, full)
			}
		}
	}
}
