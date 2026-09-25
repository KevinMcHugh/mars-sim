package sim

import (
	"slices"
	"testing"
)

// entityIDsNearSorted must return exactly what a scan of every entity would:
// observeNearby's memories and stimuli depend on both the set and its order.
func TestEntityIDsNearSortedMatchesAFullScan(t *testing.T) {
	cfg := testConfig()
	cfg.Seed = 3
	cfg.Width, cfg.Height = 70, 45
	cfg.StartColonists, cfg.StartMice, cfg.StartAliens = 20, 20, 2
	w := newTestWorld(t, cfg)
	for tick := 0; tick < 200; tick++ {
		w.step()
		if tick%20 != 0 {
			continue
		}
		for _, center := range []Point{{0, 0}, {w.Width - 1, w.Height - 1}, {w.Width / 2, w.Height / 2}, {17, 31}} {
			for _, radius := range []int{0, 1, 5, 16, 40} {
				var want []EntityID
				for _, id := range w.entityIDsSorted() {
					if center.Chebyshev(w.entities[id].Pos) <= radius {
						want = append(want, id)
					}
				}
				if got := w.entityIDsNearSorted(center, radius); !slices.Equal(got, want) {
					t.Fatalf("tick %d, %v r=%d: got %v, want %v", w.tick, center, radius, got, want)
				}
			}
		}
	}
}
