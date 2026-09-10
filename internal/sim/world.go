package sim

import "math/rand"

// Terrain is what fills a single tile. The world is a dense grid of tiles; as
// colonists dig and build, tiles change terrain.
type Terrain uint8

const (
	// Rock is unexcavated Martian regolith. Colonists cannot walk through it,
	// but they can mine it into Floor. Aliens burrow through it freely.
	Rock Terrain = iota
	// Floor is open, walkable space: a mined-out cavern or corridor.
	Floor
	// Wall is a built structure. Blocks colonist movement like Rock, but it is
	// constructed rather than natural.
	Wall
	// NutrientPod dispenses food; a colonist stands beside it to eat. Blocks
	// movement like a wall.
	NutrientPod
	// Toilet relieves the bladder need; used from an adjacent tile.
	Toilet

	numTerrains // keep last: the number of terrain kinds
)

// Walkable reports whether a colonist can stand on this terrain. Aliens ignore
// this; they move through anything.
func (t Terrain) Walkable() bool {
	return t == Floor
}

// Tile is one cell of the world. It is deliberately a struct rather than a bare
// Terrain so we have room to grow (ore, moisture, temperature, ...) without
// touching every call site.
type Tile struct {
	Terrain Terrain
}

// World is the mutable game state for a single underground level. It is owned by
// the Engine's goroutine and must not be read or written from any other
// goroutine; frontends observe it through immutable Snapshots instead.
type World struct {
	Width, Height int
	tiles         []Tile // row-major, len == Width*Height

	// occ is a dense occupancy index parallel to tiles: occ[i] is the EntityID
	// standing on that tile, or 0 for empty (IDs start at 1). It turns "who is
	// here?" from an O(entities) scan into an O(1) lookup, and it is the reason
	// at most one entity may occupy a tile.
	occ []EntityID

	// Aggregate counts maintained incrementally so callers never rescan the
	// grid or the entity set to answer "how many of X?".
	terrainCounts [numTerrains]int
	kindCounts    [numKinds]int

	// Spatial index: entities bucketed by chunk, so neighbor queries scan only
	// nearby chunks. chunkEntities is indexed by chunk (cy*chunkCols + cx).
	chunkCols, chunkRows int
	chunkEntities        [][]EntityID

	// Regions & rooms: floor tiles grouped into per-chunk regions, then into
	// rooms (connected components of the region graph). Maintained incrementally
	// as terrain changes so reachability queries stay cheap. See rooms.go.
	regionOf    []RegionID // parallel to tiles; 0 = not floor / no region
	regions     map[RegionID]*region
	nextRegion  RegionID
	dirtyChunks map[int]struct{} // chunks whose regions need recompute
	roomCount   int

	// Reusable scratch buffers for refreshSpatial (avoid per-call allocation).
	floodStack  []Point
	roomScratch []RegionID

	// Reactive plumbing: systems subscribe to world events; the job board is the
	// first consumer, tracking the mineable frontier from TileChanged events.
	subscribers []func(Event)
	board       *jobBoard
	pf          *pathfinder

	// Shared flow fields, rebuilt lazily when terrain (or, for the frontier,
	// claims) change. fields[t] routes seekers to facility terrain t; frontier
	// routes miners to the nearest unclaimed diggable rock. See flowfield.go.
	fields   [numTerrains]*flowField
	frontier *flowField

	entities map[EntityID]*Entity
	nextID   EntityID

	tick int
	rng  *rand.Rand
	log  *eventLog
	cfg  Config
}

// newWorld allocates an all-Rock world of the given size.
func newWorld(cfg Config, rng *rand.Rand) *World {
	n := cfg.Width * cfg.Height
	w := &World{
		Width:    cfg.Width,
		Height:   cfg.Height,
		tiles:    make([]Tile, n),
		occ:      make([]EntityID, n),
		entities: make(map[EntityID]*Entity),
		nextID:   1,
		rng:      rng,
		log:      newEventLog(cfg.LogSize),
		cfg:      cfg,
	}
	w.terrainCounts[Rock] = n // every tile starts as Rock

	w.chunkCols = ceilDiv(cfg.Width, chunkSize)
	w.chunkRows = ceilDiv(cfg.Height, chunkSize)
	w.chunkEntities = make([][]EntityID, w.chunkCols*w.chunkRows)

	w.regionOf = make([]RegionID, n)
	w.regions = make(map[RegionID]*region)
	w.nextRegion = 1
	w.dirtyChunks = make(map[int]struct{})

	w.board = newJobBoard(w)
	w.subscribe(func(e Event) {
		if tc, ok := e.(TileChanged); ok {
			w.board.onTileChanged(tc)
		}
	})
	w.pf = newPathfinder(w)

	for i := 0; i < int(numNeeds); i++ {
		if f := cfg.Needs[i].Facility; f != Rock && w.fields[f] == nil {
			w.fields[f] = newFlowField(w, facilitySeed(w, f))
		}
	}
	w.frontier = newFlowField(w, func(add func(Point)) {
		// Goals: walkable neighbors of every unclaimed frontier rock tile.
		for p := range w.board.frontier {
			if _, taken := w.board.claimed[p]; taken {
				continue
			}
			for _, d := range neighbors8 {
				add(p.Add(d.X, d.Y))
			}
		}
	})
	w.subscribe(func(e Event) {
		if _, ok := e.(TileChanged); ok {
			for _, f := range w.fields {
				if f != nil {
					f.stale = true // any terrain change can shift goals or routes
				}
			}
			w.frontier.stale = true
		}
	})
	return w
}

// index converts a coordinate to a slice offset. Callers must ensure the point
// is in bounds.
func (w *World) index(p Point) int {
	return p.Y*w.Width + p.X
}

// InBounds reports whether p lies inside the world grid.
func (w *World) InBounds(p Point) bool {
	return p.X >= 0 && p.X < w.Width && p.Y >= 0 && p.Y < w.Height
}

// TerrainAt returns the terrain at p, or Rock for out-of-bounds cells so the
// edge of the world reads as solid.
func (w *World) TerrainAt(p Point) Terrain {
	if !w.InBounds(p) {
		return Rock
	}
	return w.tiles[w.index(p)].Terrain
}

// SetTerrain overwrites the terrain at p if it is in bounds, keeping the terrain
// counts in step.
func (w *World) SetTerrain(p Point, t Terrain) {
	if !w.InBounds(p) {
		return
	}
	i := w.index(p)
	old := w.tiles[i].Terrain
	if old == t {
		return
	}
	w.terrainCounts[old]--
	w.terrainCounts[t]++
	w.tiles[i].Terrain = t
	w.dirtyChunks[w.chunkIndexOf(p)] = struct{}{}
	w.emit(TileChanged{Pos: p, Old: old, New: t})
}

// Walkable reports whether a colonist can stand at p.
func (w *World) Walkable(p Point) bool {
	return w.InBounds(p) && w.TerrainAt(p).Walkable()
}

// ---- Occupancy ---------------------------------------------------------------

// occupied reports whether any entity stands on p.
func (w *World) occupied(p Point) bool {
	return w.InBounds(p) && w.occ[w.index(p)] != 0
}

// occupiedByOther reports whether an entity other than self stands on p.
func (w *World) occupiedByOther(p Point, self EntityID) bool {
	if !w.InBounds(p) {
		return false
	}
	id := w.occ[w.index(p)]
	return id != 0 && id != self
}

// moveEntity relocates an entity, updating the occupancy index. Callers must
// ensure the destination is in bounds and unoccupied.
func (w *World) moveEntity(e *Entity, to Point) {
	if to.Equal(e.Pos) {
		return
	}
	from := e.Pos
	w.occ[w.index(from)] = 0
	w.occ[w.index(to)] = e.ID
	if oc, nc := w.chunkIndexOf(from), w.chunkIndexOf(to); oc != nc {
		w.removeFromChunkIndex(oc, e.ID)
		w.chunkEntities[nc] = append(w.chunkEntities[nc], e.ID)
	}
	e.Pos = to
}

// ---- Entities ----------------------------------------------------------------

// spawn creates an entity of the given kind at p and registers it, returning the
// new entity so the caller can tune it. The tile must be in bounds and empty.
func (w *World) spawn(kind Kind, p Point) *Entity {
	e := newEntity(w.nextID, kind, p, w.cfg)
	for i := range e.needSince {
		e.needSince[i] = w.tick // needs start rising from now
	}
	w.nextID++
	w.entities[e.ID] = e
	w.occ[w.index(p)] = e.ID
	w.kindCounts[kind]++
	ci := w.chunkIndexOf(p)
	w.chunkEntities[ci] = append(w.chunkEntities[ci], e.ID)
	return e
}

// remove deletes an entity from the world and clears its occupancy.
func (w *World) remove(id EntityID) {
	e := w.entities[id]
	if e == nil {
		return
	}
	w.occ[w.index(e.Pos)] = 0
	w.kindCounts[e.Kind]--
	w.removeFromChunkIndex(w.chunkIndexOf(e.Pos), id)
	delete(w.entities, id)
}

// countKind returns how many living entities of a kind exist.
func (w *World) countKind(kind Kind) int {
	return w.kindCounts[kind]
}

// countTerrain returns how many tiles currently hold the given terrain.
func (w *World) countTerrain(t Terrain) int {
	return w.terrainCounts[t]
}
