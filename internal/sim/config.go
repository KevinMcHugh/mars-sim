package sim

import "time"

// Config holds every tunable knob for a simulation run in one place, so
// balancing the game means editing values here rather than hunting through the
// systems. Zero values are not meaningful; use DefaultConfig and adjust.
type Config struct {
	// World shape.
	Width, Height int
	// Rock composition percentages. The remainder is ordinary rock.
	IronRockPercent int
	IceRockPercent  int

	// Seed makes a run reproducible. Same seed + same code => same game.
	Seed int64

	// Starting population.
	StartColonists int
	StartAliens    int
	StartCats      int
	StartMice      int

	// Starting equipment. The colony ship arrives with a handful of firearms
	// for defense against aliens; worldgen hands them out to distinct
	// colonists (see generate in worldgen.go).
	StartPistols  int
	StartShotguns int

	// GraveyardSize is how many recent deaths (any kind) are kept as frozen
	// records for the roster's "dead" filter; 0 disables death tracking
	// entirely. See docs/combat.md.
	GraveyardSize int

	// Timing.
	TicksPerSecond int // default simulation speed
	LogSize        int // how many recent events to retain

	// Colonist stats.
	ColonistHP         int
	MineTicks          int // ticks of work to excavate one Rock tile
	BuildTicks         int // ticks of work to raise one Wall
	FacilityBuildTicks int // ticks of work to build a nutrient pod or toilet
	FleeRadius         int // flee when an alien is within this many tiles
	// StompRadius is how far an idle colonist notices a mouse and gives chase to
	// crush it. Stomping is an idle whim: only colonists with nothing pressing
	// (no threat, no urgent need, no work) hunt pests.
	ColonistStompRadius int
	// GoreSightRadius is how far a colonist notices gore on the ground (see
	// observeGore in systems.go and EvtSawGore in lifeevents.go). Smaller than
	// the creature-sighting radii: a bloodstain doesn't announce itself the way
	// a moving alien does.
	GoreSightRadius int

	// Needs. One NeedSpec per NeedKind, indexed by that kind.
	Needs                [numNeeds]NeedSpec
	StarveDamage         int // HP lost per tick while a Fatal need sits at Max
	ColonistsPerFacility int // desired colonists served by each facility of a kind (min 1)
	RestTicks            int // ticks an idle colonist rests before re-checking for work
	StuckLimit           int // ticks a colonist waits on a blocked path before abandoning the job

	// MaxConcurrentProjects is the ceiling on how many rooms can be under
	// construction at once (min 1 is enforced); the actual cap also scales
	// down for a small colony (see maxConcurrentProjects in project.go) so an
	// early cramped cavern still builds one room at a time. Raising it lets a
	// larger colony's facility supply keep pace with growth; see
	// construction.md.
	MaxConcurrentProjects int

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
	SocialWindowTicks      int // rolling window used for introvert conversation fatigue

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

	// Weapon stats. A colonist carrying one stands and fights an alien within
	// Range instead of fleeing, firing once every FireRest ticks. See
	// combat.go and docs/combat.md.
	PistolDamage    int
	PistolRange     int
	PistolFireRest  int
	ShotgunDamage   int
	ShotgunRange    int
	ShotgunFireRest int

	// Cat stats. Cats have no needs; they hunt mice on the floor by instinct.
	CatHP         int
	CatSlowness   int // cat acts once every N ticks (>=1); higher is slower
	CatPounceRest int // cooldown ticks after catching a mouse

	// Mouse stats. Mice share the colonists' NeedFood but grow hungry far faster
	// (they nibble constantly), and flee cats rather than aliens.
	MouseHP         int
	MouseHungerRise int // NeedFood gained per tick for mice (vs. Needs[NeedFood].Rise for colonists)
	MouseFleeRadius int // flee when a cat is within this many tiles

	// Mouse breeding. Two adjacent mice of opposite sex mate; the female then
	// carries a litter for MouseGestationTicks before birthing MouseLitterMin..Max
	// pups onto nearby floor. MouseBreedCooldown spaces out a female's litters,
	// and a newborn cannot breed for MouseMaturityTicks.
	MouseGestationTicks int
	MouseLitterMin      int
	MouseLitterMax      int
	MouseBreedCooldown  int
	MouseMaturityTicks  int
}

// DefaultConfig returns a balanced starting point for a playable scaffold.
func DefaultConfig() Config {
	return Config{
		Width:               80,
		Height:              40,
		IronRockPercent:     10,
		IceRockPercent:      5,
		Seed:                time.Now().UnixNano(),
		StartColonists:      6,
		StartAliens:         3,
		StartCats:           2,
		StartMice:           8,
		StartPistols:        1,
		StartShotguns:       1,
		GraveyardSize:       50,
		TicksPerSecond:      8,
		LogSize:             64,
		ColonistHP:          40,
		MineTicks:           6,
		BuildTicks:          8,
		FacilityBuildTicks:  12,
		FleeRadius:          5,
		ColonistStompRadius: 4,
		GoreSightRadius:     3,

		StarveDamage:          1,
		ColonistsPerFacility:  5,
		RestTicks:             10,
		StuckLimit:            8,
		MaxConcurrentProjects: 2,
		TraitChance:           30,
		FamilyChance:          35,

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
		SocialWindowTicks:      200,

		FrontierFieldMinColonists: 800,
		FrontierFieldMinArea:      90000, // ~300x300 and up
		Needs: [numNeeds]NeedSpec{
			NeedFood: {
				Name: "food", Rise: 2, SeekAt: 650, Max: 1000,
				Facility: NutrientPod, UseTicks: 18, Fatal: true,
				// A colonist grabs a portion in 3 ticks and eats it away from
				// the pod, instead of occupying its one access tile for the
				// full 18 — far more throughput per pod at the same cost.
				GrabTicks: 3,
			},
			NeedBladder: {
				Name: "bladder", Rise: 3, SeekAt: 600, Max: 1000,
				Facility: Toilet, UseTicks: 10, Fatal: false,
			},
			NeedSocial: {
				Name: "social", Rise: 2, SeekAt: 500, Max: 1000,
				Facility: Rock, UseTicks: 0, Fatal: false,
			},
			NeedSleep: {
				// Sleep builds slowly and, once sought, takes a long lie-down to
				// clear. Non-fatal like bladder: a colonist with no bunk waits
				// rather than dying.
				Name: "sleep", Rise: 1, SeekAt: 700, Max: 1000,
				Facility: Bed, UseTicks: 40, Fatal: false,
			},
		},
		AlienHP:       30,
		AlienDamage:   6,
		AlienBiteRest: 3,
		AlienSlowness: 2,

		PistolDamage:    10,
		PistolRange:     3,
		PistolFireRest:  1,
		ShotgunDamage:   20,
		ShotgunRange:    2,
		ShotgunFireRest: 2,

		CatHP:         12,
		CatSlowness:   2,
		CatPounceRest: 4,

		MouseHP:         4,
		MouseHungerRise: 8, // 4x the colonist food rise: mice eat very frequently
		MouseFleeRadius: 6,

		MouseGestationTicks: 300,
		MouseLitterMin:      2,
		MouseLitterMax:      5,
		MouseBreedCooldown:  200,
		MouseMaturityTicks:  400,
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
