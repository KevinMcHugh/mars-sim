package sim

import "testing"

// hallWorld is propertyWorld with a hall's worth of chairs — two, at (10, 6)
// and (12, 6) — and no one idle chatting elsewhere (talk-chance 0), so a
// conversation in the test is one the hall arranged.
func hallWorld(t *testing.T) *World {
	t.Helper()
	w := propertyWorld(t)
	w.cfg.TalkChance = 0
	w.SetTerrain(Point{10, 6}, Chair)
	w.SetTerrain(Point{12, 6}, Chair)
	w.refreshSpatial()
	return w
}

// calm zeroes every need of e except the ones the test is about.
func calm(w *World, e *Entity) {
	for k := DriveKind(0); k < numDrives; k++ {
		w.setDrive(e, k, 0)
	}
}

// Colonists who want company go to the hall for it: with the colony's two
// socializers starting at opposite ends of the cavern, the first
// conversation they hold is in the hall, not where they stood.
func TestSocializersMeetInTheHall(t *testing.T) {
	w := hallWorld(t)
	a := w.spawn(Colonist, Point{20, 14})
	b := w.spawn(Colonist, Point{6, 14})
	for _, e := range []*Entity{a, b} {
		calm(w, e)
		w.setDrive(e, DriveSocial, w.cfg.Drives[DriveSocial].SeekAt)
	}
	for i := 0; i < 400; i++ {
		w.step()
		for _, e := range []*Entity{a, b} {
			calm(w, e)
			w.setDrive(e, DriveSocial, max(w.driveLevel(e, DriveSocial), w.cfg.Drives[DriveSocial].SeekAt))
		}
		if a.State == Talking && b.State == Talking {
			if !w.inHall(a.Pos) || !w.inHall(b.Pos) {
				t.Fatalf("tick %d: talking at %v and %v, outside the hall", w.tick, a.Pos, b.Pos)
			}
			return
		}
	}
	t.Fatal("the two socializers never held a conversation")
}

// A lone colonist in the hall waits for company rather than wandering off,
// and a colonist arriving finds it there.
func TestAColonistWaitsInTheHallForCompany(t *testing.T) {
	w := hallWorld(t)
	a := w.spawn(Colonist, Point{11, 8})
	calm(w, a)
	w.setDrive(a, DriveSocial, w.cfg.Drives[DriveSocial].SeekAt)
	for i := 0; i < 60; i++ {
		w.step()
		calm(w, a)
		w.setDrive(a, DriveSocial, max(w.driveLevel(a, DriveSocial), w.cfg.Drives[DriveSocial].SeekAt))
		if !w.inHall(a.Pos) {
			t.Fatalf("tick %d: a waited at %v, outside the hall", w.tick, a.Pos)
		}
	}
	b := w.spawn(Colonist, Point{18, 12})
	calm(w, b)
	w.setDrive(b, DriveSocial, w.cfg.Drives[DriveSocial].SeekAt)
	for i := 0; i < 300; i++ {
		w.step()
		for _, e := range []*Entity{a, b} {
			calm(w, e)
			w.setDrive(e, DriveSocial, max(w.driveLevel(e, DriveSocial), w.cfg.Drives[DriveSocial].SeekAt))
		}
		if a.State == Talking && b.State == Talking {
			return
		}
	}
	t.Fatal("the newcomer never found the colonist waiting in the hall")
}

// A hungry colonist with a meal in its pocket takes it to the hall to eat.
func TestMealsAreEatenInTheHall(t *testing.T) {
	w := hallWorld(t)
	e := w.spawn(Colonist, Point{20, 14})
	calm(w, e)
	e.Inventory.Add(Meal, 1)
	w.setDrive(e, DriveFood, w.cfg.Drives[DriveFood].SeekAt)
	for i := 0; i < 200; i++ {
		w.step()
		if e.State == Eating {
			if !w.seated(e.Pos) {
				t.Fatalf("tick %d: ate at %v, not beside a chair", w.tick, e.Pos)
			}
			return
		}
	}
	t.Fatalf("the colonist never ate (state %v, job %v)", e.State, e.Job)
}

// Critical hunger eats where it stands: the walk is time it may not have.
func TestACriticallyHungryColonistEatsWhereItStands(t *testing.T) {
	w := hallWorld(t)
	e := w.spawn(Colonist, Point{20, 14})
	calm(w, e)
	e.Inventory.Add(Meal, 1)
	w.setDrive(e, DriveFood, w.cfg.Drives[DriveFood].CriticalAt)
	for i := 0; i < 30; i++ {
		w.step()
		if e.State == Eating {
			if w.inHall(e.Pos) {
				t.Fatalf("a critically hungry colonist walked to the hall (%v)", e.Pos)
			}
			return
		}
	}
	t.Fatalf("the colonist never ate (state %v, job %v)", e.State, e.Job)
}

// Without a chair there is no hall, and talk and meals stay where they were.
func TestNoHallNoChange(t *testing.T) {
	w := propertyWorld(t)
	if w.hallOpen() {
		t.Fatal("a hall is open with no chairs")
	}
	e := w.spawn(Colonist, Point{20, 14})
	if _, ok := w.mealSeat(e); ok {
		t.Fatal("a meal seat exists with no chairs")
	}
}

// Conversation in the hall is better; a chat elsewhere is not.
func TestHallTalkBonus(t *testing.T) {
	w := hallWorld(t)
	a := w.spawn(Colonist, Point{11, 7})
	b := w.spawn(Colonist, Point{13, 7})
	if got := w.hallTalkBonus(a, b); got != w.cfg.HallTalkBonus {
		t.Fatalf("bonus in the hall = %d, want %d", got, w.cfg.HallTalkBonus)
	}
	b.Pos = Point{20, 14}
	if got := w.hallTalkBonus(a, b); got != 0 {
		t.Fatalf("bonus with one partner outside = %d, want 0", got)
	}
}

// The colony commissions a hall on its own, and it ends up with chairs.
func TestColonyBuildsMeetingHall(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	for i := 0; i < 6000 && w.countTerrain(Chair) < 2; i++ {
		w.step()
	}
	if got := w.countTerrain(Chair); got < 2 {
		t.Fatalf("the colony built %d chairs in 6000 ticks, want at least 2", got)
	}
}

// colonists-per-chair 0 turns the whole thing off.
func TestNoHallWhenDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens, cfg.ColonistsPerChair = 0, 0
	w := newTestWorld(t, cfg)
	if w.wantsHall() {
		t.Fatal("the colony wants a hall with colonists-per-chair 0")
	}
}
