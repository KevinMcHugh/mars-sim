package sim

import (
	"math/rand"
	"testing"
)

func intCond(eq int) *intCondition { return &intCondition{Eq: &eq} }

func gtCond(v int) *intCondition { return &intCondition{Gt: &v} }

// A leaf condition with no fields set must match every species -- that's
// what makes an unconditional name entry (no "when" at all) work.
func TestNameConditionEmptyMatchesEverything(t *testing.T) {
	var c nameCondition
	if !c.matches(AlienSpecies{}) {
		t.Fatal("empty condition did not match the zero species")
	}
	if !c.matches(AlienSpecies{Limbs: 8, Arms: 8, Skin: SkinArmored, Temperament: TemperamentHostile}) {
		t.Fatal("empty condition did not match an elaborate species")
	}
}

// "all": every sub-condition must hold -- the beetle example from the ask
// (six legs *and* armored skin).
func TestNameConditionAll(t *testing.T) {
	beetle := nameCondition{All: []nameCondition{
		{Legs: intCond(6)},
		{Skin: "armored"},
	}}
	sixLeggedArmored := AlienSpecies{Limbs: 6, Arms: 0, Skin: SkinArmored}
	if !beetle.matches(sixLeggedArmored) {
		t.Fatal("all-condition did not match a six-legged armored species")
	}
	sixLeggedSmooth := AlienSpecies{Limbs: 6, Arms: 0, Skin: SkinSmooth}
	if beetle.matches(sixLeggedSmooth) {
		t.Fatal("all-condition matched a six-legged species with the wrong skin")
	}
	fourLeggedArmored := AlienSpecies{Limbs: 4, Arms: 0, Skin: SkinArmored}
	if beetle.matches(fourLeggedArmored) {
		t.Fatal("all-condition matched a species with the wrong leg count")
	}
}

// "any": at least one sub-condition must hold -- the ask's "cautious or
// friendly reads as a gremlin" example.
func TestNameConditionAny(t *testing.T) {
	c := nameCondition{Any: []nameCondition{
		{Temperament: "friendly"},
		{Temperament: "cautious"},
	}}
	for _, temp := range []AlienTemperament{TemperamentFriendly, TemperamentCautious} {
		if !c.matches(AlienSpecies{Temperament: temp}) {
			t.Fatalf("any-condition did not match temperament %v", temp)
		}
	}
	if c.matches(AlienSpecies{Temperament: TemperamentHostile}) {
		t.Fatal("any-condition matched hostile, which is in neither branch")
	}
}

// "not" negates a sub-condition, and nests inside "all"/"any" like any other
// boolean combinator.
func TestNameConditionNot(t *testing.T) {
	tailed := true
	c := nameCondition{All: []nameCondition{
		{Temperament: "hostile"},
		{Not: &nameCondition{Tail: &tailed}},
	}}
	if !c.matches(AlienSpecies{Temperament: TemperamentHostile, Tail: false}) {
		t.Fatal("hostile-and-not-tailed did not match a tailless hostile species")
	}
	if c.matches(AlienSpecies{Temperament: TemperamentHostile, Tail: true}) {
		t.Fatal("hostile-and-not-tailed matched a tailed hostile species")
	}
	if c.matches(AlienSpecies{Temperament: TemperamentFriendly, Tail: false}) {
		t.Fatal("hostile-and-not-tailed matched a non-hostile species")
	}
}

// The snake example from the ask: zero limbs *and* scaly skin.
func TestNameConditionSnakeExample(t *testing.T) {
	snake := nameCondition{All: []nameCondition{
		{Limbs: intCond(0)},
		{Skin: "scaly"},
	}}
	if !snake.matches(AlienSpecies{Limbs: 0, Arms: 0, Skin: SkinScaly}) {
		t.Fatal("snake condition did not match a limbless scaly species")
	}
	if snake.matches(AlienSpecies{Limbs: 0, Arms: 0, Skin: SkinFurry}) {
		t.Fatal("snake condition matched a limbless species with the wrong skin")
	}
}

// The centaur example: exactly 4 legs and 2 arms.
func TestNameConditionCentaurExample(t *testing.T) {
	centaur := nameCondition{All: []nameCondition{
		{Legs: intCond(4)},
		{Arms: intCond(2)},
	}}
	if !centaur.matches(AlienSpecies{Limbs: 6, Arms: 2}) { // 6 limbs, 2 arms -> 4 legs
		t.Fatal("centaur condition did not match a 4-legs-2-arms species")
	}
	if centaur.matches(AlienSpecies{Limbs: 4, Arms: 2}) { // 2 legs, 2 arms
		t.Fatal("centaur condition matched a species with only 2 legs")
	}
}

// The bug/centipede examples: a leg-count threshold via "gt", with the two
// thresholds nesting correctly (a centipede is also a bug).
func TestNameConditionLegThresholds(t *testing.T) {
	bug := nameCondition{Legs: gtCond(4)}
	centipede := nameCondition{Legs: gtCond(6)}

	sixLegged := AlienSpecies{Limbs: 6, Arms: 0}
	eightLegged := AlienSpecies{Limbs: 8, Arms: 0}
	fourLegged := AlienSpecies{Limbs: 4, Arms: 0}

	if !bug.matches(sixLegged) || !bug.matches(eightLegged) {
		t.Fatal("bug (>4 legs) did not match a 6- or 8-legged species")
	}
	if bug.matches(fourLegged) {
		t.Fatal("bug (>4 legs) matched a 4-legged species")
	}
	if centipede.matches(sixLegged) {
		t.Fatal("centipede (>6 legs) matched a 6-legged species")
	}
	if !centipede.matches(eightLegged) {
		t.Fatal("centipede (>6 legs) did not match an 8-legged species")
	}
}

// Height/weight tiers and color are naming conditions too, per the ask.
func TestNameConditionSizeAndColor(t *testing.T) {
	titan := nameCondition{Any: []nameCondition{{Height: "huge"}, {Weight: "huge"}}}
	huge := AlienSpecies{HeightMinCM: 330, HeightMaxCM: 350}
	if !titan.matches(huge) {
		t.Fatal("titan condition did not match a huge-height species")
	}
	if titan.matches(AlienSpecies{HeightMinCM: 100, HeightMaxCM: 120, WeightMinKG: 10, WeightMaxKG: 20}) {
		t.Fatal("titan condition matched an ordinary-sized species")
	}

	green := nameCondition{Color: "green"}
	if !green.matches(AlienSpecies{Color: "green"}) || green.matches(AlienSpecies{Color: "blue"}) {
		t.Fatal("color condition did not discriminate correctly")
	}
}

// pickAlienName must draw only from matching entries, and fall back to
// "alien"/"aliens" when nothing in the pool matches (including an empty
// pool) -- a species always needs a name.
func TestPickAlienNameFallsBackWhenNothingMatches(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	sp := AlienSpecies{Temperament: TemperamentHostile, Limbs: 2, Arms: 2}
	entries := []AlienNameEntry{
		{Singular: "gremlin", Plural: "gremlins", When: nameCondition{Temperament: "friendly"}},
	}
	singular, plural := pickAlienName(rng, sp, entries)
	if singular != "alien" || plural != "aliens" {
		t.Fatalf("no-match fallback = %q/%q, want alien/aliens", singular, plural)
	}

	singular, plural = pickAlienName(rng, sp, nil)
	if singular != "alien" || plural != "aliens" {
		t.Fatalf("empty-pool fallback = %q/%q, want alien/aliens", singular, plural)
	}
}

// pickAlienName's plural falls back to singular+"s" when an entry omits it.
func TestPickAlienNamePluralDefaultsToSingularPlusS(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	entries := []AlienNameEntry{{Singular: "blorp"}}
	singular, plural := pickAlienName(rng, AlienSpecies{}, entries)
	if singular != "blorp" || plural != "blorps" {
		t.Fatalf("got %q/%q, want blorp/blorps", singular, plural)
	}
}

// defaultAlienNames() -- the pool compiled in from alien-names.yaml -- must
// parse cleanly and always be able to name any species: at least one entry
// must be unconditional, so pickAlienName's fallback is never reached in
// practice.
func TestDefaultAlienNamesAlwaysNamesAnySpecies(t *testing.T) {
	names := defaultAlienNames()
	if len(names) == 0 {
		t.Fatal("defaultAlienNames() returned nothing")
	}
	unconditional := false
	for _, e := range names {
		if e.When.isZero() {
			unconditional = true
			break
		}
	}
	if !unconditional {
		t.Fatal("defaultAlienNames() has no unconditional entry to fall back to")
	}

	rng := rand.New(rand.NewSource(1))
	// A handful of extreme/unusual builds, to make sure none of them ever
	// fails to find a candidate.
	for _, sp := range []AlienSpecies{
		{Limbs: 6, Arms: 0, Skin: SkinArmored, Temperament: TemperamentHostile}, // beetle-ish
		{Limbs: 0, Arms: 0, Skin: SkinScaly, Temperament: TemperamentHostile},   // snake-ish
		{Limbs: 6, Arms: 2, Skin: SkinSmooth, Temperament: TemperamentFriendly}, // centaur-ish
		{Limbs: 2, Arms: 2, Skin: SkinFurry, Temperament: TemperamentCautious, Color: "pale"},
	} {
		singular, plural := pickAlienName(rng, sp, names)
		if singular == "" || plural == "" {
			t.Fatalf("species %+v got an empty name", sp)
		}
	}
}

// LoadAlienNames must parse a document using every boolean operator together
// and reject an entry with no name.
func TestLoadAlienNamesParsesBooleanOperators(t *testing.T) {
	doc := []byte(`
names:
  - name: stalker
    plural: stalkers
    when:
      all:
        - temperament: hostile
        - legs: { eq: 2 }
        - not:
            tail: true
`)
	entries, err := LoadAlienNames(doc, "test")
	if err != nil {
		t.Fatalf("LoadAlienNames: %v", err)
	}
	if len(entries) != 1 || entries[0].Singular != "stalker" {
		t.Fatalf("entries = %+v, want one 'stalker' entry", entries)
	}
	stalker := AlienSpecies{Temperament: TemperamentHostile, Limbs: 2, Arms: 0, Tail: false}
	if !entries[0].When.matches(stalker) {
		t.Fatal("parsed condition did not match a hostile, two-legged, tailless species")
	}
	tailed := stalker
	tailed.Tail = true
	if entries[0].When.matches(tailed) {
		t.Fatal("parsed condition matched a tailed species despite the 'not tail' clause")
	}
}

func TestLoadAlienNamesRejectsMissingName(t *testing.T) {
	doc := []byte(`
names:
  - plural: nameless
`)
	if _, err := LoadAlienNames(doc, "test"); err == nil {
		t.Fatal("LoadAlienNames accepted an entry with no name")
	}
}
