package sim

import "math/rand/v2"

// newPCG returns the PCG source for a seed. Every stream in the sim is a PCG
// because, unlike math/rand v1's sources, its state is exported through
// MarshalBinary, which is what lets a save capture the RNG mid-game.
//
// PCG takes 128 bits of seed; both halves are derived from the int64 with
// splitmix64 so neighbouring seeds (1, 2, 3 ...) and the small XOR constants
// that separate streams still start from well-mixed, unrelated states.
func newPCG(seed int64) *rand.PCG {
	s := uint64(seed)
	return rand.NewPCG(splitmix64(&s), splitmix64(&s))
}

// newRand is newPCG wrapped for drawing, for a stream nothing needs to save
// (a worldgen-only stream, or a test's).
func newRand(seed int64) *rand.Rand { return rand.New(newPCG(seed)) }

func splitmix64(s *uint64) uint64 {
	*s += 0x9E3779B97F4A7C15
	z := *s
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// rngSources holds the PCG behind each stream that lives past world
// generation. A rand.Rand does not expose its source, so World keeps these
// alongside the *rand.Rand it draws from; restoring a source's state
// rewinds its Rand with it, since v2's Rand carries no state of its own.
// Worldgen-only streams (composition, caverns, alien lore) are spent before
// the first tick and are not here.
type rngSources struct {
	sim, personality, age, nest *rand.PCG
}

// rngState is the saved form of rngSources: each stream's MarshalBinary
// output, nil for a stream the world never created.
type rngState struct {
	Sim, Personality, Age, Nest []byte
}

func (w *World) saveRNG() (rngState, error) {
	var st rngState
	for _, s := range w.rngFields(&st) {
		if s.src == nil {
			continue
		}
		b, err := s.src.MarshalBinary()
		if err != nil {
			return rngState{}, err
		}
		*s.buf = b
	}
	return st, nil
}

// loadRNG restores every stream saveRNG captured. A stream present in the
// state but missing from the world (or the reverse) is left alone; the loader
// that builds the World decides which streams exist.
func (w *World) loadRNG(st rngState) error {
	for _, s := range w.rngFields(&st) {
		if s.src == nil || *s.buf == nil {
			continue
		}
		if err := s.src.UnmarshalBinary(*s.buf); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) rngFields(st *rngState) []struct {
	src *rand.PCG
	buf *[]byte
} {
	return []struct {
		src *rand.PCG
		buf *[]byte
	}{
		{w.rngSrc.sim, &st.Sim},
		{w.rngSrc.personality, &st.Personality},
		{w.rngSrc.age, &st.Age},
		{w.rngSrc.nest, &st.Nest},
	}
}
