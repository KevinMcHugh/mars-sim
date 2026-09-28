package sim

import (
	"bytes"
	"strings"
	"testing"
)

// Ranks come from an integer walk up the thresholds, so exact powers land on
// the rank they reach. A float log puts 1000 at base 10 on rank 2.
func TestRanksAreExactAtThresholds(t *testing.T) {
	s := skillSpec{Unit: 1, Base: 10, Labels: []string{"", "a", "b", "c", "d"}}
	for _, c := range []struct {
		practice uint32
		want     int
	}{{0, 0}, {1, 1}, {9, 1}, {10, 2}, {99, 2}, {100, 3}, {999, 3}, {1000, 4}, {1 << 31, 4}} {
		if got := s.rankAt(c.practice); got != c.want {
			t.Errorf("rankAt(%d) = %d, want %d", c.practice, got, c.want)
		}
	}
	for k := SkillKind(1); k < numSkills; k++ {
		spec := &skillSpecs[k]
		if len(spec.Effects) != len(spec.Labels) {
			t.Errorf("%s: %d effects for %d ranks", k, len(spec.Effects), len(spec.Labels))
		}
		for r := 1; r < len(spec.Labels); r++ {
			if got := spec.rankAt(spec.threshold(r)); got != r {
				t.Errorf("%s: practice at rank %d's threshold is rank %d", k, r, got)
			}
			if got := spec.rankAt(spec.threshold(r) - 1); got != r-1 {
				t.Errorf("%s: practice one short of rank %d is rank %d", k, r, got)
			}
		}
	}
}

// Practice is credited in base ticks, a new title is a memory, and the same
// title again is not.
func TestPractiseRecordsNewTitles(t *testing.T) {
	w := propertyWorld(t)
	e := w.spawn(Colonist, Point{10, 7})
	w.practise(e, SkillCooking, 119)
	if e.rank(SkillCooking) != 0 || len(e.Memories) != 0 {
		t.Fatalf("119 ticks of cooking: rank %d, %d memories; want rank 0 and none", e.rank(SkillCooking), len(e.Memories))
	}
	w.practise(e, SkillCooking, 1)
	if e.rank(SkillCooking) != 1 || e.profession != SkillCooking {
		t.Fatalf("120 ticks of cooking: rank %d, profession %s; want rank 1, cooking", e.rank(SkillCooking), e.profession)
	}
	if n := len(e.Memories); n != 1 || e.Memories[0].Text != "Became a kitchen hand." {
		t.Fatalf("memories after the first rank: %+v", e.Memories)
	}
	w.practise(e, SkillCooking, 100)
	if len(e.Memories) != 1 {
		t.Fatalf("practice within a rank added a memory: %+v", e.Memories)
	}
}

// A colonist's trade is the skill it stands highest in, as a share of each
// skill's top rank, not the one with the biggest rank number: mining has
// eight ranks to cooking's four. It keeps its trade until another stands
// higher even a rank down.
func TestProfessionComparesStandingWithHysteresis(t *testing.T) {
	w := propertyWorld(t)
	e := w.spawn(Colonist, Point{10, 7})
	w.practise(e, SkillCooking, int(skillSpecs[SkillCooking].threshold(2))) // cook: 2 of 4
	w.practise(e, SkillMining, int(skillSpecs[SkillMining].threshold(3)))   // 3 of 8
	if e.profession != SkillCooking {
		t.Fatalf("a cook with a little mining: profession %s, want cooking", e.profession)
	}
	w.setRank(e, SkillMining, 4) // 4 of 8: level with 2 of 4, not ahead
	w.updateProfession(e)
	if e.profession != SkillCooking {
		t.Fatalf("mining level with cooking: profession %s, want cooking kept", e.profession)
	}
	w.practise(e, SkillMining, int(skillSpecs[SkillMining].threshold(5))-int(e.practice[SkillMining])) // 5 of 8: a rank ahead
	if e.profession != SkillMining {
		t.Fatalf("mining ahead by a rank: profession %s, want mining", e.profession)
	}
	if last := e.Memories[len(e.Memories)-1].Text; last != "Took up mining as a trade." {
		t.Fatalf("last memory %q; want the change of trade", last)
	}
}

// Skill cuts work ticks by the rank's TicksPct, before traits; with skills
// off it changes nothing.
func TestSkillSpeedsUpWork(t *testing.T) {
	w := propertyWorld(t)
	e := w.spawn(Colonist, Point{10, 7})
	base := w.cfg.MineTicks
	if got := w.workTicks(e, SkillMining, base); got != base {
		t.Fatalf("untrained miner: %d ticks, want %d", got, base)
	}
	w.setRank(e, SkillMining, -1)
	want := (base*skillSpecs[SkillMining].Effects[e.rank(SkillMining)].TicksPct + 50) / 100
	if got := w.workTicks(e, SkillMining, base); got != want || got >= base {
		t.Fatalf("master miner: %d ticks, want %d (fewer than %d)", got, want, base)
	}
	w.cfg.Skills = false
	if got := w.workTicks(e, SkillMining, base); got != base {
		t.Fatalf("skills off: %d ticks, want %d", got, base)
	}
}

// Yield above 100% is counted: a master smith at 140% makes two extra units
// in five runs, and none while the depot has no room.
func TestYieldIsCountedNotRolled(t *testing.T) {
	w := propertyWorld(t)
	e := w.spawn(Colonist, Point{10, 7})
	w.setRank(e, SkillSmithing, -1)
	room := func() bool { return true }
	extra := 0
	for i := 0; i < 5; i++ {
		if w.skillYield(e, SkillSmithing, room) {
			extra++
		}
	}
	if extra != 2 {
		t.Fatalf("master smith: %d extra units in 5 runs, want 2", extra)
	}
	full := func() bool { return false }
	for i := 0; i < 5; i++ {
		if w.skillYield(e, SkillSmithing, full) {
			t.Fatal("paid out an extra unit with no room for it")
		}
	}
	if !w.skillYield(e, SkillSmithing, room) {
		t.Fatal("the yield held back for room should pay out once there is room")
	}
}

// A master chef cooking the colony's scum makes more meals than the recipes
// say, and is credited the recipes' base ticks.
func TestAMasterChefCooksMore(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.InfiniteFood, w.cfg.MealReserve = false, 100
	house := Point{10, 6}
	w.SetTerrain(house, Scumhouse)
	w.refreshSpatial()
	c := w.storageContainers[house]
	c.Inventory.Add(CaveScum, 20)
	c.credit(Community, CaveScum, 20)
	cook := w.spawn(Colonist, Point{10, 7})
	w.setRank(cook, SkillCooking, -1)
	before := cook.practice[SkillCooking]
	if !w.tryAssignCraft(cook) {
		t.Fatal("no colony cooking job")
	}
	for i := 0; i < 1000 && cook.Job == JobCraft; i++ {
		w.jobCraft(cook)
	}
	out := w.storageContainers[w.outputDepot(house)]
	meals := out.held(Community, Meal) + w.openQty(Ask, Meal, out.Pos, Community)
	// It cooks all 20 scum, 10 recipes of 2 scum for 1 meal; at 130% the
	// accumulator passes 100 three times.
	if meals != 13 {
		t.Fatalf("a master chef made %d meals from 20 scum, want 13", meals)
	}
	if got := cook.practice[SkillCooking] - before; got != 10*12 {
		t.Fatalf("credited %d ticks of cooking, want %d", got, 10*12)
	}
}

// Backgrounds come from their own stream: rolling them shifts no other
// stream, so everything else about a new world is as it would be without
// skills, and somebody arrives knowing something.
func TestBackgroundsDrawFromTheirOwnStream(t *testing.T) {
	on := DefaultConfig()
	on.Seed = 11
	off := on
	off.Skills = false
	a, b := NewEngine(on).world, NewEngine(off).world
	sa, _ := a.rngSrc.sim.MarshalBinary()
	sb, _ := b.rngSrc.sim.MarshalBinary()
	pa, _ := a.rngSrc.personality.MarshalBinary()
	pb, _ := b.rngSrc.personality.MarshalBinary()
	if !bytes.Equal(sa, sb) || !bytes.Equal(pa, pb) {
		t.Fatal("rolling backgrounds moved the simulation or personality stream")
	}
	if goldenHash(a) != goldenHash(b) {
		t.Fatal("rolling backgrounds changed the generated world")
	}
	skilled := 0
	for _, e := range a.entities {
		if e.Kind != Colonist {
			continue
		}
		if len(e.skillViews()) > 0 {
			skilled++
		}
		if be := b.entities[e.ID]; be == nil || len(be.skillViews()) != 0 {
			t.Fatalf("with skills off, colonist %d arrived with a background", e.ID)
		}
	}
	if skilled == 0 {
		t.Fatal("no colonist arrived with a background")
	}
}

// The roster reads skills from the snapshot: every ranked skill, and the
// trade the colonist is known for.
func TestSkillsReachTheSnapshot(t *testing.T) {
	w := propertyWorld(t)
	e := w.spawn(Colonist, Point{10, 7})
	w.setRank(e, SkillSmithing, 2)
	w.updateProfession(e)
	v := w.entityView(e, nil, false)
	if len(v.Skills) != 1 || v.Skills[0].Skill != SkillSmithing || v.Skills[0].Label != "journeyman smith" {
		t.Fatalf("skills in the view: %+v", v.Skills)
	}
	if v.Profession != SkillSmithing || !strings.Contains(v.ProfessionLabel, "smith") {
		t.Fatalf("profession in the view: %s %q", v.Profession, v.ProfessionLabel)
	}
}
