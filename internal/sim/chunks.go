package sim

// chunkSize is the edge length, in tiles, of a spatial chunk. Chunks bucket
// entities so neighbor queries scan only the relevant part of the map instead
// of every entity, and (in the next step) they bound region recomputation to a
// single chunk when a tile changes.
const chunkSize = 16

func ceilDiv(a, b int) int { return (a + b - 1) / b }

// chunkIndexOf returns the chunk-grid index for an in-bounds point.
func (w *World) chunkIndexOf(p Point) int {
	return (p.Y/chunkSize)*w.chunkCols + (p.X / chunkSize)
}

// removeFromChunkIndex removes an entity id from a chunk bucket by swap-delete
// (order within a bucket does not matter; queries tie-break on ID).
func (w *World) removeFromChunkIndex(ci int, id EntityID) {
	b := w.chunkEntities[ci]
	for i, x := range b {
		if x == id {
			b[i] = b[len(b)-1]
			w.chunkEntities[ci] = b[:len(b)-1]
			return
		}
	}
}
