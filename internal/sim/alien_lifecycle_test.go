package sim

import (
	"strings"
	"testing"
)

// lifecycleConfig is testConfig with every alien species living as stages
// stages, its adults split into castes when castes is set.
func lifecycleConfig(stages int, castes bool) Config {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := []int{0, 0, 0, 0}
	w[stages-1] = 1
	cfg.AlienOneFormWeight, cfg.AlienTwoFormWeight, cfg.AlienThreeFormWeight, cfg.AlienFourFormWeight = w[0], w[1], w[2], w[3]
	cfg.AlienCastePercent = 0
	if castes {
		cfg.AlienCastePercent = 100
	}
	return cfg
}

// Every life grows: each stage is larger than the one before, the adult is
// 100%, a mobile form keeps every feature and limb the mobile form before it
// had (it only gains or pushes further), and castes all have odds.
func TestLifecyclesGrowAndOnlyAdd(t *testing.T) {
	for _, castes := range []bool{false, true} {
		for stages := 2; stages <= 4; stages++ {
			cfg := lifecycleConfig(stages, castes)
			cfg.AlienSpeciesCount = 8
			for seed := int64(1); seed <= 20; seed++ {
				cfg.Seed = seed
				for _, sp := range rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg) {
					checkLifecycle(t, sp, castes)
				}
			}
		}
	}
}

func checkLifecycle(t *testing.T, sp AlienSpecies, castes bool) {
	t.Helper()
	forms := sp.LifeForms()
	if len(forms) == 0 {
		t.Fatalf("%s: a forced lifecycle rolled a single form", sp.Singular)
	}
	last := sp.stageCount() - 1
	var prev *AlienForm
	for i := range forms {
		f := forms[i]
		if f.Stage == last {
			if castes && (f.Name == "" || f.Weight <= 0) {
				t.Errorf("%s: adult caste %+v has no name or odds", sp.Singular, f)
			}
			if !castes && (f.Name != "" || f.SizePct != 100) {
				t.Errorf("%s: plain adult %+v should be unnamed at 100%%", sp.Singular, f)
			}
			if f.Inert || f.Ticks != 0 {
				t.Errorf("%s: adult form %+v is inert or has a stage length", sp.Singular, f)
			}
			continue
		}
		if f.Stage != i || f.Ticks <= 0 || f.Name == "" {
			t.Errorf("%s: young form %d is %+v", sp.Singular, i, f)
		}
		if i > 0 && f.SizePct <= forms[i-1].SizePct {
			t.Errorf("%s: stage %d (%d%%) is no bigger than stage %d (%d%%)", sp.Singular, i, f.SizePct, i-1, forms[i-1].SizePct)
		}
		if f.Inert {
			continue
		}
		if prev != nil && !gainsOnly(*prev, f) {
			t.Errorf("%s: %s %+v loses something the %s %+v had", sp.Singular, f.Name, f, prev.Name, *prev)
		}
		prev = &forms[i]
	}
	// The plain adult (or the largest caste's body plan) keeps everything
	// the last mobile young form had.
	adult := AlienForm{Limbs: sp.Limbs, Arms: sp.Arms, Tail: sp.Tail, Anatomy: sp.Anatomy}
	if prev != nil && !gainsOnly(*prev, adult) {
		t.Errorf("%s: the adult loses something its %s had", sp.Singular, prev.Name)
	}
}

// gainsOnly reports whether next has at least everything prev had.
func gainsOnly(prev, next AlienForm) bool {
	a, b := prev.Anatomy, next.Anatomy
	return next.Limbs >= prev.Limbs && next.Arms >= prev.Arms && (next.Tail || !prev.Tail) &&
		b.Horns >= a.Horns && b.Antlers >= a.Antlers && b.Spines >= a.Spines && b.Shell >= a.Shell &&
		b.Claws >= a.Claws && b.Stinger >= a.Stinger && (b.TailTip == a.TailTip || a.TailTip == TailPlain)
}

// At the shipped odds most species have a single form: a lifecycle is a
// discovery, not the norm.
func TestMostSpeciesHaveASingleForm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 10
	single, total := 0, 0
	for seed := int64(1); seed <= 40; seed++ {
		cfg.Seed = seed
		for _, sp := range rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg) {
			total++
			if sp.FormCount == 0 {
				single++
			}
		}
	}
	if single*2 <= total {
		t.Fatalf("%d of %d species have a single form; want most", single, total)
	}
	if single == total {
		t.Fatal("no species rolled a lifecycle at the shipped odds")
	}
}

// Every form spawns alive, however small: a tiny egg whose head rounded to
// nothing would count as dead and never get a turn to hatch.
func TestEveryFormSpawnsAlive(t *testing.T) {
	cfg := lifecycleConfig(4, true)
	for seed := int64(1); seed <= 10; seed++ {
		cfg.Seed = seed
		w := newTestWorld(t, cfg)
		for form := range w.alienSpecies[0].LifeForms() {
			e := w.spawn(Alien, Point{5, 5, LandingLevel})
			e.life.form = form
			w.resizeAlien(e, 100)
			if !e.Alive() || e.MaxHP < minAlienFormHP {
				t.Fatalf("seed %d form %d: spawned dead (HP %d, parts %v)", seed, form, e.HP, e.Parts)
			}
			w.remove(e.ID, "test")
		}
	}
}

// An alien grows into its next stage when its time comes: it takes the next
// form, grows (its hit points rise), and the log says what happened.
func TestAlienGrowsIntoItsNextStage(t *testing.T) {
	cfg := lifecycleConfig(3, false)
	w := newTestWorld(t, cfg)
	sp := w.alienSpecies[0]
	at := Point{w.Width / 2, w.Height / 2, LandingLevel} // the landing cave, which the colony has seen
	w.reveal(at)
	e := w.spawn(Alien, at)
	if w.dormant(e) {
		t.Fatal("test alien spawned dormant; the log would stay quiet")
	}
	e.life.form, e.life.growAt = 0, w.tick+1
	w.resizeAlien(e, 100)
	before, hp := w.alienNounFor(e), e.MaxHP
	for i := 0; i < 2; i++ {
		w.step()
	}
	if e.life.form != 1 {
		t.Fatalf("still form %d after its time came", e.life.form)
	}
	if e.MaxHP <= hp {
		t.Fatalf("max HP %d after growing, was %d", e.MaxHP, hp)
	}
	if want := sp.Forms[1].Ticks; e.life.growAt <= w.tick || e.life.growAt > w.tick+want {
		t.Fatalf("next growth at %d, want within %d ticks of %d", e.life.growAt, want, w.tick)
	}
	verb := leaveVerb(sp.Forms[0])
	found := false
	for _, l := range w.log.tail(50) {
		if l.Kind == LogBirth && strings.Contains(l.Text, capitalizeFirst(before)+" "+verb+" into ") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no log line for %s that %s; log: %v", before, verb, w.log.tail(5))
	}
}

// An egg or cocoon is no threat: nobody flees from or fights one, and it
// never moves.
func TestInertFormsAreNoThreat(t *testing.T) {
	cfg := lifecycleConfig(3, false)
	for seed := int64(1); seed <= 30; seed++ {
		cfg.Seed = seed
		w := newTestWorld(t, cfg)
		inertForm := -1
		for i, f := range w.alienSpecies[0].LifeForms() {
			if f.Inert {
				inertForm = i
			}
		}
		if inertForm < 0 {
			continue
		}
		at := Point{w.Width / 2, w.Height / 2, LandingLevel}
		w.reveal(at)
		e := w.spawn(Alien, at)
		e.life.form, e.life.growAt = inertForm, w.tick+10000
		if w.dormant(e) {
			t.Fatal("test egg spawned dormant, which would hide it anyway")
		}
		if _, ok := w.nearestAlien(at.Add(1, 0), 5); ok {
			t.Fatal("an egg or cocoon counts as a threat")
		}
		pos := e.Pos
		for i := 0; i < 20; i++ {
			w.animalTurn(e)
		}
		if e.Pos != pos {
			t.Fatalf("an inert %s moved from %v to %v", w.alienFormNoun(e), pos, e.Pos)
		}
		return
	}
	t.Fatal("no seed rolled an inert form to test")
}

// The young do not hunt: a Hostile species' mobile young grazes, and never
// bites an adjacent colonist.
func TestYoungFormsDoNotHunt(t *testing.T) {
	cfg := lifecycleConfig(2, false)
	for seed := int64(1); seed <= 30; seed++ {
		cfg.Seed = seed
		w := newTestWorld(t, cfg)
		if f := w.alienSpecies[0].Forms[0]; f.Inert {
			continue
		}
		setAlienTemperament(w, 0, TemperamentHostile)
		at := Point{w.Width / 2, w.Height / 2, LandingLevel}
		w.reveal(at)
		e := w.spawn(Alien, at)
		e.life.form, e.life.growAt = 0, w.tick+10000
		if w.dormant(e) {
			t.Fatal("test alien spawned dormant, which would keep it from hunting anyway")
		}
		victim := w.spawn(Colonist, at.Add(1, 0))
		hp := victim.HP
		for i := 0; i < 30; i++ {
			w.animalTurn(e)
			if e.State == Hunting {
				t.Fatalf("a hostile %s is hunting", w.alienFormNoun(e))
			}
		}
		if victim.HP != hp {
			t.Fatalf("a young %s bit a colonist", w.alienFormNoun(e))
		}
		return
	}
	t.Fatal("no seed rolled a mobile young form to test")
}

// A strike scales with the striker's size: a young form deals less than the
// adult, a queen more, and a species that deals damage always deals some.
func TestStrikeDamageScalesWithForm(t *testing.T) {
	cfg := lifecycleConfig(2, true)
	w := newTestWorld(t, cfg)
	w.alienSpecies[0].BiteDamage = 20
	e := w.spawn(Alien, Point{5, 5, LandingLevel})
	for i, f := range w.alienSpecies[0].LifeForms() {
		e.life.form = i
		if got, want := w.alienDamage(e), max(1, scaleRound(20, f.SizePct, 100)); got != want {
			t.Errorf("%s (%d%%) deals %d, want %d", w.alienFormNoun(e), f.SizePct, got, want)
		}
	}
}

// The lore tab describes a species' life, and a single-form species'
// description is exactly what it was before lifecycles.
func TestDescriptionTellsTheLife(t *testing.T) {
	cfg := lifecycleConfig(3, true)
	cfg.Seed = 5
	sp := rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg)[0]
	d := sp.Description()
	if !strings.Contains(d, "Their life has 3 stages: ") || !strings.Contains(d, "Adults come as ") {
		t.Fatalf("description does not tell the life: %s", d)
	}
	plain := sp
	plain.Anatomy, plain.Forms, plain.FormCount = AlienAnatomy{}, [maxAlienForms]AlienForm{}, 0
	if plain.Description() != plain.entry() {
		t.Fatalf("a featureless single-form species' description changed: %s", plain.Description())
	}
}

// The life of a species is part of the seed: the same seed rolls the same
// forms.
func TestLifecyclesAreDeterministic(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 12
	cfg.Seed = 77
	a := rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg)
	b := rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("species %d rolled differently from the same seed", i)
		}
	}
}
