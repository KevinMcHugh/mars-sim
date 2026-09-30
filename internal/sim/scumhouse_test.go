package sim

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"testing"
)

// scumhouseWorld is propertyWorld (open floor, nobody in it) with a scumhouse
// at (20, 6) and, if burn, an incinerator at (20, 14).
func scumhouseWorld(t *testing.T, burn bool) (w *World, house Point) {
	t.Helper()
	w = propertyWorld(t)
	house = Point{20, 6}
	w.SetTerrain(house, Scumhouse)
	if burn {
		w.SetTerrain(Point{20, 14}, Incinerator)
	}
	w.refreshSpatial()
	return w, house
}

// Every death that leaves a body leaves one of its own kind.
func TestDeathsLeaveBodiesOfTheirKind(t *testing.T) {
	w := propertyWorld(t)
	c := w.spawn(Colonist, Point{8, 8})
	m := w.spawn(Rat, Point{9, 8})
	w.stomp(c, m)
	if w.corpsesOfAt(Point{9, 8}, AnimalCorpse) != 1 {
		t.Fatalf("a stomped rat left %d animal carcasses", w.corpsesOfAt(Point{9, 8}, AnimalCorpse))
	}

	a := w.spawn(Alien, Point{12, 8})
	for a.Alive() && w.entities[a.ID] != nil {
		w.shoot(c, a, Shotgun, weaponStats(Shotgun, w.cfg))
	}
	if w.corpsesOfAt(Point{12, 8}, AlienCorpse) != 1 {
		t.Fatal("a shot alien left no alien carcass")
	}

	starving := w.spawn(Colonist, Point{15, 12})
	starving.Needs[NeedFood] = w.cfg.Needs[NeedFood].Max
	starving.HP = 1
	for i := 0; i < 5 && w.entities[starving.ID] != nil; i++ {
		w.step()
	}
	if w.entities[starving.ID] != nil || w.corpsesOfAt(starving.Pos, ColonistCorpse) != 1 {
		t.Fatalf("a starved colonist left %d colonists' bodies", w.corpsesOfAt(starving.Pos, ColonistCorpse))
	}
}

// A cleaner takes biomatter to the scumhouse and sells it to the colony, and
// takes a colonist's body to the incinerator. Nothing a scumhouse could use is
// burned.
func TestCleanersFeedTheScumhouseAndBurnOnlyTheDead(t *testing.T) {
	w, house := scumhouseWorld(t, true)
	w.tick = marketInterval
	w.runMarket() // the colony's standing bids for biomatter
	mess := Point{12, 10}
	w.addCorpse(mess, AlienCorpse)
	w.addCorpse(mess, ColonistCorpse)
	w.addGore(mess)
	cleaner := w.spawn(Colonist, Point{12, 11})
	purse := cleaner.wallet

	if !w.tryAssignClean(cleaner) {
		t.Fatal("no cleaning job")
	}
	for i := 0; i < 200 && cleaner.Job == JobClean; i++ {
		w.jobClean(cleaner)
	}
	c := w.storageContainers[house]
	if c.held(Community, AlienCorpse) != 1 || c.held(Community, Viscera) != 1 {
		t.Fatalf("scumhouse ledger = %+v, want the colony's alien carcass and viscera", c.Ledger)
	}
	if want := purse + w.biomatterPrice(AlienCorpse) + w.biomatterPrice(Viscera); cleaner.wallet != want {
		t.Fatalf("the cleaner has %v, want %v after selling what it cleaned up", cleaner.wallet, want)
	}
	if c.Inventory.Count(ColonistCorpse) != 0 {
		t.Fatal("a colonist's body went into the scumhouse")
	}
	if !loggedContaining(w, "incinerates a body") {
		t.Fatal("the colonist's body was not burned")
	}
	if w.refuseAt(mess) != 0 || carryingRefuse(cleaner) {
		t.Fatalf("refuse left: %d on the tile, carrying %v", w.refuseAt(mess), cleaner.Inventory)
	}
	if !c.ledgerBalanced() {
		t.Fatalf("scumhouse ledger does not match its contents: %+v", c.Ledger)
	}
}

// With a scumhouse but no incinerator, the cleaner still gathers biomatter
// and leaves colonists' bodies where they lie: nothing else takes those.
func TestWithoutAnIncineratorColonistsBodiesStay(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	w.tick = marketInterval
	w.runMarket()
	w.addCorpse(Point{10, 10}, ColonistCorpse)
	w.addCorpse(Point{14, 10}, AnimalCorpse)
	cleaner := w.spawn(Colonist, Point{12, 12})

	for round := 0; round < 3; round++ {
		if !w.tryAssignClean(cleaner) {
			break
		}
		for i := 0; i < 200 && cleaner.Job == JobClean; i++ {
			w.jobClean(cleaner)
		}
	}
	if w.storageContainers[house].held(Community, AnimalCorpse) != 1 {
		t.Fatal("the animal carcass never reached the scumhouse")
	}
	if w.corpsesOfAt(Point{10, 10}, ColonistCorpse) != 1 || cleaner.Inventory.Has(ColonistCorpse) {
		t.Fatal("a colonist's body was picked up with nowhere to take it")
	}
}

// A cook turns the colony's biomatter into the colony's meals, for a wage.
// The meals are not free: the scumhouse sells them, and a colonist buys one.
func TestCookingTurnsTheColonysScumIntoItsMeals(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	c := w.storageContainers[house]
	c.Inventory.Add(CaveScum, 2)
	c.credit(Community, CaveScum, 2)
	cook := w.spawn(Colonist, Point{12, 10})
	wage := cook.wallet

	if !w.tryAssignCraft(cook) {
		t.Fatal("no cooking job")
	}
	other := w.spawn(Colonist, Point{14, 10})
	if w.tryAssignCraft(other) {
		t.Fatal("a second cook claimed the same scumhouse")
	}
	for i := 0; i < 200 && cook.Job == JobCraft; i++ {
		w.jobCraft(cook)
	}
	// The meal goes straight on sale (offerColonyMeals): the colony's own,
	// held in the ask's escrow until someone buys it.
	if c.held(Community, CaveScum) != 0 || w.openQty(Ask, Meal, house, Community) != 1 || !c.ledgerBalanced() {
		t.Fatalf("scumhouse after cooking: %+v", c.Ledger)
	}
	if cook.wallet != wage+Money(w.cfg.WageCook) {
		t.Fatalf("the cook was paid %v, want %v", cook.wallet-wage, w.cfg.WageCook)
	}
	if _, ok := w.nearestMealDepot(other); ok {
		t.Fatal("the colony's meal is free for the taking")
	}
	w.tick = marketInterval
	w.runMarket()
	if ask, ok := w.bestAsk(Meal, house); !ok || ask.Actor != Community || ask.Price != w.refPrice(Meal) {
		t.Fatalf("the scumhouse is not selling its meal: best ask %+v", ask)
	}
	other.Needs[NeedFood] = w.cfg.Needs[NeedFood].Max
	purse := other.wallet
	if !w.tryBuyMeal(other) || other.wallet != purse-w.refPrice(Meal) {
		t.Fatalf("a hungry colonist could not buy the meal (paid %v)", purse-other.wallet)
	}
	if got, ok := w.nearestMealDepot(other); !ok || got != house {
		t.Fatal("the meal it bought is not there for it to eat")
	}
	assertMoneyConserved(t, w)
}

// A patch scraped bare is gone, and a wall built on one destroys it.
func TestScumScrapedBareIsGone(t *testing.T) {
	w := propertyWorld(t)
	p := Point{10, 10}
	w.setScum(p, w.cfg.ScumMax)
	for i := 0; i < w.cfg.ScumMax; i++ {
		if !w.takeScum(p) {
			t.Fatalf("patch ran out after %d units", i)
		}
	}
	if w.takeScum(p) {
		t.Fatal("scraped an empty patch")
	}
	if _, ok := w.scum[p]; ok {
		t.Fatal("a bare patch is still on the map")
	}
	w.setScum(p, 1)
	w.SetTerrain(p, Wall)
	if w.scumAt(p) != 0 {
		t.Fatal("scum survived a wall being built on it")
	}
}

// Scum accretes: with no patches anywhere it still spawns at a low rate, and
// a lone patch spreads to its neighbours far more often than scum appears
// elsewhere.
func TestScumSpawnsAndSpreads(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 60, 36
	cfg.ScumPercent = 30
	cfg.ScumSpawnPPM = 50
	cfg.ScumSpreadPercent = 50
	w := newTestWorld(t, cfg)
	noScum(w)
	// A patch in solid rock, so its neighbours are eligible to grow.
	var seed Point
	for y := 4; y < cfg.Height-4 && seed == (Point{}); y++ {
		for x := 4; x < cfg.Width-4; x++ {
			solid := true
			for dy := -3; dy <= 3; dy++ {
				for dx := -3; dx <= 3; dx++ {
					solid = solid && w.TerrainAt(Point{x + dx, y + dy}) == Rock
				}
			}
			if solid {
				seed = Point{x, y}
				break
			}
		}
	}
	if seed == (Point{}) {
		t.Fatal("no solid rock to seed a patch in")
	}
	w.setScum(seed, w.cfg.ScumMax)
	near, far := 0, 0
	for i := 0; i < 600; i++ {
		w.growScum()
		w.tick++
	}
	for p := range w.scum {
		if p == seed {
			continue
		}
		if seed.Chebyshev(p) <= 3 {
			near++
		} else {
			far++
		}
	}
	// 49 tiles within 3 of the seed against 2,111 elsewhere.
	if near == 0 || far == 0 {
		t.Fatalf("near=%d far=%d: want both spread from the patch and spawns elsewhere", near, far)
	}
	if near*2111 < far*49*3 {
		t.Fatalf("near=%d far=%d: scum is no likelier beside scum", near, far)
	}
	cfg.ScumSpawnPPM, cfg.ScumSpreadPercent = 0, 0
	w2 := newTestWorld(t, cfg)
	before := len(w2.scum)
	for i := 0; i < 400; i++ {
		w2.growScum()
		w2.tick++
	}
	if len(w2.scum) != before {
		t.Fatal("scum grew with both rates at zero")
	}
}

// Scum grows at the rate its settings say on the ground that's generated,
// whatever the map's size. It used to sample the whole map and scale its
// chances to make up for the ungenerated chunks it skipped, which stopped
// working once a chance passed certainty: on a 10,000×10,000 map the colony
// had barely explored, scum came back about 600 times too slowly, and a
// six-colonist colony starved (see docs/scumhouse.md).
var scumRateMapSizes = []int{256, 1000, 10000}

func TestScumSpawnsAtItsRateOnAnyMap(t *testing.T) {
	const ticks = 200
	for _, size := range scumRateMapSizes {
		cfg := testConfig()
		cfg.Width, cfg.Height = size, size
		cfg.ScumPercent = 100 // room for every patch, so only the rate counts
		cfg.ScumSpawnPPM = 10000
		cfg.ScumSpreadPercent = 0
		w := newTestWorld(t, cfg)
		noScum(w)
		rock := 0
		for _, k := range w.genChunks {
			for off := 0; off < genChunkArea; off++ {
				p := Point{int(k.cx)<<genChunkBits + off%genChunkSize, int(k.cy)<<genChunkBits + off/genChunkSize}
				if w.InBounds(p) && w.TerrainAt(p) == Rock {
					rock++
				}
			}
		}
		for i := 0; i < ticks; i++ {
			w.growScum()
			w.tick++
		}
		// Each rock tile rolls ticks/16 visits at 1%; a second spawn on a
		// tile thickens its patch rather than making another.
		want := float64(rock) * (1 - math.Exp(-ticks/float64(scumTrialDivisor)*0.01))
		if got := float64(len(w.scum)); math.Abs(got-want) > want*0.05 {
			t.Errorf("%dx%d map, %d chunks: %.0f patches spawned in %d ticks, want about %.0f",
				size, size, len(w.genChunks), got, ticks, want)
		}
		assertScumPatchesListed(t, w)
	}
}

// Spread is the same on any map too, and growScum draws it from the patches
// rather than visiting tiles: each patch sends a unit to one of the nine
// tiles in and around it at ScumSpreadPercent/16 a tick. With no room for new
// patches and no cap on a patch, every draw that lands on a patch thickens it,
// so the units added are exactly countable.
func TestScumSpreadsAtItsRateOnAnyMap(t *testing.T) {
	const ticks = 100
	for _, size := range scumRateMapSizes {
		cfg := testConfig()
		cfg.Width, cfg.Height = size, size
		w := newTestWorld(t, cfg)
		w.cfg.ScumPercent = 0 // no room: spread only thickens
		w.cfg.ScumMax = 1 << 30
		w.cfg.ScumSpawnPPM, w.cfg.ScumSpreadPercent = 0, 40
		before, landing := 0, 0
		for p, s := range w.scum {
			before += s.amount
			for _, d := range neighbors9 {
				if _, ok := w.scum[p.Add(d.X, d.Y)]; ok {
					landing++
				}
			}
		}
		for i := 0; i < ticks; i++ {
			w.growScum()
			w.tick++
		}
		after := 0
		for _, s := range w.scum {
			after += s.amount
		}
		want := float64(landing) / 9 * 0.40 / scumTrialDivisor * ticks
		if got := float64(after - before); math.Abs(got-want) > want*0.05 {
			t.Errorf("%dx%d map, %d patches: spread added %.0f units in %d ticks, want about %.0f",
				size, size, len(w.scum), got, ticks, want)
		}
	}
}

// assertScumPatchesListed checks that scumPatches lists exactly the patches
// on the map, in cmpScumPatch order: growScum draws from it.
func assertScumPatchesListed(t *testing.T, w *World) {
	t.Helper()
	if len(w.scumPatches) != len(w.scum) {
		t.Fatalf("scumPatches lists %d patches, the map holds %d", len(w.scumPatches), len(w.scum))
	}
	if !slices.IsSortedFunc(w.scumPatches, cmpScumPatch) {
		t.Fatal("scumPatches is out of order")
	}
	for _, p := range w.scumPatches {
		if _, ok := w.scum[p]; !ok {
			t.Fatalf("scumPatches lists %v, which has no patch", p)
		}
	}
}

// The exposed-scum index agrees with a brute-force check after a colony has
// dug and built for a while.
func TestExposedScumIndexStaysInStep(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 60, 36
	w := newTestWorld(t, cfg)
	if len(w.scum) == 0 {
		t.Fatal("worldgen seeded no scum")
	}
	for i := 0; i < 1500; i++ {
		w.step()
	}
	for p := range w.scum {
		_, indexed := w.exposedScum[p]
		if indexed != w.scumExposed(p) {
			t.Fatalf("patch %v: indexed %v, exposed %v", p, indexed, w.scumExposed(p))
		}
	}
	assertScumPatchesListed(t, w)
}

// A scraper brings a patch's scum in for the colony.
func TestScrapersBringScumInForTheColony(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	w.tick = marketInterval
	w.runMarket()
	patch := Point{8, 12}
	w.setScum(patch, w.cfg.ScumMax)
	w.refreshScumExposure(patch)
	s := w.spawn(Colonist, Point{12, 12})
	purse := s.wallet

	if !w.tryAssignScrape(s, false) {
		t.Fatal("no scraping job")
	}
	for i := 0; i < 400 && s.Job == JobScrape; i++ {
		w.jobScrape(s)
	}
	if got := w.storageContainers[house].held(Community, CaveScum); got != w.cfg.ScumMax {
		t.Fatalf("scumhouse holds %d of the colony's scum, want %d", got, w.cfg.ScumMax)
	}
	if s.Inventory.Has(CaveScum) || s.foreignCargo(CaveScum) != 0 {
		t.Fatal("the scraper kept scum, or its cargo record, after delivering")
	}
	if want := purse + Money(w.cfg.ScumMax)*Money(w.cfg.PriceCaveScum); s.wallet != want {
		t.Fatalf("the scraper has %v, want %v after selling its scum", s.wallet, want)
	}
}

// A colonist scraping to feed itself keeps what it scrapes, and can cook it.
func TestAHungryScraperKeepsItsScum(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	w.tick = marketInterval
	w.runMarket()
	patch := Point{8, 12}
	w.setScum(patch, w.cfg.ScumMax)
	w.refreshScumExposure(patch)
	s := w.spawn(Colonist, Point{12, 12})
	if !w.tryAssignFoodWork(s, true) {
		t.Fatal("no food work for a hungry colonist")
	}
	for i := 0; i < 400 && s.Job == JobScrape; i++ {
		w.jobScrape(s)
	}
	me := ColonistOwner(s.ID)
	if got := w.storageContainers[house].held(me, CaveScum); got != w.cfg.ScumMax {
		t.Fatalf("the scraper owns %d scum at the scumhouse, want %d", got, w.cfg.ScumMax)
	}
	if !w.tryAssignFoodWork(s, true) || s.Job != JobCraft || s.craftFor != me {
		t.Fatalf("a hungry colonist with its own scum did not cook it (job %v)", s.Job)
	}
}

// The gate for this phase: with the safety net off, a colony that lands with
// only three meals each — enough for about 1750 ticks — builds a scumhouse,
// scrapes scum, and feeds itself through 8000 ticks without anyone starving.
func TestColonyFeedsItselfWithoutTheSafetyNet(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 60, 36
	cfg.StartAliens = 0 // hunger is the test, not aliens
	cfg.InfiniteFood = false
	cfg.CrashPodMeals = 3
	w := newTestWorld(t, cfg)
	n := w.countKind(Colonist)
	for i := 0; i < 8000; i++ {
		w.step()
	}
	for _, d := range w.deceasedColonists {
		if d.Cause == "starved" {
			t.Fatalf("%s starved at tick %d", d.Profile.Name, d.DiedTick)
		}
	}
	if w.countKind(Colonist) != n {
		t.Fatalf("%d of %d colonists alive", w.countKind(Colonist), n)
	}
	if w.countTerrain(Scumhouse) == 0 || w.communityMeals() == 0 {
		t.Fatalf("scumhouses %d, colony meals %d: the colony is not making food",
			w.countTerrain(Scumhouse), w.communityMeals())
	}
}

// With construction costs on, a builder pays from its own stock: it will not
// take a task it cannot pay for, builds with what it carries, and fetches the
// rest from its own chest.
func TestConstructionCostsAreSpentFromTheBuildersStock(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.ConstructionCosts = true
	b := w.spawn(Colonist, Point{8, 8})
	wall := &buildTask{pos: Point{14, 8}, terrain: Wall, phase: 0}
	w.projects = append(w.projects, &project{name: "test", tasks: []*buildTask{wall}})

	if _, ok := w.claimNearestTask(b.Pos, b.ID); ok {
		t.Fatal("claimed a wall with no rock to build it from")
	}

	chest := Point{8, 12}
	w.SetTerrain(chest, Storage)
	w.refreshSpatial()
	c := w.storageContainers[chest]
	c.Inventory.Add(RawRock, 1)
	c.credit(ColonistOwner(b.ID), RawRock, 1)
	task, ok := w.claimNearestTask(b.Pos, b.ID)
	if !ok {
		t.Fatal("did not claim a wall it can pay for from its own chest")
	}
	w.assignTask(b, task)
	for i := 0; i < 200 && b.Job == JobBuild; i++ {
		w.jobBuild(b)
	}
	if w.TerrainAt(wall.pos) != Wall {
		t.Fatal("the wall was not built")
	}
	if c.held(ColonistOwner(b.ID), RawRock) != 0 || b.Inventory.Has(RawRock) {
		t.Fatal("the rock was not spent")
	}
}

// Without the safety net the planner puts up a scumhouse before anything
// else; with it, only when ordered.
func TestPlannerBuildsAScumhouseWhenFoodIsNotFree(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	cfg.InfiniteFood = false
	w := newTestWorld(t, cfg)
	w.planRooms()
	if !named(w.projects, "scumhouse") {
		t.Fatalf("projects = %v, want a scumhouse first", projectNames(w.projects))
	}

	cfg.InfiniteFood = true
	w = newTestWorld(t, cfg)
	for i := 0; i < 20; i++ {
		w.planRooms()
	}
	if named(w.projects, "scumhouse") {
		t.Fatal("planned a scumhouse unasked while pods feed everyone")
	}
	(&Engine{world: w}).apply(OrderScumhouse{})
	w.projects = nil
	w.planRooms()
	if !named(w.projects, "scumhouse") {
		t.Fatalf("an ordered scumhouse was not planned: %v", projectNames(w.projects))
	}
}

// The rim of a natural cavern nobody has found is not exposed scum; breaking
// into the cavern exposes it.
func TestHiddenCavernScumIsNotExposedUntilFound(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.Width, cfg.Height = 40, 24
	w := newTestWorld(t, cfg)
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			w.SetTerrain(Point{x, y}, Rock)
		}
	}
	for y := 2; y <= 5; y++ {
		for x := 2; x <= 6; x++ {
			cellAt(w, Point{x, y}).Explored = false
			w.carveHidden(Point{x, y})
		}
	}
	rim := Point{7, 3}
	cellAt(w, rim).Explored = false
	w.setScum(rim, w.cfg.ScumMax)
	w.refreshScumExposure(rim)
	if _, ok := w.exposedScum[rim]; ok {
		t.Fatal("scum on a hidden cavern's rim is exposed before anyone found the cavern")
	}
	w.SetTerrain(Point{7, 5}, Floor) // breaks into the cavern, away from the rim tile
	if !w.discovered(Point{6, 3}) {
		t.Fatal("digging beside the cavern did not discover it")
	}
	if _, ok := w.exposedScum[rim]; !ok {
		t.Fatal("scum on a found cavern's rim is not exposed")
	}
}

// Published scum is reused between frames, but never stale: on every tick of
// a colony scraping and regrowing scum, it matches a fresh copy.
func TestPublishedScumIsNeverStale(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 60, 36
	cfg.ScumSpawnPPM, cfg.ScumSpreadPercent = 20000, 90 // grow often, to catch a missed invalidation
	w := newTestWorld(t, cfg)
	reused := 0
	for i := 0; i < 2000; i++ {
		w.step()
		prev := w.snapScum
		got := w.publishedScum()
		if len(prev) > 0 && fmt.Sprintf("%p", prev) == fmt.Sprintf("%p", got) {
			reused++
		}
		want := map[Point]uint8{}
		for p := range w.exposedScum {
			if n := w.scumAt(p); n > 0 {
				want[p] = uint8(n)
			}
		}
		if !maps.Equal(got, want) {
			t.Fatalf("tick %d: published scum %v, want %v", w.tick, got, want)
		}
	}
	if reused == 0 {
		t.Fatal("published scum was never reused; the cache does nothing")
	}
}

// A kitchen is laid out as an assembly line: the scumhouse, a gap, and its
// pantry (with an aisle either side when the site is wide enough), so the
// stove and the pantry each keep their own access tiles. The planner links
// the pantry to its scumhouse.
func TestScumhouseRoomHasALinkedPantry(t *testing.T) {
	if got := scumhouseRoom.roomWidth(2); got != 5 {
		t.Fatalf("a full kitchen is %d wide, want 5", got)
	}
	cfg := testConfig()
	cfg.Width, cfg.Height = 80, 50
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.InfiniteFood = false
	w := newTestWorld(t, cfg)
	w.planRooms()
	var house, pantry Point
	var hasHouse, hasPantry bool
	for _, p := range w.projects {
		for _, task := range p.tasks {
			switch task.terrain {
			case Scumhouse:
				house, hasHouse = task.pos, true
			case Storage:
				pantry, hasPantry = task.pos, true
			}
		}
	}
	if !hasHouse || !hasPantry {
		t.Fatalf("kitchen planned with scumhouse %v, pantry %v", hasHouse, hasPantry)
	}
	if pantry != house.Add(2, 0) {
		t.Fatalf("pantry at %v, want two tiles along from the scumhouse at %v", pantry, house)
	}
	if w.pantryOf[house] != pantry {
		t.Fatal("the planner did not link the pantry to its scumhouse")
	}
	// Wide or narrow, the tile between the stove and the pantry is floor, so
	// the pantry has an access tile the cook at the stove never stands on.
	gap := house.Add(1, 0)
	for _, p := range w.projects {
		for _, task := range p.tasks {
			if task.pos == gap && task.terrain != Floor {
				t.Fatalf("the gap between stove and pantry at %v is planned as %v", gap, task.terrain)
			}
		}
	}
}

// The assembly line: meals cooked at the stove go into the pantry, and a
// colonist can reach the pantry while a cook stands at the stove.
func TestTheCookDoesNotBlockThePantry(t *testing.T) {
	w := propertyWorld(t)
	house, pantry := Point{10, 6}, Point{12, 6}
	w.SetTerrain(house, Scumhouse)
	w.SetTerrain(pantry, Storage)
	w.pantryOf[house], w.pantryHouse[pantry] = pantry, house
	// Walls either side of the stove, as in a narrow room: its one open
	// access tile below it is where the cook will stand.
	w.SetTerrain(Point{9, 6}, Wall)
	w.SetTerrain(Point{9, 7}, Wall)
	w.SetTerrain(Point{11, 7}, Wall)
	w.refreshSpatial()
	c := w.storageContainers[house]
	c.Inventory.Add(CaveScum, 2)
	c.credit(Community, CaveScum, 2)

	cook := w.spawn(Colonist, Point{10, 7})
	if !w.tryAssignCraft(cook) {
		t.Fatal("no cooking job")
	}
	for i := 0; i < 200 && cook.Job == JobCraft; i++ {
		w.jobCraft(cook)
	}
	if got := w.storageContainers[pantry].held(Community, Meal) + w.openQty(Ask, Meal, pantry, Community); got != 1 {
		t.Fatalf("the pantry holds %d of the colony's meals, want the 1 just cooked", got)
	}
	if c.Inventory.Count(Meal) != 0 {
		t.Fatal("the meal stayed in the stove's depot")
	}
	cook.Job = JobCraft // a cook standing at the stove, working the next recipe
	if _, ok := w.pathToAdjacent(Point{16, 10}, pantry); !ok {
		t.Fatal("nobody can reach the pantry while the cook stands at the stove")
	}
	buyer := w.spawn(Colonist, Point{16, 10})
	buyer.Needs[NeedFood] = w.cfg.Needs[NeedFood].Max
	if !w.tryBuyMeal(buyer) || w.storageContainers[pantry].held(ColonistOwner(buyer.ID), Meal) != 1 {
		t.Fatal("a hungry colonist could not buy the meal at the pantry")
	}
	assertMoneyConserved(t, w)
}

// A narrow silo room is a dead-end corridor, one tile of which reaches the
// chest. Someone loitering on that tile — eating a meal already in hand,
// chatting, idle — steps aside for a colonist who needs it, rather than
// starving the queue behind them.
func TestLoiterersMakeWayAtADeadEnd(t *testing.T) {
	for _, loiter := range []string{"eating", "idle", "cat"} {
		w := propertyWorld(t)
		// Walls either side of a corridor x=10, y=6..9, the chest at its end.
		for y := 5; y <= 9; y++ {
			w.SetTerrain(Point{9, y}, Wall)
			w.SetTerrain(Point{11, y}, Wall)
		}
		chest, end := Point{10, 5}, Point{10, 6}
		w.SetTerrain(chest, Storage)
		w.refreshSpatial()
		kind := Colonist
		if loiter == "cat" {
			kind = Cat
		}
		b := w.spawn(kind, end)
		if loiter == "eating" {
			b.Job, b.eat, b.Target, b.Progress = JobEat, eatMeal, chest, 0
		}
		e := w.spawn(Colonist, Point{10, 12})
		arrived := false
		for i := 0; i < 40 && !arrived; i++ {
			arrived, _ = w.travelTo(e, chest)
		}
		if !arrived {
			t.Fatalf("%s loiterer: the colonist never reached the chest; it is at %v, the loiterer at %v", loiter, e.Pos, b.Pos)
		}
		if loiter == "eating" && (b.Job != JobEat || b.eat != eatMeal) {
			t.Fatal("making way interrupted the loiterer's meal")
		}
	}
}
