package sim

import "testing"

// charterWorld is policyWorld under the charter, granted.
func charterWorld(t *testing.T) (w *World, silo, kitchen Point) {
	t.Helper()
	w, silo, kitchen = policyWorld(t, PolicyCharter)
	w.grantCharter()
	if !w.charter.granted {
		t.Fatal("no charter granted")
	}
	return w, silo, kitchen
}

// The charter goes to the configured corporation, once, with its capital
// minted from off-world; no other policy grants one.
func TestTheCharterIsGrantedOnceWithItsCapital(t *testing.T) {
	w, _, _ := charterWorld(t)
	if got := w.balance(w.charterOwner()); got != Money(w.cfg.CharterCapital) {
		t.Fatalf("the charter company holds %v, want its capital %v", got, w.cfg.CharterCapital)
	}
	w.grantCharter()
	if got := w.balance(w.charterOwner()); got != Money(w.cfg.CharterCapital) {
		t.Fatal("the charter was granted twice")
	}
	if w.charterName() != w.corporations[0].Name {
		t.Fatalf("charter held by %q, want the roster's first, %q", w.charterName(), w.corporations[0].Name)
	}
	assertMoneyConserved(t, w)

	m, _, _ := policyWorld(t, PolicyMarket)
	m.grantCharter()
	if m.charter.granted {
		t.Fatal("a charter granted under the market")
	}
}

// Under the charter nobody else builds a reactor: not the colony, not a
// colonist with savings. The company commissions one as soon as the colony
// has a kitchen to sell to.
func TestOnlyTheCharterBuildsAReactor(t *testing.T) {
	w, silo, _ := charterWorld(t)
	c := w.storageContainers[silo]
	c.Inventory.Add(UraniumOre, 1)
	c.credit(Community, UraniumOre, 1)
	if w.wantsReactor() {
		t.Fatal("the colony wants a reactor of its own under the charter")
	}
	e := w.spawn(Colonist, Point{12, 10, LandingLevel})
	e.wallet += Money(w.cfg.ReactorSavings)
	w.moneyIssued += Money(w.cfg.ReactorSavings)
	w.refreshColonyBatteryBids()
	w.commissionReactors()
	if e.reactorCommissioned {
		t.Fatal("a colonist commissioned a reactor under the charter")
	}
	w.commissionCharterReactor()
	if len(w.projects) != 1 || w.projects[0].issuer != w.charterOwner() {
		t.Fatalf("the charter commissioned nothing: projects %v", projectNames(w.projects))
	}
	w.commissionCharterReactor()
	if len(w.projects) != 1 {
		t.Fatal("the charter commissioned a second reactor")
	}
	assertMoneyConserved(t, w)
}

// The company sells at its stoves at charter-price, the colony bids that
// price, and every sale pays the colony its royalty, a battery at a time
// included.
func TestTheCharterSellsAndPaysItsRoyalty(t *testing.T) {
	w, _, kitchen := charterWorld(t)
	me := w.charterOwner()
	k := w.storageContainers[kitchen]
	k.Inventory.Add(Battery, 10)
	k.credit(me, Battery, 10)
	treasury := w.treasury
	w.refreshColonyBatteryBids()
	w.refreshCharterSales()
	bought := k.held(Community, Battery)
	if bought == 0 {
		t.Fatal("the colony bought none of the charter's batteries")
	}
	paid := Money(bought) * Money(w.cfg.CharterPrice)
	royalty := paid * Money(w.cfg.CharterRoyalty) / 100
	if w.charter.sold != bought || w.charter.royalties != royalty {
		t.Fatalf("sold %d (want %d), royalties %v (want %v)", w.charter.sold, bought, w.charter.royalties, royalty)
	}
	if want := treasury - paid + royalty; w.treasury+escrowedBy(w, Community) != want {
		t.Fatalf("treasury %v (+%v in escrow), want %v", w.treasury, escrowedBy(w, Community), want)
	}
	// One battery at a time still pays, once the cents add up.
	w2, _, _ := charterWorld(t)
	for i := 0; i < 5; i++ {
		w2.chargeRoyalty(w2.charterOwner(), Battery, 1, 6)
	}
	if w2.charter.royalties != 3 {
		t.Fatalf("five $6 sales at 10%% paid %v, want $3", w2.charter.royalties)
	}
	assertMoneyConserved(t, w)
}

// Once a day the company sends home whatever it holds above its capital:
// money leaving the colony's supply, and still accounted for.
func TestTheCharterSendsItsProfitHome(t *testing.T) {
	w, _, _ := charterWorld(t)
	w.mint(w.charterOwner(), 250) // as if it had sold
	day := w.cfg.TicksPerDay()
	w.tick = day
	w.remitCharterProfit()
	if got := w.balance(w.charterOwner()); got != Money(w.cfg.CharterCapital) {
		t.Fatalf("after a day the company holds %v, want its capital %v", got, w.cfg.CharterCapital)
	}
	if w.charter.remitted != 250 {
		t.Fatalf("remitted %v, want 250", w.charter.remitted)
	}
	w.mint(w.charterOwner(), 40)
	w.remitCharterProfit()
	if w.charter.remitted != 250 {
		t.Fatal("remitted twice in one day")
	}
	assertMoneyConserved(t, w)
}

// The company's reactor is worked by a colonist it pays, as the colony's
// is; a colonist doesn't work it for itself.
func TestTheCharterHiresItsOperator(t *testing.T) {
	w, _, _ := charterWorld(t)
	at := Point{20, 6, LandingLevel}
	w.SetTerrain(at, Reactor)
	w.refreshSpatial()
	me := w.charterOwner()
	w.setFixtureOwner(at, me, AccessCommunal)
	c := w.storageContainers[at]
	c.Inventory.Add(UraniumOre, 1)
	c.credit(me, UraniumOre, 1)
	op := w.spawn(Colonist, Point{12, 10, LandingLevel})
	wage, capital := op.wallet, w.balance(me)
	if w.tryAssignOwnReactor(op) {
		t.Fatal("a colonist worked the charter's reactor as its own")
	}
	if !w.operatorWanted() || !w.tryAssignReactor(op) {
		t.Fatal("no operator's job at the charter's reactor")
	}
	for i := 0; i < 1000 && op.Job == JobCraft; i++ {
		w.jobCraft(op)
	}
	if c.held(me, FuelPellet) != 40 {
		t.Fatalf("the charter's ore should be its 40 pellets: %+v", c.Ledger)
	}
	if op.wallet != wage+Money(w.cfg.WageReactor) || w.balance(me) != capital-Money(w.cfg.WageReactor) {
		t.Fatalf("operator has %v (want %v), company %v (want %v)", op.wallet, wage+Money(w.cfg.WageReactor), w.balance(me), capital-Money(w.cfg.WageReactor))
	}
	assertMoneyConserved(t, w)
}

// escrowedBy is what owner has put up in its open bids.
func escrowedBy(w *World, owner Owner) Money {
	var n Money
	for _, o := range w.orders {
		if o.Actor == owner {
			n += o.escrow
		}
	}
	return n
}
