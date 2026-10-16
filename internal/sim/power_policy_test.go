package sim

import "testing"

// A policy reads and writes by name, and an unknown name is an error a
// player can act on.
func TestPowerPolicyNames(t *testing.T) {
	for p := PowerPolicy(0); p < numPowerPolicies; p++ {
		var got PowerPolicy
		if err := got.Set(p.String()); err != nil || got != p {
			t.Errorf("%v: Set(%q) = %v, %v", p, p.String(), got, err)
		}
	}
	var p PowerPolicy
	if err := p.Set("feudal"); err == nil {
		t.Error("an unknown policy was accepted")
	}
}

// policyWorld is reactorWorld's silo and a kitchen, under policy, with no
// reactor yet.
func policyWorld(t *testing.T, policy PowerPolicy) (w *World, silo, kitchen Point) {
	t.Helper()
	w = propertyWorld(t)
	w.cfg.Power = true
	w.cfg.PowerPolicy, w.powerPolicy = policy, policy
	silo, kitchen = Point{8, 8, LandingLevel}, Point{20, 12, LandingLevel}
	w.SetTerrain(silo, Storage)
	w.SetTerrain(kitchen, Scumhouse)
	w.refreshSpatial()
	return w, silo, kitchen
}

// Under the market the colony builds no reactor and buys no fuel, even
// holding uranium; it bids for batteries at its stoves instead.
func TestUnderTheMarketTheColonyBuysBatteriesNotReactors(t *testing.T) {
	w, silo, kitchen := policyWorld(t, PolicyMarket)
	c := w.storageContainers[silo]
	c.Inventory.Add(UraniumOre, 1)
	c.credit(Community, UraniumOre, 1)
	if w.wantsReactor() {
		t.Fatal("the colony wants a reactor of its own under the market")
	}
	w.refreshReactorFuel()
	if n := w.openQty(Bid, UraniumOre, silo, Community); n != 0 {
		t.Fatalf("the colony bid for %d uranium under the market", n)
	}
	w.refreshColonyBatteryBids()
	if got := w.openQty(Bid, Battery, kitchen, Community); got != w.cfg.KitchenBatteryStock {
		t.Fatalf("battery bid for %d at the stove, want %d", got, w.cfg.KitchenBatteryStock)
	}
	// Under the monopoly it makes its own and bids for none.
	m, _, k := policyWorld(t, PolicyMonopoly)
	m.refreshColonyBatteryBids()
	if got := m.openQty(Bid, Battery, k, Community); got != 0 {
		t.Fatalf("the monopoly bid for %d batteries", got)
	}
}

// A colonist with reactor-savings commissions a reactor while the colony's
// stoves want batteries nobody has sold them, and not otherwise.
func TestABaronCommissionsAReactorWhenTheStovesWantPower(t *testing.T) {
	w, _, _ := policyWorld(t, PolicyMarket)
	e := w.spawn(Colonist, Point{12, 10, LandingLevel})
	e.wallet += Money(w.cfg.ReactorSavings)
	w.moneyIssued += Money(w.cfg.ReactorSavings)
	w.commissionReactors()
	if e.reactorCommissioned {
		t.Fatal("commissioned a reactor with no demand for batteries")
	}
	w.refreshColonyBatteryBids()
	w.commissionReactors()
	if !e.reactorCommissioned || len(w.projects) != 1 || w.projects[0].issuer != ColonistOwner(e.ID) {
		t.Fatalf("no reactor commissioned: projects %v", projectNames(w.projects))
	}
	// Under the monopoly nobody may.
	m, _, _ := policyWorld(t, PolicyMonopoly)
	f := m.spawn(Colonist, Point{12, 10, LandingLevel})
	f.wallet += Money(m.cfg.ReactorSavings)
	m.moneyIssued += Money(m.cfg.ReactorSavings)
	m.commissionReactors()
	if f.reactorCommissioned {
		t.Fatal("a colonist commissioned a reactor under the monopoly")
	}
}

// ownedReactor is a reactor owned by a new colonist, communal like a chef's
// kitchen: others sell into it, only its owner works it.
func ownedReactor(t *testing.T, w *World) (*Entity, *StorageContainer) {
	t.Helper()
	at := Point{20, 6, LandingLevel}
	w.SetTerrain(at, Reactor)
	w.refreshSpatial()
	owner := w.spawn(Colonist, Point{12, 10, LandingLevel})
	w.setFixtureOwner(at, ColonistOwner(owner.ID), AccessCommunal)
	return owner, w.storageContainers[at]
}

// A reactor's owner buys its fuel where miners sell it, at the silo, from
// its own wallet: ice to its stock, and one ore when it has neither ore nor
// pellets. What it buys is then hauled to its reactor for hire.
func TestABaronBuysItsFuelAtTheSilo(t *testing.T) {
	w, silo, _ := policyWorld(t, PolicyMarket)
	owner, c := ownedReactor(t, w)
	owner.wallet += 500
	w.moneyIssued += 500
	me := ColonistOwner(owner.ID)
	w.refreshOwnerReactorBids()
	if got := w.openQty(Bid, WaterIce, silo, me); got != w.cfg.ReactorIceStock {
		t.Fatalf("ice bid for %d at the silo, want %d", got, w.cfg.ReactorIceStock)
	}
	if got := w.openQty(Bid, UraniumOre, silo, me); got != 1 {
		t.Fatalf("uranium bid for %d at the silo, want 1", got)
	}
	// Once it owns ore at the silo, a haul order takes it to the reactor.
	sc := w.storageContainers[silo]
	sc.Inventory.Add(UraniumOre, 1)
	sc.credit(me, UraniumOre, 1)
	w.refreshOwnerReactorBids()
	hauls := 0
	for _, o := range w.workOrders {
		if o.Kind == WorkHaul && o.Issuer == me && o.Item == UraniumOre && o.From == silo && o.Pos == c.Pos {
			hauls += o.Units
		}
	}
	if hauls != 1 {
		t.Fatalf("%d units of the baron's ore ordered hauled to its reactor, want 1", hauls)
	}
	assertMoneyConserved(t, w)
}

// Only a reactor's owner works it, on its own goods; the colony's operator
// leaves a baron's reactor alone.
func TestOnlyTheOwnerWorksABaronsReactor(t *testing.T) {
	w, _, _ := policyWorld(t, PolicyMarket)
	owner, c := ownedReactor(t, w)
	me := ColonistOwner(owner.ID)
	c.Inventory.Add(UraniumOre, 1)
	c.credit(me, UraniumOre, 1)
	other := w.spawn(Colonist, Point{14, 10, LandingLevel})
	if w.tryAssignOwnReactor(other) || w.tryAssignReactor(other) {
		t.Fatal("another colonist took work at a baron's reactor")
	}
	if !w.tryAssignOwnReactor(owner) {
		t.Fatal("the owner would not work its own reactor")
	}
	for i := 0; i < 1000 && owner.Job == JobCraft; i++ {
		w.jobCraft(owner)
	}
	if c.held(me, FuelPellet) != 40 || c.held(me, UraniumOre) != 0 {
		t.Fatalf("the owner's ore should be 40 pellets of its own: %+v", c.Ledger)
	}
}

// Under the public option the colony runs no reactor until it has gone
// public-option-days without a battery; then it does, for good.
func TestThePublicOptionStepsInAfterDaysWithoutPower(t *testing.T) {
	w, silo, _ := policyWorld(t, PolicyPublicOption)
	c := w.storageContainers[silo]
	c.Inventory.Add(UraniumOre, 1)
	c.credit(Community, UraniumOre, 1)
	if w.colonyRunsPower() || w.wantsReactor() {
		t.Fatal("the public option ran power before it went without")
	}
	days := w.cfg.PublicOptionDays * w.cfg.TicksPerDay()
	for i := 0; i <= days+1; i++ {
		w.tick++
		w.notePowerShortage()
	}
	if !w.colonyRunsPower() || !w.wantsReactor() {
		t.Fatal("days without a battery, and the colony still runs no reactor")
	}
}
