package sim

import "testing"

// uneasy puts e's safety at level and holds it there (no growth), so a test
// reads only what wants do.
func uneasy(w *World, e *Entity, level int) {
	w.setDrive(e, DriveSafety, level)
	e.drives[DriveSafety].rate = 0
}

// openBid is e's open want order, or nil.
func openWant(w *World, e *Entity) *Order {
	o := e.wants[DriveSafety].order
	if o == nil || w.orders[o.ID] != o {
		return nil
	}
	return o
}

// What a colonist will pay starts at the value at SeekAt, rises to
// gun-willingness percent of it at the ceiling, and comes only out of what it
// has beyond want-reserve-meals meals' worth: half of that until the drive is
// critical.
func TestWillingnessRisesWithFear(t *testing.T) {
	w, _, cs := marketWorld(t)
	e := cs[0]
	spec := w.cfg.Drives[DriveSafety]
	uneasy(w, e, spec.SeekAt)
	if got := w.willingness(e, DriveSafety, 20, 150); got != 20 {
		t.Fatalf("at seek-at: %v, want $20", got)
	}
	uneasy(w, e, spec.Max)
	if got := w.willingness(e, DriveSafety, 20, 150); got != 30 {
		t.Fatalf("at the ceiling: %v, want $30", got)
	}
	reserve := Money(w.cfg.WantReserveMeals) * w.valueOf(Meal)
	uneasy(w, e, spec.SeekAt)
	e.wallet = reserve + 10
	if got := w.willingness(e, DriveSafety, 20, 150); got != 5 {
		t.Fatalf("with $10 beyond its food money, uneasy: %v, want half of that", got)
	}
	e.wallet = reserve
	if got := w.willingness(e, DriveSafety, 20, 150); got != 0 {
		t.Fatalf("with only its food money: %v, want nothing", got)
	}
}

// A hungry colonist wants nothing else until it has eaten.
func TestFoodComesFirst(t *testing.T) {
	w, _, cs := marketWorld(t)
	e := cs[0]
	uneasy(w, e, w.cfg.Drives[DriveSafety].SeekAt+100)
	w.setDrive(e, DriveFood, w.cfg.Drives[DriveFood].SeekAt)
	w.postWants(e)
	if openWant(w, e) != nil {
		t.Fatal("a hungry colonist bid for a gun")
	}
}

// An unarmed colonist that feels unsafe bids for a gun at the silo, for a day;
// a calm one does not. A bid nobody fills lapses, and it bids again.
func TestAnUneasyColonistBidsForAGun(t *testing.T) {
	w, silo, cs := marketWorld(t)
	calm, scared := cs[0], cs[1]
	uneasy(w, calm, 0)
	uneasy(w, scared, w.cfg.Drives[DriveSafety].SeekAt+100)

	w.postWants(calm)
	w.postWants(scared)
	if openWant(w, calm) != nil {
		t.Fatal("a calm colonist bid for a gun")
	}
	o := openWant(w, scared)
	if o == nil || o.Side != Bid || o.Item != Pistol || o.Depot != silo || o.Expires != w.tick+w.cfg.WantTTL {
		t.Fatalf("want order %+v, want a day's pistol bid at the silo", o)
	}
	if line := w.wantLine(scared); line == "" {
		t.Fatal("the inspector has nothing to say about a colonist that wants a gun")
	}

	w.tick = o.Expires
	w.expireOrders()
	w.postWants(scared)
	if ws := scared.wants[DriveSafety]; ws.lapses != 1 || openWant(w, scared) == nil || openWant(w, scared) == o {
		t.Fatalf("after a lapse: %+v, want a fresh bid and one lapse", ws)
	}
}

// A gun for sale fills the bid; the buyer fetches it from the silo before it
// takes other work, and then wants nothing more until it has had a day to
// settle.
func TestABoughtGunIsFetched(t *testing.T) {
	w, silo, cs := marketWorld(t)
	seller, buyer := cs[0], cs[1]
	stock(w, silo, ColonistOwner(seller.ID), Pistol, 1)
	w.post(Ask, Pistol, 1, 15, ColonistOwner(seller.ID), silo, 0)
	uneasy(w, buyer, w.cfg.Drives[DriveSafety].SeekAt+50)

	w.postWants(buyer)
	w.postWants(buyer) // notes the fill
	if buyer.gunWaiting != weaponRank(Pistol) || buyer.wallet != 100-15 {
		t.Fatalf("waiting %d, wallet %v: the pistol was not bought for $15", buyer.gunWaiting, buyer.wallet)
	}
	if !w.tryFetchGun(buyer) || buyer.Job != JobFetch || buyer.Target != silo {
		t.Fatalf("job %v at %v, want a fetch from the silo", buyer.Job, buyer.Target)
	}
	for i := 0; i < 200 && buyer.Job == JobFetch; i++ {
		w.runJob(buyer)
	}
	if bestWeapon(buyer.Inventory) != Pistol || buyer.gunWaiting != 0 {
		t.Fatalf("carries %v, waiting %d", bestWeapon(buyer.Inventory), buyer.gunWaiting)
	}
	w.syncDriveActivity(buyer)
	w.postWants(buyer)
	if openWant(w, buyer) != nil {
		t.Fatal("wanted a better gun the moment it got one")
	}
	w.tick += w.cfg.WantSettleTicks
	w.postWants(buyer)
	if o := openWant(w, buyer); o == nil || o.Item == Pistol {
		t.Fatalf("after settling, still uneasy: want %+v, want a bid for a better gun", o)
	}
}

// Trading up: the better gun is fetched, and the old one goes on sale at the
// silo, in the colonist's name, at its value.
func TestTradingUpPutsTheOldGunOnSale(t *testing.T) {
	w, silo, cs := marketWorld(t)
	e := cs[0]
	e.Inventory.Add(Pistol, 1)
	stock(w, silo, ColonistOwner(e.ID), Shotgun, 1)
	e.gunWaiting = weaponRank(Shotgun)
	if !w.tryFetchGun(e) {
		t.Fatal("no fetch")
	}
	for i := 0; i < 200 && e.Job == JobFetch; i++ {
		w.runJob(e)
	}
	if bestWeapon(e.Inventory) != Shotgun || e.Inventory.Count(Pistol) != 0 {
		t.Fatalf("carries %+v", e.Inventory)
	}
	ask, ok := w.bestAsk(Pistol, silo)
	if !ok || ask.Actor != ColonistOwner(e.ID) || ask.Price != w.gunValue(Pistol) {
		t.Fatalf("the old pistol is not on sale: %+v", ask)
	}
	// The pistol on sale is in the silo, not just on its books: a buyer
	// could never take a gun the ledger has and the container does not.
	if c := w.storageContainers[silo]; !c.ledgerBalanced() || c.Inventory.Count(Pistol) != 1 {
		t.Fatalf("silo holds %d pistols, ledger %+v", c.Inventory.Count(Pistol), c.Ledger)
	}
}

// A colonist never sells the gun in its hand to a bid, however good.
func TestACarriedGunIsNotStock(t *testing.T) {
	w, silo, cs := marketWorld(t)
	armed, buyer := cs[0], cs[1]
	armed.Inventory.Add(AssaultRifle, 1)
	b, _ := w.post(Bid, AssaultRifle, 1, 90, ColonistOwner(buyer.ID), silo, 0)
	if _, own := w.ownStockFor(armed, b); own {
		t.Fatal("the rifle in its hand counted as stock to sell")
	}
}
