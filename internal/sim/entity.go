package sim

import "fmt"

// Kind identifies what an entity fundamentally is. Behavior is dispatched on
// Kind by the per-tick systems.
type Kind uint8

const (
	// Colonist is a human worker: walks on Floor, mines Rock, builds structures,
	// tends to its drives, and flees from aliens.
	Colonist Kind = iota
	// Alien is a subterranean mutant that walks the floor to hunt and eat
	// colonists. Most start dormant in hidden caverns (see docs/caverns.md).
	Alien
	// Cat is a surface predator that stalks the floor hunting rats. It has no
	// drives of its own; it hunts by instinct.
	Cat
	// Rat is a pest that scurries the floor, scavenging bodies, gore, and cave
	// scum, and nibbling from nutrient pods when there is nothing else.
	// It has a hunger drive and starves without food; cats eat it.
	Rat
	// Chicken is a colonist's bird, brought down in its crash pod. It grazes
	// cave scum and eats feed from its keeper's trough, and starves without
	// either. Cats and chickens ignore each other. See docs/chickens.md.
	Chicken

	numKinds // keep last: the number of entity kinds
)

func (k Kind) String() string {
	switch k {
	case Colonist:
		return "colonist"
	case Alien:
		return "alien"
	case Cat:
		return "cat"
	case Rat:
		return "rat"
	case Chicken:
		return "chicken"
	default:
		return "unknown"
	}
}

// State is a coarse label for what an entity is currently doing. It is derived
// from the entity's Job each tick and surfaced in the UI.
type State uint8

const (
	Idle        State = iota
	Moving            // travelling toward Target
	Mining            // excavating Rock
	Building          // constructing a structure
	Eating            // using a nutrient pod
	Relieving         // using a toilet
	Sleeping          // sleeping in a bed
	Fleeing           // running from a nearby predator (colonist from alien, rat from cat)
	Hunting           // predator closing on prey (alien on colonist, cat on rat)
	Feeding           // predator eating prey it has caught
	Talking           // chatting with another colonist (builds affinity)
	Stomping          // colonist chasing down and crushing a pest rat
	Fighting          // armed colonist standing its ground and firing on an alien
	Cleaning          // colonist scrubbing refuse off a tile, or feeding the incinerator
	Hauling           // colonist carrying gathered refuse to an incinerator
	Storing           // colonist unloading general materials into storage
	Demolishing       // colonist breaking down a wall to escape a sealed room
	Crafting          // colonist working a recipe at a workshop (the scumhouse)
	Scraping          // colonist scraping cave scum off a surface

	numStates // keep last: the number of states
)

func (s State) String() string {
	switch s {
	case Idle:
		return "idle"
	case Moving:
		return "moving"
	case Mining:
		return "mining"
	case Building:
		return "building"
	case Eating:
		return "eating"
	case Relieving:
		return "relieving"
	case Sleeping:
		return "sleeping"
	case Talking:
		return "talking"
	case Fleeing:
		return "fleeing"
	case Hunting:
		return "hunting"
	case Feeding:
		return "feeding"
	case Stomping:
		return "stomping"
	case Fighting:
		return "fighting"
	case Cleaning:
		return "cleaning"
	case Hauling:
		return "hauling"
	case Storing:
		return "storing"
	case Demolishing:
		return "demolishing"
	case Crafting:
		return "crafting"
	case Scraping:
		return "scraping"
	default:
		return "?"
	}
}

// BodyPart identifies one wound location tracked separately from an entity's
// overall HP. Only Colonist and Alien use body parts (see Entity.hasParts);
// cats and rats stay on a single HP pool, since nothing hits them with
// anything more precise than a pounce or a boot.
//
// The enum has two halves. Everything below numBaseBodyParts is anatomy every
// body has; everything above it is a *mutant* part, grown in play by uranium
// exposure (see mutation.go). Which of them a particular entity actually has
// is per-entity data, not a property of the enum: a part exists for an entity
// only while its MaxParts entry is above zero, which is what distinguishes an
// ungrown part from a destroyed one (Parts zero, MaxParts still set).
type BodyPart uint8

const (
	Head BodyPart = iota
	Torso
	LeftArm
	RightArm
	LeftLeg
	RightLeg

	// numBaseBodyParts separates the anatomy everyone is born with from the
	// mutant parts below. Keep it directly after the last ordinary part.
	numBaseBodyParts

	// Mutant parts. None of them is Vital: a mutation adds somewhere to be
	// wounded, it never adds a new way to die outright.
	ThirdArm
	ExtraEye
	Tail
	VestigialTwin

	numBodyParts // keep last: the number of body parts
)

// Mutant reports whether a part is grown by mutation rather than born with.
func (p BodyPart) Mutant() bool { return p > numBaseBodyParts && p < numBodyParts }

func (p BodyPart) String() string {
	switch p {
	case Head:
		return "head"
	case Torso:
		return "torso"
	case LeftArm:
		return "left arm"
	case RightArm:
		return "right arm"
	case LeftLeg:
		return "left leg"
	case RightLeg:
		return "right leg"
	case ThirdArm:
		return "third arm"
	case ExtraEye:
		return "extra eye"
	case Tail:
		return "tail"
	case VestigialTwin:
		return "vestigial twin"
	default:
		return "?"
	}
}

// Short is an abbreviated label that fits the roster's 7-cell bar labels
// (see bar() in render_roster.go), where String()'s "left arm"/"right leg"
// would get truncated mid-word.
func (p BodyPart) Short() string {
	switch p {
	case Head:
		return "head"
	case Torso:
		return "torso"
	case LeftArm:
		return "l.arm"
	case RightArm:
		return "r.arm"
	case LeftLeg:
		return "l.leg"
	case RightLeg:
		return "r.leg"
	case ThirdArm:
		return "3.arm"
	case ExtraEye:
		return "eye"
	case Tail:
		return "tail"
	case VestigialTwin:
		return "twin"
	default:
		return "?"
	}
}

// Vital reports whether destroying this part is fatal on its own. The torso
// carries the vital organs; losing the head is, well, losing the head.
func (p BodyPart) Vital() bool { return p == Head || p == Torso }

// bodyPartWeight is each part's share (out of 100) of an entity's MaxHP,
// used both to size a part's HP pool and to weight which one an attack lands
// on. The torso is the biggest and toughest target (it carries the vital
// organs); the head is vital but small; limbs split the remainder. The *base*
// weights sum to 100 so distributeBodyParts can hand any leftover from integer
// rounding to the torso and still total exactly MaxHP.
//
// A mutant part's weight is deliberately outside that hundred: growing one
// adds its share on top of the body already there (see growPart) instead of
// thinning the parts a colonist was born with, which would make a mutation
// quietly weaken every limb it did not add.
var bodyPartWeight = [numBodyParts]int{
	Head:     15,
	Torso:    35,
	LeftArm:  12,
	RightArm: 12,
	LeftLeg:  13,
	RightLeg: 13,

	ThirdArm:      12,
	ExtraEye:      5,
	Tail:          8,
	VestigialTwin: 15,
}

// distributeBodyParts splits maxHP across the base body parts by
// bodyPartWeight, crediting any rounding remainder to the torso so the parts
// always sum to exactly maxHP. Mutant parts are not included: nobody is born
// with one, and each is added separately by growPart.
func distributeBodyParts(maxHP int) [numBodyParts]int {
	var parts [numBodyParts]int
	sum := 0
	for p := BodyPart(0); p < numBaseBodyParts; p++ {
		parts[p] = maxHP * bodyPartWeight[p] / 100
		sum += parts[p]
	}
	parts[Torso] += maxHP - sum
	return parts
}

// JobKind is the concrete task executing beneath a colonist's FocusKind. Focus
// is the goal, Job owns exact targets/progress/claims, and State is the display
// projection of the work performed this tick.
type JobKind uint8

const (
	JobNone     JobKind = iota
	JobMine             // excavate the Rock tile at Target
	JobBuild            // construct BuildKind on the Floor tile at Target
	JobUse              // walk to the facility at Target and satisfy Drive
	JobTalk             // walk to partner and chat, raising the pair's affinity
	JobClean            // scrub refuse off Target, then haul it to an incinerator
	JobStore            // unload general materials into the storage at Target
	JobDemolish         // break down the Wall tile at Target to escape a sealed room
	JobEat              // take a meal from the depot at Target (if needed) and eat it
	JobCraft            // work a recipe at the workshop at Target
	JobScrape           // scrape the cave scum at Target, then haul it to a scumhouse
	JobScavenge         // (rats) eat the body, gore, or scum at Target where it lies
	JobSell             // take surplus meals from the depot at Target to the silo and offer them
	JobCarry            // carry its own goods to a buyer's depot and ask the price (see producer.go)
	JobTend             // keep its chickens' trough in feed: scrape scum, mix feed at a scumhouse, fill the trough (see chickens.go)
)

// cleanStage is where a JobClean colonist is in the haul. The job is two legs
// with the same shape (walk somewhere, work for a while), so it is one job with
// a stage rather than two job kinds: abandoning it half-done has to release the
// same claim either way, and a colonist that has already picked refuse up must
// never be left holding it (see docs/sanitation.md).
type cleanStage uint8

const (
	cleanGather cleanStage = iota // walking to Target to scrub it clean
	cleanHaul                     // carrying the load to the incinerator at Target
)

// EntityID uniquely identifies an entity for its lifetime. IDs are never reused.
type EntityID uint64

// Memory is a notable event remembered by a colonist. Memories are exposed in
// chronological order through snapshots; routine movement and idling are
// deliberately not recorded. Rule is the stable configured reaction ID that
// produced it — carried for filtering and collapse alongside player-facing
// Text, which is what frontends display.
//
// A Memory can stand for a *run* of the same minor event rather than a single
// occurrence: a colonist who mines twelve times in a row holds one Memory with
// Count 12 spanning Tick..LastTick, not twelve near-identical lines (see
// rememberPercept in world.go and docs/memories.md). An ordinary, uncollapsed memory
// has Count 1 and LastTick == Tick, so a frontend can render every Memory the
// same way and only reach for the span when Count > 1.
type Memory struct {
	Tick     int // when it happened; the first occurrence of a collapsed run
	LastTick int // the most recent occurrence; == Tick unless collapsed
	Count    int // occurrences folded into this memory; 1 when uncollapsed
	Text     string
	Rule     RuleID // stable reaction identity used for consecutive collapse
}

// Entity is a single actor in the world. Rather than a strict ECS, we use one
// struct whose fields the systems interpret according to Kind. This keeps the
// scaffold readable; fields can graduate into real components as systems
// multiply.
type Entity struct {
	ID   EntityID
	Kind Kind
	Pos  Point

	HP    int
	MaxHP int

	// Parts holds current HP per BodyPart for Colonist and Alien (see
	// hasParts); other kinds leave it zero and unused. Combat damage (a bite,
	// a gunshot) lands on one part rather than the aggregate pool: a wound
	// that empties a vital part (Head or Torso) kills outright even if HP
	// remains, the way a called shot should. Non-vital parts (limbs) can be
	// destroyed without being fatal. Initialized by distributeBodyParts so
	// Parts always sums to MaxHP at spawn.
	//
	// MaxParts is the matching ceiling per part, and doubles as the entity's
	// anatomy: a part it does not have reads zero there. It is stored rather
	// than recomputed from MaxHP because mutation makes the two diverge — a
	// grown part adds HP of its own (see growPart in mutation.go), so MaxHP
	// alone no longer says how the body is divided up.
	Parts    [numBodyParts]int
	MaxParts [numBodyParts]int

	// uraniumExposure counts the ticks this colonist has spent under a
	// uranium dose. It is cumulative and never decays; each full
	// UraniumExposureTicks of it is one roll against mutation. Because it
	// never decays, this counter — not MutationChance — is what sets how many
	// colonists ever mutate. See mutation.go.
	uraniumExposure int

	// drives holds each drive's lazy state: a true level as of a tick, the
	// rate it grows at from there, and where it sits in the drive's bands (see
	// drives.go). Storing a base + timestamp instead of ticking every colonist
	// every tick lets idle colonists rest without their drives drifting out of
	// date; the base is folded forward only when the rate changes. Used by
	// colonists (all drives) and rats, chickens and aliens (food only).
	drives [numDrives]driveState
	// driveBase is each drive's base rate for this kind of entity, in
	// thousandths of a point a tick, before any modifier. driveTrait is the
	// percent its traits scale that by (100 = unchanged), and driveActivity
	// the drive activity its rates were last composed for. effects are the
	// drive effects running on it, and nextEffectTick the earliest tick one
	// of them changes stage (0: none). See drive_activity.go and
	// drive_effects.go.
	driveBase      [numDrives]int
	driveTrait     [numDrives]int
	driveActivity  DriveActivity
	effects        []activeEffect
	nextEffectTick int

	// Personality (colonists only). Profile holds the name, attributes, and
	// traits; driveTrait, restTicks, and workScale are the trait-resolved effective
	// parameters the systems read, so the hot paths never re-scan traits. See
	// personality.go.
	Profile   *Profile
	restTicks int // idle rest duration (base scaled by traits)
	// sleepTicks is how long this colonist's night in bed lasts: the sleep
	// drive's UseTicks, give or take a clock hour per sleep trait. asleep is
	// whether it is in bed now, and sleepBanked is the sleep already done
	// tonight, kept when a night is interrupted. See docs/days.md.
	sleepTicks  int
	asleep      bool
	sleepBanked int
	workScale   float64 // mine/build time multiplier (1.0 = baseline)

	// Skills (colonists only). practice is base work ticks of completed work
	// per skill; ranks and labels are derived from it, never stored. yieldAcc
	// counts toward a recipe's next extra unit, and profession is the skill
	// the colonist is known for. See skills.go and docs/skills.md.
	practice [numSkills]uint32
	// earned is what the colonist has been earning at each kind of work, in
	// thousandths of a dollar per 100 ticks, smoothed, and earnedTick when it
	// last earned at it; SkillNone is work with no skill (hauling, supplying).
	// See reservation in valuation.go.
	earned     [numSkills]int64
	earnedTick [numSkills]int
	yieldAcc   [numSkills]int32
	profession SkillKind

	// Inventory is carried by colonists. Each slot contains one homogeneous
	// stack; other entity kinds leave it empty.
	Inventory Inventory
	// wallet is the colonist's dollars (colonists only). Only transfer and mint
	// change it; see money.go and docs/money.md.
	wallet Money
	// podOrigin is the top-left of the crash pod this colonist arrived in,
	// when hasPod; colonists placed directly by tests or older code have none.
	// See crashpod.go.
	podOrigin Point
	hasPod    bool
	// keeper is the colonist a pet (a chicken or a cat) came down with, 0 for
	// a stray. trough is where a chicken eats and where its keeper fills it,
	// when hasTrough; a keeper has the same trough. tend is where a JobTend
	// keeper is. See chickens.go.
	keeper    EntityID
	trough    Point
	hasTrough bool
	tend      tendStage
	// eat is where a JobEat colonist is: fetching a meal, or eating one. See
	// food.go.
	eat eatStage
	// eatKeep marks a JobEat fetch for the pocket, not the mouth: the meal it
	// takes out goes in its pockets and the job ends (see tryPocketMeal).
	eatKeep bool
	// recipe and craftFor are a JobCraft colonist's recipe (an index into
	// recipes) and whose inputs it is working; scrape is where a JobScrape
	// colonist is. See scumhouse.go.
	recipe   int
	craftFor Owner
	craftRun int // recipes worked back to back at this stove (see cooksOn)
	scrape   scrapeStage
	sell     sellStage
	// scrapeFor is whose a JobScrape colonist's scum is (the colony's unless
	// a plan has it scraping for itself); scrapeQty, when set, is the load it
	// stops at. plan is the production plan it is working, if any, and the
	// carry fields are a JobCarry colonist's errand. See producer.go.
	scrapeFor  Owner
	scrapeQty  int
	scrapeKeep bool  // scraping to cook for itself, not to sell
	scrapeSeed bool  // scraping to seed an incubator, not to feed a scumhouse
	seedAt     Point // the incubator a scrapeSeed colonist is loading

	// foraging marks a job a hungry colonist with nothing to eat chose to
	// find food (planForage): it is kept until it ends, never dropped and
	// re-planned mid-way. clearJob clears it. forageNoted is whether this
	// hunger's "went foraging" line has been logged; eating resets it, and
	// forageRetry is the tick a forager that found nothing to do looks again.
	foraging    bool
	forageNoted bool
	forageRetry int

	plan       planID
	carry      carryStage
	carryItem  ItemKind
	carryQty   int
	carryPrice Money
	carryTo    Point
	// carryFor and carryWork, when set, make a JobCarry haul for hire: the
	// goods are carryFor's, and each unit delivered is paid from the work
	// order carryWork. See hauling.go.
	carryFor  Owner
	carryWork OrderID
	// fieldDetour counts down the ticks a JobUse colonist routes concretely
	// instead of following the shared field; see jobUse.
	fieldDetour int
	// commissioned records that this colonist has commissioned its house, so
	// it commissions one at most (see commissionHouses).
	commissioned bool
	// kitchenCommissioned records that it has commissioned a kitchen of its
	// own, and kitchen where its stove stands once built (hasKitchen). See
	// commissionKitchens.
	kitchenCommissioned, hasKitchen bool
	kitchen                         Point
	// cargo records which carried units are not the carrier's own: the
	// colony's ore a builder fetched, the meals a hauler is moving. Every unit
	// on no line is the carrier's. See cargo.go and docs/property.md.
	cargo []cargoLine
	// kin is the colonist's node in the colony's family tree (colonists only; 0
	// for aliens). Relations caches the derived display ties until the family
	// tree changes. See relationships.go.
	kin              kinID
	relations        []Relation
	relationRevision uint64

	// affect is the colonist's bounded charge/grip state and cached display label.
	// Focus scoring reads only its numeric axes; the label is display-only.
	affect     AffectState
	affectHome MoodVector

	// Focus is the colonist's current goal; Job is the concrete executor beneath
	// it. mindDirty and nextThinkTick gate arbitration only; the selected executor
	// still runs every tick. Stimuli and their aggregate score use fixed storage so
	// ingestion and arbitration never allocate.
	focus              FocusKind
	focusSince         int
	mindDirty          bool
	nextThinkTick      int
	stimuli            [MaxActiveStimuli]Stimulus
	stimulusCount      int
	stimulusFocusBias  [numFocusKinds]int
	nextStimulusExpiry int

	// Memories is a bounded history of notable experiences. The internal slice
	// is copied into EntityView so frontends cannot mutate the live world.
	Memories   []Memory
	perceiving map[perceptionKey]Occurrence // persistent facts currently in range
	seesThreat bool                         // cached alien-presence edge; fight/flee still use nearestAlien

	// Social conversation fatigue is counted within a rolling social window.
	socialTalkCount   int
	socialWindowStart int
	socialCapacity    int
	socialPenalty     int

	// Current job and its parameters.
	Job            JobKind
	Target         Point     // tile the job operates on or travels to
	BuildKind      Terrain   // JobBuild: terrain to construct
	Drive          DriveKind // JobUse: which drive this fulfills
	useFacility    Point
	useFacilitySet bool
	// carrying reports whether a JobUse colonist has grabbed a portable drive
	// (see DriveSpec.GrabTicks) and stepped away from the facility to finish it,
	// rather than still occupying the facility's access tile.
	carrying bool
	partner  EntityID // JobTalk: the colonist being talked with; 0 if none
	Progress int      // ticks accumulated on the current action

	// Rest scheduling: an idle colonist with no available work rests (skips the
	// work search) until wakeTick instead of re-scanning the map every tick.
	resting  bool
	wakeTick int

	// Cached navigation: path is the remaining A* route toward a tile adjacent to
	// pathGoal, followed one step per tick (pathAt is the next step). stuck counts
	// consecutive ticks blocked by another colonist before the job is abandoned.
	path     []Point
	pathAt   int
	pathGoal Point
	stuck    int

	// clean is the stage of a JobClean colonist's haul (gather, then deliver);
	// meaningless for any other job. Target means the refuse tile while
	// gathering and the incinerator while hauling.
	clean cleanStage

	// mineClaimed reports whether a JobMine colonist has claimed a specific rock
	// (Target) to dig, as opposed to still following the frontier field to reach
	// the digging edge.
	mineClaimed bool

	// task is the construction-project task this colonist has claimed (nil unless
	// it is building one). Distinguishes coordinated project work from a lone
	// emergency build.
	task *buildTask

	// disconnectedTicks counts consecutive ticks this colonist's room has been
	// cut off from the colony's main connected network (see World.mainRoom).
	// Reset to 0 the moment it can reach mainRoom again. See FocusEscape.
	disconnectedTicks int

	// Display + shared behavior scratch.
	State  State
	Quarry EntityID // (predator) the prey being hunted; 0 if none
	// Cooldown paces repeated actions: predators between attacks, and an
	// armed colonist between shots while fighting an alien (see fightAlien).
	Cooldown int

	// Species indexes World.alienSpecies (Alien only; see spawn in
	// world.go and alienSpeciesFor in lore.go): which of this world's
	// rolled alien species this individual belongs to.
	Species int

	// Rat reproduction (rats only). sex decides who can carry a litter; a
	// female rat that mates becomes pregnant until dueTick, when she births a
	// litter. mateReadyTick gates breeding: it holds a newborn back until it
	// matures and spaces out a female's litters after she gives birth.
	sex           Sex
	pregnant      bool
	dueTick       int
	mateReadyTick int
}

// newEntity builds an entity with kind-appropriate starting stats. Colonists get
// baseline effective parameters here; assignPersonality later scales them by any
// traits it rolls.
func newEntity(id EntityID, kind Kind, p Point, cfg Config) *Entity {
	e := &Entity{ID: id, Kind: kind, Pos: p, State: Idle, workScale: 1, focus: FocusIdle}
	if kind == Colonist {
		e.mindDirty = true
	}
	if kind == Colonist {
		e.affect.Label = MoodSteady
	}
	if kind == Colonist {
		e.perceiving = make(map[perceptionKey]Occurrence)
	}
	switch kind {
	case Colonist:
		e.MaxHP = cfg.ColonistHP
		for i := 0; i < int(numDrives); i++ {
			e.driveBase[i] = cfg.Drives[i].Rate
		}
		e.driveActivity = DriveIdle
		e.restTicks = cfg.RestTicks
		e.sleepTicks = cfg.Drives[DriveSleep].UseTicks
	case Alien:
		e.MaxHP = cfg.AlienHP
		// Only the food drive rises: Friendly and Cautious species graze cave
		// scum when it presses (see alienGraze). Aliens never starve.
		e.driveBase[DriveFood] = cfg.AlienHungerRate
	case Cat:
		e.MaxHP = cfg.CatHP
	case Rat:
		e.MaxHP = cfg.RatHP
		// Rats share the colonists' DriveFood but nibble constantly, so only their
		// food drive rises (fast); the others stay flat.
		e.driveBase[DriveFood] = cfg.RatHungerRate
	case Chicken:
		e.MaxHP = cfg.ChickenHP
		// Like a rat, a chicken has only the food drive.
		e.driveBase[DriveFood] = cfg.ChickenHungerRate
	}
	for i := range e.driveTrait {
		e.driveTrait[i] = 100
	}
	e.HP = e.MaxHP
	if e.hasParts() {
		e.Parts = distributeBodyParts(e.MaxHP)
		e.MaxParts = e.Parts
	}
	return e
}

// hasParts reports whether this entity's wounds are tracked per body part.
// Cats and rats die from a single pounce or stomp regardless of HP, so they
// have no need of the detail.
func (e *Entity) hasParts() bool { return e.Kind == Colonist || e.Kind == Alien }

// hasPart reports whether this entity actually has a given body part: every
// base part for a kind tracked by body part, plus whichever mutant parts it
// has grown. A destroyed part is still a part it has — MaxParts keeps its
// ceiling — so "gone" and "never grown" stay distinguishable.
func (e *Entity) hasPart(p BodyPart) bool { return p < numBodyParts && e.MaxParts[p] > 0 }

// Alive reports whether the entity still has hit points and, for a kind
// tracked by body part, has not had a vital part (Head or Torso) destroyed —
// a called shot kills even with HP still nominally in the tank.
func (e *Entity) Alive() bool {
	if e.HP <= 0 {
		return false
	}
	if e.hasParts() && (e.Parts[Head] <= 0 || e.Parts[Torso] <= 0) {
		return false
	}
	return true
}

// displayName is the colonist's name for player-facing text (logs, memories),
// or a numbered fallback if it has no profile.
func (e *Entity) displayName() string {
	if e.Profile != nil && e.Profile.Name != "" {
		return e.Profile.Name
	}
	return fmt.Sprintf("colonist #%d", e.ID)
}

// possessive is the colonist's possessive determiner for log lines: never
// "its", since colonists have pronouns. Falls back to "their" with no profile.
func (e *Entity) possessive() string {
	if e.Profile == nil {
		return "their"
	}
	return e.Profile.Gender.Possessive()
}

// clearPath discards any cached navigation route.
func (e *Entity) clearPath() {
	e.path = e.path[:0]
	e.pathAt = 0
	e.stuck = 0
}
