package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
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
	"lore":       {every: time.Second, build: loreTopic},
	"names":      {every: time.Second, build: namesTopic},
	"perf":       {every: chartEvery, build: func(s *sim.Snapshot) any { return perfTopic(s) }},
	"population": {every: chartEvery, build: func(s *sim.Snapshot) any { return populationTopic(s) }},
	"metrics":    {every: chartEvery, build: func(s *sim.Snapshot) any { return metricsTopic(s) }},
	"jobs":       {every: boardEvery, build: func(s *sim.Snapshot) any { return jobsTopic(s) }},
	"storage":    {every: boardEvery, build: func(s *sim.Snapshot) any { return storageTopic(s) }},
	"market":     {every: boardEvery, build: func(s *sim.Snapshot) any { return marketTopic(s) }},
	"roster":     {every: rosterEvery, build: func(s *sim.Snapshot) any { return rosterTopic(s, false, false) }},
	"zones":      {every: boardEvery, build: func(s *sim.Snapshot) any { return zonesTopic(s) }},
	"zoning":     {every: boardEvery, build: func(s *sim.Snapshot) any { return zoningTopic(s) }},
	"recruit":    {every: boardEvery, build: func(s *sim.Snapshot) any { return recruitTopic(s) }},
	// Every advance, not on an interval: placing happens paused, when the
	// only advance is the one a move command causes, and the page must see
	// that move. A handful of ships is nothing to rebuild.
	"ships": {every: 0, build: func(s *sim.Snapshot) any { return shipsTopic(s) }},
}

// ShipsTopic is the Ships tab: every colony ship's footprint, and whether
// they may still be landed and moved (only before the first tick; see
// sim.LandShip and sim.MoveShip).
type ShipsTopic struct {
	Placing bool       `json:"placing"`
	Ships   []ShipLine `json:"ships"`
}

// ShipLine is one ship: its id, its footprint's top-left, size, and shape,
// and how many came down in it. A ship still aloft has no position, and only
// the next one to land has a size and shape yet.
type ShipLine struct {
	ID int `json:"id"`
	X  int `json:"x"`
	Y  int `json:"y"`
	W  int `json:"w"`
	H  int `json:"h"`
	// Shape is the footprint row by row: '#' hull, '.' deck, ' ' outside
	// the ship. Kind names it: stick, hub-and-spoke, or cluster.
	Shape     []string `json:"shape,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Colonists int      `json:"colonists"`
	Aloft     bool     `json:"aloft,omitempty"`
}

func shipsTopic(s *sim.Snapshot) ShipsTopic {
	t := ShipsTopic{Placing: s.Tick == 0, Ships: make([]ShipLine, 0, len(s.Ships))}
	for _, sh := range s.Ships {
		t.Ships = append(t.Ships, ShipLine{ID: sh.ID, X: sh.X, Y: sh.Y, W: sh.Width, H: sh.Height,
			Shape: sh.Shape, Kind: sh.ShapeName, Colonists: sh.Colonists, Aloft: sh.Aloft})
	}
	return t
}

// namesTopic is every living colonist's name by id, for the map's hover
// readout: frames carry ids, not names. It changes only when someone arrives
// or dies, so after the first send it is almost never sent again.
func namesTopic(s *sim.Snapshot) any {
	names := map[string]string{}
	for _, e := range s.Entities {
		if e.Kind == sim.Colonist {
			names[strconv.FormatUint(uint64(e.ID), 10)] = entityName(e)
		}
	}
	return names
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
	if name == "log" {
		tp, ok = newLogTopic(), true
	}
	if !ok {
		tp, ok = paramTopic(name)
	}
	if !ok {
		return fmt.Errorf("unknown topic %q", name)
	}
	t.subs[name] = &topicState{tp: tp}
	return nil
}

// Restart is for a new game: every subscription starts over, as if just
// subscribed, so each is sent at once and the log sends its whole new ring.
func (t *Topics) Restart() {
	for name := range t.subs {
		_ = t.Subscribe(name) // it subscribed once, so it resolves again
	}
}

// Refresh makes every topic due at the next Due, whatever its interval, and
// still sends only those that changed. A command calls for it: the panel
// that sent it should show what it did. Paused, nothing else publishes, so
// a second order placed within the market topic's interval was applied but
// never shown until the game ran again.
func (t *Topics) Refresh() {
	for _, st := range t.subs {
		st.built = time.Time{}
	}
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
	World        LoreWorld         `json:"world"`
	Species      []LoreSpecies     `json:"species"`
	Guns         []LoreGun         `json:"guns"`
	Corporations []LoreCorporation `json:"corporations"`
}

// LoreGun is the make and model one kind of gun carries. See
// docs/arms-makers.md.
type LoreGun struct {
	Kind  string `json:"kind"`  // "pistol"
	Maker string `json:"maker"` // "MarsCorp"
	Model string `json:"model"` // "M-117"
}

// LoreCorporation is one rolled company. Description names the guns it makes.
type LoreCorporation struct {
	Name        string `json:"name"`
	HQ          string `json:"hq"`
	Founded     int    `json:"founded"`
	Description string `json:"description"`
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
	Label    string `json:"label"` // the roster label, "Xeno · hostile"
	Glyph    string `json:"glyph"` // what the map draws it as (glyphs.ForAlien)
	Singular string `json:"singular"`
	Plural   string `json:"plural"`
	// ScientificName is the species' binomial, "Pseudursus ares".
	ScientificName string `json:"scientificName"`
	Temperament    string `json:"temperament"`
	HeightMinCM    int    `json:"heightMinCm"`
	HeightMaxCM    int    `json:"heightMaxCm"`
	WeightMinKG    int    `json:"weightMinKg"`
	WeightMaxKG    int    `json:"weightMaxKg"`
	Eyes           int    `json:"eyes"`
	Limbs          int    `json:"limbs"`
	Arms           int    `json:"arms"`
	Legs           int    `json:"legs"`
	Tail           bool   `json:"tail"`
	Wings          bool   `json:"wings"`
	Skin           string `json:"skin"`
	Color          string `json:"color"`
	Pattern        string `json:"pattern"`
	Attacks        string `json:"attacks"` // "bite, claws, tail"
	BiteDamage     int    `json:"biteDamage"`
	BiteRest       int    `json:"biteRest"` // ticks between bites
	Slowness       int    `json:"slowness"` // ticks per step
	Description    string `json:"description"`
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
		Species:      make([]LoreSpecies, 0, len(s.AlienSpecies)),
		Guns:         make([]LoreGun, 0, len(s.GunModels)),
		Corporations: make([]LoreCorporation, 0, len(s.Corporations)),
	}
	for _, g := range s.GunModels {
		t.Guns = append(t.Guns, LoreGun{Kind: g.Kind.String(), Maker: g.Brand, Model: g.Model})
	}
	for i, c := range s.Corporations {
		t.Corporations = append(t.Corporations, LoreCorporation{
			Name: c.Name, HQ: c.HQ, Founded: c.Founded, Description: c.Description(i, s.GunModels),
		})
	}
	for _, sp := range s.AlienSpecies {
		t.Species = append(t.Species, LoreSpecies{
			Label:          sp.RosterLabel(),
			Glyph:          glyphs.ForAlien(sp),
			Singular:       sp.Singular,
			Plural:         sp.Plural,
			ScientificName: sp.ScientificName,
			Temperament:    sp.Temperament.String(),
			HeightMinCM:    sp.HeightMinCM,
			HeightMaxCM:    sp.HeightMaxCM,
			WeightMinKG:    sp.WeightMinKG,
			WeightMaxKG:    sp.WeightMaxKG,
			Eyes:           sp.Eyes,
			Limbs:          sp.Limbs,
			Arms:           sp.Arms,
			Legs:           sp.Legs(),
			Tail:           sp.Tail,
			Wings:          sp.Wings,
			Skin:           sp.Skin.String(),
			Color:          sp.Color,
			Pattern:        sp.Pattern.String(),
			Attacks:        sp.AttacksLabel(),
			BiteDamage:     sp.BiteDamage,
			BiteRest:       sp.BiteRest,
			Slowness:       sp.Slowness,
			Description:    sp.Description(),
		})
	}
	return t
}
