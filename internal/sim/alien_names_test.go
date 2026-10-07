package sim

import (
	"reflect"
	"strings"
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

// A pattern condition composes with skin: "toad" is slimy *and* spotted.
func TestNameConditionPatternAndSkin(t *testing.T) {
	toad := nameCondition{All: []nameCondition{
		{Skin: "slimy"},
		{Pattern: "spotted"},
	}}
	if !toad.matches(AlienSpecies{Skin: SkinSlimy, Pattern: PatternSpotted}) {
		t.Fatal("toad condition did not match a slimy spotted species")
	}
	if toad.matches(AlienSpecies{Skin: SkinSlimy, Pattern: PatternStriped}) {
		t.Fatal("toad condition matched a striped species")
	}
	if toad.matches(AlienSpecies{Skin: SkinChitinous, Pattern: PatternSpotted}) {
		t.Fatal("toad condition matched a chitinous species")
	}
}

// Every skin and pattern the roller can produce round-trips through its
// String() into a condition that matches it, so a YAML entry can name any of
// them.
func TestNameConditionEveryHideAndPattern(t *testing.T) {
	for _, skin := range alienSkins {
		for _, pat := range []AlienPattern{PatternSolid, PatternStriped, PatternSpotted} {
			sp := AlienSpecies{Skin: skin, Pattern: pat}
			c := nameCondition{Skin: skin.String(), Pattern: pat.String()}
			if !c.matches(sp) {
				t.Fatalf("condition %+v did not match %v/%v", c, skin, pat)
			}
		}
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
	rng := newRand(1)
	sp := AlienSpecies{Temperament: TemperamentHostile, Limbs: 2, Arms: 2}
	entries := []AlienNameEntry{
		{Singular: "gremlin", Plural: "gremlins", When: nameCondition{Temperament: "friendly"}},
	}
	singular, plural, emoji := pickAlienName(rng, sp, entries, nil)
	if singular != "alien" || plural != "aliens" || emoji != "" {
		t.Fatalf("no-match fallback = %q/%q/%q, want alien/aliens/\"\"", singular, plural, emoji)
	}

	singular, plural, emoji = pickAlienName(rng, sp, nil, nil)
	if singular != "alien" || plural != "aliens" || emoji != "" {
		t.Fatalf("empty-pool fallback = %q/%q/%q, want alien/aliens/\"\"", singular, plural, emoji)
	}
}

// pickAlienName's plural falls back to singular+"s" when an entry omits it.
func TestPickAlienNamePluralDefaultsToSingularPlusS(t *testing.T) {
	rng := newRand(1)
	entries := []AlienNameEntry{{Singular: "blorp"}}
	singular, plural, _ := pickAlienName(rng, AlienSpecies{}, entries, nil)
	if singular != "blorp" || plural != "blorps" {
		t.Fatalf("got %q/%q, want blorp/blorps", singular, plural)
	}
}

// pickAlienName draws an emoji from the winning entry's own candidates, not
// from some other matching entry's list.
func TestPickAlienNameDrawsEmojiFromTheWinningEntry(t *testing.T) {
	rng := newRand(1)
	entries := []AlienNameEntry{{Singular: "gremlin", Plural: "gremlins", Emoji: []string{"🦎", "🐍"}}}
	for i := 0; i < 20; i++ {
		_, _, emoji := pickAlienName(rng, AlienSpecies{}, entries, nil)
		if emoji != "🦎" && emoji != "🐍" {
			t.Fatalf("emoji = %q, want one of the entry's own candidates", emoji)
		}
	}
}

// An entry with no emoji list must not manufacture one.
func TestPickAlienNameEmojiEmptyWhenEntryListsNone(t *testing.T) {
	rng := newRand(1)
	entries := []AlienNameEntry{{Singular: "alien", Plural: "aliens"}}
	_, _, emoji := pickAlienName(rng, AlienSpecies{}, entries, nil)
	if emoji != "" {
		t.Fatalf("emoji = %q, want \"\" for an entry with no emoji list", emoji)
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

	rng := newRand(1)
	// A handful of extreme/unusual builds, to make sure none of them ever
	// fails to find a candidate.
	for _, sp := range []AlienSpecies{
		{Limbs: 6, Arms: 0, Skin: SkinArmored, Temperament: TemperamentHostile}, // beetle-ish
		{Limbs: 0, Arms: 0, Skin: SkinScaly, Temperament: TemperamentHostile},   // snake-ish
		{Limbs: 6, Arms: 2, Skin: SkinSmooth, Temperament: TemperamentFriendly}, // centaur-ish
		{Limbs: 2, Arms: 2, Skin: SkinFurry, Temperament: TemperamentCautious, Color: "pale"},
	} {
		singular, plural, _ := pickAlienName(rng, sp, names, nil)
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

// A group expands, in file order, into one entry per name, each carrying the
// group's own condition and emoji; a bare string is a name whose plural is
// the name plus "s".
func TestLoadAlienNamesExpandsGroups(t *testing.T) {
	doc := []byte(`
names:
  - name: alien
  - group:
      - rept
      - { name: scaly, plural: scalies }
    emoji: ["🦎"]
    when:
      skin: scaly
  - name: xeno
`)
	entries, err := LoadAlienNames(doc, "test")
	if err != nil {
		t.Fatalf("LoadAlienNames: %v", err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Singular+"/"+e.Plural)
	}
	if want := []string{"alien/aliens", "rept/repts", "scaly/scalies", "xeno/xenos"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded names = %v, want %v", got, want)
	}
	for _, e := range entries[1:3] {
		if !reflect.DeepEqual(e.Emoji, []string{"🦎"}) || e.When.Skin != "scaly" {
			t.Fatalf("group member %q lost the group's emoji/condition: %+v", e.Singular, e)
		}
	}
}

func TestLoadAlienNamesRejectsMalformedGroups(t *testing.T) {
	for name, doc := range map[string]string{
		"name and group": "names:\n  - name: a\n    group: [b]\n",
		"group plural":   "names:\n  - group: [b]\n    plural: bs\n",
		"empty member":   "names:\n  - group: [{plural: xs}]\n",
	} {
		if _, err := LoadAlienNames([]byte(doc), "test"); err == nil {
			t.Errorf("%s: LoadAlienNames accepted it", name)
		}
	}
}

// pickAlienName never hands out a name in used while a matching name is free.
func TestPickAlienNameSkipsUsedNames(t *testing.T) {
	entries := []AlienNameEntry{{Singular: "grelk"}, {Singular: "xeno"}}
	used := map[string]bool{"grelk": true}
	for seed := int64(0); seed < 50; seed++ {
		if s, p, _ := pickAlienName(newRand(seed), AlienSpecies{}, entries, used); s != "xeno" || p != "xenos" {
			t.Fatalf("seed %d: got %q/%q, want xeno/xenos", seed, s, p)
		}
	}
}

// With every matching name taken, the name is qualified by color, then by a
// number, rather than repeated.
func TestPickAlienNameQualifiesWhenEveryNameIsTaken(t *testing.T) {
	entries := []AlienNameEntry{{Singular: "grelk"}}
	sp := AlienSpecies{Color: "green", Pattern: PatternStriped}
	used := map[string]bool{"grelk": true}
	s, p, _ := pickAlienName(newRand(1), sp, entries, used)
	if s != "green-striped grelk" || p != "green-striped grelks" {
		t.Fatalf("got %q/%q, want green-striped grelk/grelks", s, p)
	}
	used[s] = true
	s, p, _ = pickAlienName(newRand(1), sp, entries, used)
	if s != "green-striped grelk 2" || p != "green-striped grelks 2" {
		t.Fatalf("got %q/%q, want the numbered form", s, p)
	}
}

// No two species in a rolled roster share a name, even when the roster is
// bigger than the pool has names for.
func TestAlienRosterNamesAreDistinct(t *testing.T) {
	cfg := DefaultConfig()
	for _, count := range []int{8, 80} {
		cfg.AlienSpeciesCount = count
		roster := rollAlienSpeciesRoster(newRand(7), cfg)
		seen := map[string]bool{}
		for _, sp := range roster {
			key := strings.ToLower(sp.Singular)
			if seen[key] {
				t.Fatalf("count %d: name %q repeated", count, sp.Singular)
			}
			seen[key] = true
		}
	}
}

// A wings condition checks the species' Wings flag, and composes with skin:
// "gargoyle" is rocky *and* winged, "dodo" is feathered and wingless.
func TestNameConditionWings(t *testing.T) {
	yes, no := true, false
	gargoyle := nameCondition{All: []nameCondition{{Skin: "rocky"}, {Wings: &yes}}}
	if !gargoyle.matches(AlienSpecies{Skin: SkinRocky, Wings: true}) {
		t.Fatal("gargoyle condition did not match a winged rocky species")
	}
	if gargoyle.matches(AlienSpecies{Skin: SkinRocky}) {
		t.Fatal("gargoyle condition matched a wingless species")
	}
	dodo := nameCondition{All: []nameCondition{{Skin: "feathered"}, {Wings: &no}}}
	if !dodo.matches(AlienSpecies{Skin: SkinFeathered}) || dodo.matches(AlienSpecies{Skin: SkinFeathered, Wings: true}) {
		t.Fatal("dodo condition did not track Wings")
	}
	if (nameCondition{Wings: &yes}).isZero() {
		t.Fatal("a wings-only condition reads as unconditional")
	}
}

func gteCond(v int) *intCondition { return &intCondition{Gte: &v} }

// Feature leaves test the species' graded anatomy, its tail tip and the apex
// roll, and a condition using any of them is a feature condition.
func TestNameConditionFeatures(t *testing.T) {
	yes := true
	horned := AlienSpecies{Anatomy: AlienAnatomy{Horns: 6, Shell: 2, TailTip: TailSpikedClub}, Apex: true}
	bare := AlienSpecies{}
	for _, tc := range []struct {
		name string
		c    nameCondition
	}{
		{"horns", nameCondition{Horns: gteCond(6)}},
		{"shell", nameCondition{Shell: intCond(2)}},
		{"tail-tip", nameCondition{TailTip: "spiked-club"}},
		{"apex", nameCondition{Apex: &yes}},
		{"nested", nameCondition{All: []nameCondition{{Any: []nameCondition{{Horns: gteCond(1)}}}}}},
	} {
		if !tc.c.matches(horned) {
			t.Errorf("%s: does not match a species that has it", tc.name)
		}
		if tc.c.matches(bare) {
			t.Errorf("%s: matches a featureless species", tc.name)
		}
		if !tc.c.usesFeatures() || tc.c.isZero() {
			t.Errorf("%s: usesFeatures %v, isZero %v", tc.name, tc.c.usesFeatures(), tc.c.isZero())
		}
	}
	if (nameCondition{Skin: "furry"}).usesFeatures() {
		t.Error("a skin condition counts as a feature condition")
	}
}

// No built-in feature name matches a species before anatomy is rolled: if
// one did, it would leak into the first naming pass and shift every later
// species' build (see renameForFeatures).
func TestFeatureNamesNeverMatchABareBody(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 10
	features := 0
	for _, e := range defaultAlienNames() {
		if e.When.usesFeatures() {
			features++
		}
	}
	if features == 0 {
		t.Fatal("the built-in names have no feature names")
	}
	for seed := int64(1); seed <= 30; seed++ {
		cfg.Seed = seed
		for _, sp := range rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg) {
			sp.Anatomy, sp.Apex = AlienAnatomy{}, false
			for _, e := range defaultAlienNames() {
				if e.When.usesFeatures() && e.When.matches(sp) {
					t.Fatalf("feature name %q matches a featureless %s species", e.Singular, sp.Skin)
				}
			}
		}
	}
}

// A species renamed for its features fits the name it took, and the pass
// actually renames some.
func TestFeatureNamesFitTheirSpecies(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 10
	renamed := 0
	for seed := int64(1); seed <= 40; seed++ {
		cfg.Seed = seed
		for _, sp := range rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg) {
			for _, e := range defaultAlienNames() {
				if !e.When.usesFeatures() || !strings.HasSuffix(sp.Singular, e.Singular) {
					continue
				}
				fits := false
				for _, f := range defaultAlienNames() {
					if f.Singular == e.Singular && f.When.matches(sp) {
						fits = true
					}
				}
				if !fits {
					t.Errorf("%q does not fit its species: %+v apex %v", sp.Singular, sp.Anatomy, sp.Apex)
				}
				renamed++
				break
			}
		}
	}
	if renamed < 20 {
		t.Fatalf("only %d of 400 species took a feature name", renamed)
	}
	t.Logf("%d of 400 species took a feature name", renamed)
}

// Scientific names can follow features too: a horned species can be a
// Ceratoceras cornutum, and only a horned one.
func TestScientificNameFollowsFeatures(t *testing.T) {
	tx := alienTaxonomy{
		Prefixes: []taxonEntry{{Form: "cerato", When: nameCondition{Horns: gteCond(1)}}},
		Roots:    []taxonEntry{{Form: "ceras", Gender: "n", When: nameCondition{Horns: gteCond(1)}}},
		Epithets: []taxonEntry{{Form: "cornutus", Feminine: "cornuta", Neuter: "cornutum", When: nameCondition{Horns: gteCond(1)}}},
	}
	got := scientificName(newRand(1), AlienSpecies{Anatomy: AlienAnatomy{Horns: 3}}, tx, nil)
	if got != "Ceratoceras cornutum" {
		t.Fatalf("horned species named %q, want Ceratoceras cornutum", got)
	}
}
