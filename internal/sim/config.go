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
	// Natural caverns: pockets of open floor worldgen hollows out of the rock
	// away from the landing site, hidden until the colony digs into one, and
	// sometimes joined to a neighbor by a winding passage. See
	// docs/caverns.md.
	CavernPercent        int `cfg:"cavern-percent" doc:"percent of the map hollowed into natural caverns"`
	CavernMin            int `cfg:"cavern-min" doc:"minimum tiles in a natural cavern"`
	CavernMax            int `cfg:"cavern-max" doc:"maximum tiles in a natural cavern"`
	CavernPassagePercent int `cfg:"cavern-passage-percent" doc:"chance (percent) that a cavern is joined to its nearest neighbor by a passage"`
	// Alien nests: the chance a natural cavern is home to a few aliens of one
	// species, lying dormant until the colony breaks in. See docs/caverns.md.
	CavernNestPercent int `cfg:"cavern-nest-percent" doc:"chance (percent) that a natural cavern holds an alien nest"`
	CavernNestMin     int `cfg:"cavern-nest-min" doc:"minimum aliens in a nest"`
	CavernNestMax     int `cfg:"cavern-nest-max" doc:"maximum aliens in a nest"`
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

	// Schedules is the director's script: major scripted occurrences (a rat
	// plague, an alien swarm, a supply drop) each armed for a tick window.
	// Deliberately untagged like Seed: it is not a scalar tunable the `cfg`
	// reflection can drive a flag or template line from, and its meaningful
	// zero value (nil) is "no director file was given" rather than a default
	// worth documenting in mars-sim.yaml. It is loaded from director.yaml (or
	// -director) the same way mars-sim.yaml loads into everything else. See
	// director.go and docs/director.md.
	Schedules []Schedule

	// Starting population.
	StartColonists int `cfg:"colonists" sec:"Starting population" doc:"starting number of colonists"`
	StartAliens    int `cfg:"aliens" doc:"starting number of aliens"`
	StartCats      int `cfg:"cats" doc:"starting number of cats"`
	StartRats      int `cfg:"rats" doc:"starting number of rats"`

	// GraveyardSize is how many recent deaths (any kind) are kept in the
	// bounded graveyard feed used for the roster's "dead" filter on
	// non-colonist kinds; 0 disables that feed entirely. It does not affect
	// a colonist's permanent record in World.deceasedColonists, which is
	// unbounded and always kept regardless of this setting. See
	// docs/combat.md.
	GraveyardSize int `cfg:"graveyard-size" doc:"recent deaths kept for the roster's dead filter (0 disables)"`

	// Money. The colony's supply of dollars is fixed: the founding grant seeds
	// the community treasury once, at world creation, and every colonist mints
	// a purse when it arrives (CrashPodPurse, below). Nothing else creates or
	// destroys money yet. Money settings are int64 rather than Money because
	// the flag binder only knows the three scalar kinds (see bindConfigFlags
	// in main.go). See docs/money.md.
	FoundingGrant int64 `cfg:"founding-grant" sec:"Economy" doc:"dollars the colony treasury starts with"`
	// InfiniteFood is the safety net: nutrient pods make meals out of nothing.
	// Off, a pod serves nothing and the colony eats only what it landed with
	// and what it produces. See docs/food.md.
	InfiniteFood bool `cfg:"infinite-food" doc:"nutrient pods make free meals out of nothing (the safety net)"`

	// Food production. Cave scum is a biofilm on cave surfaces, the renewable
	// base of the food chain: ScumPercent of rock tiles carry a patch of up to
	// ScumMax units, and a scraped patch regrows one unit every
	// ScumRegrowTicks. A scumhouse turns scum and every other kind of
	// biomatter into meals by the recipes in scumhouse.go. The colony makes
	// food while it holds fewer than MealReserve meals per colonist. See
	// docs/scumhouse.md.
	ScumPercent     int `cfg:"scum-percent" sec:"Food production" doc:"percent of rock tiles carrying a patch of cave scum"`
	ScumMax         int `cfg:"scum-max" doc:"units of scum a full patch holds"`
	ScumRegrowTicks int `cfg:"scum-regrow-ticks" doc:"ticks for a scraped patch to regrow one unit of scum"`
	ScrapeTicks     int `cfg:"scrape-ticks" doc:"ticks of work to scrape one unit of scum off a patch"`
	MealReserve     int `cfg:"meal-reserve" doc:"the colony makes food while it holds fewer meals than this per colonist"`
	// ConstructionCosts makes building consume materials: raw rock, and ore
	// for machines (see constructionCost). Off, building is free, as it
	// always was. See docs/construction.md.
	ConstructionCosts bool `cfg:"construction-costs" doc:"building consumes materials from the builder's own stock"`

	// Market. Reference prices are the colony charter's price list: what the
	// colony bids for ore at its silo (paid prospecting) and what a colonist
	// asks for a surplus meal. 0 means not traded at a reference price. The
	// colony keeps SiloBidQty units of standing bids per ore; a colonist
	// keeps MealKeep of its own meals and takes the rest to market; a hungry
	// colonist pays up to MealWillingness times the meal price. See
	// docs/market.md.
	PriceMeal       int64 `cfg:"price-meal" sec:"Market" doc:"reference price of a meal, in dollars"`
	PriceRawRock    int64 `cfg:"price-raw-rock" doc:"what the colony pays for raw rock at its silo (0: it buys none)"`
	PriceIronOre    int64 `cfg:"price-iron-ore" doc:"what the colony pays for iron ore at its silo"`
	PriceWaterIce   int64 `cfg:"price-water-ice" doc:"what the colony pays for water ice at its silo"`
	PriceUraniumOre int64 `cfg:"price-uranium-ore" doc:"what the colony pays for uranium ore at its silo"`
	PriceClay       int64 `cfg:"price-clay" doc:"what the colony pays for clay at its silo"`
	SiloBidQty      int   `cfg:"silo-bid-qty" doc:"units of each ore the colony keeps a standing bid for at its silo"`
	OrderTTL        int   `cfg:"order-ttl" doc:"ticks a colonist's resting order lives before it expires"`
	MealKeep        int   `cfg:"meal-keep" doc:"meals a colonist keeps for itself before it takes the rest to market"`
	MealWillingness int   `cfg:"meal-willingness" doc:"a hungry colonist pays up to this many times the meal price"`

	// Labor. The colony pays for its public works: every task of a room it
	// plans is a work order funded from the treasury at these wages, and a
	// room it cannot fund is not planned. It also pays BountyPay for each
	// unit of biomatter delivered to a scumhouse, keeping BountyUnits of
	// bounty open per scumhouse. A colonist with HouseSavings dollars
	// commissions its own house (0 disables), whose toilet charges others
	// ToiletFee a use. See docs/labor.md.
	// Valuation and the producer planner. A colonist values its own time at
	// LaborPrice dollars per 100 ticks of work, and takes on a plan only if it
	// clears PlanMinProfit after inputs and labor. It considers the
	// PlanCandidates best bids it could fill. A hungry colonist's unfilled bid
	// for a meal rests for DemandTTL ticks; a plan that has not delivered in
	// PlanTTL ticks is dropped, its derived bids with it. PriceCaveScum is
	// scum's reference value until it trades. See docs/valuation.md.
	LaborPrice     int64 `cfg:"labor-price" sec:"Valuation" doc:"what a colonist reckons 100 ticks of its own work are worth, in dollars"`
	PlanMinProfit  int64 `cfg:"plan-min-profit" doc:"the least profit, in dollars, that makes a production plan worth taking on"`
	PlanCandidates int   `cfg:"plan-candidates" doc:"how many of the best open bids a colonist's producer planner considers"`
	PlanTTL        int   `cfg:"plan-ttl" doc:"ticks a production plan may take before it is dropped with its derived bids"`
	DemandTTL      int   `cfg:"demand-ttl" doc:"ticks a hungry colonist's unfilled bid for a meal rests in the book"`
	PriceCaveScum  int64 `cfg:"price-cave-scum" doc:"reference value of a unit of cave scum until it trades"`

	WageDig      int64 `cfg:"wage-dig" sec:"Labor" doc:"what the colony pays to dig out one tile of a room"`
	WageWall     int64 `cfg:"wage-wall" doc:"what the colony pays to raise one wall"`
	WageFixture  int64 `cfg:"wage-fixture" doc:"what the colony pays to build one fixture (pod, toilet, bed, ...)"`
	BountyPay    int64 `cfg:"bounty-pay" doc:"what the colony pays per unit of biomatter delivered to a scumhouse (0 disables)"`
	BountyUnits  int   `cfg:"bounty-units" doc:"units of biomatter bounty the colony keeps open per scumhouse"`
	HouseSavings int64 `cfg:"house-savings" doc:"a colonist with this much money commissions its own house (0 disables)"`
	ToiletFee    int64 `cfg:"toilet-fee" doc:"what a house's toilet charges anyone but its owner per use (0: private)"`

	// Crash pods. Every colonist arrives in one — at worldgen, from the spawn
	// command, or from a director arrival — carrying its own bunk, toilet, and
	// locker, and this manifest. See crashpod.go and docs/crash-pods.md.
	CrashPodPurse    int64 `cfg:"crash-pod-purse" sec:"Crash pods" doc:"dollars each colonist arrives with"`
	CrashPodMeals    int   `cfg:"crash-pod-meals" doc:"meals stocked in each crash pod's locker"`
	CrashPodPistols  int   `cfg:"crash-pod-pistols" doc:"pistols each colonist arrives carrying"`
	CrashPodShotguns int   `cfg:"crash-pod-shotguns" doc:"shotguns each colonist arrives carrying"`

	// Timing.
	TicksPerSecond int `cfg:"tps" sec:"Timing" doc:"simulation ticks per second"`
	LogSize        int `cfg:"log-size" doc:"number of recent events retained"`

	// Colonist stats.
	ColonistHP int `cfg:"colonist-hp" sec:"Colonists" doc:"colonist hit points"`
	MineTicks  int `cfg:"mine-ticks" doc:"ticks of work to excavate one rock tile"`
	BuildTicks int `cfg:"build-ticks" doc:"ticks of work to raise one wall"`
	// DemolishTicks is how long breaking a wall down takes for a colonist
	// escaping a sealed room (see FocusEscape, docs/escape.md). Costlier than
	// raising one (BuildTicks): breaking out should be a last resort, not a
	// cheaper substitute for a door once those exist.
	DemolishTicks      int `cfg:"demolish-ticks" doc:"ticks of work to break down one wall tile when escaping a sealed room"`
	FacilityBuildTicks int `cfg:"facility-ticks" doc:"ticks of work to build a pod or toilet"`
	FleeRadius         int `cfg:"flee-radius" doc:"colonist flees when an alien is within this many tiles"`
	// StompRadius is how far an idle colonist notices a rat and gives chase to
	// crush it. Stomping is an idle whim: only colonists with nothing pressing
	// (no threat, no urgent need, no work) hunt pests.
	ColonistStompRadius int `cfg:"stomp-radius" doc:"an idle colonist chases and crushes a rat within this many tiles"`
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

	// EscapeGraceTicks is how long a colonist's room must stay cut off from the
	// colony's main connected network (see rooms.go's mainRoom) before it gives
	// up waiting for reconnection and starts breaking down the nearest wall
	// itself (FocusEscape). The grace period absorbs the ordinary one-tick lag
	// between a terrain change and refreshSpatial folding it in — it is not
	// meant to be tuned as a difficulty knob. See docs/escape.md.
	EscapeGraceTicks int `cfg:"escape-grace-ticks" doc:"ticks a colonist's room must stay cut off from the colony before it breaks out on its own"`

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

	// Stature. Every mutation also resizes the colonist by
	// MutationStaturePercent of their current height, up or down, and their
	// body (HP and every part) scales with them. StatureMinCM and StatureMaxCM
	// are the hard limits a mutated colonist can reach — the two-foot and
	// ten-foot ends of the colony. 0 percent disables resizing and leaves
	// mutation growing parts only. See mutation.go and docs/mutation.md.
	MutationStaturePercent int `cfg:"mutation-stature-percent" sec:"Stature" doc:"percent a mutation grows or shrinks a colonist's height (0 disables resizing)"`
	StatureMinCM           int `cfg:"stature-min-cm" doc:"shortest a colonist can be shrunk to, in centimetres"`
	StatureMaxCM           int `cfg:"stature-max-cm" doc:"tallest a colonist can grow to, in centimetres"`

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

	// Affect. Charge, grip and valence each run in [-MoodMax, MoodMax].
	// Conversation company/quality produce a temporary signed outcome which is
	// converted to a vector. The three axes settle at three speeds: charge
	// fastest, then grip, then valence, which answers to hours rather than
	// minutes and so decays a point at a time.
	MoodMax                   int `cfg:"mood-max" sec:"Affect" doc:"charge, grip and valence each run in [-mood-max, mood-max]"`
	ConversationCompanyWeight int `cfg:"conversation-company-weight" doc:"conversation outcome from how one feels about the other"`
	ConversationQualityWeight int `cfg:"conversation-quality-weight" doc:"conversation outcome from how the chat itself went"`
	SocialWindowTicks         int `cfg:"social-window-ticks" doc:"ticks in the rolling window for social conversation fatigue"`
	MoodChargeDecayPerTick    int `cfg:"mood-charge-decay-per-tick" doc:"charge points that decay toward home per colonist turn"`
	MoodGripDecayPerTick      int `cfg:"mood-grip-decay-per-tick" doc:"grip points that decay toward home per colonist turn"`
	MoodValenceDecayTicks     int `cfg:"mood-valence-decay-ticks" doc:"ticks per point of valence decay toward neutral"`
	MoodLabelSwitchMargin     int `cfg:"mood-label-switch-margin" doc:"claim advantage required to switch mood attractors"`

	// Where an event stops nudging affect and starts relocating it. An event
	// whose impact is at or below MoodPushImpact adds its vector; at or above
	// MoodPullImpact it replaces affect with it; between the two it does some
	// of each. See docs/mood-space.md.
	MoodPushImpact int `cfg:"mood-push-impact" doc:"event impact at or below which affect is only nudged"`
	MoodPullImpact int `cfg:"mood-pull-impact" doc:"event impact at or above which affect is relocated outright"`

	// How fast a colonist stops being new to something. Each remembered
	// occasion of a kind moves its appraisal this much further from its fresh
	// reading toward its worn one, capped at fully worn. Occasions leave with
	// the memories that hold them, so this also sets how fast wear recovers.
	MoodWearPerOccasion int `cfg:"mood-wear-per-occasion" doc:"percent of the way toward an event's worn reading per remembered occasion"`

	// Mining strategy switch. Below both thresholds, miners use cached A* to a
	// claimed tile (cheaper for small colonies); at or above either, they follow
	// the shared frontier flow field (cheaper once many miners share the sweep).
	FrontierFieldMinColonists int `cfg:"frontier-field-colonists" sec:"Mining strategy" doc:"colony size at/above which miners use the shared frontier flow field"`
	FrontierFieldMinArea      int `cfg:"frontier-field-area" doc:"map area (tiles) at/above which miners use the shared frontier flow field"`

	// Alien stats. AlienDamage/AlienBiteRest/AlienSlowness are baselines: each
	// species rolled for this world's lore (see lore.go) scales them by its
	// rolled size and temperament, so what an alien actually deals and how
	// fast it moves varies by seed and by species even at the same config.
	// AlienReferenceWeightKG is the specimen weight at which a species deals
	// exactly AlienDamage. AlienSpeciesCount is how many distinct species a
	// seed rolls; every Alien entity belongs to one of them.
	// AlienCautiousRadius is how close a colonist has to come before a
	// Cautious species reacts and closes in, rather than only a Hostile
	// species' unconditional hunt.
	AlienSpeciesCount      int `cfg:"alien-species-count" sec:"Aliens" doc:"distinct alien species this seed rolls (every alien belongs to one)"`
	AlienHP                int `cfg:"alien-hp" doc:"alien hit points"`
	AlienDamage            int `cfg:"alien-damage" doc:"baseline HP removed per bite, before a species' size scales it"`
	AlienBiteRest          int `cfg:"alien-bite-rest" doc:"baseline cooldown ticks between bites, before a species' temperament scales it"`
	AlienSlowness          int `cfg:"alien-slowness" doc:"baseline: alien acts once every N ticks (higher = slower), before a species' temperament scales it"`
	AlienReferenceWeightKG int `cfg:"alien-reference-weight-kg" doc:"specimen weight in kg at which a species deals exactly alien-damage"`
	AlienCautiousRadius    int `cfg:"alien-cautious-radius" doc:"how close a colonist must come before a Cautious species reacts and closes in"`

	// AlienNames configures the pool of names ("xenos," "critters," ...) a
	// rolled species can be given, each gated by a condition over its build
	// (legs, arms, skin, color, temperament, size). Deliberately untagged
	// like Schedules: it is not a scalar tunable, and its meaningful zero
	// value (nil/empty) falls back to defaultAlienNames() rather than a
	// default worth documenting in mars-sim.yaml. It is loaded from
	// alien-names.yaml (or -alien-names) the same way director.yaml loads
	// Schedules. See lore.go, alien_names.go and docs/lore.md.
	AlienNames []AlienNameEntry

	// Weapon stats. A colonist carrying one stands and fights an alien within
	// Range instead of fleeing, firing once every FireRest ticks. See
	// combat.go and docs/combat.md.
	PistolDamage    int `cfg:"pistol-damage" sec:"Weapons" doc:"HP removed per pistol shot"`
	PistolRange     int `cfg:"pistol-range" doc:"max tiles a pistol can fire from"`
	PistolFireRest  int `cfg:"pistol-fire-rest" doc:"cooldown ticks between pistol shots"`
	ShotgunDamage   int `cfg:"shotgun-damage" doc:"HP removed per shotgun blast"`
	ShotgunRange    int `cfg:"shotgun-range" doc:"max tiles a shotgun can fire from"`
	ShotgunFireRest int `cfg:"shotgun-fire-rest" doc:"cooldown ticks between shotgun blasts"`

	// Cat stats. Cats have no needs; they hunt rats on the floor by instinct.
	CatHP         int `cfg:"cat-hp" sec:"Cats" doc:"cat hit points"`
	CatSlowness   int `cfg:"cat-slowness" doc:"cat acts once every N ticks (higher = slower)"`
	CatPounceRest int `cfg:"cat-pounce-rest" doc:"cooldown ticks after a cat catches a rat"`

	// Rat stats. Rats share the colonists' NeedFood but grow hungry far faster
	// (they nibble constantly), and flee cats rather than aliens.
	RatHP         int `cfg:"rat-hp" sec:"Rats" doc:"rat hit points"`
	RatHungerRise int `cfg:"rat-hunger-rise" doc:"food need a rat gains per tick (rats eat frequently)"`
	// RatScavengeRadius is how far a hungry rat looks for a body, gore, or
	// cave scum to eat before it settles for raiding a nutrient pod. See
	// scavenge.go.
	RatScavengeRadius int `cfg:"rat-scavenge-radius" doc:"how far a hungry rat looks for bodies, gore, or scum to eat"`
	RatFleeRadius     int `cfg:"rat-flee-radius" doc:"rat flees when a cat is within this many tiles"`

	// Rat breeding. Two adjacent rats of opposite sex mate; the female then
	// carries a litter for RatGestationTicks before birthing RatLitterMin..Max
	// pups onto nearby floor. RatBreedCooldown spaces out a female's litters,
	// and a newborn cannot breed for RatMaturityTicks.
	RatGestationTicks int `cfg:"rat-gestation" doc:"ticks a pregnant rat carries a litter before giving birth"`
	RatLitterMin      int `cfg:"rat-litter-min" doc:"smallest rat litter size"`
	RatLitterMax      int `cfg:"rat-litter-max" doc:"largest rat litter size"`
	RatBreedCooldown  int `cfg:"rat-breed-cooldown" doc:"ticks a rat waits before it can mate again"`
	RatMaturityTicks  int `cfg:"rat-maturity" doc:"ticks a newborn rat takes to mature enough to breed"`
}

// DefaultConfig returns a balanced starting point for a playable scaffold.
func DefaultConfig() Config {
	return Config{
		Width:                80,
		Height:               40,
		IronRockPercent:      10,
		IceRockPercent:       5,
		UraniumRockPercent:   1,
		ClayRockPercent:      5,
		RockVeinMin:          8,
		RockVeinMax:          24,
		CavernPercent:        4,
		CavernMin:            15,
		CavernMax:            60,
		CavernPassagePercent: 50,
		CavernNestPercent:    10,
		CavernNestMin:        2,
		CavernNestMax:        4,
		FogOfWar:             true,
		Seed:                 time.Now().UnixNano(),
		StartColonists:       6,
		StartAliens:          3,
		StartCats:            2,
		StartRats:            8,
		// Placeholders until the market gives money a use: a treasury worth a
		// few dozen purses, so the colony can outspend any one settler.
		FoundingGrant: 5000,
		InfiniteFood:  true,
		CrashPodPurse: 100,
		// Scum is tuned so a small colony that keeps digging can feed itself
		// once its crash-pod meals run out: see the sustain test in
		// scumhouse_test.go before changing these.
		ScumPercent:       6,
		ScumMax:           3,
		ScumRegrowTicks:   400,
		ScrapeTicks:       6,
		MealReserve:       3,
		ConstructionCosts: false,
		// The charter's prices: a meal a few hours' pay, uranium dearest
		// because it costs the miner a dose, raw rock not bought at all — there
		// is always more, and buying it would drain the treasury on nothing.
		PriceMeal:       5,
		PriceRawRock:    0,
		PriceIronOre:    3,
		PriceWaterIce:   2,
		PriceUraniumOre: 6,
		PriceClay:       2,
		SiloBidQty:      64,
		OrderTTL:        2000,
		MealKeep:        5,
		MealWillingness: 3,
		// Wages sized so a typical room costs the colony about a hundred
		// dollars: fifty rooms from the founding grant, less what it spends
		// buying ore. A house is a real purchase, several weeks of prospecting.
		// A colonist's time is worth a couple of dollars per 100 ticks: less
		// than a scraper earns selling scum into a meal-maker's bid, so the
		// chain from a hungry colonist's bid down to the cave wall pays at
		// every link. See docs/valuation.md.
		LaborPrice:     2,
		PlanMinProfit:  1,
		PlanCandidates: 4,
		PlanTTL:        1500,
		DemandTTL:      300,
		PriceCaveScum:  1,
		WageDig:        2,
		WageWall:       2,
		WageFixture:    5,
		BountyPay:      1,
		BountyUnits:    30,
		HouseSavings:   300,
		ToiletFee:      2,
		// A meal clears hunger for roughly 325 ticks at the baseline rise, so
		// ten carry a colonist a few thousand ticks: long enough to settle in,
		// short enough that food production matters once the safety net is
		// off. Every settler lands armed, the way frontier settlers did.
		CrashPodMeals:       10,
		CrashPodPistols:     1,
		CrashPodShotguns:    0,
		GraveyardSize:       50,
		TicksPerSecond:      8,
		LogSize:             64,
		ColonistHP:          40,
		MineTicks:           6,
		BuildTicks:          8,
		DemolishTicks:       16,
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
			// The default combat posture is documented in docs/combat.md.
			FocusFight: {Name: "fight", Base: 15, NeedWeight: 0, ChargeWeight: 20, GripWeight: 40, DistanceWeight: 1},
			// FocusEscape has no matching need, so its score is just Base: a
			// sealed room is a structural fact, not a rising pressure. Base
			// deliberately clears even a maxed-out fatal need (NeedWeight
			// pressure(100) + FocusCriticalBonus(100) + FocusFatalBonus(150) =
			// 350): reachability, not local coping, is what actually stayed
			// broken, and the nearest wall very often sits between the
			// colonist and the very facility that need is failing to reach —
			// so escaping first is usually also the fastest way back to it.
			// focusCandidates excludes it outright while a threat is visible,
			// so it never competes with FocusFlee/FocusFight's much lower
			// bases; self-preservation from an immediate predator always wins.
			// See docs/escape.md.
			FocusEscape: {Name: "escape", Base: 400, NeedWeight: 0, ChargeWeight: 0, GripWeight: 0, DistanceWeight: 0},
		},
		FocusCurrentBonus:     25,
		FocusSwitchMargin:     10,
		FocusCriticalBonus:    100,
		FocusFatalBonus:       150,
		ActiveStimulusLimit:   8,
		RestTicks:             10,
		StuckLimit:            8,
		MaxConcurrentProjects: 2,
		EscapeGraceTicks:      32,
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

		// A 15% step is big enough to read on the roster the moment it
		// happens, and small enough that the extremes take a run of one-sided
		// luck: from an average 178 cm it is two straight growths to clear
		// seven feet, four to touch the ten-foot ceiling, and eight shrinks
		// to reach the two-foot floor. The limits are 2 ft and 10 ft in round
		// centimetres.
		MutationStaturePercent: 15,
		StatureMinCM:           61,
		StatureMaxCM:           305,

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
		MoodValenceDecayTicks:     12,
		MoodLabelSwitchMargin:     5,

		MoodPushImpact:      30,
		MoodPullImpact:      70,
		MoodWearPerOccasion: 14,

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
		AlienSpeciesCount:      1,
		AlienHP:                30,
		AlienDamage:            6,
		AlienBiteRest:          3,
		AlienSlowness:          2,
		AlienReferenceWeightKG: 80,
		AlienCautiousRadius:    3,

		PistolDamage:    10,
		PistolRange:     3,
		PistolFireRest:  1,
		ShotgunDamage:   20,
		ShotgunRange:    2,
		ShotgunFireRest: 2,

		CatHP:         12,
		CatSlowness:   2,
		CatPounceRest: 4,

		RatHP:         4,
		RatHungerRise: 8, // 4x the colonist food rise: rats eat very frequently
		// Farther than a colonist cleans (clean-radius): a rat finds the dead
		// before the colony does.
		RatScavengeRadius: 12,
		RatFleeRadius:     6,

		RatGestationTicks: 300,
		RatLitterMin:      2,
		RatLitterMax:      5,
		RatBreedCooldown:  200,
		RatMaturityTicks:  400,
	}
}

// tickInterval converts a ticks-per-second rate into a sleep duration. There is
// no upper cap on the rate: the only floors are one tick per second and a
// non-zero interval. Asking for more ticks than the machine can simulate just
// runs flat out: Engine.Run catches up on late ticks only within maxTickLag.
func tickInterval(ticksPerSecond int) time.Duration {
	if ticksPerSecond < 1 {
		ticksPerSecond = 1
	}
	return max(time.Second/time.Duration(ticksPerSecond), 1)
}
