package sim

import "testing"

// A colonist carrying a weapon should stand and fight an approaching alien
// rather than flee, and win: the shotgun in this test deals enough damage
// that even the worst-case (non-vital) hit kills the alien in two shots. The
// alien's own bite is neutered (AlienDamage 0) so the test isolates whether
// gunfire can kill an alien at all, rather than which of two combatants a
// particular RNG draw happens to favor — that balance question belongs to
// playtesting, not this unit test.
func TestArmedColonistKillsAlien(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	// Keep the target from biting between the two shots. A bite now creates an
	// active flee stimulus, which is a separate behavior from gunfire itself.
	cfg.AlienSlowness = 4
	cfg.AlienDamage = 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	colonist := w.spawn(Colonist, center)
	colonist.affect.Grip = cfg.MoodMax // composed enough to stand and fight
	colonist.Inventory.Add(Shotgun, 1)
	alien := w.spawn(Alien, center.Add(cfg.ShotgunRange, 0))

	for i := 0; i < 200 && w.entities[alien.ID] != nil; i++ {
		w.step()
	}
	if w.entities[alien.ID] != nil {
		t.Fatal("armed colonist never killed the approaching alien")
	}
	if w.entities[colonist.ID] == nil {
		t.Fatal("armed colonist should have survived the fight")
	}
}

// An armed colonist starting from neutral affect (no cheated grip) must
// still stand and fight a fresh alien encounter rather than flee it forever.
// This pins the regression found during Phase 4/5 playtesting: the one-time
// grip hit from merely *seeing* an alien (`saw-alien`) used to outweigh the
// armed-colonist fight posture, and the focus switch hysteresis then locked
// colonists into fleeing even after grip decayed back toward neutral,
// leaving aliens never fought. See D-002 in
// The default is documented by the affect and combat behavior docs.
func TestArmedColonistFightsFromNeutralAffect(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	colonist := w.spawn(Colonist, center)
	colonist.Inventory.Add(Shotgun, 1)
	alien := w.spawn(Alien, center.Add(cfg.ShotgunRange+2, 0))

	for i := 0; i < 300 && w.entities[alien.ID] != nil && w.entities[colonist.ID] != nil; i++ {
		w.step()
	}
	if w.entities[colonist.ID] == nil {
		t.Fatal("armed colonist died fleeing an alien it should have fought")
	}
	if w.entities[alien.ID] != nil {
		t.Fatal("armed colonist starting from neutral affect never fought off the alien")
	}
}

// Without a weapon, a colonist should still flee an alien rather than engage
// it — arming the colony ship must not change unarmed behavior.
func TestUnarmedColonistStillFlees(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	colonist := w.spawn(Colonist, center)
	alien := w.spawn(Alien, center.Add(3, 0))

	w.colonistTurn(colonist)
	if colonist.State != Fleeing {
		t.Fatalf("unarmed colonist state = %v, want Fleeing", colonist.State)
	}
	_ = alien
}

// An armed colonist within weapon range should stand and fight instead.
func TestArmedColonistFightsInsteadOfFleeing(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	colonist := w.spawn(Colonist, center)
	colonist.affect.Grip = cfg.MoodMax // high grip favors confrontation
	colonist.Inventory.Add(Pistol, 1)
	w.spawn(Alien, center.Add(cfg.PistolRange, 0))

	w.colonistTurn(colonist)
	if colonist.State != Fighting {
		t.Fatalf("armed colonist state = %v, want Fighting", colonist.State)
	}
}

// distributeBodyParts must always account for every point of MaxHP: no damage
// source should be able to vanish HP that isn't tracked by some body part.
func TestBodyPartsSumToMaxHP(t *testing.T) {
	for _, maxHP := range []int{1, 4, 12, 30, 40, 97, 250} {
		parts := distributeBodyParts(maxHP)
		sum := 0
		for _, hp := range parts {
			sum += hp
		}
		if sum != maxHP {
			t.Errorf("distributeBodyParts(%d) sums to %d, want %d", maxHP, sum, maxHP)
		}
	}
}

// A vital part (head or torso) destroyed should be fatal even if the entity's
// aggregate HP has not run out — a called shot kills outright.
func TestVitalPartDestroyedIsFatal(t *testing.T) {
	cfg := testConfig()
	col := newEntity(1, Colonist, Point{}, cfg)
	if !col.Alive() {
		t.Fatal("freshly spawned colonist should be alive")
	}

	col.Parts[Head] = 0
	if col.Alive() {
		t.Fatal("a destroyed head should be fatal even with HP remaining")
	}
}

// A destroyed limb should not be fatal on its own.
func TestLimbDestroyedIsNotFatal(t *testing.T) {
	cfg := testConfig()
	col := newEntity(1, Colonist, Point{}, cfg)
	col.Parts[LeftArm] = 0
	if !col.Alive() {
		t.Fatal("a destroyed limb should not be fatal by itself")
	}
}

// Stomping a mouse, an alien bite that kills a colonist, and gunfire that
// kills an alien should all leave gore on the tile where it happened.
func TestViolentDeathsLeaveGore(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	mouseSpot := Point{5, 5}
	w.SetTerrain(mouseSpot, Floor)
	colonist := w.spawn(Colonist, Point{4, 5})
	mouse := w.spawn(Mouse, mouseSpot)
	w.stomp(colonist, mouse)
	if w.tiles[w.index(mouseSpot)].Gore == 0 {
		t.Error("stomping a mouse should leave gore")
	}

	alienSpot := Point{10, 5}
	w.SetTerrain(alienSpot, Floor)
	victimSpot := Point{9, 5}
	w.SetTerrain(victimSpot, Floor)
	alien := w.spawn(Alien, alienSpot)
	victim := w.spawn(Colonist, victimSpot)
	victim.HP = 1
	victim.Parts = [numBodyParts]int{1, 1, 1, 1, 1, 1}
	w.bite(alien, victim)
	if w.tiles[w.index(victimSpot)].Gore == 0 {
		t.Error("a fatal alien bite should leave gore")
	}

	gunSpot := Point{15, 5}
	w.SetTerrain(gunSpot, Floor)
	shooterSpot := Point{14, 5}
	w.SetTerrain(shooterSpot, Floor)
	target := w.spawn(Alien, gunSpot)
	target.HP = 1
	target.Parts = [numBodyParts]int{1, 1, 1, 1, 1, 1}
	shooter := w.spawn(Colonist, shooterSpot)
	w.shoot(shooter, target, Shotgun, weaponStats(Shotgun, cfg))
	if w.tiles[w.index(gunSpot)].Gore == 0 {
		t.Error("a killing shot should leave gore")
	}
}

// The colony ship's starting pistol and shotgun should be issued to distinct
// colonists at worldgen.
func TestColonyShipEquipsStartingColonists(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0
	cfg.StartColonists = 4
	cfg.StartPistols, cfg.StartShotguns = 1, 1
	w := newTestWorld(t, cfg)

	pistols, shotguns := 0, 0
	for _, e := range w.entities {
		if e.Kind != Colonist {
			continue
		}
		for _, stack := range e.Inventory {
			switch stack.Kind {
			case Pistol:
				pistols += stack.Count
			case Shotgun:
				shotguns += stack.Count
			}
		}
	}
	if pistols != cfg.StartPistols {
		t.Errorf("pistols issued = %d, want %d", pistols, cfg.StartPistols)
	}
	if shotguns != cfg.StartShotguns {
		t.Errorf("shotguns issued = %d, want %d", shotguns, cfg.StartShotguns)
	}
}
