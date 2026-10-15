package wire

import (
	"cmp"
	"math"
	"slices"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// Zoning's two topics (see docs/zoning.md). "zones" is what the map draws:
// every zoned tile as row runs, the colour of each kind, the defense zones,
// and the tiles marked for digging. The page holds it for its whole life, as
// it does "names", so the overlay is always there; it changes only when
// someone zones, posts or stands down a defense zone, or digs a marked tile. "zoning" is the Zones tab:
// the structure-type table, every structure, the clearing orders, and what
// the colony is waiting on a zone for.

// ZonesTopic is the zone overlay.
type ZonesTopic struct {
	// Kinds is every zone kind by its value: Kinds[0] is "none", and a run's
	// Kind indexes this list.
	Kinds []ZoneKind `json:"kinds"`
	// Runs are [y, x0, x1, kind, locked] (locked 1: a colony ship's ground),
	// sorted by row then column.
	Runs [][5]int `json:"runs"`
	// Defense is every defense zone's area, [id, level, x0, y0, x1, y1], by
	// id, and DefenseColor the "#rrggbb" the map tints them. Defense zones
	// are not on the zone grid: they overlap the other kinds and lie on any
	// level (see docs/defense.md).
	Defense      [][6]int `json:"defense"`
	DefenseColor string   `json:"defenseColor"`
	// Digs are the tiles excavation orders have marked and nobody has dug
	// yet, as row runs [level, y, x0, x1], sorted by level, row, then
	// column, so the map can show what is marked for mining on any level
	// whatever tab is open (see docs/excavation.md).
	Digs [][4]int `json:"digs"`
}

// DefenseColor is the tint of a defense zone, on the map and the Zones tab.
const DefenseColor = "#e53935"

// ZoneKind is one zone kind's name and the "#rrggbb" it is tinted.
type ZoneKind struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func zonesTopic(s *sim.Snapshot) ZonesTopic {
	t := ZonesTopic{Kinds: zoneKinds(), Runs: make([][5]int, 0, len(s.Zones)),
		Defense: make([][6]int, 0, len(s.Defense)), DefenseColor: DefenseColor}
	for _, d := range s.Defense {
		t.Defense = append(t.Defense, [6]int{d.ID, int(d.Level), d.X0, d.Y0, d.X1, d.Y1})
	}
	t.Digs = digRuns(s)
	for _, r := range s.Zones {
		locked := 0
		if r.Locked {
			locked = 1
		}
		t.Runs = append(t.Runs, [5]int{r.Y, r.X0, r.X1, int(r.Kind), locked})
	}
	return t
}

// digRuns is every undug tile of every excavation order, as sorted row runs.
// A tile under the fog stays marked until it is found and dug (taskDone).
func digRuns(s *sim.Snapshot) [][4]int {
	var tiles []sim.Point
	for _, p := range s.Projects {
		if p.Name != sim.ExcavationName {
			continue
		}
		for _, tk := range p.Tasks {
			if !tk.Done {
				tiles = append(tiles, tk.Pos)
			}
		}
	}
	slices.SortFunc(tiles, func(a, b sim.Point) int {
		return cmp.Or(cmp.Compare(a.Level, b.Level), cmp.Compare(a.Y, b.Y), cmp.Compare(a.X, b.X))
	})
	runs := make([][4]int, 0, len(tiles))
	for _, pt := range tiles {
		if n := len(runs); n > 0 {
			r := &runs[n-1]
			if r[0] == int(pt.Level) && r[1] == pt.Y && r[3] >= pt.X-1 {
				r[3] = max(r[3], pt.X)
				continue
			}
		}
		runs = append(runs, [4]int{int(pt.Level), pt.Y, pt.X, pt.X})
	}
	return runs
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
	// Types is the fixture table: each kind of fixture, and which zone it is
	// built in (a chest as storage; a pantry or a ship's locker goes with its
	// room). A room's zone is its fixtures'.
	Types []StructureType `json:"types"`
	// Tiles is how many tiles each kind covers, by kind name.
	Tiles map[string]int `json:"tiles"`
	// Structures is every standing or rising structure, by id.
	Structures []Structure `json:"structures"`
	// Clears are the clearing orders still open, oldest first.
	Clears []Dig `json:"clears"`
	// Waiting names the fixture kinds the colony wants and no zone has room
	// for (manual zoning only).
	Waiting []StructureType `json:"waiting"`
	// Defense is every defense zone, by id; GuardWage what a guard is paid
	// an hour on station, Armed how many living colonists carry a weapon,
	// and MaxDefense the most tiles one zone may cover.
	Defense    []Defense `json:"defense"`
	GuardWage  int64     `json:"guardWage"`
	Armed      int       `json:"armed"`
	MaxDefense int       `json:"maxDefense"`
	MaxDetail  int       `json:"maxDetail"`
	// EMT is the colony's EMT detail (docs/emergency-services.md).
	EMT EMT `json:"emt"`
}

// EMT is the EMT detail: how many the colony keeps on call, the most it may,
// who is on call now, what one is paid an hour and what the colony has paid
// in all, and how many infirmary cots stand.
type EMT struct {
	Detail int     `json:"detail"`
	Max    int     `json:"max"`
	OnCall []Guard `json:"onCall"`
	Wage   int64   `json:"wage"`
	Paid   int64   `json:"paid"`
	Cots   int     `json:"cots"`
}

// Defense is one defense zone: its area, the detail it asks for, the guards
// holding it (by id and name), how many of them are inside the area now, and
// what the colony has paid them.
type Defense struct {
	ID        int     `json:"id"`
	Level     int     `json:"level"`
	X0        int     `json:"x0"`
	Y0        int     `json:"y0"`
	X1        int     `json:"x1"`
	Y1        int     `json:"y1"`
	Detail    int     `json:"detail"`
	Guards    []Guard `json:"guards"`
	OnStation int     `json:"onStation"`
	Paid      int64   `json:"paid"`
}

// Guard is one colonist on a defense detail.
type Guard struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// StructureType is a kind of fixture and the zone kind it is built in. (The
// name is older than fixture-level zoning: rooms used to carry the tag.)
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
	Ship   bool   `json:"ship"`
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
		Defense:    make([]Defense, 0, len(s.Defense)),
		GuardWage:  int64(s.GuardWage),
		Armed:      s.Armed,
		MaxDefense: sim.MaxDefenseTiles,
		MaxDetail:  sim.MaxDefenseDetail,
		EMT: EMT{Detail: s.EMT.Detail, Max: s.EMT.Max, OnCall: make([]Guard, 0, len(s.EMT.OnCall)),
			Wage: int64(s.EMT.Wage), Paid: int64(s.EMT.Paid), Cots: s.EMT.Cots},
	}
	var names map[sim.EntityID]string
	nameOf := func(id sim.EntityID) string {
		if names == nil {
			names = map[sim.EntityID]string{}
			for _, e := range s.Entities {
				if e.Kind == sim.Colonist {
					names[e.ID] = entityName(e)
				}
			}
		}
		return names[id]
	}
	for _, id := range s.EMT.OnCall {
		t.EMT.OnCall = append(t.EMT.OnCall, Guard{ID: uint64(id), Name: nameOf(id)})
	}
	for _, d := range s.Defense {
		if names == nil && len(d.Guards) > 0 {
			names = map[sim.EntityID]string{}
			for _, e := range s.Entities {
				if e.Kind == sim.Colonist {
					names[e.ID] = entityName(e)
				}
			}
		}
		v := Defense{ID: d.ID, Level: int(d.Level), X0: d.X0, Y0: d.Y0, X1: d.X1, Y1: d.Y1,
			Detail: d.Detail, Guards: make([]Guard, 0, len(d.Guards)), OnStation: d.OnStation, Paid: int64(d.Paid)}
		for _, id := range d.Guards {
			v.Guards = append(v.Guards, Guard{ID: uint64(id), Name: names[id]})
		}
		t.Defense = append(t.Defense, v)
	}
	for _, k := range sim.FixtureKinds() {
		t.Types = append(t.Types, StructureType{Name: k.String(), Zone: sim.FixtureZone(k).String()})
	}
	for _, k := range sim.ZoneKinds() {
		t.Tiles[k.String()] = 0
	}
	for _, r := range s.Zones {
		t.Tiles[r.Kind.String()] += r.X1 - r.X0 + 1
	}
	for _, v := range s.Structures {
		t.Structures = append(t.Structures, Structure{ID: v.ID, Type: v.Name, Zone: int(v.Zone),
			X0: v.X0, Y0: v.Y0, X1: v.X1, Y1: v.Y1, Built: v.Built, Ship: v.Ship, Rising: v.Rising})
	}
	for _, k := range s.ZoneWaiting {
		t.Waiting = append(t.Waiting, StructureType{Name: k.String(), Zone: sim.FixtureZone(k).String()})
	}
	for _, p := range s.Projects {
		if p.Name != sim.ClearingName {
			continue
		}
		d := Dig{ID: p.ID, X0: math.MaxInt, Y0: math.MaxInt, Tiles: len(p.Tasks), Done: p.TasksDone()}
		for _, tk := range p.Tasks {
			d.X0, d.Y0 = min(d.X0, tk.Pos.X), min(d.Y0, tk.Pos.Y)
			d.X1, d.Y1 = max(d.X1, tk.Pos.X), max(d.Y1, tk.Pos.Y)
			d.Level = int(tk.Pos.Level)
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
