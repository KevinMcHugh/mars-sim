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

// rollHit picks the body part an attack lands on target, weighted by hitWeight
// over the parts that target actually has (see Entity.hasPart). Weighting over
// the *entity's* anatomy rather than the whole enum is what lets a mutation
// add a limb: a grown third arm becomes another place to be hit, and it
// dilutes the odds of any one shot finding a vital part. It draws from the
// simulation RNG (w.rng), never the personality stream: which part gets hit
// decides who lives, so it must stay in the deterministic sim stream (see
// AGENTS.md).
func (w *World) rollHit(target *Entity) BodyPart {
	total := 0
	for part := BodyPart(0); part < numBodyParts; part++ {
		if target.hasPart(part) {
			total += hitWeight[part]
		}
	}
	if total <= 0 {
		return Torso // an entity with no parts tracked: fall back to the trunk
	}
	roll := w.rng.Intn(total)
	for part := BodyPart(0); part < numBodyParts; part++ {
		if !target.hasPart(part) {
			continue
		}
		if roll < hitWeight[part] {
			return part
		}
		roll -= hitWeight[part]
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
	part := w.rollHit(alien)
	fatal := applyDamage(alien, part, spec.damage)
	witnesses := w.colonistsWithin(colonist.Pos, w.cfg.FleeRadius, colonist.ID)
	noun := w.alienNounFor(alien)

	if fatal {
		w.addGore(alien.Pos)
		w.addCorpse(alien.Pos) // nothing eats an alien: the body is the colony's to dispose of
		w.remove(alien.ID, fmt.Sprintf("shot by %s with a %s", colonist.displayName(), weapon))
		w.remember(colonist, event(EvtKilledAlien, "Killed %s with a %s!", noun, weapon))
		w.log.add(fmt.Sprintf("%s guns down %s with a %s.", colonist.displayName(), noun, weapon))
		for _, wit := range witnesses {
			w.remember(wit, event(EvtWitnessedAlienKilled, "Watched %s kill %s.", colonist.displayName(), noun))
		}
		return
	}
	w.remember(colonist, event(EvtWoundedAlien, "Shot %s in the %s with a %s.", noun, part, weapon))
	for _, wit := range witnesses {
		w.remember(wit, event(EvtWitnessedGunfight, "Watched %s fight off %s.", colonist.displayName(), noun))
	}
	w.log.add(fmt.Sprintf("%s fires a %s at %s.", colonist.displayName(), weapon, noun))
}
