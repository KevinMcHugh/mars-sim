package sim

import (
	"math/rand"
	"testing"
)

// kinWorld builds a colonist-free world with random family ties disabled, so a
// test can wire its own tree deterministically.
func kinWorld() *World {
	cfg := DefaultConfig()
	cfg.Width, cfg.Height = 30, 20
	cfg.TraitChance, cfg.FamilyChance = 0, 0
	return newWorld(cfg, rand.New(rand.NewSource(1)))
}

// relationTo returns subject's derived familial tie to other, if any.
func relationTo(w *World, subject, other *Entity) (RelationKind, bool) {
	children := w.kinChildren()
	for _, rel := range w.relativesOf(subject, children) {
		if rel.Other == other.ID {
			return rel.Kind, true
		}
	}
	return 0, false
}

func mustRelate(t *testing.T, w *World, subject, other *Entity, want RelationKind) {
	t.Helper()
	got, ok := relationTo(w, subject, other)
	if !ok {
		t.Fatalf("#%d has no derived relation to #%d (wanted %s)", subject.ID, other.ID, want)
	}
	if got != want {
		t.Fatalf("#%d -> #%d: got %s want %s", subject.ID, other.ID, got, want)
	}
}

// A three-generation line derives parent/child and grandparent/grandchild, and
// leaves unrelated generations without a direct tie.
func TestKinLineDerivation(t *testing.T) {
	w := kinWorld()
	grandma := w.spawn(Colonist, Point{1, 1})
	parent := w.spawn(Colonist, Point{2, 1})
	child := w.spawn(Colonist, Point{3, 1})

	// parent is grandma's child; child is parent's child.
	if !w.wireRelation(parent, grandma, RelChild) {
		t.Fatal("could not wire parent as grandma's child")
	}
	if !w.wireRelation(child, parent, RelChild) {
		t.Fatal("could not wire child as parent's child")
	}

	mustRelate(t, w, parent, grandma, RelParent)
	mustRelate(t, w, grandma, parent, RelChild)
	mustRelate(t, w, child, parent, RelParent)
	mustRelate(t, w, parent, child, RelChild)
	mustRelate(t, w, child, grandma, RelGrandparent)
	mustRelate(t, w, grandma, child, RelGrandchild)
}

// Siblings share a parent, and a sibling's child is an aunt/uncle & nibling pair.
func TestKinSiblingsAndNiblings(t *testing.T) {
	w := kinWorld()
	a := w.spawn(Colonist, Point{1, 1})
	b := w.spawn(Colonist, Point{2, 1})
	kid := w.spawn(Colonist, Point{3, 1})

	if !w.wireRelation(b, a, RelSibling) {
		t.Fatal("could not wire siblings")
	}
	if !w.wireRelation(kid, a, RelChild) {
		t.Fatal("could not wire kid as a's child")
	}

	mustRelate(t, w, a, b, RelSibling)
	mustRelate(t, w, b, a, RelSibling)
	mustRelate(t, w, kid, b, RelAuntUncle) // b is kid's aunt/uncle
	mustRelate(t, w, b, kid, RelNibling)   // kid is b's nibling
}

// Spouse compatibility follows orientation and gender.
func TestSpouseCompatibility(t *testing.T) {
	man := func(o Orientation) *Profile { return &Profile{Gender: GenderMan, Orientation: o} }
	woman := func(o Orientation) *Profile { return &Profile{Gender: GenderWoman, Orientation: o} }

	cases := []struct {
		name string
		a, b *Profile
		want bool
	}{
		{"hetero man/woman", man(Heterosexual), woman(Heterosexual), true},
		{"hetero two men", man(Heterosexual), man(Heterosexual), false},
		{"homosexual two men", man(Homosexual), man(Homosexual), true},
		{"homosexual man/woman", man(Homosexual), woman(Homosexual), false},
		{"bisexual + hetero opposite", woman(Bisexual), man(Heterosexual), true},
		{"asexual never", man(Asexual), woman(Heterosexual), false},
		{"one-sided attraction", man(Homosexual), woman(Bisexual), false},
	}
	for _, c := range cases {
		if got := spouseCompatible(c.a, c.b); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// With family generation disabled, no colonist has a familial tie.
func TestFamilyChanceDisabled(t *testing.T) {
	w := kinWorld() // FamilyChance == 0
	for i := 0; i < 30; i++ {
		e := w.spawn(Colonist, Point{i % w.Width, i / w.Width})
		if rels := w.relativesOf(e, w.kinChildren()); len(rels) != 0 {
			t.Fatalf("colonist #%d got relations with FamilyChance=0: %v", e.ID, rels)
		}
	}
}

// Generated family ties are reciprocal and spouse ties respect orientation:
// if A sees B as a relation, B sees A as the reciprocal kind.
func TestGeneratedFamilyReciprocal(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 7
	cfg.Width, cfg.Height = 40, 24
	cfg.FamilyChance = 100 // maximize ties so the colony is densely related
	w := newWorld(cfg, rand.New(rand.NewSource(7)))

	for i := 0; i < 40; i++ {
		w.spawn(Colonist, Point{i % w.Width, i / w.Width})
	}

	reciprocal := map[RelationKind]RelationKind{
		RelSpouse: RelSpouse, RelSibling: RelSibling,
		RelParent: RelChild, RelChild: RelParent,
		RelGrandparent: RelGrandchild, RelGrandchild: RelGrandparent,
		RelAuntUncle: RelNibling, RelNibling: RelAuntUncle,
	}

	children := w.kinChildren()
	total := 0
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		for _, rel := range w.relativesOf(e, children) {
			total++
			other := w.entities[rel.Other]
			back, ok := relationTo(w, other, e)
			if !ok {
				t.Fatalf("#%d sees #%d as %s but #%d sees no tie back",
					e.ID, other.ID, rel.Kind, other.ID)
			}
			if back != reciprocal[rel.Kind] {
				t.Fatalf("#%d->#%d is %s but #%d->#%d is %s (want %s)",
					e.ID, other.ID, rel.Kind, other.ID, e.ID, back, reciprocal[rel.Kind])
			}
			if rel.Kind == RelSpouse && !spouseCompatible(e.Profile, other.Profile) {
				t.Fatalf("#%d married #%d against orientation", e.ID, other.ID)
			}
		}
	}
	if total == 0 {
		t.Fatal("FamilyChance=100 produced no family ties at all")
	}
}

// Two idle colonists with nothing else to do talk and build mutual affinity.
func TestTalkingRaisesAffinity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	cfg.Width, cfg.Height = 30, 20
	cfg.StartColonists, cfg.StartAliens = 0, 0
	cfg.TalkChance = 100 // always chat when idle
	w := newWorld(cfg, rand.New(rand.NewSource(3)))

	// A small floor pocket fully ringed by wall: no rock to mine, no work, so the
	// colonists fall through to socializing.
	cx, cy := w.Width/2, w.Height/2
	for y := cy - 1; y <= cy+1; y++ {
		for x := cx - 2; x <= cx+2; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	for y := cy - 2; y <= cy+2; y++ {
		for x := cx - 3; x <= cx+3; x++ {
			if w.TerrainAt(Point{x, y}) != Floor {
				w.SetTerrain(Point{x, y}, Wall)
			}
		}
	}
	w.refreshSpatial()

	a := w.spawn(Colonist, Point{cx - 1, cy})
	b := w.spawn(Colonist, Point{cx + 1, cy})
	// Keep their needs quiet so nothing preempts the chat.
	for _, e := range []*Entity{a, b} {
		for i := 0; i < int(numNeeds); i++ {
			e.Needs[i], e.needSince[i] = 0, 0
		}
	}

	for i := 0; i < 60; i++ {
		w.step()
	}

	if got := w.affinity[a.ID][b.ID]; got <= 0 {
		t.Fatalf("expected affinity a->b to rise, got %d", got)
	}
	if got := w.affinity[b.ID][a.ID]; got <= 0 {
		t.Fatalf("expected affinity b->a to rise, got %d", got)
	}
	if v := w.affinity[a.ID][b.ID]; v > w.cfg.AffinityMax {
		t.Fatalf("affinity exceeded the cap: %d > %d", v, w.cfg.AffinityMax)
	}
	if a.mood == 0 && b.mood == 0 {
		t.Fatalf("expected talking to move mood, both still 0")
	}
}

// A conversation's affinity change follows the chat's quality sign, shrinks as
// affinity nears the cap in that direction, and never carries it past the cap.
func TestTalkAffinityDeltaDiminishes(t *testing.T) {
	w := kinWorld()
	limit := w.talkAffinityCap()

	if d := w.talkAffinityDelta(0, 80); d <= 0 {
		t.Fatalf("a positive-quality chat should raise affinity, got %d", d)
	}
	if d := w.talkAffinityDelta(0, -80); d >= 0 {
		t.Fatalf("a negative-quality chat should lower affinity, got %d", d)
	}
	// Diminishing returns: the same great chat moves a near-cap pair less.
	if near, far := w.talkAffinityDelta(limit-5, 100), w.talkAffinityDelta(0, 100); near >= far {
		t.Fatalf("expected diminishing returns near the cap: near=%d far=%d", near, far)
	}
	// At the cap, further same-direction chatting adds nothing.
	if d := w.talkAffinityDelta(limit, 100); d != 0 {
		t.Fatalf("positive chat at +cap should add 0, got %d", d)
	}
	if d := w.talkAffinityDelta(-limit, -100); d != 0 {
		t.Fatalf("negative chat at -cap should add 0, got %d", d)
	}
}

// Repeated chatting is a diminishing-returns feedback loop that saturates at
// half of AffinityMax, in either valence, without overshooting.
func TestTalkAffinitySaturatesAtHalf(t *testing.T) {
	w := kinWorld()
	limit := w.talkAffinityCap()

	v := 0
	for i := 0; i < 200; i++ {
		v = clampInt(v+w.talkAffinityDelta(v, 100), -w.cfg.AffinityMax, w.cfg.AffinityMax)
		if v > limit {
			t.Fatalf("positive chatting overshot the half-cap: %d > %d", v, limit)
		}
	}
	if v != limit {
		t.Fatalf("positive chatting should saturate at %d, reached %d", limit, v)
	}

	v = 0
	for i := 0; i < 200; i++ {
		v = clampInt(v+w.talkAffinityDelta(v, -100), -w.cfg.AffinityMax, w.cfg.AffinityMax)
		if v < -limit {
			t.Fatalf("negative chatting overshot the half-cap: %d < %d", v, -limit)
		}
	}
	if v != -limit {
		t.Fatalf("negative chatting should saturate at %d, reached %d", -limit, v)
	}
}

// Mood shifts capture the intended feel: a good chat with someone disliked lifts
// mood, a so-so chat with a friend still nets a small lift, and only a genuinely
// bad chat with a friend turns mood negative.
func TestTalkMoodRules(t *testing.T) {
	w := kinWorld()
	limit := w.talkAffinityCap()

	if d := w.talkMoodDelta(60, -limit); d <= 0 {
		t.Fatalf("a good chat with someone disliked should lift mood, got %d", d)
	}
	if d := w.talkMoodDelta(-20, limit); d <= 0 {
		t.Fatalf("a so-so chat with a friend should still nudge mood up, got %d", d)
	}
	if d := w.talkMoodDelta(-80, limit); d >= 0 {
		t.Fatalf("a genuinely bad chat with a friend should lower mood, got %d", d)
	}
}

// Conversation quality leans toward the valence of existing affinity: fond pairs
// tend to have better chats than hostile ones.
func TestTalkQualityFollowsValence(t *testing.T) {
	w := kinWorld()
	limit := w.talkAffinityCap()

	sum := func(existing int) int {
		total := 0
		for i := 0; i < 500; i++ {
			total += w.rollTalkQuality(existing)
		}
		return total
	}
	if fond, hostile := sum(limit), sum(-limit); fond <= hostile {
		t.Fatalf("fond pairs should chat better on average: fond=%d hostile=%d", fond, hostile)
	}
}

// Affinity can now go negative, clamped to -AffinityMax.
func TestAffinityGoesNegative(t *testing.T) {
	w := kinWorld()
	w.addAffinity(1, 2, -10)
	if got := w.affinityBetween(1, 2); got != -10 {
		t.Fatalf("affinity a->b: got %d want -10", got)
	}
	if got := w.affinityBetween(2, 1); got != -10 {
		t.Fatalf("affinity is symmetric: b->a got %d want -10", got)
	}
	w.addAffinity(1, 2, -10*w.cfg.AffinityMax)
	if got := w.affinityBetween(1, 2); got != -w.cfg.AffinityMax {
		t.Fatalf("affinity should clamp at -%d, got %d", w.cfg.AffinityMax, got)
	}
}
