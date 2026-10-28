package sim

import (
	"fmt"
	"slices"
)

// ---- Defense zones ---------------------------------------------------------
//
// A defense zone is an area the player wants kept safe: a rectangle on any
// level and a detail, the number of armed colonists to keep on station there.
// Nothing is built in one. Armed colonists looking for work take a post that
// is short of its detail (JobGuard), walk to a station inside the area, and
// patrol it, shooting any alien that comes within guard-sight of them and of
// the area. The colony pays each guard wage-guard for every hour on station,
// straight from the treasury as far as it goes, as it pays its cook.
//
// A guard leaves its post when anything outranks work (hunger, sleep, an
// alien close enough to fight or flee from on its own account): leaving
// clears the job, which frees the place for the next armed colonist looking
// for work, so a detail keeps itself staffed in shifts without a roster.
//
// Defense zones are not on the zone grid (zones.go). That grid holds one kind
// per tile, on the landing level only, and every kind on it is somewhere to
// build: a defense zone overlaps the residence it guards, is most wanted
// below the landing level (round a shaft aliens climb out of), and needs an
// identity to carry its detail. See docs/defense.md.

// MaxDefenseTiles caps one defense zone's area, as maxExcavationTiles caps a
// dig: a guard picks its station by searching the area.
const MaxDefenseTiles = 1600

// MaxDefenseDetail caps how many guards one defense zone asks for.
const MaxDefenseDetail = 20

// guardDwellTicks is how long a guard holds one station before it patrols to
// another tile of its area.
const guardDwellTicks = 48

// OrderDefense asks for an area to be kept safe: the rectangle with corners
// (X0, Y0) and (X1, Y1), inclusive, in either order, on Level, held by Detail
// armed colonists. The outcome is logged.
type OrderDefense struct {
	X0, Y0, X1, Y1 int
	Level          Level // the zero value is the landing level (orderLevel)
	Detail         int
}

func (OrderDefense) isCommand() {}

// SetDefenseDetail changes how many guards a defense zone asks for. Guards
// over the new detail go back to other work.
type SetDefenseDetail struct {
	ID     int
	Detail int
}

func (SetDefenseDetail) isCommand() {}

// CancelDefense removes a defense zone; its guards go back to other work.
type CancelDefense struct{ ID int }

func (CancelDefense) isCommand() {}

// defensePost is one defense zone.
type defensePost struct {
	id             int
	level          Level
	x0, y0, x1, y1 int
	detail         int
	// guards are the colonists holding JobGuard for this post, in the order
	// they took it.
	guards []EntityID
	// paid is what the colony has paid its guards, for the Zones tab.
	paid Money
	// openedTick is when it was ordered.
	openedTick int
}

// contains reports whether p lies in the post's area.
func (d *defensePost) contains(p Point) bool {
	return p.Level == d.level && p.X >= d.x0 && p.X <= d.x1 && p.Y >= d.y0 && p.Y <= d.y1
}

// near reports whether p lies within margin tiles of the post's area.
func (d *defensePost) near(p Point, margin int) bool {
	return p.Level == d.level && p.X >= d.x0-margin && p.X <= d.x1+margin &&
		p.Y >= d.y0-margin && p.Y <= d.y1+margin
}

// defensePost is the post with this id, or nil.
func (w *World) defensePost(id int) *defensePost {
	for _, d := range w.defensePosts {
		if d.id == id {
			return d
		}
	}
	return nil
}

// orderDefense carries out an OrderDefense, and reports whether it did.
func (w *World) orderDefense(c OrderDefense) bool {
	x0, y0, x1, y1 := w.clampRect(c.X0, c.Y0, c.X1, c.Y1)
	level := orderLevel(c.Level)
	if x0 > x1 || y0 > y1 || w.layer(level) == nil {
		w.logEvent(LogBuildStart, "There is nothing the colony has seen there to defend.")
		return false
	}
	if n := (x1 - x0 + 1) * (y1 - y0 + 1); n > MaxDefenseTiles {
		w.logEvent(LogBuildStart, fmt.Sprintf("A defense zone can cover at most %d tiles; that area is %d.", MaxDefenseTiles, n))
		return false
	}
	seen := false
	for y := y0; y <= y1 && !seen; y++ {
		for x := x0; x <= x1; x++ {
			if w.discovered(Point{x, y, level}) {
				seen = true
				break
			}
		}
	}
	if !seen {
		w.logEvent(LogBuildStart, "There is nothing the colony has seen there to defend.")
		return false
	}
	detail := min(max(c.Detail, 1), MaxDefenseDetail)
	w.nextDefenseID++
	w.defensePosts = append(w.defensePosts, &defensePost{id: w.nextDefenseID, level: level,
		x0: x0, y0: y0, x1: x1, y1: y1, detail: detail, openedTick: w.tick})
	w.logEventAt(LogBuildStart, rectSite(x0, y0, x1, y1, level), fmt.Sprintf("The colony posts a detail of %s to defend %d,%d to %d,%d%s, at %v an hour each.",
		plural(detail, "armed colonist"), x0, y0, x1, y1, levelPhrase(level), Money(w.cfg.WageGuard)))
	return true
}

// levelPhrase is ", level N" for a level other than the landing level.
func levelPhrase(l Level) string {
	if l == LandingLevel {
		return ""
	}
	return fmt.Sprintf(", level %d", l)
}

// setDefenseDetail carries out a SetDefenseDetail, and reports whether it did.
func (w *World) setDefenseDetail(c SetDefenseDetail) bool {
	d := w.defensePost(c.ID)
	if d == nil {
		return false
	}
	detail := min(max(c.Detail, 1), MaxDefenseDetail)
	if detail == d.detail {
		return false
	}
	d.detail = detail
	// The guards who came last go first.
	for len(d.guards) > detail {
		if g := w.entities[d.guards[len(d.guards)-1]]; g != nil {
			w.clearJob(g) // releaseGuard drops it from d.guards
		} else {
			d.guards = d.guards[:len(d.guards)-1]
		}
	}
	w.logEventAt(LogBuildStart, rectSite(d.x0, d.y0, d.x1, d.y1, d.level), fmt.Sprintf("The detail defending %d,%d to %d,%d is now %s.",
		d.x0, d.y0, d.x1, d.y1, plural(detail, "armed colonist")))
	return true
}

// cancelDefense carries out a CancelDefense, and reports whether it did.
func (w *World) cancelDefense(id int) bool {
	d := w.defensePost(id)
	if d == nil {
		return false
	}
	for _, gid := range slices.Clone(d.guards) {
		if g := w.entities[gid]; g != nil {
			w.clearJob(g)
		}
	}
	w.defensePosts = slices.DeleteFunc(w.defensePosts, func(o *defensePost) bool { return o == d })
	w.logEventAt(LogBuildStart, rectSite(d.x0, d.y0, d.x1, d.y1, d.level), fmt.Sprintf("The colony stands down the detail defending %d,%d to %d,%d%s.",
		d.x0, d.y0, d.x1, d.y1, levelPhrase(d.level)))
	return true
}

// ---- Guard duty ------------------------------------------------------------

// tryAssignGuard puts an armed colonist looking for work on the first defense
// zone, by id, that is short of its detail and has a station it can reach.
func (w *World) tryAssignGuard(e *Entity) bool {
	if len(w.defensePosts) == 0 || bestWeapon(e.Inventory) == ItemNone || !calmEnoughToGuard(e) {
		return false
	}
	for _, d := range w.defensePosts {
		if len(d.guards) >= d.detail {
			continue
		}
		station, ok := w.guardStation(e, d)
		if !ok {
			continue
		}
		d.guards = append(d.guards, e.ID)
		e.Job, e.Target, e.Progress = JobGuard, station, 0
		e.guardPost, e.guardDwell = d.id, 0
		return true
	}
	return false
}

// guardStation picks the tile of d's area that e should stand on: the
// walkable, seen tile nearest e, in its room, that no other guard of d is
// headed for or standing on. Ties break row-major, so the pick is
// deterministic.
func (w *World) guardStation(e *Entity, d *defensePost) (Point, bool) {
	taken := make(map[Point]bool, len(d.guards))
	for _, gid := range d.guards {
		if g := w.entities[gid]; g != nil && g.ID != e.ID {
			taken[g.Target] = true
		}
	}
	room := w.roomOf(e.Pos)
	best, bestDist, found := Point{}, 0, false
	for y := d.y0; y <= d.y1; y++ {
		for x := d.x0; x <= d.x1; x++ {
			p := Point{x, y, d.level}
			if taken[p] || !w.Walkable(p) || !w.discovered(p) || w.roomOf(p) != room {
				continue
			}
			if dist := w.travelEstimate(e.Pos, p); !found || dist < bestDist {
				best, bestDist, found = p, dist, true
			}
		}
	}
	return best, found
}

// patrolStation picks a random walkable tile of d's area in e's room for a
// guard to move on to, or reports none.
func (w *World) patrolStation(e *Entity, d *defensePost) (Point, bool) {
	room := w.roomOf(e.Pos)
	// A few draws rather than a list of every tile: an area can be 1600
	// tiles, and a guard patrols every guardDwellTicks.
	for range 8 {
		p := Point{d.x0 + w.rng.IntN(d.x1-d.x0+1), d.y0 + w.rng.IntN(d.y1-d.y0+1), d.level}
		if p != e.Pos && w.Walkable(p) && w.discovered(p) && !w.occupiedByOther(p, e.ID) &&
			!w.buildTiles[p] && w.roomOf(p) == room {
			return p, true
		}
	}
	return Point{}, false
}

// releaseGuard drops e from its defense post. clearJob calls it.
func (w *World) releaseGuard(e *Entity) {
	if d := w.defensePost(e.guardPost); d != nil {
		d.guards = slices.DeleteFunc(d.guards, func(id EntityID) bool { return id == e.ID })
	}
	e.guardPost, e.guardDwell, e.guardHour = 0, 0, 0
}

// jobGuard is one tick of guard duty: shoot an alien threatening the area,
// else walk to the station, else hold it (and now and then patrol on).
func (w *World) jobGuard(e *Entity) {
	d := w.defensePost(e.guardPost)
	weapon := bestWeapon(e.Inventory)
	// Calm to serve: a guard who comes to feel unsafe gives the post up, and
	// the next calm, armed colonist takes it (see docs/safety.md).
	if d == nil || weapon == ItemNone || !calmEnoughToGuard(e) {
		w.clearJob(e)
		return
	}
	e.resting = false
	if a, ok := w.nearestThreat(e.Pos, w.cfg.GuardSight); ok && d.near(a.Pos, w.cfg.GuardSight) {
		w.fightAlien(e, a, weapon)
		w.serveGuard(e, d)
		return
	}
	if e.Pos != e.Target {
		if e.Pos.Adjacent(e.Target) {
			if !w.occupiedByOther(e.Target, e.ID) && w.Walkable(e.Target) {
				w.moveEntity(e, e.Target)
				e.State = Moving
				w.serveGuard(e, d)
				return
			}
			// Someone is standing on it: this tile will do.
			if d.contains(e.Pos) {
				e.Target = e.Pos
			} else if s, ok := w.guardStation(e, d); ok {
				e.Target = s
			}
		} else {
			e.State = Moving
			if _, ok := w.travelTo(e, e.Target); !ok {
				// The way is shut: pick another station, or give the post
				// up so someone who can reach it takes it.
				if s, ok := w.guardStation(e, d); ok && s != e.Target {
					e.Target = s
					e.clearPath()
				} else {
					w.clearJob(e)
					return
				}
			}
			w.serveGuard(e, d)
			return
		}
	}
	e.State = Guarding
	e.guardDwell++
	w.serveGuard(e, d)
	if e.guardDwell >= guardDwellTicks || w.idleWouldBlock(e.Pos) {
		if p, ok := w.patrolStation(e, d); ok {
			e.Target, e.guardDwell = p, 0
		}
	}
}

// serveGuard counts a tick of e's duty at d, and pays it an hour's wage for
// every hour it has spent inside the area.
func (w *World) serveGuard(e *Entity, d *defensePost) {
	if !d.contains(e.Pos) {
		return
	}
	w.practise(e, SkillGuarding, 1)
	e.guardHour++

	if e.guardHour < w.cfg.TicksPerHour() {
		return
	}
	e.guardHour = 0
	if pay := Money(w.cfg.WageGuard); pay > 0 && w.transfer(Community, ColonistOwner(e.ID), pay) {
		d.paid += pay // as far as the treasury goes
	}
}

// ---- Publishing ------------------------------------------------------------

// DefenseView is one defense zone, for frontends.
type DefenseView struct {
	ID             int
	Level          Level
	X0, Y0, X1, Y1 int
	Detail         int
	// Guards are the colonists holding the post, and OnStation how many of
	// them are inside the area now.
	Guards    []EntityID
	OnStation int
	Paid      Money
}

// publishedDefense is every defense zone by id. It is built fresh for each
// snapshot (OnStation moves every tick a guard does), and costs nothing in a
// game with none.
func (w *World) publishedDefense() []DefenseView {
	if len(w.defensePosts) == 0 {
		return nil
	}
	out := make([]DefenseView, 0, len(w.defensePosts))
	for _, d := range w.defensePosts {
		v := DefenseView{ID: d.id, Level: d.level, X0: d.x0, Y0: d.y0, X1: d.x1, Y1: d.y1,
			Detail: d.detail, Guards: slices.Clone(d.guards), Paid: d.paid}
		for _, gid := range d.guards {
			if g := w.entities[gid]; g != nil && d.contains(g.Pos) {
				v.OnStation++
			}
		}
		out = append(out, v)
	}
	return out
}

// armedColonists is how many living colonists carry a weapon.
func (w *World) armedColonists() int {
	n := 0
	for _, e := range w.entities {
		if e.Kind == Colonist && e.Alive() && bestWeapon(e.Inventory) != ItemNone {
			n++
		}
	}
	return n
}
