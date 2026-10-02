package sim

import (
	"fmt"
	"math"
)

// DriveKind enumerates a colonist's drives: pressures that accumulate on their
// own and are discharged by acting on them. Every drive is the same shape — a
// lazily accumulating level, a seek threshold, a critical threshold, a ceiling,
// and a Consequence for reaching the ceiling — so adding one is meant to be a
// table edit: append a kind here and add its DriveSpec in defaultDrives. Most
// drives have a Facility that discharges them; social is discharged by
// conversation. See docs/drives.md and, for where this is going,
// docs/drives-redesign.md.
type DriveKind uint8

// DrivePhase is one drive's discrete state. Each drive owns an independent
// phase derived from its lazy level; satisfying a drive remains an executor
// fact rather than a phase.
type DrivePhase uint8

const (
	DriveSatisfied DrivePhase = iota
	DriveGrowing
	DrivePressing
	DriveCritical
)

func (p DrivePhase) String() string {
	switch p {
	case DriveSatisfied:
		return "satisfied"
	case DriveGrowing:
		return "growing"
	case DrivePressing:
		return "pressing"
	case DriveCritical:
		return "critical"
	default:
		return "drive phase"
	}
}

// Consequence is what happens to a colonist whose drive reaches its ceiling
// (DriveSpec.Max). It is a drive's identity, not a balance knob, so it is not
// tunable from the settings file: the rest of the simulation (which drive
// outranks which, the grace periods, what a frontend paints red) keys off it.
//
// Death is a drain (HP, every tick at the ceiling); loneliness is an
// experience (an occurrence the colonist feels and remembers); passing out is
// an event (it happens once and discharges the drive), and so is soiling
// oneself. See docs/drives.md.
type Consequence uint8

const (
	// ConsequenceNone: nothing happens at the ceiling beyond maximal pressure.
	ConsequenceNone Consequence = iota
	// ConsequenceDeath: HP drains (StarveDamage a tick) while the drive sits at
	// its ceiling, and is restored when it is satisfied. Starvation.
	ConsequenceDeath
	// ConsequenceLoneliness: the colonist feels lonely — a "felt-lonely"
	// occurrence through the perception grammar, so its mood hit and memory
	// are cognition.yaml rows — on reaching the ceiling, and again every
	// ConsequenceEvery ticks while it stays there. Unmet social.
	ConsequenceLoneliness
	// ConsequencePassOut: the colonist collapses where it stands and lies
	// unconscious for PassOutTicks, then comes to with the drive met. An
	// event: it happens once and discharges the drive. Unmet sleep.
	ConsequencePassOut
	// ConsequenceSoiling: the colonist wets itself where it stands. An event:
	// the drive resets, and a `soil` occurrence carries the embarrassment to
	// the colonist and the disgust to anyone close enough to see. Unmet
	// bladder.
	ConsequenceSoiling

	numConsequences // keep last: the count of consequences
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
	default:
		return "consequence"
	}
}

const (
	defaultTraitChance           = 30
	defaultMoodMax               = 100
	defaultMoodLabelSwitchMargin = 5
)

func defaultDrives() [numDrives]DriveSpec {
	return [numDrives]DriveSpec{
		DriveFood: {
			Name: "food", Rise: 2, SeekAt: 650, CriticalAt: 1000, Max: 1000,
			Facility: NutrientPod, UseTicks: 18, Consequence: ConsequenceDeath,
			// A colonist grabs a portion in 3 ticks and eats it away from
			// the pod, instead of occupying its one access tile for the
			// full 18 — far more throughput per pod at the same cost.
			GrabTicks: 3,
		},
		DriveBladder: {
			// A colonist that never reaches a toilet wets itself.
			Name: "bladder", Rise: 3, SeekAt: 600, CriticalAt: 900, Max: 1000,
			Facility: Toilet, UseTicks: 10, Consequence: ConsequenceSoiling,
		},
		DriveSocial: {
			// A colonist left without company feels lonely, and feels it again
			// every 200 ticks (about four conversations' worth) it goes on.
			Name: "social", Rise: 2, SeekAt: 500, CriticalAt: 850, Max: 1000,
			Facility: Rock, UseTicks: 0,
			Consequence: ConsequenceLoneliness, ConsequenceEvery: 200,
		},
		DriveSleep: {
			// Sleep builds slowly and, once sought, takes a long lie-down to
			// clear. A colonist that gets no sleep at all passes out wherever
			// it is, for longer than a night in a bunk (PassOutTicks).
			Name: "sleep", Rise: 1, SeekAt: 700, CriticalAt: 900, Max: 1000,
			Facility: Bed, UseTicks: 40, Consequence: ConsequencePassOut,
		},
	}
}

const (
	DriveFood DriveKind = iota
	DriveBladder
	DriveSocial
	DriveSleep

	numDrives // keep last: the count of drives
)

func (n DriveKind) String() string {
	switch n {
	case DriveFood:
		return "food"
	case DriveBladder:
		return "bladder"
	case DriveSocial:
		return "social"
	case DriveSleep:
		return "sleep"
	default:
		return "drive"
	}
}

// DriveSpec describes how one drive behaves: how fast it accumulates (Rise),
// its thresholds (SeekAt, CriticalAt, Max), what discharges it (Facility,
// UseTicks, GrabTicks), and what reaching Max does (Consequence). Levels run
// 0..Max; 0 means satisfied. The cfg tags make the numeric fields tunable from
// the config file and the command line, one knob per drive (see
// configfile.go). Name, Facility and Consequence are deliberately untagged:
// they are the drive's identity and its plumbing, not balance, and changing
// them from a file would let a config rename a drive out from under the code
// that looks it up.
type DriveSpec struct {
	Name        string
	Rise        int         `cfg:"rise" doc:"level gained per tick"`
	SeekAt      int         `cfg:"seek-at" doc:"level at which the colonist drops work to satisfy it"`
	CriticalAt  int         `cfg:"critical-at" doc:"level at which the drive becomes critical"`
	Max         int         `cfg:"max" doc:"ceiling; the drive's consequence applies while it sits here"`
	Facility    Terrain     // structure that resets this drive to 0
	UseTicks    int         `cfg:"use-ticks" doc:"ticks spent using the facility"`
	Consequence Consequence // what reaching Max does to the colonist
	// ConsequenceEvery is how often an experience consequence (loneliness)
	// recurs while the drive stays at Max. Zero means once per stay at the
	// ceiling. Drains (death) apply every tick regardless.
	ConsequenceEvery int `cfg:"consequence-every" doc:"ticks between repeats of the drive's felt consequence while it stays at its ceiling (0: once until satisfied)"`
	// GrabTicks, if positive and less than UseTicks, makes this drive portable:
	// a colonist spends only GrabTicks at the facility, then carries it away
	// and spends the rest of UseTicks finishing elsewhere, freeing the
	// facility's access tile for the next colonist immediately rather than
	// occupying it for the whole UseTicks. Zero means the drive can only be
	// satisfied in place (bladder, sleep — there is nothing to take away).
	GrabTicks int `cfg:"grab-ticks" doc:"ticks at the facility before carrying the rest away (0 = must be used in place)"`
}

// Fatal reports whether reaching this drive's ceiling kills. Arbitration
// leans on it: a drive that kills outranks one that does not (see
// mostUrgentDrive).
func (s DriveSpec) Fatal() bool { return s.Consequence == ConsequenceDeath }

// driveLevel returns an entity's current level for one drive, computed lazily
// from its stored base and the elapsed ticks, clamped to [0, Max]. Rats share
// the food drive with colonists but hunger at their own faster rate.
func (w *World) driveLevel(e *Entity, i DriveKind) int {
	spec := w.cfg.Drives[i]
	// driveRise is the entity's per-drive rate: colonists' is trait-scaled and rats
	// hunger fast (see personality.go and newEntity).
	lvl := e.Drives[i] + e.driveRise[i]*(w.tick-e.driveSince[i])
	if lvl > spec.Max {
		lvl = spec.Max
	}
	if lvl < 0 {
		lvl = 0
	}
	return lvl
}

// syncDrivePhase projects one lazy drive level into its discrete phase and caches
// the next tick at which rising alone can change that phase. A zero boundary
// tick means no future crossing is scheduled (the phase is critical or rise is
// zero).
func (w *World) syncDrivePhase(e *Entity, n DriveKind) (changed bool) {
	return w.syncDrivePhaseAtLevel(e, n, w.driveLevel(e, n))
}

func (w *World) syncDrivePhaseAtLevel(e *Entity, n DriveKind, level int) (changed bool) {
	spec := w.cfg.Drives[n]
	phase := phaseForLevel(level, spec)
	changed = e.drivePhase[n] != phase
	e.drivePhase[n] = phase
	e.nextDrivePhaseTick[n] = nextDrivePhaseTick(w.tick, level, e.driveRise[n], phase, spec)
	if changed {
		w.markMindDirty(e)
	}
	return changed
}

func nextDrivePhaseTick(now, level, rise int, phase DrivePhase, spec DriveSpec) int {
	if rise <= 0 || phase == DriveCritical {
		return 0
	}
	target := 1
	switch phase {
	case DriveGrowing:
		target = spec.SeekAt
	case DrivePressing:
		target = spec.CriticalAt
	}
	if target <= level {
		return now
	}
	return now + (target-level+rise-1)/rise
}

func phaseForLevel(level int, spec DriveSpec) DrivePhase {
	switch {
	case level == 0:
		return DriveSatisfied
	case level < spec.SeekAt:
		return DriveGrowing
	case level < spec.CriticalAt:
		return DrivePressing
	default:
		return DriveCritical
	}
}

// drivePressure normalizes the actionable part of a drive to [0, 100]. Growing
// and satisfied drives emit no pressure; critical pressure occupies [75, 100].
func drivePressure(level int, spec DriveSpec) int {
	if level < spec.SeekAt {
		return 0
	}
	if level < spec.CriticalAt {
		return clampInt(1+74*(level-spec.SeekAt)/atLeast1(spec.CriticalAt-spec.SeekAt), 1, 74)
	}
	if spec.CriticalAt == spec.Max && level >= spec.Max {
		return 100
	}
	return clampInt(75+25*(level-spec.CriticalAt)/atLeast1(spec.Max-spec.CriticalAt), 75, 100)
}

// resetDrive satisfies a drive: its base drops to 0 as of the current tick and
// its phase/boundary cache are synchronized immediately.
func (w *World) resetDrive(e *Entity, i DriveKind) {
	e.Drives[i] = 0
	e.driveSince[i] = w.tick
	if i == DriveFood {
		e.forageNoted, e.forageRetry = false, 0 // fed: the next hunger is a new search
	}
	e.nextConsequence[i] = 0
	w.syncDrivePhase(e, i)
	if damage := e.starvationDamage[i]; damage > 0 {
		e.HP = min(e.MaxHP, e.HP+damage)
		e.starvationDamage[i] = 0
	}
}

// applyDriveConsequences applies the Consequence of every drive sitting at its
// ceiling. It reads levels lazily, so it is correct even for a colonist that
// has been resting for many ticks.
func (w *World) applyDriveConsequences(e *Entity) {
	for i := DriveKind(0); i < numDrives; i++ {
		spec := w.cfg.Drives[i]
		if spec.Consequence == ConsequenceNone || w.driveLevel(e, i) < spec.Max {
			continue
		}
		switch spec.Consequence {
		case ConsequenceDeath:
			w.starve(e, i, spec)
		case ConsequenceLoneliness:
			// Only colonists feel anything; a rat's social drive never rises,
			// but nothing here should depend on that.
			if e.Kind == Colonist && w.consequenceDue(e, i, spec) {
				w.emitDone(e, ActionFeel, NounLoneliness, "Felt lonely.")
			}
		case ConsequencePassOut:
			if e.Kind == Colonist && e.passedOutUntil == 0 && !w.usingFacility(e, DriveSleep) {
				w.passOut(e)
			}
		case ConsequenceSoiling:
			if e.Kind == Colonist && !w.usingFacility(e, DriveBladder) {
				w.wetSelf(e)
			}
		}
	}
}

// passOut is ConsequencePassOut: the colonist drops whatever it was doing and
// collapses where it stands. It stays down for PassOutTicks — no focus, no
// job, no fleeing; an alien that finds it there finds it helpless — and
// comes to in stayPassedOut with the sleep drive met.
func (w *World) passOut(e *Entity) {
	w.clearJob(e)
	e.focus, e.focusSince = FocusIdle, w.tick
	e.resting = false
	e.clearPath()
	e.passedOutUntil = w.tick + w.cfg.PassOutTicks
	e.State = PassedOut
	o := w.occurrence(e, ActionCollapse, nil, e.Pos, "Passed out from exhaustion.")
	w.emitOccurrence(o)
	w.logEvent(LogNote, fmt.Sprintf("%s passed out from exhaustion.", e.displayName()))
	w.markMindDirty(e)
}

// usingFacility reports whether e is already at the facility that satisfies d,
// using it: asleep beside its bed, or at the toilet. A drive keeps rising until
// the use finishes, so one that arrived near the ceiling reaches it there —
// where the colonist is already doing what the consequence would force. On
// the way is no exemption: you can collapse in the corridor, or not make it.
func (w *World) usingFacility(e *Entity, d DriveKind) bool {
	return e.Job == JobUse && e.Drive == d && !e.carrying && e.useFacilitySet &&
		e.Pos.Adjacent(e.useFacility) && w.TerrainAt(e.useFacility) == w.cfg.Drives[d].Facility
}

// wetSelf is ConsequenceSoiling: the bladder empties where the colonist
// stands. It discharges the drive and leaves the colonist doing whatever it
// was doing — the cost is the embarrassment (soiled-self) and what anyone
// close enough sees (witnessed-soiling), both cognition.yaml rows. It can
// happen while passed out.
func (w *World) wetSelf(e *Entity) {
	w.resetDrive(e, DriveBladder)
	o := w.occurrence(e, ActionSoil, nil, e.Pos, "")
	o.ActorText = fmt.Sprintf("Wet %s.", e.reflexive())
	o.WitnessText = fmt.Sprintf("Saw %s wet %s.", e.displayName(), e.reflexive())
	w.emitOccurrence(o)
	w.logEvent(LogNote, fmt.Sprintf("%s wet %s.", e.displayName(), e.reflexive()))
}

// stayPassedOut runs an unconscious colonist's turn and reports whether it is
// still down. The body goes on (affect decays, a sealed room is noticed,
// uranium doses), but nothing is perceived or chosen. On the tick it comes to
// the sleep drive resets and the colonist thinks again from scratch.
func (w *World) stayPassedOut(e *Entity) bool {
	if e.passedOutUntil == 0 {
		return false
	}
	w.decayAffect(e)
	w.updateDisconnected(e)
	w.applyUraniumExposure(e)
	if w.tick < e.passedOutUntil {
		e.State = PassedOut
		return true
	}
	e.passedOutUntil = 0
	e.State = Idle
	w.resetDrive(e, DriveSleep)
	w.markMindDirty(e)
	return true // the waking tick is spent coming to
}

// consequenceDue reports whether an experience consequence should fire now,
// and books the next one if so: the first tick at the ceiling, then every
// ConsequenceEvery ticks while the drive stays there (never again, with 0).
// resetDrive clears the booking, so the next stay at the ceiling starts fresh.
func (w *World) consequenceDue(e *Entity, i DriveKind, spec DriveSpec) bool {
	if w.tick < e.nextConsequence[i] {
		return false
	}
	if spec.ConsequenceEvery > 0 {
		e.nextConsequence[i] = w.tick + spec.ConsequenceEvery
	} else {
		e.nextConsequence[i] = math.MaxInt
	}
	return true
}

// starve is ConsequenceDeath: it drains HP for a drive at its ceiling, unless
// the colonist is already on its way to satisfying it, and books the damage
// against the drive so satisfying it heals exactly that much (see resetDrive).
func (w *World) starve(e *Entity, i DriveKind, spec DriveSpec) {
	// A colonist already eating, or on its way to its own meal, is
	// guaranteed food: jobEat ends the job if the meal turns out to be
	// out of reach, and the grace with it.
	if i == DriveFood && e.Job == JobEat {
		return
	}
	// So is one cooking its own supper out of scum already in the
	// scumhouse: a forager who dug it out, scraped it and carried it
	// home died at the stove, a recipe short of the meal.
	if i == DriveFood && e.Job == JobCraft && e.craftFor == ColonistOwner(e.ID) &&
		recipeMakesMeals(recipes[e.recipe]) {
		return
	}
	if e.Job == JobUse && e.Drive == i {
		// A colonist that has already grabbed a portable drive (see
		// DriveSpec.GrabTicks) is guaranteed to finish regardless of the
		// facility's reachability or crowding — it is no longer there.
		if e.carrying {
			return
		}
		// Reaching food does not reset the drive until UseTicks elapse. Give
		// an entity committed to a reachable source enough grace to traverse
		// its queue and finish eating rather than dying mid-meal.
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
	e.HP -= w.cfg.StarveDamage
	e.starvationDamage[i] += w.cfg.StarveDamage
}

// mostUrgentDrive returns the drive a colonist should address first, if any is past
// its seek threshold. A fatal drive (starvation) outranks any non-fatal one that
// is also urgent — otherwise a non-fatal drive like bladder, which rises faster
// and caps further past its threshold, would permanently outrank food and let
// the colonist starve. Within the same fatal-ness, the drive furthest past its
// threshold wins.
func (w *World) mostUrgentDrive(e *Entity) (DriveKind, bool) {
	worst := DriveKind(0)
	worstOver := -1
	worstFatal := false
	found := false
	for i := 0; i < int(numDrives); i++ {
		spec := w.cfg.Drives[i]
		over := w.driveLevel(e, DriveKind(i)) - spec.SeekAt
		if over < 0 {
			continue
		}
		better := !found ||
			(spec.Fatal() && !worstFatal) ||
			(spec.Fatal() == worstFatal && over > worstOver)
		if better {
			worst, worstOver, worstFatal, found = DriveKind(i), over, spec.Fatal(), true
		}
	}
	return worst, found
}

// useState maps a drive to the display state shown while fulfilling it.
func useState(n DriveKind) State {
	switch n {
	case DriveFood:
		return Eating
	case DriveBladder:
		return Relieving
	case DriveSleep:
		return Sleeping
	default:
		return Idle
	}
}
