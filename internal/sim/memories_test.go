package sim

import (
	"math/rand"
	"testing"
)

func TestRememberKeepsRecentMemories(t *testing.T) {
	cfg := DefaultConfig()
	w := newWorld(cfg, rand.New(rand.NewSource(1)))
	col := newEntity(1, Colonist, Point{}, cfg)
	w.entities[col.ID] = col

	for i := 0; i < maxColonistMemories+3; i++ {
		w.tick = i
		w.remember(col, "event")
	}
	if got, want := len(col.Memories), maxColonistMemories; got != want {
		t.Fatalf("memory count = %d, want %d", got, want)
	}
	if got, want := col.Memories[0].Tick, 3; got != want {
		t.Fatalf("oldest retained tick = %d, want %d", got, want)
	}
}

func TestSnapshotCopiesMemories(t *testing.T) {
	cfg := DefaultConfig()
	w := newWorld(cfg, rand.New(rand.NewSource(1)))
	col := newEntity(1, Colonist, Point{}, cfg)
	w.entities[col.ID] = col
	w.remember(col, "a meal")

	snap := w.snapshot(false, 1)
	snap.Entities[0].Memories[0].Text = "mutated"
	if got := col.Memories[0].Text; got != "a meal" {
		t.Fatalf("snapshot mutation changed live memory to %q", got)
	}
}
