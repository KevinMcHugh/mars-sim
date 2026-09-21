package sim

import (
	"fmt"
	"strings"
)

// NounID and ActionID are the stable content vocabulary used by occurrences.
// They deliberately describe parts of an event rather than pre-composed event
// kinds: adding "play" and "cat" makes both doing and witnessing cat play
// expressible without another Go enum value.
type NounID string
type ActionID string
type TagID string
type RuleID string

const (
	NounColonist  NounID = "colonist"
	NounAlien     NounID = "alien"
	NounCat       NounID = "cat"
	NounMouse     NounID = "mouse"
	NounGore      NounID = "gore"
	NounMeal      NounID = "meal"
	NounToilet    NounID = "toilet"
	NounBed       NounID = "bed"
	NounNeed      NounID = "need"
	NounRock      NounID = "rock"
	NounStructure NounID = "structure"
	NounRefuse    NounID = "refuse"
)

const (
	ActionPresent    ActionID = "present"
	ActionBite       ActionID = "bite"
	ActionAttack     ActionID = "attack"
	ActionKill       ActionID = "kill"
	ActionCrush      ActionID = "crush"
	ActionCatch      ActionID = "catch"
	ActionWound      ActionID = "wound"
	ActionConverse   ActionID = "converse"
	ActionEat        ActionID = "eat"
	ActionUse        ActionID = "use"
	ActionSleep      ActionID = "sleep"
	ActionSatisfy    ActionID = "satisfy"
	ActionMine       ActionID = "mine"
	ActionClear      ActionID = "clear"
	ActionConstruct  ActionID = "construct"
	ActionClean      ActionID = "clean"
	ActionIncinerate ActionID = "incinerate"
	ActionMutate     ActionID = "mutate"
)

const (
	TagGore         TagID = "gore"
	TagFinishedWork TagID = "finished-work"
	TagConversation TagID = "conversation"
	TagMutation     TagID = "mutation"
	TagIncineration TagID = "incineration"
)

// ChannelID says how an observer learned about an occurrence. Direct is
// reserved for a participant's own action or experience; the other channels
// are produced by configurable spatial perception rules.
type ChannelID string

const (
	ChannelDirect    ChannelID = "direct"
	ChannelSight     ChannelID = "sight"
	ChannelHearing   ChannelID = "hearing"
	ChannelProximity ChannelID = "proximity"
)

// ObserverRole is the observer's relationship to an occurrence. Role is kept
// separate from channel so "performed cat play" and "saw cat play" share one
// underlying occurrence.
type ObserverRole string

const (
	RoleActor   ObserverRole = "actor"
	RoleTarget  ObserverRole = "target"
	RoleWitness ObserverRole = "witness"
)

// PerceptPhase distinguishes instantaneous actions from persistent context.
// Ongoing percepts refresh attention but do not create another mood hit or
// memory; leaving and re-entering produces a new Enter percept.
type PerceptPhase string

const (
	PhaseInstant PerceptPhase = "instant"
	PhaseEnter   PerceptPhase = "enter"
	PhaseOngoing PerceptPhase = "ongoing"
	PhaseExit    PerceptPhase = "exit"
)

// FactRef is one participant in an occurrence. Label is captured before a
// participant can be removed from the world, so a fatal event can still render
// a useful memory.
type FactRef struct {
	Noun   NounID
	Entity EntityID
	Label  string
}

func (w *World) factRef(e *Entity) FactRef {
	if e == nil {
		return FactRef{}
	}
	return FactRef{Noun: nounForKind(e.Kind), Entity: e.ID, Label: e.displayName()}
}

func nounForKind(kind Kind) NounID {
	switch kind {
	case Colonist:
		return NounColonist
	case Alien:
		return NounAlien
	case Cat:
		return NounCat
	case Mouse:
		return NounMouse
	default:
		return ""
	}
}

// ObserverAppraisal carries a contextual target for one observer. Conversation
// outcome is the first user: the same occurrence can land differently for each
// participant because social fatigue is personal.
type ObserverAppraisal struct {
	Observer EntityID
	Target   MoodVector
}

// Occurrence is a world fact emitted once by feature code. It contains no
// cognition policy: configurable perception decides who learns about it and
// configurable reactions decide what that percept means.
type Occurrence struct {
	Actor       FactRef
	Action      ActionID
	Object      FactRef
	Location    Point
	Tags        []TagID
	Text        string // fallback when no role-specific text is supplied
	ActorText   string
	TargetText  string
	WitnessText string
	Appraisals  []ObserverAppraisal
}

func occurrence(actor *Entity, action ActionID, object *Entity, at Point, format string, args ...any) Occurrence {
	var actorRef, objectRef FactRef
	if actor != nil {
		actorRef = FactRef{Noun: nounForKind(actor.Kind), Entity: actor.ID, Label: actor.displayName()}
	}
	if object != nil {
		objectRef = FactRef{Noun: nounForKind(object.Kind), Entity: object.ID, Label: object.displayName()}
	}
	return Occurrence{
		Actor: actorRef, Action: action, Object: objectRef, Location: at,
		Text: fmt.Sprintf(format, args...),
	}
}

func (o Occurrence) source(selector StimulusSource) EntityID {
	switch selector {
	case StimulusSourceNone:
		return 0
	case StimulusSourceObject:
		return o.Object.Entity
	case StimulusSourceActor:
		return o.Actor.Entity
	default:
		if o.Actor.Entity != 0 {
			return o.Actor.Entity
		}
		return o.Object.Entity
	}
}

func (o Occurrence) appraisalFor(observer EntityID) (MoodVector, bool) {
	for _, appraisal := range o.Appraisals {
		if appraisal.Observer == observer {
			return appraisal.Target, true
		}
	}
	return MoodVector{}, false
}

func (o Occurrence) textFor(role ObserverRole) string {
	switch role {
	case RoleActor:
		if o.ActorText != "" {
			return o.ActorText
		}
	case RoleTarget:
		if o.TargetText != "" {
			return o.TargetText
		}
	case RoleWitness:
		if o.WitnessText != "" {
			return o.WitnessText
		}
	}
	return o.Text
}

// Percept is an observer-relative view of one occurrence.
type Percept struct {
	Observer   EntityID
	Channel    ChannelID
	Role       ObserverRole
	Phase      PerceptPhase
	Occurrence Occurrence
}

// OccurrencePattern is the configurable half of matching shared by perception
// and reaction rules. Empty fields are wildcards.
type OccurrencePattern struct {
	ActorNoun  NounID
	Action     ActionID
	ObjectNoun NounID
}

func (p OccurrencePattern) matches(o Occurrence) bool {
	return (p.ActorNoun == "" || p.ActorNoun == o.Actor.Noun) &&
		(p.Action == "" || p.Action == o.Action) &&
		(p.ObjectNoun == "" || p.ObjectNoun == o.Object.Noun)
}

func (p OccurrencePattern) specificity() int {
	n := 0
	if p.ActorNoun != "" {
		n++
	}
	if p.Action != "" {
		n++
	}
	if p.ObjectNoun != "" {
		n++
	}
	return n
}

// PerceptPattern adds observer-relative fields to an occurrence match. Empty
// fields remain wildcards.
type PerceptPattern struct {
	OccurrencePattern
	Channel ChannelID
	Role    ObserverRole
	Phase   PerceptPhase
}

func (p PerceptPattern) matches(percept Percept) bool {
	return p.OccurrencePattern.matches(percept.Occurrence) &&
		(p.Channel == "" || p.Channel == percept.Channel) &&
		(p.Role == "" || p.Role == percept.Role) &&
		(p.Phase == "" || p.Phase == percept.Phase)
}

func (p PerceptPattern) specificity() int {
	n := p.OccurrencePattern.specificity()
	if p.Channel != "" {
		n++
	}
	if p.Role != "" {
		n++
	}
	if p.Phase != "" {
		n++
	}
	return n
}

func patternsOverlap(a, b PerceptPattern) bool {
	return idsOverlap(string(a.ActorNoun), string(b.ActorNoun)) &&
		idsOverlap(string(a.Action), string(b.Action)) &&
		idsOverlap(string(a.ObjectNoun), string(b.ObjectNoun)) &&
		idsOverlap(string(a.Channel), string(b.Channel)) &&
		idsOverlap(string(a.Role), string(b.Role)) &&
		idsOverlap(string(a.Phase), string(b.Phase))
}

func idsOverlap(a, b string) bool { return a == "" || b == "" || a == b }

func hasTag(tags []TagID, want TagID) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func renderMemoryTemplate(template string, percept Percept) string {
	if template == "" {
		return percept.Occurrence.textFor(percept.Role)
	}
	replacer := strings.NewReplacer(
		"{actor}", percept.Occurrence.Actor.Label,
		"{object}", percept.Occurrence.Object.Label,
		"{action}", string(percept.Occurrence.Action),
		"{observer}", fmt.Sprintf("#%d", percept.Observer),
	)
	return replacer.Replace(template)
}

// perceptionKey is the bounded edge-trigger identity retained by a colonist.
// Rule plus source keeps two aliens distinct while allowing environmental facts
// such as "gore nearby" to use source zero as one aggregate observation.
type perceptionKey struct {
	Rule   RuleID
	Source EntityID
	Noun   NounID
}

// emitOccurrence evaluates configured instantaneous perception rules. Direct
// participant rules and spatial witness rules share the same path, so a feature
// emits one occurrence rather than separate actor/witness life events.
func (w *World) emitOccurrence(o Occurrence) {
	for i := range w.cognition.Perceptions {
		rule := &w.cognition.Perceptions[i]
		if rule.Cadence != CadenceInstant || !rule.Match.matches(o) {
			continue
		}
		switch rule.Channel {
		case ChannelDirect:
			var id EntityID
			switch rule.Role {
			case RoleActor:
				id = o.Actor.Entity
			case RoleTarget:
				id = o.Object.Entity
			}
			if observer := w.entities[id]; observer != nil && observer.Kind == Colonist && observer.Alive() {
				w.rememberPercept(observer, Percept{
					Observer: observer.ID, Channel: rule.Channel, Role: rule.Role,
					Phase: PhaseInstant, Occurrence: o,
				})
			}
		default:
			radius := w.perceptionRadius(*rule)
			for _, observer := range w.colonistsWithin(o.Location, radius, 0) {
				if observer.ID == o.Actor.Entity || observer.ID == o.Object.Entity {
					continue
				}
				if rule.LineOfSight && !w.hasLineOfSight(observer.Pos, o.Location) {
					continue
				}
				w.rememberPercept(observer, Percept{
					Observer: observer.ID, Channel: rule.Channel, Role: rule.Role,
					Phase: PhaseInstant, Occurrence: o,
				})
			}
		}
	}
}

// observePersistent evaluates configured enter/stay perception rules for one
// colonist and one optional noun filter. An empty filter observes all persistent
// facts; tests and compatibility wrappers can request only gore.
func (w *World) observePersistent(observer *Entity, only NounID) {
	if observer == nil || observer.Kind != Colonist {
		return
	}
	current := make(map[perceptionKey]Occurrence)
	for i := range w.cognition.Perceptions {
		rule := &w.cognition.Perceptions[i]
		if rule.Cadence == CadenceInstant || rule.Match.Action != ActionPresent {
			continue
		}
		if only != "" && rule.Match.ActorNoun != only {
			continue
		}
		radius := w.perceptionRadius(*rule)
		switch rule.Match.ActorNoun {
		case NounGore:
			if p, ok := w.goreWithin(observer.Pos, radius); ok {
				o := Occurrence{
					Actor:  FactRef{Noun: NounGore, Label: "gore"},
					Action: ActionPresent, Location: p,
					Text: "Saw the aftermath of violence nearby.",
				}
				w.notePersistentPercept(observer, rule, o, current)
			}
		default:
			for _, id := range w.entityIDsSorted() {
				other := w.entities[id]
				if other == nil || other == observer || !other.Alive() {
					continue
				}
				o := Occurrence{
					Actor: w.factRef(other), Action: ActionPresent, Location: other.Pos,
					Text: fmt.Sprintf("Saw %s #%d.", other.Kind, other.ID),
				}
				if !rule.Match.matches(o) || observer.Pos.Chebyshev(other.Pos) > radius {
					continue
				}
				if rule.LineOfSight && !w.hasLineOfSight(observer.Pos, other.Pos) {
					continue
				}
				w.notePersistentPercept(observer, rule, o, current)
			}
		}
	}
	if only != "" {
		for key, occurrence := range observer.perceiving {
			if key.Noun != only {
				current[key] = occurrence
			}
		}
	}
	hadThreat := observer.seesThreat
	for key, occurrence := range observer.perceiving {
		if _, stillPresent := current[key]; stillPresent {
			continue
		}
		if rule := w.perceptionRule(key.Rule); rule != nil {
			w.rememberPercept(observer, Percept{
				Observer: observer.ID, Channel: rule.Channel, Role: rule.Role,
				Phase: PhaseExit, Occurrence: occurrence,
			})
		}
	}
	observer.perceiving = current
	observer.seesThreat = false
	for key := range current {
		if key.Noun == NounAlien {
			observer.seesThreat = true
			break
		}
	}
	if hadThreat != observer.seesThreat {
		w.markMindDirty(observer)
	}
}

func (w *World) notePersistentPercept(observer *Entity, rule *PerceptionRule, o Occurrence, current map[perceptionKey]Occurrence) {
	key := perceptionKey{Rule: rule.ID, Source: o.source(StimulusSourceActor), Noun: o.Actor.Noun}
	current[key] = o
	phase := PhaseEnter
	if _, alreadyPresent := observer.perceiving[key]; alreadyPresent {
		if rule.Cadence != CadenceEnterOngoing {
			return
		}
		phase = PhaseOngoing
	}
	w.rememberPercept(observer, Percept{
		Observer: observer.ID, Channel: rule.Channel, Role: rule.Role,
		Phase: phase, Occurrence: o,
	})
}

func (w *World) perceptionRule(id RuleID) *PerceptionRule {
	for i := range w.cognition.Perceptions {
		if w.cognition.Perceptions[i].ID == id {
			return &w.cognition.Perceptions[i]
		}
	}
	return nil
}

func (w *World) perceptionRadius(rule PerceptionRule) int {
	if rule.Distance > 0 {
		return rule.Distance
	}
	switch rule.Radius {
	case RadiusFlee:
		return w.cfg.FleeRadius
	case RadiusStomp:
		return w.cfg.ColonistStompRadius
	case RadiusGoreSight:
		return w.cfg.GoreSightRadius
	default:
		return 0
	}
}

func (w *World) goreWithin(origin Point, radius int) (Point, bool) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			p := origin.Add(x, y)
			if w.InBounds(p) && w.tiles[w.index(p)].Gore > 0 {
				return p, true
			}
		}
	}
	return Point{}, false
}

// hasLineOfSight uses an integer Bresenham walk. The destination may itself be
// blocking; only intervening Rock and Wall tiles occlude sight.
func (w *World) hasLineOfSight(from, to Point) bool {
	x0, y0, x1, y1 := from.X, from.Y, to.X, to.Y
	dx, dy := absInt(x1-x0), absInt(y1-y0)
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy
	for x0 != x1 || y0 != y1 {
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
		if x0 == x1 && y0 == y1 {
			return true
		}
		switch w.TerrainAt(Point{x0, y0}) {
		case Rock, Wall:
			return false
		}
	}
	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
