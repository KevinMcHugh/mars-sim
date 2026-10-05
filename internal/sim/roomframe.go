package sim

// ---- Room frames: which way a room faces ------------------------------------------
//
// Every room is the same shell: a row of facilities along the back wall, open
// rows in front of them, and a front wall with a doorway in the middle. Only
// which way it faces changes. A roomFrame lays that shell out in the room's
// own coordinates and turns it onto the map, so the planner can put a doorway
// on any side: a room used to face south only, and a cavern whose rock lay to
// the east or west had no site in it at all. See docs/construction.md.

// roomFacing is the side of a room its doorway is on.
type roomFacing uint8

const (
	faceSouth roomFacing = iota // doorway toward +Y, the original layout
	faceNorth                   // doorway toward -Y
	faceEast                    // doorway toward +X
	faceWest                    // doorway toward -X
)

// roomFacings is every facing, in the order the planner breaks ties between
// sites the same distance from the map center: south first, so a colony that
// could build the old way still does.
var roomFacings = [...]roomFacing{faceSouth, faceNorth, faceEast, faceWest}

func (f roomFacing) String() string {
	switch f {
	case faceNorth:
		return "north"
	case faceEast:
		return "east"
	case faceWest:
		return "west"
	default:
		return "south"
	}
}

// Rows of a room, by v (see roomFrame).
const (
	roomBackV  = -1                 // the back wall
	roomFrontV = roomFrontClear + 1 // the front wall, with the doorway
)

// roomFrame places a room's own coordinates on the map. u runs along the bay:
// 0 is the first interior column, -1 and width the side walls, -2 and
// width+1 the lanes outside them. v runs from back to front: roomBackV-1 is
// the row behind the back wall, roomBackV the back wall, 0 the facility row,
// roomFrontV the front wall, and roomFrontV+roomApproach the approach row
// outside the doorway. The zero frame faces south.
type roomFrame struct {
	o     Point // the map tile at u=0, v=0
	face  roomFacing
	width int // interior width
}

// at is the map tile at (u, v) in the frame.
func (f roomFrame) at(u, v int) Point {
	switch f.face {
	case faceNorth:
		return Point{f.o.X + u, f.o.Y - v}
	case faceEast:
		return Point{f.o.X + v, f.o.Y + u}
	case faceWest:
		return Point{f.o.X - v, f.o.Y + u}
	default:
		return Point{f.o.X + u, f.o.Y + v}
	}
}

// doorU is the doorway's column.
func (f roomFrame) doorU() int { return f.width / 2 }

// doorStep is the tile just outside the doorway, reserved in w.doorTiles.
func (f roomFrame) doorStep() Point { return f.at(f.doorU(), roomFrontV+roomApproach) }

// anchor is the middle of the facility row: where the planner measures a
// site's distance from.
func (f roomFrame) anchor() Point { return f.at(f.width/2, 0) }

// frameAt is the frame facing face, width wide, whose anchor is anchor.
func frameAt(anchor Point, face roomFacing, width int) roomFrame {
	f := roomFrame{face: face, width: width}
	switch face {
	case faceEast, faceWest:
		f.o = Point{anchor.X, anchor.Y - width/2}
	default:
		f.o = Point{anchor.X - width/2, anchor.Y}
	}
	return f
}

// box is the map rectangle covering frame cells (u0, v0) to (u1, v1),
// inclusive, as its least and greatest corners.
func (f roomFrame) box(u0, v0, u1, v1 int) (lo, hi Point) {
	a, b := f.at(u0, v0), f.at(u1, v1)
	return Point{min(a.X, b.X), min(a.Y, b.Y)}, Point{max(a.X, b.X), max(a.Y, b.Y)}
}

// splitCheckMargin is how far beyond a room's footprint siteKeepsColonyWhole
// looks for a way round it. A detour longer than this counts as none.
const splitCheckMargin = 12

// siteKeepsColonyWhole reports whether a room in frame f leaves every route
// it would cross with a way round. The footprint is the room from side wall
// to side wall and back wall to front wall: once it stands, its inside is
// reached only through the doorway, a dead end, so for getting past it the
// room is as solid as its walls. The open, discovered tiles bordering the
// footprint are where every route through it enters and leaves, so if any
// two of them in the same room now can still reach each other without it,
// every route through can go round instead. Tiles another project will build
// on (designated) count as solid too, so two plans can't close a gap between
// them that each would have left open alone.
//
// The search for a way round stays within splitCheckMargin of the footprint,
// so a site is turned down when the only way round is a long detour. That
// errs toward refusing a site, never toward splitting the colony.
//
// Free-standing rooms (findFreeStandingSite) are what make this needed: a
// backed room's lanes and approach row already form a way round it, but a
// room out on open floor could fill a corridor wall to wall.
func (w *World) siteKeepsColonyWhole(f roomFrame, designated map[Point]bool) bool {
	lo, hi := f.box(-1, roomBackV, f.width, roomFrontV)
	return w.footprintKeepsColonyWhole(lo, hi, designated)
}

// footprintKeepsColonyWhole is siteKeepsColonyWhole for any footprint, lo to
// hi inclusive, walls included: a room grown or joined (roomplan.go) is not a
// frame's shape.
func (w *World) footprintKeepsColonyWhole(lo, hi Point, designated map[Point]bool) bool {
	inFoot := func(p Point) bool { return p.X >= lo.X && p.X <= hi.X && p.Y >= lo.Y && p.Y <= hi.Y }
	blo := Point{max(0, lo.X-splitCheckMargin), max(0, lo.Y-splitCheckMargin)}
	bhi := Point{min(w.Width-1, hi.X+splitCheckMargin), min(w.Height-1, hi.Y+splitCheckMargin)}
	open := func(p Point) bool {
		return p.X >= blo.X && p.X <= bhi.X && p.Y >= blo.Y && p.Y <= bhi.Y &&
			!inFoot(p) && !designated[p] && w.Walkable(p)
	}
	// changes reports whether the room changes p: a footprint tile that is not
	// already a wall. A party wall is solid before and after, so the tiles
	// beyond it (a neighbor's inside) are no route through this room.
	changes := func(p Point) bool { return inFoot(p) && w.TerrainAt(p) != Wall }

	// The border, in row-major order, with the room each tile is in now.
	type borderTile struct {
		p    Point
		room RoomID
	}
	var border []borderTile
	for y := lo.Y - 1; y <= hi.Y+1; y++ {
		for x := lo.X - 1; x <= hi.X+1; x++ {
			p := Point{x, y}
			if !open(p) || !w.discovered(p) {
				continue
			}
			for _, d := range neighbors8 {
				if changes(p.Add(d.X, d.Y)) {
					border = append(border, borderTile{p, w.roomOf(p)})
					break
				}
			}
		}
	}

	bw := bhi.X - blo.X + 1
	seen := make([]bool, bw*(bhi.Y-blo.Y+1))
	idx := func(p Point) int { return (p.Y-blo.Y)*bw + (p.X - blo.X) }
	var queue []Point
	for i, start := range border {
		if seen[idx(start.p)] {
			continue // reached from an earlier tile of its room
		}
		// The first border tile of a room not yet flooded: flood from it, and
		// every later border tile of the same room must turn up.
		pending := 0
		for _, b := range border[i+1:] {
			if b.room == start.room && !seen[idx(b.p)] {
				pending++
			}
		}
		if pending == 0 {
			continue
		}
		seen[idx(start.p)] = true
		queue = append(queue[:0], start.p)
		for len(queue) > 0 && pending > 0 {
			p := queue[0]
			queue = queue[1:]
			for _, d := range neighbors8 {
				n := p.Add(d.X, d.Y)
				if !open(n) || seen[idx(n)] {
					continue
				}
				seen[idx(n)] = true
				queue = append(queue, n)
				for _, b := range border[i+1:] {
					if b.p == n && b.room == start.room {
						pending--
						break
					}
				}
			}
		}
		if pending > 0 {
			return false
		}
	}
	return true
}
