package sim

// Place classes: where a colonist is, as far as its drives are concerned.
// Safety is the first drive whose rate depends on it (DriveSpec.Place): it
// falls at home and under guard and builds on the frontier. syncDriveActivity
// recomputes the class after every colonist turn and refreshes the drives only
// when it (or the colonist's best weapon) has changed. See docs/safety.md.

// PlaceClass is where a colonist stands. The zero value is unset, never a
// real class, so a colonist whose class has not been worked out yet grows its
// drives at 100%.
type PlaceClass uint8

const (
	placeNone PlaceClass = iota // unset
	// PlaceHome is inside the colonist's own ship or a room it commissioned.
	PlaceHome
	// PlaceSettled is anywhere else the colony has zoned: its rooms stand in
	// zones whether the player or zoning-auto drew them, and ships hold their
	// ground as residence.
	PlaceSettled
	// PlaceFrontier is everywhere else: dug passages, caverns, rock faces, and
	// every level but the landing level, which has no zones.
	PlaceFrontier
	// PlaceGuarded is inside a defense zone with someone else on station.
	PlaceGuarded

	placeEnd // keep last
)

// numPlaces counts the real classes (not placeNone).
const numPlaces = int(placeEnd) - 1

// placeNames names each class for the config file and the docs, indexed by
// PlaceClass.index().
var placeNames = [numPlaces]string{"home", "settled", "frontier", "guarded"}

// index is p's slot in per-class arrays such as DriveSpec.Place.
func (p PlaceClass) index() int { return int(p) - 1 }

func (p PlaceClass) String() string {
	if p == placeNone || p >= placeEnd {
		return "unset"
	}
	return placeNames[p.index()]
}

// numWeaponRanks is how many weapon ranks DriveSpec.Armed has a percent for:
// weaponRank 1 (pistol) through 3 (rifle).
const numWeaponRanks = 3

// weaponRankNames names DriveSpec.Armed's slots for the config file.
var weaponRankNames = [numWeaponRanks]string{"pistol", "shotgun", "rifle"}

// placePercent is the percent of drive d's rate that applies where e stands:
// its place class's, plus PlaceDepth for every level below the landing level
// when that is the frontier.
func (w *World) placePercent(e *Entity, d DriveKind) int {
	if e.place == placeNone {
		return 100
	}
	spec := &w.cfg.Drives[d]
	pct := spec.Place[e.place.index()]
	if e.place == PlaceFrontier {
		pct += spec.PlaceDepth * depthOf(e.Pos.Level)
	}
	return pct
}

// placeOf works out e's place class from where it stands now.
func (w *World) placeOf(e *Entity) PlaceClass {
	p := e.Pos
	if w.guardedAt(e, p) {
		return PlaceGuarded
	}
	if w.atHome(e, p) {
		return PlaceHome
	}
	if w.zoneAt(p) != NoZone {
		return PlaceSettled
	}
	return PlaceFrontier
}

// guardedAt reports whether p lies in a defense zone that someone other than
// e is standing in on station.
func (w *World) guardedAt(e *Entity, p Point) bool {
	for _, d := range w.defensePosts {
		if !d.contains(p) {
			continue
		}
		for _, id := range d.guards {
			if g := w.entities[id]; id != e.ID && g != nil && g.Alive() && d.contains(g.Pos) {
				return true
			}
		}
	}
	return false
}

// atHome reports whether p is inside e's own ship (its hull and walkway) or
// a room e commissioned (a house, a kitchen of its own).
func (w *World) atHome(e *Entity, p Point) bool {
	if sh := w.shipByID(e.ship); sh != nil && sh.structure != nil {
		if s := sh.structure; len(s.tiles) > 0 && s.tiles[0].Level == p.Level &&
			p.X >= s.x0 && p.X <= s.x1 && p.Y >= s.y0 && p.Y <= s.y1 {
			return true
		}
	}
	for _, r := range w.homeRooms(e) {
		if r.lo.Level == p.Level && inBox(r.lo, r.hi, p) {
			return true
		}
	}
	return false
}

// homeRooms is every standing room e commissioned, by structure id. The
// world indexes them by commissioner once per change to the structure
// registry, so reading them each turn costs a comparison and a lookup.
func (w *World) homeRooms(e *Entity) []*roomRecord {
	if w.homeRoomsRev != w.structureRev+1 {
		clear(w.homeRoomsBy)
		if w.homeRoomsBy == nil {
			w.homeRoomsBy = map[EntityID][]*roomRecord{}
		}
		for _, s := range w.sortedStructures() {
			if s.room != nil && s.room.issuer.Kind == OwnerColonist {
				id := EntityID(s.room.issuer.ID)
				w.homeRoomsBy[id] = append(w.homeRoomsBy[id], s.room)
			}
		}
		w.homeRoomsRev = w.structureRev + 1
	}
	return w.homeRoomsBy[e.ID]
}

// daringSeed separates the daring trait stream (Bold, Timid) from the others
// (see ownStreamGroup and docs/rng-streams.md).
const daringSeed = 0x2F8B1D93A4C76E15
