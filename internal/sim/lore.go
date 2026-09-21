package sim

import (
	"fmt"
	"math/rand"
	"strings"
)

// ---- Lore: the world beyond the colony --------------------------------------
//
// World generation used to describe only the cavern and the handful of
// creatures in it. Lore is the start of a wider layer describing the world
// the colony landed in: right now that is just the one alien species native
// to it, but the same idea (roll something once per seed, off its own RNG
// stream, and let the sim read it back out) is meant to grow to cover
// organizations, corporations, other colonies, and whatever else the setting
// picks up later.
//
// rollAlienSpecies gives every world exactly one kind of alien: a build
// (height/weight range, eye and limb count, arms vs. legs, tail or not), a
// colloquial name ("xenos," "critters," "gremlins," ...), and a temperament
// from curious-and-passive to a relentless killing machine. Every Alien
// entity spawned in this world — at worldgen, by a director swarm, by a
// debug spawn — is one of this one rolled species; there is no per-entity
// variation yet (see Extending it in docs/lore.md).
//
// Lore rolls on its own seed-derived RNG stream, not w.rng or w.prng:
//   - Not w.prng, the personality stream, because a species' size and
//     temperament are not mere flavor — they set actual bite damage and
//     combat pace (see speciesDamage/scaledByAggression below), so they must
//     stay reproducible on the deterministic side of the personality/sim
//     split (see AGENTS.md).
//   - Not w.rng, the simulation stream, for the same reason growRockVeins'
//     compositionRNG (worldgen.go) is not: rolling the species must not
//     shift where anything spawns or any later w.rng-driven decision, so it
//     cannot share that stream's draw sequence.
//
// It is rolled in newWorld rather than generate() so that every World has a
// valid species regardless of which of the two a caller uses to build one —
// generate() is the normal path, but a number of tests build a World with
// newWorld directly and still spawn and fight Aliens against it.

// alienLoreSeed is this world's lore stream's XOR key against cfg.Seed,
// distinct from prng's, agePRNG's, and compositionRNG's own keys so none of
// the four ever draw from the same sequence.
const alienLoreSeed = 0x452821E638D01377

// AlienSpecies describes the one kind of alien native to a world: what it
// looks like, what colonists call it, and how dangerous it is. Rolled once by
// rollAlienSpecies and stored on World.alienSpecies; every Alien entity in
// the game is one of these. Exported so a Snapshot can carry a copy of it to
// frontends.
type AlienSpecies struct {
	// Singular and Plural are the colloquial words colonists use in place of
	// "alien"/"aliens" -- "xeno"/"xenos", "critter"/"critters", and so on.
	// Always lowercase; callers capitalize where a sentence needs it (see
	// capitalizeFirst).
	Singular string
	Plural   string

	// Adult size, as a range: no two specimens are identical, but every one
	// of them falls somewhere between these bounds. Bite damage is scaled
	// from the range's midpoint weight -- see speciesDamage.
	HeightMinCM, HeightMaxCM int
	WeightMinKG, WeightMaxKG int

	Eyes  int // how many eyes it has
	Limbs int // how many limbs it walks/grasps with, arms and legs together
	Arms  int // of Limbs, how many are prehensile arms rather than legs

	Tail bool

	// Aggression runs 0 (curious and passive) to 100 (a relentless killing
	// machine) -- the "E.T. to Xenomorph" scale the species is rolled
	// against. It scales bite pace and burrow speed (see
	// scaledByAggression), not damage: a vicious species is *faster*, not
	// simply harder-hitting -- size already owns hitting harder.
	Aggression int

	// BiteDamage, BiteRest, and Slowness are precomputed once at roll time
	// from Config's alien baselines (AlienDamage/AlienBiteRest/
	// AlienSlowness) so combat never recomputes them per hit or per turn.
	// See speciesDamage and scaledByAggression.
	BiteDamage int
	BiteRest   int
	Slowness   int
}

// alienNoun is one candidate name humans reach for instead of "alien."
type alienNoun struct{ singular, plural string }

// alienNouns is the fixed set rollAlienSpecies draws from. Adding a new one
// is a one-line edit here.
var alienNouns = []alienNoun{
	{"alien", "aliens"},
	{"ET", "ETs"},
	{"xeno", "xenos"},
	{"critter", "critters"},
	{"bug", "bugs"},
	{"gremlin", "gremlins"},
}

// rollAlienSpecies generates one alien species from rng, scaling its derived
// combat stats from cfg's alien baselines.
func rollAlienSpecies(rng *rand.Rand, cfg Config) AlienSpecies {
	noun := alienNouns[rng.Intn(len(alienNouns))]
	sp := AlienSpecies{
		Singular:   noun.singular,
		Plural:     noun.plural,
		Eyes:       1 + rng.Intn(6), // 1..6
		Limbs:      2 + rng.Intn(7), // 2..8
		Aggression: rng.Intn(101),   // 0..100
	}
	sp.Arms = rng.Intn(sp.Limbs + 1) // 0..Limbs; the rest are legs -- both
	// "all legs" (Arms == 0) and "all arms" (Arms == Limbs) are valid rolls.
	sp.Tail = rng.Intn(2) == 0

	baseHeight := 45 + rng.Intn(330) // a 45cm gremlin up to a ~375cm brute
	spread := 10 + rng.Intn(baseHeight/3+10)
	sp.HeightMinCM = max(20, baseHeight-spread)
	sp.HeightMaxCM = baseHeight + spread

	// Weight follows height through a per-species density (kg per cm),
	// rather than a fixed formula, so a species can read as wiry or
	// hulking independent of how tall it rolled.
	density := 0.15 + rng.Float64()*0.55
	sp.WeightMinKG = max(1, int(float64(sp.HeightMinCM)*density))
	sp.WeightMaxKG = max(sp.WeightMinKG+1, int(float64(sp.HeightMaxCM)*density))

	sp.BiteDamage = speciesDamage(sp, cfg)
	sp.BiteRest = scaledByAggression(cfg.AlienBiteRest, sp.Aggression)
	sp.Slowness = scaledByAggression(cfg.AlienSlowness, sp.Aggression)
	return sp
}

// speciesDamage scales cfg.AlienDamage by this species' size relative to
// cfg.AlienReferenceWeightKG, the weight at which a specimen deals exactly
// the configured baseline: a species heavier than the reference hits harder,
// a lighter one hits softer. A zero (or negative) baseline passes through
// unscaled rather than being floored to 1 -- that is how combat_test.go
// neuters an alien's bite to isolate other behavior, and scaling must not
// turn "off" back into "on."
func speciesDamage(sp AlienSpecies, cfg Config) int {
	if cfg.AlienDamage <= 0 {
		return cfg.AlienDamage
	}
	ref := cfg.AlienReferenceWeightKG
	if ref <= 0 {
		ref = 1
	}
	weight := (sp.WeightMinKG + sp.WeightMaxKG) / 2
	dmg := scaleRound(cfg.AlienDamage, weight, ref)
	if dmg < 1 {
		dmg = 1
	}
	return dmg
}

// scaledByAggression scales a baseline cooldown (AlienBiteRest or
// AlienSlowness) by aggression, 0..100: the midpoint, 50, reproduces base
// exactly; 0 (curious and passive) makes it half again slower; 100 (a
// relentless killer) makes it strike and move twice as fast. Aggression is
// deliberately not wired into damage -- see AlienSpecies.
func scaledByAggression(base, aggression int) int {
	v := scaleRound(base, 150-aggression, 100)
	if v < 1 {
		v = 1
	}
	return v
}

// AggressionTier is a short label for the roster's tight one-line slot.
func (sp AlienSpecies) AggressionTier() string {
	switch {
	case sp.Aggression < 15:
		return "docile"
	case sp.Aggression < 35:
		return "wary"
	case sp.Aggression < 60:
		return "opportunistic"
	case sp.Aggression < 85:
		return "aggressive"
	default:
		return "vicious"
	}
}

// AggressionLabel is a fuller phrase for narrative text, spanning the same
// curious-and-passive-to-relentless-killer scale the species was rolled
// against.
func (sp AlienSpecies) AggressionLabel() string {
	switch {
	case sp.Aggression < 15:
		return "docile, more curious about the colony than hostile toward it"
	case sp.Aggression < 35:
		return "wary, and quick to slip away from a fight it doesn't have to have"
	case sp.Aggression < 60:
		return "opportunistic, pressing any advantage it finds"
	case sp.Aggression < 85:
		return "aggressive, hunting the colony relentlessly"
	default:
		return "a remorseless killing machine that gives no quarter"
	}
}

// RosterLabel is the one-line summary the roster shows for an Alien in place
// of a colonist's pronouns/age line.
func (sp AlienSpecies) RosterLabel() string {
	return fmt.Sprintf("%s · %s", capitalizeFirst(sp.Singular), sp.AggressionTier())
}

// Description is a full narrative summary of the species, for logs or a
// future lore/codex display -- everything a colonist could plausibly have
// worked out about the local wildlife by looking at one.
func (sp AlienSpecies) Description() string {
	legs := sp.Limbs - sp.Arms
	return fmt.Sprintf(
		"%s stand %d-%d cm and weigh %d-%d kg, with %s, %s and %s, %s. Temperament: %s.",
		capitalizeFirst(sp.Plural), sp.HeightMinCM, sp.HeightMaxCM, sp.WeightMinKG, sp.WeightMaxKG,
		pluralize(sp.Eyes, "eye", "eyes"), pluralize(sp.Arms, "arm", "arms"), pluralize(legs, "leg", "legs"),
		tailPhrase(sp.Tail), sp.AggressionLabel(),
	)
}

func tailPhrase(hasTail bool) string {
	if hasTail {
		return "and a tail"
	}
	return "and no tail"
}

// pluralize renders "n word" with the right singular/plural noun.
func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// capitalizeFirst upper-cases a string's first byte, for turning a lowercase
// noun into the start of a sentence ("xeno" -> "Xeno"). ASCII-only, which
// covers every entry in alienNouns.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// alienNoun returns this world's rolled species name with the right
// indefinite article -- "an alien", "a xeno", "an ET" -- for narration in
// place of the generic "alien".
func (w *World) alienNoun() string { return withArticle(w.alienSpecies.Singular) }

// alienPlural returns the plural colonists use for this world's species,
// e.g. "aliens", "xenos", "gremlins".
func (w *World) alienPlural() string { return w.alienSpecies.Plural }
