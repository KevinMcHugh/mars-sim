package sim

// jobBoard tracks the mineable frontier (rock tiles that border open floor) so
// mining does not require per-colonist map scans. The frontier is maintained
// incrementally from TileChanged events; the shared frontier flow field routes
// every miner toward the nearest *unclaimed* frontier tile.
//
// Claims are made on arrival, not at assignment: a colonist follows the field to
// the digging edge and claims a specific adjacent rock only when it gets there.
// A claim is owner-keyed so releasing is safe, and keeps two colonists off the
// same tile. The board also keeps O(1) counts of builds in progress.
type jobBoard struct {
	w        *World
	frontier map[Point]struct{} // all mineable rock (rock bordering floor)
	claimed  map[Point]EntityID // frontier tiles currently being mined, by owner
	building [numTerrains]int   // builds in progress, by terrain kind
}

func newJobBoard(w *World) *jobBoard {
	return &jobBoard{
		w:        w,
		frontier: make(map[Point]struct{}),
		claimed:  make(map[Point]EntityID),
	}
}

// onTileChanged keeps the mining frontier in step with the terrain: the changed
// tile and each of its neighbors may have gained or lost frontier status.
func (b *jobBoard) onTileChanged(ev TileChanged) {
	b.refreshFrontierCell(ev.Pos)
	for _, d := range neighbors8 {
		b.refreshFrontierCell(ev.Pos.Add(d.X, d.Y))
	}
}

// refreshFrontierCell recomputes whether one cell belongs to the frontier.
func (b *jobBoard) refreshFrontierCell(p Point) {
	w := b.w
	if w.InBounds(p) && w.TerrainAt(p) == Rock && w.bordersFloor(p) {
		b.frontier[p] = struct{}{}
		return
	}
	// No longer mineable (mined out, walled off, or out of bounds): drop it and
	// release any claim so the miner's job resolves next tick.
	delete(b.frontier, p)
	delete(b.claimed, p)
}

// isFrontier reports whether p is a mineable rock tile.
func (b *jobBoard) isFrontier(p Point) bool {
	_, ok := b.frontier[p]
	return ok
}

// isClaimed reports whether some colonist is already mining p.
func (b *jobBoard) isClaimed(p Point) bool {
	_, ok := b.claimed[p]
	return ok
}

// unclaimedCount is the number of frontier tiles nobody is mining yet.
func (b *jobBoard) unclaimedCount() int {
	return len(b.frontier) - len(b.claimed)
}

// claimMine marks a frontier tile as being mined by id, and dirties the frontier
// field so other miners route around it.
func (b *jobBoard) claimMine(p Point, id EntityID) {
	b.claimed[p] = id
	b.w.frontier.stale = true
}

// releaseMine drops id's claim on p (if it holds it), reopening the tile as a
// frontier goal.
func (b *jobBoard) releaseMine(p Point, id EntityID) {
	if b.claimed[p] == id {
		delete(b.claimed, p)
		b.w.frontier.stale = true
	}
}

// startBuild / endBuild track builds in progress so plannedFacilities is O(1).
func (b *jobBoard) startBuild(kind Terrain) { b.building[kind]++ }

func (b *jobBoard) endBuild(kind Terrain) {
	if b.building[kind] > 0 {
		b.building[kind]--
	}
}

func (b *jobBoard) inProgress(kind Terrain) int { return b.building[kind] }
