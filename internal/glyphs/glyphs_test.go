package glyphs

import (
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// All is what frontends index into, so a duplicate would give one symbol two
// indexes; and every glyph a function here can return must be in it.
func TestAllIsASetOfEveryPickableGlyph(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range All {
		if seen[s] {
			t.Errorf("%+q is listed twice", s)
		}
		seen[s] = true
	}
	var picked []string
	for tr := sim.Terrain(0); tr < 64; tr++ {
		picked = append(picked, ForTerrain(tr))
	}
	for c := sim.RockComposition(0); c < 16; c++ {
		picked = append(picked, ForComposition(c))
	}
	for k := sim.Kind(0); k < 8; k++ {
		for st := sim.State(0); st < 64; st++ {
			picked = append(picked, ForEntity(sim.EntityView{Kind: k, State: st}))
		}
	}
	for _, g := range []sim.Gender{sim.GenderMan, sim.GenderWoman, sim.Gender(99)} {
		for _, age := range []int{20, seniorAge} {
			picked = append(picked, ForColonist(&sim.Profile{Gender: g, Age: age}))
		}
	}
	picked = append(picked, Gore, Corpse, Scum, Mars)
	for _, s := range picked {
		if !Known(s) {
			t.Errorf("%+q can be picked but is not in All", s)
		}
	}
	for _, s := range []string{Rock, IronRock, IceRock, Uranium, ClayRock, Floor, Hull} {
		if !Swatch(s) || !Known(s) {
			t.Errorf("%+q should be a listed swatch", s)
		}
	}
	if Swatch(Bed) || Swatch(Alien) {
		t.Error("a picture glyph reads as a swatch")
	}
}

func TestForTilePutsRefuseOverTerrain(t *testing.T) {
	cases := []struct {
		tile sim.Tile
		want string
	}{
		{sim.Tile{Terrain: sim.Rock, Composition: sim.WaterIceBearingRock}, IceRock},
		{sim.Tile{Terrain: sim.Bed}, Bed},
		{sim.Tile{Terrain: sim.Floor, Gore: 1}, Gore},
		{sim.Tile{Terrain: sim.Floor, Gore: 1, Corpses: 1}, Corpse},
	}
	for _, c := range cases {
		if got := ForTile(c.tile); got != c.want {
			t.Errorf("ForTile(%+v) = %+q, want %+q", c.tile, got, c.want)
		}
	}
}

func TestForAlienTrustsOnlyListedEmoji(t *testing.T) {
	if got := ForAlien(sim.AlienSpecies{Emoji: Beetle}); got != Beetle {
		t.Errorf("listed emoji: %+q", got)
	}
	if got := ForAlien(sim.AlienSpecies{Emoji: "\U0001F921"}); got != Alien {
		t.Errorf("unlisted emoji: %+q, want the generic alien", got)
	}
}
