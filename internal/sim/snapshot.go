package sim

import (
	"math"
	"slices"
	"sort"
)

// EntityView is a read-only copy of an entity for a single frame. Frontends
// receive these instead of *Entity so they can never touch live game state.
// A living entity comes from Snapshot.Entities; a dead one (Dead == true)
// comes from Snapshot.Graveyard (bounded, any kind) or, for a colonist,
// Snapshot.Deceased (permanent, by ID) instead — a frozen record from the
// moment it died, not a still-simulated thing occupying a tile. See
// docs/combat.md.
type EntityView struct {
	ID        EntityID
	Kind      Kind
	Pos       Point
	HP        int
	MaxHP     int
	State     State
	Focus     FocusKind
	Drives    [numDrives]int
	Profile   *Profile  // colonists only; a deep copy, safe to read
	Inventory Inventory // colonists only; copied by value
	// Wallet is the colonist's dollars (colonists only). On a Deceased record
	// it is the money frozen at death. See docs/money.md.
	Wallet Money

	// Parts and MaxParts are per-body-part current/max HP (Colonist and Alien
	// only; see Entity.hasParts and docs/combat.md). A zero MaxParts entry
	// means this entity does not have that part at all — every mutant part on
	// anyone who has not grown one (see docs/mutation.md) — so a renderer must
	// skip those rather than drawing a 0/0 gauge.
	Parts    [numBodyParts]int
	MaxParts [numBodyParts]int

	// AlienSpecies is the rolled species this entity belongs to (Alien only;
	// see Entity.Species and World.alienSpeciesFor). Zero-valued for every
	// other kind. See docs/lore.md.
	AlienSpecies AlienSpecies
	// AlienForm is the alien's stage of life or caste ("grub", "queen"), empty
	// for a single-form species or a plain adult. See docs/alien-lifecycles.md.
	AlienForm string
	// Keeper is the colonist a pet (a chicken or a cat) came down with in its
	// ship, 0 for a stray or anything that is not a pet. See
	// docs/chickens.md.
	Keeper EntityID

	// Relations are the colonist's familial ties to other colonists, derived from
	// the family tree; Affinities are its tracked warmth toward colonists it has
	// talked with, strongest first. Both are colonists only, and computed only
	// with entityView's full parameter — set for a still-living colonist and
	// for a Snapshot.Deceased record (captured once, at the moment of death),
	// but left empty on a bounded Snapshot.Graveyard record. See
	// relationships.go.
	Relations  []Relation
	Affinities []Affinity
	Charge     int    // affect activation in [-MoodMax, MoodMax] (colonists only)
	Grip       int    // affect control in [-MoodMax, MoodMax] (colonists only)
	Valence    int    // how life has been going, in [-MoodMax, MoodMax] (colonists only)
	MoodLabel  string // the attractor's name, read through valence (colonists only)
	Memories   []Memory

	// Skills is every skill the colonist has a rank in, in SkillKind order;
	// Profession is the skill it's known for (SkillNone: none yet) and
	// ProfessionLabel its title in it, like "journeyman smith". Colonists
	// only. See docs/skills.md.
	Skills          []SkillView
	Profession      SkillKind
	ProfessionLabel string
	// Backstory is a colonist's one-line past, "Worked as a drill operator
	// for MarsCorp." ("" for none). Flavor only; see docs/arms-makers.md.
	Backstory string

	// Dead, DiedTick, and Cause are set only on a Snapshot.Graveyard or
	// Snapshot.Deceased entry: it died at DiedTick (from Cause, a short
	// player-facing phrase like "shot by Zoe Vargas with a MarsCorp M-117 shotgun"), and
	// every other field is frozen from that moment — Pos is where it died,
	// not where anything is now.
	Dead     bool
	DiedTick int
	Cause    string
}

// TaskView is a read-only copy of one construction task for display: a single
// tile to be built, and who (if anyone) is currently building it.
type TaskView struct {
	Pos     Point
	Terrain Terrain // desired terrain for Pos
	Phase   int     // lower phases in the project must finish first
	Done    bool
	Owner   EntityID // colonist currently building it; 0 if unclaimed
}

// ProjectView is a read-only copy of a queued construction project: the tasks
// the colony has planned, and when it queued them, for the job board.
type ProjectView struct {
	ID         int
	Name       string
	QueuedTick int // tick the colony designated this project
	Phase      int // earliest phase with unfinished work
	Tasks      []TaskView
}

// StorageView is an immutable copy of one placed container and its contents.
type StorageView struct {
	Pos       Point
	Terrain   Terrain // Storage (a chest, locker, or pantry) or Scumhouse
	Pantry    bool    // a kitchen's pantry: the chest its scumhouse cooks into
	Inventory StorageInventory
	// Ledger is whose the contents are, sorted by owner then item. A copy:
	// safe to read. See docs/property.md.
	Ledger []LedgerLine
}

// TasksDone counts tasks already built.
func (p ProjectView) TasksDone() int {
	n := 0
	for _, t := range p.Tasks {
		if t.Done {
			n++
		}
	}
	return n
}

// TasksRemaining counts tasks not yet built — at least this many more build
// actions are needed to finish the project.
func (p ProjectView) TasksRemaining() int {
	return len(p.Tasks) - p.TasksDone()
}

// Assignees lists, in task order, the distinct colonists currently building a
// task in this project.
func (p ProjectView) Assignees() []EntityID {
	seen := make(map[EntityID]bool, len(p.Tasks))
	var out []EntityID
	for _, t := range p.Tasks {
		if t.Owner != 0 && !seen[t.Owner] {
			seen[t.Owner] = true
			out = append(out, t.Owner)
		}
	}
	return out
}

// EconomyView is the colony's money supply at a glance, for the market tab.
// Issued always equals Circulating + Frozen + Escrowed + Exported; a frontend
// can show them side by side without re-deriving any of them. See
// docs/money.md.
type EconomyView struct {
	Treasury    Money // the community's balance
	Circulating Money // treasury plus every living colonist's wallet
	Frozen      Money // locked in dead colonists' wallets
	Escrowed    Money // held by open bids until they fill or are cancelled
	Exported    Money // paid off-world: the recruiter's fees and recruits' passage
	Issued      Money // every dollar ever minted: Circulating + Frozen + Escrowed + Exported

	// The order book (see docs/market.md): every open order oldest first,
	// every book that has ever had an order by depot then item, and the
	// most recent trades, oldest first.
	Orders []OrderView
	Books  []BookView
	Trades []Trade
	// WorkOrders is every open work order, oldest first (see
	// docs/labor.md).
	WorkOrders []WorkOrderView
	// DigWage is what one tile of an excavation order pays, and DigMax the most
	// tiles one order may cover (see docs/excavation.md).
	DigWage Money
	DigMax  int
	// Suspended is every standing order a player has suspended, bids first,
	// then in item order (see docs/colony-orders.md).
	Suspended []SuspendedView
	// Wide is every colony-wide order a player has set, bids first, then in
	// item order (see docs/colony-orders.md).
	Wide []WideOrderView
	// Silo is the colony's market depot, when it has one.
	Silo    Point
	HasSilo bool
	// Prices is every good's smoothed value, in item order; Plans every open
	// production plan, oldest first; ChainDepth the deepest of them; Starved
	// how many colonists have starved. See docs/valuation.md.
	Prices     []PriceView
	Plans      []PlanView
	ChainDepth int
	Starved    int
}

// PriceView is one good's value: its smoothed trade price, or its reference
// value if it has never traded.
type PriceView struct {
	Item   ItemKind
	Value  Money
	Traded bool
}

// PlanView is an immutable copy of one production plan.
type PlanView struct {
	Actor   EntityID
	Summary string // "craft 1 meal for $15 at (6, 6)"
	Depth   int
	Waiting bool // still waiting on inputs from its derived bids
}

// WorkOrderView is an immutable copy of one open work order.
type WorkOrderView struct {
	ID     OrderID
	Kind   WorkKind
	Issuer Owner
	Pay    Money // per unit
	Units  int
	Pos    Point
}

// OrderView is an immutable copy of one open order.
type OrderView struct {
	ID    OrderID
	Side  Side
	Item  ItemKind
	Qty   int
	Price Money
	Actor Owner
	Depot Point
	// Posted is the tick it was posted (a reprice keeps the original's),
	// and Manual whether a player placed or repriced it for the colony (see
	// docs/colony-orders.md).
	Posted int
	Manual bool
	// Wide is set for an order a colony-wide order placed.
	Wide bool
	// Expires is the tick it expires, 0 never. Escrow is the money a bid
	// still holds (an ask's escrow is its Qty in goods). Filled and Fills
	// are what it has traded so far, and with whom (see Fill and
	// docs/order-detail.md); Fills is a copy.
	Expires int
	Escrow  Money
	Filled  int
	Fills   []Fill
}

// BookView summarizes one (item, depot) book: the best price and depth on
// each side, and what it last traded at.
type BookView struct {
	Item             ItemKind
	Depot            Point
	BestBid, BestAsk Money
	BidQty, AskQty   int // units on offer at every price
	Last             Money
	Volume           int
	Traded           bool
}

// economyView copies the money supply and the order book for a frame.
func (w *World) economyView() EconomyView {
	v := EconomyView{
		Treasury:    w.treasury,
		Circulating: w.moneyInCirculation(),
		Frozen:      w.moneyFrozen,
		Escrowed:    w.moneyEscrowed(),
		Exported:    w.moneyExported,
		Issued:      w.moneyIssued,
		Trades:      append([]Trade(nil), w.trades...),
	}
	v.Silo, v.HasSilo = w.marketDepot()
	v.Suspended = w.suspendedOrders()
	v.Wide = w.wideOrders()
	v.DigWage, v.DigMax = w.wageFor(Floor), maxExcavationTiles
	for _, o := range w.sortedWork(nil) {
		v.WorkOrders = append(v.WorkOrders, WorkOrderView{ID: o.ID, Kind: o.Kind, Issuer: o.Issuer,
			Pay: o.Pay, Units: o.Units, Pos: o.Pos})
	}
	for _, o := range w.sortedOrders(nil) {
		v.Orders = append(v.Orders, OrderView{ID: o.ID, Side: o.Side, Item: o.Item, Qty: o.Qty,
			Price: o.Price, Actor: o.Actor, Depot: o.Depot, Posted: o.Posted, Manual: o.manual, Wide: o.wide,
			Expires: o.Expires, Escrow: o.escrow, Filled: o.Filled, Fills: slices.Clone(o.Fills)})
	}
	for k, b := range w.books {
		bv := BookView{Item: k.Item, Depot: k.Depot, Last: b.last, Volume: b.volume, Traded: b.traded}
		for _, o := range b.bids {
			bv.BidQty += o.Qty
		}
		for _, o := range b.asks {
			bv.AskQty += o.Qty
		}
		if len(b.bids) > 0 {
			bv.BestBid = b.bids[0].Price
		}
		if len(b.asks) > 0 {
			bv.BestAsk = b.asks[0].Price
		}
		v.Books = append(v.Books, bv)
	}
	for k := ItemKind(0); k < numItemKinds; k++ {
		if val := w.valueOf(k); val > 0 {
			v.Prices = append(v.Prices, PriceView{Item: k, Value: val, Traded: w.prices[k].traded})
		}
	}
	ids := make([]planID, 0, len(w.plans))
	for id := range w.plans {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		p := w.plans[id]
		v.Plans = append(v.Plans, PlanView{Actor: p.actor, Summary: p.summary(), Depth: p.depth,
			Waiting: p.kind == planCraft && !p.crafted && len(p.derived) > 0})
	}
	v.ChainDepth, v.Starved = w.chainDepth(), w.starved
	sort.Slice(v.Books, func(i, j int) bool {
		if v.Books[i].Depot != v.Books[j].Depot {
			return lessPoint(v.Books[i].Depot, v.Books[j].Depot)
		}
		return v.Books[i].Item < v.Books[j].Item
	})
	return v
}

// ScumAt reports how much cave scum a colonist could scrape off p right now.
func (s *Snapshot) ScumAt(p Point) int { return int(s.Scum[p]) }

// SaltAt reports whether there is a salt deposit on p that the colony can reach.
func (s *Snapshot) SaltAt(p Point) bool {
	_, ok := s.Salt[p]
	return ok
}

// publishedSalt returns an immutable copy of the exposed salt, reusing the
// last one while it is still exact (saltRev). Salt changes only when a tile
// is exposed or built over, so on most ticks this is the same map.
func (w *World) publishedSalt() map[Point]struct{} {
	if w.snapSalt != nil && w.snapSaltRev == w.saltRev {
		return w.snapSalt
	}
	out := make(map[Point]struct{})
	w.eachLayer(func(l *Layer) {
		for p := range l.exposedSalt {
			out[p] = struct{}{}
		}
	})
	w.snapSalt, w.snapSaltRev = out, w.saltRev
	return out
}

// publishedScum returns an immutable copy of the scum on every exposed patch,
// reusing the last one published while it is still exact: nothing has written
// to an exposed patch since (scumRev). Growth (growScum) bumps it too.
//
// It used to be rebuilt every frame. Publishing happens every tick, and on a
// big map that made the scum copy most of what the engine did.
func (w *World) publishedScum() map[Point]uint8 {
	if w.snapScum != nil && w.snapScumRev == w.scumRev {
		return w.snapScum
	}
	out := make(map[Point]uint8)
	w.eachLayer(func(l *Layer) {
		for p := range l.exposedScum {
			if n := w.scumAt(p); n > 0 {
				out[p] = uint8(min(n, math.MaxUint8)) // scum-max is validated to fit; never wrap
			}
		}
	})
	w.snapScum, w.snapScumRev = out, w.scumRev
	return out
}

// FixtureAt returns the ownership record of the fixture at p, if there is one.
func (s *Snapshot) FixtureAt(p Point) (FixtureView, bool) {
	i := sort.Search(len(s.Fixtures), func(i int) bool { return !lessPoint(s.Fixtures[i].Pos, p) })
	if i < len(s.Fixtures) && s.Fixtures[i].Pos == p {
		return s.Fixtures[i], true
	}
	return FixtureView{}, false
}

// DriveMeta describes a drive for display: its name, ceiling, and what reaching
// the ceiling does (DriveSpec.CeilingConsequence). Carried in the snapshot so
// frontends can render drive bars without reaching into Config.
type DriveMeta struct {
	Name        string
	Max         int
	Consequence Consequence
}

// Fatal reports whether a drive at its ceiling kills: the bar a frontend paints
// red.
func (m DriveMeta) Fatal() bool { return m.Consequence == ConsequenceDeath }

// Stats summarizes the world at a glance for the UI header.
type Stats struct {
	Colonists int
	Aliens    int
	Cats      int
	Rats      int
	Chickens  int
	FloorDug  int // tiles of discovered Floor (excavation progress; undiscovered caverns excluded)
	// ExploredTiles is how many tiles World.reveal has ever uncovered (see
	// World.exploredCount). Only meaningful when FogOfWar is on -- with it
	// off every tile already reads as explored (see Snapshot.ExploredAt), so
	// it is published as 0.
	ExploredTiles int
	Pods          int // nutrient pods built
	Toilets       int // toilets built
	Beds          int // dormitory bunks built
	// Incinerators built, and Refuse still on the floor (gore stains plus
	// bodies) waiting to be hauled to one. See docs/sanitation.md.
	Incinerators      int
	StorageContainers int
	Refuse            int
	Rooms             int // distinct rooms (connected floor areas) the colony has discovered
	// ChunksGenerated and Chunks are how many 64x64 worldgen chunks exist so
	// far, and how many the map has. Chunks are generated lazily, as the
	// colony explores (see docs/worldgen-chunks.md); Chunks is 0 for a world
	// built without generation.
	ChunksGenerated int
	Chunks          int
	// Day is the colony day since landing (day 1 is the landing day), at
	// Config.TicksPerDay ticks a day, and MinuteOfDay is the clock time in
	// minutes since midnight (0..1439). See docs/days.md.
	Day         int
	MinuteOfDay int
}

// Snapshot is an immutable, self-contained picture of the world at one tick.
// The Engine keeps mutating the real world after handing a Snapshot to
// frontends, so nothing here aliases live state: everything colony-sized is
// copied outright, and the terrain is a page-shared grid whose pages are copied
// before they can change (see tilegrid.go). Either way a frame is safe to read
// on another goroutine for as long as it is held.
//
// The one exception is opt-in: under TilesLive (see Engine.ShareLiveTiles),
// Tiles aliases the live map, and the frame's terrain is only good on the
// engine's goroutine until the next tick. Everything else stays a copy.
type Snapshot struct {
	Tick int
	// TicksPerDay is Config.TicksPerDay, for a frontend turning some other
	// tick (an order's Posted) into a day and a clock time with DayOf and
	// MinuteOfDay. See docs/days.md.
	TicksPerDay int
	// Ships is every colony ship that has landed, in landing order, then any
	// still aloft (see LandShip). Before the first tick (Tick 0) a frontend
	// may land the next aloft one with LandShip and move landed ones with
	// MoveShip.
	Ships  []ShipView
	Width  int
	Height int
	// Seed is this run's world seed -- the one fact that, together with the
	// rest of this Snapshot, would let someone else regenerate the same
	// world. Shown on the lore panel so a player can share or record it. See
	// docs/lore.md.
	Seed int64
	// Config is the settings this run was started with, for the browser's
	// Game tab, which lists them all (wire's "config" topic). It points at
	// the World's own copy rather than copying it every frame: a World's
	// Config never changes after newWorld, so sharing it is safe. Read-only.
	Config *Config
	// Tiles is the terrain, as an immutable page-shared grid rather than a
	// per-frame copy of the map. See tilegrid.go for why it is not a plain
	// slice. Frontends should read it through TileAt / TerrainAt: Tiles.At
	// holds only generated chunks, so with the fog off it would show the rest
	// as bare rock where TileAt shows the preview.
	//
	// Tiles is the landing level's grid. LevelTiles holds every level's,
	// indexed by Level, nil for a level the colony has never broken into;
	// TileAt, TerrainAt and ExploredAt read whichever level the Point they
	// are given is on.
	Tiles      *TileGrid
	LevelTiles []*TileGrid
	// TileChanges says which pages of Tiles (the landing level) changed since
	// the previous Snapshot, for a consumer that forwards terrain
	// incrementally (the browser's wire encoder). See tilegrid.go.
	TileChanges TileChanges
	Entities    []EntityView
	// Graveyard is the most recent violent/starvation deaths (bounded by
	// Config.GraveyardSize), oldest first, for the roster's "dead" filter.
	// See docs/combat.md.
	Graveyard []EntityView
	// Deceased is every colonist who has ever died, keyed by EntityID and
	// never trimmed — unlike Graveyard, which also covers rats/cats/aliens
	// and drops old entries. Consulted for durable by-ID lookups: a dead
	// colonist's name, family relations, and frozen inventory all resolve
	// through this map indefinitely. See docs/combat.md.
	//
	// The map is shared by every snapshot published between two deaths, so
	// it must not be modified.
	Deceased map[EntityID]EntityView
	// Log is the retained colony log, oldest first. Kind is the type column;
	// Text is the sentence. See log.go.
	Log        []LogEntry
	Stats      Stats
	DrivesMeta [numDrives]DriveMeta

	// Projects are the colony's queued construction work, for the job board.
	// PendingFacilityRooms / PendingDormitories / PendingTrashRooms /
	// PendingStorageRooms are manual orders not yet turned into a project because
	// another is already in progress.
	Projects             []ProjectView
	PendingFacilityRooms int
	PendingDormitories   int
	PendingTrashRooms    int
	PendingStorageRooms  int
	PendingScumhouses    int
	PendingFoundries     int
	PendingHalls         int
	PendingIncubators    int
	// PendingStairs counts stairs ordered but not yet marked out.
	PendingStairs int
	Storages      []StorageView
	// Scum is how much cave scum is on every exposed patch that has any,
	// computed fresh each frame because patches regrow lazily (see
	// scumhouse.go). Read it with ScumAt.
	Scum map[Point]uint8
	// Salt is every deposit of salt a colonist could reach (the same
	// exposure rule as scum). It is shared between frames until it changes,
	// and never written after publication. Read it with SaltAt.
	Salt map[Point]struct{}
	// Fixtures is the ownership of every placed fixture (pods, toilets, beds,
	// incinerators, storage), sorted by position. The slice is shared between
	// frames until a fixture changes, and never written after publication.
	Fixtures []FixtureView
	// Zoning (see docs/zoning.md). Zones is every zoned tile as row runs,
	// sorted by row then column; Structures every standing or rising
	// structure, by id. Both are shared between frames until they change,
	// and never written after publication. ZoneWaiting lists the fixture
	// kinds the colony wants and no zone has room for. ZoningAuto is the
	// zoning-auto setting; ClearWage what clearing one tile pays.
	Zones       []ZoneRun
	Structures  []StructureView
	ZoneWaiting []Terrain
	ZoningAuto  bool
	ClearWage   Money

	// AlienSpecies is this world's roster of rolled alien species -- each
	// one's build, colloquial name, and temperament. Every Alien in Entities
	// carries a copy of the one it belongs to on its own EntityView.AlienSpecies;
	// this is the full roster, for a codex-style listing. See docs/lore.md.
	AlienSpecies []AlienSpecies
	// Corporations is this world's roster of companies, and GunModels the make
	// and model each gun kind carries (Maker indexes Corporations). Flavor
	// only. See docs/arms-makers.md.
	Corporations []Corporation
	GunModels    []GunModel

	// Economy is the money supply; each colonist's own balance is on its
	// EntityView.Wallet.
	Economy EconomyView
	// Recruiting is the recruiter's terms and the set on offer (see
	// docs/recruiting.md).
	Recruiting RecruitingView

	AffinityMax    int // affinity display bars run [-AffinityMax, AffinityMax]
	MoodMax        int // charge and grip each run in [-MoodMax, MoodMax]
	ScumMax        int // a full scum patch; Scum values run 1..ScumMax
	Paused         bool
	TicksPerSecond int

	// Perf is the engine's recent timing history, oldest first, one sample
	// per PerfBucket of wall-clock time. It is shared between snapshots and
	// must not be modified. See docs/perf-screen.md.
	Perf []PerfSample
	// Population is the colony's vital signs over the whole game, oldest
	// first; see population.go. Shared between snapshots, never written.
	Population []PopulationSample
	// Metrics is everything the chart system can plot, sampled hourly on
	// the colony clock over the whole game; see metrics.go and
	// docs/charts.md. Shared between snapshots, never written.
	Metrics *MetricsView

	// FlowFields lists every shared flow field, in a stable order, for a
	// frontend that offers to show one. FlowField is the one it asked for
	// with ShowFlowField, or nil when none is shown. A FlowFieldView is
	// shared between snapshots until the field changes, and never written
	// after publication. See docs/flow-field-view.md.
	FlowFields []FlowFieldRef
	FlowField  *FlowFieldView

	// FogOfWar says whether Tile.Explored is being maintained, so a frontend
	// knows whether to hide the unexplored map. It is false on a hand-built
	// Snapshot, which is what makes every tile of one read as explored (see
	// ExploredAt) instead of a test fixture rendering as a blank screen.
	FogOfWar bool

	// previews show ungenerated chunks when the fog is off (see TileAt), one
	// per level like LevelTiles. They are shared between snapshots and safe
	// for concurrent use; nil for a world built without generation.
	previews []*ChunkPreview
}

// Levels lists the levels the colony has broken into, shallowest first.
func (s *Snapshot) Levels() []Level {
	var out []Level
	for l, g := range s.LevelTiles {
		if g != nil {
			out = append(out, Level(l))
		}
	}
	if len(out) == 0 && s.Tiles != nil {
		out = append(out, s.Tiles.Level()) // a hand-built frame
	}
	return out
}

// tilesOf is the grid of the level p is on: LevelTiles when it has one, and
// Tiles otherwise (a hand-built frame sets only Tiles; nothing else exists).
func (s *Snapshot) tilesOf(p Point) *TileGrid {
	if int(p.Level) >= 0 && int(p.Level) < len(s.LevelTiles) && s.LevelTiles[p.Level] != nil {
		return s.LevelTiles[p.Level]
	}
	if len(s.LevelTiles) == 0 {
		return s.Tiles
	}
	return nil
}

// previewOf is the preview for the level p is on, or nil.
func (s *Snapshot) previewOf(p Point) *ChunkPreview {
	if int(p.Level) >= 0 && int(p.Level) < len(s.previews) {
		return s.previews[p.Level]
	}
	return nil
}

// ExploredAt reports whether the colony has seen p, and so whether a frontend
// may draw what is on it. With fog of war off — including on any Snapshot
// nobody set FogOfWar on — every in-bounds tile is explored.
//
// Asking here rather than reading Tile.Explored directly is what keeps the fog
// off the engine's hot paths: turning fog off marks no tiles, so a huge map's
// tile array stays the mostly-untouched zero pages that make publishing a
// frame cheap (see tilegrid.go).
func (s *Snapshot) ExploredAt(p Point) bool {
	if p.X < 0 || p.X >= s.Width || p.Y < 0 || p.Y >= s.Height {
		return false
	}
	if s.tilesOf(p) == nil && len(s.LevelTiles) > 0 {
		return false // a level the colony has never broken into
	}
	if !s.FogOfWar {
		return true
	}
	return s.tilesOf(p).At(p).Explored
}

// TerrainAt reads the published grid; out-of-bounds reads return Rock so callers
// (the renderer) can treat the world edge as solid. See TileAt for chunks the
// simulation has not generated yet.
func (s *Snapshot) TerrainAt(p Point) Terrain {
	if p.X < 0 || p.X >= s.Width || p.Y < 0 || p.Y >= s.Height {
		return Rock
	}
	if s.previewing(p) {
		return s.previewOf(p).At(p).Terrain
	}
	return s.tilesOf(p).TerrainAt(p)
}

// TileAt reads the published grid's full Tile (terrain, composition, and gore),
// for renderers that need it. Out-of-bounds reads return clean ordinary rock.
//
// A chunk the simulation has not generated yet is not in the grid. With fog of
// war on, all of it is unexplored, so it reads as unexplored Rock and is never
// drawn. With fog off, the whole map is on show, so it reads from a preview:
// exactly what the chunk will hold when the colony's exploration generates it,
// computed without generating it (see ChunkPreview).
func (s *Snapshot) TileAt(p Point) Tile {
	if p.X < 0 || p.X >= s.Width || p.Y < 0 || p.Y >= s.Height {
		return Tile{Terrain: Rock, Composition: OrdinaryRock}
	}
	if s.previewing(p) {
		return s.previewOf(p).At(p)
	}
	return s.tilesOf(p).At(p)
}

// previewing reports whether the in-bounds p should be read from the preview:
// fog is off and its chunk has not been generated.
func (s *Snapshot) previewing(p Point) bool {
	g := s.tilesOf(p)
	return g != nil && s.previewOf(p) != nil && !s.FogOfWar && !g.hasPage(p)
}

// snapshot builds an immutable view of the world's current state.
func (w *World) snapshot(paused bool, tps int) *Snapshot {
	w.snapFrame++
	levelTiles := make([]*TileGrid, len(w.layers))
	previews := make([]*ChunkPreview, len(w.layers))
	var tileChanges TileChanges
	w.eachLayer(func(l *Layer) {
		g, changes := w.publishedTiles(l)
		levelTiles[l.Level], previews[l.Level] = g, l.preview
		if l.Level == LandingLevel {
			tileChanges = changes
		}
	})
	tileChanges.Frame = w.snapFrame

	ents := make([]EntityView, 0, len(w.entities))
	kinChildren := w.cachedKinChildren()
	// Terrain totals come from the incremental counts SetTerrain maintains;
	// counting them by walking the grid would put the map's whole area back on
	// every tick, which is exactly what the shared grid above avoids.
	stats := Stats{
		Rooms:         len(w.discoveredRooms),
		FloorDug:      w.countTerrain(Floor) - w.hiddenFloor(),
		ExploredTiles: w.exploredTilesStat(),
		Pods:          w.countTerrain(NutrientPod),
		Toilets:       w.countTerrain(Toilet),
		Beds:          w.countTerrain(Bed),

		Incinerators:      w.countTerrain(Incinerator),
		StorageContainers: w.countTerrain(Storage),
		Refuse:            w.refuseTotal(),
	}
	tpd := w.cfg.TicksPerDay()
	stats.Day, stats.MinuteOfDay = DayOf(w.tick, tpd), MinuteOfDay(w.tick, tpd)
	// Chunks counts every level the colony has broken into: each has the
	// map's full complement.
	w.eachLayer(func(l *Layer) {
		stats.ChunksGenerated += len(l.genChunks)
		if l.gen != nil {
			stats.Chunks += l.gen.chunkCols() * l.gen.chunkRows()
		}
	})
	for _, e := range w.entities {
		ev := w.entityView(e, kinChildren, true)
		ents = append(ents, ev)
		switch e.Kind {
		case Colonist:
			stats.Colonists++
		case Alien:
			stats.Aliens++
		case Cat:
			stats.Cats++
		case Rat:
			stats.Rats++
		case Chicken:
			stats.Chickens++
		}
	}

	var drivesMeta [numDrives]DriveMeta
	for i := 0; i < int(numDrives); i++ {
		spec := w.cfg.Drives[i]
		drivesMeta[i] = DriveMeta{Name: spec.Name, Max: spec.Max, Consequence: spec.CeilingConsequence()}
	}

	projects := make([]ProjectView, 0, len(w.projects))
	for _, p := range w.projects {
		phase, _ := w.activeProjectPhase(p)
		tasks := make([]TaskView, 0, len(p.tasks))
		for _, t := range p.tasks {
			tasks = append(tasks, TaskView{
				Pos:     t.pos,
				Terrain: t.terrain,
				Phase:   t.phase,
				Done:    w.taskDone(t),
				Owner:   t.owner,
			})
		}
		projects = append(projects, ProjectView{
			ID:         p.id,
			Name:       p.name,
			QueuedTick: p.queuedTick,
			Phase:      phase,
			Tasks:      tasks,
		})
	}

	storages := w.snapshotStorages()

	return &Snapshot{
		Tick:                 w.tick,
		TicksPerDay:          tpd,
		Width:                w.Width,
		Height:               w.Height,
		Seed:                 w.cfg.Seed,
		Config:               &w.cfg,
		Tiles:                levelTiles[LandingLevel],
		LevelTiles:           levelTiles,
		TileChanges:          tileChanges,
		Entities:             ents,
		Log:                  w.log.tail(len(w.log.entries)),
		Stats:                stats,
		DrivesMeta:           drivesMeta,
		Projects:             projects,
		PendingFacilityRooms: w.manualFacilityRooms,
		PendingDormitories:   w.manualDormitories,
		PendingTrashRooms:    w.manualTrashRooms,
		PendingStorageRooms:  w.manualStorageRooms,
		PendingScumhouses:    w.manualScumhouses,
		PendingFoundries:     w.manualFoundries,
		PendingHalls:         w.manualHalls,
		PendingIncubators:    w.manualIncubators,
		PendingStairs:        w.manualStairs,
		Storages:             storages,
		Fixtures:             w.publishedFixtures(),
		Zones:                w.publishedZones(),
		Structures:           w.publishedStructures(),
		ZoneWaiting:          w.zoneWaiting(),
		ZoningAuto:           w.cfg.ZoningAuto,
		ClearWage:            Money(w.cfg.WageDemolish),
		Scum:                 w.publishedScum(),
		Salt:                 w.publishedSalt(),
		Graveyard:            append([]EntityView(nil), w.graveyard...),
		Deceased:             w.publishedDeceasedColonists(),
		AlienSpecies:         append([]AlienSpecies(nil), w.alienSpecies...),
		Corporations:         append([]Corporation(nil), w.corporations...),
		GunModels:            append([]GunModel(nil), w.gunModels...),
		Population:           w.popHist,
		Metrics:              w.metricsView(),
		Economy:              w.economyView(),
		Recruiting:           w.recruitingView(),
		AffinityMax:          w.cfg.AffinityMax,
		MoodMax:              w.cfg.MoodMax,
		ScumMax:              w.cfg.ScumMax,
		Ships:                w.shipViews(),
		Paused:               paused,
		TicksPerSecond:       tps,
		FogOfWar:             w.cfg.FogOfWar,
		previews:             previews,
	}
}

// snapshotStorages copies every storage container, ledger included, sorted by
// position so the list never depends on map iteration order.
func (w *World) snapshotStorages() []StorageView {
	storages := make([]StorageView, 0)
	for _, container := range w.storageContainers {
		storages = append(storages, StorageView{
			Pos:       container.Pos,
			Terrain:   container.Terrain,
			Pantry:    w.isPantry(container.Pos),
			Inventory: container.Inventory,
			Ledger:    append([]LedgerLine(nil), container.Ledger...),
		})
	}
	sort.Slice(storages, func(i, j int) bool {
		return lessPoint(storages[i].Pos, storages[j].Pos)
	})
	return storages
}

// publishedDeceasedColonists returns the deceased archive as snapshots
// publish it: a copy, so a Snapshot never shares the map the world keeps
// writing to (the same reasoning as Graveyard's copy above), but one copy
// per death rather than one per frame. Every snapshot between two deaths
// shares it, which is safe because nothing writes to it after it is made:
// a death replaces it (World.remove clears it) instead of editing it.
func (w *World) publishedDeceasedColonists() map[EntityID]EntityView {
	if w.publishedDeceased == nil {
		w.publishedDeceased = make(map[EntityID]EntityView, len(w.deceasedColonists))
		for id, ev := range w.deceasedColonists {
			w.publishedDeceased[id] = ev
		}
	}
	return w.publishedDeceased
}

// entityView builds a read-only copy of e for display. full additionally
// computes family/affinity ties, which only make sense for a still-living
// colonist among still-living kin; a frozen graveyard record (see
// World.remove) passes false and a nil kinChildren, leaving those empty
// rather than stale.
func (w *World) entityView(e *Entity, kinChildren map[kinID][]kinID, full bool) EntityView {
	ev := EntityView{
		ID:        e.ID,
		Kind:      e.Kind,
		Pos:       e.Pos,
		HP:        e.HP,
		MaxHP:     e.MaxHP,
		State:     e.State,
		Focus:     e.focus,
		Drives:    w.currentDrives(e),
		Profile:   e.Profile.clone(),
		Inventory: e.Inventory,
		Wallet:    e.wallet,
		Keeper:    e.keeperOf(),
	}
	if e.hasParts() {
		ev.Parts = e.Parts
		ev.MaxParts = e.MaxParts
	}
	if e.Kind == Alien {
		ev.AlienSpecies = w.alienSpeciesFor(e)
		if f, ok := w.formOf(e); ok {
			ev.AlienForm = f.Name
		}
	}
	if e.Kind == Colonist {
		ev.Charge = e.affect.Charge
		ev.Grip = e.affect.Grip
		ev.Valence = e.affect.Valence
		ev.MoodLabel = w.affectName(e.affect)
		ev.Memories = append([]Memory(nil), e.Memories...)
		ev.Skills = e.skillViews()
		ev.Profession, ev.ProfessionLabel = e.profession, e.professionLabel()
		ev.Backstory = w.backstory(e)
		if full {
			ev.Relations = append([]Relation(nil), w.cachedRelations(e, kinChildren)...)
			ev.Affinities = w.affinitiesOf(e.ID)
		}
	}
	return ev
}

// currentDrives returns a colonist's drive levels as of now, computed lazily.
func (w *World) currentDrives(e *Entity) [numDrives]int {
	var out [numDrives]int
	for i := 0; i < int(numDrives); i++ {
		out[i] = w.driveLevel(e, DriveKind(i))
	}
	return out
}

// exploredTilesStat is Stats.ExploredTiles: the explored count with fog of war
// on, and 0 with it off, where the count means nothing to a frontend (every
// tile reads as explored) even though the simulation still keeps it.
func (w *World) exploredTilesStat() int {
	if !w.cfg.FogOfWar {
		return 0
	}
	n := 0
	w.eachLayer(func(l *Layer) { n += l.exploredCount })
	return n
}

// hiddenFloor is how much undiscovered cavern floor there is, on every level.
func (w *World) hiddenFloor() int {
	n := 0
	w.eachLayer(func(l *Layer) { n += l.hiddenFloor })
	return n
}
