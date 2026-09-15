package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

const tileWidth = 2

// Glyphs occupy two terminal cells so the map grid stays aligned. Open floor is
// two spaces (also two cells), which reads as empty cavern against the solid
// terrain. fitGlyph adds padding for terminals whose width table reports an
// emoji as narrow and uses a plain two-cell fallback for an unexpectedly wide
// glyph.
const (
	glyphRock   = "\U0001F7EB"       // 🟫 unexcavated regolith
	glyphFloor  = "  "               // open, walkable space
	glyphWall   = "\U0001F9F1"       // 🧱 built wall
	glyphPod    = "\U0001F37D\uFE0F" // 🍽️ nutrient pod (food)
	glyphToilet = "\U0001F6BD"       // 🚽 toilet (bladder)
	glyphBed    = "\U0001F6CF\uFE0F" // 🛏️ dormitory bunk (sleep)

	glyphColonist = "\U0001F477"       // 👷 colonist of unknown age/gender (no profile)
	glyphFleeing  = "\U0001F631"       // 😱 colonist running from an alien
	glyphTalking  = "\U0001F5E3\uFE0F" // 🗣️ colonist chatting with another
	glyphAlien    = "\U0001F47D"       // 👽 subterranean mutant
	glyphCat      = "\U0001F408"       // 🐈 floor predator hunting mice
	glyphMouse    = "\U0001F401"       // 🐁 pest that raids the food pods
	glyphStomp    = "\U0001F97E"       // 🥾 colonist chasing down a mouse to stomp it

	glyphManAdult     = "\U0001F468" // 👨 adult man colonist
	glyphWomanAdult   = "\U0001F469" // 👩 adult woman colonist
	glyphPersonAdult  = "\U0001F9D1" // 🧑 adult non-binary colonist
	glyphManSenior    = "\U0001F474" // 👴 senior man colonist
	glyphWomanSenior  = "\U0001F475" // 👵 senior woman colonist
	glyphPersonSenior = "\U0001F9D3" // 🧓 senior non-binary colonist
)

// seniorAge is the age at which a colonist's default glyph switches from an
// adult to a senior variant.
const seniorAge = 60

var fittedGlyphs = func() map[string]string {
	out := make(map[string]string, 19)
	for _, glyph := range []string{
		glyphRock, glyphFloor, glyphWall, glyphPod, glyphToilet, glyphBed,
		glyphColonist, glyphFleeing, glyphTalking, glyphAlien, glyphCat,
		glyphMouse, glyphStomp,
		glyphManAdult, glyphWomanAdult, glyphPersonAdult,
		glyphManSenior, glyphWomanSenior, glyphPersonSenior,
	} {
		out[glyph] = fitGlyphMeasured(glyph)
	}
	return out
}()

// colonistGlyph picks the default map glyph for a colonist at rest, based on
// their gender identity and whether they've reached seniorAge. A colonist
// without a profile falls back to glyphColonist.
func colonistGlyph(p *sim.Profile) string {
	if p == nil {
		return glyphColonist
	}
	senior := p.Age >= seniorAge
	switch p.Gender {
	case sim.GenderMan:
		if senior {
			return glyphManSenior
		}
		return glyphManAdult
	case sim.GenderWoman:
		if senior {
			return glyphWomanSenior
		}
		return glyphWomanAdult
	default:
		if senior {
			return glyphPersonSenior
		}
		return glyphPersonAdult
	}
}

func terrainGlyph(t sim.Terrain) string {
	var glyph string
	switch t {
	case sim.Floor:
		glyph = glyphFloor
	case sim.Wall:
		glyph = glyphWall
	case sim.NutrientPod:
		glyph = glyphPod
	case sim.Toilet:
		glyph = glyphToilet
	case sim.Bed:
		glyph = glyphBed
	default:
		glyph = glyphRock
	}
	return fitGlyph(glyph)
}

func entityGlyph(e sim.EntityView) string {
	var glyph string
	switch e.Kind {
	case sim.Alien:
		glyph = glyphAlien
	case sim.Cat:
		glyph = glyphCat
	case sim.Mouse:
		glyph = glyphMouse
	case sim.Colonist:
		switch e.State {
		case sim.Fleeing:
			glyph = glyphFleeing
		case sim.Talking:
			glyph = glyphTalking
		case sim.Stomping:
			glyph = glyphStomp
		default:
			glyph = colonistGlyph(e.Profile)
		}
	default:
		glyph = glyphColonist
	}
	return fitGlyph(glyph)
}

func fitGlyph(glyph string) string {
	if fitted, ok := fittedGlyphs[glyph]; ok {
		return fitted
	}
	return fitGlyphMeasured(glyph)
}

func fitGlyphMeasured(glyph string) string {
	switch width := lipgloss.Width(glyph); {
	case width == tileWidth:
		return glyph
	case width < tileWidth:
		return glyph + strings.Repeat(" ", tileWidth-width)
	default:
		return "??"
	}
}
