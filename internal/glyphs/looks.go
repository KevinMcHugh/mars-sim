package glyphs

import "github.com/kevinmchugh/mars-sim/internal/sim"

// Looks: a colonist's figure in their own skin tone and hair, for the browser
// only (see docs/colonist-looks.md).
//
// Everything in All is a single code point because terminals only agree about
// the width of those (internal/ui/tui/glyphs.go). A browser has no grid to
// shear, but it has the same underlying problem in another form: an emoji
// font that does not know a sequence draws its parts side by side, so
// 👨🏿‍🦰 on an older font is 👨🏿🦰, or 👨⬛🦰. Which sequences fuse depends
// on the font, so only the page can tell. A Look is therefore not a glyph but
// a list of candidates, best first, ending in the plain figure from All that
// every emoji font has; the page draws the first one that measures as a
// single glyph.

// Unicode pieces a look is built from.
const (
	zwj  = "\u200D"
	vs16 = "\uFE0F"

	hairRed   = "\U0001F9B0" // 🦰
	hairWhite = "\U0001F9B3" // 🦳
	hairBald  = "\U0001F9B2" // 🦲

	blond  = "\U0001F471" // 👱 person with blond hair: blond is a base, not a component
	male   = "\u2642"     // ♂
	female = "\u2640"     // ♀
)

// skinModifier is the Fitzpatrick modifier for a tone; sim's five-point scale
// is the emoji one, in the same order.
func skinModifier(t sim.SkinTone) string {
	if t > sim.SkinDark {
		return ""
	}
	return string(rune(0x1F3FB + int(t)))
}

// Look is the candidate list for one figure, best first. The last candidate is
// always the plain figure from All.
type Look []string

// lookKey is everything a look depends on. Hair is folded to what emoji can
// show: black and brown are the figure's default hair, so both fold to black.
type lookKey struct {
	figure string // the plain figure from ForColonist
	tone   sim.SkinTone
	hair   sim.HairColor
}

// seniors have no hair components in Unicode, only skin tones.
func senior(figure string) bool {
	return figure == ManSenior || figure == WomanSenior || figure == PersonSenior
}

func foldHair(figure string, h sim.HairColor) sim.HairColor {
	if senior(figure) {
		return sim.HairBlack
	}
	switch h {
	case sim.HairRed, sim.HairWhite, sim.HairBald, sim.HairBlonde:
		return h
	}
	return sim.HairBlack
}

// build lists the candidates for k: tone and hair together, then tone alone,
// then hair alone, then the plain figure. Tone outranks hair when only one
// fuses because it is the one the colonist's flavor text leads with.
func (k lookKey) build() Look {
	tone := skinModifier(k.tone)
	withHair := func(tone string) string {
		switch k.hair {
		case sim.HairRed:
			return k.figure + tone + zwj + hairRed
		case sim.HairWhite:
			return k.figure + tone + zwj + hairWhite
		case sim.HairBald:
			return k.figure + tone + zwj + hairBald
		case sim.HairBlonde:
			// Blond is its own figure: 👱 for a person, 👱‍♂️ / 👱‍♀️ gendered.
			switch k.figure {
			case ManAdult:
				return blond + tone + zwj + male + vs16
			case WomanAdult:
				return blond + tone + zwj + female + vs16
			default:
				return blond + tone
			}
		}
		return ""
	}
	var l Look
	add := func(s string) {
		if s == "" {
			return
		}
		for _, have := range l {
			if have == s {
				return
			}
		}
		l = append(l, s)
	}
	add(withHair(tone))
	add(k.figure + tone)
	add(withHair(""))
	add(k.figure)
	return l
}

// Looks lists every look a colonist can have, once each, in a fixed order:
// the browser's map indexes past the end of All into it (see LookIndex).
var Looks []Look

var lookIndex = map[lookKey]int{}

func init() {
	figures := []string{ManAdult, WomanAdult, PersonAdult, ManSenior, WomanSenior, PersonSenior}
	hairs := []sim.HairColor{sim.HairBlack, sim.HairBlonde, sim.HairRed, sim.HairWhite, sim.HairBald}
	for _, f := range figures {
		for t := sim.SkinLight; t <= sim.SkinDark; t++ {
			for _, h := range hairs {
				k := lookKey{f, t, foldHair(f, h)}
				if _, ok := lookIndex[k]; ok {
					continue
				}
				lookIndex[k] = len(Looks)
				Looks = append(Looks, k.build())
			}
		}
	}
}

// lookKeyFor is the look a colonist's resting figure takes, if it has one: a
// colonist with a profile who is not a mutant (a mutant's figure is 🧟, which
// has no tones).
func lookKeyFor(p *sim.Profile) (lookKey, bool) {
	figure := ForColonist(p)
	if p == nil || figure == Mutant {
		return lookKey{}, false
	}
	return lookKey{figure, p.SkinTone, foldHair(figure, p.HairColor)}, true
}

// ForColonistLook is ForColonist with the colonist's own skin and hair: the
// candidates the page picks from. Nil when the figure has no look.
func ForColonistLook(p *sim.Profile) Look {
	if k, ok := lookKeyFor(p); ok {
		return Looks[lookIndex[k]]
	}
	return nil
}

// entityLookKey is e's look when ForEntity draws it as its resting figure, not
// a state glyph or another creature.
func entityLookKey(e sim.EntityView) (lookKey, bool) {
	if e.Kind != sim.Colonist || ForEntity(e) != ForColonist(e.Profile) {
		return lookKey{}, false
	}
	return lookKeyFor(e.Profile)
}

// ForEntityLook is ForEntity's look: the colonist's own figure when ForEntity
// draws them at rest, nil when it draws something else.
func ForEntityLook(e sim.EntityView) Look {
	if k, ok := entityLookKey(e); ok {
		return Looks[lookIndex[k]]
	}
	return nil
}

// LookIndex is the glyph index a frame carries for e in a frontend that draws
// looks: len(All)+i for Looks[i]. ok is false when e has no look, and the
// frame carries ForEntity's index in All instead.
func LookIndex(e sim.EntityView) (index int, ok bool) {
	k, ok := entityLookKey(e)
	if !ok {
		return 0, false
	}
	return len(All) + lookIndex[k], true
}
