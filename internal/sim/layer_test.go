package sim

import "testing"

// TestOneLayerAtTheLandingLevel pins the shape of the world while it has only
// one level: the landing level exists, is home, and nothing else does.
func TestOneLayerAtTheLandingLevel(t *testing.T) {
	w := newTestWorld(t, DefaultConfig())
	if w.home.Level != LandingLevel {
		t.Fatalf("home is level %d, want the landing level %d", w.home.Level, LandingLevel)
	}
	if got := w.layer(LandingLevel); got != &w.home {
		t.Fatalf("layer(LandingLevel) = %p, want &w.home (%p)", got, &w.home)
	}
	for _, l := range []Level{SurfaceLevel, LandingLevel + 1, -1} {
		if w.layer(l) != nil {
			t.Errorf("layer(%d) exists; only the landing level should", l)
		}
	}
}

// TestNothingIsOnTheSurface runs a colony long enough to trade and checks that
// every place it names is on the landing level. The zero Loc is on the
// surface, so a Loc built without a level shows up here as level 0 instead
// of passing as right while there is only one level. See docs/z-levels.md.
func TestNothingIsOnTheSurface(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 2
	cfg.Width, cfg.Height = 300, 150
	cfg.StartColonists = 40
	w := NewEngine(cfg).world
	for i := 0; i < 1200; i++ {
		w.step()
	}
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Level != LandingLevel {
			t.Errorf("entity %d (%v) at %v is on level %d", id, e.Kind, e.Pos, e.Level)
		}
	}
	if len(w.books) == 0 {
		t.Fatal("no order book opened in 1200 ticks; pick a config that trades")
	}
	for k := range w.books {
		if k.Depot.Level != LandingLevel {
			t.Errorf("a %v book's depot is %v, not on the landing level", k.Item, k.Depot)
		}
	}
	for id, o := range w.orders {
		if o.Depot.Level != LandingLevel {
			t.Errorf("order %d's depot is %v, not on the landing level", id, o.Depot)
		}
	}
}

func TestLessLocOrdersByLevelFirst(t *testing.T) {
	cases := []struct {
		a, b Loc
		want bool
	}{
		{at(1, Point{9, 9}), at(2, Point{0, 0}), true}, // shallower first, wherever it is
		{at(2, Point{0, 0}), at(1, Point{9, 9}), false},
		{at(1, Point{5, 0}), at(1, Point{0, 1}), true}, // then row-major, as lessPoint
		{at(1, Point{0, 1}), at(1, Point{5, 0}), false},
		{at(1, Point{3, 3}), at(1, Point{3, 3}), false},
	}
	for _, c := range cases {
		if got := lessLoc(c.a, c.b); got != c.want {
			t.Errorf("lessLoc(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	if (Loc{}).Level != SurfaceLevel {
		t.Error("the zero Loc should be on the surface, so a missing level is visible")
	}
}
