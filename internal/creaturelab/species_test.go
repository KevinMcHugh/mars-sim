package creaturelab

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The lab stores the rolled species as JSON and reads it back for every
// brief and export, so the round trip must lose nothing.
func TestSpeciesJSONRoundTrip(t *testing.T) {
	for seed := int64(1); seed <= 200; seed++ {
		sp := Roll(seed)
		data, err := json.Marshal(sp)
		if err != nil {
			t.Fatal(err)
		}
		var back sim.AlienSpecies
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatal(err)
		}
		if back != sp {
			t.Fatalf("seed %d: round trip changed the species:\n%+v\n%+v", seed, sp, back)
		}
	}
}

// One slot per form of a life, or one adult slot for a single form, and the
// slot list matches the species' own forms.
func TestFormsAreTheSpritesSlots(t *testing.T) {
	sawSingle, sawLife := false, false
	for seed := int64(1); seed <= 300; seed++ {
		sp := Roll(seed)
		forms := Forms(sp)
		life := sp.LifeForms()
		if len(life) == 0 {
			sawSingle = true
			if len(forms) != 1 || forms[0].Name != "adult" || forms[0].Label != sp.Singular || forms[0].Legs != sp.Legs() {
				t.Fatalf("seed %d: single-form slots %+v", seed, forms)
			}
			continue
		}
		sawLife = true
		if len(forms) != len(life) {
			t.Fatalf("seed %d: %d slots for %d forms", seed, len(forms), len(life))
		}
		for i, f := range forms {
			if f.Index != i || f.SizePct != life[i].SizePct || f.Inert != life[i].Inert {
				t.Fatalf("seed %d: slot %d is %+v, form is %+v", seed, i, f, life[i])
			}
			if f.Name == "" || !strings.HasPrefix(f.Label, sp.Singular) {
				t.Fatalf("seed %d: slot %d unnamed: %+v", seed, i, f)
			}
		}
	}
	if !sawSingle || !sawLife {
		t.Fatalf("300 seeds should roll both single-form species and lives (single %v, life %v)", sawSingle, sawLife)
	}
}

// A brief carries the house style, the field notes, the form's own body, and
// the species' other accepted sprites, but never the form's own old one.
func TestBrief(t *testing.T) {
	var sp sim.AlienSpecies
	for seed := int64(1); ; seed++ {
		sp = Roll(seed)
		if sp.FormCount >= 2 {
			break
		}
	}
	forms := Forms(sp)
	last := len(forms) - 1
	siblings := map[int]string{0: `<svg id="first"/>`, last: `<svg id="mine"/>`}
	brief, err := Brief(sp, last, siblings)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{HouseStyle, sp.Description(), forms[last].Name, `<svg id="first"/>`} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief lacks %q", want)
		}
	}
	if strings.Contains(brief, `<svg id="mine"/>`) {
		t.Error("brief shows the form its own accepted sprite as a sibling")
	}
	if _, err := Brief(sp, len(forms), nil); !errors.Is(err, ErrNoSuchForm) {
		t.Errorf("out-of-range form: err %v, want ErrNoSuchForm", err)
	}
}

func TestBodyLineCounts(t *testing.T) {
	sp := sim.AlienSpecies{Eyes: 1}
	got := bodyLine(sp, Form{Arms: 0, Legs: 4, Tail: true, Features: []string{"hooked claws"}})
	want := "It has 1 eye, no arms, 4 legs, a tail and no wings. It bears hooked claws."
	if got != want {
		t.Fatalf("bodyLine = %q, want %q", got, want)
	}
}
