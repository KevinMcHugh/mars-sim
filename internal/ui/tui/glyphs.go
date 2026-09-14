package tui

import "github.com/kevinmchugh/mars-sim/internal/sim"

// Every glyph is chosen to render two terminal cells wide so the grid stays
// aligned. Open floor is two spaces (also two cells), which reads as empty
// cavern against the solid terrain. Some terminals size emoji differently; if
// the grid ever looks sheared, that is the cause.
const (
	glyphRock   = "\U0001F7EB"       // 🟫 unexcavated regolith
	glyphFloor  = "  "               // open, walkable space
	glyphWall   = "\U0001F9F1"       // 🧱 built wall
	glyphPod    = "\U0001F37D\uFE0F" // 🍽️ nutrient pod (food)
	glyphToilet = "\U0001F6BD"       // 🚽 toilet (bladder)
	glyphBed    = "\U0001F6CF\uFE0F" // 🛏️ dormitory bunk (sleep)

	glyphColonist = "\U0001F477"       // 👷 colonist at work
	glyphFleeing  = "\U0001F631"       // 😱 colonist running from an alien
	glyphTalking  = "\U0001F5E3\uFE0F" // 🗣️ colonist chatting with another
	glyphAlien    = "\U0001F47D"       // 👽 subterranean mutant
	glyphCat      = "\U0001F408"       // 🐈 floor predator hunting mice
	glyphMouse    = "\U0001F401"       // 🐁 pest that raids the food pods
	glyphStomp    = "\U0001F97E"       // 🥾 colonist chasing down a mouse to stomp it
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
