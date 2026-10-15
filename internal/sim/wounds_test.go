package sim

import "testing"

// hit lands dmg on c's part as an alien's strike does: the wound opens and
// the world hears of it.
func hit(w *World, c *Entity, part BodyPart, dmg int) {
	applyDamage(c, part, dmg)
	w.hurt(c)
}

// A hit opens a wound and makes the colonist a casualty: stable for a graze,
// serious once the part is below half, critical once the colonist is.
func TestAHitOpensAWound(t *testing.T) {
	w, c := driveWorld(t, aidConfig(func(c *Config) { c.Aid = false }))
	arm := c.MaxParts[LeftArm]
	hit(w, c, LeftArm, 1)
	if c.wounds[LeftArm] != WoundOpen || !w.isCasualty(c) {
		t.Fatalf("wound %v, casualty %v", c.wounds[LeftArm], w.isCasualty(c))
	}
	if got := w.perilOf(c); got != PerilStable {
		t.Fatalf("a graze: peril %v, want stable", got)
	}
	hit(w, c, LeftArm, arm/2)
	if got := w.perilOf(c); got != PerilSerious {
		t.Fatalf("an arm below half: peril %v, want serious", got)
	}
	c.HP = c.MaxHP * (w.cfg.PerilCriticalHPPct - 1) / 100
	if got := w.perilOf(c); got != PerilCritical {
		t.Fatalf("badly hurt: peril %v, want critical", got)
	}
}

// A colonist alone dresses its own wound, at half speed, and is no longer a
// casualty; a bystander beside it does the dressing at full speed, and both
// remember it.
func TestWoundsGetDressed(t *testing.T) {
	w, c := driveWorld(t, aidConfig(nil))
	c.Profile.Traits = nil
	for d := DriveKind(0); d < numDrives; d++ {
		w.setDrive(c, d, 0)
	}
	hit(w, c, LeftArm, 2)
	self := w.dressTicks(c, true)
	if took := stepUntil(w, 3*self, func() bool { return c.wounds[LeftArm] == WoundDressed }); took > 3*self {
		t.Fatalf("never dressed its own wound; focus %v job %v", c.focus, c.Job)
	}
	if w.isCasualty(c) {
		t.Fatal("still a casualty with its wound dressed")
	}

	w, c, b := aidWorld(t, nil)
	hit(w, c, RightLeg, 2)
	c.aidedBy = b.ID // the bystander has it; the casualty does not race it
	b.Job, b.aidTarget, b.focus = JobAid, c.ID, FocusAid
	if took := stepUntil(w, 40, func() bool { return c.wounds[RightLeg] == WoundDressed }); took > 40 {
		t.Fatalf("bystander never dressed it; focus %v job %v", b.focus, b.Job)
	}
	if memoriesOf(b, "helped") != 1 || memoriesOf(c, "was-helped") != 1 {
		t.Fatalf("helped %d, was-helped %d", memoriesOf(b, "helped"), memoriesOf(c, "was-helped"))
	}
	if b.practice[SkillMedicine] == 0 {
		t.Fatal("dressing taught the helper no medicine")
	}
}

// An open wound never heals; a dressed one heals a point an hour asleep in a
// bunk, three in a cot, half of one down on the floor, until the part is
// whole. A destroyed part is closed by dressing, and never grows back.
func TestDressedWoundsHealLyingDown(t *testing.T) {
	w, c := driveWorld(t, aidConfig(func(c *Config) { c.Aid = false }))
	hour := w.cfg.TicksPerHour()
	hit(w, c, Torso, 4)
	c.State = Sleeping
	for range 3 * hour {
		w.healWounds(c)
	}
	if c.Parts[Torso] != c.MaxParts[Torso]-4 {
		t.Fatal("an open wound healed")
	}
	w.dressWound(c, c)
	hp := c.HP
	for range 2 * hour {
		w.healWounds(c)
	}
	if got := c.HP - hp; got != 2*w.cfg.HealPerHour {
		t.Fatalf("two hours in a bunk healed %d HP, want %d", got, 2*w.cfg.HealPerHour)
	}
	c.State = Idle
	for range hour {
		w.healWounds(c)
	}
	if c.HP-hp != 2*w.cfg.HealPerHour {
		t.Fatal("healed standing up")
	}
	c.State = Sleeping
	for range 4 * hour {
		w.healWounds(c)
	}
	if c.Parts[Torso] != c.MaxParts[Torso] || c.wounds[Torso] != WoundNone || c.dressed != 0 {
		t.Fatalf("torso %d/%d wound %v dressed %d after a night", c.Parts[Torso], c.MaxParts[Torso], c.wounds[Torso], c.dressed)
	}

	hit(w, c, LeftArm, c.MaxParts[LeftArm])
	w.dressWound(c, c)
	if c.wounds[LeftArm] != WoundNone || c.dressed != 0 || w.isCasualty(c) {
		t.Fatal("a destroyed arm is still a wound after dressing")
	}
}

// With bleeding on, a bad open wound drains HP until the colonist collapses
// from blood loss, a casualty nobody can rouse; dressed, it stops bleeding,
// heals on the floor, and comes round once healed above bleed-wake.
func TestBleedingCollapsesAndComesRound(t *testing.T) {
	w, c := driveWorld(t, aidConfig(func(c *Config) { c.Aid, c.Bleeding, c.BleedPct = false, true, 400 }))
	for d := DriveKind(0); d < numDrives; d++ {
		w.setDrive(c, d, 0)
	}
	hit(w, c, Torso, c.MaxParts[Torso]-1)
	if w.bleedRate(c) == 0 {
		t.Fatal("a torso wound at one HP is not bleeding")
	}
	if took := stepUntil(w, 2000, func() bool { return c.passedOut }); took > 2000 {
		t.Fatalf("never collapsed; HP %d/%d", c.HP, c.MaxHP)
	}
	if !c.downBled || w.perilOf(c) < PerilSerious || !w.isCasualty(c) {
		t.Fatalf("down from blood loss %v peril %v", c.downBled, w.perilOf(c))
	}
	if b := w.spawn(Colonist, c.Pos.Add(1, 0)); w.canHelp(b, c) {
		// A bystander can dress the wound; it cannot rouse a colonist down
		// from blood loss.
		w.dressWound(b, c)
		if w.canHelp(b, c) {
			t.Fatal("a bystander is offered a colonist it can do nothing for")
		}
	}
	if w.bleedRate(c) != 0 {
		t.Fatal("still bleeding once dressed")
	}
	c.HP = c.MaxHP * w.cfg.BleedWakePct / 100
	w.step()
	w.step()
	if c.passedOut {
		t.Fatalf("still down at %d/%d HP, not bleeding", c.HP, c.MaxHP)
	}
}

// A colonist looking for work takes an open place on the EMT detail, keeps
// working, is paid by the hour, and goes off call when it sleeps.
func TestEMTDetailStaffsItselfAndIsPaid(t *testing.T) {
	w, c := driveWorld(t, aidConfig(func(c *Config) { c.EMTDetail, c.EMTWage = 1, 250 }))
	other := w.spawn(Colonist, c.Pos.Add(2, 0))
	w.tryTakeOnCall(c)
	w.tryTakeOnCall(other)
	if !c.onCall || other.onCall || len(w.onCall) != 1 {
		t.Fatalf("on call: %v %v %v", c.onCall, other.onCall, w.onCall)
	}
	w.treasury = 100
	wallet := c.wallet
	for range w.cfg.TicksPerHour() {
		w.serveOnCall(c)
	}
	// $2.50 an hour: $2 paid, 50 cents carried to the next payday.
	if c.wallet-wallet != 2 || w.emtPaid != 2 || c.emtOwed != 50 {
		t.Fatalf("paid %d (total %d, owed %d¢), want $2 and 50¢ owed", c.wallet-wallet, w.emtPaid, c.emtOwed)
	}
	c.State = Sleeping
	w.serveOnCall(c)
	if c.onCall || len(w.onCall) != 0 {
		t.Fatal("still on call asleep")
	}
	w.setEMTDetail(SetEMTDetail{Detail: 1})
	w.tryTakeOnCall(other)
	w.setEMTDetail(SetEMTDetail{Detail: 0})
	if other.onCall || len(w.onCall) != 0 {
		t.Fatal("a cut detail kept its EMT")
	}
}

// Each colonist asks its own price for being on call: what the calls take
// of its time at what its time is worth, plus a premium a Caring colonist
// halves and a Callous one doubles. Below its ask it will not take the
// place; once its time is worth more than the wage, it quits on payday.
func TestColonistsWeighTheEMTWage(t *testing.T) {
	w, c := driveWorld(t, aidConfig(func(c *Config) { c.EMTDetail, c.EMTWage = 1, 0 }))
	ask := w.emtAsk(c)
	if ask <= 0 {
		t.Fatalf("ask %d¢: being on call should cost something", ask)
	}
	w.tryTakeOnCall(c)
	if c.onCall {
		t.Fatal("took the place for nothing")
	}
	c.Profile.Traits = []Trait{TraitCaring}
	caring := w.emtAsk(c)
	c.Profile.Traits = []Trait{TraitCallous}
	callous := w.emtAsk(c)
	c.Profile.Traits = nil
	if !(caring < ask && ask < callous) {
		t.Fatalf("asks: caring %d¢, plain %d¢, callous %d¢", caring, ask, callous)
	}
	w.emtShare = 500 // calls take half an EMT's time
	if busy := w.emtAsk(c); busy <= ask {
		t.Fatalf("ask %d¢ with calls taking half its time, %d¢ before", busy, ask)
	}
	w.emtShare = w.cfg.EMTCallShare
	w.setEMTWage(SetEMTWage{Cents: ask})
	w.tryTakeOnCall(c)
	if !c.onCall {
		t.Fatalf("would not take %d¢ asking %d¢", w.emtWage, ask)
	}
	if v := w.publishedEMT(); v.Wage != ask || len(v.OnCall) != 1 {
		t.Fatalf("view %+v", v)
	}
	// Its time is suddenly worth much more: it quits on payday.
	c.earned[SkillMining], c.earnedTick[SkillMining] = 50_000, w.tick+1
	w.tick++
	for range w.cfg.TicksPerHour() {
		w.serveOnCall(c)
	}
	if c.onCall {
		t.Fatalf("stayed on call asking %d¢ for %d¢", w.emtAsk(c), w.emtWage)
	}
}

// An on-call EMT hears of a casualty out of anyone's sight and goes to it.
func TestEMTRespondsAcrossTheColony(t *testing.T) {
	w, c := driveWorld(t, aidConfig(nil))
	emt := w.spawn(Colonist, Point{16, 16, LandingLevel})
	for _, e := range []*Entity{c, emt} {
		e.Profile.Traits = nil
		for d := DriveKind(0); d < numDrives; d++ {
			w.setDrive(e, d, 0)
		}
	}
	if emt.Pos.Within(c.Pos, w.cfg.AidSight) {
		t.Fatal("test setup: the EMT can see the casualty")
	}
	knockOut(w, c)
	if _, ok := w.aidOptionFor(emt, false); ok {
		t.Fatal("offered a casualty out of sight while off call")
	}
	w.emtDetail = 1
	w.tryTakeOnCall(emt)
	if took := stepUntil(w, 60, func() bool { return !c.passedOut }); took > 60 {
		t.Fatalf("EMT never came; focus %v job %v at %v", emt.focus, emt.Job, emt.Pos)
	}
}

// An EMT drags a colonist down from blood loss to a free cot, the two side
// by side the whole way; laid there it is not hungry and heals at the cot's
// rate.
func TestEMTDragsACasualtyToACot(t *testing.T) {
	w, c := driveWorld(t, aidConfig(func(c *Config) { c.Bleeding = true }))
	cot := Point{14, 5, LandingLevel}
	w.SetTerrain(cot, Cot)
	w.refreshSpatial()
	emt := w.spawn(Colonist, c.Pos.Add(1, 0))
	for _, e := range []*Entity{c, emt} {
		e.Profile.Traits = nil
		for d := DriveKind(0); d < numDrives; d++ {
			w.setDrive(e, d, 0)
		}
	}
	w.emtDetail = 1
	w.tryTakeOnCall(emt)
	hit(w, c, Torso, c.MaxParts[Torso]-1)
	c.HP = c.MaxHP / 5 // well below bleed-wake: it stays down once dressed
	w.collapseFromBloodLoss(c)
	if took := stepUntil(w, 200, func() bool { return c.inCot }); took > 200 {
		t.Fatalf("never laid in a cot; casualty at %v, EMT %v job %v focus %v", c.Pos, emt.Pos, emt.Job, emt.focus)
	}
	if !c.Pos.Adjacent(cot) || w.cotClaims[cot] != c.ID {
		t.Fatalf("in a cot at %v, claims %v", c.Pos, w.cotClaims)
	}
	if c.wounds[Torso] != WoundDressed {
		t.Fatal("dragged before its wound was dressed")
	}
	if memoriesOf(emt, "helped") != 1 {
		t.Fatal("the EMT does not remember it")
	}
	if w.entityAt(c.Pos) != c || w.entityAt(emt.Pos) != emt {
		t.Fatal("occupancy out of step after the drag")
	}
	if rate := c.drives[DriveFood].rate; rate > 0 {
		t.Fatalf("hungry in a cot: food rate %d", rate)
	}
}

// Swapping two entities keeps both indexes in step.
func TestSwapEntities(t *testing.T) {
	w, a := driveWorld(t, aidConfig(nil))
	b := w.spawn(Colonist, a.Pos.Add(1, 0))
	ap, bp := a.Pos, b.Pos
	w.swapEntities(a, b)
	if a.Pos != bp || b.Pos != ap || w.entityAt(ap) != b || w.entityAt(bp) != a {
		t.Fatal("swap left the occupancy index wrong")
	}
}
