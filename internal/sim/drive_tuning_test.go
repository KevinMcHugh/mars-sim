package sim

import (
	"fmt"
	"os"
	"testing"
)

// TestDriveTuningReport plays default games and prints the numbers drives are
// tuned against (docs/drives.md, "Tuning"): meals per colonist-day, finished
// and interrupted nights, time in bed, starvation, time spent mining, and how
// often the ceiling consequences fire (pass-outs, soilings, loneliness). It
// asserts nothing and only runs with MARS_DRIVE_TUNING set:
//
//	MARS_DRIVE_TUNING=1 go test ./internal/sim -run TestDriveTuningReport -v
func TestDriveTuningReport(t *testing.T) {
	if os.Getenv("MARS_DRIVE_TUNING") == "" {
		t.Skip("set MARS_DRIVE_TUNING=1 to print the drive tuning report")
	}
	const ticks = 10800
	var sum tuningTally
	for seed := int64(1); seed <= 6; seed++ {
		cfg := DefaultConfig()
		cfg.Seed = seed
		w := NewEngine(cfg).world
		tally := runTuningGame(w, ticks)
		t.Logf("seed %d: %s", seed, tally)
		sum.add(tally)
	}
	t.Logf("total : %s", sum)
}

type tuningTally struct {
	colonistTicks, meals, finished, interrupted, inBed, mining, starved int
	passOuts, soilings, lonely                                          int
	ticksPerDay                                                         int
	wokenBy                                                             [numFocusKinds]int
}

func (a *tuningTally) add(b tuningTally) {
	a.colonistTicks += b.colonistTicks
	a.meals += b.meals
	a.finished += b.finished
	a.interrupted += b.interrupted
	a.inBed += b.inBed
	a.mining += b.mining
	a.starved += b.starved
	a.passOuts += b.passOuts
	a.soilings += b.soilings
	a.lonely += b.lonely
	a.ticksPerDay = b.ticksPerDay
	for i := range a.wokenBy {
		a.wokenBy[i] += b.wokenBy[i]
	}
}

func (a tuningTally) String() string {
	days := float64(a.colonistTicks) / float64(max(a.ticksPerDay, 1))
	pct := func(n int) float64 { return 100 * float64(n) / float64(max(a.colonistTicks, 1)) }
	woken := ""
	for f, n := range a.wokenBy {
		if n > 0 {
			woken += fmt.Sprintf(" %s:%d", FocusKind(f), n)
		}
	}
	return fmt.Sprintf("colonist-days %.0f  meals/day %.2f  nights finished %d interrupted %d (by%s)  in bed %.1f%%  mining %.1f%%  starved %d  passed out %d  soiled %d  lonely %d",
		days, float64(a.meals)/max(days, 1), a.finished, a.interrupted, woken, pct(a.inBed), pct(a.mining), a.starved,
		a.passOuts, a.soilings, a.lonely)
}

func runTuningGame(w *World, ticks int) tuningTally {
	tally := tuningTally{ticksPerDay: w.cfg.TicksPerDay()}
	prevFood := map[EntityID]int{}
	prevSleeping := map[EntityID]bool{}
	// Consequences are counted as the memories they leave: a reaction's
	// count only grows, short of the bounded log dropping it.
	seen := map[EntityID][3]int{}
	remembered := func(e *Entity) [3]int {
		return [3]int{memoriesOf(e, "passed-out"), memoriesOf(e, "soiled-self"), memoriesOf(e, "felt-lonely")}
	}
	starved0 := w.starved
	for i := 0; i < ticks; i++ {
		w.step()
		for _, id := range w.entityIDsSorted() {
			e := w.entities[id]
			if e == nil || e.Kind != Colonist || !e.Alive() {
				continue
			}
			tally.colonistTicks++
			food := w.driveLevel(e, DriveFood)
			if prev, ok := prevFood[id]; ok && food < prev-100 {
				tally.meals++
			}
			prevFood[id] = food
			sleeping := e.State == Sleeping
			if sleeping {
				tally.inBed++
			}
			if prevSleeping[id] && !sleeping {
				if w.driveLevel(e, DriveSleep) > 0 { // got up before the night was done
					tally.interrupted++
					tally.wokenBy[e.focus]++
				} else {
					tally.finished++
				}
			}
			prevSleeping[id] = sleeping
			now, before := remembered(e), seen[id]
			tally.passOuts += max(now[0]-before[0], 0)
			tally.soilings += max(now[1]-before[1], 0)
			tally.lonely += max(now[2]-before[2], 0)
			seen[id] = now
			if a, walking := activityOf(e); a == ActMining && !walking {
				tally.mining++
			}
		}
	}
	tally.starved = w.starved - starved0
	return tally
}
