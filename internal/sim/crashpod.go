package sim

import "fmt"

// ---- Crash pods ------------------------------------------------------------
//
// Every colonist arrives in a crash pod: a small prefab stamped into the world
// where it lands, holding the colonist's own bunk, toilet, and locker, all
// private to it, plus a manifest of meals, a purse, and a gun. Worldgen, the
// spawn command, and the director's arrival occurrence all go through arrive,
// so what a pod carries is decided in one place. See docs/crash-pods.md.
//
// The footprint is two rows, podWidth wide:
//
//	B . T . L     B bunk, T toilet, L locker (a storage container)
//	. . @ . .     @ where the colonist steps out
//
// Fixtures sit one tile apart, as in any room (see construction.md), so each
// keeps free access tiles, and every tile between and in front of them is
// floor. That shape is what makes stamping one safe anywhere: a fixture is
// never orthogonally next to another, so whatever path crossed the footprint
// before it landed can still get around each fixture through the footprint's
// own floor.

const (
	podWidth  = 5
	podHeight = 2
	// podCrashSlack is how many rings beyond the first crash-through site the
	// search keeps looking for a clear one. An open site a little farther out
	// beats smashing a hole in the rock right by the colony, but not by much:
	// past this, the pod just comes down through the rock.
	podCrashSlack = 8
)

// podFixture is one fixture in the pod layout, as an offset from its top-left.
type podFixture struct {
	dx, dy  int
	terrain Terrain
}

var podFixtures = [...]podFixture{
	{0, 0, Bed},
	{2, 0, Toilet},
	{4, 0, Storage},
}

// podDoor is where the colonist stands when it steps out.
var podDoor = Point{2, 1}

// arrive brings one colonist into the world in a crash pod and returns it, or
// nil if no site could be found anywhere (a map with no room left at all).
// Every way a colonist enters the game comes through here. announce logs the
// landing; worldgen passes false so the opening log is not one line per
// settler.
func (w *World) arrive(announce bool) *Entity {
	o, crashed, ok := w.findPodSite()
	if !ok {
		return nil
	}
	for dy := 0; dy < podHeight; dy++ {
		for dx := 0; dx < podWidth; dx++ {
			if p := o.Add(dx, dy); w.TerrainAt(p) == Rock {
				w.SetTerrain(p, Floor) // the impact clears it; the rock is simply gone
			}
		}
	}
	for _, f := range podFixtures {
		w.SetTerrain(o.Add(f.dx, f.dy), f.terrain)
	}

	e := w.spawn(Colonist, o.Add(podDoor.X, podDoor.Y))
	me := ColonistOwner(e.ID)
	for _, f := range podFixtures {
		w.setFixtureOwner(o.Add(f.dx, f.dy), me, AccessPrivate)
	}
	locker := w.storageContainers[o.Add(podFixtures[2].dx, podFixtures[2].dy)]
	if n := w.cfg.CrashPodMeals; n > 0 && locker.Inventory.Add(Meal, n) {
		locker.credit(me, Meal, n)
	}
	for i := 0; i < w.cfg.CrashPodShotguns; i++ {
		e.Inventory.Add(Shotgun, 1)
	}
	for i := 0; i < w.cfg.CrashPodPistols; i++ {
		e.Inventory.Add(Pistol, 1)
	}
	e.podOrigin, e.hasPod = o, true

	if announce {
		how := "lands"
		if crashed {
			how = "smashes down through the rock"
		}
		w.log.add(fmt.Sprintf("A crash pod %s at (%d, %d): %s has arrived.", how, o.X, o.Y, e.displayName()))
	}
	return e
}

// findPodSite picks where the next pod comes down: the top-left of a footprint
// nearest the map center, preferring open floor to rock. A site whose
// footprint is all floor lands cleanly; otherwise the pod crashes through,
// and any rock under the footprint is cleared. crashed reports which.
//
// The search walks square rings of origins outward from the center and takes
// the first clear site; the first crash site it passes is held as a fallback
// and taken once the search is podCrashSlack rings beyond it without finding
// open floor. Ring order and the fixed scan within each ring make the choice
// the same for a given world, and nothing here draws randomness.
//
// Pods fill the map from the middle out, so w.podRingHint remembers roughly
// where the last one landed and the next search starts a few rings inside it
// rather than re-checking the full, already-packed middle every time. That is
// what keeps a 2000-colonist worldgen from rescanning the whole cavern per
// settler. It can miss a clear site that opened up nearer the middle since,
// which only costs a slightly longer walk.
func (w *World) findPodSite() (o Point, crashed, ok bool) {
	// Pods land in the lower half of the map only. Rooms are sited against
	// rock *above* them (roomSiteClear wants rock behind the back wall and
	// clear rows in front), so the upper half of the landing cavern is where
	// the colony's first rooms go. Pods that spread up into it — first as a
	// square block from the middle, then as a band that crept upward once the
	// oval's narrow bottom rows were used — left small colonies with nowhere
	// to site even one room. Below the middle, once the open floor runs out,
	// they crash into the rock instead.
	//
	// The fixture row starts one below the middle row, so the middle row
	// itself stays walkable margin: in the smallest cavern (see minCaveRy) it
	// is exactly the approach row of a room against the top rim.
	center := Point{w.Width/2 - podWidth/2, w.Height/2 + 1}
	maxR := max(w.Width, w.Height)
	designated := make(map[Point]bool)
	for _, pr := range w.projects {
		for _, t := range pr.tasks {
			designated[t.pos] = true
		}
	}
	crashAt, crashRing, crashRock := Point{}, -1, 0
	for r := max(0, w.podRingHint-podWidth); r <= maxR; r++ {
		if crashRing >= 0 && r > crashRing+podCrashSlack {
			break
		}
		found, clear := false, false
		forEachRingPoint(center, r, func(p Point) bool {
			if p.Y < center.Y {
				return false // the upper half is for rooms
			}
			rock, marginRock, valid := w.podSiteRock(p, designated)
			if !valid {
				return false
			}
			if rock == 0 && marginRock == 0 {
				o, found, clear = p, true, true
				return true
			}
			if crashRing < 0 {
				crashAt, crashRing, crashRock = p, r, rock
			}
			return false
		})
		if found && clear {
			w.podRingHint = r
			return o, false, true
		}
	}
	if crashRing >= 0 {
		w.podRingHint = crashRing
		return crashAt, crashRock > 0, true
	}
	return Point{}, false, false
}

// forEachRingPoint visits the points on ring r around c, stopping early when
// visit returns true. A ring is stretched two-to-one across, like the landing
// cavern (see caveRadii): it holds the points whose max(ceil(|dx|/2), |dy|) is
// r. Within a ring, rows nearest c's come first, the row below before the row
// above.
//
// Both choices keep pods in the cavern's middle rows. Rooms are sited against
// the rock at a cavern's rim with several clear rows in front (see
// roomSiteClear); plain square rings from the center grew a block of pods
// that reached the top and bottom rims of a wide, short cavern and left the
// colony nowhere to build its first room.
func forEachRingPoint(c Point, r int, visit func(Point) bool) {
	if r == 0 {
		visit(c)
		return
	}
	for i := 0; i <= 2*r; i++ {
		dy := (i + 1) / 2 // 0, 1, 1, 2, 2, ...
		if i%2 == 0 {
			dy = -dy // below before above: 0, +1, -1, +2, -2, ...
		}
		y := c.Y + dy
		if dy == r || dy == -r {
			for x := c.X - 2*r; x <= c.X+2*r; x++ {
				if visit(Point{x, y}) {
					return
				}
			}
			continue
		}
		for _, x := range [4]int{c.X - 2*r, c.X - 2*r + 1, c.X + 2*r - 1, c.X + 2*r} {
			if visit(Point{x, y}) {
				return
			}
		}
	}
}

// podSiteRock reports whether a pod may land with its top-left at o, how many
// of its footprint tiles are still rock, and how many of its margin tiles are.
//
// A clean landing needs both to be zero: open floor all round, out in the
// middle of a cavern. That is deliberate — a pod that settled against the
// cavern wall would take exactly the rock-backed edge the colony's rooms are
// sited on (see findRoomSite), and the first pods to land used to fill those
// edges and leave the colony nowhere to build.
//
// The footprint must be rock or bare floor with nobody standing on it and no
// construction designated there, and must not cover a room's reserved door
// approach. The one-tile margin around it must hold no wall and no fixture, so
// pods never land inside a built room or wedge against someone else's bunk.
// A pod that is not all floor must touch existing floor in its margin: a pod
// crashing into solid rock with no way out would seal its colonist in.
//
// Only floor the colony has discovered counts. The floor of a natural cavern
// nobody has broken into (see caverns.md) is out of bounds for the footprint:
// a pod landing "cleanly" there would open the cavern around a colonist with
// no way back to the colony. Nor does it count as a way out in the margin.
func (w *World) podSiteRock(o Point, designated map[Point]bool) (rock, marginRock int, ok bool) {
	if o.X < 1 || o.Y < 1 || o.X+podWidth >= w.Width || o.Y+podHeight >= w.Height {
		return 0, 0, false // the margin must be on the map too
	}
	for dy := 0; dy < podHeight; dy++ {
		for dx := 0; dx < podWidth; dx++ {
			p := o.Add(dx, dy)
			switch w.TerrainAt(p) {
			case Rock:
				rock++
			case Floor:
				if !w.discovered(p) || w.occupied(p) {
					return 0, 0, false
				}
			default:
				return 0, 0, false
			}
			if w.doorTiles[p] || designated[p] {
				return 0, 0, false
			}
		}
	}
	touchesFloor := false
	for dy := -1; dy <= podHeight; dy++ {
		for dx := -1; dx <= podWidth; dx++ {
			if dx >= 0 && dx < podWidth && dy >= 0 && dy < podHeight {
				continue
			}
			t := w.TerrainAt(o.Add(dx, dy))
			if t == Wall || isFixtureTerrain(t) {
				return 0, 0, false
			}
			touchesFloor = touchesFloor || (t == Floor && w.discovered(o.Add(dx, dy)))
			if t == Rock {
				marginRock++
			}
		}
	}
	// A footprint that is partly floor already opens onto whatever that floor
	// does.
	if rock > 0 && !touchesFloor && rock == podWidth*podHeight {
		return 0, 0, false
	}
	return rock, marginRock, true
}
