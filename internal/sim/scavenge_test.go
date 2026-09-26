package sim

import "testing"

// noScum clears worldgen's cave scum, so a test's only food is what it puts
// down itself.
func noScum(w *World) {
	w.scum = map[Point]scumPatch{}
	w.exposedScum = map[Point]struct{}{}
}

// hungryRat puts a rat at p with its hunger just past seeking.
func hungryRat(w *World, p Point) *Entity {
	r := w.spawn(Rat, p)
	r.Needs[NeedFood] = w.cfg.Needs[NeedFood].SeekAt + 10
	r.needSince[NeedFood] = w.tick
	return r
}

// A hungry rat eats a body lying nearby — even with a pod in reach — and
// that is one body the colony will never render.
func TestRatsEatTheDeadBeforeRaidingPods(t *testing.T) {
	w := propertyWorld(t)
	noScum(w)
	w.SetTerrain(Point{9, 10}, NutrientPod)
	w.refreshSpatial()
	body := Point{14, 10}
	w.addCorpse(body, AlienCorpse)
	r := hungryRat(w, Point{10, 10}) // right beside the pod
	for i := 0; i < 100 && w.corpsesAt(body) > 0; i++ {
		w.step()
		if r.Job == JobUse {
			t.Fatal("the rat went for the pod with a carcass in reach")
		}
	}
	if w.corpsesAt(body) != 0 {
		t.Fatal("the rat never ate the carcass")
	}
	if w.needLevel(r, NeedFood) >= w.cfg.Needs[NeedFood].SeekAt {
		t.Fatalf("the rat is still hungry (%d) after eating", w.needLevel(r, NeedFood))
	}
}

// Rats eat gore and exposed scum too, and only scum they can get at.
func TestRatsEatGoreAndExposedScum(t *testing.T) {
	w := propertyWorld(t)
	noScum(w)
	gore := Point{12, 10}
	w.addGore(gore)
	r := hungryRat(w, Point{10, 10})
	for i := 0; i < 100 && w.goreAt(gore) > 0; i++ {
		w.step()
	}
	if w.goreAt(gore) != 0 {
		t.Fatal("the rat never ate the gore")
	}

	buried := Point{30, 30} // deep in rock: not exposed
	w.scum[buried] = scumPatch{amount: w.cfg.ScumMax, since: w.tick}
	w.refreshScumExposure(buried)
	face := Point{5, 4} // rock on the edge of the open floor
	w.scum[face] = scumPatch{amount: w.cfg.ScumMax, since: w.tick}
	w.refreshScumExposure(face)
	if w.scavengeable(buried) || !w.scavengeable(face) {
		t.Fatalf("scavengeable: buried %v, face %v", w.scavengeable(buried), w.scavengeable(face))
	}
	r.Needs[NeedFood] = w.cfg.Needs[NeedFood].SeekAt + 10
	r.needSince[NeedFood] = w.tick
	r.Job = JobNone
	for i := 0; i < 200 && w.scumAt(face) == w.cfg.ScumMax; i++ {
		w.step()
	}
	if w.scumAt(face) == w.cfg.ScumMax {
		t.Fatal("the rat never ate the exposed scum")
	}
	if w.scumAt(buried) != w.cfg.ScumMax {
		t.Fatal("scum sealed in rock was eaten")
	}
}

// With the safety net off, rats live on biomatter where they used to starve at
// once. Food is scarce — colonists scrape the same patches — so what is
// compared is how long the rats last in total, not whether they make it.
func TestRatsLiveOnScumWithoutTheSafetyNet(t *testing.T) {
	ratTicks := func(scum int) int {
		cfg := testConfig()
		cfg.Width, cfg.Height = 60, 36
		cfg.StartAliens, cfg.StartCats = 0, 0
		cfg.StartRats = 6
		cfg.InfiniteFood = false
		cfg.ScumPercent = scum
		w := newTestWorld(t, cfg)
		total := 0
		for i := 0; i < 600; i++ {
			w.step()
			total += w.countKind(Rat)
		}
		return total
	}
	if fed, starved := ratTicks(6), ratTicks(0); fed <= starved*3/2 {
		t.Fatalf("rat-ticks alive: %d with scum, %d without", fed, starved)
	}
}
