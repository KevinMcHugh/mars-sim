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

	entities map[EntityID]*Entity
	nextID   EntityID

	tick int
	rng  *rand.Rand
	log  *eventLog
	cfg  Config
}

// newWorld allocates an all-Rock world of the given size.
func newWorld(cfg Config, rng *rand.Rand) *World {
	w := &World{
		Width:    cfg.Width,
		Height:   cfg.Height,
		tiles:    make([]Tile, cfg.Width*cfg.Height),
		entities: make(map[EntityID]*Entity),
		nextID:   1,
		rng:      rng,
		log:      newEventLog(cfg.LogSize),
		cfg:      cfg,
	}
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

// SetTerrain overwrites the terrain at p if it is in bounds.
func (w *World) SetTerrain(p Point, t Terrain) {
	if !w.InBounds(p) {
		return
	}
	w.tiles[w.index(p)].Terrain = t
}

// Walkable reports whether a colonist can stand at p.
func (w *World) Walkable(p Point) bool {
	return w.InBounds(p) && w.TerrainAt(p).Walkable()
}

// spawn creates an entity of the given kind at p and registers it, returning the
// new entity so the caller can tune it.
func (w *World) spawn(kind Kind, p Point) *Entity {
	e := newEntity(w.nextID, kind, p, w.cfg)
	w.nextID++
	w.entities[e.ID] = e
	return e
}

// remove deletes an entity from the world.
func (w *World) remove(id EntityID) {
	delete(w.entities, id)
}

// countKind returns how many living entities of a kind exist.
func (w *World) countKind(kind Kind) int {
	n := 0
	for _, e := range w.entities {
		if e.Kind == kind {
			n++
		}
	}
	return n
}
