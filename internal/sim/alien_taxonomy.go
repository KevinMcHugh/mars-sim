package sim

import (
	_ "embed"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ---- Alien taxonomy: scientific names --------------------------------------
//
// Alongside the colloquial name colonists use ("reptile", "grelk"), every
// rolled species gets a scientific name: a binomial "Genus epithet" such as
// Pseudursus ares. The genus is a Greek or Latin combining form (prefix)
// joined to a noun root, and the epithet is a single word; each of the three
// is drawn from the entries in alien-taxonomy.yaml whose condition matches
// the species' build, using the same nameCondition tree alien-names.yaml
// uses. So a red, scaly, hostile species can come out as Erythrosaurus ferox
// or Deinosaurus martis, but never as Leucomys placens. See
// docs/alien-taxonomy.md.
//
// Names are drawn from their own stream (alienTaxonomySeed), not the lore
// roster's: drawing them from the lore stream would shift every species
// rolled after the first, so adding scientific names would have re-rolled
// every existing seed's roster. They are flavor only and never touch
// gameplay.

// alienTaxonomySeed is the scientific-name stream's XOR key against
// cfg.Seed, distinct from every other stream's (see docs/rng-streams.md).
const alienTaxonomySeed = 0x6C62272E07BB0142

// taxonEntry is one word part and the condition a species must meet to use
// it. Mimic on a root marks an Earth animal the species resembles; on a
// prefix it means the prefix ("pseudo") only goes in front of a mimic root.
//
// Meaning is the English gloss ScientificEtymology shows for the part:
// "red" for erythro, "bear" for ursus, "of Mars" for martis.
//
// Gender is a root's grammatical gender, "m", "f" or "n": a genus takes the
// gender of the noun it ends in, so Pithecus is masculine, Medusa feminine
// and Zoon neuter whatever prefix comes first. An epithet that is a Latin
// adjective agreeing with it lists its other two forms in Feminine and
// Neuter (Form is the masculine): hirsutus, hirsuta, hirsutum. An
// invariable epithet (ferox, martis) leaves both empty.
type taxonEntry struct {
	Form     string        `yaml:"form"`
	Meaning  string        `yaml:"meaning"`
	Mimic    bool          `yaml:"mimic,omitempty"`
	Gender   string        `yaml:"gender,omitempty"`
	Feminine string        `yaml:"feminine,omitempty"`
	Neuter   string        `yaml:"neuter,omitempty"`
	When     nameCondition `yaml:"when"`
}

// agreeing is the epithet's form for a genus of the given gender. An
// invariable epithet, or an unknown gender (a hand-built test taxonomy that
// never set one), gets Form.
func (e taxonEntry) agreeing(gender string) string {
	switch {
	case gender == "f" && e.Feminine != "":
		return e.Feminine
	case gender == "n" && e.Neuter != "":
		return e.Neuter
	}
	return e.Form
}

// alienTaxonomy is alien-taxonomy.yaml: the word parts a scientific name is
// built from.
type alienTaxonomy struct {
	Prefixes []taxonEntry `yaml:"prefixes"`
	Roots    []taxonEntry `yaml:"roots"`
	Epithets []taxonEntry `yaml:"epithets"`
}

//go:embed alien-taxonomy.yaml
var embeddedAlienTaxonomy []byte

// defaultTaxonomy parses the embedded alien-taxonomy.yaml. Like
// defaultAlienNames, a parse failure is a bug in this repo, so it panics.
func defaultTaxonomy() alienTaxonomy {
	tx, err := loadTaxonomy(embeddedAlienTaxonomy, "alien-taxonomy.yaml (embedded)")
	if err != nil {
		panic(fmt.Sprintf("mars-sim: embedded alien-taxonomy.yaml: %v", err))
	}
	return tx
}

// loadTaxonomy parses and checks an alien-taxonomy.yaml document. Every form
// must be lowercase ASCII letters (they are glued together and capitalized
// as-is), and each list needs at least one unconditional entry so that a
// name can always be built, whatever the species rolled. Every entry needs a
// meaning, so ScientificEtymology can gloss any name built. Every root needs a
// gender, so an epithet that declines always knows which form to take, and
// an epithet that declines gives both of its other forms.
func loadTaxonomy(data []byte, name string) (alienTaxonomy, error) {
	var tx alienTaxonomy
	if err := yaml.Unmarshal(data, &tx); err != nil {
		return tx, fmt.Errorf("%s: %w", name, err)
	}
	for _, list := range []struct {
		key     string
		entries []taxonEntry
	}{{"prefixes", tx.Prefixes}, {"roots", tx.Roots}, {"epithets", tx.Epithets}} {
		unconditional := false
		for i, e := range list.entries {
			if !isLowerASCIIWord(e.Form) {
				return tx, fmt.Errorf("%s: %s entry %d: form %q must be lowercase letters a-z", name, list.key, i, e.Form)
			}
			if strings.TrimSpace(e.Meaning) == "" {
				return tx, fmt.Errorf("%s: %s %q needs a meaning", name, list.key, e.Form)
			}
			if e.When.isZero() && !e.Mimic {
				unconditional = true
			}
			if list.key == "roots" && e.Gender != "m" && e.Gender != "f" && e.Gender != "n" {
				return tx, fmt.Errorf("%s: root %q needs gender m, f or n", name, e.Form)
			}
			if list.key == "epithets" && (e.Feminine != "" || e.Neuter != "") &&
				(!isLowerASCIIWord(e.Feminine) || !isLowerASCIIWord(e.Neuter)) {
				return tx, fmt.Errorf("%s: epithet %q must give both feminine and neuter forms, lowercase a-z", name, e.Form)
			}
		}
		if !unconditional {
			return tx, fmt.Errorf("%s: %s needs at least one entry with no condition", name, list.key)
		}
	}
	return tx, nil
}

func isLowerASCIIWord(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

// scientificName draws a binomial for sp that is not already in used (the
// lower-cased names earlier species in the roster took). It retries the
// draw a few times on a collision, which is safe because nothing else draws
// from rng; if every attempt collides (only possible with a very large
// roster) it numbers the epithet so the name is still unique.
func scientificName(rng *rand.Rand, sp AlienSpecies, tx alienTaxonomy, used map[string]bool) string {
	roots := matchingTaxa(tx.Roots, sp)
	epithets := matchingTaxa(tx.Epithets, sp)
	var name string
	for attempt := 0; attempt < 16; attempt++ {
		root := pickTaxon(rng, roots)
		var prefixes []taxonEntry
		for _, p := range tx.Prefixes {
			if (!p.Mimic || root.Mimic) && p.When.matches(sp) {
				prefixes = append(prefixes, p)
			}
		}
		genus := joinTaxa(pickTaxon(rng, prefixes).Form, root.Form)
		name = capitalizeFirst(genus) + " " + pickTaxon(rng, epithets).agreeing(root.Gender)
		if !used[strings.ToLower(name)] {
			return name
		}
	}
	for n := 2; ; n++ {
		if s := fmt.Sprintf("%s %d", name, n); !used[strings.ToLower(s)] {
			return s
		}
	}
}

func matchingTaxa(entries []taxonEntry, sp AlienSpecies) []taxonEntry {
	var out []taxonEntry
	for _, e := range entries {
		if e.When.matches(sp) {
			out = append(out, e)
		}
	}
	return out
}

// pickTaxon draws one entry. An empty list (a taxonomy with no unconditional
// entry, which loadTaxonomy refuses) yields the zero entry rather than a
// panic, and joinTaxa copes with an empty form.
func pickTaxon(rng *rand.Rand, entries []taxonEntry) taxonEntry {
	if len(entries) == 0 {
		return taxonEntry{}
	}
	return entries[rng.IntN(len(entries))]
}

// joinTaxa glues a combining form to a root the way Greek and Latin names
// are built: a prefix's final vowel is dropped before a root that starts
// with one, so erythro + ops is Erythrops, not Erythroops, and pseudo +
// ursus is Pseudursus.
func joinTaxa(prefix, root string) string {
	if prefix != "" && root != "" && isVowel(prefix[len(prefix)-1]) && isVowel(root[0]) {
		prefix = prefix[:len(prefix)-1]
	}
	return prefix + root
}

func isVowel(c byte) bool {
	switch c {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

// TaxonGloss is one word part of a scientific name and what it means, as
// ScientificEtymology recovers it.
type TaxonGloss struct {
	Part    string // "prefix", "root" or "epithet"
	Form    string // the word part, unelided: "pseudo" in Pseudursus; an epithet as the name spells it, "hirsuta"
	Meaning string // "false", "bear", "hairy"
}

var etymologyTaxonomy = sync.OnceValue(defaultTaxonomy)

// ScientificEtymology takes a binomial apart again into the word parts
// scientificName built it from, so a frontend can explain it: Pseudursus
// ares is pseudo- "false" + -ursus "bear", ares "Mars". It runs the naming
// logic backwards against the embedded taxonomy rather than reading parts
// stored on the species, so it needs no save state and explains names in
// saves made before it existed. A name it cannot take apart (a hand-built
// species, or a word part since removed from the file) gives nil; the
// numbering on an exhausted roster ("Areozoon martis 2") is ignored.
func ScientificEtymology(name string) []TaxonGloss {
	return etymology(etymologyTaxonomy(), name)
}

func etymology(tx alienTaxonomy, name string) []TaxonGloss {
	words := strings.Fields(name)
	if len(words) < 2 {
		return nil
	}
	genus, epithet := strings.ToLower(words[0]), words[1]

	// The epithet is a single entry, in whichever gender's form it took.
	var ep *taxonEntry
	gender := "" // the gender its form pins down, "" for an invariable one
	for i, e := range tx.Epithets {
		switch epithet {
		case e.Form:
			if e.Feminine != "" {
				gender = "m"
			}
		case e.Feminine:
			gender = "f"
		case e.Neuter:
			gender = "n"
		default:
			continue
		}
		ep = &tx.Epithets[i]
		break
	}
	if ep == nil {
		return nil
	}

	// The genus is prefix + root with joinTaxa's elision undone. Try every
	// root the genus ends in and every prefix that joins with it to spell
	// the genus, under the same rules scientificName draws by: pseudo- only
	// before a mimic root, and a root agreeing with the epithet's gender.
	// File order decides between two readings, so the answer is stable.
	for _, r := range tx.Roots {
		if !strings.HasSuffix(genus, r.Form) || (gender != "" && r.Gender != gender) {
			continue
		}
		for _, p := range tx.Prefixes {
			if (p.Mimic && !r.Mimic) || joinTaxa(p.Form, r.Form) != genus {
				continue
			}
			return []TaxonGloss{
				{Part: "prefix", Form: p.Form, Meaning: p.Meaning},
				{Part: "root", Form: r.Form, Meaning: r.Meaning},
				{Part: "epithet", Form: epithet, Meaning: ep.Meaning},
			}
		}
	}
	return nil
}
