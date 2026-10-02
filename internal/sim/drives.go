package sim

// DriveKind enumerates the drives a colonist must satisfy. Adding a need is meant
// to be a table edit: append a kind here and add its DriveSpec in Config. Most
// needs have a Facility to satisfy them; social is satisfied by conversation.
type DriveKind uint8

// DrivePhase is one need's discrete physiological state. Each need owns an
// independent phase derived from its lazy level; satisfying a need remains an
// executor fact rather than a phase.
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
		return "need phase"
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
			Facility: NutrientPod, UseTicks: 18, Fatal: true,
			// A colonist grabs a portion in 3 ticks and eats it away from
			// the pod, instead of occupying its one access tile for the
			// full 18 — far more throughput per pod at the same cost.
			GrabTicks: 3,
		},
		DriveBladder: {
			Name: "bladder", Rise: 3, SeekAt: 600, CriticalAt: 900, Max: 1000,
			Facility: Toilet, UseTicks: 10, Fatal: false,
		},
		DriveSocial: {
			Name: "social", Rise: 2, SeekAt: 500, CriticalAt: 850, Max: 1000,
			Facility: Rock, UseTicks: 0, Fatal: false,
		},
		DriveSleep: {
			// Sleep builds slowly and, once sought, takes a night to clear:
			// 720 ticks awake and 360 in bed make a 1080-tick day, so a night
			// is eight clock hours and an hour is 45 ticks (see days.md).
			// Non-fatal like bladder: a colonist with no bunk waits rather
			// than dying.
			Name: "sleep", Rise: 1, SeekAt: 720, CriticalAt: 900, Max: 1000,
			Facility: Bed, UseTicks: 360, Fatal: false,
		},
	}
}

const (
	DriveFood DriveKind = iota
	DriveBladder
	DriveSocial
	DriveSleep

	numDrives // keep last: the count of needs
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
		return "need"
	}
}

// DriveSpec describes how one need behaves. Levels run 0..Max; 0 means satisfied.
// The cfg tags make the numeric fields tunable from the config file and the
// command line, one knob per need (see configfile.go). Name and Facility are
// deliberately untagged: they are the need's identity and its plumbing, not
// balance, and changing them from a file would let a config rename a need out
// from under the code that looks it up.
type DriveSpec struct {
	Name       string
	Rise       int     `cfg:"rise" doc:"level gained per tick"`
	SeekAt     int     `cfg:"seek-at" doc:"level at which the colonist drops work to satisfy it"`
	CriticalAt int     `cfg:"critical-at" doc:"level at which the need becomes critical"`
	Max        int     `cfg:"max" doc:"ceiling; a fatal need sitting here drains HP"`
	Facility   Terrain // structure that resets this need to 0
	UseTicks   int     `cfg:"use-ticks" doc:"ticks spent using the facility"`
	Fatal      bool    `cfg:"fatal" doc:"whether sitting at the ceiling damages the colonist"`
	// GrabTicks, if positive and less than UseTicks, makes this need portable:
	// a colonist spends only GrabTicks at the facility, then carries it away
	// and spends the rest of UseTicks finishing elsewhere, freeing the
	// facility's access tile for the next colonist immediately rather than
	// occupying it for the whole UseTicks. Zero means the need can only be
	// satisfied in place (bladder, sleep — there is nothing to take away).
	GrabTicks int `cfg:"grab-ticks" doc:"ticks at the facility before carrying the rest away (0 = must be used in place)"`
}

// TicksPerDay is how many ticks make one colony day, derived from the sleep
// need rather than tuned on its own: one waking stretch (an unslept colonist's
// sleep need rising from 0 to SeekAt at the base Rise) plus one night (the
// bed's UseTicks). That is the rhythm a well-housed colonist actually lives
// on, so "day 3" means "the colony has slept about twice". Deriving it keeps
// the calendar honest when sleep is retuned; a separate knob would drift.
// The walk to a bed and trait-scaled rise rates are deliberately ignored: a
// day has to be one fixed length for the whole colony. Always at least 1.
func (c *Config) TicksPerDay() int {
	spec := c.Drives[DriveSleep]
	awake := spec.Max // a sleep need that never rises: fall back to the ceiling
	if spec.Rise > 0 {
		awake = (spec.SeekAt + spec.Rise - 1) / spec.Rise
	}
	return max(awake+spec.UseTicks, 1)
}

// LandingHour is the clock time the colony lands at, tick 0. Colonists land
// with no sleep need, so landing is the start of their waking stretch: the
// morning. The day number turns over at midnight, not at landing, so a clock
// reading of 23:59 and the next 00:00 are on consecutive days.
const LandingHour = 6

// landingOffset is how far into its day, in ticks, the landing falls.
func landingOffset(ticksPerDay int) int {
	return ticksPerDay * LandingHour / 24
}

// DayOf is the colony day tick falls on. The landing (tick 0) is day 1.
func DayOf(tick, ticksPerDay int) int {
	ticksPerDay = max(ticksPerDay, 1)
	return (tick+landingOffset(ticksPerDay))/ticksPerDay + 1
}

// MinuteOfDay is the clock time tick falls on, in minutes since midnight
// (0..1439): the day's TicksPerDay ticks stretched over 24 hours, with the
// landing at LandingHour.
func MinuteOfDay(tick, ticksPerDay int) int {
	ticksPerDay = max(ticksPerDay, 1)
	return (tick + landingOffset(ticksPerDay)) % ticksPerDay * (24 * 60) / ticksPerDay
}

// driveLevel returns an entity's current level for one need, computed lazily
// from its stored base and the elapsed ticks, clamped to [0, Max]. Rats share
// the food need with colonists but hunger at their own faster rate.
func (w *World) driveLevel(e *Entity, i DriveKind) int {
	spec := w.cfg.Drives[i]
	// driveRise is the entity's per-need rate: colonists' is trait-scaled and rats
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

// syncDrivePhase projects one lazy need level into its discrete phase and caches
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

// drivePressure normalizes the actionable part of a need to [0, 100]. Growing
// and satisfied needs emit no pressure; critical pressure occupies [75, 100].
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

// resetDrive satisfies a need: its base drops to 0 as of the current tick and
// its phase/boundary cache are synchronized immediately.
func (w *World) resetDrive(e *Entity, i DriveKind) {
	e.Drives[i] = 0
	e.driveSince[i] = w.tick
	if i == DriveFood {
		e.forageNoted, e.forageRetry = false, 0 // fed: the next hunger is a new search
	}
	w.syncDrivePhase(e, i)
	if damage := e.starvationDamage[i]; damage > 0 {
		e.HP = min(e.MaxHP, e.HP+damage)
		e.starvationDamage[i] = 0
	}
}

// applyStarvation drains HP for any fatal need currently sitting at its max.
// It reads levels lazily, so it is correct even for a colonist that has been
// resting for many ticks.
func (w *World) applyStarvation(e *Entity) {
	for i := 0; i < int(numDrives); i++ {
		spec := w.cfg.Drives[i]
		if spec.Fatal && w.driveLevel(e, DriveKind(i)) >= spec.Max {
			// A colonist already eating, or on its way to its own meal, is
			// guaranteed food: jobEat ends the job if the meal turns out to be
			// out of reach, and the grace with it.
			if DriveKind(i) == DriveFood && e.Job == JobEat {
				continue
			}
			// So is one cooking its own supper out of scum already in the
			// scumhouse: a forager who dug it out, scraped it and carried it
			// home died at the stove, a recipe short of the meal.
			if DriveKind(i) == DriveFood && e.Job == JobCraft && e.craftFor == ColonistOwner(e.ID) &&
				recipeMakesMeals(recipes[e.recipe]) {
				continue
			}
			if e.Job == JobUse && e.Drive == DriveKind(i) {
				// A colonist that has already grabbed a portable need (see
				// DriveSpec.GrabTicks) is guaranteed to finish regardless of the
				// facility's reachability or crowding — it is no longer there.
				if e.carrying {
					continue
				}
				// Reaching food does not reset the need until UseTicks elapse. Give
				// an entity committed to a reachable source enough grace to traverse
				// its queue and finish eating rather than dying mid-meal.
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
			e.HP -= w.cfg.StarveDamage
			e.starvationDamage[i] += w.cfg.StarveDamage
		}
	}
}

// mostUrgentDrive returns the need a colonist should address first, if any is past
// its seek threshold. A fatal need (starvation) outranks any non-fatal one that
// is also urgent — otherwise a non-fatal need like bladder, which rises faster
// and caps further past its threshold, would permanently outrank food and let
// the colonist starve. Within the same fatal-ness, the need furthest past its
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
			(spec.Fatal && !worstFatal) ||
			(spec.Fatal == worstFatal && over > worstOver)
		if better {
			worst, worstOver, worstFatal, found = DriveKind(i), over, spec.Fatal, true
		}
	}
	return worst, found
}

// useState maps a need to the display state shown while fulfilling it.
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
