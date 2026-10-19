package sim

// Features that fight: an alien's graded anatomy (alien_anatomy.go) turned
// into combat. Horns, antlers and tusks gore, a stinger stings, a beak or
// mandibles bite harder, claws rake and pincers crush harder,
// a clubbed tail hits harder, and a shell blunts what hits it. Everything
// here is read from the alien's current form (a young form's lesser
// features, a queen's pushed ones), so a species grows into its weapons.
// See docs/alien-lifecycles.md.
//
// The two new attack modes are granted by anatomy, never rolled: a species
// with horns gores. They sit in their own list (featureAttackModes) rather
// than in attackModes, which rollAttackModes walks drawing from the lore
// stream; appending there would have re-rolled every later species' attacks.

// featureAttackModes is the modes anatomy grants, in the order Attacks lists
// them after the rolled ones.
var featureAttackModes = [...]AttackMode{AttackGore, AttackSting}

// featureAttacks is the attack modes a body's features grant.
func featureAttacks(a AlienAnatomy) AttackSet {
	var s AttackSet
	if a.Horns > 0 || a.Antlers > 0 || a.Tusks > 0 {
		s |= AttackSetOf(AttackGore)
	}
	if a.Stinger > 0 || a.TailTip == TailStinger {
		s |= AttackSetOf(AttackSting)
	}
	if a.Pincers > 0 {
		s |= AttackSetOf(AttackClaw) // a rolled mode, but pincers always use it
	}
	return s
}

// anatomyOf is the alien's body as it is now: its form's features, or for a
// single-form species the species'.
func (w *World) anatomyOf(e *Entity) AlienAnatomy {
	if f, ok := w.formOf(e); ok {
		return f.Anatomy
	}
	return w.alienSpeciesFor(e).Anatomy
}

// attacksNow is the alien's species' attack modes that its current body can
// use: a feature mode needs the feature (a young form may not have grown its
// horns yet), and claws need arms (a pincered species' armless grub cannot
// pinch). It is never empty; with nothing left, it bites.
func (w *World) attacksNow(e *Entity) []AttackMode {
	body := featureAttacks(w.anatomyOf(e))
	f, young := w.formOf(e)
	var modes []AttackMode
	for _, m := range w.alienSpeciesFor(e).Attacks() {
		if (m == AttackGore || m == AttackSting) && !body.Has(m) {
			continue
		}
		if m == AttackClaw && young && f.Arms == 0 {
			continue
		}
		modes = append(modes, m)
	}
	if len(modes) == 0 {
		return []AttackMode{AttackBite}
	}
	return modes
}

// weaponTable is how hard a body's features hit, and how much a shell turns
// aside. Most species use ordinaryWeapons; the rare apex species
// (AlienSpecies.Apex) use apexWeapons.
type weaponTable struct {
	gorePerHorn, gorePerTine, goreMax int    // gore damage, percent of base
	gorePerTusk                       int    // gore bonus per tusk grade, percent
	beakBite, mandibleBite            int    // bite mode with a beak or mandibles, percent
	clawPerGrade                      int    // claw mode bonus per grade, percent
	pincerPerGrade                    int    // claw mode bonus per pincer grade, percent
	club, spikedClub                  int    // tail mode with a club, percent
	sting, barbedSting                int    // sting damage, percent
	shellBlocks                       [4]int // percent of a hit turned aside, by shell grade
}

// ordinaryWeapons is most species. The numbers were cut after a balance run
// (a carapace blocking half of every shot made a breeding species all but
// impossible to clear); see docs/alien-lifecycles.md.
var ordinaryWeapons = weaponTable{
	gorePerHorn: 8, gorePerTine: 5, goreMax: 200,
	gorePerTusk: 15,
	beakBite:    130, mandibleBite: 125,
	clawPerGrade: 25, pincerPerGrade: 30,
	club: 150, spikedClub: 175,
	sting: 50, barbedSting: 75,
	shellBlocks: [4]int{0, 10, 20, 30},
}

// apexWeapons is the rare apex species: very deadly on purpose. Its rarity
// (alien-apex-percent), not its numbers, is what keeps a colony's odds fair.
var apexWeapons = weaponTable{
	gorePerHorn: 15, gorePerTine: 10, goreMax: 300,
	gorePerTusk: 30,
	beakBite:    175, mandibleBite: 160,
	clawPerGrade: 50, pincerPerGrade: 60,
	club: 200, spikedClub: 250,
	sting: 100, barbedSting: 150,
	shellBlocks: [4]int{0, 25, 45, 60},
}

// weaponsOf is the table an alien's features fight by.
func (w *World) weaponsOf(e *Entity) *weaponTable {
	if w.alienSpeciesFor(e).Apex {
		return &apexWeapons
	}
	return &ordinaryWeapons
}

// modeDamage scales a strike's base damage by the feature behind it, by the
// alien's weaponTable: gore by horns, tines and tusks (capped), a bite by a
// beak or mandibles, claws by grade, a tail by its club, and a sting at a
// fixed share (more if barbed) that always finds the body, a vital part (see
// strike). Strangle, a plain bite, and every mode of a featureless body are
// unchanged.
func (w *World) modeDamage(e *Entity, mode AttackMode, base int) int {
	a, t := w.anatomyOf(e), w.weaponsOf(e)
	pct := 100
	switch mode {
	case AttackGore:
		pct = min(t.goreMax, 100+t.gorePerHorn*a.Horns+t.gorePerTine*a.Antlers+t.gorePerTusk*a.Tusks)
	case AttackBite:
		switch a.Mouth {
		case MouthBeak:
			pct = t.beakBite
		case MouthMandibles:
			pct = t.mandibleBite
		}
	case AttackClaw:
		pct = 100 + t.clawPerGrade*a.Claws + t.pincerPerGrade*a.Pincers
	case AttackTail:
		switch a.TailTip {
		case TailClub:
			pct = t.club
		case TailSpikedClub:
			pct = t.spikedClub
		}
	case AttackSting:
		pct = t.sting
		if a.Stinger >= 2 {
			pct = t.barbedSting
		}
	}
	if pct == 100 || base <= 0 {
		return base
	}
	return max(1, scaleRound(base, pct, 100))
}

// armored is a hit on target after its shell: an alien with a shell takes
// less from every gunshot and every strike. A hit that does damage at all
// still does at least 1.
func (w *World) armored(target *Entity, dmg int) int {
	if target.Kind != Alien || dmg <= 0 {
		return dmg
	}
	shell := w.anatomyOf(target).Shell
	if shell <= 0 {
		return dmg
	}
	return max(1, scaleRound(dmg, 100-w.weaponsOf(target).shellBlocks[min(shell, 3)], 100))
}

// alienApexSeed separates the apex roll's stream from the other lore
// streams, so adding it moved no other draw.
const alienApexSeed = 0x13198A2E03707344

// rollApex marks the rare apex species: alien-apex-percent of species that
// have a feature and fight (a featureless body has nothing to be deadly
// with, nor has one whose only feature is a bill or a proboscis, and a
// Friendly species never attacks). It draws once per species
// whatever the anatomy or temperament, so one species never shifts
// another's roll.
func rollApex(cfg Config, roster []AlienSpecies) {
	rng := newRand(cfg.Seed ^ alienApexSeed)
	for i := range roster {
		hit := rng.IntN(100) < cfg.AlienApexPercent
		roster[i].Apex = hit && roster[i].Anatomy.armed() && roster[i].Temperament != TemperamentFriendly
	}
}
