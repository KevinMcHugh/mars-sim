package sim

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// ---- Lore: the world beyond the colony --------------------------------------
//
// World generation used to describe only the cavern and the handful of
// creatures in it. Lore is the start of a layer describing the world the
// colony landed in: right now that is the alien species native to it, but the
// same idea (roll something once per seed, off its own RNG stream, and let
// the sim read it back out) is meant to grow to cover organizations,
// corporations, other colonies, and whatever else the setting picks up later.
//
// rollAlienSpecies gives a world Config.AlienSpeciesCount kinds of alien
// (one by default): each gets a build (height/weight range, eye and limb
// count, arms vs. legs, tail or not, skin, color), a name drawn from a
// condition-gated pool (see alien_names.go), and a Temperament from
// AlienTemperament -- friendly, cautious, or hostile -- that decides whether
// and how it fights. Every Alien entity spawned in a world (at worldgen, by a
// director swarm, by a debug spawn) is assigned one of that world's rolled
// species at spawn time (see World.spawn and Entity.Species).
//
// Lore rolls on its own seed-derived RNG stream, not w.rng or w.prng:
//   - Not w.prng, the personality stream, because a species' size and
//     temperament are not mere flavor — they set actual bite damage and
//     combat behavior (see speciesDamage/scaledByTemperament below and
//     alienTurn in systems.go), so they must stay reproducible on the
//     deterministic side of the personality/sim split (see AGENTS.md).
//   - Not w.rng, the simulation stream, for the same reason worldgen's
//     streams (featureRand in worldgen_chunks.go) are not: rolling species must not shift
//     where anything spawns or any later w.rng-driven decision, so it cannot
//     share that stream's draw sequence. Which *species* a given Alien
//     entity is assigned, though, is drawn from w.rng at spawn time (see
//     World.spawn) -- that is an ordinary gameplay decision like where a
//     colonist lands, not part of generating the species themselves.
//
// Species are rolled in newWorld rather than generate() so that every World
// has a valid roster regardless of which of the two a caller uses to build
// one — generate() is the normal path, but a number of tests build a World
// with newWorld directly and still spawn and fight Aliens against it.

// alienLoreSeed is this world's lore stream's XOR key against cfg.Seed,
// distinct from prng's, agePRNG's, and worldgen's own keys so none of them
// ever draw from the same sequence.
const alienLoreSeed = 0x452821E638D01377

// AlienTemperament is how a species relates to the colony -- whether it ever
// initiates a fight, and how readily.
type AlienTemperament uint8

const (
	// TemperamentFriendly never initiates combat: it neither hunts nor
	// reacts to a nearby colonist, only wanders. (Colonists may still fight
	// or flee one on their own initiative -- that side of the interaction
	// is unchanged for now; see docs/lore.md.)
	TemperamentFriendly AlienTemperament = iota
	// TemperamentCautious does not hunt, but bites back once a colonist
	// comes within Config.AlienCautiousRadius.
	TemperamentCautious
	// TemperamentHostile hunts the nearest colonist anywhere on the map, the
	// way every alien behaved before temperament existed.
	TemperamentHostile
)

func (t AlienTemperament) String() string {
	switch t {
	case TemperamentFriendly:
		return "friendly"
	case TemperamentCautious:
		return "cautious"
	case TemperamentHostile:
		return "hostile"
	default:
		return "unknown"
	}
}

// rollTemperament draws a species' temperament: friendly is rare, cautious
// and hostile are both common and evenly split of what's left. A 0-aggression
// "never fights" species used to be representable but not guaranteed by the
// old continuous roll; making Friendly its own enum value makes "never
// initiates combat" a case the code has to handle, not a value that happens
// to zero everything out.
func rollTemperament(rng *rand.Rand) AlienTemperament {
	switch r := rng.IntN(100); {
	case r < 10:
		return TemperamentFriendly
	case r < 55:
		return TemperamentCautious
	default:
		return TemperamentHostile
	}
}

// AlienSkin is a species' hide, both flavor (naming, description) and a
// naming condition (see alien_names.go's `skin` field).
type AlienSkin uint8

const (
	SkinSmooth AlienSkin = iota
	SkinScaly
	SkinFurry
	SkinArmored
	SkinBony
	SkinChitinous
	SkinSlimy
)

var alienSkins = [...]AlienSkin{
	SkinSmooth, SkinScaly, SkinFurry, SkinArmored, SkinBony, SkinChitinous, SkinSlimy,
}

func (s AlienSkin) String() string {
	switch s {
	case SkinSmooth:
		return "smooth"
	case SkinScaly:
		return "scaly"
	case SkinFurry:
		return "furry"
	case SkinArmored:
		return "armored"
	case SkinBony:
		return "bony"
	case SkinChitinous:
		return "chitinous"
	case SkinSlimy:
		return "slimy"
	default:
		return "unknown"
	}
}

// AttackMode is one way a species can hurt its prey. Which modes a species
// has is rolled once, gated by its anatomy (see rollAttackModes): only a
// species with a tail can thrash one, only one with arms has claws, and
// strangling needs at least two arms to get a grip. Every species has a
// mouth, so biting is always available.
type AttackMode uint8

const (
	AttackBite AttackMode = iota
	AttackClaw
	AttackTail
	AttackStrangle
)

// attackModes is every mode in roll order. rollAttackModes walks it, so
// reordering it (or inserting into the middle) shifts lore-stream draws.
var attackModes = [...]AttackMode{AttackBite, AttackClaw, AttackTail, AttackStrangle}

func (m AttackMode) String() string {
	switch m {
	case AttackBite:
		return "bite"
	case AttackClaw:
		return "claws"
	case AttackTail:
		return "tail"
	case AttackStrangle:
		return "strangle"
	default:
		return "unknown"
	}
}

// canUse reports whether a species' build allows an attack mode at all.
func (sp AlienSpecies) canUse(m AttackMode) bool {
	switch m {
	case AttackClaw:
		return sp.Arms > 0
	case AttackTail:
		return sp.Tail
	case AttackStrangle:
		return sp.Arms >= 2
	default: // AttackBite: every species has a mouth
		return true
	}
}

// AttackSet is a set of AttackModes, one bit each. A bitmask rather than a
// slice so AlienSpecies stays a comparable value that a Snapshot copies
// without sharing a backing array.
type AttackSet uint8

// AttackSetOf builds a set from modes.
func AttackSetOf(modes ...AttackMode) AttackSet {
	var s AttackSet
	for _, m := range modes {
		s |= 1 << m
	}
	return s
}

// Has reports whether m is in the set.
func (s AttackSet) Has(m AttackMode) bool { return s&(1<<m) != 0 }

// rollAttackModes picks which of the modes its anatomy allows a species
// actually fights with: each allowed mode is kept on a coin flip, so two
// clawed, tailed species need not fight alike. A species that keeps nothing
// bites -- every species has at least one way to attack.
func rollAttackModes(rng *rand.Rand, sp AlienSpecies) AttackSet {
	var set AttackSet
	for _, m := range attackModes {
		if sp.canUse(m) && rng.IntN(2) == 0 {
			set |= AttackSetOf(m)
		}
	}
	if set == 0 {
		set = AttackSetOf(AttackBite)
	}
	return set
}

// Attacks is the species' attack modes in attackModes order, never empty: a
// species built by hand (tests, mostly) with no AttackModes bites, the same
// fallback rollAttackModes uses.
func (sp AlienSpecies) Attacks() []AttackMode {
	var modes []AttackMode
	for _, m := range attackModes {
		if sp.AttackModes.Has(m) {
			modes = append(modes, m)
		}
	}
	if len(modes) == 0 {
		return []AttackMode{AttackBite}
	}
	return modes
}

// AttacksLabel lists the species' attack modes for a stat line: "bite,
// claws, tail".
func (sp AlienSpecies) AttacksLabel() string {
	modes := sp.Attacks()
	names := make([]string, len(modes))
	for i, m := range modes {
		names[i] = m.String()
	}
	return strings.Join(names, ", ")
}

// attackPhrase is how a description says a species fights: "biting,
// raking with their claws, and thrashing their tails".
func (sp AlienSpecies) attackPhrase() string {
	modes := sp.Attacks()
	phrases := make([]string, len(modes))
	for i, m := range modes {
		switch m {
		case AttackClaw:
			phrases[i] = "raking with their claws"
		case AttackTail:
			phrases[i] = "thrashing their tails"
		case AttackStrangle:
			phrases[i] = "strangling with their arms"
		default:
			phrases[i] = "biting"
		}
	}
	return joinList(phrases)
}

// alienColors is the palette a species' Color is drawn from -- both flavor
// and a naming condition (see alien_names.go's `color` field, and the
// per-color entries in alien-names.yaml).
var alienColors = [...]string{
	"red", "orange", "yellow", "green", "blue", "purple", "gray", "black", "white", "pale",
	"iridescent",
}

// AlienPattern is how a species' Color is laid over its hide: one solid
// color, or striped or spotted with it. Flavor and a naming condition (see
// alien_names.go's `pattern` field), like Skin and Color.
type AlienPattern uint8

const (
	PatternSolid AlienPattern = iota
	PatternStriped
	PatternSpotted
)

func (p AlienPattern) String() string {
	switch p {
	case PatternSolid:
		return "solid"
	case PatternStriped:
		return "striped"
	case PatternSpotted:
		return "spotted"
	default:
		return "unknown"
	}
}

// rollPattern draws a species' pattern: striped and spotted are 5% each and
// solid is everything else, so a patterned species is a genuine find.
func rollPattern(rng *rand.Rand) AlienPattern {
	switch r := rng.IntN(100); {
	case r < 5:
		return PatternStriped
	case r < 10:
		return PatternSpotted
	default:
		return PatternSolid
	}
}

// AlienSizeTier buckets a continuous height or weight range into the coarse
// words a naming condition or a description reaches for ("huge," "tiny").
type AlienSizeTier uint8

const (
	SizeTiny AlienSizeTier = iota
	SizeSmall
	SizeAverage
	SizeLarge
	SizeHuge
)

func (t AlienSizeTier) String() string {
	switch t {
	case SizeTiny:
		return "tiny"
	case SizeSmall:
		return "small"
	case SizeAverage:
		return "average"
	case SizeLarge:
		return "large"
	case SizeHuge:
		return "huge"
	default:
		return "unknown"
	}
}

// AlienSpecies describes one kind of alien native to a world: what it looks
// like, what colonists call it, and how it fights. Rolled once per species by
// rollAlienSpecies and stored on World.alienSpecies; every Alien entity in
// the game is assigned one (Entity.Species indexes into that slice).
// Exported so a Snapshot can carry copies of it to frontends.
type AlienSpecies struct {
	// Singular and Plural are the colloquial words colonists use in place of
	// "alien"/"aliens" -- drawn from a condition-gated pool, see
	// alien_names.go. Always lowercase; callers capitalize where a sentence
	// needs it (see capitalizeFirst).
	Singular string
	Plural   string

	// ScientificName is the species' binomial, "Genus epithet" -- for
	// example "Pseudursus ares" -- built from Greek and Latin word parts
	// that fit its build (see alien_taxonomy.go). Already capitalized the
	// way a binomial is written: genus capitalized, epithet lowercase.
	// Empty for a hand-built species that never went through the roster.
	ScientificName string

	// Emoji is a candidate glyph drawn alongside the name, from the same
	// alien-names.yaml entry's own emoji list (see alien_names.go). Empty
	// when the winning entry listed none. This package treats it as opaque
	// data -- rendering it safely (falling back to a generic glyph for
	// anything a frontend does not recognize) is the frontend's job; see
	// docs/lore.md.
	Emoji string

	// Adult size, as a range: no two specimens are identical, but every one
	// of them falls somewhere between these bounds. Bite damage is scaled
	// from the range's midpoint weight -- see speciesDamage.
	HeightMinCM, HeightMaxCM int
	WeightMinKG, WeightMaxKG int

	Eyes  int // how many eyes it has
	Limbs int // how many limbs it walks/grasps with, arms and legs together
	Arms  int // of Limbs, how many are prehensile arms rather than legs

	Tail  bool
	Skin  AlienSkin
	Color string // one of alienColors
	// Pattern is how Color is laid out: solid, striped, or spotted.
	Pattern AlienPattern

	// Temperament decides whether and how this species fights -- see
	// AlienTemperament and alienTurn in systems.go.
	Temperament AlienTemperament

	// AttackModes is how it hurts prey -- see rollAttackModes. Read it
	// through Attacks(), which never returns empty.
	AttackModes AttackSet

	// BiteDamage, BiteRest, and Slowness are precomputed once at roll time
	// from Config's alien baselines (AlienDamage/AlienBiteRest/
	// AlienSlowness) so combat never recomputes them per hit or per turn.
	// See speciesDamage and scaledByTemperament.
	BiteDamage int
	BiteRest   int
	Slowness   int
}

// Legs is how many of a species' Limbs are legs rather than arms -- derived,
// not stored, so Arms is the one source of truth for the split.
func (sp AlienSpecies) Legs() int { return sp.Limbs - sp.Arms }

// HeightTier and WeightTier bucket the species' range midpoint into the
// coarse size words a naming condition or Description reaches for.
func (sp AlienSpecies) HeightTier() AlienSizeTier {
	return sizeTier((sp.HeightMinCM+sp.HeightMaxCM)/2, 60, 120, 220, 320)
}

func (sp AlienSpecies) WeightTier() AlienSizeTier {
	return sizeTier((sp.WeightMinKG+sp.WeightMaxKG)/2, 15, 40, 90, 180)
}

// sizeTier buckets v against three ascending thresholds into the five
// AlienSizeTier values.
func sizeTier(v, small, average, large, huge int) AlienSizeTier {
	switch {
	case v < small:
		return SizeTiny
	case v < average:
		return SizeSmall
	case v < large:
		return SizeAverage
	case v < huge:
		return SizeLarge
	default:
		return SizeHuge
	}
}

// rollAlienSpecies generates one alien species from rng, scaling its derived
// combat stats from cfg's alien baselines and drawing its name from names
// (the entries whose condition matches what was just rolled -- see
// alien_names.go), skipping any name in used: the names earlier species in
// the same roster already took (nil for none).
func rollAlienSpecies(rng *rand.Rand, cfg Config, names []AlienNameEntry, used map[string]bool) AlienSpecies {
	sp := AlienSpecies{
		Eyes:        1 + rng.IntN(6), // 1..6
		Limbs:       2 + rng.IntN(7), // 2..8
		Temperament: rollTemperament(rng),
		Skin:        alienSkins[rng.IntN(len(alienSkins))],
		Color:       alienColors[rng.IntN(len(alienColors))],
	}
	sp.Arms = rng.IntN(sp.Limbs + 1) // 0..Limbs; the rest are legs -- both
	// "all legs" (Arms == 0) and "all arms" (Arms == Limbs) are valid rolls.
	sp.Tail = rng.IntN(2) == 0
	sp.Pattern = rollPattern(rng)

	baseHeight := 45 + rng.IntN(330) // a 45cm gremlin up to a ~375cm brute
	spread := 10 + rng.IntN(baseHeight/3+10)
	sp.HeightMinCM = max(20, baseHeight-spread)
	sp.HeightMaxCM = baseHeight + spread

	// Weight follows height through a per-species density (kg per cm),
	// rather than a fixed formula, so a species can read as wiry or
	// hulking independent of how tall it rolled.
	density := 0.15 + rng.Float64()*0.55
	sp.WeightMinKG = max(1, int(float64(sp.HeightMinCM)*density))
	sp.WeightMaxKG = max(sp.WeightMinKG+1, int(float64(sp.HeightMaxCM)*density))

	sp.Singular, sp.Plural, sp.Emoji = pickAlienName(rng, sp, names, used)
	// After the name, so adding attack modes left this species' own build
	// and name exactly as before; only later species' draws shift.
	sp.AttackModes = rollAttackModes(rng, sp)

	sp.BiteDamage = speciesDamage(sp, cfg)
	sp.BiteRest = scaledByTemperament(cfg.AlienBiteRest, sp.Temperament)
	sp.Slowness = scaledByTemperament(cfg.AlienSlowness, sp.Temperament)
	return sp
}

// rollAlienSpeciesRoster rolls Config.AlienSpeciesCount species (at least
// one, even if misconfigured to less) for a world, then gives each a
// scientific name. The names come from their own stream, seeded from
// cfg.Seed, so drawing them never shifts what rng rolls for the species
// themselves (see alien_taxonomy.go).
func rollAlienSpeciesRoster(rng *rand.Rand, cfg Config) []AlienSpecies {
	count := cfg.AlienSpeciesCount
	if count < 1 {
		count = 1
	}
	names := cfg.AlienNames
	if len(names) == 0 {
		names = defaultAlienNames()
	}
	roster := make([]AlienSpecies, count)
	used := make(map[string]bool, count) // names taken so far: no two species share one
	for i := range roster {
		roster[i] = rollAlienSpecies(rng, cfg, names, used)
		used[strings.ToLower(roster[i].Singular)] = true
	}
	taxa := defaultTaxonomy()
	taxRNG := newRand(cfg.Seed ^ alienTaxonomySeed)
	binomials := make(map[string]bool, count) // no two species share a scientific name either
	for i := range roster {
		roster[i].ScientificName = scientificName(taxRNG, roster[i], taxa, binomials)
		binomials[strings.ToLower(roster[i].ScientificName)] = true
	}
	return roster
}

// speciesDamage scales cfg.AlienDamage by this species' size relative to
// cfg.AlienReferenceWeightKG, the weight at which a specimen deals exactly
// the configured baseline: a species heavier than the reference hits harder,
// a lighter one hits softer. A zero (or negative) baseline passes through
// unscaled rather than being floored to 1 -- that is how combat_test.go
// neuters an alien's bite to isolate other behavior, and scaling must not
// turn "off" back into "on".
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

// scaledByTemperament scales a baseline cooldown (AlienBiteRest or
// AlienSlowness) by temperament: a Hostile species strikes and moves twice
// as fast as the baseline, a Cautious one reproduces the baseline exactly,
// and a Friendly one (which never fights, but still wanders) moves half
// again slower -- a calmer creature, not merely a slower killer.
func scaledByTemperament(base int, t AlienTemperament) int {
	var v int
	switch t {
	case TemperamentHostile:
		v = scaleRound(base, 50, 100)
	case TemperamentFriendly:
		v = scaleRound(base, 150, 100)
	default: // TemperamentCautious
		v = base
	}
	if v < 1 {
		v = 1
	}
	return v
}

// RosterLabel is the one-line summary the roster shows for an Alien in place
// of a colonist's pronouns/age line. The emoji prefix is flavor text here --
// this is a variable-width display string, safely truncatable by a
// frontend, not a fixed-width map glyph (see AlienSpecies.Emoji) -- so it is
// included whenever the rolled species has one, whether or not a frontend's
// map would recognize it as a registered glyph.
func (sp AlienSpecies) RosterLabel() string {
	label := fmt.Sprintf("%s · %s", capitalizeFirst(sp.Singular), sp.Temperament.String())
	if sp.Emoji == "" {
		return label
	}
	return sp.Emoji + " " + label
}

// Description is a short field-guide entry for the species, for logs and the
// lore tab -- everything a colonist could plausibly have worked out about the
// local wildlife by looking at one. It reads like a wiki entry rather than a
// stat block (the lore tab already lists the raw numbers above it), and its
// framing follows Temperament: a friendly species is introduced as good
// company, a cautious one as something to approach carefully, a hostile one
// as a predator. Sizes are given in metric with imperial in parentheses.
// The wording is a pure function of the species -- no RNG -- so it never
// touches determinism.
func (sp AlienSpecies) Description() string {
	size := fmt.Sprintf("stand %s tall, weighing %s", sp.heightRangePhrase(), sp.weightRangePhrase())
	name := capitalizeFirst(sp.Plural)
	covering, coveringPlural := sp.coveringPhrase()
	switch sp.Temperament {
	case TemperamentFriendly:
		return fmt.Sprintf("%s %s. They have %s. They are covered in %s and interact well with humans.",
			name, size, joinList(sp.bodyParts()), covering)
	case TemperamentHostile:
		weapons := []string{pluralize(sp.Eyes, "eye", "eyes")}
		if sp.Arms > 0 {
			weapons = append(weapons, pluralize(sp.Arms, "fearsome arm", "fearsome arms"))
		}
		blend := "stands out against"
		if blendsWithMars(sp.Color) {
			blend = "blends into"
		}
		if coveringPlural {
			blend = strings.Replace(blend, "stands", "stand", 1)
			blend = strings.Replace(blend, "blends", "blend", 1)
		}
		sep := " and " // "with 5 eyes and crawl", not "with 5 eyes, and crawl"
		if len(weapons) > 1 {
			sep = ", and "
		}
		return fmt.Sprintf("The feared %s %s. They hunt humans with %s%s%s. They kill by %s. Their %s %s the Martian rock.",
			name, size, joinList(weapons), sep, sp.gaitPhrase(), sp.attackPhrase(), covering, blend)
	default: // TemperamentCautious
		return fmt.Sprintf("%s %s. They are skittish around humans; approach with caution. They can be recognized by their %s, %s. Get too close and they lash out by %s.",
			name, size, covering, joinList(sp.bodyParts()), sp.attackPhrase())
	}
}

// heightRangePhrase renders the height range as metres with feet and inches
// in parentheses: "1.9-3 m (6'3\"-9'10\")".
func (sp AlienSpecies) heightRangePhrase() string {
	return fmt.Sprintf("%s-%s m (%s-%s)", metres(sp.HeightMinCM), metres(sp.HeightMaxCM),
		FormatHeight(sp.HeightMinCM), FormatHeight(sp.HeightMaxCM))
}

// weightRangePhrase renders the weight range as kilograms with pounds in
// parentheses: "35-54 kg (77-119 lb)".
func (sp AlienSpecies) weightRangePhrase() string {
	return fmt.Sprintf("%d-%d kg (%d-%d lb)", sp.WeightMinKG, sp.WeightMaxKG,
		scaleRound(sp.WeightMinKG, 22046, 10000), scaleRound(sp.WeightMaxKG, 22046, 10000))
}

// metres renders centimetres as metres to one decimal place, dropping a
// trailing ".0" so 299 cm reads "3" rather than "3.0".
func metres(cm int) string {
	tenths := scaleRound(cm, 1, 10)
	if tenths%10 == 0 {
		return fmt.Sprintf("%d", tenths/10)
	}
	return fmt.Sprintf("%d.%d", tenths/10, tenths%10)
}

// bodyParts lists eyes, arms, legs, and (if present) the tail as count
// phrases, for a species description's anatomy sentence. A part the species
// has none of is left out rather than read as "0 arms" or "no arms".
func (sp AlienSpecies) bodyParts() []string {
	parts := []string{pluralize(sp.Eyes, "eye", "eyes")}
	if sp.Arms > 0 {
		parts = append(parts, pluralize(sp.Arms, "arm", "arms"))
	}
	if legs := sp.Legs(); legs > 0 {
		parts = append(parts, pluralize(legs, "leg", "legs"))
	}
	if sp.Tail {
		parts = append(parts, "a tail")
	}
	return parts
}

// gaitPhrase is how a hostile species gets around, picked from its leg count
// so a legless one slithers rather than "crawls on 0 legs".
func (sp AlienSpecies) gaitPhrase() string {
	switch legs := sp.Legs(); legs {
	case 0:
		return "slither along without legs"
	case 1:
		return "hop on 1 leg"
	case 2:
		return "stride on 2 legs"
	default:
		return fmt.Sprintf("crawl on %d legs", legs)
	}
}

// coveringPhrase names what the species' hide is made of, with its color
// (and pattern) worked in -- "red chitin", "slimy purple skin", "green-
// striped scales" -- and reports whether that noun is plural, so a sentence
// built around it can agree its verb ("scales blend", "skin blends").
func (sp AlienSpecies) coveringPhrase() (string, bool) {
	c := sp.ColorPhrase()
	switch sp.Skin {
	case SkinScaly:
		return c + " scales", true
	case SkinFurry:
		return c + " fur", false
	case SkinArmored:
		return c + " armor plates", true
	case SkinBony:
		return "bony " + c + " skin", false
	case SkinChitinous:
		return c + " chitin", false
	case SkinSlimy:
		return "slimy " + c + " skin", false
	default: // SkinSmooth
		return "smooth " + c + " skin", false
	}
}

// blendsWithMars reports whether a hide color is camouflage against Martian
// regolith and basalt, for a hostile species' description.
func blendsWithMars(color string) bool {
	switch color {
	case "red", "orange", "yellow", "gray":
		return true
	}
	return false
}

// joinList renders phrases as an English list with an Oxford comma: "a",
// "a and b", "a, b, and c".
func joinList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
}

// ColorPhrase is Color with its Pattern folded in: "green" for a solid
// species, "green-striped" or "green-spotted" otherwise.
func (sp AlienSpecies) ColorPhrase() string {
	if sp.Pattern == PatternSolid {
		return sp.Color
	}
	return sp.Color + "-" + sp.Pattern.String()
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
// covers every entry alien-names.yaml ships with.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// alienSpeciesFor resolves the species an Alien entity was assigned at spawn
// (see World.spawn). It tolerates an out-of-range or unset index (index 0,
// the zero value) by falling back to the first rolled species, and an empty
// roster (should not happen outside a hand-built test World) by returning
// the zero AlienSpecies rather than panicking.
func (w *World) alienSpeciesFor(e *Entity) AlienSpecies {
	if len(w.alienSpecies) == 0 {
		return AlienSpecies{}
	}
	idx := e.Species
	if idx < 0 || idx >= len(w.alienSpecies) {
		idx = 0
	}
	return w.alienSpecies[idx]
}

// alienNounFor returns e's species name with the right indefinite article --
// "an alien", "a xeno", "an ET" -- for narration in place of the generic
// "alien".
func (w *World) alienNounFor(e *Entity) string { return withArticle(w.alienSpeciesFor(e).Singular) }

// alienPluralFor returns the plural colonists use for e's species, e.g.
// "aliens", "xenos", "gremlins".
func (w *World) alienPluralFor(e *Entity) string { return w.alienSpeciesFor(e).Plural }
