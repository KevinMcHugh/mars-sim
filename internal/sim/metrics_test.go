package sim

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
)

// series finds a series by key in a view, or nil.
func series(v *MetricsView, key string) *SeriesView {
	for i := range v.Series {
		if v.Series[i].Key == key {
			return &v.Series[i]
		}
	}
	return nil
}

// cloneView deep-copies a view, to check later sampling never writes one.
func cloneView(v *MetricsView) *MetricsView {
	c := *v
	c.Ticks, c.Hours = slices.Clone(v.Ticks), slices.Clone(v.Hours)
	c.Series = slices.Clone(v.Series)
	for i := range c.Series {
		c.Series[i].Values = slices.Clone(v.Series[i].Values)
	}
	return &c
}

// Samples fall on the first tick of a clock hour, on the current interval,
// and the history halves to span the whole game. A Total survives halving
// whole: each kept reading is still the running total at its tick. A
// published view is never written by later sampling.
func TestMetricsSampleHourlyAndSpanTheGame(t *testing.T) {
	w := propertyWorld(t)
	w.spawn(Colonist, Point{8, 8})
	tpd := w.cfg.TicksPerDay()
	var published, frozen *MetricsView
	const end = 120000
	for tick := 1; tick <= end; tick++ {
		w.tick = tick
		w.moneyMoved++ // a running total equal to the tick
		w.sampleMetrics()
		if tick == 30000 {
			published = w.snapshot(false, 0).Metrics
			frozen = cloneView(published)
		}
	}
	v := w.metricsView()
	if n := len(v.Ticks); n == 0 || n > metricHistory {
		t.Fatalf("history holds %d samples, want 1..%d", n, metricHistory)
	}
	if v.Every != 12 {
		t.Fatalf("interval %dh after %d ticks, want 12 (1, 2, 4, then 12)", v.Every, end)
	}
	for i, tick := range v.Ticks {
		h := v.Hours[i]
		if h%v.Every != 0 || clockHour(tick, tpd) != h || clockHour(tick-1, tpd) == h {
			t.Fatalf("sample %d at tick %d (hour %d) is not the first tick of an hour on the %dh grid", i, tick, h, v.Every)
		}
		if i > 0 && h-v.Hours[i-1] != v.Every {
			t.Fatalf("samples %d and %d are %dh apart, want %d", i-1, i, h-v.Hours[i-1], v.Every)
		}
	}
	if last := v.Ticks[len(v.Ticks)-1]; end-last > v.Every*tpd/24+1 {
		t.Fatalf("the last sample is at tick %d of %d: the history does not reach now", last, end)
	}
	moved := series(v, "money-moved/")
	if moved == nil || moved.Start != 0 || len(moved.Values) != len(v.Ticks) {
		t.Fatalf("money-moved series %+v does not cover the history", moved)
	}
	for i, x := range moved.Values {
		if x != int64(v.Ticks[i]) {
			t.Fatalf("money-moved reads %d at tick %d: a running total lost its place in halving", x, v.Ticks[i])
		}
	}
	if !reflect.DeepEqual(published, frozen) {
		t.Fatal("a published metrics view was changed by later sampling")
	}
}

// Every colonist's wallet and the treasury get a balance series; a dead
// colonist's ends and keeps its history. A good gets a price series only once
// it has traded, and a stock series once some is stored.
func TestMetricSeriesComeAndGoWithTheirSubjects(t *testing.T) {
	w, silo, cs := marketWorld(t)
	sample := func() *MetricsView {
		t.Helper()
		tpd := w.cfg.TicksPerDay()
		for w.tick++; clockHour(w.tick, tpd) == clockHour(w.tick-1, tpd); w.tick++ {
		}
		w.sampleMetrics()
		return w.metricsView()
	}
	v := sample()
	if s := series(v, "balance/treasury"); s == nil || s.Subject != "Treasury" || s.Values[0] != int64(w.treasury) {
		t.Fatalf("treasury series %+v", s)
	}
	key := "balance/c" + strconv.Itoa(int(cs[0].ID))
	if s := series(v, key); s == nil || s.Values[0] != 100 || s.Subject != cs[0].displayName() {
		t.Fatalf("colonist balance series %+v", s)
	}
	if series(v, "price/iron-ore") != nil || series(v, "stock/meal") != nil {
		t.Fatal("a good nobody has traded or stored already has a series")
	}

	stock(w, silo, ColonistOwner(cs[0].ID), IronOre, 2)
	w.post(Ask, IronOre, 2, 4, ColonistOwner(cs[0].ID), silo, 0)
	w.post(Bid, IronOre, 2, 4, ColonistOwner(cs[1].ID), silo, 0)
	w.remove(cs[0].ID, "test")
	v = sample()
	if s := series(v, "price/iron-ore"); s == nil || s.Values[len(s.Values)-1] != 4 {
		t.Fatalf("price series after a trade: %+v", s)
	}
	if s := series(v, "stock/iron-ore"); s == nil || s.Values[len(s.Values)-1] != 2 {
		t.Fatalf("stock series after storing ore: %+v", s)
	}
	if s := series(v, "traded/iron-ore"); s == nil || s.Values[len(s.Values)-1] != 2 {
		t.Fatalf("traded series after a trade: %+v", s)
	}
	if s := series(v, "turnover/all"); s == nil || s.Values[len(s.Values)-1] != 8 {
		t.Fatalf("all-goods turnover after an $8 trade: %+v", s)
	}
	if s := series(v, "bids-posted/iron-ore"); s == nil || s.Values[len(s.Values)-1] != 1 {
		t.Fatalf("bids posted: %+v", s)
	}
	s := series(v, key)
	if s == nil || !s.Ended || len(s.Values) != 1 {
		t.Fatalf("a dead colonist's balance series %+v, want ended with its one reading", s)
	}
	if n := len(series(sample(), key).Values); n != 1 {
		t.Fatalf("an ended series grew to %d readings", n)
	}
}

// Money changes hands when it is paid from one party to another: a trade's
// price, but not the bid's escrow going in, nor what the bid gets back.
func TestMoneyMovedCountsPaymentsNotEscrow(t *testing.T) {
	w, silo, cs := marketWorld(t)
	seller, buyer := ColonistOwner(cs[0].ID), ColonistOwner(cs[1].ID)
	stock(w, silo, seller, Meal, 3)
	w.post(Ask, Meal, 3, 5, seller, silo, 0)
	w.post(Bid, Meal, 2, 7, buyer, silo, 0) // escrows $14, pays $10, gets $4 back
	if w.moneyMoved != 10 || w.payments != 1 {
		t.Fatalf("moved $%d in %d payments, want $10 in 1", w.moneyMoved, w.payments)
	}
	rest, _ := w.post(Bid, Meal, 5, 3, buyer, silo, 0) // rests below the ask
	w.cancel(rest)
	if w.moneyMoved != 10 {
		t.Fatalf("a bid resting and cancelled moved $%d", w.moneyMoved-10)
	}
	w.transfer(buyer, Community, 6)
	if w.moneyMoved != 16 || w.payments != 2 {
		t.Fatalf("a direct payment: moved $%d in %d payments, want $16 in 2", w.moneyMoved, w.payments)
	}
}
