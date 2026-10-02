package sim

import (
	"fmt"
	"strings"
)

// FocusKind is the goal a colonist is currently pursuing. Jobs remain the
// execution layer: a drive focus may execute either JobUse or JobBuild, while an
// idle focus may execute an opportunistic conversation.
type FocusKind uint8

const (
	FocusIdle FocusKind = iota
	FocusWork
	FocusEat
	FocusRelieve
	FocusSocialize
	FocusSleep
	FocusFlee
	FocusFight
	FocusEscape

	numFocusKinds
)

func (f FocusKind) String() string {
	switch f {
	case FocusIdle:
		return "idle"
	case FocusWork:
		return "work"
	case FocusEat:
		return "eat"
	case FocusRelieve:
		return "relieve"
	case FocusSocialize:
		return "socialize"
	case FocusSleep:
		return "sleep"
	case FocusFlee:
		return "flee"
	case FocusFight:
		return "fight"
	case FocusEscape:
		return "escape"
	default:
		return "focus"
	}
}

// FocusSpec contains the tunable part of one focus's score. ChargeWeight and
// GripWeight consume the numeric affect axes; display labels never feed scores.
type FocusSpec struct {
	Name           string
	Base           int `cfg:"base" doc:"baseline score before state contributions"`
	DriveWeight    int `cfg:"drive-weight" doc:"matching drive pressure contribution"`
	ChargeWeight   int `cfg:"charge-weight" doc:"signed response to affect charge"`
	GripWeight     int `cfg:"grip-weight" doc:"signed response to affect grip"`
	DistanceWeight int `cfg:"distance-weight" doc:"penalty per cheap distance unit"`
}

// FocusScore retains the explanation for one candidate's final score.
type FocusScore struct {
	Base        int
	Drive       int
	Affect      int
	Stimulus    int
	Personality int
	Commitment  int
	Distance    int
}

func (s FocusScore) Total() int {
	return s.Base + s.Drive + s.Affect + s.Stimulus +
		s.Personality + s.Commitment + s.Distance
}

// FocusCandidate is one cheap, eligible transition considered by chooseFocus.
// Exact target search and claiming remain in the selected focus's executor.
type FocusCandidate struct {
	Kind     FocusKind
	Drive    DriveKind
	Threat   EntityID
	Eligible bool
	Score    FocusScore
}

// String is deliberately stable so tests and debug tools can compare complete
// score explanations. It is never called by normal simulation arbitration.
func (c FocusCandidate) String() string {
	return fmt.Sprintf("%s eligible=%t base=%d need=%d affect=%d stimulus=%d personality=%d commitment=%d distance=%d total=%d",
		c.Kind, c.Eligible, c.Score.Base, c.Score.Drive, c.Score.Affect,
		c.Score.Stimulus, c.Score.Personality, c.Score.Commitment,
		c.Score.Distance, c.Score.Total())
}

func formatFocusCandidates(candidates *[numFocusKinds]FocusCandidate) string {
	var b strings.Builder
	for i := range candidates {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(candidates[i].String())
	}
	return b.String()
}

func focusForDrive(n DriveKind) FocusKind {
	switch n {
	case DriveFood:
		return FocusEat
	case DriveBladder:
		return FocusRelieve
	case DriveSocial:
		return FocusSocialize
	case DriveSleep:
		return FocusSleep
	default:
		return FocusIdle
	}
}

func driveForFocus(f FocusKind) (DriveKind, bool) {
	switch f {
	case FocusEat:
		return DriveFood, true
	case FocusRelieve:
		return DriveBladder, true
	case FocusSocialize:
		return DriveSocial, true
	case FocusSleep:
		return DriveSleep, true
	default:
		return 0, false
	}
}

func affectContribution(charge, grip int, spec FocusSpec, moodMax int) int {
	n := charge*spec.ChargeWeight + grip*spec.GripWeight
	// The documented/default coordinate range is 100. Keep that overwhelmingly
	// common divisor constant so the compiler avoids a runtime integer divide;
	// custom ranges retain exact normalization.
	if moodMax == 100 {
		return n / 100
	}
	return n / atLeast1(moodMax)
}

func workJob(job JobKind) bool {
	switch job {
	case JobMine, JobBuild, JobClean, JobStore, JobCraft, JobScrape, JobSell, JobCarry, JobTend:
		return true
	default:
		return false
	}
}

const cognitionFallbackTicks = 32

// markMindDirty is the sole invalidation entry point for cached arbitration.
// Moving the deadline to now is important: an event observed early in a turn
// must affect the focus selected later in that same turn.
func (w *World) markMindDirty(e *Entity) {
	if e == nil || e.Kind != Colonist {
		return
	}
	e.mindDirty = true
	e.nextThinkTick = w.tick
}

// nextCognitionTick returns the first tick on which an internal score or
// eligibility fact can change, capped by a bounded defensive reconsideration.
// Pressing drive pressure and non-neutral affect drift every tick, so those
// states deliberately do not get a multi-tick horizon here; widening that
// bound belongs to the separately gated validity-horizon step.
func (w *World) nextCognitionTick(e *Entity) int {
	next := w.tick + cognitionFallbackTicks
	if e.resting && e.wakeTick > w.tick && e.wakeTick < next {
		next = e.wakeTick
	}
	if e.nextStimulusExpiry > w.tick && e.nextStimulusExpiry < next {
		next = e.nextStimulusExpiry
	}
	// Charge and grip only: valence drifts too, but nothing scored reads it, so
	// a lingering mood is no reason to make a colonist think again.
	if !e.affectSettled() {
		return w.tick + 1
	}
	sleepProgressOnly := e.focus == FocusSleep && e.Job == JobUse && e.Drive == DriveSleep &&
		!e.carrying && e.useFacilitySet && e.Pos.Adjacent(e.useFacility) &&
		w.TerrainAt(e.useFacility) == w.cfg.Drives[DriveSleep].Facility
	for n := DriveKind(0); n < numDrives; n++ {
		level := w.driveLevel(e, n)
		phase := e.drives[n].phase
		if (phase == DrivePressing || phase == DriveCritical) && level < w.cfg.Drives[n].Max &&
			!(sleepProgressOnly && n == DriveSleep) {
			return w.tick + 1
		}
		if crossing := e.drives[n].nextCrossing; crossing > w.tick && crossing < next {
			next = crossing
		}
	}
	return next
}

// focusThreat is the alien a colonist's threat foci answer to. Anyone reacts
// to an alien within FleeRadius. A colonist already fleeing also keeps
// answering to one out to FleeRadius+FleeReleaseMargin — flee's hysteresis
// band — and held reports that the threat is only in that band, where flee
// stays eligible but fight and the other threat-gated checks do not start.
// Without the band, stepping one tile out of FleeRadius dropped flee, the
// next focus walked back in, and the colonist flickered every tick.
func (w *World) focusThreat(e *Entity) (threat *Entity, held bool) {
	if a, ok := w.nearestAlien(e.Pos, w.cfg.FleeRadius); ok {
		return a, false
	}
	if e.focus == FocusFlee && w.cfg.FleeReleaseMargin > 0 {
		if a, ok := w.nearestAlien(e.Pos, w.cfg.FleeRadius+w.cfg.FleeReleaseMargin); ok {
			return a, true
		}
	}
	return nil, false
}

func (w *World) currentFocusEligible(e *Entity, threat *Entity) bool {
	switch e.focus {
	case FocusIdle:
		return true
	case FocusWork:
		return workJob(e.Job) || !e.resting || w.tick >= e.wakeTick
	case FocusEat, FocusRelieve, FocusSocialize, FocusSleep:
		need, _ := driveForFocus(e.focus)
		phase := e.drives[need].phase
		if phase != DrivePressing && phase != DriveCritical {
			return false
		}
		if !w.cfg.Drives[need].Fatal {
			for n := DriveKind(0); n < numDrives; n++ {
				if w.cfg.Drives[n].Fatal && (e.drives[n].phase == DrivePressing || e.drives[n].phase == DriveCritical) {
					return false
				}
			}
		}
		return true
	case FocusFlee:
		return threat != nil
	case FocusFight:
		return threat != nil && bestWeapon(e.Inventory) != ItemNone
	case FocusEscape:
		return threat == nil && e.disconnectedTicks >= w.cfg.EscapeGraceTicks
	default:
		return false
	}
}

func cachedFocusCandidate(e *Entity, threat *Entity) FocusCandidate {
	selected := FocusCandidate{Kind: e.focus, Eligible: true}
	if need, ok := driveForFocus(e.focus); ok {
		selected.Drive = need
	}
	if threat != nil && (e.focus == FocusFlee || e.focus == FocusFight) {
		selected.Threat = threat.ID
	}
	return selected
}

// focusInputs is everything fillFocusCandidates reads. The world builds it
// from a colonist; the lab builds it from the story on screen. Neither path
// has its own copy of the score.
type focusInputs struct {
	focuses                                 [numFocusKinds]FocusSpec
	needs                                   [numDrives]DriveSpec
	level                                   [numDrives]int
	phase                                   [numDrives]DrivePhase
	charge, grip, moodMax                   int
	stimulus                                [numFocusKinds]int
	current                                 FocusKind
	workEligible                            bool
	threat                                  bool
	holdFlee                                bool // already fleeing, alien only in the release band
	threatID                                EntityID
	armed                                   bool
	escape                                  bool
	currentBonus, criticalBonus, fatalBonus int
}

// focusCandidates fills caller-owned storage so normal arbitration allocates
// nothing. Shared facts (the visible threat and each lazy drive level) are read
// once per call.
func (w *World) focusCandidates(e *Entity, out *[numFocusKinds]FocusCandidate) {
	var level [numDrives]int
	var phase [numDrives]DrivePhase
	for n := DriveKind(0); n < numDrives; n++ {
		level[n] = w.driveLevel(e, n)
		w.syncDrivePhase(e, n)
		phase[n] = e.drives[n].phase
	}
	threat, holdFlee := w.focusThreat(e)
	hasThreat := threat != nil && !holdFlee
	var threatID EntityID
	if threat != nil {
		threatID = threat.ID
	}
	fillFocusCandidates(focusInputs{
		focuses:       w.cfg.Focuses,
		needs:         w.cfg.Drives,
		level:         level,
		phase:         phase,
		charge:        e.affect.Charge,
		grip:          e.affect.Grip,
		moodMax:       w.cfg.MoodMax,
		stimulus:      e.stimulusFocusBias,
		current:       e.focus,
		workEligible:  workJob(e.Job) || !e.resting || w.tick >= e.wakeTick,
		threat:        hasThreat,
		holdFlee:      holdFlee,
		threatID:      threatID,
		armed:         bestWeapon(e.Inventory) != ItemNone,
		escape:        !hasThreat && e.disconnectedTicks >= w.cfg.EscapeGraceTicks,
		currentBonus:  w.cfg.FocusCurrentBonus,
		criticalBonus: w.cfg.FocusCriticalBonus,
		fatalBonus:    w.cfg.FocusFatalBonus,
	}, out)
}

func fillFocusCandidates(in focusInputs, out *[numFocusKinds]FocusCandidate) {
	for f := FocusKind(0); f < numFocusKinds; f++ {
		spec := in.focuses[f]
		out[f] = FocusCandidate{
			Kind:     f,
			Eligible: f == FocusIdle,
			Score: FocusScore{
				Base:   spec.Base,
				Affect: affectContribution(in.charge, in.grip, spec, in.moodMax),
			},
		}
	}

	out[FocusWork].Eligible = in.workEligible
	for f := FocusKind(0); f < numFocusKinds; f++ {
		out[f].Score.Stimulus = in.stimulus[f]
	}

	fatalPressing := false
	for n := DriveKind(0); n < numDrives; n++ {
		spec := in.needs[n]
		phase := in.phase[n]
		if phase != DrivePressing && phase != DriveCritical {
			continue
		}
		pressure := drivePressure(in.level[n], spec)
		f := focusForDrive(n)
		c := &out[f]
		c.Drive = n
		c.Eligible = true
		c.Score.Drive = pressure * in.focuses[f].DriveWeight / 100
		if phase == DriveCritical {
			c.Score.Drive += in.criticalBonus
		}
		if spec.Fatal {
			c.Score.Drive += in.fatalBonus
			fatalPressing = true
		}
	}

	// Preserve the existing hard invariant that a pressing fatal drive outranks
	// non-fatal drives. Threats remain eligible and can still dominate it.
	if fatalPressing {
		for n := DriveKind(0); n < numDrives; n++ {
			if in.needs[n].Fatal {
				continue
			}
			out[focusForDrive(n)].Eligible = false
		}
	}

	if in.threat {
		out[FocusFlee].Eligible = true
		out[FocusFlee].Threat = in.threatID
		if in.armed {
			out[FocusFight].Eligible = true
			out[FocusFight].Threat = in.threatID
			// Standing and firing has no locomotion cost when the target is in
			// range. Evasion always spends at least one movement step, which also
			// preserves the established armed-colonist behavior on an otherwise
			// exact tie.
			out[FocusFlee].Score.Distance = -in.focuses[FocusFlee].DistanceWeight
		}
	} else if in.holdFlee {
		// Flee's release band (see focusThreat): the colonist keeps running
		// from an alien it has not yet put clearly behind it. Fight is not
		// offered here — nobody turns to shoot at something they could not
		// have started a fight with.
		out[FocusFlee].Eligible = true
		out[FocusFlee].Threat = in.threatID
	}

	// A room cut off from the colony's main network for EscapeGraceTicks
	// straight is worth breaking out of on its own, ahead of even a fatal
	// drive: reachability, not local coping, is what actually stayed broken,
	// and the nearest wall is very often the same one sealing off the very
	// facility that drive is failing to reach. An immediate predator is the one
	// thing that still outranks it — self-preservation never waits on a wall.
	// See updateDisconnected and rooms.go's mainRoom.
	out[FocusEscape].Eligible = in.escape

	if in.current < numFocusKinds && out[in.current].Eligible {
		out[in.current].Score.Commitment = in.currentBonus
	}
}

func betterFocus(a, b FocusCandidate, current FocusKind) bool {
	at, bt := a.Score.Total(), b.Score.Total()
	if at != bt {
		return at > bt
	}
	if a.Kind == current || b.Kind == current {
		return a.Kind == current
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.Threat < b.Threat
}

func (w *World) chooseFocus(e *Entity, candidates *[numFocusKinds]FocusCandidate) FocusCandidate {
	w.focusCandidates(e, candidates)
	return selectFocus(candidates, e.focus, w.cfg.FocusSwitchMargin)
}

func selectFocus(candidates *[numFocusKinds]FocusCandidate, current FocusKind, margin int) FocusCandidate {
	best, found := leadingFocus(candidates, current)
	if current < numFocusKinds {
		held := candidates[current]
		if held.Eligible && (!found || best.Kind != current) &&
			best.Score.Total() <= held.Score.Total()+margin {
			return held
		}
	}
	return best
}

func leadingFocus(candidates *[numFocusKinds]FocusCandidate, current FocusKind) (FocusCandidate, bool) {
	best := FocusCandidate{}
	found := false
	for i := range candidates {
		c := candidates[i]
		if !c.Eligible || found && !betterFocus(c, best, current) {
			continue
		}
		best, found = c, true
	}
	return best, found
}
