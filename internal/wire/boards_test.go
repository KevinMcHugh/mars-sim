package wire

import (
	"fmt"
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// colony runs a real game flat out until it is past tick n.
func colony(t *testing.T, n int) *sim.Snapshot {
	t.Helper()
	cfg := sim.DefaultConfig()
	cfg.Seed = 7
	cfg.Width, cfg.Height = 200, 200
	cfg.TicksPerSecond = 1_000_000
	cfg.ZoningAuto = true // a colony that builds its silo without anyone drawing zones
	eng := sim.NewEngine(cfg)
	var snap *sim.Snapshot
	for deadline := time.Now().Add(time.Minute); snap == nil || snap.Tick < n; {
		if s, _ := eng.Advance(50 * time.Millisecond); s != nil {
			snap = s
		}
		if time.Now().After(deadline) {
			t.Fatalf("stuck before tick %d", n)
		}
	}
	return snap
}

// The list tabs carry what the TUI's show, from a colony a few thousand ticks
// in: projects with their tiles, containers, and a working market.
func TestBoardTopics(t *testing.T) {
	snap := colony(t, 3000)

	var jobs JobsTopic
	due(t, snap, "jobs", &jobs)
	if len(jobs.Projects) != len(snap.Projects) {
		t.Fatalf("%d projects, snapshot has %d", len(jobs.Projects), len(snap.Projects))
	}
	for i, p := range jobs.Projects {
		src := snap.Projects[i]
		if p.Name != src.Name || len(p.Tasks) != len(src.Tasks) || p.Done != src.TasksDone() {
			t.Errorf("project %d = %+v", i, p)
		}
		for _, tk := range p.Tasks {
			if tk.Done && tk.Builder != nil {
				t.Errorf("a done task has a builder: %+v", tk)
			}
		}
	}

	var storage []StorageRow
	due(t, snap, "storage", &storage)
	if len(storage) != len(snap.Storages) || len(storage) == 0 {
		t.Fatalf("%d containers, snapshot has %d", len(storage), len(snap.Storages))
	}
	for _, r := range storage {
		if r.Label == "" || r.Slots == 0 || r.Used > r.Slots || (r.Used > 0) != (r.Top != "") {
			t.Errorf("container = %+v", r)
		}
		sum := 0
		for _, h := range r.Contents {
			sum += h.Count
		}
		if sum != r.Items || r.Ledger == nil {
			t.Errorf("container contents = %+v, ledger = %+v, items %d", r.Contents, r.Ledger, r.Items)
		}
	}

	var m MarketTopic
	due(t, snap, "market", &m)
	if m.Accounts[0].Key != "colony" || len(m.Accounts) != snap.Stats.Colonists+1 {
		t.Fatalf("accounts = %+v", m.Accounts)
	}
	for i := 2; i < len(m.Accounts); i++ {
		if m.Accounts[i].Balance > m.Accounts[i-1].Balance {
			t.Errorf("accounts not richest first: %+v", m.Accounts)
		}
	}
	if m.Supply.Issued != int64(snap.Economy.Issued) || len(m.Prices) == 0 {
		t.Errorf("supply %+v, %d prices", m.Supply, len(m.Prices))
	}
	if len(snap.Economy.Trades) > 0 && m.Trades[0].Tick != snap.Economy.Trades[len(snap.Economy.Trades)-1].Tick {
		t.Errorf("trades not newest first: %+v", m.Trades[0])
	}

	// The order desk lists exactly the colony's orders, and every depot is
	// communal with the silo first.
	colonyOrders := 0
	for _, o := range snap.Economy.Orders {
		if o.Actor == sim.Community {
			colonyOrders++
		}
	}
	if len(m.Colony.Orders) != colonyOrders || colonyOrders == 0 {
		t.Errorf("%d colony orders on the desk, %d in the snapshot", len(m.Colony.Orders), colonyOrders)
	}
	if len(m.Colony.Depots) == 0 || !m.Colony.Depots[0].Silo || len(m.Colony.Items) == 0 {
		t.Errorf("depots %+v, items %v", m.Colony.Depots, m.Colony.Items)
	}
	for _, d := range m.Colony.Depots {
		if f, ok := snap.FixtureAt(sim.Point{X: d.X, Y: d.Y, Level: sim.LandingLevel}); ok && f.Access != sim.AccessCommunal {
			t.Errorf("depot %+v is not communal", d)
		}
	}

	// Every account's page resolves, and a colonist's balance matches.
	for _, a := range m.Accounts {
		var acct AccountTopic
		due(t, snap, "account:"+a.Key, &acct)
		if !acct.Found || acct.Balance != a.Balance || acct.Label != a.Label {
			t.Errorf("account %s = %+v, listed as %+v", a.Key, acct, a)
		}
	}
	// Every open order's page resolves, and its book lists it.
	for _, so := range snap.Economy.Orders {
		var o OrderTopic
		due(t, snap, fmt.Sprintf("order:%d", so.ID), &o)
		if !o.Found || o.Open != so.Qty || o.Qty != o.Open+o.Filled || o.Item != so.Item.String() {
			t.Errorf("order %d = %+v, snapshot has %+v", so.ID, o, so)
		}
		var b BookTopic
		due(t, snap, fmt.Sprintf("book:%d,%d:%s", so.Depot.X, so.Depot.Y, so.Item), &b)
		listed := false
		for _, r := range append(b.Bids, b.Asks...) {
			listed = listed || r.ID == uint64(so.ID)
		}
		if !listed {
			t.Errorf("order %d is missing from its book %+v", so.ID, b)
		}
	}

	var gone AccountTopic
	due(t, snap, fmt.Sprintf("account:%d", 1<<40), &gone)
	if gone.Found {
		t.Error("an unknown colonist's account was found")
	}
	if err := NewTopics().Subscribe("account:treasury"); err == nil {
		t.Error("a malformed account subscribed")
	}
}

// A container's row totals its stacks by item, in item order, and carries
// its ledger for the storage tab's searches.
func TestStorageRowContents(t *testing.T) {
	snap := fixture(true)
	me := sim.ColonistOwner(1)
	snap.Entities[0].Profile = &sim.Profile{Name: "Uma Xu"}
	var inv sim.StorageInventory
	inv[0] = sim.ItemStack{Kind: sim.Meal, Count: 4}
	inv[1] = sim.ItemStack{Kind: sim.RawRock, Count: 2}
	inv[3] = sim.ItemStack{Kind: sim.Meal, Count: 3}
	snap.Storages = []sim.StorageView{{Pos: sim.Point{X: 1, Y: 1, Level: sim.LandingLevel}, Inventory: inv,
		Ledger: []sim.LedgerLine{{Owner: me, Item: sim.Meal, Count: 7}, {Owner: sim.Community, Item: sim.RawRock, Count: 2}}}}
	var rows []StorageRow
	due(t, snap, "storage", &rows)
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	r := rows[0]
	want := []Holding{{Item: sim.RawRock.String(), Count: 2}, {Item: sim.Meal.String(), Count: 7}}
	if sim.RawRock > sim.Meal {
		want[0], want[1] = want[1], want[0]
	}
	if fmt.Sprint(r.Contents) != fmt.Sprint(want) || r.Top != sim.Meal.String()+" ×7" {
		t.Errorf("contents = %+v, top %q", r.Contents, r.Top)
	}
	if len(r.Ledger) != 2 || r.Ledger[0].Owner != "Uma Xu" || r.Ledger[1].Owner != "the colony" {
		t.Errorf("ledger = %+v", r.Ledger)
	}
}

// Holdings total across containers, by owner, in item order.
func TestAccountHoldings(t *testing.T) {
	snap := fixture(true)
	me := sim.ColonistOwner(1)
	snap.Entities[0].Profile = &sim.Profile{Name: "Uma Xu"}
	snap.Storages = []sim.StorageView{
		{Pos: sim.Point{X: 1, Y: 1, Level: sim.LandingLevel}, Ledger: []sim.LedgerLine{{Owner: me, Item: sim.Meal, Count: 2}, {Owner: sim.Community, Item: sim.Meal, Count: 5}}},
		{Pos: sim.Point{X: 2, Y: 1, Level: sim.LandingLevel}, Ledger: []sim.LedgerLine{{Owner: me, Item: sim.Meal, Count: 3}, {Owner: me, Item: sim.RawRock, Count: 1}}},
	}
	var a AccountTopic
	due(t, snap, "account:1", &a)
	if len(a.Holdings) != 2 || a.Holdings[0].Item != sim.RawRock.String() || a.Holdings[1].Count != 5 {
		t.Errorf("holdings = %+v", a.Holdings)
	}
}
