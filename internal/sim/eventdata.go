package sim

import (
	"bytes"
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// events.yaml holds what each life event means: its tags, how much it moves a
// colonist, the attention it creates, and the line a run of them collapses to.
// It is embedded, so the binary is still self-contained and a malformed file
// is a build-time mistake rather than something a player can trip over.
//
// What is *not* in here is when an event happens, who it happens to, and what
// its text says. Those live at the emit sites, because they are woven into the
// game logic that produces them -- "a colonist was bitten" is a fact about a
// bite, not a row in a table. Putting the trigger in data would mean inventing
// a language for conditions over world state, which is the same trap the trait
// rules avoided by reacting to tags.
//
//go:embed events.yaml
var embeddedEventDefinitions []byte

func init() {
	if err := loadEventDefinitions(embeddedEventDefinitions); err != nil {
		panic("events.yaml: " + err.Error())
	}
}

// parseEventTags turns the names a data file uses into the bitmask the engine
// matches on.
func parseEventTags(names []string) (EventTag, error) {
	var tags EventTag
	for _, n := range names {
		var bit EventTag
		for _, known := range eventTagNames {
			if known.Name == n {
				bit = known.Tag
				break
			}
		}
		if bit == 0 {
			return 0, fmt.Errorf("%q is not a known tag", n)
		}
		tags |= bit
	}
	return tags, nil
}

type eventDefinitionFile struct {
	Events map[string]eventDefinition `yaml:"events"`
}

type eventDefinition struct {
	Tags        []string       `yaml:"tags"`
	Impact      int            `yaml:"impact"`
	Fresh       moodVectorData `yaml:"fresh"`
	Worn        moodVectorData `yaml:"worn"`
	CollapsesTo string         `yaml:"collapses-to"`
	Stimulus    *stimulusData  `yaml:"stimulus"`
}

type moodVectorData struct {
	Charge  int `yaml:"charge"`
	Grip    int `yaml:"grip"`
	Valence int `yaml:"valence"`
}

func (v moodVectorData) vector() MoodVector {
	return MoodVector{Charge: v.Charge, Grip: v.Grip, Valence: v.Valence}
}

type stimulusData struct {
	Salience int            `yaml:"salience"`
	Lifetime int            `yaml:"lifetime"`
	Focus    map[string]int `yaml:"focus"`
}

// loadEventDefinitions fills the three per-kind tables from YAML. It is strict
// on purpose: these tables used to be Go array literals, where the compiler
// caught a misspelled kind and the array's length guaranteed every kind had a
// row. Both of those guarantees now have to be bought back here, so an unknown
// name, a missing one, an unknown tag and an unknown focus are all errors
// rather than a silently zero-valued event nobody notices for a month.
func loadEventDefinitions(data []byte) error {
	var file eventDefinitionFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return err
	}

	byName := make(map[string]LifeEventKind, numLifeEventKinds)
	for k := LifeEventKind(0); k < numLifeEventKinds; k++ {
		byName[lifeEventKindNames[k]] = k
	}
	focusByName := make(map[string]FocusKind, numFocusKinds)
	for f := FocusKind(0); f < numFocusKinds; f++ {
		focusByName[f.String()] = f
	}

	var appraisals [numLifeEventKinds]moodAppraisal
	var stimuli [numLifeEventKinds]StimulusSpec
	var collapse [numLifeEventKinds]string
	seen := make(map[LifeEventKind]bool, numLifeEventKinds)

	for name, def := range file.Events {
		kind, ok := byName[name]
		if !ok {
			return fmt.Errorf("%q is not a known event kind", name)
		}
		seen[kind] = true

		tags, err := parseEventTags(def.Tags)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if def.Impact < 0 || def.Impact > 100 {
			return fmt.Errorf("%s: impact %d outside [0, 100]", name, def.Impact)
		}
		appraisals[kind] = moodAppraisal{
			Tags:   tags,
			Impact: def.Impact,
			Fresh:  def.Fresh.vector(),
			Worn:   def.Worn.vector(),
		}
		collapse[kind] = def.CollapsesTo

		if def.Stimulus != nil {
			spec := StimulusSpec{Salience: def.Stimulus.Salience, Lifetime: def.Stimulus.Lifetime}
			for f, score := range def.Stimulus.Focus {
				focus, ok := focusByName[f]
				if !ok {
					return fmt.Errorf("%s: %q is not a known focus", name, f)
				}
				spec.Contribution[focus] = score
			}
			stimuli[kind] = spec
		}
	}

	var missing []string
	for k := LifeEventKind(0); k < numLifeEventKinds; k++ {
		if !seen[k] {
			missing = append(missing, lifeEventKindNames[k])
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("no definition for %s", strings.Join(missing, ", "))
	}

	lifeEventAppraisals = appraisals
	stimulusSpecs = stimuli
	lifeEventCollapseText = collapse
	return nil
}
