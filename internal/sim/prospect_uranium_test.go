package sim

import "testing"

// While someone bids for uranium and there is none to sell, one colonist
// prospects for it: straight to an exposed uranium face the colony can see,
// and only one at a time.
func TestAProspectorGoesAfterUranium(t *testing.T) {
	w, silo, _ := policyWorld(t, PolicyMonopoly)
	face := Point{4, 10, LandingLevel} // rock just west of the open floor
	cellAt(w, face).Composition = UraniumBearingRock
	if w.uraniumWanted() {
		t.Fatal("uranium wanted with no bid for it")
	}
	w.refreshReactorFuel() // the colony bids for its reserve
	w.tick++
	if !w.uraniumWanted() {
		t.Fatal("a bid for uranium, none anywhere, and nobody wants it")
	}
	a := w.spawn(Colonist, Point{6, 10, LandingLevel})
	b := w.spawn(Colonist, Point{7, 12, LandingLevel})
	if !w.tryProspectUranium(a) || a.Job != JobMine || a.Target != face {
		t.Fatalf("prospector job %v at %v, want mining the uranium at %v", a.Job, a.Target, face)
	}
	if w.tryProspectUranium(b) {
		t.Fatal("a second prospector went after uranium")
	}
	// Once there is ore to sell, nobody prospects.
	c := w.storageContainers[silo]
	c.Inventory.Add(UraniumOre, 1)
	c.credit(ColonistOwner(b.ID), UraniumOre, 1)
	w.tick++
	if w.uraniumWanted() {
		t.Fatal("uranium wanted with ore on sale at the silo")
	}
}

// The colony bids a little over the reference price for its reactor's
// fuel, as a baron does: at the reference price, ore lying on a miner's line
// at the silo never moved to it.
func TestTheColonyBidsAPremiumForFuel(t *testing.T) {
	w, silo, _ := policyWorld(t, PolicyMonopoly)
	w.refreshReactorFuel()
	b, ok := w.bestBid(UraniumOre, silo)
	if !ok || b.Price != fuelBidPrice(w.refPrice(UraniumOre)) || b.Price <= w.refPrice(UraniumOre) {
		t.Fatalf("colony's uranium bid %+v, want one at %v", b, fuelBidPrice(w.refPrice(UraniumOre)))
	}
}
