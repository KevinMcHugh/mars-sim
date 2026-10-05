package sim

import (
	"regexp"
	"strings"
	"testing"
)

// The embedded word-part file must parse and pass loadTaxonomy's checks;
// defaultTaxonomy panics otherwise, so this pins it to a readable failure.
func TestEmbeddedTaxonomyLoads(t *testing.T) {
	if _, err := loadTaxonomy(embeddedAlienTaxonomy, "alien-taxonomy.yaml"); err != nil {
		t.Fatal(err)
	}
}

func TestLoadTaxonomyRejectsBadFiles(t *testing.T) {
	for name, doc := range map[string]string{
		"uppercase form":       "prefixes: [{form: Areo}]\nroots: [{form: zoon, gender: n}]\nepithets: [{form: martis}]",
		"no unconditional":     "prefixes: [{form: areo, when: {skin: scaly}}]\nroots: [{form: zoon, gender: n}]\nepithets: [{form: martis}]",
		"only a mimic":         "prefixes: [{form: pseudo, mimic: true}]\nroots: [{form: zoon, gender: n}]\nepithets: [{form: martis}]",
		"empty epithet list":   "prefixes: [{form: areo}]\nroots: [{form: zoon, gender: n}]",
		"root with no gender":  "prefixes: [{form: areo}]\nroots: [{form: zoon}]\nepithets: [{form: martis}]",
		"root with bad gender": "prefixes: [{form: areo}]\nroots: [{form: zoon, gender: x}]\nepithets: [{form: martis}]",
		"half a declension":    "prefixes: [{form: areo}]\nroots: [{form: zoon, gender: n}]\nepithets: [{form: martis}, {form: hirsutus, feminine: hirsuta}]",
	} {
		if _, err := loadTaxonomy([]byte(doc), name); err == nil {
			t.Errorf("%s: loadTaxonomy accepted it", name)
		}
	}
}

func TestJoinTaxaElidesVowels(t *testing.T) {
	for _, tc := range []struct{ prefix, root, want string }{
		{"erythro", "ops", "erythrops"},
		{"pseudo", "ursus", "pseudursus"},
		{"lepido", "urus", "lepidurus"},
		{"dasy", "urus", "dasyurus"},
		{"mega", "therium", "megatherium"},
		{"", "zoon", "zoon"},
	} {
		if got := joinTaxa(tc.prefix, tc.root); got != tc.want {
			t.Errorf("joinTaxa(%q, %q) = %q, want %q", tc.prefix, tc.root, got, tc.want)
		}
	}
}

// Every rolled species gets a well-formed binomial, unique in its roster,
// and the same seed always rolls the same names.
func TestRosterScientificNames(t *testing.T) {
	binomial := regexp.MustCompile(`^[A-Z][a-z]+ [a-z]+$`)
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 6
	for seed := int64(0); seed < 200; seed++ {
		cfg.Seed = seed
		roster := rollAlienSpeciesRoster(newRand(seed^alienLoreSeed), cfg)
		again := rollAlienSpeciesRoster(newRand(seed^alienLoreSeed), cfg)
		seen := map[string]bool{}
		for i, sp := range roster {
			if !binomial.MatchString(sp.ScientificName) {
				t.Fatalf("seed %d: malformed scientific name %q", seed, sp.ScientificName)
			}
			if seen[strings.ToLower(sp.ScientificName)] {
				t.Fatalf("seed %d: two species named %q", seed, sp.ScientificName)
			}
			seen[strings.ToLower(sp.ScientificName)] = true
			if again[i].ScientificName != sp.ScientificName {
				t.Fatalf("seed %d: same seed named species %d %q then %q", seed, i, sp.ScientificName, again[i].ScientificName)
			}
		}
	}
}

// Naming must not consume the lore stream: a species' build has to come
// out the same whether or not it is then given a scientific name, or adding
// names would have re-rolled every existing seed's roster.
func TestScientificNamesDoNotShiftTheRoster(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 4
	cfg.Seed = 99
	roster := rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg)
	rng := newRand(cfg.Seed ^ alienLoreSeed)
	names := defaultAlienNames()
	used := map[string]bool{}
	for i, got := range roster {
		want := rollAlienSpecies(rng, cfg, names, used)
		used[strings.ToLower(want.Singular)] = true
		got.ScientificName = ""
		if got != want {
			t.Fatalf("species %d differs once named:\n%+v\n%+v", i, got, want)
		}
	}
}

// Word parts follow the build: with only conditional entries besides the
// fallbacks, a species must never draw a part whose condition it fails.
func TestScientificNameFitsTheBuild(t *testing.T) {
	tx := alienTaxonomy{
		Prefixes: []taxonEntry{{Form: "areo"}, {Form: "pseudo", Mimic: true}, {Form: "leuco", When: nameCondition{Color: "white"}}},
		Roots:    []taxonEntry{{Form: "saurus", Mimic: true, When: nameCondition{Skin: "scaly"}}},
		Epithets: []taxonEntry{{Form: "ferox", When: nameCondition{Temperament: "hostile"}}},
	}
	sp := AlienSpecies{Skin: SkinScaly, Color: "red", Temperament: TemperamentHostile}
	rng := newRand(1)
	for i := 0; i < 50; i++ {
		got := scientificName(rng, sp, tx, nil)
		if got != "Areosaurus ferox" && got != "Pseudosaurus ferox" {
			t.Fatalf("scientificName = %q, want Areosaurus or Pseudosaurus ferox", got)
		}
	}
}

// A collision on every attempt still yields a unique name.
func TestScientificNameNumbersOnExhaustion(t *testing.T) {
	tx := alienTaxonomy{
		Prefixes: []taxonEntry{{Form: "areo"}},
		Roots:    []taxonEntry{{Form: "zoon"}},
		Epithets: []taxonEntry{{Form: "martis"}},
	}
	used := map[string]bool{"areozoon martis": true}
	if got := scientificName(newRand(1), AlienSpecies{}, tx, used); got != "Areozoon martis 2" {
		t.Fatalf("scientificName = %q, want %q", got, "Areozoon martis 2")
	}
}

// An epithet that declines agrees with the gender of the root the genus ends
// in, whatever the prefix: Pithecus hirsutus, Medusa hirsuta, Zoon hirsutum.
func TestScientificNameEpithetAgreesWithGenus(t *testing.T) {
	hirsutus := taxonEntry{Form: "hirsutus", Feminine: "hirsuta", Neuter: "hirsutum"}
	for _, c := range []struct {
		root taxonEntry
		want string
	}{
		{taxonEntry{Form: "pithecus", Gender: "m"}, "Areopithecus hirsutus"},
		{taxonEntry{Form: "medusa", Gender: "f"}, "Areomedusa hirsuta"},
		{taxonEntry{Form: "zoon", Gender: "n"}, "Areozoon hirsutum"},
	} {
		tx := alienTaxonomy{
			Prefixes: []taxonEntry{{Form: "areo"}},
			Roots:    []taxonEntry{c.root},
			Epithets: []taxonEntry{hirsutus},
		}
		if got := scientificName(newRand(1), AlienSpecies{}, tx, nil); got != c.want {
			t.Errorf("scientificName = %q, want %q", got, c.want)
		}
	}
}

// Every declining epithet in the built-in file, paired with every root,
// yields the form for that root's gender, and an invariable one never
// changes.
func TestEmbeddedEpithetsDecline(t *testing.T) {
	tx := defaultTaxonomy()
	declining := 0
	for _, e := range tx.Epithets {
		if e.Feminine == "" {
			for _, g := range []string{"m", "f", "n"} {
				if got := e.agreeing(g); got != e.Form {
					t.Errorf("invariable %q became %q for gender %s", e.Form, got, g)
				}
			}
			continue
		}
		declining++
		for _, r := range tx.Roots {
			want := map[string]string{"m": e.Form, "f": e.Feminine, "n": e.Neuter}[r.Gender]
			if got := e.agreeing(r.Gender); got != want {
				t.Errorf("%s + %s = %q, want %q", r.Form, e.Form, got, want)
			}
		}
	}
	if declining == 0 {
		t.Fatal("no declining epithets in the embedded taxonomy")
	}
}
