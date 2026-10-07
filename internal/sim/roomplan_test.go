package sim

import "testing"

// builtRoom is rockSiteWorld with a finished, south-facing room of r with n
// fixtures in a rock niche at o, its lanes and approach row carved out, and
// its project pruned, as a colony would have left it.
func builtRoom(t *testing.T, r roomRecipe, o Point, n int) (*World, *roomRecord) {
	t.Helper()
	w := rockSiteWorld(t)
	width := r.roomWidth(n)
	carve(w, Point{o.X - 2, o.Y + roomBackV, o.Level}, Point{o.X + width + 1, o.Y + roomFrontV + roomApproach, o.Level}, Floor)
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

// alsoBuilt adds a finished room of r with n fixtures in frame f to w,
// designated where it stands (sharing any wall already there), raised, and
// its project pruned.
func alsoBuilt(t *testing.T, w *World, r roomRecipe, f roomFrame, n int) *roomRecord {
	t.Helper()
	if !w.designateRoom(r, f, n, Community) {
		t.Fatalf("test setup: the %s at %v was not designated", r.name, f.o)
	}
	raise(w, w.projects[len(w.projects)-1])
	w.pruneProjects()
	return w.roomRecords[len(w.roomRecords)-1]
}

// shapeTasks sorts a project's tasks by what they do.
func shapeTasks(p *project) (demolish, walls []Point, fixtures map[Point]Terrain) {
	fixtures = map[Point]Terrain{}
	for _, tk := range p.tasks {
		switch {
		case tk.clears == Wall:
			demolish = append(demolish, tk.pos)
		case tk.terrain == Wall:
			walls = append(walls, tk.pos)
		case FixtureZone(tk.terrain) != NoZone:
			fixtures[tk.pos] = tk.terrain
		}
	}
	return demolish, walls, fixtures
}

// layoutOf is a bare layout for the rule's own tests: a room inside lo..hi
// with doorways doors and fixtures occ.
func layoutOf(lo, hi Point, doors []Point, occ map[Point]Terrain) *roomLayout {
	l := &roomLayout{lo: lo, hi: hi, w: hi.X - lo.X + 1, h: hi.Y - lo.Y + 1, occ: occ}
	for _, d := range doors {
		l.inner = append(l.inner, doorInside(lo, hi, d))
	}
	l.order(nil)
	return l
}

// The layout rule: fixtures a step apart at least, the floor inside a
// doorway kept clear, every doorway still reaching every other, and a
// workshop or chest keeping two tiles to be worked from.
func TestLayoutRule(t *testing.T) {
	// A 5x3 room, doorway in the middle of the front (south) wall.
	lo, hi := Point{0, 0, LandingLevel}, Point{4, 2, LandingLevel}
	door := Point{2, 3, LandingLevel}
	l := layoutOf(lo, hi, []Point{door}, map[Point]Terrain{{1, 0, LandingLevel}: Bed})
	for _, tc := range []struct {
		p    Point
		kind Terrain
		ok   bool
		why  string
	}{
		{Point{3, 0, LandingLevel}, Bed, true, "two along the back wall from a bunk"},
		{Point{2, 0, LandingLevel}, Bed, false, "beside a bunk"},
		{Point{2, 1, LandingLevel}, Bed, false, "diagonal to a bunk"},
		{Point{2, 2, LandingLevel}, Bed, false, "the tile inside the doorway"},
		{Point{4, 2, LandingLevel}, Scumhouse, true, "a stove in a front corner, worked from two tiles"},
		{Point{5, 0, LandingLevel}, Bed, false, "outside the room"},
	} {
		if got := l.fits(tc.p, tc.kind); got != tc.ok {
			t.Errorf("%s (%v): fits = %v, want %v", tc.why, tc.p, got, tc.ok)
		}
	}

	// A narrow room, one tile wide: a bunk at the back is reached from the
	// one tile in front of it, a stove is not enough.
	narrow := layoutOf(Point{0, 0, LandingLevel}, Point{0, 2, LandingLevel}, []Point{{0, 3, LandingLevel}}, map[Point]Terrain{})
	if !narrow.fits(Point{0, 0, LandingLevel}, Bed) {
		t.Error("a narrow room refused a bunk at its back")
	}
	if narrow.fits(Point{0, 0, LandingLevel}, Scumhouse) {
		t.Error("a narrow room took a stove with one tile to work it from")
	}

	// A corridor with a doorway at each end: a fixture in the middle would
	// cut one doorway off from the other.
	hall := layoutOf(Point{0, 0, LandingLevel}, Point{4, 0, LandingLevel}, []Point{{-1, 0, LandingLevel}, {5, 0, LandingLevel}}, map[Point]Terrain{})
	if hall.fits(Point{2, 0, LandingLevel}, Bed) {
		t.Error("a bunk cut a corridor in two")
	}
}

// Fixtures of one zone share a room: an incubator goes into a kitchen with
// floor to spare, and the kitchen is a production room from then on. A bunk
// never goes there.
func TestAnIncubatorFitsIntoAKitchen(t *testing.T) {
	w, rec := builtRoom(t, scumhouseRoom, Point{14, 10, LandingLevel}, 2) // inside x 14..18, y 10..12
	if w.improveRooms(units(1, Bed)) {
		t.Fatal("a bunk went into a kitchen")
	}
	if !w.improveRooms(units(1, Incubator)) {
		t.Fatal("the incubator did not go into the kitchen")
	}
	p := w.projects[0]
	if p.room != rec || p.name != "scumhouse fit-out" {
		t.Fatalf("project %q, want a scumhouse fit-out of the kitchen", p.name)
	}
	_, walls, fixtures := shapeTasks(p)
	if len(walls) != 0 || len(fixtures) != 1 {
		t.Fatalf("fit-out raises %d walls and %d fixtures, want only the incubator", len(walls), len(fixtures))
	}
	for at, k := range fixtures {
		if k != Incubator || !inBox(rec.lo, rec.hi, at) {
			t.Fatalf("a %v at %v, want an incubator inside the kitchen", k, at)
		}
	}
	if got := w.structureName(rec.structure); got != "production room" {
		t.Fatalf("the kitchen with an incubator coming is called %q, want production room", got)
	}
}

// Two dormitories back to back across one wall join: the wall comes down,
// both doorways stay, and the room is both deep, its bunks in two rows.
func TestRoomsBackToBackJoin(t *testing.T) {
	w, a := builtRoom(t, dormRoom, Point{14, 10, LandingLevel}, 2) // inside y 10..12, back wall y 9
	carve(w, Point{12, 4, LandingLevel}, Point{20, 8, LandingLevel}, Floor)
	w.refreshSpatial()
	// Facing north, its back wall a's: inside y 6..8, doorway at (15, 5).
	b := alsoBuilt(t, w, dormRoom, roomFrame{o: Point{14, 8, LandingLevel}, face: faceNorth, width: 3}, 2)
	w.refreshSpatial()

	if !w.tidyRooms() {
		t.Fatal("the two dormitories were not joined")
	}
	p := w.projects[0]
	demolish, walls, _ := shapeTasks(p)
	if want := []Point{{14, 9, LandingLevel}, {15, 9, LandingLevel}, {16, 9, LandingLevel}}; !equalPoints(demolish, want) {
		t.Fatalf("tears down %v, want the shared wall %v", demolish, want)
	}
	if len(walls) != 0 {
		t.Fatalf("raises walls at %v: the joined room needs none", walls)
	}
	if len(w.roomRecords) != 1 || w.roomRecords[0] != a {
		t.Fatal("the older dormitory did not live on as the joined room")
	}
	if a.lo != (Point{14, 6, LandingLevel}) || a.hi != (Point{16, 12, LandingLevel}) || len(a.doors) != 2 {
		t.Fatalf("joined room %v..%v with doorways %v, want (14,6)..(16,12) with both", a.lo, a.hi, a.doors)
	}
	if b.structure != nil && w.structures[b.structure.id] != nil {
		t.Fatal("the newer dormitory's structure is still registered")
	}
	for _, step := range []Point{{15, 14, LandingLevel}, {15, 4, LandingLevel}} {
		if !w.doorTiles[step] {
			t.Fatalf("door step %v is no longer reserved", step)
		}
	}
	if w.roomFloor[Point{15, 9, LandingLevel}] != a {
		t.Fatal("where the wall stood is not indexed as the room's floor")
	}
}

// Rooms side by side facing opposite ways join too: the wall between comes
// down and each keeps its doorway, one in each long wall.
func TestRoomsFacingOppositeWaysJoin(t *testing.T) {
	w, a := builtRoom(t, dormRoom, Point{14, 10, LandingLevel}, 2) // inside x 14..16, y 10..12
	carve(w, Point{18, 8, LandingLevel}, Point{22, 14, LandingLevel}, Floor)
	w.refreshSpatial()
	// Facing north beside it: inside x 18..20, y 10..12, doorway at (19, 9).
	alsoBuilt(t, w, dormRoom, roomFrame{o: Point{18, 12, LandingLevel}, face: faceNorth, width: 3}, 2)
	w.refreshSpatial()
	if !w.tidyRooms() {
		t.Fatal("the two dormitories were not joined")
	}
	demolish, _, _ := shapeTasks(w.projects[0])
	if want := []Point{{17, 10, LandingLevel}, {17, 11, LandingLevel}, {17, 12, LandingLevel}}; !equalPoints(demolish, want) {
		t.Fatalf("tears down %v, want the wall between %v", demolish, want)
	}
	if a.lo != (Point{14, 10, LandingLevel}) || a.hi != (Point{20, 12, LandingLevel}) {
		t.Fatalf("joined room %v..%v, want (14,10)..(20,12)", a.lo, a.hi)
	}
	if !equalPoints(a.doors, []Point{{15, 13, LandingLevel}, {19, 9, LandingLevel}}) {
		t.Fatalf("doorways %v, want (15,13) and (19,9)", a.doors)
	}
}

// A room hemmed in on three sides grows through the wall its doorway is in,
// and the doorway moves out with it: the old step is let go, the new one
// reserved.
func TestGrowingThroughADoorwayMovesIt(t *testing.T) {
	w, rec := builtRoom(t, dormRoom, Point{14, 10, LandingLevel}, 2) // inside x 14..16, y 10..12, door (15, 13)
	carve(w, Point{12, 14, LandingLevel}, Point{18, 22, LandingLevel}, Floor)
	carve(w, Point{12, 8, LandingLevel}, Point{12, 13, LandingLevel}, Hull)
	carve(w, Point{18, 8, LandingLevel}, Point{18, 13, LandingLevel}, Hull)
	carve(w, Point{12, 8, LandingLevel}, Point{18, 8, LandingLevel}, Hull)
	w.refreshSpatial()

	st := w.roomPlanState()
	if !w.growRoom([]*roomRecord{rec}, units(3, Bed), st) {
		t.Fatal("the dormitory did not grow")
	}
	if rec.hi.Y <= 12 || rec.lo != (Point{14, 10, LandingLevel}) || rec.hi.X != 16 {
		t.Fatalf("grew to %v..%v, want deeper to the south only", rec.lo, rec.hi)
	}
	if len(rec.doors) != 1 || rec.doors[0] != (Point{15, rec.hi.Y + 1, rec.hi.Level}) {
		t.Fatalf("doorway %v, want it moved to (15, %d)", rec.doors, rec.hi.Y+1)
	}
	if w.doorTiles[Point{15, 14, LandingLevel}] || !w.doorTiles[Point{15, rec.hi.Y + 2, rec.hi.Level}] {
		t.Fatal("the door step reservation did not move with the doorway")
	}
	demolish, walls, _ := shapeTasks(w.projects[0])
	for _, q := range demolish {
		if q.Y != 13 {
			t.Errorf("tears down %v: only the old front wall comes down", q)
		}
	}
	for _, q := range walls {
		if q == rec.doors[0] {
			t.Fatal("a wall goes up on the new doorway")
		}
	}
}

// A kitchen grows by a stove and its pantry together, two tiles apart and
// linked as a new kitchen's are.
func TestAKitchenGrowsByAStoveAndItsPantry(t *testing.T) {
	w, rec := builtRoom(t, scumhouseRoom, Point{14, 10, LandingLevel}, 2)
	st := w.roomPlanState()
	if !w.growRoom([]*roomRecord{rec}, recipeUnits(scumhouseRoom, 2), st) {
		t.Fatal("the kitchen did not grow")
	}
	_, _, fixtures := shapeTasks(w.projects[0])
	var stove, pantry Point
	for at, k := range fixtures {
		switch k {
		case Scumhouse:
			stove = at
		case Storage:
			pantry = at
		}
	}
	if len(fixtures) != 2 || stove.Manhattan(pantry) != 2 || (stove.X != pantry.X && stove.Y != pantry.Y) {
		t.Fatalf("fixtures %v, want a stove and a pantry two tiles apart in a line", fixtures)
	}
	if w.pantryOf[stove] != pantry || w.fixtureZone(pantry, Storage) != ZoneProduction {
		t.Fatal("the new stove is not linked to its pantry")
	}
}

// A room never grows into another room's floor, nor raises a wall against
// one standing just past its new wall.
func TestGrowingStaysOutOfOtherRooms(t *testing.T) {
	w, rec := builtRoom(t, dormRoom, Point{14, 10, LandingLevel}, 2)
	other := &roomRecord{lo: Point{20, 10, LandingLevel}, hi: Point{22, 12, LandingLevel}, zone: ZoneStorage}
	w.roomRecords = append(w.roomRecords, other)
	w.indexRoom(other)
	carve(w, Point{17, 9, LandingLevel}, Point{23, 13, LandingLevel}, Floor)
	w.refreshSpatial()
	st := w.roomPlanState()
	for k := 1; k <= 6; k++ {
		sh := roomShape{recs: []*roomRecord{rec}, lo: rec.lo, hi: Point{rec.hi.X + k, rec.hi.Y, rec.hi.Level}}
		_, _, ok := w.shapeWork(sh, st)
		// k = 1 and 2 stop short of the neighbour (k = 2 puts the new wall at
		// x 19, one short of its floor; nothing stands there, so it is fine).
		if want := k <= 2; ok != want {
			t.Errorf("growing east by %d: ok = %v, want %v", k, ok, want)
		}
	}
	carve(w, Point{20, 9, LandingLevel}, Point{20, 13, LandingLevel}, Wall)
	w.refreshSpatial()
	sh := roomShape{recs: []*roomRecord{rec}, lo: rec.lo, hi: Point{rec.hi.X + 2, rec.hi.Y, rec.hi.Level}}
	if _, _, ok := w.shapeWork(sh, w.roomPlanState()); ok {
		t.Error("raised a new wall at x 19 against the wall at x 20")
	}
}

// The planner puts an ordered dormitory's bunks into the dormitory it has
// before it marks out another, and waits for a room going up rather than
// planning a second beside it.
func TestThePlannerUsesTheRoomsItHas(t *testing.T) {
	w, rec := builtRoom(t, dormRoom, Point{14, 10, LandingLevel}, 2)
	w.manualDormitories = 1
	w.planRooms()
	if len(w.projects) != 1 || w.projects[0].room != rec {
		t.Fatalf("projects %v, want the order to go into the dormitory", projectNames(w.projects))
	}
	if w.manualDormitories != 0 {
		t.Fatal("the order was not used up")
	}
	if !w.roomGoingUp(ZoneResidence) {
		t.Fatal("a dormitory being fitted out is not going up")
	}
	before := len(w.roomRecords)
	if w.placeFixtures(dormRoom, units(4, Bed), true) || len(w.roomRecords) != before {
		t.Fatal("marked out another dormitory while one was going up")
	}
}

// Colonists carry a merger through: the shared wall comes down and the
// joined room stands whole with its two doorways and all its bunks.
func TestColonistsBuildAMerger(t *testing.T) {
	w, a := builtRoom(t, dormRoom, Point{14, 10, LandingLevel}, 2)
	carve(w, Point{12, 4, LandingLevel}, Point{20, 8, LandingLevel}, Floor)
	w.refreshSpatial()
	alsoBuilt(t, w, dormRoom, roomFrame{o: Point{14, 8, LandingLevel}, face: faceNorth, width: 3}, 2)
	w.refreshSpatial()
	if !w.tidyRooms() {
		t.Fatal("the two dormitories were not joined")
	}
	for i := 0; i < 3; i++ {
		w.spawn(Colonist, Point{16 + i, 14, LandingLevel})
	}
	stepFed(t, w, 4000, func() bool { return len(w.projects) == 0 })
	if len(w.projects) > 0 {
		t.Fatal("the merger never finished")
	}
	for x := a.lo.X; x <= a.hi.X; x++ {
		if w.TerrainAt(Point{x, 9, LandingLevel}) == Wall {
			t.Fatalf("the shared wall still stands at (%d, 9)", x)
		}
	}
	for _, d := range a.doors {
		if w.TerrainAt(d) != Floor {
			t.Fatalf("doorway %v is %v", d, w.TerrainAt(d))
		}
	}
	if w.countTerrain(Bed) != 4 {
		t.Fatalf("%d bunks, want the 4 the two rooms had", w.countTerrain(Bed))
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
