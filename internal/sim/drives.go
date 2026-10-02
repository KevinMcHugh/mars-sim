package sim

// Drives: the scores a colonist has to keep down (hunger, bladder, company,
// sleep). A drive's level grows at a rate composed from a base and stacked
// modifiers — what the colonist is doing, its traits, the bands other drives
// sit in, and timed effects — and what the level does to the colonist is
// declared as bands (see drive_bands.go). Levels stay lazy: a base plus the
// tick it was taken, folded forward whenever an input to the rate changes, so
// a colonist whose rate is steady costs nothing per tick. See docs/drives.md.

// driveUnit is how many stored units make one point of a drive. Levels and
// rates are kept in thousandths so a rate can be scaled by any percent; with
// whole points, food rising 2 a tick could only be halved or stopped.
const driveUnit = 1000

// DriveKind enumerates the drives a colonist must satisfy. Adding a drive is
// meant to be a table edit: append a kind here and add its DriveSpec in
// defaultDrives. Most drives have a Facility to satisfy them; social is
// satisfied by conversation.
type DriveKind uint8

// DrivePhase is one drive's discrete urgency. Each drive owns an independent
// phase read off its band; satisfying a drive remains an executor fact rather
// than a phase.
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

const (
	defaultTraitChance           = 30
	defaultMoodMax               = 100
	defaultMoodLabelSwitchMargin = 5
)

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

func defaultDrives() [numDrives]DriveSpec {
	d := [numDrives]DriveSpec{
		DriveFood: {
			Name: "food", Rate: 1750, SeekAt: 650, CriticalAt: 1000, Max: 1000,
			Facility: NutrientPod, UseTicks: 18,
			// A colonist grabs a portion in 3 ticks and eats it away from
			// the pod, instead of occupying its one access tile for the
			// full 18 — far more throughput per pod at the same cost.
			GrabTicks: 3,
			// Starvation: HP drains while hunger sits at its ceiling.
			Consequences: []DriveConsequence{atCeiling(ConsequenceDeath)},
		},
		DriveBladder: {
			Name: "bladder", Rate: 3000, SeekAt: 600, CriticalAt: 900, Max: 1000,
			Facility: Toilet, UseTicks: 10,
			// A colonist that never reaches a toilet wets itself.
			Consequences: []DriveConsequence{atCeiling(ConsequenceSoiling)},
		},
		DriveSocial: {
			Name: "social", Rate: 2000, SeekAt: 500, CriticalAt: 850, Max: 1000,
			Facility: Rock, UseTicks: 0,
			// A colonist left without company feels lonely, and feels it
			// again every 200 ticks (about four conversations' worth) it
			// goes on.
			Consequences:     []DriveConsequence{atCeiling(ConsequenceLoneliness)},
			ConsequenceEvery: 200,
		},
		DriveSleep: {
			// Sleep builds slowly and, once sought, takes a night to clear:
			// 720 ticks awake and 360 in bed make a 1080-tick day, so a night
			// is eight clock hours and an hour is 45 ticks (see days.md).
			// Its rate is the same whatever the colonist is doing, because
			// the colony calendar is derived from it. A colonist that gets
			// no sleep at all passes out wherever it is (PassOutTicks).
			Name: "sleep", Rate: 1000, SeekAt: 720, CriticalAt: 900, Max: 1000,
			Facility: Bed, UseTicks: 360,
			Consequences: []DriveConsequence{atCeiling(ConsequencePassOut)},
		},
	}
	// Every drive grows at its full rate in every drive activity unless set
	// otherwise below, so a new DriveActivity starts out neutral.
	for i := range d {
		for a := range d[i].Activity {
			d[i].Activity[a] = 100
		}
	}
	// Hunger never stops, but it is slowest asleep and fastest at hard labor.
	d[DriveFood].Activity = activityPercents(10, 75, 100, 140)
	// Asleep, the other drives all but stop: a night is longer than food or
	// bladder take to come due, and a colonist pulled out of bed by them
	// never finishes one (see days.md).
	d[DriveBladder].Activity[DriveAsleep.index()] = 10
	d[DriveSocial].Activity[DriveAsleep.index()] = 0
	return d
}

// activityPercents lists a drive's percents in DriveActivity order: asleep,
// idle, working, labor.
func activityPercents(asleep, idle, working, labor int) [numDriveActivities]int {
	var p [numDriveActivities]int
	p[DriveAsleep.index()] = asleep
	p[DriveIdle.index()] = idle
	p[DriveWorking.index()] = working
	p[DriveLabor.index()] = labor
	return p
}

// DriveSpec describes how one drive behaves. Levels run Min..Max; 0 means
// satisfied. The cfg tags make the numeric fields tunable from the config file
// and the command line, one knob per drive (see configfile.go). Name and
// Facility are deliberately untagged: they are the drive's identity and its
// plumbing, not balance, and changing them from a file would let a config
// rename a drive out from under the code that looks it up. Consequences and
// Ramps are untagged for the same reason: what a drive does is its identity
// (the rest of the simulation keys off it), and the config file has no lists
// anyway.
type DriveSpec struct {
	Name       string
	Rate       int     `cfg:"rate" doc:"base growth per tick, in thousandths of a point"`
	Min        int     `cfg:"min" doc:"floor the level never falls below"`
	SeekAt     int     `cfg:"seek-at" doc:"level at which the colonist drops work to satisfy it"`
	CriticalAt int     `cfg:"critical-at" doc:"level at which the drive becomes critical"`
	Max        int     `cfg:"max" doc:"ceiling; the drive's consequences there apply while it sits here"`
	Facility   Terrain // structure that resets this drive to 0
	UseTicks   int     `cfg:"use-ticks" doc:"ticks spent using the facility"`
	// GrabTicks, if positive and less than UseTicks, makes this drive portable:
	// a colonist spends only GrabTicks at the facility, then carries it away
	// and spends the rest of UseTicks finishing elsewhere, freeing the
	// facility's access tile for the next colonist immediately rather than
	// occupying it for the whole UseTicks. Zero means the drive can only be
	// satisfied in place (bladder, sleep — there is nothing to take away).
	GrabTicks int `cfg:"grab-ticks" doc:"ticks at the facility before carrying the rest away (0 = must be used in place)"`
	// Activity is the percent of Rate this drive grows at in each drive
	// activity (see drive_activity.go), indexed by DriveActivity.index().
	Activity [numDriveActivities]int `cfg:"activity" doc:"percent of the base rate in the %s drive activity"`
	// Consequences and Ramps declare what levels do (see drive_bands.go).
	Consequences []DriveConsequence
	Ramps        []DriveRamp
	// ConsequenceEvery is how often an experience consequence (loneliness)
	// recurs while the drive stays in its range. Zero means once per stay.
	// Drains apply every tick and events once, regardless.
	ConsequenceEvery int `cfg:"consequence-every" doc:"ticks between repeats of the drive's felt consequence while it stays in range (0: once until satisfied)"`
}

// TicksPerDay is how many ticks make one colony day, derived from the sleep
// drive rather than tuned on its own: one waking stretch (an unslept colonist's
// sleep drive rising from 0 to SeekAt at the base Rate) plus one night (the
// bed's UseTicks). That is the rhythm a well-housed colonist actually lives
// on, so "day 3" means "the colony has slept about twice". Deriving it keeps
// the calendar honest when sleep is retuned; a separate knob would drift.
// The walk to a bed and trait-scaled rates are deliberately ignored: a day has
// to be one fixed length for the whole colony. Always at least 1.
func (c *Config) TicksPerDay() int {
	spec := c.Drives[DriveSleep]
	awake := spec.Max // a sleep drive that never rises: fall back to the ceiling
	if spec.Rate > 0 {
		awake = (spec.SeekAt*driveUnit + spec.Rate - 1) / spec.Rate
	}
	return max(awake+spec.UseTicks, 1)
}

// LandingHour is the clock time the colony lands at, tick 0. Colonists land
// with no sleep drive, so landing is the start of their waking stretch: the
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

// driveState is one entity's lazy state for one drive. base is the true
// level, in driveUnits, as of tick since; rate is how much it grows per tick
// from there. offset is the masking active effects apply (see
// drive_effects.go): the level everything reads is the true level less it.
// band, phase and nextCrossing cache where that felt level sits in the
// drive's band table and the next tick the current rate moves it across an
// edge (0: never). hpDrained is the HP this drive's consequences have taken,
// handed back when it is satisfied.
type driveState struct {
	base, since, rate int
	offset            int
	band              int
	phase             DrivePhase
	nextCrossing      int
	hpDrained         int
	// nextFeel is the first tick an experience consequence may be felt again
	// during this stay in its band; 0 until it is first felt. Changing band
	// or satisfying the drive clears it (see consequenceDue).
	nextFeel int
}

// driveTrue is e's true level for drive d, in driveUnits: its base grown at
// its rate since it was taken, clamped to the drive's range.
func (w *World) driveTrue(e *Entity, d DriveKind) int {
	spec := &w.cfg.Drives[d]
	s := &e.drives[d]
	return clampInt(s.base+s.rate*(w.tick-s.since), spec.Min*driveUnit, spec.Max*driveUnit)
}

// driveFelt is the level drive d acts at, in driveUnits: the true level less
// any masking, clamped to the drive's range.
func (w *World) driveFelt(e *Entity, d DriveKind) int {
	spec := &w.cfg.Drives[d]
	return clampInt(w.driveTrue(e, d)-e.drives[d].offset, spec.Min*driveUnit, spec.Max*driveUnit)
}

// driveLevel returns an entity's current level for one drive, in whole
// points: the felt level, computed lazily from the stored base and the ticks
// since. Everything that acts on a drive reads this.
func (w *World) driveLevel(e *Entity, d DriveKind) int {
	return floorDiv(w.driveFelt(e, d), driveUnit)
}

func floorDiv(n, d int) int {
	q := n / d
	if n%d != 0 && (n < 0) != (d < 0) {
		q--
	}
	return q
}

// driveRate composes e's current growth rate for drive d, in driveUnits per
// tick: (base + Σadd) × activity% × trait% × coupling% × effect%. Every factor
// is applied in a fixed order with integer arithmetic, so a seed always gives
// the same rates.
func (w *World) driveRate(e *Entity, d DriveKind) int {
	rate := e.driveBase[d]
	for _, fx := range e.effects {
		rate += w.effectStage(fx).Drive[d].Add
	}
	if e.Kind == Colonist {
		rate = rate * w.cfg.Drives[d].Activity[e.driveActivity.index()] / 100
	}
	rate = rate * e.driveTrait[d] / 100
	for src := DriveKind(0); src < numDrives; src++ {
		for _, c := range w.driveTables[src].rates[e.drives[src].band] {
			if c.target == d {
				rate = rate * (100 + c.change) / 100
			}
		}
	}
	for _, fx := range e.effects {
		rate = rate * (100 + w.effectStage(fx).Drive[d].RateChange) / 100
	}
	return rate
}

// refreshDrive is the one place a drive's rate changes. It folds the growth
// so far into the base at the old rate, adds instant (driveUnits) to the true
// level, recomputes the rate and masking from the current inputs, and
// re-places the felt level in its band. Call it whenever anything driveRate
// or the masking reads has just changed.
func (w *World) refreshDrive(e *Entity, d DriveKind, instant int) {
	spec := &w.cfg.Drives[d]
	s := &e.drives[d]
	s.base = clampInt(w.driveTrue(e, d)+instant, spec.Min*driveUnit, spec.Max*driveUnit)
	s.since = w.tick
	s.rate = w.driveRate(e, d)
	s.offset = 0
	for _, fx := range e.effects {
		s.offset += w.effectStage(fx).Drive[d].Offset * driveUnit
	}
	w.syncDriveBand(e, d)
}

// refreshDrives refreshes every drive, as for a change that can touch any of
// them (a new activity, a trait).
func (w *World) refreshDrives(e *Entity) {
	for d := DriveKind(0); d < numDrives; d++ {
		w.refreshDrive(e, d, 0)
	}
}

// setDrive puts drive d at level whole points as of now, keeping its rate.
func (w *World) setDrive(e *Entity, d DriveKind, level int) {
	s := &e.drives[d]
	s.base, s.since = level*driveUnit, w.tick
	w.refreshDrive(e, d, 0)
}

// initDrives starts an entity's drives at the given whole-point levels as of
// now, once its base rates and traits are known.
func (w *World) initDrives(e *Entity, levels [numDrives]int) {
	for d := DriveKind(0); d < numDrives; d++ {
		e.drives[d] = driveState{base: levels[d] * driveUnit, since: w.tick}
	}
	w.refreshDrives(e)
}

func phaseForLevel(level int, spec DriveSpec) DrivePhase {
	switch {
	case level <= 0:
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

// resetDrive satisfies a drive: its true level drops to 0 as of the current
// tick, and any HP its consequences drained is given back.
func (w *World) resetDrive(e *Entity, d DriveKind) {
	e.drives[d].base, e.drives[d].since = 0, w.tick
	if d == DriveFood {
		e.forageNoted, e.forageRetry = false, 0 // fed: the next hunger is a new search
	}
	e.drives[d].nextFeel = 0
	w.refreshDrive(e, d, 0)
	if damage := e.drives[d].hpDrained; damage > 0 {
		e.HP = min(e.MaxHP, e.HP+damage)
		e.drives[d].hpDrained = 0
	}
}

// mostUrgentDrive returns the drive a colonist should address first, if any is
// past its seek threshold. A fatal drive (starvation) outranks any non-fatal
// one that is also urgent — otherwise a non-fatal drive like bladder, which
// rises faster and caps further past its threshold, would permanently outrank
// food and let the colonist starve. Within the same fatal-ness, the drive
// furthest past its threshold wins.
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
