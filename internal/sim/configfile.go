package sim

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFileName is the config file mars-sim looks for in the working
// directory. It is meant to be committed: a colony's balance is part of the
// project, the same way .editorconfig or a Makefile is.
const ConfigFileName = "mars-sim.yaml"

// SeedKey is the config-file setting for the world seed. It is not an ordinary
// knob (see Config.Seed) because 0 does not mean zero — it means "pick a fresh
// time-based seed", the same as the -seed flag.
const SeedKey = "seed"

// A Knob is one tunable, discovered by reflection from the `cfg` struct tags on
// Config (and on the NeedSpec entries inside it). One knob description drives
// three surfaces that used to be maintained by hand and drift apart: the
// command-line flag, the key in the config file, and the commented template
// that documents both.
//
// Knobs are bound to a specific Config: Ptr points into the struct passed to
// Knobs, so a caller can hand it straight to flag.IntVar.
type Knob struct {
	// Name is the flag name and, for a top-level knob, also the file key.
	// Nested knobs flatten their path with dashes, so the need spec that the
	// file writes as needs.food.rise is -need-food-rise on the command line.
	Name string
	// Key is the dotted path in the config file: "colonists", "needs.food.rise".
	Key string
	// Path is Key split into its segments.
	Path []string
	// Doc is the one-line description shown in -h and above the key in the
	// template.
	Doc string
	// Section, when non-empty, starts a new group in the template at this knob.
	Section string
	// Ptr points at the field inside the Config passed to Knobs: *int, *int64
	// or *bool.
	Ptr any
}

// Knobs returns every tunable in cfg, in struct order, with pointers into cfg.
func Knobs(cfg *Config) []Knob {
	var out []Knob
	collectKnobs(reflect.ValueOf(cfg).Elem(), nil, "", &out)
	return out
}

// collectKnobs walks a struct value, appending a Knob for every `cfg`-tagged
// scalar field and recursing into named spec arrays.
func collectKnobs(v reflect.Value, prefix []string, section string, out *[]Knob) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag, ok := f.Tag.Lookup("cfg")
		if !ok {
			continue
		}
		if s := f.Tag.Get("sec"); s != "" {
			section = s
		}
		path := append(append([]string{}, prefix...), tag)
		field := v.Field(i)

		if field.Kind() == reflect.Array {
			for n := 0; n < field.Len(); n++ {
				// Copy the prefix per element: appending to path in a loop
				// would hand every spec the same backing array.
				elem := append(append([]string{}, path...), configArrayElementName(tag, n))
				collectKnobs(field.Index(n), elem, section, out)
				section = "" // the section heading belongs to the first knob only
			}
			continue
		}

		*out = append(*out, Knob{
			Name:    knobFlagName(path),
			Key:     strings.Join(path, "."),
			Path:    path,
			Doc:     f.Tag.Get("doc"),
			Section: section,
			Ptr:     field.Addr().Interface(),
		})
		section = ""
	}
}

func configArrayElementName(tag string, index int) string {
	switch tag {
	case "needs":
		return NeedKind(index).String()
	case "focuses":
		return FocusKind(index).String()
	default:
		panic("unsupported config spec array: " + tag)
	}
}

// knobFlagName flattens a config-file path into a command-line flag name.
// Top-level knobs keep their key verbatim (-colonists); nested ones read as a
// phrase (-need-food-rise), which is why the prefix is singular.
func knobFlagName(path []string) string {
	if len(path) == 1 {
		return path[0]
	}
	if path[0] == "needs" {
		return "need-" + strings.Join(path[1:], "-")
	}
	if path[0] == "focuses" {
		return "focus-" + strings.Join(path[1:], "-")
	}
	return strings.Join(path, "-")
}

// ConfigTemplate renders every knob as a commented-out YAML file at its default
// value: a settings file that changes nothing until a line is uncommented, and
// that documents the full set of knobs where a player will look for them.
//
// The seed is included by hand because it is not a knob; everything else comes
// from the struct tags, so a new tunable shows up here the moment it is added.
func ConfigTemplate() []byte {
	def := DefaultConfig()
	var b bytes.Buffer

	b.WriteString(`# mars-sim settings
#
# This file is the middle ground between the defaults compiled into
# internal/sim/config.go and the flags you type at the shell: a set of options
# you can commit. Every setting below is shown at its default value and
# commented out, so the file as generated changes nothing. Uncomment a line to
# change it.
#
# Precedence, lowest to highest:
#
#   DefaultConfig() in internal/sim/config.go
#   this file (` + ConfigFileName + ` in the working directory, or -config PATH)
#   command-line flags
#
# So a value set here becomes the default the flags start from: run with
# -h to see the effective defaults, and pass a flag to override this file for
# one run. Use -config "" to ignore the file entirely.
#
# Regenerate this file with:  go run . -print-config > ` + ConfigFileName + `
# (that resets it to the defaults, so re-apply your edits afterwards).

# The world seed. Same seed + same code => same game. 0 means pick a fresh
# seed from the clock on every run, which is the default.
# ` + SeedKey + `: 0
`)

	var prev []string
	for _, k := range Knobs(&def) {
		if k.Section != "" {
			fmt.Fprintf(&b, "\n%s\n", sectionHeading(k.Section))
			if k.Path[0] == "needs" {
				b.WriteString(needsPreamble)
			}
			prev = nil
		}
		// Emit any parent keys this knob needs ("needs:", "  food:") that the
		// previous knob did not already open, with one blank line before the
		// shallowest of them so each need reads as its own block.
		parents := k.Path[:len(k.Path)-1]
		opened := false
		for d := range parents {
			if !opened && d < len(prev) && prev[d] == parents[d] {
				continue
			}
			if !opened {
				b.WriteString("\n")
				opened = true
			}
			fmt.Fprintf(&b, "# %s%s:\n", indent(d), parents[d])
		}
		prev = parents

		d := len(parents)
		if k.Doc != "" {
			// "## " marks prose, "# " marks a setting you can switch on by
			// deleting those two characters. Doing that to a doc line leaves a
			// comment rather than breaking the file.
			fmt.Fprintf(&b, "## %s%s\n", indent(d), k.Doc)
		}
		fmt.Fprintf(&b, "# %s%s: %s\n\n", indent(d), k.Path[len(k.Path)-1], formatValue(k.Ptr))
	}

	return bytes.ReplaceAll(b.Bytes(), []byte("\n\n\n"), []byte("\n\n"))
}

// needsPreamble warns about the one sharp edge of an all-commented file: a
// nested value needs its parents uncommented too.
const needsPreamble = `
# One spec per colonist need. These are nested, so uncommenting a value means
# uncommenting the "needs:" and need-name lines above it as well.
`

func sectionHeading(name string) string {
	const width = 74
	line := "# ── " + name + " "
	if n := width - len([]rune(line)); n > 0 {
		line += strings.Repeat("─", n)
	}
	return line
}

func indent(depth int) string { return strings.Repeat("  ", depth) }

func formatValue(ptr any) string {
	switch p := ptr.(type) {
	case *int:
		return fmt.Sprint(*p)
	case *int64:
		return fmt.Sprint(*p)
	case *bool:
		return fmt.Sprint(*p)
	default:
		return fmt.Sprint(reflect.ValueOf(ptr).Elem().Interface())
	}
}

// ApplyConfigFile applies the settings in a config file to cfg and reports the
// keys it set, in file order. name is used in error messages.
//
// Unknown keys are an error rather than a warning: a typo in a committed
// settings file that silently does nothing is the failure mode this file
// exists to avoid.
func ApplyConfigFile(cfg *Config, data []byte, name string) ([]string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, nil // empty, or every line commented out
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s:%d: want a mapping of settings at the top level", name, root.Line)
	}

	index := map[string]Knob{}
	for _, k := range Knobs(cfg) {
		index[k.Key] = k
	}

	var set []string
	if err := applyMapping(cfg, root, nil, index, name, &set); err != nil {
		return nil, err
	}
	return set, nil
}

// applyMapping walks one YAML mapping, recursing into nested ones so a knob's
// dotted key is matched by its position in the file.
func applyMapping(cfg *Config, node *yaml.Node, prefix []string, index map[string]Knob, name string, set *[]string) error {
	seen := map[string]int{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode, valNode := node.Content[i], node.Content[i+1]
		key := keyNode.Value
		if line, dup := seen[key]; dup {
			return fmt.Errorf("%s:%d: %q is set twice (already set on line %d)", name, keyNode.Line, key, line)
		}
		seen[key] = keyNode.Line

		path := strings.Join(append(append([]string{}, prefix...), key), ".")

		if valNode.Kind == yaml.MappingNode {
			if !isKnobPrefix(index, path) {
				return unknownKeyError(name, keyNode, path)
			}
			if err := applyMapping(cfg, valNode, append(prefix, key), index, name, set); err != nil {
				return err
			}
			continue
		}

		if path == SeedKey && len(prefix) == 0 {
			var seed int64
			if err := valNode.Decode(&seed); err != nil {
				return fmt.Errorf("%s:%d: %s: want a whole number, got %q", name, valNode.Line, path, valNode.Value)
			}
			if seed != 0 { // 0 keeps the time-based seed, like -seed 0
				cfg.Seed = seed
			}
			*set = append(*set, path)
			continue
		}

		knob, ok := index[path]
		if !ok {
			return unknownKeyError(name, keyNode, path)
		}
		if err := assign(knob, valNode); err != nil {
			return fmt.Errorf("%s:%d: %s: %w", name, valNode.Line, path, err)
		}
		*set = append(*set, path)
	}
	return nil
}

// assign decodes one scalar node into the knob's field, rejecting anything the
// field cannot hold. yaml would happily give us 0 for a quoted string.
func assign(k Knob, node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("want a single value")
	}
	switch p := k.Ptr.(type) {
	case *int:
		var v int
		if err := node.Decode(&v); err != nil {
			return fmt.Errorf("want a whole number, got %q", node.Value)
		}
		*p = v
	case *int64:
		var v int64
		if err := node.Decode(&v); err != nil {
			return fmt.Errorf("want a whole number, got %q", node.Value)
		}
		*p = v
	case *bool:
		var v bool
		if err := node.Decode(&v); err != nil {
			return fmt.Errorf("want true or false, got %q", node.Value)
		}
		*p = v
	default:
		return fmt.Errorf("unsupported setting type %T", k.Ptr)
	}
	return nil
}

// isKnobPrefix reports whether any knob lives under this path, which is what
// makes a nested mapping ("needs:", "needs.food:") legal.
func isKnobPrefix(index map[string]Knob, path string) bool {
	for key := range index {
		if strings.HasPrefix(key, path+".") {
			return true
		}
	}
	return false
}

func unknownKeyError(name string, node *yaml.Node, path string) error {
	return fmt.Errorf("%s:%d: unknown setting %q (run with -print-config to list every setting)", name, node.Line, path)
}

// ConfigKeys returns every settable config-file key, sorted. Tests and error
// messages use it; nothing in the simulation does.
func ConfigKeys() []string {
	def := DefaultConfig()
	keys := []string{SeedKey}
	for _, k := range Knobs(&def) {
		keys = append(keys, k.Key)
	}
	sort.Strings(keys)
	return keys
}
