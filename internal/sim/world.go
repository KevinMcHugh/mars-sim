package sim

import "math/rand"

const maxColonistMemories = 64

// remember adds a notable experience, retaining the most recent memories, and
// applies whatever mood effect the LifeEvent carries (see lifeevents.go).
// Every notable thing that happens to or near a colonist — including a
// conversation's outcome, via finishTalk's eventMood — should be built with
// event() or eventMood() and land here, so mood and memory can never drift
// apart, and there is exactly one mechanism for "this happened, and here is
// how it felt": the same way remove() is the one funnel for deaths.
func (w *World) remember(e *Entity, evt LifeEvent) {
	if e == nil || e.Kind != Colonist {
		return
	}
	e.Memories = append(e.Memories, Memory{Tick: w.tick, Text: evt.Text, Kind: evt.Kind})
	if len(e.Memories) > maxColonistMemories {
		e.Memories = e.Memories[len(e.Memories)-maxColonistMemories:]
	}
	w.applyMoodEffects(e, evt)
}

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
	// Bed satisfies the sleep need; a colonist sleeps in the bunk from an
	// adjacent tile, the same way it uses any other facility. Blocks movement.
	Bed

	numTerrains // keep last: the number of terrain kinds
)

func (t Terrain) String() string {
	switch t {
	case Rock:
		return "rock"
	case Floor:
		return "floor"
	case Wall:
		return "wall"
	case NutrientPod:
		return "nutrient pod"
	case Toilet:
		return "toilet"
	case Bed:
		return "bed"
	default:
		return "unknown"
	}
}

// Walkable reports whether a colonist can stand on this terrain. Aliens ignore
// this; they move through anything.
func (t Terrain) Walkable() bool {
	return t == Floor
}

// RockComposition identifies the useful material embedded in a rock tile.
// Composition is separate from Terrain because every variety has the same
// movement and mining behavior; it only changes the material yielded.
type RockComposition uint8

const (
	OrdinaryRock RockComposition = iota
	IronBearingRock
	WaterIceBearingRock
	// UraniumBearingRock yields uranium ore, the one deposit that is dangerous
	// to be around: a colonist that mines near it or carries the ore
	// accumulates a dose, and a long enough dose can mutate them. See
	// mutation.go and docs/mutation.md.
	UraniumBearingRock
)

func (c RockComposition) String() string {
	switch c {
	case OrdinaryRock:
		return "ordinary rock"
	case IronBearingRock:
		return "iron-bearing rock"
	case WaterIceBearingRock:
		return "water ice-bearing rock"
	case UraniumBearingRock:
		return "uranium-bearing rock"
	default:
		return "unknown rock"
	}
}

// Tile is one cell of the world. It is deliberately a struct rather than a bare
// Terrain so we have room to grow (ore, moisture, temperature, ...) without
// touching every call site.
type Tile struct {
	Terrain     Terrain
	Composition RockComposition // meaningful only while Terrain is Rock
	// Gore is a violent death's visible residue on this tile: 0 is clean, and
	// it climbs (capped at maxGore) as more kills happen here. It is purely
	// cosmetic — it never affects Walkable or anything else — and, unlike
	// Terrain, is not reset by SetTerrain, so a mined-out or built-over tile
	// keeps its stains.
	Gore int
}

// maxGore caps a tile's Gore so a well-fought corner cannot climb the count
// forever for no additional visible effect (today's renderer draws one splatter
// glyph for any Gore > 0; the cap keeps room for a future intensity display).
const maxGore = 3

// addGore marks p as the site of a violent death, capping the tile's Gore at
// maxGore. Used by anything that kills something messily: alien bites, gunfire,
// and a colonist's boot. Like SetTerrain, it must mark the tile's page dirty:
// w.tiles is also the published TileGrid's source, and a page only gets
// re-copied into the next Snapshot if something flags it changed (see
// tilegrid.go) — skipping that would let a gore change go on being invisible
// to every future frame until an unrelated terrain edit on the same page
// happened to flush it.
func (w *World) addGore(p Point) {
	if !w.InBounds(p) {
		return
	}
	i := w.index(p)
	if w.tiles[i].Gore < maxGore {
		w.tiles[i].Gore++
		w.markTilePageDirty(i)
	}
}

// World is the mutable game state for a single underground level. It is owned by
// the Engine's goroutine and must not be read or written from any other
// goroutine; frontends observe it through immutable Snapshots instead.
type World struct {
	Width, Height int
	tiles         []Tile // row-major, len == Width*Height

	// The published tile grid handed to frontends in Snapshots, plus the pages
	// of it that have gone stale since. Frames share every page that did not
	// change, so publishing costs a page table and the handful of pages a tick
	// actually touched instead of a copy of the whole map. See tilegrid.go.
	snapGrid   *TileGrid
	pageDirty  []bool // pageDirty[pi]: page pi differs from snapGrid
	dirtyPages []int  // the same pages, in mark order, for cheap iteration

	// occ is a dense occupancy index parallel to tiles: occ[i] is the EntityID
	// standing on that tile, or 0 for empty (IDs start at 1). It turns "who is
	// here?" from an O(entities) scan into an O(1) lookup, and it is the reason
	// at most one entity may occupy a tile.
	occ []EntityID

	// Aggregate counts maintained incrementally so callers never rescan the
	// grid or the entity set to answer "how many of X?".
	terrainCounts [numTerrains]int
	kindCounts    [numKinds]int

	// kindEntities[k] holds the ID of every living entity of kind k. Kept in
	// step by spawn/remove so a global "nearest of this kind, anywhere" search
	// (see nearestOfKindAnywhere) can scan the handful of matching entities
	// directly instead of nearestMatch's chunk-ring expansion, which is only
	// cheap when the answer is nearby — an unbounded search (a cat with no
	// mouse left nearby, say) forces it to visit every chunk on the map to
	// confirm nothing closer exists.
	kindEntities [numKinds]map[EntityID]struct{}

	// facilityTiles[t] holds every tile currently of terrain t, for the handful
	// of terrain kinds that back a need (NutrientPod, Toilet, Bed). Kept in step
	// by SetTerrain so chooseFacility and facilitySeed can visit just those
	// tiles instead of scanning the whole grid — essential on large maps, where
	// a full Width*Height scan dwarfs the tiny number of actual facilities.
	// nil for untracked terrain kinds (Rock, Floor, Wall).
	facilityTiles [numTerrains]map[Point]struct{}

	// carvedAny/carvedMin/carvedMax track the bounding box of every tile that
	// has ever been changed away from Rock. Terrain only ever moves Rock ->
	// Floor -> Wall/facility in play, never back, so this box only grows; it
	// is used to cap how far findRoomSiteAllowingRock's search radius needs to
	// grow before it can conclude no site exists, without scanning the whole
	// map. See roomSiteClear: a valid site's side walls must already be
	// Floor or Wall, so no valid site can lie outside this box.
	carvedAny bool
	carvedMin Point
	carvedMax Point

	// Scratch for chooseFacility's per-call BFS, reused across calls via a
	// generation stamp instead of reallocating (and zeroing) a Width*Height
	// slice every time a colonist needs a facility. See flowField.seen/gen for
	// the same trick.
	facilityDist    []int32
	facilityDistGen []int32
	facilityGen     int32
	facilityQueue   []Point

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

	// Construction projects the colony builds collaboratively (see project.go).
	projects      []*project
	nextProjectID int
	nextPlanTick  int
	// Manual room orders wait here until the current project finishes and a
	// suitable site is available. Keeping them in the world preserves the
	// engine's single-owner rule for simulation state.
	manualFacilityRooms int
	manualDormitories   int
	// buildTiles holds every not-yet-built task tile, rebuilt each tick. Colonists
	// route around these so a crowd never parks on a tile a builder needs clear —
	// otherwise a facility mobbed by its neighbors could never be raised. See
	// rebuildBuildTiles.
	buildTiles map[Point]bool

	// Family tree and affinity, both colonist-only. kin holds every tree node
	// (colonists plus phantom ancestors); affinity[a][b] is a's warmth toward b,
	// raised when they talk. See relationships.go.
	kin                 map[kinID]*kinPerson
	nextKinID           kinID
	kinRevision         uint64
	kinChildrenCache    map[kinID][]kinID
	kinChildrenRevision uint64
	affinity            map[EntityID]map[EntityID]int

	entities map[EntityID]*Entity
	nextID   EntityID

	// graveyard holds the most recent deaths as frozen EntityViews (oldest
	// first), for the roster's "dead" filter — see docs/combat.md. Bounded at
	// cfg.GraveyardSize by remove(), the only place entities die.
	graveyard []EntityView

	tick    int
	rng     *rand.Rand
	prng    *rand.Rand // personality generation, separate so flavor never perturbs the sim
	agePRNG *rand.Rand // age generation, isolated so adding age does not shift personality
	log     *eventLog
	cfg     Config
}

// newWorld allocates an all-Rock world of the given size.
func newWorld(cfg Config, rng *rand.Rand) *World {
	n := cfg.Width * cfg.Height
	w := &World{
		Width:            cfg.Width,
		Height:           cfg.Height,
		tiles:            make([]Tile, n),
		occ:              make([]EntityID, n),
		entities:         make(map[EntityID]*Entity),
		buildTiles:       make(map[Point]bool),
		kin:              make(map[kinID]*kinPerson),
		nextKinID:        1,
		kinRevision:      1,
		kinChildrenCache: make(map[kinID][]kinID),
		affinity:         make(map[EntityID]map[EntityID]int),
		nextID:           1,
		rng:              rng,
		prng:             rand.New(rand.NewSource(cfg.Seed ^ 0x5DEECE66D)),
		agePRNG:          rand.New(rand.NewSource(cfg.Seed ^ 0x6A09E667)),
		log:              newEventLog(cfg.LogSize),
		cfg:              cfg,
	}
	w.terrainCounts[Rock] = n // every tile starts as Rock

	for k := Kind(0); k < numKinds; k++ {
		w.kindEntities[k] = make(map[EntityID]struct{})
	}

	w.facilityDist = make([]int32, n)
	w.facilityDistGen = make([]int32, n)

	w.pageDirty = make([]bool, ceilDiv(n, tilePageLen))

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
			w.facilityTiles[f] = make(map[Point]struct{})
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

// TileAt returns the tile at p. Out-of-bounds cells behave as ordinary rock.
func (w *World) TileAt(p Point) Tile {
	if !w.InBounds(p) {
		return Tile{Terrain: Rock, Composition: OrdinaryRock}
	}
	return w.tiles[w.index(p)]
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
	if w.facilityTiles[old] != nil {
		delete(w.facilityTiles[old], p)
	}
	if w.facilityTiles[t] != nil {
		w.facilityTiles[t][p] = struct{}{}
	}
	if t != Rock {
		if !w.carvedAny {
			w.carvedAny = true
			w.carvedMin, w.carvedMax = p, p
		} else {
			w.carvedMin.X, w.carvedMax.X = min(w.carvedMin.X, p.X), max(w.carvedMax.X, p.X)
			w.carvedMin.Y, w.carvedMax.Y = min(w.carvedMin.Y, p.Y), max(w.carvedMax.Y, p.Y)
		}
	}
	w.tiles[i].Terrain = t
	w.markTilePageDirty(i)
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

// entityAt returns the entity standing on p, or nil when p is out of bounds or
// empty.
func (w *World) entityAt(p Point) *Entity {
	if !w.InBounds(p) {
		return nil
	}
	return w.entities[w.occ[w.index(p)]]
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
		// Stagger starting need levels so a freshly settled colony does not all
		// get hungry on the same tick and stampede the facilities at once.
		if kind == Colonist {
			if seek := w.cfg.Needs[i].SeekAt; seek > 0 {
				e.Needs[i] = w.rng.Intn(seek)
			}
		}
	}
	if kind == Colonist {
		w.assignPersonality(e) // name, attributes, traits + their effective params
		w.assignKin(e)         // family tree node + any tie to an existing colonist
	}
	if kind == Mouse {
		e.sex = w.rollMouseSex() // decides which mice can carry a litter
	}
	w.nextID++
	w.entities[e.ID] = e
	w.occ[w.index(p)] = e.ID
	w.kindCounts[kind]++
	w.kindEntities[kind][e.ID] = struct{}{}
	ci := w.chunkIndexOf(p)
	w.chunkEntities[ci] = append(w.chunkEntities[ci], e.ID)
	return e
}

// remove deletes an entity from the world, clears its occupancy, and — every
// call here is a death — freezes it into the graveyard with cause as a short
// player-facing phrase ("starved", "shot by Zoe Vargas with a shotgun"). See
// docs/combat.md.
func (w *World) remove(id EntityID, cause string) {
	e := w.entities[id]
	if e == nil {
		return
	}
	if w.cfg.GraveyardSize > 0 {
		dead := w.entityView(e, nil, false)
		dead.Dead, dead.DiedTick, dead.Cause = true, w.tick, cause
		w.graveyard = append(w.graveyard, dead)
		if over := len(w.graveyard) - w.cfg.GraveyardSize; over > 0 {
			w.graveyard = w.graveyard[over:]
		}
	}
	w.occ[w.index(e.Pos)] = 0
	w.kindCounts[e.Kind]--
	delete(w.kindEntities[e.Kind], id)
	w.removeFromChunkIndex(w.chunkIndexOf(e.Pos), id)
	if e.kin != 0 {
		if kp := w.kin[e.kin]; kp != nil {
			kp.entity = 0 // keep the node so surviving relatives stay connected
		}
	}
	w.dropAffinity(id)
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
