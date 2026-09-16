package sim

import "fmt"

// ---- Combat --------------------------------------------------------------
//
// Combat lands damage on one BodyPart rather than a shared HP pool: rollHit
// picks the spot, applyDamage does the bookkeeping, and callers (bite, shoot)
// handle the death/witness/logging fallout. See docs/combat.md.

// hitWeight mirrors bodyPartWeight: an attack is more likely to land on the
// biggest target (the torso) than a called shot to the head or a limb.
var hitWeight = bodyPartWeight

// rollHit picks the body part an attack lands on, weighted by hitWeight. It
// draws from the simulation RNG (w.rng), never the personality stream: which
// part gets hit decides who lives, so it must stay in the deterministic sim
// stream (see AGENTS.md).
func (w *World) rollHit() BodyPart {
	total := 0
	for _, wt := range hitWeight {
		total += wt
	}
	roll := w.rng.Intn(total)
	for part, wt := range hitWeight {
		if roll < wt {
			return BodyPart(part)
		}
		roll -= wt
	}
	return Torso // unreachable given weights sum to total, but a safe fallback
}

// applyDamage lands dmg on target's part, floored at zero, and mirrors it into
// the aggregate HP pool so existing HP-based display and checks keep working.
// It returns whether the hit was fatal (a vital part destroyed, or HP
// exhausted).
func applyDamage(target *Entity, part BodyPart, dmg int) (fatal bool) {
	if target.hasParts() {
		target.Parts[part] -= dmg
		if target.Parts[part] < 0 {
			target.Parts[part] = 0
		}
	}
	target.HP -= dmg
	if target.HP < 0 {
		target.HP = 0
	}
	return !target.Alive()
}

// weaponSpec is one weapon's combat stats, resolved from Config so callers
// don't switch on ItemKind repeatedly.
type weaponSpec struct {
	damage   int
	rng      int // max distance the weapon can fire from
	fireRest int // cooldown ticks between shots
}

// weaponStats resolves a weapon item to its combat stats. ItemNone (unarmed)
// has no ranged attack; callers must check bestWeapon first.
func weaponStats(weapon ItemKind, cfg Config) weaponSpec {
	switch weapon {
	case Shotgun:
		return weaponSpec{damage: cfg.ShotgunDamage, rng: cfg.ShotgunRange, fireRest: cfg.ShotgunFireRest}
	case Pistol:
		return weaponSpec{damage: cfg.PistolDamage, rng: cfg.PistolRange, fireRest: cfg.PistolFireRest}
	default:
		return weaponSpec{}
	}
}

// equipColonyShip hands out the colony ship's starting firearms: one weapon
// per colonist, up to cfg.StartPistols pistols and cfg.StartShotguns shotguns,
// spread across distinct colonists (so the first defenders aren't all carrying
// the same gun) rather than piled onto one. Excess weapons beyond the
// colonist count are simply not issued. colonists is already in a random
// order (worldgen draws spawn positions from a shuffled tile list), so taking
// it in order is itself a random draw of who gets armed.
func equipColonyShip(colonists []*Entity, cfg Config) {
	next := 0
	issue := func(kind ItemKind, count int) {
		for i := 0; i < count && next < len(colonists); i++ {
			colonists[next].Inventory.Add(kind, 1)
			next++
		}
	}
	issue(Shotgun, cfg.StartShotguns)
	issue(Pistol, cfg.StartPistols)
}

// fightAlien lets an armed colonist stand its ground against a nearby alien
// instead of fleeing: close the distance if the alien is out of weapon range,
// otherwise hold position and fire once the reload cooldown clears. Called
// from colonistTurn in place of the flee branch when the colonist carries a
// weapon.
func (w *World) fightAlien(e, alien *Entity, weapon ItemKind) {
	e.resting = false
	e.State = Fighting
	spec := weaponStats(weapon, w.cfg)

	if e.Pos.Chebyshev(alien.Pos) > spec.rng {
		// Out of range: close the distance. If the alien cannot be reached on
		// foot (walled off, or the colonist wedged), just hold position and
		// wait — better to stand ground than wander into the open.
		w.travelTo(e, alien.Pos)
		return
	}
	if e.Cooldown > 0 {
		e.Cooldown-- // reloading / recovering from recoil
		return
	}
	w.shoot(e, alien, weapon, spec)
	e.Cooldown = spec.fireRest
}

// shoot fires a colonist's weapon at an alien, landing damage on a random body
// part. A fatal hit kills the alien outright and leaves gore at its position;
// otherwise the alien presses on (its own turn still handles closing in and
// biting). Nearby colonists remember watching the fight.
func (w *World) shoot(colonist, alien *Entity, weapon ItemKind, spec weaponSpec) {
	part := w.rollHit()
	fatal := applyDamage(alien, part, spec.damage)
	witnesses := w.colonistsWithin(colonist.Pos, w.cfg.FleeRadius, colonist.ID)

	if fatal {
		w.addGore(alien.Pos)
		w.remove(alien.ID, fmt.Sprintf("shot by %s with a %s", colonist.displayName(), weapon))
		w.remember(colonist, fmt.Sprintf("Killed an alien with a %s!", weapon))
		w.log.add(fmt.Sprintf("%s guns down an alien with a %s.", colonist.displayName(), weapon))
		for _, wit := range witnesses {
			w.remember(wit, fmt.Sprintf("Watched %s kill an alien.", colonist.displayName()))
		}
		return
	}
	w.remember(colonist, fmt.Sprintf("Shot an alien in the %s with a %s.", part, weapon))
	for _, wit := range witnesses {
		w.remember(wit, fmt.Sprintf("Watched %s fight off an alien.", colonist.displayName()))
	}
	w.log.add(fmt.Sprintf("%s fires a %s at an alien.", colonist.displayName(), weapon))
}
