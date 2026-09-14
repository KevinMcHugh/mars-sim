package sim

import "time"

// Config holds every tunable knob for a simulation run in one place, so
// balancing the game means editing values here rather than hunting through the
// systems. Zero values are not meaningful; use DefaultConfig and adjust.
type Config struct {
	// World shape.
	Width, Height int

	// Seed makes a run reproducible. Same seed + same code => same game.
	Seed int64

	// Starting population.
	StartColonists int
	StartAliens    int
	StartCats      int
	StartMice      int

	// Timing.
	TicksPerSecond int // default simulation speed
	LogSize        int // how many recent events to retain

	// Colonist stats.
	ColonistHP         int
	MineTicks          int // ticks of work to excavate one Rock tile
	BuildTicks         int // ticks of work to raise one Wall
	FacilityBuildTicks int // ticks of work to build a nutrient pod or toilet
	FleeRadius         int // flee when an alien is within this many tiles

	// Needs. One NeedSpec per NeedKind, indexed by that kind.
	Needs                [numNeeds]NeedSpec
	StarveDamage         int // HP lost per tick while a Fatal need sits at Max
	ColonistsPerFacility int // desired colonists served by each facility of a kind (min 1)
	RestTicks            int // ticks an idle colonist rests before re-checking for work
	StuckLimit           int // ticks a colonist waits on a blocked path before abandoning the job

	// Personality. TraitChance is the percent chance a colonist receives a trait
	// from each trait group at spawn (0 disables traits; attributes are still
	// generated). See personality.go.
	TraitChance int

	// Family. FamilyChance is the percent chance a newly generated colonist is
	// tied to an existing one (spouse, sibling, parent/child, aunt/uncle,
	// nibling, or grandparent/grandchild). Uses the personality RNG, so it never
	// perturbs the sim. 0 disables family generation. See relationships.go.
	FamilyChance int

	// Socializing. An idle colonist with nothing productive to do may seek out a
	// nearby colonist and talk, which shifts the pair's affinity and both their
	// moods. Affinity is tracked only; nothing simulates against it yet.
	TalkChance       int // percent chance an idle colonist starts a conversation (0 disables talking)
	TalkRadius       int // how far a colonist looks for a conversation partner
	TalkTicks        int // ticks a conversation lasts before its outcome is applied
	TalkAffinityGain int // base affinity step per conversation (scaled by outcome and diminishing returns)
	AffinityMax      int // affinity runs in [-AffinityMax, AffinityMax]; talking alone saturates at half of it

	// Conversation quality shapes both the affinity change and the mood change a
	// chat produces. Quality is a signed roll in [-100, 100]: TalkQualityBias is
	// its baseline lean (chats are mildly positive by default), TalkQualityValence
	// is how strongly existing affinity pulls quality toward its own sign (the
	// positive-feedback loop that exacerbates like and dislike alike), and
	// TalkQualitySpread is the random swing around that mean, so any pair can still
	// have a surprisingly good or bad conversation.
	TalkQualityBias    int
	TalkQualityValence int
	TalkQualitySpread  int

	// Mood. Each colonist carries a mood in [-MoodMax, MoodMax] (0 = neutral).
	// Nothing simulates against mood yet, but tasks move it. A finished
	// conversation shifts both participants by a company term (how they feel about
	// the other, from affinity) plus a conversation term (how the chat went, from
	// quality): a good chat with someone you dislike lifts your mood, while a
	// merely so-so chat with a friend still nets a small lift.
	MoodMax                int
	MoodCompanyWeight      int
	MoodConversationWeight int

	// Mining strategy switch. Below both thresholds, miners use cached A* to a
	// claimed tile (cheaper for small colonies); at or above either, they follow
	// the shared frontier flow field (cheaper once many miners share the sweep).
	FrontierFieldMinColonists int
	FrontierFieldMinArea      int

	// Alien stats.
	AlienHP       int
	AlienDamage   int // HP removed per bite
	AlienBiteRest int // cooldown ticks between bites
	AlienSlowness int // alien acts once every N ticks (>=1); higher is slower

	// Cat stats. Cats have no needs; they hunt mice on the floor by instinct.
	CatHP         int
	CatSlowness   int // cat acts once every N ticks (>=1); higher is slower
	CatPounceRest int // cooldown ticks after catching a mouse

	// Mouse stats. Mice share the colonists' NeedFood but grow hungry far faster
	// (they nibble constantly), and flee cats rather than aliens.
	MouseHP         int
	MouseHungerRise int // NeedFood gained per tick for mice (vs. Needs[NeedFood].Rise for colonists)
	MouseFleeRadius int // flee when a cat is within this many tiles
}

// DefaultConfig returns a balanced starting point for a playable scaffold.
func DefaultConfig() Config {
	return Config{
		Width:              80,
		Height:             40,
		Seed:               time.Now().UnixNano(),
		StartColonists:     6,
		StartAliens:        3,
		StartCats:          2,
		StartMice:          8,
		TicksPerSecond:     8,
		LogSize:            64,
		ColonistHP:         40,
		MineTicks:          6,
		BuildTicks:         8,
		FacilityBuildTicks: 12,
		FleeRadius:         5,

		StarveDamage:         1,
		ColonistsPerFacility: 10,
		RestTicks:            10,
		StuckLimit:           8,
		TraitChance:          30,
		FamilyChance:         35,

		TalkChance:       25,
		TalkRadius:       6,
		TalkTicks:        12,
		TalkAffinityGain: 4,
		AffinityMax:      100,

		TalkQualityBias:    20,
		TalkQualityValence: 50,
		TalkQualitySpread:  50,

		MoodMax:                100,
		MoodCompanyWeight:      6,
		MoodConversationWeight: 10,

		FrontierFieldMinColonists: 800,
		FrontierFieldMinArea:      90000, // ~300x300 and up
		Needs: [numNeeds]NeedSpec{
			NeedFood: {
				Name: "food", Rise: 2, SeekAt: 650, Max: 1000,
				Facility: NutrientPod, UseTicks: 18, Fatal: true,
			},
			NeedBladder: {
				Name: "bladder", Rise: 3, SeekAt: 600, Max: 1000,
				Facility: Toilet, UseTicks: 10, Fatal: false,
			},
		},
		AlienHP:       30,
		AlienDamage:   6,
		AlienBiteRest: 3,
		AlienSlowness: 2,

		CatHP:         12,
		CatSlowness:   2,
		CatPounceRest: 4,

		MouseHP:         4,
		MouseHungerRise: 8, // 4x the colonist food rise: mice eat very frequently
		MouseFleeRadius: 6,
	}
}

// tickInterval converts a ticks-per-second rate into a sleep duration, clamped
// to something sane.
func tickInterval(ticksPerSecond int) time.Duration {
	if ticksPerSecond < 1 {
		ticksPerSecond = 1
	}
	if ticksPerSecond > 60 {
		ticksPerSecond = 60
	}
	return time.Second / time.Duration(ticksPerSecond)
}
