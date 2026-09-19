package sim

import (
	"math"
	"testing"
)

// mutationWorld returns a world with no starting population, so tests place
// exactly the colonists and deposits they care about.
func mutationWorld(t *testing.T) *World {
	t.Helper()
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.IronRockPercent, cfg.IceRockPercent, cfg.UraniumRockPercent, cfg.ClayRockPercent = 0, 0, 0, 0
	return newTestWorld(t, cfg)
}

// Mining a uranium-bearing tile yields uranium ore alongside the raw rock, the
// same way iron and ice deposits do.
func TestMiningUraniumYieldsOre(t *testing.T) {
	yield := miningYield(Tile{Terrain: Rock, Composition: UraniumBearingRock})
	var ore int
	for _, stack := range yield {
		if stack.Kind == UraniumOre {
			ore += stack.Count
		}
	}
	if ore != 1 {
		t.Fatalf("uranium-bearing rock yielded %d uranium ore, want 1", ore)
	}
}

// Worldgen grows uranium veins to the configured abundance without disturbing
// the other deposits.
func TestWorldgenGrowsUraniumVeins(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 40, 30
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.IronRockPercent, cfg.IceRockPercent, cfg.UraniumRockPercent, cfg.ClayRockPercent = 10, 5, 3, 0
	cfg.Seed = 161803
	w := NewEngine(cfg).world

	count := 0
	for _, tile := range w.tiles {
		if tile.Composition == UraniumBearingRock {
			count++
		}
	}
	if want := len(w.tiles) * cfg.UraniumRockPercent / 100; count != want {
		t.Fatalf("uranium-bearing tiles = %d, want %d", count, want)
	}
}

// A colonist is under a dose while carrying ore, and while standing next to an
// unexcavated deposit — and under neither condition, is not.
func TestUraniumExposureSources(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})

	if w.uraniumExposed(e) {
		t.Fatal("a colonist away from uranium should not be exposed")
	}

	w.tiles[w.index(Point{6, 5})].Composition = UraniumBearingRock
	if !w.uraniumExposed(e) {
		t.Fatal("a colonist beside a uranium deposit should be exposed")
	}

	w.tiles[w.index(Point{6, 5})].Composition = OrdinaryRock
	e.Inventory.Add(UraniumOre, 1)
	if !w.uraniumExposed(e) {
		t.Fatal("a colonist carrying uranium ore should be exposed")
	}
}

// Exposure accumulates only while exposed, and a full dose is spent on a roll
// rather than latching, so a colonist that keeps working keeps rolling.
func TestUraniumExposureAccumulatesAndRolls(t *testing.T) {
	w := mutationWorld(t)
	w.cfg.MutationChance = 0 // isolate the counter from the mutation itself
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})

	for i := 0; i < 10; i++ {
		w.applyUraniumExposure(e)
	}
	if e.uraniumExposure != 0 {
		t.Fatalf("unexposed colonist accumulated %d exposure, want 0", e.uraniumExposure)
	}

	e.Inventory.Add(UraniumOre, 1)
	for i := 0; i < w.cfg.UraniumExposureTicks-1; i++ {
		w.applyUraniumExposure(e)
	}
	if e.uraniumExposure != w.cfg.UraniumExposureTicks-1 {
		t.Fatalf("exposure = %d, want %d", e.uraniumExposure, w.cfg.UraniumExposureTicks-1)
	}
	w.applyUraniumExposure(e) // completes the dose: rolls and resets
	if e.uraniumExposure != 0 {
		t.Fatalf("a completed dose should reset the counter, got %d", e.uraniumExposure)
	}
}

// A full dose with a certain roll mutates: the colonist grows a body part it
// was not born with, gains HP for it, and picks up the Mutant trait. Resizing
// is switched off here so the part's own arithmetic is visible on its own; the
// stature tests below cover the other half of a mutation.
func TestMutationGrowsPartAndTrait(t *testing.T) {
	w := mutationWorld(t)
	w.cfg.MutationChance = 100
	w.cfg.MutationStaturePercent = 0
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})
	e.Inventory.Add(UraniumOre, 1)
	beforeHP, beforeMax := e.HP, e.MaxHP

	for i := 0; i < w.cfg.UraniumExposureTicks; i++ {
		w.applyUraniumExposure(e)
	}

	grown := BodyPart(0)
	for _, part := range mutantParts {
		if e.hasPart(part) {
			grown = part
		}
	}
	if grown == 0 {
		t.Fatal("a certain mutation grew no mutant part")
	}
	if !e.Profile.HasTrait(TraitMutant) {
		t.Fatal("a mutated colonist should carry the Mutant trait")
	}
	if e.MaxHP <= beforeMax || e.HP <= beforeHP {
		t.Fatalf("growing a part should add HP: %d/%d before, %d/%d after",
			beforeHP, beforeMax, e.HP, e.MaxHP)
	}
	if e.MaxHP-beforeMax != e.MaxParts[grown] {
		t.Fatalf("MaxHP grew by %d, but the new %s is worth %d",
			e.MaxHP-beforeMax, grown, e.MaxParts[grown])
	}
	if e.Parts[grown] != e.MaxParts[grown] {
		t.Fatalf("a freshly grown %s should be undamaged, got %d/%d",
			grown, e.Parts[grown], e.MaxParts[grown])
	}
	// Base anatomy is untouched: a mutation adds flesh, it does not redivide
	// the body a colonist already had.
	base := distributeBodyParts(beforeMax)
	for part := BodyPart(0); part < numBaseBodyParts; part++ {
		if e.MaxParts[part] != base[part] {
			t.Fatalf("mutation changed the %s from %d to %d", part, base[part], e.MaxParts[part])
		}
	}
}

// Mutation never runs out of control: every mutant part can be grown, and a
// colonist that has them all simply stops growing new ones.
func TestMutationStopsWhenEveryPartIsGrown(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})
	for i := 0; i < len(mutantParts)+5; i++ {
		w.mutate(e)
	}
	for _, part := range mutantParts {
		if !e.hasPart(part) {
			t.Fatalf("repeated mutation never grew a %s", part)
		}
	}
	if got := len(e.Profile.Traits); got != 1 {
		t.Fatalf("repeated mutation should add the Mutant trait once, got %v", e.Profile.Traits)
	}
}

// An attack can only land on a part the target actually has: a grown mutant
// part becomes a target, an ungrown one never does.
func TestRollHitOnlyLandsOnPartsTheTargetHas(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	plain := w.spawn(Colonist, Point{5, 5})
	w.SetTerrain(Point{7, 5}, Floor)
	mutant := w.spawn(Colonist, Point{7, 5})
	w.growPart(mutant, Tail)

	for i := 0; i < 500; i++ {
		if part := w.rollHit(plain); part.Mutant() {
			t.Fatalf("hit an unmutated colonist's %s", part)
		}
	}
	sawTail := false
	for i := 0; i < 500; i++ {
		switch part := w.rollHit(mutant); {
		case part == Tail:
			sawTail = true
		case part.Mutant():
			t.Fatalf("hit a %s the mutant never grew", part)
		}
	}
	if !sawTail {
		t.Fatal("500 attacks on a tailed mutant never once hit the tail")
	}
}

// A mutant-lover warms to a mutant faster than the mutant warms back: the
// bonus is a fact about the admirer, so affinity moves by different amounts in
// the two directions.
func TestMutantLoverGainsExtraAffinityTowardMutants(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	w.SetTerrain(Point{6, 5}, Floor)
	lover := w.spawn(Colonist, Point{5, 5})
	mutant := w.spawn(Colonist, Point{6, 5})
	w.giveTrait(lover, TraitMutantLover)
	w.giveTrait(mutant, TraitMutant)

	w.finishTalk(lover, mutant)

	toward := w.affinityBetween(lover.ID, mutant.ID)
	back := w.affinityBetween(mutant.ID, lover.ID)
	if toward-back != w.cfg.MutantLoverAffinityBonus {
		t.Fatalf("lover->mutant %d and mutant->lover %d differ by %d, want the %d bonus",
			toward, back, toward-back, w.cfg.MutantLoverAffinityBonus)
	}
}

// Without the trait pairing, a conversation still moves both directions
// equally: the bonus is the only thing that can make affinity asymmetric.
func TestOrdinaryConversationStaysSymmetric(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	w.SetTerrain(Point{6, 5}, Floor)
	a := w.spawn(Colonist, Point{5, 5})
	b := w.spawn(Colonist, Point{6, 5})
	w.giveTrait(a, TraitMutantLover) // a lover with no mutant to admire

	w.finishTalk(a, b)

	if x, y := w.affinityBetween(a.ID, b.ID), w.affinityBetween(b.ID, a.ID); x != y {
		t.Fatalf("affinity a->b %d != b->a %d without a mutant in the pair", x, y)
	}
}

// Mutation costs ordinary colonists grip; Mutant-Lovers reflect that axis.
func TestMutationGripDependsOnMutantLoverTrait(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	w.SetTerrain(Point{6, 5}, Floor)
	plain := w.spawn(Colonist, Point{5, 5})
	lover := w.spawn(Colonist, Point{6, 5})
	w.giveTrait(lover, TraitMutantLover)

	w.mutate(plain)
	w.mutate(lover)

	if plain.affect.Grip >= 0 {
		t.Fatalf("ordinary mutation grip = %d, want negative", plain.affect.Grip)
	}
	if lover.affect.Grip <= 0 {
		t.Fatalf("mutant-lover mutation grip = %d, want positive", lover.affect.Grip)
	}
}

// Mutation is gameplay, not flavor, so it must run on the simulation RNG: two
// runs of the same seed mutate the same colonists at the same ticks.
func TestMutationIsDeterministicForASeed(t *testing.T) {
	run := func() []string {
		cfg := testConfig()
		cfg.Width, cfg.Height = 40, 30
		cfg.UraniumRockPercent = 20 // plenty of uranium, so mutations actually happen
		cfg.UraniumExposureTicks = 1
		cfg.MutationChance = 100
		cfg.Seed = 999
		w := NewEngine(cfg).world
		// Guarantee that this RNG test reaches mutation independently of which
		// rock the evolving focus system chooses to mine first.
		for _, id := range w.entityIDsSorted() {
			if e := w.entities[id]; e.Kind == Colonist {
				e.Inventory = Inventory{}
				if !e.Inventory.Add(UraniumOre, 1) {
					t.Fatal("could not equip deterministic uranium source")
				}
				break
			}
		}
		w.step()
		var out []string
		for _, id := range w.entityIDsSorted() {
			e := w.entities[id]
			if e.Kind == Colonist && e.Profile.HasTrait(TraitMutant) {
				out = append(out, e.displayName())
			}
		}
		return out
	}
	first, second := run(), run()
	if len(first) == 0 {
		t.Fatal("guaranteed uranium dose did not mutate a colonist")
	}
	if len(first) != len(second) {
		t.Fatalf("same seed mutated %d colonists then %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("same seed mutated %v then %v", first, second)
		}
	}
}

// Mutation is meant to be a rarity, and the defaults are what make it one.
// Because exposure never decays, a short MutationChance is not enough on its
// own — an earlier balance pass mutated ~70% of a colony — so this pins the
// outcome the defaults are tuned for rather than the individual knobs. See
// "Tuning the rate" in docs/mutation.md.
func TestDefaultsKeepMutationRare(t *testing.T) {
	mutants, total := 0, 0
	for seed := int64(1); seed <= 4; seed++ {
		cfg := DefaultConfig()
		cfg.Seed = seed * 7919
		cfg.Width, cfg.Height = 80, 40
		cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 30, 0, 0, 0
		w := NewEngine(cfg).world
		for i := 0; i < 5000; i++ {
			w.step()
		}
		for _, id := range w.entityIDsSorted() {
			e := w.entities[id]
			if e.Kind != Colonist {
				continue
			}
			total++
			if e.Profile.HasTrait(TraitMutant) {
				mutants++
			}
		}
	}
	// Generous headroom over the ~1% the defaults aim for: this is a guard
	// against mutation becoming routine again, not a pin on an exact rate.
	if limit := total / 10; mutants > limit {
		t.Fatalf("%d of %d colonists mutated under the default config; "+
			"mutation should stay rare (at most %d here)", mutants, total, limit)
	}
}

// Every mutation resizes the colonist by the configured step, and the body
// goes with them: weight keeps their build, and HP and every body part scale
// so a taller colonist is genuinely a bigger one.
func TestMutationResizesColonistAndBody(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})
	e.Profile.HeightCM, e.Profile.WeightKG = 180, 80
	e.Profile.BornHeightCM, e.Profile.BornWeightKG = 180, 80
	beforeHP, beforeMax := e.HP, e.MaxHP
	beforeTorso := e.MaxParts[Torso]

	w.mutate(e)

	step := w.cfg.MutationStaturePercent
	grown, shrunk := scaleRound(180, 100+step, 100), scaleRound(180, 100, 100+step)
	after := e.Profile.HeightCM
	if after != grown && after != shrunk {
		t.Fatalf("mutation moved height to %d cm, want %d (grown) or %d (shrunk)", after, grown, shrunk)
	}
	if want := scaleRound(80*after, after, 180*180); e.Profile.WeightKG != want {
		t.Fatalf("weight = %d kg at %d cm, want %d (unchanged build)", e.Profile.WeightKG, after, want)
	}
	// A shrinking colonist also grew a part on this same mutation, so compare
	// direction rather than exact totals: the body must move with the height.
	if after > 180 && (e.MaxHP <= beforeMax || e.MaxParts[Torso] <= beforeTorso) {
		t.Fatalf("stretching to %d cm left MaxHP %d->%d and torso %d->%d",
			after, beforeMax, e.MaxHP, beforeTorso, e.MaxParts[Torso])
	}
	if after < 180 && e.MaxParts[Torso] >= beforeTorso {
		t.Fatalf("shrinking to %d cm left the torso at %d->%d", after, beforeTorso, e.MaxParts[Torso])
	}
	if e.HP <= 0 || e.HP > e.MaxHP || beforeHP <= 0 {
		t.Fatalf("resize left HP %d/%d", e.HP, e.MaxHP)
	}
}

// A resize never heals or wounds: an undamaged colonist stays whole, and a
// damaged one keeps roughly the same share of their body intact.
func TestResizeKeepsWoundsProportional(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})
	applyDamage(e, LeftArm, e.Parts[LeftArm]) // destroyed outright
	applyDamage(e, Torso, e.Parts[Torso]/2)   // half gone
	beforeShare := float64(e.Parts[Torso]) / float64(e.MaxParts[Torso])

	w.setStature(e, e.Profile.HeightCM*2)

	if e.Parts[LeftArm] != 0 {
		t.Fatalf("a destroyed arm grew back to %d on resize", e.Parts[LeftArm])
	}
	if !e.hasPart(LeftArm) {
		t.Fatal("a destroyed arm stopped being a part the colonist has")
	}
	if share := float64(e.Parts[Torso]) / float64(e.MaxParts[Torso]); math.Abs(share-beforeShare) > 0.02 {
		t.Fatalf("torso was %.2f intact before the resize and %.2f after", beforeShare, share)
	}
}

// Shrinking to the floor never amputates: every part the colonist has stays a
// part they have, however small it gets.
func TestShrinkingNeverRoundsAPartAway(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})
	w.growPart(e, Tail)
	w.setStature(e, w.cfg.StatureMinCM)

	for part := BodyPart(0); part < numBodyParts; part++ {
		if part == Tail || part < numBaseBodyParts {
			if !e.hasPart(part) {
				t.Fatalf("shrinking to %d cm cost the colonist their %s", w.cfg.StatureMinCM, part)
			}
		}
	}
	if e.HP < 1 || e.MaxHP < 1 {
		t.Fatalf("shrinking left the colonist at %d/%d HP", e.HP, e.MaxHP)
	}
}

// Growing and shrinking are each other's inverse, so a colonist stretched and
// shrunk back over and over ends up exactly where they started — the same
// height, and the same weight, which is recomputed from the body they were
// born with rather than ground down by rescaling a rounded integer.
func TestWeightSurvivesRepeatedResizing(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})
	height, weight := e.Profile.HeightCM, e.Profile.WeightKG

	for i := 0; i < 50; i++ {
		w.setStature(e, w.grownStature(e.Profile.HeightCM))
		w.setStature(e, w.shrunkStature(e.Profile.HeightCM))
	}

	if e.Profile.HeightCM != height {
		t.Fatalf("50 stretch/shrink cycles left the colonist at %d cm, want %d back where they started",
			e.Profile.HeightCM, height)
	}
	if e.Profile.WeightKG != weight {
		t.Fatalf("after 50 stretch/shrink cycles back at %d cm the colonist weighs %d kg, want %d",
			height, e.Profile.WeightKG, weight)
	}
}

// Uranium has no opinion about direction while both are open, but a colonist
// pinned at a limit always goes the other way rather than wasting the dose.
func TestStatureRollDirections(t *testing.T) {
	w := mutationWorld(t)

	var up, down int
	for i := 0; i < 200; i++ {
		switch h := w.rollStature(180); {
		case h > 180:
			up++
		case h < 180:
			down++
		default:
			t.Fatalf("a colonist in the middle of the range did not resize at all (%d cm)", h)
		}
	}
	if up == 0 || down == 0 {
		t.Fatalf("200 rolls from 180 cm went up %d times and down %d", up, down)
	}
	if h := w.rollStature(w.cfg.StatureMaxCM); h >= w.cfg.StatureMaxCM {
		t.Fatalf("a colonist at the ceiling rolled %d cm, want a shrink", h)
	}
	if h := w.rollStature(w.cfg.StatureMinCM); h <= w.cfg.StatureMinCM {
		t.Fatalf("a colonist at the floor rolled %d cm, want a growth", h)
	}
}

// The whole point: a long enough career at the vein produces both ends of the
// colony — a ten-foot colonist and a two-foot one — and never anything outside
// the configured limits.
func TestRepeatedMutationReachesBothExtremes(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})

	var tallest, shortest = e.Profile.HeightCM, e.Profile.HeightCM
	for i := 0; i < 2000; i++ {
		w.mutate(e)
		h := e.Profile.HeightCM
		if h < w.cfg.StatureMinCM || h > w.cfg.StatureMaxCM {
			t.Fatalf("mutation put a colonist at %d cm, outside [%d, %d]",
				h, w.cfg.StatureMinCM, w.cfg.StatureMaxCM)
		}
		tallest, shortest = max(tallest, h), min(shortest, h)
	}
	if tallest != w.cfg.StatureMaxCM {
		t.Fatalf("2000 mutations peaked at %d cm, never reaching the %d cm ceiling", tallest, w.cfg.StatureMaxCM)
	}
	if shortest != w.cfg.StatureMinCM {
		t.Fatalf("2000 mutations bottomed out at %d cm, never reaching the %d cm floor", shortest, w.cfg.StatureMinCM)
	}
	if got := FormatHeight(w.cfg.StatureMaxCM); got != `10'0"` {
		t.Fatalf("the ceiling reads %s, want 10'0\"", got)
	}
	if got := FormatHeight(w.cfg.StatureMinCM); got != `2'0"` {
		t.Fatalf("the floor reads %s, want 2'0\"", got)
	}
}

// A colonist who has grown every mutant part still mutates: uranium goes on
// resizing them, and it is still a memory-worthy event that costs them grip.
func TestFullyGrownMutantStillResizes(t *testing.T) {
	w := mutationWorld(t)
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})
	for _, part := range mutantParts {
		w.growPart(e, part)
	}
	height, memories := e.Profile.HeightCM, len(e.Memories)

	w.mutate(e)

	if e.Profile.HeightCM == height {
		t.Fatalf("a fully grown mutant was not resized (still %d cm)", height)
	}
	if len(e.Memories) != memories+1 {
		t.Fatalf("a resize-only mutation recorded %d memories, want 1", len(e.Memories)-memories)
	}
	if last := e.Memories[len(e.Memories)-1]; last.Kind != EvtMutated {
		t.Fatalf("a resize-only mutation remembered %v, want EvtMutated", last.Kind)
	}
	if e.affect.Grip >= 0 {
		t.Fatalf("a resize-only mutation left grip at %d, want body horror", e.affect.Grip)
	}
}

// With resizing switched off, mutation is exactly what it was before stature
// existed: parts only, and a colonist with all four stops mutating.
func TestStatureCanBeDisabled(t *testing.T) {
	w := mutationWorld(t)
	w.cfg.MutationStaturePercent = 0
	w.SetTerrain(Point{5, 5}, Floor)
	e := w.spawn(Colonist, Point{5, 5})
	height := e.Profile.HeightCM

	for i := 0; i < len(mutantParts)+3; i++ {
		w.mutate(e)
	}
	if e.Profile.HeightCM != height {
		t.Fatalf("resizing is disabled but the colonist moved from %d to %d cm", height, e.Profile.HeightCM)
	}
	if len(e.Memories) != len(mutantParts) {
		t.Fatalf("%d mutations with every part grown, want %d", len(e.Memories), len(mutantParts))
	}
}

// Heights render in the units the colony talks in, rounded in inches so no
// colonist is ever 5'12".
func TestFormatHeight(t *testing.T) {
	cases := []struct {
		cm   int
		want string
	}{
		{61, `2'0"`},
		{145, `4'9"`},
		{178, `5'10"`},
		{182, `6'0"`},
		{305, `10'0"`},
	}
	for _, tc := range cases {
		if got := FormatHeight(tc.cm); got != tc.want {
			t.Errorf("FormatHeight(%d) = %s, want %s", tc.cm, got, tc.want)
		}
	}
}
