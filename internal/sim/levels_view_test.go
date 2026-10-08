package sim

import "testing"

// The player's area orders (dig, clear) name the level they are on, and a
// clearing never takes out a stair, shaft or hole.
func TestAreaOrdersNameTheirLevel(t *testing.T) {
	w, _, bottom := stairWorld(t)
	w.treasury = 1_000_000
	deep := bottom.Level
	if !w.orderExcavation(OrderExcavation{X0: 5, Y0: 3, X1: 35, Y1: 13, Level: deep}) {
		t.Fatalf("a dig on level %d was refused: %v", deep, w.log.tail(1))
	}
	for _, task := range w.projects[len(w.projects)-1].tasks {
		if task.pos.Level != deep {
			t.Fatalf("a dig ordered on level %d has a task at %v", deep, task.pos)
		}
	}
	if w.orderExcavation(OrderExcavation{X0: 5, Y0: 3, X1: 35, Y1: 13, Level: deep + 1}) {
		t.Error("a dig on a level nobody has broken into went ahead")
	}

	if w.clearArea(ClearArea{X0: bottom.X, Y0: bottom.Y, X1: bottom.X, Y1: bottom.Y, Level: deep}) {
		t.Error("a clearing took out the foot of a stair")
	}
	wall := Point{20, 8, deep}
	w.SetTerrain(wall, Wall)
	if !w.clearArea(ClearArea{X0: 5, Y0: 3, X1: 35, Y1: 13, Level: deep}) {
		t.Fatalf("a clearing on level %d was refused: %v", deep, w.log.tail(1))
	}
	p := w.projects[len(w.projects)-1]
	if len(p.tasks) != 1 || p.tasks[0].pos != wall {
		t.Errorf("the clearing's tasks are %v, want just the wall at %v", p.tasks, wall)
	}
}

// Every level's tile changes are published, numbered alike, so a frontend
// can follow whichever level it shows.
func TestSnapshotPublishesEveryLevelsChanges(t *testing.T) {
	w, _, bottom := stairWorld(t)
	s := w.snapshot(false, 8)
	if len(s.LevelChanges) <= int(bottom.Level) || !s.LevelChanges[bottom.Level].All {
		t.Fatalf("first snapshot's changes: %+v", s.LevelChanges)
	}
	w.SetTerrain(Point{25, 8, bottom.Level}, Wall)
	s = w.snapshot(false, 8)
	deep, land := s.LevelTileChanges(bottom.Level), s.LevelTileChanges(LandingLevel)
	if deep.Frame != s.TileChanges.Frame || land.Frame != deep.Frame {
		t.Errorf("frames differ: landing %d, deep %d, TileChanges %d", land.Frame, deep.Frame, s.TileChanges.Frame)
	}
	if len(deep.Pages) != 1 || len(land.Pages) != 0 {
		t.Errorf("a wall on level %d changed pages %v there and %v on the landing level", bottom.Level, deep.Pages, land.Pages)
	}
	if s.LevelGrid(bottom.Level+1) != nil || s.LevelPageKnown(bottom.Level+1, 0) {
		t.Error("a level nobody has broken into has a grid")
	}
	dst := make([]Tile, TilePageSide*TilePageSide)
	if !s.ReadLevelPage(bottom.Level, deep.Pages[0], dst) || dst[8*TilePageSide+25].Terrain != Wall {
		t.Error("ReadLevelPage does not show the new wall")
	}
}
