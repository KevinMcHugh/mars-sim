package sim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultCognitionConfigFileName is the standard cognition and perception file.
const DefaultCognitionConfigFileName = "cognition.yaml"

// AttractorSpec declares one named region in affect space.
type AttractorSpec struct {
	Kind     MoodKind
	GoodName string
	BadName  string
	Charge   int
	Grip     int
	Radius   int
}

// StimulusSource selects which occurrence participant distinguishes concurrent
// stimuli for the same reaction. None intentionally coalesces every occurrence.
type StimulusSource string

const (
	StimulusSourceActor  StimulusSource = "actor"
	StimulusSourceObject StimulusSource = "object"
	StimulusSourceNone   StimulusSource = "none"
)

// EventStimulusSpec declares transient executive attention caused by a
// reaction. Contributions are focus scores at salience 100.
type EventStimulusSpec struct {
	Salience     int
	Lifetime     int
	Source       StimulusSource
	Contribution [numFocusKinds]int
}

// MemorySpec controls whether and how a matched reaction enters long-term
// memory. An empty Template uses text supplied by the occurrence. Collapse is
// the generic wording used after consecutive repetitions.
type MemorySpec struct {
	Template string
	Collapse string
}

// ReactionSpec maps one compositional percept to cognitive effects.
type ReactionSpec struct {
	ID       RuleID
	Priority int
	Match    PerceptPattern
	Tags     []TagID
	Memory   *MemorySpec
	Impact   int
	Target   MoodVector
	Stimulus *EventStimulusSpec
}

// RadiusID names a simulation distance knob available to perception rules.
type RadiusID string

const (
	RadiusFlee      RadiusID = "flee-radius"
	RadiusStomp     RadiusID = "stomp-radius"
	RadiusGoreSight RadiusID = "gore-sight-radius"
)

// PerceptionCadence controls when a matching world fact becomes a percept.
type PerceptionCadence string

const (
	CadenceInstant      PerceptionCadence = "instant"
	CadenceEnter        PerceptionCadence = "enter"
	CadenceEnterOngoing PerceptionCadence = "enter-and-ongoing"
)

// PerceptionRule turns a world occurrence or persistent state into an
// observer-relative percept.
type PerceptionRule struct {
	ID          RuleID
	Match       OccurrencePattern
	Channel     ChannelID
	Role        ObserverRole
	Radius      RadiusID
	Distance    int
	LineOfSight bool
	Cadence     PerceptionCadence
}

// TraitModifier applies deterministic percentage scales when a reaction's tags
// and an observer's trait match. Zero scales are normalized to 100 (identity).
type TraitModifier struct {
	ID      RuleID
	Trait   Trait
	AnyTags []TagID
	AllTags []TagID
	NotTags []TagID
	Impact  int
	Charge  int
	Grip    int
	Valence int
}

// ArbitrationConfig holds global focus-arbitration bonuses and limits.
type ArbitrationConfig struct {
	CurrentBonus        int
	SwitchMargin        int
	CriticalBonus       int
	FatalBonus          int
	ActiveStimulusLimit int
}

// NumMoodAttractors is the number of named spatial attractors.
const NumMoodAttractors = 9

// CognitionConfig is both the authoring model and the compiled runtime model.
// Slices retain deterministic declaration order. Maps are used only for exact
// lookup and never iterated by simulation code.
type CognitionConfig struct {
	Attractors  [NumMoodAttractors]AttractorSpec
	Focuses     [numFocusKinds]FocusSpec
	Arbitration ArbitrationConfig

	Nouns       []NounID
	Actions     []ActionID
	Tags        []TagID
	Perceptions []PerceptionRule
	Reactions   []ReactionSpec
	Modifiers   []TraitModifier

	reactionIndex map[RuleID]int
}

func (m MoodKind) String() string {
	switch m {
	case MoodDriven:
		return "driven"
	case MoodElated:
		return "elated"
	case MoodGiddy:
		return "giddy"
	case MoodAdrift:
		return "adrift"
	case MoodListless:
		return "listless"
	case MoodSpent:
		return "spent"
	case MoodContent:
		return "content"
	case MoodComposed:
		return "composed"
	case MoodSteady:
		return "steady"
	case MoodSettling:
		return "settling"
	default:
		return fmt.Sprintf("mood-%d", m)
	}
}

func ParseMoodKind(s string) (MoodKind, error) {
	switch canonicalID(s) {
	case "driven":
		return MoodDriven, nil
	case "elated":
		return MoodElated, nil
	case "giddy":
		return MoodGiddy, nil
	case "adrift":
		return MoodAdrift, nil
	case "listless":
		return MoodListless, nil
	case "spent":
		return MoodSpent, nil
	case "content":
		return MoodContent, nil
	case "composed":
		return MoodComposed, nil
	case "steady":
		return MoodSteady, nil
	case "settling":
		return MoodSettling, nil
	default:
		return 0, fmt.Errorf("unknown mood kind %q", s)
	}
}

func ParseFocusKind(s string) (FocusKind, error) {
	switch canonicalID(s) {
	case "idle":
		return FocusIdle, nil
	case "work":
		return FocusWork, nil
	case "eat":
		return FocusEat, nil
	case "relieve":
		return FocusRelieve, nil
	case "socialize":
		return FocusSocialize, nil
	case "sleep":
		return FocusSleep, nil
	case "flee":
		return FocusFlee, nil
	case "fight":
		return FocusFight, nil
	default:
		return 0, fmt.Errorf("unknown focus kind %q", s)
	}
}

func ParseTrait(s string) (Trait, error) {
	want := canonicalID(s)
	for trait := Trait(0); trait < numTraits; trait++ {
		if canonicalID(traitSpecs[trait].Name) == want {
			return trait, nil
		}
	}
	return 0, fmt.Errorf("unknown trait %q", s)
}

func canonicalID(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
}

func validKebabID(s string) bool {
	if s == "" || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	previousHyphen := false
	for _, r := range s {
		switch {
		case r == '-':
			if previousHyphen {
				return false
			}
			previousHyphen = true
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			previousHyphen = false
		default:
			return false
		}
	}
	return true
}

func reaction(id string, match PerceptPattern, impact int, target MoodVector, tags ...TagID) ReactionSpec {
	return ReactionSpec{
		ID: RuleID(id), Match: match, Tags: tags, Memory: &MemorySpec{},
		Impact: impact, Target: target,
	}
}

func directMatch(action ActionID, actor, object NounID, role ObserverRole) PerceptPattern {
	return PerceptPattern{
		OccurrencePattern: OccurrencePattern{ActorNoun: actor, Action: action, ObjectNoun: object},
		Channel:           ChannelDirect, Role: role, Phase: PhaseInstant,
	}
}

func sightMatch(action ActionID, actor, object NounID, phase PerceptPhase) PerceptPattern {
	return PerceptPattern{
		OccurrencePattern: OccurrencePattern{ActorNoun: actor, Action: action, ObjectNoun: object},
		Channel:           ChannelSight, Role: RoleWitness, Phase: phase,
	}
}

// DefaultCognitionConfig returns the shipped composition vocabulary and rules.
func DefaultCognitionConfig() CognitionConfig {
	var c CognitionConfig
	for i, a := range moodAttractors {
		c.Attractors[i] = AttractorSpec{
			Kind: a.Kind, GoodName: a.GoodName, BadName: a.BadName,
			Charge: a.Charge, Grip: a.Grip, Radius: a.Radius,
		}
	}
	c.Focuses = [numFocusKinds]FocusSpec{
		FocusIdle:      {Name: "idle", Base: 0, NeedWeight: 0, ChargeWeight: -10, GripWeight: 0, DistanceWeight: 0},
		FocusWork:      {Name: "work", Base: 25, NeedWeight: 0, ChargeWeight: 20, GripWeight: 10, DistanceWeight: 1},
		FocusEat:       {Name: "eat", Base: 40, NeedWeight: 100, ChargeWeight: 0, GripWeight: 5, DistanceWeight: 1},
		FocusRelieve:   {Name: "relieve", Base: 40, NeedWeight: 100, ChargeWeight: 0, GripWeight: 0, DistanceWeight: 1},
		FocusSocialize: {Name: "socialize", Base: 40, NeedWeight: 100, ChargeWeight: 10, GripWeight: 5, DistanceWeight: 1},
		FocusSleep:     {Name: "sleep", Base: 40, NeedWeight: 100, ChargeWeight: -30, GripWeight: 0, DistanceWeight: 1},
		FocusFlee:      {Name: "flee", Base: 0, NeedWeight: 0, ChargeWeight: 20, GripWeight: -40, DistanceWeight: 1},
		FocusFight:     {Name: "fight", Base: 15, NeedWeight: 0, ChargeWeight: 20, GripWeight: 40, DistanceWeight: 1},
	}
	c.Arbitration = ArbitrationConfig{
		CurrentBonus: 25, SwitchMargin: 10, CriticalBonus: 100,
		FatalBonus: 150, ActiveStimulusLimit: 8,
	}

	c.Nouns = []NounID{
		NounColonist, NounAlien, NounCat, NounMouse, NounGore, NounMeal,
		NounToilet, NounBed, NounNeed, NounRock, NounStructure, NounRefuse,
	}
	c.Actions = []ActionID{
		ActionPresent, ActionBite, ActionAttack, ActionKill, ActionCrush,
		ActionCatch, ActionWound, ActionConverse, ActionEat, ActionUse,
		ActionSleep, ActionSatisfy, ActionMine, ActionClear, ActionConstruct,
		ActionClean, ActionIncinerate, ActionMutate,
	}
	c.Tags = []TagID{TagGore, TagFinishedWork, TagConversation, TagMutation, TagIncineration}

	c.Perceptions = []PerceptionRule{
		{ID: "direct-actor", Channel: ChannelDirect, Role: RoleActor, Cadence: CadenceInstant},
		{ID: "direct-target", Channel: ChannelDirect, Role: RoleTarget, Cadence: CadenceInstant},
		{ID: "visible-alien", Match: OccurrencePattern{ActorNoun: NounAlien, Action: ActionPresent}, Channel: ChannelSight, Role: RoleWitness, Radius: RadiusFlee, Cadence: CadenceEnterOngoing},
		{ID: "visible-mouse", Match: OccurrencePattern{ActorNoun: NounMouse, Action: ActionPresent}, Channel: ChannelSight, Role: RoleWitness, Radius: RadiusStomp, Cadence: CadenceEnter},
		{ID: "visible-gore", Match: OccurrencePattern{ActorNoun: NounGore, Action: ActionPresent}, Channel: ChannelSight, Role: RoleWitness, Radius: RadiusGoreSight, Cadence: CadenceEnter},
		{ID: "witness-alien-kill", Match: OccurrencePattern{ActorNoun: NounAlien, Action: ActionKill, ObjectNoun: NounColonist}, Channel: ChannelSight, Role: RoleWitness, Radius: RadiusFlee, Cadence: CadenceInstant},
		{ID: "witness-alien-attack", Match: OccurrencePattern{ActorNoun: NounAlien, Action: ActionBite, ObjectNoun: NounColonist}, Channel: ChannelSight, Role: RoleWitness, Radius: RadiusFlee, Cadence: CadenceInstant},
		{ID: "witness-mouse-crushed", Match: OccurrencePattern{ActorNoun: NounColonist, Action: ActionCrush, ObjectNoun: NounMouse}, Channel: ChannelSight, Role: RoleWitness, Radius: RadiusStomp, Cadence: CadenceInstant},
		{ID: "witness-cat-catch", Match: OccurrencePattern{ActorNoun: NounCat, Action: ActionCatch, ObjectNoun: NounMouse}, Channel: ChannelSight, Role: RoleWitness, Radius: RadiusStomp, Cadence: CadenceInstant},
		{ID: "witness-alien-killed", Match: OccurrencePattern{ActorNoun: NounColonist, Action: ActionKill, ObjectNoun: NounAlien}, Channel: ChannelSight, Role: RoleWitness, Radius: RadiusFlee, Cadence: CadenceInstant},
		{ID: "witness-gunfight", Match: OccurrencePattern{ActorNoun: NounColonist, Action: ActionWound, ObjectNoun: NounAlien}, Channel: ChannelSight, Role: RoleWitness, Radius: RadiusFlee, Cadence: CadenceInstant},
		{ID: "witness-mutation", Match: OccurrencePattern{ActorNoun: NounColonist, Action: ActionMutate}, Channel: ChannelSight, Role: RoleWitness, Radius: RadiusFlee, Cadence: CadenceInstant},
	}

	sawAlien := reaction("saw-alien", sightMatch(ActionPresent, NounAlien, "", PhaseEnter), 45, MoodVector{8, -10, -15})
	sawAlien.Stimulus = &EventStimulusSpec{
		Salience: 100, Lifetime: 12, Source: StimulusSourceActor,
		Contribution: [numFocusKinds]int{FocusFlee: 500, FocusFight: 500},
	}
	sawMouse := reaction("saw-mouse", sightMatch(ActionPresent, NounMouse, "", PhaseEnter), 8, MoodVector{2, -3, 0})
	sawGore := reaction("saw-gore", sightMatch(ActionPresent, NounGore, "", PhaseEnter), 30, MoodVector{-3, -7, -6}, TagGore)
	sawGore.Stimulus = &EventStimulusSpec{
		Salience: 35, Lifetime: 30, Source: StimulusSourceNone,
		Contribution: [numFocusKinds]int{FocusWork: 20},
	}
	bitten := reaction("bitten", directMatch(ActionBite, NounAlien, NounColonist, RoleTarget), 70, MoodVector{55, -44, -30})
	bitten.Stimulus = &EventStimulusSpec{
		Salience: 100, Lifetime: 20, Source: StimulusSourceActor,
		Contribution: [numFocusKinds]int{FocusFlee: 300, FocusFight: 150},
	}
	witnessedKilled := reaction("witnessed-colonist-killed", sightMatch(ActionKill, NounAlien, NounColonist, PhaseInstant), 95, MoodVector{28, -74, -60}, TagGore)
	witnessedKilled.Stimulus = &EventStimulusSpec{
		Salience: 90, Lifetime: 20, Source: StimulusSourceActor,
		Contribution: [numFocusKinds]int{FocusFlee: 250, FocusFight: 100},
	}
	witnessedAttacked := reaction("witnessed-colonist-attacked", sightMatch(ActionBite, NounAlien, NounColonist, PhaseInstant), 75, MoodVector{35, -63, -40})
	witnessedAttacked.Stimulus = &EventStimulusSpec{
		Salience: 70, Lifetime: 12, Source: StimulusSourceActor,
		Contribution: [numFocusKinds]int{FocusFlee: 200, FocusFight: 100},
	}
	crushedMouse := reaction("crushed-mouse", directMatch(ActionCrush, NounColonist, NounMouse, RoleActor), 6, MoodVector{-1, 2, 0})
	witnessedMouse := reaction("witnessed-mouse-crushed", sightMatch(ActionCrush, NounColonist, NounMouse, PhaseInstant), 8, MoodVector{-1, -2, -1})
	witnessedCat := reaction("witnessed-cat-catch", sightMatch(ActionCatch, NounCat, NounMouse, PhaseInstant), 5, MoodVector{1, 1, 0})
	killedAlien := reaction("killed-alien", directMatch(ActionKill, NounColonist, NounAlien, RoleActor), 55, MoodVector{45, 52, 35})
	witnessedAlien := reaction("witnessed-alien-killed", sightMatch(ActionKill, NounColonist, NounAlien, PhaseInstant), 35, MoodVector{5, 6, 4})
	woundedAlien := reaction("wounded-alien", directMatch(ActionWound, NounColonist, NounAlien, RoleActor), 25, MoodVector{4, 5, 2})
	witnessedFight := reaction("witnessed-gunfight", sightMatch(ActionWound, NounColonist, NounAlien, PhaseInstant), 50, MoodVector{35, -25, -20})
	conversation := reaction("conversation", PerceptPattern{
		OccurrencePattern: OccurrencePattern{ActorNoun: NounColonist, Action: ActionConverse, ObjectNoun: NounColonist},
		Channel:           ChannelDirect, Phase: PhaseInstant,
	}, 20, MoodVector{}, TagConversation)
	ate := reaction("ate", directMatch(ActionEat, NounColonist, NounMeal, RoleActor), 10, MoodVector{4, 2, 1})
	usedToilet := reaction("used-toilet", directMatch(ActionUse, NounColonist, NounToilet, RoleActor), 4, MoodVector{1, 2, 0})
	slept := reaction("slept", directMatch(ActionSleep, NounColonist, NounBed, RoleActor), 25, MoodVector{15, 2, 2})
	needSatisfied := reaction("need-satisfied", directMatch(ActionSatisfy, NounColonist, NounNeed, RoleActor), 8, MoodVector{2, 2, 0})
	finishedMining := reaction("finished-mining", directMatch(ActionMine, NounColonist, NounRock, RoleActor), 15, MoodVector{-1, 5, 0}, TagFinishedWork)
	clearedRock := reaction("cleared-rock", directMatch(ActionClear, NounColonist, NounRock, RoleActor), 15, MoodVector{-1, 5, 0}, TagFinishedWork)
	finishedConstruction := reaction("finished-construction", directMatch(ActionConstruct, NounColonist, NounStructure, RoleActor), 18, MoodVector{-1, 6, 3}, TagFinishedWork)
	cleanedRefuse := reaction("cleaned-refuse", directMatch(ActionClean, NounColonist, NounRefuse, RoleActor), 14, MoodVector{-1, 5, 0}, TagFinishedWork)
	incineratedRefuse := reaction("incinerated-refuse", directMatch(ActionIncinerate, NounColonist, NounRefuse, RoleActor), 16, MoodVector{-1, 7, 1}, TagFinishedWork, TagIncineration)
	mutated := reaction("mutated", directMatch(ActionMutate, NounColonist, "", RoleActor), 60, MoodVector{18, -70, -35}, TagMutation)
	witnessedMutation := reaction("witnessed-mutation", sightMatch(ActionMutate, NounColonist, "", PhaseInstant), 35, MoodVector{2, -8, -10}, TagMutation)

	for _, r := range []*ReactionSpec{
		&finishedMining, &clearedRock, &finishedConstruction, &cleanedRefuse,
		&incineratedRefuse, &ate, &usedToilet, &slept, &needSatisfied,
	} {
		r.Memory.Collapse = map[RuleID]string{
			"finished-mining":       "Finished mining.",
			"cleared-rock":          "Cleared rock for a room.",
			"finished-construction": "Finished construction.",
			"cleaned-refuse":        "Cleaned up refuse.",
			"incinerated-refuse":    "Burned refuse in the incinerator.",
			"ate":                   "Had a meal.",
			"used-toilet":           "Used the toilet.",
			"slept":                 "Slept in a bed.",
			"need-satisfied":        "Satisfied a need.",
		}[r.ID]
	}
	workStimulus := func(r *ReactionSpec) {
		r.Stimulus = &EventStimulusSpec{
			Salience: 25, Lifetime: 10, Source: StimulusSourceNone,
			Contribution: [numFocusKinds]int{FocusWork: 15},
		}
	}
	workStimulus(&finishedMining)
	workStimulus(&clearedRock)
	workStimulus(&finishedConstruction)
	workStimulus(&cleanedRefuse)
	workStimulus(&incineratedRefuse)

	c.Reactions = []ReactionSpec{
		sawAlien, sawMouse, sawGore, bitten, witnessedKilled, witnessedAttacked,
		crushedMouse, witnessedMouse, witnessedCat, killedAlien, witnessedAlien,
		woundedAlien, witnessedFight, conversation, ate, usedToilet, slept,
		needSatisfied, finishedMining, clearedRock, finishedConstruction,
		cleanedRefuse, incineratedRefuse, mutated, witnessedMutation,
	}
	c.Modifiers = []TraitModifier{
		{ID: "industrious-finished-work", Trait: TraitIndustrious, AnyTags: []TagID{TagFinishedWork}, Charge: 200, Grip: 200, Valence: 200, Impact: 100},
		{ID: "introvert-conversation", Trait: TraitIntrovert, AnyTags: []TagID{TagConversation}, Charge: -100, Grip: 100, Valence: 100, Impact: 100},
		{ID: "tidy-gore", Trait: TraitTidy, AnyTags: []TagID{TagGore}, Charge: 220, Grip: 220, Valence: 220, Impact: 100},
		{ID: "tidy-incineration", Trait: TraitTidy, AnyTags: []TagID{TagIncineration}, Charge: 100, Grip: 200, Valence: 100, Impact: 100},
		{ID: "mutant-lover-mutation", Trait: TraitMutantLover, AnyTags: []TagID{TagMutation}, Charge: 100, Grip: -100, Valence: -100, Impact: 100},
	}
	if err := compileCognition(&c); err != nil {
		panic("invalid default cognition config: " + err.Error())
	}
	return c
}

func (c *CognitionConfig) reaction(id RuleID) (*ReactionSpec, bool) {
	i, ok := c.reactionIndex[id]
	if !ok || i < 0 || i >= len(c.Reactions) {
		return nil, false
	}
	return &c.Reactions[i], true
}

func (c *CognitionConfig) reactionFor(percept Percept) (*ReactionSpec, bool) {
	best := -1
	bestPriority, bestSpecificity := 0, 0
	for i := range c.Reactions {
		r := &c.Reactions[i]
		if !r.Match.matches(percept) {
			continue
		}
		specificity := r.Match.specificity()
		if best < 0 || r.Priority > bestPriority ||
			r.Priority == bestPriority && specificity > bestSpecificity {
			best, bestPriority, bestSpecificity = i, r.Priority, specificity
		}
	}
	if best < 0 {
		return nil, false
	}
	return &c.Reactions[best], true
}

func compileCognition(c *CognitionConfig) error {
	if err := validateVocabulary(c); err != nil {
		return err
	}
	c.reactionIndex = make(map[RuleID]int, len(c.Reactions))
	perceptionIDs := make(map[RuleID]bool, len(c.Perceptions))
	for i := range c.Perceptions {
		r := &c.Perceptions[i]
		if err := validateRuleID(r.ID, "perception"); err != nil {
			return err
		}
		if perceptionIDs[r.ID] {
			return fmt.Errorf("duplicate perception rule id %q", r.ID)
		}
		perceptionIDs[r.ID] = true
		if err := validateOccurrencePattern(c, r.Match, "perception "+string(r.ID)); err != nil {
			return err
		}
		if !validChannel(r.Channel) {
			return fmt.Errorf("perception %s: unknown channel %q", r.ID, r.Channel)
		}
		if !validRole(r.Role) {
			return fmt.Errorf("perception %s: unknown role %q", r.ID, r.Role)
		}
		if r.Cadence != CadenceInstant && r.Cadence != CadenceEnter && r.Cadence != CadenceEnterOngoing {
			return fmt.Errorf("perception %s: unknown cadence %q", r.ID, r.Cadence)
		}
		if r.Channel == ChannelDirect {
			if r.Radius != "" || r.Distance != 0 || r.LineOfSight || r.Cadence != CadenceInstant {
				return fmt.Errorf("perception %s: direct perception must be instant and have no radius", r.ID)
			}
		} else if !validRadius(r.Radius) && r.Distance <= 0 {
			return fmt.Errorf("perception %s: spatial perception needs a known radius or positive distance", r.ID)
		}
	}
	for i := range c.Reactions {
		r := &c.Reactions[i]
		if err := validateRuleID(r.ID, "reaction"); err != nil {
			return err
		}
		if _, exists := c.reactionIndex[r.ID]; exists {
			return fmt.Errorf("duplicate reaction id %q", r.ID)
		}
		c.reactionIndex[r.ID] = i
		if err := validatePerceptPattern(c, r.Match, "reaction "+string(r.ID)); err != nil {
			return err
		}
		if r.Impact < 0 || r.Impact > 100 {
			return fmt.Errorf("reaction %s: impact must be in 0..100", r.ID)
		}
		if r.Stimulus != nil && (r.Stimulus.Salience <= 0 || r.Stimulus.Lifetime <= 0) {
			return fmt.Errorf("reaction %s: stimulus salience and lifetime must be positive", r.ID)
		}
		if r.Stimulus != nil && r.Stimulus.Source != StimulusSourceActor &&
			r.Stimulus.Source != StimulusSourceObject && r.Stimulus.Source != StimulusSourceNone {
			return fmt.Errorf("reaction %s: unknown stimulus source %q", r.ID, r.Stimulus.Source)
		}
		for _, tag := range r.Tags {
			if !containsTag(c.Tags, tag) {
				return fmt.Errorf("reaction %s: unknown tag %q", r.ID, tag)
			}
		}
		for j := 0; j < i; j++ {
			other := &c.Reactions[j]
			if r.Priority == other.Priority && r.Match.specificity() == other.Match.specificity() &&
				patternsOverlap(r.Match, other.Match) {
				return fmt.Errorf("reactions %s and %s are ambiguous at priority %d", other.ID, r.ID, r.Priority)
			}
		}
	}
	modifierIDs := make(map[RuleID]bool, len(c.Modifiers))
	for i := range c.Modifiers {
		m := &c.Modifiers[i]
		if err := validateRuleID(m.ID, "modifier"); err != nil {
			return err
		}
		if modifierIDs[m.ID] {
			return fmt.Errorf("duplicate modifier id %q", m.ID)
		}
		modifierIDs[m.ID] = true
		if m.Trait >= numTraits {
			return fmt.Errorf("modifier %s: invalid trait", m.ID)
		}
		for _, tags := range [][]TagID{m.AnyTags, m.AllTags, m.NotTags} {
			for _, tag := range tags {
				if !containsTag(c.Tags, tag) {
					return fmt.Errorf("modifier %s: unknown tag %q", m.ID, tag)
				}
			}
		}
		normalizeModifier(m)
	}
	return nil
}

func validateVocabulary(c *CognitionConfig) error {
	if err := uniqueIDs("noun", nounStrings(c.Nouns)); err != nil {
		return err
	}
	if err := uniqueIDs("action", actionStrings(c.Actions)); err != nil {
		return err
	}
	return uniqueIDs("tag", tagStrings(c.Tags))
}

func uniqueIDs(kind string, ids []string) error {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !validKebabID(id) {
			return fmt.Errorf("%s id %q must be non-empty lowercase kebab-case", kind, id)
		}
		if seen[id] {
			return fmt.Errorf("duplicate %s id %q", kind, id)
		}
		seen[id] = true
	}
	return nil
}

func validateRuleID(id RuleID, kind string) error {
	if !validKebabID(string(id)) {
		return fmt.Errorf("%s id %q must be non-empty lowercase kebab-case", kind, id)
	}
	return nil
}

func validateOccurrencePattern(c *CognitionConfig, p OccurrencePattern, where string) error {
	if p.ActorNoun != "" && !containsNoun(c.Nouns, p.ActorNoun) {
		return fmt.Errorf("%s: unknown actor noun %q", where, p.ActorNoun)
	}
	if p.Action != "" && !containsAction(c.Actions, p.Action) {
		return fmt.Errorf("%s: unknown action %q", where, p.Action)
	}
	if p.ObjectNoun != "" && !containsNoun(c.Nouns, p.ObjectNoun) {
		return fmt.Errorf("%s: unknown object noun %q", where, p.ObjectNoun)
	}
	return nil
}

func validatePerceptPattern(c *CognitionConfig, p PerceptPattern, where string) error {
	if err := validateOccurrencePattern(c, p.OccurrencePattern, where); err != nil {
		return err
	}
	if p.Channel != "" && !validChannel(p.Channel) {
		return fmt.Errorf("%s: unknown channel %q", where, p.Channel)
	}
	if p.Role != "" && !validRole(p.Role) {
		return fmt.Errorf("%s: unknown role %q", where, p.Role)
	}
	if p.Phase != "" && p.Phase != PhaseInstant && p.Phase != PhaseEnter &&
		p.Phase != PhaseOngoing && p.Phase != PhaseExit {
		return fmt.Errorf("%s: unknown phase %q", where, p.Phase)
	}
	return nil
}

func validChannel(channel ChannelID) bool {
	return channel == ChannelDirect || channel == ChannelSight ||
		channel == ChannelHearing || channel == ChannelProximity
}

func validRole(role ObserverRole) bool {
	return role == RoleActor || role == RoleTarget || role == RoleWitness
}

func validRadius(radius RadiusID) bool {
	return radius == RadiusFlee || radius == RadiusStomp || radius == RadiusGoreSight
}

func containsNoun(values []NounID, want NounID) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsAction(values []ActionID, want ActionID) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsTag(values []TagID, want TagID) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func normalizeModifier(m *TraitModifier) {
	for _, scale := range []*int{&m.Impact, &m.Charge, &m.Grip, &m.Valence} {
		if *scale == 0 {
			*scale = 100
		}
	}
}

func nounStrings(values []NounID) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}

func actionStrings(values []ActionID) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}

func tagStrings(values []TagID) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}

// YAML authoring representation. Pointer fields preserve overlay semantics.
type yamlAttractor struct {
	Good   *string `yaml:"good,omitempty"`
	Bad    *string `yaml:"bad,omitempty"`
	Charge *int    `yaml:"charge,omitempty"`
	Grip   *int    `yaml:"grip,omitempty"`
	Radius *int    `yaml:"radius,omitempty"`
}

type yamlFocus struct {
	Base           *int `yaml:"base,omitempty"`
	NeedWeight     *int `yaml:"need_weight,omitempty"`
	ChargeWeight   *int `yaml:"charge_weight,omitempty"`
	GripWeight     *int `yaml:"grip_weight,omitempty"`
	DistanceWeight *int `yaml:"distance_weight,omitempty"`
}

type yamlTarget struct {
	Charge  int `yaml:"charge"`
	Grip    int `yaml:"grip"`
	Valence int `yaml:"valence"`
}

type yamlStimulus struct {
	Salience      int            `yaml:"salience"`
	Lifetime      int            `yaml:"lifetime"`
	Source        string         `yaml:"source"`
	Contributions map[string]int `yaml:"contributions,omitempty"`
}

type yamlOccurrenceMatch struct {
	ActorNoun  string `yaml:"actor_noun,omitempty"`
	Action     string `yaml:"action,omitempty"`
	ObjectNoun string `yaml:"object_noun,omitempty"`
}

type yamlPerceptMatch struct {
	ActorNoun  string `yaml:"actor_noun,omitempty"`
	Action     string `yaml:"action,omitempty"`
	ObjectNoun string `yaml:"object_noun,omitempty"`
	Channel    string `yaml:"channel,omitempty"`
	Role       string `yaml:"role,omitempty"`
	Phase      string `yaml:"phase,omitempty"`
}

type yamlSense struct {
	Channel     string `yaml:"channel"`
	Role        string `yaml:"role"`
	Radius      string `yaml:"radius,omitempty"`
	Distance    int    `yaml:"distance,omitempty"`
	LineOfSight bool   `yaml:"line_of_sight,omitempty"`
	Cadence     string `yaml:"cadence"`
}

type yamlPerception struct {
	ID       string               `yaml:"id"`
	Disabled bool                 `yaml:"disabled,omitempty"`
	Match    *yamlOccurrenceMatch `yaml:"match,omitempty"`
	Sense    *yamlSense           `yaml:"sense,omitempty"`
}

type yamlMemory struct {
	Record   *bool  `yaml:"record,omitempty"`
	Text     string `yaml:"text,omitempty"`
	Collapse string `yaml:"collapse,omitempty"`
}

type yamlAffect struct {
	Impact *int        `yaml:"impact,omitempty"`
	Target *yamlTarget `yaml:"target,omitempty"`
}

type yamlReaction struct {
	ID       string            `yaml:"id"`
	Disabled bool              `yaml:"disabled,omitempty"`
	Priority *int              `yaml:"priority,omitempty"`
	Match    *yamlPerceptMatch `yaml:"match,omitempty"`
	Tags     []string          `yaml:"tags,omitempty"`
	Memory   *yamlMemory       `yaml:"memory,omitempty"`
	Affect   *yamlAffect       `yaml:"affect,omitempty"`
	Stimulus *yamlStimulus     `yaml:"stimulus,omitempty"`
}

type yamlScales struct {
	Impact  int `yaml:"impact,omitempty"`
	Charge  int `yaml:"charge,omitempty"`
	Grip    int `yaml:"grip,omitempty"`
	Valence int `yaml:"valence,omitempty"`
}

type yamlModifier struct {
	ID       string      `yaml:"id"`
	Disabled bool        `yaml:"disabled,omitempty"`
	Trait    string      `yaml:"trait,omitempty"`
	AnyTags  []string    `yaml:"any_tags,omitempty"`
	AllTags  []string    `yaml:"all_tags,omitempty"`
	NotTags  []string    `yaml:"not_tags,omitempty"`
	Scales   *yamlScales `yaml:"scales,omitempty"`
}

type yamlRoot struct {
	SchemaVersion *int `yaml:"schema_version,omitempty"`
	Vocabulary    *struct {
		Nouns   []string `yaml:"nouns,omitempty"`
		Actions []string `yaml:"actions,omitempty"`
		Tags    []string `yaml:"tags,omitempty"`
	} `yaml:"vocabulary,omitempty"`
	Attractors  map[string]yamlAttractor `yaml:"attractors,omitempty"`
	Focuses     map[string]yamlFocus     `yaml:"focuses,omitempty"`
	Arbitration *struct {
		CurrentBonus        *int `yaml:"current_bonus,omitempty"`
		SwitchMargin        *int `yaml:"switch_margin,omitempty"`
		CriticalBonus       *int `yaml:"critical_bonus,omitempty"`
		FatalBonus          *int `yaml:"fatal_bonus,omitempty"`
		ActiveStimulusLimit *int `yaml:"active_stimulus_limit,omitempty"`
	} `yaml:"arbitration,omitempty"`
	Perceptions []yamlPerception `yaml:"perceptions,omitempty"`
	Reactions   []yamlReaction   `yaml:"reactions,omitempty"`
	Modifiers   []yamlModifier   `yaml:"modifiers,omitempty"`
}

// ApplyCognitionYAML strictly parses and overlays YAML on cfg.
func ApplyCognitionYAML(cfg *CognitionConfig, data []byte) error {
	var root yamlRoot
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&root); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("cognition config contains multiple YAML documents")
		}
		return err
	}
	if root.SchemaVersion != nil && *root.SchemaVersion != 1 {
		return fmt.Errorf("unsupported cognition schema_version %d", *root.SchemaVersion)
	}
	if root.Vocabulary != nil {
		if root.Vocabulary.Nouns != nil {
			cfg.Nouns = appendIDs(cfg.Nouns, root.Vocabulary.Nouns)
		}
		if root.Vocabulary.Actions != nil {
			cfg.Actions = appendActions(cfg.Actions, root.Vocabulary.Actions)
		}
		if root.Vocabulary.Tags != nil {
			cfg.Tags = appendTags(cfg.Tags, root.Vocabulary.Tags)
		}
	}
	for name, y := range root.Attractors {
		kind, err := ParseMoodKind(name)
		if err != nil {
			return fmt.Errorf("attractor %s: %w", name, err)
		}
		a := cfg.Attractors[kind]
		a.Kind = kind
		if y.Good != nil {
			a.GoodName = *y.Good
		}
		if y.Bad != nil {
			a.BadName = *y.Bad
		}
		if y.Charge != nil {
			a.Charge = *y.Charge
		}
		if y.Grip != nil {
			a.Grip = *y.Grip
		}
		if y.Radius != nil {
			a.Radius = *y.Radius
		}
		cfg.Attractors[kind] = a
	}
	for name, y := range root.Focuses {
		kind, err := ParseFocusKind(name)
		if err != nil {
			return fmt.Errorf("focus %s: %w", name, err)
		}
		f := cfg.Focuses[kind]
		f.Name = kind.String()
		if y.Base != nil {
			f.Base = *y.Base
		}
		if y.NeedWeight != nil {
			f.NeedWeight = *y.NeedWeight
		}
		if y.ChargeWeight != nil {
			f.ChargeWeight = *y.ChargeWeight
		}
		if y.GripWeight != nil {
			f.GripWeight = *y.GripWeight
		}
		if y.DistanceWeight != nil {
			f.DistanceWeight = *y.DistanceWeight
		}
		cfg.Focuses[kind] = f
	}
	if a := root.Arbitration; a != nil {
		if a.CurrentBonus != nil {
			cfg.Arbitration.CurrentBonus = *a.CurrentBonus
		}
		if a.SwitchMargin != nil {
			cfg.Arbitration.SwitchMargin = *a.SwitchMargin
		}
		if a.CriticalBonus != nil {
			cfg.Arbitration.CriticalBonus = *a.CriticalBonus
		}
		if a.FatalBonus != nil {
			cfg.Arbitration.FatalBonus = *a.FatalBonus
		}
		if a.ActiveStimulusLimit != nil {
			cfg.Arbitration.ActiveStimulusLimit = *a.ActiveStimulusLimit
		}
	}
	for _, y := range root.Perceptions {
		if err := applyYAMLPerception(cfg, y); err != nil {
			return err
		}
	}
	for _, y := range root.Reactions {
		if err := applyYAMLReaction(cfg, y); err != nil {
			return err
		}
	}
	for _, y := range root.Modifiers {
		if err := applyYAMLModifier(cfg, y); err != nil {
			return err
		}
	}
	return compileCognition(cfg)
}

func appendIDs(existing []NounID, values []string) []NounID {
	for _, value := range values {
		id := NounID(canonicalID(value))
		if !containsNoun(existing, id) {
			existing = append(existing, id)
		}
	}
	return existing
}

func appendActions(existing []ActionID, values []string) []ActionID {
	for _, value := range values {
		id := ActionID(canonicalID(value))
		if !containsAction(existing, id) {
			existing = append(existing, id)
		}
	}
	return existing
}

func appendTags(existing []TagID, values []string) []TagID {
	for _, value := range values {
		id := TagID(canonicalID(value))
		if !containsTag(existing, id) {
			existing = append(existing, id)
		}
	}
	return existing
}

func applyYAMLPerception(cfg *CognitionConfig, y yamlPerception) error {
	id := RuleID(canonicalID(y.ID))
	i := perceptionRuleIndex(cfg.Perceptions, id)
	if y.Disabled {
		if i >= 0 {
			cfg.Perceptions = append(cfg.Perceptions[:i], cfg.Perceptions[i+1:]...)
		}
		return nil
	}
	var rule PerceptionRule
	if i >= 0 {
		rule = cfg.Perceptions[i]
	} else {
		rule.ID = id
	}
	if y.Match != nil {
		rule.Match = OccurrencePattern{
			ActorNoun:  NounID(canonicalID(y.Match.ActorNoun)),
			Action:     ActionID(canonicalID(y.Match.Action)),
			ObjectNoun: NounID(canonicalID(y.Match.ObjectNoun)),
		}
	}
	if y.Sense != nil {
		rule.Channel = ChannelID(canonicalID(y.Sense.Channel))
		rule.Role = ObserverRole(canonicalID(y.Sense.Role))
		rule.Radius = RadiusID(canonicalID(y.Sense.Radius))
		rule.Distance = y.Sense.Distance
		rule.LineOfSight = y.Sense.LineOfSight
		rule.Cadence = PerceptionCadence(canonicalID(y.Sense.Cadence))
	}
	if i >= 0 {
		cfg.Perceptions[i] = rule
	} else {
		cfg.Perceptions = append(cfg.Perceptions, rule)
	}
	return nil
}

func applyYAMLReaction(cfg *CognitionConfig, y yamlReaction) error {
	id := RuleID(canonicalID(y.ID))
	i := reactionRuleIndex(cfg.Reactions, id)
	if y.Disabled {
		if i >= 0 {
			cfg.Reactions = append(cfg.Reactions[:i], cfg.Reactions[i+1:]...)
		}
		return nil
	}
	var rule ReactionSpec
	if i >= 0 {
		rule = cfg.Reactions[i]
	} else {
		rule.ID = id
	}
	if y.Priority != nil {
		rule.Priority = *y.Priority
	}
	if y.Match != nil {
		rule.Match = PerceptPattern{
			OccurrencePattern: OccurrencePattern{
				ActorNoun:  NounID(canonicalID(y.Match.ActorNoun)),
				Action:     ActionID(canonicalID(y.Match.Action)),
				ObjectNoun: NounID(canonicalID(y.Match.ObjectNoun)),
			},
			Channel: ChannelID(canonicalID(y.Match.Channel)),
			Role:    ObserverRole(canonicalID(y.Match.Role)),
			Phase:   PerceptPhase(canonicalID(y.Match.Phase)),
		}
	}
	if y.Tags != nil {
		rule.Tags = make([]TagID, len(y.Tags))
		for i, tag := range y.Tags {
			rule.Tags[i] = TagID(canonicalID(tag))
		}
	}
	if y.Memory != nil {
		record := true
		if y.Memory.Record != nil {
			record = *y.Memory.Record
		}
		if record {
			rule.Memory = &MemorySpec{Template: y.Memory.Text, Collapse: y.Memory.Collapse}
		} else {
			rule.Memory = nil
		}
	}
	if y.Affect != nil {
		if y.Affect.Impact != nil {
			rule.Impact = *y.Affect.Impact
		}
		if y.Affect.Target != nil {
			rule.Target = MoodVector{
				Charge: y.Affect.Target.Charge, Grip: y.Affect.Target.Grip,
				Valence: y.Affect.Target.Valence,
			}
		}
	}
	if y.Stimulus != nil {
		source := StimulusSource(canonicalID(y.Stimulus.Source))
		if source == "" {
			source = StimulusSourceActor
		}
		rule.Stimulus = &EventStimulusSpec{
			Salience: y.Stimulus.Salience, Lifetime: y.Stimulus.Lifetime,
			Source: source,
		}
		for name, value := range y.Stimulus.Contributions {
			focus, err := ParseFocusKind(name)
			if err != nil {
				return fmt.Errorf("reaction %s stimulus contribution: %w", id, err)
			}
			rule.Stimulus.Contribution[focus] = value
		}
	}
	if i >= 0 {
		cfg.Reactions[i] = rule
	} else {
		cfg.Reactions = append(cfg.Reactions, rule)
	}
	return nil
}

func applyYAMLModifier(cfg *CognitionConfig, y yamlModifier) error {
	id := RuleID(canonicalID(y.ID))
	i := modifierRuleIndex(cfg.Modifiers, id)
	if y.Disabled {
		if i >= 0 {
			cfg.Modifiers = append(cfg.Modifiers[:i], cfg.Modifiers[i+1:]...)
		}
		return nil
	}
	var rule TraitModifier
	if i >= 0 {
		rule = cfg.Modifiers[i]
	} else {
		rule.ID = id
	}
	if y.Trait != "" {
		trait, err := ParseTrait(y.Trait)
		if err != nil {
			return fmt.Errorf("modifier %s: %w", id, err)
		}
		rule.Trait = trait
	}
	if y.AnyTags != nil {
		rule.AnyTags = stringTags(y.AnyTags)
	}
	if y.AllTags != nil {
		rule.AllTags = stringTags(y.AllTags)
	}
	if y.NotTags != nil {
		rule.NotTags = stringTags(y.NotTags)
	}
	if y.Scales != nil {
		rule.Impact, rule.Charge, rule.Grip, rule.Valence =
			y.Scales.Impact, y.Scales.Charge, y.Scales.Grip, y.Scales.Valence
	}
	if i >= 0 {
		cfg.Modifiers[i] = rule
	} else {
		cfg.Modifiers = append(cfg.Modifiers, rule)
	}
	return nil
}

func stringTags(values []string) []TagID {
	out := make([]TagID, len(values))
	for i, value := range values {
		out[i] = TagID(canonicalID(value))
	}
	return out
}

func perceptionRuleIndex(rules []PerceptionRule, id RuleID) int {
	for i := range rules {
		if rules[i].ID == id {
			return i
		}
	}
	return -1
}

func reactionRuleIndex(rules []ReactionSpec, id RuleID) int {
	for i := range rules {
		if rules[i].ID == id {
			return i
		}
	}
	return -1
}

func modifierRuleIndex(rules []TraitModifier, id RuleID) int {
	for i := range rules {
		if rules[i].ID == id {
			return i
		}
	}
	return -1
}

// LoadCognitionConfigFile loads a cognition file from disk.
func LoadCognitionConfigFile(path string) (CognitionConfig, error) {
	cfg := DefaultCognitionConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := ApplyCognitionYAML(&cfg, data); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// CognitionVocabularyJSON is the machine-readable source for authoring tools.
// Passing a loaded config includes project-specific vocabulary extensions;
// with no argument it exports the compiled defaults.
func CognitionVocabularyJSON(configs ...CognitionConfig) []byte {
	def := DefaultCognitionConfig()
	if len(configs) > 0 {
		def = configs[0]
	}
	traits := make([]string, 0, numTraits)
	for trait := Trait(0); trait < numTraits; trait++ {
		traits = append(traits, canonicalID(trait.String()))
	}
	v := struct {
		SchemaVersion int `json:"schema_version"`
		Vocabulary    struct {
			Nouns    []string `json:"nouns"`
			Actions  []string `json:"actions"`
			Tags     []string `json:"tags"`
			Channels []string `json:"channels"`
			Roles    []string `json:"roles"`
			Phases   []string `json:"phases"`
			Cadences []string `json:"cadences"`
			Radii    []string `json:"radii"`
			Traits   []string `json:"traits"`
			Focuses  []string `json:"focuses"`
		} `json:"vocabulary"`
		Schema map[string][]string `json:"schema"`
	}{SchemaVersion: 1}
	v.Vocabulary.Nouns = nounStrings(def.Nouns)
	v.Vocabulary.Actions = actionStrings(def.Actions)
	v.Vocabulary.Tags = tagStrings(def.Tags)
	v.Vocabulary.Channels = []string{string(ChannelDirect), string(ChannelSight), string(ChannelHearing), string(ChannelProximity)}
	v.Vocabulary.Roles = []string{string(RoleActor), string(RoleTarget), string(RoleWitness)}
	v.Vocabulary.Phases = []string{string(PhaseInstant), string(PhaseEnter), string(PhaseOngoing), string(PhaseExit)}
	v.Vocabulary.Cadences = []string{string(CadenceInstant), string(CadenceEnter), string(CadenceEnterOngoing)}
	v.Vocabulary.Radii = []string{string(RadiusFlee), string(RadiusStomp), string(RadiusGoreSight)}
	v.Vocabulary.Traits = traits
	for focus := FocusKind(0); focus < numFocusKinds; focus++ {
		v.Vocabulary.Focuses = append(v.Vocabulary.Focuses, focus.String())
	}
	v.Schema = map[string][]string{
		"perception": {"id", "match", "sense"},
		"reaction":   {"id", "priority", "match", "tags", "memory", "affect", "stimulus"},
		"modifier":   {"id", "trait", "any_tags", "all_tags", "not_tags", "scales"},
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}

// CognitionConfigTemplate renders the shipped config in deterministic YAML.
func CognitionConfigTemplate() []byte {
	def := DefaultCognitionConfig()
	var b bytes.Buffer
	b.WriteString(`# mars-sim compositional perception and cognition settings
schema_version: 1

# Stable content vocabulary used by editor dropdowns and rule validation.
vocabulary:
  nouns: [`)
	writeInlineStrings(&b, nounStrings(def.Nouns))
	b.WriteString("]\n  actions: [")
	writeInlineStrings(&b, actionStrings(def.Actions))
	b.WriteString("]\n  tags: [")
	writeInlineStrings(&b, tagStrings(def.Tags))
	b.WriteString("]\n\nattractors:\n")
	for _, a := range def.Attractors {
		fmt.Fprintf(&b, "  %-10s: { good: %-12q, bad: %-12q, charge: %3d, grip: %3d, radius: %2d }\n",
			a.Kind.String(), a.GoodName, a.BadName, a.Charge, a.Grip, a.Radius)
	}
	b.WriteString("\nfocuses:\n")
	for i, f := range def.Focuses {
		fmt.Fprintf(&b, "  %-10s: { base: %3d, need_weight: %3d, charge_weight: %3d, grip_weight: %3d, distance_weight: %d }\n",
			FocusKind(i).String(), f.Base, f.NeedWeight, f.ChargeWeight, f.GripWeight, f.DistanceWeight)
	}
	fmt.Fprintf(&b, `
arbitration:
  current_bonus: %d
  switch_margin: %d
  critical_bonus: %d
  fatal_bonus: %d
  active_stimulus_limit: %d

# Perception rules say who can notice an occurrence or persistent state.
perceptions:
`, def.Arbitration.CurrentBonus, def.Arbitration.SwitchMargin,
		def.Arbitration.CriticalBonus, def.Arbitration.FatalBonus,
		def.Arbitration.ActiveStimulusLimit)
	for _, p := range def.Perceptions {
		fmt.Fprintf(&b, "  - id: %s\n", p.ID)
		if p.Match != (OccurrencePattern{}) {
			fmt.Fprintf(&b, "    match: { actor_noun: %s, action: %s",
				valueOrNull(string(p.Match.ActorNoun)), valueOrNull(string(p.Match.Action)))
			if p.Match.ObjectNoun != "" {
				fmt.Fprintf(&b, ", object_noun: %s", p.Match.ObjectNoun)
			}
			b.WriteString(" }\n")
		}
		fmt.Fprintf(&b, "    sense: { channel: %s, role: %s", p.Channel, p.Role)
		if p.Radius != "" {
			fmt.Fprintf(&b, ", radius: %s", p.Radius)
		}
		if p.Distance > 0 {
			fmt.Fprintf(&b, ", distance: %d", p.Distance)
		}
		if p.LineOfSight {
			b.WriteString(", line_of_sight: true")
		}
		fmt.Fprintf(&b, ", cadence: %s }\n", p.Cadence)
	}
	b.WriteString(`
# Reaction rules select one deterministic cognitive interpretation per percept.
reactions:
`)
	for _, r := range def.Reactions {
		fmt.Fprintf(&b, "  - id: %s\n", r.ID)
		if r.Priority != 0 {
			fmt.Fprintf(&b, "    priority: %d\n", r.Priority)
		}
		fmt.Fprintf(&b, "    match: { actor_noun: %s, action: %s",
			valueOrNull(string(r.Match.ActorNoun)), valueOrNull(string(r.Match.Action)))
		if r.Match.ObjectNoun != "" {
			fmt.Fprintf(&b, ", object_noun: %s", r.Match.ObjectNoun)
		}
		if r.Match.Channel != "" {
			fmt.Fprintf(&b, ", channel: %s", r.Match.Channel)
		}
		if r.Match.Role != "" {
			fmt.Fprintf(&b, ", role: %s", r.Match.Role)
		}
		if r.Match.Phase != "" {
			fmt.Fprintf(&b, ", phase: %s", r.Match.Phase)
		}
		b.WriteString(" }\n")
		if len(r.Tags) > 0 {
			b.WriteString("    tags: [")
			writeInlineStrings(&b, tagStrings(r.Tags))
			b.WriteString("]\n")
		}
		if r.Memory != nil {
			fmt.Fprintf(&b, "    memory: { record: true")
			if r.Memory.Template != "" {
				fmt.Fprintf(&b, ", text: %q", r.Memory.Template)
			}
			if r.Memory.Collapse != "" {
				fmt.Fprintf(&b, ", collapse: %q", r.Memory.Collapse)
			}
			b.WriteString(" }\n")
		} else {
			b.WriteString("    memory: { record: false }\n")
		}
		fmt.Fprintf(&b, "    affect: { impact: %d, target: { charge: %d, grip: %d, valence: %d } }\n",
			r.Impact, r.Target.Charge, r.Target.Grip, r.Target.Valence)
		if r.Stimulus != nil {
			fmt.Fprintf(&b, "    stimulus: { salience: %d, lifetime: %d, source: %s, contributions: {",
				r.Stimulus.Salience, r.Stimulus.Lifetime, r.Stimulus.Source)
			first := true
			for i, contribution := range r.Stimulus.Contribution {
				if contribution == 0 {
					continue
				}
				if !first {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "%s: %d", FocusKind(i), contribution)
				first = false
			}
			b.WriteString("} }\n")
		}
	}
	b.WriteString(`
# Trait modifiers compose over reaction tags. Scales are percentages.
modifiers:
`)
	for _, m := range def.Modifiers {
		fmt.Fprintf(&b, "  - id: %s\n    trait: %s\n", m.ID, canonicalID(m.Trait.String()))
		if len(m.AnyTags) > 0 {
			b.WriteString("    any_tags: [")
			writeInlineStrings(&b, tagStrings(m.AnyTags))
			b.WriteString("]\n")
		}
		if len(m.AllTags) > 0 {
			b.WriteString("    all_tags: [")
			writeInlineStrings(&b, tagStrings(m.AllTags))
			b.WriteString("]\n")
		}
		if len(m.NotTags) > 0 {
			b.WriteString("    not_tags: [")
			writeInlineStrings(&b, tagStrings(m.NotTags))
			b.WriteString("]\n")
		}
		fmt.Fprintf(&b, "    scales: { impact: %d, charge: %d, grip: %d, valence: %d }\n",
			m.Impact, m.Charge, m.Grip, m.Valence)
	}
	return b.Bytes()
}

func valueOrNull(value string) string {
	if value == "" {
		return `""`
	}
	return value
}

func writeInlineStrings(b *bytes.Buffer, values []string) {
	for i, value := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(value)
	}
}

// SortedReactionIDs is useful to tools and tests without exposing map order.
func (c *CognitionConfig) SortedReactionIDs() []RuleID {
	ids := make([]RuleID, 0, len(c.Reactions))
	for _, r := range c.Reactions {
		ids = append(ids, r.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
