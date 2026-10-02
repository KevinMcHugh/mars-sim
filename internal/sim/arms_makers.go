package sim

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// ---- Lore: arms makers and the guns they sell -------------------------------
//
// The second piece of lore after alien species (see lore.go): every world
// rolls a handful of corporations back home, and every gun the colony can hold
// -- the pistols and shotguns that come down in crash pods and supply drops,
// and the assault rifle machined at the gun bench to a licensed design -- gets
// a maker from that roster and a model name ("MarsCorp M-117"). It is pure
// flavor today: a model never changes a gun's stats, which stay the Config
// weapon tunables. See docs/arms-makers.md.
//
// It rolls on its own seed-derived stream, like species do, so adding or
// retuning it never shifts the species roster, worldgen, or the simulation
// stream (and so no golden hash).

// armsLoreSeed is the arms-maker lore stream's XOR key against cfg.Seed,
// distinct from every other stream's key.
const armsLoreSeed = 0x1F83D9ABFB41BD6B

// weaponKinds is every gun the colony can hold, in the fixed order its models
// are rolled (and listed) in.
var weaponKinds = [...]ItemKind{Pistol, Shotgun, AssaultRifle}

// Corporation is one company the world's lore rolls: a name, the letters its
// model names lead with, and a little history for the lore tab.
type Corporation struct {
	Name    string // "MarsCorp", "Halvorsen Dynamics"
	Code    string // what model names lead with: "M", "HD"
	HQ      string // "Phobos", "Lagos"
	hqPrep  string // "on", "in"
	Founded int    // year
	scheme  modelScheme
}

// GunModel is the make and model the colony's guns of one kind carry.
type GunModel struct {
	Kind  ItemKind
	Maker int    // index into the world's corporations
	Brand string // the maker's name, copied so a view needs no lookup
	Model string // "M-117"
}

// Name is the make and model, "MarsCorp M-117".
func (g GunModel) Name() string { return g.Brand + " " + g.Model }

// modelScheme is how a corporation numbers its products. Each maker keeps one
// house style, so two guns from the same company read as siblings.
type modelScheme uint8

const (
	schemeCodeNumber       modelScheme = iota // "M-117"
	schemeModel                               // "Model 12"
	schemeMark                                // "Mk IV"
	schemeCodeNumberSuffix                    // "HD-45C"
	numModelSchemes
)

// corporationRoots and corporationSuffixes build a company name: a root is
// drawn without replacement so no two corporations share one, and a glued
// suffix joins without a space ("MarsCorp", not "Mars Corp").
var corporationRoots = [...]string{
	"Mars", "Ares", "Tharsis", "Olympus", "Phobos", "Deimos", "Hellas", "Valles",
	"Borealis", "Elysium", "Kestrel", "Halvorsen", "Ostrander", "Vulcan", "Orion",
	"Tycho", "Ferrum", "Redline", "Okafor", "Nakamura", "Brandt", "Sokolov",
	"Castellan", "Meridian",
}

var corporationSuffixes = [...]struct {
	word  string
	glued bool
}{
	{"Corp", true}, {"Tech", true}, {"Arms", false}, {"Dynamics", false},
	{"Industries", false}, {"Armory", false}, {"Ballistics", false},
	{"Heavy Industries", false}, {"Defense Systems", false}, {"Ordnance", false},
	{"Consolidated", false}, {"& Sons", false}, {"Works", false}, {"Armaments", false},
}

// corporationHQs is where a corporation is headquartered, each with the
// preposition the lore blurb puts in front of it ("in Lagos", "on Phobos").
var corporationHQs = [...]struct{ prep, place string }{
	{"in", "Houston"}, {"in", "Shenzhen"}, {"in", "Lagos"}, {"in", "Stuttgart"},
	{"in", "São Paulo"}, {"in", "Bangalore"}, {"on", "Luna"}, {"on", "Phobos"},
	{"on", "Ceres"}, {"on", "Tharsis Montes"}, {"in", "the L5 yards"},
}

// rollCorporationRoster rolls Config.CorporationCount corporations (at least
// one, so every gun has someone to make it).
func rollCorporationRoster(rng *rand.Rand, cfg Config) []Corporation {
	count := max(cfg.CorporationCount, 1)
	count = min(count, len(corporationRoots))
	roots := append([]string(nil), corporationRoots[:]...)
	roster := make([]Corporation, count)
	for i := range roster {
		r := rng.IntN(len(roots))
		root := roots[r]
		roots = append(roots[:r], roots[r+1:]...) // no two companies share a root
		suf := corporationSuffixes[rng.IntN(len(corporationSuffixes))]
		hq := corporationHQs[rng.IntN(len(corporationHQs))]
		c := Corporation{
			HQ:      hq.place,
			hqPrep:  hq.prep,
			Founded: 2031 + rng.IntN(110),
			scheme:  modelScheme(rng.IntN(int(numModelSchemes))),
		}
		if suf.glued {
			c.Name = root + suf.word
			c.Code = root[:1]
		} else {
			c.Name = root + " " + suf.word
			c.Code = initials(c.Name)
		}
		roster[i] = c
	}
	return roster
}

// initials is the first letter of each word that starts with one: "Ares Heavy
// Industries" is "AHI", and "Brandt & Sons" is "BS".
func initials(name string) string {
	var b strings.Builder
	for _, w := range strings.Fields(name) {
		if c := w[0]; c >= 'A' && c <= 'Z' {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// rollGunModels gives every weapon kind a maker from corps and a model name in
// that maker's house style, unique across the world.
func rollGunModels(rng *rand.Rand, corps []Corporation) []GunModel {
	models := make([]GunModel, 0, len(weaponKinds))
	used := make(map[string]bool, len(weaponKinds))
	for _, kind := range weaponKinds {
		maker := rng.IntN(len(corps))
		c := corps[maker]
		model := rollModelName(rng, c)
		// A clash only happens when one maker draws the same number twice;
		// rolling again a bounded number of times is cheap on a stream that
		// drives nothing else, and a letter breaks any tie left over.
		for tries := 0; used[c.Name+" "+model] && tries < 8; tries++ {
			model = rollModelName(rng, c)
		}
		if used[c.Name+" "+model] {
			model += string(rune('A' + len(models)))
		}
		used[c.Name+" "+model] = true
		models = append(models, GunModel{Kind: kind, Maker: maker, Brand: c.Name, Model: model})
	}
	return models
}

// rollModelName draws one model name in c's house style.
func rollModelName(rng *rand.Rand, c Corporation) string {
	switch c.scheme {
	case schemeModel:
		return fmt.Sprintf("Model %d", 2+rng.IntN(98))
	case schemeMark:
		return "Mk " + roman(1+rng.IntN(12))
	case schemeCodeNumberSuffix:
		return fmt.Sprintf("%s-%d%c", c.Code, 10+rng.IntN(90), 'A'+rng.IntN(6))
	default:
		n := 1 + rng.IntN(99)
		if rng.IntN(2) == 0 {
			n = 100 + rng.IntN(900)
		}
		return fmt.Sprintf("%s-%d", c.Code, n)
	}
}

// roman renders 1..39 as a Roman numeral, enough for a "Mk" number.
func roman(n int) string {
	var b strings.Builder
	for ; n >= 10; n -= 10 {
		b.WriteByte('X')
	}
	ones := [...]string{"", "I", "II", "III", "IV", "V", "VI", "VII", "VIII", "IX"}
	b.WriteString(ones[n])
	return b.String()
}

// Description is the corporation's lore-tab blurb: a pure function of the
// corporation and the world's gun models, so rewording it never shifts a seed.
func (c Corporation) Description(index int, guns []GunModel) string {
	var made []string
	for _, g := range guns {
		if g.Maker == index {
			made = append(made, fmt.Sprintf("the %s %s", g.Model, g.Kind))
		}
	}
	s := fmt.Sprintf("%s, founded %d and headquartered %s %s.", c.Name, c.Founded, c.hqPrep, c.HQ)
	if len(made) == 0 {
		return s + " None of its products made it to the colony."
	}
	return s + " The colony knows it for " + joinList(made) + "."
}

// gunModelFor returns the model a gun of kind carries in this world, and false
// for anything that is not a gun (or a world with no models rolled).
func (w *World) gunModelFor(kind ItemKind) (GunModel, bool) {
	for _, g := range w.gunModels {
		if g.Kind == kind {
			return g, true
		}
	}
	return GunModel{}, false
}

// weaponPhrase names a gun the way narration does, article included: "a
// MarsCorp M-117 shotgun", or just "a shotgun" without a rolled model.
func (w *World) weaponPhrase(kind ItemKind) string {
	noun := kind.String()
	if g, ok := w.gunModelFor(kind); ok {
		noun = g.Name() + " " + noun
	}
	switch strings.ToLower(noun[:1]) {
	case "a", "e", "i", "o", "u":
		return "an " + noun
	}
	return "a " + noun
}
