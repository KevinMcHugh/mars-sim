package sim

// Features that fight: an alien's graded anatomy (alien_anatomy.go) turned
// into combat. Horns and antlers gore, a stinger stings, claws rake harder,
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
	if a.Horns > 0 || a.Antlers > 0 {
		s |= AttackSetOf(AttackGore)
	}
	if a.Stinger > 0 || a.TailTip == TailStinger {
		s |= AttackSetOf(AttackSting)
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
// horns yet). It is never empty; with nothing left, it bites.
func (w *World) attacksNow(e *Entity) []AttackMode {
	body := featureAttacks(w.anatomyOf(e))
	var modes []AttackMode
	for _, m := range w.alienSpeciesFor(e).Attacks() {
		if (m == AttackGore || m == AttackSting) && !body.Has(m) {
			continue
		}
		modes = append(modes, m)
	}
	if len(modes) == 0 {
		return []AttackMode{AttackBite}
	}
	return modes
}

// modeDamage scales a strike's base damage by the feature behind it: each
// horn adds 8% and each antler tine 5% to a gore (at most double), claws rake
// a quarter harder per grade, a club tail hits half again as hard and a
// spiked one three quarters, and a stinger stings at half (a barbed one at
// three quarters) but always finds the body, a vital part (see strike). The
// numbers were cut after a balance run; see docs/alien-lifecycles.md.
// Bite and strangle, and every mode of a featureless body, are unchanged.
func (w *World) modeDamage(e *Entity, mode AttackMode, base int) int {
	a := w.anatomyOf(e)
	pct := 100
	switch mode {
	case AttackGore:
		pct = min(200, 100+8*a.Horns+5*a.Antlers)
	case AttackClaw:
		pct = 100 + 25*a.Claws
	case AttackTail:
		switch a.TailTip {
		case TailClub:
			pct = 150
		case TailSpikedClub:
			pct = 175
		}
	case AttackSting:
		pct = 50
		if a.Stinger >= 2 {
			pct = 75
		}
	}
	if pct == 100 || base <= 0 {
		return base
	}
	return max(1, scaleRound(base, pct, 100))
}

// shellBlocks is the percent of a hit a shell turns aside, by grade: none,
// a patch, plates, a full carapace. At 50% for a carapace a breeding species
// became all but impossible to clear with pistols.
var shellBlocks = [4]int{0, 10, 20, 30}

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
	return max(1, scaleRound(dmg, 100-shellBlocks[min(shell, 3)], 100))
}
