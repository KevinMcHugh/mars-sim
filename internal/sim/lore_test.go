package sim

import (
	"math/rand"
	"testing"
)

// Rolling the same seed twice must produce the exact same species: this is
// gameplay (bite damage, combat pace), not flavor, so it has to be
// reproducible the way everything else on a seed is (see AGENTS.md).
func TestRollAlienSpeciesIsDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	a := rollAlienSpecies(rand.New(rand.NewSource(12345)), cfg)
	b := rollAlienSpecies(rand.New(rand.NewSource(12345)), cfg)
	if a != b {
		t.Fatalf("same seed rolled different species:\n%+v\n%+v", a, b)
	}
}

// Every rolled species must satisfy its own invariants regardless of what
// the RNG stream happens to draw: valid ranges, arms never outnumbering
// limbs, and derived combat stats that never round down to something
// unusable.
func TestRollAlienSpeciesInvariants(t *testing.T) {
	cfg := DefaultConfig()
	for seed := int64(0); seed < 500; seed++ {
		sp := rollAlienSpecies(rand.New(rand.NewSource(seed)), cfg)
		if sp.Singular == "" || sp.Plural == "" {
			t.Fatalf("seed %d: empty species name: %+v", seed, sp)
		}
		if sp.HeightMinCM <= 0 || sp.HeightMaxCM < sp.HeightMinCM {
			t.Fatalf("seed %d: bad height range %d..%d", seed, sp.HeightMinCM, sp.HeightMaxCM)
		}
		if sp.WeightMinKG <= 0 || sp.WeightMaxKG < sp.WeightMinKG {
			t.Fatalf("seed %d: bad weight range %d..%d", seed, sp.WeightMinKG, sp.WeightMaxKG)
		}
		if sp.Eyes < 1 {
			t.Fatalf("seed %d: species has no eyes", seed)
		}
		if sp.Limbs < 2 {
			t.Fatalf("seed %d: species has fewer than 2 limbs: %d", seed, sp.Limbs)
		}
		if sp.Arms < 0 || sp.Arms > sp.Limbs {
			t.Fatalf("seed %d: arms %d out of range for %d limbs", seed, sp.Arms, sp.Limbs)
		}
		if sp.Aggression < 0 || sp.Aggression > 100 {
			t.Fatalf("seed %d: aggression %d out of 0..100", seed, sp.Aggression)
		}
		if sp.BiteDamage < 1 {
			t.Fatalf("seed %d: bite damage %d, want >= 1 for a positive baseline", seed, sp.BiteDamage)
		}
		if sp.BiteRest < 1 || sp.Slowness < 1 {
			t.Fatalf("seed %d: bite rest %d / slowness %d, want >= 1", seed, sp.BiteRest, sp.Slowness)
		}
	}
}

// Different seeds should tend to roll different species: the whole point is
// that the fictional world reads differently from one playthrough to the
// next.
func TestRollAlienSpeciesVariesAcrossSeeds(t *testing.T) {
	cfg := DefaultConfig()
	seen := map[AlienSpecies]bool{}
	for seed := int64(0); seed < 100; seed++ {
		seen[rollAlienSpecies(rand.New(rand.NewSource(seed)), cfg)] = true
	}
	if len(seen) < 20 {
		t.Fatalf("only %d distinct species across 100 seeds, want plenty of variety", len(seen))
	}
}

// A configured AlienDamage of 0 (how combat_test.go neuters an alien's bite
// to isolate other behavior) must stay 0 after size-scaling, not get floored
// back up to 1 -- scaling must not turn "off" into "on".
func TestSpeciesDamageZeroBaselinePassesThrough(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienDamage = 0
	for seed := int64(0); seed < 50; seed++ {
		sp := rollAlienSpecies(rand.New(rand.NewSource(seed)), cfg)
		if sp.BiteDamage != 0 {
			t.Fatalf("seed %d: bite damage = %d with AlienDamage 0, want 0", seed, sp.BiteDamage)
		}
	}
}

// speciesDamage must actually scale with size: a heavier species deals more
// damage than a lighter one at the same baseline and reference weight, and a
// species at exactly the reference weight reproduces the baseline exactly.
func TestSpeciesDamageScalesWithSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienDamage = 10
	cfg.AlienReferenceWeightKG = 100

	light := AlienSpecies{WeightMinKG: 40, WeightMaxKG: 60}    // midpoint 50
	average := AlienSpecies{WeightMinKG: 90, WeightMaxKG: 110} // midpoint 100
	heavy := AlienSpecies{WeightMinKG: 190, WeightMaxKG: 210}  // midpoint 200

	lightDmg := speciesDamage(light, cfg)
	averageDmg := speciesDamage(average, cfg)
	heavyDmg := speciesDamage(heavy, cfg)

	if averageDmg != cfg.AlienDamage {
		t.Fatalf("species at the reference weight dealt %d, want the baseline %d", averageDmg, cfg.AlienDamage)
	}
	if !(lightDmg < averageDmg && averageDmg < heavyDmg) {
		t.Fatalf("damage did not scale monotonically with size: light=%d average=%d heavy=%d", lightDmg, averageDmg, heavyDmg)
	}
}

// scaledByAggression must reproduce the baseline exactly at the midpoint
// (50) and move a curious-and-passive species (0) and a relentless one (100)
// symmetrically to either side of it.
func TestScaledByAggression(t *testing.T) {
	base := 10
	if got := scaledByAggression(base, 50); got != base {
		t.Fatalf("aggression 50 = %d, want the unscaled baseline %d", got, base)
	}
	passive := scaledByAggression(base, 0)
	relentless := scaledByAggression(base, 100)
	if passive <= base {
		t.Fatalf("aggression 0 (passive) = %d, want slower than baseline %d", passive, base)
	}
	if relentless >= base {
		t.Fatalf("aggression 100 (relentless) = %d, want faster than baseline %d", relentless, base)
	}
}

// Combat must actually read the rolled species rather than the flat Config
// baselines: a bite's damage and an alien's pacing should come from
// w.alienSpecies.
func TestBiteUsesRolledSpeciesDamage(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	alien := w.spawn(Alien, Point{0, 0})
	victim := w.spawn(Colonist, Point{1, 0})
	victim.HP = 999
	victim.Parts = [numBodyParts]int{999, 999, 999, 999, 999, 999}

	w.bite(alien, victim)

	if got, want := victim.HP, 999-w.alienSpecies.BiteDamage; got != want {
		t.Fatalf("victim HP after bite = %d, want %d (999 - species bite damage %d)", got, want, w.alienSpecies.BiteDamage)
	}
}

// A world built through the normal generate() path and a Snapshot taken from
// it must agree on which species this seed rolled.
func TestSnapshotExposesAlienSpecies(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)
	snap := w.snapshot(false, 1)
	if snap.AlienSpecies != w.alienSpecies {
		t.Fatalf("snapshot species = %+v, want %+v", snap.AlienSpecies, w.alienSpecies)
	}
}
