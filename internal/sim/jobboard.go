package sim

// jobBoard lets colonists pull work instead of each scanning the map every tick.
//
// The bulk of colony work is mining, and the mineable frontier (rock tiles that
// border open floor) changes only when terrain changes — so the board maintains
// it incrementally from TileChanged events. A colonist claims the nearest
// reachable frontier tile; the claim keeps other colonists off the same tile and
// is released when the tile is mined out (its TileChanged drops it) or the job is
// abandoned.
//
// Building is demand-driven rather than tile-driven (there are always plenty of
// build spots), so the board does not track build sites. It does keep O(1) counts
// of builds in progress, which replaces an O(colonists) scan when deciding
// whether the colony still needs another facility.
type jobBoard struct {
	w        *World
	frontier map[Point]struct{} // all mineable rock (rock bordering floor)
	claimed  map[Point]struct{} // frontier tiles a colonist is currently mining
	building [numTerrains]int   // builds in progress, by terrain kind
}

func newJobBoard(w *World) *jobBoard {
	return &jobBoard{
		w:        w,
		frontier: make(map[Point]struct{}),
		claimed:  make(map[Point]struct{}),
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

// claimNearestMine finds the nearest unclaimed frontier tile reachable from the
// given room, marks it claimed, and returns it. Ties break deterministically by
// position so the result does not depend on map iteration order.
func (b *jobBoard) claimNearestMine(from Point, room RoomID) (Point, bool) {
	var best Point
	found := false
	bestDist := 1 << 30
	for p := range b.frontier {
		if _, taken := b.claimed[p]; taken {
			continue
		}
		if !b.reachableFrom(p, room) {
			continue
		}
		d := from.Chebyshev(p)
		if !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
			best, bestDist, found = p, d, true
		}
	}
	if found {
		b.claimed[best] = struct{}{}
	}
	return best, found
}

// reachableFrom reports whether a frontier rock tile can be reached by a colonist
// in the given room: some floor neighbor lies in that room.
func (b *jobBoard) reachableFrom(rock Point, room RoomID) bool {
	if room == 0 {
		return false
	}
	w := b.w
	for _, d := range neighbors8 {
		n := rock.Add(d.X, d.Y)
		if w.Walkable(n) && w.roomOf(n) == room {
			return true
		}
	}
	return false
}

// releaseMine unclaims a mine target (job abandoned or finished).
func (b *jobBoard) releaseMine(target Point) {
	delete(b.claimed, target)
}

// startBuild / endBuild track builds in progress so plannedFacilities is O(1).
func (b *jobBoard) startBuild(kind Terrain) { b.building[kind]++ }

func (b *jobBoard) endBuild(kind Terrain) {
	if b.building[kind] > 0 {
		b.building[kind]--
	}
}

func (b *jobBoard) inProgress(kind Terrain) int { return b.building[kind] }

// lessPoint gives a stable ordering over points (row-major).
func lessPoint(a, b Point) bool {
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.X < b.X
}
