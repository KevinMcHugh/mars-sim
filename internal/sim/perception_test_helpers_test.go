package sim

func testReaction(w *World, id RuleID) *ReactionSpec {
	reaction, ok := w.cognition.reaction(id)
	if !ok {
		panic("missing test reaction " + id)
	}
	return reaction
}

func testPercept(e *Entity, reaction *ReactionSpec) Percept {
	return Percept{
		Observer:       e.ID,
		Channel:        reaction.Match.Channel,
		Role:           reaction.Match.Role,
		Phase:          reaction.Match.Phase,
		ObjectRelation: reaction.Match.ObjectRelation,
		Occurrence: Occurrence{
			Actor:  FactRef{Noun: reaction.Match.ActorNoun},
			Action: reaction.Match.Action,
			Object: FactRef{Noun: reaction.Match.ObjectNoun},
		},
	}
}

func rememberTestReaction(w *World, e *Entity, id RuleID, source EntityID, text string, contextual *MoodVector) {
	reaction := testReaction(w, id)
	o := Occurrence{
		Actor:  FactRef{Noun: reaction.Match.ActorNoun, Entity: source},
		Action: reaction.Match.Action,
		Object: FactRef{Noun: reaction.Match.ObjectNoun},
		Text:   text,
	}
	switch reaction.Match.Role {
	case RoleActor:
		o.Actor.Entity = e.ID
	case RoleTarget:
		o.Object.Entity = e.ID
	}
	if source != 0 {
		if o.Actor.Entity == 0 {
			o.Actor.Entity = source
		} else if o.Object.Entity == 0 {
			o.Object.Entity = source
		}
	}
	if contextual != nil {
		o.Appraisals = []ObserverAppraisal{{Observer: e.ID, Target: *contextual}}
	}
	w.rememberPercept(e, Percept{
		Observer: e.ID, Channel: reaction.Match.Channel, Role: reaction.Match.Role,
		Phase: reaction.Match.Phase, Occurrence: o,
	})
}

func rememberTest(w *World, e *Entity, id RuleID, text string) {
	rememberTestReaction(w, e, id, 0, text, nil)
}

func rememberTestFrom(w *World, e *Entity, id RuleID, source EntityID, text string) {
	rememberTestReaction(w, e, id, source, text, nil)
}

func addTestStimulus(w *World, e *Entity, id RuleID, source EntityID) bool {
	reaction := testReaction(w, id)
	o := Occurrence{
		Actor:  FactRef{Noun: reaction.Match.ActorNoun, Entity: source},
		Action: reaction.Match.Action,
		Object: FactRef{Noun: reaction.Match.ObjectNoun},
	}
	return w.addStimulus(e, reaction, Percept{
		Observer: e.ID, Channel: reaction.Match.Channel, Role: reaction.Match.Role,
		Phase: reaction.Match.Phase, Occurrence: o,
	})
}
