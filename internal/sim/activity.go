package sim

// ---- Colonist activity ---------------------------------------------------------
//
// What the colonists spend their time doing, for the Activity tab. Every tick
// each living colonist is counted under one Activity; the counts accumulate
// until the next Population sample, which carries them (see population.go).
// Like the rest of that history it is read-only bookkeeping: nothing in the
// simulation reads it back. See docs/activity-screen.md.

// Activity is a coarse, player-facing label for what a colonist is doing. It
// is coarser than State (hauling and storing are one thing to a player) and
// finer than it where State is vague: a colonist walking somewhere is credited
// to whatever it is walking there to do, since "moving" is most of the day and
// says nothing about how the colony spends it.
type Activity uint8

const (
	ActIdle        Activity = iota
	ActEating               // eating, or fetching a meal
	ActSleeping             // sleeping, or heading to bed
	ActRelieving            // using a toilet, or heading to one
	ActSocializing          // chatting
	ActCooking              // working a scumhouse recipe
	ActMining               // excavating rock
	ActBuilding             // constructing a structure
	ActHauling              // storing materials, selling and carrying goods
	ActCleaning             // scrubbing refuse, feeding the incinerator, scraping scum
	ActFighting             // firing on an alien, stomping a rat
	ActFleeing              // running from an alien
	ActEscaping             // breaking out of a sealed room

	NumActivities
)

func (a Activity) String() string {
	switch a {
	case ActIdle:
		return "idle"
	case ActEating:
		return "eating"
	case ActSleeping:
		return "sleeping"
	case ActRelieving:
		return "relieving"
	case ActSocializing:
		return "socializing"
	case ActCooking:
		return "cooking"
	case ActMining:
		return "mining"
	case ActBuilding:
		return "building"
	case ActHauling:
		return "hauling"
	case ActCleaning:
		return "cleaning"
	case ActFighting:
		return "fighting"
	case ActFleeing:
		return "fleeing"
	case ActEscaping:
		return "escaping"
	default:
		return "?"
	}
}

// activityOf classifies what colonist e did this tick: its State when that
// names the work, else (Moving, Idle) the job or focus it is travelling for.
func activityOf(e *Entity) Activity {
	switch e.State {
	case Eating:
		return ActEating
	case Sleeping:
		return ActSleeping
	case Relieving:
		return ActRelieving
	case Talking:
		return ActSocializing
	case Crafting:
		return ActCooking
	case Mining:
		return ActMining
	case Building:
		return ActBuilding
	case Hauling, Storing:
		return ActHauling
	case Cleaning, Scraping:
		return ActCleaning
	case Fighting, Stomping:
		return ActFighting
	case Fleeing:
		return ActFleeing
	case Demolishing:
		return ActEscaping
	}
	switch e.Job {
	case JobMine:
		return ActMining
	case JobBuild:
		return ActBuilding
	case JobEat:
		return ActEating
	case JobTalk:
		return ActSocializing
	case JobClean, JobScrape:
		return ActCleaning
	case JobStore, JobSell, JobCarry:
		return ActHauling
	case JobCraft:
		return ActCooking
	case JobDemolish:
		return ActEscaping
	}
	// JobUse, or no job: the focus says what the walk is for.
	switch e.focus {
	case FocusEat:
		return ActEating
	case FocusSleep:
		return ActSleeping
	case FocusRelieve:
		return ActRelieving
	case FocusSocialize:
		return ActSocializing
	case FocusFight:
		return ActFighting
	case FocusFlee:
		return ActFleeing
	case FocusEscape:
		return ActEscaping
	}
	return ActIdle
}

// tallyActivity counts colonist e's activity for this tick, toward the next
// Population sample.
func (w *World) tallyActivity(e *Entity) {
	if e.Kind == Colonist && e.Alive() {
		w.actTally[activityOf(e)]++
	}
}
