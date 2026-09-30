package wire

import (
	"cmp"
	"slices"
	"strconv"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The list tabs' topics, one per TUI details tab: "jobs" (the job board,
// render_jobboard.go), "storage" (render_storage.go) and "market"
// (render_market.go), plus "account:<owner>" for one market account's page,
// "colony" or a colonist's id. They change slowly next to the map, so they
// are rebuilt at most every boardEvery and, as every topic, sent only when
// they changed.
const boardEvery = 500 * time.Millisecond

// ---- jobs -----------------------------------------------------------------

// JobsTopic is the job board: every queued project, and the manual orders
// still waiting for a build site.
type JobsTopic struct {
	Projects []Project `json:"projects"`
	// Pending counts orders by what they will build ("facility room": 2).
	Pending map[string]int `json:"pending"`
}

// Project is one queued construction project.
type Project struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	QueuedTick int      `json:"queuedTick"`
	Done       int      `json:"done"`
	Tasks      []Task   `json:"tasks"`
	Assignees  []Person `json:"assignees"`
}

// Task is one tile of a project. Builder is set while someone is building it.
type Task struct {
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Terrain string  `json:"terrain"`
	Phase   int     `json:"phase"`
	Done    bool    `json:"done"`
	Builder *Person `json:"builder,omitempty"`
}

// Person is a colonist by id and name, for the page to link.
type Person struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

func jobsTopic(s *sim.Snapshot) JobsTopic {
	t := JobsTopic{Projects: make([]Project, 0, len(s.Projects)), Pending: map[string]int{}}
	names := colonistNames(s)
	person := func(id sim.EntityID) Person {
		name, ok := names[id]
		if !ok {
			name = "colonist #" + strconv.FormatUint(uint64(id), 10)
		}
		return Person{ID: uint64(id), Name: name}
	}
	for _, p := range s.Projects {
		pr := Project{ID: p.ID, Name: p.Name, QueuedTick: p.QueuedTick, Done: p.TasksDone(),
			Tasks: make([]Task, 0, len(p.Tasks)), Assignees: []Person{}}
		for _, tk := range p.Tasks {
			task := Task{X: tk.Pos.X, Y: tk.Pos.Y, Terrain: tk.Terrain.String(), Phase: tk.Phase, Done: tk.Done}
			if tk.Owner != 0 && !tk.Done {
				b := person(tk.Owner)
				task.Builder = &b
			}
			pr.Tasks = append(pr.Tasks, task)
		}
		for _, id := range p.Assignees() {
			pr.Assignees = append(pr.Assignees, person(id))
		}
		t.Projects = append(t.Projects, pr)
	}
	for what, n := range map[string]int{
		"facility room": s.PendingFacilityRooms, "dormitory": s.PendingDormitories,
		"trash room": s.PendingTrashRooms, "storage room": s.PendingStorageRooms,
		"scumhouse": s.PendingScumhouses, "foundry": s.PendingFoundries,
	} {
		if n > 0 {
			t.Pending[what] = n // a JSON object: encoding/json sorts its keys
		}
	}
	return t
}

// ---- storage --------------------------------------------------------------

// StorageRow is one container in the storage list. Its contents and ledger
// are the tile inspector's (tile:<x>,<y>), which the page opens on a click.
type StorageRow struct {
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Label    string `json:"label"`
	Used     int    `json:"used"`
	Slots    int    `json:"slots"`
	Items    int    `json:"items"`
	Capacity int    `json:"capacity"`
	// Top is what it holds most of, "meal ×9", for a glance down the list.
	Top string `json:"top,omitempty"`
}

func storageTopic(s *sim.Snapshot) []StorageRow {
	rows := make([]StorageRow, 0, len(s.Storages))
	for _, st := range s.Storages {
		info := storageInfo(s, st)
		r := StorageRow{X: st.Pos.X, Y: st.Pos.Y, Label: info.Label, Used: info.Used, Slots: info.Slots,
			Items: info.Items, Capacity: info.Capacity}
		totals := map[string]int{}
		var best string
		for _, c := range info.Contents {
			totals[c.Item] += c.Count
			if best == "" || totals[c.Item] > totals[best] || totals[c.Item] == totals[best] && c.Item < best {
				best = c.Item
			}
		}
		if best != "" {
			r.Top = best + " ×" + strconv.Itoa(totals[best])
		}
		rows = append(rows, r)
	}
	return rows
}

// ---- market ---------------------------------------------------------------

// MarketTopic is the market tab: the accounts, the money supply, and the
// market's own lists, as the TUI's treasury page shows them.
type MarketTopic struct {
	// Accounts is the treasury first, then every living colonist richest
	// first, ties by id.
	Accounts   []Account   `json:"accounts"`
	Supply     MoneySupply `json:"supply"`
	Books      []Book      `json:"books"`
	Prices     []Price     `json:"prices"`
	Plans      []PlanLine  `json:"plans"`
	ChainDepth int         `json:"chainDepth"`
	Work       []WorkLine  `json:"work"`
	Trades     []TradeLine `json:"trades"` // newest first
}

// Account is one balance. Key is its account:<key> topic: "colony" or a
// colonist's id.
type Account struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Balance int64  `json:"balance"`
}

// MoneySupply is the colony's money at a glance. See docs/money.md.
type MoneySupply struct {
	Treasury    int64 `json:"treasury"`
	Circulating int64 `json:"circulating"`
	Escrowed    int64 `json:"escrowed"`
	Frozen      int64 `json:"frozen"`
	Issued      int64 `json:"issued"`
	Starved     int   `json:"starved"`
}

// Book is one (item, depot) order book with anything in it or any history.
type Book struct {
	Item    string `json:"item"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	BestBid int64  `json:"bestBid"`
	BidQty  int    `json:"bidQty"`
	BestAsk int64  `json:"bestAsk"`
	AskQty  int    `json:"askQty"`
	Traded  bool   `json:"traded"`
	Last    int64  `json:"last"`
	Volume  int    `json:"volume"`
}

// Price is one good's value; Traded is false while it is still the charter's
// reference price.
type Price struct {
	Item   string `json:"item"`
	Value  int64  `json:"value"`
	Traded bool   `json:"traded"`
}

// PlanLine is one open production plan.
type PlanLine struct {
	Actor   Person `json:"actor"`
	Summary string `json:"summary"`
	Waiting bool   `json:"waiting"` // on inputs from its own bids
}

// WorkLine totals the open work orders of one issuer and kind.
type WorkLine struct {
	Issuer string `json:"issuer"`
	Kind   string `json:"kind"`
	Units  int    `json:"units"`
	Held   int64  `json:"held"` // what the issuer holds to pay for them
}

// TradeLine is one trade.
type TradeLine struct {
	Tick   int    `json:"tick"`
	Seller string `json:"seller"`
	Buyer  string `json:"buyer"`
	Qty    int    `json:"qty"`
	Item   string `json:"item"`
	Price  int64  `json:"price"`
}

// maxTradeLines is how many recent trades the market tab lists.
const maxTradeLines = 20

func marketTopic(s *sim.Snapshot) MarketTopic {
	econ := s.Economy
	t := MarketTopic{
		Accounts: []Account{{Key: "colony", Label: "The colony (treasury)", Balance: int64(econ.Treasury)}},
		Supply: MoneySupply{Treasury: int64(econ.Treasury), Circulating: int64(econ.Circulating),
			Escrowed: int64(econ.Escrowed), Frozen: int64(econ.Frozen), Issued: int64(econ.Issued), Starved: econ.Starved},
		Books:      []Book{},
		Prices:     make([]Price, 0, len(econ.Prices)),
		Plans:      make([]PlanLine, 0, len(econ.Plans)),
		ChainDepth: econ.ChainDepth,
		Work:       []WorkLine{},
		Trades:     []TradeLine{},
	}
	var colonists []sim.EntityView
	for _, e := range s.Entities {
		if e.Kind == sim.Colonist {
			colonists = append(colonists, e)
		}
	}
	slices.SortStableFunc(colonists, func(a, b sim.EntityView) int {
		if c := cmp.Compare(b.Wallet, a.Wallet); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	for _, e := range colonists {
		t.Accounts = append(t.Accounts, Account{Key: strconv.FormatUint(uint64(e.ID), 10), Label: entityName(e), Balance: int64(e.Wallet)})
	}
	for _, b := range econ.Books {
		if b.BidQty == 0 && b.AskQty == 0 && !b.Traded {
			continue
		}
		t.Books = append(t.Books, Book{Item: b.Item.String(), X: b.Depot.X, Y: b.Depot.Y,
			BestBid: int64(b.BestBid), BidQty: b.BidQty, BestAsk: int64(b.BestAsk), AskQty: b.AskQty,
			Traded: b.Traded, Last: int64(b.Last), Volume: b.Volume})
	}
	for _, p := range econ.Prices {
		t.Prices = append(t.Prices, Price{Item: p.Item.String(), Value: int64(p.Value), Traded: p.Traded})
	}
	names := colonistNames(s)
	for _, p := range econ.Plans {
		t.Plans = append(t.Plans, PlanLine{Actor: Person{ID: uint64(p.Actor), Name: names[p.Actor]}, Summary: p.Summary, Waiting: p.Waiting})
	}
	// Work orders totalled by issuer and kind, in first-seen order: the
	// orders are oldest first, so the order is stable.
	index := map[[2]string]int{}
	for _, o := range econ.WorkOrders {
		key := [2]string{ownerLabel(s, o.Issuer), o.Kind.String()}
		i, ok := index[key]
		if !ok {
			i = len(t.Work)
			index[key] = i
			t.Work = append(t.Work, WorkLine{Issuer: key[0], Kind: key[1]})
		}
		t.Work[i].Units += o.Units
		t.Work[i].Held += int64(o.Pay) * int64(o.Units)
	}
	for i := len(econ.Trades) - 1; i >= 0 && len(t.Trades) < maxTradeLines; i-- {
		tr := econ.Trades[i]
		t.Trades = append(t.Trades, TradeLine{Tick: tr.Tick, Seller: ownerLabel(s, tr.Seller), Buyer: ownerLabel(s, tr.Buyer),
			Qty: tr.Qty, Item: tr.Item.String(), Price: int64(tr.Price)})
	}
	return t
}

// ---- account ----------------------------------------------------------------

// AccountTopic is one account's page: its balance, what it owns, and what it
// has open on the market.
type AccountTopic struct {
	Found    bool   `json:"found"` // false once the colonist is dead or gone
	Label    string `json:"label"`
	Balance  int64  `json:"balance"`
	SharePct int64  `json:"sharePct"` // of circulating money
	// Holdings totals its items across every storage ledger, in item order.
	Holdings []Holding  `json:"holdings"`
	Fixtures int        `json:"fixtures"`
	Orders   []OrderRow `json:"orders"`
	Plans    []string   `json:"plans"` // a colonist's open plans
}

// Holding is how many of an item an owner has in storage.
type Holding struct {
	Item  string `json:"item"`
	Count int    `json:"count"`
}

// OrderRow is one open order.
type OrderRow struct {
	Side  string `json:"side"`
	Qty   int    `json:"qty"`
	Item  string `json:"item"`
	Price int64  `json:"price"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
}

// accountParam resolves the key after "account:".
func accountParam(key string) (topic, bool) {
	owner := sim.Community
	if key != "colony" {
		id, err := strconv.ParseUint(key, 10, 64)
		if err != nil {
			return topic{}, false
		}
		owner = sim.ColonistOwner(sim.EntityID(id))
	}
	return topic{every: boardEvery, build: func(s *sim.Snapshot) any { return accountTopic(s, owner) }}, true
}

func accountTopic(s *sim.Snapshot, owner sim.Owner) AccountTopic {
	t := AccountTopic{Holdings: []Holding{}, Orders: []OrderRow{}, Plans: []string{}}
	var actor sim.EntityID
	if owner.Kind == sim.OwnerCommunity {
		t.Found, t.Label, t.Balance = true, "The colony (treasury)", int64(s.Economy.Treasury)
	} else {
		for _, e := range s.Entities {
			if e.ID == owner.ID && e.Kind == sim.Colonist {
				t.Found, t.Label, t.Balance, actor = true, entityName(e), int64(e.Wallet), e.ID
				break
			}
		}
		if !t.Found {
			return t
		}
	}
	if c := int64(s.Economy.Circulating); c > 0 {
		t.SharePct = t.Balance * 100 / c
	}
	totals := map[sim.ItemKind]int{}
	for _, st := range s.Storages {
		for _, l := range st.Ledger {
			if l.Owner == owner {
				totals[l.Item] += l.Count
			}
		}
	}
	kinds := make([]sim.ItemKind, 0, len(totals))
	for k := range totals {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds) // item order, not the map's
	for _, k := range kinds {
		t.Holdings = append(t.Holdings, Holding{Item: k.String(), Count: totals[k]})
	}
	for _, f := range s.Fixtures {
		if f.Owner == owner {
			t.Fixtures++
		}
	}
	for _, o := range s.Economy.Orders {
		if o.Actor == owner {
			t.Orders = append(t.Orders, OrderRow{Side: o.Side.String(), Qty: o.Qty, Item: o.Item.String(),
				Price: int64(o.Price), X: o.Depot.X, Y: o.Depot.Y})
		}
	}
	if actor != 0 {
		for _, p := range s.Economy.Plans {
			if p.Actor == actor {
				line := p.Summary
				if p.Waiting {
					line += " (waiting on inputs)"
				}
				t.Plans = append(t.Plans, line)
			}
		}
	}
	return t
}
