package sim

import "fmt"

// Kind identifies what an entity fundamentally is. Behavior is dispatched on
// Kind by the per-tick systems.
type Kind uint8

const (
	// Colonist is a human worker: walks on Floor, mines Rock, builds structures,
	// tends to its needs, and flees from aliens.
	Colonist Kind = iota
	// Alien is a subterranean mutant that burrows through any terrain to hunt
	// and eat colonists.
	Alien
	// Cat is a surface predator that stalks the floor hunting mice. It has no
	// needs of its own; it hunts by instinct.
	Cat
	// Mouse is a pest that scurries the floor and nibbles from nutrient pods.
	// It has a hunger need and starves without food; cats eat it.
	Mouse

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
	case Mouse:
		return "mouse"
	default:
		return "unknown"
	}
}

// State is a coarse label for what an entity is currently doing. It is derived
// from the entity's Job each tick and surfaced in the UI.
type State uint8

const (
	Idle      State = iota
	Moving          // travelling toward Target
	Mining          // excavating Rock
	Building        // constructing a structure
	Eating          // using a nutrient pod
	Relieving       // using a toilet
	Sleeping        // sleeping in a bed
	Fleeing         // running from a nearby predator (colonist from alien, mouse from cat)
	Hunting         // predator closing on prey (alien on colonist, cat on mouse)
	Feeding         // predator eating prey it has caught
	Talking         // chatting with another colonist (builds affinity)
	Stomping        // colonist chasing down and crushing a pest mouse
	Fighting        // armed colonist standing its ground and firing on an alien
	Cleaning        // colonist scrubbing refuse off a tile, or feeding the incinerator
	Hauling         // colonist carrying gathered refuse to an incinerator
	Storing         // colonist unloading general materials into storage
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
	default:
		return "?"
	}
}

// BodyPart identifies one wound location tracked separately from an entity's
// overall HP. Only Colonist and Alien use body parts (see Entity.hasParts);
// cats and mice stay on a single HP pool, since nothing hits them with
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
	JobNone  JobKind = iota
	JobMine          // excavate the Rock tile at Target
	JobBuild         // construct BuildKind on the Floor tile at Target
	JobUse           // walk to the facility at Target and satisfy Need
	JobTalk          // walk to partner and chat, raising the pair's affinity
	JobClean         // scrub refuse off Target, then haul it to an incinerator
	JobStore         // unload general materials into the storage at Target
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

	// Needs are stored lazily: Needs[i] is the level as of tick needSince[i], so
	// the current level is Needs[i] + needRise[i]*(now-needSince[i]) (see
	// needLevel). Storing a base + timestamp instead of ticking every colonist
	// every tick lets idle colonists rest without their needs drifting out of
	// date. Used by colonists (all needs) and mice (food only).
	Needs             [numNeeds]int
	needSince         [numNeeds]int
	needPhase         [numNeeds]NeedPhase
	nextNeedPhaseTick [numNeeds]int
	// starvationDamage tracks HP lost to each fatal need separately from wounds.
	// Satisfying that need restores its own deprivation damage without healing
	// unrelated injuries such as alien bites.
	starvationDamage [numNeeds]int

	// Personality (colonists only). Profile holds the name, attributes, and
	// traits; needRise, restTicks, and workScale are the trait-resolved effective
	// parameters the systems read, so the hot paths never re-scan traits. See
	// personality.go.
	Profile   *Profile
	needRise  [numNeeds]int // per-need rise per tick (base scaled by traits)
	restTicks int           // idle rest duration (base scaled by traits)
	workScale float64       // mine/build time multiplier (1.0 = baseline)

	// Inventory is carried by colonists. Each slot contains one homogeneous
	// stack; other entity kinds leave it empty.
	Inventory Inventory
	// kin is the colonist's node in the colony's family tree (colonists only; 0
	// for aliens). Relations caches the derived display ties until the family
	// tree changes. See relationships.go.
	kin              kinID
	relations        []Relation
	relationRevision uint64

	// affect is the colonist's bounded charge/grip state and cached display label.
	// Focus scoring reads only its numeric axes; the label is display-only.
	affect AffectState

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
	seesThreat bool                         // cached alien-presence transition state

	// Social conversation fatigue is counted within a rolling social window.
	socialTalkCount   int
	socialWindowStart int
	socialCapacity    int
	socialPenalty     int

	// Current job and its parameters.
	Job            JobKind
	Target         Point    // tile the job operates on or travels to
	BuildKind      Terrain  // JobBuild: terrain to construct
	Need           NeedKind // JobUse: which need this fulfills
	useFacility    Point
	useFacilitySet bool
	// carrying reports whether a JobUse colonist has grabbed a portable need
	// (see NeedSpec.GrabTicks) and stepped away from the facility to finish it,
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

	// Display + shared behavior scratch.
	State  State
	Quarry EntityID // (predator) the prey being hunted; 0 if none
	// Cooldown paces repeated actions: predators between attacks, and an
	// armed colonist between shots while fighting an alien (see fightAlien).
	Cooldown int

	// Mouse reproduction (mice only). sex decides who can carry a litter; a
	// female mouse that mates becomes pregnant until dueTick, when she births a
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
		for i := 0; i < int(numNeeds); i++ {
			e.needRise[i] = cfg.Needs[i].Rise
		}
		e.restTicks = cfg.RestTicks
	case Alien:
		e.MaxHP = cfg.AlienHP
	case Cat:
		e.MaxHP = cfg.CatHP
	case Mouse:
		e.MaxHP = cfg.MouseHP
		// Mice share the colonists' NeedFood but nibble constantly, so only their
		// food need rises (fast); the others stay flat.
		e.needRise[NeedFood] = cfg.MouseHungerRise
	}
	e.HP = e.MaxHP
	if e.hasParts() {
		e.Parts = distributeBodyParts(e.MaxHP)
		e.MaxParts = e.Parts
	}
	return e
}

// hasParts reports whether this entity's wounds are tracked per body part.
// Cats and mice die from a single pounce or stomp regardless of HP, so they
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

// clearPath discards any cached navigation route.
func (e *Entity) clearPath() {
	e.path = e.path[:0]
	e.pathAt = 0
	e.stuck = 0
}
