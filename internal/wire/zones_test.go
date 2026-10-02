package wire

import (
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
