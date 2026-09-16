package sim

import (
	"strings"
	"testing"
)

// cleanTestWorld builds an empty world with one carved chamber, so cleaning
// tests control exactly what refuse exists and what there is to clean it with.
// The colonist is spawned in the middle of the chamber; the incinerator, when
// asked for, sits inside its right-hand edge with floor all around it.
func cleanTestWorld(t *testing.T, withIncinerator bool) (*World, *Entity, Point) {
	t.Helper()
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	for y := 5; y <= 15; y++ {
		for x := 5; x <= 20; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	incinerator := Point{19, 10}
	if withIncinerator {
		w.SetTerrain(incinerator, Incinerator)
	}
	w.refreshSpatial()

	return w, w.spawn(Colonist, Point{10, 10}), incinerator
}

// runColonist gives one colonist n turns without advancing the world clock, so
// a cleaning job runs to completion without needs rising, projects being
// planned, or anything else competing for the colonist's attention.
func runColonist(w *World, e *Entity, n int) {
	for i := 0; i < n; i++ {
		w.colonistTurn(e)
	}
}

// The whole loop: a colonist with nothing urgent to do walks to a mess, scrubs
// it up, carries it to the incinerator, and burns it — leaving the tile clean
// and its own hands empty.
func TestColonistCleansAndIncineratesRefuse(t *testing.T) {
	w, colonist, _ := cleanTestWorld(t, true)
	mess := Point{7, 7}
	w.addGore(mess)
	w.addCorpse(mess)

	runColonist(w, colonist, 300)

	if got := w.refuseAt(mess); got != 0 {
		t.Errorf("refuse at %v = %d, want 0 (cleaned up)", mess, got)
	}
	if got := colonist.Inventory.Count(Viscera) + colonist.Inventory.Count(Corpse); got != 0 {
		t.Errorf("colonist still carries %d refuse, want 0 (burned)", got)
	}
	if w.refuseTotal() != 0 {
		t.Errorf("world refuse total = %d, want 0", w.refuseTotal())
	}
	if !loggedContaining(w, "incinerates") {
		t.Errorf("no incineration in the log: %v", w.log.tail(len(w.log.entries)))
	}
}

// Cleaning is work, not an idle whim: a colonist surrounded by diggable rock
// still cleans first, or in a real colony the mess would never be touched.
func TestCleaningOutranksMining(t *testing.T) {
	w, colonist, _ := cleanTestWorld(t, true)
	w.addGore(Point{9, 9})

	w.colonistTurn(colonist)

	if colonist.Job != JobClean {
		t.Fatalf("job = %v, want JobClean rather than falling through to mining", colonist.Job)
	}
}

// With nowhere to burn refuse, a colonist leaves it alone: picking a body up
// with no incinerator in reach would only move the mess into an inventory slot.
func TestNoCleaningWithoutIncinerator(t *testing.T) {
	w, colonist, _ := cleanTestWorld(t, false)
	mess := Point{7, 7}
	w.addGore(mess)

	runColonist(w, colonist, 50)

	if colonist.Job == JobClean {
		t.Error("colonist took a cleaning job with no incinerator to haul to")
	}
	if got := w.refuseAt(mess); got != 1 {
		t.Errorf("refuse at %v = %d, want 1 (left alone)", mess, got)
	}
	if got := colonist.Inventory.Count(Viscera); got != 0 {
		t.Errorf("colonist picked up %d viscera with nowhere to put it", got)
	}
}

// A load already in hand must never be stranded: once an incinerator exists,
// the carrier's next work search is a haul, whatever else is on offer.
func TestCarriedRefuseIsHauledOnceAnIncineratorExists(t *testing.T) {
	w, colonist, incinerator := cleanTestWorld(t, true)
	colonist.Inventory.Add(Corpse, 1)

	w.colonistTurn(colonist)

	if colonist.Job != JobClean || colonist.clean != cleanHaul {
		t.Fatalf("job = %v stage = %v, want a JobClean haul", colonist.Job, colonist.clean)
	}
	if colonist.Target != incinerator {
		t.Errorf("haul target = %v, want the incinerator at %v", colonist.Target, incinerator)
	}

	runColonist(w, colonist, 200)
	if got := colonist.Inventory.Count(Corpse); got != 0 {
		t.Errorf("colonist still carries %d corpses, want 0 (burned)", got)
	}
}

// Two colonists should not converge on the same splatter: the first claims it,
// and the second either finds other refuse or does something else entirely.
func TestRefuseTileIsClaimedByOneCleaner(t *testing.T) {
	w, first, _ := cleanTestWorld(t, true)
	second := w.spawn(Colonist, Point{11, 10})
	mess := Point{7, 7}
	w.addGore(mess)

	w.colonistTurn(first)
	w.colonistTurn(second)

	if first.Job != JobClean || first.Target != mess {
		t.Fatalf("first colonist job = %v target = %v, want a clean of %v", first.Job, first.Target, mess)
	}
	if second.Job == JobClean && second.Target == mess {
		t.Error("both colonists claimed the same refuse tile")
	}
}

// Refuse inside solid rock — an alien shot dead while burrowing — is not
// cleanable yet, and a cleaner must not be sent to stand at a tile it can never
// scrub. Mining through to it later is what makes it cleanable.
func TestUnreachableRefuseIsNotTargeted(t *testing.T) {
	w, colonist, _ := cleanTestWorld(t, true)
	buried := Point{7, 3} // outside the carved chamber, still Rock
	w.addGore(buried)

	w.colonistTurn(colonist)

	if colonist.Job == JobClean {
		t.Errorf("colonist targeted refuse at %v, which is inside rock", colonist.Target)
	}
	if got := w.refuseAt(buried); got != 1 {
		t.Errorf("refuse inside rock = %d, want 1 (still there, waiting to be dug out)", got)
	}
}

// Raising a structure over a mess scrapes it away. Without that, refuse under a
// wall would be reported forever by a colony that could never reach it — while
// digging, which exposes a stain rather than burying it, must leave it alone.
func TestBuildingOverRefuseClearsItButDiggingDoesNot(t *testing.T) {
	w, _, _ := cleanTestWorld(t, false)

	built := Point{8, 8}
	w.addGore(built)
	w.addCorpse(built)
	w.SetTerrain(built, Wall)
	if got := w.refuseAt(built); got != 0 {
		t.Errorf("refuse under a new wall = %d, want 0", got)
	}
	if w.refuseTotal() != 0 {
		t.Errorf("refuse total = %d, want 0 after the only mess was built over", w.refuseTotal())
	}

	dug := Point{7, 3} // rock, outside the chamber
	w.addGore(dug)
	w.SetTerrain(dug, Floor)
	if got := w.refuseAt(dug); got != 1 {
		t.Errorf("refuse on a freshly mined tile = %d, want 1 (exposed, not erased)", got)
	}
}

// A Tidy colonist — the one the sight of gore hits hardest — ranges further to
// get rid of it.
func TestTidyColonistSearchesFurtherForMess(t *testing.T) {
	w, plain, _ := cleanTestWorld(t, true)
	tidy := w.spawn(Colonist, Point{11, 11})
	tidy.Profile = &Profile{Traits: []Trait{TraitTidy}}

	if got, want := w.cleanRadius(plain), w.cfg.CleanRadius; got != want {
		t.Errorf("plain colonist clean radius = %d, want %d", got, want)
	}
	if got, want := w.cleanRadius(tidy), 2*w.cfg.CleanRadius; got != want {
		t.Errorf("tidy colonist clean radius = %d, want %d", got, want)
	}
}

// A death that nothing eats leaves a body behind to be hauled away; one that
// ends in a predator's stomach leaves only the stains.
func TestDeathsLeaveCorpsesWhenNothingEatsThem(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	stompSpot := Point{5, 5}
	w.SetTerrain(stompSpot, Floor)
	w.SetTerrain(Point{4, 5}, Floor)
	stomper := w.spawn(Colonist, Point{4, 5})
	w.stomp(stomper, w.spawn(Mouse, stompSpot))
	if got := w.tiles[w.index(stompSpot)].Corpses; got != 1 {
		t.Errorf("corpses at the stomp = %d, want 1", got)
	}

	eatenSpot := Point{8, 5}
	w.SetTerrain(eatenSpot, Floor)
	w.SetTerrain(Point{9, 5}, Floor)
	cat := w.spawn(Cat, Point{9, 5})
	w.pounce(cat, w.spawn(Mouse, eatenSpot))
	if got := w.tiles[w.index(eatenSpot)].Corpses; got != 0 {
		t.Errorf("corpses where a cat ate its catch = %d, want 0", got)
	}
}

// Starving to death leaves a body where the colonist fell.
func TestStarvationLeavesACorpse(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	spot := Point{6, 6}
	w.SetTerrain(spot, Floor)
	w.refreshSpatial()
	victim := w.spawn(Colonist, spot)
	victim.HP = 1
	victim.Needs[NeedFood] = w.cfg.Needs[NeedFood].Max
	victim.needSince[NeedFood] = w.tick

	w.colonistTurn(victim)

	if w.entities[victim.ID] != nil {
		t.Fatal("colonist should have starved")
	}
	if got := w.tiles[w.index(spot)].Corpses; got != 1 {
		t.Errorf("corpses where the colonist starved = %d, want 1", got)
	}
}

// Once life support and bunks are covered, a mess on the floor is what makes
// the colony mark out a trash room — and it only ever wants the one.
func TestColonyPlansATrashRoomForItsRefuse(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0
	w := newTestWorld(t, cfg)

	// Satisfy every other demand so sanitation is what planRooms has left.
	center := Point{w.Width / 2, w.Height / 2}
	desired := w.desiredFacilities(w.countKind(Colonist))
	for i, kind := range []Terrain{NutrientPod, Toilet, Bed} {
		for n := 0; n < desired; n++ {
			w.SetTerrain(center.Add(-3+i, -3-n), kind)
		}
	}
	w.refreshSpatial()

	w.planRooms()
	if named(w.projects, "trash room") {
		t.Fatal("a trash room was planned with no refuse to burn")
	}

	w.addGore(center.Add(2, 2))
	w.planRooms()
	if !named(w.projects, "trash room") {
		t.Fatalf("no trash room planned for a dirty colony; projects = %v", projectNames(w.projects))
	}

	// One incinerator serves the whole colony: a second mess must not queue a
	// second room.
	before := len(w.projects)
	w.addGore(center.Add(3, 3))
	w.planRooms()
	if len(w.projects) != before {
		t.Errorf("projects = %v, want no second trash room", projectNames(w.projects))
	}
}

// End to end, through step(): a colony that keeps finding messes builds itself
// a trash room and burns the refuse in it, with nothing scripted beyond the
// mess itself. This is the test that would catch the loop breaking anywhere
// along its length — planning, siting, building, claiming, hauling, burning.
func TestColonyBuildsATrashRoomAndBurnsItsRefuse(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0 // no colonist-eating; the mess below is the only refuse
	w := newTestWorld(t, cfg)

	for i := 0; i < 8000; i++ {
		w.step()
		// Re-dirty the colony now and then. A single stain is not a reliable
		// fixture: a room raised on top of it scrapes it away (see SetTerrain),
		// and the point here is the loop, not one particular tile.
		if i%200 == 0 && w.refuseTotal() == 0 {
			dirtyNearAColonist(w)
		}
		if loggedContaining(w, "incinerates") {
			if w.countTerrain(Incinerator) == 0 {
				t.Fatal("refuse was burned without an incinerator existing")
			}
			return
		}
	}
	t.Fatalf("colony never incinerated anything: refuse=%d incinerators=%d projects=%v",
		w.refuseTotal(), w.countTerrain(Incinerator), projectNames(w.projects))
}

// dirtyNearAColonist drops gore on a free floor tile beside some colonist, the
// way a stomped mouse or a gunned-down alien would.
func dirtyNearAColonist(w *World) {
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		if e.Kind != Colonist {
			continue
		}
		for _, d := range neighbors8 {
			if p := e.Pos.Add(d.X, d.Y); w.Walkable(p) && !w.occupied(p) {
				w.addGore(p)
				return
			}
		}
	}
}

func named(projects []*project, name string) bool {
	for _, p := range projects {
		if p.name == name {
			return true
		}
	}
	return false
}

func projectNames(projects []*project) []string {
	out := make([]string, 0, len(projects))
	for _, p := range projects {
		out = append(out, p.name)
	}
	return out
}

func loggedContaining(w *World, substr string) bool {
	for _, line := range w.log.tail(len(w.log.entries)) {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}
