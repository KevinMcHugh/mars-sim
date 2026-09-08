package tui

import "github.com/kevinmchugh/mars-sim/internal/sim"

// Every glyph is chosen to render two terminal cells wide so the grid stays
// aligned. Open floor is two spaces (also two cells), which reads as empty
// cavern against the solid terrain. Some terminals size emoji differently; if
// the grid ever looks sheared, that is the cause.
const (
	glyphRock  = "\U0001F7EB" // 🟫 unexcavated regolith
	glyphFloor = "  "         // open, walkable space
	glyphWall  = "\U0001F9F1" // 🧱 built wall

	glyphColonist = "\U0001F477" // 👷 colonist at work
	glyphFleeing  = "\U0001F631" // 😱 colonist running from an alien
	glyphAlien    = "\U0001F47D" // 👽 subterranean mutant
)

func terrainGlyph(t sim.Terrain) string {
	switch t {
	case sim.Floor:
		return glyphFloor
	case sim.Wall:
		return glyphWall
	default:
		return glyphRock
	}
}

func entityGlyph(e sim.EntityView) string {
	switch e.Kind {
	case sim.Alien:
		return glyphAlien
	case sim.Colonist:
		if e.State == sim.Fleeing {
			return glyphFleeing
		}
		return glyphColonist
	default:
		return glyphColonist
	}
}
