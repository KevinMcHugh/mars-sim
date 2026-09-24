package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/charmbracelet/lipgloss"
)

// marketListWidth fits a colonist's full name and a six-figure balance.
const marketListWidth = 40

// marketAccount is one row of the market tab's account list: the colony's
// treasury or one living colonist's wallet.
type marketAccount struct {
	label    string
	balance  sim.Money
	owner    sim.Owner
	colonist *sim.EntityView // nil for the treasury
}

// marketAccounts lists the treasury first, then every living colonist richest
// first, ties by ID so the order is stable from frame to frame.
func (m Model) marketAccounts() []marketAccount {
	s := m.latest
	accounts := []marketAccount{{label: "The colony (treasury)", balance: s.Economy.Treasury, owner: sim.Community}}
	var colonists []marketAccount
	for i := range s.Entities {
		e := &s.Entities[i]
		if e.Kind != sim.Colonist {
			continue
		}
		colonists = append(colonists, marketAccount{label: colonistName(*e), balance: e.Wallet,
			owner: sim.ColonistOwner(e.ID), colonist: e})
	}
	sort.SliceStable(colonists, func(i, j int) bool {
		if colonists[i].balance != colonists[j].balance {
			return colonists[i].balance > colonists[j].balance
		}
		return colonists[i].colonist.ID < colonists[j].colonist.ID
	})
	return append(accounts, colonists...)
}

// renderMarket draws the market tab: every account and its balance, and the
// colony's money supply. It grows with the economy — the order book and the
// depot ledgers land here as they are built. See docs/money.md.
func (m Model) renderMarket() string {
	header := m.renderHeader()
	footer := m.footerLine("MARKET  ↑↓/jk select account  tab/esc map  s spawn  b build  space pause  q quit")
	rows := m.rosterRows()

	accounts := m.marketAccounts()
	sel := clamp(m.marketSelected, 0, len(accounts)-1)
	listWidth, detailWidth := m.splitPanels(marketListWidth)
	body := m.renderMarketList(accounts, sel, rows, listWidth)
	if detailWidth > 0 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body,
			strings.Repeat(" ", panelGap),
			m.renderMarketDetail(accounts[sel], rows, detailWidth))
	}
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) renderMarketList(accounts []marketAccount, sel, rows, width int) string {
	inner := panelInner(width)
	capacity := max(1, rows-3)
	start := max(0, sel-capacity+1)
	end := min(len(accounts), start+capacity)

	var b strings.Builder
	b.WriteString(labelStyle.Render(fmt.Sprintf("MARKET"+divider+"ACCOUNTS (%d)", len(accounts))))
	for i := start; i < end; i++ {
		a := accounts[i]
		marker := "•"
		if i == sel {
			marker = "›"
		}
		amount := a.balance.String()
		name := cells.Truncate(marker+" "+a.label, max(1, inner-cells.Width(amount)-1))
		line := name + strings.Repeat(" ", max(1, inner-cells.Width(name)-cells.Width(amount))) + amount
		b.WriteByte('\n')
		if i == sel {
			b.WriteString(rosterSelStyle.Render(cells.Truncate(line, inner)))
		} else {
			b.WriteString(cells.Truncate(line, inner))
		}
	}
	return sidebarStyle.Width(width - borderCells).Height(rows - borderCells).MaxHeight(rows).Render(b.String())
}

func (m Model) renderMarketDetail(a marketAccount, rows, width int) string {
	inner := panelInner(width)
	econ := m.latest.Economy

	var b strings.Builder
	stat := func(label, val string) {
		b.WriteString(cells.Truncate(fmt.Sprintf("%-13s %s", label, val), inner))
		b.WriteByte('\n')
	}
	b.WriteString(titleStyle.Render(a.label))
	b.WriteString("\n\n")
	stat("Balance:", a.balance.String())
	if econ.Circulating > 0 {
		stat("Share:", fmt.Sprintf("%d%% of circulating money", int64(a.balance)*100/int64(econ.Circulating)))
	}

	b.WriteString("\n")
	b.WriteString(labelStyle.Render("HOLDINGS IN STORAGE"))
	b.WriteByte('\n')
	for _, line := range m.holdingLines(a.owner) {
		b.WriteString(cells.Truncate(line, inner))
		b.WriteByte('\n')
	}
	if n := m.fixturesOwnedBy(a.owner); n > 0 {
		stat("Fixtures:", fmt.Sprintf("%d owned", n))
	}

	b.WriteString("\n")
	b.WriteString(labelStyle.Render("MONEY SUPPLY"))
	b.WriteByte('\n')
	stat("Circulating:", econ.Circulating.String())
	stat("Frozen:", econ.Frozen.String()+" (held by the dead)")
	stat("Issued:", econ.Issued.String())
	return sidebarStyle.Width(width - borderCells).Height(rows - borderCells).MaxHeight(rows).Render(b.String())
}

// holdingLines totals what owner holds across every storage ledger, one line
// per item kind in item order.
func (m Model) holdingLines(owner sim.Owner) []string {
	totals := map[sim.ItemKind]int{}
	var kinds []sim.ItemKind
	for _, st := range m.latest.Storages {
		for _, l := range st.Ledger {
			if l.Owner != owner {
				continue
			}
			if _, seen := totals[l.Item]; !seen {
				kinds = append(kinds, l.Item)
			}
			totals[l.Item] += l.Count
		}
	}
	if len(kinds) == 0 {
		return []string{"nothing"}
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	lines := make([]string, 0, len(kinds))
	for _, k := range kinds {
		lines = append(lines, fmt.Sprintf("%s ×%d", k, totals[k]))
	}
	return lines
}

// fixturesOwnedBy counts the placed fixtures owner owns.
func (m Model) fixturesOwnedBy(owner sim.Owner) int {
	n := 0
	for _, f := range m.latest.Fixtures {
		if f.Owner == owner {
			n++
		}
	}
	return n
}

// ownerLabel names an owner for display: a colonist by name (living or dead),
// the colony, or nobody.
func (m Model) ownerLabel(o sim.Owner) string {
	switch o.Kind {
	case sim.OwnerCommunity:
		return "the colony"
	case sim.OwnerColonist:
		for i := range m.latest.Entities {
			if e := m.latest.Entities[i]; e.ID == o.ID {
				return colonistName(e)
			}
		}
		if e, ok := m.latest.Deceased[o.ID]; ok {
			return colonistName(e) + " (dead)"
		}
	}
	return o.String()
}
