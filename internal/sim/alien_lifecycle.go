package sim

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// Alien lifecycles: some species change over a life, from an egg or a grub
// through other stages to the adult, which may split into castes (queen,
// worker, drone; bull and betty). Most species have a single form. Each
// stage is larger than the last and keeps everything it had, gaining or
// pushing further the species' own features; the same color, hide, pattern
// and eyes carry through, so the young are recognizably the adult's. See
// docs/alien-lifecycles.md.
//
// Lifecycles are rolled with the roster, on their own stream
// (alienLifecycleSeed), so adding them re-rolled no species' build, name or
// temperament. Which form an individual spawns as, which caste it grows
// into, and exactly when it grows are gameplay draws on World.rng.

// alienLifecycleSeed separates the lifecycle stream from the other lore
// streams.
const alienLifecycleSeed = 0x3243F6A8885A308D

// maxAlienForms bounds a lifecycle: up to four stages, the last split into
// up to four castes. A fixed array (not a slice) keeps AlienSpecies
// comparable and a snapshot's copy of the roster independent of the world's.
const maxAlienForms = 7

// AlienForm is one shape in a species' life: a stage, or a caste of the last
// stage.
type AlienForm struct {
	// Name is the stage or caste word ("grub", "cocoon", "queen"), empty for
	// the plain adult of a line without castes, which goes by the species'
	// own name.
	Name string
	// Stage is the form's place in the life, from 0. Castes share the last.
	Stage int
	// Weight is a caste's relative odds when the stage before it grows up; 0
	// for anything but a caste.
	Weight int
	// SizePct is the form's size as a percent of the adult's: rising through
	// the stages, 100 for the adult, more for a queen.
	SizePct int
	// Inert marks an egg, cocoon or pupa: it does not move or act, and no
	// colonist treats it as a threat.
	Inert bool
	// Ticks is roughly how long the stage lasts; 0 for the last stage.
	Ticks int
	// Lays marks the form that brings the next generation: the plain adult
	// of a line without castes, or its laying caste (a queen, a betty). It
	// lays the species' first form, an egg or live young (see layBrood).
	Lays bool

	Limbs, Arms int
	Tail, Wings bool
	Anatomy     AlienAnatomy
}

// LifeForms is the species' forms in stage order, or nil for a species with
// a single form.
func (sp AlienSpecies) LifeForms() []AlienForm { return sp.Forms[:sp.FormCount] }

// stageCount is how many stages the species' life has (1 for a single form).
func (sp AlienSpecies) stageCount() int {
	if sp.FormCount == 0 {
		return 1
	}
	return sp.Forms[sp.FormCount-1].Stage + 1
}

// wordFamily is the kind of creature a stage word comes from.
type wordFamily uint8

const (
	familyAny wordFamily = iota
	familyMammal
	familyInsect
	familyBird
	familyAquatic
	familyPlant
)

// stageWord is a word for a stage of life, where in a life it may stand, and
// what leaving it is called.
type stageWord struct {
	name   string
	family wordFamily
	early  bool   // may be the first stage
	middle bool   // may stand between the first and the adult
	inert  bool   // a casing that does not move: egg, cocoon, pupa
	leave  string // "hatches", "emerges", "molts"
}

// stageWords borrows Earth's words and lets a species use them in an order
// Earth would not: a joey can pupate, an egg can come in the middle.
var stageWords = []stageWord{
	{"egg", familyAny, true, true, true, "hatches"},
	{"joey", familyMammal, true, true, false, "molts"},
	{"puggle", familyMammal, true, false, false, "molts"},
	{"kit", familyMammal, true, false, false, "molts"},
	{"cub", familyMammal, true, true, false, "molts"},
	{"yearling", familyMammal, false, true, false, "molts"},
	{"grub", familyInsect, true, false, false, "molts"},
	{"larva", familyInsect, true, true, false, "molts"},
	{"nymph", familyInsect, true, true, false, "molts"},
	{"instar", familyInsect, false, true, false, "molts"},
	{"pupa", familyInsect, false, true, true, "emerges"},
	{"cocoon", familyInsect, false, true, true, "emerges"},
	{"chrysalis", familyInsect, false, true, true, "emerges"},
	{"hatchling", familyBird, true, false, false, "molts"},
	{"eyas", familyBird, true, false, false, "molts"},
	{"squab", familyBird, true, true, false, "molts"},
	{"fledgling", familyBird, false, true, false, "molts"},
	{"spawn", familyAquatic, true, false, false, "molts"},
	{"polliwog", familyAquatic, true, false, false, "molts"},
	{"elver", familyAquatic, true, true, false, "molts"},
	{"eft", familyAquatic, false, true, false, "molts"},
	{"bud", familyPlant, true, false, false, "molts"},
	{"sporeling", familyPlant, true, true, false, "molts"},
	{"husk", familyPlant, false, true, true, "emerges"},
}

// casteSet is a way the adult stage splits: each caste's word, odds, and
// size range as a percent of the adult.
type casteSet struct {
	family wordFamily
	castes []casteWord
}

type casteWord struct {
	name           string
	weight         int
	minPct, maxPct int
	lays           bool // the caste that brings the next generation
}

var casteSets = []casteSet{
	{familyInsect, []casteWord{{"queen", 1, 130, 170, true}, {"worker", 6, 80, 95, false}, {"drone", 3, 65, 85, false}}},
	{familyMammal, []casteWord{{"bull", 1, 110, 135, false}, {"betty", 1, 90, 105, true}}},
	{familyMammal, []casteWord{{"jack", 1, 100, 115, false}, {"jill", 1, 90, 100, true}}},
	{familyAny, []casteWord{{"matriarch", 1, 140, 175, true}, {"drudge", 5, 75, 90, false}, {"sire", 2, 90, 110, false}}},
}

// familyOf is the word family a hide suggests. Bony and rocky hides suggest
// nothing in particular.
func familyOf(skin AlienSkin) wordFamily {
	switch skin {
	case SkinFurry, SkinHairy:
		return familyMammal
	case SkinChitinous, SkinArmored:
		return familyInsect
	case SkinFeathered:
		return familyBird
	case SkinScaly, SkinSlimy, SkinSmooth, SkinGelatinous:
		return familyAquatic
	case SkinWoody, SkinMossy:
		return familyPlant
	default:
		return familyAny
	}
}

// wrongFamilyPercent is how often a species draws its words from any family
// rather than the one its hide suggests: the furred thing that pupates, the
// shelled thing with joeys. That is where the unsettling part comes from.
const wrongFamilyPercent = 20

// eggFirstPercent is how often a line begins as an egg, whatever its family:
// the platypus clause.
const eggFirstPercent = 40

// inertMiddlePercent is how often a middle stage is a casing (cocoon, pupa)
// when the stage before it was not.
const inertMiddlePercent = 35

// pickWeighted returns an index into weights with probability proportional to
// its weight, or -1 when they sum to nothing.
func pickWeighted(rng *rand.Rand, weights []int) int {
	total := 0
	for _, w := range weights {
		total += max(w, 0)
	}
	if total <= 0 {
		return -1
	}
	r := rng.IntN(total)
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		if r < w {
			return i
		}
		r -= w
	}
	return len(weights) - 1
}

// pickStageWord draws a word fit for the position, from the family (or any
// family), never one the line already used. ok is false only if the
// vocabulary has nothing left.
func pickStageWord(rng *rand.Rand, family wordFamily, first, wantInert bool, used map[string]bool) (stageWord, bool) {
	var fits []stageWord
	for _, w := range stageWords {
		if used[w.name] || w.inert != wantInert || (first && !w.early) || (!first && !w.middle) {
			continue
		}
		if family != familyAny && w.family != family && w.family != familyAny {
			continue
		}
		fits = append(fits, w)
	}
	if len(fits) == 0 && family != familyAny {
		return pickStageWord(rng, familyAny, first, wantInert, used)
	}
	if len(fits) == 0 {
		return stageWord{}, false
	}
	return fits[rng.IntN(len(fits))], true
}

// rollLifecycle gives a species its forms, or leaves FormCount at 0 for a
// single form, which is what most species get.
func rollLifecycle(rng *rand.Rand, sp *AlienSpecies, cfg Config) {
	stages := 1 + pickWeighted(rng, []int{
		cfg.AlienOneFormWeight, cfg.AlienTwoFormWeight, cfg.AlienThreeFormWeight, cfg.AlienFourFormWeight,
	})
	if stages <= 1 {
		return
	}
	family := familyOf(sp.Skin)
	if rng.IntN(100) < wrongFamilyPercent {
		family = familyAny
	}
	castes := rng.IntN(100) < cfg.AlienCastePercent

	// Sizes rise through the life to the adult's 100%.
	pcts := make([]int, stages)
	pcts[0] = 8 + rng.IntN(23) // 8..30
	for i := 1; i < stages-1; i++ {
		pcts[i] = pcts[i-1] + (100-pcts[i-1])*(30+rng.IntN(41))/100
	}
	pcts[stages-1] = 100

	used := map[string]bool{}
	var forms []AlienForm
	prevInert := false
	for i := 0; i < stages-1; i++ {
		var word stageWord
		ok := false
		switch {
		case i == 0 && rng.IntN(100) < eggFirstPercent:
			word, ok = stageWordNamed("egg"), true
		case i > 0 && !prevInert && rng.IntN(100) < inertMiddlePercent:
			word, ok = pickStageWord(rng, family, false, true, used)
		}
		if !ok {
			word, ok = pickStageWord(rng, family, i == 0, false, used)
		}
		if !ok {
			break // vocabulary exhausted: the life is as long as it got
		}
		used[word.name] = true
		prevInert = word.inert
		f := AlienForm{
			Name:    word.name,
			Stage:   i,
			SizePct: pcts[i],
			Inert:   word.inert,
			Ticks:   max(1, cfg.AlienStageTicks*(75+rng.IntN(51))/100),
		}
		if !word.inert {
			// A mobile young form has the adult's body in part: fewer limbs,
			// a tail only past halfway, no wings yet, and lesser features.
			f.Limbs = (sp.Limbs*(i+1) + stages - 1) / stages
			f.Arms = min(sp.Arms*(i+1)/stages, f.Limbs)
			f.Tail = sp.Tail && (i+1)*2 >= stages
			f.Anatomy = scaledAnatomy(sp.Anatomy, i+1, stages)
		}
		forms = append(forms, f)
	}
	last := len(forms) // the adult stage's index
	if last == 0 {
		return
	}
	adult := AlienForm{Stage: last, SizePct: 100, Limbs: sp.Limbs, Arms: sp.Arms, Tail: sp.Tail, Wings: sp.Wings, Anatomy: sp.Anatomy, Lays: true}
	if castes {
		set := pickCasteSet(rng, family)
		for _, c := range set.castes {
			f := adult
			f.Name, f.Weight, f.Lays = c.name, c.weight, c.lays
			f.SizePct = c.minPct + rng.IntN(c.maxPct-c.minPct+1)
			f.Anatomy = pushedAnatomy(sp.Anatomy, f.SizePct)
			forms = append(forms, f)
		}
	} else {
		forms = append(forms, adult)
	}
	for i, f := range forms {
		sp.Forms[i] = f
	}
	sp.FormCount = len(forms)
}

// stageWordNamed looks a word up by name.
func stageWordNamed(name string) stageWord {
	for _, w := range stageWords {
		if w.name == name {
			return w
		}
	}
	panic("unknown stage word " + name)
}

// pickCasteSet draws a caste set that fits the family (or any family).
func pickCasteSet(rng *rand.Rand, family wordFamily) casteSet {
	var fits []casteSet
	for _, s := range casteSets {
		if family == familyAny || s.family == family || s.family == familyAny {
			fits = append(fits, s)
		}
	}
	return fits[rng.IntN(len(fits))]
}

// pushedAnatomy is the adult's features at a caste's size: a caste larger
// than the adult pushes each feature the species has further (more horns, a
// heavier shell), a smaller one carries less. A feature the species lacks
// stays absent: a caste is more of the species' own idea, not a new one.
func pushedAnatomy(adult AlienAnatomy, pct int) AlienAnatomy {
	if pct <= 100 {
		return scaledAnatomy(adult, pct, 100)
	}
	push := func(v, most int) int {
		if v == 0 {
			return 0
		}
		return min(most, max(v+1, v*pct/100))
	}
	return AlienAnatomy{
		Horns:   push(adult.Horns, maxHorns),
		Antlers: push(adult.Antlers, maxAntlers),
		Spines:  push(adult.Spines, 3),
		Shell:   push(adult.Shell, 3),
		Claws:   push(adult.Claws, 3),
		Stinger: push(adult.Stinger, 2),
		TailTip: adult.TailTip,
	}
}

// ---- Individuals ------------------------------------------------------------

// formOf is an alien's current form, and false for one whose species has a
// single form.
func (w *World) formOf(e *Entity) (AlienForm, bool) {
	if e.life == nil {
		return AlienForm{}, false
	}
	sp := w.alienSpeciesFor(e)
	if e.life.form < 0 || e.life.form >= sp.FormCount {
		return AlienForm{}, false
	}
	return sp.Forms[e.life.form], true
}

// inert reports whether an alien is in a form that does not move or act (an
// egg, a cocoon). Like a dormant alien, it is no threat to anyone.
func (w *World) inert(e *Entity) bool {
	f, ok := w.formOf(e)
	return ok && f.Inert
}

// beginLife starts a newly spawned alien of a species with a lifecycle at a
// random stage (and, at the adult stage, a caste by its odds), part of the
// way through it, so a nest holds a spread of ages. A single-form species
// draws nothing.
func (w *World) beginLife(e *Entity) {
	sp := w.alienSpeciesFor(e)
	if sp.FormCount == 0 {
		return
	}
	stage := w.rng.IntN(sp.stageCount())
	form := w.pickFormAt(sp, stage)
	e.life = &LifeStage{form: form}
	if t := sp.Forms[form].Ticks; t > 0 {
		e.life.growAt = w.tick + 1 + w.rng.IntN(t)
	}
	if sp.Forms[form].Lays {
		// Part of the way to its next brood, so a nest's layers do not all
		// lay on the same tick.
		e.life.layAt = w.tick + 1 + w.rng.IntN(max(1, w.cfg.AlienLayTicks))
	}
	w.resizeAlien(e, 100)
}

// pickFormAt picks the form an alien takes at a stage: the stage's one form,
// or a caste by its odds.
func (w *World) pickFormAt(sp AlienSpecies, stage int) int {
	first, weights := -1, []int(nil)
	for i, f := range sp.LifeForms() {
		if f.Stage == stage {
			if first < 0 {
				first = i
			}
			weights = append(weights, f.Weight)
		}
	}
	if len(weights) <= 1 {
		return first
	}
	return first + pickWeighted(w.rng, weights)
}

// growUp moves an alien on to its next stage once its time comes: it takes
// the next stage's form (a caste by its odds), grows to that size, and the
// colony's log records it, as the colony can see it.
func (w *World) growUp(e *Entity) {
	sp := w.alienSpeciesFor(e)
	from := sp.Forms[e.life.form]
	before := w.alienNounFor(e)
	oldPct := from.SizePct
	e.life.form = w.pickFormAt(sp, from.Stage+1)
	e.life.growAt = 0
	if t := sp.Forms[e.life.form].Ticks; t > 0 {
		e.life.growAt = w.tick + t
	}
	if sp.Forms[e.life.form].Lays {
		e.life.layAt = w.tick + max(1, w.cfg.AlienLayTicks)
	}
	w.resizeAlien(e, oldPct)
	e.clearPath()
	e.Quarry = 0
	if !w.dormant(e) {
		w.logEvent(LogBirth, fmt.Sprintf("%s %s into %s.", capitalizeFirst(before), leaveVerb(from), w.alienNounFor(e)))
	}
}

// layBrood is a laying form's next generation: once its time comes it lays
// the species' first form (an egg, or live young: a joey, a grub) on a free
// floor tile beside it, at the very start of that stage. It tries again
// after alien-lay-ticks whether or not it laid, and it does not lay when
// alien-brood-cap of its species already live within alien-brood-radius of
// it, when alien-species-cap of its species live anywhere, or when no tile
// beside it is free. The local cap is a nest crowding itself, like a rat
// litter; the species cap is what keeps the young that wander off from
// letting it lay forever. Laying does not use the layer's turn.
func (w *World) layBrood(e *Entity) {
	e.life.layAt = w.tick + max(1, w.cfg.AlienLayTicks)
	if w.broodCrowded(e) || w.speciesPopulation(e) >= w.cfg.AlienSpeciesCap {
		return
	}
	spot, ok := Point{}, false
	for _, d := range neighbors8 {
		if p := e.Pos.Add(d.X, d.Y); w.Walkable(p) && !w.occupied(p) {
			spot, ok = p, true
			break
		}
	}
	if !ok {
		return
	}
	young := w.spawnAs(Alien, spot, e.Species)
	w.startYoung(young)
	if !w.dormant(e) {
		first, _ := w.formOf(young)
		verb := "bears"
		if first.Inert {
			verb = "lays"
		}
		w.logEvent(LogBirth, fmt.Sprintf("%s %s %s.", capitalizeFirst(w.alienNounFor(e)), verb, w.alienNounFor(young)))
	}
}

// broodCrowded reports whether a layer's surroundings already hold
// alien-brood-cap of its species, itself included.
func (w *World) broodCrowded(e *Entity) bool {
	n := 0
	for _, id := range w.entityIDsNearSorted(e.Pos, w.cfg.AlienBroodRadius) {
		if c := w.entities[id]; c != nil && c.Alive() && sameSpecies(c, e) {
			n++
		}
	}
	return n >= w.cfg.AlienBroodCap
}

// speciesPopulation counts the living aliens of e's species. Laying is
// rare, so a scan of the aliens is cheap enough.
func (w *World) speciesPopulation(e *Entity) int {
	n := 0
	for id := range w.kindEntities[Alien] {
		if c := w.entities[id]; c != nil && c.Alive() && c.Species == e.Species {
			n++
		}
	}
	return n
}

// startYoung puts a newly laid alien at the very start of its species'
// first form, at full health: spawning gave it a random stage, as a nest
// alien gets, and a brood is newborn.
func (w *World) startYoung(e *Entity) {
	sp := w.alienSpeciesFor(e)
	e.HP, e.Parts = e.MaxHP, e.MaxParts
	e.life.form, e.life.layAt = 0, 0
	e.life.growAt = 0
	if t := sp.Forms[0].Ticks; t > 0 {
		e.life.growAt = w.tick + t
	}
	w.resizeAlien(e, 100)
}

// leaveVerb is what leaving a form is called: an egg hatches, a cocoon's
// occupant emerges, anything else molts.
func leaveVerb(f AlienForm) string {
	for _, w := range stageWords {
		if w.name == f.Name {
			return w.leave
		}
	}
	return "molts"
}

// resizeAlien sets an alien's hit points for its current form's size,
// keeping the share of health it had at the old size (oldPct): a wounded
// grub is a wounded nymph.
func (w *World) resizeAlien(e *Entity, oldPct int) {
	f, ok := w.formOf(e)
	if !ok {
		return
	}
	base := w.species[Alien].HP
	newMax := max(minAlienFormHP, scaleRound(base, f.SizePct, 100))
	oldMax := e.MaxHP
	if oldMax <= 0 {
		oldMax = max(minAlienFormHP, scaleRound(base, oldPct, 100))
	}
	e.MaxHP = newMax
	e.HP = max(1, scaleRound(e.HP, newMax, oldMax))
	full := distributeBodyParts(newMax)
	for p := range e.Parts {
		switch {
		case e.MaxParts[p] == 0:
			e.Parts[p] = full[p]
		case e.Parts[p] > 0:
			// A part still standing stays standing: rounding must never
			// sever a limb or, for a vital part, kill.
			e.Parts[p] = min(full[p], max(1, scaleRound(e.Parts[p], full[p], e.MaxParts[p])))
		}
	}
	e.MaxParts = full
}

// minAlienFormHP is the fewest hit points any form of life has. Below it the
// head's share of the body rounds to nothing, and a body with an empty vital
// part counts as dead (Entity.Alive): an egg that small would never get a
// turn, and so never hatch.
const minAlienFormHP = 7

// alienDamage is what an alien's strike deals at its current size: its
// species' damage, scaled down for a young form and up for a queen. A
// species that deals damage at all always deals at least 1.
func (w *World) alienDamage(e *Entity) int {
	dmg := w.alienSpeciesFor(e).BiteDamage
	if f, ok := w.formOf(e); ok && dmg > 0 {
		dmg = max(1, scaleRound(dmg, f.SizePct, 100))
	}
	return dmg
}

// alienFormNoun is the species' singular with the form's word: "grelk grub",
// "grelk queen", or just "grelk" for the plain adult.
func (w *World) alienFormNoun(e *Entity) string {
	noun := w.alienSpeciesFor(e).Singular
	if f, ok := w.formOf(e); ok && f.Name != "" {
		noun += " " + f.Name
	}
	return noun
}

// ---- Description --------------------------------------------------------------

// lifePhrase is the lore tab's account of a species' life, or "" for a
// single form: "Their life has 3 stages: an egg; a grub, about a fifth of
// adult size, with 2 legs and a single nub of a horn; then the adult. Adults
// come as queens, workers or drones."
func (sp AlienSpecies) lifePhrase() string {
	if sp.FormCount == 0 {
		return ""
	}
	var stages []string
	castes := false
	for _, f := range sp.LifeForms() {
		if f.Stage == sp.stageCount()-1 {
			castes = castes || f.Name != ""
			continue
		}
		stages = append(stages, f.youngPhrase())
	}
	adult := "then the adult"
	if castes {
		adult = "then the adult castes"
	}
	out := fmt.Sprintf("Their life has %d stages: %s; %s.", sp.stageCount(), strings.Join(stages, "; "), adult)
	if castes {
		out += " " + sp.castePhrase()
	}
	return out + " " + sp.broodPhrase()
}

// broodPhrase says who brings the next generation and how: "Adults lay
// eggs.", "Only the betties bear young."
func (sp AlienSpecies) broodPhrase() string {
	verb := "bear young"
	if sp.Forms[0].Inert {
		verb = "lay eggs"
	}
	for _, f := range sp.LifeForms() {
		if f.Lays && f.Name != "" {
			return fmt.Sprintf("Only the %s %s.", pluralizeWord(f.Name), verb)
		}
	}
	return "Adults " + verb + "."
}

// youngPhrase describes a stage before the adult: "an egg", "a grub, about a
// fifth of adult size, with 2 legs and a single nub of a horn".
func (f AlienForm) youngPhrase() string {
	name := withArticle(f.Name)
	if f.Inert {
		return name
	}
	parts := []string{}
	if legs := f.Limbs - f.Arms; legs > 0 {
		parts = append(parts, pluralize(legs, "leg", "legs"))
	} else {
		parts = append(parts, "no legs")
	}
	if f.Arms > 0 {
		parts = append(parts, pluralize(f.Arms, "arm", "arms"))
	}
	if f.Tail {
		parts = append(parts, "a tail")
	}
	parts = append(parts, f.Anatomy.featurePhrases()...)
	return fmt.Sprintf("%s, %s, with %s", name, sizeWords(f.SizePct), joinList(parts))
}

// castePhrase names the adult castes, largest first: "Adults come as queens,
// workers or drones; a queen bears a crown of 9 horns."
func (sp AlienSpecies) castePhrase() string {
	var names []string
	biggest := AlienForm{}
	for _, f := range sp.LifeForms() {
		if f.Stage == sp.stageCount()-1 && f.Name != "" {
			names = append(names, pluralizeWord(f.Name))
			if f.SizePct > biggest.SizePct {
				biggest = f
			}
		}
	}
	out := "Adults come as " + joinOr(names) + "."
	if feats := biggest.Anatomy.featurePhrases(); len(feats) > 0 && biggest.SizePct > 100 {
		out = strings.TrimSuffix(out, ".") + fmt.Sprintf("; %s, the largest, bears %s.", withArticle(biggest.Name), joinList(feats))
	}
	return out
}

// sizeWords renders a young form's size against the adult's in words.
func sizeWords(pct int) string {
	switch {
	case pct <= 12:
		return "a tenth of adult size"
	case pct <= 22:
		return "about a fifth of adult size"
	case pct <= 40:
		return "about a third of adult size"
	case pct <= 60:
		return "about half adult size"
	case pct <= 85:
		return "nearly adult size"
	default:
		return "all but full-grown"
	}
}

// pluralizeWord makes a caste word plural: "queens", "bullies" would be
// wrong, so only the -y after a consonant changes ("betty" -> "betties").
func pluralizeWord(s string) string {
	if strings.HasSuffix(s, "y") && len(s) > 1 && !strings.ContainsRune("aeiou", rune(s[len(s)-2])) {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}

// joinOr joins with commas and a final "or": "queens, workers or drones".
func joinOr(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}
