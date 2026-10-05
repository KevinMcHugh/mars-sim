package sim

// Drive activities: how hard a colonist is working its body, which scales how
// fast each drive grows (DriveSpec.Activity). Two tables make it pluggable:
// driveActivityNames names the classes, and activityDrives files every
// player-facing Activity under one. A new class is an enum value and a name
// here, and gets a config knob on every drive automatically; a new Activity
// needs a row in activityDrives, which TestEveryActivityHasADriveActivity
// enforces. See docs/drives.md.

// DriveActivity is how strenuous what a colonist is doing is, as far as its
// drives are concerned. The zero value is unset, never a real class, so an
// Activity missing from activityDrives cannot silently fall into one.
type DriveActivity uint8

const (
	driveActivityNone DriveActivity = iota // unset
	DriveAsleep
	DriveIdle
	DriveWorking
	DriveLabor
	// DriveUnconscious is a colonist that passed out (ConsequencePassOut):
	// asleep on the floor, which rests it less than a bed does.
	DriveUnconscious

	driveActivityEnd // keep last
)

// numDriveActivities counts the real classes (not driveActivityNone).
const numDriveActivities = int(driveActivityEnd) - 1

// driveActivityNames names each class for the config file and the docs,
// indexed by DriveActivity.index().
var driveActivityNames = [numDriveActivities]string{"asleep", "idle", "working", "labor", "unconscious"}

// index is a's slot in per-class arrays such as DriveSpec.Activity.
func (a DriveActivity) index() int { return int(a) - 1 }

func (a DriveActivity) String() string {
	if a == driveActivityNone || a >= driveActivityEnd {
		return "unset"
	}
	return driveActivityNames[a.index()]
}

// activityDrive is the drive activity of one Activity: Doing while the
// colonist is visibly at it, Walking while it is on its way.
type activityDrive struct {
	Doing, Walking DriveActivity
}

// activityDrives files every Activity under a drive activity. Walking is
// ordinary work except where the walk is the strenuous part (a hauler is
// carrying the load; a fleeing colonist is running).
var activityDrives = [NumActivities]activityDrive{
	ActIdle:        {DriveIdle, DriveWorking},
	ActEating:      {DriveIdle, DriveWorking},
	ActSleeping:    {DriveAsleep, DriveWorking},
	ActRelieving:   {DriveIdle, DriveWorking},
	ActSocializing: {DriveIdle, DriveWorking},
	ActCooking:     {DriveWorking, DriveWorking},
	ActMining:      {DriveLabor, DriveWorking},
	ActBuilding:    {DriveLabor, DriveWorking},
	ActHauling:     {DriveLabor, DriveLabor},
	ActCleaning:    {DriveWorking, DriveWorking},
	ActFighting:    {DriveLabor, DriveLabor},
	ActFleeing:     {DriveLabor, DriveLabor},
	ActEscaping:    {DriveLabor, DriveWorking},
}

// driveActivityOf is the drive activity colonist e is in now. Doing applies
// only while its State names the activity (asleep means in bed, not merely
// wanting to be); walking there is Walking, and standing around waiting for
// it (no bed free yet) is idle. Passed out is its own class: the Activity tab
// counts it as sleeping, but the floor is not a bed.
func driveActivityOf(e *Entity) DriveActivity {
	if e.State == PassedOut {
		return DriveUnconscious
	}
	if a, ok := activityOfState(e.State); ok {
		return activityDrives[a].Doing
	}
	if e.State == Moving {
		return activityDrives[activityOfPurpose(e)].Walking
	}
	return DriveIdle
}

// syncDriveActivity refreshes a colonist's drives when its drive activity has
// changed since its last turn. step calls it after every colonist turn; the
// rates otherwise hold, so this costs one comparison a tick.
func (w *World) syncDriveActivity(e *Entity) {
	if e.Kind != Colonist || !e.Alive() {
		return
	}
	if a := driveActivityOf(e); a != e.driveActivity {
		e.driveActivity = a
		w.refreshDrives(e)
	}
}
