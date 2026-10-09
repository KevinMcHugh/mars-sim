package wire

import (
	"cmp"
	"math"
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
	Level   int     `json:"level"`
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
			task := Task{X: tk.Pos.X, Y: tk.Pos.Y, Level: int(tk.Pos.Level), Terrain: tk.Terrain.String(), Phase: tk.Phase, Done: tk.Done}
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
		"meeting hall": s.PendingHalls, "incubator": s.PendingIncubators,
		"stair down": s.PendingStairs, "shaft level": s.PendingShaftLevels,
		"ladder": s.PendingLadders,
	} {
		if n > 0 {
			t.Pending[what] = n // a JSON object: encoding/json sorts its keys
		}
	}
	return t
}

// ---- storage --------------------------------------------------------------

// StorageRow is one container in the storage list. Its contents and ledger
// are the tile inspector's (tile:<x>,<y>,<level>), which the page opens on a click;
// the totals and ledger here are what the tab's pool and searches add up.
type StorageRow struct {
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Level    int    `json:"level"`
	Label    string `json:"label"`
	Used     int    `json:"used"`
	Slots    int    `json:"slots"`
	Items    int    `json:"items"`
	Capacity int    `json:"capacity"`
	// Top is what it holds most of, "meal ×9", for a glance down the list.
	Top string `json:"top,omitempty"`
	// Contents totals its stacks by item, in item order.
	Contents []Holding `json:"contents"`
	// Ledger is whose they are, sorted by owner then item (the snapshot's).
	Ledger []LedgerItem `json:"ledger"`
}

func storageTopic(s *sim.Snapshot) []StorageRow {
	rows := make([]StorageRow, 0, len(s.Storages))
	for _, st := range s.Storages {
		info := storageInfo(s, st)
		r := StorageRow{X: st.Pos.X, Y: st.Pos.Y, Level: int(st.Pos.Level), Label: info.Label, Used: info.Used, Slots: info.Slots,
			Items: info.Items, Capacity: info.Capacity, Contents: []Holding{}, Ledger: info.Ledger}
		totals := map[sim.ItemKind]int{}
		for _, stack := range st.Inventory {
			if stack.Count > 0 {
				totals[stack.Kind] += stack.Count
			}
		}
		kinds := make([]sim.ItemKind, 0, len(totals))
		for k := range totals {
			kinds = append(kinds, k)
		}
		slices.Sort(kinds) // item order, not the map's
		var best Holding
		for _, k := range kinds {
			h := Holding{Item: k.String(), Count: totals[k]}
			r.Contents = append(r.Contents, h)
			if best.Item == "" || h.Count > best.Count || h.Count == best.Count && h.Item < best.Item {
				best = h
			}
		}
		if best.Item != "" {
			r.Top = best.Item + " ×" + strconv.Itoa(best.Count)
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
	// Dig is what the dig tool needs to price an excavation order before it
	// is sent: the wage a tile pays and the most tiles one order may cover.
	Dig DigTerms `json:"dig"`
	// Digs are the excavation orders still open, oldest first.
	Digs []Dig `json:"digs"`
	// Colony is what the order desk needs to place, reprice and withdraw
	// the colony's orders (docs/colony-orders.md).
	Colony ColonyDesk `json:"colony"`
}

// ColonyDesk is the colony's side of the book: its open orders, the depots
// it may trade at, and the goods it may name.
type ColonyDesk struct {
	// Orders is every open order in the colony's name, oldest first.
	Orders []ColonyOrder `json:"orders"`
	// Depots is every communal container, the silo first, then by position.
	Depots []Depot `json:"depots"`
	// Items is every good an order may name, in item order.
	Items []string `json:"items"`
	// Suspended is every standing order a player has stopped, by side and
	// item, bids first.
	Suspended []Suspended `json:"suspended"`
	// Wide is every colony-wide order, bids first, then in item order.
	Wide []WideOrder `json:"wide"`
}

// WideOrder is a colony-wide order: a standing order by side and item, with
// no depot, that the colony keeps on the book where the good changes hands.
// Open is how many units its orders have open now, at Depots depots.
type WideOrder struct {
	Side   string `json:"side"`
	Item   string `json:"item"`
	Qty    int    `json:"qty"`
	Price  int64  `json:"price"`
	Open   int    `json:"open"`
	Depots int    `json:"depots"`
}

// Suspended is a side and an item whose standing orders the colony has
// stopped posting until a player resumes them.
type Suspended struct {
	Side string `json:"side"`
	Item string `json:"item"`
}

// ColonyOrder is one of the colony's open orders. ID is what a reprice or a
// cancel names. Manual is set for one a player placed or repriced; the
// colony's own standing orders are topped up again if withdrawn.
type ColonyOrder struct {
	ID     uint64 `json:"id"`
	Side   string `json:"side"`
	Item   string `json:"item"`
	Qty    int    `json:"qty"`
	Price  int64  `json:"price"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Posted int    `json:"posted"`
	Manual bool   `json:"manual"`
	// Wide is set for an order a colony-wide order placed.
	Wide bool `json:"wide"`
}

// Depot is a communal container the colony may trade at, and what the colony
// holds there (on its own ledger line, so free to offer).
type Depot struct {
	X        int       `json:"x"`
	Y        int       `json:"y"`
	Label    string    `json:"label"`
	Silo     bool      `json:"silo"`
	Holdings []Holding `json:"holdings"`
}

// Dig is one open excavation order: its project id (what a cancel names), the
// rectangle that bounds its tiles, how many are dug, and what its open work
// orders hold.
type Dig struct {
	ID    int   `json:"id"`
	X0    int   `json:"x0"`
	Y0    int   `json:"y0"`
	X1    int   `json:"x1"`
	Y1    int   `json:"y1"`
	Level int   `json:"level"` // an order covers one level
	Tiles int   `json:"tiles"`
	Done  int   `json:"done"`
	Held  int64 `json:"held"`
}

// DigTerms are the terms of an excavation order (docs/excavation.md).
type DigTerms struct {
	Wage     int64 `json:"wage"`
	MaxTiles int   `json:"maxTiles"`
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
	Exported    int64 `json:"exported"` // paid off-world: recruiting (docs/recruiting.md)
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
			Escrowed: int64(econ.Escrowed), Frozen: int64(econ.Frozen), Exported: int64(econ.Exported), Issued: int64(econ.Issued), Starved: econ.Starved},
		Books:      []Book{},
		Prices:     make([]Price, 0, len(econ.Prices)),
		Plans:      make([]PlanLine, 0, len(econ.Plans)),
		ChainDepth: econ.ChainDepth,
		Work:       []WorkLine{},
		Trades:     []TradeLine{},
		Dig:        DigTerms{Wage: int64(econ.DigWage), MaxTiles: econ.DigMax},
		Digs:       []Dig{},
		Colony:     colonyDesk(s),
	}
	for _, p := range s.Projects {
		if p.Name != sim.ExcavationName {
			continue
		}
		d := Dig{ID: p.ID, X0: math.MaxInt, Y0: math.MaxInt, Tiles: len(p.Tasks), Done: p.TasksDone()}
		for _, tk := range p.Tasks {
			d.X0, d.Y0 = min(d.X0, tk.Pos.X), min(d.Y0, tk.Pos.Y)
			d.X1, d.Y1 = max(d.X1, tk.Pos.X), max(d.Y1, tk.Pos.Y)
			d.Level = int(tk.Pos.Level)
		}
		for _, o := range econ.WorkOrders {
			if o.Kind == sim.WorkDig && o.Pos.X >= d.X0 && o.Pos.X <= d.X1 && o.Pos.Y >= d.Y0 && o.Pos.Y <= d.Y1 && digHas(p, o.Pos) {
				d.Held += int64(o.Pay) * int64(o.Units)
			}
		}
		t.Digs = append(t.Digs, d)
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

// OrderRow is one open order. ID is its order:<id> topic.
type OrderRow struct {
	ID    uint64 `json:"id"`
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
			t.Orders = append(t.Orders, OrderRow{ID: uint64(o.ID), Side: o.Side.String(), Qty: o.Qty, Item: o.Item.String(),
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

func colonyDesk(s *sim.Snapshot) ColonyDesk {
	econ := s.Economy
	d := ColonyDesk{Orders: []ColonyOrder{}, Depots: []Depot{}, Items: []string{}, Suspended: []Suspended{},
		Wide: []WideOrder{}}
	for _, su := range econ.Suspended {
		d.Suspended = append(d.Suspended, Suspended{Side: su.Side.String(), Item: su.Item.String()})
	}
	for _, wo := range econ.Wide {
		d.Wide = append(d.Wide, WideOrder{Side: wo.Side.String(), Item: wo.Item.String(), Qty: wo.Qty,
			Price: int64(wo.Price), Open: wo.Open, Depots: wo.Depots})
	}
	for _, o := range econ.Orders {
		if o.Actor == sim.Community {
			d.Orders = append(d.Orders, ColonyOrder{ID: uint64(o.ID), Side: o.Side.String(), Item: o.Item.String(),
				Qty: o.Qty, Price: int64(o.Price), X: o.Depot.X, Y: o.Depot.Y, Posted: o.Posted, Manual: o.Manual, Wide: o.Wide})
		}
	}
	for _, st := range s.Storages {
		if f, ok := s.FixtureAt(st.Pos); ok && f.Access != sim.AccessCommunal {
			continue
		}
		dp := Depot{X: st.Pos.X, Y: st.Pos.Y, Label: storageLabel(s, st), Silo: econ.HasSilo && st.Pos == econ.Silo,
			Holdings: []Holding{}}
		for _, l := range st.Ledger { // sorted by owner then item
			if l.Owner == sim.Community && l.Count > 0 {
				dp.Holdings = append(dp.Holdings, Holding{Item: l.Item.String(), Count: l.Count})
			}
		}
		d.Depots = append(d.Depots, dp)
	}
	slices.SortStableFunc(d.Depots, func(a, b Depot) int {
		if a.Silo != b.Silo {
			if a.Silo {
				return -1
			}
			return 1
		}
		if c := cmp.Compare(a.Y, b.Y); c != 0 {
			return c
		}
		return cmp.Compare(a.X, b.X)
	})
	for _, k := range sim.TradableItems() {
		d.Items = append(d.Items, k.String())
	}
	return d
}

// digHas reports whether a project has a task on pos.
func digHas(p sim.ProjectView, pos sim.Point) bool {
	for _, tk := range p.Tasks {
		if tk.Pos == pos {
			return true
		}
	}
	return false
}
