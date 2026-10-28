package sim

import "testing"

// reactorAt puts a reactor at the usual spot, owned by owner, holding its
// goods.
func reactorAt(t *testing.T, w *World, owner Owner, stock ...ItemStack) Point {
	t.Helper()
	at := Point{20, 6, LandingLevel}
	w.SetTerrain(at, Reactor)
	w.refreshSpatial()
	w.setFixtureOwner(at, owner, AccessCommunal)
	c := w.storageContainers[at]
	for _, s := range stock {
		c.Inventory.Add(s.Kind, s.Count)
		c.credit(owner, s.Kind, s.Count)
	}
	return at
}

// Changing to the monopoly nationalizes a baron's reactor: the colony pays
// its price from the treasury, and the reactor and the goods in it become
// the colony's.
func TestNationalizingABaronsReactor(t *testing.T) {
	w, _, _ := policyWorld(t, PolicyMarket)
	baron := w.spawn(Colonist, Point{12, 10, LandingLevel})
	me := ColonistOwner(baron.ID)
	at := reactorAt(t, w, me, ItemStack{FuelPellet, 10}, ItemStack{Battery, 6})
	price := w.reactorPrice(at)
	purse, treasury := baron.wallet, w.treasury
	if !w.setPowerPolicy(PolicyMonopoly) {
		t.Fatal("the policy did not change")
	}
	if w.fixtureOwner(at) != Community || baron.wallet != purse+price || w.treasury != treasury-price {
		t.Fatalf("owner %v, baron paid %v (want %v), treasury down %v", w.fixtureOwner(at), baron.wallet-purse, price, treasury-w.treasury)
	}
	c := w.storageContainers[at]
	if c.held(Community, FuelPellet) != 10 || c.held(Community, Battery) != 6 || c.held(me, Battery) != 0 {
		t.Fatalf("the reactor's goods did not go with it: %+v", c.Ledger)
	}
	assertMoneyConserved(t, w)
}

// A change the colony can't pay for is refused, and nothing moves.
func TestAPolicyChangeTheColonyCantAffordIsRefused(t *testing.T) {
	w, _, _ := policyWorld(t, PolicyMarket)
	baron := w.spawn(Colonist, Point{12, 10, LandingLevel})
	at := reactorAt(t, w, ColonistOwner(baron.ID))
	w.moneyIssued -= w.treasury
	w.treasury = 0
	if w.setPowerPolicy(PolicyMonopoly) {
		t.Fatal("the colony nationalized a reactor with an empty treasury")
	}
	if w.powerPolicy != PolicyMarket || w.fixtureOwner(at) != ColonistOwner(baron.ID) {
		t.Fatal("a refused change still changed something")
	}
}

// From the monopoly to the market the colony's reactor is for sale, and the
// first colonist who can afford one buys it instead of building.
func TestTheMarketBuysTheColonysReactor(t *testing.T) {
	w, _, _ := policyWorld(t, PolicyMonopoly)
	at := reactorAt(t, w, Community)
	if !w.setPowerPolicy(PolicyMarket) || len(w.reactorsForSale()) != 1 {
		t.Fatalf("after the change %d reactors for sale, want 1", len(w.reactorsForSale()))
	}
	e := w.spawn(Colonist, Point{12, 10, LandingLevel})
	e.wallet += Money(w.cfg.ReactorSavings)
	w.moneyIssued += Money(w.cfg.ReactorSavings)
	treasury, price := w.treasury, w.reactorPrice(at)
	w.commissionReactors()
	if w.fixtureOwner(at) != ColonistOwner(e.ID) || !e.reactorCommissioned || w.treasury != treasury+price {
		t.Fatalf("owner %v, commissioned %v, treasury up %v (want %v)", w.fixtureOwner(at), e.reactorCommissioned, w.treasury-treasury, price)
	}
	if len(w.projects) != 0 {
		t.Fatal("the buyer commissioned a reactor as well")
	}
	assertMoneyConserved(t, w)
}

// Granting a charter mid-game has the company buy every reactor, the
// colony's included; ending it has the colony buy the company out, and the
// company's money go home.
func TestGrantingAndEndingACharter(t *testing.T) {
	w, _, kitchen := policyWorld(t, PolicyMonopoly)
	at := reactorAt(t, w, Community, ItemStack{FuelPellet, 5})
	treasury, price := w.treasury, w.reactorPrice(at)
	if !w.setPowerPolicy(PolicyCharter) || !w.charter.granted {
		t.Fatal("no charter granted")
	}
	if w.fixtureOwner(at) != w.charterOwner() || w.treasury != treasury+price {
		t.Fatalf("owner %v, treasury up %v (want %v)", w.fixtureOwner(at), w.treasury-treasury, price)
	}
	if got := w.balance(w.charterOwner()); got != Money(w.cfg.CharterCapital) {
		t.Fatalf("the company holds %v after the buyout, want its capital %v", got, w.cfg.CharterCapital)
	}
	assertMoneyConserved(t, w)

	// Batteries of the company's at a stove go with it when it leaves.
	me := w.charterOwner()
	k := w.storageContainers[kitchen]
	k.Inventory.Add(Battery, 4)
	k.credit(me, Battery, 4)
	w.lastPolicyChange = -1 // past the cooldown
	if !w.setPowerPolicy(PolicyMarket) {
		t.Fatal("the charter did not end")
	}
	if w.charter.granted || w.fixtureOwner(at) != Community || k.held(Community, Battery) != 4 {
		t.Fatalf("granted %v, reactor %v, colony's batteries at the stove %d", w.charter.granted, w.fixtureOwner(at), k.held(Community, Battery))
	}
	if w.charter.wallet != 0 {
		t.Fatalf("the company left $%d behind", w.charter.wallet)
	}
	assertMoneyConserved(t, w)
}

// A change waits power-policy-cooldown days after the last.
func TestAPolicyChangeWaitsOutTheCooldown(t *testing.T) {
	w, _, _ := policyWorld(t, PolicyMonopoly)
	if !w.setPowerPolicy(PolicyMarket) {
		t.Fatal("the first change was refused")
	}
	if w.setPowerPolicy(PolicyMonopoly) {
		t.Fatal("changed again at once")
	}
	w.tick += w.cfg.PolicyCooldownDays * w.cfg.TicksPerDay()
	if !w.setPowerPolicy(PolicyMonopoly) {
		t.Fatal("refused after the cooldown")
	}
}

// The command reaches the world through the engine.
func TestSetPowerPolicyIsACommand(t *testing.T) {
	cfg := testConfig()
	cfg.Power = true
	e := NewEngine(cfg)
	e.apply(SetPowerPolicy{Policy: PolicyMarket})
	if e.world.powerPolicy != PolicyMarket {
		t.Fatalf("policy %v after the command, want market", e.world.powerPolicy)
	}
}

// The battery price the player sets depends on the policy: the monopoly's
// tariff (a battery's value), the colony's bid at its stoves under the
// market, and nothing under the charter, whose price is the company's.
func TestTheBatteryPriceFollowsThePolicy(t *testing.T) {
	w, _, _ := policyWorld(t, PolicyMonopoly)
	if !w.setBatteryPrice(9) || w.valueOf(Battery) != 9 {
		t.Fatalf("tariff 9: a battery is worth %v", w.valueOf(Battery))
	}
	if w.setBatteryPrice(-1) || w.setBatteryPrice(maxBatteryPrice+1) {
		t.Fatal("an out-of-range price was accepted")
	}

	m, _, k := policyWorld(t, PolicyMarket)
	m.refreshColonyBatteryBids()
	if b, ok := m.bestBid(Battery, k); !ok || b.Price != Money(m.cfg.PriceBattery) {
		t.Fatalf("battery bid %+v, want one at %v", b, m.cfg.PriceBattery)
	}
	if !m.setBatteryPrice(7) {
		t.Fatal("the bid price did not change")
	}
	if n := m.openQty(Bid, Battery, k, Community); n != 0 {
		t.Fatalf("%d batteries still bid for at the old price", n)
	}
	m.refreshColonyBatteryBids()
	if b, ok := m.bestBid(Battery, k); !ok || b.Price != 7 {
		t.Fatalf("battery bid %+v after the change, want one at $7", b)
	}
	assertMoneyConserved(t, m)

	c, _, _ := charterWorld(t)
	if c.setBatteryPrice(1) {
		t.Fatal("the player set the charter company's price")
	}
}

// The power metrics count what the reactors and stoves did.
func TestPowerIsCharted(t *testing.T) {
	w, c := poweredKitchen(t, ItemStack{CaveScum, 4}, ItemStack{Battery, 1})
	cookOnce(w) // powered, then cold for the second meal
	for i := 0; i < 3 && c.held(Community, CaveScum) > 0; i++ {
		cookOnce(w)
	}
	if w.cookedPowered != 1 || w.cookedCold != 1 {
		t.Fatalf("cooked %d powered and %d cold, want 1 and 1", w.cookedPowered, w.cookedCold)
	}
	keys := map[string]bool{}
	for _, d := range MetricDefs() {
		keys[d.Key] = true
	}
	for _, k := range []string{"colony-batteries", "battery-price", "batteries-charged", "cooked-powered", "cooked-cold", "charter-royalties", "charter-remitted"} {
		if !keys[k] {
			t.Errorf("no %q metric", k)
		}
	}
}
