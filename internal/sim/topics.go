package sim

import "fmt"

// ---- Conversation topics -----------------------------------------------------
//
// A conversation used to be about nothing in particular: two colonists talked,
// and only the quality roll and their affinity decided how it went. Now one of
// the two raises a topic, and the topic is something the speaker actually has:
//
//   - one of its own memories ("recalling: Bitten in the arm by a grelk!"),
//   - another colonist it has feelings about (gossip), or
//   - a piece of lore about the world beyond the colony (see LoreItem).
//
// Gossip is the one topic with a mechanical effect: when the chat went well,
// the listener's affinity toward the subject moves part of the way toward the
// speaker's (see applyGossip). Memories and lore are, for now, what the two
// remember talking about; the topic is chosen and carried so later systems
// can hang effects on them without reopening finishTalk.
//
// Topics draw from their own stream, topicRNG: gossip moves affinity, so it is
// not flavor and cannot ride the personality stream, and putting the draws on
// w.rng would shift every later simulation draw in every seed.

// conversationTopicSeed is the topic stream's XOR key against cfg.Seed,
// distinct from every other stream's.
const conversationTopicSeed = 0x2B7E151628AED2A6

// TopicKind is what a conversation was about.
type TopicKind uint8

const (
	// TopicNone is small talk: the speaker had nothing to raise.
	TopicNone TopicKind = iota
	// TopicMemory is one of the speaker's own memories.
	TopicMemory
	// TopicColonist is another colonist, neither of the two talking.
	TopicColonist
	// TopicLore is a LoreItem: something about the world itself.
	TopicLore
)

func (k TopicKind) String() string {
	switch k {
	case TopicNone:
		return "small talk"
	case TopicMemory:
		return "memory"
	case TopicColonist:
		return "colonist"
	case TopicLore:
		return "lore"
	default:
		return "unknown"
	}
}

// LoreKind is which family of lore a LoreItem belongs to. Species is the only
// one today; history (and organizations, other colonies, ...) are expected to
// join it as further kinds.
type LoreKind uint8

const (
	LoreSpecies LoreKind = iota
)

func (k LoreKind) String() string {
	switch k {
	case LoreSpecies:
		return "species"
	default:
		return "unknown"
	}
}

// LoreItem is one thing colonists can talk about that describes the world
// rather than the colony: today an alien species, later a piece of history.
// A new kind of lore joins conversation by implementing this and being
// returned from World.loreItems; nothing in the conversation code needs to
// know what it is.
type LoreItem interface {
	// LoreKind is the item's family.
	LoreKind() LoreKind
	// LoreID is stable for the life of a world and unique across every kind,
	// e.g. "species:0".
	LoreID() string
	// TopicPhrase is how a colonist names it in conversation, as the object
	// of "about": "the grelks".
	TopicPhrase() string
}

// speciesLore is an alien species seen as lore: the roster index makes its ID.
type speciesLore struct {
	index   int
	species AlienSpecies
}

func (s speciesLore) LoreKind() LoreKind  { return LoreSpecies }
func (s speciesLore) LoreID() string      { return fmt.Sprintf("species:%d", s.index) }
func (s speciesLore) TopicPhrase() string { return "the " + s.species.Plural }

// loreItems is every piece of lore colonists can talk about, in a stable order
// (topic draws index into it). Each lore source appends its own items here.
// Every species is talkable whether or not the colony has met one yet: the
// colonists arrive knowing the survey, and there is no per-colonist knowledge
// of lore to gate on (see docs/conversation-topics.md).
func (w *World) loreItems() []LoreItem {
	items := make([]LoreItem, 0, len(w.alienSpecies))
	for i, sp := range w.alienSpecies {
		items = append(items, speciesLore{index: i, species: sp})
	}
	return items
}

// ConversationTopic is what one conversation was about and who raised it.
// Exactly one of Memory, Colonist, Lore is set, matching Kind.
type ConversationTopic struct {
	Kind     TopicKind
	Speaker  EntityID
	Memory   Memory   // TopicMemory: a copy of the memory recounted
	Colonist EntityID // TopicColonist: who was talked about
	Lore     LoreItem // TopicLore
}

// chooseTopic has one of a and b (drawn) raise a topic it has something to say
// about. Kinds the speaker has nothing for drop out of the weighting, so a
// colonist with no opinions of anyone still talks about what it remembers; a
// zero result (TopicNone) means every enabled kind was empty.
func (w *World) chooseTopic(a, b *Entity) ConversationTopic {
	rng := w.topicRNG
	if rng == nil { // a hand-built World; topics are optional
		return ConversationTopic{}
	}
	speaker, listener := a, b
	if rng.IntN(2) == 1 {
		speaker, listener = b, a
	}
	memories := w.recountableMemories(speaker)
	colonists := w.gossipSubjects(speaker, listener)
	lore := w.loreItems()

	weights := [...]struct {
		kind   TopicKind
		weight int
		count  int
	}{
		{TopicMemory, w.cfg.TalkTopicMemoryWeight, len(memories)},
		{TopicColonist, w.cfg.TalkTopicColonistWeight, len(colonists)},
		{TopicLore, w.cfg.TalkTopicLoreWeight, len(lore)},
	}
	total := 0
	for _, k := range weights {
		if k.weight > 0 && k.count > 0 {
			total += k.weight
		}
	}
	if total == 0 {
		return ConversationTopic{Speaker: speaker.ID}
	}
	r := rng.IntN(total)
	kind := TopicNone
	for _, k := range weights {
		if k.weight <= 0 || k.count == 0 {
			continue
		}
		if r < k.weight {
			kind = k.kind
			break
		}
		r -= k.weight
	}
	t := ConversationTopic{Kind: kind, Speaker: speaker.ID}
	switch kind {
	case TopicMemory:
		t.Memory = memories[rng.IntN(len(memories))]
	case TopicColonist:
		t.Colonist = colonists[rng.IntN(len(colonists))]
	case TopicLore:
		t.Lore = lore[rng.IntN(len(lore))]
	}
	return t
}

// recountableMemories is the speaker's memories worth retelling: everything
// but its other conversations, which would nest one retelling inside another
// ("recalling: Had a conversation with Bo, recalling: ...").
func (w *World) recountableMemories(e *Entity) []Memory {
	out := make([]Memory, 0, len(e.Memories))
	for _, m := range e.Memories {
		if m.Rule != conversationRule {
			out = append(out, m)
		}
	}
	return out
}

// conversationRule is the reaction ID a conversation memory is recorded under
// (cognition.yaml).
const conversationRule RuleID = "conversation"

// gossipSubjects is every living colonist, other than the two talking, that
// the speaker has a nonzero opinion of, in ID order. Iterating the affinity map
// directly would let map order pick the subject (see docs/determinism.md).
func (w *World) gossipSubjects(speaker, listener *Entity) []EntityID {
	opinions := w.affinity[speaker.ID]
	if len(opinions) == 0 {
		return nil
	}
	var out []EntityID
	for _, id := range w.entityIDsSorted() {
		if id == speaker.ID || id == listener.ID || opinions[id] == 0 {
			continue
		}
		if e := w.entities[id]; e.Kind == Colonist && e.Alive() {
			out = append(out, id)
		}
	}
	return out
}

// applyGossip carries the speaker's opinion of the subject to the listener: a
// good conversation (quality > 0) moves the listener's affinity toward the
// subject TalkGossipPercent of the way to the speaker's. Moving toward, rather
// than adding, means gossip can never make the listener feel more strongly
// than the speaker does, and two colonists who already agree change nothing.
// A bad conversation convinces nobody.
func (w *World) applyGossip(t ConversationTopic, listener *Entity, quality int) {
	if t.Kind != TopicColonist || quality <= 0 || w.cfg.TalkGossipPercent <= 0 {
		return
	}
	theirs := w.affinityBetween(t.Speaker, t.Colonist)
	mine := w.affinityBetween(listener.ID, t.Colonist)
	if delta := (theirs - mine) * w.cfg.TalkGossipPercent / 100; delta != 0 {
		w.bumpAffinity(listener.ID, t.Colonist, delta)
	}
}

// conversationText is what e remembers of a conversation with other about t.
func (w *World) conversationText(e, other *Entity, t ConversationTopic) string {
	base := "Had a conversation with " + other.displayName()
	switch t.Kind {
	case TopicMemory:
		if t.Speaker == e.ID {
			return fmt.Sprintf("%s, recalling: %q", base, t.Memory.Text)
		}
		return fmt.Sprintf("%s, who recalled: %q", base, t.Memory.Text)
	case TopicColonist:
		if subject := w.entities[t.Colonist]; subject != nil {
			return fmt.Sprintf("%s about %s.", base, subject.displayName())
		}
	case TopicLore:
		return fmt.Sprintf("%s about %s.", base, t.Lore.TopicPhrase())
	}
	return base + "."
}
