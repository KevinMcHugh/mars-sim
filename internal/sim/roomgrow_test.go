package sim

import "testing"

// builtDorm is builtRoom for a dormitory.
func builtDorm(t *testing.T, o Point, n int) (*World, *roomRecord) {
	t.Helper()
	return builtRoom(t, dormRoom, o, n)
}

// builtRoom is rockSiteWorld with a finished, south-facing room of r with n
// fixtures in a rock niche at o, its lanes and approach row carved out, and
// its project pruned, as a colony would have left it.
func builtRoom(t *testing.T, r roomRecipe, o Point, n int) (*World, *roomRecord) {
	t.Helper()
	w := rockSiteWorld(t)
	width := r.roomWidth(n)
	carve(w, Point{o.X - 2, o.Y + roomBackV}, Point{o.X + width + 1, o.Y + roomFrontV + roomApproach}, Floor)
	w.refreshSpatial()
	f := roomFrame{o: o, width: width}
	if !w.roomSiteClear(f, map[Point]bool{}, siteRules{}) {
		t.Fatalf("test setup: the %s's site is not clear", r.name)
	}
	if !w.designateRoom(r, f, n, Community) {
		t.Fatalf("test setup: the %s was not designated", r.name)
	}
	raise(w, w.projects[0])
	w.pruneProjects()
	return w, w.roomRecords[0]
}

// roomSealed reports whether rec's walls stand whole but for one doorway.
func roomSealed(w *World, rec *roomRecord) bool {
	gaps := 0
	for v := roomBackV; v <= roomFrontV; v++ {
		for u := -1; u <= rec.f.width; u++ {
			edge := v == roomBackV || v == roomFrontV || u < 0 || u == rec.f.width
			if edge && w.TerrainAt(rec.f.at(u, v)) != Wall {
				gaps++
			}
		}
	}
	return gaps == 1
}

// A dormitory short of bunks grows: its right wall comes down, a new one goes
// up four tiles further out, the back and front walls are carried across to
// meet it, and two bunks go in at the same spacing as the old ones. The
// treasury pays for all of it, the demolition at its own wage.
func TestDormitoryGrowsByMovingASideWall(t *testing.T) {
	o := Point{18, 10}
	w, rec := builtDorm(t, o, 2) // interior x 18..20, side walls 17 and 21
	before := w.balance(Community)

	if !w.expandRoom(dormRoom, 2) {
		t.Fatal("the dormitory did not grow")
	}
	p := w.projects[0]
	if p.room != rec {
		t.Fatal("the expansion is not tied to the dormitory it grows")
	}
	var demolish, beds []Point
	for _, tk := range p.tasks {
		switch {
		case tk.terrain == Floor && tk.clears == Wall:
			demolish = append(demolish, tk.pos)
			if tk.phase != roomDemolishPhase {
				t.Errorf("demolition at %v in phase %d, want %d", tk.pos, tk.phase, roomDemolishPhase)
			}
			if tk.order == nil || tk.order.Pay != Money(w.cfg.WageDemolish) {
				t.Errorf("demolition at %v is not paid wage-demolish", tk.pos)
			}
		case tk.terrain == Bed:
			beds = append(beds, tk.pos)
		}
	}
	if want := []Point{{21, 10}, {21, 11}, {21, 12}}; !equalPoints(demolish, want) {
		t.Fatalf("tears down %v, want the old right wall's inside rows %v", demolish, want)
	}
	if want := []Point{{22, 10}, {24, 10}}; !equalPoints(beds, want) {
		t.Fatalf("new bunks at %v, want %v", beds, want)
	}
	if paid := before - w.balance(Community); paid != w.projectCost(p) || paid <= 0 {
		t.Fatalf("the treasury paid %v for an expansion costing %v", paid, w.projectCost(p))
	}
	if w.expandRoom(dormRoom, 2) {
		t.Fatal("a second expansion was marked out while the first is under way")
	}

	raise(w, p)
	if rec.n != 4 || rec.f.width != 7 || rec.f.o != o {
		t.Fatalf("record after growing: %+v n=%d, want width 7 from %v with 4 bunks", rec.f, rec.n, o)
	}
	if !roomSealed(w, rec) {
		t.Fatal("the grown dormitory's walls are not whole but for its doorway")
	}
	if got := w.countTerrain(Bed); got != 4 {
		t.Fatalf("%d bunks, want 4", got)
	}
	if !w.sameRoom(Point{24, 11}, Point{19, 14}) {
		t.Fatal("the new end of the dormitory is not reached through the old doorway")
	}
}

// With no room to the right, the room grows to the left; its frame's origin
// moves with its left wall.
func TestRoomGrowsLeftWhenTheRightIsTaken(t *testing.T) {
	o := Point{18, 10}
	w, rec := builtDorm(t, o, 2)
	carve(w, Point{23, 9}, Point{23, 13}, Hull) // something in the way
	w.refreshSpatial()

	if !w.expandRoom(dormRoom, 1) {
		t.Fatal("the dormitory did not grow")
	}
	raise(w, w.projects[0])
	if want := (Point{16, 10}); rec.f.o != want || rec.f.width != 5 || rec.n != 3 {
		t.Fatalf("record after growing left: %+v n=%d, want width 5 from %v", rec.f, rec.n, want)
	}
	if w.TerrainAt(Point{16, 10}) != Bed || w.TerrainAt(Point{17, 10}) != Floor || w.TerrainAt(Point{15, 11}) != Wall {
		t.Fatal("the left end is not a bunk, a gap where the wall stood, and a new wall")
	}
	if !roomSealed(w, rec) {
		t.Fatal("the grown dormitory's walls are not whole but for its doorway")
	}
}

// A storage room has an aisle, so its next container stands where the old
// wall stood, and the aisle moves out past it.
func TestStorageRoomGrowsIntoItsOldWall(t *testing.T) {
	w := rockSiteWorld(t)
	o := Point{18, 10}
	width := storageRoom.roomWidth(1) // aisle, container, aisle: x 18..20
	carve(w, Point{o.X - 2, o.Y + roomBackV}, Point{o.X + width + 5, o.Y + roomFrontV + roomApproach}, Floor)
	w.refreshSpatial()
	if !w.designateRoom(storageRoom, roomFrame{o: o, width: width}, 1, Community) {
		t.Fatal("test setup: the storage room was not designated")
	}
	raise(w, w.projects[0])
	w.pruneProjects()
	rec := w.roomRecords[0]

	if !w.expandRoom(storageRoom, 1) {
		t.Fatal("the storage room did not grow")
	}
	raise(w, w.projects[0])
	if w.TerrainAt(Point{19, 10}) != Storage || w.TerrainAt(Point{21, 10}) != Storage {
		t.Fatal("want containers at x 19 (the old one) and 21 (where the old wall stood)")
	}
	if w.TerrainAt(Point{22, 10}) != Floor || w.TerrainAt(Point{23, 10}) != Wall {
		t.Fatal("want the aisle at x 22 and the new wall at x 23")
	}
	if rec.n != 2 || !roomSealed(w, rec) {
		t.Fatalf("record n=%d, sealed=%v; want 2 containers behind whole walls", rec.n, roomSealed(w, rec))
	}
}

// A kitchen grows by a whole kitchen: a stove where its old wall stood and,
// two tiles on, a pantry linked to it, as a new kitchen's would be. It never
// grows by a stove alone, and a narrow kitchen of one stove does not grow.
func TestKitchenGrowsByAStoveAndItsPantry(t *testing.T) {
	o := Point{18, 10}
	w, rec := builtRoom(t, scumhouseRoom, o, 2) // aisle, stove, gap, pantry, aisle: x 18..22
	if w.expandRoom(scumhouseRoom, 1) {
		t.Fatal("a kitchen grew by a stove without its pantry")
	}
	if !w.expandRoom(scumhouseRoom, scumhouseRoom.fullBay()) {
		t.Fatal("the kitchen did not grow")
	}
	raise(w, w.projects[0])
	house, pantry := Point{23, 10}, Point{25, 10}
	if w.TerrainAt(house) != Scumhouse || w.TerrainAt(pantry) != Storage {
		t.Fatalf("want a stove at %v and a pantry at %v", house, pantry)
	}
	if got, ok := w.pantryFor(house); !ok || got != pantry {
		t.Fatalf("the new stove's pantry is %v (%v), want %v", got, ok, pantry)
	}
	if rec.n != 4 || !roomSealed(w, rec) {
		t.Fatalf("record n=%d, sealed=%v; want 4 fixtures behind whole walls", rec.n, roomSealed(w, rec))
	}

	narrow := scumhouseRoom
	narrow.aisle = false
	w, _ = builtRoom(t, narrow, o, 1)
	if w.expandRoom(scumhouseRoom, scumhouseRoom.fullBay()) {
		t.Fatal("a one-stove kitchen grew; its new end would pair a pantry with the old stove's neighbor")
	}
}

// An ordered kitchen, incubator or meeting hall goes into one the colony
// already has, a new room's worth of fixtures at a time, as an ordered
// dormitory does.
func TestOrdersGrowEveryKindOfExpandingRoom(t *testing.T) {
	for _, tc := range []struct {
		r     roomRecipe
		n     int
		order func(w *World) *int
	}{
		{scumhouseRoom, 2, func(w *World) *int { return &w.manualScumhouses }},
		{incubatorRoom, 2, func(w *World) *int { return &w.manualIncubators }},
		{hallRoom, 2, func(w *World) *int { return &w.manualHalls }},
		{storageRoom, 1, func(w *World) *int { return &w.manualStorageRooms }},
	} {
		w, rec := builtRoom(t, tc.r, Point{14, 10}, tc.n)
		*tc.order(w) = 1
		w.planRooms()
		if len(w.projects) != 1 || w.projects[0].room != rec {
			t.Fatalf("%s: the order did not grow the %s the colony has", tc.r.name, tc.r.name)
		}
		if want := tc.n + tc.r.fullBay(); rec.n != want {
			t.Fatalf("%s: grew to %d fixtures, want %d", tc.r.name, rec.n, want)
		}
	}
}

// An expansion never raises a wall against one already standing: where the
// new side wall would stand one tile short of another wall, it is refused,
// and where that wall stands exactly where the new one would go, it is shared.
func TestExpansionNeverDoublesAWall(t *testing.T) {
	o := Point{18, 10}
	w, rec := builtDorm(t, o, 2)
	carve(w, Point{22, 9}, Point{28, 13}, Floor)
	carve(w, Point{26, 9}, Point{26, 13}, Wall) // a neighbor's wall, just past x 25
	w.refreshSpatial()
	none := map[Point]bool{}

	if w.expansionClear(rec, 2, true, none, none) {
		t.Fatal("accepted a new side wall at x 25 against the wall at x 26")
	}
	if !w.expansionClear(rec, 1, true, none, none) {
		t.Fatal("refused a one-bunk expansion with clear floor past its new wall")
	}

	carve(w, Point{26, 9}, Point{26, 13}, Floor)
	carve(w, Point{25, 9}, Point{25, 13}, Wall) // now exactly where the new wall goes
	w.refreshSpatial()
	if !w.expansionClear(rec, 2, true, none, none) {
		t.Fatal("refused to share a wall standing where the new side wall goes")
	}
	if !w.designateExpansion(rec, 2, true) {
		t.Fatal("the expansion was not designated")
	}
	for _, tk := range w.projects[0].tasks {
		if tk.pos.X == 25 {
			t.Fatalf("a %v task at %v on the shared wall", tk.terrain, tk.pos)
		}
	}
}

// An expansion does not take in another room's floor: a finished room's
// aisle is ordinary discovered floor, and only the record of it tells.
func TestExpansionStaysOutOfOtherRooms(t *testing.T) {
	o := Point{18, 10}
	w, rec := builtDorm(t, o, 2)
	// A finished neighbor whose left wall is x 23: floor at x 22 is its lane,
	// and an expansion by one would put its new wall there, against it.
	// Further out, a record covering x 23..27 makes x 24.. off limits too.
	w.roomRecords = append(w.roomRecords, &roomRecord{recipe: dormRoom, f: roomFrame{o: Point{24, 10}, width: 3}, n: 2})
	carve(w, Point{22, 9}, Point{27, 13}, Floor)
	w.refreshSpatial()
	none := map[Point]bool{}
	if w.expansionClear(rec, 2, true, none, none) {
		t.Fatal("an expansion took in another room's footprint")
	}
}

// The planner grows a dormitory the colony already has, for an ordered one,
// before it looks for a new site, and only when room-expansion is on.
func TestPlannerGrowsADormitoryBeforeBuildingAnother(t *testing.T) {
	for _, on := range []bool{true, false} {
		w, rec := builtDorm(t, Point{18, 10}, 2)
		w.cfg.RoomExpansion = on
		w.manualDormitories = 1
		w.planRooms()
		grew := len(w.projects) == 1 && w.projects[0].room == rec
		if grew != on {
			t.Fatalf("room-expansion %v: grew the dormitory = %v", on, grew)
		}
		if on && w.manualDormitories != 0 {
			t.Fatal("the order was not used up by the expansion")
		}
	}
}

// Colonists carry an expansion through: they tear the wall down from inside,
// dig, wall and fit the new end, and the room is whole again, every builder
// paid out of the escrow the treasury put up. They are kept fed (stepFed):
// this world has no life support, and a hungry colonist's emergency pod is
// not what this is about.
func TestColonistsBuildAnExpansion(t *testing.T) {
	o := Point{18, 10}
	w, rec := builtDorm(t, o, 2)
	if !w.expandRoom(dormRoom, 2) {
		t.Fatal("the dormitory did not grow")
	}
	for i := 0; i < 3; i++ {
		w.spawn(Colonist, Point{16 + i, 14})
	}
	stepFed(t, w, 4000, func() bool { return len(w.projects) == 0 })
	if len(w.projects) > 0 {
		t.Fatal("the expansion never finished")
	}
	if !roomSealed(w, rec) || w.countTerrain(Bed) != 4 {
		t.Fatalf("finished expansion: sealed=%v bunks=%d, want sealed with 4", roomSealed(w, rec), w.countTerrain(Bed))
	}
	if n := len(w.sortedWork(func(o *WorkOrder) bool { return o.Issuer == Community })); n != 0 {
		t.Fatalf("%d of the expansion's work orders are still open", n)
	}
}

func equalPoints(a, b []Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
