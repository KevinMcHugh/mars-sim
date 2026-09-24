package sim

import (
	"math/rand"
	"testing"
)

// marketWorld is propertyWorld with a communal chest at (12, 6) — the colony's
// silo — and three colonists, each with $100 and nothing else.
func marketWorld(t *testing.T) (w *World, silo Point, cs [3]*Entity) {
	t.Helper()
	w = propertyWorld(t)
	silo = Point{12, 6}
	w.SetTerrain(silo, Storage)
	w.refreshSpatial()
	for i := range cs {
		cs[i] = w.spawn(Colonist, Point{8 + 2*i, 10})
		cs[i].wallet = 100
		w.moneyIssued += 100 - Money(w.cfg.CrashPodPurse)
	}
	return w, silo, cs
}

// stock puts n of kind into the depot at p, on owner's line.
func stock(w *World, p Point, owner Owner, kind ItemKind, n int) {
	c := w.storageContainers[p]
	c.Inventory.Add(kind, n)
	c.credit(owner, kind, n)
}

// A bid takes the cheapest asks first, the older one first at a tie, trades at
// the resting price, and gets back what it offered over.
func TestMatchingIsPriceThenTimeAtTheRestingPrice(t *testing.T) {
	w, silo, cs := marketWorld(t)
	a, b, buyer := ColonistOwner(cs[0].ID), ColonistOwner(cs[1].ID), ColonistOwner(cs[2].ID)
	stock(w, silo, a, IronOre, 5)
	stock(w, silo, b, IronOre, 5)
	w.post(Ask, IronOre, 2, 5, a, silo, 0)                  // dearest
	older, _ := w.post(Ask, IronOre, 2, 4, b, silo, 0)      // cheaper
	newer, _ := w.post(Ask, IronOre, 2, 4, a, silo, 0)      // as cheap, younger
	o, filled := w.post(Bid, IronOre, 3, 6, buyer, silo, 0) // crosses all three

	if filled != 3 || o.Qty != 0 {
		t.Fatalf("filled %d, left %d", filled, o.Qty)
	}
	if len(w.trades) != 2 || w.trades[0].Seller != b || w.trades[0].Qty != 2 || w.trades[1].Seller != a || w.trades[1].Qty != 1 {
		t.Fatalf("trades = %+v, want the older $4 ask first", w.trades)
	}
	if w.trades[0].Price != 4 || w.trades[1].Price != 4 {
		t.Fatal("did not trade at the resting price")
	}
	if cs[2].wallet != 100-3*4 {
		t.Fatalf("buyer paid %v, want $12 (the $2/unit over its limit refunded)", 100-cs[2].wallet)
	}
	if w.orders[older.ID] != nil || w.orders[newer.ID] == nil || w.orders[newer.ID].Qty != 1 {
		t.Fatal("the book did not fill the older ask first")
	}
	if w.storageContainers[silo].held(buyer, IronOre) != 3 {
		t.Fatal("the buyer did not get its ore")
	}
	assertMoneyConserved(t, w)
}

// One unit on offer sells once. The second bidder's order rests, its money
// held, and nothing is sold twice.
func TestNothingIsSoldTwice(t *testing.T) {
	w, silo, cs := marketWorld(t)
	seller := ColonistOwner(cs[0].ID)
	stock(w, silo, seller, Meal, 1)
	w.post(Ask, Meal, 1, 5, seller, silo, 0)
	_, first := w.post(Bid, Meal, 1, 5, ColonistOwner(cs[1].ID), silo, 0)
	second, got := w.post(Bid, Meal, 1, 5, ColonistOwner(cs[2].ID), silo, 0)
	if first != 1 || got != 0 || second.Qty != 1 {
		t.Fatalf("fills: %d then %d", first, got)
	}
	if cs[2].wallet != 95 || w.moneyEscrowed() != 5 {
		t.Fatalf("the resting bid holds %v; wallet %v", w.moneyEscrowed(), cs[2].wallet)
	}
	if !w.storageContainers[silo].ledgerBalanced() {
		t.Fatal("ledger unbalanced")
	}
	assertMoneyConserved(t, w)
}

// An order that cannot be funded is refused; cancelling and expiring give the
// escrow back.
func TestEscrowIsFundedAndReturned(t *testing.T) {
	w, silo, cs := marketWorld(t)
	me := ColonistOwner(cs[0].ID)
	if o, _ := w.post(Bid, IronOre, 50, 3, me, silo, 0); o != nil {
		t.Fatal("placed a $150 bid from a $100 wallet")
	}
	if o, _ := w.post(Ask, IronOre, 1, 3, me, silo, 0); o != nil {
		t.Fatal("offered ore it does not have")
	}
	stock(w, silo, me, IronOre, 4)
	ask, _ := w.post(Ask, IronOre, 4, 3, me, silo, 10)
	bid, _ := w.post(Bid, Clay, 10, 2, me, silo, 0)
	c := w.storageContainers[silo]
	if c.held(me, IronOre) != 0 || cs[0].wallet != 80 {
		t.Fatalf("escrow not taken: ore %d, wallet %v", c.held(me, IronOre), cs[0].wallet)
	}
	w.cancel(bid)
	if cs[0].wallet != 100 {
		t.Fatalf("cancel refunded to %v", cs[0].wallet)
	}
	w.tick += 10
	w.expireOrders()
	if w.orders[ask.ID] != nil || c.held(me, IronOre) != 4 {
		t.Fatal("an expired ask did not give its goods back")
	}
	assertMoneyConserved(t, w)
}

// Dying cancels a colonist's orders before its wallet freezes.
func TestDeathCancelsOrders(t *testing.T) {
	w, silo, cs := marketWorld(t)
	me := ColonistOwner(cs[0].ID)
	w.post(Bid, Clay, 10, 3, me, silo, 0)
	w.remove(cs[0].ID, "test")
	if len(w.orders) != 0 || w.moneyFrozen != 100 {
		t.Fatalf("orders %d, frozen %v", len(w.orders), w.moneyFrozen)
	}
	assertMoneyConserved(t, w)
}

// Random trading, twice from the same seed: identical trades both times, and
// every ledger and the money supply balance throughout.
func TestRandomTradingIsDeterministicAndConserved(t *testing.T) {
	run := func() []Trade {
		w, silo, cs := marketWorld(t)
		r := rand.New(rand.NewSource(3))
		for _, c := range cs {
			for _, k := range []ItemKind{IronOre, Clay, Meal} {
				stock(w, silo, ColonistOwner(c.ID), k, 20)
			}
		}
		kinds := []ItemKind{IronOre, Clay, Meal}
		for i := 0; i < 400; i++ {
			w.tick++
			who := ColonistOwner(cs[r.Intn(3)].ID)
			if r.Intn(3) == 0 {
				who = Community
			}
			side := Side(r.Intn(2))
			w.post(side, kinds[r.Intn(3)], 1+r.Intn(4), Money(1+r.Intn(8)), who, silo, 1+r.Intn(30))
			if i%7 == 0 {
				w.expireOrders()
			}
			if !w.storageContainers[silo].ledgerBalanced() {
				t.Fatalf("step %d: ledger unbalanced", i)
			}
			assertMoneyConserved(t, w)
		}
		return append([]Trade(nil), w.trades...)
	}
	a, b := run(), run()
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("trade counts %d and %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("trade %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// Paid prospecting: ore unloaded at the silo is sold into the colony's
// standing bid, and the miner is paid from the treasury.
func TestProspectorsArePaidByTheColony(t *testing.T) {
	w, silo, cs := marketWorld(t)
	miner := cs[0]
	w.tick = marketInterval
	w.runMarket() // the colony posts its standing bids
	if w.openQty(Bid, IronOre, silo, Community) != w.cfg.SiloBidQty {
		t.Fatal("no standing bid for iron ore at the silo")
	}
	miner.Inventory.Add(RawRock, (InventorySlotCount-1)*MaxStackSize)
	miner.Inventory.Add(IronOre, 10)
	treasury := w.treasury + w.moneyEscrowed() // the colony's money, standing bids included
	if !w.tryAssignStore(miner) || miner.Target != silo {
		t.Fatalf("miner did not head for the silo: job %v target %v", miner.Job, miner.Target)
	}
	for i := 0; i < 100 && miner.Job == JobStore; i++ {
		w.jobStore(miner)
	}
	price := Money(w.cfg.PriceIronOre)
	if miner.wallet != 100+10*price {
		t.Fatalf("miner has %v, want $%d more for 10 iron ore", miner.wallet, 10*price)
	}
	if w.storageContainers[silo].held(Community, IronOre) != 10 {
		t.Fatal("the colony does not own the ore it bought")
	}
	if w.moneyEscrowed()+w.treasury != treasury-10*price {
		t.Fatal("the payment did not come out of the colony's standing bid")
	}
	assertMoneyConserved(t, w)
}

// A hungry colonist with nothing of its own buys a meal at market rather than
// eat gruel; a colonist with meals to spare takes them there to sell.
func TestHungryColonistsBuyWhatOthersSell(t *testing.T) {
	w, silo, cs := marketWorld(t)
	seller, buyer := cs[0], cs[1]
	locker := Point{8, 14}
	w.SetTerrain(locker, Storage)
	w.setFixtureOwner(locker, ColonistOwner(seller.ID), AccessPrivate)
	w.SetTerrain(Point{20, 12}, NutrientPod) // the safety net, which should go unused
	w.refreshSpatial()
	stock(w, locker, ColonistOwner(seller.ID), Meal, w.cfg.MealKeep+3)

	if !w.tryAssignSellMeals(seller) {
		t.Fatal("the seller would not take its surplus to market")
	}
	for i := 0; i < 200 && seller.Job == JobSell; i++ {
		w.jobSell(seller)
	}
	if w.openQty(Ask, Meal, silo, ColonistOwner(seller.ID)) != 3 {
		t.Fatalf("offered %d meals, want the 3 over meal-keep", w.openQty(Ask, Meal, silo, ColonistOwner(seller.ID)))
	}

	buyer.Needs[NeedFood] = w.cfg.Needs[NeedFood].SeekAt + 10
	w.syncNeedPhase(buyer, NeedFood)
	for i := 0; i < 200 && w.needLevel(buyer, NeedFood) > 0; i++ {
		w.runNeedFocus(buyer, NeedFood)
		if buyer.Job == JobUse {
			t.Fatal("the buyer went for gruel with meals on sale")
		}
	}
	if w.needLevel(buyer, NeedFood) != 0 || buyer.wallet != 100-Money(w.cfg.PriceMeal) ||
		seller.wallet != 100+Money(w.cfg.PriceMeal) {
		t.Fatalf("hunger %d, buyer %v, seller %v", w.needLevel(buyer, NeedFood), buyer.wallet, seller.wallet)
	}
	assertMoneyConserved(t, w)
}
