package wire

import (
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

func rosterIDs(t *testing.T, snap *sim.Snapshot, name string) []uint64 {
	t.Helper()
	var rows []RosterRow
	due(t, snap, name, &rows)
	ids := make([]uint64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// The filters admit what the TUI's do: living colonists always; nonhuman adds
// the other kinds; dead adds dead colonists, and dead others only with
// nonhuman too. Each death is listed once, and rows are by ID.
func TestRosterFilters(t *testing.T) {
	snap := fixture(true) // living: colonist 1, cat 2, alien 9
	snap.Entities[0].Profile = &sim.Profile{Name: "Uma Xu", Age: 34, Gender: sim.GenderNonbinary}
	snap.Entities[0].MoodLabel = "steady"
	snap.Deceased = map[sim.EntityID]sim.EntityView{
		8: {ID: 8, Kind: sim.Colonist, Dead: true, Cause: "starved", Profile: &sim.Profile{Name: "Ada"}},
		4: {ID: 4, Kind: sim.Colonist, Dead: true, Cause: "shot", Profile: &sim.Profile{Name: "Bo"}},
	}
	snap.Graveyard = []sim.EntityView{
		{ID: 4, Kind: sim.Colonist, Dead: true}, // also in Deceased
		{ID: 6, Kind: sim.Rat, Dead: true, Cause: "eaten"},
	}
	for name, want := range map[string][]uint64{
		"roster":               {1},
		"roster:":              {1},
		"roster:nonhuman":      {1, 2, 9},
		"roster:dead":          {1, 4, 8},
		"roster:dead,nonhuman": {1, 2, 4, 6, 8, 9},
		"roster:nonhuman,dead": {1, 2, 4, 6, 8, 9},
	} {
		got := rosterIDs(t, snap, name)
		if len(got) != len(want) {
			t.Errorf("%s = %v, want %v", name, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s = %v, want %v", name, got, want)
				break
			}
		}
	}

	var rows []RosterRow
	due(t, snap, "roster:dead", &rows)
	if r := rows[0]; r.Name != "Uma Xu" || r.Info != "they/them · age 34" || r.State != "mining · steady" || r.Glyph == "" {
		t.Errorf("colonist row = %+v", r)
	}
	if r := rows[1]; !r.Dead || r.State != "dead — shot" {
		t.Errorf("dead row = %+v", r)
	}

	tp := NewTopics()
	if err := tp.Subscribe("roster:zombies"); err == nil {
		t.Error("an unknown filter subscribed")
	}
}
