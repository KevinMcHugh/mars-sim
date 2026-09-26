package sim

import (
	_ "embed"
	"fmt"
	"math/rand"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---- Alien names: a configurable, condition-gated name pool -----------------
//
// A rolled AlienSpecies needs a name -- what colonists call it -- and which
// names make sense depends on what the species turned out to look like: a
// six-legged armored thing reads as a "beetle," a limbless scaly one as a
// "snake." Rather than hand-coding that mapping, the name pool lives in data
// (alien-names.yaml, compiled in via go:embed as defaultAlienNames, and
// overridable at runtime with -alien-names -- see docs/lore.md) as a list of
// {name, plural, when} entries, where `when` is a small boolean condition
// tree over the species' build. pickAlienName collects every entry whose
// condition matches the rolled species and draws one at random, so
// overlapping conditions (a name that fits several kinds of species) are
// normal, not an error.

// AlienNameFileName is the file mars-sim looks for in the working directory,
// alongside mars-sim.yaml and director.yaml. It is optional: a run with no
// file falls back to defaultAlienNames(), the same set compiled in from
// alien-names.yaml. See docs/lore.md.
const AlienNameFileName = "alien-names.yaml"

// AlienNameEntry is one candidate name and the condition a rolled species
// must satisfy to be eligible for it. Emoji is a matching set of candidate
// glyphs for the same name -- a rolled species draws one of them the same
// way it draws one of the eligible names, so "reptile" can turn up as 🦎 one
// seed and 🐍 the next. It is optional and purely cosmetic: this package
// (and everything in it, including a species' rolled Emoji field) never
// interprets the string as anything but data -- rendering it safely is the
// frontend's job. See docs/lore.md.
type AlienNameEntry struct {
	Singular string        `yaml:"name"`
	Plural   string        `yaml:"plural"`
	Emoji    []string      `yaml:"emoji,omitempty"`
	When     nameCondition `yaml:"when"`
}

// nameCondition is a boolean condition tree over a rolled species' build. The
// zero value (no fields set) matches every species, which is how an
// unconditional name like "alien" or "xeno" is expressed: it simply omits
// `when`. All, Any, and Not combine sub-conditions; every other field is a
// leaf predicate, and a leaf that is left unset (nil, or "") is not checked
// at all rather than failing the match -- a condition tests only the traits
// it names.
type nameCondition struct {
	All []nameCondition `yaml:"all,omitempty"`
	Any []nameCondition `yaml:"any,omitempty"`
	Not *nameCondition  `yaml:"not,omitempty"`

	Temperament string `yaml:"temperament,omitempty"` // friendly | cautious | hostile
	Skin        string `yaml:"skin,omitempty"`        // smooth | scaly | furry | armored | bony | chitinous | slimy
	Color       string `yaml:"color,omitempty"`
	Pattern     string `yaml:"pattern,omitempty"` // solid | striped | spotted
	Height      string `yaml:"height,omitempty"`  // tiny | small | average | large | huge
	Weight      string `yaml:"weight,omitempty"`  // tiny | small | average | large | huge
	Tail        *bool  `yaml:"tail,omitempty"`

	Legs  *intCondition `yaml:"legs,omitempty"`
	Arms  *intCondition `yaml:"arms,omitempty"`
	Limbs *intCondition `yaml:"limbs,omitempty"`
	Eyes  *intCondition `yaml:"eyes,omitempty"`
}

// intCondition is a numeric comparison against one of a species' counts. Every
// set field must hold for the condition to match; combine with All/Any/Not
// in the enclosing nameCondition for anything looser than "all of these."
type intCondition struct {
	Eq  *int `yaml:"eq,omitempty"`
	Gt  *int `yaml:"gt,omitempty"`
	Gte *int `yaml:"gte,omitempty"`
	Lt  *int `yaml:"lt,omitempty"`
	Lte *int `yaml:"lte,omitempty"`
}

// isZero reports whether c constrains nothing at all -- the unconditional
// case an entry with no `when` key parses to, and so matches every species.
func (c nameCondition) isZero() bool {
	return len(c.All) == 0 && len(c.Any) == 0 && c.Not == nil &&
		c.Temperament == "" && c.Skin == "" && c.Color == "" && c.Pattern == "" && c.Height == "" && c.Weight == "" &&
		c.Tail == nil && c.Legs == nil && c.Arms == nil && c.Limbs == nil && c.Eyes == nil
}

func (c intCondition) matches(v int) bool {
	if c.Eq != nil && v != *c.Eq {
		return false
	}
	if c.Gt != nil && v <= *c.Gt {
		return false
	}
	if c.Gte != nil && v < *c.Gte {
		return false
	}
	if c.Lt != nil && v >= *c.Lt {
		return false
	}
	if c.Lte != nil && v > *c.Lte {
		return false
	}
	return true
}

// matches reports whether sp satisfies c. Leaf fields left unset are skipped
// (not a failure), so a condition only constrains the traits it actually
// names; All/Any/Not nest to build up anything looser or stricter than a
// flat conjunction.
func (c nameCondition) matches(sp AlienSpecies) bool {
	for _, sub := range c.All {
		if !sub.matches(sp) {
			return false
		}
	}
	if len(c.Any) > 0 {
		matched := false
		for _, sub := range c.Any {
			if sub.matches(sp) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if c.Not != nil && c.Not.matches(sp) {
		return false
	}
	if c.Temperament != "" && !strings.EqualFold(c.Temperament, sp.Temperament.String()) {
		return false
	}
	if c.Skin != "" && !strings.EqualFold(c.Skin, sp.Skin.String()) {
		return false
	}
	if c.Color != "" && !strings.EqualFold(c.Color, sp.Color) {
		return false
	}
	if c.Pattern != "" && !strings.EqualFold(c.Pattern, sp.Pattern.String()) {
		return false
	}
	if c.Height != "" && !strings.EqualFold(c.Height, sp.HeightTier().String()) {
		return false
	}
	if c.Weight != "" && !strings.EqualFold(c.Weight, sp.WeightTier().String()) {
		return false
	}
	if c.Tail != nil && *c.Tail != sp.Tail {
		return false
	}
	if c.Legs != nil && !c.Legs.matches(sp.Limbs-sp.Arms) {
		return false
	}
	if c.Arms != nil && !c.Arms.matches(sp.Arms) {
		return false
	}
	if c.Limbs != nil && !c.Limbs.matches(sp.Limbs) {
		return false
	}
	if c.Eyes != nil && !c.Eyes.matches(sp.Eyes) {
		return false
	}
	return true
}

// pickAlienName draws one name at random from every entry in names whose
// condition matches sp, then, independently, one emoji at random from that
// entry's own candidates (if it listed any) -- two rolls, so two species
// that land on the same name need not land on the same glyph. rng is the
// caller's lore stream (see rollAlienSpecies), so both picks stay part of
// the same reproducible roll. An empty or wholly non-matching pool falls
// back to "alien"/"aliens" with no emoji -- unreachable with
// defaultAlienNames() (its first entry is unconditional), but a
// user-supplied -alien-names file could in principle define nothing
// unconditional, and a species still needs a name either way.
func pickAlienName(rng *rand.Rand, sp AlienSpecies, names []AlienNameEntry) (singular, plural, emoji string) {
	var candidates []AlienNameEntry
	for _, e := range names {
		if e.When.matches(sp) {
			candidates = append(candidates, e)
		}
	}
	if len(candidates) == 0 {
		return "alien", "aliens", ""
	}
	e := candidates[rng.Intn(len(candidates))]
	if len(e.Emoji) > 0 {
		emoji = e.Emoji[rng.Intn(len(e.Emoji))]
	}
	if e.Plural == "" {
		return e.Singular, e.Singular + "s", emoji
	}
	return e.Singular, e.Plural, emoji
}

//go:embed alien-names.yaml
var embeddedAlienNames []byte

// defaultAlienNames is the built-in name pool, parsed once from the embedded
// alien-names.yaml. It is what every world uses unless Config.AlienNames was
// populated from a file (see main.go's -alien-names loading), which keeps it
// available to any caller that builds a World directly -- tests included --
// without going through the CLI's file-loading path at all.
func defaultAlienNames() []AlienNameEntry {
	names, err := LoadAlienNames(embeddedAlienNames, "alien-names.yaml (embedded)")
	if err != nil {
		// The embedded file is compiled into the binary; a parse failure here
		// is a build-time bug in this repo, not a runtime condition to handle
		// gracefully.
		panic(fmt.Sprintf("mars-sim: embedded alien-names.yaml: %v", err))
	}
	return names
}

// rawAlienNames is alien-names.yaml's top-level shape.
type rawAlienNames struct {
	Names []AlienNameEntry `yaml:"names"`
}

// LoadAlienNames parses an alien-names.yaml document into a name pool. name
// is used in error messages. Every entry needs a non-empty `name`; `plural`
// falls back to `name` + "s" when omitted (pickAlienName applies the same
// fallback, so this only matters for validating the file itself).
func LoadAlienNames(data []byte, name string) ([]AlienNameEntry, error) {
	var raw rawAlienNames
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	for i, e := range raw.Names {
		if strings.TrimSpace(e.Singular) == "" {
			return nil, fmt.Errorf("%s: entry %d: name is required", name, i)
		}
	}
	return raw.Names, nil
}
