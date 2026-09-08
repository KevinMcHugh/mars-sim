package sim

// Kind identifies what an entity fundamentally is. Behavior is dispatched on
// Kind by the per-tick systems.
type Kind uint8

const (
	// Colonist is a human worker. They walk on Floor, mine Rock, build Walls,
	// and flee from aliens.
	Colonist Kind = iota
	// Alien is a subterranean mutant. It burrows through any terrain to hunt
	// and eat colonists.
	Alien
)

func (k Kind) String() string {
	switch k {
	case Colonist:
		return "colonist"
	case Alien:
		return "alien"
	default:
		return "unknown"
	}
}

// State is a coarse label for what an entity is currently doing. It drives the
// simple per-kind state machines in systems.go and is surfaced in the UI.
type State uint8

const (
	Idle     State = iota
	Moving         // travelling toward Target
	Mining         // excavating Rock at Target
	Building       // constructing a Wall at Target
	Fleeing        // running away from a nearby alien
	Hunting        // (alien) closing on a colonist
	Feeding        // (alien) eating a colonist it has caught
)

func (s State) String() string {
	switch s {
	case Idle:
		return "idle"
	case Moving:
		return "moving"
	case Mining:
		return "mining"
	case Building:
		return "building"
	case Fleeing:
		return "fleeing"
	case Hunting:
		return "hunting"
	case Feeding:
		return "feeding"
	default:
		return "?"
	}
}

// EntityID uniquely identifies an entity for its lifetime. IDs are never reused.
type EntityID uint64

// Entity is a single actor in the world. Rather than a strict ECS, we use one
// struct with a shared set of fields that the systems interpret according to
// Kind. This keeps the scaffold readable; fields can graduate into real
// components as systems multiply.
type Entity struct {
	ID   EntityID
	Kind Kind
	Pos  Point

	HP    int
	MaxHP int

	// Behavior scratch space, meaning depends on Kind/State.
	State     State
	Target    Point    // a tile of interest (dig/build/move destination)
	HasTarget bool     // whether Target is meaningful
	Quarry    EntityID // (alien) the colonist being hunted; 0 if none
	Progress  int      // ticks accumulated on the current Mining/Building job
	Cooldown  int      // ticks until this entity may act again
}

// newEntity builds an entity with kind-appropriate starting stats.
func newEntity(id EntityID, kind Kind, p Point, cfg Config) *Entity {
	e := &Entity{ID: id, Kind: kind, Pos: p, State: Idle}
	switch kind {
	case Colonist:
		e.MaxHP = cfg.ColonistHP
	case Alien:
		e.MaxHP = cfg.AlienHP
	}
	e.HP = e.MaxHP
	return e
}

// Alive reports whether the entity still has hit points.
func (e *Entity) Alive() bool { return e.HP > 0 }
