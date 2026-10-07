package sim

// Components are the optional parts of an Entity: data only some creatures
// have, held by a nil-able pointer so "does it have X" replaces "is it kind
// K". A species attaches its components when one is spawned; events can
// attach them later. They live on the entity, never in a side table keyed by
// EntityID, so no map iteration can ever decide a tie with them. See
// docs/species-and-behaviors.md.

// Breeding is a sexually breeding creature's reproductive state (rats). A
// female that mates is pregnant until dueTick, when she gives birth (see
// animalTurn and giveBirth). mateReadyTick gates mating: it holds a newborn
// back until it matures and spaces out a female's litters.
type Breeding struct {
	sex           Sex
	pregnant      bool
	dueTick       int
	mateReadyTick int
}

// PetBond is a pet that came down with a colonist (a chicken or a cat). keeper
// is that colonist. A chicken also eats at its keeper's trough while
// hasTrough; the keeper holds the same trough in its own Entity.trough, which
// is the colonist's side of the bond (it fills it), so a fixture that moves or
// is torn down updates both. A stray has no PetBond. See chickens.go.
type PetBond struct {
	keeper    EntityID
	trough    Point
	hasTrough bool
}

// LifeStage is an alien's place in its species' lifecycle (aliens whose
// species has more than one form): which of AlienSpecies.Forms it is now,
// and the tick it grows into the next stage (0 at the last). See
// alien_lifecycle.go.
type LifeStage struct {
	form   int
	growAt int
	layAt  int // a laying form's next brood (0 for any other form); see layBrood
}

// keeperOf is the colonist a pet came down with, 0 for a stray or a non-pet.
func (e *Entity) keeperOf() EntityID {
	if e.pet == nil {
		return 0
	}
	return e.pet.keeper
}

// petTrough is where a pet eats, if it has a trough.
func (e *Entity) petTrough() (Point, bool) {
	if e.pet == nil || !e.pet.hasTrough {
		return Point{}, false
	}
	return e.pet.trough, true
}
