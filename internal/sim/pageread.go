package sim

import "sort"

// Bulk reads of the published terrain, a page at a time, for a frontend that
// forwards terrain rather than drawing it tile by tile: the browser's wire
// encoder (see docs/browser-frontend.md). Snapshot.TileAt answers the same
// questions one tile at a time, at the price of a bounds check, a page lookup
// and a refuse-map lookup per tile; a page is 4096 of those.

// PageIndex returns the page table index, as in TileChanges.Pages and
// PageOrigin, of the page holding the in-bounds p.
func (g *TileGrid) PageIndex(p Point) int {
	return (p.Y>>gridPageBits)<<g.colShift | (p.X >> gridPageBits)
}

// ReadPage fills dst, which must hold TilePageSide*TilePageSide tiles, with
// page pi as TileAt reads each tile: row by row within the page, dst[0] being
// the tile at PageOrigin(pi). Tiles past the edge of the map read as Rock.
//
// Refuse is left out (Gore and Corpses read zero): read it once for the whole
// map from Tiles.RefuseTiles instead.
//
// It reports false, leaving dst alone, when the page has nothing to show: its
// chunk has not been generated and fog of war is on, so every tile of it
// would read as unexplored Rock. With fog off, an ungenerated page reads from
// the preview, as TileAt does.
func (s *Snapshot) ReadPage(pi int, dst []Tile) bool {
	dst = dst[:gridPageLen]
	g := s.Tiles
	var page *tilePage
	if pi >= 0 && pi < len(g.pages) {
		page = g.pages[pi]
	}
	origin := g.PageOrigin(pi)
	if page != nil {
		for off, c := range page {
			dst[off] = Tile{Terrain: c.Terrain, Composition: c.Composition, Explored: c.Explored}
		}
		// A page's cells past the map edge are never written, so they are
		// already the zero Rock; nothing to clip.
		return true
	}
	if !s.PageKnown(pi) {
		return false
	}
	for off := range dst {
		p := Point{origin.X + off&gridPageMask, origin.Y + off>>gridPageBits}
		switch {
		case p.X >= s.Width || p.Y >= s.Height:
			dst[off] = Tile{Terrain: Rock}
		case s.preview != nil:
			dst[off] = s.preview.At(p)
		default:
			dst[off] = Tile{Terrain: Rock}
		}
	}
	return true
}

// PageKnown reports whether ReadPage(pi) has anything to show, without
// reading it: the page's chunk is generated, or the fog is off.
func (s *Snapshot) PageKnown(pi int) bool {
	g := s.Tiles
	return !s.FogOfWar || pi >= 0 && pi < len(g.pages) && g.pages[pi] != nil
}

// RefuseTile is one tile with refuse on it: a violent death's gore, bodies,
// or both. See Tile.Gore and Tile.Corpses.
type RefuseTile struct {
	Pos     Point
	Gore    uint8
	Corpses uint16
}

// RefuseTiles lists every tile with refuse on it, sorted by row then column.
// It is a few hundred entries in a long game, and TileChanges.Refuse says
// when it has changed.
func (g *TileGrid) RefuseTiles() []RefuseTile {
	out := make([]RefuseTile, 0, len(g.refuse))
	for p, r := range g.refuse {
		out = append(out, RefuseTile{Pos: p, Gore: r.Gore, Corpses: uint16(min(r.total(), 0xFFFF))})
	}
	sort.Slice(out, func(i, j int) bool { return lessPoint(out[i].Pos, out[j].Pos) })
	return out
}
