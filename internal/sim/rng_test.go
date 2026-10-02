package sim

import (
	"math/rand/v2"
	"testing"
)

// Saving the RNG mid-game and loading it back rewinds every persistent
// stream: the draws after a load repeat the draws after the save.
func TestRNGStateRoundTrips(t *testing.T) {
	w := newTestWorld(t, testConfig())
	for i := 0; i < 50; i++ {
		w.step()
	}

	st, err := w.saveRNG()
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"sim": st.Sim, "personality": st.Personality, "age": st.Age, "nest": st.Nest, "skill": st.Skill, "topic": st.Topic} {
		if b == nil {
			t.Fatalf("%s stream was not saved", name)
		}
	}

	streams := []*rand.Rand{w.rng, w.prng, w.agePRNG, w.nestRNG, w.skillRNG, w.topicRNG}
	draw := func() []uint64 {
		var out []uint64
		for _, r := range streams {
			for i := 0; i < 8; i++ {
				out = append(out, r.Uint64())
			}
		}
		return out
	}
	before := draw()
	if err := w.loadRNG(st); err != nil {
		t.Fatal(err)
	}
	after := draw()
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("draw %d after load = %d, want %d", i, after[i], before[i])
		}
	}
}

// Every stream is seeded from its own derivation of the seed, so no two
// persistent streams start in the same place.
func TestRNGStreamsAreDistinct(t *testing.T) {
	w := newTestWorld(t, testConfig())
	seen := map[uint64]string{}
	for name, r := range map[string]*rand.Rand{"sim": w.rng, "personality": w.prng, "age": w.agePRNG, "nest": w.nestRNG, "skill": w.skillRNG, "topic": w.topicRNG} {
		v := r.Uint64()
		if other, ok := seen[v]; ok {
			t.Fatalf("%s and %s streams produced the same first draw", name, other)
		}
		seen[v] = name
	}
}
