package sim

import (
	"fmt"
	"slices"
)

// Drive consequences: what a drive's level does to the colonist. A drive's
// range is cut into bands at every threshold that matters (SeekAt,
// CriticalAt, and the edges of each consequence), and within a band every
// effect is constant. That keeps drives lazy: nothing about a colonist changes
// between crossings, and the next crossing is a tick that can be computed and
// scheduled. A smooth effect (movement slowing as exhaustion builds) is a
// ramp: a few small steps, expanded into bands when the config is compiled.
// See docs/drives.md.

// ConsequenceKind is what a consequence does while its drive is in range.
type ConsequenceKind uint8

const (
	consequenceNone ConsequenceKind = iota
	// ConsequenceHPDrain takes Value HP a tick (starvation). Satisfying the
	// drive gives that HP back.
	ConsequenceHPDrain
	// ConsequenceRate changes Target's growth rate by Value percent: +30
	// makes it grow at 130%, -50 at half. This is how drives affect one
	// another (exhaustion building faster the hungrier a colonist is).
	ConsequenceRate
)

// DriveConsequence applies Kind with Value while its drive's level is in
// From..To, inclusive.
type DriveConsequence struct {
	From, To int
	Kind     ConsequenceKind
	Target   DriveKind // ConsequenceRate: the drive whose rate changes
	Value    int
}

// DriveRamp is a consequence whose Value moves from FromValue at level From to
// ToValue at level To in Steps even steps, and stays at ToValue above To. It
// expands into Steps+1 DriveConsequences.
type DriveRamp struct {
	From, To           int
	FromValue, ToValue int
	Steps              int
	Kind               ConsequenceKind
	Target             DriveKind
}

// expand turns a ramp into its consequences, through max.
func (r DriveRamp) expand(max int) []DriveConsequence {
	out := make([]DriveConsequence, 0, r.Steps+1)
	for k := 0; k <= r.Steps; k++ {
		lo := r.From + (r.To-r.From)*k/r.Steps
		hi := max
		if k < r.Steps {
			hi = r.From + (r.To-r.From)*(k+1)/r.Steps - 1
		}
		if hi < lo {
			continue
		}
		out = append(out, DriveConsequence{
			From: lo, To: hi, Kind: r.Kind, Target: r.Target,
			Value: r.FromValue + (r.ToValue-r.FromValue)*k/r.Steps,
		})
	}
	return out
}

// driveRateChange is one band's ConsequenceRate on another drive.
type driveRateChange struct {
	target DriveKind
	change int
}

// driveTable is one drive's compiled bands. Band b covers levels from
// edges[b-1] (or below everything, for b = 0) up to but not including
// edges[b] (or everything above, for the last band).
type driveTable struct {
	edges   []int
	phase   []DrivePhase
	hpDrain []int
	rates   [][]driveRateChange
	// drainFrom is the lowest level any band drains HP at, so the per-tick
	// starvation check can skip a drive below it without finding its band.
	drainFrom int
}

// bandOf is the band a whole-point level falls in.
func (t *driveTable) bandOf(level int) int {
	b, found := slices.BinarySearch(t.edges, level)
	if found {
		b++
	}
	return b
}

// compileDrives builds every drive's band table, or reports the first spec
// whose consequences are malformed.
func compileDrives(cfg *Config) ([numDrives]driveTable, error) {
	var out [numDrives]driveTable
	for d := DriveKind(0); d < numDrives; d++ {
		t, err := compileDrive(cfg.Drives[d], cfg.StarveDamage)
		if err != nil {
			return out, fmt.Errorf("drive %s: %w", d, err)
		}
		out[d] = t
	}
	return out, nil
}

// CheckDrives reports whether the config's drive consequences and effects
// compile. main validates a config with it before a game starts.
func (c *Config) CheckDrives() error {
	if _, err := compileDrives(c); err != nil {
		return err
	}
	return checkDriveEffects(c.DriveEffects)
}

func compileDrive(spec DriveSpec, starveDamage int) (driveTable, error) {
	cons := slices.Clone(spec.Consequences)
	for i, r := range spec.Ramps {
		if r.Steps < 1 || r.To <= r.From {
			return driveTable{}, fmt.Errorf("ramp %d: needs at least one step and From below To (got %d steps, %d..%d)",
				i, r.Steps, r.From, r.To)
		}
		cons = append(cons, r.expand(spec.Max)...)
	}
	if spec.Fatal {
		cons = append(cons, DriveConsequence{From: spec.Max, To: spec.Max, Kind: ConsequenceHPDrain, Value: starveDamage})
	}
	edges := []int{1, spec.SeekAt, spec.CriticalAt}
	for i, c := range cons {
		switch {
		case c.To < c.From:
			return driveTable{}, fmt.Errorf("consequence %d: To %d is below From %d", i, c.To, c.From)
		case c.Kind != ConsequenceHPDrain && c.Kind != ConsequenceRate:
			return driveTable{}, fmt.Errorf("consequence %d: unknown kind %d", i, c.Kind)
		case c.Kind == ConsequenceRate && c.Target >= numDrives:
			return driveTable{}, fmt.Errorf("consequence %d: unknown target drive %d", i, c.Target)
		case c.Kind == ConsequenceRate && c.Value < -100:
			return driveTable{}, fmt.Errorf("consequence %d: a rate cannot fall below zero (change %d%%)", i, c.Value)
		}
		edges = append(edges, c.From, c.To+1)
	}
	slices.Sort(edges)
	edges = slices.Compact(edges)
	t := driveTable{
		edges:   edges,
		phase:   make([]DrivePhase, len(edges)+1),
		hpDrain: make([]int, len(edges)+1),
		rates:   make([][]driveRateChange, len(edges)+1),
		// Above any reachable level until a draining band says otherwise.
		drainFrom: spec.Max + 1,
	}
	for b := range t.phase {
		// Every level in a band behaves alike, so its lowest level stands
		// for it (and one below the first edge for the band under it).
		rep := edges[0] - 1
		if b > 0 {
			rep = edges[b-1]
		}
		t.phase[b] = phaseForLevel(rep, spec)
		for _, c := range cons {
			if rep < c.From || rep > c.To {
				continue
			}
			switch c.Kind {
			case ConsequenceHPDrain:
				t.hpDrain[b] += c.Value
				if c.Value > 0 {
					t.drainFrom = min(t.drainFrom, rep)
				}
			case ConsequenceRate:
				t.rates[b] = append(t.rates[b], driveRateChange{target: c.Target, change: c.Value})
			}
		}
	}
	return t, nil
}

// syncDriveBand places drive d's felt level in its band, caches its phase and
// the next tick the current rate carries it across an edge, and passes a band
// change on: a phase change makes the colonist reconsider what it is doing,
// and a change in the band's rate consequences refreshes the drives they act
// on.
func (w *World) syncDriveBand(e *Entity, d DriveKind) {
	t := &w.driveTables[d]
	s := &e.drives[d]
	felt := w.driveFelt(e, d)
	band := t.bandOf(floorDiv(felt, driveUnit))
	old := s.band
	s.band = band
	if phase := t.phase[band]; phase != s.phase {
		s.phase = phase
		w.markMindDirty(e)
	}
	s.nextCrossing = w.nextBandCrossing(e, d)
	// A crossing sooner than the colonist next means to think has to be
	// noticed: the sleeping fast path skips the deadline check until then.
	if s.nextCrossing > 0 && s.nextCrossing < e.nextThinkTick {
		e.nextThinkTick = s.nextCrossing
	}
	if band != old && (len(t.rates[band]) > 0 || len(t.rates[old]) > 0) {
		for target := DriveKind(0); target < numDrives; target++ {
			if bandChangesRate(t.rates[old], target) || bandChangesRate(t.rates[band], target) {
				w.refreshDrive(e, target, 0)
			}
		}
	}
}

func bandChangesRate(changes []driveRateChange, target DriveKind) bool {
	for _, c := range changes {
		if c.target == target {
			return true
		}
	}
	return false
}

// nextBandCrossing is the first tick at which drive d's felt level, growing
// at its current rate, leaves its band: up through the band's top edge, or
// down below its floor. 0 means it never does at this rate (it is steady, or
// the clamp to the drive's range stops it short).
func (w *World) nextBandCrossing(e *Entity, d DriveKind) int {
	spec := &w.cfg.Drives[d]
	t := &w.driveTables[d]
	s := &e.drives[d]
	// Count from the unclamped felt level: masked below the floor, it has
	// that much further to climb before anything changes.
	raw := w.driveTrue(e, d) - s.offset
	// The masking holds the felt level off the clamp the true level hits.
	hi := min(spec.Max*driveUnit-s.offset, spec.Max*driveUnit)
	lo := max(spec.Min*driveUnit-s.offset, spec.Min*driveUnit)
	switch {
	case s.rate > 0 && s.band < len(t.edges):
		edge := t.edges[s.band] * driveUnit
		if edge > hi {
			return 0
		}
		return w.tick + (edge-raw+s.rate-1)/s.rate
	case s.rate < 0 && s.band > 0:
		floor := t.edges[s.band-1]*driveUnit - 1 // the first level below the band
		if floor < lo {
			return 0
		}
		return w.tick + (raw-floor-s.rate-1)/(-s.rate)
	}
	return 0
}

// syncDrivePhase refreshes drive d's band and phase from its current level.
// The level moves continuously but the band only at scheduled crossings, so
// callers sync when a crossing is due.
func (w *World) syncDrivePhase(e *Entity, d DriveKind) {
	w.syncDriveBand(e, d)
}

// applyStarvation drains HP for every drive whose band carries a drain. It
// reads levels lazily, so it is correct even for a colonist that has been
// resting for many ticks.
func (w *World) applyStarvation(e *Entity) {
	for i := 0; i < int(numDrives); i++ {
		d := DriveKind(i)
		t := &w.driveTables[d]
		if t.drainFrom > w.cfg.Drives[d].Max {
			continue // nothing about this drive ever drains HP
		}
		level := w.driveLevel(e, d)
		if level < t.drainFrom {
			continue
		}
		drain := t.hpDrain[t.bandOf(level)]
		if drain <= 0 {
			continue
		}
		spec := w.cfg.Drives[d]
		// A colonist already eating, or on its way to its own meal, is
		// guaranteed food: jobEat ends the job if the meal turns out to be
		// out of reach, and the grace with it.
		if d == DriveFood && e.Job == JobEat {
			continue
		}
		// So is one cooking its own supper out of scum already in the
		// scumhouse: a forager who dug it out, scraped it and carried it
		// home died at the stove, a recipe short of the meal.
		if d == DriveFood && e.Job == JobCraft && e.craftFor == ColonistOwner(e.ID) &&
			recipeMakesMeals(recipes[e.recipe]) {
			continue
		}
		if e.Job == JobUse && e.Drive == d {
			// A colonist that has already grabbed a portable drive (see
			// DriveSpec.GrabTicks) is guaranteed to finish regardless of the
			// facility's reachability or crowding — it is no longer there.
			if e.carrying {
				continue
			}
			// Reaching food does not reset the drive until UseTicks elapse.
			// Give an entity committed to a reachable source enough grace to
			// traverse its queue and finish eating rather than dying mid-meal.
			if w.facilityReachable(e, spec.Facility) {
				continue
			}
		}
		// The same grace applies while reachable life support is under
		// construction. This is especially important at startup, when staggered
		// hunger can reach Max shortly before the first facility room completes.
		if e.Kind == Colonist && w.reachableFacilityConstruction(e.Pos, spec.Facility) {
			continue
		}
		e.HP -= drain
		e.drives[d].hpDrained += drain
	}
}
