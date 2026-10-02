package sim

import "testing"

// memoriesOf counts the occurrences of one reaction in a colonist's memories,
// collapsed runs included.
func memoriesOf(e *Entity, rule RuleID) int {
	n := 0
	for _, m := range e.Memories {
		if m.Rule == rule {
			n += m.Count
		}
	}
	return n
}

// The shipped drives each declare their consequence at the ceiling, and only
// food's kills.
func TestShippedCeilingConsequences(t *testing.T) {
	cfg := DefaultConfig()
	want := [numDrives]Consequence{
		DriveFood:    ConsequenceDeath,
		DriveBladder: ConsequenceSoiling,
		DriveSocial:  ConsequenceLoneliness,
		DriveSleep:   ConsequencePassOut,
	}
	for d := DriveKind(0); d < numDrives; d++ {
		spec := cfg.Drives[d]
		if got := spec.CeilingConsequence(); got != want[d] {
			t.Errorf("%s at its ceiling: %v, want %v", d, got, want[d])
		}
		if spec.Fatal() != (d == DriveFood) {
			t.Errorf("%s Fatal() = %v", d, spec.Fatal())
		}
	}
}

// A consequence of the ceiling follows a retuned ceiling.
func TestCeilingConsequenceFollowsMax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Drives[DriveFood].Max = 800
	cfg.Drives[DriveFood].CriticalAt = 800
	table, err := compileDrive(cfg.Drives[DriveFood], cfg.StarveDamage)
	if err != nil {
		t.Fatal(err)
	}
	if got := table.hpDrain[table.bandOf(800)]; got != cfg.StarveDamage {
		t.Fatalf("drain at the retuned ceiling = %d, want %d", got, cfg.StarveDamage)
	}
}

// A social drive at its ceiling is felt as loneliness: once on arrival, again
// every ConsequenceEvery ticks while it stays, and afresh after it is met.
func TestLonelinessIsFeltAtTheCeiling(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	spec := w.cfg.Drives[DriveSocial]
	setDriveRate(w, c, DriveSocial, 0) // hold the level where the test puts it
	w.setDrive(c, DriveSocial, spec.Max-1)

	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 0 {
		t.Fatalf("felt lonely below the ceiling: %d", got)
	}

	w.setDrive(c, DriveSocial, spec.Max)
	valence := c.affect.Valence
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 1 {
		t.Fatalf("felt-lonely at the ceiling = %d, want 1", got)
	}
	if c.affect.Valence >= valence {
		t.Fatalf("loneliness did not lower valence: %d -> %d", valence, c.affect.Valence)
	}

	w.tick += spec.ConsequenceEvery - 1
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 1 {
		t.Fatalf("felt lonely again before consequence-every: %d", got)
	}
	w.tick++
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 2 {
		t.Fatalf("felt-lonely after consequence-every = %d, want 2", got)
	}

	w.resetDrive(c, DriveSocial)
	w.setDrive(c, DriveSocial, spec.Max)
	w.tick++
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 3 {
		t.Fatalf("a new stay at the ceiling should be felt at once: %d, want 3", got)
	}
}

// Only the side of a conversation that came to it wanting company socialized.
func TestConversationWhileLonelySocializes(t *testing.T) {
	w := roomsTestWorld(20, 20)
	carve(w, Point{5, 5}, Point{6, 5}, Floor)
	lonely := w.spawn(Colonist, Point{5, 5})
	content := w.spawn(Colonist, Point{6, 5})
	spec := w.cfg.Drives[DriveSocial]
	w.setDrive(lonely, DriveSocial, spec.SeekAt)
	w.setDrive(content, DriveSocial, spec.SeekAt-1)

	w.finishTalk(lonely, content)
	if got := memoriesOf(lonely, "socialized"); got != 1 {
		t.Errorf("lonely partner socialized = %d, want 1", got)
	}
	if got := memoriesOf(content, "socialized"); got != 0 {
		t.Errorf("content partner socialized = %d, want 0", got)
	}
}

// A sleep drive at its ceiling drops the colonist where it stands. It lies
// there while the drive falls at the unconscious rate, and comes to, still
// tired, once it is below critical-at, with the memory of it.
func TestSleepDeprivedColonistPassesOut(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)
	at := Point{w.Width / 2, w.Height / 2}
	w.SetTerrain(at, Floor)
	c := w.spawn(Colonist, at)
	for n := DriveKind(0); n < numDrives; n++ {
		w.setDrive(c, n, 0)
	}
	spec := cfg.Drives[DriveSleep]
	w.setDrive(c, DriveSleep, spec.Max)

	w.step()
	if c.State != PassedOut || !c.passedOut {
		t.Fatalf("state = %v, want passed out", c.State)
	}
	if got := memoriesOf(c, "passed-out"); got != 1 {
		t.Fatalf("passed-out memories = %d, want 1", got)
	}
	if c.drives[DriveSleep].rate >= 0 || c.driveActivity != DriveUnconscious {
		t.Fatalf("sleep rate %d in drive activity %v: it should fall while unconscious", c.drives[DriveSleep].rate, c.driveActivity)
	}
	fall := -spec.Rate * spec.Activity[DriveUnconscious.index()] / 100
	down := (spec.Max - spec.CriticalAt) * driveUnit / fall
	for i := 1; i < down; i++ {
		w.step()
		if c.State != PassedOut || c.Pos != at || c.Job != JobNone {
			t.Fatalf("tick %d: state %v job %v at %v; want lying where it fell", i, c.State, c.Job, c.Pos)
		}
	}
	for i := 0; i < 3 && c.passedOut; i++ {
		w.step()
	}
	if c.passedOut || c.State == PassedOut {
		t.Fatalf("still out after %d ticks with sleep at %d", down+3, w.driveLevel(c, DriveSleep))
	}
	if lvl := w.driveLevel(c, DriveSleep); lvl >= spec.CriticalAt || lvl < spec.SeekAt {
		t.Fatalf("sleep drive on coming to = %d, want just below critical-at %d: still tired", lvl, spec.CriticalAt)
	}
	if got := memoriesOf(c, "passed-out"); got != 1 {
		t.Fatalf("passed out again: %d memories", got)
	}
}

// A drive whose consequence is passing out must fall while unconscious, or
// the colonist would never come to.
func TestPassOutNeedsAFallingDrive(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Drives[DriveSleep].Activity[DriveUnconscious.index()] = 0
	if err := cfg.CheckDrives(); err == nil {
		t.Fatal("a pass-out drive that never falls was accepted")
	}
}

// A colonist already asleep beside its bed when the drive reaches the ceiling
// finishes its sleep there rather than passing out.
func TestAsleepInBedDoesNotPassOut(t *testing.T) {
	w := roomsTestWorld(20, 20)
	bed, stand := Point{6, 5}, Point{5, 5}
	carve(w, stand, stand, Floor)
	w.SetTerrain(bed, Bed)
	c := w.spawn(Colonist, stand)
	w.setDrive(c, DriveSleep, w.cfg.Drives[DriveSleep].Max)
	c.Job, c.Drive, c.useFacility, c.useFacilitySet = JobUse, DriveSleep, bed, true

	w.applyDriveConsequences(c)
	if c.passedOut {
		t.Fatal("passed out while asleep in bed")
	}
	c.Job = JobNone
	w.applyDriveConsequences(c)
	if !c.passedOut {
		t.Fatal("did not pass out once out of bed")
	}
}

// A bladder at its ceiling empties where the colonist stands: the drive resets
// and the colonist remembers the embarrassment. Anyone close enough sees it,
// and a Tidy witness minds more.
func TestFullBladderWetsSelfAndIsSeen(t *testing.T) {
	w := roomsTestWorld(30, 20)
	carve(w, Point{5, 5}, Point{20, 5}, Floor)
	c := w.spawn(Colonist, Point{5, 5})
	near := w.spawn(Colonist, Point{6, 5})
	tidy := w.spawn(Colonist, Point{7, 5})
	near.Profile.Traits, tidy.Profile.Traits = nil, []Trait{TraitTidy}
	far := w.spawn(Colonist, Point{5 + w.cfg.GoreSightRadius + 5, 5})
	w.setDrive(c, DriveBladder, w.cfg.Drives[DriveBladder].Max)
	grip := c.affect.Grip

	w.applyDriveConsequences(c)
	if lvl := w.driveLevel(c, DriveBladder); lvl != 0 {
		t.Fatalf("bladder after wetting self = %d, want 0", lvl)
	}
	if got := memoriesOf(c, "soiled-self"); got != 1 {
		t.Fatalf("soiled-self memories = %d, want 1", got)
	}
	if c.affect.Grip >= grip {
		t.Fatalf("embarrassment did not lower grip: %d -> %d", grip, c.affect.Grip)
	}
	if memoriesOf(near, "witnessed-soiling") != 1 || memoriesOf(tidy, "witnessed-soiling") != 1 {
		t.Fatalf("nearby colonists did not see it: %d, %d",
			memoriesOf(near, "witnessed-soiling"), memoriesOf(tidy, "witnessed-soiling"))
	}
	if memoriesOf(far, "witnessed-soiling") != 0 {
		t.Fatal("a colonist out of sight saw it")
	}
	if tidy.affect.Valence >= near.affect.Valence {
		t.Fatalf("Tidy witness valence %d not below plain witness %d", tidy.affect.Valence, near.affect.Valence)
	}

	// Once is once: the drive is met, so nothing more until it fills again.
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "soiled-self"); got != 1 {
		t.Fatalf("wet self again with an empty bladder: %d", got)
	}
}

// A colonist already at the toilet when the bladder tops out is using it.
func TestAtToiletDoesNotWetSelf(t *testing.T) {
	w := roomsTestWorld(20, 20)
	toilet, stand := Point{6, 5}, Point{5, 5}
	carve(w, stand, stand, Floor)
	w.SetTerrain(toilet, Toilet)
	c := w.spawn(Colonist, stand)
	w.setDrive(c, DriveBladder, w.cfg.Drives[DriveBladder].Max)
	c.Job, c.Drive, c.useFacility, c.useFacilitySet = JobUse, DriveBladder, toilet, true

	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "soiled-self"); got != 0 {
		t.Fatal("wet self while at the toilet")
	}
}

// Soiling's memories and log line use the colonist's own pronouns.
func TestWetSelfUsesPronouns(t *testing.T) {
	for _, tc := range []struct {
		g    Gender
		want string
	}{{GenderMan, "himself"}, {GenderWoman, "herself"}, {GenderNonbinary, "themself"}} {
		w := roomsTestWorld(20, 20)
		carve(w, Point{5, 5}, Point{6, 5}, Floor)
		c := w.spawn(Colonist, Point{5, 5})
		seer := w.spawn(Colonist, Point{6, 5})
		c.Profile.Gender = tc.g
		w.wetSelf(c, DriveBladder)
		if got, want := c.Memories[len(c.Memories)-1].Text, "Wet "+tc.want+"."; got != want {
			t.Errorf("%v: memory %q, want %q", tc.g, got, want)
		}
		if got, want := seer.Memories[len(seer.Memories)-1].Text, "Saw "+c.displayName()+" wet "+tc.want+"."; got != want {
			t.Errorf("%v: witness memory %q, want %q", tc.g, got, want)
		}
	}
}

// A consequence need not wait for the ceiling: loneliness declared from
// critical up is felt there, and leaving that band and coming back is a new
// stay that is felt at once.
func TestExperienceBelowTheCeiling(t *testing.T) {
	w, c := driveWorld(t, func(cfg *Config) {
		s := &cfg.Drives[DriveSocial]
		s.Consequences = []DriveConsequence{{From: s.CriticalAt, To: DriveCeiling, Kind: ConsequenceLoneliness}}
	})
	spec := w.cfg.Drives[DriveSocial]
	setDriveRate(w, c, DriveSocial, 0)
	w.setDrive(c, DriveSocial, spec.CriticalAt)
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 1 {
		t.Fatalf("felt-lonely at critical = %d, want 1", got)
	}
	w.setDrive(c, DriveSocial, spec.CriticalAt-1)
	w.setDrive(c, DriveSocial, spec.CriticalAt)
	w.tick++
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 2 {
		t.Fatalf("felt-lonely after leaving and re-entering the band = %d, want 2", got)
	}
}

// A colonist starved for company, with nobody it could talk to, goes to bed
// rather than standing in the socialize focus until it passes out. Its only
// companion here is asleep, so unavailable. Before socialize stepped aside
// (companyInReach), social pinned at its ceiling outranked sleep, the
// colonist waited for a partner who never came, and it passed out with a bed
// in reach: most of the default game's pass-outs were exactly this.
func TestLonelyColonistWithNobodyToTalkToGoesToBed(t *testing.T) {
	w, lonely, sleeper := lonelyBedroom(t, Point{4, 7})
	sleeper.Job, sleeper.Drive = JobUse, DriveSleep
	sleeper.useFacility, sleeper.useFacilitySet = Point{3, 7}, true
	sleeper.focus = FocusSleep
	w.setDrive(sleeper, DriveSleep, w.cfg.Drives[DriveSleep].Max-50)

	for i := 0; i < 40 && !w.usingFacility(lonely, DriveSleep); i++ {
		w.step()
		if lonely.passedOut {
			t.Fatalf("passed out on tick %d in the %s focus", i, lonely.focus)
		}
	}
	if !w.usingFacility(lonely, DriveSleep) {
		t.Fatalf("not in bed after 40 ticks: focus %s, state %v, social %d, sleep %d",
			lonely.focus, lonely.State, w.driveLevel(lonely, DriveSocial), w.driveLevel(lonely, DriveSleep))
	}
	if !w.usingFacility(sleeper, DriveSleep) {
		t.Fatal("the sleeper was pulled out of bed")
	}
}

// The other half: with someone free to talk to in reach, the same colonist
// still puts company first. Stepping aside is for when nobody could answer.
func TestLonelyColonistWithSomeoneToTalkToSocializes(t *testing.T) {
	w, lonely, other := lonelyBedroom(t, Point{6, 4})
	if !w.availableToTalk(other) {
		t.Fatal("setup: the other colonist should be free to talk")
	}
	w.step()
	if lonely.focus != FocusSocialize || lonely.Job != JobTalk || lonely.partner != other.ID {
		t.Fatalf("focus %s, job %v, partner #%d: want a talk with #%d",
			lonely.focus, lonely.Job, lonely.partner, other.ID)
	}
}

// lonelyBedroom is one room with two beds and two colonists, the second at
// otherAt, two tiles from the first and well within talk-radius. The first is
// already standing in the socialize focus with social at its ceiling and
// sleep pressing. The second has every drive empty.
func lonelyBedroom(t *testing.T, otherAt Point) (w *World, lonely, other *Entity) {
	t.Helper()
	w = roomsTestWorld(20, 12)
	carve(w, Point{2, 2}, Point{15, 8}, Floor)
	w.SetTerrain(Point{12, 3}, Bed)
	w.SetTerrain(Point{3, 7}, Bed)
	w.refreshSpatial()
	lonely = w.spawn(Colonist, Point{4, 5})
	other = w.spawn(Colonist, otherAt)
	for _, e := range []*Entity{lonely, other} {
		for d := DriveKind(0); d < numDrives; d++ {
			w.setDrive(e, d, 0)
		}
	}
	social, sleep := w.cfg.Drives[DriveSocial], w.cfg.Drives[DriveSleep]
	w.setDrive(lonely, DriveSocial, social.Max)
	w.setDrive(lonely, DriveSleep, sleep.SeekAt+100)
	lonely.focus = FocusSocialize
	if !w.facilityReachable(lonely, Bed) {
		t.Fatal("setup: the bed should be in reach")
	}
	return w, lonely, other
}

// With a hall, company out of talk-radius still counts if it is headed there:
// another colonist already socializing, or one at work whose own social drive
// is critical, who will be. Without the second, two lonely colonists at work
// would each wait for the other to go first and neither ever would.
func TestHallCompanyIncludesLonelyWorkers(t *testing.T) {
	w, lonely, other := lonelyBedroom(t, Point{14, 3})
	w.SetTerrain(Point{14, 8}, Chair)
	w.refreshSpatial()
	if !w.hallOpen() || lonely.Pos.Chebyshev(other.Pos) <= w.cfg.TalkRadius {
		t.Fatal("setup: want an open hall and the other colonist out of talk-radius")
	}
	other.focus, other.Job = FocusWork, JobMine
	if w.companyInReach(lonely) {
		t.Fatal("a content colonist at work counted as company")
	}
	w.setDrive(other, DriveSocial, w.cfg.Drives[DriveSocial].Max)
	w.syncDrivePhase(other, DriveSocial)
	if !w.companyInReach(lonely) {
		t.Fatal("a lonely colonist at work did not count as headed for the hall")
	}
	other.focus = FocusSleep
	if w.companyInReach(lonely) {
		t.Fatal("a lonely colonist asleep counted as headed for the hall")
	}
}
