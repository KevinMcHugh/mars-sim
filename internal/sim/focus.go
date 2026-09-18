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
	default:
		return "focus"
	}
}

// FocusSpec contains the tunable part of one focus's score. ChargeWeight and
// GripWeight are introduced now so configuration remains stable when affect
// starts contributing in Phase 4; scalar mood never feeds either value.
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

func workJob(job JobKind) bool {
	switch job {
	case JobMine, JobBuild, JobClean, JobStore:
		return true
	default:
		return false
	}
}

// needPressureBeforePhases is Phase 1's temporary urgency projection. Phase 2
// replaces it with the documented NeedPhase/CriticalAt normalization.
func needPressureBeforePhases(level int, spec NeedSpec) int {
	if level < spec.SeekAt {
		return 0
	}
	if level >= spec.Max {
		return 100
	}
	return clampInt(1+99*(level-spec.SeekAt)/atLeast1(spec.Max-spec.SeekAt), 1, 100)
}

// focusCandidates fills caller-owned storage so normal arbitration allocates
// nothing. Shared facts (the visible threat and each lazy need level) are read
// once per call.
func (w *World) focusCandidates(e *Entity, out *[numFocusKinds]FocusCandidate) {
	for f := FocusKind(0); f < numFocusKinds; f++ {
		out[f] = FocusCandidate{
			Kind:     f,
			Eligible: f == FocusIdle,
			Score:    FocusScore{Base: w.cfg.Focuses[f].Base},
		}
	}

	out[FocusWork].Eligible = workJob(e.Job) || !e.resting || w.tick >= e.wakeTick

	fatalPressing := false
	for n := NeedKind(0); n < numNeeds; n++ {
		spec := w.cfg.Needs[n]
		pressure := needPressureBeforePhases(w.needLevel(e, n), spec)
		if pressure == 0 {
			continue
		}
		f := focusForNeed(n)
		c := &out[f]
		c.Need = n
		c.Eligible = true
		c.Score.Need = pressure * w.cfg.Focuses[f].NeedWeight / 100
		if pressure == 100 {
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

	if threat, ok := w.nearestAlien(e.Pos, w.cfg.FleeRadius); ok {
		out[FocusFlee].Eligible = true
		out[FocusFlee].Threat = threat.ID
		out[FocusFlee].Score.Stimulus = 500
		if bestWeapon(e.Inventory) != ItemNone {
			out[FocusFight].Eligible = true
			out[FocusFight].Threat = threat.ID
			out[FocusFight].Score.Stimulus = 500
			// Standing and firing has no locomotion cost when the target is in
			// range. Evasion always spends at least one movement step, which also
			// preserves the established armed-colonist behavior on an otherwise
			// exact tie.
			out[FocusFlee].Score.Distance = -w.cfg.Focuses[FocusFlee].DistanceWeight
		}
	}

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
