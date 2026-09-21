package sim

import (
	"bytes"
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

// traits.yaml holds how each trait reacts to what happens: which tags it cares
// about, and what it scales when one of them turns up. Embedded, like
// events.yaml, so the binary stays self-contained and a malformed file is a
// build-time mistake.
//
//go:embed traits.yaml
var embeddedTraitRules []byte

func init() {
	if err := loadTraitRules(embeddedTraitRules); err != nil {
		panic("traits.yaml: " + err.Error())
	}
}

type traitRuleFile struct {
	Rules []traitRuleData `yaml:"rules"`
}

type traitRuleData struct {
	Trait    string   `yaml:"trait"`
	Any      []string `yaml:"any"`
	All      []string `yaml:"all"`
	None     []string `yaml:"none"`
	Impact   int      `yaml:"impact"`
	Charge   int      `yaml:"charge"`
	Grip     int      `yaml:"grip"`
	Valence  int      `yaml:"valence"`
	WearRate int      `yaml:"wear-rate"`
}

// loadTraitRules fills traitRules from YAML, in file order, because order is
// application order. Strict for the same reason the event loader is: this was
// a Go literal where the compiler checked every trait and tag name, and a rule
// that quietly matches nothing or does nothing is indistinguishable from one
// that works until someone checks the numbers.
func loadTraitRules(data []byte) error {
	var file traitRuleFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return err
	}

	traitByName := make(map[string]Trait, numTraits)
	for t := Trait(0); t < numTraits; t++ {
		traitByName[traitNames[t]] = t
	}

	rules := make([]traitRule, 0, len(file.Rules))
	for i, def := range file.Rules {
		trait, ok := traitByName[def.Trait]
		if !ok {
			return fmt.Errorf("rule %d: %q is not a known trait", i, def.Trait)
		}
		r := traitRule{
			Trait:    trait,
			Impact:   def.Impact,
			Charge:   def.Charge,
			Grip:     def.Grip,
			Valence:  def.Valence,
			WearRate: def.WearRate,
		}
		for _, m := range []struct {
			field string
			names []string
			into  *EventTag
		}{{"any", def.Any, &r.Any}, {"all", def.All, &r.All}, {"none", def.None, &r.None}} {
			tags, err := parseEventTags(m.names)
			if err != nil {
				return fmt.Errorf("rule %d (%s): %s: %w", i, def.Trait, m.field, err)
			}
			*m.into = tags
		}
		if r.Impact == 0 && r.Charge == 0 && r.Grip == 0 && r.Valence == 0 && r.WearRate == 0 {
			return fmt.Errorf("rule %d (%s) does nothing", i, def.Trait)
		}
		rules = append(rules, r)
	}

	traitRules = rules
	return nil
}
