package sim

// cargoLine records that n of the kind a colonist is carrying belong to
// owner, not to the carrier. Every unit not on a line is the carrier's own.
//
// Ownership used to be one owner per item kind, so every unit of a kind in
// the pockets was taken to be the same owner's. A builder that fetched the
// colony's iron and then mined some of its own delivered all of it to the
// colony, unpaid; a hauler ate the colony's meal it was carrying, free; and a
// planner sold colony meals from a closed haul order as its own. See
// docs/property.md.
type cargoLine struct {
	Owner Owner
	Kind  ItemKind
	N     int
}

// carriedFor is how many of kind e carries that are owner's: its line, or
// for e itself whatever is on no line.
func (e *Entity) carriedFor(owner Owner, kind ItemKind) int {
	if owner == ColonistOwner(e.ID) || owner.Kind == OwnerNone {
		return max(0, e.Inventory.Count(kind)-e.foreignCargo(kind))
	}
	for _, l := range e.cargo {
		if l.Owner == owner && l.Kind == kind {
			return l.N
		}
	}
	return 0
}

// ownCarried is how many of kind e carries that are its own.
func (e *Entity) ownCarried(kind ItemKind) int {
	return e.carriedFor(ColonistOwner(e.ID), kind)
}

// foreignCargo is how many of kind e carries for anyone else.
func (e *Entity) foreignCargo(kind ItemKind) int {
	n := 0
	for _, l := range e.cargo {
		if l.Kind == kind {
			n += l.N
		}
	}
	return n
}

// addCargo records that n of kind just put in e's pockets are owner's. The
// caller adds the items; e's own need no record.
func (e *Entity) addCargo(owner Owner, kind ItemKind, n int) {
	if n <= 0 || owner.Kind == OwnerNone || owner == ColonistOwner(e.ID) {
		return
	}
	for i := range e.cargo {
		if e.cargo[i].Owner == owner && e.cargo[i].Kind == kind {
			e.cargo[i].N += n
			return
		}
	}
	e.cargo = append(e.cargo, cargoLine{Owner: owner, Kind: kind, N: n})
}

// takeCargo records that n of owner's kind left e's pockets (the caller
// removes the items). For e's own units there is nothing to record.
func (e *Entity) takeCargo(owner Owner, kind ItemKind, n int) {
	for i := range e.cargo {
		l := &e.cargo[i]
		if l.Owner != owner || l.Kind != kind {
			continue
		}
		l.N -= min(n, l.N)
		if l.N == 0 {
			e.cargo = append(e.cargo[:i], e.cargo[i+1:]...)
		}
		return
	}
}

// unloadCargo splits all n of kind in e's pockets by owner — every line, then
// e's own for the rest — and forgets the lines. The caller removes the items
// and credits each share.
func (e *Entity) unloadCargo(kind ItemKind) []cargoLine {
	total := e.Inventory.Count(kind)
	var out []cargoLine
	kept := e.cargo[:0]
	for _, l := range e.cargo {
		if l.Kind != kind {
			kept = append(kept, l)
			continue
		}
		n := min(l.N, total)
		if n > 0 {
			out = append(out, cargoLine{Owner: l.Owner, Kind: kind, N: n})
			total -= n
		}
	}
	e.cargo = kept
	if total > 0 {
		out = append(out, cargoLine{Owner: ColonistOwner(e.ID), Kind: kind, N: total})
	}
	return out
}

// dropCargoOf forgets every line for kind: the items were destroyed.
func (e *Entity) dropCargoOf(kind ItemKind) {
	kept := e.cargo[:0]
	for _, l := range e.cargo {
		if l.Kind != kind {
			kept = append(kept, l)
		}
	}
	e.cargo = kept
}
