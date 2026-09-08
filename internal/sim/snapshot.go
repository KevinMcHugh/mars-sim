package sim

// EntityView is a read-only copy of an entity for a single frame. Frontends
// receive these instead of *Entity so they can never touch live game state.
type EntityView struct {
	ID    EntityID
	Kind  Kind
	Pos   Point
	HP    int
	MaxHP int
	State State
	Needs [numNeeds]int
}

// Stats summarizes the world at a glance for the UI header.
type Stats struct {
	Colonists int
	Aliens    int
	FloorDug  int // tiles of Floor that exist (excavation progress)
	Pods      int // nutrient pods built
	Toilets   int // toilets built
}

// Snapshot is an immutable, self-contained picture of the world at one tick.
// It is a deep copy: the Engine keeps mutating the real world after handing a
// Snapshot to frontends, so nothing here aliases live state.
type Snapshot struct {
	Tick     int
	Width    int
	Height   int
	Tiles    []Tile // row-major copy, len == Width*Height
	Entities []EntityView
	Log      []string
	Stats    Stats

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
	stats := Stats{}
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
		ents = append(ents, EntityView{
			ID:    e.ID,
			Kind:  e.Kind,
			Pos:   e.Pos,
			HP:    e.HP,
			MaxHP: e.MaxHP,
			State: e.State,
			Needs: e.Needs,
		})
		switch e.Kind {
		case Colonist:
			stats.Colonists++
		case Alien:
			stats.Aliens++
		}
	}

	return &Snapshot{
		Tick:           w.tick,
		Width:          w.Width,
		Height:         w.Height,
		Tiles:          tiles,
		Entities:       ents,
		Log:            w.log.tail(len(w.log.entries)),
		Stats:          stats,
		Paused:         paused,
		TicksPerSecond: tps,
	}
}
