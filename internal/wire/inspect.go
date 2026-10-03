package wire

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/glyphs"
	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The inspector's topics: one creature ("entity:<id>") or one tile
// ("tile:<x>,<y>"), with what the TUI's inspectors show
// (internal/ui/tui/render_roster.go, renderCursorInspector). They take a
// parameter, so they are resolved by prefix rather than listed in topicTable.

// inspectEvery is how often an inspector is rebuilt: often enough that a
// health bar moves while you watch, and one creature is cheap to build.
const inspectEvery = 250 * time.Millisecond

// paramTopic resolves "entity:<id>" and "tile:<x>,<y>", and the other
// topics that take a parameter: roster:, account:, book: and order:.
func paramTopic(name string) (topic, bool) {
	kind, arg, ok := strings.Cut(name, ":")
	if !ok {
		return topic{}, false
	}
	switch kind {
	case "entity":
		id, err := strconv.ParseUint(arg, 10, 64)
		if err != nil {
			return topic{}, false
		}
		return topic{every: inspectEvery, build: func(s *sim.Snapshot) any { return entityTopic(s, sim.EntityID(id)) }}, true
	case "tile":
		p, ok := parsePoint(arg)
		if !ok {
			return topic{}, false
		}
		return topic{every: inspectEvery, build: func(s *sim.Snapshot) any { return tileTopic(s, p) }}, true
	case "roster":
		return rosterParam(arg)
	case "account":
		return accountParam(arg)
	case "book":
		return bookParam(arg)
	case "order":
		return orderParam(arg)
	}
	return topic{}, false
}

// ---- entity ---------------------------------------------------------------

// EntityTopic is one creature, living or dead. Found is false once it is
// gone without a record: a rat that died and has aged out of the graveyard.
type EntityTopic struct {
	Found bool   `json:"found"`
	ID    uint64 `json:"id"`
	Kind  string `json:"kind"`
	Glyph string `json:"glyph"`
	// Look is Glyph in the colonist's own skin and hair (see RosterRow.Look).
	Look  glyphs.Look `json:"look,omitempty"`
	Name  string      `json:"name"`
	X     int         `json:"x"`
	Y     int         `json:"y"`
	State string      `json:"state"`
	Focus string      `json:"focus"`
	HP    int         `json:"hp"`
	MaxHP int         `json:"maxHp"`
	// Parts are the body parts it has (a zero-max part is one it lacks, and
	// is left out). Colonists and aliens only.
	Parts []PartHP `json:"parts"`
	Dead  bool     `json:"dead"`
	// DiedTick and Cause are set when Dead; X and Y are then where it died.
	DiedTick int    `json:"diedTick,omitempty"`
	Cause    string `json:"cause,omitempty"`
	// Species is an alien's roster label ("🦗 Bug · hostile").
	Species string `json:"species,omitempty"`
	// Colonist is everything a colonist has that other creatures do not.
	Colonist *ColonistDetail `json:"colonist,omitempty"`
}

// PartHP is one body part's health.
type PartHP struct {
	Name string `json:"name"`
	HP   int    `json:"hp"`
	Max  int    `json:"max"`
}

// ColonistDetail is the colonist half of the inspector.
type ColonistDetail struct {
	Pronouns    string `json:"pronouns"`
	Orientation string `json:"orientation"`
	Age         int    `json:"age"`
	Height      string `json:"height"` // feet and inches, as the TUI shows it first
	HeightCM    int    `json:"heightCm"`
	WeightKG    int    `json:"weightKg"`
	Skin        string `json:"skin"`
	Hair        string `json:"hair"`
	Wallet      int64  `json:"wallet"`
	// Mood is the three affect axes, each in [-MoodMax, MoodMax], and the
	// label they resolve to.
	Mood      Mood        `json:"mood"`
	Needs     []NeedLevel `json:"needs"`
	Inventory []Stack     `json:"inventory"` // non-empty slots only
	Slots     int         `json:"slots"`
	Traits    []TraitInfo `json:"traits"`
	// Skills are those it has a rank in, in skill order; Profession is the
	// one it's known for ("" for none yet) and ProfessionLabel its title in
	// it, like "journeyman smith" (docs/skills.md).
	Skills          []SkillLevel `json:"skills"`
	Profession      string       `json:"profession,omitempty"`
	ProfessionLabel string       `json:"professionLabel,omitempty"`
	// Backstory is its one-line past, "Worked as a drill operator for
	// MarsCorp." ("" for none). Flavor only (docs/arms-makers.md).
	Backstory   string     `json:"backstory,omitempty"`
	Family      []Kin      `json:"family"`
	Affinities  []Acquaint `json:"affinities"`
	AffinityMax int        `json:"affinityMax"`
	// Memories are newest first.
	Memories []MemoryLine `json:"memories"`
}

// Mood is a colonist's affect.
type Mood struct {
	Charge  int    `json:"charge"`
	Grip    int    `json:"grip"`
	Valence int    `json:"valence"`
	Label   string `json:"label"`
	Max     int    `json:"max"`
}

// NeedLevel is one need's bar. Fatal needs kill at Max.
type NeedLevel struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
	Max   int    `json:"max"`
	Fatal bool   `json:"fatal"`
}

// Stack is some number of one item, in a slot.
type Stack struct {
	Slot  int    `json:"slot"` // 1-based, as the TUI numbers them
	Item  string `json:"item"`
	Count int    `json:"count"`
}

// TraitInfo is a trait and its one-line description.
type TraitInfo struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// SkillLevel is one skill's rank, out of the skill's top rank, its label at
// that rank, and the practice (base work ticks) behind it.
type SkillLevel struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Rank     int    `json:"rank"`
	MaxRank  int    `json:"maxRank"`
	Practice uint32 `json:"practice"`
}

// Kin is a family tie; ID is the relative, for the page to inspect next.
type Kin struct {
	Relation string `json:"relation"`
	ID       uint64 `json:"id"`
	Name     string `json:"name"`
}

// Acquaint is an affinity toward another colonist, in
// [-AffinityMax, AffinityMax].
type Acquaint struct {
	ID    uint64 `json:"id"`
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// MemoryLine is one memory. Count > 1 is a run of the same event from Tick
// to LastTick (docs/memories.md).
type MemoryLine struct {
	Tick     int    `json:"tick"`
	LastTick int    `json:"lastTick"`
	Count    int    `json:"count"`
	Text     string `json:"text"`
}

// findEntity looks for id among the living, then dead colonists, then the
// graveyard (dead creatures of other kinds), as the TUI's roster does.
func findEntity(s *sim.Snapshot, id sim.EntityID) (sim.EntityView, bool) {
	for i := range s.Entities {
		if s.Entities[i].ID == id {
			return s.Entities[i], true
		}
	}
	if e, ok := s.Deceased[id]; ok {
		return e, true
	}
	for i := len(s.Graveyard) - 1; i >= 0; i-- {
		if s.Graveyard[i].ID == id {
			return s.Graveyard[i], true
		}
	}
	return sim.EntityView{}, false
}

func entityTopic(s *sim.Snapshot, id sim.EntityID) EntityTopic {
	e, ok := findEntity(s, id)
	if !ok {
		return EntityTopic{ID: uint64(id)}
	}
	t := EntityTopic{
		Found: true,
		ID:    uint64(e.ID),
		Kind:  e.Kind.String(),
		Glyph: glyphs.ForEntity(e),
		Look:  glyphs.ForEntityLook(e),
		Name:  entityName(e),
		X:     e.Pos.X,
		Y:     e.Pos.Y,
		State: e.State.String(),
		Focus: e.Focus.String(),
		HP:    e.HP,
		MaxHP: e.MaxHP,
		Parts: []PartHP{},
		Dead:  e.Dead,
	}
	if e.Dead {
		t.DiedTick, t.Cause = e.DiedTick, e.Cause
	}
	for i, hp := range e.Parts {
		if e.MaxParts[i] > 0 {
			t.Parts = append(t.Parts, PartHP{Name: sim.BodyPart(i).Short(), HP: hp, Max: e.MaxParts[i]})
		}
	}
	if e.Kind == sim.Alien {
		t.Species = e.AlienSpecies.RosterLabel()
	}
	if p := e.Profile; p != nil {
		t.Colonist = colonistDetail(s, e, p)
	}
	return t
}

func colonistDetail(s *sim.Snapshot, e sim.EntityView, p *sim.Profile) *ColonistDetail {
	c := &ColonistDetail{
		Pronouns:    p.Gender.Pronouns(),
		Orientation: p.Orientation.String(),
		Age:         p.Age,
		Height:      sim.FormatHeight(p.HeightCM),
		HeightCM:    p.HeightCM,
		WeightKG:    p.WeightKG,
		Skin:        p.SkinTone.String(),
		Hair:        p.HairColor.String(),
		Wallet:      int64(e.Wallet),
		Mood:        Mood{Charge: e.Charge, Grip: e.Grip, Valence: e.Valence, Label: e.MoodLabel, Max: s.MoodMax},
		Needs:       make([]NeedLevel, 0, len(e.Needs)),
		Inventory:   []Stack{},
		Slots:       len(e.Inventory),
		Traits:      make([]TraitInfo, 0, len(p.Traits)),
		Skills:      make([]SkillLevel, 0, len(e.Skills)),
		Family:      make([]Kin, 0, len(e.Relations)),
		Affinities:  make([]Acquaint, 0, len(e.Affinities)),
		AffinityMax: s.AffinityMax,
		Memories:    make([]MemoryLine, 0, len(e.Memories)),
	}
	for i, v := range e.Needs {
		m := s.NeedsMeta[i]
		c.Needs = append(c.Needs, NeedLevel{Name: m.Name, Value: v, Max: m.Max, Fatal: m.Fatal})
	}
	for i, st := range e.Inventory {
		if st.Count > 0 {
			c.Inventory = append(c.Inventory, Stack{Slot: i + 1, Item: st.Kind.String(), Count: st.Count})
		}
	}
	for _, tr := range p.Traits {
		c.Traits = append(c.Traits, TraitInfo{Name: tr.Name(), Desc: tr.Desc()})
	}
	for _, sk := range e.Skills {
		c.Skills = append(c.Skills, SkillLevel{Name: sk.Skill.String(), Label: sk.Label, Rank: sk.Rank, MaxRank: sk.MaxRank, Practice: sk.Practice})
	}
	if e.Profession != sim.SkillNone {
		c.Profession, c.ProfessionLabel = e.Profession.String(), e.ProfessionLabel
	}
	c.Backstory = e.Backstory
	if len(e.Relations) > 0 || len(e.Affinities) > 0 {
		names := colonistNames(s)
		for _, r := range e.Relations {
			c.Family = append(c.Family, Kin{Relation: r.Kind.String(), ID: uint64(r.Other), Name: names[r.Other]})
		}
		for _, a := range e.Affinities {
			c.Affinities = append(c.Affinities, Acquaint{ID: uint64(a.Other), Name: names[a.Other], Value: a.Value})
		}
	}
	for i := len(e.Memories) - 1; i >= 0; i-- {
		m := e.Memories[i]
		c.Memories = append(c.Memories, MemoryLine{Tick: m.Tick, LastTick: m.LastTick, Count: m.Count, Text: m.Text})
	}
	return c
}

// entityName is a colonist's name, or "alien #12" for anything without one.
func entityName(e sim.EntityView) string {
	if e.Profile != nil && e.Profile.Name != "" {
		return e.Profile.Name
	}
	return fmt.Sprintf("%s #%d", e.Kind, e.ID)
}

// colonistNames names every colonist, living or dead, so a family tree that
// reaches a dead relative still names them.
func colonistNames(s *sim.Snapshot) map[sim.EntityID]string {
	names := make(map[sim.EntityID]string, len(s.Deceased)+len(s.Entities))
	for _, e := range s.Deceased {
		names[e.ID] = entityName(e)
	}
	for _, e := range s.Entities {
		if e.Kind == sim.Colonist {
			names[e.ID] = entityName(e)
		}
	}
	return names
}

// ---- tile -----------------------------------------------------------------

// TileTopic is one tile. On an unexplored tile only X, Y and Explored are
// set: the inspector must not hand back what the fog hides.
type TileTopic struct {
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Explored bool   `json:"explored"`
	Terrain  string `json:"terrain,omitempty"` // a rock's composition, for rock
	Glyph    string `json:"glyph,omitempty"`
	// Fixture is who owns the facility here and who may use it.
	Fixture *FixtureInfo `json:"fixture,omitempty"`
	Storage *StorageInfo `json:"storage,omitempty"`
	// Creatures are those standing here, for the page to inspect.
	Creatures []Creature `json:"creatures"`
}

// FixtureInfo is a facility's ownership. Owner is a display label;
// OwnerID is set when the owner is a colonist.
type FixtureInfo struct {
	Owner   string `json:"owner"`
	OwnerID uint64 `json:"ownerId,omitempty"`
	Access  string `json:"access"`
	Price   int64  `json:"price,omitempty"` // per use, for paid access
}

// StorageInfo is a container's contents and whose they are.
type StorageInfo struct {
	Label    string       `json:"label"` // chest, pantry, a locker, a workshop
	Used     int          `json:"used"`
	Slots    int          `json:"slots"`
	Items    int          `json:"items"`
	Capacity int          `json:"capacity"`
	Contents []Stack      `json:"contents"`
	Ledger   []LedgerItem `json:"ledger"`
}

// LedgerItem is how many of an item one owner holds in a container.
type LedgerItem struct {
	Owner string `json:"owner"`
	Item  string `json:"item"`
	Count int    `json:"count"`
}

// Creature is one creature on a tile.
type Creature struct {
	ID    uint64      `json:"id"`
	Glyph string      `json:"glyph"`
	Look  glyphs.Look `json:"look,omitempty"`
	Name  string      `json:"name"`
	State string      `json:"state"`
}

func tileTopic(s *sim.Snapshot, p sim.Point) TileTopic {
	t := TileTopic{X: p.X, Y: p.Y, Creatures: []Creature{}}
	if !s.ExploredAt(p) {
		return t
	}
	t.Explored = true
	tile := s.TileAt(p)
	if tile.Terrain == sim.Rock {
		t.Terrain = tile.Composition.String()
	} else {
		t.Terrain = tile.Terrain.String()
	}
	if g := glyphs.ForTerrain(tile.Terrain); !glyphs.Swatch(g) {
		t.Glyph = g
	}
	if f, ok := s.FixtureAt(p); ok {
		info := &FixtureInfo{Owner: ownerLabel(s, f.Owner), Access: f.Access.String()}
		if f.Owner.Kind == sim.OwnerColonist {
			info.OwnerID = uint64(f.Owner.ID)
		}
		if f.Access == sim.AccessPaid {
			info.Price = int64(f.Price)
		}
		t.Fixture = info
	}
	for i := range s.Storages {
		if st := s.Storages[i]; st.Pos == p {
			t.Storage = storageInfo(s, st)
			break
		}
	}
	for _, e := range s.Entities {
		if e.Pos == p {
			t.Creatures = append(t.Creatures, Creature{ID: uint64(e.ID), Glyph: glyphs.ForEntity(e), Look: glyphs.ForEntityLook(e), Name: entityName(e), State: e.State.String()})
		}
	}
	return t
}

func storageInfo(s *sim.Snapshot, st sim.StorageView) *StorageInfo {
	info := &StorageInfo{
		Label:    storageLabel(s, st),
		Slots:    len(st.Inventory),
		Capacity: len(st.Inventory) * sim.MaxStackSize,
		Contents: []Stack{},
		Ledger:   make([]LedgerItem, 0, len(st.Ledger)),
	}
	for i, stack := range st.Inventory {
		if stack.Count > 0 {
			info.Used++
			info.Items += stack.Count
			info.Contents = append(info.Contents, Stack{Slot: i + 1, Item: stack.Kind.String(), Count: stack.Count})
		}
	}
	for _, l := range st.Ledger {
		info.Ledger = append(info.Ledger, LedgerItem{Owner: ownerLabel(s, l.Owner), Item: l.Item.String(), Count: l.Count})
	}
	return info
}

// storageLabel names a container as the TUI's storage tab does: a workshop's
// store by the workshop, a kitchen's pantry, someone's locker, or a chest.
func storageLabel(s *sim.Snapshot, st sim.StorageView) string {
	if st.Terrain == sim.Scumhouse || st.Terrain == sim.Forge || st.Terrain == sim.GunBench || st.Terrain == sim.Incubator {
		return st.Terrain.String()
	}
	if st.Pantry {
		return "pantry"
	}
	if f, ok := s.FixtureAt(st.Pos); ok && f.Access == sim.AccessPrivate {
		return ownerLabel(s, f.Owner) + "'s locker"
	}
	return "chest"
}

// ownerLabel names an owner as the TUI does: a colonist by name (living or
// dead), the colony, an item listed for sale, or nobody.
func ownerLabel(s *sim.Snapshot, o sim.Owner) string {
	if id, ok := o.Order(); ok {
		for _, ord := range s.Economy.Orders {
			if ord.ID == id {
				return "for sale by " + ownerLabel(s, ord.Actor)
			}
		}
		return "for sale"
	}
	switch o.Kind {
	case sim.OwnerCommunity:
		return "the colony"
	case sim.OwnerColonist:
		for i := range s.Entities {
			if e := s.Entities[i]; e.ID == o.ID {
				return entityName(e)
			}
		}
		if e, ok := s.Deceased[o.ID]; ok {
			return entityName(e) + " (dead)"
		}
	}
	return o.String()
}
