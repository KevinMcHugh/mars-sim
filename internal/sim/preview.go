package sim

import "sync"

// ChunkPreview shows frontends what a chunk the simulation has not generated
// yet will hold, without generating it. It has its own worldGen and never
// touches the World, so however much of the map a frontend looks at, the
// simulation's world (and which of its chunks exist) is unaffected. That is
// the rule lazy generation depends on: only the simulation's own exploration
// generates a chunk. See docs/worldgen-chunks.md.
//
// A chunk's content is a pure function of the config and its coordinates, so
// what the preview shows is exactly what the chunk will hold when it is
// generated, until the colony changes it.
//
// One preview is shared by every Snapshot an engine publishes, and so by
// every frontend reading them: it is safe for concurrent use.
type ChunkPreview struct {
	mu     sync.Mutex
	gen    *worldGen
	chunks genCache[chunkKey, *chunkContent]
}

// previewCacheChunks bounds how many previewed chunks are kept: between this
// many and twice it, since genCache keeps two generations. A screenful of map
// is a handful of chunks, and a pan previews the few it uncovers. A view of
// the whole map (a minimap, say) should walk it chunk by chunk rather than
// row by row across a map wider than this many chunks, or it will preview
// each chunk once per row of it.
const previewCacheChunks = 256

func newChunkPreview(cfg Config, level Level) *ChunkPreview {
	return &ChunkPreview{gen: newWorldGen(cfg, level), chunks: newGenCache[chunkKey, *chunkContent](previewCacheChunks)}
}

// At returns the tile at the in-bounds p as generation will first lay it
// down: its ore, and hidden cavern floor where there will be one. Nothing is
// explored.
func (c *ChunkPreview) At(p Point) Tile {
	k := chunkKey{int32(p.X >> genChunkBits), int32(p.Y >> genChunkBits)}
	c.mu.Lock()
	content, ok := c.chunks.get(k)
	if !ok {
		content = c.gen.chunk(int(k.cx), int(k.cy))
		c.chunks.put(k, content)
	}
	c.mu.Unlock()
	off := offset(p.X, p.Y)
	t := Tile{Terrain: Rock, Composition: content.comp[off]}
	if content.isFloor(off) {
		t.Terrain = Floor
	}
	return t
}
