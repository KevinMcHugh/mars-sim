package sim

// A pagedGrid is a sparse, map-sized 2D array. Storage is allocated one page at
// a time, on first write; a page nothing has written reads as the zero value of
// T and costs one nil slice header.
//
// It exists because the navigation structures — flow fields, A* scratch, the
// facility BFS, the region labels, the occupancy index — are dense arrays over
// the whole map but only ever hold a meaningful value on a walkable tile. A
// colony that has excavated 174k tiles of a 100M-tile map was paying for 100M
// entries in each of them: at 124 bytes per tile across all of them together,
// ~11.8 GiB of a ~16 GiB process, more than 99% of it permanently zero. Paging
// them makes that memory track the size of the colony instead of the size of
// the world. See docs/sparse-grids.md.
//
// Pages are square rather than row-major runs, which is the one place this
// differs from the tile grid published to frontends (tilegrid.go). That grid
// memcpys whole pages, so contiguity is what matters to it. These grids are
// written a tile at a time by things that spread outward from the colony, so
// what matters is how much of a page a blob-shaped colony actually uses. On a
// 10000-wide map a 4096-entry row-major page is a strip two fifths of a row
// long: a 500x500 colony would touch one or two of them per row it spans, ~750
// pages for 250k tiles. The same colony fits in 64 square pages.
//
// Pages are never freed. Terrain only ever opens up (Rock -> Floor -> built),
// so a page that has been written to is one the colony reached, and a colony
// does not un-reach it.
const (
	gridPageBits = 6                           // 64x64 tiles per page
	gridPageSide = 1 << gridPageBits           // 64
	gridPageMask = gridPageSide - 1            // 63
	gridPageLen  = gridPageSide * gridPageSide // 4096 entries
)

// A page is a whole number of chunks on a side, which lets anything sweeping
// one chunk hoist the page lookup out of its loop (see pageAt). The compiler
// rejects a gridPageSide that stops dividing evenly.
const _ = gridPageSide - chunkSize*(gridPageSide/chunkSize)

type pagedGrid[T any] struct {
	// colShift indexes a page row by shifting rather than multiplying. The
	// page table is rounded up to a power-of-two width to allow it: the slack
	// is empty slice headers (965 KB instead of 592 KB on a 10000x10000 map),
	// and it buys a shift in place of an IMUL on the hottest read in the
	// simulation.
	colShift int
	pages    [][]T
}

// newPagedGrid returns an all-zero grid covering a width x height map. Only the
// page table is allocated up front: ceil(w/64)*ceil(h/64) slice headers, which
// is 592 KB for a 10000x10000 map against the 381 MB a dense []int32 would take.
func newPagedGrid[T any](width, height int) pagedGrid[T] {
	shift := 0
	for 1<<shift < ceilDiv(width, gridPageSide) {
		shift++
	}
	return pagedGrid[T]{colShift: shift, pages: make([][]T, ceilDiv(height, gridPageSide)<<shift)}
}

// pageIndex is the page table slot holding (x, y).
func (g *pagedGrid[T]) pageIndex(x, y int) int {
	return (y>>gridPageBits)<<g.colShift | (x >> gridPageBits)
}

// offset is the slot within a page holding (x, y).
func offset(x, y int) int { return (y&gridPageMask)<<gridPageBits | (x & gridPageMask) }

// at reads (x, y), which must be in bounds. An unwritten page reads as the zero
// value, so every grid here is built so that zero means "nothing known": an
// unstamped generation, no region, no occupant.
func (g *pagedGrid[T]) at(x, y int) T {
	if page := g.pages[g.pageIndex(x, y)]; page != nil {
		return page[offset(x, y)]
	}
	var zero T
	return zero
}

// pageAt returns the page backing (x, y), or nil if nothing has written to it.
// It is how a caller sweeping one chunk hoists the page lookup out of its
// loop — a page holds a whole number of chunks and chunks are aligned, so
// every tile of a chunk is in the page its origin is. Index the result with
// offset(x, y); a nil page means every tile in the chunk reads as the zero
// value, which is usually a whole sweep the caller can skip.
func (g *pagedGrid[T]) pageAt(x, y int) []T { return g.pages[g.pageIndex(x, y)] }

// interiorPage returns the page holding (x, y) when all eight of its
// neighbours are in that page too, and nil when (x, y) sits on a page edge.
// It is the fast path for the 8-connected searches that do all the work here:
// a node that is not on an edge — (62/64)^2, about 94% of them — can index its
// whole neighbourhood off one lookup instead of nine. The caller must still
// bounds-check against the map, since the last page in a row or column can run
// past its edge.
//
// It does not allocate: every caller has already written (x, y) before
// expanding from it, so the page is there.
func (g *pagedGrid[T]) interiorPage(x, y int) []T {
	if ox, oy := x&gridPageMask, y&gridPageMask; ox == 0 || ox == gridPageMask || oy == 0 || oy == gridPageMask {
		return nil
	}
	return g.pages[g.pageIndex(x, y)]
}

// pageAtAlloc is pageAt for a sweep that writes, allocating the page up front.
func (g *pagedGrid[T]) pageAtAlloc(x, y int) []T {
	pi := g.pageIndex(x, y)
	page := g.pages[pi]
	if page == nil {
		page = make([]T, gridPageLen)
		g.pages[pi] = page
	}
	return page
}

// set writes v at (x, y), allocating its page if this is the first write to it.
func (g *pagedGrid[T]) set(x, y int, v T) {
	*g.ptr(x, y) = v
}

// ptr returns a pointer to (x, y), allocating its page if needed. It is for
// read-modify-write on a hot path, where going through at+set would look the
// page up twice. The pointer is valid until the next call: pages are never
// reallocated, but callers should not hold one across a set elsewhere.
func (g *pagedGrid[T]) ptr(x, y int) *T {
	pi := g.pageIndex(x, y)
	page := g.pages[pi]
	if page == nil {
		page = make([]T, gridPageLen)
		g.pages[pi] = page
	}
	return &page[offset(x, y)]
}

// pagesAllocated reports how many pages have been written to, for tests and
// diagnostics that assert the sparsity actually holds.
func (g *pagedGrid[T]) pagesAllocated() int {
	n := 0
	for _, page := range g.pages {
		if page != nil {
			n++
		}
	}
	return n
}
