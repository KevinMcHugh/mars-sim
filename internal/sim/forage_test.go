package sim

import (
	"strings"
	"testing"
)

// forageWorld is propertyWorld (open floor from (5,5) to (22,15), rock all
// round) with a scumhouse, no scum anywhere, scum growth off, and the safety
// net off: a hungry colonist there has nothing to eat until it finds some.
func forageWorld(t *testing.T) (*World, Point) {
	t.Helper()
	w := propertyWorld(t)
	w.cfg.InfiniteFood = false
	w.cfg.ScumSpawnPPM, w.cfg.ScumSpreadPercent = 0, 0
	house := Point{10, 6, LandingLevel}
	w.SetTerrain(house, Scumhouse)
	w.refreshSpatial()
	noScum(w)
	return w, house
}

// hungryColonist spawns a broke colonist at p with pressing hunger.
func hungryColonist(w *World, p Point) *Entity {
	e := w.spawn(Colonist, p)
	w.transfer(ColonistOwner(e.ID), Community, e.wallet)
	setHunger(w, e, w.cfg.Drives[DriveFood].SeekAt)
	return e
}

// The doom loop: hungry, drop work, no food, take work, drop it again. A
// hungry colonist with nothing to eat used to drop its dig every turn and be
// handed it straight back by assignWorkJob, so the dig never got past its
// first tick. Whatever it does now, it sees through: rock gets dug.
func TestAHungryColonistFinishesWhatItStarts(t *testing.T) {
	w, _ := forageWorld(t)
	e := hungryColonist(w, Point{22, 10, LandingLevel})
	rock := w.landing().terrainCounts[Rock]
	starts := 0
	for i := 0; i < 60; i++ {
		before := e.Job
		w.hungryWithoutFood(e)
		if before == JobNone && e.Job != JobNone {
			starts++
		}
	}
	if dug := rock - w.landing().terrainCounts[Rock]; dug == 0 {
		t.Fatalf("60 hungry turns and no rock dug (%d jobs started): it is dropping and re-taking its work", starts)
	}
	if starts > 20 {
		t.Fatalf("%d jobs started in 60 turns: it is flickering between jobs", starts)
	}
}

// Work that feeds nobody, under way when hunger turns pressing, is dropped
// once, not re-taken: the colonist forages instead.
func TestAHungryColonistDropsUnrelatedWorkForGood(t *testing.T) {
	w, _ := forageWorld(t)
	e := hungryColonist(w, Point{12, 10, LandingLevel})
	w.SetTerrain(Point{15, 10, LandingLevel}, Rock) // a pillar: nearest rock, nothing unseen round it
	w.refreshSpatial()
	w.assignMineTarget(e, Point{15, 10, LandingLevel})
	w.landing().board.claimMine(Point{15, 10, LandingLevel}, e.ID)
	w.hungryWithoutFood(e)
	if e.Job != JobMine || !e.foraging || e.Target == (Point{15, 10, LandingLevel}) {
		t.Fatalf("job %v target %v foraging %v; want a forager's dig into unseen rock, not the pillar",
			e.Job, e.Target, e.foraging)
	}
}

// Rock nobody has seen may hide scum. A hungry colonist with nothing to eat
// digs for it, scrapes what it exposes, cooks it and eats, well before it
// would have starved.
func TestAForagerDigsOutItsSupper(t *testing.T) {
	w, _ := forageWorld(t)
	// Scum two tiles into the rock all round: hidden until a dig exposes it.
	for y := 3; y <= 17; y++ {
		for x := 3; x <= 24; x++ {
			if x == 3 || x == 24 || y == 3 || y == 17 {
				w.setScum(Point{x, y, LandingLevel}, 3)
			}
		}
	}
	if len(w.landing().exposedScum) != 0 {
		t.Fatalf("%d patches exposed before any digging", len(w.landing().exposedScum))
	}
	e := hungryColonist(w, Point{14, 10, LandingLevel})
	id := e.ID
	fed := false
	for i := 0; i < 400 && !fed; i++ {
		w.step()
		e = w.entities[id]
		if e == nil {
			t.Fatalf("starved at tick %d", w.tick)
		}
		fed = w.driveLevel(e, DriveFood) < w.cfg.Drives[DriveFood].SeekAt/2
	}
	if !fed {
		t.Fatalf("not fed after 400 ticks: job %v, hunger %d, carrying %d scum", e.Job, w.driveLevel(e, DriveFood), e.ownCarried(CaveScum))
	}
	logged := false
	for _, l := range w.log.entries {
		logged = logged || strings.Contains(l.Text, "goes looking for scum")
	}
	if !logged {
		t.Fatal("fed without ever logging that it went foraging")
	}
	if dug := w.landing().exploredCount; dug == 0 {
		t.Fatal("nothing explored")
	}
}

// Meals on the colony's stove are food on its way: a hungry colonist waits
// for them rather than walking off to dig, but only while there are enough
// for everyone hungry. A neighbour cooking its own supper is no reason to
// wait at all.
func TestAForagerWaitsForTheColonysCooking(t *testing.T) {
	w, house := forageWorld(t)
	c := w.storageContainers[house]
	c.Inventory.Add(CaveScum, 2)
	c.credit(Community, CaveScum, 2) // one meal's worth
	cook := w.spawn(Colonist, Point{11, 7, LandingLevel})
	r, ok := scumMealRecipe()
	if !ok {
		t.Fatal("no scum recipe")
	}
	for i, rr := range recipes {
		if rr.Name == r.Name {
			cook.recipe = i
		}
	}
	cook.Job, cook.Target, cook.craftFor = JobCraft, house, Community
	w.workshopClaims[house] = cook.ID
	e := hungryColonist(w, Point{14, 10, LandingLevel})
	e.focus = FocusEat
	if w.planForage(e) {
		t.Fatalf("went foraging (job %v) with a meal on the colony's stove for it", e.Job)
	}

	other := hungryColonist(w, Point{16, 10, LandingLevel})
	other.focus = FocusEat
	w.hungryTick = -1 // a new count this tick
	if !w.planForage(e) || e.Job != JobMine {
		t.Fatalf("job %v: one meal coming for two hungry colonists; it should forage", e.Job)
	}
	w.clearJob(e)
	other.focus = FocusIdle // fed: no longer waiting on the stove

	w.hungryTick = -1
	cook.craftFor = ColonistOwner(cook.ID)
	if !w.planForage(e) || e.Job != JobMine {
		t.Fatalf("job %v: a neighbour's own supper is no reason to wait", e.Job)
	}
}

// A forager's pack full of rubble has no room for scum: it unloads first,
// everything in one trip.
func TestAForagerEmptiesAFullPackFirst(t *testing.T) {
	w, _ := forageWorld(t)
	chest := Point{8, 12, LandingLevel}
	w.SetTerrain(chest, Storage)
	w.refreshSpatial()
	e := hungryColonist(w, Point{14, 10, LandingLevel})
	for e.Inventory.CanAdd(CaveScum, 1) {
		e.Inventory.Add(RawRock, 64)
	}
	if !w.planForage(e) || e.Job != JobStore || e.Target != chest {
		t.Fatalf("job %v target %v; want unloading at the chest %v", e.Job, e.Target, chest)
	}
}

// Short of food with its walls scraped bare, the colony digs where it will
// expose scum: into rock it has not seen, not the nearest face. With food
// to spare, mining is ordinary mining again.
func TestTheColonyProspectsWhenItsShort(t *testing.T) {
	w, _ := forageWorld(t)
	pillar := Point{15, 10, LandingLevel}
	w.SetTerrain(pillar, Rock)
	w.refreshSpatial()
	e := w.spawn(Colonist, Point{14, 10, LandingLevel})
	setHunger(w, e, 0)

	w.cfg.MealReserve = 3
	if !w.foodWanted() {
		t.Fatal("a colony with no meals doesn't want food")
	}
	w.assignWorkJob(e)
	if e.Job != JobMine || e.Target == pillar || w.unexploredAround(e.Target) == 0 {
		t.Fatalf("short of food: job %v target %v; want a dig into unseen rock", e.Job, e.Target)
	}
	w.clearJob(e)

	w.cfg.MealReserve = 0
	w.assignWorkJob(e)
	if e.Job != JobMine || e.Target != pillar {
		t.Fatalf("food to spare: job %v target %v; want the nearest rock, %v", e.Job, e.Target, pillar)
	}
}

// Of the rock it could dig, a prospector picks the most unseen tiles per
// tick of walking and digging, and never rock with nothing unseen round it.
func TestProspectingPrefersTheUnknown(t *testing.T) {
	w, _ := forageWorld(t)
	w.SetTerrain(Point{15, 10, LandingLevel}, Rock) // pillar
	w.refreshSpatial()
	e := hungryColonist(w, Point{14, 10, LandingLevel})
	if !w.tryProspect(e, false) {
		t.Fatal("nothing to prospect")
	}
	if w.unexploredAround(e.Target) == 0 {
		t.Fatalf("prospected %v, which has nothing unseen round it", e.Target)
	}
	dig := w.workTicks(e, SkillMining, w.cfg.MineTicks)
	got := w.unexploredAround(e.Target) * 1000 / (dig + e.Pos.Chebyshev(e.Target))
	for p := range w.landing().board.frontier {
		if p == e.Target || !w.frontierReachable(p, w.roomOf(e.Pos)) {
			continue
		}
		if r := w.unexploredAround(p) * 1000 / (dig + e.Pos.Chebyshev(p)); r > got {
			t.Fatalf("prospected %v (%d/1000 a tick) over %v (%d/1000)", e.Target, got, p, r)
		}
	}
}
