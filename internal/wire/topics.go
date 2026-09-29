package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/glyphs"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// Topics are the wire's second tier (see docs/wire-format.md): one panel's
// worth of data, as JSON, sent only while the page has that panel open. A
// frame carries what the map draws 60 times a second; a topic carries what a
// panel reads a few times a second at most, and costs nothing while nobody is
// looking at it.
//
// A topic is sent when it is first subscribed to, and after that at most once
// per its interval, and only if its JSON changed: a panel over a paused game
// receives nothing at all.

// topic is one kind of panel data: how often it may be rebuilt, and how.
type topic struct {
	every time.Duration
	build func(*sim.Snapshot) any
}

// topicTable is every topic a page can subscribe to by a fixed name. Topics
// with a parameter ("entity:12") are resolved by paramTopic.
var topicTable = map[string]topic{
	"lore": {every: time.Second, build: loreTopic},
}

// Topics tracks one page's subscriptions and what was last sent for each.
type Topics struct {
	subs map[string]*topicState
}

type topicState struct {
	tp    topic
	built time.Time // when it was last built; zero until the first build
	sent  []byte    // the JSON last sent, to skip an unchanged rebuild
}

// NewTopics returns an empty subscription set.
func NewTopics() *Topics { return &Topics{subs: map[string]*topicState{}} }

// Subscribe starts sending name. Subscribing again restarts it: the next Due
// sends it whatever changed, so a panel that reopens gets its data at once.
func (t *Topics) Subscribe(name string) error {
	tp, ok := topicTable[name]
	if !ok {
		tp, ok = paramTopic(name)
	}
	if !ok {
		return fmt.Errorf("unknown topic %q", name)
	}
	t.subs[name] = &topicState{tp: tp}
	return nil
}

// Unsubscribe stops sending name. Unsubscribing from something not
// subscribed is a no-op.
func (t *Topics) Unsubscribe(name string) { delete(t.subs, name) }

// Due builds the subscribed topics whose interval has passed at now, and
// returns those whose JSON differs from what was last sent (all of them, the
// first time), by name. It returns nil when there is nothing to send.
func (t *Topics) Due(snap *sim.Snapshot, now time.Time) map[string]json.RawMessage {
	if snap == nil {
		return nil
	}
	var out map[string]json.RawMessage
	for name, st := range t.subs {
		tp := st.tp
		if !st.built.IsZero() && now.Sub(st.built) < tp.every {
			continue
		}
		st.built = now
		b, err := json.Marshal(tp.build(snap))
		if err != nil {
			b, _ = json.Marshal(map[string]string{"error": err.Error()})
		}
		if st.sent != nil && bytes.Equal(b, st.sent) {
			continue
		}
		st.sent = b
		if out == nil {
			out = map[string]json.RawMessage{}
		}
		out[name] = b
	}
	return out
}

// ---- lore -----------------------------------------------------------------

// LoreTopic is the Lore panel: facts about the world, and every rolled alien
// species, as the TUI's lore tab shows them (internal/ui/tui/render_lore.go).
type LoreTopic struct {
	World   LoreWorld     `json:"world"`
	Species []LoreSpecies `json:"species"`
}

// LoreWorld is the world's size, how much of it the colony has explored and
// generated, and the seed that would rebuild it.
type LoreWorld struct {
	Width           int   `json:"width"`
	Height          int   `json:"height"`
	FogOfWar        bool  `json:"fogOfWar"`
	ExploredTiles   int   `json:"exploredTiles"`
	ChunksGenerated int   `json:"chunksGenerated"`
	Chunks          int   `json:"chunks"`
	Seed            int64 `json:"seed"`
}

// LoreSpecies is one rolled alien species. See docs/lore.md.
type LoreSpecies struct {
	Label       string `json:"label"` // the roster label, "Xeno · hostile"
	Glyph       string `json:"glyph"` // what the map draws it as (glyphs.ForAlien)
	Singular    string `json:"singular"`
	Plural      string `json:"plural"`
	Temperament string `json:"temperament"`
	HeightMinCM int    `json:"heightMinCm"`
	HeightMaxCM int    `json:"heightMaxCm"`
	WeightMinKG int    `json:"weightMinKg"`
	WeightMaxKG int    `json:"weightMaxKg"`
	Eyes        int    `json:"eyes"`
	Limbs       int    `json:"limbs"`
	Arms        int    `json:"arms"`
	Legs        int    `json:"legs"`
	Tail        bool   `json:"tail"`
	Skin        string `json:"skin"`
	Color       string `json:"color"`
	Pattern     string `json:"pattern"`
	BiteDamage  int    `json:"biteDamage"`
	BiteRest    int    `json:"biteRest"` // ticks between bites
	Slowness    int    `json:"slowness"` // ticks per step
	Description string `json:"description"`
}

func loreTopic(s *sim.Snapshot) any {
	t := LoreTopic{
		World: LoreWorld{
			Width:           s.Width,
			Height:          s.Height,
			FogOfWar:        s.FogOfWar,
			ExploredTiles:   s.Stats.ExploredTiles,
			ChunksGenerated: s.Stats.ChunksGenerated,
			Chunks:          s.Stats.Chunks,
			Seed:            s.Seed,
		},
		Species: make([]LoreSpecies, 0, len(s.AlienSpecies)),
	}
	for _, sp := range s.AlienSpecies {
		t.Species = append(t.Species, LoreSpecies{
			Label:       sp.RosterLabel(),
			Glyph:       glyphs.ForAlien(sp),
			Singular:    sp.Singular,
			Plural:      sp.Plural,
			Temperament: sp.Temperament.String(),
			HeightMinCM: sp.HeightMinCM,
			HeightMaxCM: sp.HeightMaxCM,
			WeightMinKG: sp.WeightMinKG,
			WeightMaxKG: sp.WeightMaxKG,
			Eyes:        sp.Eyes,
			Limbs:       sp.Limbs,
			Arms:        sp.Arms,
			Legs:        sp.Legs(),
			Tail:        sp.Tail,
			Skin:        sp.Skin.String(),
			Color:       sp.Color,
			Pattern:     sp.Pattern.String(),
			BiteDamage:  sp.BiteDamage,
			BiteRest:    sp.BiteRest,
			Slowness:    sp.Slowness,
			Description: sp.Description(),
		})
	}
	return t
}
