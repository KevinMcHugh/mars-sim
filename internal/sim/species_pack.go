package sim

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
)

// A species pack is a set of alien species pulled from Creature Lab
// (docs/creature-lab.md), each frozen as the lab stored it, with an SVG
// sprite for every form of its life. When Config.SpeciesPack holds any, a
// world picks its roster from them instead of rolling one, and every alien of
// a packed species draws with its form's sprite in the browser. See
// docs/species-pack.md.

// SpeciesPackFileName is the pack the game reads by default, beside the
// binary, the way it reads alien-names.yaml. `mars-sim -fetch-species URL`
// writes it.
const SpeciesPackFileName = "species-pack.json"

// MaxPackedSpriteBytes bounds one sprite in a pack, as Creature Lab bounds a
// candidate.
const MaxPackedSpriteBytes = 64 << 10

// SpeciesPack is the file: the same shape as Creature Lab's /api/export, so
// an export saved to disk is a pack too.
type SpeciesPack struct {
	// Source is the lab it was pulled from, for the record.
	Source string `json:"source,omitempty"`
	// ExportedAt is when it was pulled (the export's own name for it).
	ExportedAt string          `json:"exportedAt,omitempty"`
	Species    []PackedSpecies `json:"species"`
}

// PackedSpecies is one species in a pack.
type PackedSpecies struct {
	ID           string `json:"id"`
	Seed         int64  `json:"seed"`
	GeneratorRev string `json:"generatorRev,omitempty"`
	// Species is the species as rolled, with every field the game reads.
	Species AlienSpecies   `json:"species"`
	Sprites []PackedSprite `json:"sprites"`
}

// PackedSprite is the accepted sprite for one form: Form indexes the
// species' LifeForms(), or is 0 for a single-form species.
type PackedSprite struct {
	Form int    `json:"form"`
	Name string `json:"name,omitempty"`
	SVG  string `json:"svg"`
}

// slots is how many sprites a species draws with: one per form of its life,
// or one for a single form.
func (sp AlienSpecies) slots() int { return max(1, sp.FormCount) }

// LoadSpeciesPack parses and checks a pack file. name labels errors. A pack
// with no species is an error: an empty file is a mistake, not a choice
// (pass -species-pack "" to play without one).
func LoadSpeciesPack(data []byte, name string) ([]PackedSpecies, error) {
	var pack SpeciesPack
	if err := json.Unmarshal(data, &pack); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(pack.Species) == 0 {
		return nil, fmt.Errorf("%s: no species", name)
	}
	for i, ps := range pack.Species {
		sp := ps.Species
		where := fmt.Sprintf("%s: species %d (%q)", name, i, sp.Singular)
		switch {
		case sp.Singular == "" || sp.Plural == "":
			return nil, fmt.Errorf("%s: no name", where)
		case sp.FormCount < 0 || sp.FormCount > maxAlienForms:
			return nil, fmt.Errorf("%s: %d forms (the most is %d)", where, sp.FormCount, maxAlienForms)
		case sp.Limbs < sp.Arms || sp.Eyes < 0 || sp.HeightMaxCM <= 0 || sp.WeightMaxKG <= 0:
			return nil, fmt.Errorf("%s: an impossible body", where)
		}
		seen := make([]bool, sp.slots())
		for _, s := range ps.Sprites {
			switch {
			case s.Form < 0 || s.Form >= len(seen):
				return nil, fmt.Errorf("%s: a sprite for form %d, which it does not have", where, s.Form)
			case seen[s.Form]:
				return nil, fmt.Errorf("%s: two sprites for form %d", where, s.Form)
			case !strings.HasPrefix(strings.TrimSpace(s.SVG), "<"):
				return nil, fmt.Errorf("%s: form %d's sprite is not an SVG", where, s.Form)
			case len(s.SVG) > MaxPackedSpriteBytes:
				return nil, fmt.Errorf("%s: form %d's sprite is %d bytes (the limit is %d)", where, s.Form, len(s.SVG), MaxPackedSpriteBytes)
			}
			seen[s.Form] = true
		}
	}
	return pack.Species, nil
}

// packedRoster is the roster for a world with a species pack: count species
// drawn from it on rng (the lore stream), no two sharing a name, in pack
// order of drawing. Asking for more species than the pack holds gives every
// distinct one: a pack replaces rolling rather than mixing with it, so a
// world's species are all ones somebody looked at. Each one's combat pace and
// damage are worked out again from this world's settings, since the lab
// rolled them under the defaults. picked is each roster entry's index in pack.
func packedRoster(rng *rand.Rand, cfg Config, pack []PackedSpecies, count int) (roster []AlienSpecies, picked []int) {
	used := map[string]bool{}
	sci := map[string]bool{}
	for _, i := range rng.Perm(len(pack)) {
		if len(roster) == count {
			break
		}
		sp := pack[i].Species
		name, binomial := strings.ToLower(sp.Singular), strings.ToLower(sp.ScientificName)
		if used[name] || (binomial != "" && sci[binomial]) {
			continue // two lab species can share a name; a world cannot
		}
		used[name], sci[binomial] = true, true
		sp.BiteDamage = speciesDamage(sp, cfg)
		sp.BiteRest = scaledByTemperament(cfg.AlienBiteRest, sp.Temperament)
		sp.Slowness = scaledByTemperament(cfg.AlienSlowness, sp.Temperament)
		roster = append(roster, sp)
		picked = append(picked, i)
	}
	return roster, picked
}

// alienSprites is a world's sprites: every SVG its packed species draw with,
// and for each roster entry, each form's place in that list plus one (0 for a
// form with no sprite, which draws with the species' emoji). Rolled species
// have none. Immutable once built, so a Snapshot shares it.
type alienSprites struct {
	SVGs []string
	Of   [][maxAlienForms]int
}

// buildAlienSprites lays out the sprites of a packed roster.
func buildAlienSprites(pack []PackedSpecies, picked []int) alienSprites {
	var s alienSprites
	s.Of = make([][maxAlienForms]int, len(picked))
	for r, i := range picked {
		for _, sprite := range pack[i].Sprites {
			s.SVGs = append(s.SVGs, sprite.SVG)
			s.Of[r][sprite.Form] = len(s.SVGs)
		}
	}
	return s
}

// spriteFor is the sprite an alien draws with, as an index into the world's
// sprite SVGs plus one, or 0 for none.
func (w *World) spriteFor(e *Entity) int {
	if e.Kind != Alien || e.Species < 0 || e.Species >= len(w.sprites.Of) {
		return 0
	}
	form := 0 // a single-form species' one sprite
	if e.life != nil {
		form = e.life.form
	}
	if form < 0 || form >= maxAlienForms {
		return 0
	}
	return w.sprites.Of[e.Species][form]
}
