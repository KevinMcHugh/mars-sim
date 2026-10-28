package tui

import (
	"fmt"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// powerLines describes the colony's power on the treasury's market page: the
// policy and how to change it, the battery price in force, the reactors and
// who owns them, and the charter company's account. p cycles the policy and
// [ ] move the price (handleMarketKey). See docs/power.md.
func (m Model) powerLines() []string {
	pv := m.latest.Economy.Power
	if !pv.On {
		return []string{"power is off: stoves need no batteries"}
	}
	policy := fmt.Sprintf("policy %s  (p: change to %s)", pv.Policy, nextPolicy(pv.Policy))
	if pv.CooldownTicks > 0 && pv.TicksPerDay > 0 {
		policy = fmt.Sprintf("policy %s  (can change again in %d hours)", pv.Policy, (pv.CooldownTicks*24+pv.TicksPerDay-1)/pv.TicksPerDay)
	}
	price := fmt.Sprintf("tariff $%d a battery  ([ ]: a dollar less/more)", pv.Price)
	switch pv.PriceKind {
	case sim.PriceColonyBid:
		price = fmt.Sprintf("the colony pays $%d a battery at its stoves  ([ ]: a dollar less/more)", pv.Price)
	case sim.PriceCharter:
		price = fmt.Sprintf("%s charges $%d a battery", pv.Charter.Name, pv.Price)
	}
	lines := []string{policy, price, fmt.Sprintf("the colony holds %d batteries", pv.ColonyBatteries)}
	for _, r := range pv.Reactors {
		owner := m.ownerLabel(r.Owner)
		if r.Owner.Kind == sim.OwnerCorporation && pv.Charter.Granted {
			owner = pv.Charter.Name
		}
		line := fmt.Sprintf("reactor (%d,%d) %s, %d batteries, worth $%d", r.Pos.X, r.Pos.Y, owner, r.Batteries, r.Price)
		if r.ForSale {
			line += ", for sale"
		}
		lines = append(lines, line)
	}
	if c := pv.Charter; c.Granted {
		lines = append(lines, fmt.Sprintf("%s: sold %d, paid the colony $%d, sent home $%d, holds $%d", c.Name, c.Sold, c.Royalties, c.Remitted, c.Wallet))
	}
	return lines
}

// nextPolicy is the policy after p, in order, wrapping: what p cycles to.
func nextPolicy(p sim.PowerPolicy) sim.PowerPolicy {
	return (p + 1) % sim.PowerPolicy(len(sim.PowerPolicies()))
}
