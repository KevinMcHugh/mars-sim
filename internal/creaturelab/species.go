// Package creaturelab is Creature Lab: a catalog of alien species rolled by
// the game's own roster code, kept in Postgres with an SVG sprite for every
// form of each species' life. Species are created one at a time, on demand,
// and their sprites are drawn whenever someone gets to it (usually Claude,
// over MCP), so no world pays for art. See docs/creature-lab.md.
//
// This file is the bridge to internal/sim: rolling a species, the sprite
// slots its lifecycle implies, and the brief an artist draws a slot from. It
// holds no database code, so it can be tested without Postgres.
package creaturelab

import (
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// MaxSeed bounds the seeds NewSeed hands out. Seeds travel as JSON numbers
// to MCP clients and browsers, where anything past 2^53 silently loses
// precision; 2^31 keeps them short enough to read aloud as well. A caller
// may still pass any int64 explicitly.
const MaxSeed = 1 << 31

// NewSeed picks a fresh seed for a species nobody asked for by number.
// It uses the global generator on purpose: which seed the lab picks is not
// part of any simulation.
func NewSeed() int64 { return 1 + rand.Int64N(MaxSeed-1) }

// Roll is sim.RollLabSpecies, named here so callers read as the lab.
func Roll(seed int64) sim.AlienSpecies { return sim.RollLabSpecies(seed) }

// GeneratorRev is the commit of mars-sim this binary was built from, stored
// with every species so a row says which version of the roster code rolled
// it. Rolling the same seed on a later commit can give a different species
// (new features add draws), which is why the rolled species is stored whole
// rather than re-derived from its seed.
func GeneratorRev() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	rev, dirty := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "unknown"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}

// Traits is a species in words a person or a model can read, for the web
// pages, the API and MCP. The stored row keeps the sim.AlienSpecies itself;
// this is only its presentation.
type Traits struct {
	Singular       string   `json:"singular"`
	Plural         string   `json:"plural"`
	ScientificName string   `json:"scientificName"`
	Emoji          string   `json:"emoji,omitempty"`
	Temperament    string   `json:"temperament"`
	Apex           bool     `json:"apex,omitempty"` // one of the rare, very deadly species
	Skin           string   `json:"skin"`
	Color          string   `json:"color"`
	Pattern        string   `json:"pattern"`
	Eyes           int      `json:"eyes"`
	Arms           int      `json:"arms"`
	Legs           int      `json:"legs"`
	Tail           bool     `json:"tail"`
	Wings          bool     `json:"wings"`
	HeightCM       [2]int   `json:"heightCM"`
	WeightKG       [2]int   `json:"weightKG"`
	Attacks        []string `json:"attacks"`
	Features       []string `json:"features,omitempty"`
	Description    string   `json:"description"`
}

// TraitsOf describes sp.
func TraitsOf(sp sim.AlienSpecies) Traits {
	attacks := make([]string, 0, 3)
	for _, m := range sp.Attacks() {
		attacks = append(attacks, m.String())
	}
	return Traits{
		Singular:       sp.Singular,
		Plural:         sp.Plural,
		ScientificName: sp.ScientificName,
		Emoji:          sp.Emoji,
		Temperament:    sp.Temperament.String(),
		Apex:           sp.Apex,
		Skin:           sp.Skin.String(),
		Color:          sp.Color,
		Pattern:        sp.Pattern.String(),
		Eyes:           sp.Eyes,
		Arms:           sp.Arms,
		Legs:           sp.Legs(),
		Tail:           sp.Tail,
		Wings:          sp.Wings,
		HeightCM:       [2]int{sp.HeightMinCM, sp.HeightMaxCM},
		WeightKG:       [2]int{sp.WeightMinKG, sp.WeightMaxKG},
		Attacks:        attacks,
		Features:       sp.Anatomy.FeaturePhrases(),
		Description:    sp.Description(),
	}
}

// Form is one sprite slot: a stage or caste of the species' life, or the one
// adult form most species have. Index is the slot's number, the same as the
// form's index in sim.AlienSpecies.LifeForms() (0 for a single-form species).
type Form struct {
	Index    int      `json:"index"`
	Name     string   `json:"name"`  // "egg", "grub", "queen"; "adult" for a plain adult
	Label    string   `json:"label"` // what the game calls one: "grelk egg", "grelk"
	Stage    int      `json:"stage"`
	SizePct  int      `json:"sizePct"`
	Inert    bool     `json:"inert,omitempty"`
	Arms     int      `json:"arms"`
	Legs     int      `json:"legs"`
	Tail     bool     `json:"tail,omitempty"`
	Wings    bool     `json:"wings,omitempty"`
	Features []string `json:"features,omitempty"`
	Lays     bool     `json:"lays,omitempty"`
}

// Forms lists sp's sprite slots in stage order. A single-form species has
// one, the adult; a species with a life has one per form, castes included,
// because each draws on the map as its own creature.
func Forms(sp sim.AlienSpecies) []Form {
	life := sp.LifeForms()
	if len(life) == 0 {
		return []Form{{
			Name:     "adult",
			Label:    sp.Singular,
			SizePct:  100,
			Arms:     sp.Arms,
			Legs:     sp.Legs(),
			Tail:     sp.Tail,
			Wings:    sp.Wings,
			Features: sp.Anatomy.FeaturePhrases(),
		}}
	}
	out := make([]Form, len(life))
	for i, f := range life {
		name, label := f.Name, sp.Singular
		if name == "" {
			name = "adult"
		} else {
			label += " " + f.Name
		}
		out[i] = Form{
			Index:    i,
			Name:     name,
			Label:    label,
			Stage:    f.Stage,
			SizePct:  f.SizePct,
			Inert:    f.Inert,
			Arms:     f.Arms,
			Legs:     f.Limbs - f.Arms,
			Tail:     f.Tail,
			Wings:    f.Wings,
			Features: f.Anatomy.FeaturePhrases(),
			Lays:     f.Lays,
		}
	}
	return out
}

// HouseStyle is the drawing contract every sprite follows, the same rules the
// Scum Lab Sprite Designer gives Claude (tools/scum-lab/tools/sprites.js),
// minus its reply format: here the SVG goes back through a tool call.
const HouseStyle = `You are drawing a map sprite for mars-sim, a colony sim set in caverns under Mars. The map is a grid of square tiles; each creature is drawn in one tile.

Every SVG must follow these rules:
- Root element <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128">, square, no width or height attributes.
- Transparent background. The sprite sits on orange floor (#f78765), dark red rock (#a8402a), or near-black fog (#2e0d0b), so use a dark outline or strong value contrast to stay legible on all three.
- It is drawn as small as 16x16 pixels. Favor a bold silhouette, a few large shapes, strokes no thinner than 4 units, and at most about six colors. Fine detail turns to mush.
- Fill most of the 128 square with the subject (about 8 units of margin), centered, facing the viewer or three-quarter view, like an emoji.
- Fully self-contained: no <script>, <foreignObject>, <image>, external href, @import, web fonts, or <text>. Gradients and filters are allowed but keep them simple.
- Keep it small: a sprite is usually 20-60 lines.`

// Brief is everything an artist needs to draw one slot: the house style, the
// species' field notes (fieldNotes: generated or rewritten, as the species
// shows them), and what is particular to this form. siblings are the
// accepted sprites of the species' other forms, keyed by form index, so a
// life reads as one creature growing up; nil or empty for none.
func Brief(sp sim.AlienSpecies, fieldNotes string, form int, siblings map[int]string) (string, error) {
	forms := Forms(sp)
	if form < 0 || form >= len(forms) {
		return "", fmt.Errorf("%w: form %d (species has %d)", ErrNoSuchForm, form, len(forms))
	}
	f := forms[form]
	var b strings.Builder
	b.WriteString(HouseStyle)
	b.WriteString("\n\nThe species' field notes, as the player reads them in the game:\n\n<field_notes>\n")
	b.WriteString(fieldNotes)
	b.WriteString("\n</field_notes>\n\n")
	fmt.Fprintf(&b, "Colour: %s, %s. Covering: %s. Eyes: %d.\n\n", sp.Color, sp.Pattern, sp.Skin, sp.Eyes)
	if sp.Apex {
		b.WriteString("This is an apex species, rare and very deadly: its features are its weapons, so make them read as dangerous.\n\n")
	}
	b.WriteString(formBrief(sp, forms, f))
	if len(siblings) > 0 {
		b.WriteString("\n\nSprites already accepted for this species' other forms are below. Keep the palette, line weight and features consistent so the forms read as one creature at different points in its life.\n")
		for _, g := range forms {
			if svg, ok := siblings[g.Index]; ok && g.Index != f.Index {
				fmt.Fprintf(&b, "\n<accepted_sprite form=%q>\n%s\n</accepted_sprite>\n", g.Name, svg)
			}
		}
	}
	return b.String(), nil
}

// formBrief is the part of the brief particular to f.
func formBrief(sp sim.AlienSpecies, forms []Form, f Form) string {
	var b strings.Builder
	if len(forms) == 1 {
		b.WriteString("Draw the adult; this species has a single form.\n")
	} else {
		fmt.Fprintf(&b, "This species' life has %d forms; draw form %d, the %s (%q on the map).\n", len(forms), f.Index, f.Name, f.Label)
	}
	switch {
	case f.Inert:
		fmt.Fprintf(&b, "It is an inert %s, %s. It does not move or act. Draw the casing itself, with no limbs or face, in the species' colour and covering so it is recognizably theirs. Every form fills its tile, so show its smallness through its shape, not by drawing it small.", f.Name, sim.AlienForm{SizePct: f.SizePct}.SizeWords())
	case f.SizePct < 100:
		fmt.Fprintf(&b, "It is a young form, %s. %s Young forms graze and do not hunt: draw it as a juvenile (bigger head and eyes for its body, softer pose), not a smaller copy of the adult.", sim.AlienForm{SizePct: f.SizePct}.SizeWords(), bodyLine(sp, f))
	case f.SizePct > 100:
		fmt.Fprintf(&b, "It is a caste bigger than the plain adult (%d%% of adult size). %s Push its features further than the adult's.", f.SizePct, bodyLine(sp, f))
	case f.Name != "adult":
		fmt.Fprintf(&b, "It is an adult caste (%d%% of adult size). %s", f.SizePct, bodyLine(sp, f))
	default:
		b.WriteString(bodyLine(sp, f))
	}
	if f.Lays {
		b.WriteString(" This form lays or bears the next generation.")
	}
	b.WriteString("\n\nMatch every countable feature exactly (eyes, arms, legs, wings, tail) so a player could check the sprite against these notes. Let the temperament show in the pose and expression, and make the attack visible if a body part delivers it. Height and weight only tell you the build, stocky or lanky: every creature is drawn filling one tile.")
	return b.String()
}

// bodyLine states f's countable body: "It has 3 eyes, 2 arms, 4 legs, a tail
// and no wings, and hooked claws."
func bodyLine(sp sim.AlienSpecies, f Form) string {
	parts := []string{plural(sp.Eyes, "eye"), plural(f.Arms, "arm"), plural(f.Legs, "leg")}
	if f.Tail {
		parts = append(parts, "a tail")
	} else {
		parts = append(parts, "no tail")
	}
	if f.Wings {
		parts = append(parts, "a pair of wings")
	} else {
		parts = append(parts, "no wings")
	}
	out := "It has " + strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1] + "."
	if len(f.Features) > 0 {
		out += " It bears " + strings.Join(f.Features, ", ") + "."
	}
	return out
}

func plural(n int, word string) string {
	if n == 0 {
		return "no " + word + "s"
	}
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
