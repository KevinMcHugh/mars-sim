package sim

import "testing"

// defenseWorld is zoneWorld's open chamber (x 4..35, y 4..19) with no aliens.
func defenseWorld(t *testing.T) *World {
	t.Helper()
	w := zoneWorld(t)
	w.cfg.StartAliens = 0
	return w
}

func guardsOf(w *World, id int) []EntityID {
	if d := w.defensePost(id); d != nil {
		return d.guards
	}
	return nil
}

// An armed colonist with nothing to do takes a defense post and walks into
// the area; an unarmed one is never posted.
func TestAnArmedColonistTakesADefensePost(t *testing.T) {
	w := defenseWorld(t)
	armed := w.spawn(Colonist, Point{6, 6, LandingLevel})
	armed.Inventory.Add(Pistol, 1)
	unarmed := w.spawn(Colonist, Point{7, 6, LandingLevel})

	if !w.orderDefense(OrderDefense{X0: 28, Y0: 14, X1: 32, Y1: 18, Detail: 2}) {
		t.Fatal("the defense order was refused")
	}
	d := w.defensePosts[0]
	for i := 0; i < 300 && !d.contains(armed.Pos); i++ {
		w.step()
	}
	if armed.Job != JobGuard || armed.guardPost != d.id {
		t.Fatalf("armed colonist is %v, not guarding post %d", armed.Job, d.id)
	}
	if !d.contains(armed.Pos) {
		t.Fatalf("guard never reached the area: at %v", armed.Pos)
	}
	if unarmed.Job == JobGuard {
		t.Fatal("an unarmed colonist was posted")
	}
	if got := guardsOf(w, d.id); len(got) != 1 || got[0] != armed.ID {
		t.Fatalf("post's guards = %v, want just %d", got, armed.ID)
	}
	if v := w.publishedDefense(); len(v) != 1 || v[0].OnStation != 1 || v[0].Detail != 2 {
		t.Fatalf("published defense = %+v", v)
	}
}

// A post never holds more guards than its detail; lowering the detail sends
// the latest guards back to work, and cancelling it releases them all.
func TestTheDetailCapsTheGuards(t *testing.T) {
	w := defenseWorld(t)
	var guards []*Entity
	for i := 0; i < 3; i++ {
		e := w.spawn(Colonist, Point{6 + i, 6, LandingLevel})
		e.Inventory.Add(Pistol, 1)
		guards = append(guards, e)
	}
	w.orderDefense(OrderDefense{X0: 20, Y0: 8, X1: 30, Y1: 16, Detail: 2})
	id := w.defensePosts[0].id
	for i := 0; i < 50; i++ {
		w.step()
	}
	if n := len(guardsOf(w, id)); n != 2 {
		t.Fatalf("%d guards on a detail of 2", n)
	}
	posted := 0
	for _, e := range guards {
		if e.Job == JobGuard {
			posted++
		}
	}
	if posted != 2 {
		t.Fatalf("%d colonists hold JobGuard, want 2", posted)
	}

	w.setDefenseDetail(SetDefenseDetail{ID: id, Detail: 1})
	if n := len(guardsOf(w, id)); n != 1 {
		t.Fatalf("%d guards after cutting the detail to 1", n)
	}
	w.cancelDefense(id)
	if len(w.defensePosts) != 0 {
		t.Fatal("cancelled post still listed")
	}
	for _, e := range guards {
		if e.Job == JobGuard || e.guardPost != 0 {
			t.Fatalf("colonist %d still on guard after the post was cancelled", e.ID)
		}
	}
}

// A guard on station is paid wage-guard from the treasury for every hour it
// spends inside the area, and the money is conserved.
func TestAGuardOnStationIsPaidByTheHour(t *testing.T) {
	w := defenseWorld(t)
	g := w.spawn(Colonist, Point{20, 10, LandingLevel})
	g.Inventory.Add(Pistol, 1)
	w.orderDefense(OrderDefense{X0: 18, Y0: 8, X1: 22, Y1: 12, Detail: 1})
	d := w.defensePosts[0]
	before := g.wallet
	hours := 3
	for i := 0; i < hours*w.cfg.TicksPerHour()+20; i++ {
		w.step()
	}
	if d.paid <= 0 {
		t.Fatalf("guard was never paid (job %v at %v)", g.Job, g.Pos)
	}
	if g.wallet-before != d.paid {
		t.Fatalf("wallet grew by %v, post paid %v", g.wallet-before, d.paid)
	}
	if d.paid%Money(w.cfg.WageGuard) != 0 || d.paid > Money(hours)*Money(w.cfg.WageGuard) {
		t.Fatalf("paid %v for at most %d hours at %v", d.paid, hours, Money(w.cfg.WageGuard))
	}
	assertMoneyConserved(t, w)
}

// A guard shoots an alien that comes near its area from beyond the distance
// at which a colonist would notice it for itself (flee-radius): the first
// shot is the guard's, not the fight focus's.
func TestAGuardShootsAnAlienNearItsZone(t *testing.T) {
	w := defenseWorld(t)
	w.cfg.AlienDamage = 0
	g := w.spawn(Colonist, Point{10, 10, LandingLevel})
	g.Inventory.Add(Shotgun, 1)
	g.affect.Grip = w.cfg.MoodMax
	w.orderDefense(OrderDefense{X0: 8, Y0: 8, X1: 12, Y1: 12, Detail: 1})
	for i := 0; i < 20 && g.Job != JobGuard; i++ {
		w.step()
	}
	if g.Job != JobGuard {
		t.Fatalf("colonist did not take the post: job %v", g.Job)
	}
	far := w.cfg.FleeRadius + 2
	if far > w.cfg.GuardSight {
		t.Skip("guard-sight does not reach past flee-radius at these settings")
	}
	alien := w.spawn(Alien, Point{g.Pos.X + far, g.Pos.Y, LandingLevel})
	full := alien.HP
	for i := 0; i < 100 && w.entities[alien.ID] != nil && alien.HP == full; i++ {
		w.step()
	}
	if w.entities[alien.ID] != nil && alien.HP == full {
		t.Fatal("the guard never fired on the alien near its zone")
	}
}

// A defense zone may lie on any level, but only where the colony has seen
// something, and no larger than MaxDefenseTiles.
func TestADefenseOrderNeedsSeenGroundAndASaneSize(t *testing.T) {
	w := defenseWorld(t)
	if w.orderDefense(OrderDefense{X0: 0, Y0: 0, X1: 39, Y1: 23, Level: LandingLevel + 5, Detail: 1}) {
		t.Fatal("a defense zone on a level nobody has broken into was accepted")
	}
	if MaxDefenseTiles < 40*24 {
		if w.orderDefense(OrderDefense{X0: 0, Y0: 0, X1: 39, Y1: 23, Detail: 1}) {
			t.Fatal("an oversized defense zone was accepted")
		}
	}
	if !w.orderDefense(OrderDefense{X0: 5, Y0: 5, X1: 6, Y1: 6, Detail: 99}) {
		t.Fatal("a small defense zone on seen floor was refused")
	}
	if got := w.defensePosts[len(w.defensePosts)-1].detail; got != MaxDefenseDetail {
		t.Fatalf("detail %d, want it capped at %d", got, MaxDefenseDetail)
	}
}

// A defense zone survives a save and load with its guards.
func TestDefenseZonesSurviveSaveAndLoad(t *testing.T) {
	w := defenseWorld(t)
	g := w.spawn(Colonist, Point{6, 6, LandingLevel})
	g.Inventory.Add(Pistol, 1)
	w.orderDefense(OrderDefense{X0: 20, Y0: 8, X1: 24, Y1: 12, Detail: 1})
	for i := 0; i < 10; i++ {
		w.step()
	}
	l := saveAndLoad(t, w)
	if len(l.defensePosts) != 1 || len(l.defensePosts[0].guards) != 1 {
		t.Fatalf("loaded posts = %+v", l.defensePosts)
	}
	if lg := l.entities[g.ID]; lg == nil || lg.Job != JobGuard || lg.guardPost != l.defensePosts[0].id {
		t.Fatal("the guard lost its post across save and load")
	}
}
