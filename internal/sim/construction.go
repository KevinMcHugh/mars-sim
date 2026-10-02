package sim

// ---- Construction costs ------------------------------------------------------
//
// With construction-costs on, building a structure consumes materials. The
// builder pays from its own stock — what it carries, topped up from its own
// lines in chests it can reach — and a colonist never takes on a task it could
// not pay for, so it mines instead and the rock that mining yields is what
// the colony builds with. It is on by default (economy phase E8); off,
// building is free, as it used to be. Whoever pays for the work pays for its materials first: a public work
// draws on the colony's own stock (what it bought at its silo), a commission
// on its commissioner's, as far as a builder can reach them. Past that the
// builder donates what it spends, which is what keeps the colony building
// with an empty storeroom. See docs/construction.md and docs/hauling.md.

// constructionCost is what raising a tile of terrain t consumes. Digging
// (Floor) costs nothing; it is what produces material in the first place.
func constructionCost(t Terrain) []ItemStack {
	switch t {
	case Wall:
		return []ItemStack{{RawRock, 1}}
	case NutrientPod, Toilet, Bed, Chair:
		return []ItemStack{{RawRock, 2}}
	case Storage:
		return []ItemStack{{RawRock, 2}, {IronOre, 1}}
	case Incinerator:
		return []ItemStack{{RawRock, 2}, {IronOre, 2}}
	case Scumhouse:
		return []ItemStack{{RawRock, 2}, {Clay, 2}}
	case Forge:
		return []ItemStack{{Clay, 4}} // a clay furnace: nothing else
	case GunBench:
		return []ItemStack{{RawRock, 2}, {IronOre, 2}}
	case Incubator:
		return []ItemStack{{RawRock, 2}, {Clay, 1}}
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
func missingMaterials(e *Entity, cost []ItemStack, payers []Owner) []ItemStack {
	var out []ItemStack
	for _, c := range cost {
		have := 0
		for _, p := range payers {
			have += e.carriedFor(p, c.Kind)
		}
		if have < c.Count {
			out = append(out, ItemStack{c.Kind, c.Count - have})
		}
	}
	return out
}

// materialPayers is whose stock pays for e building for issuer, in order:
// the issuer's, then e's own. A lone emergency build has no issuer.
func materialPayers(e *Entity, issuer Owner) []Owner {
	me := ColonistOwner(e.ID)
	if issuer.Kind == OwnerNone || issuer == me {
		return []Owner{me}
	}
	return []Owner{issuer, me}
}

// materialSource finds the nearest chest e can reach and use that holds
// everything in missing on one payer's line — the first payer that has it
// anywhere. Ties break by position.
func (w *World) materialSource(e *Entity, missing []ItemStack, payers []Owner) (Point, Owner, bool) {
	room := w.roomOf(e.Pos)
	for _, payer := range payers {
		var best Point
		bestDist, found := 1<<30, false
		for p, c := range w.home.storageContainers {
			if c.Terrain != Storage || !w.canUseFixture(e, p) || !w.taskReachable(p, room) {
				continue
			}
			enough := true
			for _, m := range missing {
				if c.held(payer, m.Kind) < m.Count {
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
		if found {
			return best, payer, true
		}
	}
	return Point{}, Owner{}, false
}

// taskIssuer is who is paying for e's current build: its project's issuer,
// or nobody for a lone emergency build.
func taskIssuer(e *Entity) Owner {
	if e.task != nil && e.task.proj != nil {
		return e.task.proj.issuer
	}
	return Owner{}
}

// canAffordBuild reports whether e could pay for building t for issuer: it
// carries the materials, or can fetch the rest from the issuer's stock or its
// own. Always true with construction costs off.
func (w *World) canAffordBuild(e *Entity, t Terrain, issuer Owner) bool {
	payers := materialPayers(e, issuer)
	missing := missingMaterials(e, w.buildCost(t), payers)
	if len(missing) == 0 {
		return true
	}
	if !e.Inventory.CanAddAll(missing...) {
		return false
	}
	_, _, ok := w.materialSource(e, missing, payers)
	return ok
}

// gatherBuildMaterials is the part of a build job that gets the materials in
// hand. It reports whether the build can go ahead this tick; false means it is
// still fetching (ok) or cannot be paid for at all (!ok, and the job should
// be dropped).
func (w *World) gatherBuildMaterials(e *Entity) (ready, ok bool) {
	payers := materialPayers(e, taskIssuer(e))
	missing := missingMaterials(e, w.buildCost(e.BuildKind), payers)
	if len(missing) == 0 {
		return true, true
	}
	src, payer, found := w.materialSource(e, missing, payers)
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
	c := w.home.storageContainers[src]
	for _, m := range missing {
		if !c.debit(payer, m.Kind, m.Count) {
			return false, false
		}
		e.Inventory.Add(m.Kind, m.Count)
		e.addCargo(payer, m.Kind, m.Count) // the payer's until it is built with
	}
	return false, true // in hand; walk to the site next tick
}

// payForBuild consumes a finished structure's cost from the builder's hands.
func (w *World) payForBuild(e *Entity) bool {
	cost := w.buildCost(e.BuildKind)
	payers := materialPayers(e, taskIssuer(e))
	if len(missingMaterials(e, cost, payers)) > 0 {
		return false
	}
	// Spent from the payers in order: the issuer's units before the
	// builder's own.
	for _, c := range cost {
		left := c.Count
		for _, p := range payers {
			n := min(left, e.carriedFor(p, c.Kind))
			e.Inventory.Remove(c.Kind, n)
			e.takeCargo(p, c.Kind, n)
			left -= n
		}
	}
	return true
}
