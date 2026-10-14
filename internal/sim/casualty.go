package sim

import (
	"fmt"
	"slices"
)

// Casualties and aid. A casualty is a colonist that cannot help itself: today,
// one passed out where it fell. Every casualty is in World.casualties, a slice
// kept sorted by ID as colonists go down and come round, so nothing has to scan
// the world to find one; it is almost always empty. A colonist that can see one
// may choose to help (FocusAid), scored beside every other focus: how bad it
// looks (Peril), how the helper feels about the casualty, its traits and how
// far it is. Helping is JobAid: walk over, bring it round, and share a meal if
// it is hungry. See docs/emergency-services.md.

// compassionSeed separates the compassion trait stream from the others (see
// rng.go and ownStreamGroup).
const compassionSeed = 0x6C62272E07BB0142

// Peril is how much danger a casualty is in. Other systems switch on it and
// never read the hunger and HP behind it themselves.
type Peril uint8

const (
	PerilNone     Peril = iota // not a casualty
	PerilStable                // down, but not getting worse
	PerilSerious               // getting worse: hungry while it lies there
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
// anyway, so there is nothing to keep in step.
func (w *World) perilOf(e *Entity) Peril {
	if e == nil || !e.passedOut || !e.Alive() {
		return PerilNone
	}
	food := &w.cfg.Drives[DriveFood]
	level := w.driveLevel(e, DriveFood)
	switch {
	case level >= food.Max, e.MaxHP > 0 && e.HP*100 < e.MaxHP*w.cfg.PerilCriticalHPPct:
		return PerilCritical
	case level >= food.SeekAt:
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

// addCasualty puts e in the index. passOut calls it.
func (w *World) addCasualty(e *Entity) {
	i, found := slices.BinarySearch(w.casualties, e.ID)
	if !found {
		w.casualties = slices.Insert(w.casualties, i, e.ID)
	}
}

// dropCasualty takes e out of the index when it comes round or dies. Its
// rescue record goes with it: a colonist up on its feet owes nobody guilt.
func (w *World) dropCasualty(e *Entity) {
	if i, found := slices.BinarySearch(w.casualties, e.ID); found {
		w.casualties = slices.Delete(w.casualties, i, i+1)
	}
	if helper := w.entities[e.aidedBy]; helper != nil && helper.aidTarget == e.ID {
		helper.aidTarget = 0
	}
	e.aidedBy = 0
	e.aidPassedBy = nil
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

// aidOption is the casualty e would help, if any, and what helping it scores
// beyond the focus's base and affect: peril (in the Drive slot), how e
// feels about it (Personality) and how far it is. Only a casualty e can see
// (aid-sight, its own level), reach, and that nobody else is helping is
// offered; the best of them wins, ties to the lowest ID.
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
		if c == nil || c == e || !e.Pos.Within(c.Pos, w.cfg.AidSight) {
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
		if !w.canReach(e, c) {
			continue
		}
		o := aidOption{
			casualty:    id,
			peril:       peril,
			personality: w.aidRegard(e, c, threat),
			distance:    w.travelEstimate(e.Pos, c.Pos),
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
// choice replaces an earlier one; helping removes it.
func (w *World) notePass(e *Entity, offered EntityID, chosen FocusCandidate) {
	c := w.entities[offered]
	if c == nil || !w.isCasualty(c) {
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

// runAidFocus carries out FocusAid on the casualty arbitration chose: claim
// it, walk over, and bring it round.
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
		c.aidedBy = e.ID
		w.forgetPass(c, e.ID)
	}
	e.resting = false
	if !e.Pos.Adjacent(c.Pos) {
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
	e.Progress++
	if e.Progress < w.cfg.AidRouseTicks {
		return
	}
	w.rouse(e, c)
	w.finishFocus(e)
}

// releaseAid lets go of a helper's claim on its casualty. clearJob calls it.
func (w *World) releaseAid(e *Entity) {
	if c := w.entities[e.aidTarget]; c != nil && c.aidedBy == e.ID {
		c.aidedBy = 0
	}
	e.aidTarget = 0
}

// rouse brings casualty c round. It wakes still exhausted, but the roused
// effect holds its sleep off for a while, long enough to eat and find a bed;
// without it, nothing would have lowered its sleep and it would pass out
// again on its next turn. If c is hungry and helper carries a meal of its
// own, the meal is c's now: a gift, the one change of hands no market sees.
func (w *World) rouse(helper, c *Entity) {
	gave := false
	if c.drives[DriveFood].phase >= DrivePressing || w.driveLevel(c, DriveFood) >= w.cfg.Drives[DriveFood].SeekAt {
		if helper.ownCarried(Meal) > 0 && c.ownCarried(Meal) == 0 && c.Inventory.CanAdd(Meal, 1) {
			helper.Inventory.Remove(Meal, 1)
			c.Inventory.Add(Meal, 1)
			gave = true
		}
	}
	c.passedOut = false
	c.State = Idle
	w.dropCasualty(c)
	if kind, ok := w.driveEffectNamed(rousedEffectName); ok {
		w.applyEffect(c, kind)
	}
	w.markMindDirty(c)
	if w.cfg.AidGratitude != 0 {
		w.bumpAffinity(c.ID, helper.ID, w.cfg.AidGratitude)
	}

	o := w.occurrence(helper, ActionRescue, c, c.Pos, "")
	if gave {
		o.ActorText = fmt.Sprintf("Helped %s up off the floor and shared a meal.", c.displayName())
		o.TargetText = fmt.Sprintf("Was helped up off the floor by %s, who shared a meal.", helper.displayName())
	} else {
		o.ActorText = fmt.Sprintf("Helped %s up off the floor.", c.displayName())
		o.TargetText = fmt.Sprintf("Was helped up off the floor by %s.", helper.displayName())
	}
	w.emitOccurrence(o)
	w.aidRescues++
	w.logEvent(LogNote, fmt.Sprintf("%s brought %s round.", helper.displayName(), c.displayName()))
}

// casualtyDied settles a casualty's death with the living: guilt for each
// colonist it remembers passing it by (none for a Callous one, unless it was
// family), and grief for whoever was on the way to it. remove calls it before
// the body is gone, so the occurrences can still name it.
func (w *World) casualtyDied(e *Entity) {
	if !w.isCasualty(e) {
		return
	}
	w.aidDeaths++
	if helper := w.entities[e.aidedBy]; helper != nil && helper.Alive() {
		o := w.occurrence(helper, ActionLose, e, helper.Pos, "")
		o.ActorText = fmt.Sprintf("Could not save %s.", e.displayName())
		w.emitOccurrence(o)
	}
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
