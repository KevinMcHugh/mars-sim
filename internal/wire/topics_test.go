package wire

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/glyphs"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// A topic goes out when subscribed, then no more often than its interval, and
// only when its JSON changed; unsubscribed, it never goes.
func TestTopicsSendOnSubscribeThenOnlyChanges(t *testing.T) {
	snap := fixture(true)
	snap.AlienSpecies = []sim.AlienSpecies{{Singular: "grub", Plural: "grubs", Emoji: glyphs.Beetle, Eyes: 3}}
	tp := NewTopics()
	t0 := time.Unix(1000, 0)

	if got := tp.Due(snap, t0); got != nil {
		t.Fatalf("nothing subscribed, got %v", got)
	}
	if err := tp.Subscribe("lore"); err != nil {
		t.Fatal(err)
	}
	got := tp.Due(snap, t0)
	if _, ok := got["lore"]; !ok {
		t.Fatalf("first Due after Subscribe sent %v, want lore", got)
	}

	// Within the interval: nothing, even if it changed.
	changed := *snap
	changed.Seed = 99
	if got := tp.Due(&changed, t0.Add(100*time.Millisecond)); got != nil {
		t.Errorf("inside the interval, got %v", got)
	}
	// After it, but unchanged: nothing.
	if got := tp.Due(snap, t0.Add(2*time.Second)); got != nil {
		t.Errorf("unchanged, got %v", got)
	}
	// After it, changed: sent.
	if got := tp.Due(&changed, t0.Add(4*time.Second)); got["lore"] == nil {
		t.Errorf("changed after the interval, got %v", got)
	}

	// Resubscribing sends at once, unchanged or not.
	if err := tp.Subscribe("lore"); err != nil {
		t.Fatal(err)
	}
	if got := tp.Due(&changed, t0.Add(4*time.Second+time.Millisecond)); got["lore"] == nil {
		t.Errorf("resubscribe did not resend")
	}

	tp.Unsubscribe("lore")
	if got := tp.Due(snap, t0.Add(time.Hour)); got != nil {
		t.Errorf("after unsubscribe, got %v", got)
	}
	if err := tp.Subscribe("nope"); err == nil {
		t.Error("an unknown topic subscribed")
	}
}

// The lore topic carries what the TUI's lore tab shows: world facts, and each
// species with the glyph the map draws it as.
func TestLoreTopic(t *testing.T) {
	snap := fixture(true)
	snap.Stats.ExploredTiles, snap.Stats.ChunksGenerated, snap.Stats.Chunks = 500, 6, 6
	snap.AlienSpecies = []sim.AlienSpecies{
		{Singular: "grub", Plural: "grubs", Emoji: glyphs.Beetle, Limbs: 6, Arms: 2, Temperament: sim.TemperamentHostile},
		{Singular: "xeno", Plural: "xenos", Emoji: "\U0001F921"}, // not a listed glyph
	}
	tp := NewTopics()
	if err := tp.Subscribe("lore"); err != nil {
		t.Fatal(err)
	}
	var lore LoreTopic
	if err := json.Unmarshal(tp.Due(snap, time.Unix(0, 0))["lore"], &lore); err != nil {
		t.Fatal(err)
	}
	w := lore.World
	if w.Width != 150 || w.Height != 70 || w.Seed != 7 || !w.FogOfWar || w.ExploredTiles != 500 || w.Chunks != 6 {
		t.Errorf("world = %+v", w)
	}
	if len(lore.Species) != 2 {
		t.Fatalf("species = %+v", lore.Species)
	}
	g := lore.Species[0]
	if g.Glyph != glyphs.Beetle || g.Legs != 4 || g.Temperament != sim.TemperamentHostile.String() || g.Description == "" || g.Label == "" {
		t.Errorf("grub = %+v", g)
	}
	if lore.Species[1].Glyph != glyphs.Alien {
		t.Errorf("an unlisted emoji reached the page: %q", lore.Species[1].Glyph)
	}
}
