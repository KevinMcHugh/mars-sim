package sim

// A Species is what kind of creature an entity is, as a value rather than a
// type switch: its name and perception noun, its body, where it spawns, and the
// stats newEntity gives it. See docs/species-and-behaviors.md for where this is
// heading (behaviors as an ordered ladder, components, alien species as
// generated values).
//
// The table is indexed by Kind. kindIdentity holds the half that never
// changes with Config and is readable without a World (Kind.String, hasParts);
// newSpeciesTable fills in the Config-driven stats a World keeps in
// World.species.
type Species struct {
	Kind Kind
	// Name is the lowercase word for one of them ("rat"): Kind.String, and the
	// "rat #12" label an unnamed creature goes by.
	Name string
	// Noun is the creature's word in the perception grammar, empty for one
	// nothing perceives yet (chickens).
	Noun NounID
	// Body marks per-part HP (Head, Torso, limbs); see hasParts.
	Body bool
	// Spawn is where Engine.spawn places a new one.
	Spawn spawnSite

	// HP is its starting and maximum hit points.
	HP int
	// HungerRate is the base rate of its food drive, 0 for a creature with no
	// hunger. Only colonists have the other drives, and they are seeded from
	// Config.Drives in newEntity, not from here.
	HungerRate int

	// The rest drives animalTurn (cats, rats, chickens); colonists and aliens
	// still have turns of their own.
	//
	// Starves: a full food drive kills it, leaving a Corpse.
	Starves bool
	Corpse  ItemKind
	// Breeds: one is spawned with a Breeding component and a rolled sex.
	Breeds bool
	// Paced: it acts once every Slowness ticks, counting down Cooldown in
	// between.
	Paced    bool
	Slowness int
	// ladder is its behaviors in priority order (behaviors.go).
	ladder []behavior
}

// spawnSite is where Engine.spawn puts a new creature of a species.
type spawnSite uint8

const (
	spawnFloor  spawnSite = iota // any floor tile
	spawnShip                    // in a colony ship of its own (colonists)
	spawnCavern                  // on hidden cavern floor near the map center (aliens)
)

// kindIdentity is every species' Config-independent identity.
var kindIdentity = [numKinds]Species{
	Colonist: {Kind: Colonist, Name: "colonist", Noun: NounColonist, Body: true, Spawn: spawnShip},
	Alien:    {Kind: Alien, Name: "alien", Noun: NounAlien, Body: true, Spawn: spawnCavern},
	Cat:      {Kind: Cat, Name: "cat", Noun: NounCat, Spawn: spawnFloor},
	Rat:      {Kind: Rat, Name: "rat", Noun: NounRat, Spawn: spawnFloor},
	Chicken:  {Kind: Chicken, Name: "chicken", Spawn: spawnFloor},
}

// newSpeciesTable builds the per-world species table: kindIdentity plus the
// stats cfg sets.
func newSpeciesTable(cfg Config) [numKinds]Species {
	t := kindIdentity
	t[Colonist].HP = cfg.ColonistHP
	t[Colonist].HungerRate = cfg.Drives[DriveFood].Rate

	t[Alien].HP = cfg.AlienHP
	// Aliens never starve, but the food drive rises: Friendly and Cautious
	// species graze cave scum when it presses (see alienGraze).
	t[Alien].HungerRate = cfg.AlienHungerRate

	// Cats have no drives: they hunt rats by instinct, and roam when there
	// are none.
	cat := &t[Cat]
	cat.HP = cfg.CatHP
	cat.Paced, cat.Slowness = true, cfg.CatSlowness
	cat.ladder = []behavior{
		hunt{prey: Rat, rest: cfg.CatPounceRest},
		wander{},
	}

	// Rats have only the food drive, rising fast. They bolt from cats, eat
	// the colony's leavings, and breed when nothing presses (see
	// docs/entities-and-ai.md).
	rat := &t[Rat]
	rat.HP, rat.HungerRate = cfg.RatHP, cfg.RatHungerRate
	rat.Starves, rat.Corpse = true, AnimalCorpse
	rat.Breeds = true
	rat.ladder = []behavior{
		flee{from: Cat, radius: cfg.RatFleeRadius},
		forage{sources: []foodSource{forageScavenge, foragePod}},
		breed{},
		wander{},
	}

	// Chickens eat from their keeper's trough, else graze scum, and stay
	// near the trough. See docs/chickens.md.
	chicken := &t[Chicken]
	chicken.HP, chicken.HungerRate = cfg.ChickenHP, cfg.ChickenHungerRate
	chicken.Starves, chicken.Corpse = true, AnimalCorpse
	chicken.Paced, chicken.Slowness = true, cfg.ChickenSlowness
	chicken.ladder = []behavior{
		forage{sources: []foodSource{(*World).chickenFeed, (*World).chickenGraze}},
		stayNearTrough{roam: cfg.ChickenRoam},
		wander{},
	}
	return t
}

// speciesOf is the species an entity belongs to.
func (w *World) speciesOf(e *Entity) *Species { return &w.species[e.Kind] }
