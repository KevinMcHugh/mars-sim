package sim

// ---- Valuation -------------------------------------------------------------------
//
// The order book only moves if colonists have prices in their heads. Each item
// has a smoothed trade price, remembered from every fill; until an item has
// traded, its value is the colony charter's reference price. A hungry
// colonist bids for a meal in proportion to how hungry it is, capped by what
// it can spend, and a producer reckons its own time at labor-price. All of it
// is integer arithmetic on state the world already holds. See
// docs/valuation.md.

// priceSmoothing is how far a trade pulls an item's remembered price: one
// part in priceSmoothing. Eight parts keeps one freak trade from resetting a
// market while letting a sustained move show within a few dozen fills.
const priceSmoothing = 8

// priceMemory is an item's smoothed trade price, in thousandths of a dollar
// so the smoothing does not round away small moves.
type priceMemory struct {
	milli  int64
	traded bool
}

// recordPrice folds a fill at price into item's remembered price.
func (w *World) recordPrice(item ItemKind, price Money) {
	m := &w.prices[item]
	p := int64(price) * 1000
	if !m.traded {
		m.milli, m.traded = p, true
		return
	}
	m.milli += (p - m.milli) / priceSmoothing
}

// referenceValue is what an item is worth before it has ever traded: the
// charter's price, or for biomatter what the colony pays for it at its
// scumhouses.
func (w *World) referenceValue(k ItemKind) Money {
	if p := w.refPrice(k); p > 0 {
		return p
	}
	return w.biomatterPrice(k)
}

// valueOf is what an item is worth now: its smoothed trade price once it has
// traded, its reference value before.
func (w *World) valueOf(k ItemKind) Money {
	if m := w.prices[k]; m.traded {
		return Money((m.milli + 500) / 1000)
	}
	return w.referenceValue(k)
}

// laborCost is what ticks of a colonist's own work are worth to it.
func (w *World) laborCost(ticks int) Money {
	if ticks <= 0 || w.cfg.LaborPrice <= 0 {
		return 0
	}
	return Money((int64(ticks)*w.cfg.LaborPrice + 99) / 100)
}

// mealBidLimit is the most e will pay for a meal now. It starts at the meal's
// value and rises with hunger, to meal-willingness times the value when
// starving. A colonist spends at most half its money on one meal until hunger
// is critical, and then everything it has: a dollar matters more to someone
// who has few of them, until nothing matters more than eating.
func (w *World) mealBidLimit(e *Entity) Money {
	base := w.valueOf(Meal)
	if base <= 0 {
		return 0
	}
	spec := w.cfg.Needs[NeedFood]
	level, top := w.needLevel(e, NeedFood), max(1, spec.Max)
	willing := int64(max(1, w.cfg.MealWillingness))
	limit := Money(int64(base) * (100 + (willing-1)*100*int64(min(level, top))/int64(top)) / 100)
	critical := level*10 >= top*9
	if !critical {
		limit = min(limit, e.wallet/2)
	}
	return min(limit, e.wallet)
}
