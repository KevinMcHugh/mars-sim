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

// ---- What a colonist's time is worth -------------------------------------------------

// earnSmoothing is how far one plan's rate pulls a colonist's remembered rate
// at that work: one part in earnSmoothing.
const earnSmoothing = 4

// noteEarnings folds a rate e just earned at work in skill k (thousandths of a
// dollar per 100 ticks) into what it remembers earning there.
func (w *World) noteEarnings(e *Entity, k SkillKind, milli int64) {
	if k >= numSkills {
		return
	}
	if e.earnedTick[k] == 0 {
		e.earned[k] = milli
	} else {
		e.earned[k] += (milli - e.earned[k]) / earnSmoothing
	}
	e.earnedTick[k] = max(1, w.tick)
}

// reservation is what e reckons 100 ticks of its time are worth, in
// thousandths of a dollar: labor-price, or what it has been earning at its
// best-paying work if that's more. What it earned fades back to labor-price
// over rate-memory ticks without earning there again, so a smith with no
// smithing to do goes back to reckoning its time like anyone's. A plan has to
// pay e at least this for its time, so a colonist that earns well at its
// trade passes over work that pays less, until the price of that work rises
// past it.
func (w *World) reservation(e *Entity) int64 {
	base := w.cfg.LaborPrice * 1000
	best, mem := base, int64(w.cfg.RateMemory)
	if mem <= 0 {
		return base
	}
	for k := range e.earned {
		if e.earnedTick[k] == 0 || e.earned[k] <= base {
			continue
		}
		age := int64(w.tick - e.earnedTick[k])
		if age >= mem {
			continue
		}
		best = max(best, base+(e.earned[k]-base)*(mem-age)/mem)
	}
	return best
}

// laborCostFor is what ticks of e's own time are worth to it, at its
// reservation, in whole dollars rounded up.
func (w *World) laborCostFor(e *Entity, ticks int) Money {
	r := w.reservation(e)
	if ticks <= 0 || r <= 0 {
		return 0
	}
	return Money((int64(ticks)*r + 99_999) / 100_000)
}

// ownWorkTicks is how long e takes over work of base ticks in skill k, by its
// rank: the part of a plan skill speeds up. Walking is the same for everyone.
func (w *World) ownWorkTicks(e *Entity, k SkillKind, base int) int {
	pct := w.skillEffect(e, k).TicksPct
	if pct == 100 {
		return base
	}
	return atLeast1((base*pct + 50) / 100)
}
