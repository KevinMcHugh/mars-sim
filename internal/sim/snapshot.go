package sim

import "sort"

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
	Inventory StorageInventory
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
	Mice      int
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
	// never trimmed — unlike Graveyard, which also covers mice/cats/aliens
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
	Storages             []StorageView

	// AlienSpecies is this world's roster of rolled alien species -- each
	// one's build, colloquial name, and temperament. Every Alien in Entities
	// carries a copy of the one it belongs to on its own EntityView.AlienSpecies;
	// this is the full roster, for a codex-style listing. See docs/lore.md.
	AlienSpecies []AlienSpecies

	AffinityMax    int // affinity display bars run [-AffinityMax, AffinityMax]
	MoodMax        int // charge and grip each run in [-MoodMax, MoodMax]
	Paused         bool
	TicksPerSecond int

	// Perf is the engine's recent timing history, oldest first, one sample
	// per PerfBucket of wall-clock time. It is shared between snapshots and
	// must not be modified. See docs/perf-screen.md.
	Perf []PerfSample

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
		case Mouse:
			stats.Mice++
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

	storages := make([]StorageView, 0, len(w.storageContainers))
	for _, container := range w.storageContainers {
		storages = append(storages, StorageView{
			Pos:       container.Pos,
			Inventory: container.Inventory,
		})
	}
	sort.Slice(storages, func(i, j int) bool {
		return lessPoint(storages[i].Pos, storages[j].Pos)
	})

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
		Storages:             storages,
		Graveyard:            append([]EntityView(nil), w.graveyard...),
		Deceased:             w.publishedDeceasedColonists(),
		AlienSpecies:         append([]AlienSpecies(nil), w.alienSpecies...),
		AffinityMax:          w.cfg.AffinityMax,
		MoodMax:              w.cfg.MoodMax,
		Paused:               paused,
		TicksPerSecond:       tps,
		FogOfWar:             w.cfg.FogOfWar,
	}
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
