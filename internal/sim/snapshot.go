package sim

import (
	"math"
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
	Needs     [numNeeds]int
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

	// Dead, DiedTick, and Cause are set only on a Snapshot.Graveyard or
	// Snapshot.Deceased entry: it died at DiedTick (from Cause, a short
	// player-facing phrase like "shot by Zoe Vargas with a shotgun"), and
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
	Terrain   Terrain // Storage (a chest or locker) or Scumhouse
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
// Issued always equals Circulating + Frozen; a frontend can show the three
// side by side without re-deriving any of them. See docs/money.md.
type EconomyView struct {
	Treasury    Money // the community's balance
	Circulating Money // treasury plus every living colonist's wallet
	Frozen      Money // locked in dead colonists' wallets
	Escrowed    Money // held by open bids until they fill or are cancelled
	Issued      Money // every dollar ever minted: Circulating + Frozen + Escrowed

	// The order book (see docs/market.md): every open order oldest first,
	// every book that has ever had an order by depot then item, and the
	// most recent trades, oldest first.
	Orders []OrderView
	Books  []BookView
	Trades []Trade
	// WorkOrders is every open work order, oldest first (see
	// docs/labor.md).
	WorkOrders []WorkOrderView
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
		Issued:      w.moneyIssued,
		Trades:      append([]Trade(nil), w.trades...),
	}
	v.Silo, v.HasSilo = w.marketDepot()
	for _, o := range w.sortedWork(nil) {
		v.WorkOrders = append(v.WorkOrders, WorkOrderView{ID: o.ID, Kind: o.Kind, Issuer: o.Issuer,
			Pay: o.Pay, Units: o.Units, Pos: o.Pos})
	}
	for _, o := range w.sortedOrders(nil) {
		v.Orders = append(v.Orders, OrderView{ID: o.ID, Side: o.Side, Item: o.Item, Qty: o.Qty,
			Price: o.Price, Actor: o.Actor, Depot: o.Depot})
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

// publishedScum returns an immutable copy of the scum on every exposed patch,
// reusing the last one published while it is still exact: nothing has written
// to the scum since (scumRev), and no published patch has regrown a unit yet
// (snapScumUntil). Regrowth is lazy (see scumAt) — there is no write to watch
// when a patch ticks up — so the copy records the earliest tick one will.
//
// It used to be rebuilt every frame. Publishing happens every tick, and on a
// big map that made the scum copy most of what the engine did.
func (w *World) publishedScum() map[Point]uint8 {
	if w.snapScum != nil && w.snapScumRev == w.scumRev && w.tick < w.snapScumUntil {
		return w.snapScum
	}
	out := make(map[Point]uint8, len(w.exposedScum))
	until := math.MaxInt
	for p := range w.exposedScum {
		n := w.scumAt(p)
		if n > 0 {
			out[p] = uint8(n)
		}
		if regrow := w.cfg.ScumRegrowTicks; regrow > 0 && n < w.cfg.ScumMax {
			s := w.scum[p]
			until = min(until, s.since+((w.tick-s.since)/regrow+1)*regrow)
		}
	}
	w.snapScum, w.snapScumRev, w.snapScumUntil = out, w.scumRev, until
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

// NeedMeta describes a need for display: its name, ceiling, and whether maxing
// it out is fatal. Carried in the snapshot so frontends can render need bars
// without reaching into Config.
type NeedMeta struct {
	Name  string
	Max   int
	Fatal bool
}

// Stats summarizes the world at a glance for the UI header.
type Stats struct {
	Colonists int
	Aliens    int
	Cats      int
	Rats      int
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
}

// Snapshot is an immutable, self-contained picture of the world at one tick.
// The Engine keeps mutating the real world after handing a Snapshot to
// frontends, so nothing here aliases live state: everything colony-sized is
// copied outright, and the terrain is a page-shared grid whose pages are copied
// before they can change (see tilegrid.go). Either way a frame is safe to read
// on another goroutine for as long as it is held.
type Snapshot struct {
	Tick   int
	Width  int
	Height int
	// Seed is this run's world seed -- the one fact that, together with the
	// rest of this Snapshot, would let someone else regenerate the same
	// world. Shown on the lore panel so a player can share or record it. See
	// docs/lore.md.
	Seed int64
	// Tiles is the terrain, as an immutable page-shared grid rather than a
	// per-frame copy of the map — read it with TerrainAt (or Tiles.At). See
	// tilegrid.go for why it is not a plain slice.
	Tiles    *TileGrid
	Entities []EntityView
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
	Deceased  map[EntityID]EntityView
	Log       []string
	Stats     Stats
	NeedsMeta [numNeeds]NeedMeta

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
	Storages             []StorageView
	// Scum is how much cave scum is on every exposed patch that has any,
	// computed fresh each frame because patches regrow lazily (see
	// scumhouse.go). Read it with ScumAt.
	Scum map[Point]uint8
	// Fixtures is the ownership of every placed fixture (pods, toilets, beds,
	// incinerators, storage), sorted by position. The slice is shared between
	// frames until a fixture changes, and never written after publication.
	Fixtures []FixtureView

	// AlienSpecies is this world's roster of rolled alien species -- each
	// one's build, colloquial name, and temperament. Every Alien in Entities
	// carries a copy of the one it belongs to on its own EntityView.AlienSpecies;
	// this is the full roster, for a codex-style listing. See docs/lore.md.
	AlienSpecies []AlienSpecies

	// Economy is the money supply; each colonist's own balance is on its
	// EntityView.Wallet.
	Economy EconomyView

	AffinityMax    int // affinity display bars run [-AffinityMax, AffinityMax]
	MoodMax        int // charge and grip each run in [-MoodMax, MoodMax]
	Paused         bool
	TicksPerSecond int

	// Perf is the engine's recent timing history, oldest first, one sample
	// per PerfBucket of wall-clock time. It is shared between snapshots and
	// must not be modified. See docs/perf-screen.md.
	Perf []PerfSample
	// Population is the colony's vital signs over the whole game, oldest
	// first; see population.go. Shared between snapshots, never written.
	Population []PopulationSample

	// FogOfWar says whether Tile.Explored is being maintained, so a frontend
	// knows whether to hide the unexplored map. It is false on a hand-built
	// Snapshot, which is what makes every tile of one read as explored (see
	// ExploredAt) instead of a test fixture rendering as a blank screen.
	FogOfWar bool
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
	if !s.FogOfWar {
		return true
	}
	return s.Tiles.At(p).Explored
}

// TerrainAt reads the published grid; out-of-bounds reads return Rock so callers
// (the renderer) can treat the world edge as solid.
func (s *Snapshot) TerrainAt(p Point) Terrain {
	if p.X < 0 || p.X >= s.Width || p.Y < 0 || p.Y >= s.Height {
		return Rock
	}
	return s.Tiles.TerrainAt(p)
}

// TileAt reads the published grid's full Tile (terrain, composition, and gore),
// for renderers that need it. Out-of-bounds reads return clean ordinary rock.
func (s *Snapshot) TileAt(p Point) Tile {
	if p.X < 0 || p.X >= s.Width || p.Y < 0 || p.Y >= s.Height {
		return Tile{Terrain: Rock, Composition: OrdinaryRock}
	}
	return s.Tiles.At(p)
}

// snapshot builds an immutable view of the world's current state.
func (w *World) snapshot(paused bool, tps int) *Snapshot {
	tiles := w.publishedTiles()

	ents := make([]EntityView, 0, len(w.entities))
	kinChildren := w.cachedKinChildren()
	// Terrain totals come from the incremental counts SetTerrain maintains;
	// counting them by walking the grid would put the map's whole area back on
	// every tick, which is exactly what the shared grid above avoids.
	stats := Stats{
		Rooms:         len(w.discoveredRooms),
		FloorDug:      w.terrainCounts[Floor] - w.hiddenFloor,
		ExploredTiles: w.exploredTilesStat(),
		Pods:          w.terrainCounts[NutrientPod],
		Toilets:       w.terrainCounts[Toilet],
		Beds:          w.terrainCounts[Bed],

		Incinerators:      w.terrainCounts[Incinerator],
		StorageContainers: w.terrainCounts[Storage],
		Refuse:            w.refuseTotal(),
	}
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
		}
	}

	var needsMeta [numNeeds]NeedMeta
	for i := 0; i < int(numNeeds); i++ {
		spec := w.cfg.Needs[i]
		needsMeta[i] = NeedMeta{Name: spec.Name, Max: spec.Max, Fatal: spec.Fatal}
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
		Width:                w.Width,
		Height:               w.Height,
		Seed:                 w.cfg.Seed,
		Tiles:                tiles,
		Entities:             ents,
		Log:                  w.log.tail(len(w.log.entries)),
		Stats:                stats,
		NeedsMeta:            needsMeta,
		Projects:             projects,
		PendingFacilityRooms: w.manualFacilityRooms,
		PendingDormitories:   w.manualDormitories,
		PendingTrashRooms:    w.manualTrashRooms,
		PendingStorageRooms:  w.manualStorageRooms,
		PendingScumhouses:    w.manualScumhouses,
		Storages:             storages,
		Fixtures:             w.publishedFixtures(),
		Scum:                 w.publishedScum(),
		Graveyard:            append([]EntityView(nil), w.graveyard...),
		Deceased:             w.publishedDeceasedColonists(),
		AlienSpecies:         append([]AlienSpecies(nil), w.alienSpecies...),
		Population:           w.popHist,
		Economy:              w.economyView(),
		AffinityMax:          w.cfg.AffinityMax,
		MoodMax:              w.cfg.MoodMax,
		Paused:               paused,
		TicksPerSecond:       tps,
		FogOfWar:             w.cfg.FogOfWar,
	}
}

// snapshotStorages copies every storage container, ledger included, sorted by
// position so the list never depends on map iteration order.
func (w *World) snapshotStorages() []StorageView {
	storages := make([]StorageView, 0, len(w.storageContainers))
	for _, container := range w.storageContainers {
		storages = append(storages, StorageView{
			Pos:       container.Pos,
			Terrain:   container.Terrain,
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
		Needs:     w.currentNeeds(e),
		Profile:   e.Profile.clone(),
		Inventory: e.Inventory,
		Wallet:    e.wallet,
	}
	if e.hasParts() {
		ev.Parts = e.Parts
		ev.MaxParts = e.MaxParts
	}
	if e.Kind == Alien {
		ev.AlienSpecies = w.alienSpeciesFor(e)
	}
	if e.Kind == Colonist {
		ev.Charge = e.affect.Charge
		ev.Grip = e.affect.Grip
		ev.Valence = e.affect.Valence
		ev.MoodLabel = e.affect.MoodName()
		ev.Memories = append([]Memory(nil), e.Memories...)
		if full {
			ev.Relations = append([]Relation(nil), w.cachedRelations(e, kinChildren)...)
			ev.Affinities = w.affinitiesOf(e.ID)
		}
	}
	return ev
}

// currentNeeds returns a colonist's need levels as of now, computed lazily.
func (w *World) currentNeeds(e *Entity) [numNeeds]int {
	var out [numNeeds]int
	for i := 0; i < int(numNeeds); i++ {
		out[i] = w.needLevel(e, NeedKind(i))
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
	return w.exploredCount
}
