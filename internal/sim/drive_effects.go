package sim

import (
	"fmt"
	"slices"
)

// Drive effects: what an event (a cup of coffee, a drink of water, later any
// substance) does to a colonist's drives over time. One event can touch
// several drives and change what it does as it wears off, so an effect is a
// profile of stages played in order. Within a stage everything is constant,
// which keeps drives lazy: a stage boundary is a scheduled refresh, the same
// steps-not-curves rule as drive consequences. See docs/drives.md.

// DriveEffectKind identifies a profile: its index in Config.DriveEffects.
type DriveEffectKind int

// DriveEffectProfile is one kind of effect. Stacks says whether applying it
// again while it is active adds a second instance (caffeine on caffeine) or
// starts the one already running over.
type DriveEffectProfile struct {
	Name   string
	Stacks bool
	Stages []DriveEffectStage // played in order; the effect ends after the last
}

// DriveEffectStage is what an effect does for Ticks ticks.
type DriveEffectStage struct {
	Ticks int
	Drive [numDrives]DriveEffectTerms // the zero value leaves a drive alone
}

// DriveEffectTerms is a stage's influence on one drive.
type DriveEffectTerms struct {
	// Instant is added to the true level once, as the stage begins, in
	// whole points: a permanent change (a big meal sating hunger).
	Instant int
	// Offset masks the level while the stage lasts, in whole points: the
	// colonist feels Offset less than the true level, which keeps growing
	// underneath. When the masking ends the felt level jumps back (the
	// caffeine crash).
	Offset int
	// Add is added to the base growth rate, in thousandths of a point a tick
	// (water filling the bladder).
	Add int
	// RateChange changes the growth rate by this percent: -60 slows it to
	// 40%, +20 speeds it to 120%.
	RateChange int
}

// activeEffect is one effect running on an entity: which stage it is in and
// the tick that stage ends.
type activeEffect struct {
	kind  DriveEffectKind
	stage int
	ends  int
}

func checkDriveEffects(profiles []DriveEffectProfile) error {
	for i, p := range profiles {
		if len(p.Stages) == 0 {
			return fmt.Errorf("drive effect %d (%s): needs at least one stage", i, p.Name)
		}
		for j, st := range p.Stages {
			if st.Ticks < 1 {
				return fmt.Errorf("drive effect %d (%s) stage %d: lasts %d ticks, want at least 1", i, p.Name, j, st.Ticks)
			}
			for d, terms := range st.Drive {
				if terms.RateChange < -100 {
					return fmt.Errorf("drive effect %d (%s) stage %d: %s rate change %d%% would reverse it",
						i, p.Name, j, DriveKind(d), terms.RateChange)
				}
			}
		}
	}
	return nil
}

func (w *World) effectStage(fx activeEffect) *DriveEffectStage {
	return &w.cfg.DriveEffects[fx.kind].Stages[fx.stage]
}

// applyEffect starts effect kind on e: its first stage's instant changes land
// now, and its masking and rate changes hold until the stage ends.
func (w *World) applyEffect(e *Entity, kind DriveEffectKind) {
	if !w.cfg.DriveEffects[kind].Stacks {
		e.effects = slices.DeleteFunc(e.effects, func(fx activeEffect) bool { return fx.kind == kind })
	}
	fx := activeEffect{kind: kind, ends: w.tick + w.cfg.DriveEffects[kind].Stages[0].Ticks}
	e.effects = append(e.effects, fx)
	w.refreshEffectDrives(e, w.effectStage(fx))
}

// advanceEffects moves every effect whose stage has run out on to its next
// stage, or ends it. colonistTurn calls it first thing each turn: an effect
// wears on whatever the colonist is doing.
func (w *World) advanceEffects(e *Entity) {
	if e.nextEffectTick == 0 || w.tick < e.nextEffectTick {
		return
	}
	var entered []*DriveEffectStage
	kept := e.effects[:0]
	for _, fx := range e.effects {
		for fx.stage < len(w.cfg.DriveEffects[fx.kind].Stages) && w.tick >= fx.ends {
			fx.stage++
			if fx.stage < len(w.cfg.DriveEffects[fx.kind].Stages) {
				fx.ends += w.effectStage(fx).Ticks
				entered = append(entered, w.effectStage(fx))
			}
		}
		if fx.stage < len(w.cfg.DriveEffects[fx.kind].Stages) {
			kept = append(kept, fx)
		}
	}
	e.effects = kept
	var instant [numDrives]int
	for _, st := range entered {
		for d := range st.Drive {
			instant[d] += st.Drive[d].Instant * driveUnit
		}
	}
	// Every drive can have lost masking or a rate change from a stage that
	// ended, so all of them are refreshed, not only those the new stages name.
	for d := DriveKind(0); d < numDrives; d++ {
		w.refreshDrive(e, d, instant[d])
	}
	w.scheduleEffects(e)
}

// refreshEffectDrives refreshes every drive after an effect has started,
// applying the instant changes of the stage it started in.
func (w *World) refreshEffectDrives(e *Entity, entered *DriveEffectStage) {
	for d := DriveKind(0); d < numDrives; d++ {
		w.refreshDrive(e, d, entered.Drive[d].Instant*driveUnit)
	}
	w.scheduleEffects(e)
}

// scheduleEffects caches the earliest stage end, the next tick advanceEffects
// has anything to do.
func (w *World) scheduleEffects(e *Entity) {
	e.nextEffectTick = 0
	for _, fx := range e.effects {
		if e.nextEffectTick == 0 || fx.ends < e.nextEffectTick {
			e.nextEffectTick = fx.ends
		}
	}
	if e.nextEffectTick > 0 && e.nextEffectTick < e.nextThinkTick {
		e.nextThinkTick = e.nextEffectTick
	}
}
