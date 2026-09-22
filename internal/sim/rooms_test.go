package sim

import (
	"math/rand"
	"testing"
)

func roomsTestWorld(w, h int) *World {
	cfg := DefaultConfig()
	cfg.Width, cfg.Height = w, h
	cfg.TraitChance = 0 // baseline colonists for deterministic mechanics tests
	return newWorld(cfg, rand.New(rand.NewSource(1)))
}

func carve(w *World, from, to Point, t Terrain) {
	for y := from.Y; y <= to.Y; y++ {
		for x := from.X; x <= to.X; x++ {
			w.SetTerrain(Point{x, y}, t)
		}
	}
}

// bruteRoomCount counts connected floor components with a global flood fill —
// the ground truth the two-level maintenance must match.
func bruteRoomCount(w *World) int {
	seen := make([]bool, len(w.tiles))
	count := 0
	var stack []Point
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			i := y*w.Width + x
			if w.tiles[i].Terrain != Floor || seen[i] {
				continue
			}
			count++
			stack = append(stack[:0], Point{x, y})
			seen[i] = true
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				for _, d := range neighbors8 {
					q := p.Add(d.X, d.Y)
					if !w.InBounds(q) {
						continue
					}
					j := w.index(q)
					if w.tiles[j].Terrain == Floor && !seen[j] {
						seen[j] = true
						stack = append(stack, q)
					}
				}
			}
		}
	}
	return count
}

// Two separated caverns are two rooms; connecting them merges to one; walling
// the connection splits them back to two.
func TestRoomsMergeAndSplit(t *testing.T) {
	w := roomsTestWorld(40, 24)
	carve(w, Point{2, 5}, Point{4, 5}, Floor)  // room A
	carve(w, Point{8, 5}, Point{10, 5}, Floor) // room B
	w.refreshSpatial()
	if w.roomCount != 2 {
		t.Fatalf("expected 2 rooms, got %d", w.roomCount)
	}

	carve(w, Point{5, 5}, Point{7, 5}, Floor) // connect A-B
	w.refreshSpatial()
	if w.roomCount != 1 {
		t.Fatalf("expected 1 room after connecting, got %d", w.roomCount)
	}
	if !w.sameRoom(Point{2, 5}, Point{10, 5}) {
		t.Fatal("connected tiles should be in the same room")
	}

	w.SetTerrain(Point{6, 5}, Wall) // split
	w.refreshSpatial()
	if w.roomCount != 2 {
		t.Fatalf("expected 2 rooms after walling, got %d", w.roomCount)
	}
	if w.sameRoom(Point{2, 5}, Point{10, 5}) {
		t.Fatal("split tiles should be in different rooms")
	}
}

// A corridor that straddles a chunk border (x=16) must still read as one room,
// and walling the border tile must split it — exercising cross-chunk linking.
func TestRoomsCrossChunkBoundary(t *testing.T) {
	w := roomsTestWorld(48, 24)
	carve(w, Point{10, 8}, Point{25, 8}, Floor) // spans chunks 0 and 1 in x
	w.refreshSpatial()
	if w.roomCount != 1 {
		t.Fatalf("corridor across a chunk border should be 1 room, got %d", w.roomCount)
	}
	if !w.sameRoom(Point{10, 8}, Point{25, 8}) {
		t.Fatal("cells across the chunk border should share a room")
	}

	w.SetTerrain(Point{16, 8}, Wall) // wall exactly on the chunk boundary
	w.refreshSpatial()
	if w.roomCount != 2 {
		t.Fatalf("walling the border tile should split into 2 rooms, got %d", w.roomCount)
	}
}

// Incremental maintenance must always agree with a full flood fill, across many
// random edits.
func TestRoomsIncrementalMatchesBruteForce(t *testing.T) {
	w := roomsTestWorld(64, 48)
	rng := rand.New(rand.NewSource(7))

	// Seed a random floor layout.
	for i := 0; i < 1200; i++ {
		p := Point{rng.Intn(w.Width), rng.Intn(w.Height)}
		w.SetTerrain(p, Floor)
	}
	w.refreshSpatial()
	if got, want := w.roomCount, bruteRoomCount(w); got != want {
		t.Fatalf("initial layout: roomCount %d, brute %d", got, want)
	}

	// Apply random incremental edits and re-check against ground truth.
	for step := 0; step < 200; step++ {
		p := Point{rng.Intn(w.Width), rng.Intn(w.Height)}
		if rng.Intn(2) == 0 {
			w.SetTerrain(p, Floor)
		} else {
			w.SetTerrain(p, Rock)
		}
		w.refreshSpatial()
		if got, want := w.roomCount, bruteRoomCount(w); got != want {
			t.Fatalf("edit %d at %v: roomCount %d, brute %d", step, p, got, want)
		}
	}
}

// The default starting cavern is a single room.
func TestStartingCavernIsOneRoom(t *testing.T) {
	eng := NewEngine(DefaultConfig())
	if eng.world.roomCount != 1 {
		t.Fatalf("starting cavern should be 1 room, got %d", eng.world.roomCount)
	}
}

// mainRoom always tracks the room with the most floor tiles, ties broken by
// the smaller RoomID for determinism — not whichever room happened to exist
// first or was discovered first in map iteration order (which Go randomizes).
// See updateDisconnected, which every colonist is checked against.
func TestMainRoomTracksLargestRoom(t *testing.T) {
	w := roomsTestWorld(40, 24)
	carve(w, Point{2, 5}, Point{3, 5}, Floor)   // small room: 2 tiles
	carve(w, Point{10, 5}, Point{19, 5}, Floor) // big room: 10 tiles
	w.refreshSpatial()
	if w.roomCount != 2 {
		t.Fatalf("expected 2 rooms, got %d", w.roomCount)
	}
	big := w.roomOf(Point{10, 5})
	if w.mainRoom != big {
		t.Fatalf("mainRoom = %d, want the bigger room %d", w.mainRoom, big)
	}

	// Growing the small room past the big one flips which is main.
	carve(w, Point{2, 6}, Point{3, 15}, Floor) // +20 tiles onto the small room
	w.refreshSpatial()
	small := w.roomOf(Point{2, 5})
	if w.mainRoom != small {
		t.Fatalf("mainRoom = %d, want the now-bigger room %d", w.mainRoom, small)
	}

	// Walling the small room back off it entirely must never leave mainRoom
	// pointing at a room that no longer exists. Re-derive the survivor's
	// RoomID rather than reusing `big`: recomputing the dirty chunk can
	// reassign fresh RegionIDs to an untouched room's portion that shares it,
	// so a room's own ID is not guaranteed stable across an unrelated edit —
	// only which physical room mainRoom names is.
	carve(w, Point{2, 5}, Point{3, 15}, Wall)
	w.refreshSpatial()
	if stillBig := w.roomOf(Point{10, 5}); w.mainRoom != stillBig {
		t.Fatalf("mainRoom = %d after removing the bigger room, want %d", w.mainRoom, stillBig)
	}
}

// A colonist whose room is cut off from mainRoom accumulates disconnected
// ticks every turn regardless of what else it is doing, and the counter
// resets the instant it reconnects (see updateDisconnected).
func TestUpdateDisconnectedTracksCutoffRoom(t *testing.T) {
	w := roomsTestWorld(40, 24)
	carve(w, Point{10, 5}, Point{19, 5}, Floor) // the main room: 10 tiles
	pocket := Point{2, 5}
	w.SetTerrain(pocket, Floor) // isolated: 1 tile, well short of the main room
	w.refreshSpatial()
	if w.roomOf(pocket) == w.mainRoom {
		t.Fatal("test setup did not isolate the pocket")
	}

	e := w.spawn(Colonist, pocket)
	for i := 1; i <= 3; i++ {
		w.updateDisconnected(e)
		if e.disconnectedTicks != i {
			t.Fatalf("after %d calls, disconnectedTicks = %d, want %d", i, e.disconnectedTicks, i)
		}
	}

	// Connect the pocket to the main room; the counter must reset immediately.
	carve(w, Point{3, 5}, Point{9, 5}, Floor)
	w.refreshSpatial()
	if w.roomOf(pocket) != w.mainRoom {
		t.Fatal("test setup did not reconnect the pocket")
	}
	w.updateDisconnected(e)
	if e.disconnectedTicks != 0 {
		t.Fatalf("disconnectedTicks = %d after reconnecting, want 0", e.disconnectedTicks)
	}
}
