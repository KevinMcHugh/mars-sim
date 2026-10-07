package sim

import (
	"fmt"
	"strings"
	"testing"
)

// Horns and antlers gore; a stinger, on the body or the tail, stings; other
// features grant no mode of their own.
func TestFeaturesGrantAttacks(t *testing.T) {
	for _, tc := range []struct {
		a    AlienAnatomy
		want AttackSet
	}{
		{AlienAnatomy{}, 0},
		{AlienAnatomy{Horns: 1}, AttackSetOf(AttackGore)},
		{AlienAnatomy{Antlers: 6}, AttackSetOf(AttackGore)},
		{AlienAnatomy{Stinger: 1}, AttackSetOf(AttackSting)},
		{AlienAnatomy{TailTip: TailStinger}, AttackSetOf(AttackSting)},
		{AlienAnatomy{Horns: 3, Stinger: 2}, AttackSetOf(AttackGore, AttackSting)},
		{AlienAnatomy{Shell: 3, Claws: 3, Spines: 3, TailTip: TailSpikedClub}, 0},
	} {
		if got := featureAttacks(tc.a); got != tc.want {
			t.Errorf("%+v grants %v, want %v", tc.a, got, tc.want)
		}
	}
}

// Every rolled species with horns gores and with a stinger stings, and the
// description of one that fights says so.
func TestRolledFeaturesFight(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 10
	gored, stung := 0, 0
	for seed := int64(1); seed <= 20; seed++ {
		cfg.Seed = seed
		for _, sp := range rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg) {
			want := featureAttacks(sp.Anatomy)
			if sp.AttackModes&want != want {
				t.Fatalf("%s has %+v but attacks %v", sp.Singular, sp.Anatomy, sp.AttacksLabel())
			}
			if want.Has(AttackGore) {
				gored++
				// A Friendly species' entry never says how it fights: it doesn't.
				if sp.Temperament != TemperamentFriendly && !strings.Contains(sp.Description(), "goring with their ") {
					t.Errorf("%s gores, but its description does not say so: %s", sp.Singular, sp.Description())
				}
			}
			if want.Has(AttackSting) {
				stung++
			}
		}
	}
	if gored == 0 || stung == 0 {
		t.Fatalf("200 species rolled %d gorers and %d stingers; the test covers nothing", gored, stung)
	}
}

// weaponWorld is a world whose one alien species is single-form with the
// given anatomy and attack modes, and an alien of it beside a sturdy
// colonist on revealed floor.
func weaponWorld(t *testing.T, a AlienAnatomy, modes AttackSet) (*World, *Entity, *Entity) {
	t.Helper()
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	sp := &w.alienSpecies[0]
	sp.Anatomy, sp.AttackModes, sp.BiteDamage = a, modes, 20
	w.buildAlienSpecies()
	at := Point{w.Width / 2, w.Height / 2}
	w.reveal(at)
	alien := w.spawn(Alien, at)
	victim := w.spawn(Colonist, at.Add(1, 0))
	victim.HP, victim.MaxHP = 999, 999
	victim.Parts = [numBodyParts]int{999, 999, 999, 999, 999, 999}
	victim.MaxParts = victim.Parts
	return w, alien, victim
}

// Each feature scales its mode's damage, and a featureless body's every mode
// deals exactly the base.
func TestFeatureDamage(t *testing.T) {
	for _, tc := range []struct {
		a    AlienAnatomy
		mode AttackMode
		want int // percent of base 20
	}{
		{AlienAnatomy{}, AttackBite, 100},
		{AlienAnatomy{}, AttackClaw, 100},
		{AlienAnatomy{}, AttackTail, 100},
		{AlienAnatomy{Horns: 5}, AttackGore, 140},
		{AlienAnatomy{Antlers: 10}, AttackGore, 150},
		{AlienAnatomy{Horns: 12, Antlers: 14}, AttackGore, 200},
		{AlienAnatomy{Claws: 2}, AttackClaw, 150},
		{AlienAnatomy{TailTip: TailClub}, AttackTail, 150},
		{AlienAnatomy{TailTip: TailSpikedClub}, AttackTail, 175},
		{AlienAnatomy{Stinger: 1}, AttackSting, 50},
		{AlienAnatomy{Stinger: 2}, AttackSting, 75},
		{AlienAnatomy{Horns: 6}, AttackBite, 100},
	} {
		w, alien, _ := weaponWorld(t, tc.a, AttackSetOf(tc.mode))
		if got, want := w.modeDamage(alien, tc.mode, 20), 20*tc.want/100; got != want {
			t.Errorf("%+v %v: %d, want %d", tc.a, tc.mode, got, want)
		}
	}
}

// A sting always finds the body: whatever the hit roll would have said, the
// torso takes it and nothing else does.
func TestStingFindsTheBody(t *testing.T) {
	w, alien, victim := weaponWorld(t, AlienAnatomy{Stinger: 2}, AttackSetOf(AttackSting))
	before := victim.Parts
	for i := 0; i < 5; i++ {
		w.strike(alien, victim)
	}
	for p := range victim.Parts {
		hit := victim.Parts[p] != before[p]
		if (BodyPart(p) == Torso) != hit {
			t.Fatalf("after 5 stings, %v hit=%v; parts %v -> %v", BodyPart(p), hit, before, victim.Parts)
		}
	}
}

// A young form that has not grown its horns cannot gore, though its species
// does; the adult can.
func TestYoungCannotUseFeaturesItLacks(t *testing.T) {
	w, alien, _ := weaponWorld(t, AlienAnatomy{Horns: 6}, AttackSetOf(AttackBite, AttackGore))
	sp := &w.alienSpecies[0]
	sp.Forms[0] = AlienForm{Name: "grub", Stage: 0, SizePct: 30, Ticks: 100, Limbs: 2}
	sp.Forms[1] = AlienForm{Stage: 1, SizePct: 100, Limbs: sp.Limbs, Anatomy: sp.Anatomy, Lays: true}
	sp.FormCount = 2
	w.buildAlienSpecies()
	alien.life = &LifeStage{form: 0, growAt: w.tick + 100}
	if got := fmt.Sprint(w.attacksNow(alien)); got != "[bite]" {
		t.Fatalf("hornless grub attacks %s, want [bite]", got)
	}
	alien.life.form = 1
	if got := fmt.Sprint(w.attacksNow(alien)); got != "[bite gore]" {
		t.Fatalf("horned adult attacks %s, want [bite gore]", got)
	}
}

// A shell turns aside part of every gunshot: a full carapace 30%.
func TestShellBluntsGunfire(t *testing.T) {
	for shell, want := range map[int]int{0: 20, 1: 18, 2: 16, 3: 14} {
		w, alien, colonist := weaponWorld(t, AlienAnatomy{Shell: shell}, AttackSetOf(AttackBite))
		alien.HP, alien.MaxHP = 999, 999
		alien.Parts = [numBodyParts]int{999, 999, 999, 999, 999, 999}
		alien.MaxParts = alien.Parts
		w.shoot(colonist, alien, Pistol, weaponSpec{damage: 20})
		if got := 999 - alien.HP; got != want {
			t.Errorf("shell %d: a 20-damage shot took %d, want %d", shell, got, want)
		}
	}
}
