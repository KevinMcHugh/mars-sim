package sim

// Kind identifies what an entity fundamentally is. Behavior is dispatched on
// Kind by the per-tick systems.
type Kind uint8

const (
	// Colonist is a human worker: walks on Floor, mines Rock, builds structures,
	// tends to its needs, and flees from aliens.
	Colonist Kind = iota
	// Alien is a subterranean mutant that burrows through any terrain to hunt
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

// State is a coarse label for what an entity is currently doing. It is derived
// from the entity's Job each tick and surfaced in the UI.
type State uint8

const (
	Idle      State = iota
	Moving          // travelling toward Target
	Mining          // excavating Rock
	Building        // constructing a structure
	Eating          // using a nutrient pod
	Relieving       // using a toilet
	Fleeing         // running from a nearby alien
	Hunting         // (alien) closing on a colonist
	Feeding         // (alien) eating a colonist it has caught
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
	case Eating:
		return "eating"
	case Relieving:
		return "relieving"
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

// JobKind is the task a colonist is currently committed to. It is the single
// source of truth for behavior; State is just a display projection of it.
type JobKind uint8

const (
	JobNone  JobKind = iota
	JobMine          // excavate the Rock tile at Target
	JobBuild         // construct BuildKind on the Floor tile at Target
	JobUse           // walk to the facility at Target and satisfy Need
)

// EntityID uniquely identifies an entity for its lifetime. IDs are never reused.
type EntityID uint64

// Entity is a single actor in the world. Rather than a strict ECS, we use one
// struct whose fields the systems interpret according to Kind. This keeps the
// scaffold readable; fields can graduate into real components as systems
// multiply.
type Entity struct {
	ID   EntityID
	Kind Kind
	Pos  Point

	HP    int
	MaxHP int

	// Needs holds each need's current level (0 = satisfied, up to the need's
	// Max), indexed by NeedKind. Colonists only.
	Needs [numNeeds]int

	// Current job and its parameters.
	Job       JobKind
	Target    Point    // tile the job operates on or travels to
	BuildKind Terrain  // JobBuild: terrain to construct
	Need      NeedKind // JobUse: which need this fulfills
	Progress  int      // ticks accumulated on the current action

	// Display + shared behavior scratch.
	State    State
	Quarry   EntityID // (alien) the colonist being hunted; 0 if none
	Cooldown int      // (alien) paces movement/biting
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
