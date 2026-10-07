package sim

import (
	_ "embed"
	"fmt"
	"math/rand/v2"
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
// normal, not an error. A `group` entry lists several names under one
// condition (repts, reptoids, scalies, lizards), and no two species in a
// roster get the same name.

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
	Skin        string `yaml:"skin,omitempty"`        // smooth | scaly | furry | armored | bony | chitinous | slimy | rocky | woody | mossy | gelatinous | hairy | feathered
	Color       string `yaml:"color,omitempty"`
	Pattern     string `yaml:"pattern,omitempty"` // solid | striped | spotted
	Height      string `yaml:"height,omitempty"`  // tiny | small | average | large | huge
	Weight      string `yaml:"weight,omitempty"`  // tiny | small | average | large | huge
	Tail        *bool  `yaml:"tail,omitempty"`
	Wings       *bool  `yaml:"wings,omitempty"`

	Legs  *intCondition `yaml:"legs,omitempty"`
	Arms  *intCondition `yaml:"arms,omitempty"`
	Limbs *intCondition `yaml:"limbs,omitempty"`
	Eyes  *intCondition `yaml:"eyes,omitempty"`

	// Graded features (alien_anatomy.go): counts for horns and antler tines,
	// grades 1-3 for spines, shell and claws, 1-2 for a stinger, and what a
	// tail ends in. apex is the rare deadly species (alien_weapons.go). A
	// name gated on any of these is a feature name: the first naming pass
	// never sees one match (a species has no features yet when it is named),
	// and the feature-naming pass may rename a species to it (see
	// renameForFeatures).
	Horns   *intCondition `yaml:"horns,omitempty"`
	Antlers *intCondition `yaml:"antlers,omitempty"`
	Spines  *intCondition `yaml:"spines,omitempty"`
	Shell   *intCondition `yaml:"shell,omitempty"`
	Claws   *intCondition `yaml:"claws,omitempty"`
	Stinger *intCondition `yaml:"stinger,omitempty"`
	TailTip string        `yaml:"tail-tip,omitempty"` // plain | club | spiked-club | stinger
	Apex    *bool         `yaml:"apex,omitempty"`
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
		c.Tail == nil && c.Wings == nil && c.Legs == nil && c.Arms == nil && c.Limbs == nil && c.Eyes == nil &&
		!c.featureLeaf()
}

// featureLeaf reports whether c itself (not its sub-conditions) tests a
// graded feature or apex.
func (c nameCondition) featureLeaf() bool {
	return c.Horns != nil || c.Antlers != nil || c.Spines != nil || c.Shell != nil ||
		c.Claws != nil || c.Stinger != nil || c.TailTip != "" || c.Apex != nil
}

// usesFeatures reports whether c tests a graded feature or apex anywhere in
// its tree: whether an entry under it is a feature name.
func (c nameCondition) usesFeatures() bool {
	if c.featureLeaf() {
		return true
	}
	for _, sub := range c.All {
		if sub.usesFeatures() {
			return true
		}
	}
	for _, sub := range c.Any {
		if sub.usesFeatures() {
			return true
		}
	}
	return c.Not != nil && c.Not.usesFeatures()
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
	if c.Wings != nil && *c.Wings != sp.Wings {
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
	a := sp.Anatomy
	for _, leaf := range []struct {
		c *intCondition
		v int
	}{{c.Horns, a.Horns}, {c.Antlers, a.Antlers}, {c.Spines, a.Spines}, {c.Shell, a.Shell}, {c.Claws, a.Claws}, {c.Stinger, a.Stinger}} {
		if leaf.c != nil && !leaf.c.matches(leaf.v) {
			return false
		}
	}
	if c.TailTip != "" && !strings.EqualFold(c.TailTip, a.TailTip.String()) {
		return false
	}
	if c.Apex != nil && *c.Apex != sp.Apex {
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
//
// used holds the names (lower-cased singulars) earlier species in the same
// roster already took; nil means none. A name in it is not a candidate, so
// two species never share a name while the pool still has a fitting one to
// give. When every matching name is taken, the draw falls back to the full
// matching set and qualifies the result with the species' color
// ("green-striped grelk"), then with a number, until it is unused. The
// caller records the returned singular in used. See docs/lore.md.
func pickAlienName(rng *rand.Rand, sp AlienSpecies, names []AlienNameEntry, used map[string]bool) (singular, plural, emoji string) {
	var matching, fresh []AlienNameEntry
	for _, e := range names {
		if !e.When.matches(sp) {
			continue
		}
		matching = append(matching, e)
		if !used[strings.ToLower(e.Singular)] {
			fresh = append(fresh, e)
		}
	}
	candidates := fresh
	if len(candidates) == 0 {
		candidates = matching
	}
	if len(candidates) == 0 {
		singular, plural = "alien", "aliens"
	} else {
		e := candidates[rng.IntN(len(candidates))]
		if len(e.Emoji) > 0 {
			emoji = e.Emoji[rng.IntN(len(e.Emoji))]
		}
		singular, plural = e.Singular, e.Plural
		if plural == "" {
			plural = singular + "s"
		}
	}
	singular, plural = distinctAlienName(sp, singular, plural, used)
	return singular, plural, emoji
}

// distinctAlienName returns singular/plural unchanged if the name is not in
// used, else the first of "<color> <name>", "<color> <name> 2", "<color>
// <name> 3", ... that is free. The color qualifier is tried first because it
// reads like something colonists would actually say to tell two kinds of
// grelk apart; the number is only the guarantee that the loop ends.
func distinctAlienName(sp AlienSpecies, singular, plural string, used map[string]bool) (string, string) {
	if !used[strings.ToLower(singular)] {
		return singular, plural
	}
	if c := sp.ColorPhrase(); c != "" {
		singular, plural = c+" "+singular, c+" "+plural
		if !used[strings.ToLower(singular)] {
			return singular, plural
		}
	}
	for n := 2; ; n++ {
		s := fmt.Sprintf("%s %d", singular, n)
		if !used[strings.ToLower(s)] {
			return s, fmt.Sprintf("%s %d", plural, n)
		}
	}
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
	Names []rawAlienNameEntry `yaml:"names"`
}

// rawAlienNameEntry is one entry as written in the file: either a single
// name (the AlienNameEntry fields) or a name group -- several names sharing
// one `when` and one emoji list -- under `group`. LoadAlienNames expands a
// group into one AlienNameEntry per name, so nothing past loading ever sees
// a group.
type rawAlienNameEntry struct {
	AlienNameEntry `yaml:",inline"`
	Group          []AlienNameForm `yaml:"group,omitempty"`
}

// AlienNameForm is one name in a group: a singular and an optional plural.
// In YAML it is either a mapping ({name: scaly, plural: scalies}) or, when
// the plural is just the name plus "s", a bare string (rept).
type AlienNameForm struct {
	Singular string `yaml:"name"`
	Plural   string `yaml:"plural"`
}

// UnmarshalYAML accepts a bare string as shorthand for {name: <string>}.
func (f *AlienNameForm) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		f.Singular = n.Value
		return nil
	}
	type plain AlienNameForm
	return n.Decode((*plain)(f))
}

// LoadAlienNames parses an alien-names.yaml document into a name pool. name
// is used in error messages. Every entry needs either a non-empty `name` or
// a non-empty `group` (not both), and every name in a group needs a `name`;
// `plural` is filled in as `name` + "s" when omitted (pickAlienName applies
// the same fallback for entries built in code). Groups are expanded in
// place, in file order, into one entry per name -- exactly the pool the file
// would make if each name were written out with its own copy of the group's
// `when` and `emoji` -- so a group is shorthand, not a different weighting.
func LoadAlienNames(data []byte, name string) ([]AlienNameEntry, error) {
	var raw rawAlienNames
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	var out []AlienNameEntry
	for i, e := range raw.Names {
		hasName := strings.TrimSpace(e.Singular) != ""
		switch {
		case hasName && len(e.Group) > 0:
			return nil, fmt.Errorf("%s: entry %d: set name or group, not both", name, i)
		case hasName:
			out = append(out, withDefaultPlural(e.AlienNameEntry))
		case len(e.Group) > 0:
			if e.Plural != "" {
				return nil, fmt.Errorf("%s: entry %d: plural belongs on each name in the group", name, i)
			}
			for j, f := range e.Group {
				if strings.TrimSpace(f.Singular) == "" {
					return nil, fmt.Errorf("%s: entry %d: group name %d: name is required", name, i, j)
				}
				entry := e.AlienNameEntry
				entry.Singular, entry.Plural = f.Singular, f.Plural
				out = append(out, withDefaultPlural(entry))
			}
		default:
			return nil, fmt.Errorf("%s: entry %d: name (or group) is required", name, i)
		}
	}
	return out, nil
}

func withDefaultPlural(e AlienNameEntry) AlienNameEntry {
	if e.Plural == "" {
		e.Plural = e.Singular + "s"
	}
	return e
}

// alienFeatureNameSeed separates the feature-naming pass's stream from the
// other lore streams.
const alienFeatureNameSeed = 0x0801F2E2858EFC16

// featureRenamePercent is how often a species whose features fit a feature
// name takes one, when one is free: often enough that a crown of horns
// usually shows in the name, not so often that every horned species is a
// "crownhorn".
const featureRenamePercent = 60

// renameForFeatures is the second naming pass, after anatomy and the apex
// roll: a species whose features fit one of the feature names (entries
// gated on horns, a stinger, apex and the like; see usesFeatures) takes one,
// featureRenamePercent of the time, freeing the name the first pass gave it.
// The first pass runs before anatomy exists, so it can never pick a feature
// name, and this pass draws from its own stream: no species' build,
// temperament or first-pass name moved when it was added. used is the
// roster's taken names, kept current.
func renameForFeatures(cfg Config, roster []AlienSpecies, names []AlienNameEntry, used map[string]bool) {
	rng := newRand(cfg.Seed ^ alienFeatureNameSeed)
	for i := range roster {
		sp := &roster[i]
		roll := rng.IntN(100) // one draw per species, whatever it matches
		var fresh []AlienNameEntry
		for _, e := range names {
			if e.When.usesFeatures() && e.When.matches(*sp) && !used[strings.ToLower(e.Singular)] {
				fresh = append(fresh, e)
			}
		}
		if len(fresh) == 0 || roll >= featureRenamePercent {
			continue
		}
		e := fresh[rng.IntN(len(fresh))]
		emoji := ""
		if len(e.Emoji) > 0 {
			emoji = e.Emoji[rng.IntN(len(e.Emoji))]
		}
		plural := e.Plural
		if plural == "" {
			plural = e.Singular + "s"
		}
		delete(used, strings.ToLower(sp.Singular))
		sp.Singular, sp.Plural = distinctAlienName(*sp, e.Singular, plural, used)
		sp.Emoji = emoji
		used[strings.ToLower(sp.Singular)] = true
	}
}
