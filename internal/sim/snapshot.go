package sim

// EntityView is a read-only copy of an entity for a single frame. Frontends
// receive these instead of *Entity so they can never touch live game state.
type EntityView struct {
	ID        EntityID
	Kind      Kind
	Pos       Point
	HP        int
	MaxHP     int
	State     State
	Needs     [numNeeds]int
	Profile   *Profile  // colonists only; a deep copy, safe to read
	Inventory Inventory // colonists only; copied by value

	// Relations are the colonist's familial ties to other colonists, derived from
	// the family tree; Affinities are its tracked warmth toward colonists it has
	// talked with, strongest first. Both are colonists only. See relationships.go.
	Relations  []Relation
	Affinities []Affinity
	Mood       int // disposition in [-MoodMax, MoodMax], 0 neutral (colonists only)
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
	FloorDug  int // tiles of Floor that exist (excavation progress)
	Pods      int // nutrient pods built
	Toilets   int // toilets built
	Rooms     int // distinct rooms (connected floor areas)
}

// Snapshot is an immutable, self-contained picture of the world at one tick.
// It is a deep copy: the Engine keeps mutating the real world after handing a
// Snapshot to frontends, so nothing here aliases live state.
type Snapshot struct {
	Tick      int
	Width     int
	Height    int
	Tiles     []Tile // row-major copy, len == Width*Height
	Entities  []EntityView
	Log       []string
	Stats     Stats
	NeedsMeta [numNeeds]NeedMeta

	AffinityMax    int // affinity display bars run [-AffinityMax, AffinityMax]
	MoodMax        int // mood display bar runs [-MoodMax, MoodMax]
	Paused         bool
	TicksPerSecond int
}

// TerrainAt reads the copied grid; out-of-bounds reads return Rock so callers
// (the renderer) can treat the world edge as solid.
func (s *Snapshot) TerrainAt(p Point) Terrain {
	if p.X < 0 || p.X >= s.Width || p.Y < 0 || p.Y >= s.Height {
		return Rock
	}
	return s.Tiles[p.Y*s.Width+p.X].Terrain
}

// snapshot builds an immutable copy of the world's current state.
func (w *World) snapshot(paused bool, tps int) *Snapshot {
	tiles := make([]Tile, len(w.tiles))
	copy(tiles, w.tiles)

	ents := make([]EntityView, 0, len(w.entities))
	kinChildren := w.kinChildren()
	stats := Stats{Rooms: w.roomCount}
	for _, t := range tiles {
		switch t.Terrain {
		case Floor:
			stats.FloorDug++
		case NutrientPod:
			stats.Pods++
		case Toilet:
			stats.Toilets++
		}
	}
	for _, e := range w.entities {
		ev := EntityView{
			ID:        e.ID,
			Kind:      e.Kind,
			Pos:       e.Pos,
			HP:        e.HP,
			MaxHP:     e.MaxHP,
			State:     e.State,
			Needs:     w.currentNeeds(e),
			Profile:   e.Profile.clone(),
			Inventory: e.Inventory,
		}
		if e.Kind == Colonist {
			ev.Relations = w.relativesOf(e, kinChildren)
			ev.Affinities = w.affinitiesOf(e.ID)
			ev.Mood = e.mood
		}
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

	return &Snapshot{
		Tick:           w.tick,
		Width:          w.Width,
		Height:         w.Height,
		Tiles:          tiles,
		Entities:       ents,
		Log:            w.log.tail(len(w.log.entries)),
		Stats:          stats,
		NeedsMeta:      needsMeta,
		AffinityMax:    w.cfg.AffinityMax,
		MoodMax:        w.cfg.MoodMax,
		Paused:         paused,
		TicksPerSecond: tps,
	}
}

// currentNeeds returns a colonist's need levels as of now, computed lazily.
func (w *World) currentNeeds(e *Entity) [numNeeds]int {
	var out [numNeeds]int
	for i := 0; i < int(numNeeds); i++ {
		out[i] = w.needLevel(e, NeedKind(i))
	}
	return out
}
