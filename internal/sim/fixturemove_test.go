package sim

import "testing"

// Relocating a fixture carries everything that was the old one's to the new
// one: a chest's goods and whose they are, an incubator's scum, a fixture's
// owner and who may use it, a stove's pantry link both ways.
func TestRelocatingAFixtureCarriesWhatIsItsOwn(t *testing.T) {
	w := rockSiteWorld(t)
	carve(w, Point{4, 4}, Point{20, 12}, Floor)
	w.refreshSpatial()

	oldChest, newChest := Point{6, 6}, Point{10, 6}
	w.SetTerrain(oldChest, Storage)
	w.SetTerrain(newChest, Storage)
	owner := ColonistOwner(7)
	stock(w, oldChest, owner, IronOre, 5)
	w.setFixtureOwner(oldChest, owner, AccessPrivate)
	w.relocateFixture(oldChest, newChest)
	c := w.storageContainers[newChest]
	if c == nil || c.Pos != newChest || c.held(owner, IronOre) != 5 {
		t.Fatalf("the goods did not follow the chest: %+v", c)
	}
	if _, ok := w.storageContainers[oldChest]; ok {
		t.Fatal("the old place still has a container")
	}
	if f := w.fixtures[newChest]; f == nil || f.Owner != owner || f.Access != AccessPrivate {
		t.Fatalf("the new chest's record is %+v, want the owner's and private", f)
	}

	oldInc, newInc := Point{6, 10}, Point{10, 10}
	w.SetTerrain(oldInc, Incubator)
	w.SetTerrain(newInc, Incubator)
	stock(w, oldInc, Community, CaveScum, w.incubatorSeed()+3)
	w.relocateFixture(oldInc, newInc)
	if got := w.ripeScum(w.storageContainers[newInc]); got != 3 {
		t.Fatalf("the moved incubator has %d ripe scum, want the 3 it had", got)
	}

	stove, pantry, newStove := Point{14, 6}, Point{16, 6}, Point{14, 10}
	w.SetTerrain(stove, Scumhouse)
	w.SetTerrain(pantry, Storage)
	w.SetTerrain(newStove, Scumhouse)
	w.pantryOf[stove], w.pantryHouse[pantry] = pantry, stove
	w.relocateFixture(stove, newStove)
	if w.pantryOf[newStove] != pantry || w.pantryHouse[pantry] != newStove {
		t.Fatal("the pantry is not linked to the stove's new place")
	}
	if _, ok := w.pantryOf[stove]; ok {
		t.Fatal("the stove's old place still has a pantry")
	}
	// Tearing the old place down afterwards undoes none of it.
	w.demolish(stove)
	if w.pantryOf[newStove] != pantry {
		t.Fatal("tearing down the old stove dropped the new one's pantry")
	}
}

// Colonists carry a move through: the fixture goes up in its new place
// without materials, the old comes down after, and the goods are in the new
// chest, still their owner's.
func TestColonistsMoveAChest(t *testing.T) {
	w, a := builtRoom(t, storageRoom, Point{14, 10}, 1) // chest at (15, 10)
	b := &roomRecord{lo: Point{20, 10}, hi: Point{22, 12}, zone: ZoneStorage, issuer: Community}
	carve(w, Point{19, 9}, Point{23, 14}, Floor)
	w.refreshSpatial()
	from := Point{15, 10}
	stock(w, from, Community, IronOre, 4)
	// A chest nearer the old place than the new: had the goods been emptied
	// out the usual way, they would have gone here.
	decoy := Point{12, 12}
	w.SetTerrain(decoy, Storage)
	w.refreshSpatial()
	to := Point{21, 10}
	if !w.designateMove(a, b, []fixtureMove{{from: from, to: to, kind: Storage}}) {
		t.Fatal("the move was not designated")
	}
	for i := 0; i < 3; i++ {
		w.spawn(Colonist, Point{16 + i, 14})
	}
	stepFed(t, w, 4000, func() bool { return len(w.projects) == 0 })
	if len(w.projects) > 0 {
		t.Fatal("the move never finished")
	}
	if w.TerrainAt(from) != Floor || w.TerrainAt(to) != Storage {
		t.Fatalf("old place %v, new place %v; want floor and a chest", w.TerrainAt(from), w.TerrainAt(to))
	}
	if got := w.storageContainers[to].held(Community, IronOre); got != 4 {
		t.Fatalf("the moved chest holds %d of the colony's ore, want 4", got)
	}
	if got := w.storageContainers[decoy].held(Community, IronOre); got != 0 {
		t.Fatalf("%d ore was emptied into the nearer chest instead of moving with its own", got)
	}
}

// A small room is emptied into a bigger one of its zone nearby, and once it
// stands empty its walls are cleared away and its record goes with them.
func TestASmallRoomIsEmptiedAndClearedAway(t *testing.T) {
	w, big := builtRoom(t, dormRoom, Point{8, 10}, 2) // inside x 8..10
	carve(w, Point{14, 8}, Point{22, 15}, Floor)
	w.refreshSpatial()
	small := alsoBuilt(t, w, dormRoom, roomFrame{o: Point{18, 10}, width: 1}, 1) // one bunk at (18, 10)
	w.refreshSpatial()
	if _, _, ok := mergeBox(big, small); ok {
		t.Fatal("test setup: the rooms are close enough to join")
	}
	if !w.tidyRooms() {
		t.Fatal("the small dormitory was not emptied")
	}
	p := w.projects[0]
	if p.from != small || p.room != big {
		t.Fatalf("project %q moves from %p into %p, want the small room into the big", p.name, p.from, p.room)
	}
	for i := 0; i < 3; i++ {
		w.spawn(Colonist, Point{12 + i, 14})
	}
	stepFed(t, w, 4000, func() bool { return len(w.projects) == 0 })
	if w.TerrainAt(Point{18, 10}) != Floor || w.countTerrain(Bed) != 3 {
		t.Fatalf("after the move: (18,10) is %v and there are %d bunks, want floor and 3", w.TerrainAt(Point{18, 10}), w.countTerrain(Bed))
	}
	if !w.tidyRooms() {
		t.Fatal("the empty room was not cleared away")
	}
	stepFed(t, w, 4000, func() bool { return len(w.projects) == 0 })
	for _, rec := range w.roomRecords {
		if rec == small {
			t.Fatal("the cleared room is still recorded")
		}
	}
	if w.TerrainAt(Point{17, 10}) == Wall || w.TerrainAt(Point{19, 10}) == Wall {
		t.Fatal("the empty room's walls still stand")
	}
	if len(w.roomRecords) != 1 || w.countTerrain(Bed) != 3 {
		t.Fatalf("%d rooms and %d bunks, want the one room with all 3", len(w.roomRecords), w.countTerrain(Bed))
	}
}
