package wire

import (
	"encoding/json"
	"strings"
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
	snap.Corporations = []sim.Corporation{{Name: "MarsCorp", Code: "M", HQ: "Phobos", Founded: 2090}}
	snap.GunModels = []sim.GunModel{{Kind: sim.Shotgun, Maker: 0, Brand: "MarsCorp", Model: "M-117"}}
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
	if want := (LoreGun{Kind: "shotgun", Maker: "MarsCorp", Model: "M-117"}); len(lore.Guns) != 1 || lore.Guns[0] != want {
		t.Errorf("guns = %+v, want [%+v]", lore.Guns, want)
	}
	if len(lore.Corporations) != 1 || lore.Corporations[0].Name != "MarsCorp" || !strings.Contains(lore.Corporations[0].Description, "M-117 shotgun") {
		t.Errorf("corporations = %+v", lore.Corporations)
	}
}

// The ships topic lists every ship's footprint, and says they may be moved
// only before the first tick.
func TestShipsTopic(t *testing.T) {
	snap := fixture(true)
	snap.Tick = 0
	snap.Ships = []sim.ShipView{{ID: 1, X: 10, Y: 20, Width: 25, Height: 6, Colonists: 20}}
	tp := NewTopics()
	if err := tp.Subscribe("ships"); err != nil {
		t.Fatal(err)
	}
	var ships ShipsTopic
	if err := json.Unmarshal(tp.Due(snap, time.Unix(0, 0))["ships"], &ships); err != nil {
		t.Fatal(err)
	}
	want := ShipLine{ID: 1, X: 10, Y: 20, W: 25, H: 6, Colonists: 20}
	if !ships.Placing || len(ships.Ships) != 1 || ships.Ships[0] != want {
		t.Fatalf("ships = %+v", ships)
	}
	snap.Tick = 1
	if shipsTopic(snap).Placing {
		t.Fatal("ships may still be placed after the first tick")
	}
}
