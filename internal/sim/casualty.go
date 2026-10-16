package sim

import (
	"fmt"
	"slices"
	"strings"
)

// Casualties and aid. A casualty is a colonist that needs someone: one passed
// out where it fell (from exhaustion, or from blood loss), or one with a wound
// nobody has dressed. Every casualty is in World.casualties, a slice kept
// sorted by ID as colonists are hurt, go down, are seen to and come round, so
// nothing has to scan the world to find one; it is almost always empty. A
// colonist that can see one may choose to help (FocusAid), scored beside
// every other focus: how bad it looks (Peril), how the helper feels about the
// casualty, its traits and how far it is. An on-call EMT knows of every
// casualty and is paid to come (emt.go). Helping is JobAid: walk over, dress
// its wounds, bring it round (sharing a meal if it is hungry), or, for an
// EMT, drag one that cannot be brought round to an infirmary cot. A walking
// wounded colonist can dress its own wounds, slowly. See
// docs/emergency-services.md.

// compassionSeed separates the compassion trait stream from the others (see
// rng.go and ownStreamGroup).
const compassionSeed = 0x6C62272E07BB0142

// Peril is how much danger a casualty is in. Other systems switch on it and
// never read the hunger, wounds and HP behind it themselves.
type Peril uint8

const (
	PerilNone     Peril = iota // not a casualty
	PerilStable                // needs someone, but not getting worse
	PerilSerious               // getting worse: hungry while it lies there, a bad wound, bleeding
	PerilCritical              // close to death: starving, or badly hurt
)

func (p Peril) String() string {
	switch p {
	case PerilNone:
		return "none"
	case PerilStable:
		return "stable"
	case PerilSerious:
		return "serious"
	case PerilCritical:
		return "critical"
	default:
		return "?"
	}
}

// aidPass is one colonist that was offered a casualty's rescue by its own
// arbitration and chose something else. light is a pass with a reason (its
// own pressing hunger), which weighs less if the casualty dies.
type aidPass struct {
	id    EntityID
	light bool
}

// perilOf is the one place that maps a colonist's state to a Peril. It is
// read on demand: the casualty index is tiny, and the hunger it reads is lazy
// anyway, so there is nothing to keep in step. A casualty laid in an
// infirmary cot is not hungry: the cot holds its hunger.
func (w *World) perilOf(e *Entity) Peril {
	if e == nil || e.Kind != Colonist || !e.Alive() {
		return PerilNone
	}
	open := e.openWounds() > 0
	if !e.passedOut && !open {
		return PerilNone
	}
	food := &w.cfg.Drives[DriveFood]
	hungry := 0
	if e.passedOut && !e.inCot {
		hungry = w.driveLevel(e, DriveFood)
	}
	switch {
	case e.MaxHP > 0 && e.HP*100 < e.MaxHP*w.cfg.PerilCriticalHPPct, hungry >= food.Max:
		return PerilCritical
	case hungry >= food.SeekAt, e.downBled && !e.inCot, w.bleedRate(e) > 0, e.badWound():
		return PerilSerious
	default:
		return PerilStable
	}
}

// isCasualty reports whether e is in the casualty index.
func (w *World) isCasualty(e *Entity) bool {
	_, found := slices.BinarySearch(w.casualties, e.ID)
	return found
}

// refreshCasualty puts e in the index or takes it out, as its peril says.
// Everything that can make a colonist a casualty or stop it being one calls
// it: passing out, coming round, a wound, a dressing.
func (w *World) refreshCasualty(e *Entity) {
	if w.perilOf(e) != PerilNone {
		w.addCasualty(e)
	} else if w.isCasualty(e) {
		w.dropCasualty(e)
	}
}

// addCasualty puts e in the index, and tells every on-call EMT: they hear of
// a casualty anywhere in the colony, not only one they can see.
func (w *World) addCasualty(e *Entity) {
	i, found := slices.BinarySearch(w.casualties, e.ID)
	if found {
		return
	}
	w.casualties = slices.Insert(w.casualties, i, e.ID)
	e.casualtySince = w.tick
	w.postRescue(e)
	for _, id := range w.onCall {
		if emt := w.entities[id]; emt != nil && emt != e {
			w.markMindDirty(emt)
		}
	}
}

// dropCasualty takes e out of the index when it is seen to, comes round or
// dies. Its rescue record goes with it: a colonist up on its feet owes nobody
// guilt.
func (w *World) dropCasualty(e *Entity) {
	if i, found := slices.BinarySearch(w.casualties, e.ID); found {
		w.casualties = slices.Delete(w.casualties, i, i+1)
	}
	helper := w.entities[e.aidedBy]
	w.settleRescue(e, helper)
	if helper != nil && helper.aidTarget == e.ID {
		helper.aidTarget = 0
	}
	e.aidedBy = 0
	e.aidPassedBy = nil
	e.aidReached = false
}

// noteResponse records how long a casualty waited before a helper set out to
// it, apart for on-call EMTs and for bystanders: the measure of whether a
// detail that keeps working is close enough to matter.
func (w *World) noteResponse(helper *Entity, ticks int) {
	if helper.onCall {
		w.emtCallsWindow++
		w.emtResponses++
		w.emtResponseTicks += ticks
	} else {
		w.aidResponses++
		w.aidResponseTicks += ticks
	}
}

// closeKin reports whether b is a's spouse, parent, child or sibling.
func (w *World) closeKin(a, b *Entity) bool {
	if a.kin == 0 || b.kin == 0 {
		return false
	}
	for _, r := range w.cachedRelations(a, w.cachedKinChildren()) {
		if r.Other != b.ID {
			continue
		}
		switch r.Kind {
		case RelSpouse, RelParent, RelChild, RelSibling:
			return true
		}
	}
	return false
}

// canHelp reports whether e could do anything for casualty c now: dress a
// wound, bring it round from exhaustion, or (an on-call EMT only) drag it,
// down from blood loss, to a free infirmary cot. A colonist can dress its own
// wounds but do nothing else for itself. Someone a helper can do nothing for
// is not offered, so nobody stands over a colonist it cannot help.
func (w *World) canHelp(e, c *Entity) bool {
	if c.openWounds() > 0 {
		return e != c || !e.passedOut
	}
	if e == c || !c.passedOut {
		return false
	}
	if !c.downBled {
		return true
	}
	if !e.onCall || c.inCot {
		return false
	}
	_, ok := w.freeCot(c)
	return ok
}

// aidOption is the casualty e would help, if any, and what helping it scores
// beyond the focus's base and affect: peril (in the Drive slot), how e
// feels about it and any duty it has (Personality) and how far it is. A
// casualty is offered when e can help it, nobody else is helping, and e can
// see (aid-sight, its own level) and reach it; an on-call EMT is offered
// every casualty it can reach. The best wins, ties to the lowest ID.
type aidOption struct {
	casualty    EntityID
	peril       int
	personality int
	distance    int
}

func (w *World) aidOptionFor(e *Entity, threat bool) (aidOption, bool) {
	if !w.cfg.Aid || len(w.casualties) == 0 || e.passedOut || e.Kind != Colonist {
		return aidOption{}, false
	}
	var best aidOption
	bestScore, found := 0, false
	weight := w.cfg.Focuses[FocusAid].DistanceWeight
	for _, id := range w.casualties {
		c := w.entities[id]
		if c == nil {
			continue
		}
		self := c == e
		if !self && !e.onCall && !e.Pos.Within(c.Pos, w.cfg.AidSight) {
			continue
		}
		if c.aidedBy != 0 && c.aidedBy != e.ID {
			continue
		}
		var peril int
		switch w.perilOf(c) {
		case PerilStable:
			peril = w.cfg.AidPerilStable
		case PerilSerious:
			peril = w.cfg.AidPerilSerious
		case PerilCritical:
			peril = w.cfg.AidPerilCritical
		default:
			continue
		}
		if !w.canHelp(e, c) || !self && !w.canReach(e, c) {
			continue
		}
		o := aidOption{casualty: id, peril: peril}
		if !self {
			o.personality = w.aidRegard(e, c, threat)
			if e.onCall {
				o.personality += w.emtDuty(e)
			}
			o.distance = w.travelEstimate(e.Pos, c.Pos)
		}
		score := o.peril + o.personality - o.distance*weight
		if !found || score > bestScore {
			best, bestScore, found = o, score, true
		}
	}
	return best, found
}

// aidRegard is how e's ties and nature weigh on helping c: affinity, close
// family, Caring and Callous, and a coward's fear while an alien is in sight.
func (w *World) aidRegard(e, c *Entity, threat bool) int {
	n := 0
	if w.cfg.AffinityMax > 0 {
		n += w.cfg.AidAffinityWeight * w.affinityBetween(e.ID, c.ID) / w.cfg.AffinityMax
	}
	kin := w.closeKin(e, c)
	if kin {
		n += w.cfg.AidKinBonus
	}
	if p := e.Profile; p != nil {
		if p.HasTrait(TraitCaring) {
			n += w.cfg.AidTraitBonus
		}
		if p.HasTrait(TraitCallous) && !kin {
			n -= w.cfg.AidTraitBonus
		}
		if threat && p.HasTrait(TraitCowardly) {
			n -= w.cfg.AidCowardPenalty
		}
	}
	return n
}

// noticeCasualty makes a colonist think again the first time an unhelped
// casualty is in sight, rather than at its next scheduled reconsideration: a
// starving casualty has minutes, not half an hour.
func (w *World) noticeCasualty(e *Entity) {
	if !w.cfg.Aid || len(w.casualties) == 0 || e.focus == FocusAid {
		return
	}
	for _, id := range w.casualties {
		c := w.entities[id]
		if c == nil || c == e || c.aidedBy != 0 || !e.Pos.Within(c.Pos, w.cfg.AidSight) {
			continue
		}
		if e.noticedCasualty != id {
			e.noticedCasualty = id
			w.markMindDirty(e)
		}
		return
	}
}

// notePass records, on the casualty its arbitration offered, that e chose
// something else, unless what it chose is self-preservation (a threat, a
// sealed room, or a fatal drive gone critical), which is no failing. A later
// choice replaces an earlier one; helping removes it. Passing over one's own
// wounds is nobody's guilt.
func (w *World) notePass(e *Entity, offered EntityID, chosen FocusCandidate) {
	c := w.entities[offered]
	if c == nil || c == e || !w.isCasualty(c) {
		return
	}
	light := false
	switch chosen.Kind {
	case FocusFlee, FocusFight, FocusEscape:
		w.forgetPass(c, e.ID)
		return
	case FocusEat, FocusRelieve, FocusSocialize, FocusSleep:
		if w.cfg.Drives[chosen.Drive].Fatal() {
			if e.drives[chosen.Drive].phase == DriveCritical {
				w.forgetPass(c, e.ID)
				return
			}
			light = true
		}
	}
	i, found := slices.BinarySearchFunc(c.aidPassedBy, e.ID, func(p aidPass, id EntityID) int {
		return cmpID(p.id, id)
	})
	if found {
		c.aidPassedBy[i].light = light
		return
	}
	if len(c.aidPassedBy) >= w.cfg.AidPassedByMax {
		return
	}
	c.aidPassedBy = slices.Insert(c.aidPassedBy, i, aidPass{id: e.ID, light: light})
}

func (w *World) forgetPass(c *Entity, id EntityID) {
	c.aidPassedBy = slices.DeleteFunc(c.aidPassedBy, func(p aidPass) bool { return p.id == id })
}

func cmpID(a, b EntityID) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// aidDone is what a helper has done for its casualty since it claimed it,
// for the one occurrence that tells them both when it is done.
type aidDone struct {
	dressed int  // wounds dressed
	roused  bool // brought round from exhaustion
	gave    bool // and given a meal
	dragged bool // laid in an infirmary cot
}

// runAidFocus carries out FocusAid on the casualty arbitration chose: claim
// it, walk over, and do what it needs, one thing at a time: its wounds first,
// then bring it round, or (an EMT, for a casualty down from blood loss) drag
// it to a cot. When there is nothing left the helper can do, it is done.
func (w *World) runAidFocus(e *Entity, selected FocusCandidate) {
	c := w.entities[selected.Casualty]
	if c == nil || !w.isCasualty(c) || (c.aidedBy != 0 && c.aidedBy != e.ID) {
		w.finishFocus(e)
		w.runIdleFocus(e)
		return
	}
	if e.Job != JobAid || e.aidTarget != c.ID {
		w.clearJob(e)
		e.Job, e.aidTarget, e.Progress = JobAid, c.ID, 0
		e.aidDone = aidDone{}
		if c.aidedBy == 0 && c != e && !c.aidReached {
			c.aidReached = true
			w.noteResponse(e, w.tick-c.casualtySince)
		}
		c.aidedBy = e.ID
		w.forgetPass(c, e.ID)
	}
	e.resting = false
	self := c == e
	if !self && !e.Pos.Adjacent(c.Pos) {
		arrived, ok := w.travelTo(e, c.Pos)
		if !ok {
			w.finishFocus(e)
			w.runIdleFocus(e)
			return
		}
		if !arrived {
			e.State = Moving
			return
		}
	}
	e.State = Aiding
	switch {
	case c.openWounds() > 0:
		e.Progress++
		if e.Progress < w.dressTicks(e, self) {
			return
		}
		e.Progress = 0
		w.practise(e, SkillMedicine, w.cfg.AidDressTicks)
		w.dressWound(e, c)
		if !w.isCasualty(c) {
			w.finishAid(e, c)
		}
	case !self && c.passedOut && !c.downBled:
		e.Progress++
		if e.Progress < w.cfg.AidRouseTicks {
			return
		}
		w.practise(e, SkillMedicine, w.cfg.AidRouseTicks)
		w.rouse(e, c)
		w.finishAid(e, c)
	case !self && c.passedOut && c.downBled && e.onCall && !c.inCot:
		if w.dragStep(e, c) {
			w.finishAid(e, c)
		}
	default:
		w.finishAid(e, c)
	}
}

// dressTicks is how long e takes to dress one wound: the base, cut by its
// medicine rank and its traits, and slower on itself.
func (w *World) dressTicks(e *Entity, self bool) int {
	t := w.workTicks(e, SkillMedicine, w.cfg.AidDressTicks)
	if self {
		t = atLeast1(t * w.cfg.AidSelfDressPct / 100)
	}
	return t
}

// releaseAid lets go of a helper's claim on its casualty, and of the cot it
// was dragging it to. clearJob calls it.
func (w *World) releaseAid(e *Entity) {
	if c := w.entities[e.aidTarget]; c != nil && c.aidedBy == e.ID {
		c.aidedBy = 0
	}
	w.releaseDrag(e)
	e.aidTarget = 0
}

// rouse brings casualty c round from exhaustion. It wakes still exhausted,
// but the roused effect holds its sleep off for a while, long enough to eat
// and find a bed; without it, nothing would have lowered its sleep and it
// would pass out again a few ticks later. If c is hungry and helper carries
// a meal of its own, the meal is c's now: a gift, the one change of hands no
// market sees.
func (w *World) rouse(helper, c *Entity) {
	if c.drives[DriveFood].phase >= DrivePressing || w.driveLevel(c, DriveFood) >= w.cfg.Drives[DriveFood].SeekAt {
		if helper.ownCarried(Meal) > 0 && c.ownCarried(Meal) == 0 && c.Inventory.CanAdd(Meal, 1) {
			helper.Inventory.Remove(Meal, 1)
			c.Inventory.Add(Meal, 1)
			helper.aidDone.gave = true
		}
	}
	// Marked before it comes round: coming round settles the call, which
	// pays only a helper that did something.
	helper.aidDone.roused = true
	w.comeRound(c)
	if kind, ok := w.driveEffectNamed(rousedEffectName); ok {
		w.applyEffect(c, kind)
	}
}

// comeRound is a casualty getting up: on its own, or roused. It leaves any
// cot it lay in, and is a casualty after only if a wound is still open.
func (w *World) comeRound(c *Entity) {
	c.passedOut, c.downBled = false, false
	c.State = Idle
	w.leaveCot(c)
	w.refreshCasualty(c)
	w.markMindDirty(c)
}

// finishAid ends a helper's job on c: one occurrence telling them both what
// was done (helped, was helped), the casualty's gratitude, and the helper
// back to choosing. Seeing to oneself is no story.
func (w *World) finishAid(e, c *Entity) {
	if e.aidDone.dragged {
		// Laid in a cot, a casualty is still one (it heals there), so the
		// call is settled here rather than when it leaves the index.
		w.settleRescue(c, e)
	}
	done := e.aidDone
	e.aidDone = aidDone{}
	w.finishFocus(e)
	if c == e || done == (aidDone{}) {
		return
	}
	if w.cfg.AidGratitude != 0 {
		w.bumpAffinity(c.ID, e.ID, w.cfg.AidGratitude)
	}
	var did []string
	if done.dressed > 0 {
		wounds := "wound"
		if done.dressed > 1 {
			wounds = "wounds"
		}
		did = append(did, fmt.Sprintf("dressed %s %s", possessive(c.displayName()), wounds))
	}
	if done.roused {
		did = append(did, fmt.Sprintf("helped %s up off the floor", c.displayName()))
	}
	if done.gave {
		did = append(did, "shared a meal")
	}
	if done.dragged {
		did = append(did, fmt.Sprintf("dragged %s to an infirmary cot", c.displayName()))
	}
	what := strings.Join(did, ", and ")
	o := w.occurrence(e, ActionRescue, c, c.Pos, "")
	o.ActorText = capitalizeFirst(what) + "."
	o.TargetText = fmt.Sprintf("Was seen to by %s, who %s.", e.displayName(), what)
	w.emitOccurrence(o)
	w.aidRescues++
	w.logEvent(LogNote, fmt.Sprintf("%s %s.", e.displayName(), what))
}

// possessive is name's possessive form.
func possessive(name string) string {
	if strings.HasSuffix(name, "s") {
		return name + "'"
	}
	return name + "'s"
}

// casualtyDied settles a casualty's death with the living: grief for whoever
// was on the way to it, and, if it died lying helpless, guilt for each
// colonist it remembers passing it by (none for a Callous one, unless it was
// family). A walking wounded colonist killed in a fight did not die for want
// of a dressing, so nobody who passed its scratch feels guilty. remove calls
// it before the body is gone, so the occurrences can still name it.
func (w *World) casualtyDied(e *Entity) {
	if !w.isCasualty(e) {
		return
	}
	w.aidDeaths++
	if helper := w.entities[e.aidedBy]; helper != nil && helper != e && helper.Alive() {
		o := w.occurrence(helper, ActionLose, e, helper.Pos, "")
		o.ActorText = fmt.Sprintf("Could not save %s.", e.displayName())
		w.emitOccurrence(o)
	}
	if e.passedOut {
		for _, p := range e.aidPassedBy {
			g := w.entities[p.id]
			if g == nil || !g.Alive() || g.Kind != Colonist {
				continue
			}
			if g.Profile != nil && g.Profile.HasTrait(TraitCallous) && !w.closeKin(g, e) {
				continue
			}
			action, text := ActionAbandon, "Walked past %s, who died."
			if p.light {
				action, text = ActionFail, "Did not stop for %s, who died."
			}
			o := w.occurrence(g, action, e, g.Pos, "")
			o.ActorText = fmt.Sprintf(text, e.displayName())
			w.emitOccurrence(o)
		}
	}
	w.leaveCot(e)
	w.dropCasualty(e)
}

// rousedEffectName names the drive effect rouse starts (see rousedEffect).
const rousedEffectName = "roused"

// rousedEffect is what being brought round does to the sleep drive: a jolt
// down from the ceiling it collapsed at, then three hours of it growing at a
// quarter speed, long enough to eat and reach a bed.
func rousedEffect() DriveEffectProfile {
	var st DriveEffectStage
	st.Ticks = 135
	st.Drive[DriveSleep] = DriveEffectTerms{Instant: -50, RateChange: -75}
	return DriveEffectProfile{Name: rousedEffectName, Stages: []DriveEffectStage{st}}
}

// driveEffectNamed finds a configured drive effect by name.
func (w *World) driveEffectNamed(name string) (DriveEffectKind, bool) {
	for i, p := range w.cfg.DriveEffects {
		if p.Name == name {
			return DriveEffectKind(i), true
		}
	}
	return 0, false
}

// helplessBias is how many tiles nearer a hostile hunter reckons prey that
// cannot flee or fight back.
func (w *World) helplessBias(c *Entity) int {
	if c.Kind == Colonist && c.passedOut {
		return w.cfg.HelplessPreyDiscount
	}
	return 0
}
