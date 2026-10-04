package sim

import "testing"

// nearestMineByScan is the full-frontier scan claimNearestMine replaced: every
// frontier tile, nearest first, ties to the row-major-first tile. It claims
// nothing. claimNearestMine must always agree with it.
func nearestMineByScan(w *World, e *Entity) (Point, bool) {
	room := w.roomOf(e.Pos)
	if room == 0 {
		return Point{}, false
	}
	var best Point
	found := false
	bestDist := 0
	for p := range w.board.frontier {
		if w.board.isClaimed(p) || !w.frontierReachable(p, room) ||
			!e.Inventory.CanAddAll(miningYield(w.TileAt(p))...) {
			continue
		}
		if d := e.Pos.Chebyshev(p); !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
			best, bestDist, found = p, d, true
		}
	}
	return best, found
}

// TestClaimNearestMineMatchesFullScan runs real colonies and, every few ticks,
// asks for every colonist's nearest mine both ways: the chunk-ring search must
// pick the same tile as the full scan (or agree there is none), including the
// row-major tie-break and colonists whose room has no reachable frontier.
func TestClaimNearestMineMatchesFullScan(t *testing.T) {
	cases := []struct {
		name  string
		cfg   func() Config
		ticks int
	}{
		{"seed16-200x200", func() Config {
			c := DefaultConfig()
			c.Seed = 16
			c.StartColonists = 20
			c.Width, c.Height = 200, 200
			c.ZoningAuto = true
			c.StartAliens = 0
			c.CavernNestPercent = 0
			return c
		}, 3000},
		{"seed4-caverns-300x300", func() Config {
			c := DefaultConfig()
			c.Seed = 4
			c.StartColonists = 30
			c.Width, c.Height = 300, 300
			c.ZoningAuto = true
			c.CavernPercent = 20
			return c
		}, 2000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := NewEngine(tc.cfg()).world
			found, none, ties := 0, 0, 0
			for tick := 0; tick < tc.ticks; tick++ {
				w.step()
				if tick%10 != 0 {
					continue
				}
				for _, id := range w.entityTurnOrder() {
					e := w.entities[id]
					if e == nil || e.Kind != Colonist || !e.Alive() {
						continue
					}
					want, wantOK := nearestMineByScan(w, e)
					got, gotOK := w.claimNearestMine(e)
					if got != want || gotOK != wantOK {
						t.Fatalf("tick %d colonist %d at %v: ring search %v,%v; full scan %v,%v",
							tick, id, e.Pos, got, gotOK, want, wantOK)
					}
					if !gotOK {
						none++
						continue
					}
					found++
					w.board.releaseMine(got, e.ID)
					// Count queries the tie-break decided: another eligible
					// tile at the same distance that loses on row-major order.
					for _, d := range neighbors8 {
						q := got.Add(d.X, d.Y)
						if q != got && w.board.isFrontier(q) && !w.board.isClaimed(q) &&
							e.Pos.Chebyshev(q) == e.Pos.Chebyshev(got) && w.frontierReachable(q, w.roomOf(e.Pos)) {
							ties++
							break
						}
					}
				}
			}
			if found == 0 || ties == 0 {
				t.Fatalf("test exercised found=%d none=%d ties=%d queries; pick a config with more mining", found, none, ties)
			}
			t.Logf("found=%d none=%d ties=%d", found, none, ties)
		})
	}
}

// TestClaimNearestMineNoneFits covers the search's worst case, which real runs
// rarely reach: a colonist who can carry no rock at all, so no tile qualifies
// and every chunk on the map is visited. It must claim nothing, as the full
// scan does, and then find the scan's tile once the pack is empty again.
func TestClaimNearestMineNoneFits(t *testing.T) {
	w := benchWorldSized(160, 160, 1)
	w.refreshSpatial() // rooms, so the colonist has one
	var e *Entity
	for _, id := range w.entityTurnOrder() {
		e = w.entities[id]
	}
	for e.Inventory.Add(RawRock, 1) {
	}
	if _, ok := nearestMineByScan(w, e); ok {
		t.Fatal("full scan found a mine for a full pack; the test setup is wrong")
	}
	if p, ok := w.claimNearestMine(e); ok {
		t.Fatalf("claimed %v for a colonist who cannot carry its yield", p)
	}
	e.Inventory.RemoveAll(RawRock)
	want, wantOK := nearestMineByScan(w, e)
	got, gotOK := w.claimNearestMine(e)
	if !wantOK || got != want || !gotOK {
		t.Fatalf("empty pack: ring search %v,%v; full scan %v,%v", got, gotOK, want, wantOK)
	}
	if !w.board.isClaimed(got) {
		t.Fatalf("claimNearestMine returned %v without claiming it", got)
	}
}
