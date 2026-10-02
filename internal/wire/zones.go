package wire

import (
	"math"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// Zoning's two topics (see docs/zoning.md). "zones" is what the map draws:
// every zoned tile as row runs, and the colour of each kind. The page holds
// it for its whole life, as it does "names", so the overlay is always there;
// it changes only when someone zones something. "zoning" is the Zones tab:
// the structure-type table, every structure, the clearing orders, and what
// the colony is waiting on a zone for.

// ZonesTopic is the zone overlay.
type ZonesTopic struct {
	// Kinds is every zone kind by its value: Kinds[0] is "none", and a run's
	// Kind indexes this list.
	Kinds []ZoneKind `json:"kinds"`
	// Runs are [y, x0, x1, kind, locked] (locked 1: a crash pod's ground),
	// sorted by row then column.
	Runs [][5]int `json:"runs"`
}

// ZoneKind is one zone kind's name and the "#rrggbb" it is tinted.
type ZoneKind struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func zonesTopic(s *sim.Snapshot) ZonesTopic {
	t := ZonesTopic{Kinds: zoneKinds(), Runs: make([][5]int, 0, len(s.Zones))}
	for _, r := range s.Zones {
		locked := 0
		if r.Locked {
			locked = 1
		}
		t.Runs = append(t.Runs, [5]int{r.Y, r.X0, r.X1, int(r.Kind), locked})
	}
	return t
}

// zoneKinds lists the kinds by value, "none" first.
func zoneKinds() []ZoneKind {
	out := []ZoneKind{{Name: sim.NoZone.String()}}
	for _, k := range sim.ZoneKinds() {
		out = append(out, ZoneKind{Name: k.String(), Color: k.Color()})
	}
	return out
}

// ZoningTopic is the Zones tab.
type ZoningTopic struct {
	// Auto is zoning-auto: the colony zones and sites its rooms itself.
	Auto bool `json:"auto"`
	// Treasury, and the wages the tab prices a paint or a clearing with.
	Treasury  int64 `json:"treasury"`
	ClearWage int64 `json:"clearWage"`
	DigWage   int64 `json:"digWage"`
	// Types is the structure-type table: what each type is, and which zone
	// it is built in.
	Types []StructureType `json:"types"`
	// Tiles is how many tiles each kind covers, by kind name.
	Tiles map[string]int `json:"tiles"`
	// Structures is every standing or rising structure, by id.
	Structures []Structure `json:"structures"`
	// Clears are the clearing orders still open, oldest first.
	Clears []Dig `json:"clears"`
	// Waiting names the structure types the colony wants and no zone has
	// room for (manual zoning only).
	Waiting []StructureType `json:"waiting"`
}

// StructureType is a structure type and the zone kind it belongs in.
type StructureType struct {
	Name string `json:"name"`
	Zone string `json:"zone"`
}

// Structure is one structure: its type, the zone it belongs in (a kind
// value, as a zones run carries), the rectangle it stands on, and how many
// of its tiles are built.
type Structure struct {
	ID     int    `json:"id"`
	Type   string `json:"type"`
	Zone   int    `json:"zone"`
	X0     int    `json:"x0"`
	Y0     int    `json:"y0"`
	X1     int    `json:"x1"`
	Y1     int    `json:"y1"`
	Built  int    `json:"built"`
	Pod    bool   `json:"pod"`
	Rising bool   `json:"rising"`
}

func zoningTopic(s *sim.Snapshot) ZoningTopic {
	econ := s.Economy
	t := ZoningTopic{
		Auto:       s.ZoningAuto,
		Treasury:   int64(econ.Treasury),
		ClearWage:  int64(s.ClearWage),
		DigWage:    int64(econ.DigWage),
		Types:      []StructureType{},
		Tiles:      map[string]int{},
		Structures: make([]Structure, 0, len(s.Structures)),
		Clears:     []Dig{},
		Waiting:    []StructureType{},
	}
	for _, st := range sim.StructureTypes() {
		t.Types = append(t.Types, StructureType{Name: st.String(), Zone: st.Zone().String()})
	}
	for _, k := range sim.ZoneKinds() {
		t.Tiles[k.String()] = 0
	}
	for _, r := range s.Zones {
		t.Tiles[r.Kind.String()] += r.X1 - r.X0 + 1
	}
	for _, v := range s.Structures {
		t.Structures = append(t.Structures, Structure{ID: v.ID, Type: v.Type.String(), Zone: int(v.Type.Zone()),
			X0: v.X0, Y0: v.Y0, X1: v.X1, Y1: v.Y1, Built: v.Built, Pod: v.Pod, Rising: v.Rising})
	}
	for _, st := range s.ZoneWaiting {
		t.Waiting = append(t.Waiting, StructureType{Name: st.String(), Zone: st.Zone().String()})
	}
	for _, p := range s.Projects {
		if p.Name != sim.ClearingName {
			continue
		}
		d := Dig{ID: p.ID, X0: math.MaxInt, Y0: math.MaxInt, Tiles: len(p.Tasks), Done: p.TasksDone()}
		for _, tk := range p.Tasks {
			d.X0, d.Y0 = min(d.X0, tk.Pos.X), min(d.Y0, tk.Pos.Y)
			d.X1, d.Y1 = max(d.X1, tk.Pos.X), max(d.Y1, tk.Pos.Y)
		}
		for _, o := range econ.WorkOrders {
			if o.Kind == sim.WorkClear && digHas(p, o.Pos) {
				d.Held += int64(o.Pay) * int64(o.Units)
			}
		}
		t.Clears = append(t.Clears, d)
	}
	return t
}
