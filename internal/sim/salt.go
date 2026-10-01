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
	_, ok := w.salt[p]
	return ok
}

// clearSalt destroys the deposit on p: something was built over it.
func (w *World) clearSalt(p Point) {
	delete(w.salt, p)
}
