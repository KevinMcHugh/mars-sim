package sim

import (
	"fmt"
	"io"
	"strings"
)

// ---- Economy trace -------------------------------------------------------------
//
// The tuning harness for valuation: it runs a world synchronously for a fixed
// number of ticks and writes one CSV row every so often — money, starvation,
// production plans and how deep their chains reach, and each traded good's
// smoothed price and volume. Synchronous stepping (no engine, no clock) makes
// a trace a pure function of its settings, so two seeds, or two settings on
// one seed, compare row for row. `mars-sim -econ-trace` drives it. See
// docs/valuation.md.

// tracedGoods are the goods a trace follows, in column order.
var tracedGoods = [...]ItemKind{Meal, CaveScum, IronOre, WaterIce, UraniumOre, Clay}

// TraceEconomy runs a world built from cfg for ticks ticks and writes a CSV
// trace to out: a header, then a row every every ticks and one for the last.
func TraceEconomy(cfg Config, ticks, every int, out io.Writer) error {
	w := NewEngine(cfg).world // built exactly as a game would be, but never run
	header := []string{"seed", "tick", "colonists", "starved", "treasury", "escrowed", "wallets", "plans", "chain_depth"}
	for _, k := range tracedGoods {
		name := strings.ReplaceAll(k.String(), " ", "_")
		header = append(header, "price_"+name, "volume_"+name)
	}
	if _, err := fmt.Fprintln(out, strings.Join(header, ",")); err != nil {
		return err
	}
	every = max(1, every)
	for i := 1; i <= ticks; i++ {
		w.step()
		if i%every != 0 && i != ticks {
			continue
		}
		if _, err := fmt.Fprintln(out, w.traceRow()); err != nil {
			return err
		}
	}
	return nil
}

// traceRow is one CSV row of the economy trace.
func (w *World) traceRow() string {
	var wallets Money
	for _, e := range w.entities {
		if e.Kind == Colonist {
			wallets += e.wallet
		}
	}
	cols := []string{
		fmt.Sprint(w.cfg.Seed), fmt.Sprint(w.tick), fmt.Sprint(w.countKind(Colonist)), fmt.Sprint(w.starved),
		fmt.Sprint(int64(w.treasury)), fmt.Sprint(int64(w.moneyEscrowed())), fmt.Sprint(int64(wallets)),
		fmt.Sprint(len(w.plans)), fmt.Sprint(w.chainDepth()),
	}
	for _, k := range tracedGoods {
		cols = append(cols, fmt.Sprint(int64(w.valueOf(k))), fmt.Sprint(w.volumeOf(k)))
	}
	return strings.Join(cols, ",")
}

// volumeOf is how many units of k have ever traded, at every depot.
func (w *World) volumeOf(k ItemKind) int {
	n := 0
	for key, b := range w.books {
		if key.Item == k {
			n += b.volume
		}
	}
	return n
}
