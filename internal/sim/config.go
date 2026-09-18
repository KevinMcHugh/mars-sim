package sim

import "time"

// Config holds every tunable knob for a simulation run in one place, so
// balancing the game means editing values here rather than hunting through the
// systems. Zero values are not meaningful; use DefaultConfig and adjust.
type Config struct {
	// World shape.
	Width  int `cfg:"width" sec:"World" doc:"world width in tiles"`
	Height int `cfg:"height" doc:"world height in tiles"`
	// Rock composition percentages. The remainder is ordinary rock.
	IronRockPercent    int `cfg:"iron-rock-percent" doc:"percent of rock tiles bearing iron"`
	IceRockPercent     int `cfg:"ice-rock-percent" doc:"percent of rock tiles bearing water ice"`
	UraniumRockPercent int `cfg:"uranium-rock-percent" doc:"percent of rock tiles bearing uranium"`
	ClayRockPercent    int `cfg:"clay-rock-percent" doc:"percent of rock tiles bearing clay"`
	RockVeinMin        int `cfg:"rock-vein-min" doc:"minimum tiles in a generated rock deposit vein"`
	RockVeinMax        int `cfg:"rock-vein-max" doc:"maximum tiles in a generated rock deposit vein"`
	// FogOfWar hides rock the colony has not dug up to yet: a tile is only
	// shown once something has changed the terrain within one tile of it. It
	// costs the simulation nothing (no system reads it) and is a display
	// setting only in the sense that a frontend is what draws the fog — what
	// the colony has *seen* is world state, so it lives here rather than in
	// the UI. Off shows the whole map, as the game did before. See
	// docs/fog-of-war.md.
	FogOfWar bool `cfg:"fog-of-war" doc:"hide rock the colony has not dug up to yet"`

	// Seed makes a run reproducible. Same seed + same code => same game.
	//
	// Deliberately untagged: it is not a balance knob, and its default is the
	// wall clock, so it cannot appear in a generated template. The config file
	// and the -seed flag both carry it as a special case where 0 means "pick a
	// fresh time-based seed" (see configfile.go and main.go).
	Seed int64

	// Starting population.
	StartColonists int `cfg:"colonists" sec:"Starting population" doc:"starting number of colonists"`
	StartAliens    int `cfg:"aliens" doc:"starting number of aliens"`
	StartCats      int `cfg:"cats" doc:"starting number of cats"`
	StartMice      int `cfg:"mice" doc:"starting number of mice"`

	// Starting equipment. The colony ship arrives with a handful of firearms
	// for defense against aliens; worldgen hands them out to distinct
	// colonists (see generate in worldgen.go).
	StartPistols  int `cfg:"pistols" doc:"pistols the colony ship arrives with"`
	StartShotguns int `cfg:"shotguns" doc:"shotguns the colony ship arrives with"`

	// GraveyardSize is how many recent deaths (any kind) are kept as frozen
	// records for the roster's "dead" filter; 0 disables death tracking
	// entirely. See docs/combat.md.
	GraveyardSize int `cfg:"graveyard-size" doc:"recent deaths kept for the roster's dead filter (0 disables)"`

	// Timing.
	TicksPerSecond int `cfg:"tps" sec:"Timing" doc:"simulation ticks per second"`
	LogSize        int `cfg:"log-size" doc:"number of recent events retained"`

	// Colonist stats.
	ColonistHP         int `cfg:"colonist-hp" sec:"Colonists" doc:"colonist hit points"`
	MineTicks          int `cfg:"mine-ticks" doc:"ticks of work to excavate one rock tile"`
	BuildTicks         int `cfg:"build-ticks" doc:"ticks of work to raise one wall"`
	FacilityBuildTicks int `cfg:"facility-ticks" doc:"ticks of work to build a pod or toilet"`
	FleeRadius         int `cfg:"flee-radius" doc:"colonist flees when an alien is within this many tiles"`
	// StompRadius is how far an idle colonist notices a mouse and gives chase to
	// crush it. Stomping is an idle whim: only colonists with nothing pressing
	// (no threat, no urgent need, no work) hunt pests.
	ColonistStompRadius int `cfg:"stomp-radius" doc:"an idle colonist chases and crushes a mouse within this many tiles"`
	// GoreSightRadius is how far a colonist notices gore on the ground (see
	// observeGore in systems.go and EvtSawGore in lifeevents.go). Smaller than
	// the creature-sighting radii: a bloodstain doesn't announce itself the way
	// a moving alien does.
	GoreSightRadius int `cfg:"gore-sight-radius" doc:"a colonist notices gore on the ground within this many tiles"`

	// Sanitation. A colonist with no urgent need cleans up refuse — gore and
	// corpses — and hauls it to an incinerator to burn. CleanRadius is how far
	// it looks for a mess (larger than GoreSightRadius, which is about noticing
	// one, not going to find it); a Tidy colonist searches twice as far. See
	// docs/sanitation.md.
	CleanRadius           int `cfg:"clean-radius" sec:"Sanitation" doc:"how far a colonist looks for refuse to clean up (a Tidy colonist looks twice as far)"`
	CleanTicks            int `cfg:"clean-ticks" doc:"ticks of work to scrub one tile of refuse clean"`
	IncinerateTicks       int `cfg:"incinerate-ticks" doc:"ticks spent feeding a load of refuse into an incinerator"`
	IncineratorBuildTicks int `cfg:"incinerator-ticks" doc:"ticks of work to build an incinerator"`

	// Needs. One NeedSpec per NeedKind, indexed by that kind.
	Needs                [numNeeds]NeedSpec `cfg:"needs" sec:"Needs"`
	StarveDamage         int                `cfg:"starve-damage" doc:"HP lost per tick while a fatal need sits at its max"`
	ColonistsPerFacility int                `cfg:"per-facility" doc:"colonists served by each life-support facility"`

	// Focus arbitration. One FocusSpec per FocusKind, indexed by that kind.
	Focuses             [numFocusKinds]FocusSpec `cfg:"focuses" sec:"Focus arbitration"`
	FocusCurrentBonus   int                      `cfg:"focus-current-bonus" doc:"score bonus for continuing the current eligible focus"`
	FocusSwitchMargin   int                      `cfg:"focus-switch-margin" doc:"minimum score lead required to replace an eligible focus"`
	FocusCriticalBonus  int                      `cfg:"focus-critical-bonus" doc:"score bonus for a need at its critical boundary"`
	FocusFatalBonus     int                      `cfg:"focus-fatal-bonus" doc:"score bonus for a pressing fatal need"`
	ActiveStimulusLimit int                      `cfg:"active-stimulus-limit" doc:"maximum transient life-event appraisals retained per colonist"`

	RestTicks  int `cfg:"rest-ticks" sec:"Work and construction" doc:"ticks an idle colonist rests before re-checking for work"`
	StuckLimit int `cfg:"stuck-limit" doc:"ticks a colonist waits on a blocked path before abandoning the job"`

	// MaxConcurrentProjects is the ceiling on how many rooms can be under
	// construction at once (min 1 is enforced); the actual cap also scales
	// down for a small colony (see maxConcurrentProjects in project.go) so an
	// early cramped cavern still builds one room at a time. Raising it lets a
	// larger colony's facility supply keep pace with growth; see
	// construction.md.
	MaxConcurrentProjects int `cfg:"max-concurrent-projects" doc:"rooms that can be under construction at once"`

	// Personality. TraitChance is the percent chance a colonist receives a trait
	// from each trait group at spawn (0 disables traits; attributes are still
	// generated). See personality.go.
	TraitChance int `cfg:"trait-chance" sec:"Personality" doc:"percent chance a colonist gets a trait from each trait group (0 disables)"`

	// Mutation. A colonist near a uranium deposit or carrying uranium ore takes
	// a dose: every UraniumExposureTicks of accumulated exposure is one roll at
	// MutationChance percent to grow an extra body part and become a Mutant.
	// MutantLoverAffinityBonus is the extra affinity a Mutant-Lover gains
	// toward a mutant per conversation, on top of the ordinary talk step. See
	// mutation.go and docs/mutation.md.
	UraniumExposureTicks     int `cfg:"uranium-exposure-ticks" sec:"Mutation" doc:"ticks of uranium exposure per mutation roll"`
	MutationChance           int `cfg:"mutation-chance" doc:"percent chance each full uranium dose mutates a colonist (0 disables mutation)"`
	MutantLoverAffinityBonus int `cfg:"mutant-lover-affinity" doc:"extra affinity a mutant-lover gains toward a mutant per conversation"`

	// Family. FamilyChance is the percent chance a newly generated colonist is
	// tied to an existing one (spouse, sibling, parent/child, aunt/uncle,
	// nibling, or grandparent/grandchild). Uses the personality RNG, so it never
	// perturbs the sim. 0 disables family generation. See relationships.go.
	FamilyChance int `cfg:"family-chance" sec:"Family and heredity" doc:"percent chance a new colonist is tied to an existing one by family (0 disables)"`

	// Heredity. Once a colonist has a family, that family decides part of who
	// they are: they take its surname, they take after their closest relatives,
	// and they start out already knowing them. See heredity.go.
	//
	// AppearanceInheritChance is the percent chance each heritable feature (skin
	// tone, natural hair color, height) is taken from a relative rather than
	// kept as rolled, at full relatedness — it is scaled down for more distant
	// kin. 0 makes every colonist's looks independent.
	//
	// SpouseSurnameChance is the percent chance a colonist who marries into the
	// colony takes their spouse's surname instead of keeping their own; blood
	// relatives always share a surname regardless.
	//
	// FamilyAffinity is the starting affinity between the closest relatives, as
	// a percent of AffinityMax, scaled down per relation kind;
	// FamilyAffinitySpread is the random swing around it in the same units, so
	// relatives aren't all equally close. 0 starts family at a stranger's zero.
	AppearanceInheritChance int `cfg:"appearance-inherit-chance" doc:"percent chance each of a colonist's features is inherited from a close relative (0 disables)"`
	SpouseSurnameChance     int `cfg:"spouse-surname-chance" doc:"percent chance a colonist marrying in takes their spouse's surname"`
	FamilyAffinity          int `cfg:"family-affinity" doc:"starting affinity between close relatives, as a percent of affinity-max (0 disables)"`
	FamilyAffinitySpread    int `cfg:"family-affinity-spread" doc:"random swing around the starting family affinity, in the same units"`

	// Socializing. An idle colonist with nothing productive to do may seek out a
	// nearby colonist and talk, which shifts the pair's affinity and affect.
	// Affinity is tracked only; nothing simulates against it yet.
	TalkChance       int `cfg:"talk-chance" sec:"Socializing" doc:"percent chance an idle colonist starts a conversation (0 disables talking)"`
	TalkRadius       int `cfg:"talk-radius" doc:"how far a colonist looks for a conversation partner"`
	TalkTicks        int `cfg:"talk-ticks" doc:"ticks a conversation lasts before affinity is credited"`
	TalkAffinityGain int `cfg:"talk-affinity-gain" doc:"base affinity step per conversation (scaled by outcome and diminishing returns)"`
	AffinityMax      int `cfg:"affinity-max" doc:"affinity runs in [-affinity-max, affinity-max]; talking alone saturates at half"`

	// Conversation quality shapes both the affinity change and affect outcome a
	// chat produces. Quality is a signed roll in [-100, 100]: TalkQualityBias is
	// its baseline lean (chats are mildly positive by default), TalkQualityValence
	// is how strongly existing affinity pulls quality toward its own sign (the
	// positive-feedback loop that exacerbates like and dislike alike), and
	// TalkQualitySpread is the random swing around that mean, so any pair can still
	// have a surprisingly good or bad conversation.
	TalkQualityBias    int `cfg:"talk-quality-bias" doc:"baseline lean of conversation quality (-100..100)"`
	TalkQualityValence int `cfg:"talk-quality-valence" doc:"how strongly existing affinity biases conversation quality"`
	TalkQualitySpread  int `cfg:"talk-quality-spread" doc:"random swing around a conversation's mean quality"`

	// Affect. Charge and grip each run in [-MoodMax, MoodMax]. Conversation
	// company/quality produce a temporary signed outcome which is converted to a
	// vector, and charge settles faster than grip by default.
	MoodMax                   int `cfg:"mood-max" sec:"Affect" doc:"charge and grip each run in [-mood-max, mood-max]"`
	ConversationCompanyWeight int `cfg:"conversation-company-weight" doc:"conversation outcome from how one feels about the other"`
	ConversationQualityWeight int `cfg:"conversation-quality-weight" doc:"conversation outcome from how the chat itself went"`
	SocialWindowTicks         int `cfg:"social-window-ticks" doc:"ticks in the rolling window for social conversation fatigue"`
	MoodChargeDecayPerTick    int `cfg:"mood-charge-decay-per-tick" doc:"charge points that decay toward home per colonist turn"`
	MoodGripDecayPerTick      int `cfg:"mood-grip-decay-per-tick" doc:"grip points that decay toward home per colonist turn"`
	MoodLabelSwitchMargin     int `cfg:"mood-label-switch-margin" doc:"claim advantage required to switch mood attractors"`

	// Mining strategy switch. Below both thresholds, miners use cached A* to a
	// claimed tile (cheaper for small colonies); at or above either, they follow
	// the shared frontier flow field (cheaper once many miners share the sweep).
	FrontierFieldMinColonists int `cfg:"frontier-field-colonists" sec:"Mining strategy" doc:"colony size at/above which miners use the shared frontier flow field"`
	FrontierFieldMinArea      int `cfg:"frontier-field-area" doc:"map area (tiles) at/above which miners use the shared frontier flow field"`

	// Alien stats.
	AlienHP       int `cfg:"alien-hp" sec:"Aliens" doc:"alien hit points"`
	AlienDamage   int `cfg:"alien-damage" doc:"HP removed per alien bite"`
	AlienBiteRest int `cfg:"alien-bite-rest" doc:"cooldown ticks between alien bites"`
	AlienSlowness int `cfg:"alien-slowness" doc:"alien acts once every N ticks (higher = slower)"`

	// Weapon stats. A colonist carrying one stands and fights an alien within
	// Range instead of fleeing, firing once every FireRest ticks. See
	// combat.go and docs/combat.md.
	PistolDamage    int `cfg:"pistol-damage" sec:"Weapons" doc:"HP removed per pistol shot"`
	PistolRange     int `cfg:"pistol-range" doc:"max tiles a pistol can fire from"`
	PistolFireRest  int `cfg:"pistol-fire-rest" doc:"cooldown ticks between pistol shots"`
	ShotgunDamage   int `cfg:"shotgun-damage" doc:"HP removed per shotgun blast"`
	ShotgunRange    int `cfg:"shotgun-range" doc:"max tiles a shotgun can fire from"`
	ShotgunFireRest int `cfg:"shotgun-fire-rest" doc:"cooldown ticks between shotgun blasts"`

	// Cat stats. Cats have no needs; they hunt mice on the floor by instinct.
	CatHP         int `cfg:"cat-hp" sec:"Cats" doc:"cat hit points"`
	CatSlowness   int `cfg:"cat-slowness" doc:"cat acts once every N ticks (higher = slower)"`
	CatPounceRest int `cfg:"cat-pounce-rest" doc:"cooldown ticks after a cat catches a mouse"`

	// Mouse stats. Mice share the colonists' NeedFood but grow hungry far faster
	// (they nibble constantly), and flee cats rather than aliens.
	MouseHP         int `cfg:"mouse-hp" sec:"Mice" doc:"mouse hit points"`
	MouseHungerRise int `cfg:"mouse-hunger-rise" doc:"food need a mouse gains per tick (mice eat frequently)"`
	MouseFleeRadius int `cfg:"mouse-flee-radius" doc:"mouse flees when a cat is within this many tiles"`

	// Mouse breeding. Two adjacent mice of opposite sex mate; the female then
	// carries a litter for MouseGestationTicks before birthing MouseLitterMin..Max
	// pups onto nearby floor. MouseBreedCooldown spaces out a female's litters,
	// and a newborn cannot breed for MouseMaturityTicks.
	MouseGestationTicks int `cfg:"mouse-gestation" doc:"ticks a pregnant mouse carries a litter before giving birth"`
	MouseLitterMin      int `cfg:"mouse-litter-min" doc:"smallest mouse litter size"`
	MouseLitterMax      int `cfg:"mouse-litter-max" doc:"largest mouse litter size"`
	MouseBreedCooldown  int `cfg:"mouse-breed-cooldown" doc:"ticks a mouse waits before it can mate again"`
	MouseMaturityTicks  int `cfg:"mouse-maturity" doc:"ticks a newborn mouse takes to mature enough to breed"`
}

// DefaultConfig returns a balanced starting point for a playable scaffold.
func DefaultConfig() Config {
	return Config{
		Width:               80,
		Height:              40,
		IronRockPercent:     10,
		IceRockPercent:      5,
		UraniumRockPercent:  1,
		ClayRockPercent:     5,
		RockVeinMin:         8,
		RockVeinMax:         24,
		FogOfWar:            true,
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

		CleanRadius:           10,
		CleanTicks:            6,
		IncinerateTicks:       8,
		IncineratorBuildTicks: 20,

		StarveDamage:         1,
		ColonistsPerFacility: 5,
		Focuses: [numFocusKinds]FocusSpec{
			FocusIdle:      {Name: "idle", Base: 0, NeedWeight: 0, ChargeWeight: -10, GripWeight: 0, DistanceWeight: 0},
			FocusWork:      {Name: "work", Base: 25, NeedWeight: 0, ChargeWeight: 20, GripWeight: 10, DistanceWeight: 1},
			FocusEat:       {Name: "eat", Base: 40, NeedWeight: 100, ChargeWeight: 0, GripWeight: 5, DistanceWeight: 1},
			FocusRelieve:   {Name: "relieve", Base: 40, NeedWeight: 100, ChargeWeight: 0, GripWeight: 0, DistanceWeight: 1},
			FocusSocialize: {Name: "socialize", Base: 40, NeedWeight: 100, ChargeWeight: 10, GripWeight: 5, DistanceWeight: 1},
			FocusSleep:     {Name: "sleep", Base: 40, NeedWeight: 100, ChargeWeight: -30, GripWeight: 0, DistanceWeight: 1},
			FocusFlee:      {Name: "flee", Base: 0, NeedWeight: 0, ChargeWeight: 20, GripWeight: -40, DistanceWeight: 1},
			// FocusFight carries a positive base so an armed colonist's default
			// posture is to stand and fight: the one-time grip hit from merely
			// *seeing* an alien (EvtSawAlien, -10 grip) must not by itself out-vote
			// that posture, or every armed colonist flees on first sight and the
			// switch hysteresis (FocusCurrentBonus+FocusSwitchMargin) then locks
			// them into fleeing even as grip decays back toward neutral. Genuinely
			// frightening events (being bitten, watching a colonist killed) still
			// carry enough grip penalty to tip an armed colonist toward flight. See
			// D-002 in docs/cascading_wsts_execution_plan.md.
			FocusFight: {Name: "fight", Base: 15, NeedWeight: 0, ChargeWeight: 20, GripWeight: 40, DistanceWeight: 1},
		},
		FocusCurrentBonus:     25,
		FocusSwitchMargin:     10,
		FocusCriticalBonus:    100,
		FocusFatalBonus:       150,
		ActiveStimulusLimit:   8,
		RestTicks:             10,
		StuckLimit:            8,
		MaxConcurrentProjects: 2,
		TraitChance:           30,
		FamilyChance:          35,

		AppearanceInheritChance: 75,
		SpouseSurnameChance:     50,
		FamilyAffinity:          55,
		FamilyAffinitySpread:    15,

		// Mutation is meant to be a rarity the colony talks about, not a
		// career stage every miner passes through. Exposure is cumulative and
		// never decays, so the *number* of rolls, not the chance on any one of
		// them, is what decides how many colonists end up mutants: at a dose
		// per 100 ticks, dropping the chance from 25% to 1% still mutated a
		// fifth of the colony, because a working miner simply rolls that many
		// times. A dose worth 2000 ticks of exposure — several minutes spent
		// beside a vein or carrying the ore, not one tile of digging — with a
		// 1-in-100 roll at the end of it lands near 1% of colonists in a
		// typical run, drifting to a few percent in a very long one because
		// the dose is permanent. See docs/mutation.md.
		UraniumExposureTicks:     2000,
		MutationChance:           1,
		MutantLoverAffinityBonus: 3,

		TalkChance:       25,
		TalkRadius:       6,
		TalkTicks:        12,
		TalkAffinityGain: 4,
		AffinityMax:      100,

		TalkQualityBias:    20,
		TalkQualityValence: 50,
		TalkQualitySpread:  50,

		MoodMax:                   100,
		ConversationCompanyWeight: 6,
		ConversationQualityWeight: 10,
		SocialWindowTicks:         200,
		MoodChargeDecayPerTick:    2,
		MoodGripDecayPerTick:      1,
		MoodLabelSwitchMargin:     5,

		FrontierFieldMinColonists: 800,
		FrontierFieldMinArea:      90000, // ~300x300 and up
		Needs: [numNeeds]NeedSpec{
			NeedFood: {
				Name: "food", Rise: 2, SeekAt: 650, CriticalAt: 1000, Max: 1000,
				Facility: NutrientPod, UseTicks: 18, Fatal: true,
				// A colonist grabs a portion in 3 ticks and eats it away from
				// the pod, instead of occupying its one access tile for the
				// full 18 — far more throughput per pod at the same cost.
				GrabTicks: 3,
			},
			NeedBladder: {
				Name: "bladder", Rise: 3, SeekAt: 600, CriticalAt: 900, Max: 1000,
				Facility: Toilet, UseTicks: 10, Fatal: false,
			},
			NeedSocial: {
				Name: "social", Rise: 2, SeekAt: 500, CriticalAt: 850, Max: 1000,
				Facility: Rock, UseTicks: 0, Fatal: false,
			},
			NeedSleep: {
				// Sleep builds slowly and, once sought, takes a long lie-down to
				// clear. Non-fatal like bladder: a colonist with no bunk waits
				// rather than dying.
				Name: "sleep", Rise: 1, SeekAt: 700, CriticalAt: 900, Max: 1000,
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
