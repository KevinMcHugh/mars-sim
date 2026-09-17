package sim

import (
	"math/rand"
	"testing"
)

// heredityWorld builds a colonist-free world with random family ties disabled and
// heredity at full strength, so a test can wire its own tree and then run the
// inheritance pass over it deliberately.
func heredityWorld() *World {
	cfg := DefaultConfig()
	cfg.Width, cfg.Height = 30, 20
	cfg.TraitChance, cfg.FamilyChance = 0, 0
	cfg.AppearanceInheritChance = 100
	cfg.SpouseSurnameChance = 100
	cfg.FamilyAffinitySpread = 0
	return newWorld(cfg, rand.New(rand.NewSource(1)))
}

// colonistAged spawns a colonist of a given age at a free tile.
func colonistAged(w *World, x, age int) *Entity {
	e := w.spawn(Colonist, Point{x, 1})
	e.Profile.Age = age
	return e
}

// A colonist generated into an existing family takes that family's surname, and
// keeps their own given name.
func TestBloodRelativesShareASurname(t *testing.T) {
	w := heredityWorld()
	parent := colonistAged(w, 1, 50)
	child := colonistAged(w, 2, 25)
	parent.Profile.setSurname("Ravenna")

	if !w.wireRelation(child, parent, RelChild) {
		t.Fatal("could not wire child onto parent")
	}
	w.inheritFamily(child)

	if got := child.Profile.surname; got != "Ravenna" {
		t.Fatalf("child surname = %q, want the family's %q", got, "Ravenna")
	}
	if want := child.Profile.given + " Ravenna"; child.Profile.Name != want {
		t.Fatalf("child name = %q, want %q", child.Profile.Name, want)
	}
}

// A line converges on one surname however it grew: each new member takes the
// name from the earliest of their blood relatives to have joined the colony, so
// wiring a grandchild onto a parent still lands on the founder's name.
func TestFamilyLineConvergesOnOneSurname(t *testing.T) {
	w := heredityWorld()
	founder := colonistAged(w, 1, 75)
	founder.Profile.setSurname("Ravenna")
	parent := colonistAged(w, 2, 50)
	child := colonistAged(w, 3, 25)

	if !w.wireRelation(parent, founder, RelChild) {
		t.Fatal("could not wire parent onto founder")
	}
	w.inheritFamily(parent)
	parent.Profile.setSurname("Ravenna") // the founder's name, adopted above
	if !w.wireRelation(child, parent, RelChild) {
		t.Fatal("could not wire child onto parent")
	}
	w.inheritFamily(child)

	for _, e := range []*Entity{parent, child} {
		if got := e.Profile.surname; got != "Ravenna" {
			t.Fatalf("#%d surname = %q, want the line's %q", e.ID, got, "Ravenna")
		}
	}
}

// Marrying in is the one way to join a family and keep your own name.
func TestMarryingInTakesTheSurnameOnlyByChance(t *testing.T) {
	spouses := func(surnameChance int) (a, b *Entity) {
		w := heredityWorld()
		w.cfg.SpouseSurnameChance = surnameChance
		a = colonistAged(w, 1, 40)
		b = colonistAged(w, 2, 40)
		a.Profile.Gender, a.Profile.Orientation = GenderMan, Heterosexual
		b.Profile.Gender, b.Profile.Orientation = GenderWoman, Heterosexual
		a.Profile.setSurname("Ravenna")
		b.Profile.setSurname("Kestrel")
		if !w.wireRelation(b, a, RelSpouse) {
			t.Fatal("could not wire spouses")
		}
		w.inheritFamily(b)
		return a, b
	}

	if _, b := spouses(100); b.Profile.surname != "Ravenna" {
		t.Fatalf("at chance 100 the spouse should take the name, got %q", b.Profile.surname)
	}
	if _, b := spouses(0); b.Profile.surname != "Kestrel" {
		t.Fatalf("at chance 0 the spouse should keep their own name, got %q", b.Profile.surname)
	}
}

// Nobody inherits a spouse's looks: marriage passes a name, not a face.
func TestMarryingInDoesNotInheritAppearance(t *testing.T) {
	w := heredityWorld()
	a := colonistAged(w, 1, 40)
	b := colonistAged(w, 2, 40)
	a.Profile.Gender, a.Profile.Orientation = GenderMan, Heterosexual
	b.Profile.Gender, b.Profile.Orientation = GenderWoman, Heterosexual
	a.Profile.SkinTone, a.Profile.hairBase, a.Profile.HairColor = SkinDark, HairRed, HairRed
	b.Profile.SkinTone, b.Profile.hairBase, b.Profile.HairColor = SkinLight, HairBlack, HairBlack

	if !w.wireRelation(b, a, RelSpouse) {
		t.Fatal("could not wire spouses")
	}
	w.inheritFamily(b)

	if b.Profile.SkinTone != SkinLight || b.Profile.HairColor != HairBlack {
		t.Fatalf("spouse took on partner's looks: %s skin, %s hair", b.Profile.SkinTone, b.Profile.HairColor)
	}
}

// With inheritance certain, a child takes its parent's natural hair color exactly
// and its skin tone within one step of the parent's.
func TestAppearanceIsInheritedFromCloseKin(t *testing.T) {
	w := heredityWorld()
	parent := colonistAged(w, 1, 50)
	child := colonistAged(w, 2, 25)
	parent.Profile.SkinTone, parent.Profile.hairBase, parent.Profile.HairColor = SkinDark, HairRed, HairRed
	child.Profile.SkinTone, child.Profile.hairBase, child.Profile.HairColor = SkinLight, HairBlonde, HairBlonde

	if !w.wireRelation(child, parent, RelChild) {
		t.Fatal("could not wire child onto parent")
	}
	w.inheritFamily(child)

	if child.Profile.hairBase != HairRed || child.Profile.HairColor != HairRed {
		t.Fatalf("child hair = %s (base %s), want red", child.Profile.HairColor, child.Profile.hairBase)
	}
	if d := int(SkinDark) - int(child.Profile.SkinTone); d < 0 || d > 1 {
		t.Fatalf("child skin tone %s is not within one step of the parent's %s",
			child.Profile.SkinTone, SkinDark)
	}
}

// At chance 0 a colonist keeps every feature they were rolled with, but still
// joins the family's name: the two are independent knobs.
func TestAppearanceInheritChanceZeroKeepsRolledLooks(t *testing.T) {
	w := heredityWorld()
	w.cfg.AppearanceInheritChance = 0
	parent := colonistAged(w, 1, 50)
	child := colonistAged(w, 2, 25)
	parent.Profile.setSurname("Ravenna")
	parent.Profile.SkinTone, parent.Profile.hairBase, parent.Profile.HairColor = SkinDark, HairRed, HairRed
	child.Profile.SkinTone, child.Profile.hairBase, child.Profile.HairColor = SkinLight, HairBlonde, HairBlonde
	height := child.Profile.HeightCM

	if !w.wireRelation(child, parent, RelChild) {
		t.Fatal("could not wire child onto parent")
	}
	w.inheritFamily(child)

	if child.Profile.SkinTone != SkinLight || child.Profile.HairColor != HairBlonde ||
		child.Profile.HeightCM != height {
		t.Fatalf("looks changed with inheritance disabled: %+v", child.Profile)
	}
	if child.Profile.surname != "Ravenna" {
		t.Fatalf("surname should still be shared, got %q", child.Profile.surname)
	}
}

// Going grey or bald is a colonist's own age showing, so it is neither passed
// down by an old relative nor undone by a young one's inherited color.
func TestAgedHairIsNotInherited(t *testing.T) {
	w := heredityWorld()
	parent := colonistAged(w, 1, 70)
	parent.Profile.hairBase, parent.Profile.HairColor = HairBrown, HairWhite

	young := colonistAged(w, 2, 25)
	young.Profile.hairBase, young.Profile.HairColor = HairBlonde, HairBlonde
	if !w.wireRelation(young, parent, RelChild) {
		t.Fatal("could not wire young child onto parent")
	}
	w.inheritFamily(young)
	if young.Profile.HairColor != HairBrown {
		t.Fatalf("young child shows %s hair, want the parent's natural brown", young.Profile.HairColor)
	}

	greying := colonistAged(w, 3, 46)
	greying.Profile.hairBase, greying.Profile.HairColor = HairBlonde, HairWhite
	if !w.wireRelation(greying, parent, RelChild) {
		t.Fatal("could not wire greying child onto parent")
	}
	w.inheritFamily(greying)
	if greying.Profile.HairColor != HairWhite {
		t.Fatalf("greying child shows %s hair, want to stay white", greying.Profile.HairColor)
	}
	if greying.Profile.hairBase != HairBrown {
		t.Fatalf("greying child's natural hair = %s, want the inherited brown", greying.Profile.hairBase)
	}
}

// Height passes down as a z-score against the colonist's own gender mean, so a
// very tall parent produces children who are tall for their gender rather than
// literally as tall as the parent.
func TestHeightIsInheritedAcrossGenders(t *testing.T) {
	w := heredityWorld()
	parent := colonistAged(w, 1, 50)
	parent.Profile.Gender = GenderMan
	setHeightZ(parent.Profile, 3)

	total, n := 0, 60
	for i := 0; i < n; i++ {
		child := colonistAged(w, 2+i%20, 25)
		child.Profile.Gender = GenderWoman
		setHeightZ(child.Profile, 0)
		if !w.wireRelation(child, parent, RelChild) {
			t.Fatal("could not wire child onto parent")
		}
		w.inheritFamily(child)
		total += child.Profile.HeightCM
		w.remove(child.ID, "test")
	}
	// Women average 165cm; daughters of a +3sd father should sit well above that
	// (+1.8sd ≈ 177cm on average) without reaching his own 199cm.
	if avg := total / n; avg < 172 || avg > 190 {
		t.Fatalf("average daughter height %dcm, want tall-for-a-woman but not her father's height", avg)
	}
}

// Family already know each other: relatives start warm rather than at a
// stranger's zero, mutually, and closer ties start warmer.
func TestFamilyStartsWithAffinity(t *testing.T) {
	w := heredityWorld()
	a := colonistAged(w, 1, 60)
	sib := colonistAged(w, 2, 55)
	nib := colonistAged(w, 3, 25)

	if !w.wireRelation(sib, a, RelSibling) {
		t.Fatal("could not wire siblings")
	}
	w.inheritFamily(sib)
	if !w.wireRelation(nib, a, RelChild) {
		t.Fatal("could not wire nibling")
	}
	w.inheritFamily(nib)

	sibling := w.affinityBetween(a.ID, sib.ID)
	if sibling <= 0 {
		t.Fatalf("siblings start at %d, want a warm start", sibling)
	}
	if back := w.affinityBetween(sib.ID, a.ID); back != sibling {
		t.Fatalf("family affinity is not mutual: %d vs %d", sibling, back)
	}
	auntUncle := w.affinityBetween(nib.ID, sib.ID)
	if auntUncle <= 0 || auntUncle >= sibling {
		t.Fatalf("aunt/uncle affinity %d should be warm but below a sibling's %d", auntUncle, sibling)
	}
}

// FamilyAffinity = 0 leaves relatives as strangers, for a run that wants
// affinity to come only from conversations.
func TestFamilyAffinityZeroStartsAtStranger(t *testing.T) {
	w := heredityWorld()
	w.cfg.FamilyAffinity = 0
	a := colonistAged(w, 1, 60)
	b := colonistAged(w, 2, 55)
	if !w.wireRelation(b, a, RelSibling) {
		t.Fatal("could not wire siblings")
	}
	w.inheritFamily(b)
	if got := w.affinityBetween(a.ID, b.ID); got != 0 {
		t.Fatalf("family affinity = %d with the feature off, want 0", got)
	}
}

// The name index tracks who holds which name: it frees a name when its holder
// dies, and a colonist being renamed never evicts the namesake who was there
// first (which would leave two colonists with one name and a broken index).
func TestColonistNamesAreIndexedAndFreed(t *testing.T) {
	w := heredityWorld()
	a := colonistAged(w, 1, 40)
	if got := w.colonistNames[a.Profile.Name]; got != a.ID {
		t.Fatalf("index holds #%d for %q, want #%d", got, a.Profile.Name, a.ID)
	}

	b := colonistAged(w, 2, 40)
	b.Profile.Gender = a.Profile.Gender
	b.Profile.given, b.Profile.surname = a.Profile.given, a.Profile.surname
	b.Profile.Name = a.Profile.Name
	w.uniquifyName(b)
	if b.Profile.Name == a.Profile.Name {
		t.Fatalf("#%d kept #%d's name %q", b.ID, a.ID, a.Profile.Name)
	}
	if got := w.colonistNames[a.Profile.Name]; got != a.ID {
		t.Fatalf("#%d's name was evicted from the index (now #%d)", a.ID, got)
	}

	name := a.Profile.Name
	w.remove(a.ID, "test")
	if _, ok := w.colonistNames[name]; ok {
		t.Fatalf("%q is still reserved after its holder died", name)
	}
}

// End to end over a generated colony: every colonist carries the surname of the
// first of their blood relatives to land on Mars, every family tie starts warm,
// and no two colonists answer to the same name.
func TestGeneratedColonyFamilies(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed, cfg.StartColonists = 7, 24
	cfg.StartAliens, cfg.StartMice, cfg.StartCats = 0, 0, 0
	w := NewEngine(cfg).world

	children := w.kinChildren()
	names := map[string]EntityID{}
	ties := 0
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		if e == nil || e.Kind != Colonist {
			continue
		}
		if prev, dup := names[e.Profile.Name]; dup {
			t.Fatalf("#%d and #%d are both called %q", prev, e.ID, e.Profile.Name)
		}
		names[e.Profile.Name] = e.ID

		var line *Entity
		for _, rel := range w.relativesOf(e, children) {
			other := w.entities[rel.Other]
			if other == nil {
				continue
			}
			ties++
			if got := w.affinityBetween(e.ID, other.ID); got <= 0 {
				t.Fatalf("#%d and their %s #%d start at %d, want a warm start",
					e.ID, rel.Kind, other.ID, got)
			}
			if kinBonds[rel.Kind].genes > 0 && (line == nil || other.ID < line.ID) {
				line = other
			}
		}
		if line != nil && e.Profile.surname != line.Profile.surname {
			t.Fatalf("#%d (%s) does not carry the surname of their line's #%d (%s)",
				e.ID, e.Profile.Name, line.ID, line.Profile.Name)
		}
	}
	if ties == 0 {
		t.Fatal("seed produced no families; the test proves nothing")
	}
}

// Heredity stays on the personality stream: the whole pass is reproducible from
// the seed, surnames and starting affinity included.
func TestHeredityDeterministic(t *testing.T) {
	colony := func() []string {
		cfg := DefaultConfig()
		cfg.Seed, cfg.StartColonists = 99, 20
		cfg.StartAliens, cfg.StartMice, cfg.StartCats = 0, 0, 0
		w := NewEngine(cfg).world
		var out []string
		for _, id := range w.entityIDsSorted() {
			e := w.entities[id]
			if e == nil || e.Kind != Colonist {
				continue
			}
			line := e.Profile.Name
			for _, aff := range w.affinitiesOf(e.ID) {
				line += "|" + string(rune('0'+aff.Value%10))
			}
			out = append(out, line)
		}
		return out
	}
	a, b := colony(), colony()
	if len(a) != len(b) {
		t.Fatalf("colonist count differs: %d != %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("heredity nondeterministic at %d: %q != %q", i, a[i], b[i])
		}
	}
}
