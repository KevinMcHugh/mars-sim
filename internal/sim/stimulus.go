package sim

// MaxActiveStimuli is the fixed storage reserved on each colonist. The
// configured limit may be lower, but may not exceed this allocation-free cap.
const MaxActiveStimuli = 16

// Stimulus is a bounded, transient appraisal of a recent or ongoing event.
// Source is zero for events that are not tied to a particular entity.
type Stimulus struct {
	Kind      LifeEventKind
	Source    EntityID
	Salience  int
	ExpiresAt int
}

// StimulusSpec declares the transient attention created by one life event.
// Contributions are the focus scores produced at salience 100.
type StimulusSpec struct {
	Salience     int
	Lifetime     int
	Contribution [numFocusKinds]int
}

var stimulusSpecs = [numLifeEventKinds]StimulusSpec{
	EvtSawAlien: {
		Salience: 100, Lifetime: 12,
		Contribution: [numFocusKinds]int{FocusFlee: 500, FocusFight: 500},
	},
	EvtBitten: {
		Salience: 100, Lifetime: 20,
		Contribution: [numFocusKinds]int{FocusFlee: 300, FocusFight: 150},
	},
	EvtWitnessedColonistKilled: {
		Salience: 90, Lifetime: 20,
		Contribution: [numFocusKinds]int{FocusFlee: 250, FocusFight: 100},
	},
	EvtWitnessedColonistAttacked: {
		Salience: 70, Lifetime: 12,
		Contribution: [numFocusKinds]int{FocusFlee: 200, FocusFight: 100},
	},
	EvtSawGore: {
		Salience: 35, Lifetime: 30,
		Contribution: [numFocusKinds]int{FocusWork: 20},
	},
	EvtFinishedMining:       finishedWorkStimulusSpec(),
	EvtClearedRock:          finishedWorkStimulusSpec(),
	EvtFinishedConstruction: finishedWorkStimulusSpec(),
	EvtCleanedRefuse:        finishedWorkStimulusSpec(),
	EvtIncineratedRefuse:    finishedWorkStimulusSpec(),
}

func finishedWorkStimulusSpec() StimulusSpec {
	return StimulusSpec{
		Salience: 25, Lifetime: 10,
		Contribution: [numFocusKinds]int{FocusWork: 15},
	}
}

// addStimulus inserts the configured appraisal for evt or refreshes the same
// (kind, source). It considers the incoming stimulus in eviction selection, so
// a weak new event cannot displace every stronger live event.
func (w *World) addStimulus(e *Entity, evt LifeEvent) bool {
	if evt.Kind >= numLifeEventKinds {
		return false
	}
	spec := stimulusSpecs[evt.Kind]
	if spec.Salience <= 0 || spec.Lifetime <= 0 || w.cfg.ActiveStimulusLimit <= 0 {
		return false
	}
	next := Stimulus{
		Kind: evt.Kind, Source: evt.Source, Salience: spec.Salience,
		ExpiresAt: w.tick + spec.Lifetime,
	}
	for i := 0; i < e.stimulusCount; i++ {
		if e.stimuli[i].Kind == next.Kind && e.stimuli[i].Source == next.Source {
			old := e.stimuli[i]
			changed := old != next
			e.stimuli[i] = next
			// Refreshing only the expiry of an ongoing perception leaves every
			// aggregate score unchanged. Recompute just the minimum deadline when
			// the refreshed entry used to own it.
			if old.Kind == next.Kind && old.Salience == next.Salience {
				if old.ExpiresAt == e.nextStimulusExpiry {
					w.refreshNextStimulusExpiry(e)
				}
			} else {
				w.refreshStimulusCache(e)
				w.markMindDirty(e)
			}
			return changed
		}
	}

	limit := w.cfg.ActiveStimulusLimit
	if limit > MaxActiveStimuli {
		limit = MaxActiveStimuli
	}
	if e.stimulusCount < limit {
		e.stimuli[e.stimulusCount] = next
		e.stimulusCount++
		e.addStimulusBias(next, 1)
		if e.nextStimulusExpiry == 0 || next.ExpiresAt < e.nextStimulusExpiry {
			e.nextStimulusExpiry = next.ExpiresAt
		}
		w.markMindDirty(e)
		return true
	}

	worst := 0
	for i := 1; i < e.stimulusCount; i++ {
		if stimulusWeaker(e.stimuli[i], e.stimuli[worst]) {
			worst = i
		}
	}
	if !stimulusWeaker(e.stimuli[worst], next) {
		return false
	}
	old := e.stimuli[worst]
	e.stimuli[worst] = next
	e.addStimulusBias(old, -1)
	e.addStimulusBias(next, 1)
	w.refreshNextStimulusExpiry(e)
	w.markMindDirty(e)
	return true
}

// stimulusWeaker implements the eviction order. Exact ties are equivalent for
// scoring; retaining the earlier slot makes the result stable without RNG.
func stimulusWeaker(a, b Stimulus) bool {
	if a.Salience != b.Salience {
		return a.Salience < b.Salience
	}
	if a.ExpiresAt != b.ExpiresAt {
		return a.ExpiresAt < b.ExpiresAt
	}
	return a.Source < b.Source
}

// expireStimuli removes entries at their exact boundary tick. Removal is stable
// so inspection and subsequent eviction remain deterministic.
func (w *World) expireStimuli(e *Entity) bool {
	write := 0
	for read := 0; read < e.stimulusCount; read++ {
		s := e.stimuli[read]
		if s.ExpiresAt <= w.tick {
			continue
		}
		e.stimuli[write] = s
		write++
	}
	if write == e.stimulusCount {
		return false
	}
	for i := write; i < e.stimulusCount; i++ {
		e.stimuli[i] = Stimulus{}
	}
	e.stimulusCount = write
	w.refreshStimulusCache(e)
	w.markMindDirty(e)
	return true
}

func (e *Entity) addStimulusBias(s Stimulus, scale int) {
	for f, contribution := range stimulusSpecs[s.Kind].Contribution {
		e.stimulusFocusBias[f] += scale * contribution * s.Salience / 100
	}
}

func (w *World) refreshNextStimulusExpiry(e *Entity) {
	e.nextStimulusExpiry = 0
	for i := 0; i < e.stimulusCount; i++ {
		expires := e.stimuli[i].ExpiresAt
		if expires > w.tick && (e.nextStimulusExpiry == 0 || expires < e.nextStimulusExpiry) {
			e.nextStimulusExpiry = expires
		}
	}
}

// refreshStimulusCache updates the aggregate score and earliest expiry in one
// bounded pass whenever fixed stimulus storage changes. Arbitration then reads
// both facts in O(numFocusKinds), independent of the buffer occupancy.
func (w *World) refreshStimulusCache(e *Entity) {
	e.stimulusFocusBias = [numFocusKinds]int{}
	e.nextStimulusExpiry = 0
	for i := 0; i < e.stimulusCount; i++ {
		s := e.stimuli[i]
		if s.ExpiresAt <= w.tick {
			continue
		}
		if e.nextStimulusExpiry == 0 || s.ExpiresAt < e.nextStimulusExpiry {
			e.nextStimulusExpiry = s.ExpiresAt
		}
		for f, contribution := range stimulusSpecs[s.Kind].Contribution {
			e.stimulusFocusBias[f] += contribution * s.Salience / 100
		}
	}
}

// stimulusBiases derives aggregates for component tests that directly populate
// stimulus storage. Production arbitration uses stimulusFocusBias instead.
func (w *World) stimulusBiases(e *Entity, out *[numFocusKinds]int) {
	*out = [numFocusKinds]int{}
	for i := 0; i < e.stimulusCount; i++ {
		s := e.stimuli[i]
		if s.ExpiresAt <= w.tick {
			continue
		}
		for f, contribution := range stimulusSpecs[s.Kind].Contribution {
			out[f] += contribution * s.Salience / 100
		}
	}
}

func (w *World) stimulusBias(e *Entity, focus FocusKind) int {
	var biases [numFocusKinds]int
	w.stimulusBiases(e, &biases)
	return biases[focus]
}
