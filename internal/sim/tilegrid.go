package sim

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
// Pages are slices of the row-major grid, so a page is one contiguous memcpy
// and a lookup is a shift and a mask. The size is a compromise: smaller pages
// copy less per changed tile, larger pages make the page table (which *is*
// copied per published grid) smaller. At 4096 tiles a page, a 7000x7000 map
// has a ~12k-entry table — under 100 KB per frame, against the 49 MB a full
// grid copy would cost.
const (
	tilePageBits = 12
	tilePageLen  = 1 << tilePageBits
	tilePageMask = tilePageLen - 1
)

type TileGrid struct {
	width, height int
	pages         [][]Tile
}

// NewTileGrid builds a standalone grid from a row-major tile slice. The engine
// publishes grids incrementally (see World.publishedTiles); this is for
// frontends and tests that need to synthesize one. A tiles slice shorter than
// width*height reads as Rock past its end rather than panicking on some later
// lookup deep inside a render.
func NewTileGrid(width, height int, tiles []Tile) *TileGrid {
	n := width * height
	g := &TileGrid{
		width:  width,
		height: height,
		pages:  make([][]Tile, ceilDiv(n, tilePageLen)),
	}
	for pi := range g.pages {
		g.pages[pi] = clonePage(tiles, pi, n)
	}
	return g
}

// clonePage copies page pi out of a row-major tile slice of n cells.
func clonePage(tiles []Tile, pi, n int) []Tile {
	lo := pi * tilePageLen
	hi := min(lo+tilePageLen, n)
	page := make([]Tile, hi-lo) // zero value is Rock, so a short slice pads solid
	if lo < len(tiles) {
		copy(page, tiles[lo:min(hi, len(tiles))])
	}
	return page
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
	i := p.Y*g.width + p.X
	return g.pages[i>>tilePageBits][i&tilePageMask]
}

// TerrainAt returns the terrain at p, or Rock out of bounds.
func (g *TileGrid) TerrainAt(p Point) Terrain { return g.At(p).Terrain }

// markTilePageDirty notes that the tile at row-major index i changed, so the
// next published grid re-copies its page. Called from SetTerrain, the only
// writer of the tile slice.
func (w *World) markTilePageDirty(i int) {
	pi := i >> tilePageBits
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
		w.snapGrid = NewTileGrid(w.Width, w.Height, w.tiles)
		w.clearDirtyPages()
		return w.snapGrid
	}
	if len(w.dirtyPages) == 0 {
		return w.snapGrid
	}
	// Copy the page table (pointers only), then swap in fresh copies of the
	// changed pages. Grids already published keep the old table, and with it
	// the pre-change pages, so nothing a frontend holds is disturbed.
	pages := make([][]Tile, len(w.snapGrid.pages))
	copy(pages, w.snapGrid.pages)
	for _, pi := range w.dirtyPages {
		pages[pi] = clonePage(w.tiles, pi, len(w.tiles))
	}
	w.clearDirtyPages()
	w.snapGrid = &TileGrid{width: w.Width, height: w.Height, pages: pages}
	return w.snapGrid
}

func (w *World) clearDirtyPages() {
	for _, pi := range w.dirtyPages {
		w.pageDirty[pi] = false
	}
	w.dirtyPages = w.dirtyPages[:0]
}
