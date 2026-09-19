package sim

import (
	"math/rand"
	"testing"
)

// benchWorld builds a large world with a carved central chamber and many
// colonists, for measuring per-tick cost as population scales. There is a rock
// frontier around the chamber so colonists have real mining and building work.
func benchWorld(colonists int) *World {
	return benchWorldSized(160, 160, colonists)
}

func benchWorldSized(width, height, colonists int) *World {
	cfg := DefaultConfig()
	cfg.Seed = 1
	cfg.Width, cfg.Height = width, height
	w := newWorld(cfg, rand.New(rand.NewSource(1)))

	const border = 20
	for y := border; y < w.Height-border; y++ {
		for x := border; x < w.Width-border; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	floors := w.freeFloorTiles()
	w.rng.Shuffle(len(floors), func(i, j int) { floors[i], floors[j] = floors[j], floors[i] })
	if colonists > len(floors) {
		colonists = len(floors)
	}
	for i := 0; i < colonists; i++ {
		w.spawn(Colonist, floors[i])
	}
	return w
}

func benchmarkStep(b *testing.B, colonists int) {
	w := benchWorld(colonists)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.step()
	}
}

func BenchmarkStep500(b *testing.B)  { benchmarkStep(b, 500) }
func BenchmarkStep2000(b *testing.B) { benchmarkStep(b, 2000) }

// BenchmarkStepIdle500 measures an established colony while every colonist is
// in the existing resting fast path: no need can become urgent and no wake-up
// can trigger a work search. This is the baseline for cognition caching, which
// must not make otherwise dormant colonists more expensive.
func BenchmarkStepIdle500(b *testing.B) {
	w := benchWorld(500)
	for _, e := range w.entities {
		if e.Kind != Colonist {
			continue
		}
		prepareRestingBenchmarkColonist(w, e)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.step()
	}
}

func prepareRestingBenchmarkColonist(w *World, e *Entity) {
	e.needRise = [numNeeds]int{}
	e.focus, e.Job, e.State = FocusIdle, JobNone, Idle
	e.resting = true
	e.wakeTick = int(^uint(0) >> 1)
	e.mindDirty = false
	e.nextThinkTick = e.wakeTick
}

func prepareSleepingBenchmarkColonist(w *World, e *Entity) bool {
	var bed Point
	found := false
	for _, d := range neighbors8 {
		p := e.Pos.Add(d.X, d.Y)
		if w.InBounds(p) && w.entityAt(p) == nil && w.TerrainAt(p) == Floor {
			bed, found = p, true
			break
		}
	}
	if !found {
		return false
	}
	w.SetTerrain(bed, Bed)
	e.needRise = [numNeeds]int{}
	e.Needs[NeedSleep] = w.cfg.Needs[NeedSleep].SeekAt
	e.needPhase[NeedSleep] = NeedPressing
	e.focus, e.Job, e.Need, e.State = FocusSleep, JobUse, NeedSleep, Sleeping
	e.useFacility, e.useFacilitySet = bed, true
	e.resting = false
	e.mindDirty = false
	e.nextThinkTick = int(^uint(0) >> 1)
	return true
}

func BenchmarkRestingColonistFastPath(b *testing.B) {
	w := benchWorldSized(64, 64, 1)
	e := w.entities[1]
	prepareRestingBenchmarkColonist(w, e)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.tick++
		w.colonistTurn(e)
	}
}

func BenchmarkSleepingColonistFastPath(b *testing.B) {
	w := benchWorldSized(64, 64, 1)
	w.cfg.Needs[NeedSleep].UseTicks = int(^uint(0) >> 1)
	e := w.entities[1]
	if !prepareSleepingBenchmarkColonist(w, e) {
		b.Fatal("no adjacent bed site")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.tick++
		w.colonistTurn(e)
	}
}

func BenchmarkStepSleeping500(b *testing.B) {
	w := benchWorld(500)
	w.cfg.Needs[NeedSleep].UseTicks = int(^uint(0) >> 1)
	for _, e := range w.entities {
		if e.Kind == Colonist && !prepareSleepingBenchmarkColonist(w, e) {
			prepareRestingBenchmarkColonist(w, e)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.step()
	}
}

func BenchmarkStepMixed500(b *testing.B) {
	w := benchWorld(500)
	i := 0
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		if e.Kind != Colonist {
			continue
		}
		switch i % 3 {
		case 0:
			prepareRestingBenchmarkColonist(w, e)
		case 1:
			prepareSleepingBenchmarkColonist(w, e)
		}
		i++
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.step()
	}
}

// BenchmarkRoomRefresh measures the incremental cost of one terrain change in a
// large open map: one chunk re-flooded plus a room relabel over the region
// graph. It should stay flat as the map grows, unlike a global flood fill.
func BenchmarkRoomRefresh(b *testing.B) {
	w := benchWorld(0) // 160x160 with a large carved chamber, no colonists
	w.refreshSpatial()
	p := Point{w.Width / 2, w.Height / 2}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			w.SetTerrain(p, Wall)
		} else {
			w.SetTerrain(p, Floor)
		}
		w.refreshSpatial()
	}
}

// BenchmarkPathfind measures one A* search across a large open room (a common
// case: a colonist near the middle routing to a frontier tile at the edge).
func BenchmarkPathfind(b *testing.B) {
	w := benchWorld(0) // 160x160 with a large carved chamber
	w.refreshSpatial()
	from := Point{w.Width / 2, w.Height / 2}
	target := Point{20, 20} // rock at the chamber's rock border
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.pathToAdjacent(from, target)
	}
}

// BenchmarkNeedSeek measures a tick where a large hungry population converges on
// a handful of facilities. This is the case flow fields target: one shared BFS
// per tick instead of a full-grid facility scan per colonist.
func BenchmarkNeedSeek(b *testing.B) {
	w := benchWorld(1000)
	// Scatter pods and toilets across the carved chamber as shared destinations.
	for gy := 30; gy < 130; gy += 25 {
		for gx := 30; gx < 130; gx += 25 {
			w.SetTerrain(Point{gx, gy}, NutrientPod)
			w.SetTerrain(Point{gx + 2, gy}, Toilet)
		}
	}
	w.refreshSpatial()
	// Make everyone hungry so they all seek a pod at once.
	for _, e := range w.entities {
		if e.Kind == Colonist {
			e.Needs[NeedFood] = w.cfg.Needs[NeedFood].SeekAt
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.step()
	}
}

// BenchmarkStepBigMap stresses the regime the shared frontier field targets: a
// large map with a big colony, where per-miner path search would multiply.
func BenchmarkStepBigMap(b *testing.B) {
	w := benchWorldSized(300, 300, 3000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.step()
	}
}

// benchWorldSmallColony builds a huge map with only a small carved-out colony
// near the center, mirroring a fresh game on a big map: most of the grid is
// still untouched Rock. This is the regime a full Width*Height scan in a
// per-tick or per-need-seek path would blow up on.
func benchWorldSmallColony(mapSize, chamber, colonists int) *World {
	cfg := DefaultConfig()
	cfg.Seed = 1
	cfg.Width, cfg.Height = mapSize, mapSize
	w := newWorld(cfg, rand.New(rand.NewSource(1)))

	cx, cy := mapSize/2, mapSize/2
	for y := cy - chamber/2; y < cy+chamber/2; y++ {
		for x := cx - chamber/2; x < cx+chamber/2; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	for gy := cy - chamber/2 + 5; gy < cy+chamber/2; gy += 20 {
		for gx := cx - chamber/2 + 5; gx < cx+chamber/2; gx += 20 {
			w.SetTerrain(Point{gx, gy}, NutrientPod)
			w.SetTerrain(Point{gx + 2, gy}, Toilet)
			w.SetTerrain(Point{gx + 4, gy}, Bed)
		}
	}
	floors := w.freeFloorTiles()
	w.rng.Shuffle(len(floors), func(i, j int) { floors[i], floors[j] = floors[j], floors[i] })
	if colonists > len(floors) {
		colonists = len(floors)
	}
	for i := 0; i < colonists; i++ {
		w.spawn(Colonist, floors[i])
	}
	w.refreshSpatial()
	return w
}

func BenchmarkStepSmallColonyOnHugeMap2500(b *testing.B) {
	w := benchWorldSmallColony(2500, 60, 50)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.step()
	}
}

func BenchmarkStepSmallColonyOnHugeMap10000(b *testing.B) {
	w := benchWorldSmallColony(10000, 60, 50)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.step()
	}
}

// BenchmarkFindRoomSiteNoFit models planRooms' routine "no site available yet"
// case on a huge, mostly-untouched map: a normal-sized room that just doesn't
// currently fit anywhere in the small starting chamber (every side-wall lane
// already claimed). Before capping the search radius to the carved area,
// failing to find a site forced the box search to double all the way out to
// the full map before giving up — the "every 16 ticks" pause.
func BenchmarkFindRoomSiteNoFit(b *testing.B) {
	w := benchWorldSmallColony(10000, 12, 0) // chamber too small for any room this wide
	width := bayWidth(roomFacilities)
	if _, ok := w.findRoomSite(width); ok {
		b.Fatal("expected no site to fit; benchmark no longer exercises the no-fit path")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.findRoomSite(width)
	}
}

// BenchmarkPublishSmallColonyOnHugeMap* measure one published frame — a tick
// plus the Snapshot the engine hands frontends — on a huge, mostly-untouched
// map. The two sizes should land within noise of each other: publishing shares
// unchanged tile pages between frames and reads terrain totals off the
// incremental counts, so a frame costs what the tick touched, not what the map
// measures. Copying the grid (and counting terrain by walking it) put the map's
// whole area on every tick and made 7000x7000 half the tick rate of 5000x5000.
func benchmarkPublish(b *testing.B, mapSize int) {
	w := benchWorldSmallColony(mapSize, 60, 50)
	w.step()
	w.snapshot(false, 8) // pay for the first grid outside the timed loop
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.step()
		w.snapshot(false, 8)
	}
}

func BenchmarkPublishSmallColonyOnHugeMap2500(b *testing.B)  { benchmarkPublish(b, 2500) }
func BenchmarkPublishSmallColonyOnHugeMap10000(b *testing.B) { benchmarkPublish(b, 10000) }
