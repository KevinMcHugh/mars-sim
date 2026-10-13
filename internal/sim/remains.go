package sim

// Remains is one identified body lying on a tile: who it was (look the ID up
// in Snapshot.Deceased or Snapshot.Graveyard) and what kind of body it left.
//
// Refuse counts bodies by kind and nothing more (refuseCell), which is all the
// cleaners need. The remains index sits beside it so a player who finds a body
// can ask whose it is. See docs/remains.md.
type Remains struct {
	ID   EntityID
	Body ItemKind
}

// leaveBody leaves e's body of the given kind on p, remembering whose it is.
// It is addCorpse for a death with a known victim; refuse that arrives without
// one (a hauler tipping its load down a hole) still goes through addCorpse and
// lies there unidentified.
func (w *World) leaveBody(e *Entity, p Point, kind ItemKind) {
	before := w.corpsesOfAt(p, kind)
	w.addCorpse(p, kind)
	if w.corpsesOfAt(p, kind) == before { // out of bounds, or the tile is full
		return
	}
	if w.remains == nil {
		w.remains = make(map[Point][]Remains)
	}
	w.remains[p] = append(w.remains[p], Remains{ID: e.ID, Body: kind})
	w.remainsRev++
}

// forgetRemains drops the record of one body of kind on p, the oldest first,
// once that body has left the tile. A body picked up loses its name: carried
// cargo is a plain item stack, so the identity does not travel with it.
func (w *World) forgetRemains(p Point, kind ItemKind) {
	rs := w.remains[p]
	for i, r := range rs {
		if r.Body != kind {
			continue
		}
		w.remainsRev++
		if len(rs) == 1 {
			delete(w.remains, p)
			return
		}
		w.remains[p] = append(rs[:i:i], rs[i+1:]...)
		return
	}
}

// clearAllRemains forgets every body on p, for when building over the tile
// scrapes its refuse away.
func (w *World) clearAllRemains(p Point) {
	if _, ok := w.remains[p]; ok {
		delete(w.remains, p)
		w.remainsRev++
	}
}

// publishedRemains returns an immutable copy of the remains index, reusing the
// last one while it is still exact (remainsRev): bodies come and go only when
// something dies or a cleaner hauls one off.
func (w *World) publishedRemains() map[Point][]Remains {
	if w.snapRemains != nil && w.snapRemainsRev == w.remainsRev {
		return w.snapRemains
	}
	out := make(map[Point][]Remains, len(w.remains))
	for p, rs := range w.remains {
		out[p] = append([]Remains(nil), rs...)
	}
	w.snapRemains, w.snapRemainsRev = out, w.remainsRev
	return out
}

// RemainsAt lists the identified bodies on p, oldest first. A tile can hold
// more bodies than this names (Tile.Corpses); the rest are unidentified.
func (s *Snapshot) RemainsAt(p Point) []Remains { return s.Remains[p] }
