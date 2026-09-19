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
	grandma.Profile.Age = 70
	parent.Profile.Age = 45
	child.Profile.Age = 20

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

func TestParentMustBeAtLeastTwentyYearsOlder(t *testing.T) {
	w := kinWorld()
	parent := w.spawn(Colonist, Point{1, 1})
	child := w.spawn(Colonist, Point{2, 1})

	parent.Profile.Age = 38
	child.Profile.Age = 19
	if w.wireRelation(parent, child, RelParent) {
		t.Fatal("accepted parent only 19 years older than child")
	}

	child.Profile.Age = 18
	if !w.wireRelation(child, parent, RelChild) {
		t.Fatal("rejected parent exactly 20 years older than child")
	}
}

// A grandparent is two parent gaps above their grandchild, even when the
// generation in between never joined the colony and so has no age to check.
func TestGrandparentSpansTwoParentGaps(t *testing.T) {
	w := kinWorld()
	grandma := w.spawn(Colonist, Point{1, 1})
	kid := w.spawn(Colonist, Point{2, 1})

	grandma.Profile.Age = 61
	kid.Profile.Age = 43
	if w.wireRelation(grandma, kid, RelGrandparent) {
		t.Fatal("accepted a grandparent only 18 years older than their grandchild")
	}
	if w.wireRelation(kid, grandma, RelGrandchild) {
		t.Fatal("accepted a grandchild only 18 years younger than their grandparent")
	}

	kid.Profile.Age = 21
	if !w.wireRelation(grandma, kid, RelGrandparent) {
		t.Fatal("rejected a grandparent exactly 40 years older than their grandchild")
	}
	mustRelate(t, w, kid, grandma, RelGrandparent)
	mustRelate(t, w, grandma, kid, RelGrandchild)
}

// A phantom parent shared by siblings does not launder the gap: a grandparent
// hung off it has to work for every colonist that phantom is a parent of.
func TestGrandparentCheckedAgainstEveryGrandchild(t *testing.T) {
	w := kinWorld()
	older := w.spawn(Colonist, Point{1, 1})
	younger := w.spawn(Colonist, Point{2, 1})
	grandma := w.spawn(Colonist, Point{3, 1})

	older.Profile.Age = 61
	younger.Profile.Age = 18
	grandma.Profile.Age = 58

	if !w.wireRelation(younger, older, RelSibling) {
		t.Fatal("could not wire siblings")
	}
	// grandma clears 40 years over younger, but the phantom parent they share
	// would make her older's grandmother too, at a 3-year gap.
	if w.wireRelation(grandma, younger, RelGrandparent) {
		t.Fatal("accepted a grandparent younger than one of the grandchildren it gains")
	}
}

// Siblings share their parents, so a colonist can only join a sibling set whose
// parents are old enough for them too.
func TestSiblingMustFitSharedParent(t *testing.T) {
	w := kinWorld()
	parent := w.spawn(Colonist, Point{1, 1})
	kid := w.spawn(Colonist, Point{2, 1})
	latecomer := w.spawn(Colonist, Point{3, 1})

	parent.Profile.Age = 45
	kid.Profile.Age = 20
	latecomer.Profile.Age = 40

	if !w.wireRelation(kid, parent, RelChild) {
		t.Fatal("could not wire kid as parent's child")
	}
	if w.wireRelation(latecomer, kid, RelSibling) {
		t.Fatal("accepted a sibling only 5 years younger than their shared parent")
	}
}

// Every derived relation in a densely related generated colony respects the age
// gaps its kind implies, however the tie was wired.
func TestGeneratedFamilyAgesHoldUp(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 11
	cfg.Width, cfg.Height = 40, 24
	cfg.FamilyChance = 100 // maximize ties so the colony is densely related
	w := newWorld(cfg, rand.New(rand.NewSource(11)))

	for i := 0; i < 120; i++ {
		w.spawn(Colonist, Point{i % w.Width, i / w.Width})
	}

	// How many parent links separate each kind, from the subject's viewpoint:
	// positive means the other is that many generations above.
	above := map[RelationKind]int{
		RelParent: 1, RelChild: -1, RelGrandparent: 2, RelGrandchild: -2,
	}

	children := w.kinChildren()
	checked := 0
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		for _, rel := range w.relativesOf(e, children) {
			gen, ok := above[rel.Kind]
			if !ok {
				continue // sibling, aunt/uncle, nibling and spouse carry no gap
			}
			other := w.entities[rel.Other]
			older, younger, want := other, e, gen
			if gen < 0 {
				older, younger, want = e, other, -gen
			}
			checked++
			if gap := older.Profile.Age - younger.Profile.Age; gap < want*minParentAgeGap {
				t.Fatalf("#%d (age %d) is #%d's (age %d) %s but only %d years apart; %s needs %d",
					other.ID, other.Profile.Age, e.ID, e.Profile.Age, rel.Kind,
					gap, rel.Kind, want*minParentAgeGap)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no generational relations were generated; the sweep proved nothing")
	}
}

// Siblings share a parent, and a sibling's child is an aunt/uncle & nibling pair.
func TestKinSiblingsAndNiblings(t *testing.T) {
	w := kinWorld()
	a := w.spawn(Colonist, Point{1, 1})
	b := w.spawn(Colonist, Point{2, 1})
	kid := w.spawn(Colonist, Point{3, 1})
	a.Profile.Age = 60
	b.Profile.Age = 40
	kid.Profile.Age = 20

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

// Spouse compatibility follows orientation and gender. A non-binary colonist
// is plausible with anyone who isn't asexual: orientation labels don't say
// anything about attraction to a non-binary person, so wireRelation settles
// those pairings with a coin flip instead (see TestNonbinaryPairingCoinFlip).
func TestSpouseCompatibility(t *testing.T) {
	man := func(o Orientation) *Profile { return &Profile{Gender: GenderMan, Orientation: o} }
	woman := func(o Orientation) *Profile { return &Profile{Gender: GenderWoman, Orientation: o} }
	enby := func(o Orientation) *Profile { return &Profile{Gender: GenderNonbinary, Orientation: o} }

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
		{"enby + heterosexual man plausible", enby(Bisexual), man(Heterosexual), true},
		{"enby + homosexual man plausible", enby(Bisexual), man(Homosexual), true},
		{"enby + enby plausible", enby(Heterosexual), enby(Homosexual), true},
		{"enby + asexual never", enby(Bisexual), man(Asexual), false},
		{"asexual enby never", enby(Asexual), man(Heterosexual), false},
	}
	for _, c := range cases {
		if got := spouseCompatible(c.a, c.b); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// A pairing involving a non-binary colonist is decided by a coin flip rather
// than orientation, so a heterosexual (or homosexual, or any) colonist can
// end up married to an enby -- and, on the same terms, sometimes doesn't.
func TestNonbinaryPairingCoinFlip(t *testing.T) {
	w := kinWorld()
	sawMarried, sawRejected := false, false
	for i := 0; i < 60 && !(sawMarried && sawRejected); i++ {
		enby := w.spawn(Colonist, Point{0, 0})
		hetero := w.spawn(Colonist, Point{1, 0})
		enby.Profile.Gender, enby.Profile.Orientation = GenderNonbinary, Bisexual
		hetero.Profile.Gender, hetero.Profile.Orientation = GenderMan, Heterosexual
		if w.wireRelation(enby, hetero, RelSpouse) {
			sawMarried = true
		} else {
			sawRejected = true
		}
	}
	if !sawMarried {
		t.Fatal("a heterosexual colonist never married an enby across many rolls")
	}
	if !sawRejected {
		t.Fatal("the enby pairing always succeeded; the coin flip doesn't seem to reject")
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
	if a.affect.Charge == 0 && a.affect.Grip == 0 && b.affect.Charge == 0 && b.affect.Grip == 0 {
		t.Fatalf("expected talking to move affect, both still neutral")
	}
}

// An urgent social need preempts ordinary work and forces a colonist to seek a
// conversation even when opportunistic talking is disabled.
func TestSocialNeedPreemptsWork(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.TalkChance = 0
	// DefaultConfig's seed is time-based, and traits are rolled off it: an
	// Asocial colonist has needRise 0 for social, so it never becomes urgent and
	// this test used to fail a run in six.
	cfg.Seed, cfg.TraitChance = 11, 0
	w := newWorld(cfg, rand.New(rand.NewSource(11)))
	center := Point{w.Width / 2, w.Height / 2}
	w.SetTerrain(center, Floor)
	w.SetTerrain(center.Add(1, 0), Floor)
	w.refreshSpatial()

	a := w.spawn(Colonist, center)
	b := w.spawn(Colonist, center.Add(1, 0))
	for _, e := range []*Entity{a, b} {
		for i := 0; i < int(numNeeds); i++ {
			e.Needs[i], e.needSince[i] = 0, w.tick
		}
	}
	a.Needs[NeedSocial] = cfg.Needs[NeedSocial].SeekAt
	w.step()

	if a.Job != JobTalk || a.partner != b.ID {
		t.Fatalf("urgent social need should start talking instead of work: job=%v partner=%d", a.Job, a.partner)
	}
}

// Two colonists who both urgently want company must be able to *finish* a
// conversation, not just start one. The urgent-social branch used to clear the
// colonist's job and begin a fresh talk every tick, and beginTalk resets the
// shared timer — so a mutually urgent pair restarted the same conversation
// forever, never reached TalkTicks, and never had the need satisfied. Since an
// urgent social need preempts all ordinary work, the whole colony then stopped
// mining and building for good.
func TestMutuallyUrgentColonistsFinishConversation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.TalkChance = 0                // only the urgent need may start this chat
	cfg.Seed, cfg.TraitChance = 11, 0 // no Asocial roll: see TestSocialNeedPreemptsWork
	w := newWorld(cfg, rand.New(rand.NewSource(11)))
	center := Point{w.Width / 2, w.Height / 2}
	w.SetTerrain(center, Floor)
	w.SetTerrain(center.Add(1, 0), Floor)
	w.refreshSpatial()

	a := w.spawn(Colonist, center)
	b := w.spawn(Colonist, center.Add(1, 0))
	seekAt := cfg.Needs[NeedSocial].SeekAt
	for _, e := range []*Entity{a, b} {
		for i := 0; i < int(numNeeds); i++ {
			e.Needs[i], e.needSince[i] = 0, w.tick
		}
		e.Needs[NeedSocial] = seekAt // both urgent, both preempted into talking
	}

	for i := 0; i < cfg.TalkTicks*3; i++ {
		w.step()
	}

	for _, e := range []*Entity{a, b} {
		if got := w.currentNeeds(e)[NeedSocial]; got >= seekAt {
			t.Errorf("#%d still socially urgent after %d ticks: %d (urgent at %d) — the conversation never completed",
				e.ID, cfg.TalkTicks*3, got, seekAt)
		}
	}
	if w.affinity[a.ID][b.ID] == 0 {
		t.Error("no affinity between the pair: the conversation was never credited")
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
