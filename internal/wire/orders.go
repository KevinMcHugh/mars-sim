package wire

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The market's per-order pages (docs/order-detail.md): "book:<x>,<y>:<item>"
// lists the open orders in one book, and "order:<id>" is one order's detail.
// Like account:<key>, they take a parameter, so paramTopic resolves them.

// ---- book -------------------------------------------------------------------

// BookTopic is the open orders in one (item, depot) book, each side in the
// order it matches: bids dearest first, asks cheapest first, then oldest.
type BookTopic struct {
	Item  string      `json:"item"`
	X     int         `json:"x"`
	Y     int         `json:"y"`
	Depot string      `json:"depot"`
	Bids  []BookOrder `json:"bids"`
	Asks  []BookOrder `json:"asks"`
}

// BookOrder is one open order in a book. ID is its order:<id> topic.
type BookOrder struct {
	ID       uint64 `json:"id"`
	Owner    string `json:"owner"`
	OwnerKey string `json:"ownerKey"`
	Qty      int    `json:"qty"` // still open
	Price    int64  `json:"price"`
	Filled   int    `json:"filled"`
}

// bookParam resolves the "<x>,<y>:<item>" after "book:".
func bookParam(arg string) (topic, bool) {
	at, name, ok := strings.Cut(arg, ":")
	if !ok {
		return topic{}, false
	}
	pos, ok := parsePoint(at)
	item, known := sim.ParseItemKind(name)
	if !ok || !known {
		return topic{}, false
	}
	return topic{every: boardEvery, build: func(s *sim.Snapshot) any { return bookTopic(s, item, pos) }}, true
}

func bookTopic(s *sim.Snapshot, item sim.ItemKind, depot sim.Point) BookTopic {
	t := BookTopic{Item: item.String(), X: depot.X, Y: depot.Y, Depot: depotLabel(s, depot), Bids: []BookOrder{}, Asks: []BookOrder{}}
	var bids, asks []sim.OrderView
	for _, o := range s.Economy.Orders {
		if o.Item != item || o.Depot != depot {
			continue
		}
		if o.Side == sim.Bid {
			bids = append(bids, o)
		} else {
			asks = append(asks, o)
		}
	}
	// The book's own priority (market.go's before): price, then the older
	// order, which is the lower ID.
	slices.SortStableFunc(bids, func(a, b sim.OrderView) int { return cmp.Or(cmp.Compare(b.Price, a.Price), cmp.Compare(a.ID, b.ID)) })
	slices.SortStableFunc(asks, func(a, b sim.OrderView) int { return cmp.Or(cmp.Compare(a.Price, b.Price), cmp.Compare(a.ID, b.ID)) })
	row := func(o sim.OrderView) BookOrder {
		return BookOrder{ID: uint64(o.ID), Owner: ownerLabel(s, o.Actor), OwnerKey: ownerKey(o.Actor),
			Qty: o.Qty, Price: int64(o.Price), Filled: o.Filled}
	}
	for _, o := range bids {
		t.Bids = append(t.Bids, row(o))
	}
	for _, o := range asks {
		t.Asks = append(t.Asks, row(o))
	}
	return t
}

// ---- order ------------------------------------------------------------------

// OrderTopic is one open order's detail. Found is false once it has left the
// book: filled, withdrawn, expired, or repriced (which posts a new order).
type OrderTopic struct {
	Found    bool   `json:"found"`
	ID       uint64 `json:"id"`
	Side     string `json:"side"`
	Item     string `json:"item"`
	Owner    string `json:"owner"`
	OwnerKey string `json:"ownerKey"` // its account:<key> topic
	Manual   bool   `json:"manual"`
	// Price is per unit. Qty is what it was posted for, Filled how much of
	// that has traded and Open what is left; Total is Qty × Price, what
	// the whole order would come to at its limit.
	Price  int64 `json:"price"`
	Qty    int   `json:"qty"`
	Filled int   `json:"filled"`
	Open   int   `json:"open"`
	Total  int64 `json:"total"`
	// Escrow is the money a bid still holds (Open × Price); an ask holds
	// its Open goods instead, and Escrow is 0.
	Escrow int64  `json:"escrow"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Depot  string `json:"depot"`
	// Posted is the tick it opened, and PostedDay and PostedMinute that
	// tick as the top bar's clock reads it. OpenMinutes is how long it has
	// been open, in colony minutes (a colony day is 1440 of them however
	// many ticks it lasts), and ExpiresMinutes how long it has left, 0 for
	// an order that never expires.
	Posted         int `json:"posted"`
	PostedDay      int `json:"postedDay"`
	PostedMinute   int `json:"postedMinute"`
	OpenMinutes    int `json:"openMinutes"`
	ExpiresMinutes int `json:"expiresMinutes,omitempty"`
	// Fills is who it has traded with, in the order they first did; Paid
	// sums their totals (what a bid has spent, or an ask has earned).
	Fills []OrderFill `json:"fills"`
	Paid  int64       `json:"paid"`
}

// OrderFill is what one order has traded with one counterparty.
type OrderFill struct {
	Key   string `json:"key"` // the counterparty's account:<key> topic
	Label string `json:"label"`
	Qty   int    `json:"qty"`
	Total int64  `json:"total"`
}

// orderParam resolves the id after "order:".
func orderParam(arg string) (topic, bool) {
	id, err := strconv.ParseUint(arg, 10, 64)
	if err != nil {
		return topic{}, false
	}
	return topic{every: boardEvery, build: func(s *sim.Snapshot) any { return orderTopic(s, sim.OrderID(id)) }}, true
}

func orderTopic(s *sim.Snapshot, id sim.OrderID) OrderTopic {
	t := OrderTopic{ID: uint64(id), Fills: []OrderFill{}}
	i := slices.IndexFunc(s.Economy.Orders, func(o sim.OrderView) bool { return o.ID == id })
	if i < 0 {
		return t
	}
	o := s.Economy.Orders[i]
	tpd := max(s.TicksPerDay, 1)
	minutes := func(ticks int) int { return ticks * (24 * 60) / tpd }
	t.Found, t.Side, t.Item, t.Manual = true, o.Side.String(), o.Item.String(), o.Manual
	t.Owner, t.OwnerKey = ownerLabel(s, o.Actor), ownerKey(o.Actor)
	t.Price, t.Filled, t.Open = int64(o.Price), o.Filled, o.Qty
	t.Qty = o.Qty + o.Filled
	t.Total = int64(t.Qty) * t.Price
	if o.Side == sim.Bid {
		t.Escrow = int64(o.Escrow)
	}
	t.X, t.Y, t.Depot = o.Depot.X, o.Depot.Y, depotLabel(s, o.Depot)
	t.Posted, t.PostedDay, t.PostedMinute = o.Posted, sim.DayOf(o.Posted, tpd), sim.MinuteOfDay(o.Posted, tpd)
	t.OpenMinutes = minutes(max(s.Tick-o.Posted, 0))
	if o.Expires > 0 {
		t.ExpiresMinutes = minutes(max(o.Expires-s.Tick, 0))
	}
	for _, f := range o.Fills {
		t.Fills = append(t.Fills, OrderFill{Key: ownerKey(f.With), Label: ownerLabel(s, f.With), Qty: f.Qty, Total: int64(f.Total)})
		t.Paid += int64(f.Total)
	}
	return t
}

// ---- helpers ----------------------------------------------------------------

// ownerKey is an owner's account:<key>: "colony" or a colonist's id.
func ownerKey(o sim.Owner) string {
	if o.Kind == sim.OwnerCommunity {
		return "colony"
	}
	return strconv.FormatUint(uint64(o.ID), 10)
}

// depotLabel names the container at p as the order desk does: the silo, or
// what storageLabel calls it.
func depotLabel(s *sim.Snapshot, p sim.Point) string {
	if s.Economy.HasSilo && p == s.Economy.Silo {
		return "silo"
	}
	for _, st := range s.Storages {
		if st.Pos == p {
			return storageLabel(s, st)
		}
	}
	return "depot"
}

// parsePoint reads "<x>,<y>".
// The browser shows the landing level only (see docs/stairs.md), so a point
// it names is on the landing level.
func parsePoint(arg string) (sim.Point, bool) {
	xs, ys, ok := strings.Cut(arg, ",")
	x, errX := strconv.Atoi(xs)
	y, errY := strconv.Atoi(ys)
	if !ok || errX != nil || errY != nil {
		return sim.Point{}, false
	}
	return sim.Point{X: x, Y: y, Level: sim.LandingLevel}, true
}
