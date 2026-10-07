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

// activityOf classifies what colonist e did this tick, and whether it was
// walking there rather than doing it. Its State counts when that names the
// work; otherwise (Moving, Idle) the job or focus says what the colonist is
// on its way to do, and a Moving colonist is walking there.
func activityOf(e *Entity) (a Activity, walking bool) {
	if a, ok := activityOfState(e.State); ok {
		return a, false
	}
	return activityOfPurpose(e), e.State == Moving || e.State == Climbing
}

// activityOfState is the activity a State names, if it names one.
func activityOfState(s State) (Activity, bool) {
	switch s {
	case Eating:
		return ActEating, true
	case Sleeping, PassedOut:
		return ActSleeping, true
	case Relieving:
		return ActRelieving, true
	case Talking:
		return ActSocializing, true
	case Crafting:
		return ActCooking, true
	case Mining:
		return ActMining, true
	case Building:
		return ActBuilding, true
	case Hauling, Storing:
		return ActHauling, true
	case Cleaning, Scraping:
		return ActCleaning, true
	case Fighting, Stomping:
		return ActFighting, true
	case Fleeing:
		return ActFleeing, true
	case Demolishing:
		return ActEscaping, true
	}
	return 0, false
}

// activityOfPurpose is what a colonist that is not visibly working is on its
// way to do: its job's, else its focus's.
func activityOfPurpose(e *Entity) Activity {
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
	case JobCraft, JobTend:
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
		a, walking := activityOf(e)
		w.actTally[a]++
		if walking {
			w.walkTally[a]++
		}
	}
}
