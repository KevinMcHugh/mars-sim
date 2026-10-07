package sim

import "testing"

// foundryWorld is propertyWorld with a silo at (6, 6), a forge at (16, 6) and
// a gun bench at (18, 6), the colony holding ore at the silo, and n
// colonists with $100 each, none of them hungry. The colony makes no food
// and bids for no biomatter, so any work that happens is the foundry's chain.
func foundryWorld(t *testing.T, n int, ore int) (w *World, silo, forge, bench Point, cols []*Entity) {
	t.Helper()
	w = propertyWorld(t)
	w.cfg.MealReserve, w.cfg.ScumhouseBidQty = 0, 0
	// Nor does it fit its facility room out: with floor to spare it takes a
	// bunk and chairs (see roomplan.go), and colonists sleeping and talking
	// in them is not the work this measures.
	w.cfg.RoomExpansion, w.cfg.RoomMerge = false, false
	silo, forge, bench = Point{6, 6, LandingLevel}, Point{16, 6, LandingLevel}, Point{18, 6, LandingLevel}
	w.SetTerrain(silo, Storage)
	w.SetTerrain(forge, Forge)
	w.SetTerrain(bench, GunBench)
	w.refreshSpatial()
	stock(w, silo, Community, IronOre, ore)
	for i := 0; i < n; i++ {
		e := w.spawn(Colonist, Point{10 + 2*i, 12, LandingLevel})
		quietDrives(w, e)
		e.wallet = 100
		w.moneyIssued += 100 - Money(w.cfg.CrashPodPurse)
		cols = append(cols, e)
	}
	return w, silo, forge, bench, cols
}

// runFoundry steps w until the colony owns a rifle or limit ticks pass,
// keeping everyone fed and rested so the test is about work, and checking
// the money audit every tick. It returns the deepest plan it saw.
func runFoundry(t *testing.T, w *World, limit int) (depth int) {
	t.Helper()
	for i := 0; i < limit && w.armoryStock() == 0; i++ {
		w.step()
		for _, e := range w.entities {
			if e.Kind == Colonist {
				quietDrives(w, e)
			}
		}
		depth = max(depth, w.chainDepth())
		assertMoneyConserved(t, w)
	}
	return depth
}

// The foundry's gate: the colony's standing bid for a rifle at its silo
// reaches three links down with no scripted help. A gunsmith bids for steel
// at the gun bench; a smith bids for ore at the forge; a hauler buys the
// colony's surplus ore at the silo and carries it to the forge. The smith
// smelts, carries the steel to the bench, and the gunsmith machines a rifle
// and sells it to the colony.
func TestARifleBidReachesTheSilo(t *testing.T) {
	w, silo, forge, bench, cols := foundryWorld(t, 4, 30)
	depth := runFoundry(t, w, 6000)
	if w.armoryStock() == 0 {
		t.Fatalf("the colony never bought a rifle (plans: %d, orders: %d)", len(w.plans), len(w.orders))
	}
	if depth < 3 {
		t.Fatalf("deepest plan was %d links below the rifle bid, want 3", depth)
	}
	if got := w.storageContainers[silo].held(Community, AssaultRifle); got != 1 {
		t.Fatalf("the colony holds %d rifles at the silo, want 1", got)
	}
	traded := map[ItemKind]Point{}
	for _, tr := range w.trades {
		if tr.Item == IronOre && tr.Depot == forge || tr.Item == SteelIngot && tr.Depot == bench {
			traded[tr.Item] = tr.Depot
		}
	}
	if _, ok := traded[IronOre]; !ok {
		t.Fatal("no ore ever traded at the forge: the smith's derived bid was never filled")
	}
	if _, ok := traded[SteelIngot]; !ok {
		t.Fatal("no steel ever traded at the gun bench: the gunsmith's derived bid was never filled")
	}
	for _, e := range cols {
		if e.wallet < 0 {
			t.Fatalf("%s ended with %v", e.displayName(), e.wallet)
		}
	}
}

// A miner's own ore answers a smith's bid too: a colonist holding ore in a
// chest sells it straight to the forge, and one carrying ore takes it there
// from where it stands.
func TestOwnOreFillsTheForge(t *testing.T) {
	w, _, forge, _, cols := foundryWorld(t, 2, 0)
	smith, miner := ColonistOwner(cols[0].ID), cols[1]
	chest := Point{8, 14, LandingLevel}
	w.SetTerrain(chest, Storage)
	w.refreshSpatial()
	stock(w, chest, ColonistOwner(miner.ID), IronOre, 5)
	bid, _ := w.post(Bid, IronOre, 2, 8, smith, forge, 0)
	if !w.tryAssignProduce(miner) || miner.Job != JobCarry || miner.Target != chest {
		t.Fatalf("the miner took job %v to %v, want to fetch its ore from %v", miner.Job, miner.Target, chest)
	}
	for i := 0; i < 400 && w.orders[bid.ID] != nil; i++ {
		w.step()
		quietDrives(w, miner)
	}
	if got := w.storageContainers[forge].held(smith, IronOre); got != 2 {
		t.Fatalf("the smith holds %d ore at the forge, want 2", got)
	}
	if got := w.storageContainers[chest].held(ColonistOwner(miner.ID), IronOre); got != 3 {
		t.Fatalf("the miner kept %d ore in its chest, want 3", got)
	}

	carrier := w.spawn(Colonist, Point{12, 12, LandingLevel})
	carrier.Inventory.Add(IronOre, 2)
	w.transfer(ColonistOwner(carrier.ID), Community, carrier.wallet) // too poor to machine the rifle itself
	w.post(Bid, IronOre, 2, 8, smith, forge, 0)
	w.candidatesTick = -1 // the book changed since this tick's candidates were read
	if !w.tryAssignProduce(carrier) || carrier.Job != JobCarry || carrier.carry != carryDeliver || carrier.Target != forge {
		t.Fatalf("a colonist carrying ore took job %v to %v, want to deliver it to the forge", carrier.Job, carrier.Target)
	}
}

// The armory bids only for what it is short of, and only once a gun bench
// stands.
func TestArmoryBidsForItsShortfall(t *testing.T) {
	w := propertyWorld(t)
	silo := Point{6, 6, LandingLevel}
	w.SetTerrain(silo, Storage)
	w.refreshSpatial()
	w.refreshArmoryBids()
	if n := w.openQty(Bid, AssaultRifle, silo, Community); n != 0 {
		t.Fatalf("with no gun bench the colony bids for %d rifles", n)
	}
	w.SetTerrain(Point{18, 6, LandingLevel}, GunBench)
	stock(w, silo, Community, AssaultRifle, 1)
	w.refreshArmoryBids()
	w.refreshArmoryBids() // idempotent
	if n, want := w.openQty(Bid, AssaultRifle, silo, Community), w.cfg.ArmoryRifles-1; n != want {
		t.Fatalf("holding one rifle, the colony bids for %d, want %d", n, want)
	}
}

// With the armory wanted, the planner marks out a foundry once the rooms
// that keep the colony alive are in hand, and the colony builds it, paying
// for its forge in clay alone.
func TestTheColonyBuildsAFoundry(t *testing.T) {
	if got := constructionCost(Forge); len(got) != 1 || got[0] != (ItemStack{Clay, 4}) {
		t.Fatalf("a forge costs %v, want clay alone", got)
	}
	cfg := testConfig()
	cfg.Width, cfg.Height = 80, 48 // the default test map is mined out before a foundry fits
	cfg.ConstructionCosts = true
	w := newTestWorld(t, cfg)
	for i := 0; i < 8000; i++ {
		w.step()
		if w.countTerrain(Forge) > 0 && w.countTerrain(GunBench) > 0 {
			return
		}
	}
	t.Fatalf("no foundry by tick %d: %d forges and %d gun benches planned", w.tick,
		w.plannedFacilities(Forge), w.plannedFacilities(GunBench))
}

// With armory-rifles 0 the colony wants no foundry and posts no bid.
func TestNoArmoryNoFoundry(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.ArmoryRifles = 0
	silo := Point{6, 6, LandingLevel}
	w.SetTerrain(silo, Storage)
	w.SetTerrain(Point{18, 6, LandingLevel}, GunBench)
	w.refreshSpatial()
	w.refreshArmoryBids()
	if w.wantsFoundry() || w.openQty(Bid, AssaultRifle, silo, Community) != 0 {
		t.Fatal("with armory-rifles 0 the colony still wants rifles")
	}
}
