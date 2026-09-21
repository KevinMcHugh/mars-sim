package sim

import "math"

// Personality gives colonists names, attributes, and traits. Most attributes
// (age, gender, orientation, skin tone, hair color) are populated for flavor
// and future systems but nothing simulates against them yet. Height and weight
// used to be in that group and no longer are: mutation resizes a colonist and
// scales their body with them, so HeightCM is live gameplay state once uranium
// is involved (see mutation.go). Traits likewise change how a colonist plays:
// they scale need rates and work behavior. As new needs and systems arrive,
// new traits slot in over the same machinery.
//
// Personality is generated from a dedicated RNG stream (World.prng) so that
// adding flavor never shifts the simulation's own RNG — with traits disabled the
// sim plays bit-for-bit as it did before personalities existed.

// Sex distinguishes male and female mice for breeding; colonists don't carry
// a sex attribute, since biological sex isn't otherwise simulated.
type Sex uint8

const (
	SexMale Sex = iota
	SexFemale
)

func (s Sex) String() string {
	switch s {
	case SexMale:
		return "male"
	case SexFemale:
		return "female"
	default:
		return "unknown"
	}
}

// Gender is a colonist's gender identity.
type Gender uint8

const (
	GenderMan Gender = iota
	GenderWoman
	GenderNonbinary
)

func (g Gender) String() string {
	switch g {
	case GenderMan:
		return "man"
	case GenderWoman:
		return "woman"
	case GenderNonbinary:
		return "non-binary"
	default:
		return "unknown"
	}
}

// Pronouns returns the short subject/object pronoun pair used in the UI.
func (g Gender) Pronouns() string {
	switch g {
	case GenderMan:
		return "he/him"
	case GenderWoman:
		return "she/her"
	case GenderNonbinary:
		return "they/them"
	default:
		return "they/them"
	}
}

// Orientation is a colonist's sexual orientation.
type Orientation uint8

const (
	Heterosexual Orientation = iota
	Homosexual
	Bisexual
	Asexual
)

func (o Orientation) String() string {
	switch o {
	case Heterosexual:
		return "heterosexual"
	case Homosexual:
		return "homosexual"
	case Bisexual:
		return "bisexual"
	case Asexual:
		return "asexual"
	default:
		return "unknown"
	}
}

// SkinTone is a colonist's skin tone, on the five-point scale emoji use for
// skin tone modifiers.
type SkinTone uint8

const (
	SkinLight SkinTone = iota
	SkinMediumLight
	SkinMedium
	SkinMediumDark
	SkinDark
)

func (s SkinTone) String() string {
	switch s {
	case SkinLight:
		return "light"
	case SkinMediumLight:
		return "medium-light"
	case SkinMedium:
		return "medium"
	case SkinMediumDark:
		return "medium-dark"
	case SkinDark:
		return "dark"
	default:
		return "unknown"
	}
}

// HairColor is a colonist's hair color (or its absence). Red, White, and Bald
// each have a dedicated emoji hair component; Black, Brown, and Blonde render
// with the plain, unmodified glyph since emoji has no component for them.
type HairColor uint8

const (
	HairBlack HairColor = iota
	HairBrown
	HairBlonde
	HairRed
	HairWhite
	HairBald
)

func (h HairColor) String() string {
	switch h {
	case HairBlack:
		return "black"
	case HairBrown:
		return "brown"
	case HairBlonde:
		return "blonde"
	case HairRed:
		return "red"
	case HairWhite:
		return "white"
	case HairBald:
		return "bald"
	default:
		return "unknown"
	}
}

// Trait is a personality trait that changes a colonist's needs or behavior.
type Trait uint8

const (
	TraitBigEater Trait = iota
	TraitLightEater
	TraitIndustrious
	TraitLazy
	TraitAsocial
	TraitIntrovert
	TraitExtrovert
	TraitTidy
	// TraitMutant is not rolled at spawn: it is acquired in play, the moment
	// uranium exposure first changes a colonist's body (see mutation.go).
	TraitMutant
	TraitMutantLover
	TraitResilient
	TraitCowardly
	TraitOptimist
	TraitPessimist

	numTraits // keep last
)

// traitNames is how each trait is spelled in traits.yaml. Explicit rather than
// derived from traitSpecs.Name, so renaming what the roster calls a trait
// cannot silently break the file that says what it does.
var traitNames = [numTraits]string{
	TraitBigEater:    "big-eater",
	TraitLightEater:  "light-eater",
	TraitIndustrious: "industrious",
	TraitLazy:        "lazy",
	TraitAsocial:     "asocial",
	TraitIntrovert:   "introvert",
	TraitExtrovert:   "extrovert",
	TraitTidy:        "tidy",
	TraitMutant:      "mutant",
	TraitMutantLover: "mutant-lover",
	TraitResilient:   "resilient",
	TraitCowardly:    "cowardly",
	TraitOptimist:    "optimist",
	TraitPessimist:   "pessimist",
}

// traitGroup collects mutually exclusive traits: a colonist gets at most one
// trait from each group (you can't be both a big eater and a light eater).
type traitGroup uint8

const (
	groupAppetite traitGroup = iota
	groupWorkEthic
	groupSocial
	// groupTemperament holds traits about disposition rather than a work/food/
	// social need — today just Tidy, but it is its own axis (not mutually
	// exclusive with anything else) so a colonist could still be both an
	// Industrious Extrovert and Tidy. A future opposite (e.g. "Slob", numbed to
	// gore) would join this group.
	groupTemperament
	// groupMutation holds what uranium did to a colonist. Nothing in it is
	// rollable, so a colonist is never generated pre-mutated; the group exists
	// so an acquired trait still has a home in the same table as every other
	// trait rather than becoming a special case on Profile.
	groupMutation
	// groupMutantAttitude is how a colonist feels about mutants — its own axis,
	// so it does not compete with temperament, and so the obvious opposite (a
	// purist who recoils from them) can join it later.
	groupMutantAttitude
	// groupNerve is how fast a colonist stops being new to things — whether
	// repetition hardens them or wears them down. Its own axis: being steady
	// under fire says nothing about being squeamish about mess, so a colonist
	// can be both Tidy and Resilient. Declared last so adding it leaves every
	// earlier group's roll for a given seed untouched.
	groupNerve
	// groupOutlook is where a colonist's mood settles when nothing in
	// particular is happening. Its own axis rather than part of
	// groupTemperament: being squeamish about mess says nothing about whether
	// you expect things to go well, and a colonist should be able to be both.
	// Declared last so adding it leaves every earlier group's roll for a given
	// seed untouched.
	groupOutlook

	numTraitGroups // keep last
)

// traitSpec is the static description and effects of a trait. Effect scales are
// multipliers where 1.0 (or an unset 0, treated as 1.0) means no change.
type traitSpec struct {
	Name  string
	Desc  string
	group traitGroup
	// acquired marks a trait that is only ever gained during play, never
	// generated at spawn. rollTraits skips it (and skips its group entirely if
	// nothing in the group is rollable, without drawing a number, so adding
	// such a group cannot shift any other personality roll for a given seed).
	acquired bool

	needRiseScale  [numNeeds]float64 // per-need multiplier on how fast it rises
	restScale      float64           // multiplier on idle rest duration
	workScale      float64           // multiplier on mine/build time (lower = faster)
	socialNoNeed   bool
	socialScale    float64
	socialCapacity int
	socialPenalty  int

	// affectHome displaces where this colonist's mood settles once nothing is
	// happening to them, away from the neutral origin everyone else returns
	// to. Summed across traits and resolved at spawn, like every other field
	// here. See docs/affect.md.
	affectHome MoodVector
}

// traitSpecs is the trait table. Adding a trait is a table edit here (plus a
// group if it is a new axis); the systems read effects generically.
var traitSpecs = [numTraits]traitSpec{
	TraitBigEater: {
		Name: "Big Eater", Desc: "Burns through rations and hungers faster.",
		group: groupAppetite, needRiseScale: [numNeeds]float64{NeedFood: 1.5},
	},
	TraitLightEater: {
		Name: "Light Eater", Desc: "Makes rations last and hungers slower.",
		group: groupAppetite, needRiseScale: [numNeeds]float64{NeedFood: 0.7},
	},
	TraitIndustrious: {
		Name: "Industrious", Desc: "Works quickly and rests little.",
		group: groupWorkEthic, workScale: 0.75, restScale: 0.5,
	},
	TraitLazy: {
		Name: "Lazy", Desc: "Works slowly and rests often.",
		group: groupWorkEthic, workScale: 1.4, restScale: 2.0,
	},
	TraitAsocial: {
		Name: "Asocial", Desc: "Does not need social interaction.",
		group: groupSocial, socialNoNeed: true,
	},
	TraitIntrovert: {
		Name: "Introvert", Desc: "Needs less socializing, but too much conversation wears on morale.",
		group: groupSocial, socialScale: 0.5, socialCapacity: 2, socialPenalty: 5,
	},
	TraitExtrovert: {
		Name: "Extrovert", Desc: "Needs frequent social interaction to feel fulfilled.",
		group: groupSocial, socialScale: 1.5, socialCapacity: 6,
	},
	TraitTidy: {
		Name: "Tidy", Desc: "Squeamish about mess; the sight of gore hits morale harder.",
		group: groupTemperament,
		// No need-rise/rest/work/social effect — Tidy's only effect is the extra
		// EvtSawGore mood penalty declared in lifeevents.go, gated on this trait.
	},
	TraitMutant: {
		Name: "Mutant", Desc: "Uranium rewrote them; they carry parts nobody is born with.",
		group: groupMutation, acquired: true,
		// No scalar effect. Being a mutant shows up as the extra body parts
		// mutation grew (see mutation.go) and in how other colonists take
		// them — this trait is the marker both of those read.
	},
	TraitOptimist: {
		Name: "Optimist", Desc: "Settles back into expecting things to work out.",
		group: groupOutlook,
		// Valence carries the outlook itself; the small grip lift is the part
		// that shows in behavior, since an optimist rattles a little less.
		affectHome: MoodVector{Grip: 8, Valence: 25},
	},
	TraitPessimist: {
		Name: "Pessimist", Desc: "Settles back into expecting the worst, whatever the day held.",
		group:      groupOutlook,
		affectHome: MoodVector{Grip: -8, Valence: -25},
	},
	TraitResilient: {
		Name: "Resilient", Desc: "Slow to grow numb; the hundredth horror still lands like the first.",
		group: groupNerve,
		// No scalar effect. Resilience is a WearRate rule in affect.go: it
		// changes how fast experience stops being new, not how hard it hits.
	},
	TraitCowardly: {
		Name: "Cowardly", Desc: "Rattled easily, and worn down fast by what rattles them.",
		group: groupNerve,
		// No scalar effect either — see traitRules for the wear rate it bends
		// and the weight it puts on a threat.
	},
	TraitMutantLover: {
		Name: "Mutant-Lover", Desc: "Drawn to the changed; warms to mutants far faster than to anyone else.",
		group: groupMutantAttitude,
		// No scalar effect either: its work is the directional affinity bonus
		// in finishTalk and the mood effects gated on it in lifeevents.go.
	},
}

// Name returns a trait's display name.
func (t Trait) Name() string { return traitSpecs[t].Name }

// Desc returns a trait's one-line description.
func (t Trait) Desc() string { return traitSpecs[t].Desc }

func (t Trait) String() string { return traitSpecs[t].Name }

// Profile is a colonist's identity: a name, populated attributes, and any
// traits. Aliens have no Profile.
type Profile struct {
	Age         int
	Name        string
	Gender      Gender
	Orientation Orientation
	HeightCM    int
	WeightKG    int
	SkinTone    SkinTone
	HairColor   HairColor
	Traits      []Trait

	// BornHeightCM and BornWeightKG are the body this colonist was generated
	// with, after any heredity re-framing. HeightCM and WeightKG drift away
	// from them once uranium starts resizing people (see mutation.go), and
	// every resize recomputes the weight from these rather than from the last
	// weight: a colonist who is stretched and shrunk back a dozen times must
	// end up the weight they started at, and rescaling a rounded integer over
	// and over would instead grind them away to nothing.
	BornHeightCM int
	BornWeightKG int

	// Heredity bookkeeping, unexported because nothing outside generation needs
	// it: these are the forms of an attribute that pass between relatives, as
	// opposed to the displayed value that age or a marriage may have changed.
	// See heredity.go.
	given    string    // the half of Name that is never inherited
	surname  string    // the half of Name a family shares
	hairBase HairColor // hair color before age greys or thins it
	heightZ  float64   // height in standard deviations from the gender mean
}

// setSurname moves a profile into a family's name, rebuilding the display name.
// Profiles built by hand (in tests and tools) have no given name; they keep the
// Name they were given rather than becoming a bare surname.
func (p *Profile) setSurname(s string) {
	if p.given == "" {
		return
	}
	p.surname = s
	p.Name = p.given + " " + s
}

// clone deep-copies a Profile (including its Traits slice) so a Snapshot never
// aliases live state.
func (p *Profile) clone() *Profile {
	if p == nil {
		return nil
	}
	c := *p
	c.Traits = append([]Trait(nil), p.Traits...)
	return &c
}

// HasTrait reports whether the profile carries a trait.
func (p *Profile) HasTrait(t Trait) bool {
	for _, x := range p.Traits {
		if x == t {
			return true
		}
	}
	return false
}

// assignPersonality gives a colonist a generated Profile and resolves its traits
// into the per-colonist effective parameters the systems read (need rise, rest
// duration, work speed). Uses the personality RNG so it never perturbs the sim.
func (w *World) assignPersonality(e *Entity) {
	p := &Profile{}
	p.Age = w.rollAge()
	p.Gender = w.rollGender()
	p.Orientation = w.rollOrientation()
	w.rollBody(p)
	p.SkinTone = w.rollSkinTone()
	p.hairBase, p.HairColor = w.rollHair(p.Age)
	p.Traits = w.rollTraits()
	e.Profile = p
	w.rollName(e)

	w.resolveTraitEffects(e)
	// A colonist starts where they will settle, rather than at everyone's
	// origin: a pessimist has been a pessimist since before the sim began.
	e.affect.Charge = e.affectHome.Charge
	e.affect.Grip = e.affectHome.Grip
	e.affect.Valence = e.affectHome.Valence
	w.refreshMoodAttractor(e)
}

// rollAge generates an adult colonist age. Keeping colonists adults means every
// generated person can work, while the range still leaves room for believable
// parent/child relationships.
func (w *World) rollAge() int {
	return 18 + w.agePRNG.Intn(63) // 18..80
}

// resolveTraitEffects recomputes a colonist's effective parameters from its
// traits, starting from the config baselines set in newEntity.
func (w *World) resolveTraitEffects(e *Entity) {
	riseMul := [numNeeds]float64{}
	for i := range riseMul {
		riseMul[i] = 1
	}
	restMul, workMul, socialMul := 1.0, 1.0, 1.0
	socialNoNeed := false
	socialCapacity, socialPenalty := 1<<30, 0
	home := MoodVector{}
	for _, tr := range e.Profile.Traits {
		s := traitSpecs[tr]
		home.Charge += s.affectHome.Charge
		home.Grip += s.affectHome.Grip
		home.Valence += s.affectHome.Valence
		for i := 0; i < int(numNeeds); i++ {
			if s.needRiseScale[i] > 0 {
				riseMul[i] *= s.needRiseScale[i]
			}
		}
		if s.restScale > 0 {
			restMul *= s.restScale
		}
		if s.workScale > 0 {
			workMul *= s.workScale
		}
		if s.socialNoNeed {
			socialNoNeed = true
		} else if s.socialScale > 0 {
			socialMul *= s.socialScale
		}
		if s.socialCapacity > 0 && s.socialCapacity < socialCapacity {
			socialCapacity = s.socialCapacity
			socialPenalty = s.socialPenalty
		}
	}
	for i := 0; i < int(numNeeds); i++ {
		if NeedKind(i) == NeedSocial && socialNoNeed {
			e.needRise[i] = 0
			continue
		}
		scale := riseMul[i]
		if NeedKind(i) == NeedSocial {
			scale *= socialMul
		}
		e.needRise[i] = atLeast1(int(math.Round(float64(w.cfg.Needs[i].Rise) * scale)))
	}
	e.restTicks = atLeast1(int(math.Round(float64(w.cfg.RestTicks) * restMul)))
	e.workScale = workMul
	e.socialCapacity, e.socialPenalty = socialCapacity, socialPenalty
	lim := w.cfg.MoodMax
	e.affectHome = MoodVector{
		Charge:  clampInt(home.Charge, -lim, lim),
		Grip:    clampInt(home.Grip, -lim, lim),
		Valence: clampInt(home.Valence, -lim, lim),
	}
	// Deliberately not touching e.affect: this also runs when a trait is
	// acquired in play (see giveTrait), and a colonist who has just mutated
	// should not have the mood that produced forgotten. Spawn seeds affect
	// from the resolved home itself.
}

// rollTraits picks at most one trait from each group, each group taken with
// TraitChance probability. A group with nothing rollable in it (see
// traitSpec.acquired) is skipped before any number is drawn, so adding one
// leaves every other colonist's generation identical for the same seed.
func (w *World) rollTraits() []Trait {
	var out []Trait
	for g := traitGroup(0); g < numTraitGroups; g++ {
		group := rollableTraitsInGroup(g)
		if len(group) == 0 {
			continue
		}
		if w.prng.Intn(100) >= w.cfg.TraitChance {
			continue
		}
		out = append(out, group[w.prng.Intn(len(group))])
	}
	return out
}

// traitsInGroup returns the traits belonging to a group, in declaration order.
func traitsInGroup(g traitGroup) []Trait {
	var out []Trait
	for t := Trait(0); t < numTraits; t++ {
		if traitSpecs[t].group == g {
			out = append(out, t)
		}
	}
	return out
}

// rollableTraitsInGroup is traitsInGroup without the traits that can only be
// acquired in play.
func rollableTraitsInGroup(g traitGroup) []Trait {
	var out []Trait
	for _, t := range traitsInGroup(g) {
		if !traitSpecs[t].acquired {
			out = append(out, t)
		}
	}
	return out
}

// rollGender picks a gender identity: mostly binary, occasionally non-binary.
func (w *World) rollGender() Gender {
	switch r := w.prng.Intn(100); {
	case r < 47:
		return GenderMan
	case r < 94:
		return GenderWoman
	default:
		return GenderNonbinary
	}
}

func (w *World) rollOrientation() Orientation {
	switch r := w.prng.Intn(100); {
	case r < 80:
		return Heterosexual
	case r < 90:
		return Bisexual
	case r < 97:
		return Homosexual
	default:
		return Asexual
	}
}

// rollBody generates a plausible height (cm) and weight (kg), loosely correlated
// with gender and with each other through a body-mass index. Height is drawn as
// a z-score — standard deviations from the colonist's own gender mean — and kept
// on the profile so a relative's frame can be inherited across genders without
// dragging their absolute centimetres along with it (see heredity.go).
func (w *World) rollBody(p *Profile) {
	p.heightZ = w.prng.NormFloat64()
	bmi := clampFloat(24.0+w.prng.NormFloat64()*3.5, 16, 38)
	p.HeightCM = heightFromZ(p.Gender, p.heightZ)
	p.WeightKG = weightFor(p.HeightCM, bmi)
	p.rememberBornBody()
}

// setHeightZ re-frames a colonist at a new height z-score while keeping the
// build they were rolled with: inheriting a relative's frame should change how
// tall a colonist is, not how heavy-set.
func setHeightZ(p *Profile, z float64) {
	bmi := bmiOf(p.HeightCM, p.WeightKG)
	p.heightZ = z
	p.HeightCM = heightFromZ(p.Gender, z)
	p.WeightKG = weightFor(p.HeightCM, bmi)
	p.rememberBornBody()
}

// rememberBornBody records the body a colonist arrived with, which is what
// mutation rescales their weight from ever after (see setStature in
// mutation.go). Every path that *generates* a body ends here — the original
// roll and a heredity re-framing alike — because inheriting a relative's frame
// changes who the colonist always was, while a mutation changes what became of
// them.
func (p *Profile) rememberBornBody() {
	p.BornHeightCM, p.BornWeightKG = p.HeightCM, p.WeightKG
}

// heightFromZ converts a z-score into centimetres against the gender's mean.
func heightFromZ(g Gender, z float64) int {
	mean, sd := 178.0, 7.0
	switch g {
	case GenderWoman:
		mean, sd = 165.0, 6.5
	case GenderNonbinary:
		mean, sd = 172.0, 8.0
	}
	return clampInt(int(math.Round(mean+z*sd)), 145, 205)
}

// bmiOf recovers the body-mass index a profile's height and weight imply, so a
// re-framed colonist keeps the build they were rolled with. Hand-built profiles
// with no body fall back to the population mean rather than dividing by zero.
func bmiOf(heightCM, weightKG int) float64 {
	if heightCM <= 0 || weightKG <= 0 {
		return 24
	}
	m := float64(heightCM) / 100
	return float64(weightKG) / (m * m)
}

func weightFor(heightCM int, bmi float64) int {
	m := float64(heightCM) / 100
	return atLeast1(int(math.Round(bmi * m * m)))
}

// rollSkinTone picks a skin tone uniformly across the five emoji tone points.
func (w *World) rollSkinTone() SkinTone {
	return SkinTone(w.prng.Intn(5))
}

// rollHair picks both the color a colonist's hair grew in as and the color they
// actually show today: age greys and thins hair, so an older colonist often
// shows white or bald over whatever they were born with. The two are kept apart
// because relatives inherit the natural color, not the aged one — a grandmother
// gone white still passes her brown hair down (see heredity.go).
func (w *World) rollHair(age int) (base, shown HairColor) {
	base = w.rollHairBase()
	whiteChance, baldChance := 5, 3
	switch {
	case age >= 60:
		whiteChance, baldChance = 55, 15
	case age >= 45:
		whiteChance, baldChance = 20, 8
	}
	switch r := w.prng.Intn(100); {
	case r < whiteChance:
		return base, HairWhite
	case r < whiteChance+baldChance:
		return base, HairBald
	default:
		return base, base
	}
}

// rollHairBase picks a natural hair color. White and bald are not options here:
// both are things age does to hair, applied on top by rollHair.
func (w *World) rollHairBase() HairColor {
	switch r := w.prng.Intn(100); {
	case r < 37:
		return HairBlack
	case r < 69:
		return HairBrown
	case r < 95:
		return HairBlonde
	default:
		return HairRed
	}
}

// rollName gives a profile a first + last name, drawing the first name from a
// pool that suits the gender identity. The surname is only provisional: a
// colonist generated into an existing family takes that family's name instead
// (see heredity.go).
func (w *World) rollName(e *Entity) {
	p := e.Profile
	p.given = w.rollGivenName(p.Gender)
	p.surname = lastNames[w.prng.Intn(len(lastNames))]
	p.Name = p.given + " " + p.surname
	w.uniquifyName(e)
}

// givenNameRedraws bounds the search for a full name nobody in the colony is
// already using. The pools are finite, so a big enough colony will eventually
// exhaust them; a duplicate name is worse than a loop that gives up, but far
// better than failing to place a colonist at all.
const givenNameRedraws = 8

// uniquifyName re-draws a colonist's given name until no other colonist answers
// to the same full name. The pools are small enough (24 given names against 26
// surnames) that a colony of a few dozen hits a collision about half the time by
// chance alone, and heredity makes it likelier still by pulling whole families
// onto one surname — and two colonists with the same name are simply
// indistinguishable in the roster. Only the given name moves: the surname may be
// a family's, which is the part worth keeping.
func (w *World) uniquifyName(e *Entity) {
	p := e.Profile
	w.releaseName(e)
	for i := 0; i < givenNameRedraws && w.nameTaken(p.Name, e.ID); i++ {
		p.given = w.rollGivenName(p.Gender)
		p.Name = p.given + " " + p.surname
	}
	w.colonistNames[p.Name] = e.ID
}

// nameTaken reports whether a colonist other than except already goes by name.
func (w *World) nameTaken(name string, except EntityID) bool {
	id, ok := w.colonistNames[name]
	return ok && id != except
}

// releaseName frees a colonist's name for reuse: they are dying, or about to be
// renamed into a family. Guarded on ownership so renaming never evicts the
// namesake who was there first.
func (w *World) releaseName(e *Entity) {
	if e.Profile == nil {
		return
	}
	if w.colonistNames[e.Profile.Name] == e.ID {
		delete(w.colonistNames, e.Profile.Name)
	}
}

// rollGivenName draws a first name from a pool that suits the gender identity.
func (w *World) rollGivenName(g Gender) string {
	var pool []string
	switch g {
	case GenderMan:
		pool = firstNamesMasc
	case GenderWoman:
		pool = firstNamesFem
	default: // non-binary draws from either pool
		if w.prng.Intn(2) == 0 {
			pool = firstNamesMasc
		} else {
			pool = firstNamesFem
		}
	}
	return pool[w.prng.Intn(len(pool))]
}

// scaleTicks scales a base work duration by a colonist's workScale, never going
// below one tick.
func scaleTicks(base int, scale float64) int {
	if scale <= 0 {
		scale = 1
	}
	return atLeast1(int(math.Round(float64(base) * scale)))
}

func atLeast1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Name pools. Kept deliberately varied to suit a multinational Mars corp.
var (
	firstNamesMasc = []string{
		"Arjun", "Bo", "Cyrus", "Diego", "Ehsan", "Felix", "Goro", "Hassan",
		"Ivan", "Jamal", "Kwame", "Liang", "Mateo", "Niko", "Omar", "Pavel",
		"Quinn", "Ravi", "Sven", "Tariq", "Ugo", "Viktor", "Wei", "Yusuf",
	}
	firstNamesFem = []string{
		"Amara", "Bianca", "Chiara", "Dalia", "Esme", "Fatima", "Greta", "Hana",
		"Ingrid", "Jia", "Kira", "Lucia", "Mei", "Nadia", "Oksana", "Priya",
		"Rosa", "Sana", "Tamar", "Uma", "Vera", "Wanjiru", "Yara", "Zoe",
	}
	lastNames = []string{
		"Adeyemi", "Boone", "Cho", "Duarte", "Eriksson", "Fournier", "Gupta",
		"Haddad", "Ibarra", "Jansen", "Kovac", "Lindqvist", "Moreau", "Nakamura",
		"Okafor", "Petrov", "Quaranta", "Rossi", "Salazar", "Tanaka", "Ustinov",
		"Vargas", "Whitfield", "Xu", "Yilmaz", "Zheng",
	}
)
