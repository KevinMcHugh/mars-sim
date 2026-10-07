package sim

// ---- Salt -----------------------------------------------------------------------

// Salt is a deposit on a tile, laid down by world generation (saltPlan in
// worldgen_chunks.go, applyChunk) and then only ever lost. It rides on the
// rock the way cave scum does, so mining through the tile leaves it on the
// floor; unlike scum it has no accretion step, so nothing makes more. It
// shares a tile with scum never: generation steers salt around scum, and
// addScum will not start a patch on salt. See docs/salt.md.

// hasSalt reports whether p carries a salt deposit.
func (w *World) hasSalt(p Point) bool {
	_, ok := w.lay(p).salt[p]
	return ok
}

// clearSalt destroys the deposit on p: something was built over it.
func (w *World) clearSalt(p Point) {
	if _, ok := w.lay(p).salt[p]; !ok {
		return
	}
	delete(w.lay(p).salt, p)
	if _, exposed := w.lay(p).exposedSalt[p]; exposed {
		delete(w.lay(p).exposedSalt, p)
		w.saltRev++
	}
}

// refreshSaltExposure re-evaluates p and its neighbours after a terrain
// change, as refreshScumExposure does for scum, by the same rule
// (scumExposed): the deposit is on floor the colony has discovered, or on rock
// beside it. Only exposed salt is published, so a frontend never learns what
// lies under undiscovered rock, and a big map's millions of deposits never
// reach a frame.
func (w *World) refreshSaltExposure(p Point) {
	check := func(q Point) {
		if _, ok := w.lay(q).salt[q]; !ok {
			return
		}
		_, was := w.lay(q).exposedSalt[q]
		if now := w.scumExposed(q); now == was {
			return
		} else if now {
			w.lay(q).exposedSalt[q] = struct{}{}
		} else {
			delete(w.lay(q).exposedSalt, q)
		}
		w.saltRev++
	}
	check(p)
	for _, d := range neighbors8 {
		check(p.Add(d.X, d.Y))
	}
}
