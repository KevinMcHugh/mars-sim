package tui

import (
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	tea "github.com/charmbracelet/bubbletea"
)

// The market's selection follows the account, not the row: the list sorts by
// balance every frame, and a colonist whose balance changed rank used to take
// the highlight with it.
func TestMarketSelectionFollowsTheAccount(t *testing.T) {
	snap := func(a, b sim.Money) *sim.Snapshot {
		return &sim.Snapshot{Entities: []sim.EntityView{
			{ID: 1, Kind: sim.Colonist, Wallet: a},
			{ID: 2, Kind: sim.Colonist, Wallet: b},
		}}
	}
	m := New(nil, nil)
	m.latest = snap(200, 100) // treasury, #1, #2
	next, _ := m.handleMarketKey(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	accounts := m.marketAccounts()
	if got := accounts[m.marketSelection(accounts)].owner; got != sim.ColonistOwner(1) {
		t.Fatalf("selected %v, want colonist #1", got)
	}
	m.latest = snap(50, 100) // #2 is now the richer, and sorts first
	accounts = m.marketAccounts()
	if got := accounts[m.marketSelection(accounts)].owner; got != sim.ColonistOwner(1) {
		t.Fatalf("after a re-sort the selection moved to %v; want it to stay on colonist #1", got)
	}
}
