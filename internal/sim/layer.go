package sim

// Level is how deep a layer of the world sits. Level 0 is the surface, the
// landing level below it is where colony ships come down (today's whole map),
// and deeper levels count up from there. See docs/z-levels.md.
type Level int

const (
	// SurfaceLevel is the open ground above the rock. Nothing is there yet.
	SurfaceLevel Level = 0
	// LandingLevel is where colony ships land, and the only level that exists
	// so far.
	LandingLevel Level = 1
	// MaxLevel is the deepest Config.DeepestLevel may ask for.
	MaxLevel = 16
)

// Layer is the state of one level of the world: its tiles and everything
// indexed by a tile on it. Anything that only makes sense inside one grid
// lives here; what spans levels (entities, the economy, flow fields, the
// region graph, scratch buffers) stays on World.
//
// There is one layer so far, World.home, the landing level. Every
// "w.todoLayer()." in the code is a place that assumes there is only one level:
// when a second level arrives, removing home turns each of them into a
// compile error to be decided. See docs/z-levels.md.
type Layer struct {
	// Level is where this layer sits.
	Level Level

	// tiles is terrain, composition and discovery, stored in the same 64x64
	// pages as every other per-tile grid (see pagedgrid.go). One page is one
	// worldgen chunk (see worldgen_chunks.go).
	tiles pagedGrid[tileCell]
	// The published tile grid handed to frontends in Snapshots, plus the pages
	// of it that have gone stale since. Frames share every page that did not
	// change, so publishing costs a page table and the handful of pages a tick
	// actually touched instead of a copy of the whole map. With tileSharing
	// set to TilesLive the grid aliases tiles instead and nothing is copied;
	// the dirty list is then only reported, in Snapshot.TileChanges. See
	// tilegrid.go.
	//
	// The grid is not saved (see save.go): under TilesLive its pages alias
	// tiles' own, which the save codec cannot keep. A loaded world publishes
	// its first grid afresh, with All set.
	snapGrid   *TileGrid `save:"-"`
	pageDirty  []bool    // pageDirty[pi]: page pi changed since the last publish
	dirtyPages []int     // the same pages, in mark order, for cheap iteration
	// occ is the occupancy index: occ holds the EntityID standing on a tile, or
	// 0 for empty (IDs start at 1). It turns "who is here?" from an
	// O(entities) scan into an O(1) lookup, and it is the reason at most one
	// entity may occupy a tile. It is paged rather than dense because entities
	// only ever stand on walkable tiles, and 0 — the zero value an unwritten
	// page reads as — already means empty. See pagedgrid.go.
	occ pagedGrid[EntityID]
	// Aggregate counts maintained incrementally so callers never rescan the
	// grid or the entity set to answer "how many of X?".
	terrainCounts [numTerrains]int
	// exploredCount is how many tiles reveal has ever marked Explored, kept
	// incrementally the same way terrainCounts is: reveal only increments it
	// the one time a tile flips (see reveal), so a frontend asking "how much
	// of the map has the colony seen?" (the lore panel) never has to walk the
	// grid to answer it. Kept with fog off too, but only published with it on
	// (see snapshot) -- Snapshot.FogOfWar is what a caller checks first.
	exploredCount int
	// hiddenFloor counts non-Rock tiles that are not yet Explored: the floor of
	// natural caverns the colony has not broken into. Every tile the colony
	// changes is revealed as it changes, so this is exactly the undiscovered
	// cavern floor, and Stats.FloorDug subtracts it. See docs/caverns.md.
	hiddenFloor int
	// goreTotal/corpseTotal are the same idea for tile refuse: the colony's
	// sanitation planning asks "is there anything to clean up?" every planning
	// cycle, which must not mean walking the map. See refuseTotal.
	goreTotal   int
	corpseTotal int
	// refuse indexes the tiles carrying gore or a body. It is sparse for the
	// same reason storageContainers is: it describes the handful of tiles
	// something died on, and storing it per-tile inflated every cell on the
	// map to carry it. Absent means clean; see setRefuse.
	//
	// refuseRev advances on every write, so publishing can hand the previous
	// frame's copy straight back when nothing died or got cleaned up this
	// tick — which is almost every tick. snapRefuseRev is the revision the
	// currently published grid's copy was taken at.
	refuse        map[Point]refuseCell
	refuseRev     uint64
	snapRefuseRev uint64
	// facilityTiles[t] holds every tile currently of terrain t, for the handful
	// of terrain kinds colonists walk to (NutrientPod, Toilet, Bed, Incinerator). Kept in step
	// by SetTerrain so chooseFacility and facilitySeed can visit just those
	// tiles instead of scanning the whole grid — essential on large maps, where
	// a full Width*Height scan dwarfs the tiny number of actual facilities.
	// nil for untracked terrain kinds (Rock, Floor, Wall).
	facilityTiles [numTerrains]map[Point]struct{}
	// carvedAny/carvedMin/carvedMax track the bounding box of every tile that
	// has ever been changed away from Rock. Nothing turns back into Rock in
	// play (a cleared structure becomes Floor), so this box only grows; it
	// is used to cap how far the room planner's search radius needs to
	// grow before it can conclude no site exists, without scanning the whole
	// map. See roomSiteClear: a valid site's side walls must already be
	// Floor or Wall, so no valid site can lie outside this box.
	carvedAny bool
	carvedMin Point
	carvedMax Point
	// Spatial index: entities bucketed by chunk, so neighbor queries scan only
	// nearby chunks. chunkEntities is indexed by chunk (cy*chunkCols + cx).
	chunkEntities [][]EntityID
	// Regions & rooms: floor tiles grouped into per-chunk regions, then into
	// rooms (connected components of the region graph). Maintained incrementally
	// as terrain changes so reachability queries stay cheap. See rooms.go.
	regionOf    pagedGrid[RegionID] // 0 = not floor / no region
	dirtyChunks map[int]struct{}    // chunks whose regions need recompute
	// Reactive plumbing: systems subscribe to world events; the job board is the
	// first consumer, tracking the mineable frontier from TileChanged events.
	board *jobBoard
	// salt holds every tile carrying a deposit of salt (see salt.go). It
	// never overlaps scum, and is never added to after generation.
	salt map[Point]struct{}
	// exposedSalt is the salt a colonist could reach, as exposedScum is for
	// scum, so publishing never walks the whole map's deposits. saltRev
	// advances when it changes and lets publishing reuse the last copy
	// (snapSalt, taken at snapSaltRev); see publishedSalt.
	exposedSalt map[Point]struct{}
	// Cave scum (see scumhouse.go): the sparse patches, the ones a colonist
	// can currently reach (on floor, or on rock that borders walkable floor),
	// which patch each scraper is headed to, and which workshop each cook has
	// claimed. exposedScum is kept in step from TileChanged events, the way
	// the job board keeps the mining frontier, so finding scum to scrape never
	// walks the map.
	scum        map[Point]scumPatch
	exposedScum map[Point]struct{}
	// scumPatches lists every patch in scum, sorted by cmpScumPatch, so
	// growScum can draw a patch at random without the map's order deciding
	// which (see setScum and patchList).
	scumPatches patchList
	// scumThin is every patch below ScumMax: once the level has no room for
	// new patches, the only ones growth can still add a unit to (see
	// growScum).
	scumThin map[Point]struct{}
	// scumThinPages counts scumThin's patches on each tile page (indexed by
	// tiles.pageIndex; nil until the first), so most of growScum's draws are
	// turned away by a slice read rather than a map lookup.
	scumThinPages []int32
	// gen generates chunks: their ore veins and hidden caverns. See
	// worldgen_chunks.go. nil for a world built without generate (tests),
	// where every tile simply starts as Rock.
	gen *worldGen
	// genDone marks the chunks generated so far and genSeen the chunks
	// holding a tile the colony has seen, both by tile page index (one page
	// is one chunk). genChunks lists the generated chunks sorted by row then
	// column, so sampling from them depends on which chunks exist, never on
	// the order they were generated in. See generateChunkAt.
	genDone, genSeen []bool
	genChunks        []chunkKey
	// preview is handed to Snapshots so a frontend with the fog off can see
	// ungenerated chunks. The World never reads it. Not saved: frontends read
	// it from their own goroutines, and it is only a cache; afterLoad builds
	// a fresh one.
	preview *ChunkPreview `save:"-"`
}

// newLayer allocates an all-Rock layer of the given size.
func newLayer(level Level, width, height int) *Layer {
	l := &Layer{
		Level:       level,
		tiles:       newPagedGrid[tileCell](width, height),
		refuse:      make(map[Point]refuseCell),
		occ:         newPagedGrid[EntityID](width, height),
		regionOf:    newPagedGrid[RegionID](width, height),
		dirtyChunks: make(map[int]struct{}),
		scum:        make(map[Point]scumPatch),
		scumThin:    make(map[Point]struct{}),
		salt:        make(map[Point]struct{}),
		exposedSalt: make(map[Point]struct{}),
		exposedScum: make(map[Point]struct{}),
	}
	l.terrainCounts[Rock] = width * height // every tile starts as Rock
	l.pageDirty = make([]bool, len(l.tiles.pages))
	l.chunkEntities = make([][]EntityID, ceilDiv(width, chunkSize)*ceilDiv(height, chunkSize))
	return l
}

// layer returns the layer at level l, or nil when the colony has never
// broken into it (always, so far, for anything but the landing level).
func (w *World) layer(l Level) *Layer {
	if int(l) < 0 || int(l) >= len(w.layers) {
		return nil
	}
	return w.layers[l]
}

// landing is the landing level's layer: where colony ships land and, so far,
// where the colony builds. Code that calls it is landing-level by design
// (pod sites, the construction planner, the colony's silo), not by accident.
func (w *World) landing() *Layer { return w.layers[LandingLevel] }

// lay is the layer p is on. For a level that does not exist (an unset target
// such as Point{-1, -1}, which is on the surface) it returns noLayer, an empty
// layer whose maps read as empty, so a lookup there finds nothing, as it
// always did for a tile nothing was on. Writing to it is a bug: its maps are
// nil and panic, and its counts are shared. Callers that write check InBounds
// first.
func (w *World) lay(p Point) *Layer {
	if l := p.Level; l >= 0 && int(l) < len(w.layers) {
		if layer := w.layers[l]; layer != nil {
			return layer
		}
	}
	return &noLayer
}

// noLayer is what lay returns for a level that does not exist. See lay.
var noLayer Layer

// layerIn returns the layer holding p, or nil when p is off the map or on a
// level that does not exist: InBounds and lay in one check, for the tile
// reads every search makes on every step.
func (w *World) layerIn(p Point) *Layer {
	if uint(p.X) >= uint(w.Width) || uint(p.Y) >= uint(w.Height) || uint(p.Level) >= uint(len(w.layers)) {
		return nil
	}
	return w.layers[p.Level]
}

// levelExists reports whether the colony has broken into level l, so it has
// a layer.
func (w *World) levelExists(l Level) bool {
	return l >= 0 && int(l) < len(w.layers) && w.layers[l] != nil
}

// eachLayer calls fn for every layer that exists, shallowest first: the
// fixed order every loop over levels must use, so map-free iteration stays
// deterministic.
func (w *World) eachLayer(fn func(*Layer)) {
	for _, l := range w.layers {
		if l != nil {
			fn(l)
		}
	}
}

// containerAt is the storage container at p, or nil.
func (w *World) containerAt(p Point) *StorageContainer { return w.storageContainers[p] }

// eachContainer calls fn for every storage container on every level, in map
// order: callers must not let the order decide anything (they sum).
func (w *World) eachContainer(fn func(*StorageContainer)) {
	for _, c := range w.storageContainers {
		fn(c)
	}
}

// eachFacility calls fn for every tile of the tracked facility terrain kind,
// on every level, levels in order and each level's in map order: callers
// break ties themselves (by lessPoint) or only ask whether any exists.
func (w *World) eachFacility(kind Terrain, fn func(Point)) {
	for _, l := range w.layers {
		if l != nil {
			for p := range l.facilityTiles[kind] {
				fn(p)
			}
		}
	}
}

// unclaimedFrontier is how much mineable rock nobody has claimed, on every
// level.
func (w *World) unclaimedFrontier() int {
	n := 0
	w.eachLayer(func(l *Layer) { n += l.board.unclaimedCount() })
	return n
}

// buildsInProgress is how many lone builds of kind are under way, on every
// level.
func (w *World) buildsInProgress(kind Terrain) int {
	n := 0
	w.eachLayer(func(l *Layer) { n += l.board.inProgress(kind) })
	return n
}
