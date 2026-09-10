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

// A claimed tile is not handed to a second colonist, and releasing frees it.
func TestBoardClaimIsExclusive(t *testing.T) {
	w := roomsTestWorld(40, 24)
	carve(w, Point{5, 5}, Point{9, 7}, Floor)
	w.refreshSpatial()

	from := Point{7, 6}
	room := w.roomOf(from)
	if room == 0 {
		t.Fatal("expected a room at the carved area")
	}

	first, ok := w.board.claimNearestMine(from, room)
	if !ok {
		t.Fatal("expected to claim a mine job")
	}
	second, ok := w.board.claimNearestMine(from, room)
	if !ok {
		t.Fatal("expected a second, different mine job")
	}
	if first == second {
		t.Fatalf("two claims returned the same tile %v", first)
	}

	w.board.releaseMine(first)
	again, ok := w.board.claimNearestMine(from, room)
	if !ok || again != first {
		t.Fatalf("released tile should be re-claimable: got %v ok=%v want %v", again, ok, first)
	}
}

// A colonist cannot claim frontier that is only reachable from another room.
func TestBoardClaimRespectsRooms(t *testing.T) {
	w := roomsTestWorld(48, 24)
	carve(w, Point{3, 5}, Point{5, 7}, Floor)   // room A
	carve(w, Point{20, 5}, Point{22, 7}, Floor) // room B, far away
	w.refreshSpatial()

	roomA := w.roomOf(Point{4, 6})
	roomB := w.roomOf(Point{21, 6})
	if roomA == 0 || roomB == 0 || roomA == roomB {
		t.Fatalf("expected two distinct rooms, got A=%d B=%d", roomA, roomB)
	}

	// Every tile claimed from room A must border room A, never room B's frontier.
	for i := 0; i < 20; i++ {
		target, ok := w.board.claimNearestMine(Point{4, 6}, roomA)
		if !ok {
			break
		}
		if !w.board.reachableFrom(target, roomA) {
			t.Fatalf("claimed %v not reachable from room A", target)
		}
		if w.board.reachableFrom(target, roomB) && !w.board.reachableFrom(target, roomA) {
			t.Fatalf("claimed room B frontier %v from room A", target)
		}
	}
}
