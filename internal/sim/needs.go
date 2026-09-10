package sim

// NeedKind enumerates the drives a colonist must satisfy. Adding a need is meant
// to be a table edit: append a kind here, add its NeedSpec in Config, and give
// it a Facility to satisfy it — the systems iterate needs generically.
type NeedKind uint8

const (
	NeedFood NeedKind = iota
	NeedBladder

	numNeeds // keep last: the count of needs
)

func (n NeedKind) String() string {
	switch n {
	case NeedFood:
		return "food"
	case NeedBladder:
		return "bladder"
	default:
		return "need"
	}
}

// NeedSpec describes how one need behaves. Levels run 0..Max; 0 means satisfied.
type NeedSpec struct {
	Name     string
	Rise     int     // level gained per tick
	SeekAt   int     // level at which the colonist drops work to satisfy it
	Max      int     // ceiling; a Fatal need at Max drains HP
	Facility Terrain // structure that resets this need to 0
	UseTicks int     // ticks spent using the facility
	Fatal    bool    // whether sitting at Max damages the colonist
}

// needLevel returns a colonist's current level for one need, computed lazily
// from its stored base and the elapsed ticks, clamped to [0, Max].
func (w *World) needLevel(e *Entity, i NeedKind) int {
	spec := w.cfg.Needs[i]
	lvl := e.Needs[i] + spec.Rise*(w.tick-e.needSince[i])
	if lvl > spec.Max {
		lvl = spec.Max
	}
	if lvl < 0 {
		lvl = 0
	}
	return lvl
}

// resetNeed satisfies a need: its base drops to 0 as of the current tick.
func (w *World) resetNeed(e *Entity, i NeedKind) {
	e.Needs[i] = 0
	e.needSince[i] = w.tick
}

// applyStarvation drains HP for any fatal need currently sitting at its max.
// It reads levels lazily, so it is correct even for a colonist that has been
// resting for many ticks.
func (w *World) applyStarvation(e *Entity) {
	for i := 0; i < int(numNeeds); i++ {
		spec := w.cfg.Needs[i]
		if spec.Fatal && w.needLevel(e, NeedKind(i)) >= spec.Max {
			e.HP -= w.cfg.StarveDamage
		}
	}
}

// mostUrgentNeed returns the need furthest past its seek threshold, if any is.
// Ties favor the lower NeedKind (food before bladder).
func (w *World) mostUrgentNeed(e *Entity) (NeedKind, bool) {
	worst := NeedKind(0)
	worstOver := -1
	for i := 0; i < int(numNeeds); i++ {
		over := w.needLevel(e, NeedKind(i)) - w.cfg.Needs[i].SeekAt
		if over >= 0 && over > worstOver {
			worst, worstOver = NeedKind(i), over
		}
	}
	return worst, worstOver >= 0
}

// useState maps a need to the display state shown while fulfilling it.
func useState(n NeedKind) State {
	switch n {
	case NeedFood:
		return Eating
	case NeedBladder:
		return Relieving
	default:
		return Idle
	}
}
