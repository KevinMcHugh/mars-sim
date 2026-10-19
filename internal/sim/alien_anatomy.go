package sim

import (
	"fmt"
	"math/rand/v2"
)

// Graded anatomy: Earth-inspired features an alien species has in degrees,
// so a lifecycle can grow along them ("one little horn, then three, then a
// crown of nine"). Every species rolls them, single-form or not; most get
// one or two, some none. They are description only for now: nothing in
// combat reads them yet (see docs/alien-lifecycles.md).
//
// They roll on their own stream (alienAnatomySeed), after the roster and its
// taxonomy, so adding them re-rolled no species' build, name or temperament.
// Tusks and mouthparts came later and roll on a stream of their own again
// (alienHeadSeed), so adding them moved no existing horn, shell or claw.

// alienAnatomySeed separates the anatomy stream from the other lore streams.
const alienAnatomySeed = 0x2B7E151628AED2A6

// alienHeadSeed separates the tusk-and-mouthpart stream from the anatomy
// stream, so adding them re-rolled no other feature.
const alienHeadSeed = 0x5BE0CD19137E2179

// TailTip is what a tail ends in.
type TailTip uint8

const (
	TailPlain      TailTip = iota
	TailClub               // a bony club
	TailSpikedClub         // a club studded with spikes
	TailStinger            // a stinger at the tip
)

// String is the tail tip's word in a name condition: plain, club,
// spiked-club, stinger.
func (t TailTip) String() string {
	switch t {
	case TailClub:
		return "club"
	case TailSpikedClub:
		return "spiked-club"
	case TailStinger:
		return "stinger"
	default:
		return "plain"
	}
}

// Mouthpart is what a species has in place of a plain mouth.
type Mouthpart uint8

const (
	MouthPlain     Mouthpart = iota
	MouthBeak                // a hooked, tearing beak
	MouthBill                // a flat, ducklike bill: the platypus
	MouthMandibles           // a pair of side-closing insect jaws
	MouthProboscis           // a long sucking tube
)

// mouthparts is every non-plain mouthpart, in roll order.
var mouthparts = [...]Mouthpart{MouthBeak, MouthBill, MouthMandibles, MouthProboscis}

// String is the mouthpart's word in a name condition: plain, beak, bill,
// mandibles, proboscis.
func (m Mouthpart) String() string {
	switch m {
	case MouthBeak:
		return "beak"
	case MouthBill:
		return "bill"
	case MouthMandibles:
		return "mandibles"
	case MouthProboscis:
		return "proboscis"
	default:
		return "plain"
	}
}

// AlienAnatomy is a body's graded features. Zero is absent for each.
type AlienAnatomy struct {
	Horns   int // count: 1 nub ... a crown of a dozen
	Antlers int // tines across both antlers
	Spines  int // 1 a few quills, 2 a ridge of spines, 3 a coat of quills
	Shell   int // 1 a shell patch, 2 plates, 3 a full carapace
	Claws   int // 1 blunt nubs, 2 hooked claws, 3 scythes
	Stinger int // 1 a stinger, 2 a barbed stinger (on the body, not the tail)
	TailTip TailTip
	Tusks   int       // 1 stubby tusks, 2 a pair of tusks, 3 great curving tusks
	Mouth   Mouthpart // a beak, a bill, mandibles or a proboscis, or plain
}

// any reports whether the body has any graded feature at all.
func (a AlienAnatomy) any() bool { return a != AlienAnatomy{} }

// armed reports whether the body has a feature that could make it deadly:
// anything but a harmless mouthpart (a bill, a proboscis) alone. The apex
// roll needs one. (Spines count, as they did before mouthparts existed.)
func (a AlienAnatomy) armed() bool {
	if a.Mouth == MouthBill || a.Mouth == MouthProboscis {
		a.Mouth = MouthPlain
	}
	return a.any()
}

// features are the most a fully grown body can carry.
const (
	maxHorns   = 12
	maxAntlers = 14
)

// rollAnatomy gives a species its adult features: each feature is rare on
// its own, so most species carry one or two and some carry none. A tail tip
// needs a tail.
func rollAnatomy(rng *rand.Rand, sp AlienSpecies) AlienAnatomy {
	var a AlienAnatomy
	if rng.IntN(100) < 22 {
		a.Horns = 1 + rng.IntN(maxHorns)
	}
	if rng.IntN(100) < 10 {
		a.Antlers = 2 + rng.IntN(maxAntlers-1)
	}
	if rng.IntN(100) < 18 {
		a.Spines = 1 + rng.IntN(3)
	}
	if rng.IntN(100) < 20 {
		a.Shell = 1 + rng.IntN(3)
	}
	if rng.IntN(100) < 25 {
		a.Claws = 1 + rng.IntN(3)
	}
	if rng.IntN(100) < 12 {
		a.Stinger = 1 + rng.IntN(2)
	}
	if sp.Tail && rng.IntN(100) < 30 {
		a.TailTip = TailTip(1 + rng.IntN(3))
	}
	return a
}

// rollHeadFeatures gives a species its tusks and mouthpart, on their own
// stream (alienHeadSeed): each is rare, like the other features. A
// mouthpart is a whole mouth, so a species has at most one; tusks can sit
// beside any of them (a tusked, billed thing is the platypus tone).
func rollHeadFeatures(rng *rand.Rand, a AlienAnatomy) AlienAnatomy {
	if rng.IntN(100) < 12 {
		a.Tusks = 1 + rng.IntN(3)
	}
	if rng.IntN(100) < 20 {
		a.Mouth = mouthparts[rng.IntN(len(mouthparts))]
	}
	return a
}

// scaledAnatomy is the adult's features at a fraction of the way through its
// growth (num/den, 0 < num/den <= 1): each graded feature shrinks with it and
// may not have appeared yet, but never exceeds the adult's, so every stage
// is a lesser version of the next. A feature that has appeared is at least
// a single nub of itself.
func scaledAnatomy(adult AlienAnatomy, num, den int) AlienAnatomy {
	scale := func(v int) int {
		if v == 0 {
			return 0
		}
		s := v * num / den // rounds down: early stages come in under the adult
		if s == 0 && num*2 >= den {
			s = 1 // past halfway, a feature has at least begun
		}
		return s
	}
	a := AlienAnatomy{
		Horns:   scale(adult.Horns),
		Antlers: scale(adult.Antlers),
		Spines:  scale(adult.Spines),
		Shell:   scale(adult.Shell),
		Claws:   scale(adult.Claws),
		Stinger: scale(adult.Stinger),
		Tusks:   scale(adult.Tusks),
		Mouth:   adult.Mouth, // a mouth is there from the first form that eats
	}
	if adult.TailTip != TailPlain && num*3 >= den*2 {
		a.TailTip = adult.TailTip // a tail's ornament comes late
	}
	return a
}

// featurePhrases names a body's graded features for a description, in a
// fixed order: "a crown of 9 horns", "hooked claws", "a barbed stinger".
func (a AlienAnatomy) featurePhrases() []string {
	var out []string
	switch {
	case a.Horns == 1:
		out = append(out, "a single nub of a horn")
	case a.Horns >= 2 && a.Horns <= 3:
		out = append(out, fmt.Sprintf("%d horns", a.Horns))
	case a.Horns >= 4 && a.Horns <= 7:
		out = append(out, fmt.Sprintf("a cluster of %d horns", a.Horns))
	case a.Horns >= 8:
		out = append(out, fmt.Sprintf("a crown of %d horns", a.Horns))
	}
	switch {
	case a.Antlers == 1:
		out = append(out, "a budding antler")
	case a.Antlers >= 2 && a.Antlers <= 5:
		out = append(out, fmt.Sprintf("small antlers of %d tines", a.Antlers))
	case a.Antlers >= 6:
		out = append(out, fmt.Sprintf("branching antlers of %d tines", a.Antlers))
	}
	switch a.Tusks {
	case 1:
		out = append(out, "stubby tusks")
	case 2:
		out = append(out, "a pair of tusks")
	case 3:
		out = append(out, "great curving tusks")
	}
	switch a.Mouth {
	case MouthBeak:
		out = append(out, "a hooked beak")
	case MouthBill:
		out = append(out, "a flat, ducklike bill")
	case MouthMandibles:
		out = append(out, "clacking mandibles")
	case MouthProboscis:
		out = append(out, "a long proboscis")
	}
	switch a.Spines {
	case 1:
		out = append(out, "a few quills")
	case 2:
		out = append(out, "a ridge of spines")
	case 3:
		out = append(out, "a coat of quills")
	}
	switch a.Shell {
	case 1:
		out = append(out, "a patch of shell")
	case 2:
		out = append(out, "overlapping shell plates")
	case 3:
		out = append(out, "a full carapace")
	}
	switch a.Claws {
	case 1:
		out = append(out, "blunt claw nubs")
	case 2:
		out = append(out, "hooked claws")
	case 3:
		out = append(out, "scythe-like claws")
	}
	switch a.Stinger {
	case 1:
		out = append(out, "a stinger")
	case 2:
		out = append(out, "a barbed stinger")
	}
	switch a.TailTip {
	case TailClub:
		out = append(out, "a clubbed tail")
	case TailSpikedClub:
		out = append(out, "a spiked tail club")
	case TailStinger:
		out = append(out, "a tail ending in a stinger")
	}
	return out
}
