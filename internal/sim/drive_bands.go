package sim

import (
	"fmt"
	"math"
	"slices"
)

// Drive consequences: what a drive's level does to the colonist. A drive's
// range is cut into bands at every threshold that matters (SeekAt,
// CriticalAt, and the edges of each consequence), and within a band every
// consequence is constant. That keeps drives lazy: nothing about a colonist
// changes between crossings, and the next crossing is a tick that can be
// computed and scheduled. A smooth effect (movement slowing as exhaustion
// builds) is a ramp: a few small steps, expanded into bands when the config is
// compiled. See docs/drives.md.

// Consequence is what a drive does to a colonist while its level is in a
// consequence's range. What a drive does is its identity, not a balance knob:
// consequences are declared in defaultDrives, never in the settings file,
// because the rest of the simulation keys off them (a drive with a death
// consequence outranks one without; a frontend paints its bar red).
//
// They come in four shapes, and each is written once for any drive:
//
//   - a drain takes something every tick in range and gives it back when the
//     drive is satisfied (death: HP);
//   - an experience is something the colonist feels, an occurrence through the
//     perception grammar, on reaching the range and again every
//     DriveSpec.ConsequenceEvery ticks while it stays (loneliness);
//   - an event happens once and ends the stay (soiling resets the drive; a
//     colonist that passes out lies there until the drive has fallen);
//   - a rate change scales another drive's growth while in range.
type Consequence uint8

const (
	// ConsequenceNone: nothing happens beyond the drive's pressure.
	ConsequenceNone Consequence = iota
	// ConsequenceDeath (a drain): Value HP a tick, StarveDamage if Value is
	// 0, given back when the drive is satisfied. Starvation.
	ConsequenceDeath
	// ConsequenceLoneliness (an experience): the colonist feels lonely, a
	// "felt-lonely" occurrence whose mood hit and memory are cognition.yaml
	// rows. Unmet social.
	ConsequenceLoneliness
	// ConsequencePassOut (an event): the colonist collapses where it stands
	// and lies unconscious, the drive falling (it must, in the unconscious
	// drive activity), until it is below CriticalAt. Unmet sleep.
	ConsequencePassOut
	// ConsequenceSoiling (an event): the colonist wets itself where it
	// stands. The drive resets, and a `soil` occurrence carries the
	// embarrassment to the colonist and the sight of it to anyone close
	// enough. Unmet bladder.
	ConsequenceSoiling
	// ConsequenceRate (a rate change): Target's growth rate changes by Value
	// percent, +30 to 130%, -50 to half. How drives affect one another
	// (exhaustion building faster the hungrier a colonist is).
	ConsequenceRate

	numConsequences // keep last
)

func (c Consequence) String() string {
	switch c {
	case ConsequenceNone:
		return "none"
	case ConsequenceDeath:
		return "death"
	case ConsequenceLoneliness:
		return "loneliness"
	case ConsequencePassOut:
		return "passing out"
	case ConsequenceSoiling:
		return "soiling"
	case ConsequenceRate:
		return "rate"
	default:
		return "consequence"
	}
}

// acts reports whether c does something to the colonist itself (as opposed to
// changing another drive's rate), and so is dispatched each tick.
func (c Consequence) acts() bool {
	return c == ConsequenceDeath || c == ConsequenceLoneliness || c == ConsequencePassOut || c == ConsequenceSoiling
}

// DriveConsequence applies Kind with Value while its drive's level is in
// From..To, inclusive.
type DriveConsequence struct {
	From, To int
	Kind     Consequence
	Target   DriveKind // ConsequenceRate: the drive whose rate changes
	Value    int
}

// DriveCeiling, as a consequence's From or To, stands for the drive's Max
// wherever the settings put it, so a consequence of reaching the ceiling
// follows a retuned ceiling.
const DriveCeiling = -1

// atCeiling is a consequence of reaching a drive's ceiling and staying there.
func atCeiling(kind Consequence) DriveConsequence {
	return DriveConsequence{From: DriveCeiling, To: DriveCeiling, Kind: kind}
}

// resolve replaces DriveCeiling with the drive's Max.
func (c DriveConsequence) resolve(max int) DriveConsequence {
	if c.From == DriveCeiling {
		c.From = max
	}
	if c.To == DriveCeiling {
		c.To = max
	}
	return c
}

// DriveRamp is a consequence whose Value moves from FromValue at level From to
// ToValue at level To in Steps even steps, and stays at ToValue above To. It
// expands into Steps+1 DriveConsequences.
type DriveRamp struct {
	From, To           int
	FromValue, ToValue int
	Steps              int
	Kind               Consequence
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

// Fatal reports whether the drive can kill: whether any of its consequences is
// death. Arbitration leans on it: a drive that kills outranks one that does
// not (see mostUrgentDrive and fillFocusCandidates).
func (s DriveSpec) Fatal() bool {
	for _, c := range s.Consequences {
		if c.Kind == ConsequenceDeath {
			return true
		}
	}
	for _, r := range s.Ramps {
		if r.Kind == ConsequenceDeath {
			return true
		}
	}
	return false
}

// CeilingConsequence is what the drive does at its ceiling, for a frontend to
// show: its death consequence there if it has one, else the first other one
// that acts there, else none.
func (s DriveSpec) CeilingConsequence() Consequence {
	found := ConsequenceNone
	for _, c := range s.Consequences {
		c = c.resolve(s.Max)
		if !c.Kind.acts() || s.Max < c.From || s.Max > c.To {
			continue
		}
		if c.Kind == ConsequenceDeath {
			return c.Kind
		}
		if found == ConsequenceNone {
			found = c.Kind
		}
	}
	return found
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
	// acts lists the experience and event consequences in force in each
	// band, in declaration order.
	acts [][]Consequence
	// actFrom is the lowest level any band drains HP or acts at, so the
	// per-tick consequence check can skip a drive below it without finding
	// its band.
	actFrom int
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
	cons := make([]DriveConsequence, 0, len(spec.Consequences))
	for _, c := range spec.Consequences {
		cons = append(cons, c.resolve(spec.Max))
	}
	for i, r := range spec.Ramps {
		if r.From == DriveCeiling {
			r.From = spec.Max
		}
		if r.To == DriveCeiling {
			r.To = spec.Max
		}
		if r.Steps < 1 || r.To <= r.From {
			return driveTable{}, fmt.Errorf("ramp %d: needs at least one step and From below To (got %d steps, %d..%d)",
				i, r.Steps, r.From, r.To)
		}
		cons = append(cons, r.expand(spec.Max)...)
	}
	edges := []int{1, spec.SeekAt, spec.CriticalAt}
	for i, c := range cons {
		switch {
		case c.To < c.From:
			return driveTable{}, fmt.Errorf("consequence %d: To %d is below From %d", i, c.To, c.From)
		case c.Kind == ConsequenceNone || c.Kind >= numConsequences:
			return driveTable{}, fmt.Errorf("consequence %d: unknown kind %d", i, c.Kind)
		case c.Kind == ConsequenceRate && c.Target >= numDrives:
			return driveTable{}, fmt.Errorf("consequence %d: unknown target drive %d", i, c.Target)
		case c.Kind == ConsequenceRate && c.Value < -100:
			return driveTable{}, fmt.Errorf("consequence %d: a rate cannot fall below zero (change %d%%)", i, c.Value)
		case c.Kind == ConsequencePassOut && spec.Rate*spec.Activity[DriveUnconscious.index()] >= 0:
			return driveTable{}, fmt.Errorf("consequence %d: a colonist that passes out comes to when the drive falls, "+
				"so it must fall while unconscious (rate %d at %d%%)", i, spec.Rate, spec.Activity[DriveUnconscious.index()])
		case c.Kind == ConsequencePassOut && c.From <= spec.CriticalAt:
			return driveTable{}, fmt.Errorf("consequence %d: a colonist comes to below critical-at %d, "+
				"so passing out has to start above it (from %d)", i, spec.CriticalAt, c.From)
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
		acts:    make([][]Consequence, len(edges)+1),
		// Above any reachable level until a band that acts says otherwise.
		actFrom: spec.Max + 1,
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
			case ConsequenceDeath:
				drain := c.Value
				if drain == 0 {
					drain = starveDamage
				}
				t.hpDrain[b] += drain
			case ConsequenceRate:
				t.rates[b] = append(t.rates[b], driveRateChange{target: c.Target, change: c.Value})
			default:
				if !slices.Contains(t.acts[b], c.Kind) {
					t.acts[b] = append(t.acts[b], c.Kind)
				}
			}
			if c.Kind.acts() {
				t.actFrom = min(t.actFrom, rep)
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
	if band != old {
		s.nextFeel = 0 // a new band is a new stay: its experiences are felt at once
	}
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

// applyDriveConsequences applies every drive's consequences for the band its
// level is in now: drains every tick, experiences on their cadence, and
// events. colonistTurn calls it first thing. It reads levels lazily, so it is
// correct even for a colonist that has been resting for many ticks.
func (w *World) applyDriveConsequences(e *Entity) {
	for d := DriveKind(0); d < numDrives; d++ {
		t := &w.driveTables[d]
		if t.actFrom > w.cfg.Drives[d].Max {
			continue // nothing about this drive ever acts
		}
		level := w.driveLevel(e, d)
		if level < t.actFrom {
			continue
		}
		b := t.bandOf(level)
		if drain := t.hpDrain[b]; drain > 0 {
			w.starve(e, d, drain)
		}
		if e.Kind != Colonist {
			continue // only colonists feel, faint or blush
		}
		for _, c := range t.acts[b] {
			switch c {
			case ConsequenceLoneliness:
				if w.consequenceDue(e, d) {
					w.emitDone(e, ActionFeel, NounLoneliness, "Felt lonely.")
				}
			case ConsequencePassOut:
				if !e.passedOut && !w.usingFacility(e, d) {
					w.passOut(e, d)
				}
			case ConsequenceSoiling:
				if !w.usingFacility(e, d) {
					w.wetSelf(e, d)
				}
			}
		}
	}
}

// consequenceDue reports whether an experience consequence of drive d should
// fire now, and books the next one if so: at once on arriving in its band,
// then every ConsequenceEvery ticks while the drive stays there (never again,
// with 0). Leaving the band, or satisfying the drive, clears the booking.
func (w *World) consequenceDue(e *Entity, d DriveKind) bool {
	s := &e.drives[d]
	if s.nextFeel != 0 && w.tick < s.nextFeel {
		return false
	}
	if every := w.cfg.Drives[d].ConsequenceEvery; every > 0 {
		s.nextFeel = w.tick + every
	} else {
		s.nextFeel = math.MaxInt
	}
	return true
}

// starve is ConsequenceDeath: it drains HP for drive d, unless the colonist is
// already on its way to satisfying it, and books the damage against the drive
// so satisfying it heals exactly that much (see resetDrive).
func (w *World) starve(e *Entity, d DriveKind, drain int) {
	spec := w.cfg.Drives[d]
	// A colonist already eating, or on its way to its own meal, is
	// guaranteed food: jobEat ends the job if the meal turns out to be
	// out of reach, and the grace with it.
	if d == DriveFood && e.Job == JobEat {
		return
	}
	// So is one cooking its own supper out of scum already in the
	// scumhouse: a forager who dug it out, scraped it and carried it
	// home died at the stove, a recipe short of the meal.
	if d == DriveFood && e.Job == JobCraft && e.craftFor == ColonistOwner(e.ID) &&
		recipeMakesMeals(recipes[e.recipe]) {
		return
	}
	if e.Job == JobUse && e.Drive == d {
		// A colonist that has already grabbed a portable drive (see
		// DriveSpec.GrabTicks) is guaranteed to finish regardless of the
		// facility's reachability or crowding — it is no longer there.
		if e.carrying {
			return
		}
		// Reaching food does not reset the drive until UseTicks elapse.
		// Give an entity committed to a reachable source enough grace to
		// traverse its queue and finish eating rather than dying mid-meal.
		if w.facilityReachable(e, spec.Facility) {
			return
		}
	}
	// The same grace applies while reachable life support is under
	// construction. This is especially important at startup, when staggered
	// hunger can reach Max shortly before the first facility room completes.
	if e.Kind == Colonist && w.reachableFacilityConstruction(e.Pos, spec.Facility) {
		return
	}
	e.HP -= drain
	e.drives[d].hpDrained += drain
}

// usingFacility reports whether e is already at the facility that satisfies d,
// using it: asleep beside its bed, or at the toilet. A drive keeps growing
// until the use finishes, so one that arrived near its ceiling reaches it
// there, where the colonist is already doing what the consequence would force.
// On the way is no exemption: you can collapse in the corridor, or not make it.
func (w *World) usingFacility(e *Entity, d DriveKind) bool {
	return e.Job == JobUse && e.Drive == d && !e.carrying && e.useFacilitySet &&
		e.Pos.Adjacent(e.useFacility) && w.TerrainAt(e.useFacility) == w.cfg.Drives[d].Facility
}

// passOut is ConsequencePassOut: the colonist drops whatever it was doing and
// collapses where it stands, into the unconscious drive activity, where drive
// d falls (validated by compileDrive). It stays down — no focus, no job, no
// fleeing; an alien that finds it there finds it helpless — until d is below
// CriticalAt, so how long it lies there comes out of the drive's rates rather
// than a timer. It comes to still tired, and goes to find a bed.
func (w *World) passOut(e *Entity, d DriveKind) {
	w.clearJob(e)
	e.focus, e.focusSince = FocusIdle, w.tick
	e.resting = false
	e.clearPath()
	e.passedOut, e.passedOutDrive = true, d
	e.State = PassedOut
	// Its drives fall or slow from this tick, not from the end of the turn.
	e.driveActivity = DriveUnconscious
	w.refreshDrives(e)
	o := w.occurrence(e, ActionCollapse, nil, e.Pos, "Passed out from exhaustion.")
	w.emitOccurrence(o)
	w.logEvent(LogNote, fmt.Sprintf("%s passed out from exhaustion.", e.displayName()))
	w.markMindDirty(e)
}

// stayPassedOut runs an unconscious colonist's turn and reports whether it is
// still down. The body goes on (affect decays, a sealed room is noticed,
// uranium doses), but nothing is perceived or chosen. On the tick the drive
// that put it down falls below CriticalAt it comes to and thinks again from
// scratch.
func (w *World) stayPassedOut(e *Entity) bool {
	if !e.passedOut {
		return false
	}
	w.decayAffect(e)
	w.updateDisconnected(e)
	w.applyUraniumExposure(e)
	if w.driveLevel(e, e.passedOutDrive) >= w.cfg.Drives[e.passedOutDrive].CriticalAt {
		e.State = PassedOut
		return true
	}
	e.passedOut = false
	e.State = Idle
	w.markMindDirty(e)
	return true // the waking tick is spent coming to
}

// wetSelf is ConsequenceSoiling: drive d (the bladder) empties where the
// colonist stands. It discharges the drive and leaves the colonist doing
// whatever it was doing; the cost is the embarrassment (soiled-self) and what
// anyone close enough sees (witnessed-soiling), both cognition.yaml rows. It
// can happen while passed out.
func (w *World) wetSelf(e *Entity, d DriveKind) {
	w.resetDrive(e, d)
	o := w.occurrence(e, ActionSoil, nil, e.Pos, "")
	o.ActorText = fmt.Sprintf("Wet %s.", e.reflexive())
	o.WitnessText = fmt.Sprintf("Saw %s wet %s.", e.displayName(), e.reflexive())
	w.emitOccurrence(o)
	w.logEvent(LogNote, fmt.Sprintf("%s wet %s.", e.displayName(), e.reflexive()))
}
