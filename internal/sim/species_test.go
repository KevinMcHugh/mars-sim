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

// A species breeds exactly when its ladder has a breed rung, and only those
// spawn with a Breeding component: a breed rung without one could never
// mate, and a Breeding nobody acts on is dead weight.
func TestBreedingComponentMatchesTheLadder(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	for k := Kind(0); k < numKinds; k++ {
		sp := w.species[k]
		hasRung := false
		for _, b := range sp.ladder {
			if _, ok := b.(breed); ok {
				hasRung = true
			}
		}
		if hasRung != sp.Breeds {
			t.Errorf("%v: breed rung %v, Breeds %v", k, hasRung, sp.Breeds)
		}
		if k == Colonist {
			continue // colonists land in ships, not by spawn
		}
		p, ok := w.randomFloor()
		if !ok {
			t.Fatal("no floor to spawn on")
		}
		if e := w.spawn(k, p); (e.breeding != nil) != sp.Breeds {
			t.Errorf("spawned %v has Breeding %v, species Breeds %v", k, e.breeding != nil, sp.Breeds)
		}
	}
}

// Pets that land with a colonist carry a PetBond naming it; a chicken's bond
// also holds its keeper's trough. Strays have none.
func TestLandedPetsCarryAPetBond(t *testing.T) {
	for _, tc := range []struct {
		name          string
		chicken, cat  int
		kind          Kind
		wantTroughSet bool
	}{
		{"chicken", 1, 0, Chicken, true},
		{"cat", 0, 1, Cat, false},
	} {
		w := petWorld(t, 0, tc.chicken, tc.cat, 2)
		for id := range w.kindEntities[tc.kind] {
			pet := w.entities[id]
			if pet.pet == nil {
				t.Fatalf("%s #%d landed without a PetBond", tc.name, id)
			}
			keeper := w.entities[pet.keeperOf()]
			if keeper == nil || keeper.Kind != Colonist {
				t.Fatalf("%s #%d's keeper is not a colonist", tc.name, id)
			}
			trough, ok := pet.petTrough()
			if ok != tc.wantTroughSet || (ok && (!keeper.hasTrough || keeper.trough != trough)) {
				t.Fatalf("%s #%d trough %v,%v; keeper's %v,%v", tc.name, id, trough, ok, keeper.trough, keeper.hasTrough)
			}
		}
	}
	w := petWorld(t, 0, 0, 0, 1)
	p, _ := w.randomFloor()
	if stray := w.spawn(Chicken, p); stray.pet != nil {
		t.Fatal("a spawned stray chicken has a PetBond")
	}
}
