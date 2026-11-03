package glyphs

import "github.com/kevinmchugh/mars-sim/internal/sim"

// Dolls: a colonist drawn as a paper doll, a figure composed in the browser
// from layered SVG parts in their own skin and hair, with whatever uranium
// grew on them (see docs/body-sprites.md).
//
// A doll is a key, not a picture. The key is a pure function of four small
// enums, laid out mixed-radix, so every doll a game could need is enumerated
// up front and a frame can name one with its ordinary uint16 glyph index: no
// table grows mid-game, and a colonist who mutates just starts sending a
// different key. The page holds the art and composes a key's figure the first
// time a frame names it.

// DollFigures are the figure digit's values, in order: the plain figures
// ForColonist draws a colonist at rest as.
var DollFigures = []string{ManAdult, WomanAdult, PersonAdult, ManSenior, WomanSenior, PersonSenior}

// DollTones is the skin tone digit's size: sim's five-point scale, in order.
const DollTones = int(sim.SkinDark) + 1

// DollHairs is the hair digit's size: every sim.HairColor, in order. Unlike
// looks, a doll folds no hair away; seniors keep theirs.
const DollHairs = int(sim.HairBald) + 1

// DollParts are the parts digit's bits, in order: bit i is set when the
// colonist has grown DollParts[i].
var DollParts = []sim.BodyPart{sim.ThirdArm, sim.ExtraEye, sim.Tail, sim.VestigialTwin}

// NumDolls is how many keys there are: 0 to NumDolls-1.
var NumDolls = len(DollFigures) * DollTones * DollHairs * (1 << len(DollParts))

var dollFigure = func() map[string]int {
	m := map[string]int{}
	for i, f := range DollFigures {
		m[f] = i
	}
	return m
}()

// dollKey packs the digits, figure most significant, parts least.
func dollKey(figure int, tone sim.SkinTone, hair sim.HairColor, parts int) int {
	return ((figure*DollTones+int(tone))*DollHairs+int(hair))<<len(DollParts) | parts
}

// grownParts is the parts digit for a body: a bit for each mutant part it has
// (MaxParts above zero, so a destroyed part still shows: it was grown).
func grownParts(maxParts []int) int {
	bits := 0
	for i, p := range DollParts {
		if int(p) < len(maxParts) && maxParts[p] > 0 {
			bits |= 1 << i
		}
	}
	return bits
}

// profileDoll is the key for a profile and a body, or false when the profile
// has none to draw: no profile, or a tone or hair outside the enums.
func profileDoll(p *sim.Profile, maxParts []int) (int, bool) {
	if p == nil || int(p.SkinTone) >= DollTones || int(p.HairColor) >= DollHairs {
		return 0, false
	}
	return dollKey(dollFigure[figureFor(p)], p.SkinTone, p.HairColor, grownParts(maxParts)), true
}

// DollKey is the doll e draws as, if it draws as one: a mutant colonist at
// rest, whom ForEntity would otherwise draw as Mutant. A colonist drawn by
// their state (fleeing, talking, aiding…) keeps the state's glyph.
func DollKey(e sim.EntityView) (int, bool) {
	if e.Kind != sim.Colonist || ForEntity(e) != Mutant {
		return 0, false
	}
	return profileDoll(e.Profile, e.MaxParts[:])
}

// RestingDollKey is the doll a colonist draws as at rest, whatever they are
// doing now: the roster's figure, as ForColonist is the roster's glyph.
func RestingDollKey(e sim.EntityView) (int, bool) {
	if e.Kind != sim.Colonist || ForColonist(e.Profile) != Mutant {
		return 0, false
	}
	return profileDoll(e.Profile, e.MaxParts[:])
}
