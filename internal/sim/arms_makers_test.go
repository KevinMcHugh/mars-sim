package sim

import (
	"reflect"
	"strings"
	"testing"
)

func rollArms(seed int64, cfg Config) ([]Corporation, []GunModel) {
	rng := newRand(seed ^ armsLoreSeed)
	corps := rollCorporationRoster(rng, cfg)
	return corps, rollGunModels(rng, corps)
}

func TestArmsLoreIsDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	c1, g1 := rollArms(42, cfg)
	c2, g2 := rollArms(42, cfg)
	if !reflect.DeepEqual(c1, c2) || !reflect.DeepEqual(g1, g2) {
		t.Fatalf("same seed rolled different arms lore:\n%v %v\n%v %v", c1, g1, c2, g2)
	}
}

func TestArmsLoreInvariants(t *testing.T) {
	cfg := DefaultConfig()
	for seed := int64(0); seed < 200; seed++ {
		corps, guns := rollArms(seed, cfg)
		if len(corps) != cfg.CorporationCount {
			t.Fatalf("seed %d: %d corporations, want %d", seed, len(corps), cfg.CorporationCount)
		}
		names := map[string]bool{}
		for _, c := range corps {
			if names[c.Name] {
				t.Fatalf("seed %d: corporation name %q repeated", seed, c.Name)
			}
			names[c.Name] = true
			if c.Code == "" {
				t.Fatalf("seed %d: %q has no model code", seed, c.Name)
			}
		}
		if len(guns) != len(weaponKinds) {
			t.Fatalf("seed %d: %d gun models, want %d", seed, len(guns), len(weaponKinds))
		}
		models := map[string]bool{}
		for i, g := range guns {
			if g.Kind != weaponKinds[i] {
				t.Fatalf("seed %d: model %d is for %v, want %v", seed, i, g.Kind, weaponKinds[i])
			}
			if g.Brand != corps[g.Maker].Name || g.Model == "" {
				t.Fatalf("seed %d: bad model %+v", seed, g)
			}
			if models[g.Name()] {
				t.Fatalf("seed %d: model %q repeated", seed, g.Name())
			}
			models[g.Name()] = true
		}
	}
}

func TestCorporationCountClamps(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CorporationCount = 0
	if corps, _ := rollArms(1, cfg); len(corps) != 1 {
		t.Fatalf("count 0 rolled %d corporations, want 1", len(corps))
	}
	cfg.CorporationCount = 1000
	if corps, _ := rollArms(1, cfg); len(corps) != len(corporationRoots) {
		t.Fatalf("count 1000 rolled %d corporations, want %d", len(corps), len(corporationRoots))
	}
}

// TestWeaponPhraseNamesTheModel checks narration reaches for the rolled make
// and model, with the right article.
func TestWeaponPhraseNamesTheModel(t *testing.T) {
	w := newWorld(DefaultConfig(), newPCG(1))
	g, ok := w.gunModelFor(Shotgun)
	if !ok {
		t.Fatal("no shotgun model rolled")
	}
	got := w.weaponPhrase(Shotgun)
	if !strings.HasSuffix(got, g.Name()+" shotgun") {
		t.Fatalf("weaponPhrase(Shotgun) = %q, want it to name %q", got, g.Name())
	}
	w.gunModels = []GunModel{{Kind: Pistol, Brand: "Ares Arms", Model: "AA-9"}}
	if got := w.weaponPhrase(Pistol); got != "an Ares Arms AA-9 pistol" {
		t.Fatalf("weaponPhrase(Pistol) = %q", got)
	}
	if got := w.weaponPhrase(AssaultRifle); got != "an assault rifle" {
		t.Fatalf("weaponPhrase without a model = %q", got)
	}
}

func TestRoman(t *testing.T) {
	for n, want := range map[int]string{1: "I", 4: "IV", 9: "IX", 12: "XII", 39: "XXXIX"} {
		if got := roman(n); got != want {
			t.Errorf("roman(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestArmsLoreSample(t *testing.T) {
	if !testing.Verbose() {
		t.Skip()
	}
	for seed := int64(1); seed <= 4; seed++ {
		corps, guns := rollArms(seed, DefaultConfig())
		for i, c := range corps {
			t.Log(c.Description(i, guns))
		}
	}
}
