package sim

import (
	"fmt"
	"math/rand"
)

const maxColonistMemories = 64

// remember is the single ingestion funnel for notable life events. One call
// applies charge/grip appraisal, updates transient stimulus state when
// configured, and records or collapses long-term memory. These products have
// separate lifetimes, but call sites cannot accidentally update only one.
func (w *World) remember(e *Entity, evt LifeEvent) {
	if e == nil || e.Kind != Colonist {
		return
	}
	// Affect is applied per occurrence either way: collapsing is about what the
	// memory log reads like, not about the twelfth dig having stopped counting.
	// It does count for less, because wear reads this log and a collapsed run
	// is one occasion in it -- but that is habituation, not a skipped update.
	// Order matters here: appraisal runs before the memory is recorded, so an
	// event a colonist has never had is appraised at its fresh reading.
	w.applyAffect(e, evt)
	w.addStimulus(e, evt)
	if !w.collapseRepeat(e, evt) {
		e.Memories = append(e.Memories, Memory{
			Tick:     w.tick,
			LastTick: w.tick,
			Count:    1,
			Text:     evt.Text,
			Kind:     evt.Kind,
		})
		if len(e.Memories) > maxColonistMemories {
			e.Memories = e.Memories[len(e.Memories)-maxColonistMemories:]
		}
	}
}

// collapseRepeat folds evt into the colonist's most recent memory, reporting
// whether it did. It collapses only when the kind is a minor, repetitive one
// (a non-empty lifeEventCollapseText entry) and the newest memory is already
// that same kind: a run is *consecutive* occurrences, so anything else the
// colonist did — a meal in the middle of a mining shift — breaks the run and
// the next dig starts a fresh memory. Folding into any older matching memory
// instead would compress a whole life into one line per kind and lose the
// order things happened in, which is most of what the log is for.
//
// The folded memory takes the kind's generic text, keeps Tick at the first
// occurrence, and advances LastTick to this one, so the line can say both how
// many times and over what span.
func (w *World) collapseRepeat(e *Entity, evt LifeEvent) bool {
	if lifeEventCollapseText[evt.Kind] == "" || len(e.Memories) == 0 {
		return false
	}
	last := &e.Memories[len(e.Memories)-1]
	if last.Kind != evt.Kind {
		return false
	}
	last.Text = lifeEventCollapseText[evt.Kind]
	last.LastTick = w.tick
	last.Count++
	return true
}

// Terrain is what fills a single tile. The world is a dense grid of tiles; as
// colonists dig and build, tiles change terrain.
type Terrain uint8

const (
	// Rock is unexcavated Martian regolith. Colonists cannot walk through it,
	// but they can mine it into Floor. Nothing walks through it, aliens included.
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
	// Incinerator burns refuse — viscera scrubbed off the floor and the bodies
	// of the dead — hauled to it by a cleaning colonist. It is a machine, not a
	// need facility: nothing seeks it out to satisfy a drive, so it has no
	// NeedSpec; it is the disposal end of the sanitation loop and the reason a
	// trash room gets built at all. Used from an adjacent tile; blocks movement
	// like any other structure. See docs/sanitation.md.
	Incinerator
	// Storage is a large trunk with six colonist inventories worth of slots.
	// Its contents are sparse world state rather than part of every Tile; see
	// storageContainers and docs/storage.md.
	Storage

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
	case Incinerator:
		return "incinerator"
	case Storage:
		return "storage container"
	default:
		return "unknown"
	}
}

// Walkable reports whether a creature can stand on this terrain. Everyone,
// aliens included, keeps to walkable floor.
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
	ClayBearingRock
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
	case ClayBearingRock:
		return "clay-bearing rock"
	default:
		return "unknown rock"
	}
}

// Tile is one cell of the world, as callers read it. It is deliberately a
// struct rather than a bare Terrain so we have room to grow (ore, moisture,
// temperature, ...) without touching every call site.
//
// It is an assembled view, not the stored form: Gore and Corpses come from the
// sparse refuse index rather than from per-tile storage (see tileCell and
// World.refuse). Reading a Tile is how everything outside this file sees a
// cell, so the split does not reach any of them.
type Tile struct {
	Terrain     Terrain
	Composition RockComposition // meaningful only while Terrain is Rock
	// Explored records that the colony has dug (or built) its way to within
	// one tile of here (or broken into the natural cavern it lies in), so a
	// frontend may show what is on it. It only ever goes from false to true.
	// It is maintained whatever Config.FogOfWar says, because the simulation
	// uses it too: an unexplored Floor tile is an undiscovered natural cavern
	// the colony cannot yet reach (see docs/caverns.md). With fog off,
	// frontends simply read every tile as explored instead (see
	// Snapshot.ExploredAt). See docs/fog-of-war.md.
	//
	Explored bool
	// Gore is a violent death's visible residue on this tile: 0 is clean, and
	// it climbs (capped at maxGore, hence uint8) as more kills happen here. It never affects
	// Walkable or anything else, and digging a tile out does not wash it away —
	// an alien shot dead inside a rock vein leaves a stain that is only
	// reachable once somebody mines through to it. Raising a structure on the
	// tile does clear it, though: see SetTerrain.
	Gore uint8
	// Corpses is how many bodies lie on this tile, left by a death that did not
	// end in something eating the remains (a starvation, a gunned-down alien, a
	// stomped mouse). Like Gore it is tile state rather than an entity: a corpse
	// does not act, and the occupancy index allows one entity per tile, so a
	// body modelled as an entity would wall off the spot where anything died.
	// Colonists haul corpses to an incinerator; see docs/sanitation.md.
	//
	// uint16 rather than uint8: nothing caps the pile the way maxGore caps a
	// stain, and a killing floor nothing ever cleans is a real enough shape for
	// a long game that 255 is not obviously out of reach. addCorpse saturates
	// at maxCorpses rather than wrapping.
	Corpses uint16
}

// tileCell is how a tile is actually stored: the three facts that are true of
// every cell on the map. Its width is load-bearing. This is the one structure
// in the simulation that genuinely is dense — Composition is ore, and worldgen
// threads veins through a fifth of the map, so unlike the navigation grids
// (pagedgrid.go) there is no sparsity here to exploit. What is left is to make
// the record small: every byte is 95 MB on a 10000x10000 map, and twice that in
// practice because the published grid mirrors it (tilegrid.go).
//
// Gore and Corpses are not here. They were, as word-sized ints, which padded
// the record to 24 bytes for what is really three bytes of state — and then two
// of those three bytes described something true of the few hundred tiles
// anything has ever died on. They now live in World.refuse, sparse, the way
// storage container contents already do.
type tileCell struct {
	Terrain     Terrain
	Composition RockComposition // meaningful only while Terrain is Rock
	Explored    bool
}

// refuseCell is what lies on a tile something died on. Absent from the refuse
// index means a clean tile, so the index holds only tiles that are dirty.
type refuseCell struct {
	Gore    uint8
	Corpses uint16
}

// tile returns the assembled view of cell i, with any refuse on it.
func (w *World) tile(i int) Tile {
	c := w.tiles[i]
	r := w.refuse[Point{i % w.Width, i / w.Width}]
	return Tile{
		Terrain:     c.Terrain,
		Composition: c.Composition,
		Explored:    c.Explored,
		Gore:        r.Gore,
		Corpses:     r.Corpses,
	}
}

// maxGore caps a tile's Gore so a well-fought corner cannot climb the count
// forever for no additional visible effect (today's renderer draws one splatter
// glyph for any Gore > 0; the cap keeps room for a future intensity display).
const maxGore = 3

// maxCorpses caps a tile's body count at the width of the field. Nothing in
// play is expected to come near it — a colonist hauls bodies off long before —
// but a cap turns an overflow that would silently empty a tile and desync
// corpseTotal into a body that simply does not stack.
const maxCorpses = ^uint16(0)

// addGore marks p as the site of a violent death, capping the tile's Gore at
// maxGore. Used by anything that kills something messily: alien bites, gunfire,
// and a colonist's boot.
//
// It does not dirty a tile page. Refuse used to live in the tile record and so
// rode the published pages, which meant every one of these had to flag the page
// changed or the stain stayed invisible until an unrelated terrain edit on the
// same page flushed it. It now reaches frontends through the refuse index's own
// revision instead (see publishedRefuse), so a death costs a map write rather
// than a 4096-tile page copy.
func (w *World) addGore(p Point) {
	if !w.InBounds(p) {
		return
	}
	r := w.refuse[p]
	if r.Gore >= maxGore {
		return
	}
	r.Gore++
	w.setRefuse(p, r)
	w.goreTotal++
}

// addCorpse leaves a body on p. It is addGore's counterpart for remains that
// are still recognizably a body rather than a stain, and callers pick: a death
// whose remains are eaten (an alien devouring a colonist, a cat swallowing a
// mouse) leaves only gore, while a starvation, a gunshot, or a stomp leaves a
// body to be hauled away. Like addGore it reaches frontends through the refuse
// index's revision rather than a tile page (see publishedRefuse).
func (w *World) addCorpse(p Point) {
	if !w.InBounds(p) {
		return
	}
	r := w.refuse[p]
	if r.Corpses >= maxCorpses {
		return
	}
	r.Corpses++
	w.setRefuse(p, r)
	w.corpseTotal++
}

// refuseAt reports how many units of refuse — gore stains plus bodies — lie on
// p. It is what a cleaning colonist scrubs up and what the colony's demand for
// a trash room is measured in.
func (w *World) refuseAt(p Point) int {
	if !w.InBounds(p) {
		return 0
	}
	r := w.refuse[p]
	return int(r.Gore) + int(r.Corpses)
}

// goreAt and corpsesAt report one tile's refuse by kind, for the sight checks
// and the gather loop that only care whether there is any.
func (w *World) goreAt(p Point) int    { return int(w.refuse[p].Gore) }
func (w *World) corpsesAt(p Point) int { return int(w.refuse[p].Corpses) }

// refuseTotal is the whole map's outstanding refuse, maintained incrementally
// by the add/take helpers so the planner never rescans the grid to decide
// whether the colony needs somewhere to burn things.
func (w *World) refuseTotal() int { return w.goreTotal + w.corpseTotal }

// takeGore removes one gore stain from p, returning whether there was one.
// Paired with takeCorpse, it is the only way refuse leaves a tile: a colonist
// scrubbing it into its inventory (see cleaning.go).
func (w *World) takeGore(p Point) bool {
	if !w.InBounds(p) {
		return false
	}
	r := w.refuse[p]
	if r.Gore == 0 {
		return false
	}
	r.Gore--
	w.setRefuse(p, r)
	w.goreTotal--
	return true
}

// setRefuse stores a tile's refuse, dropping the entry entirely once the tile
// is clean again. Keeping the index to dirty tiles only is the whole point of
// it being sparse: a colony that cleans up after itself gives the memory back.
func (w *World) setRefuse(p Point, r refuseCell) {
	w.refuseRev++
	if r == (refuseCell{}) {
		delete(w.refuse, p)
		return
	}
	w.refuse[p] = r
}

// clearRefuse discards everything lying on a tile, keeping the running totals
// in step. Callers mark the page dirty themselves.
func (w *World) clearRefuse(p Point) {
	r := w.refuse[p]
	if r == (refuseCell{}) {
		return
	}
	w.goreTotal -= int(r.Gore)
	w.corpseTotal -= int(r.Corpses)
	w.setRefuse(p, refuseCell{})
}

// takeCorpse removes one body from p, returning whether there was one.
func (w *World) takeCorpse(p Point) bool {
	if !w.InBounds(p) {
		return false
	}
	r := w.refuse[p]
	if r.Corpses == 0 {
		return false
	}
	r.Corpses--
	w.setRefuse(p, r)
	w.corpseTotal--
	return true
}

// World is the mutable game state for a single underground level. It is owned by
// the Engine's goroutine and must not be read or written from any other
// goroutine; frontends observe it through immutable Snapshots instead.
type World struct {
	Width, Height int
	tiles         []tileCell // row-major, len == Width*Height

	// The published tile grid handed to frontends in Snapshots, plus the pages
	// of it that have gone stale since. Frames share every page that did not
	// change, so publishing costs a page table and the handful of pages a tick
	// actually touched instead of a copy of the whole map. See tilegrid.go.
	snapGrid   *TileGrid
	pageDirty  []bool // pageDirty[pi]: page pi differs from snapGrid
	dirtyPages []int  // the same pages, in mark order, for cheap iteration

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
	kindCounts    [numKinds]int
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
	// caveStack is revealAround's scratch for flooding the fog off a natural
	// cavern the colony has just broken into.
	caveStack []Point
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

	// kindEntities[k] holds the ID of every living entity of kind k. Kept in
	// step by spawn/remove so a global "nearest of this kind, anywhere" search
	// (see nearestOfKindAnywhere) can scan the handful of matching entities
	// directly instead of nearestMatch's chunk-ring expansion, which is only
	// cheap when the answer is nearby — an unbounded search (a cat with no
	// mouse left nearby, say) forces it to visit every chunk on the map to
	// confirm nothing closer exists.
	kindEntities [numKinds]map[EntityID]struct{}

	// facilityTiles[t] holds every tile currently of terrain t, for the handful
	// of terrain kinds colonists walk to (NutrientPod, Toilet, Bed, Incinerator). Kept in step
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

	// Scratch for chooseFacility's searches (facilitychoice.go), reused across
	// calls via a generation stamp instead of reallocating (and zeroing) a
	// Width*Height slice every time a colonist needs a facility. See
	// flowField.gen for the same trick.
	facilityCells pagedGrid[flowCell]
	facilityGen   int32
	facilityQueue []Point
	facilityFound []foundFacility
	// facilityCommitted is committedUsers' count for the current
	// chooseFacility call (nil until first needed), kept in
	// facilityCommittedBuf so the map is reused rather than reallocated.
	facilityCommitted    map[Point]int
	facilityCommittedBuf map[Point]int

	// Transit scratch for followField's look-through-a-crowd search, shared by
	// every flow field. It used to be three buffers per field, six fields deep,
	// for something that lives entirely inside one followField call on the
	// engine goroutine — a third of all flow-field memory for scratch no two
	// fields could ever want at the same time.
	transitSeen pagedGrid[int32]
	transitGen  int32
	transitQ    []int32

	// Spatial index: entities bucketed by chunk, so neighbor queries scan only
	// nearby chunks. chunkEntities is indexed by chunk (cy*chunkCols + cx).
	chunkCols, chunkRows int
	chunkEntities        [][]EntityID

	// Regions & rooms: floor tiles grouped into per-chunk regions, then into
	// rooms (connected components of the region graph). Maintained incrementally
	// as terrain changes so reachability queries stay cheap. See rooms.go.
	regionOf pagedGrid[RegionID] // 0 = not floor / no region
	regions  map[RegionID]*region
	// regionLinkScratch backs sortedLinks, so the abstract search can expand
	// neighbours in a stable order without allocating per node.
	regionLinkScratch []RegionID
	nextRegion        RegionID
	dirtyChunks       map[int]struct{} // chunks whose regions need recompute
	roomCount         int
	// mainRoom is the discovered room with the most floor tiles, recomputed by
	// relabelRooms — the colony's main connected network, against which every
	// colonist's own room is checked each tick. See updateDisconnected.
	mainRoom RoomID

	// Reusable scratch buffers for refreshSpatial (avoid per-call allocation).
	floodStack  []Point
	roomScratch []RegionID
	roomStack   []RegionID
	freshRooms  []roomInfo
	// Incremental relabeling (see relabelRooms): regions whose component may
	// have changed since the last refresh, the rooms those changes may have
	// invalidated, and the floor-tile size of every current room (and of the
	// discovered ones alone, which mainRoom is chosen from).
	relabelSeeds    []RegionID
	staleRooms      map[RoomID]struct{}
	rooms           map[RoomID]int
	discoveredRooms map[RoomID]int
	// relabelPass numbers relabelRooms calls; a region whose visitPass equals
	// it has been visited this pass. Cheaper than a fresh visited map per
	// call once undiscovered caverns put thousands of regions on a big map.
	relabelPass uint32

	// Reactive plumbing: systems subscribe to world events; the job board is the
	// first consumer, tracking the mineable frontier from TileChanged events.
	subscribers []func(WorldEvent)
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
	manualTrashRooms    int
	manualStorageRooms  int
	// storageContainers holds mutable contents only for tiles whose terrain is
	// Storage. Keeping it sparse avoids inflating every tile in a large map.
	storageContainers map[Point]*StorageContainer
	// fixtures holds the ownership record of every placed fixture tile (pods,
	// toilets, beds, incinerators, storage), kept in step by SetTerrain.
	// restrictedFixtures counts, per terrain, the ones that are not communal:
	// while it is zero for a kind, ownership cannot change how colonists use
	// that kind, and the access checks skip their extra work. fixtureRev
	// advances on any change so snapshots can reuse the last published list
	// (snapFixtures, taken at snapFixtureRev). See property.go.
	fixtures map[Point]*Fixture
	// podRingHint is the search ring the last crash pod landed on, so the
	// next search starts near there instead of rescanning the packed middle.
	// See findPodSite.
	podRingHint        int
	restrictedFixtures [numTerrains]int
	fixtureRev         uint64
	snapFixtureRev     uint64
	snapFixtures       []FixtureView
	// buildTiles holds every not-yet-built task tile, rebuilt each tick. Colonists
	// route around these so a crowd never parks on a tile a builder needs clear —
	// otherwise a facility mobbed by its neighbors could never be raised. See
	// rebuildBuildTiles.
	buildTiles map[Point]bool
	// doorTiles holds the single exterior tile in front of every room's
	// doorway ever designated, forever — even after the room finishes or a
	// later room's wall would otherwise cover it. roomSiteClear checks it
	// alongside a candidate site's own requirements so a new room can never
	// wall over an existing room's sole way out. Rooms are never demolished
	// or un-designated, so entries are only ever added. See designateRoom.
	doorTiles map[Point]bool

	// directorQueue is cfg.Schedules resolved to concrete (tick, occurrence)
	// firings, sorted ascending; directorNext is how far runDirector has
	// worked through it. Resolved once in newWorld, off w.rng. See director.go.
	directorQueue []scheduledEvent
	directorNext  int

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

	// The colony's money. treasury is the community's balance; moneyIssued is
	// every dollar ever minted (the founding grant plus each arrival's purse)
	// and moneyFrozen every dollar locked in a dead colonist's wallet, so the
	// supply can be audited: treasury + living wallets + moneyFrozen ==
	// moneyIssued. See money.go and docs/money.md.
	treasury    Money
	moneyIssued Money
	moneyFrozen Money

	// colonistNames indexes every living colonist's full name, so generation can
	// check a name is free in one lookup instead of scanning the roster. See
	// uniquifyName in personality.go.
	colonistNames map[string]EntityID

	// graveyard holds the most recent deaths as frozen EntityViews (oldest
	// first), for the roster's "dead" filter — see docs/combat.md. Bounded at
	// cfg.GraveyardSize by remove(), the only place entities die.
	graveyard []EntityView

	// deceasedColonists permanently archives every colonist who has ever
	// died, keyed by EntityID so family relations, name lookups, and a
	// colonist's frozen inventory all keep resolving indefinitely instead of
	// falling out of the bounded graveyard window. It is never trimmed:
	// unlike graveyard (which also holds mice/cats/aliens and must survive a
	// kill flood), the size of this map is bounded by how many colonists
	// ever existed, not by combat volume. See docs/combat.md.
	deceasedColonists map[EntityID]EntityView
	// publishedDeceased is the copy of deceasedColonists that snapshots
	// share, or nil when a death has made it stale. Snapshots used to copy
	// the archive every frame; it only changes when a colonist dies, and the
	// copy grows with every death, so a long game paid more per tick for the
	// same data. See publishedDeceasedColonists.
	publishedDeceased map[EntityID]EntityView

	tick int
	// alwaysArbitrate is a test-only differential oracle. Production leaves it
	// false; keeping the switch on World avoids a user-facing tuning knob for a
	// correctness mode.
	alwaysArbitrate bool
	rng             *rand.Rand
	prng            *rand.Rand // personality generation, separate so flavor never perturbs the sim
	agePRNG         *rand.Rand // age generation, isolated so adding age does not shift personality
	log             *eventLog
	cfg             Config

	// alienSpecies is this world's roster of rolled alien species -- each
	// one's build, colloquial name, temperament, and the combat stats (bite
	// damage/rest, slowness) every Alien entity assigned to it (see
	// Entity.Species, set in spawn) reads instead of a flat Config value.
	// Rolled once in newWorld, off its own seed-derived stream (neither rng
	// nor prng). See lore.go.
	alienSpecies []AlienSpecies

	// unfoundCaverns holds the center of every natural cavern not yet
	// discovered; a breach that reveals one rolls for its alien nest on
	// nestRNG, a seed-derived stream of its own. See rollNests and
	// docs/caverns.md.
	unfoundCaverns map[Point]struct{}
	nestRNG        *rand.Rand
	// nestCenters is revealAround's scratch: cavern centers found this flood.
	nestCenters []Point
}

// newWorld allocates an all-Rock world of the given size.
func newWorld(cfg Config, rng *rand.Rand) *World {
	n := cfg.Width * cfg.Height
	w := &World{
		Width:             cfg.Width,
		Height:            cfg.Height,
		tiles:             make([]tileCell, n),
		refuse:            make(map[Point]refuseCell),
		occ:               newPagedGrid[EntityID](cfg.Width, cfg.Height),
		entities:          make(map[EntityID]*Entity),
		colonistNames:     make(map[string]EntityID),
		buildTiles:        make(map[Point]bool),
		doorTiles:         make(map[Point]bool),
		storageContainers: make(map[Point]*StorageContainer),
		fixtures:          make(map[Point]*Fixture),
		kin:               make(map[kinID]*kinPerson),
		nextKinID:         1,
		kinRevision:       1,
		kinChildrenCache:  make(map[kinID][]kinID),
		affinity:          make(map[EntityID]map[EntityID]int),
		deceasedColonists: make(map[EntityID]EntityView),
		nextID:            1,
		rng:               rng,
		prng:              rand.New(rand.NewSource(cfg.Seed ^ 0x5DEECE66D)),
		agePRNG:           rand.New(rand.NewSource(cfg.Seed ^ 0x6A09E667)),
		log:               newEventLog(cfg.LogSize),
		cfg:               cfg,
	}
	w.alienSpecies = rollAlienSpeciesRoster(rand.New(rand.NewSource(cfg.Seed^alienLoreSeed)), cfg)
	w.terrainCounts[Rock] = n // every tile starts as Rock

	for k := Kind(0); k < numKinds; k++ {
		w.kindEntities[k] = make(map[EntityID]struct{})
	}

	w.facilityCells = newPagedGrid[flowCell](cfg.Width, cfg.Height)
	w.transitSeen = newPagedGrid[int32](cfg.Width, cfg.Height)

	w.pageDirty = make([]bool, ceilDiv(n, tilePageLen))

	w.chunkCols = ceilDiv(cfg.Width, chunkSize)
	w.chunkRows = ceilDiv(cfg.Height, chunkSize)
	w.chunkEntities = make([][]EntityID, w.chunkCols*w.chunkRows)

	w.regionOf = newPagedGrid[RegionID](cfg.Width, cfg.Height)
	w.regions = make(map[RegionID]*region)
	w.staleRooms = make(map[RoomID]struct{})
	w.rooms = make(map[RoomID]int)
	w.discoveredRooms = make(map[RoomID]int)
	w.nextRegion = 1
	w.dirtyChunks = make(map[int]struct{})

	w.board = newJobBoard(w)
	w.subscribe(func(e WorldEvent) {
		if tc, ok := e.(TileChanged); ok {
			w.board.onTileChanged(tc)
		}
	})
	w.pf = newPathfinder(w)

	for i := 0; i < int(numNeeds); i++ {
		if f := cfg.Needs[i].Facility; f != Rock && w.fields[f] == nil {
			w.trackFacility(f)
		}
	}
	// The incinerator backs no need, so the loop above never reaches it, but a
	// hauler still has to find and route to one — it needs the same tracked
	// tile set and shared field as any facility. See docs/sanitation.md.
	w.trackFacility(Incinerator)
	// Storage does not satisfy a biological need, but full colonists still seek
	// it through the same position index and pathing machinery.
	w.trackFacility(Storage)
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
	}, func(p Point) bool {
		if !w.Walkable(p) {
			return false
		}
		for _, d := range neighbors8 {
			if w.board.isUnclaimedFrontier(p.Add(d.X, d.Y)) {
				return true
			}
		}
		return false
	})
	w.subscribe(func(e WorldEvent) {
		if tc, ok := e.(TileChanged); ok {
			// The tile's walkability may have changed, and for a facility
			// field so may the goal status of every tile around it. The
			// frontier field hears about its goals from the job board, which
			// touches it whenever frontier membership or a claim changes.
			for _, f := range w.fields {
				if f != nil {
					f.touch(tc.Pos)
				}
			}
			w.frontier.touch(tc.Pos)
		}
	})
	w.directorQueue = resolveSchedules(cfg.Schedules, w.rng)
	w.mint(Community, Money(cfg.FoundingGrant))
	return w
}

// trackFacility gives a terrain kind the tile set and shared flow field that
// make it a place colonists can find and walk to: facilityTiles[kind] is kept
// in step by SetTerrain, and fields[kind] routes seekers to the nearest one.
func (w *World) trackFacility(kind Terrain) {
	if w.fields[kind] != nil {
		return
	}
	w.facilityTiles[kind] = make(map[Point]struct{})
	w.fields[kind] = newFlowField(w, facilitySeed(w, kind), facilityGoal(w, kind))
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
	return w.tile(w.index(p))
}

// SetTerrain overwrites the terrain at p if it is in bounds, keeping the terrain
// counts in step.
func (w *World) SetTerrain(p Point, t Terrain) {
	w.setTerrain(p, t, true)
}

// carveHidden turns rock at p into floor the colony has not discovered: a
// natural cavern tile. It is SetTerrain in every respect — counts, chunks,
// TileChanged — except that it lifts no fog and does not grow the carved box,
// so the cavern stays unknown (and out of every colony-facing system) until a
// dig breaks into it and revealAround floods it open. Worldgen only.
func (w *World) carveHidden(p Point) {
	if !w.InBounds(p) || w.tiles[w.index(p)].Explored || w.TerrainAt(p) != Rock {
		return
	}
	w.hiddenFloor++
	w.setTerrain(p, Floor, false)
}

func (w *World) setTerrain(p Point, t Terrain, discover bool) {
	if !w.InBounds(p) {
		return
	}
	i := w.index(p)
	old := w.tiles[i].Terrain
	if old == t {
		return
	}
	if discover {
		// Changing a tile's terrain means somebody was standing next to it, so
		// it and its neighbors are no longer unknown. This is the only place
		// fog of war is lifted, for the same reason SetTerrain is the only
		// terrain writer: every other system already funnels through here.
		// Reveal before writing, so the tile itself is revealed as whatever it
		// was: that keeps "unexplored and not Rock" meaning exactly
		// "undiscovered cavern floor". See docs/fog-of-war.md.
		w.revealAround(p)
	}
	w.terrainCounts[old]--
	w.terrainCounts[t]++
	if w.facilityTiles[old] != nil {
		delete(w.facilityTiles[old], p)
	}
	if w.facilityTiles[t] != nil {
		w.facilityTiles[t][p] = struct{}{}
	}
	if old == Storage {
		delete(w.storageContainers, p)
	}
	if t == Storage {
		w.storageContainers[p] = &StorageContainer{Pos: p}
	}
	if isFixtureTerrain(old) {
		w.dropFixture(p)
	}
	if isFixtureTerrain(t) {
		w.placeFixture(p, t)
	}
	if t != Rock && discover {
		w.growCarvedBox(p)
	}
	// Building on a tile scrapes or seals whatever was lying on it. That is
	// partly flavor and partly an invariant the cleaning system depends on:
	// refuse under a wall could never be hauled away, so leaving it there
	// would mean a colony that reports a mess no colonist can ever clean up
	// (see cleaning.go). Digging (Rock/Floor) deliberately does not clear it —
	// mining through to an old kill should expose the stain, not erase it.
	if t != Floor && t != Rock {
		w.clearRefuse(p)
	}
	w.tiles[i].Terrain = t
	w.markTilePageDirty(i)
	w.dirtyChunks[w.chunkIndexOf(p)] = struct{}{}
	w.emit(TileChanged{Pos: p, Old: old, New: t})
}

// growCarvedBox extends the carved bounding box (see carvedMin) to cover p.
func (w *World) growCarvedBox(p Point) {
	if !w.carvedAny {
		w.carvedAny = true
		w.carvedMin, w.carvedMax = p, p
		return
	}
	w.carvedMin.X, w.carvedMax.X = min(w.carvedMin.X, p.X), max(w.carvedMax.X, p.X)
	w.carvedMin.Y, w.carvedMax.Y = min(w.carvedMin.Y, p.Y), max(w.carvedMax.Y, p.Y)
}

// revealAround marks p and its eight neighbors explored, lifting the fog over
// one tile's worth of rock around a change. Terrain-derived systems do not care
// who has seen what, so this emits no TileChanged — it only dirties the
// published pages so the next Snapshot carries the reveal.
//
// The one exception is breaking into a natural cavern. If the ring revealed
// here holds undiscovered cavern floor, the colony has just holed through into
// it: the fog lifts off the whole connected cave system (and the rock rim
// around it) at once, and discoverCavernTile hands each newly found floor tile
// to the systems that ignore undiscovered floor. See docs/caverns.md.
func (w *World) revealAround(p Point) {
	w.caveStack = w.caveStack[:0]
	w.revealRing(p)
	found := 0
	for len(w.caveStack) > 0 {
		q := w.caveStack[len(w.caveStack)-1]
		w.caveStack = w.caveStack[:len(w.caveStack)-1]
		found++
		w.discoverCavernTile(q)
		w.revealRing(q)
		if _, ok := w.unfoundCaverns[q]; ok {
			w.nestCenters = append(w.nestCenters, q)
		}
	}
	if found > 0 {
		w.log.add(fmt.Sprintf("The colony breaks through into a natural cavern (%d tiles of open floor).", found))
		// Nests are rolled only now, once the whole system is revealed, so
		// their aliens land on discovered floor, awake.
		w.rollNests(w.nestCenters)
		w.nestCenters = w.nestCenters[:0]
	}
}

// revealRing reveals p and its eight neighbors, queuing any undiscovered
// cavern floor among them on caveStack.
func (w *World) revealRing(p Point) {
	w.reveal(p)
	for _, d := range neighbors8 {
		w.reveal(p.Add(d.X, d.Y))
	}
}

// reveal marks one in-bounds tile explored, for good. A non-Rock tile that was
// not yet explored can only be undiscovered cavern floor (see carveHidden), so
// it goes on caveStack for revealAround to flood onward from.
func (w *World) reveal(p Point) {
	if !w.InBounds(p) {
		return
	}
	i := w.index(p)
	if w.tiles[i].Explored {
		return
	}
	w.tiles[i].Explored = true
	w.exploredCount++
	w.markTilePageDirty(i)
	if w.tiles[i].Terrain != Rock {
		w.hiddenFloor--
		w.caveStack = append(w.caveStack, p)
	}
}

// discoverCavernTile brings one just-discovered cavern floor tile into the
// colony's world: the rock around it becomes mineable frontier, it counts
// toward the carved box room siting searches, and its chunk is re-flooded so
// its region is marked discovered (see relabelRooms' mainRoom).
func (w *World) discoverCavernTile(p Point) {
	w.growCarvedBox(p)
	w.dirtyChunks[w.chunkIndexOf(p)] = struct{}{}
	if w.board != nil {
		w.board.refreshFrontierCell(p)
		for _, d := range neighbors8 {
			w.board.refreshFrontierCell(p.Add(d.X, d.Y))
		}
	}
	if w.frontier != nil {
		w.frontier.touch(p)
	}
}

// discovered reports whether the colony knows about p, regardless of whether
// fog of war is shown. Colony-facing systems use it to ignore the floor of
// natural caverns nobody has broken into yet.
func (w *World) discovered(p Point) bool {
	return w.InBounds(p) && w.tiles[w.index(p)].Explored
}

// Explored reports whether the colony has seen p, as a frontend should show it:
// with fog of war off every in-bounds tile counts as explored. Simulation code
// that means "has the colony found this?" wants discovered instead.
func (w *World) Explored(p Point) bool {
	if !w.InBounds(p) {
		return false
	}
	if !w.cfg.FogOfWar {
		return true
	}
	return w.tiles[w.index(p)].Explored
}

// Walkable reports whether a colonist can stand at p.
func (w *World) Walkable(p Point) bool {
	return w.InBounds(p) && w.TerrainAt(p).Walkable()
}

// ---- Occupancy ---------------------------------------------------------------

// occupied reports whether any entity stands on p.
func (w *World) occupied(p Point) bool {
	return w.InBounds(p) && w.occ.at(p.X, p.Y) != 0
}

// occupiedByOther reports whether an entity other than self stands on p.
func (w *World) occupiedByOther(p Point, self EntityID) bool {
	if !w.InBounds(p) {
		return false
	}
	id := w.occ.at(p.X, p.Y)
	return id != 0 && id != self
}

// entityAt returns the entity standing on p, or nil when p is out of bounds or
// empty.
func (w *World) entityAt(p Point) *Entity {
	if !w.InBounds(p) {
		return nil
	}
	return w.entities[w.occ.at(p.X, p.Y)]
}

// moveEntity relocates an entity, updating the occupancy index. Callers must
// ensure the destination is in bounds and unoccupied.
func (w *World) moveEntity(e *Entity, to Point) {
	if to.Equal(e.Pos) {
		return
	}
	from := e.Pos
	w.occ.set(from.X, from.Y, 0)
	w.occ.set(to.X, to.Y, e.ID)
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
	species := 0
	if kind == Alien && len(w.alienSpecies) > 0 {
		// Which species this individual belongs to is an ordinary gameplay
		// draw like where a colonist lands, not part of generating the
		// species roster itself -- see lore.go.
		species = w.rng.Intn(len(w.alienSpecies))
	}
	return w.spawnAs(kind, p, species)
}

// spawnAs is spawn with the alien species already chosen (ignored for every
// other kind), for a caller that must not draw it from the simulation stream:
// an alien nest, whose members share one species rolled on the nest stream
// (see spawnNest).
func (w *World) spawnAs(kind Kind, p Point, species int) *Entity {
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
		if w.assignKin(e) {    // family tree node + any tie to an existing colonist
			w.inheritFamily(e) // the surname, looks, and warmth that come with it
		}
		// Personality resolves the effective rise rates, so initialize phases and
		// their next-boundary ticks only after that resolution is complete.
		for n := NeedKind(0); n < numNeeds; n++ {
			w.syncNeedPhase(e, n)
		}
	}
	if kind == Mouse {
		e.sex = w.rollMouseSex() // decides which mice can carry a litter
	}
	if kind == Alien {
		e.Species = species
	}
	w.nextID++
	w.entities[e.ID] = e
	w.occ.set(p.X, p.Y, e.ID)
	w.kindCounts[kind]++
	w.kindEntities[kind][e.ID] = struct{}{}
	ci := w.chunkIndexOf(p)
	w.chunkEntities[ci] = append(w.chunkEntities[ci], e.ID)
	if kind == Colonist {
		// Every colonist arrives with a purse. Minted only now, once the
		// colonist is registered, because mint pays into a living wallet.
		w.mint(ColonistOwner(e.ID), Money(w.cfg.CrashPodPurse))
	}
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
	if e.Kind == Colonist {
		w.freezeWallet(e)
		// Computed with full=true, and before any of the bookkeeping below
		// runs, so Relations/Affinities are captured as they stood at the
		// moment of death rather than left empty. The kin node keeps
		// pointing at id (see the kin comment below), so this colonist's own
		// family ties resolve here forever, not just for one frozen frame.
		dead := w.entityView(e, w.cachedKinChildren(), true)
		dead.Dead, dead.DiedTick, dead.Cause = true, w.tick, cause
		w.deceasedColonists[id] = dead
		w.publishedDeceased = nil // the next snapshot publishes a fresh copy
	}
	w.occ.set(e.Pos.X, e.Pos.Y, 0)
	w.kindCounts[e.Kind]--
	delete(w.kindEntities[e.Kind], id)
	w.removeFromChunkIndex(w.chunkIndexOf(e.Pos), id)
	// The kin node (if any) is left as-is: its entity field keeps pointing at
	// id so surviving relatives' family trees still name this colonist and
	// so descendants stay connected through them. relativesOf resolves
	// aliveness via w.entities/w.deceasedColonists, not by the node itself
	// going blank.
	w.dropAffinity(id)
	w.releaseName(e)
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
