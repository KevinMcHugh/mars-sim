package sim

import "testing"

// besideBuilt adds a finished, south-facing room of r with n fixtures at o to
// w, as builtRoom does for the first: designated where it stands (sharing any
// wall already there), raised, and its project pruned.
func besideBuilt(t *testing.T, w *World, r roomRecipe, o Point, n int) *roomRecord {
	t.Helper()
	if !w.designateRoom(r, roomFrame{o: o, width: r.roomWidth(n)}, n, Community) {
		t.Fatalf("test setup: the second %s was not designated", r.name)
	}
	raise(w, w.projects[len(w.projects)-1])
	w.pruneProjects()
	return w.roomRecords[len(w.roomRecords)-1]
}

// Two storage rooms sharing a side wall become one: the wall between them
// comes down, a container goes up where it stood, and the colony is left
// with one record, one structure, and both doorways.
func TestTwoStorageRoomsSharingAWallBecomeOne(t *testing.T) {
	w, a := builtRoom(t, storageRoom, Point{14, 10}, 1) // interior x 14..16, walls 13 and 17
	b := besideBuilt(t, w, storageRoom, Point{18, 10}, 1)
	if a.structure == nil || b.structure == nil || a.structure == b.structure {
		t.Fatal("test setup: each room should have a structure of its own")
	}
	drop := b.structure

	if !w.expandRoom(storageRoom, 1) {
		t.Fatal("the colony did not join the two storage rooms")
	}
	p := w.projects[0]
	if p.room != a || p.name != "storage room merger" {
		t.Fatalf("project %q for %p, want a storage room merger of the older room", p.name, p.room)
	}
	var demolish, fixtures []Point
	for _, tk := range p.tasks {
		switch {
		case tk.clears == Wall:
			demolish = append(demolish, tk.pos)
		case tk.terrain == Storage:
			fixtures = append(fixtures, tk.pos)
		case tk.terrain == Wall:
			t.Errorf("a wall task at %v: the joined room needs no new wall", tk.pos)
		}
	}
	if want := []Point{{17, 10}, {17, 11}, {17, 12}}; !equalPoints(demolish, want) {
		t.Fatalf("tears down %v, want the shared wall's inside rows %v", demolish, want)
	}
	if want := []Point{{17, 10}}; !equalPoints(fixtures, want) {
		t.Fatalf("containers at %v, want one where the wall stood, %v", fixtures, want)
	}
	if len(w.roomRecords) != 1 || w.roomRecords[0] != a {
		t.Fatalf("%d room records, want just the older room", len(w.roomRecords))
	}
	if a.n != 3 || a.f.width != 7 || a.f.o != (Point{14, 10}) {
		t.Fatalf("joined room n=%d width=%d at %v, want 3 fixtures, 7 wide, at (14,10)", a.n, a.f.width, a.f.o)
	}
	for x := 14; x <= 20; x++ {
		if w.roomFloor[Point{x, 11}] != a {
			t.Fatalf("(%d, 11) is not indexed as the joined room's floor", x)
		}
	}
	if w.structures[drop.id] != nil {
		t.Fatal("the dropped room's structure is still registered")
	}
	if len(a.structure.doors) != 2 {
		t.Fatalf("the joined structure holds %d doorways, want both", len(a.structure.doors))
	}
	for _, id := range w.structureAt[Point{17, 9}] {
		if id == drop.id {
			t.Fatal("the shared back wall is still indexed under the dropped structure")
		}
	}
	if ids := w.structureAt[Point{17, 9}]; len(ids) != 1 {
		t.Fatalf("the shared back wall is indexed under %v, want the joined structure once", ids)
	}
}

// Two dormitories with a lane between them join across it: both side walls
// come down, the lane gets the back and front wall it lacks, and a bunk goes
// in it. The planner does this before growing either room outward.
func TestDormitoriesJoinAcrossALane(t *testing.T) {
	w, a := builtDorm(t, Point{14, 10}, 2) // interior 14..16, walls 13 and 17
	carve(w, Point{18, 10 + roomBackV}, Point{18, 10 + roomFrontV + roomApproach}, Floor)
	b := besideBuilt(t, w, dormRoom, Point{20, 10}, 2) // walls 19 and 23
	w.refreshSpatial()

	if !w.expandRoom(dormRoom, 4) {
		t.Fatal("the colony did not join the two dormitories")
	}
	p := w.projects[0]
	var demolish, walls, beds []Point
	for _, tk := range p.tasks {
		switch {
		case tk.clears == Wall:
			demolish = append(demolish, tk.pos)
		case tk.terrain == Wall:
			walls = append(walls, tk.pos)
		case tk.terrain == Bed:
			beds = append(beds, tk.pos)
		}
	}
	if want := []Point{{17, 10}, {17, 11}, {17, 12}, {19, 10}, {19, 11}, {19, 12}}; !equalPoints(demolish, want) {
		t.Fatalf("tears down %v, want both side walls' inside rows %v", demolish, want)
	}
	if want := []Point{{18, 9}, {18, 13}}; !equalPoints(walls, want) {
		t.Fatalf("raises walls at %v, want the lane's back and front %v", walls, want)
	}
	if want := []Point{{18, 10}}; !equalPoints(beds, want) {
		t.Fatalf("bunks at %v, want one in the lane %v", beds, want)
	}
	if len(w.roomRecords) != 1 || a.n != 5 || a.f.width != 9 {
		t.Fatalf("records=%d n=%d width=%d, want one room of 5 bunks, 9 wide", len(w.roomRecords), a.n, a.f.width)
	}
	if b.structure == nil || w.structures[b.structure.id] != nil {
		t.Fatal("the newer dormitory's structure was not taken in")
	}
}

// Rooms that do not line up are left alone: facing different ways, back
// walls in different rows, a lane wider than maxMergeLane, a lane holding
// someone's doorway step, or a room a colonist owns.
func TestMergerRefusesRoomsThatDoNotLineUp(t *testing.T) {
	a := &roomRecord{recipe: dormRoom, f: roomFrame{o: Point{14, 10}, width: 3}, n: 2, issuer: Community}
	for _, tc := range []struct {
		name string
		b    roomFrame
		ok   bool
	}{
		{"flush", roomFrame{o: Point{18, 10}, width: 3}, true},
		{"flush on the left", roomFrame{o: Point{10, 10}, width: 3}, true},
		{"widest lane", roomFrame{o: Point{18 + maxMergeLane + 1, 10}, width: 3}, true},
		{"lane too wide", roomFrame{o: Point{18 + maxMergeLane + 2, 10}, width: 3}, false},
		{"another row", roomFrame{o: Point{18, 11}, width: 3}, false},
		{"facing north", roomFrame{o: Point{18, 10}, face: faceNorth, width: 3}, false},
		{"overlapping", roomFrame{o: Point{16, 10}, width: 3}, false},
	} {
		b := &roomRecord{recipe: dormRoom, f: tc.b, n: 2, issuer: Community}
		m, ok := besideRoom(a, b)
		if ok != tc.ok {
			t.Errorf("%s: besideRoom = %v, want %v", tc.name, ok, tc.ok)
			continue
		}
		if ok && m.frame().width != 3+1+m.d+3 {
			t.Errorf("%s: joined width %d", tc.name, m.frame().width)
		}
	}

	w, d := builtDorm(t, Point{14, 10}, 2)
	carve(w, Point{18, 10 + roomBackV}, Point{18, 10 + roomFrontV + roomApproach}, Floor)
	besideBuilt(t, w, dormRoom, Point{20, 10}, 2)
	w.refreshSpatial()
	w.doorTiles[Point{18, 11}] = true
	if w.mergeRooms(dormRoom, 4, 1, w.roomPlanState()) {
		t.Fatal("joined across a lane holding a reserved doorway step")
	}
	delete(w.doorTiles, Point{18, 11})
	d.issuer = ColonistOwner(1)
	if w.mergeRooms(dormRoom, 4, 1, w.roomPlanState()) {
		t.Fatal("joined a room a colonist owns")
	}
}

// Narrow rooms flush against each other fit nothing new where the wall
// stood, so the planner, wanting bunks, grows instead; with nothing else to
// build it joins them anyway (tidyRooms).
func TestTidyingJoinsRoomsTheBayCannotUse(t *testing.T) {
	w, a := builtDorm(t, Point{14, 10}, 2)
	besideBuilt(t, w, dormRoom, Point{18, 10}, 2)
	st := w.roomPlanState()
	if w.mergeRooms(dormRoom, 2, 1, st) {
		t.Fatal("a merger that adds no bunk counted toward the colony's demand")
	}
	if !w.tidyRooms() {
		t.Fatal("tidying did not join the two dormitories")
	}
	if len(w.roomRecords) != 1 || a.n != 4 {
		t.Fatalf("records=%d n=%d, want one room of the 4 bunks", len(w.roomRecords), a.n)
	}
	w.cfg.RoomMerge = false
	if w.tidyRooms() {
		t.Fatal("tidied with room-merge off")
	}
}

// A kitchen joins another across a lane by a stove and its pantry, linked as
// a new kitchen's are.
func TestKitchensJoinByAStoveAndItsPantry(t *testing.T) {
	w, a := builtRoom(t, scumhouseRoom, Point{14, 10}, 2) // interior 14..18, walls 13 and 19
	carve(w, Point{20, 10 + roomBackV}, Point{20, 10 + roomFrontV + roomApproach}, Floor)
	besideBuilt(t, w, scumhouseRoom, Point{22, 10}, 2) // walls 21 and 27
	w.refreshSpatial()
	if !w.expandRoom(scumhouseRoom, 2) {
		t.Fatal("the colony did not join the two kitchens")
	}
	var stove, pantry Point
	for _, tk := range w.projects[0].tasks {
		switch tk.terrain {
		case Scumhouse:
			stove = tk.pos
		case Storage:
			pantry = tk.pos
		}
	}
	if stove != (Point{19, 10}) || pantry != (Point{21, 10}) {
		t.Fatalf("stove %v and pantry %v, want (19,10) and (21,10), where the walls stood", stove, pantry)
	}
	if w.pantryOf[stove] != pantry {
		t.Fatal("the new stove is not linked to its pantry")
	}
	if a.n != 6 {
		t.Fatalf("joined kitchen has %d fixtures, want 6", a.n)
	}
}

// Colonists carry a merger through, and the joined room stands whole with
// its two doorways and all its bunks.
func TestColonistsBuildAMerger(t *testing.T) {
	w, a := builtDorm(t, Point{14, 10}, 2)
	carve(w, Point{18, 10 + roomBackV}, Point{18, 10 + roomFrontV + roomApproach}, Floor)
	besideBuilt(t, w, dormRoom, Point{20, 10}, 2)
	w.refreshSpatial()
	if !w.expandRoom(dormRoom, 4) {
		t.Fatal("the colony did not join the two dormitories")
	}
	for i := 0; i < 3; i++ {
		w.spawn(Colonist, Point{16 + i, 14})
	}
	stepFed(t, w, 4000, func() bool { return len(w.projects) == 0 })
	if len(w.projects) > 0 {
		t.Fatal("the merger never finished")
	}
	gaps := 0
	for v := roomBackV; v <= roomFrontV; v++ {
		for u := -1; u <= a.f.width; u++ {
			edge := v == roomBackV || v == roomFrontV || u < 0 || u == a.f.width
			if edge && w.TerrainAt(a.f.at(u, v)) != Wall {
				gaps++
			}
			if !edge && w.TerrainAt(a.f.at(u, v)) == Wall {
				t.Errorf("a wall still stands inside the joined room at %v", a.f.at(u, v))
			}
		}
	}
	if gaps != 2 || w.countTerrain(Bed) != 5 {
		t.Fatalf("joined room: %d gaps in its walls and %d bunks, want its 2 doorways and 5", gaps, w.countTerrain(Bed))
	}
}

// The planner waits for a room going up rather than marking out another of
// the same kind beside it, and later rooms with an aisle refuse the narrow
// fallback.
func TestPlannerWaitsForARoomGoingUp(t *testing.T) {
	w, a := builtRoom(t, incubatorRoom, Point{14, 10}, 2)
	// Put the incubator room back under construction, as if just marked out.
	w.projects = append(w.projects, &project{id: w.nextProjectID, name: incubatorRoom.name, issuer: Community, room: a,
		tasks: []*buildTask{{pos: Point{40, 40}, terrain: Wall}}})
	if !w.roomGoingUp(incubatorRoom) {
		t.Fatal("an incubator room under construction is not going up")
	}
	before := len(w.roomRecords)
	if w.growOrPlan(incubatorRoom, 2) || len(w.roomRecords) != before {
		t.Fatal("marked out another incubator room while one was going up")
	}
	if w.roomGoingUp(storageRoom) {
		t.Fatal("a storage room is going up in a world without one")
	}
}
