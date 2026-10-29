package sim

import "testing"

// The power view shows a reactor's fuel and whether it is full, and what
// the policy adds: under the market, who could commission a reactor; under
// the public option, how long the colony has gone without a battery.
func TestThePowerViewShowsEachReactorAndThePolicy(t *testing.T) {
	w, _, _ := policyWorld(t, PolicyMarket)
	baron := w.spawn(Colonist, Point{12, 10, LandingLevel})
	baron.wallet += Money(w.cfg.ReactorSavings)
	w.moneyIssued += Money(w.cfg.ReactorSavings)
	at := reactorAt(t, w, ColonistOwner(baron.ID), ItemStack{FuelPellet, 3}, ItemStack{WaterIce, 2})
	v := w.powerView()
	if len(v.Reactors) != 1 {
		t.Fatalf("reactors = %+v", v.Reactors)
	}
	r := v.Reactors[0]
	if r.Pos != at || r.Pellets != 3 || r.Ice != 2 || r.Ore != 0 || r.Full || r.Operator != 0 {
		t.Fatalf("reactor = %+v", r)
	}
	if v.ReactorSavings != Money(w.cfg.ReactorSavings) || v.CanCommission != 1 {
		t.Fatalf("commission: savings %d, able %d", v.ReactorSavings, v.CanCommission)
	}
	if v.DryTicks != -1 {
		t.Fatalf("dry ticks under the market = %d", v.DryTicks)
	}

	c := w.storageContainers[at]
	c.Inventory.Add(Battery, w.cfg.BatteryReserve)
	c.credit(ColonistOwner(baron.ID), Battery, w.cfg.BatteryReserve)
	if !w.powerView().Reactors[0].Full {
		t.Fatal("a baron holding battery-reserve isn't full")
	}

	w, _, _ = policyWorld(t, PolicyPublicOption)
	w.notePowerShortage()
	w.tick += 10
	if v := w.powerView(); v.DryTicks != 10 || v.PublicReactor {
		t.Fatalf("public option: dry %d, latched %v", v.DryTicks, v.PublicReactor)
	}
}
