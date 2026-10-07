package sim

import (
	"strings"
	"testing"
)

// topicConfig enables exactly one topic kind, so a test sees only that kind.
func topicConfig(memory, colonist, lore int) Config {
	cfg := testConfig()
	cfg.TalkTopicMemoryWeight = memory
	cfg.TalkTopicColonistWeight = colonist
	cfg.TalkTopicLoreWeight = lore
	return cfg
}

// A lore topic names a species by the plural colonists use for it, and both
// participants remember the same subject.
func TestConversationAboutLoreNamesSpecies(t *testing.T) {
	w := newTestWorld(t, topicConfig(0, 0, 1))
	w.corporations = nil // species only: corporations are lore too
	a := w.spawn(Colonist, Point{0, 0, LandingLevel})
	b := w.spawn(Colonist, Point{1, 0, LandingLevel})

	w.finishTalk(a, b)

	want := "about the " + w.alienSpecies[0].Plural
	for _, e := range []*Entity{a, b} {
		got := e.Memories[len(e.Memories)-1].Text
		if !strings.Contains(got, want) {
			t.Errorf("memory = %q, want it to mention %q", got, want)
		}
	}
}

// Every rolled species and corporation is lore, species first, each with a
// distinct ID.
func TestLoreItemsCoverEverySpeciesAndCorporation(t *testing.T) {
	cfg := testConfig()
	cfg.AlienSpeciesCount = 3
	cfg.CorporationCount = 2
	w := newTestWorld(t, cfg)
	items := w.loreItems()
	if len(items) != 5 {
		t.Fatalf("got %d lore items, want 5", len(items))
	}
	seen := map[string]bool{}
	for i, it := range items {
		want := LoreSpecies
		if i >= 3 {
			want = LoreCorporation
		}
		if it.LoreKind() != want {
			t.Errorf("item %d kind = %v, want %v", i, it.LoreKind(), want)
		}
		if seen[it.LoreID()] {
			t.Errorf("duplicate lore ID %q", it.LoreID())
		}
		seen[it.LoreID()] = true
	}
}

// A corporation is talked about by name, and a colonist who used to work there
// says so; the listener remembers whose old employer it was.
func TestConversationAboutCorporationNamesOldEmployer(t *testing.T) {
	w := newTestWorld(t, topicConfig(0, 0, 1))
	w.alienSpecies = nil // corporations only
	w.corporations = w.corporations[:1]
	a := w.spawn(Colonist, Point{0, 0, LandingLevel})
	b := w.spawn(Colonist, Point{1, 0, LandingLevel})
	a.employer, a.employerRole = 1, "a janitor"
	b.employer, b.employerRole = 1, "an intern"
	name := w.corporations[0].Name

	w.finishTalk(a, b)

	for _, e := range []*Entity{a, b} {
		got := e.Memories[len(e.Memories)-1].Text
		if !strings.Contains(got, "about "+name+", ") || !strings.Contains(got, "old employer") {
			t.Errorf("memory = %q, want it to call %s an old employer", got, name)
		}
	}
}

// A memory topic retells one of the speaker's memories, never one of its
// conversations (that would nest retellings).
func TestConversationAboutMemoryRetellsSpeakerMemory(t *testing.T) {
	w := newTestWorld(t, topicConfig(1, 0, 0))
	a := w.spawn(Colonist, Point{0, 0, LandingLevel})
	b := w.spawn(Colonist, Point{1, 0, LandingLevel})
	for _, e := range []*Entity{a, b} {
		rememberTest(w, e, "conversation", "Had a conversation with someone.")
		rememberTest(w, e, "ate", "Had a meal.")
	}

	w.finishTalk(a, b)

	for _, e := range []*Entity{a, b} {
		got := e.Memories[len(e.Memories)-1].Text
		if !strings.Contains(got, `"Had a meal."`) {
			t.Errorf("memory = %q, want it to retell the meal", got)
		}
	}
}

// With nothing to say about any enabled kind, a conversation is small talk.
func TestConversationWithoutTopicIsSmallTalk(t *testing.T) {
	w := newTestWorld(t, topicConfig(1, 1, 0))
	a := w.spawn(Colonist, Point{0, 0, LandingLevel})
	b := w.spawn(Colonist, Point{1, 0, LandingLevel})

	w.finishTalk(a, b)

	want := "Had a conversation with " + b.displayName() + "."
	if got := a.Memories[len(a.Memories)-1].Text; got != want {
		t.Errorf("memory = %q, want %q", got, want)
	}
}

// Gossip moves the listener's opinion of the subject toward the speaker's,
// and never past it.
func TestGossipCarriesSpeakerOpinion(t *testing.T) {
	cfg := topicConfig(0, 1, 0)
	cfg.TalkGossipPercent = 50
	w := newTestWorld(t, cfg)
	a := w.spawn(Colonist, Point{0, 0, LandingLevel})
	b := w.spawn(Colonist, Point{1, 0, LandingLevel})
	c := w.spawn(Colonist, Point{5, 5, LandingLevel})
	// Both dislike c equally hard from each side, so whoever speaks, the
	// listener already agrees and nothing moves...
	w.bumpAffinity(a.ID, c.ID, -40)
	w.bumpAffinity(b.ID, c.ID, -40)
	topic := ConversationTopic{Kind: TopicColonist, Speaker: a.ID, Colonist: c.ID}
	w.applyGossip(topic, b, 50)
	if got := w.affinityBetween(b.ID, c.ID); got != -40 {
		t.Fatalf("agreeing listener moved to %d, want -40", got)
	}
	// ...but a listener with no opinion moves halfway to the speaker's.
	w.affinity[b.ID][c.ID] = 0
	w.applyGossip(topic, b, 50)
	if got := w.affinityBetween(b.ID, c.ID); got != -20 {
		t.Fatalf("listener affinity = %d, want -20", got)
	}
	// A bad conversation convinces nobody.
	w.applyGossip(topic, b, -10)
	if got := w.affinityBetween(b.ID, c.ID); got != -20 {
		t.Fatalf("bad chat moved listener to %d, want -20", got)
	}
}

// Gossip subjects are colonists the speaker has feelings about, other than
// the two talking.
func TestGossipSubjectsExcludeTheTalkers(t *testing.T) {
	w := newTestWorld(t, topicConfig(0, 1, 0))
	a := w.spawn(Colonist, Point{0, 0, LandingLevel})
	b := w.spawn(Colonist, Point{1, 0, LandingLevel})
	c := w.spawn(Colonist, Point{5, 5, LandingLevel})
	w.bumpAffinity(a.ID, b.ID, 30)
	w.bumpAffinity(a.ID, c.ID, 30)
	got := w.gossipSubjects(a, b)
	if len(got) != 1 || got[0] != c.ID {
		t.Fatalf("gossip subjects = %v, want [%d]", got, c.ID)
	}
}

// Topics draw from their own stream: choosing one never shifts the
// simulation stream.
func TestTopicChoiceLeavesSimStreamAlone(t *testing.T) {
	w1 := newTestWorld(t, topicConfig(1, 1, 1))
	w2 := newTestWorld(t, topicConfig(1, 1, 1))
	a := w1.spawn(Colonist, Point{0, 0, LandingLevel})
	b := w1.spawn(Colonist, Point{1, 0, LandingLevel})
	w2.spawn(Colonist, Point{0, 0, LandingLevel}) // spawning draws from rng; match it
	w2.spawn(Colonist, Point{1, 0, LandingLevel})
	w1.chooseTopic(a, b)
	if w1.rng.Uint64() != w2.rng.Uint64() {
		t.Fatal("chooseTopic drew from the simulation stream")
	}
}
