package sim

import (
	"slices"
	"strings"
	"testing"
)

// Rolling the same seed twice must produce the exact same species roster:
// this is gameplay (bite damage, combat behavior), not flavor, so it has to
// be reproducible the way everything else on a seed is (see AGENTS.md).
func TestRollAlienSpeciesRosterIsDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 4
	a := rollAlienSpeciesRoster(newRand(12345), cfg)
	b := rollAlienSpeciesRoster(newRand(12345), cfg)
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
		roster := rollAlienSpeciesRoster(newRand(1), cfg)
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
		sp := rollAlienSpecies(newRand(seed), cfg, names, nil)
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
		if !slices.Contains(alienSkins[:], sp.Skin) || sp.Skin.String() == "unknown" {
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
		if sp.AttackModes == 0 {
			t.Fatalf("seed %d: species rolled no attack modes", seed)
		}
		for _, m := range sp.Attacks() {
			if !sp.canUse(m) {
				t.Fatalf("seed %d: species (arms %d, tail %v) rolled attack %v its anatomy forbids",
					seed, sp.Arms, sp.Tail, m)
			}
		}
	}
}

// Across many seeds every attack mode shows up somewhere, so no mode is
// accidentally unreachable through the anatomy gates.
func TestRollAttackModesReachesEveryMode(t *testing.T) {
	cfg := DefaultConfig()
	names := defaultAlienNames()
	var seen AttackSet
	for seed := int64(0); seed < 200; seed++ {
		seen |= rollAlienSpecies(newRand(seed), cfg, names, nil).AttackModes
	}
	for _, m := range attackModes {
		if !seen.Has(m) {
			t.Errorf("attack mode %v never rolled across 200 seeds", m)
		}
	}
}

// A tail-less, armless species can only bite, whatever the coin flips say.
func TestRollAttackModesRespectsAnatomy(t *testing.T) {
	sp := AlienSpecies{Limbs: 4, Arms: 0, Tail: false}
	for seed := int64(0); seed < 50; seed++ {
		if got := rollAttackModes(newRand(seed), sp); got != AttackSetOf(AttackBite) {
			t.Fatalf("seed %d: armless, tailless species rolled %v, want bite only", seed, got)
		}
	}
}

// Strangling goes for the throat at half damage, and draws no hit roll.
func TestStrangleHitsTheHeadAtHalfDamage(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	alien := w.spawn(Alien, Point{0, 0, LandingLevel})
	sp := &w.alienSpecies[alien.Species]
	sp.AttackModes = AttackSetOf(AttackStrangle)
	sp.BiteDamage = 6
	victim := w.spawn(Colonist, Point{1, 0, LandingLevel})
	victim.HP = 999
	victim.Parts = [numBodyParts]int{999, 999, 999, 999, 999, 999}

	w.strike(alien, victim)

	if victim.Parts[Head] != 996 || victim.HP != 996 {
		t.Fatalf("after strangle: head %d, HP %d, want both 996 (3 damage to the head)", victim.Parts[Head], victim.HP)
	}
	if got, want := lastMemory(victim), "Half-strangled by "+w.alienNounFor(alien)+"!"; got != want {
		t.Fatalf("victim memory = %q, want %q", got, want)
	}
}

// Each mode narrates its own blow.
func TestStrikeTargetTextByMode(t *testing.T) {
	cases := map[AttackMode]string{
		AttackBite:     "Bitten in the torso by a grelk!",
		AttackClaw:     "Clawed across the torso by a grelk!",
		AttackTail:     "Lashed across the torso by a grelk's tail!",
		AttackStrangle: "Half-strangled by a grelk!",
	}
	for mode, want := range cases {
		if got := strikeTargetText(mode, Torso, "a grelk"); got != want {
			t.Errorf("%v: %q, want %q", mode, got, want)
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
		seen[rollAlienSpecies(newRand(seed), cfg, names, nil)] = true
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
		counts[rollTemperament(newRand(seed))]++
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
		sp := rollAlienSpecies(newRand(seed), cfg, names, nil)
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
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	alien := w.spawn(Alien, Point{0, 0, LandingLevel})
	w.alienSpecies[alien.Species].AttackModes = AttackSetOf(AttackBite) // strangling halves it
	victim := w.spawn(Colonist, Point{1, 0, LandingLevel})
	victim.HP = 999
	victim.Parts = [numBodyParts]int{999, 999, 999, 999, 999, 999}

	w.strike(alien, victim)

	dmg := w.alienSpeciesFor(alien).BiteDamage
	if got, want := victim.HP, 999-dmg; got != want {
		t.Fatalf("victim HP after bite = %d, want %d (999 - species bite damage %d)", got, want, dmg)
	}
}

// Every spawned Alien must be assigned a valid index into the world's rolled
// species roster, whatever AlienSpeciesCount was configured.
func TestSpawnedAliensGetAValidSpecies(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.AlienSpeciesCount = 5
	w := newTestWorld(t, cfg)

	if got := len(w.alienSpecies); got != 5 {
		t.Fatalf("world rolled %d species, want 5", got)
	}
	for i := 0; i < 100; i++ {
		a := w.spawn(Alien, Point{i % w.Width, 0, LandingLevel})
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
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	setAlienTemperament(w, 0, TemperamentFriendly)

	alien := w.spawn(Alien, Point{5, 5, LandingLevel})
	victim := w.spawn(Colonist, Point{6, 5, LandingLevel}) // already adjacent
	startHP := victim.HP

	for i := 0; i < 50; i++ {
		w.animalTurn(alien)
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
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.AlienCautiousRadius = 3
	w := newTestWorld(t, cfg)
	setAlienTemperament(w, 0, TemperamentCautious)
	carve(w, Point{2, 5, LandingLevel}, Point{5 + cfg.AlienCautiousRadius + 5, 5, LandingLevel}, Floor) // one room, so both are reachable
	w.refreshSpatial()

	alien := w.spawn(Alien, Point{5, 5, LandingLevel})
	far := w.spawn(Colonist, Point{5 + cfg.AlienCautiousRadius + 5, 5, LandingLevel})

	w.animalTurn(alien)
	if alien.Quarry == far.ID {
		t.Fatalf("cautious alien targeted a colonist %d tiles away, outside its radius %d",
			alien.Pos.Chebyshev(far.Pos), cfg.AlienCautiousRadius)
	}

	near := w.spawn(Colonist, Point{6, 5, LandingLevel}) // adjacent, well within the radius
	alien.Cooldown = 0                                   // isolate this decision from the previous turn's pacing
	w.animalTurn(alien)
	if alien.Quarry != near.ID {
		t.Fatalf("cautious alien did not react to a colonist adjacent to it")
	}
}

// A Hostile alien must hunt a colonist anywhere it can walk to,
// unconditionally, however far away -- but, walking the floor like everyone
// else, never one sealed off behind rock.
func TestHostileAlienHuntsAcrossTheMap(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	setAlienTemperament(w, 0, TemperamentHostile)
	carve(w, Point{1, 1, LandingLevel}, Point{w.Width - 2, 1, LandingLevel}, Floor)
	carve(w, Point{w.Width - 2, 1, LandingLevel}, Point{w.Width - 2, w.Height - 2, LandingLevel}, Floor)
	w.refreshSpatial()

	alien := w.spawn(Alien, Point{1, 1, LandingLevel})
	sealed := w.spawn(Colonist, Point{w.Width / 2, w.Height / 2, LandingLevel}) // still the landing cave, closer
	prey := w.spawn(Colonist, Point{w.Width - 2, w.Height - 2, LandingLevel})

	w.animalTurn(alien)
	if alien.Quarry != prey.ID {
		t.Fatalf("hostile alien targeted #%d, want the far colonist #%d in its room, not #%d behind rock",
			alien.Quarry, prey.ID, sealed.ID)
	}
	start := alien.Pos
	for i := 0; i < 20; i++ {
		alien.Cooldown = 0
		w.animalTurn(alien)
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
	alien := w.spawn(Alien, Point{0, 0, LandingLevel})

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

// A Hostile alien hunts rats and aliens of other species, not only colonists,
// and eats what it kills.
func TestHostileAlienHuntsRatsAndOtherSpecies(t *testing.T) {
	for _, preyKind := range []Kind{Rat, Alien} {
		t.Run(preyKind.String(), func(t *testing.T) {
			w := propertyWorld(t)
			setAlienTemperament(w, 0, TemperamentHostile)
			other := w.alienSpecies[0]
			other.Temperament = TemperamentFriendly // never fights back
			w.alienSpecies = append(w.alienSpecies, other)

			hunter := w.spawnAs(Alien, Point{6, 10, LandingLevel}, 0)
			prey := w.spawnAs(preyKind, Point{16, 10, LandingLevel}, 1)
			for i := 0; i < 400 && w.entities[prey.ID] != nil; i++ {
				w.animalTurn(hunter)
				if hunter.Quarry != 0 && hunter.Quarry != prey.ID {
					t.Fatalf("hunting %d, want the %s %d", hunter.Quarry, preyKind, prey.ID)
				}
			}
			if w.entities[prey.ID] != nil {
				t.Fatalf("the hostile alien never killed the %s", preyKind)
			}
		})
	}
}

// A Hostile alien never hunts its own species, so a nest does not eat itself.
func TestHostileAlienSparesItsOwnSpecies(t *testing.T) {
	w := propertyWorld(t)
	setAlienTemperament(w, 0, TemperamentHostile)
	a := w.spawnAs(Alien, Point{6, 10, LandingLevel}, 0)
	b := w.spawnAs(Alien, Point{7, 10, LandingLevel}, 0)
	for i := 0; i < 50; i++ {
		w.animalTurn(a)
		w.animalTurn(b)
	}
	if a.Quarry != 0 || b.Quarry != 0 || a.HP != a.MaxHP || b.HP != b.MaxHP {
		t.Fatalf("same-species aliens fought: quarries %d/%d, HP %d/%d", a.Quarry, b.Quarry, a.HP, b.HP)
	}
}

// Description reads as a field-guide entry whose framing follows the
// species' temperament, with sizes in metric and imperial.
func TestDescriptionByTemperament(t *testing.T) {
	cases := []struct {
		sp   AlienSpecies
		want string
	}{
		{
			AlienSpecies{Plural: "ets", HeightMinCM: 191, HeightMaxCM: 299, WeightMinKG: 35, WeightMaxKG: 54,
				Eyes: 4, Limbs: 5, Arms: 1, Skin: SkinChitinous, Color: "red", Temperament: TemperamentFriendly},
			`Ets stand 1.9-3 m (6'3"-9'10") tall, weighing 35-54 kg (77-119 lb). They have 4 eyes, 1 arm, and 4 legs. They are covered in red chitin and interact well with humans.`,
		},
		{
			AlienSpecies{Plural: "purples", HeightMinCM: 183, HeightMaxCM: 361, WeightMinKG: 43, WeightMaxKG: 85,
				Eyes: 3, Limbs: 4, Arms: 3, Skin: SkinSlimy, Color: "purple", Temperament: TemperamentCautious,
				AttackModes: AttackSetOf(AttackStrangle)},
			`Purples stand 1.8-3.6 m (6'0"-11'10") tall, weighing 43-85 kg (95-187 lb). They are skittish around humans; approach with caution. They can be recognized by their slimy purple skin, 3 eyes, 3 arms, and 1 leg. Get too close and they lash out by strangling with their arms.`,
		},
		{
			AlienSpecies{Plural: "xenos", HeightMinCM: 149, HeightMaxCM: 261, WeightMinKG: 103, WeightMaxKG: 181,
				Eyes: 5, Limbs: 6, Arms: 3, Skin: SkinBony, Color: "red", Temperament: TemperamentHostile,
				AttackModes: AttackSetOf(AttackClaw, AttackBite)},
			`The feared Xenos stand 1.5-2.6 m (4'11"-8'7") tall, weighing 103-181 kg (227-399 lb). They hunt humans with 5 eyes and 3 fearsome arms, and crawl on 3 legs. They kill by biting and raking with their claws. Their bony red skin blends into the Martian rock.`,
		},
		{
			AlienSpecies{Plural: "worms", HeightMinCM: 40, HeightMaxCM: 60, WeightMinKG: 3, WeightMaxKG: 5,
				Eyes: 1, Limbs: 2, Arms: 2, Tail: true, Skin: SkinScaly, Color: "blue", Pattern: PatternStriped,
				Temperament: TemperamentHostile, AttackModes: AttackSetOf(AttackBite, AttackTail, AttackStrangle)},
			`The feared Worms stand 0.4-0.6 m (1'4"-2'0") tall, weighing 3-5 kg (7-11 lb). They hunt humans with 1 eye and 2 fearsome arms, and slither along without legs. They kill by biting, thrashing their tails, and strangling with their arms. Their blue-striped scales stand out against the Martian rock.`,
		},
	}
	for _, c := range cases {
		if got := c.sp.Description(); got != c.want {
			t.Errorf("Description() =\n  %s\nwant\n  %s", got, c.want)
		}
	}
}

// A part a species has none of is left out of its description entirely.
func TestDescriptionOmitsMissingParts(t *testing.T) {
	sp := AlienSpecies{Plural: "blobs", HeightMinCM: 50, HeightMaxCM: 70, WeightMinKG: 10, WeightMaxKG: 12,
		Eyes: 2, Limbs: 3, Arms: 0, Skin: SkinSlimy, Color: "green", Temperament: TemperamentFriendly}
	want := `Blobs stand 0.5-0.7 m (1'8"-2'4") tall, weighing 10-12 kg (22-26 lb). They have 2 eyes and 3 legs. They are covered in slimy green skin and interact well with humans.`
	if got := sp.Description(); got != want {
		t.Errorf("Description() =\n  %s\nwant\n  %s", got, want)
	}
}

// Wings are anatomy: every framing of the description mentions them, and the
// hostile one says they stay folded, since nothing flies yet.
func TestDescriptionMentionsWings(t *testing.T) {
	base := AlienSpecies{Plural: "gargoyles", HeightMinCM: 150, HeightMaxCM: 200, WeightMinKG: 80, WeightMaxKG: 120,
		Eyes: 2, Limbs: 4, Arms: 2, Wings: true, Skin: SkinRocky, Color: "gray"}

	friendly := base
	friendly.Temperament = TemperamentFriendly
	want := `Gargoyles stand 1.5-2 m (4'11"-6'7") tall, weighing 80-120 kg (176-265 lb). They have 2 eyes, 2 arms, 2 legs, and a pair of wings. They are covered in craggy gray stone and interact well with humans.`
	if got := friendly.Description(); got != want {
		t.Errorf("friendly Description() =\n  %s\nwant\n  %s", got, want)
	}

	cautious := base
	cautious.Temperament = TemperamentCautious
	if got := cautious.Description(); !strings.Contains(got, "craggy gray stone, 2 eyes, 2 arms, 2 legs, and a pair of wings.") {
		t.Errorf("cautious Description() does not list the wings: %s", got)
	}

	hostile := base
	hostile.Temperament = TemperamentHostile
	if got := hostile.Description(); !strings.Contains(got, "and stride on 2 legs with their wings folded.") ||
		!strings.Contains(got, "Their craggy gray stone blends into the Martian rock.") {
		t.Errorf("hostile Description() = %s", got)
	}
}

// Every hide has its own covering phrase (only smooth uses the fallback), and
// a plural one (feathers) agrees its verb.
func TestCoveringPhraseForEveryHide(t *testing.T) {
	smooth, _ := AlienSpecies{Skin: SkinSmooth, Color: "red"}.coveringPhrase()
	seen := map[string]AlienSkin{}
	for _, skin := range alienSkins {
		got, _ := AlienSpecies{Skin: skin, Color: "red"}.coveringPhrase()
		if skin != SkinSmooth && got == smooth {
			t.Errorf("%v falls back to the smooth phrase %q", skin, got)
		}
		if prev, ok := seen[got]; ok {
			t.Errorf("%v and %v share the covering phrase %q", prev, skin, got)
		}
		seen[got] = skin
	}
	sp := AlienSpecies{Plural: "harpies", HeightMinCM: 100, HeightMaxCM: 140, WeightMinKG: 20, WeightMaxKG: 30,
		Eyes: 2, Limbs: 2, Arms: 0, Wings: true, Skin: SkinFeathered, Color: "black", Temperament: TemperamentHostile}
	if got := sp.Description(); !strings.HasSuffix(got, "Their black feathers stand out against the Martian rock.") {
		t.Errorf("feathered Description() does not agree its verb: %s", got)
	}
}

// Over many seeds the roll produces winged and wingless species, and every
// hide, so no new option is unreachable.
func TestRollReachesEveryHideAndWings(t *testing.T) {
	cfg := DefaultConfig()
	skins := map[AlienSkin]bool{}
	wings := map[bool]bool{}
	for seed := int64(0); seed < 500; seed++ {
		sp := rollAlienSpecies(newRand(seed), cfg, defaultAlienNames(), nil)
		skins[sp.Skin] = true
		wings[sp.Wings] = true
	}
	for _, skin := range alienSkins {
		if !skins[skin] {
			t.Errorf("no seed rolled a %v species", skin)
		}
	}
	if !wings[true] || !wings[false] {
		t.Errorf("wings rolled %v over 500 seeds, want both", wings)
	}
}
