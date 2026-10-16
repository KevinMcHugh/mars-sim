package sim

import (
	"strings"
	"testing"
)

// safetyWorld is zoneWorld's open chamber with nobody in it: unzoned floor on
// the landing level, so anyone standing there is on the frontier.
func safetyWorld(t *testing.T) *World {
	t.Helper()
	return defenseWorld(t)
}

func safetyRate(w *World, e *Entity) int {
	w.syncDriveActivity(e)
	return e.drives[DriveSafety].rate
}

// Where a colonist stands decides how fast it comes to feel unsafe: it builds
// on the frontier (faster deeper down), barely falls in the settled colony,
// and falls fast at home and in a staffed defense zone. A weapon slows it.
func TestSafetyRateFollowsThePlace(t *testing.T) {
	w := safetyWorld(t)
	e := w.spawn(Colonist, Point{10, 10, LandingLevel})
	base := w.cfg.Drives[DriveSafety].Rate // testConfig keeps idle at 100%

	if got := safetyRate(w, e); e.place != PlaceFrontier || got != base*200/100 {
		t.Fatalf("frontier: place %v rate %d, want frontier %d", e.place, got, base*2)
	}

	e.Inventory.Add(Pistol, 1)
	if got := safetyRate(w, e); got != base*200/100*75/100 {
		t.Fatalf("armed with a pistol: rate %d, want %d", got, base*200/100*75/100)
	}
	e.Inventory.Remove(Pistol, 1)

	w.setZone(e.Pos, ZoneResidence)
	if got := safetyRate(w, e); e.place != PlaceSettled || got != base*-50/100 {
		t.Fatalf("settled: place %v rate %d, want %d", e.place, got, base*-50/100)
	}

	s := w.registerStructure(ZoneResidence, []Point{{8, 8, LandingLevel}}, nil)
	s.room = &roomRecord{lo: Point{9, 9, LandingLevel}, hi: Point{12, 12, LandingLevel}, issuer: ColonistOwner(e.ID)}
	w.structureRev++
	if got := safetyRate(w, e); e.place != PlaceHome || got != base*-300/100 {
		t.Fatalf("home: place %v rate %d, want %d", e.place, got, base*-300/100)
	}
	w.setZone(e.Pos, NoZone)
	w.forget(s)

	if !w.orderDefense(OrderDefense{X0: 8, Y0: 8, X1: 14, Y1: 14, Detail: 1}) {
		t.Fatal("defense order refused")
	}
	guard := w.spawn(Colonist, Point{12, 12, LandingLevel})
	w.defensePosts[0].guards = []EntityID{guard.ID}
	if got := safetyRate(w, e); e.place != PlaceGuarded || got != base*-200/100 {
		t.Fatalf("guarded: place %v rate %d, want %d", e.place, got, base*-200/100)
	}
	if guard.place = w.placeOf(guard); guard.place == PlaceGuarded {
		t.Fatal("a guard alone on station counts as guarded by itself")
	}

	deep := &Entity{Kind: Colonist, Pos: Point{1, 1, LandingLevel + 2}, place: PlaceFrontier}
	if got, want := w.placePercent(deep, DriveSafety), 200+2*w.cfg.Drives[DriveSafety].PlaceDepth; got != want {
		t.Fatalf("two levels down: %d%%, want %d%%", got, want)
	}
	if got := w.placePercent(deep, DriveFood); got != 100 {
		t.Fatalf("food cares where the colonist is: %d%%", got)
	}
}

// Safety is felt, not sought: however unsafe a colonist is, the drive is
// never the most urgent one and puts no focus forward.
func TestSafetyIsFeltNotSought(t *testing.T) {
	w := safetyWorld(t)
	e := w.spawn(Colonist, Point{10, 10, LandingLevel})
	for d := DriveKind(0); d < numDrives; d++ {
		w.setDrive(e, d, 0)
	}
	w.setDrive(e, DriveSafety, 900)
	if e.drives[DriveSafety].phase != DriveCritical {
		t.Fatalf("safety at 900 is %v", e.drives[DriveSafety].phase)
	}
	if d, urgent := w.mostUrgentDrive(e); urgent {
		t.Fatalf("most urgent drive is %v", d)
	}
	var out [numFocusKinds]FocusCandidate
	w.focusCandidates(e, &out)
	for _, c := range out {
		if c.Eligible && c.Drive == DriveSafety && c.Score.Drive > 0 {
			t.Fatalf("safety put forward %v", c)
		}
	}
	if !out[FocusIdle].Eligible {
		t.Fatal("idle is not eligible")
	}
}

// At its ceiling the colonist feels unsafe, a cognition row.
func TestSafetyCeilingFeelsUnsafe(t *testing.T) {
	w := safetyWorld(t)
	e := w.spawn(Colonist, Point{10, 10, LandingLevel})
	w.setDrive(e, DriveSafety, w.cfg.Drives[DriveSafety].Max)
	w.applyDriveConsequences(e)
	if n := len(e.Memories); n == 0 || e.Memories[n-1].Rule != "felt-unsafe" {
		t.Fatalf("memories %+v, want felt-unsafe last", e.Memories)
	}
}

func killingSeen(w *World, observer *Entity, victim EntityID) Percept {
	return w.perceptFor(observer, ChannelSight, RoleWitness, PhaseInstant, Occurrence{
		Actor: FactRef{Noun: NounAlien, Entity: 9000, Label: "a grelk"}, Action: ActionKill,
		Object: FactRef{Noun: NounColonist, Entity: victim, Label: "Cy"}, Text: "Saw a grelk kill Cy.",
	})
}

func safetyLevel(w *World, e *Entity) int { return w.driveLevel(e, DriveSafety) }

// Seeing a killing bumps safety by the reaction's drives row, and a Bold
// colonist takes it at 60%.
func TestWitnessingAKillingMakesAColonistUnsafe(t *testing.T) {
	w := safetyWorld(t)
	plain := w.spawn(Colonist, Point{10, 10, LandingLevel})
	bold := w.spawn(Colonist, Point{11, 10, LandingLevel})
	bold.Profile.Traits = append(bold.Profile.Traits, TraitBold)
	w.resolveTraitEffects(bold)
	for _, e := range []*Entity{plain, bold} {
		w.setDrive(e, DriveSafety, 0)
		w.rememberPercept(e, killingSeen(w, e, 98))
	}
	if got := safetyLevel(w, plain); got != 400 {
		t.Fatalf("witness safety = %d, want 400", got)
	}
	if got := safetyLevel(w, bold); got != 240 {
		t.Fatalf("bold witness safety = %d, want 240", got)
	}
}

// talkedWith stands in for a finished conversation: the listener's memory of
// it, then the speaker's memory handed over as hearsay.
func talkedWith(w *World, speaker, listener *Entity, m Memory) {
	listener.Memories = append(listener.Memories, Memory{Tick: w.tick, LastTick: w.tick, Count: 1, Rule: conversationRule,
		Text: "Had a conversation with " + speaker.displayName() + "."})
	w.tellHearsay(ConversationTopic{Kind: TopicMemory, Speaker: speaker.ID, Memory: m}, listener)
}

// A recounted killing reaches the listener as hearsay; the listener can pass
// it on, half as hard, and no further than hearsay-max-hops. Someone who saw
// it is not told again.
func TestHearsayCarriesAKillingTwoHops(t *testing.T) {
	w := safetyWorld(t)
	ada := w.spawn(Colonist, Point{10, 10, LandingLevel})
	bo := w.spawn(Colonist, Point{11, 10, LandingLevel})
	cy := w.spawn(Colonist, Point{12, 10, LandingLevel})
	di := w.spawn(Colonist, Point{13, 10, LandingLevel})
	for _, e := range []*Entity{ada, bo, cy, di} {
		w.setDrive(e, DriveSafety, 0)
	}
	w.rememberPercept(ada, killingSeen(w, ada, 98))
	seen := ada.Memories[len(ada.Memories)-1]
	if seen.Gist == nil || seen.Gist.Hops != 0 {
		t.Fatalf("witness memory gist %+v", seen.Gist)
	}

	talkedWith(w, ada, bo, seen)
	if got := safetyLevel(w, bo); got != 150 {
		t.Fatalf("first-hand hearsay: bo safety %d, want 150", got)
	}
	heard := bo.Memories[len(bo.Memories)-1]
	if heard.Gist == nil || heard.Gist.Hops != 1 {
		t.Fatalf("bo's conversation memory gist %+v, want hops 1", heard.Gist)
	}
	retell := w.recountableMemories(bo)
	if len(retell) != 1 || retell[0].Text != seen.Text {
		t.Fatalf("bo would retell %+v, want %q", retell, seen.Text)
	}

	talkedWith(w, bo, cy, retell[0])
	if got := safetyLevel(w, cy); got != 75 {
		t.Fatalf("second-hand hearsay: cy safety %d, want 75", got)
	}
	if retell := w.recountableMemories(cy); len(retell) != 0 {
		t.Fatalf("news retold past hearsay-max-hops: %+v", retell)
	}

	w.setDrive(ada, DriveSafety, 0)
	talkedWith(w, bo, ada, retell[0])
	if got := safetyLevel(w, ada); got != 0 {
		t.Fatalf("a witness was told what it saw: safety %d", got)
	}
	if got := safetyLevel(w, di); got != 0 {
		t.Fatalf("di heard nothing, safety %d", got)
	}
}

// newsTick is the first tick of news-hour on the day after tick.
func newsTick(w *World, tick int) int {
	for t := tick + 1; ; t++ {
		if w.newsHourAt(t) == w.cfg.NewsHour && w.newsHourAt(t-1) != w.cfg.NewsHour {
			return t
		}
	}
}

// A death is reported, aired at news-hour, and heard by everyone awake who did
// not already know; a sleeper hears it on waking, and a newcomer never hears
// of its own arrival.
func TestTheNightlyNewsReportsDeathsAndArrivals(t *testing.T) {
	w := safetyWorld(t)
	listener := w.spawn(Colonist, Point{10, 10, LandingLevel})
	witness := w.spawn(Colonist, Point{11, 10, LandingLevel})
	sleeper := w.spawn(Colonist, Point{12, 10, LandingLevel})
	victim := w.spawn(Colonist, Point{20, 10, LandingLevel})
	for _, e := range []*Entity{listener, witness, sleeper} {
		w.setDrive(e, DriveSafety, 0)
	}
	w.rememberPercept(witness, killingSeen(w, witness, victim.ID))
	w.remove(victim.ID, "bitten to death by a grelk")
	if len(w.news) != 1 || !strings.Contains(w.news[0].Text, "is dead") {
		t.Fatalf("news = %+v", w.news)
	}

	w.hearNews(listener)
	if safetyLevel(w, listener) != 0 {
		t.Fatal("news heard before it aired")
	}
	w.tick = newsTick(w, w.tick)
	w.airNews()
	if w.newsAired != 1 {
		t.Fatalf("aired %d items", w.newsAired)
	}
	for _, e := range []*Entity{listener, witness, sleeper} {
		w.setDrive(e, DriveSafety, 0)
		e.drives[DriveSafety].rate = 0 // hold still: only the news moves it
	}
	sleeper.State = Sleeping
	for _, e := range []*Entity{listener, witness, sleeper} {
		w.hearNews(e)
	}
	if m := listener.Memories[len(listener.Memories)-1]; m.Rule != "news-colonist-died" ||
		!strings.HasPrefix(m.Text, "Heard on the evening news: ") {
		t.Fatalf("listener memory %+v", m)
	}
	if got := safetyLevel(w, listener); got != 120 {
		t.Fatalf("listener safety %d, want 120", got)
	}
	if got := safetyLevel(w, witness); got != 0 {
		t.Fatalf("the witness heard of a death it saw: safety %d", got)
	}
	if safetyLevel(w, sleeper) != 0 {
		t.Fatal("a sleeper heard the news")
	}
	sleeper.State = Idle
	w.hearNews(sleeper)
	if got := safetyLevel(w, sleeper); got != 120 {
		t.Fatalf("sleeper on waking: safety %d, want 120", got)
	}

	recruit := w.spawn(Colonist, Point{14, 10, LandingLevel})
	w.setDrive(listener, DriveSafety, 500)
	w.setDrive(recruit, DriveSafety, 500)
	w.reportRecruits([]EntityID{recruit.ID}, []string{recruit.displayName()})
	w.tick = newsTick(w, w.tick)
	w.airNews()
	for _, e := range []*Entity{listener, recruit} {
		w.setDrive(e, DriveSafety, 500)
		e.drives[DriveSafety].rate = 0
	}
	w.hearNews(listener)

	w.hearNews(recruit)
	if got := safetyLevel(w, listener); got != 470 {
		t.Fatalf("an arrival heard: safety %d, want 470", got)
	}
	if got := safetyLevel(w, recruit); got != 500 {
		t.Fatalf("a recruit heard of its own arrival: safety %d", got)
	}
}

// A reaction's drives row round-trips through the cognition file.
func TestReactionDrivesRoundTripThroughYAML(t *testing.T) {
	def := DefaultCognitionConfig()
	var cfg CognitionConfig
	cfg = DefaultCognitionConfig()
	if err := ApplyCognitionYAML(&cfg, CognitionConfigTemplate()); err != nil {
		t.Fatal(err)
	}
	for i, r := range def.Reactions {
		got := cfg.Reactions[i]
		if len(got.Drives) != len(r.Drives) {
			t.Fatalf("%s drives %+v, want %+v", r.ID, got.Drives, r.Drives)
		}
		for j := range r.Drives {
			if got.Drives[j] != r.Drives[j] {
				t.Fatalf("%s drives %+v, want %+v", r.ID, got.Drives, r.Drives)
			}
		}
	}
	override := []byte("schema_version: 1\nreactions:\n  - id: saw-alien\n    drives: { safety: { fresh: 0, worn: 0 } }\n")
	if err := ApplyCognitionYAML(&cfg, override); err != nil {
		t.Fatal(err)
	}
	for _, r := range cfg.Reactions {
		if r.ID == "saw-alien" && len(r.Drives) != 0 {
			t.Fatalf("a zeroed drive row stayed: %+v", r.Drives)
		}
	}
}

// The News tab's view groups aired items by broadcast, newest first, and is
// shared by every snapshot until the next broadcast.
func TestPublishedNewsGroupsBroadcasts(t *testing.T) {
	w := safetyWorld(t)
	w.reportNews(NewsItem{Action: ActionDie, Text: "first", Place: Point{1, 2, LandingLevel}, HasPlace: true})
	if len(w.publishedNewsBroadcasts()) != 0 {
		t.Fatal("unaired news was published")
	}
	w.tick = newsTick(w, w.tick)
	w.airNews()
	w.reportNews(NewsItem{Action: ActionArrive, Text: "second"})
	w.reportNews(NewsItem{Action: ActionArrive, Text: "third"})
	w.tick = newsTick(w, w.tick)
	w.airNews()
	got := w.publishedNewsBroadcasts()
	if len(got) != 2 || len(got[0].Items) != 2 || got[0].Items[0].Text != "second" || got[0].Items[1].Kind != "arrival" ||
		len(got[1].Items) != 1 || got[1].Items[0].Kind != "death" || !got[1].Items[0].HasPlace {
		t.Fatalf("broadcasts %+v", got)
	}
	if again := w.publishedNewsBroadcasts(); &again[0] != &got[0] {
		t.Fatal("the view was rebuilt with nothing new aired")
	}
}
