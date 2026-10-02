package sim

import "testing"

// TestOneLayerAtTheLandingLevel pins the shape of a world nobody has dug down
// from: the landing level exists, and nothing else does.
func TestOneLayerAtTheLandingLevel(t *testing.T) {
	w := newTestWorld(t, DefaultConfig())
	if w.landing().Level != LandingLevel {
		t.Fatalf("the landing layer is level %d, want %d", w.landing().Level, LandingLevel)
	}
	if got := w.layer(LandingLevel); got != w.landing() {
		t.Fatalf("layer(LandingLevel) = %p, want the landing layer (%p)", got, w.landing())
	}
	for _, l := range []Level{SurfaceLevel, LandingLevel + 1, -1} {
		if w.layer(l) != nil {
			t.Errorf("layer(%d) exists; only the landing level should", l)
		}
	}
}

// TestNothingIsOnTheSurface runs a colony long enough to trade and checks that
// every place it names is on the landing level. The zero Point is on the
// surface, so a Point built without a level shows up here as level 0 instead
// of passing as right while the colony has one level. See docs/layers.md.
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
		if e := w.entities[id]; e.Pos.Level != LandingLevel {
			t.Errorf("entity %d (%v) is at %v, on level %d", id, e.Kind, e.Pos, e.Pos.Level)
		}
	}
	if len(w.books) == 0 {
		t.Fatal("no order book opened in 1200 ticks; pick a config that trades")
	}
	for k := range w.books {
		if k.Depot.Level != LandingLevel {
			t.Errorf("a %v book's depot %v is not on the landing level", k.Item, k.Depot)
		}
	}
	for id, o := range w.orders {
		if o.Depot.Level != LandingLevel {
			t.Errorf("order %d's depot %v is not on the landing level", id, o.Depot)
		}
	}
	w.eachContainer(func(c *StorageContainer) {
		if c.Pos.Level != LandingLevel {
			t.Errorf("a container records its position as %v", c.Pos)
		}
	})
}

func TestLessPointOrdersByLevelFirst(t *testing.T) {
	cases := []struct {
		a, b Point
		want bool
	}{
		{Point{9, 9, 1}, Point{0, 0, 2}, true}, // shallower first, wherever it is
		{Point{0, 0, 2}, Point{9, 9, 1}, false},
		{Point{5, 0, 1}, Point{0, 1, 1}, true}, // then row-major, as before levels
		{Point{0, 1, 1}, Point{5, 0, 1}, false},
		{Point{3, 3, 1}, Point{3, 3, 1}, false},
	}
	for _, c := range cases {
		if got := lessPoint(c.a, c.b); got != c.want {
			t.Errorf("lessPoint(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	if (Point{}).Level != SurfaceLevel {
		t.Error("the zero Point should be on the surface, so a missing level is visible")
	}
}

// TestNeighboursStayOnTheirLevel pins the geometry the rest of the engine
// leans on: Add stays on a level, Adjacent and Within never reach across one,
// and Chebyshev counts a level as one step.
func TestNeighboursStayOnTheirLevel(t *testing.T) {
	p := Point{5, 5, 1}
	if q := p.Add(1, -1); q.Level != 1 {
		t.Errorf("Add moved %v to level %d", p, q.Level)
	}
	below := Point{5, 5, 2}
	if p.Adjacent(below) || p.Within(below, 3) {
		t.Error("a tile straight below counts as adjacent or within range")
	}
	if !p.Within(Point{6, 6, 1}, 1) || !p.Adjacent(Point{6, 6, 1}) {
		t.Error("a diagonal neighbour on the same level does not count")
	}
	if d := p.Chebyshev(Point{8, 5, 3}); d != 5 {
		t.Errorf("Chebyshev across two levels = %d, want 3 across + 2 down = 5", d)
	}
}

// TestIndexRoundTrips checks the cell index every search queues by.
func TestIndexRoundTrips(t *testing.T) {
	w := newTestWorld(t, DefaultConfig())
	for _, p := range []Point{{0, 0, 1}, {w.Width - 1, w.Height - 1, 1}, {7, 3, 2}, {w.Width - 1, 0, 3}} {
		if got := w.pointOf(w.index(p)); got != p {
			t.Errorf("pointOf(index(%v)) = %v", p, got)
		}
	}
}
