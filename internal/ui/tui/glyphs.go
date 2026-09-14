package tui

import "github.com/kevinmchugh/mars-sim/internal/sim"

const tileWidth = 2

// Map glyphs use only ASCII characters because emoji advance widths vary by
// terminal and font. Every token is exactly two ordinary terminal cells wide;
// open floor is two spaces, which reads as empty cavern against the solid
// terrain.
const (
	glyphRock   = "##" // unexcavated regolith
	glyphFloor  = "  " // open, walkable space
	glyphWall   = "[]" // built wall
	glyphPod    = "P " // nutrient pod (food)
	glyphToilet = "T " // toilet (bladder)
	glyphBed    = "B " // dormitory bunk (sleep)

	glyphColonist = "C " // colonist at work
	glyphFleeing  = "! " // colonist running from an alien
	glyphTalking  = "S " // colonist chatting with another
	glyphAlien    = "A " // subterranean mutant
	glyphCat      = "K " // floor predator hunting mice
	glyphMouse    = "M " // pest that raids the food pods
	glyphStomp    = "^ " // colonist chasing down a mouse to stomp it
)

func terrainGlyph(t sim.Terrain) string {
	switch t {
	case sim.Floor:
		return glyphFloor
	case sim.Wall:
		return glyphWall
	case sim.NutrientPod:
		return glyphPod
	case sim.Toilet:
		return glyphToilet
	case sim.Bed:
		return glyphBed
	default:
		return glyphRock
	}
}

func entityGlyph(e sim.EntityView) string {
	switch e.Kind {
	case sim.Alien:
		return glyphAlien
	case sim.Cat:
		return glyphCat
	case sim.Mouse:
		return glyphMouse
	case sim.Colonist:
		switch e.State {
		case sim.Fleeing:
			return glyphFleeing
		case sim.Talking:
			return glyphTalking
		case sim.Stomping:
			return glyphStomp
		}
		return glyphColonist
	default:
		return glyphColonist
	}
}
