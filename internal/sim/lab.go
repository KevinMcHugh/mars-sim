package sim

import (
	"fmt"
	"math/rand/v2"
)

// The lab surface is the part of the sim a browser bench can call without a
// world: one colonist roll, and one focus decision. World.assignPersonality and
// World.chooseFocus call the same functions. Reachability of a pod, a toilet,
// a bed, or a person is a bench gate — chooseFocus does not apply it — so it
// is applied here, after the real scores, and only clears eligibility.

// LabTrait is one row of the trait table, in declaration order.
type LabTrait struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Desc     string `json:"desc"`
	Group    string `json:"group"`
	Acquired bool   `json:"acquired"`
}

// LabNeed is one need's thresholds. Levels are sim config, not cognition.yaml.
type LabNeed struct {
	ID     string `json:"id"`
	SeekAt int    `json:"seekAt"`
	CritAt int    `json:"critAt"`
	Max    int    `json:"max"`
	Fatal  bool   `json:"fatal"`
}

// LabPerson is one rolled colonist. The seed is echoed so the bench can show it.
type LabPerson struct {
	Seed        int64    `json:"seed"`
	Name        string   `json:"name"`
	Age         int      `json:"age"`
	Gender      string   `json:"gender"`
	Pronouns    string   `json:"pronouns"`
	Orientation string   `json:"orientation"`
	HeightCM    int      `json:"heightCM"`
	WeightKG    int      `json:"weightKG"`
	Skin        string   `json:"skin"`
	Hair        string   `json:"hair"`
	Traits      []string `json:"traits"`
}

// LabFocus is the authored score of one focus, as the bench is editing it.
type LabFocus struct {
	Base           int `json:"base"`
	NeedWeight     int `json:"needWeight"`
	ChargeWeight   int `json:"chargeWeight"`
	GripWeight     int `json:"gripWeight"`
	DistanceWeight int `json:"distanceWeight"`
}

// LabArbitration is the global bonus block from the cognition file.
type LabArbitration struct {
	CurrentBonus  int `json:"currentBonus"`
	SwitchMargin  int `json:"switchMargin"`
	CriticalBonus int `json:"criticalBonus"`
	FatalBonus    int `json:"fatalBonus"`
}

// LabAttractor is one mood region, in the order the file declares them.
type LabAttractor struct {
	Kind   string `json:"kind"`
	Good   string `json:"good"`
	Bad    string `json:"bad"`
	Charge int    `json:"charge"`
	Grip   int    `json:"grip"`
	Radius int    `json:"radius"`
}

// LabStimulus is one live stimulus and the contributions the reaction declares.
type LabStimulus struct {
	Kind          string         `json:"kind"`
	Salience      int            `json:"salience"`
	Contributions map[string]int `json:"contributions"`
}

// LabCognition is the slice of the open file that scoring and the mood word read.
type LabCognition struct {
	Focuses     map[string]LabFocus `json:"focuses"`
	Arbitration LabArbitration      `json:"arbitration"`
	Attractors  []LabAttractor      `json:"attractors"`
	Stimuli     []LabStimulus       `json:"stimuli"`
}

// LabSituation is the story on the bench. Facility flags are the bench gate.
type LabSituation struct {
	Charge    int            `json:"charge"`
	Grip      int            `json:"grip"`
	Valence   int            `json:"valence"`
	MoodLabel string         `json:"moodLabel"`
	Needs     map[string]int `json:"needs"`
	Traits    []string       `json:"traits"`
	Current   string         `json:"current"`
	CanWork   bool           `json:"canWork"`
	Pod       bool           `json:"pod"`
	Toilet    bool           `json:"toilet"`
	Bed       bool           `json:"bed"`
	Company   bool           `json:"company"`
	Alien     bool           `json:"alien"`
	Armed     bool           `json:"armed"`
	Sealed    bool           `json:"sealed"`
}

// LabCandidate is one focus after scoring, the bench gate, and the real chooser.
type LabCandidate struct {
	ID         string   `json:"id"`
	Eligible   bool     `json:"eligible"`
	Reasons    []string `json:"reasons"`
	Base       int      `json:"base"`
	Need       int      `json:"need"`
	Affect     int      `json:"affect"`
	Stimulus   int      `json:"stimulus"`
	Commitment int      `json:"commitment"`
	Distance   int      `json:"distance"`
	Total      int      `json:"total"`
}

// LabRef names a focus in the verdict.
type LabRef struct {
	ID string `json:"id"`
}

// LabMood is the trait baseline, clamped to the sim's mood range.
type LabMood struct {
	Charge  int `json:"charge"`
	Grip    int `json:"grip"`
	Valence int `json:"valence"`
}

// LabWord is the mood label the colonist is showing.
type LabWord struct {
	Label string `json:"label"`
	Word  string `json:"word"`
}

// LabVerdict is what the Focus Tester renders. Lead is the best eligible
// focus before the switch margin holds the current one.
type LabVerdict struct {
	Candidates []LabCandidate    `json:"candidates"`
	Current    LabRef            `json:"current"`
	Winner     LabRef            `json:"winner"`
	Lead       LabRef            `json:"lead"`
	Held       bool              `json:"held"`
	Switched   bool              `json:"switched"`
	Margin     int               `json:"margin"`
	Phases     map[string]string `json:"phases"`
	Mood       LabWord           `json:"mood"`
	Home       LabMood           `json:"home"`
}

// LabSettings is the shipped need table and the three personality/mood
// constants the bench has to share with the sim. It does not build a world
// or the cognition file.
func LabSettings() (needs [numNeeds]NeedSpec, moodMax, moodMargin, traitChance int) {
	return defaultNeeds(), defaultMoodMax, defaultMoodLabelSwitchMargin, defaultTraitChance
}

// LabTraits returns the trait table. Ids are the kebab-case names the file uses.
func LabTraits() []LabTrait {
	out := make([]LabTrait, 0, numTraits)
	for t := Trait(0); t < numTraits; t++ {
		s := traitSpecs[t]
		out = append(out, LabTrait{
			ID:       canonicalID(s.Name),
			Name:     s.Name,
			Desc:     s.Desc,
			Group:    s.group.labID(),
			Acquired: s.acquired,
		})
	}
	return out
}

// LabNeeds returns the shipped need thresholds.
func LabNeeds(needs [numNeeds]NeedSpec) []LabNeed {
	out := make([]LabNeed, 0, numNeeds)
	for n := NeedKind(0); n < numNeeds; n++ {
		spec := needs[n]
		out = append(out, LabNeed{
			ID: spec.Name, SeekAt: spec.SeekAt, CritAt: spec.CriticalAt, Max: spec.Max, Fatal: spec.Fatal,
		})
	}
	return out
}

// LabRoll draws one colonist on fresh personality and age streams keyed the
// same way World.prng and World.agePRNG are. It is not colonist N of a world:
// worldgen spends those streams on family and heredity before the person.
func LabRoll(seed int64, traitChance int) LabPerson {
	prng := rand.New(newPCG(seed ^ 0x5DEECE66D))
	age := rand.New(newPCG(seed ^ 0x6A09E667))
	p := rollProfile(prng, age, traitChance)
	traits := make([]string, len(p.Traits))
	for i, t := range p.Traits {
		traits[i] = canonicalID(t.Name())
	}
	return LabPerson{
		Seed: seed, Name: p.Name, Age: p.Age,
		Gender: p.Gender.String(), Pronouns: p.Gender.Pronouns(),
		Orientation: p.Orientation.String(),
		HeightCM:    p.HeightCM, WeightKG: p.WeightKG,
		Skin: p.SkinTone.String(), Hair: p.HairColor.String(),
		Traits: traits,
	}
}

// LabEvaluate scores the situation with the sim's focus function, then applies
// the bench's facility gate and the sim's chooser.
func LabEvaluate(needs [numNeeds]NeedSpec, moodMax, moodMargin int, cog LabCognition, sit LabSituation) (LabVerdict, error) {
	var focuses [numFocusKinds]FocusSpec
	for name, spec := range cog.Focuses {
		kind, err := ParseFocusKind(name)
		if err != nil {
			return LabVerdict{}, err
		}
		focuses[kind] = FocusSpec{
			Name: name, Base: spec.Base, NeedWeight: spec.NeedWeight,
			ChargeWeight: spec.ChargeWeight, GripWeight: spec.GripWeight,
			DistanceWeight: spec.DistanceWeight,
		}
	}
	current, err := ParseFocusKind(sit.Current)
	if err != nil {
		return LabVerdict{}, err
	}
	var level [numNeeds]int
	var phase [numNeeds]NeedPhase
	phases := make(map[string]string, numNeeds)
	for n := NeedKind(0); n < numNeeds; n++ {
		level[n] = sit.Needs[n.String()]
		phase[n] = phaseForLevel(level[n], needs[n])
		phases[n.String()] = phase[n].String()
	}
	var stimulus [numFocusKinds]int
	for _, stim := range cog.Stimuli {
		var contrib [numFocusKinds]int
		for name, value := range stim.Contributions {
			kind, err := ParseFocusKind(name)
			if err != nil {
				return LabVerdict{}, fmt.Errorf("stimulus %s: %w", stim.Kind, err)
			}
			contrib[kind] = value
		}
		accumulateStimulusBias(&stimulus, contrib, stim.Salience)
	}
	traits := make([]Trait, 0, len(sit.Traits))
	for _, id := range sit.Traits {
		trait, err := ParseTrait(id)
		if err != nil {
			return LabVerdict{}, err
		}
		traits = append(traits, trait)
	}
	attractors := make([]AttractorSpec, 0, len(cog.Attractors))
	for _, a := range cog.Attractors {
		kind, err := ParseMoodKind(a.Kind)
		if err != nil {
			return LabVerdict{}, err
		}
		attractors = append(attractors, AttractorSpec{
			Kind: kind, GoodName: a.Good, BadName: a.Bad,
			Charge: a.Charge, Grip: a.Grip, Radius: a.Radius,
		})
	}
	home := clampedAffectHome(traits, moodMax)
	label, hasLabel := MoodKind(0), false
	if sit.MoodLabel != "" {
		parsed, err := ParseMoodKind(sit.MoodLabel)
		if err != nil {
			return LabVerdict{}, err
		}
		label, hasLabel = parsed, true
	}
	moodKind, word, _ := moodReadout(attractors, sit.Charge, sit.Grip, sit.Valence, label, hasLabel, moodMargin)

	in := focusInputs{
		focuses: focuses, needs: needs, level: level, phase: phase,
		charge: sit.Charge, grip: sit.Grip, moodMax: moodMax, stimulus: stimulus,
		current: current, workEligible: sit.CanWork, threat: sit.Alien, armed: sit.Armed,
		escape:       sit.Sealed && !sit.Alien,
		currentBonus: cog.Arbitration.CurrentBonus, criticalBonus: cog.Arbitration.CriticalBonus,
		fatalBonus: cog.Arbitration.FatalBonus,
	}
	var candidates [numFocusKinds]FocusCandidate
	fillFocusCandidates(in, &candidates)
	reach := labReach{pod: sit.Pod, toilet: sit.Toilet, bed: sit.Bed, company: sit.Company}
	applyBenchReach(&candidates, current, reach)
	winner := selectFocus(&candidates, current, cog.Arbitration.SwitchMargin)
	lead, leadOK := leadingFocus(&candidates, current)

	out := make([]LabCandidate, 0, numFocusKinds)
	for f := FocusKind(0); f < numFocusKinds; f++ {
		c := candidates[f]
		out = append(out, LabCandidate{
			ID: c.Kind.String(), Eligible: c.Eligible,
			Reasons: explainFocus(f, in, reach),
			Base:    c.Score.Base, Need: c.Score.Need, Affect: c.Score.Affect,
			Stimulus: c.Score.Stimulus, Commitment: c.Score.Commitment,
			Distance: c.Score.Distance, Total: c.Score.Total(),
		})
	}
	leadID := winner.Kind.String()
	if leadOK {
		leadID = lead.Kind.String()
	}
	return LabVerdict{
		Candidates: out,
		Current:    LabRef{ID: current.String()},
		Winner:     LabRef{ID: winner.Kind.String()},
		Lead:       LabRef{ID: leadID},
		Held:       winner.Kind == current && leadOK && lead.Kind != current,
		Switched:   winner.Kind != current,
		Margin:     cog.Arbitration.SwitchMargin,
		Phases:     phases,
		Mood:       LabWord{Label: moodKind.String(), Word: word},
		Home:       LabMood{Charge: home.Charge, Grip: home.Grip, Valence: home.Valence},
	}, nil
}

type labReach struct {
	pod, toilet, bed, company bool
}

func (r labReach) allows(f FocusKind) bool {
	switch f {
	case FocusEat:
		return r.pod
	case FocusRelieve:
		return r.toilet
	case FocusSleep:
		return r.bed
	case FocusSocialize:
		return r.company
	default:
		return true
	}
}

func applyBenchReach(out *[numFocusKinds]FocusCandidate, current FocusKind, reach labReach) {
	for f := FocusKind(0); f < numFocusKinds; f++ {
		if reach.allows(f) {
			continue
		}
		out[f].Eligible = false
		if f == current {
			out[f].Score.Commitment = 0
		}
	}
}

func explainFocus(f FocusKind, in focusInputs, reach labReach) []string {
	switch f {
	case FocusIdle:
		return []string{"Always available."}
	case FocusWork:
		if !in.workEligible {
			return []string{"They're resting, and they have no job to finish."}
		}
		return []string{"Awake, or already on a job."}
	case FocusFlee:
		if !in.threat {
			return []string{"No alien in sight."}
		}
		return []string{"An alien is in sight."}
	case FocusFight:
		var reasons []string
		if !in.threat {
			reasons = append(reasons, "No alien in sight.")
		}
		if !in.armed {
			reasons = append(reasons, "They aren't carrying a weapon.")
		}
		if len(reasons) == 0 {
			return []string{"An alien is in sight, and they're armed."}
		}
		return reasons
	case FocusEscape:
		if in.threat {
			return []string{"An alien is right there. Getting away from it comes first."}
		}
		if !in.escape {
			return []string{"The room still connects to the rest of the colony."}
		}
		return []string{"Sealed off from the colony, and nothing is hunting them."}
	}

	need, ok := needForFocus(f)
	if !ok {
		return []string{"Available."}
	}
	var reasons []string
	phase := in.phase[need]
	hot := phase == NeedPressing || phase == NeedCritical
	if !hot {
		reasons = append(reasons, notPressingLine(f, phase))
	}
	foodHot := in.needs[NeedFood].Fatal &&
		(in.phase[NeedFood] == NeedPressing || in.phase[NeedFood] == NeedCritical)
	if foodHot && f != FocusEat {
		reasons = append(reasons, "Hunger is pressing, and it outranks this.")
	}
	if !reach.allows(f) {
		reasons = append(reasons, reachMissing(f))
	}
	if len(reasons) == 0 {
		return []string{pressingLine(f, phase)}
	}
	return reasons
}

func notPressingLine(f FocusKind, phase NeedPhase) string {
	quiet := phase == NeedSatisfied || phase == NeedGrowing
	switch f {
	case FocusEat:
		if quiet {
			return "They aren't hungry enough to go eat."
		}
		return "Hunger isn't pressing."
	case FocusRelieve:
		if quiet {
			return "They don't need a toilet yet."
		}
		return "Their bladder isn't pressing."
	case FocusSleep:
		if quiet {
			return "They aren't tired enough to sleep."
		}
		return "Sleep isn't pressing."
	case FocusSocialize:
		if quiet {
			return "They don't need company yet."
		}
		return "The social need isn't pressing."
	default:
		return "Not pressing."
	}
}

func pressingLine(f FocusKind, phase NeedPhase) string {
	word := "pressing"
	if phase == NeedCritical {
		word = "critical"
	}
	switch f {
	case FocusEat:
		return "Hunger is " + word + ", and a nutrient pod is in reach."
	case FocusRelieve:
		return "Their bladder is " + word + ", and a toilet is in reach."
	case FocusSleep:
		return "Sleep is " + word + ", and a bed is in reach."
	case FocusSocialize:
		return "They want company, and someone is nearby."
	default:
		return "Available."
	}
}

func reachMissing(f FocusKind) string {
	switch f {
	case FocusEat:
		return "No nutrient pod nearby."
	case FocusRelieve:
		return "No toilet nearby."
	case FocusSleep:
		return "No bed nearby."
	case FocusSocialize:
		return "Nobody nearby to talk to."
	default:
		return "Not in reach."
	}
}
