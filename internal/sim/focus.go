package sim

import (
	"fmt"
	"strings"
)

// FocusKind is the goal a colonist is currently pursuing. Jobs remain the
// execution layer: a need focus may execute either JobUse or JobBuild, while an
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
	NeedWeight     int `cfg:"need-weight" doc:"matching need pressure contribution"`
	ChargeWeight   int `cfg:"charge-weight" doc:"signed response to affect charge"`
	GripWeight     int `cfg:"grip-weight" doc:"signed response to affect grip"`
	DistanceWeight int `cfg:"distance-weight" doc:"penalty per cheap distance unit"`
}

// FocusScore retains the explanation for one candidate's final score.
type FocusScore struct {
	Base        int
	Need        int
	Affect      int
	Stimulus    int
	Personality int
	Commitment  int
	Distance    int
}

func (s FocusScore) Total() int {
	return s.Base + s.Need + s.Affect + s.Stimulus +
		s.Personality + s.Commitment + s.Distance
}

// FocusCandidate is one cheap, eligible transition considered by chooseFocus.
// Exact target search and claiming remain in the selected focus's executor.
type FocusCandidate struct {
	Kind     FocusKind
	Need     NeedKind
	Threat   EntityID
	Eligible bool
	Score    FocusScore
}

// String is deliberately stable so tests and debug tools can compare complete
// score explanations. It is never called by normal simulation arbitration.
func (c FocusCandidate) String() string {
	return fmt.Sprintf("%s eligible=%t base=%d need=%d affect=%d stimulus=%d personality=%d commitment=%d distance=%d total=%d",
		c.Kind, c.Eligible, c.Score.Base, c.Score.Need, c.Score.Affect,
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

func focusForNeed(n NeedKind) FocusKind {
	switch n {
	case NeedFood:
		return FocusEat
	case NeedBladder:
		return FocusRelieve
	case NeedSocial:
		return FocusSocialize
	case NeedSleep:
		return FocusSleep
	default:
		return FocusIdle
	}
}

func needForFocus(f FocusKind) (NeedKind, bool) {
	switch f {
	case FocusEat:
		return NeedFood, true
	case FocusRelieve:
		return NeedBladder, true
	case FocusSocialize:
		return NeedSocial, true
	case FocusSleep:
		return NeedSleep, true
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
	case JobMine, JobBuild, JobClean, JobStore:
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
// Pressing need pressure and non-neutral affect drift every tick, so those
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
	if e.affect.Charge != 0 || e.affect.Grip != 0 {
		return w.tick + 1
	}
	sleepProgressOnly := e.focus == FocusSleep && e.Job == JobUse && e.Need == NeedSleep &&
		!e.carrying && e.useFacilitySet && e.Pos.Adjacent(e.useFacility) &&
		w.TerrainAt(e.useFacility) == w.cfg.Needs[NeedSleep].Facility
	for n := NeedKind(0); n < numNeeds; n++ {
		level := w.needLevel(e, n)
		phase := e.needPhase[n]
		if (phase == NeedPressing || phase == NeedCritical) && level < w.cfg.Needs[n].Max &&
			!(sleepProgressOnly && n == NeedSleep) {
			return w.tick + 1
		}
		if crossing := e.nextNeedPhaseTick[n]; crossing > w.tick && crossing < next {
			next = crossing
		}
	}
	return next
}

func (w *World) currentFocusEligible(e *Entity, threat *Entity) bool {
	switch e.focus {
	case FocusIdle:
		return true
	case FocusWork:
		return workJob(e.Job) || !e.resting || w.tick >= e.wakeTick
	case FocusEat, FocusRelieve, FocusSocialize, FocusSleep:
		need, _ := needForFocus(e.focus)
		phase := e.needPhase[need]
		if phase != NeedPressing && phase != NeedCritical {
			return false
		}
		if !w.cfg.Needs[need].Fatal {
			for n := NeedKind(0); n < numNeeds; n++ {
				if w.cfg.Needs[n].Fatal && (e.needPhase[n] == NeedPressing || e.needPhase[n] == NeedCritical) {
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
	if need, ok := needForFocus(e.focus); ok {
		selected.Need = need
	}
	if threat != nil && (e.focus == FocusFlee || e.focus == FocusFight) {
		selected.Threat = threat.ID
	}
	return selected
}

// focusCandidates fills caller-owned storage so normal arbitration allocates
// nothing. Shared facts (the visible threat and each lazy need level) are read
// once per call.
func (w *World) focusCandidates(e *Entity, out *[numFocusKinds]FocusCandidate) {
	for f := FocusKind(0); f < numFocusKinds; f++ {
		spec := w.cfg.Focuses[f]
		out[f] = FocusCandidate{
			Kind:     f,
			Eligible: f == FocusIdle,
			Score: FocusScore{
				Base:   spec.Base,
				Affect: affectContribution(e.affect.Charge, e.affect.Grip, spec, w.cfg.MoodMax),
			},
		}
	}

	out[FocusWork].Eligible = workJob(e.Job) || !e.resting || w.tick >= e.wakeTick
	for f := FocusKind(0); f < numFocusKinds; f++ {
		out[f].Score.Stimulus = e.stimulusFocusBias[f]
	}

	fatalPressing := false
	for n := NeedKind(0); n < numNeeds; n++ {
		level := w.needLevel(e, n)
		w.syncNeedPhaseAtLevel(e, n, level)
		spec := w.cfg.Needs[n]
		phase := e.needPhase[n]
		if phase != NeedPressing && phase != NeedCritical {
			continue
		}
		pressure := needPressure(level, spec)
		f := focusForNeed(n)
		c := &out[f]
		c.Need = n
		c.Eligible = true
		c.Score.Need = pressure * w.cfg.Focuses[f].NeedWeight / 100
		if phase == NeedCritical {
			c.Score.Need += w.cfg.FocusCriticalBonus
		}
		if spec.Fatal {
			c.Score.Need += w.cfg.FocusFatalBonus
			fatalPressing = true
		}
	}

	// Preserve the existing hard invariant that a pressing fatal need outranks
	// non-fatal needs. Threats remain eligible and can still dominate it.
	if fatalPressing {
		for n := NeedKind(0); n < numNeeds; n++ {
			if w.cfg.Needs[n].Fatal {
				continue
			}
			out[focusForNeed(n)].Eligible = false
		}
	}

	threat, hasThreat := w.nearestAlien(e.Pos, w.cfg.FleeRadius)
	if hasThreat {
		out[FocusFlee].Eligible = true
		out[FocusFlee].Threat = threat.ID
		if bestWeapon(e.Inventory) != ItemNone {
			out[FocusFight].Eligible = true
			out[FocusFight].Threat = threat.ID
			// Standing and firing has no locomotion cost when the target is in
			// range. Evasion always spends at least one movement step, which also
			// preserves the established armed-colonist behavior on an otherwise
			// exact tie.
			out[FocusFlee].Score.Distance = -w.cfg.Focuses[FocusFlee].DistanceWeight
		}
	}

	// A room cut off from the colony's main network for EscapeGraceTicks
	// straight is worth breaking out of on its own, ahead of even a fatal
	// need: reachability, not local coping, is what actually stayed broken,
	// and the nearest wall is very often the same one sealing off the very
	// facility that need is failing to reach. An immediate predator is the one
	// thing that still outranks it — self-preservation never waits on a wall.
	// See updateDisconnected and rooms.go's mainRoom.
	out[FocusEscape].Eligible = !hasThreat && e.disconnectedTicks >= w.cfg.EscapeGraceTicks

	if e.focus < numFocusKinds && out[e.focus].Eligible {
		out[e.focus].Score.Commitment = w.cfg.FocusCurrentBonus
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
	best := FocusCandidate{}
	found := false
	for i := range candidates {
		c := candidates[i]
		if !c.Eligible || found && !betterFocus(c, best, e.focus) {
			continue
		}
		best, found = c, true
	}

	if e.focus < numFocusKinds {
		current := candidates[e.focus]
		if current.Eligible && best.Kind != e.focus &&
			best.Score.Total() <= current.Score.Total()+w.cfg.FocusSwitchMargin {
			return current
		}
	}
	return best
}
