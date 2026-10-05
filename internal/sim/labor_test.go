package sim

import "testing"

// The planner buys the rooms it plans: every task is a work order funded
// from the treasury, and whoever finishes a task is paid its wage.
func TestPublicWorksArePaidFromTheTreasury(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	before := w.treasury
	w.planRooms()
	if len(w.projects) == 0 {
		t.Fatal("nothing planned")
	}
	p := w.projects[0]
	if p.issuer != Community {
		t.Fatalf("issuer %v", p.issuer)
	}
	cost := w.projectCost(p)
	if w.treasury != before-cost || w.workEscrowed() != cost {
		t.Fatalf("treasury %v (was %v), escrow %v, cost %v", w.treasury, before, w.workEscrowed(), cost)
	}
	for _, task := range p.tasks {
		if task.order == nil || task.order.Pay != w.wageFor(task.terrain) {
			t.Fatalf("task %v has order %+v", task.pos, task.order)
		}
	}

	earned := Money(0)
	start := map[EntityID]Money{}
	for _, id := range w.entityIDsSorted() {
		start[id] = w.entities[id].wallet
	}
	for i := 0; i < 3000 && len(w.projects) > 0 && w.projects[0] == p; i++ {
		w.step()
	}
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Kind == Colonist && e.wallet > start[id] {
			earned += e.wallet - start[id]
		}
	}
	if earned == 0 {
		t.Fatal("nobody was paid for building the room")
	}
	assertMoneyConserved(t, w)
}

// An empty treasury halts public works, but not survival: colonists still
// build what they need to live on their own, unpaid, and nobody starves.
//
// This runs with the safety net on (testConfig); see
// TestAColonyWithNoMoneyStillFeedsItself for the same promise under scarcity.
func TestAnEmptyTreasuryHaltsPublicWorksNotSurvival(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	cfg.FoundingGrant = 0
	cfg.CrashPodMeals = 2
	w := newTestWorld(t, cfg)
	for i := 0; i < 5000; i++ {
		w.step()
		if len(w.projects) > 0 {
			t.Fatalf("tick %d: the colony planned %v with no money", w.tick, projectNames(w.projects))
		}
	}
	for _, d := range w.deceasedColonists {
		if d.Cause == "starved" {
			t.Fatalf("%s starved", d.Profile.Name)
		}
	}
	if w.countTerrain(NutrientPod) == 0 {
		t.Fatal("nobody built themselves a pod to live on")
	}
}

// A colonist with savings commissions a house, pays for it from its own
// wallet, and owns what gets built: a private bunk and a toilet it rents out.
func TestAColonistCommissionsAndPaysForAHouse(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	var rich *Entity
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Kind == Colonist {
			rich = e
			break
		}
	}
	rich.wallet += Money(cfg.HouseSavings)
	w.moneyIssued += Money(cfg.HouseSavings)
	before := rich.wallet
	w.commissionHouses()
	if !rich.commissioned || len(w.projects) != 1 || w.projects[0].issuer != ColonistOwner(rich.ID) {
		t.Fatalf("no commission: projects %v", projectNames(w.projects))
	}
	house := w.projects[0]
	if rich.wallet != before-w.projectCost(house) {
		t.Fatalf("wallet %v, want %v less the house's %v", rich.wallet, before, w.projectCost(house))
	}
	for i := 0; i < 6000 && len(w.projects) > 0 && w.projects[0] == house; i++ {
		w.step()
	}
	var bed, toilet *Fixture
	for _, task := range house.tasks {
		switch task.terrain {
		case Bed:
			bed = w.fixtures[task.pos]
		case Toilet:
			toilet = w.fixtures[task.pos]
		}
	}
	if bed == nil || toilet == nil {
		t.Fatal("the house was never finished")
	}
	me := ColonistOwner(rich.ID)
	if bed.Owner != me || bed.Access != AccessPrivate {
		t.Fatalf("bunk: %+v", bed)
	}
	if toilet.Owner != me || toilet.Access != AccessPaid || toilet.Price != Money(cfg.ToiletFee) {
		t.Fatalf("toilet: %+v", toilet)
	}
	assertMoneyConserved(t, w)
}

// A paid toilet charges anyone but its owner, and is closed to anyone who
// cannot pay.
func TestPaidToiletsChargeTheirUsers(t *testing.T) {
	w := propertyWorld(t)
	loo := Point{12, 10}
	w.SetTerrain(loo, Toilet)
	w.refreshSpatial()
	owner := w.spawn(Colonist, Point{6, 6})
	guest := w.spawn(Colonist, Point{12, 12})
	broke := w.spawn(Colonist, Point{14, 12})
	broke.wallet = 0
	w.setFixtureOwner(loo, ColonistOwner(owner.ID), AccessPaid)
	w.setFixturePrice(loo, 3)
	if w.canUseFixture(broke, loo) || !w.canUseFixture(guest, loo) || !w.canUseFixture(owner, loo) {
		t.Fatal("wrong access to the paid toilet")
	}
	ownerStart, guestStart := owner.wallet, guest.wallet
	for n := DriveKind(0); n < numDrives; n++ {
		w.setDrive(guest, n, 0) // nothing but the bladder calls
	}
	w.setDrive(guest, DriveBladder, w.cfg.Drives[DriveBladder].SeekAt+10)
	for n := DriveKind(0); n < numDrives; n++ {
		w.syncDrivePhase(guest, n)
	}
	for i := 0; i < 100 && w.driveLevel(guest, DriveBladder) > 0; i++ {
		w.step()
	}
	if w.driveLevel(guest, DriveBladder) != 0 {
		t.Fatal("the guest never used the toilet")
	}
	if guest.wallet != guestStart-3 || owner.wallet != ownerStart+3 {
		t.Fatalf("guest paid %v, owner earned %v", guestStart-guest.wallet, owner.wallet-ownerStart)
	}
}

// The colony keeps standing bids for biomatter at its scumhouse, and a
// colonist delivering its own scum sells into them on arrival.
func TestTheColonyBuysBiomatterAtItsScumhouse(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	w.tick = marketInterval
	w.runMarket()
	for _, k := range biomatterKinds {
		if got := w.openQty(Bid, k, house, Community); got != w.cfg.ScumhouseBidQty {
			t.Fatalf("colony bids for %d %s, want %d", got, k, w.cfg.ScumhouseBidQty)
		}
	}
	s := w.spawn(Colonist, Point{12, 12})
	start, treasury := s.wallet, w.treasury+w.moneyEscrowed()
	s.Inventory.Add(CaveScum, 3)
	if !w.deliverBiomatter(s, w.storageContainers[house]) {
		t.Fatal("delivery refused")
	}
	if want := start + 3*Money(w.cfg.PriceCaveScum); s.wallet != want {
		t.Fatalf("scraper has %v after selling 3 scum, want %v", s.wallet, want)
	}
	if got := w.storageContainers[house].held(Community, CaveScum); got != 3 {
		t.Fatalf("colony owns %d scum at the scumhouse, want 3", got)
	}
	if w.treasury+w.moneyEscrowed() != treasury-3*Money(w.cfg.PriceCaveScum) {
		t.Fatal("the colony did not pay for the scum")
	}
	assertMoneyConserved(t, w)
}

// A commissioner who dies takes its commission with it: the unbuilt work is
// cancelled and its escrow freezes with the dead colonist's money.
func TestACommissionDiesWithItsCommissioner(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	var e *Entity
	for _, id := range w.entityIDsSorted() {
		if c := w.entities[id]; c.Kind == Colonist {
			e = c
			break
		}
	}
	e.wallet += 1000
	w.moneyIssued += 1000
	if !w.planRoomFor(houseRoom, ColonistOwner(e.ID)) {
		t.Fatal("no house planned")
	}
	w.remove(e.ID, "test")
	for _, p := range w.projects {
		if p.issuer == ColonistOwner(e.ID) {
			t.Fatal("the dead colonist's house is still planned")
		}
	}
	for _, o := range w.workOrders {
		if o.Issuer == ColonistOwner(e.ID) {
			t.Fatal("the dead colonist's work orders are still open")
		}
	}
	assertMoneyConserved(t, w)
}

// Under scarcity (the game's defaults), a colony founded with no money still
// feeds itself: it cannot pay for any room, but it marks out its first
// scumhouse as unpaid community work — life support does not wait on money —
// and nothing else. A colonist can still spend its own crash-pod money: a
// chef with savings may buy a kitchen of its own.
func TestAColonyWithNoMoneyStillFeedsItself(t *testing.T) {
	for _, seed := range []int64{42, 1, 3} {
		cfg := DefaultConfig()
		cfg.Seed, cfg.TraitChance = seed, 0
		cfg.Width, cfg.Height = 40, 24
		cfg.StartAliens, cfg.FoundingGrant, cfg.CavernNestPercent = 0, 0, 0 // about food, not aliens
		cfg.ZoningAuto = true                                               // nobody draws zones here
		w := newTestWorld(t, cfg)
		for i := 0; i < 8000; i++ {
			w.step()
			for _, p := range w.projects {
				if p.issuer.Kind == OwnerColonist {
					continue // paid from its own wallet, not the treasury
				}
				if p.name != scumhouseRoom.name || p.issuer != Nobody {
					t.Fatalf("seed %d tick %d: planned %q for %v with no money", seed, w.tick, p.name, p.issuer)
				}
			}
		}
		if w.starved > 0 {
			t.Fatalf("seed %d: %d colonists starved", seed, w.starved)
		}
		if w.countTerrain(Scumhouse) == 0 {
			t.Fatalf("seed %d: no scumhouse was ever built", seed)
		}
	}
}

// A colonist killed mid-job releases what the job claimed: a haul order, a
// stove, a scum patch. Only starvation used to clear the job first.
func TestDeathReleasesAJobsClaims(t *testing.T) {
	w := propertyWorld(t)
	house, patch := Point{10, 6}, Point{15, 6}
	w.SetTerrain(house, Scumhouse)
	w.refreshSpatial()
	cook := w.spawn(Colonist, Point{10, 7})
	cook.Job, cook.Target = JobCraft, house
	w.workshopClaims[house] = cook.ID
	scraper := w.spawn(Colonist, Point{14, 7})
	scraper.Job, scraper.Target, scraper.scrape = JobScrape, patch, scrapeGather
	w.scumClaims[patch] = scraper.ID
	hauler := w.spawn(Colonist, Point{12, 9})
	hauler.Job, hauler.carryWork = JobCarry, 77
	w.haulClaims[77] = hauler.ID

	for _, e := range []*Entity{cook, scraper, hauler} {
		w.remove(e.ID, "bitten")
	}
	if len(w.workshopClaims) != 0 || len(w.scumClaims) != 0 || len(w.haulClaims) != 0 {
		t.Fatalf("claims outlived their claimants: workshops %v, scum %v, hauls %v", w.workshopClaims, w.scumClaims, w.haulClaims)
	}
}

// A project finished without every task's order being paid — a dig tile
// mined out by a miner who did not hold the task — refunds what is left when
// it is pruned, rather than leaving the escrow in orders nobody will close.
func TestAFinishedProjectRefundsUnpaidOrders(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	before := w.treasury + w.workEscrowed()
	w.planRooms()
	if len(w.projects) == 0 || w.workEscrowed() == 0 {
		t.Fatal("no funded project planned")
	}
	p := w.projects[0]
	for _, task := range p.tasks {
		w.SetTerrain(task.pos, task.terrain) // done by nobody the orders pay
	}
	w.pruneProjects()
	for _, q := range w.projects {
		if q == p {
			t.Fatal("the finished project was not pruned")
		}
	}
	for _, task := range p.tasks {
		if task.order != nil && w.workOrders[task.order.ID] != nil {
			t.Fatalf("order %d for %v is still open", task.order.ID, task.pos)
		}
	}
	if got := w.treasury + w.workEscrowed(); got != before {
		t.Fatalf("the colony has %v, want its %v back", got, before)
	}
	assertMoneyConserved(t, w)
}

// Without the safety net the first scumhouse is planned before anything else,
// a colonist's house included.
func TestAHouseWaitsForTheFirstScumhouse(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed, cfg.StartAliens, cfg.CavernNestPercent = 42, 0, 0
	cfg.Width, cfg.Height = 80, 50
	cfg.ZoningAuto = true
	w := newTestWorld(t, cfg)
	rich := w.entities[w.entityIDsSorted()[0]]
	w.transfer(Community, ColonistOwner(rich.ID), Money(cfg.HouseSavings))
	w.planRooms()
	if len(w.projects) == 0 || w.projects[0].name != scumhouseRoom.name {
		t.Fatalf("planned %v first; want the scumhouse", projectNames(w.projects))
	}
	if rich.commissioned {
		t.Fatal("a house was commissioned before the colony had a scumhouse")
	}
}
