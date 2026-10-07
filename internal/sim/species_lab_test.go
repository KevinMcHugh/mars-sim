package sim

import "testing"

// A lab species is the species a real world with that seed rolls when it has
// one alien species and default settings: Creature Lab's catalog is only
// useful if its species are the game's.
func TestRollLabSpeciesIsTheWorldsSpecies(t *testing.T) {
	for _, seed := range []int64{1, 7, 42, 1234567} {
		cfg := DefaultConfig()
		cfg.Seed = seed
		cfg.AlienSpeciesCount = 1
		cfg.Width, cfg.Height = 40, 24
		w := newTestWorld(t, cfg)
		if got, want := RollLabSpecies(seed), w.alienSpecies[0]; got != want {
			t.Fatalf("seed %d: lab rolled %+v, the world rolled %+v", seed, got, want)
		}
	}
}
