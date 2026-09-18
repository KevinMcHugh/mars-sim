package sim

import (
	"reflect"
	"testing"
)

func setUpCachedSleeper(t *testing.T, w *World, c *Entity) Point {
	t.Helper()
	bed := c.Pos.Add(1, 0)
	if !w.InBounds(bed) || w.entityAt(bed) != nil {
		bed = c.Pos.Add(-1, 0)
	}
	if !w.InBounds(bed) || w.entityAt(bed) != nil {
		t.Fatal("no adjacent bed site")
	}
	w.SetTerrain(c.Pos, Floor)
	w.SetTerrain(bed, Bed)
	c.Needs[NeedSleep] = w.cfg.Needs[NeedSleep].SeekAt
	c.needSince[NeedSleep] = w.tick
	w.syncNeedPhase(c, NeedSleep)
	c.focus, c.Job, c.Need, c.State = FocusSleep, JobUse, NeedSleep, Sleeping
	c.useFacility, c.useFacilitySet = bed, true
	c.resting = false
	c.mindDirty = false
	c.nextThinkTick = w.nextCognitionTick(c)
	return bed
}

func assertBehaviorEqual(t *testing.T, cached, full *World, tick int) {
	t.Helper()
	if cached.tick != full.tick || cached.terrainCounts != full.terrainCounts ||
		cached.kindCounts != full.kindCounts || !reflect.DeepEqual(cached.tiles, full.tiles) {
		t.Fatalf("world behavior diverged at tick %d", tick)
	}
	cachedIDs, fullIDs := cached.entityIDsSorted(), full.entityIDsSorted()
	if !reflect.DeepEqual(cachedIDs, fullIDs) {
		t.Fatalf("entity IDs diverged at tick %d: %v != %v", tick, cachedIDs, fullIDs)
	}
	for _, id := range cachedIDs {
		a, b := cached.entities[id], full.entities[id]
		if a.Pos != b.Pos || a.HP != b.HP || a.State != b.State || a.focus != b.focus ||
			a.Job != b.Job || a.Target != b.Target || a.BuildKind != b.BuildKind ||
			a.Need != b.Need || a.Progress != b.Progress || a.Inventory != b.Inventory ||
			a.affect != b.affect || !reflect.DeepEqual(a.Memories, b.Memories) {
			t.Fatalf("entity %d behavior diverged at tick %d:\n cached=%+v\n full=%+v", id, tick, a, b)
		}
		for n := NeedKind(0); n < numNeeds; n++ {
			if al, bl := cached.needLevel(a, n), full.needLevel(b, n); al != bl {
				t.Fatalf("entity %d need %s diverged at tick %d: %d != %d", id, n, tick, al, bl)
			}
		}
	}
}

func TestCachedCognitionMatchesAlwaysArbitrate(t *testing.T) {
	for _, seed := range []int64{1, 7, 41} {
		cfg := testConfig()
		cfg.Seed = seed
		cached := newTestWorld(t, cfg)
		full := newTestWorld(t, cfg)
		full.alwaysArbitrate = true
		for tick := 1; tick <= 180; tick++ {
			cached.step()
			full.step()
			assertBehaviorEqual(t, cached, full, tick)
		}
	}
}

func TestSleepingFastPathInterruptedByThreat(t *testing.T) {
	w, c := focusTestColonist(t)
	setUpCachedSleeper(t, w, c)
	alien := w.spawn(Alien, c.Pos.Add(0, 1))

	w.step()
	if c.focus != FocusFlee {
		t.Fatalf("focus after threat appeared = %v, want flee", c.focus)
	}
	if c.Job == JobUse {
		t.Fatal("sleep facility job survived a live-threat interruption")
	}
	if w.entities[c.ID] == nil || w.entities[alien.ID] == nil {
		t.Fatal("sleeper or threat stopped being targetable during interruption")
	}
}

func TestSleepingFastPathHonorsFatalNeedDeadline(t *testing.T) {
	w, c := focusTestColonist(t)
	setUpCachedSleeper(t, w, c)
	food := w.cfg.Needs[NeedFood]
	c.Needs[NeedFood] = food.SeekAt - 1
	c.needSince[NeedFood] = w.tick
	c.needRise[NeedFood] = 1
	w.syncNeedPhase(c, NeedFood)
	c.mindDirty = false
	c.nextThinkTick = w.nextCognitionTick(c)
	hp := c.HP

	w.step()
	if c.focus != FocusEat {
		t.Fatalf("focus at fatal need boundary = %v, want eat", c.focus)
	}
	if c.HP != hp {
		t.Fatalf("fatal need deadline slipped: HP %d -> %d", hp, c.HP)
	}
}

func TestSleepingFastPathRejectsInvalidFacility(t *testing.T) {
	w, c := focusTestColonist(t)
	bed := setUpCachedSleeper(t, w, c)
	w.SetTerrain(bed, Floor)

	w.step()
	if c.Job != JobNone {
		t.Fatalf("invalid facility job = %v, want cleared", c.Job)
	}
	if !c.mindDirty {
		t.Fatal("facility invalidation did not request reconsideration")
	}
	w.step()
	if c.Job == JobUse && c.useFacility.Equal(bed) {
		t.Fatal("colonist reacquired stale facility target")
	}
}

func TestCachedRestSnapshotKeepsLazyNeedsCurrent(t *testing.T) {
	w, c := focusTestColonist(t)
	c.focus, c.Job, c.State = FocusIdle, JobNone, Idle
	c.resting, c.wakeTick = true, w.tick+100
	for n := NeedKind(0); n < numNeeds; n++ {
		w.syncNeedPhase(c, n)
	}
	c.mindDirty = false
	c.nextThinkTick = w.nextCognitionTick(c)
	before := w.entityView(c, nil, true).Needs[NeedFood]

	w.step()
	view := w.entityView(c, nil, true)
	if view.Needs[NeedFood] <= before {
		t.Fatalf("cached snapshot food = %d, want greater than %d", view.Needs[NeedFood], before)
	}
	if view.State != Idle || view.Focus != FocusIdle {
		t.Fatalf("cached snapshot state/focus = %v/%v", view.State, view.Focus)
	}
}
