package sim

import "slices"

// Rooms group the walkable map into connected areas, which the colonist AI uses
// for reachability ("can I get from here to that job?"). They are built in two
// levels so updates stay cheap as the map changes:
//
//   - A region is a connected component of Floor cells within a single chunk.
//     When a tile changes, only that one chunk's regions are recomputed.
//   - A room is a connected component of the region graph: regions are linked
//     when their floor cells touch (8-connectivity) across a chunk border.
//     Rooms are relabelled from the small region graph, not the full tile grid.
//
// The result: excavating or walling a tile costs O(one chunk) to re-flood plus
// O(regions) to relabel, instead of a global O(map) flood fill.

// RegionID identifies a per-chunk floor component. 0 means "no region".
type RegionID int32

// RoomID identifies a connected group of regions. 0 means "no room" (not floor).
// A room's ID is the smallest RegionID it contains, so it is deterministic.
type RoomID int32

type region struct {
	id    RegionID
	chunk int
	size  int
	// discovered is set if any of the region's cells is Explored. Undiscovered
	// regions are natural cavern floor the colony has not broken into; they
	// never count as the colony's main room. See docs/caverns.md.
	discovered bool
	visitPass  uint32 // see World.relabelPass
	room       RoomID
	rep        Point                 // a representative cell (for the abstract heuristic)
	links      map[RegionID]struct{} // adjacent regions (across chunk borders)
}

// refreshSpatial brings regions and rooms up to date for any chunks that have
// changed since the last call. It is invoked after world generation and at the
// end of every tick; it is a no-op when nothing changed.
func (w *World) refreshSpatial() {
	if len(w.dirtyChunks) == 0 {
		return
	}
	dirty := make([]int, 0, len(w.dirtyChunks))
	for ci := range w.dirtyChunks {
		dirty = append(dirty, ci)
	}
	// Chunk order decides the order regions are created in, and a region's ID
	// is just the next counter value. Leaving that to map iteration handed the
	// same world different region IDs on different runs -- and since a room is
	// named for the smallest region ID it contains, and the abstract search
	// breaks ties toward the lower ID, that reached all the way out to which
	// tile a colonist stepped on.
	slices.Sort(dirty)
	// Pass 1: re-flood each dirty chunk's regions (needs no neighbor state).
	for _, ci := range dirty {
		w.recomputeChunkRegions(ci)
	}
	// Pass 2: link regions across borders, now that all dirty chunks have final
	// region IDs assigned.
	for _, ci := range dirty {
		w.linkChunkRegions(ci)
	}
	for ci := range w.dirtyChunks {
		delete(w.dirtyChunks, ci)
	}
	w.relabelRooms()
}

// recomputeChunkRegions discards the chunk's old regions and re-floods its Floor
// cells into fresh regions (without links yet).
func (w *World) recomputeChunkRegions(ci int) {
	x0, y0, x1, y1 := w.chunkBounds(ci)

	// Drop old regions in this chunk and unlink them from their neighbors. The
	// chunk lies inside a single page of regionOf (see pagedGrid.pageAt), so
	// every access in this function hoists to one lookup.
	regions := w.regionOf.pageAt(x0, y0)
	old := make(map[RegionID]struct{})
	if regions != nil {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if o := offset(x, y); regions[o] != 0 {
					old[regions[o]] = struct{}{}
					regions[o] = 0
				}
			}
		}
	}
	for rid := range old {
		r := w.regions[rid]
		if r == nil {
			continue
		}
		// Whatever room this region was in may have split or shrunk, so it
		// is relabeled from scratch; its surviving neighbors are where the
		// flood starts (see relabelRooms).
		w.staleRooms[r.room] = struct{}{}
		for nb := range r.links {
			if nr := w.regions[nb]; nr != nil {
				delete(nr.links, rid)
				w.relabelSeeds = append(w.relabelSeeds, nb)
			}
		}
		delete(w.regions, rid)
	}

	// Flood-fill new regions, staying within the chunk bounds.
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if w.tiles[y*w.Width+x].Terrain != Floor {
				continue
			}
			if regions == nil {
				regions = w.regionOf.pageAtAlloc(x0, y0)
			}
			if regions[offset(x, y)] != 0 {
				continue
			}
			rid := w.nextRegion
			w.nextRegion++
			reg := &region{id: rid, chunk: ci, rep: Point{x, y}, links: make(map[RegionID]struct{})}
			w.regions[rid] = reg
			w.relabelSeeds = append(w.relabelSeeds, rid)

			w.floodStack = append(w.floodStack[:0], Point{x, y})
			regions[offset(x, y)] = rid
			for len(w.floodStack) > 0 {
				p := w.floodStack[len(w.floodStack)-1]
				w.floodStack = w.floodStack[:len(w.floodStack)-1]
				reg.size++
				if w.tiles[p.Y*w.Width+p.X].Explored {
					reg.discovered = true
				}
				for _, d := range neighbors8 {
					qx, qy := p.X+d.X, p.Y+d.Y
					if qx < x0 || qx >= x1 || qy < y0 || qy >= y1 {
						continue
					}
					j := qy*w.Width + qx
					if o := offset(qx, qy); w.tiles[j].Terrain == Floor && regions[o] == 0 {
						regions[o] = rid
						w.floodStack = append(w.floodStack, Point{qx, qy})
					}
				}
			}
		}
	}
}

// linkChunkRegions connects this chunk's regions to adjacent regions in other
// chunks wherever their floor cells touch. (Two floor cells in the same chunk
// that touch are already the same region, so only cross-region touches matter.)
func (w *World) linkChunkRegions(ci int) {
	x0, y0, x1, y1 := w.chunkBounds(ci)
	regions := w.regionOf.pageAt(x0, y0)
	if regions == nil {
		return // nothing in this chunk is floor, so there is nothing to link
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			rid := regions[offset(x, y)]
			if rid == 0 {
				continue
			}
			for _, d := range neighbors8 {
				q := Point{x + d.X, y + d.Y}
				if !w.InBounds(q) {
					continue
				}
				nid := w.regionOf.at(q.X, q.Y)
				if nid == 0 || nid == rid {
					continue
				}
				w.regions[rid].links[nid] = struct{}{}
				w.regions[nid].links[rid] = struct{}{}
			}
		}
	}
}

// relabelRooms brings room membership up to date after refreshSpatial has
// re-flooded the dirty chunks. A room is a connected component of the region
// graph, named for the smallest RegionID it contains.
//
// It is incremental: only components touching a region that was just created,
// or that neighbored one just deleted, can have changed, so only those are
// re-flooded (relabelSeeds), and every room they previously belonged to
// (staleRooms) is dropped from w.rooms before the fresh components go in.
// Every other room keeps its label untouched. That matters because
// undiscovered natural caverns put thousands of static rooms on a big map (see
// docs/caverns.md); walking all of them on every dig cost milliseconds a tick.
//
// It also tracks mainRoom, the discovered room with the most floor tiles (an
// undiscovered cavern can be bigger than the landing site, but nobody lives
// there): the colony's main connected network, against which
// updateDisconnected checks every colonist so a pocket cut off by later
// construction (see doorTiles in project.go) or any other cause eventually
// notices and breaks itself out. See docs/escape.md.
func (w *World) relabelRooms() {
	w.relabelPass++
	pass := w.relabelPass
	fresh := w.freshRooms[:0]
	for _, start := range w.relabelSeeds {
		sr := w.regions[start]
		if sr == nil || sr.visitPass == pass {
			continue // deleted by a later dirty chunk, or already relabeled
		}
		// Gather the component, tracking its minimum ID as the room ID and its
		// total floor tiles across all member regions.
		component := w.roomScratch[:0]
		stack := append(w.roomStack[:0], start)
		sr.visitPass = pass
		info := roomInfo{id: RoomID(start)}
		for len(stack) > 0 {
			r := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			reg := w.regions[r]
			component = append(component, r)
			info.size += reg.size
			info.discovered = info.discovered || reg.discovered
			if RoomID(r) < info.id {
				info.id = RoomID(r)
			}
			if reg.room != 0 {
				// A merge can pull in a room nothing was deleted from.
				w.staleRooms[reg.room] = struct{}{}
			}
			for nb := range reg.links {
				if nr := w.regions[nb]; nr.visitPass != pass {
					nr.visitPass = pass
					stack = append(stack, nb)
				}
			}
		}
		w.roomStack = stack[:0]
		for _, r := range component {
			w.regions[r].room = info.id
		}
		w.roomScratch = component[:0]
		fresh = append(fresh, info)
	}
	w.relabelSeeds = w.relabelSeeds[:0]

	// Drop every stale room before adding the fresh ones: a component that
	// kept its smallest region is still named the same.
	for id := range w.staleRooms {
		delete(w.rooms, id)
		delete(w.discoveredRooms, id)
		delete(w.staleRooms, id)
	}
	for _, info := range fresh {
		w.rooms[info.id] = info.size
		if info.discovered {
			w.discoveredRooms[info.id] = info.size
		}
	}
	w.freshRooms = fresh[:0]
	w.roomCount = len(w.rooms)

	// Only discovered rooms can be the main one, and there are few of them (the
	// colony and whatever caverns it has broken into), so a scan is cheap.
	// Compare explicitly rather than tracking "first seen": map iteration order
	// is randomized, so only a deterministic tie-break (smallest RoomID) keeps
	// mainRoom reproducible for a given seed.
	var mainRoom RoomID
	mainSize := -1
	for id, size := range w.discoveredRooms {
		if size > mainSize || (size == mainSize && id < mainRoom) {
			mainSize, mainRoom = size, id
		}
	}
	w.mainRoom = mainRoom
}

// roomInfo summarizes one freshly flooded room for relabelRooms.
type roomInfo struct {
	id         RoomID
	size       int
	discovered bool
}

// roomOf returns the room a tile belongs to, or 0 if it is not floor.
func (w *World) roomOf(p Point) RoomID {
	if !w.InBounds(p) {
		return 0
	}
	rid := w.regionOf.at(p.X, p.Y)
	if rid == 0 {
		return 0
	}
	return w.regions[rid].room
}

// sameRoom reports whether a and b are floor tiles in the same room (mutually
// reachable on foot).
func (w *World) sameRoom(a, b Point) bool {
	ra := w.roomOf(a)
	return ra != 0 && ra == w.roomOf(b)
}

// chunkBounds returns the half-open tile bounds [x0,x1) x [y0,y1) of a chunk.
func (w *World) chunkBounds(ci int) (x0, y0, x1, y1 int) {
	cx := ci % w.chunkCols
	cy := ci / w.chunkCols
	x0, y0 = cx*chunkSize, cy*chunkSize
	x1 = min(x0+chunkSize, w.Width)
	y1 = min(y0+chunkSize, w.Height)
	return x0, y0, x1, y1
}
