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

// applyNeeds raises every need for a colonist and applies the consequence of any
// need that has maxed out (currently only starvation damages HP).
func (w *World) applyNeeds(e *Entity) {
	for i := 0; i < int(numNeeds); i++ {
		spec := w.cfg.Needs[i]
		e.Needs[i] += spec.Rise
		if e.Needs[i] > spec.Max {
			e.Needs[i] = spec.Max
		}
		if spec.Fatal && e.Needs[i] >= spec.Max {
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
		over := e.Needs[i] - w.cfg.Needs[i].SeekAt
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
