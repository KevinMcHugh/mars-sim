package sim

import "fmt"

// Behaviors are the reusable pieces creature AI is built from. A species'
// ladder (Species.ladder) lists them in priority order; animalTurn runs the
// ladder top to bottom until one acts. See docs/species-and-behaviors.md.
//
// The rungs below were lifted out of the old catTurn, ratTurn, and
// chickenTurn without changing what they do or the order they draw from the
// simulation RNG, so a seed plays out exactly as before. Reordering a ladder,
// or changing what a rung does, is a gameplay change.

// A behavior either acts for e this tick and reports true, ending the ladder,
// or declines and reports false so the next rung runs.
type behavior interface {
	act(w *World, e *Entity) bool
}

// animalTurn runs one tick of any creature but a colonist: the steps every
// species shares (starving, giving birth, pacing), then its ladder.
func (w *World) animalTurn(e *Entity) {
	sp := w.speciesOf(e)
	if sp.Starves {
		w.applyDriveConsequences(e)
		if !e.Alive() { // starved this tick
			w.clearJob(e)
			w.addCorpse(e.Pos, sp.Corpse)
			w.remove(e.ID, "starved")
			w.logEvent(LogDeath, fmt.Sprintf("%s #%d starves.", capitalizeFirst(sp.Name), e.ID))
			return
		}
	}

	// A carried litter arrives once gestation completes, whatever else the
	// mother does with the rest of her tick.
	if b := e.breeding; b != nil && b.pregnant && w.tick >= b.dueTick {
		w.giveBirth(e)
	}
	// An alien whose time in a stage is up grows into the next, and acts
	// as that from now on.
	if l := e.life; l != nil && l.growAt > 0 && w.tick >= l.growAt {
		w.growUp(e)
		sp = w.speciesOf(e)
	}
	// A laying form whose time has come lays its brood, and still acts.
	if l := e.life; l != nil && l.layAt > 0 && w.tick >= l.layAt {
		w.layBrood(e)
	}

	if sp.Paced {
		if e.Cooldown > 0 {
			e.Cooldown-- // mid-stride between slow steps, or resting after a catch
			return
		}
		// A rung may lengthen this (a cat resting after a pounce).
		e.Cooldown = sp.Slowness - 1
	}

	for _, b := range sp.ladder {
		if b.act(w, e) {
			return
		}
	}
}

// hunt chases the prey find picks and catches it when adjacent: a cat
// pouncing on the nearest rat, an alien striking what it hunts. It declines,
// clearing the quarry, when find has nothing.
type hunt struct {
	find  preyFinder
	catch func(w *World, hunter, prey *Entity)
	rest  int // ticks of Cooldown after a catch
}

// A preyFinder picks what a hunter goes after, nearest first with ties toward
// the lower ID, so the choice never depends on map order.
type preyFinder func(w *World, e *Entity) (*Entity, bool)

// preyAnywhere finds the nearest living creature carrying any of tags
// anywhere on the map (a cat after vermin).
func preyAnywhere(tags Tags) preyFinder {
	return func(w *World, e *Entity) (*Entity, bool) { return w.nearestTaggedWhere(e.Pos, tags, nil) }
}

// preyInRoom finds the nearest living creature carrying any of tags in the
// hunter's own room, never of its own species: a Hostile alien walks the
// floor like everyone else, so prey behind a wall or across rock is out of
// reach, and a nest does not eat itself.
func preyInRoom(tags Tags) preyFinder {
	return func(w *World, e *Entity) (*Entity, bool) {
		return w.nearestTaggedWhere(e.Pos, tags, func(c *Entity) bool {
			return c != e && !sameSpecies(c, e) && w.canReach(e, c)
		})
	}
}

// taggedWithin finds the nearest creature carrying any of tags within radius
// in the hunter's own room: a Cautious alien reacts to a colonist that comes
// close, but does not go looking beyond its radius.
func taggedWithin(tags Tags, radius int) preyFinder {
	return func(w *World, e *Entity) (*Entity, bool) {
		return w.nearestMatch(e.Pos, radius, func(c *Entity) bool {
			return w.tagsOf(c).Has(tags) && w.sameRoom(e.Pos, c.Pos)
		})
	}
}

func (b hunt) act(w *World, e *Entity) bool {
	prey, ok := b.find(w, e)
	if !ok {
		e.Quarry = 0
		return false
	}
	e.Quarry = prey.ID

	if e.Pos.Adjacent(prey.Pos) {
		b.catch(w, e, prey)
		e.Cooldown = b.rest
		return true
	}

	e.State = Hunting
	if _, ok := w.travelTo(e, prey.Pos); !ok {
		// The prey is unreachable on foot (walled off, or the hunter is
		// wedged): prowl instead of standing still.
		w.wanderStep(e)
	}
	return true
}

// flee bolts from the nearest creature carrying any of from within radius,
// dropping whatever job it had (a rat from a mouser).
type flee struct {
	from   Tags
	radius int
}

func (b flee) act(w *World, e *Entity) bool {
	threat, ok := w.nearestTagged(e.Pos, b.from, b.radius)
	if !ok {
		return false
	}
	w.clearJob(e)
	e.State = Fleeing
	w.fleeStep(e, threat.Pos)
	return true
}

// forage feeds a hungry creature. A foraging job already under way (a rat
// walking to a body or a pod) runs on until it ends, hungry or not. Otherwise,
// once the food drive reaches its seek level, the sources are tried in order
// and the first that takes the tick wins.
type forage struct {
	sources []foodSource
}

// A foodSource either busies a hungry creature with eating from it this tick
// (starting a job, walking there, or eating) and reports true, or reports
// false when it has nothing to offer.
type foodSource func(w *World, e *Entity) bool

func (b forage) act(w *World, e *Entity) bool {
	switch e.Job {
	case JobUse:
		w.jobUse(e)
		return true
	case JobScavenge:
		w.jobScavenge(e)
		return true
	}
	if w.driveLevel(e, DriveFood) < w.cfg.Drives[DriveFood].SeekAt {
		return false
	}
	for _, src := range b.sources {
		if src(w, e) {
			return true
		}
	}
	return false
}

// forageScavenge starts a scavenging job on the nearest body, gore, or scum in
// range: the same biomatter the scumhouse runs on (see scavenge.go).
func forageScavenge(w *World, e *Entity) bool {
	target, ok := w.nearestScavenge(e)
	if !ok {
		return false
	}
	e.Job, e.Target, e.Progress = JobScavenge, target, 0
	w.jobScavenge(e)
	return true
}

// foragePod raids a reachable nutrient pod, while pods feed anyone.
func foragePod(w *World, e *Entity) bool {
	if !w.podsFeed() {
		return false
	}
	field := w.facilityField(NutrientPod)
	if field == nil || field.at(e.Pos) < 0 {
		return false
	}
	e.Job, e.Drive, e.Progress = JobUse, DriveFood, 0
	w.jobUse(e)
	return true
}

// breed mates with an adjacent eligible partner (see tryMate).
type breed struct{}

func (breed) act(w *World, e *Entity) bool { return w.tryMate(e) }

// stayNearTrough walks a pet back toward its trough once it has strayed more
// than roam tiles from it (a chicken). A pet with no trough, or one it cannot
// reach, declines.
type stayNearTrough struct {
	roam int
}

func (b stayNearTrough) act(w *World, e *Entity) bool {
	trough, ok := e.petTrough()
	if !ok || e.Pos.Chebyshev(trough) <= b.roam {
		return false
	}
	if _, ok := w.travelTo(e, trough); !ok {
		return false
	}
	e.State = Moving
	return true
}

// dormant keeps an alien in a cave nobody has broken into to itself: it
// shuffles about now and then, unseen (dormantTurn, docs/caverns.md).
type dormant struct{}

func (dormant) act(w *World, e *Entity) bool {
	if !w.dormant(e) {
		return false
	}
	w.dormantTurn(e)
	return true
}

// grazeScum feeds a hungry peaceful alien on cave scum (alienGraze), which
// sets its own Cooldown: a bite's rest after eating, a step's while walking.
type grazeScum struct{}

func (grazeScum) act(w *World, e *Entity) bool {
	e.Quarry = 0
	return w.alienGraze(e, w.alienSpeciesFor(e))
}

// inert is an egg or cocoon's whole ladder: it lies where it is.
type inert struct{}

func (inert) act(w *World, e *Entity) bool {
	e.State, e.Quarry = Idle, 0
	return true
}

// wander takes an aimless step. It always acts, so it ends every ladder.
type wander struct{}

func (wander) act(w *World, e *Entity) bool {
	e.State = Idle
	w.wanderStep(e)
	return true
}
