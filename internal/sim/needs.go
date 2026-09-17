package sim

// NeedKind enumerates the drives a colonist must satisfy. Adding a need is meant
// to be a table edit: append a kind here and add its NeedSpec in Config. Most
// needs have a Facility to satisfy them; social is satisfied by conversation.
type NeedKind uint8

const (
	NeedFood NeedKind = iota
	NeedBladder
	NeedSocial
	NeedSleep

	numNeeds // keep last: the count of needs
)

func (n NeedKind) String() string {
	switch n {
	case NeedFood:
		return "food"
	case NeedBladder:
		return "bladder"
	case NeedSocial:
		return "social"
	case NeedSleep:
		return "sleep"
	default:
		return "need"
	}
}

// NeedSpec describes how one need behaves. Levels run 0..Max; 0 means satisfied.
// The cfg tags make the numeric fields tunable from the config file and the
// command line, one knob per need (see configfile.go). Name and Facility are
// deliberately untagged: they are the need's identity and its plumbing, not
// balance, and changing them from a file would let a config rename a need out
// from under the code that looks it up.
type NeedSpec struct {
	Name     string
	Rise     int     `cfg:"rise" doc:"level gained per tick"`
	SeekAt   int     `cfg:"seek-at" doc:"level at which the colonist drops work to satisfy it"`
	Max      int     `cfg:"max" doc:"ceiling; a fatal need sitting here drains HP"`
	Facility Terrain // structure that resets this need to 0
	UseTicks int     `cfg:"use-ticks" doc:"ticks spent using the facility"`
	Fatal    bool    `cfg:"fatal" doc:"whether sitting at the ceiling damages the colonist"`
	// GrabTicks, if positive and less than UseTicks, makes this need portable:
	// a colonist spends only GrabTicks at the facility, then carries it away
	// and spends the rest of UseTicks finishing elsewhere, freeing the
	// facility's access tile for the next colonist immediately rather than
	// occupying it for the whole UseTicks. Zero means the need can only be
	// satisfied in place (bladder, sleep — there is nothing to take away).
	GrabTicks int `cfg:"grab-ticks" doc:"ticks at the facility before carrying the rest away (0 = must be used in place)"`
}

// needLevel returns an entity's current level for one need, computed lazily
// from its stored base and the elapsed ticks, clamped to [0, Max]. Mice share
// the food need with colonists but hunger at their own faster rate.
func (w *World) needLevel(e *Entity, i NeedKind) int {
	spec := w.cfg.Needs[i]
	// needRise is the entity's per-need rate: colonists' is trait-scaled and mice
	// hunger fast (see personality.go and newEntity).
	lvl := e.Needs[i] + e.needRise[i]*(w.tick-e.needSince[i])
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
	if damage := e.starvationDamage[i]; damage > 0 {
		e.HP = min(e.MaxHP, e.HP+damage)
		e.starvationDamage[i] = 0
	}
}

// applyStarvation drains HP for any fatal need currently sitting at its max.
// It reads levels lazily, so it is correct even for a colonist that has been
// resting for many ticks.
func (w *World) applyStarvation(e *Entity) {
	for i := 0; i < int(numNeeds); i++ {
		spec := w.cfg.Needs[i]
		if spec.Fatal && w.needLevel(e, NeedKind(i)) >= spec.Max {
			if e.Job == JobUse && e.Need == NeedKind(i) {
				// A colonist that has already grabbed a portable need (see
				// NeedSpec.GrabTicks) is guaranteed to finish regardless of the
				// facility's reachability or crowding — it is no longer there.
				if e.carrying {
					continue
				}
				// Reaching food does not reset the need until UseTicks elapse. Give
				// an entity committed to a reachable source enough grace to traverse
				// its queue and finish eating rather than dying mid-meal.
				if field := w.facilityField(spec.Facility); field != nil && field.at(e.Pos) >= 0 {
					continue
				}
			}
			// The same grace applies while reachable life support is under
			// construction. This is especially important at startup, when staggered
			// hunger can reach Max shortly before the first facility room completes.
			if e.Kind == Colonist && w.reachableFacilityConstruction(e.Pos, spec.Facility) {
				continue
			}
			e.HP -= w.cfg.StarveDamage
			e.starvationDamage[i] += w.cfg.StarveDamage
		}
	}
}

// mostUrgentNeed returns the need a colonist should address first, if any is past
// its seek threshold. A fatal need (starvation) outranks any non-fatal one that
// is also urgent — otherwise a non-fatal need like bladder, which rises faster
// and caps further past its threshold, would permanently outrank food and let
// the colonist starve. Within the same fatal-ness, the need furthest past its
// threshold wins.
func (w *World) mostUrgentNeed(e *Entity) (NeedKind, bool) {
	worst := NeedKind(0)
	worstOver := -1
	worstFatal := false
	found := false
	for i := 0; i < int(numNeeds); i++ {
		spec := w.cfg.Needs[i]
		over := w.needLevel(e, NeedKind(i)) - spec.SeekAt
		if over < 0 {
			continue
		}
		better := !found ||
			(spec.Fatal && !worstFatal) ||
			(spec.Fatal == worstFatal && over > worstOver)
		if better {
			worst, worstOver, worstFatal, found = NeedKind(i), over, spec.Fatal, true
		}
	}
	return worst, found
}

// useState maps a need to the display state shown while fulfilling it.
func useState(n NeedKind) State {
	switch n {
	case NeedFood:
		return Eating
	case NeedBladder:
		return Relieving
	case NeedSleep:
		return Sleeping
	default:
		return Idle
	}
}
