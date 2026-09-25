package sim

import (
	"fmt"
	"testing"
)

// A repaired field must be exactly the field a rebuild would produce: every
// reader (seekers, miners, cleaners) steps by these distances, so any
// difference would change the simulation. This walks busy worlds tick by tick
// — digging, building, claiming and releasing frontier rock — and after every
// tick checks each field against one rebuilt from nothing.
func TestFlowFieldRepairMatchesRebuild(t *testing.T) {
	for _, tc := range []struct {
		name     string
		seed     int64
		frontier bool
	}{
		{"facilities", 21, false},
		{"frontier", 22, true},
		{"frontier-crowded", 23, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.Seed = tc.seed
			cfg.Width, cfg.Height = 90, 60
			cfg.StartColonists, cfg.StartMice, cfg.StartAliens = 30, 6, 1
			if tc.frontier {
				cfg.FrontierFieldMinArea, cfg.FrontierFieldMinColonists = 1, 1
			}
			w := newTestWorld(t, cfg)
			repairs := 0
			for tick := 0; tick < 800; tick++ {
				w.step()
				fields := append([]*flowField{w.frontier}, w.fields[:]...)
				for fi, f := range fields {
					if f == nil {
						continue
					}
					// Apply whatever changed since the field was last read,
					// even if a reader already brought it up to date earlier
					// this tick.
					if !f.full && len(f.touched) > 0 {
						repairs++
					}
					f.builtTick = -2
					f.ensureFresh()
					if err := sameAsRebuild(w, f); err != nil {
						t.Fatalf("tick %d, field %d: %v", w.tick, fi, err)
					}
				}
			}
			if repairs == 0 {
				t.Fatal("no field was ever repaired; the test exercised nothing")
			}
		})
	}
}

// sameAsRebuild compares f against a field built from scratch with the same
// goals.
func sameAsRebuild(w *World, f *flowField) error {
	fresh := newFlowField(w, f.seed, f.goal)
	fresh.rebuild()
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			p := Point{x, y}
			if got, want := f.at(p), fresh.at(p); got != want {
				return fmt.Errorf("%v: repaired distance %d, rebuilt %d", p, got, want)
			}
		}
	}
	return nil
}

// The goal predicate must agree with the seed function tile for tile, since a
// repair asks one and a rebuild asks the other.
func TestFlowFieldGoalAgreesWithSeed(t *testing.T) {
	cfg := testConfig()
	cfg.Seed = 5
	cfg.StartColonists = 20
	w := newTestWorld(t, cfg)
	for tick := 0; tick < 400; tick++ {
		w.step()
		if tick%50 != 0 {
			continue
		}
		for fi, f := range append([]*flowField{w.frontier}, w.fields[:]...) {
			if f == nil {
				continue
			}
			seeded := map[Point]bool{}
			f.seed(func(p Point) {
				if w.Walkable(p) {
					seeded[p] = true
				}
			})
			for y := 0; y < w.Height; y++ {
				for x := 0; x < w.Width; x++ {
					p := Point{x, y}
					if f.goal(p) != seeded[p] {
						t.Fatalf("tick %d, field %d, %v: goal says %v, seed says %v", w.tick, fi, p, f.goal(p), seeded[p])
					}
				}
			}
		}
	}
}

// A field touched past maxTouched gives up on repairing and rebuilds.
func TestFlowFieldTooManyTouchesRebuilds(t *testing.T) {
	w := newTestWorld(t, testConfig())
	f := w.frontier
	f.ensureFresh()
	for i := 0; i <= maxTouched; i++ {
		f.touch(Point{i % w.Width, 0})
	}
	if !f.full || len(f.touched) != 0 {
		t.Fatalf("after %d touches: full=%v, %d pending; want a full rebuild queued", maxTouched+1, f.full, len(f.touched))
	}
}
