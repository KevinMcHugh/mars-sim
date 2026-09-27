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
// on its own goroutine.
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
			refuse[p] = refuseCell{Gore: t.Gore, Corpses: t.Corpses}
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
		Corpses:     r.Corpses,
	}
}

// TerrainAt returns the terrain at p, or Rock out of bounds.
func (g *TileGrid) TerrainAt(p Point) Terrain {
	if g == nil || p.X < 0 || p.X >= g.width || p.Y < 0 || p.Y >= g.height {
		return Rock
	}
	return g.cell(p).Terrain
}

// cell reads the stored record at the in-bounds p; a page never written reads
// as zero, which is unexplored Rock.
func (g *TileGrid) cell(p Point) tileCell {
	if page := g.pages[(p.Y>>gridPageBits)<<g.colShift|(p.X>>gridPageBits)]; page != nil {
		return page[offset(p.X, p.Y)]
	}
	return tileCell{}
}

// markTilePageDirty notes that the page holding p changed, so the next
// published grid re-copies it. Called by every writer of the tile grid.
func (w *World) markTilePageDirty(p Point) {
	pi := w.tiles.pageIndex(p.X, p.Y)
	if w.pageDirty[pi] {
		return
	}
	w.pageDirty[pi] = true
	w.dirtyPages = append(w.dirtyPages, pi)
}

// publishedTiles returns the immutable grid for the next Snapshot, re-copying
// only the pages changed since the last one. A tick that changed no terrain —
// most ticks, since digging a single rock takes several — reuses the previous
// grid wholesale and costs nothing at all.
func (w *World) publishedTiles() *TileGrid {
	if w.snapGrid == nil {
		pages := make([]*tilePage, len(w.tiles.pages))
		for pi, page := range w.tiles.pages {
			if page != nil {
				pages[pi] = clonePage(page)
			}
		}
		w.clearDirtyPages()
		w.snapGrid = &TileGrid{width: w.Width, height: w.Height, colShift: w.tiles.colShift, pages: pages, refuse: w.publishedRefuse()}
		return w.snapGrid
	}
	if len(w.dirtyPages) == 0 && w.refuseRev == w.snapRefuseRev {
		return w.snapGrid
	}
	// Copy the page table (pointers only), then swap in fresh copies of the
	// changed pages. Grids already published keep the old table, and with it
	// the pre-change pages, so nothing a frontend holds is disturbed.
	pages := slices.Clone(w.snapGrid.pages)
	for _, pi := range w.dirtyPages {
		pages[pi] = clonePage(w.tiles.pages[pi])
	}
	w.clearDirtyPages()
	w.snapGrid = &TileGrid{width: w.Width, height: w.Height, colShift: w.tiles.colShift, pages: pages, refuse: w.publishedRefuse()}
	return w.snapGrid
}

// publishedRefuse returns an immutable copy of the refuse index, reusing the
// one already published when nothing has written to it since. Refuse changes
// only when something dies or a cleaner hauls it away, so the copy is rare
// even though the map is walked outright when it does happen.
func (w *World) publishedRefuse() map[Point]refuseCell {
	if w.snapGrid != nil && w.refuseRev == w.snapRefuseRev {
		return w.snapGrid.refuse
	}
	out := make(map[Point]refuseCell, len(w.refuse))
	for p, r := range w.refuse {
		out[p] = r
	}
	w.snapRefuseRev = w.refuseRev
	return out
}

func (w *World) clearDirtyPages() {
	for _, pi := range w.dirtyPages {
		w.pageDirty[pi] = false
	}
	w.dirtyPages = w.dirtyPages[:0]
}
