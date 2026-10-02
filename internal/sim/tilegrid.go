package sim

import "slices"

// TileGrid is an immutable view of the world's terrain, handed to frontends
// inside a Snapshot. It exists so publishing a frame does not cost a copy of
// the whole map: the grid is stored as fixed-size **pages**, and each new grid
// re-copies only the pages whose tiles changed since the last one, sharing the
// rest with the grids already in frontends' hands.
//
// That sharing is what makes it safe to expose: a page is copied before it can
// diverge from the grid that published it (copy-on-write), so no TileGrid ever
// changes under a reader, even though the engine keeps mutating the live world
// on its own goroutine. The exception is a grid published under TilesLive,
// whose pages alias the world's own; see TileSharing.
//
// The pages are the same 64x64 squares the World stores its tiles in
// (pagedGrid, see pagedgrid.go), so publishing a page is one contiguous memcpy
// of the world's own page, and a page the world has never written is nil here
// too and reads as unexplored Rock. That is also one worldgen chunk (see
// worldgen_chunks.go). The page table is copied per published grid: ~40k
// entries on a 10000x10000 map (rounded up to a power-of-two width, as in
// pagedGrid), about 320 KB per frame, against the 300 MB a full grid copy
// would cost.
type TileGrid struct {
	width, height int
	colShift      int // as pagedGrid.colShift
	// pages is laid out like the world's pagedGrid page table, but holds
	// array pointers rather than slice headers: the table is what every
	// published frame copies, and 8 bytes an entry is a third of 24.
	pages []*tilePage
	// refuse is the published copy of the sparse gore/corpse index (see
	// World.refuse). It is a whole map rather than a paged plane because it
	// holds the tiles something died on, which is a few hundred entries in a
	// long game — small enough that copying it outright when it changes beats
	// any sharing scheme, and small enough that it does not belong in the
	// per-tile record.
	refuse map[Point]refuseCell
}

// tilePage is one published page: a copy of one of the world's tile pages.
type tilePage = [gridPageLen]tileCell

// TilePageSide is the side, in tiles, of one TileGrid page. TileChanges.Pages
// are page table indexes; PageOrigin turns one into the top-left tile of its
// TilePageSide x TilePageSide square, which is clipped to the map.
const TilePageSide = gridPageSide

// PageOrigin returns the top-left tile of page pi (see TileChanges.Pages).
func (g *TileGrid) PageOrigin(pi int) Point {
	return Point{(pi & (1<<g.colShift - 1)) << gridPageBits, (pi >> g.colShift) << gridPageBits}
}

// clonePage copies one of the world's tile pages for publishing.
func clonePage(page []tileCell) *tilePage {
	c := tilePage(page)
	return &c
}

// NewTileGrid builds a standalone grid from a row-major tile slice. The engine
// publishes grids incrementally (see World.publishedTiles); this is for
// frontends and tests that need to synthesize one. A tiles slice shorter than
// width*height reads as Rock past its end rather than panicking on some later
// lookup deep inside a render.
func NewTileGrid(width, height int, tiles []Tile) *TileGrid {
	cells := newPagedGrid[tileCell](width, height)
	refuse := make(map[Point]refuseCell)
	for i := 0; i < min(len(tiles), width*height); i++ {
		t := tiles[i]
		p := Point{i % width, i / width}
		if c := (tileCell{Terrain: t.Terrain, Composition: t.Composition, Explored: t.Explored}); c != (tileCell{}) {
			cells.set(p.X, p.Y, c)
		}
		if t.Gore != 0 || t.Corpses != 0 {
			// A hand-built frame only says how many bodies, not whose; the
			// count is all a renderer reads.
			var c [numCorpseKinds]uint16
			c[0] = t.Corpses
			refuse[p] = refuseCell{Gore: t.Gore, Corpses: c}
		}
	}
	g := &TileGrid{width: width, height: height, colShift: cells.colShift, pages: make([]*tilePage, len(cells.pages)), refuse: refuse}
	for pi, page := range cells.pages {
		if page != nil {
			g.pages[pi] = (*tilePage)(page)
		}
	}
	return g
}

// Width and Height report the grid's dimensions in tiles.
func (g *TileGrid) Width() int  { return g.width }
func (g *TileGrid) Height() int { return g.height }

// At returns the tile at p. Out-of-bounds reads return a Rock tile so callers
// (the renderer) can treat the world edge as solid.
func (g *TileGrid) At(p Point) Tile {
	if g == nil || p.X < 0 || p.X >= g.width || p.Y < 0 || p.Y >= g.height {
		return Tile{Terrain: Rock}
	}
	c := g.cell(p)
	r := g.refuse[p]
	return Tile{
		Terrain:     c.Terrain,
		Composition: c.Composition,
		Explored:    c.Explored,
		Gore:        r.Gore,
		Corpses:     uint16(r.total()),
	}
}

// TerrainAt returns the terrain at p, or Rock out of bounds.
func (g *TileGrid) TerrainAt(p Point) Terrain {
	if g == nil || p.X < 0 || p.X >= g.width || p.Y < 0 || p.Y >= g.height {
		return Rock
	}
	return g.cell(p).Terrain
}

// hasPage reports whether the page holding the in-bounds p exists: in a
// generated world, whether its chunk has been generated.
func (g *TileGrid) hasPage(p Point) bool {
	return g.pages[(p.Y>>gridPageBits)<<g.colShift|(p.X>>gridPageBits)] != nil
}

// cell reads the stored record at the in-bounds p; a page never written reads
// as zero, which is unexplored Rock.
func (g *TileGrid) cell(p Point) tileCell {
	if page := g.pages[(p.Y>>gridPageBits)<<g.colShift|(p.X>>gridPageBits)]; page != nil {
		return page[offset(p.X, p.Y)]
	}
	return tileCell{}
}

// TileSharing picks how a World publishes its terrain to Snapshots.
type TileSharing uint8

const (
	// TilesCopyOnWrite publishes an immutable page-shared grid: every page a
	// Snapshot holds is frozen, so the frame is safe to read on another
	// goroutine while the engine keeps ticking. This is what the TUI needs,
	// and the default.
	TilesCopyOnWrite TileSharing = iota
	// TilesLive publishes a grid whose pages alias the world's tile pages
	// (and whose refuse aliases World.refuse) instead of copying them. It
	// costs no memory beyond one page table, but the grid changes under the
	// reader on the next tick: a Snapshot is only valid on the engine's
	// goroutine, between the publish that made it and the next step. It is
	// for a consumer that runs there — the browser worker's wire encoder
	// (see docs/browser-frontend.md) — and it is what saves that
	// consumer a second copy of every generated chunk. Read TileChanges
	// rather than page identity to learn what changed: live pages never move.
	TilesLive
)

// TileChanges says what in a Snapshot's Tiles differs from the Snapshot the
// same World published before it. The copy-on-write grid also shows this
// through page identity, but a live grid's pages never move, so this is the
// signal that works in both modes.
//
// It is a delta against the previous Snapshot *built*, not the previous one a
// consumer *saw*: the dirty list is cleared on every publish. A consumer that
// applies changes incrementally must see every frame, which a Subscribe
// channel does not promise (it drops stale frames). Frame is how a consumer
// notices a gap: if it last applied frame F and receives anything but F+1, it
// missed changes and must reread every page it cares about, as if All were set.
type TileChanges struct {
	// Frame numbers this World's published Snapshots, starting at 1. The
	// changes below are relative to Frame-1.
	Frame uint64
	// All is set on the first Snapshot a World publishes (and the first after
	// switching TileSharing): there is no previous grid, so every page is new.
	// Pages is empty when All is set.
	All bool
	// Pages are the page table indexes (see PageOrigin) of the pages whose
	// tiles changed, in the order they first changed. That includes a chunk
	// generated since the last frame, whose page is new. The slice belongs to
	// the Snapshot.
	Pages []int
	// Refuse is set when the gore/corpse index changed.
	Refuse bool
}

// markTilePageDirty notes that the page holding p changed, so the next
// published grid re-copies it. Called by every writer of the tile grid.
func (w *World) markTilePageDirty(p Point) {
	pi := w.home.tiles.pageIndex(p.X, p.Y)
	if w.home.pageDirty[pi] {
		return
	}
	w.home.pageDirty[pi] = true
	w.home.dirtyPages = append(w.home.dirtyPages, pi)
}

// SetTileSharing switches how later Snapshots publish terrain. Switching drops
// the published grid, so the next Snapshot rebuilds it and reports All.
func (w *World) SetTileSharing(mode TileSharing) {
	if mode == w.tileSharing {
		return
	}
	w.tileSharing = mode
	w.home.snapGrid = nil
}

// publishedTiles returns the grid for the next Snapshot and what changed in it
// since the last one. In TilesCopyOnWrite mode it re-copies only the pages
// changed since the last grid; a tick that changed no terrain — most ticks,
// since digging a single rock takes several — reuses the previous grid
// wholesale and costs nothing at all. In TilesLive mode it copies nothing.
func (w *World) publishedTiles() (*TileGrid, TileChanges) {
	w.snapFrame++
	if w.home.snapGrid == nil {
		pages := make([]*tilePage, len(w.home.tiles.pages))
		for pi, page := range w.home.tiles.pages {
			if page != nil {
				pages[pi] = w.publishPage(page)
			}
		}
		w.clearDirtyPages()
		w.home.snapGrid = &TileGrid{width: w.Width, height: w.Height, colShift: w.home.tiles.colShift, pages: pages}
		w.home.snapGrid.refuse = w.publishedRefuse()
		return w.home.snapGrid, TileChanges{Frame: w.snapFrame, All: true, Refuse: true}
	}
	changes := TileChanges{Frame: w.snapFrame, Refuse: w.home.refuseRev != w.home.snapRefuseRev}
	if len(w.home.dirtyPages) == 0 && !changes.Refuse {
		return w.home.snapGrid, changes
	}
	changes.Pages = slices.Clone(w.home.dirtyPages)
	if w.tileSharing == TilesLive {
		// Existing pages are the live ones already. A dirty page may be a
		// chunk generated since the last frame, which the table does not
		// point at yet; the world never reallocates a page, so pointing at it
		// once is enough. Nothing is copied.
		for _, pi := range w.home.dirtyPages {
			w.home.snapGrid.pages[pi] = (*tilePage)(w.home.tiles.pages[pi])
		}
		w.home.snapRefuseRev = w.home.refuseRev
		w.clearDirtyPages()
		return w.home.snapGrid, changes
	}
	// Copy the page table (pointers only), then swap in fresh copies of the
	// changed pages. Grids already published keep the old table, and with it
	// the pre-change pages, so nothing a frontend holds is disturbed.
	pages := slices.Clone(w.home.snapGrid.pages)
	for _, pi := range w.home.dirtyPages {
		pages[pi] = clonePage(w.home.tiles.pages[pi])
	}
	w.clearDirtyPages()
	w.home.snapGrid = &TileGrid{width: w.Width, height: w.Height, colShift: w.home.tiles.colShift, pages: pages, refuse: w.publishedRefuse()}
	return w.home.snapGrid, changes
}

// publishPage is how a world page enters a freshly built grid: a copy, or
// under TilesLive the page itself.
func (w *World) publishPage(page []tileCell) *tilePage {
	if w.tileSharing == TilesLive {
		return (*tilePage)(page)
	}
	return clonePage(page)
}

// publishedRefuse returns an immutable copy of the refuse index, reusing the
// one already published when nothing has written to it since. Refuse changes
// only when something dies or a cleaner hauls it away, so the copy is rare
// even though the map is walked outright when it does happen.
func (w *World) publishedRefuse() map[Point]refuseCell {
	if w.tileSharing == TilesLive {
		w.home.snapRefuseRev = w.home.refuseRev
		return w.home.refuse
	}
	if w.home.snapGrid != nil && w.home.snapGrid.refuse != nil && w.home.refuseRev == w.home.snapRefuseRev {
		return w.home.snapGrid.refuse
	}
	out := make(map[Point]refuseCell, len(w.home.refuse))
	for p, r := range w.home.refuse {
		out[p] = r
	}
	w.home.snapRefuseRev = w.home.refuseRev
	return out
}

func (w *World) clearDirtyPages() {
	for _, pi := range w.home.dirtyPages {
		w.home.pageDirty[pi] = false
	}
	w.home.dirtyPages = w.home.dirtyPages[:0]
}
