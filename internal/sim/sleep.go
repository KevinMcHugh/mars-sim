package sim

// A night's sleep. Sleep is the one drive whose satisfaction takes hours of
// clock time (see docs/days.md), which brings two rules the other drives do
// not have:
//
//   - While a colonist is in bed its other drives all but stop. That is not
//     code here: a sleeping colonist is in the asleep drive activity, and
//     each drive says how fast it grows there (DriveSpec.Activity). A night
//     is about eight clock hours, longer than food or bladder take to come
//     due, so without it hunger got colonists out of bed before any night
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
	e.State = Sleeping
	e.sleepBanked++
	if e.sleepBanked >= e.sleepTicks {
		e.sleepBanked = 0
		w.finishUse(e, w.cfg.Drives[DriveSleep])
	}
}
