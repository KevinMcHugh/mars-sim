package sim

// Enums names every value of the enums a frontend draws by, each list indexed
// by value: Terrains[Floor] is "floor". A frontend that is not written in Go
// (the browser's, see docs/browser-frontend.md) gets these once at startup
// rather than keeping its own copy of each table, so adding a terrain or a
// state here needs no change on the other side.
type Enums struct {
	Terrains     []string `json:"terrains"`
	Compositions []string `json:"compositions"`
	Kinds        []string `json:"kinds"`
	States       []string `json:"states"`
	Focuses      []string `json:"focuses"`
}

// EnumNames returns the names of every Terrain, RockComposition, Kind, State
// and FocusKind, from each type's String method.
func EnumNames() Enums {
	return Enums{
		Terrains:     names(numTerrains, Terrain.String),
		Compositions: names(numRockCompositions, RockComposition.String),
		Kinds:        names(numKinds, Kind.String),
		States:       names(numStates, State.String),
		Focuses:      names(numFocusKinds, FocusKind.String),
	}
}

func names[T ~uint8](n T, name func(T) string) []string {
	out := make([]string, n)
	for v := T(0); v < n; v++ {
		out[v] = name(v)
	}
	return out
}
