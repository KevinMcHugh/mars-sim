package sim

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// testPack packs the lab species for seeds, each form with a sprite naming
// its species and form, so a test can tell which sprite an alien drew.
func testPack(t *testing.T, seeds ...int64) []PackedSpecies {
	t.Helper()
	var pack []PackedSpecies
	for _, seed := range seeds {
		sp := RollLabSpecies(seed)
		ps := PackedSpecies{ID: fmt.Sprint("sp", seed), Seed: seed, Species: sp}
		for f := 0; f < sp.slots(); f++ {
			ps.Sprites = append(ps.Sprites, PackedSprite{Form: f, SVG: spriteSVG(sp, f)})
		}
		pack = append(pack, ps)
	}
	return pack
}

func spriteSVG(sp AlienSpecies, form int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" id="%s-%d"/>`, sp.Singular, form)
}

// seedsWithLife finds n seeds whose lab species have distinct names, the
// first with a life of several forms and the rest single-form.
func seedsWithLife(t *testing.T, n int) []int64 {
	t.Helper()
	var seeds []int64
	names := map[string]bool{}
	wantLife := true
	for seed := int64(1); seed < 2000 && len(seeds) < n; seed++ {
		sp := RollLabSpecies(seed)
		if names[sp.Singular] || (sp.FormCount >= 2) != wantLife {
			continue
		}
		names[sp.Singular] = true
		seeds = append(seeds, seed)
		wantLife = false
	}
	if len(seeds) < n {
		t.Fatalf("found only %d suitable seeds", len(seeds))
	}
	return seeds
}

func TestLoadSpeciesPack(t *testing.T) {
	good := SpeciesPack{Species: testPack(t, seedsWithLife(t, 2)...)}
	data, _ := json.Marshal(good)
	pack, err := LoadSpeciesPack(data, "good")
	if err != nil {
		t.Fatal(err)
	}
	if len(pack) != 2 || pack[0].Species != good.Species[0].Species {
		t.Fatal("a pack does not load back as it was written")
	}

	bad := map[string]func(p *SpeciesPack){
		"no species":       func(p *SpeciesPack) { p.Species = nil },
		"no name":          func(p *SpeciesPack) { p.Species[0].Species.Singular = "" },
		"too many forms":   func(p *SpeciesPack) { p.Species[0].Species.FormCount = maxAlienForms + 1 },
		"form out of life": func(p *SpeciesPack) { p.Species[1].Sprites[0].Form = 5 },
		"two for a form":   func(p *SpeciesPack) { p.Species[1].Sprites = append(p.Species[1].Sprites, p.Species[1].Sprites[0]) },
		"not an svg":       func(p *SpeciesPack) { p.Species[1].Sprites[0].SVG = "hello" },
		"huge notes":       func(p *SpeciesPack) { p.Species[0].Notes = strings.Repeat("x", MaxPackedLoreBytes+1) },
		"huge sprite": func(p *SpeciesPack) {
			p.Species[1].Sprites[0].SVG = "<svg>" + strings.Repeat(" ", MaxPackedSpriteBytes) + "</svg>"
		},
	}
	for name, spoil := range bad {
		p := SpeciesPack{Species: testPack(t, seedsWithLife(t, 2)...)}
		spoil(&p)
		data, _ := json.Marshal(p)
		if _, err := LoadSpeciesPack(data, name); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	if _, err := LoadSpeciesPack([]byte("{"), "broken"); err == nil {
		t.Error("broken JSON loaded")
	}
}

// A world with a pack draws its roster from it: count species, distinct,
// as packed, with combat worked out from this world's settings, the same
// draw for the same seed.
func TestPackedRoster(t *testing.T) {
	seeds := seedsWithLife(t, 4)
	pack := testPack(t, seeds...)
	inPack := map[string]AlienSpecies{}
	for _, ps := range pack {
		inPack[ps.Species.Singular] = ps.Species
	}

	cfg := testConfig()
	cfg.SpeciesPack = pack
	cfg.AlienSpeciesCount = 3
	cfg.AlienDamage *= 3
	w := newTestWorld(t, cfg)
	if len(w.alienSpecies) != 3 {
		t.Fatalf("roster of %d, want 3", len(w.alienSpecies))
	}
	for _, sp := range w.alienSpecies {
		packed, ok := inPack[sp.Singular]
		if !ok {
			t.Fatalf("%s is not in the pack", sp.Singular)
		}
		if sp.BiteDamage != speciesDamage(packed, cfg) || sp.Slowness != scaledByTemperament(cfg.AlienSlowness, sp.Temperament) {
			t.Fatalf("%s's combat was not worked out from this world's settings", sp.Singular)
		}
		packed.BiteDamage, packed.BiteRest, packed.Slowness = sp.BiteDamage, sp.BiteRest, sp.Slowness
		if sp != packed {
			t.Fatalf("%s changed on the way in", sp.Singular)
		}
	}
	again := newTestWorld(t, cfg)
	for i := range w.alienSpecies {
		if again.alienSpecies[i] != w.alienSpecies[i] {
			t.Fatal("the same seed and pack drew a different roster")
		}
	}

	cfg.AlienSpeciesCount = 10
	if w := newTestWorld(t, cfg); len(w.alienSpecies) != len(pack) {
		t.Fatalf("asking for more than the pack holds gave %d species, want the whole pack (%d)", len(w.alienSpecies), len(pack))
	}

	// Two packed species with one name: a world takes only one of them.
	dup := append(testPack(t, seeds[0]), testPack(t, seeds[0])...)
	dup[1].ID = "copy"
	cfg.SpeciesPack = dup
	if w := newTestWorld(t, cfg); len(w.alienSpecies) != 1 {
		t.Fatalf("a duplicate name made it into the roster: %d species", len(w.alienSpecies))
	}
}

// Each alien of a packed species carries its form's sprite to the snapshot;
// a world without a pack carries none.
func TestPackedAliensDrawTheirSprites(t *testing.T) {
	seeds := seedsWithLife(t, 1) // one species, with a life
	cfg := testConfig()
	cfg.SpeciesPack = testPack(t, seeds...)
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	sp := w.alienSpecies[0]
	at := Point{w.Width / 2, w.Height / 2, LandingLevel}
	carve(w, at.Add(-2, -2), at.Add(2, 2), Floor)
	w.refreshSpatial()

	var aliens []*Entity
	for f := 0; f < sp.FormCount; f++ {
		e := w.spawnAs(Alien, at.Add(f%2, f/2), 0)
		e.life.form = f
		aliens = append(aliens, e)
	}
	snap := w.snapshot(false, 8)
	if len(snap.Sprites) != sp.FormCount {
		t.Fatalf("%d sprites in the snapshot, want %d", len(snap.Sprites), sp.FormCount)
	}
	for f, e := range aliens {
		ev := w.entityView(e, nil, false)
		if ev.Sprite == 0 || snap.Sprites[ev.Sprite-1] != spriteSVG(sp, f) {
			t.Fatalf("form %d drew sprite %d", f, ev.Sprite)
		}
	}

	plain := newTestWorld(t, testConfig())
	if len(plain.snapshot(false, 8).Sprites) != 0 {
		t.Fatal("a rolled roster has sprites")
	}
	for _, e := range plain.entities {
		if e.Kind == Alien && plain.spriteFor(e) != 0 {
			t.Fatal("an alien of a rolled species has a sprite")
		}
	}
}

// A save carries the pack and the sprite table, so a loaded game keeps its
// creatures and their art without the file.
func TestPackedWorldSurvivesASave(t *testing.T) {
	cfg := testConfig()
	cfg.SpeciesPack = testPack(t, seedsWithLife(t, 2)...)
	cfg.AlienSpeciesCount = 2
	w := newTestWorld(t, cfg)
	loaded := saveAndLoad(t, w)
	if len(loaded.cfg.SpeciesPack) != 2 || len(loaded.sprites.SVGs) != len(w.sprites.SVGs) || loaded.sprites.SVGs[0] != w.sprites.SVGs[0] {
		t.Fatal("the pack or its sprites did not survive a save")
	}
	for i := range w.sprites.Of {
		if loaded.sprites.Of[i] != w.sprites.Of[i] {
			t.Fatal("the sprite table changed in a save")
		}
	}
}

// A packed species' lore tab shows the lab's rewrite of its field notes, or
// the generated ones when there is none, and the lab's notes.
func TestPackedLore(t *testing.T) {
	seeds := seedsWithLife(t, 2)
	pack := testPack(t, seeds...)
	pack[0].Description = "  Rewritten in the lab.  "
	pack[0].Notes = "Count the arms.\nThat tells its age."
	cfg := testConfig()
	cfg.SpeciesPack = pack
	cfg.AlienSpeciesCount = 2
	w := newTestWorld(t, cfg)
	snap := w.snapshot(false, 8)
	for i, sp := range snap.AlienSpecies {
		if sp.Singular == pack[0].Species.Singular {
			if snap.FieldNotes(i) != "Rewritten in the lab." || snap.LabNotes(i) != "Count the arms.\nThat tells its age." {
				t.Fatalf("rewritten species shows %q / %q", snap.FieldNotes(i), snap.LabNotes(i))
			}
		} else if snap.FieldNotes(i) != sp.Description() || snap.LabNotes(i) != "" {
			t.Fatalf("an unedited species shows %q / %q", snap.FieldNotes(i), snap.LabNotes(i))
		}
	}
	if loaded := saveAndLoad(t, w); len(loaded.speciesLore) != 2 {
		t.Fatal("the lab text did not survive a save")
	}

	plain := newTestWorld(t, testConfig()).snapshot(false, 8)
	if plain.FieldNotes(0) != plain.AlienSpecies[0].Description() || plain.LabNotes(0) != "" || plain.FieldNotes(99) != "" {
		t.Fatal("a rolled species' lore is not its generated description")
	}
}
