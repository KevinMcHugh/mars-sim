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
	for n := NeedKind(0); n < numNeeds; n++ {
		guest.Needs[n] = 0 // nothing but the bladder calls
	}
	guest.Needs[NeedBladder] = w.cfg.Needs[NeedBladder].SeekAt + 10
	for n := NeedKind(0); n < numNeeds; n++ {
		w.syncNeedPhase(guest, n)
	}
	for i := 0; i < 100 && w.needLevel(guest, NeedBladder) > 0; i++ {
		w.step()
	}
	if w.needLevel(guest, NeedBladder) != 0 {
		t.Fatal("the guest never used the toilet")
	}
	if guest.wallet != guestStart-3 || owner.wallet != ownerStart+3 {
		t.Fatalf("guest paid %v, owner earned %v", guestStart-guest.wallet, owner.wallet-ownerStart)
	}
}

// The colony pays a bounty for biomatter brought to its scumhouse.
func TestBiomatterDeliveriesEarnTheBounty(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	w.tick = marketInterval
	w.runMarket()
	if w.workEscrowed() != Money(w.cfg.BountyPay)*Money(w.cfg.BountyUnits) {
		t.Fatalf("bounty escrow %v", w.workEscrowed())
	}
	s := w.spawn(Colonist, Point{12, 12})
	start := s.wallet
	s.Inventory.Add(CaveScum, 3)
	s.cargo[CaveScum] = Community
	if !w.deliverBiomatter(s, w.storageContainers[house]) {
		t.Fatal("delivery refused")
	}
	if s.wallet != start+3*Money(w.cfg.BountyPay) {
		t.Fatalf("paid %v for 3 units", s.wallet-start)
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
