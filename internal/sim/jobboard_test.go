package sim

import (
	"math/rand"
	"testing"
)

// bruteFrontier computes the true set of mineable rock tiles (rock bordering
// floor) by scanning the whole grid — the ground truth the board must match.
func bruteFrontier(w *World) map[Point]bool {
	want := make(map[Point]bool)
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			p := Point{x, y}
			if w.TerrainAt(p) == Rock && w.bordersFloor(p) {
				want[p] = true
			}
		}
	}
	return want
}

func assertFrontierMatches(t *testing.T, w *World, when string) {
	t.Helper()
	want := bruteFrontier(w)
	if len(w.board.frontier) != len(want) {
		t.Fatalf("%s: frontier size %d, want %d", when, len(w.board.frontier), len(want))
	}
	for p := range want {
		if _, ok := w.board.frontier[p]; !ok {
			t.Fatalf("%s: frontier missing %v", when, p)
		}
	}
}

// The event-driven frontier must always equal a full rescan, across many random
// terrain edits (digging and filling).
func TestFrontierMatchesBruteForce(t *testing.T) {
	w := roomsTestWorld(64, 48)
	rng := rand.New(rand.NewSource(11))

	for i := 0; i < 1500; i++ {
		w.SetTerrain(Point{rng.Intn(w.Width), rng.Intn(w.Height)}, Floor)
	}
	assertFrontierMatches(t, w, "after carving")

	for step := 0; step < 400; step++ {
		p := Point{rng.Intn(w.Width), rng.Intn(w.Height)}
		switch rng.Intn(3) {
		case 0:
			w.SetTerrain(p, Floor)
		case 1:
			w.SetTerrain(p, Rock)
		default:
			w.SetTerrain(p, Wall)
		}
		assertFrontierMatches(t, w, "after edit")
	}
}

// Claims are owner-keyed: releasing only affects the owner's claim, and the
// unclaimed count reflects outstanding claims.
func TestBoardClaimOwnership(t *testing.T) {
	w := roomsTestWorld(40, 24)
	carve(w, Point{5, 5}, Point{9, 7}, Floor)
	w.refreshSpatial()

	// Two distinct frontier rock tiles bordering the carved room.
	var a, b Point
	found := 0
	for p := range w.board.frontier {
		if found == 0 {
			a, found = p, 1
		} else {
			b, found = p, 2
			break
		}
	}
	if found < 2 {
		t.Fatalf("expected at least two frontier tiles, got %d", found)
	}

	base := w.board.unclaimedCount()
	w.board.claimMine(a, 1)
	w.board.claimMine(b, 2)
	if got := w.board.unclaimedCount(); got != base-2 {
		t.Fatalf("unclaimed count after two claims: got %d want %d", got, base-2)
	}
	if !w.board.isClaimed(a) || !w.board.isClaimed(b) {
		t.Fatal("claimed tiles should report claimed")
	}

	// A non-owner release is a no-op; the owner release frees it.
	w.board.releaseMine(a, 2)
	if !w.board.isClaimed(a) {
		t.Fatal("non-owner release should not free the claim")
	}
	w.board.releaseMine(a, 1)
	if w.board.isClaimed(a) {
		t.Fatal("owner release should free the claim")
	}
	if got := w.board.unclaimedCount(); got != base-1 {
		t.Fatalf("unclaimed count after one release: got %d want %d", got, base-1)
	}
}

// The frontier field only routes to unclaimed rock: claiming every frontier tile
// makes the field unreachable everywhere, so colonists know to stop mining.
func TestFrontierFieldExcludesClaimed(t *testing.T) {
	w := roomsTestWorld(40, 24)
	carve(w, Point{5, 5}, Point{9, 7}, Floor)
	w.refreshSpatial()

	inside := Point{7, 6}
	if w.frontierField().at(inside) < 0 {
		t.Fatal("frontier should be reachable before anything is claimed")
	}
	for p := range w.board.frontier {
		w.board.claimMine(p, 1)
	}
	w.tick++ // fields rebuild at most once per tick; advance so the claim lands
	if got := w.frontierField().at(inside); got >= 0 {
		t.Fatalf("with all frontier claimed the field should be unreachable, got %d", got)
	}
}
