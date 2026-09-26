package sim

import (
	"math/rand"
	"testing"
)

// Rolling the same seed twice must produce the exact same species roster:
// this is gameplay (bite damage, combat behavior), not flavor, so it has to
// be reproducible the way everything else on a seed is (see AGENTS.md).
func TestRollAlienSpeciesRosterIsDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 4
	a := rollAlienSpeciesRoster(rand.New(rand.NewSource(12345)), cfg)
	b := rollAlienSpeciesRoster(rand.New(rand.NewSource(12345)), cfg)
	if len(a) != len(b) {
		t.Fatalf("same seed rolled rosters of different length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("same seed rolled different species at %d:\n%+v\n%+v", i, a[i], b[i])
		}
	}
}

// AlienSpeciesCount decides how many species a world rolls, and a value below
// 1 (a misconfiguration) must not leave a world with no species at all.
func TestRollAlienSpeciesRosterHonorsCount(t *testing.T) {
	cfg := DefaultConfig()
	for _, tc := range []struct{ configured, want int }{
		{1, 1}, {3, 3}, {0, 1}, {-5, 1},
	} {
		cfg.AlienSpeciesCount = tc.configured
		roster := rollAlienSpeciesRoster(rand.New(rand.NewSource(1)), cfg)
		if len(roster) != tc.want {
			t.Fatalf("AlienSpeciesCount %d rolled a roster of %d, want %d", tc.configured, len(roster), tc.want)
		}
	}
}

// Every rolled species must satisfy its own invariants regardless of what
// the RNG stream happens to draw: valid ranges, arms never outnumbering
// limbs, a temperament/skin actually in their enums, and derived combat
// stats that never round down to something unusable.
func TestRollAlienSpeciesInvariants(t *testing.T) {
	cfg := DefaultConfig()
	names := defaultAlienNames()
	for seed := int64(0); seed < 500; seed++ {
		sp := rollAlienSpecies(rand.New(rand.NewSource(seed)), cfg, names)
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
		if sp.Legs() != sp.Limbs-sp.Arms || sp.Legs() < 0 {
			t.Fatalf("seed %d: legs %d inconsistent with limbs %d / arms %d", seed, sp.Legs(), sp.Limbs, sp.Arms)
		}
		switch sp.Temperament {
		case TemperamentFriendly, TemperamentCautious, TemperamentHostile:
		default:
			t.Fatalf("seed %d: unknown temperament %v", seed, sp.Temperament)
		}
		switch sp.Skin {
		case SkinSmooth, SkinScaly, SkinFurry, SkinArmored, SkinBony, SkinChitinous, SkinSlimy:
		default:
			t.Fatalf("seed %d: unknown skin %v", seed, sp.Skin)
		}
		switch sp.Pattern {
		case PatternSolid, PatternStriped, PatternSpotted:
		default:
			t.Fatalf("seed %d: unknown pattern %v", seed, sp.Pattern)
		}
		if sp.Color == "" {
			t.Fatalf("seed %d: species has no color", seed)
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
	names := defaultAlienNames()
	seen := map[AlienSpecies]bool{}
	for seed := int64(0); seed < 100; seed++ {
		seen[rollAlienSpecies(rand.New(rand.NewSource(seed)), cfg, names)] = true
	}
	if len(seen) < 20 {
		t.Fatalf("only %d distinct species across 100 seeds, want plenty of variety", len(seen))
	}
}

// Friendly is meant to be the rare tier and cautious/hostile the common,
// roughly even split the ask described -- not exact odds, but nowhere close
// to uniform or inverted, checked over a large enough sample to be stable.
func TestRollTemperamentDistribution(t *testing.T) {
	counts := map[AlienTemperament]int{}
	const n = 20000
	for seed := int64(0); seed < n; seed++ {
		counts[rollTemperament(rand.New(rand.NewSource(seed)))]++
	}
	friendly, cautious, hostile := counts[TemperamentFriendly], counts[TemperamentCautious], counts[TemperamentHostile]
	if friendly == 0 || friendly > n/5 {
		t.Fatalf("friendly = %d/%d, want rare (a small minority)", friendly, n)
	}
	if cautious < n/5 || hostile < n/5 {
		t.Fatalf("cautious = %d, hostile = %d out of %d, want both common", cautious, hostile, n)
	}
}

// A configured AlienDamage of 0 (how combat_test.go neuters an alien's bite
// to isolate other behavior) must stay 0 after size-scaling, not get floored
// back up to 1 -- scaling must not turn "off" into "on".
func TestSpeciesDamageZeroBaselinePassesThrough(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienDamage = 0
	names := defaultAlienNames()
	for seed := int64(0); seed < 50; seed++ {
		sp := rollAlienSpecies(rand.New(rand.NewSource(seed)), cfg, names)
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

// scaledByTemperament must reproduce the baseline exactly for Cautious, and
// move Hostile and Friendly symmetrically to either side of it: a hostile
// species is faster, a friendly one calmer/slower.
func TestScaledByTemperament(t *testing.T) {
	base := 10
	if got := scaledByTemperament(base, TemperamentCautious); got != base {
		t.Fatalf("cautious = %d, want the unscaled baseline %d", got, base)
	}
	friendly := scaledByTemperament(base, TemperamentFriendly)
	hostile := scaledByTemperament(base, TemperamentHostile)
	if friendly <= base {
		t.Fatalf("friendly = %d, want slower (larger) than baseline %d", friendly, base)
	}
	if hostile >= base {
		t.Fatalf("hostile = %d, want faster (smaller) than baseline %d", hostile, base)
	}
}

// Combat must actually read the biting alien's own assigned species rather
// than a single flat Config baseline: a bite's damage should come from
// w.alienSpeciesFor(alien).
func TestBiteUsesRolledSpeciesDamage(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	alien := w.spawn(Alien, Point{0, 0})
	victim := w.spawn(Colonist, Point{1, 0})
	victim.HP = 999
	victim.Parts = [numBodyParts]int{999, 999, 999, 999, 999, 999}

	w.bite(alien, victim)

	dmg := w.alienSpeciesFor(alien).BiteDamage
	if got, want := victim.HP, 999-dmg; got != want {
		t.Fatalf("victim HP after bite = %d, want %d (999 - species bite damage %d)", got, want, dmg)
	}
}

// Every spawned Alien must be assigned a valid index into the world's rolled
// species roster, whatever AlienSpeciesCount was configured.
func TestSpawnedAliensGetAValidSpecies(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.AlienSpeciesCount = 5
	w := newTestWorld(t, cfg)

	if got := len(w.alienSpecies); got != 5 {
		t.Fatalf("world rolled %d species, want 5", got)
	}
	for i := 0; i < 100; i++ {
		a := w.spawn(Alien, Point{i % w.Width, 0})
		if a.Species < 0 || a.Species >= len(w.alienSpecies) {
			t.Fatalf("alien %d assigned out-of-range species %d (roster has %d)", a.ID, a.Species, len(w.alienSpecies))
		}
	}
}

// A Friendly alien must never initiate combat: left alone with an adjacent
// colonist and stepped repeatedly, it should never bite (the colonist's own
// HP never drops) and should never report Hunting.
func TestFriendlyAlienNeverInitiatesCombat(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	w.alienSpecies[0].Temperament = TemperamentFriendly

	alien := w.spawn(Alien, Point{5, 5})
	victim := w.spawn(Colonist, Point{6, 5}) // already adjacent
	startHP := victim.HP

	for i := 0; i < 50; i++ {
		w.alienTurn(alien)
		if alien.State == Hunting {
			t.Fatalf("tick %d: friendly alien is Hunting, want it to never initiate", i)
		}
	}
	if victim.HP != startHP {
		t.Fatalf("victim HP changed from %d to %d: a friendly alien bit it", startHP, victim.HP)
	}
}

// A Cautious alien must not chase a colonist outside Config.AlienCautiousRadius
// (it only wanders), but must react once one comes within it.
func TestCautiousAlienOnlyReactsWithinRadius(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.AlienCautiousRadius = 3
	w := newTestWorld(t, cfg)
	w.alienSpecies[0].Temperament = TemperamentCautious
	carve(w, Point{2, 5}, Point{5 + cfg.AlienCautiousRadius + 5, 5}, Floor) // one room, so both are reachable
	w.refreshSpatial()

	alien := w.spawn(Alien, Point{5, 5})
	far := w.spawn(Colonist, Point{5 + cfg.AlienCautiousRadius + 5, 5})

	w.alienTurn(alien)
	if alien.Quarry == far.ID {
		t.Fatalf("cautious alien targeted a colonist %d tiles away, outside its radius %d",
			alien.Pos.Chebyshev(far.Pos), cfg.AlienCautiousRadius)
	}

	near := w.spawn(Colonist, Point{6, 5}) // adjacent, well within the radius
	alien.Cooldown = 0                     // isolate this decision from the previous turn's pacing
	w.alienTurn(alien)
	if alien.Quarry != near.ID {
		t.Fatalf("cautious alien did not react to a colonist adjacent to it")
	}
}

// A Hostile alien must hunt a colonist anywhere it can walk to,
// unconditionally, however far away -- but, walking the floor like everyone
// else, never one sealed off behind rock.
func TestHostileAlienHuntsAcrossTheMap(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	w.alienSpecies[0].Temperament = TemperamentHostile
	carve(w, Point{1, 1}, Point{w.Width - 2, 1}, Floor)
	carve(w, Point{w.Width - 2, 1}, Point{w.Width - 2, w.Height - 2}, Floor)
	w.refreshSpatial()

	alien := w.spawn(Alien, Point{1, 1})
	sealed := w.spawn(Colonist, Point{w.Width / 2, w.Height / 2}) // still the landing cave, closer
	prey := w.spawn(Colonist, Point{w.Width - 2, w.Height - 2})

	w.alienTurn(alien)
	if alien.Quarry != prey.ID {
		t.Fatalf("hostile alien targeted #%d, want the far colonist #%d in its room, not #%d behind rock",
			alien.Quarry, prey.ID, sealed.ID)
	}
	start := alien.Pos
	for i := 0; i < 20; i++ {
		alien.Cooldown = 0
		w.alienTurn(alien)
		if !w.Walkable(alien.Pos) {
			t.Fatalf("alien left the floor for %v", alien.Pos)
		}
	}
	if alien.Pos.Chebyshev(prey.Pos) >= start.Chebyshev(prey.Pos) {
		t.Fatalf("alien made no progress toward its prey: %v -> %v", start, alien.Pos)
	}
}

// A world built through the normal generate() path and a Snapshot taken from
// it must agree on the rolled species roster, and a living Alien's
// EntityView must carry the species it was actually assigned.
func TestSnapshotExposesAlienSpecies(t *testing.T) {
	cfg := testConfig()
	cfg.AlienSpeciesCount = 3
	w := newTestWorld(t, cfg)
	alien := w.spawn(Alien, Point{0, 0})

	snap := w.snapshot(false, 1)
	if len(snap.AlienSpecies) != len(w.alienSpecies) {
		t.Fatalf("snapshot roster has %d species, want %d", len(snap.AlienSpecies), len(w.alienSpecies))
	}
	for i := range snap.AlienSpecies {
		if snap.AlienSpecies[i] != w.alienSpecies[i] {
			t.Fatalf("snapshot species %d = %+v, want %+v", i, snap.AlienSpecies[i], w.alienSpecies[i])
		}
	}

	var view *EntityView
	for i := range snap.Entities {
		if snap.Entities[i].ID == alien.ID {
			view = &snap.Entities[i]
		}
	}
	if view == nil {
		t.Fatal("spawned alien missing from snapshot")
	}
	if view.AlienSpecies != w.alienSpeciesFor(alien) {
		t.Fatalf("entity view species = %+v, want %+v", view.AlienSpecies, w.alienSpeciesFor(alien))
	}
}

// Snapshot.Seed is the one fact the lore panel shows that isn't derived from
// anything else -- it must be the actual seed a run started from.
func TestSnapshotExposesSeed(t *testing.T) {
	cfg := testConfig()
	cfg.Seed = 918273
	w := newTestWorld(t, cfg)

	if got := w.snapshot(false, 1).Seed; got != cfg.Seed {
		t.Fatalf("snapshot seed = %d, want %d", got, cfg.Seed)
	}
}

// RosterLabel prefixes the species' rolled emoji when it has one, and
// otherwise reads exactly as it did before Emoji existed.
func TestRosterLabelIncludesEmojiWhenPresent(t *testing.T) {
	withEmoji := AlienSpecies{Singular: "xeno", Temperament: TemperamentHostile, Emoji: "🦎"}
	if got, want := withEmoji.RosterLabel(), "🦎 Xeno · hostile"; got != want {
		t.Fatalf("RosterLabel() = %q, want %q", got, want)
	}

	withoutEmoji := AlienSpecies{Singular: "xeno", Temperament: TemperamentHostile}
	if got, want := withoutEmoji.RosterLabel(), "Xeno · hostile"; got != want {
		t.Fatalf("RosterLabel() = %q, want %q", got, want)
	}
}
