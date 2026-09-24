package sim

// ---- Construction costs ------------------------------------------------------
//
// With construction-costs on, building a structure consumes materials. The
// builder pays from its own stock — what it carries, topped up from its own
// lines in chests it can reach — and a colonist never takes on a task it could
// not pay for, so it mines instead and the rock that mining yields is what
// the colony builds with. Off (the default), building is free, as it always
// was. The builder donates what it spends: until labor orders exist (economy
// phase E5) there is nobody to pay it back. See docs/construction.md.

// constructionCost is what raising a tile of terrain t consumes. Digging
// (Floor) costs nothing; it is what produces material in the first place.
func constructionCost(t Terrain) []ItemStack {
	switch t {
	case Wall:
		return []ItemStack{{RawRock, 1}}
	case NutrientPod, Toilet, Bed:
		return []ItemStack{{RawRock, 2}}
	case Storage:
		return []ItemStack{{RawRock, 2}, {IronOre, 1}}
	case Incinerator:
		return []ItemStack{{RawRock, 2}, {IronOre, 2}}
	case Scumhouse:
		return []ItemStack{{RawRock, 2}, {Clay, 2}}
	default:
		return nil
	}
}

// buildCost is what building t costs under the current settings.
func (w *World) buildCost(t Terrain) []ItemStack {
	if !w.cfg.ConstructionCosts {
		return nil
	}
	return constructionCost(t)
}

// missingMaterials lists what e still needs in hand to build t.
func missingMaterials(e *Entity, cost []ItemStack) []ItemStack {
	var out []ItemStack
	for _, c := range cost {
		if have := e.Inventory.Count(c.Kind); have < c.Count {
			out = append(out, ItemStack{c.Kind, c.Count - have})
		}
	}
	return out
}

// materialSource finds the nearest chest e can reach and use that holds, on
// e's own account, everything in missing. Ties break by position.
func (w *World) materialSource(e *Entity, missing []ItemStack) (Point, bool) {
	room := w.roomOf(e.Pos)
	me := ColonistOwner(e.ID)
	var best Point
	bestDist, found := 1<<30, false
	for p, c := range w.storageContainers {
		if c.Terrain != Storage || !w.canUseFixture(e, p) || !w.taskReachable(p, room) {
			continue
		}
		enough := true
		for _, m := range missing {
			if c.held(me, m.Kind) < m.Count {
				enough = false
				break
			}
		}
		if !enough {
			continue
		}
		d := e.Pos.Chebyshev(p)
		if !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
			best, bestDist, found = p, d, true
		}
	}
	return best, found
}

// canAffordBuild reports whether e could pay for building t: it carries the
// materials, or can fetch the rest from one of its own chests. Always true
// with construction costs off.
func (w *World) canAffordBuild(e *Entity, t Terrain) bool {
	missing := missingMaterials(e, w.buildCost(t))
	if len(missing) == 0 {
		return true
	}
	if !e.Inventory.CanAddAll(missing...) {
		return false
	}
	_, ok := w.materialSource(e, missing)
	return ok
}

// gatherBuildMaterials is the part of a build job that gets the materials in
// hand. It reports whether the build can go ahead this tick; false means it is
// still fetching (ok) or cannot be paid for at all (!ok, and the job should
// be dropped).
func (w *World) gatherBuildMaterials(e *Entity) (ready, ok bool) {
	missing := missingMaterials(e, w.buildCost(e.BuildKind))
	if len(missing) == 0 {
		return true, true
	}
	src, found := w.materialSource(e, missing)
	if !found || !e.Inventory.CanAddAll(missing...) {
		return false, false
	}
	arrived, reachable := w.travelTo(e, src)
	if !reachable {
		return false, false
	}
	if !arrived {
		e.State = Moving
		return false, true
	}
	c := w.storageContainers[src]
	me := ColonistOwner(e.ID)
	for _, m := range missing {
		if !c.debit(me, m.Kind, m.Count) {
			return false, false
		}
		e.Inventory.Add(m.Kind, m.Count)
	}
	return false, true // in hand; walk to the site next tick
}

// payForBuild consumes a finished structure's cost from the builder's hands.
func (w *World) payForBuild(e *Entity) bool {
	cost := w.buildCost(e.BuildKind)
	if len(missingMaterials(e, cost)) > 0 {
		return false
	}
	for _, c := range cost {
		e.Inventory.Remove(c.Kind, c.Count)
	}
	return true
}
