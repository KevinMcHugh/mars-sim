package sim

import (
	"slices"
	"testing"
)

// ReadPage must read every tile exactly as TileAt does (refuse aside): a
// generated page, a previewed one with the fog off, and a page hanging off
// the map's edge.
func TestReadPageMatchesTileAt(t *testing.T) {
	for _, fog := range []bool{true, false} {
		cfg := DefaultConfig()
		cfg.Seed = 7
		cfg.Width, cfg.Height = 900, 700 // big enough to leave chunks ungenerated; neither a multiple of TilePageSide
		cfg.FogOfWar = fog
		snap := NewEngine(cfg).world.snapshot(false, 8)

		dst := make([]Tile, TilePageSide*TilePageSide)
		read, skipped := 0, 0
		for py := 0; py*TilePageSide < cfg.Height; py++ {
			for px := 0; px*TilePageSide < cfg.Width; px++ {
				origin := Point{px * TilePageSide, py * TilePageSide}
				pi := snap.Tiles.PageIndex(origin)
				if got := snap.Tiles.PageOrigin(pi); got != origin {
					t.Fatalf("PageOrigin(PageIndex(%v)) = %v", origin, got)
				}
				if !snap.ReadPage(pi, dst) {
					skipped++
					if !fog || snap.Tiles.hasPage(origin) {
						t.Fatalf("fog %v: page at %v skipped, want it read", fog, origin)
					}
					continue
				}
				read++
				for off, got := range dst {
					p := Point{origin.X + off%TilePageSide, origin.Y + off/TilePageSide}
					want := Tile{Terrain: Rock}
					if p.X < cfg.Width && p.Y < cfg.Height {
						want = snap.TileAt(p)
						want.Gore, want.Corpses = 0, 0
					}
					if got != want {
						t.Fatalf("fog %v: tile %v reads %+v, TileAt %+v", fog, p, got, want)
					}
				}
			}
		}
		if read == 0 || fog && skipped == 0 || !fog && skipped != 0 {
			t.Errorf("fog %v: read %d pages, skipped %d", fog, read, skipped)
		}
	}
}

func TestRefuseTilesListsRefuseInRowOrder(t *testing.T) {
	w := gridWorld(t, 200)
	w.setRefuse(Point{50, 60}, refuseCell{Gore: 2})
	w.setRefuse(Point{10, 60}, refuseCell{Corpses: [numCorpseKinds]uint16{1, 2}})
	w.setRefuse(Point{90, 5}, refuseCell{Gore: 1, Corpses: [numCorpseKinds]uint16{0, 0, 4}})
	got := w.snapshot(false, 8).Tiles.RefuseTiles()
	want := []RefuseTile{
		{Pos: Point{90, 5}, Gore: 1, Corpses: 4},
		{Pos: Point{10, 60}, Corpses: 3},
		{Pos: Point{50, 60}, Gore: 2},
	}
	if !slices.Equal(got, want) {
		t.Errorf("RefuseTiles = %+v, want %+v", got, want)
	}
}

// Every enum value has a real name: a value whose String falls through to its
// default ("unknown ...") means a keep-last count and its String disagree.
func TestEnumNamesAreComplete(t *testing.T) {
	e := EnumNames()
	for name, list := range map[string][]string{
		"terrains": e.Terrains, "compositions": e.Compositions, "kinds": e.Kinds,
		"states": e.States, "focuses": e.Focuses,
	} {
		if len(list) == 0 {
			t.Errorf("%s: empty", name)
		}
		for i, n := range list {
			if n == "" || n == "unknown" || n == "unknown rock" {
				t.Errorf("%s[%d] = %q", name, i, n)
			}
		}
	}
	if e.Terrains[Floor] != "floor" || e.Kinds[Colonist] != "colonist" {
		t.Errorf("names are not indexed by value: %v %v", e.Terrains, e.Kinds)
	}
}
