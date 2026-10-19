package tui

import (
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The market page's power lines say the policy, how to change it, the price
// in force, and each reactor with its owner; the charter's price is the
// company's, with no keys to move it.
func TestPowerLines(t *testing.T) {
	m := Model{latest: &sim.Snapshot{}}
	m.latest.Economy.Power = sim.PowerView{On: true, Policy: sim.PolicyMonopoly, Price: 3, PriceKind: sim.PriceTariff,
		Reactors: []sim.ReactorView{{Pos: sim.Point{X: 4, Y: 5}, Owner: sim.Community, Price: 150, Batteries: 12}}}
	got := strings.Join(m.powerLines(), "\n")
	for _, want := range []string{"policy monopoly", "change to market", "tariff $3", "reactor (4,5) the colony, 12 batteries, worth $150"} {
		if !strings.Contains(got, want) {
			t.Errorf("power lines missing %q:\n%s", want, got)
		}
	}
	m.latest.Economy.Power.Policy, m.latest.Economy.Power.PriceKind = sim.PolicyCharter, sim.PriceCharter
	m.latest.Economy.Power.Charter = sim.CharterView{Granted: true, Name: "MarsCorp", Sold: 7}
	got = strings.Join(m.powerLines(), "\n")
	if !strings.Contains(got, "MarsCorp charges $3") || strings.Contains(got, "[ ]") || !strings.Contains(got, "change to monopoly") {
		t.Errorf("charter power lines:\n%s", got)
	}
	if !strings.Contains(strings.Join(Model{latest: &sim.Snapshot{}}.powerLines(), ""), "power is off") {
		t.Error("power off is not said")
	}
}
