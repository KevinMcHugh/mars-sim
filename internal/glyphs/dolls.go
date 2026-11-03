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

// DollKey is the doll e draws as, if it draws as one: any colonist with a
// profile, whatever they are doing. A state ForEntity would draw in place of
// the figure (fleeing, talking, aiding…) is drawn as a badge over the doll
// instead: StateBadge.
func DollKey(e sim.EntityView) (int, bool) {
	if e.Kind != sim.Colonist {
		return 0, false
	}
	return profileDoll(e.Profile, e.MaxParts[:])
}

// Pick is the mining badge, ⛏️. It is a badge only: the terminal draws a
// miner as their own figure, and ⛏ is not in All because terminals disagree
// on its width (it defaults to text presentation; the emoji form needs VS16,
// which All never carries). See docs/terminal-cell-widths.md.
const Pick = "\u26CF\uFE0F"

// badgeOnly are badges for states the terminal draws as the colonist's
// figure: on a doll there is room to say what they are doing.
var badgeOnly = map[sim.State]string{
	sim.Mining: Pick,
}

// StateBadge is the glyph a colonist in state s wears as a badge over their
// doll, or "" for a state that shows no glyph: the glyph ForEntity draws a
// colonist in that state as, when that is not the colonist's own figure, or
// a badge-only glyph (badgeOnly) for a state it draws as the figure.
func StateBadge(s sim.State) string {
	if g, ok := badgeOnly[s]; ok {
		return g
	}
	p := &sim.Profile{}
	g := ForEntity(sim.EntityView{Kind: sim.Colonist, State: s, Profile: p})
	if g == ForColonist(p) {
		return ""
	}
	return g
}

// RestingDollKey is the doll a colonist draws as at rest, whatever they are
// doing now: the roster's figure, as ForColonist is the roster's glyph.
func RestingDollKey(e sim.EntityView) (int, bool) {
	if e.Kind != sim.Colonist {
		return 0, false
	}
	return profileDoll(e.Profile, e.MaxParts[:])
}

// DollFallbacks is, for each body (a key with its parts digit dropped: key >>
// len(DollParts)), the glyph index the map drew it as before dolls: the look
// for its figure, tone and hair, past the end of All. A doll with grown parts
// falls back to Mutant instead (wire's HelloDolls.Fallback). A function, not a
// table built at init, because Looks is built by looks.go's init, which runs
// after this file's.
func DollFallbacks() []int {
	out := make([]int, 0, NumDolls>>len(DollParts))
	for _, f := range DollFigures {
		for tone := sim.SkinTone(0); int(tone) < DollTones; tone++ {
			for hair := sim.HairColor(0); int(hair) < DollHairs; hair++ {
				out = append(out, len(All)+lookIndex[lookKey{figure: f, tone: tone, hair: foldHair(f, hair)}])
			}
		}
	}
	return out
}

// MarkParts are the body parts a doll's marks describe, in bit order: two
// bits each, after the blood tier's two (DollMarks).
var MarkParts = []sim.BodyPart{
	sim.Head, sim.Torso, sim.LeftArm, sim.RightArm, sim.LeftLeg, sim.RightLeg,
	sim.ThirdArm, sim.ExtraEye, sim.Tail, sim.VestigialTwin,
}

// A part's state in a doll's marks.
const (
	MarkWhole   = 0 // unhurt, or healed
	MarkOpen    = 1 // an open wound
	MarkDressed = 2 // a dressed wound
	MarkLost    = 3 // destroyed: a part the body had and has no more
)

// DollMarks is what has happened to e's body, for its doll to show: the
// blood tier on its clothes (sim.BloodTier) in bits 0-1, then two bits per
// part of MarkParts. Zero for anything that is not a colonist, and for a
// clean, unhurt one. It rides beside the doll's key rather than in it: blood
// and wounds come and go every few minutes of play, and folding them into the
// key would multiply it past a frame's uint16.
func DollMarks(e sim.EntityView) uint32 {
	if e.Kind != sim.Colonist {
		return 0
	}
	m := uint32(sim.BloodTier(e.Blood))
	for i, p := range MarkParts {
		var s uint32
		switch {
		case e.MaxParts[p] > 0 && e.Parts[p] <= 0:
			s = MarkLost
		case e.Wounds[p] == sim.WoundOpen:
			s = MarkOpen
		case e.Wounds[p] == sim.WoundDressed:
			s = MarkDressed
		}
		m |= s << (2 + 2*i)
	}
	return m
}
