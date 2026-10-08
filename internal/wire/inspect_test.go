package wire

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// due subscribes to name and decodes the first payload into v.
func due(t *testing.T, snap *sim.Snapshot, name string, v any) {
	t.Helper()
	tp := NewTopics()
	if err := tp.Subscribe(name); err != nil {
		t.Fatal(err)
	}
	raw, ok := tp.Due(snap, time.Unix(0, 0))[name]
	if !ok {
		t.Fatalf("%s: nothing sent", name)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatal(err)
	}
}

func TestInspectTopicNames(t *testing.T) {
	tp := NewTopics()
	for _, ok := range []string{"entity:12", "tile:3,4", "tile:-1,0", "tile:3,4,2"} {
		if err := tp.Subscribe(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"entity:", "entity:x", "entity:-1", "tile:3", "tile:a,b", "tile:", "tile:3,4,x", "tile:3,4,-1", "tile:1,2,3,4", "roster:1"} {
		if err := tp.Subscribe(bad); err == nil {
			t.Errorf("%s subscribed", bad)
		}
	}
}

// A real colony: a colonist's inspector has what the TUI's shows, and each
// crash-pod locker names its owner.
func TestInspectRealColony(t *testing.T) {
	cfg := sim.DefaultConfig()
	cfg.Seed = 7
	cfg.Width, cfg.Height = 300, 300
	eng := sim.NewEngine(cfg)
	snap, _ := eng.Advance(time.Millisecond)
	if snap == nil {
		t.Fatal("no initial frame")
	}
	var col sim.EntityView
	for _, e := range snap.Entities {
		if e.Kind == sim.Colonist {
			col = e
			break
		}
	}
	if col.Profile == nil {
		t.Fatal("no colonist")
	}

	var e EntityTopic
	due(t, snap, fmt.Sprintf("entity:%d", col.ID), &e)
	c := e.Colonist
	if !e.Found || e.Name != col.Profile.Name || e.Glyph == "" || e.X != col.Pos.X || e.Dead || c == nil {
		t.Fatalf("entity = %+v", e)
	}
	if len(c.Drives) != len(col.Drives) || c.Drives[0].Name == "" || c.Drives[0].Max == 0 {
		t.Errorf("needs = %+v", c.Drives)
	}
	if len(e.Parts) < 6 || e.Parts[0].Max == 0 {
		t.Errorf("parts = %+v", e.Parts)
	}
	if c.Mood.Max != snap.MoodMax || c.AffinityMax != snap.AffinityMax || c.Height == "" || c.Pronouns == "" {
		t.Errorf("colonist = %+v", c)
	}
	if len(c.Skills) != len(col.Skills) {
		t.Errorf("skills = %+v, want %d", c.Skills, len(col.Skills))
	}
	for i, sk := range c.Skills {
		want := col.Skills[i]
		if sk.Name != want.Skill.String() || sk.Label != want.Label || sk.Rank != want.Rank || sk.MaxRank != want.MaxRank {
			t.Errorf("skill %d = %+v, want %+v", i, sk, want)
		}
	}
	if col.Profession != sim.SkillNone && (c.Profession != col.Profession.String() || c.ProfessionLabel != col.ProfessionLabel) {
		t.Errorf("profession = %q %q, want %v %q", c.Profession, c.ProfessionLabel, col.Profession, col.ProfessionLabel)
	}
	if c.Backstory != col.Backstory {
		t.Errorf("backstory = %q, want %q", c.Backstory, col.Backstory)
	}

	lockers := 0
	for _, st := range snap.Storages {
		var tile TileTopic
		due(t, snap, fmt.Sprintf("tile:%d,%d", st.Pos.X, st.Pos.Y), &tile)
		if !tile.Explored || tile.Storage == nil || tile.Storage.Slots == 0 {
			t.Fatalf("storage tile = %+v", tile)
		}
		if f := tile.Fixture; f != nil && f.Access == "private" {
			lockers++
			if f.OwnerID == 0 || tile.Storage.Label != f.Owner+"'s locker" {
				t.Errorf("locker = %+v, %+v", f, tile.Storage)
			}
		}
	}
	if lockers == 0 {
		t.Error("no crash-pod lockers inspected")
	}

	var here TileTopic
	due(t, snap, fmt.Sprintf("tile:%d,%d", col.Pos.X, col.Pos.Y), &here)
	found := false
	for _, cr := range here.Creatures {
		found = found || cr.ID == uint64(col.ID)
	}
	if !found {
		t.Errorf("colonist not on own tile: %+v", here)
	}
}

// The names topic names living colonists by id, and nothing else.
func TestNamesTopic(t *testing.T) {
	snap := fixture(true)
	snap.Entities[0].Profile = &sim.Profile{Name: "Uma Xu"}
	var names map[string]string
	due(t, snap, "names", &names)
	if len(names) != 1 || names["1"] != "Uma Xu" {
		t.Errorf("names = %v", names)
	}
}

// The fog hides a tile's contents, creatures included.
func TestInspectUnexploredTile(t *testing.T) {
	snap := fixture(true)
	snap.Entities[0].Pos = sim.Point{X: 7, Y: 0, Level: sim.LandingLevel} // explored floor in the fixture
	var tile TileTopic
	due(t, snap, "tile:149,69", &tile) // the fixture's alien, on unexplored rock
	if tile.Explored || tile.Terrain != "" || len(tile.Creatures) != 0 {
		t.Errorf("unexplored tile = %+v", tile)
	}
	tile = TileTopic{}
	due(t, snap, "tile:7,0", &tile)
	if !tile.Explored || tile.Terrain != "floor" || len(tile.Creatures) != 1 || tile.Creatures[0].ID != 1 {
		t.Errorf("explored tile = %+v", tile)
	}
}

// A dead colonist is found in Deceased; an unknown id says so.
func TestInspectDeadAndMissing(t *testing.T) {
	snap := fixture(true)
	snap.Deceased = map[sim.EntityID]sim.EntityView{
		40: {ID: 40, Kind: sim.Colonist, Dead: true, DiedTick: 900, Cause: "starved",
			Profile: &sim.Profile{Name: "Ada Okafor"}},
	}
	var e EntityTopic
	due(t, snap, "entity:40", &e)
	if !e.Found || !e.Dead || e.DiedTick != 900 || e.Cause != "starved" || e.Name != "Ada Okafor" {
		t.Errorf("dead = %+v", e)
	}
	e = EntityTopic{}
	due(t, snap, "entity:77", &e)
	if e.Found || e.ID != 77 {
		t.Errorf("missing = %+v", e)
	}
}
