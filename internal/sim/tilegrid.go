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
// on its own goroutine. The exception is a grid published under TilesLive,
// whose pages alias the world's own tiles; see TileSharing.
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
	pages         [][]tileCell
	// refuse is the published copy of the sparse gore/corpse index (see
	// World.refuse). It is a whole map rather than a paged plane because it
	// holds the tiles something died on, which is a few hundred entries in a
	// long game — small enough that copying it outright when it changes beats
	// any sharing scheme, and small enough that it does not belong in the
	// per-tile record.
	refuse map[Point]refuseCell
}

// NewTileGrid builds a standalone grid from a row-major tile slice. The engine
// publishes grids incrementally (see World.publishedTiles); this is for
// frontends and tests that need to synthesize one. A tiles slice shorter than
// width*height reads as Rock past its end rather than panicking on some later
// lookup deep inside a render.
func NewTileGrid(width, height int, tiles []Tile) *TileGrid {
	n := width * height
	cells := make([]tileCell, min(len(tiles), n))
	refuse := make(map[Point]refuseCell)
	for i := range cells {
		t := tiles[i]
		cells[i] = tileCell{Terrain: t.Terrain, Composition: t.Composition, Explored: t.Explored}
		if t.Gore != 0 || t.Corpses != 0 {
			refuse[Point{i % width, i / width}] = refuseCell{Gore: t.Gore, Corpses: t.Corpses}
		}
	}
	g := &TileGrid{
		width:  width,
		height: height,
		pages:  make([][]tileCell, ceilDiv(n, tilePageLen)),
		refuse: refuse,
	}
	for pi := range g.pages {
		g.pages[pi] = clonePage(cells, pi, n)
	}
	return g
}

// clonePage copies page pi out of a row-major tile slice of n cells.
func clonePage(tiles []tileCell, pi, n int) []tileCell {
	lo := pi * tilePageLen
	hi := min(lo+tilePageLen, n)
	page := make([]tileCell, hi-lo) // zero value is Rock, so a short slice pads solid
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
	c := g.pages[i>>tilePageBits][i&tilePageMask]
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
	i := p.Y*g.width + p.X
	return g.pages[i>>tilePageBits][i&tilePageMask].Terrain
}

// TilePageLen is how many tiles one TileGrid page holds. Page pi covers the
// row-major tile indexes [pi*TilePageLen, (pi+1)*TilePageLen), clipped to the
// map, which is how a consumer turns TileChanges.Pages into tiles.
const TilePageLen = tilePageLen

// TileSharing picks how a World publishes its terrain to Snapshots.
type TileSharing uint8

const (
	// TilesCopyOnWrite publishes an immutable page-shared grid: every page a
	// Snapshot holds is frozen, so the frame is safe to read on another
	// goroutine while the engine keeps ticking. This is what the TUI needs,
	// and the default.
	TilesCopyOnWrite TileSharing = iota
	// TilesLive publishes a grid whose pages alias World.tiles (and whose
	// refuse aliases World.refuse) instead of copying them. It costs no
	// memory beyond a page table built once, but the grid changes under the
	// reader on the next tick: a Snapshot is only valid on the engine's
	// goroutine, between the publish that made it and the next step. It is
	// for a consumer that runs there — the browser worker's wire encoder
	// (see the browser frontend proposal, PR 60) — and it is what saves that consumer a
	// second copy of the map (300 MB at 10000x10000). Read TileChanges
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
	// Pages are the indexes of the pages (see TilePageLen) whose tiles changed,
	// in the order they first changed. The slice belongs to the Snapshot.
	Pages []int
	// Refuse is set when the gore/corpse index changed.
	Refuse bool
}

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

// SetTileSharing switches how later Snapshots publish terrain. Switching drops
// the published grid, so the next Snapshot rebuilds it and reports All.
func (w *World) SetTileSharing(mode TileSharing) {
	if mode == w.tileSharing {
		return
	}
	w.tileSharing = mode
	w.snapGrid = nil
}

// publishedTiles returns the grid for the next Snapshot and what changed in it
// since the last one. In TilesCopyOnWrite mode it re-copies only the pages
// changed since the last grid; a tick that changed no terrain — most ticks,
// since digging a single rock takes several — reuses the previous grid
// wholesale and costs nothing at all. In TilesLive mode it copies nothing.
func (w *World) publishedTiles() (*TileGrid, TileChanges) {
	w.snapFrame++
	if w.snapGrid == nil {
		w.snapGrid = &TileGrid{
			width:  w.Width,
			height: w.Height,
			pages:  make([][]tileCell, ceilDiv(len(w.tiles), tilePageLen)),
		}
		for pi := range w.snapGrid.pages {
			if w.tileSharing == TilesLive {
				w.snapGrid.pages[pi] = livePage(w.tiles, pi)
			} else {
				w.snapGrid.pages[pi] = clonePage(w.tiles, pi, len(w.tiles))
			}
		}
		w.snapGrid.refuse = w.publishedRefuse()
		w.clearDirtyPages()
		return w.snapGrid, TileChanges{Frame: w.snapFrame, All: true, Refuse: true}
	}
	changes := TileChanges{Frame: w.snapFrame, Refuse: w.refuseRev != w.snapRefuseRev}
	if len(w.dirtyPages) == 0 && !changes.Refuse {
		return w.snapGrid, changes
	}
	changes.Pages = append([]int(nil), w.dirtyPages...)
	if w.tileSharing == TilesLive {
		// The pages and the refuse map are the live ones already; there is
		// nothing to copy, only the dirty list to hand over.
		w.snapRefuseRev = w.refuseRev
		w.clearDirtyPages()
		return w.snapGrid, changes
	}
	// Copy the page table (pointers only), then swap in fresh copies of the
	// changed pages. Grids already published keep the old table, and with it
	// the pre-change pages, so nothing a frontend holds is disturbed.
	pages := make([][]tileCell, len(w.snapGrid.pages))
	copy(pages, w.snapGrid.pages)
	for _, pi := range w.dirtyPages {
		pages[pi] = clonePage(w.tiles, pi, len(w.tiles))
	}
	w.clearDirtyPages()
	w.snapGrid = &TileGrid{width: w.Width, height: w.Height, pages: pages, refuse: w.publishedRefuse()}
	return w.snapGrid, changes
}

// livePage returns page pi of the row-major tile slice without copying it.
func livePage(tiles []tileCell, pi int) []tileCell {
	lo := pi * tilePageLen
	return tiles[lo:min(lo+tilePageLen, len(tiles)):min(lo+tilePageLen, len(tiles))]
}

// publishedRefuse returns an immutable copy of the refuse index, reusing the
// one already published when nothing has written to it since. Refuse changes
// only when something dies or a cleaner hauls it away, so the copy is rare
// even though the map is walked outright when it does happen.
func (w *World) publishedRefuse() map[Point]refuseCell {
	if w.tileSharing == TilesLive {
		w.snapRefuseRev = w.refuseRev
		return w.refuse
	}
	if w.snapGrid != nil && w.snapGrid.refuse != nil && w.refuseRev == w.snapRefuseRev {
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
