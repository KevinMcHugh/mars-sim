package sim

import "testing"

// chooseFacilityReference is the exhaustive chooseFacility that the two-tier
// version in facilitychoice.go replaced: a BFS over everything the colonist can
// reach, then a scan of every facility. It is kept, test-only, as the oracle
// TestChooseFacilityMatchesReference compares the fast version against.
func (w *World) chooseFacilityReference(e *Entity, kind Terrain) Point {
	// The distance cells are reused across calls (and across ticks) via a
	// generation stamp, the same trick flowField uses (and the same flowCell),
	// so a call costs O(reachable area) rather than allocating and zeroing a
	// Width*Height slice every time — critical on a large map, where the
	// walkable area a colonist can actually reach is a tiny fraction of the
	// grid. The cells are paged for the same reason: only reachable tiles are
	// ever stamped. See pagedgrid.go.
	w.facilityGen++
	gen := w.facilityGen
	cells := &w.facilityCells
	reached := func(p Point) (int32, bool) {
		c := cells.at(p.X, p.Y)
		return c.dist, c.gen == gen
	}
	cells.set(e.Pos.X, e.Pos.Y, flowCell{gen: gen, dist: 0})
	queue := append(w.facilityQueue[:0], e.Pos)
	// Layered like flowField.rebuild, and off the same interior-page fast
	// path: uniform-cost BFS visits in non-decreasing distance order, so the
	// depth is the layer, and a node away from a page edge reaches all eight
	// neighbours through one page lookup.
	pd, levelEnd := int32(0), len(queue)
	for head := 0; head < len(queue); head++ {
		if head == levelEnd {
			pd++
			levelEnd = len(queue)
		}
		p := queue[head]
		page := cells.interiorPage(p.X, p.Y)
		for _, d := range neighbors8 {
			n := p.Add(d.X, d.Y)
			if !w.Walkable(n) {
				continue
			}
			np := page
			if np == nil {
				np = cells.pageAtAlloc(n.X, n.Y)
			}
			c := &np[offset(n.X, n.Y)]
			if c.gen == gen {
				continue
			}
			c.gen, c.dist = gen, pd+1
			queue = append(queue, n)
		}
	}
	w.facilityQueue = queue

	// Facilities of this kind are few even on a huge map, so iterate the
	// tracked set (see World.facilityTiles) instead of scanning every tile.
	facilities := w.facilityTiles[kind]

	best, bestDist := Point{}, int32(^uint32(0)>>1)
	for fac := range facilities {
		accessible := false
		congested := false
		for _, d := range neighbors8 {
			access := fac.Add(d.X, d.Y)
			if !w.Walkable(access) {
				continue
			}
			if _, ok := reached(access); !ok {
				continue
			}
			accessible = true
			if w.entityAt(access) != nil {
				congested = true
			}
			// A committed user in the approach counts as a queue even when
			// the access tile itself is currently free. This — not mere
			// nearby foot traffic — is what "congested" means: an earlier
			// version also flagged a facility whenever any other colonist
			// stood within one tile of any of its access tiles, whatever
			// that colonist was actually doing. Facilities are packed one
			// tile apart in a room (see construction.md), so their access
			// neighborhoods overlap; in a merely busy room — colonists
			// resting, chatting, walking through, using the facility next
			// door — that overbroad check could flag every facility in it as
			// "congested" at once, so this function's whole point (spread
			// users across reachable facilities) gave up and fell back to
			// "nearest for everyone," funneling a crowd onto one facility
			// while others sat genuinely idle beside it.
			queueCount := 0
			for _, other := range w.entities {
				if other == e || !other.Alive() || other.Kind != Colonist ||
					other.Job != JobUse || !other.useFacilitySet ||
					!other.useFacility.Equal(fac) {
					continue
				}
				queueCount++
			}
			if queueCount >= 1 {
				congested = true
			}
		}
		if !accessible {
			continue
		}
		// Prefer the nearest facility unless its approach is congested. If
		// every reachable option is busy, retain nearest as a fair fallback.
		//
		// d starts at "unreached", not at bestDist: seeding it from the running
		// best made a farther facility score as an exact tie (its own accesses
		// never beat bestDist, so d stayed there) and then win the lessPoint
		// tie-break. Which facility that hit depended on the iteration order of
		// w.facilityTiles -- a map -- so the same seed sent a colonist to a
		// different sink on different runs.
		d := int32(^uint32(0) >> 1)
		for _, n := range neighbors8 {
			access := fac.Add(n.X, n.Y)
			if !w.InBounds(access) {
				continue
			}
			if nd, ok := reached(access); ok && nd < d {
				d = nd
			}
		}
		if congested {
			continue
		}
		if d < bestDist || (d == bestDist && lessPoint(fac, best)) {
			best, bestDist = fac, d
		}
	}
	if bestDist < int32(^uint32(0)>>1) {
		return best
	}
	// If all facilities are congested, choosing the nearest still guarantees
	// progress once its current users leave rather than declaring the need
	// unreachable and starving the colonist.
	bestDist = int32(^uint32(0) >> 1)
	for fac := range facilities {
		for _, d := range neighbors8 {
			access := fac.Add(d.X, d.Y)
			if !w.InBounds(access) {
				continue
			}
			if nd, ok := reached(access); ok &&
				(nd < bestDist || (nd == bestDist && lessPoint(fac, best))) {
				best, bestDist = fac, nd
			}
		}
	}
	return best
}

// TestChooseFacilityMatchesReference plays several games and, between ticks
// (when rooms and the shared fields are both current), asks every colonist to
// choose every kind of facility with both versions. They must agree. The
// crowded case packs many hungry colonists around few pods so that the nearest
// pods are all congested and the second tier, the bounded search, has to
// answer too.
func TestChooseFacilityMatchesReference(t *testing.T) {
	// How each choice was answered, over all the games: by the field, by the
	// all-busy shortcut, or by the bounded search.
	var byField, allBusy, searched int
	for _, tc := range []struct {
		name      string
		seed      int64
		colonists int
		hungry    bool
	}{
		{"normal-a", 31, 30, false},
		{"normal-b", 32, 30, false},
		{"crowded", 33, 80, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.Seed = tc.seed
			cfg.Width, cfg.Height = 90, 60
			cfg.StartColonists = tc.colonists
			w := newTestWorld(t, cfg)
			compared := 0
			for tick := 0; tick < 600; tick++ {
				if tc.hungry && tick%50 == 0 {
					for _, e := range w.entities {
						if e.Kind == Colonist {
							e.Needs[NeedFood] = w.cfg.Needs[NeedFood].SeekAt
						}
					}
				}
				w.step()
				if tick%5 != 0 {
					continue
				}
				for kind, f := range w.fields {
					if f == nil || len(w.facilityTiles[kind]) == 0 {
						continue
					}
					f.builtTick = -2 // bring it up to date with this tick's last changes
					f.ensureFresh()
					for _, id := range w.entityIDsSorted() {
						e := w.entities[id]
						if e.Kind != Colonist || !e.Alive() {
							continue
						}
						want := w.chooseFacilityReference(e, Terrain(kind))
						got := w.chooseFacility(e, Terrain(kind))
						if got != want {
							t.Fatalf("tick %d, colonist %d at %v, %v: chooseFacility = %v, reference = %v",
								w.tick, id, e.Pos, Terrain(kind), got, want)
						}
						compared++
						room := w.roomOf(e.Pos)
						if room == 0 {
							searched++
							continue
						}
						w.facilityCommitted = nil
						switch _, free := w.facilityByField(e, Terrain(kind), room); {
						case free:
							byField++
						case len(w.facilityFound) > 0 && !w.anyFreeFacility(e, Terrain(kind), room):
							allBusy++
						default:
							searched++
						}
					}
				}
			}
			t.Logf("%d choices compared", compared)
		})
	}
	t.Logf("%d answered by the field, %d all busy, %d by search", byField, allBusy, searched)
	if byField == 0 || allBusy == 0 || searched == 0 {
		t.Fatal("not every path was exercised")
	}
}
