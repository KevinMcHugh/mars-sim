package wire

import (
	"fmt"
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The zone overlay and the Zones tab carry what the engine zoned: the
// colony ships' residence, a painted zone, and the structure table.
func TestZoneTopics(t *testing.T) {
	cfg := sim.DefaultConfig()
	cfg.Seed, cfg.Width, cfg.Height, cfg.StartColonists = 7, 80, 50, 3
	eng := sim.NewEngine(cfg)
	snap, _ := eng.Advance(0)
	eng.Send(sim.TogglePause{})
	eng.Send(sim.PaintZone{Kind: sim.ZoneProduction, X0: 30, Y0: 15, X1: 34, Y1: 17})
	for deadline := time.Now().Add(10 * time.Second); ; {
		if s, _ := eng.Advance(10 * time.Millisecond); s != nil {
			snap = s
		}
		if len(snap.Zones) > 0 && snap.Paused {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the paint never showed")
		}
	}

	var zones ZonesTopic
	due(t, snap, "zones", &zones)
	if len(zones.Kinds) < 4 || zones.Kinds[0].Name != "none" || zones.Kinds[int(sim.ZoneResidence)].Color == "" {
		t.Fatalf("kinds = %+v", zones.Kinds)
	}
	production, locked := 0, 0
	for _, r := range zones.Runs {
		if r[3] == int(sim.ZoneProduction) {
			production += r[2] - r[1] + 1
		}
		if r[4] == 1 {
			locked++
			if r[3] != int(sim.ZoneResidence) {
				t.Errorf("a locked run is not residence: %v", r)
			}
		}
	}
	if locked == 0 {
		t.Error("no ship ground in the overlay")
	}

	var z ZoningTopic
	due(t, snap, "zoning", &z)
	if z.Auto || z.ClearWage <= 0 || len(z.Types) == 0 {
		t.Fatalf("zoning = %+v", z)
	}
	if z.Tiles["production"] != production || production == 0 {
		t.Errorf("production tiles %d, overlay %d", z.Tiles["production"], production)
	}
	ships := 0
	for _, s := range z.Structures {
		if s.Ship {
			ships++
			if s.Type != "colony ship" || s.Zone != int(sim.ZoneResidence) || s.Built == 0 {
				t.Errorf("ship = %+v", s)
			}
		}
	}
	if ships != len(snap.Ships) || ships == 0 {
		t.Errorf("%d ships listed, the snapshot has %d", ships, len(snap.Ships))
	}
}

// The overlay marks every excavation tile not yet dug, as sorted row runs
// per level, and nothing of a room's project or a dug tile.
func TestZonesDigRuns(t *testing.T) {
	at := func(x, y int, l sim.Level, done bool) sim.TaskView {
		return sim.TaskView{Pos: sim.Point{X: x, Y: y, Level: l}, Done: done}
	}
	deep := sim.LandingLevel + 1
	snap := &sim.Snapshot{Projects: []sim.ProjectView{
		{ID: 1, Name: sim.ExcavationName, Tasks: []sim.TaskView{
			at(6, 2, deep, false), at(5, 2, deep, false), at(7, 2, deep, true), at(8, 2, deep, false),
			at(4, 3, sim.LandingLevel, false), at(3, 3, sim.LandingLevel, false),
		}},
		{ID: 2, Name: "dormitory", Tasks: []sim.TaskView{at(1, 1, sim.LandingLevel, false)}},
		{ID: 3, Name: sim.ExcavationName, Tasks: []sim.TaskView{at(5, 3, sim.LandingLevel, false)}},
	}}
	var zones ZonesTopic
	due(t, snap, "zones", &zones)
	l, d := int(sim.LandingLevel), int(deep)
	want := [][4]int{{l, 3, 3, 5}, {d, 2, 5, 6}, {d, 2, 8, 8}}
	if fmt.Sprint(zones.Digs) != fmt.Sprint(want) {
		t.Errorf("digs = %v, want %v", zones.Digs, want)
	}
}
