package sim

import "fmt"

// Siting: who picks where the colony digs down. A stair, a shaft, a hole or
// a ladder ordered with a tile (the order's At) is checked and marked out at
// once, or refused in the log with the reason; this is how the browser
// orders them. An order without a tile (At left zero, which is never a dig
// site: level 0 is the surface) leaves the tile to the planners, and only
// with Config.SitingAuto, which the terminal and headless runs turn on
// because they have no way to pick a tile. See docs/siting.md.

// sited reports whether an order's At names a tile.
func sited(p Point) bool { return p != Point{} }

// orderDigDown applies a dig-down order: OrderStair, OrderShaft, OrderHole
// or OrderLadder.
func (w *World) orderDigDown(c Command) {
	var at Point
	var what string
	switch c := c.(type) {
	case OrderStair:
		at, what = c.At, "stair"
	case OrderShaft:
		at, what = c.At, "shaft"
	case OrderHole:
		at, what = c.At, "hole"
	case OrderLadder:
		at, what = c.At, "ladder"
	default:
		return
	}
	if !sited(at) {
		if !w.cfg.SitingAuto {
			w.logEvent(LogBuildStart, fmt.Sprintf("Pick a tile for the %s: the colony sites nothing itself with siting-auto off.", what))
			return
		}
		switch c := c.(type) {
		case OrderStair:
			w.manualStairs++
		case OrderShaft:
			w.manualShaftLevels += max(1, c.Levels)
		case OrderHole:
			w.manualHoles++
		case OrderLadder:
			w.manualLadders++
		}
		return
	}
	var why string
	switch c := c.(type) {
	case OrderStair:
		if why = w.downRefusal(at, "stair"); why == "" {
			w.designateStair(at, Community)
		}
	case OrderShaft:
		var bottom Level
		if bottom, why = w.shaftRefusal(at, max(1, c.Levels)); why == "" {
			w.designateShaft(at, bottom, Community)
		}
	case OrderHole:
		if why = w.downRefusal(at, "hole"); why == "" {
			w.designateHole(at)
		}
	case OrderLadder:
		if why = w.ladderRefusal(at); why == "" {
			w.designateLadder(at)
		}
	}
	if why != "" {
		w.logEvent(LogBuildStart, fmt.Sprintf("No %s at (%d, %d): %s", what, at.X, at.Y, why))
	}
}

// siteRefusal is the rules every picked site shares, or "": a tile on a
// level that exists, which the colony has seen, not in a doorway, not
// already marked for building, and in the colony's main room, so a digger
// can get to it.
func (w *World) siteRefusal(p Point) string {
	switch {
	case !w.InBounds(p) || w.layerIn(p) == nil:
		return "There is no such tile."
	case !w.discovered(p):
		return "The colony has not seen that tile."
	case w.doorTiles[p]:
		return "Not in a doorway."
	case w.taskAt(p):
		return "That tile is already marked for building."
	case w.mainRoom == 0 || w.roomOf(p) != w.mainRoom:
		return "The colony cannot reach that tile."
	}
	return ""
}

// levelRefusal says why nothing can be dug down to level bottom, or "".
func (w *World) levelRefusal(p Point, bottom Level) string {
	if p.Level < LandingLevel || int(bottom) > w.cfg.DeepestLevel {
		return fmt.Sprintf("The colony may not dig below level %d.", w.cfg.DeepestLevel)
	}
	return ""
}

// downRefusal says why a stair or a hole (what) cannot be ordered at p, or
// "": p must be open floor over rock or floor (neither ever cuts into a
// structure below), above a level Config allows digging to, and pass
// siteRefusal. Unlike the planner (findStairSite), it does not ask for open
// floor all round: whether a stair crowds a corridor, or a hole cuts one, is
// the player's call.
func (w *World) downRefusal(p Point, what string) string {
	below := Point{p.X, p.Y, p.Level + 1}
	if why := w.levelRefusal(p, below.Level); why != "" {
		return why
	}
	if w.InBounds(p) && w.layerIn(p) != nil && w.discovered(p) {
		if w.TerrainAt(p) != Floor {
			return fmt.Sprintf("A %s can only be dug from open floor.", what)
		}
		if t := w.TerrainAt(below); t != Rock && t != Floor {
			return "Something is built under that tile."
		}
	}
	return w.siteRefusal(p)
}

// shaftRefusal says why a shaft levels deep cannot be ordered at p, or "",
// with the level it would reach: from open floor, a new shaft; from a
// shaft's top, deepening it. Either is cut short at the deepest level Config
// allows, and refused only if that leaves nothing to dig.
func (w *World) shaftRefusal(p Point, levels int) (Level, string) {
	from := p.Level
	if w.TerrainAt(p) == ShaftTop {
		from = w.shaftBottom(p)
	}
	bottom := min(from+Level(levels), Level(w.cfg.DeepestLevel))
	if why := w.levelRefusal(p, from+1); why != "" {
		return 0, why
	}
	if why := w.siteRefusal(p); why != "" {
		return 0, why
	}
	if t := w.TerrainAt(p); t != Floor && t != ShaftTop {
		return 0, "A shaft can only be sunk from open floor, or deepened from a shaft's top."
	}
	if !w.canDigShaft(p, bottom) {
		return 0, "Something is built in the shaft's way."
	}
	return bottom, ""
}

// ladderRefusal says why a ladder cannot be fitted into the hole at p, or
// "": a hole one level deep, open below, that the colony can stand beside
// and nobody is already fitting.
func (w *World) ladderRefusal(p Point) string {
	if !w.InBounds(p) || w.TerrainAt(p) != Hole {
		return "A ladder goes into a hole."
	}
	if _, levels, ok := w.fallTarget(p); !ok || levels != 1 {
		return "A ladder only fits a hole one level deep."
	}
	switch {
	case w.taskAt(p):
		return "That hole is already marked for a ladder."
	case w.mainRoom == 0 || w.fixtureCutOff(p):
		return "The colony cannot get beside that hole."
	case !w.canDigShaft(p, p.Level+1):
		return "Something is built under that hole."
	}
	return ""
}

// taskAt reports whether an unbuilt task of any project, in any phase, is
// on p.
func (w *World) taskAt(p Point) bool {
	for _, pr := range w.projects {
		for _, tk := range pr.tasks {
			if tk.pos == p && !w.taskDone(tk) {
				return true
			}
		}
	}
	return false
}
