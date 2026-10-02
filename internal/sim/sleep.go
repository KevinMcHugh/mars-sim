package sim

// A night's sleep. Sleep is the one drive whose satisfaction takes hours of
// clock time (see docs/days.md). None of it is special-cased here: a colonist
// in bed is in the asleep drive activity, where its sleep drive falls (a
// negative percent) and its other drives all but stop (DriveSpec.Activity).
// The night is over when the sleep drive reaches 0. A night that is
// interrupted is simply a drive that has fallen part of the way: the colonist
// gets up that much less tired, and the next lie-down starts from there.

// TicksPerHour is one clock hour, in ticks: TicksPerDay over 24. Sleep
// traits move a night by this much.
func (c *Config) TicksPerHour() int {
	return max(c.TicksPerDay()/24, 1)
}

// sleepTick spends one tick of e's night in bed, and ends the night once the
// sleep drive has fallen to 0.
func (w *World) sleepTick(e *Entity) {
	e.State = Sleeping
	if w.driveLevel(e, DriveSleep) == 0 {
		w.finishUse(e, w.cfg.Drives[DriveSleep])
	}
}
