package sim

import (
	"bytes"
	"testing"
)

// The landing level generates from the settings as they are; a deeper level
// from settings scaled a level at a time.
func TestDepthConfigScalesBelowTheLandingLevel(t *testing.T) {
	cfg := DefaultConfig()
	if got := depthConfig(cfg, LandingLevel); got.IronRockPercent != cfg.IronRockPercent ||
		got.UraniumRockPercent != cfg.UraniumRockPercent || got.CavernNestPercent != cfg.CavernNestPercent {
		t.Fatal("depthConfig changed the landing level's settings")
	}
	d3 := depthConfig(cfg, LandingLevel+2)
	if want := cfg.UraniumRockPercent * (100 + 2*cfg.DepthUraniumPercent) / 100; d3.UraniumRockPercent != want {
		t.Errorf("uranium two levels down = %d%%, want %d%%", d3.UraniumRockPercent, want)
	}
	if want := cfg.IronRockPercent * (100 + 2*cfg.DepthOrePercent) / 100; d3.IronRockPercent != want {
		t.Errorf("iron two levels down = %d%%, want %d%%", d3.IronRockPercent, want)
	}
	if d3.CavernNestPercent != min(100, cfg.CavernNestPercent+2*cfg.DepthNestPercent) ||
		d3.CavernNestMin != cfg.CavernNestMin+2*cfg.DepthNestSize || d3.CavernMax <= cfg.CavernMax {
		t.Errorf("nests and caverns two levels down: %d%%, min %d, cavern max %d",
			d3.CavernNestPercent, d3.CavernNestMin, d3.CavernMax)
	}
	cfg.UraniumRockPercent = 90
	if got := depthConfig(cfg, LandingLevel+5).UraniumRockPercent; got != 100 {
		t.Errorf("uranium capped at %d%%, want 100", got)
	}
}

// What a deep level actually generates is richer than the landing level: more
// uranium and iron, by the generator's own count.
func TestDeeperLevelsGenerateRicher(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	cfg.Width, cfg.Height = 256, 128
	count := func(l Level) (uranium, iron int) {
		p := newChunkPreview(cfg, l)
		for y := 0; y < cfg.Height; y++ {
			for x := 0; x < cfg.Width; x++ {
				switch p.At(Point{x, y, l}).Composition {
				case UraniumBearingRock:
					uranium++
				case IronBearingRock:
					iron++
				}
			}
		}
		return uranium, iron
	}
	u1, i1 := count(LandingLevel)
	u3, i3 := count(LandingLevel + 2)
	if u3 < 2*u1 || i3 <= i1 {
		t.Errorf("landing level: %d uranium, %d iron; two down: %d uranium, %d iron", u1, i1, u3, i3)
	}
}

// Deep nests lean hostile; the landing level's draw is the plain one.
func TestDeepNestsLeanHostile(t *testing.T) {
	cfg := testConfig()
	cfg.AlienSpeciesCount = 4
	w := newTestWorld(t, cfg)
	if len(w.alienSpecies) != 4 {
		t.Fatalf("roster has %d species, want 4", len(w.alienSpecies))
	}
	for i := range w.alienSpecies {
		w.alienSpecies[i].Temperament = TemperamentFriendly
	}
	w.alienSpecies[0].Temperament = TemperamentHostile
	hostileShare := func(l Level) float64 {
		w.nestRNG = newRand(7)
		n := 0
		for i := 0; i < 4000; i++ {
			if w.nestSpecies(l) == 0 {
				n++
			}
		}
		return float64(n) / 4000
	}
	flat := 1 / float64(len(w.alienSpecies))
	if got := hostileShare(LandingLevel); got < flat*0.8 || got > flat*1.2 {
		t.Errorf("landing level hostile share %.2f, want about %.2f", got, flat)
	}
	if deep, landing := hostileShare(LandingLevel+3), hostileShare(LandingLevel); deep < 1.5*landing {
		t.Errorf("three levels down hostile share %.2f, landing %.2f: depth should weight hostiles", deep, landing)
	}
}

// cavernPair is an all-rock world with a discovered cavern on the landing
// level and, under part of it, a hidden cavern on the level below.
func cavernPair(t *testing.T) (*World, Point) {
	t.Helper()
	cfg := testConfig()
	cfg.Width, cfg.Height = 40, 20
	cfg.DeepestLevel = 2
	w := newWorld(cfg, newPCG(1))
	carveOn(w, LandingLevel, Point{5, 5, 0}, Point{15, 12, 0}, Floor)
	w.addLayer(LandingLevel + 1)
	for y := 7; y <= 10; y++ {
		for x := 12; x <= 20; x++ {
			w.carveHidden(Point{x, y, LandingLevel + 1})
		}
	}
	w.refreshSpatial()
	return w, Point{10, 8, LandingLevel}
}

// A natural shaft joins a cavern to the cavern under it, and breaking into
// the lower one reveals it.
func TestNaturalShaftJoinsCaverns(t *testing.T) {
	w, c := cavernPair(t)
	w.naturalShaft(c, newWorldGen(w.cfg, c.Level))
	if len(w.shafts) != 1 {
		t.Fatalf("shafts = %v, want one", w.shafts)
	}
	top := w.shafts[0]
	foot := Point{top.X, top.Y, top.Level + 1}
	if w.TerrainAt(top) != ShaftTop || w.TerrainAt(foot) != ShaftBottom {
		t.Fatalf("natural shaft is %v over %v", w.TerrainAt(top), w.TerrainAt(foot))
	}
	if top.X < 12 || top.Y < 7 || top.Y > 10 {
		t.Errorf("the shaft at %v is not over the lower cavern", top)
	}
	if !w.discovered(Point{20, 10, LandingLevel + 1}) {
		t.Error("breaking into the lower cavern did not reveal it")
	}
	w.refreshSpatial()
	if !w.sameRoom(Point{5, 5, LandingLevel}, Point{20, 10, LandingLevel + 1}) {
		t.Error("the shaft does not join the two caverns")
	}
}

// With nothing open beneath it, a cavern gets no natural shaft; a sinkhole
// opens anywhere with room, onto rock it breaks into.
func TestSinkholeAndNoShaftOverRock(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 40, 20
	cfg.DeepestLevel = 2
	w := newWorld(cfg, newPCG(1))
	carveOn(w, LandingLevel, Point{5, 5, 0}, Point{10, 10, 0}, Floor)
	w.addLayer(LandingLevel + 1)
	c := Point{7, 7, LandingLevel}
	g := newWorldGen(w.cfg, c.Level)
	w.naturalShaft(c, g)
	if len(w.shafts) != 0 {
		t.Fatal("a natural shaft over solid rock")
	}
	w.sinkhole(c, g)
	if len(w.holes) != 1 {
		t.Fatalf("holes = %v, want one sinkhole", w.holes)
	}
	h := w.holes[0]
	if !w.openHole(h) || w.TerrainAt(Point{h.X, h.Y, h.Level + 1}) != Floor {
		t.Error("the sinkhole drops nowhere")
	}
	for _, d := range neighbors8 {
		if w.TerrainAt(h.Add(d.X, d.Y)) != Floor {
			t.Errorf("the sinkhole at %v plugs a passage", h)
		}
	}
}

// A hostile alien with nothing to walk to drops down a hole onto a colonist
// below.
func TestHostileAlienRaidsDownAHole(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 40, 20
	cfg.DeepestLevel = 2
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	carveOn(w, LandingLevel, Point{3, 3, 0}, Point{12, 8, 0}, Floor)
	h := Point{9, 5, LandingLevel}
	if !w.digHole(h) {
		t.Fatal("could not dig the hole")
	}
	carveOn(w, LandingLevel+1, Point{8, 4, 0}, Point{11, 7, 0}, Floor)
	w.refreshSpatial()
	setAlienTemperament(w, 0, TemperamentHostile)
	alien := w.spawnAs(Alien, Point{4, 4, LandingLevel}, 0)
	w.spawn(Colonist, Point{11, 7, LandingLevel + 1})
	for i := 0; i < 200 && alien.Pos.Level == LandingLevel; i++ {
		w.tick++
		alien.Cooldown = 0
		w.animalTurn(alien)
	}
	if w.entities[alien.ID] != nil && alien.Pos.Level != LandingLevel+1 {
		t.Fatalf("the alien is still at %v", alien.Pos)
	}
}

// A colony that digs down with natural features turned all the way up runs
// the same twice and survives a save.
func TestDepthRunsAreDeterministicAndSave(t *testing.T) {
	run := func() (*World, string) {
		cfg := testConfig()
		cfg.Seed = 11
		cfg.Width, cfg.Height = 120, 70
		cfg.StartColonists = 8
		cfg.DeepestLevel = 3
		cfg.CavernNestPercent = 30
		cfg.NaturalShaftPercent, cfg.SinkholePercent = 100, 100
		w := newTestWorld(t, cfg)
		w.manualStairs = 2
		for i := 0; i < 2500; i++ {
			w.step()
		}
		if len(w.stairs) == 0 || len(w.shafts)+len(w.holes) == 0 {
			t.Fatal("no stair dug or no natural feature found: the test proves nothing about depth")
		}
		return w, levelsHash(w)
	}
	w, a := run()
	if _, b := run(); a != b {
		t.Fatalf("runs diverge:\n%s\n%s", a, b)
	}
	loaded := saveAndLoad(t, w)
	for i := 0; i < 300; i++ {
		w.step()
		loaded.step()
	}
	if !bytes.Equal(encodeWorld(t, w), encodeWorld(t, loaded)) {
		t.Fatalf("loaded game's state differs: %s", saveDiff(w, loaded))
	}
}
