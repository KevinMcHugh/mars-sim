package sim

import "testing"

// Every Kind has a species entry that knows its own Kind and name, so a kind
// added before numKinds cannot be left out of the table.
func TestSpeciesTableCoversEveryKind(t *testing.T) {
	table := newSpeciesTable(DefaultConfig())
	for k := Kind(0); k < numKinds; k++ {
		sp := table[k]
		if sp.Kind != k || sp.Name == "" {
			t.Errorf("species[%d] = {Kind: %v, Name: %q}; want its own Kind and a name", k, sp.Kind, sp.Name)
		}
		if sp.HP <= 0 {
			t.Errorf("%v has %d HP", k, sp.HP)
		}
		if k.String() != sp.Name {
			t.Errorf("%v.String() = %q, species name %q", k, k.String(), sp.Name)
		}
	}
}

// animalTurn gives every creature run by a ladder some behavior each tick it
// acts: the last rung must always act, or a creature could stand frozen with
// a stale State. wander is that rung today.
func TestAnimalLaddersEndInARungThatAlwaysActs(t *testing.T) {
	table := newSpeciesTable(DefaultConfig())
	for k := Kind(0); k < numKinds; k++ {
		if k == Colonist || k == Alien {
			continue // their own turns, not animalTurn (yet)
		}
		ladder := table[k].ladder
		if len(ladder) == 0 {
			t.Errorf("%v has no ladder, but step runs it through animalTurn", k)
			continue
		}
		if _, ok := ladder[len(ladder)-1].(wander); !ok {
			t.Errorf("%v's ladder ends in %T, not a rung that always acts", k, ladder[len(ladder)-1])
		}
	}
}

// A creature that starves must leave a body behind.
func TestStarvingSpeciesLeaveACorpse(t *testing.T) {
	table := newSpeciesTable(DefaultConfig())
	for k := Kind(0); k < numKinds; k++ {
		if sp := table[k]; sp.Starves && sp.Corpse == ItemNone {
			t.Errorf("%v starves but has no corpse", k)
		}
	}
}

// An unnamed creature is labelled by its own species, not as a colonist.
func TestDisplayNameFallsBackToSpecies(t *testing.T) {
	cfg := testConfig()
	cases := map[Kind]string{Rat: "rat #12", Cat: "cat #12", Chicken: "chicken #12", Colonist: "colonist #12"}
	for k, want := range cases {
		if got := newEntity(12, k, Point{}, cfg).displayName(); got != want {
			t.Errorf("displayName of an unnamed %v = %q, want %q", k, got, want)
		}
	}
}
