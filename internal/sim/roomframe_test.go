package sim

import (
	"slices"
	"testing"
)

// rockSiteWorld is testConfig's map with nobody on it, filled with solid rock,
// so the only room sites are the ones a test carves.
func rockSiteWorld(t *testing.T) *World {
	t.Helper()
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	carve(w, Point{0, 0}, Point{w.Width - 1, w.Height - 1}, Rock)
	return w
}

// raise builds every task of p at once, phase by phase, as if its builders
// had finished.
func raise(w *World, p *project) {
	tasks := slices.Clone(p.tasks)
	slices.SortStableFunc(tasks, func(a, b *buildTask) int { return a.phase - b.phase })
	for _, t := range tasks {
		w.SetTerrain(t.pos, t.terrain)
	}
	w.refreshSpatial()
}

// Whichever way it faces, a frame keeps the same shell: the doorway one step
// in front of the anchor's column, out past the clear rows, and the back wall
// one step behind the facility row.
func TestRoomFrameTurnsTheLayout(t *testing.T) {
	anchor := Point{20, 12}
	for _, tc := range []struct {
		face       roomFacing
		toward     Point // one step from the back of the room toward its door
		doorOffset int   // doorStep - anchor, along toward
	}{
		{faceSouth, Point{0, 1}, roomFrontV + roomApproach},
		{faceNorth, Point{0, -1}, roomFrontV + roomApproach},
		{faceEast, Point{1, 0}, roomFrontV + roomApproach},
		{faceWest, Point{-1, 0}, roomFrontV + roomApproach},
	} {
		f := frameAt(anchor, tc.face, 3)
		if f.anchor() != anchor {
			t.Errorf("%v: anchor %v, want %v", tc.face, f.anchor(), anchor)
		}
		want := anchor.Add(tc.toward.X*tc.doorOffset, tc.toward.Y*tc.doorOffset)
		if got := f.doorStep(); got != want {
			t.Errorf("%v: door step %v, want %v", tc.face, got, want)
		}
		if got, want := f.at(f.doorU(), roomBackV), anchor.Add(-tc.toward.X, -tc.toward.Y); got != want {
			t.Errorf("%v: back wall behind the anchor at %v, want %v", tc.face, got, want)
		}
	}
}

// A niche whose rock is to the west gets a room with its back to that rock
// and its doorway facing east. Rooms used to face south only, and this niche
// had no site in it at all.
func TestRoomSiteFacesAwayFromItsRock(t *testing.T) {
	w := rockSiteWorld(t)
	// An east-facing room, three wide, anchored at the map center: interior
	// from the back wall (x = o.X-1) to the front wall (x = o.X+3), side walls
	// and lanes above and below, the approach column at x = o.X+4, and rock
	// behind at x = o.X-2.
	center := Point{w.Width / 2, w.Height / 2}
	o := Point{center.X, center.Y - 1}
	carve(w, Point{o.X - 1, o.Y - 2}, Point{o.X + 4, o.Y + 4}, Floor)
	w.refreshSpatial()

	site, ok := w.findRoomSite(3)
	if !ok {
		t.Fatal("no site in a niche backed by rock to the west")
	}
	if want := (roomFrame{o: o, face: faceEast, width: 3}); site != want {
		t.Fatalf("site = %+v, want %+v", site, want)
	}

	if !w.designateRoom(dormRoom, site, 2, Community) {
		t.Fatal("the room was not designated")
	}
	door := Point{o.X + 3, o.Y + 1}
	for _, tk := range w.projects[0].tasks {
		if tk.pos == door {
			t.Fatalf("a %v task on the doorway %v", tk.terrain, door)
		}
	}
	if step := (Point{o.X + 4, o.Y + 1}); !w.doorTiles[step] {
		t.Fatalf("door step %v not reserved", step)
	}
	raise(w, w.projects[0])
	if !w.sameRoom(Point{o.X + 1, o.Y + 1}, Point{o.X + 4, o.Y + 1}) {
		t.Fatal("the finished room's inside is not reachable through its east doorway")
	}
	for y := o.Y; y <= o.Y+2; y++ {
		if w.TerrainAt(Point{o.X - 1, y}) != Wall {
			t.Fatalf("no back wall at %v", Point{o.X - 1, y})
		}
	}
}

// A room that would fill the only gap between two parts of the colony is
// turned down, even though everything local about the site is fine; once
// there is another way round, it is accepted.
func TestRoomSiteRefusesToSplitTheColony(t *testing.T) {
	w := rockSiteWorld(t)
	f := roomFrame{o: Point{19, 11}, width: 3}
	lo, hi := f.box(0, roomBackV, f.width-1, roomFrontV)
	// Two open areas, north and south, and between them a gap exactly as
	// wide as the room, walled on both sides as if by two older rooms.
	carve(w, Point{14, lo.Y - 3}, Point{26, lo.Y - 1}, Floor)
	carve(w, Point{14, hi.Y + 1}, Point{26, hi.Y + 3}, Floor)
	carve(w, lo, hi, Floor)
	carve(w, Point{lo.X - 1, lo.Y}, Point{lo.X - 1, hi.Y}, Wall)
	carve(w, Point{hi.X + 1, lo.Y}, Point{hi.X + 1, hi.Y}, Wall)
	w.refreshSpatial()
	if !w.sameRoom(Point{20, lo.Y - 2}, Point{20, hi.Y + 2}) {
		t.Fatal("test setup: north and south are not connected through the gap")
	}

	designated := map[Point]bool{}
	if !w.roomSiteClear(f, designated, siteRules{unbacked: true}) {
		t.Fatal("test setup: the gap is not a locally clear site")
	}
	if w.siteKeepsColonyWhole(f, designated) {
		t.Fatal("a room filling the only way between north and south was judged to keep the colony whole")
	}
	if site, ok := w.findFreeStandingSite(3); ok {
		t.Fatalf("a site was found (%+v) where the only one splits the colony", site)
	}

	// A corridor round the west side gives the colony another way through.
	carve(w, Point{14, lo.Y - 3}, Point{14, hi.Y + 3}, Floor)
	w.refreshSpatial()
	if !w.siteKeepsColonyWhole(f, designated) {
		t.Fatal("the room was still refused with a corridor round it")
	}
}

// With nothing left to back onto — the map mined out to open floor — every
// kind of room still gets a site, standing free, and once built it leaves the
// floor in one piece. The meeting hall hit this first: TestColonyBuildsMeetingHall,
// at one food drive rate, mined its 40x24 map bare by tick 3000 and never
// built a chair.
func TestRoomsStandFreeWhenNothingIsLeftToBackOnto(t *testing.T) {
	for _, r := range []roomRecipe{dormRoom, lifeSupportRoom, hallRoom, scumhouseRoom, storageRoom} {
		w := rockSiteWorld(t)
		carve(w, Point{0, 0}, Point{w.Width - 1, w.Height - 1}, Floor)
		w.refreshSpatial()

		if _, ok := w.findRoomSite(r.roomWidth(r.minFac)); ok {
			t.Fatalf("%s: found a backed site on a map with no rock or wall", r.name)
		}
		if !w.planRoomFor(r, Community) {
			t.Fatalf("%s: not planned on an open, mined-out map", r.name)
		}
		raise(w, w.projects[0])
		if n := len(w.discoveredRooms); n != 1 {
			t.Fatalf("%s: the finished room left the floor in %d pieces", r.name, n)
		}
	}
}
