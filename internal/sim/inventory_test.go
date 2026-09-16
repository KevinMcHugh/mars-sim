package sim

import "testing"

func TestInventoryStacksItemsAcrossSlots(t *testing.T) {
	var inv Inventory
	if !inv.Add(RawRock, MaxStackSize+1) {
		t.Fatal("expected raw rock to fit")
	}
	if got := inv[0]; got != (ItemStack{Kind: RawRock, Count: MaxStackSize}) {
		t.Fatalf("first slot = %+v", got)
	}
	if got := inv[1]; got != (ItemStack{Kind: RawRock, Count: 1}) {
		t.Fatalf("second slot = %+v", got)
	}
}

func TestInventoryRejectsOverflowWithoutChangingStacks(t *testing.T) {
	var inv Inventory
	if !inv.Add(RawRock, InventorySlotCount*MaxStackSize) {
		t.Fatal("expected inventory to accept its full capacity")
	}
	before := inv
	if inv.Add(RawRock, 1) {
		t.Fatal("expected full inventory to reject another item")
	}
	if inv != before {
		t.Fatal("failed Add changed the inventory")
	}
}

func TestMiningAwardsRawRock(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.MineTicks = 1
	w := newTestWorld(t, cfg)

	pos := Point{w.Width / 2, w.Height / 2}
	w.SetTerrain(pos, Floor)
	target := pos.Add(1, 0)
	w.SetTerrain(target, Rock)
	miner := w.spawn(Colonist, pos)
	miner.Job, miner.Target, miner.mineClaimed = JobMine, target, true

	w.jobMine(miner)

	if got := w.TerrainAt(target); got != Floor {
		t.Fatalf("mined terrain = %v, want Floor", got)
	}
	if got := miner.Inventory[0]; got != (ItemStack{Kind: RawRock, Count: 1}) {
		t.Fatalf("miner's first slot = %+v", got)
	}
}

func TestFullInventoryPreventsMiningResourceLoss(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.MineTicks = 1
	w := newTestWorld(t, cfg)

	pos := Point{w.Width / 2, w.Height / 2}
	w.SetTerrain(pos, Floor)
	target := pos.Add(1, 0)
	w.SetTerrain(target, Rock)
	miner := w.spawn(Colonist, pos)
	if !miner.Inventory.Add(RawRock, InventorySlotCount*MaxStackSize) {
		t.Fatal("failed to fill inventory")
	}
	miner.Job, miner.Target, miner.mineClaimed = JobMine, target, true

	w.jobMine(miner)

	if got := w.TerrainAt(target); got != Rock {
		t.Fatalf("full miner excavated terrain to %v", got)
	}
}

// The incinerator empties a hauler's whole load at once, so RemoveAll has to
// clear every stack of a kind (and only that kind) and report the total burned.
func TestInventoryRemoveAllClearsOneKind(t *testing.T) {
	var inv Inventory
	inv.Add(Viscera, MaxStackSize+3)
	inv.Add(RawRock, 5)

	if got := inv.Count(Viscera); got != MaxStackSize+3 {
		t.Fatalf("viscera count = %d, want %d", got, MaxStackSize+3)
	}
	if got := inv.RemoveAll(Viscera); got != MaxStackSize+3 {
		t.Fatalf("RemoveAll returned %d, want %d", got, MaxStackSize+3)
	}
	if got := inv.Count(Viscera); got != 0 {
		t.Errorf("viscera left after RemoveAll = %d, want 0", got)
	}
	if got := inv.Count(RawRock); got != 5 {
		t.Errorf("raw rock = %d, want 5 (untouched)", got)
	}
}
