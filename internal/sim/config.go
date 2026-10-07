package sim

import "time"

// Config holds every tunable knob for a simulation run in one place, so
// balancing the game means editing values here rather than hunting through the
// systems. Zero values are not meaningful; use DefaultConfig and adjust.
type Config struct {
	// World shape.
	Width  int `cfg:"width" sec:"World" doc:"world width in tiles"`
	Height int `cfg:"height" doc:"world height in tiles"`
	// WorldgenHalo is how far (in 64x64 chunks) world generation stays ahead
	// of the colony: every chunk within this many of one holding a tile the
	// colony has seen is generated, and no other. Aliens spawn only in caves
	// of generated chunks, so this also sets how far out they can start. See
	// docs/worldgen-chunks.md.
	WorldgenHalo int `cfg:"worldgen-halo" doc:"chunks (64x64 tiles) generated ahead of what the colony has seen; at least 1"`
	// Rock composition percentages. The remainder is ordinary rock.
	IronRockPercent    int `cfg:"iron-rock-percent" doc:"percent of the map's tiles bearing iron"`
	IceRockPercent     int `cfg:"ice-rock-percent" doc:"percent of the map's tiles bearing water ice"`
	UraniumRockPercent int `cfg:"uranium-rock-percent" doc:"percent of the map's tiles bearing uranium"`
	ClayRockPercent    int `cfg:"clay-rock-percent" doc:"percent of the map's tiles bearing clay"`
	RockVeinMin        int `cfg:"rock-vein-min" doc:"minimum tiles in a generated rock deposit vein"`
	RockVeinMax        int `cfg:"rock-vein-max" doc:"maximum tiles in a generated rock deposit vein"`
	// Salt rides on the rock like cave scum, never on a tile that carries
	// scum, and never comes back once taken or built over. See docs/salt.md.
	SaltPercent int `cfg:"salt-percent" doc:"percent of the map's tiles carrying a deposit of salt"`
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
	StartCats      int `cfg:"cats" doc:"starting number of stray cats (colonists also bring cats: see crash-pod-cat-weight)"`
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
	// The wealth levy returns money to the treasury, which otherwise only
	// pays out: every TaxInterval ticks each colonist pays WealthTax percent
	// of whatever it holds above TaxFloor. Without it the treasury ran dry
	// by tick 20000 in a 20-colonist colony, the colony stopped buying
	// biomatter, and most of the colony starved. See docs/money.md.
	WealthTax   int   `cfg:"wealth-tax" doc:"percent of a colonist's money above tax-floor paid to the treasury each tax-interval (0 disables)"`
	TaxFloor    int64 `cfg:"tax-floor" doc:"dollars a colonist keeps untaxed by the wealth levy"`
	TaxInterval int   `cfg:"tax-interval" doc:"ticks between wealth levies"`
	// InfiniteFood is the safety net: nutrient pods make meals out of nothing.
	// Off (the default since economy phase E8), a pod serves nothing and the
	// colony eats only what it landed with and what it produces. On is for
	// tests and balancing. See docs/food.md.
	InfiniteFood bool `cfg:"infinite-food" doc:"nutrient pods make free meals out of nothing (the safety net)"`
	// ZoningAuto is who decides where the colony builds. Off (the game's
	// default), the player draws zones and colonists build a structure only
	// inside a zone of its kind; with no such zone they build nothing. On,
	// the colony sites its rooms itself, as it always did, and zones each one
	// as it marks it out. The terminal and headless runs have no way to draw
	// a zone, so the committed mars-sim.yaml turns this on. See
	// docs/zoning.md.
	ZoningAuto bool `cfg:"zoning-auto" doc:"colonists choose where to build and zone it themselves; off, they build only inside zones the player draws (the browser's Zones tab)"`

	// Food production. Cave scum is a biofilm on cave surfaces, the renewable
	// base of the food chain: ScumPercent of the map's tiles carry a patch of
	// up to ScumMax units to start with. After that scum accretes (growScum): it
	// spawns anywhere at a low fixed chance and spreads from existing patches,
	// up to that share of the map. A scumhouse turns scum and every other kind of
	// biomatter into meals by the recipes in scumhouse.go. The colony makes
	// food while it holds fewer than MealReserve meals per colonist. See
	// docs/scumhouse.md.
	ScumPercent       int `cfg:"scum-percent" sec:"Food production" doc:"percent of the map's tiles carrying a patch of cave scum"`
	ScumMax           int `cfg:"scum-max" doc:"units of scum a full patch holds"`
	ScumSpawnPPM      int `cfg:"scum-spawn-ppm" doc:"chance in a million, per tile visited, that scum appears on rock from nothing"`
	ScumSpreadPercent int `cfg:"scum-spread-percent" doc:"percent chance a tile visited grows a unit if a tile in or beside it holds scum"`
	ScrapeTicks       int `cfg:"scrape-ticks" doc:"ticks of work to scrape one unit of scum off a patch"`

	// The scum incubator grows scum on a schedule from a seed colonists load
	// into it, and the colony's food work draws on it instead of scraping the
	// rock. Wild scraping is for seeding incubators and for dire times: no
	// incubator built, or meals below ScumDireMeals per colonist with none
	// ripe. IncubatorGrowTicks 0 turns incubators off. See docs/incubator.md.
	IncubatorGrowTicks    int   `cfg:"incubator-grow-ticks" doc:"ticks between each unit of scum an incubator grows (0: no incubators)"`
	IncubatorCapacity     int   `cfg:"incubator-capacity" doc:"units of scum an incubator holds before it stops growing"`
	IncubatorSeed         int   `cfg:"incubator-seed" doc:"units of scum an incubator needs loaded to grow, and keeps back when harvested"`
	ColonistsPerIncubator int   `cfg:"colonists-per-incubator" doc:"the colony plans another incubator for each this many colonists"`
	ScumDireMeals         int   `cfg:"scum-dire-meals" doc:"meals per colonist below which the colony scrapes wild scum instead of waiting on its incubators"`
	WageHarvest           int64 `cfg:"wage-harvest" doc:"what the colony pays a colonist for each load of scum it carries from an incubator to a scumhouse"`
	MealReserve           int   `cfg:"meal-reserve" doc:"the colony makes food while it holds fewer meals than this per colonist"`
	// Rations: a colonist at critical hunger who cannot afford a meal is
	// given one of the colony's. See docs/food.md.
	Rations bool `cfg:"rations" doc:"the colony gives a meal to a colonist at critical hunger who cannot afford one"`
	// ConstructionCosts makes building consume materials: raw rock, and ore
	// for machines (see constructionCost), paid from the stock of whoever pays
	// for the work, then the builder's. On by default since economy phase E8;
	// off, building is free. See docs/construction.md.
	ConstructionCosts bool `cfg:"construction-costs" doc:"building consumes materials, from the payer's stock first, then the builder's"`

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
	MealPriceMax    int   `cfg:"meal-price-max" doc:"what the colony asks for a meal with its shelves bare, as a percent of price-meal; it rises to this as its stock falls below meal-reserve per colonist (100: a fixed price)"`
	PocketMealAt    int   `cfg:"pocket-meal-at" doc:"food need at which a colonist with no meal on it fetches one of its own to carry, before it's hungry enough to eat (0: never)"`

	// Price discovery (docs/pricing.md). A colonist's unsold food ask comes
	// down AskDecayPercent every AskDecayTicks, to $1 (a meal, to what its
	// scum costs); with RelistIdle,
	// colonists offer idle food they own at the kitchens and the silo at its
	// value. A hungry colonist's waiting meal bid starts at BidStartPercent
	// of a meal's value and rises BidRaisePercent of its limit every
	// BidRaiseTicks to that limit, which grows with hunger toward half its
	// money (all of it at critical), whatever the last meal sold for. With MealSellAtMarket, colonists sell meals, and judge
	// whether cooking pays, at a meal's market value, not price-meal.
	AskDecayTicks    int  `cfg:"ask-decay-ticks" doc:"ticks a colonist's unsold ask for food (meals, scum, other biomatter) waits before it comes down in price (0: asks never move)"`
	AskDecayPercent  int  `cfg:"ask-decay-percent" doc:"percent a colonist's unsold food ask comes down each time, at least $1, never below $1, or for a meal below what its scum costs"`
	RelistIdle       bool `cfg:"relist-idle" doc:"colonists offer idle food they own at the kitchens and the silo for sale at its market value: meals beyond meal-keep, scum beyond one recipe's worth"`
	BidStartPercent  int  `cfg:"bid-start-percent" doc:"a hungry colonist's waiting bid for a meal starts at this percent of a meal's value, or its limit if lower"`
	BidRaiseTicks    int  `cfg:"bid-raise-ticks" doc:"ticks a hungry colonist's waiting meal bid goes unfilled before it raises it toward its limit (0: it bids its limit at once)"`
	BidRaisePercent  int  `cfg:"bid-raise-percent" doc:"how much a hungry colonist raises its waiting meal bid each time, as a percent of its limit (what its hunger and money will pay), at least $1"`
	FreePricesAt     int  `cfg:"free-prices-at" doc:"prices float only once the colony has held this many meals per colonist, its kitchens running; until then trades don't move them, and unsold asks don't come down (0: they float from landing)"`
	MealSellAtMarket bool `cfg:"meal-sell-at-market" doc:"colonists price the meals they sell, and judge whether cooking pays, at a meal's market value (off: at price-meal)"`

	// Skills. A colonist is credited SkillPracticePercent percent of the base
	// ticks of every unit of work it completes, in that work's skill; ranks
	// come from practice, and each rank makes the work faster and, for
	// recipes, more productive. With Skills off, nobody arrives with a
	// background and no rank has an effect (practice is still counted). See
	// docs/skills.md.
	Skills               bool `cfg:"skills" sec:"Skills" doc:"colonists arrive with skills, and skilled work is faster and yields more (off: no backgrounds, no effects)"`
	SkillPracticePercent int  `cfg:"skill-practice-percent" doc:"percent of a unit of work's base ticks credited as practice in its skill"`

	// Valuation and the producer planner. A colonist values its own time at
	// LaborPrice dollars per 100 ticks of work, and takes on a plan only if it
	// clears PlanMinProfit after inputs and labor. It considers the
	// PlanCandidates best bids it could fill. A hungry colonist's unfilled bid
	// for a meal rests for DemandTTL ticks; a plan that has not delivered in
	// PlanTTL ticks is dropped, its derived bids with it. See
	// docs/valuation.md.
	LaborPrice     int64 `cfg:"labor-price" sec:"Valuation" doc:"what a colonist reckons 100 ticks of its own work are worth, in dollars"`
	PlanMinProfit  int64 `cfg:"plan-min-profit" doc:"the least profit, in dollars, that makes a production plan worth taking on"`
	PlanCandidates int   `cfg:"plan-candidates" doc:"how many of the best open bids a colonist's producer planner considers"`
	PlanTTL        int   `cfg:"plan-ttl" doc:"ticks a production plan may take before it is dropped with its derived bids"`
	RateMemory     int   `cfg:"rate-memory" doc:"ticks over which what a colonist earned at a kind of work fades back to labor-price in its reckoning of what its time is worth (0: it always reckons labor-price)"`
	DemandTTL      int   `cfg:"demand-ttl" doc:"ticks a hungry colonist's unfilled bid for a meal rests in the book"`

	// The colony buys biomatter at its scumhouses: it keeps ScumhouseBidQty
	// units of standing bids per kind at each, at these prices, and sells the
	// meals it cooks at price-meal. See docs/scumhouse.md.
	PriceCaveScum     int64 `cfg:"price-cave-scum" doc:"what the colony pays for a unit of cave scum at its scumhouses"`
	PriceViscera      int64 `cfg:"price-viscera" doc:"what the colony pays for a unit of viscera at its scumhouses"`
	PriceAnimalCorpse int64 `cfg:"price-animal-corpse" doc:"what the colony pays for an animal carcass at its scumhouses"`
	PriceAlienCorpse  int64 `cfg:"price-alien-corpse" doc:"what the colony pays for an alien carcass at its scumhouses"`
	ScumhouseBidQty   int   `cfg:"scumhouse-bid-qty" doc:"units of each kind of biomatter the colony keeps a standing bid for at each scumhouse"`
	// The colony stops buying a kind of biomatter at a scumhouse once it holds
	// ScumhouseStockCap units of it there — more than its cooks can get
	// through soon is money spent on a pile — and plans a scumhouse for
	// every ColonistsPerScumhouse colonists, since one cook works one at a
	// time. See docs/scumhouse.md.
	ScumhouseStockCap     int `cfg:"scumhouse-stock-cap" doc:"units of each kind of biomatter the colony holds at a scumhouse before it stops buying more"`
	ColonistsPerScumhouse int `cfg:"colonists-per-scumhouse" doc:"the colony plans another scumhouse for each this many colonists"`

	// The foundry: a forge smelts iron ore into steel ingots, and a gun bench
	// machines steel into assault rifles. The colony wants ArmoryRifles of
	// them in its armory (the silo) and keeps a standing bid for the
	// shortfall at PriceAssaultRifle, once it has a gun bench; the producer
	// planner does the rest, from the ore vein up. 0 builds no foundry. See
	// docs/foundry.md.
	ArmoryRifles      int   `cfg:"armory-rifles" sec:"Foundry" doc:"assault rifles the colony wants in its armory (0: it builds no foundry and buys none)"`
	PriceAssaultRifle int64 `cfg:"price-assault-rifle" doc:"what the colony pays for an assault rifle at its silo"`

	// Colony standing orders. With StandingOrdersBuildOnly, the colony posts
	// no standing orders but its silo bids for building materials
	// (buildGoods). The rest (ore resales, meal asks, biomatter, water,
	// uranium and rifle bids) mostly rested unfilled and buried the book;
	// feeding the colony is the player's call, made with an order on the
	// Market tab. See docs/colony-orders.md.
	StandingOrdersBuildOnly bool `cfg:"standing-orders-build-only" sec:"Colony standing orders" doc:"the colony posts no standing orders but its bids for building materials (rock, iron ore, clay)"`

	// Hauling and the colony as seller. With ColonySells, the colony offers
	// what it bought at its silo beyond ColonyStockReserve units of each good
	// (kept for public works), at ColonyMarkup percent over the reference
	// price. It keeps SiloMealStock of its meals at the silo, paying HaulPay
	// a unit to have them hauled in from its scumhouses. See docs/hauling.md.
	ColonySells        bool  `cfg:"colony-sells" sec:"Hauling" doc:"the colony sells the goods it bought, beyond its reserve, at its silo"`
	ColonyMarkup       int   `cfg:"colony-markup" doc:"percent over the reference price the colony asks for what it sells"`
	ColonyStockReserve int   `cfg:"colony-stock-reserve" doc:"units of each good the colony keeps back from sale for public works"`
	SiloMealStock      int   `cfg:"silo-meal-stock" doc:"meals the colony keeps at its silo, hauled in for hire from its scumhouses"`
	HaulPay            int64 `cfg:"haul-pay" doc:"what the colony pays per unit hauled to its silo"`

	// Labor. The colony pays for its public works: every task of a room it
	// plans is a work order funded from the treasury at these wages, and a
	// room it cannot fund is not planned. It pays WageCook each time a cook
	// works a recipe on the colony's stock. A colonist with HouseSavings
	// dollars commissions its own house (0 disables), whose toilet charges
	// others ToiletFee a use. WageDemolish pays for tearing a room's wall down
	// to enlarge it: dearer than raising one, since the rock is not salvaged
	// (see docs/room-expansion.md). See docs/labor.md.
	WageDig        int64 `cfg:"wage-dig" sec:"Labor" doc:"what the colony pays to dig out one tile of a room"`
	WageWall       int64 `cfg:"wage-wall" doc:"what the colony pays to raise one wall"`
	WageFixture    int64 `cfg:"wage-fixture" doc:"what the colony pays to build one fixture (pod, toilet, bed, ...)"`
	WageDemolish   int64 `cfg:"wage-demolish" doc:"what the colony pays to tear down one structure tile: a wall moved to enlarge a room, or anything a clearing order takes down"`
	WageCook       int64 `cfg:"wage-cook" doc:"what the colony pays a cook each time it works a recipe on the colony's stock"`
	HouseSavings   int64 `cfg:"house-savings" doc:"a colonist with this much money commissions its own house (0 disables)"`
	KitchenRank    int   `cfg:"kitchen-rank" doc:"cooking rank at which a colonist buys a kitchen of its own when the shared stoves are crowded (3: a chef; 0 disables)"`
	KitchenSavings int64 `cfg:"kitchen-savings" doc:"money a chef needs to commission its own kitchen: the room (about $50) and scum to cook in it"`
	ToiletFee      int64 `cfg:"toilet-fee" doc:"what a house's toilet charges anyone but its owner per use (0: private)"`

	// Arrivals. Every colonist arrives aboard a colony ship — at worldgen,
	// from the spawn command, or from a director arrival — with a locker of
	// its own, communal bunks and toilets shared with its shipmates, and this
	// manifest. The crash-pod-* names are from the one-pod-per-colonist
	// arrivals ships replaced. See ship.go and docs/ships.md.
	ShipCapacity      int `cfg:"ship-capacity" sec:"Arrivals" doc:"most settlers one colony ship carries; a larger wave comes down in several"`
	ShipBunkPercent   int `cfg:"ship-bunk-percent" doc:"communal bunks a ship carries, as a percent of its passengers (rounded up)"`
	ShipToiletPercent int `cfg:"ship-toilet-percent" doc:"communal toilets a ship carries, as a percent of its passengers (rounded up)"`
	// A ship comes down as a stick (its rooms in a row), a hub and spoke, or
	// a knobby cluster, picked per ship by these relative weights. All three
	// at 0 is all sticks. See docs/ships.md.
	ShipStickWeight   int `cfg:"ship-stick-weight" doc:"relative odds a colony ship is a stick: its rooms in a row along one aisle"`
	ShipHubWeight     int `cfg:"ship-hub-weight" doc:"relative odds a colony ship is a hub and spoke: a concourse with a room down each spoke"`
	ShipClusterWeight int `cfg:"ship-cluster-weight" doc:"relative odds a colony ship is a knobby cluster: rooms budding off a spine corridor"`
	// PlaceShips leaves the founders' ships aloft at worldgen for a frontend
	// to land one by one with LandShip before the first tick. The browser
	// sets it; anything still aloft when the game starts lands by itself.
	PlaceShips bool `cfg:"place-ships" doc:"hold the founders' ships aloft for the player to land one by one (the browser does)"`

	CrashPodPurse      int64 `cfg:"crash-pod-purse" doc:"dollars each colonist arrives with"`
	CrashPodMeals      int   `cfg:"crash-pod-meals" doc:"meals stocked in each colonist's locker, on average"`
	CrashPodMealSpread int   `cfg:"crash-pod-meal-spread" doc:"each locker's meals vary by up to this many either side of crash-pod-meals"`
	// Every colonist lands with exactly one rare item: a gun, a chicken (with
	// a trough), or a cat, picked per colonist by these relative weights. A
	// gun is a shotgun crash-pod-shotgun-percent of the time, else a pistol.
	// All three weights at 0 lands everyone with none. See
	// docs/ships.md and docs/chickens.md.
	CrashPodGunWeight      int `cfg:"crash-pod-gun-weight" doc:"relative odds a colonist's one rare item is a gun"`
	CrashPodChickenWeight  int `cfg:"crash-pod-chicken-weight" doc:"relative odds a colonist's one rare item is a chicken (with a trough in its ship's hold)"`
	CrashPodCatWeight      int `cfg:"crash-pod-cat-weight" doc:"relative odds a colonist's one rare item is a cat"`
	CrashPodShotgunPercent int `cfg:"crash-pod-shotgun-percent" doc:"percent of the guns colonists land with that are shotguns rather than pistols"`

	// Recruiting. The player pays an off-world recruiter from the treasury
	// for a set of candidates, then pays each one's passage to hire it; both
	// leave the colony's money supply. A recruit arrives with its savings
	// (minted into its wallet, like a purse) and recruit-meals meals in its
	// pockets. Savings are log-normal: recruit-savings-spread is the
	// half-width of the band about 95% of candidates fall in around the mean
	// while it is well under the mean, and a wider spread leaves most
	// candidates poor and a rare few rich. See recruit.go and
	// docs/recruiting.md.
	RecruiterFee         int64 `cfg:"recruiter-fee" sec:"Recruiting" doc:"dollars the treasury pays the recruiter for each set of candidates"`
	RecruitCandidates    int   `cfg:"recruit-candidates" doc:"candidates in each set the recruiter presents (0 disables recruiting)"`
	RecruitCost          int64 `cfg:"recruit-cost" doc:"dollars the treasury pays for each candidate hired: passage and starting supplies"`
	RecruitMeals         int   `cfg:"recruit-meals" doc:"meals each recruit arrives with, in its pockets"`
	RecruitSavingsMean   int64 `cfg:"recruit-savings-mean" doc:"average dollars a candidate brings with it, before clamping to the min and max"`
	RecruitSavingsSpread int64 `cfg:"recruit-savings-spread" doc:"about 95% of candidates' savings fall within this many dollars of the mean, while it is well under the mean; wider gives a long tail of rare rich candidates (0: everyone brings the mean)"`
	RecruitSavingsMin    int64 `cfg:"recruit-savings-min" doc:"fewest dollars a candidate brings (never below 0: no candidate arrives in debt)"`
	RecruitSavingsMax    int64 `cfg:"recruit-savings-max" doc:"most dollars a candidate brings"`

	// Timing.
	TicksPerSecond int `cfg:"tps" sec:"Timing" doc:"simulation ticks per second"`
	// StartPaused starts the engine paused. The browser sets it, so the
	// player can move the ships before the first tick (see MoveShip).
	StartPaused bool `cfg:"start-paused" doc:"start the game paused (the browser does, so the ships can be placed)"`
	LogSize     int  `cfg:"log-size" doc:"number of recent events retained"`

	// Colonist stats.
	ColonistHP int `cfg:"colonist-hp" sec:"Colonists" doc:"colonist hit points"`
	MineTicks  int `cfg:"mine-ticks" doc:"ticks of work to excavate one rock tile"`
	BuildTicks int `cfg:"build-ticks" doc:"ticks of work to raise one wall"`
	// DemolishTicks is how long breaking a wall or hull tile down takes, for
	// a colonist escaping a sealed room or a builder opening a passage (see
	// FocusEscape, planPassage, docs/escape.md). Costlier than raising one
	// (BuildTicks): breaking out should be a last resort, not a cheaper
	// substitute for a door once those exist. Against MineTicks it also
	// decides whether a way out goes round a structure through the rock or
	// through its wall.
	DemolishTicks      int `cfg:"demolish-ticks" doc:"ticks of work to break down one wall or hull tile, escaping a sealed room or opening a passage"`
	FacilityBuildTicks int `cfg:"facility-ticks" doc:"ticks of work to build a pod or toilet"`
	FleeRadius         int `cfg:"flee-radius" doc:"colonist flees when an alien is within this many tiles"`
	// FleeReleaseMargin is flee's hysteresis band: a colonist already fleeing
	// keeps fleeing until no alien is within FleeRadius+FleeReleaseMargin.
	// Without it, one step out of FleeRadius ends the flee, the next focus
	// (often a toilet or job past the same alien) walks straight back in, and
	// the colonist flickers flee/relieve every tick. Only flee is held; fight
	// still needs an alien inside FleeRadius. See docs/entities-and-ai.md.
	FleeReleaseMargin int `cfg:"flee-release-margin" doc:"a fleeing colonist keeps fleeing until no alien is within flee-radius plus this many tiles"`
	// StompRadius is how far an idle colonist notices a rat and gives chase to
	// crush it. Stomping is an idle whim: only colonists with nothing pressing
	// (no threat, no urgent drive, no work) hunt pests.
	ColonistStompRadius int `cfg:"stomp-radius" doc:"an idle colonist chases and crushes a rat within this many tiles"`
	// GoreSightRadius is how far a colonist notices gore on the ground (see
	// the visible-gore perception rule in cognition.yaml). Smaller than
	// the creature-sighting radii: a bloodstain doesn't announce itself the way
	// a moving alien does.
	GoreSightRadius int `cfg:"gore-sight-radius" doc:"a colonist notices gore on the ground within this many tiles"`

	// Sanitation. A colonist with no urgent drive cleans up refuse — gore and
	// corpses — and hauls it to an incinerator to burn. CleanRadius is how far
	// it looks for a mess (larger than GoreSightRadius, which is about noticing
	// one, not going to find it); a Tidy colonist searches twice as far. See
	// docs/sanitation.md.
	CleanRadius           int `cfg:"clean-radius" sec:"Sanitation" doc:"how far a colonist looks for refuse to clean up (a Tidy colonist looks twice as far)"`
	CleanTicks            int `cfg:"clean-ticks" doc:"ticks of work to scrub one tile of refuse clean"`
	IncinerateTicks       int `cfg:"incinerate-ticks" doc:"ticks spent feeding a load of refuse into an incinerator"`
	IncineratorBuildTicks int `cfg:"incinerator-ticks" doc:"ticks of work to build an incinerator"`

	// Drives. One DriveSpec per DriveKind, indexed by that kind.
	Drives       [numDrives]DriveSpec `cfg:"drives" sec:"Drives"`
	StarveDamage int                  `cfg:"starve-damage" doc:"HP lost per tick while a drive's death consequence applies"`
	// DriveEffects are the effect profiles events can start on a colonist's
	// drives (caffeine, water), indexed by DriveEffectKind. None ship yet;
	// the config file has no lists, so they are set in code (drive_effects.go).
	DriveEffects         []DriveEffectProfile
	ColonistsPerFacility int `cfg:"per-facility" doc:"colonists served by each life-support facility"`

	// Focus arbitration. Runtime copy of cognition.yaml's focuses/arbitration,
	// written once at load by SyncWithCognition. Tune those tables there, not
	// via these keys: a later SyncWithCognition overwrites them.
	Focuses             [numFocusKinds]FocusSpec `cfg:"focuses" sec:"Focus arbitration"`
	FocusCurrentBonus   int                      `cfg:"focus-current-bonus" doc:"score bonus for continuing the current eligible focus"`
	FocusSwitchMargin   int                      `cfg:"focus-switch-margin" doc:"minimum score lead required to replace an eligible focus"`
	FocusCriticalBonus  int                      `cfg:"focus-critical-bonus" doc:"score bonus for a need at its critical boundary"`
	FocusFatalBonus     int                      `cfg:"focus-fatal-bonus" doc:"score bonus for a pressing fatal need"`
	ActiveStimulusLimit int                      `cfg:"active-stimulus-limit" doc:"maximum transient life-event appraisals retained per colonist"`

	// Cognition is the authored cognition configuration. Focuses and
	// arbitration above are a runtime copy of its tables.
	Cognition CognitionConfig

	RestTicks  int `cfg:"rest-ticks" sec:"Work and construction" doc:"ticks an idle colonist rests before re-checking for work"`
	StuckLimit int `cfg:"stuck-limit" doc:"ticks a colonist waits on a blocked path before abandoning the job"`

	// MaxConcurrentProjects is the ceiling on how many rooms can be under
	// construction at once (min 1 is enforced); the actual cap also scales
	// down for a small colony (see maxConcurrentProjects in project.go) so an
	// early cramped cavern still builds one room at a time. Raising it lets a
	// larger colony's facility supply keep pace with growth; see
	// construction.md.
	MaxConcurrentProjects int `cfg:"max-concurrent-projects" doc:"rooms that can be under construction at once"`
	// RoomExpansion has the colony put fixtures into the rooms it already
	// has before it marks out a new one: into free floor in a room of their
	// zone (a fit-out), or by growing a room through any of its walls.
	// RoomMaxFacilities is the most fixtures, of any kind, a room holds.
	// RoomMerge has it join two rooms of one zone that stand side by side,
	// back to back or facing each other, tearing down the walls between
	// them: first when it wants more fixtures, and otherwise when it has
	// nothing else to build. See docs/room-expansion.md.
	RoomExpansion     bool `cfg:"room-expansion" doc:"put fixtures into the colony's rooms of their zone, fitting out free floor or growing a room through any wall, before building a new one"`
	RoomMaxFacilities int  `cfg:"room-max-facilities" doc:"most fixtures (of any kind: bunks, chests, stoves, incubators, chairs) one room holds"`
	RoomMerge         bool `cfg:"room-merge" doc:"join two rooms of the same zone standing side by side, back to back or facing each other into one, tearing down the walls between them: when more fixtures are wanted, or when there is nothing else to build"`

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

	// Conversation topics. Whoever raises the topic picks what kind of thing
	// to talk about by these weights, among the kinds it has something to
	// say about: one of its own memories, another colonist it has feelings
	// about, or a piece of lore (an alien species or a corporation). Talking about a
	// colonist is gossip: the listener's affinity toward the subject moves
	// TalkGossipPercent of the way toward the speaker's, when the chat went
	// well. See topics.go and docs/conversation-topics.md.
	TalkTopicMemoryWeight   int `cfg:"talk-topic-memory-weight" doc:"relative weight of talking about one of the speaker's memories (0 never)"`
	TalkTopicColonistWeight int `cfg:"talk-topic-colonist-weight" doc:"relative weight of talking about another colonist (0 never)"`
	TalkTopicLoreWeight     int `cfg:"talk-topic-lore-weight" doc:"relative weight of talking about lore, such as an alien species or a corporation (0 never)"`
	TalkGossipPercent       int `cfg:"talk-gossip-percent" doc:"percent of the gap a good chat about a colonist closes between the listener's affinity toward them and the speaker's"`

	// The meeting hall: a room of chairs the colony commissions, where
	// colonists go to socialize and to eat. See docs/meeting-hall.md.
	ColonistsPerChair int `cfg:"colonists-per-chair" sec:"Meeting hall" doc:"the colony commissions meeting-hall chairs, one for each this many colonists (0: no hall, and talk and meals stay wherever they happen)"`
	HallRange         int `cfg:"hall-range" doc:"farthest a colonist walks to the meeting hall to socialize or eat, in tiles"`
	HallTalkBonus     int `cfg:"hall-talk-bonus" doc:"conversation quality points added when both partners are in the meeting hall"`

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
	// of each. See docs/affect.md.
	MoodPushImpact int `cfg:"mood-push-impact" doc:"event impact at or below which affect is only nudged"`
	MoodPullImpact int `cfg:"mood-pull-impact" doc:"event impact at or above which affect is relocated outright"`

	// How fast a colonist stops being new to something. Each remembered
	// occasion of a kind moves its appraisal this much further from its fresh
	// reading toward its worn one, capped at fully worn. Occasions leave with
	// the memories that hold them, so this also sets how fast wear recovers.
	MoodWearPerOccasion int `cfg:"mood-wear-per-occasion" doc:"percent of the way toward an event's worn reading per remembered occasion"`
	MoodFriendAffinity  int `cfg:"mood-friend-affinity" doc:"affinity at or above which an occurrence object is perceived as a friend"`

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
	// AlienHungerRate is the food drive an alien gains per tick, in thousandths
	// of a point. Only Friendly
	// and Cautious species act on it: once it passes the food drive's seek-at
	// they graze exposed cave scum within AlienGrazeRadius (Hostile ones eat
	// colonists instead). Aliens never starve. See alienGraze.
	AlienHungerRate  int `cfg:"alien-hunger-rate" doc:"food drive a Friendly or Cautious alien gains per tick, in thousandths of a point, before it goes grazing on cave scum"`
	AlienGrazeRadius int `cfg:"alien-graze-radius" doc:"how far a hungry Friendly or Cautious alien looks for cave scum to eat"`
	// Alien lifecycles (see docs/alien-lifecycles.md): the relative odds a
	// rolled species lives as one, two, three or four forms, how often a
	// multi-stage life ends in castes, and roughly how long a stage lasts
	// (each species' stages vary by a quarter either side).
	AlienOneFormWeight   int `cfg:"alien-one-form-weight" doc:"relative odds a rolled alien species has a single form (most do)"`
	AlienTwoFormWeight   int `cfg:"alien-two-form-weight" doc:"relative odds a rolled alien species has two life stages"`
	AlienThreeFormWeight int `cfg:"alien-three-form-weight" doc:"relative odds a rolled alien species has three life stages"`
	AlienFourFormWeight  int `cfg:"alien-four-form-weight" doc:"relative odds a rolled alien species has four life stages"`
	AlienCastePercent    int `cfg:"alien-caste-percent" doc:"percent of multi-stage alien species whose adults split into castes (queen/worker/drone, bull/betty)"`
	AlienStageTicks      int `cfg:"alien-stage-ticks" doc:"roughly how many ticks one alien life stage lasts before it grows into the next"`

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
	RifleDamage     int `cfg:"rifle-damage" doc:"HP removed per assault rifle burst"`
	RifleRange      int `cfg:"rifle-range" doc:"max tiles an assault rifle can fire from"`
	RifleFireRest   int `cfg:"rifle-fire-rest" doc:"cooldown ticks between assault rifle bursts"`
	// CorporationCount is how many companies this seed's lore rolls; every gun
	// kind gets a make and model from one of them. Flavor only. See
	// arms_makers.go and docs/arms-makers.md.
	CorporationCount int `cfg:"corporation-count" doc:"companies this seed's lore rolls; each gun kind is made by one of them"`
	// CorporationEmployeePercent is the chance an arriving colonist used to
	// work for one of them: backstory flavor only.
	CorporationEmployeePercent int `cfg:"corporation-employee-percent" doc:"percent of arriving colonists who used to work for one of the lore's corporations (flavor only)"`

	// Cat stats. Cats have no drives; they hunt rats on the floor by instinct.
	CatHP         int `cfg:"cat-hp" sec:"Cats" doc:"cat hit points"`
	CatSlowness   int `cfg:"cat-slowness" doc:"cat acts once every N ticks (higher = slower)"`
	CatPounceRest int `cfg:"cat-pounce-rest" doc:"cooldown ticks after a cat catches a rat"`

	// Chicken stats. A chicken has only the food drive: it eats feed from its
	// keeper's trough, or grazes cave scum, and starves with neither. See
	// chickens.go and docs/chickens.md.
	ChickenHP          int `cfg:"chicken-hp" sec:"Chickens" doc:"chicken hit points"`
	ChickenSlowness    int `cfg:"chicken-slowness" doc:"chicken acts once every N ticks (higher = slower)"`
	ChickenHungerRate  int `cfg:"chicken-hunger-rate" doc:"food drive a chicken gains per tick, in thousandths of a point"`
	ChickenGrazeRadius int `cfg:"chicken-graze-radius" doc:"how far a hungry chicken looks for cave scum to graze"`
	ChickenRoam        int `cfg:"chicken-roam" doc:"a chicken with a trough wanders back toward it once farther than this many tiles"`
	// A keeper refills its trough once it holds fewer than TroughLow units
	// of feed, mixing enough to bring it to TroughFill.
	TroughLow  int `cfg:"trough-low" doc:"a keeper refills its trough once it holds fewer than this many units of feed (0: keepers never tend)"`
	TroughFill int `cfg:"trough-fill" doc:"units of feed a keeper fills its trough to"`

	// Rat stats. Rats share the colonists' DriveFood but grow hungry far faster
	// (they nibble constantly), and flee cats rather than aliens.
	RatHP         int `cfg:"rat-hp" sec:"Rats" doc:"rat hit points"`
	RatHungerRate int `cfg:"rat-hunger-rate" doc:"food drive a rat gains per tick, in thousandths of a point (rats eat frequently)"`
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
	cfg := Config{
		Width:                80,
		Height:               40,
		WorldgenHalo:         2,
		IronRockPercent:      10,
		IceRockPercent:       5,
		UraniumRockPercent:   1,
		ClayRockPercent:      5,
		RockVeinMin:          8,
		RockVeinMax:          24,
		SaltPercent:          3,
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
		StartCats:            0, // the colony's cats come down in its crash pods
		StartRats:            8,
		// Placeholders until the market gives money a use: a treasury worth a
		// few dozen purses, so the colony can outspend any one settler.
		FoundingGrant: 5000,
		// The floor sits below house-savings: at 300 the levy barely touched
		// a colony whose wallets averaged about that, and its treasury still
		// ran near empty; saving for a house just takes a little longer.
		WealthTax: 2, TaxFloor: 200, TaxInterval: 100,
		// Scarcity is on (economy phase E8): food is made, not conjured, and
		// building costs materials. The safety net stays a setting, for tests
		// and for balancing. See docs/economy.md.
		InfiniteFood:  false,
		CrashPodPurse: 100,
		// Scum is tuned so a small colony that keeps digging can feed itself
		// once its crash-pod meals run out: see the sustain test in
		// scumhouse_test.go before changing these.
		ScumPercent:       6,
		ScumMax:           3,
		ScumSpawnPPM:      20,
		ScumSpreadPercent: 40,
		ScrapeTicks:       6,

		IncubatorGrowTicks:    15,
		IncubatorCapacity:     12,
		IncubatorSeed:         2,
		ColonistsPerIncubator: 4,
		ScumDireMeals:         1,
		WageHarvest:           1,
		MealReserve:           3,
		Rations:               true,
		ConstructionCosts:     true,
		// The charter's prices: a meal a few hours' pay, uranium dearest
		// because it costs the miner a dose, raw rock not bought at all — there
		// is always more, and buying it would drain the treasury on nothing.
		PriceMeal:        5,
		PriceRawRock:     0,
		PriceIronOre:     3,
		PriceWaterIce:    2,
		PriceUraniumOre:  6,
		PriceClay:        2,
		SiloBidQty:       64,
		OrderTTL:         2000,
		MealKeep:         5,
		MealWillingness:  3,
		AskDecayTicks:    150,
		AskDecayPercent:  10,
		RelistIdle:       true,
		BidStartPercent:  80,
		BidRaiseTicks:    50,
		BidRaisePercent:  20,
		MealSellAtMarket: true,
		FreePricesAt:     1,
		MealPriceMax:     100,
		PocketMealAt:     300,
		// Wages sized so a typical room costs the colony about a hundred
		// dollars: fifty rooms from the founding grant, less what it spends
		// buying ore. A house is a real purchase, several weeks of prospecting.
		// A colonist's time is worth a couple of dollars per 100 ticks: less
		// than a scraper earns selling scum into a meal-maker's bid, so the
		// chain from a hungry colonist's bid down to the cave wall pays at
		// every link. See docs/valuation.md.
		// The colony sells at half again what it pays: enough over cost that
		// its resales refill the treasury, not so much that a colonist would
		// rather dig the ore itself every time. See docs/hauling.md.
		ColonySells:             true,
		StandingOrdersBuildOnly: true,
		ColonyMarkup:            50,
		ColonyStockReserve:      8,
		SiloMealStock:           6,
		HaulPay:                 1,
		Skills:                  true,
		SkillPracticePercent:    100,
		LaborPrice:              2,
		PlanMinProfit:           1,
		PlanCandidates:          4,
		PlanTTL:                 1500,
		RateMemory:              4000,
		DemandTTL:               300,
		// Priced by the meals they make (see recipes): two scum or two
		// viscera to a $5 meal, so the colony roughly breaks even after the
		// cook's wage; an alien carcass makes four.
		PriceCaveScum:     2,
		PriceViscera:      2,
		PriceAnimalCorpse: 3,
		PriceAlienCorpse:  8,
		ScumhouseBidQty:   12,
		// A cook turns 2 scum into a meal every dozen ticks or so, and a
		// colonist eats about one meal every few hundred: one cook feeds
		// roughly a dozen and a half people with nobody to spare, so a
		// scumhouse per ten keeps up with slack. Forty units is twenty meals
		// of stock: enough to cook from, not a hoard.
		ScumhouseStockCap:     40,
		ColonistsPerScumhouse: 10,
		// A rifle is three steel ingots and a steel ingot two iron ore, so a
		// rifle is six ore — $24 at the colony's resale price — plus three
		// links of work and carrying. $60 leaves each link a margin worth the
		// walk (see docs/foundry.md for the arithmetic).
		ArmoryRifles:      4,
		PriceAssaultRifle: 60,
		WageDig:           2,
		WageWall:          2,
		WageFixture:       5,
		WageDemolish:      3,
		WageCook:          1,
		HouseSavings:      300,
		KitchenRank:       3,
		KitchenSavings:    100,
		ToiletFee:         2,
		// A meal clears hunger for roughly 325 ticks at the baseline rise, so
		// ten carry a colonist a few thousand ticks: long enough to settle in,
		// short enough that food production matters once the safety net is
		// off. Every settler lands armed, the way frontier settlers did.
		// A ship of 20 sleeps 10 and has 5 toilets: enough to get by, not
		// enough to keep the colony from building. See docs/ships.md.
		ShipCapacity:      20,
		ShipBunkPercent:   50,
		ShipToiletPercent: 25,
		// Every shape equally likely: the colony's landing site looks
		// different from game to game.
		ShipStickWeight:    1,
		ShipHubWeight:      1,
		ShipClusterWeight:  1,
		CrashPodMeals:      10,
		CrashPodMealSpread: 0,
		// Half the colony lands armed, a quarter with a chicken, a quarter
		// with a cat; a quarter of the guns are shotguns.
		CrashPodGunWeight:      50,
		CrashPodChickenWeight:  25,
		CrashPodCatWeight:      25,
		CrashPodShotgunPercent: 25,

		RecruiterFee:         500,
		RecruitCandidates:    5,
		RecruitCost:          100,
		RecruitMeals:         3,
		RecruitSavingsMean:   100,
		RecruitSavingsSpread: 50,
		RecruitSavingsMin:    0,
		RecruitSavingsMax:    10000,
		GraveyardSize:        50,
		TicksPerSecond:       8,
		LogSize:              64,
		ColonistHP:           40,
		MineTicks:            6,
		BuildTicks:           8,
		DemolishTicks:        16,
		FacilityBuildTicks:   12,
		FleeRadius:           5,
		FleeReleaseMargin:    3,
		ColonistStompRadius:  4,
		GoreSightRadius:      3,

		CleanRadius:           10,
		CleanTicks:            6,
		IncinerateTicks:       8,
		IncineratorBuildTicks: 20,

		StarveDamage:          1,
		ColonistsPerFacility:  5,
		Cognition:             DefaultCognitionConfig(),
		RestTicks:             10,
		StuckLimit:            8,
		MaxConcurrentProjects: 2,
		RoomExpansion:         true,
		RoomMaxFacilities:     8,
		RoomMerge:             true,
		EscapeGraceTicks:      32,
		TraitChance:           defaultTraitChance,
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

		ColonistsPerChair: 4,
		HallRange:         48,
		HallTalkBonus:     15,

		TalkQualityBias:    20,
		TalkQualityValence: 50,
		TalkQualitySpread:  50,

		TalkTopicMemoryWeight:   3,
		TalkTopicColonistWeight: 3,
		TalkTopicLoreWeight:     2,
		TalkGossipPercent:       10,

		MoodMax:                   defaultMoodMax,
		ConversationCompanyWeight: 6,
		ConversationQualityWeight: 10,
		SocialWindowTicks:         200,
		MoodChargeDecayPerTick:    2,
		MoodGripDecayPerTick:      1,
		MoodValenceDecayTicks:     12,
		MoodLabelSwitchMargin:     defaultMoodLabelSwitchMargin,

		MoodPushImpact:      30,
		MoodPullImpact:      70,
		MoodWearPerOccasion: 14,
		MoodFriendAffinity:  30,

		FrontierFieldMinColonists: 800,
		FrontierFieldMinArea:      90000, // ~300x300 and up
		Drives:                    defaultDrives(),
		AlienSpeciesCount:         1,
		AlienHP:                   30,
		AlienDamage:               6,
		AlienBiteRest:             3,
		AlienSlowness:             2,
		AlienReferenceWeightKG:    80,
		AlienCautiousRadius:       3,
		AlienHungerRate:           2000, // two points a tick: a grazer eats about as often as a colonist
		AlienGrazeRadius:          12,
		AlienOneFormWeight:        60,
		AlienTwoFormWeight:        25,
		AlienThreeFormWeight:      12,
		AlienFourFormWeight:       3,
		AlienCastePercent:         30,
		AlienStageTicks:           2160, // two colony days

		PistolDamage:    10,
		PistolRange:     3,
		PistolFireRest:  1,
		ShotgunDamage:   20,
		ShotgunRange:    2,
		ShotgunFireRest: 2,
		RifleDamage:     15,
		RifleRange:      5,
		RifleFireRest:   1,

		CorporationCount:           4,
		CorporationEmployeePercent: 60,

		CatHP:         12,
		CatSlowness:   2,
		CatPounceRest: 4,

		ChickenHP:          4,
		ChickenSlowness:    3,
		ChickenHungerRate:  2000, // two points a tick, about a colonist's
		ChickenGrazeRadius: 10,
		ChickenRoam:        6,
		TroughLow:          4,
		TroughFill:         12,

		RatHP:         4,
		RatHungerRate: 8000, // eight points a tick: rats eat very frequently
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
	cfg.SyncWithCognition()
	return cfg
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

// SyncWithCognition copies focuses and arbitration from the authored cognition
// tables into the runtime Config fields focus.go reads.
func (c *Config) SyncWithCognition() {
	c.Focuses = c.Cognition.Focuses
	c.FocusCurrentBonus = c.Cognition.Arbitration.CurrentBonus
	c.FocusSwitchMargin = c.Cognition.Arbitration.SwitchMargin
	c.FocusCriticalBonus = c.Cognition.Arbitration.CriticalBonus
	c.FocusFatalBonus = c.Cognition.Arbitration.FatalBonus
	c.ActiveStimulusLimit = c.Cognition.Arbitration.ActiveStimulusLimit
}
