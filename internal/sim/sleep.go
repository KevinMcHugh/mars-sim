package sim

// A night's sleep. Sleep is the one need whose satisfaction takes hours of
// clock time (see docs/days.md), which brings two rules the other needs do
// not have:
//
//   - While a colonist is in bed its other needs are paused. A night is about
//     eight clock hours, longer than food or bladder take to come due, so
//     without the pause hunger got colonists out of bed before any night
//     finished.
//   - A night that is interrupted is banked, not lost: sleepBanked carries
//     over, and the next lie-down picks up where it left off.

// TicksPerHour is one clock hour, in ticks: TicksPerDay over 24. Sleep
// traits move a night by this much.
func (c *Config) TicksPerHour() int {
	return max(c.TicksPerDay()/24, 1)
}

// sleepTick spends one tick of e's night in bed, and finishes the night once
// it has slept its sleepTicks.
func (w *World) sleepTick(e *Entity) {
	w.fallAsleep(e)
	e.State = Sleeping
	e.sleepBanked++
	if e.sleepBanked >= e.sleepTicks {
		e.sleepBanked = 0
		w.wakeUp(e)
		w.finishUse(e, w.cfg.Drives[DriveSleep])
	}
}

// fallAsleep pauses e's other needs at their current levels.
func (w *World) fallAsleep(e *Entity) {
	if e.asleep {
		return
	}
	w.rebaseWakingDrives(e)
	e.asleep = true
	w.pauseDrivesWhileAsleep(e)
	w.syncWakingDrivePhases(e)
}

// wakeUp resumes e's other needs from where they were paused. colonistTurn
// calls it for a colonist that has left its bed for any reason.
func (w *World) wakeUp(e *Entity) {
	if !e.asleep {
		return
	}
	w.rebaseWakingDrives(e)
	e.asleep = false
	for n := DriveKind(0); n < numDrives; n++ {
		if n != DriveSleep {
			e.driveRise[n] = e.wakeRise[n]
		}
	}
	w.syncWakingDrivePhases(e)
}

// pauseDrivesWhileAsleep stops every need but sleep from rising, keeping the
// rates to restore on waking. The levels must already be rebased to now.
func (w *World) pauseDrivesWhileAsleep(e *Entity) {
	for n := DriveKind(0); n < numDrives; n++ {
		if n != DriveSleep {
			e.wakeRise[n], e.driveRise[n] = e.driveRise[n], 0
		}
	}
}

// rebaseWakingDrives folds the rise so far into each non-sleep need's base, so
// a change of rate applies only from now on.
func (w *World) rebaseWakingDrives(e *Entity) {
	for n := DriveKind(0); n < numDrives; n++ {
		if n != DriveSleep {
			e.Drives[n], e.driveSince[n] = w.driveLevel(e, n), w.tick
		}
	}
}

func (w *World) syncWakingDrivePhases(e *Entity) {
	for n := DriveKind(0); n < numDrives; n++ {
		if n != DriveSleep {
			w.syncDrivePhase(e, n)
		}
	}
}
