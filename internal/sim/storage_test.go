package sim

import "testing"

func TestStorageInventoryHoldsSixColonistInventories(t *testing.T) {
	var storage StorageInventory
	fullColonistLoad := InventorySlotCount * MaxStackSize
	want := 6 * fullColonistLoad

	if StorageInventorySlotCount != 6*InventorySlotCount {
		t.Fatalf("storage slots = %d, want six inventories (%d)",
			StorageInventorySlotCount, 6*InventorySlotCount)
	}
	if !storage.Add(RawRock, want) {
		t.Fatalf("storage rejected its advertised capacity of %d items", want)
	}
	before := storage
	if storage.Add(RawRock, 1) {
		t.Fatal("storage accepted an item beyond its advertised capacity")
	}
	if storage != before {
		t.Fatal("failed over-capacity add changed storage")
	}
}

func TestStorageTerrainOwnsSparseContainerState(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 12, 12
	w := newTestWorld(t, cfg)
	p := Point{4, 5}

	w.SetTerrain(p, Floor)
	w.SetTerrain(p, Storage)
	container := w.storageContainers[p]
	if container == nil || container.Pos != p {
		t.Fatalf("storage container at %v = %#v", p, container)
	}
	if !container.Inventory.Add(IronOre, 7) {
		t.Fatal("could not put material in placed storage")
	}

	snap := w.snapshot(false, 10)
	if len(snap.Storages) != 1 || snap.Storages[0].Pos != p ||
		snap.Storages[0].Inventory.Count(IronOre) != 7 {
		t.Fatalf("snapshot storage = %#v", snap.Storages)
	}

	w.SetTerrain(p, Floor)
	if _, ok := w.storageContainers[p]; ok {
		t.Fatal("container state survived removal of its storage terrain")
	}
}

func TestStorageContainerCanBeOrderedForConstruction(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 20, 20
	w := newTestWorld(t, cfg)
	e := &Engine{world: w}
	e.apply(OrderStorageRoom{})
	if w.manualStorageRooms != 1 {
		t.Fatalf("pending storage orders = %d, want 1", w.manualStorageRooms)
	}

	w.planRooms()
	if len(w.projects) != 1 {
		t.Fatalf("storage order created %d projects, want 1", len(w.projects))
	}
	if w.projects[0].name != "storage room" {
		t.Fatalf("ordered project = %q, want storage room", w.projects[0].name)
	}
	if w.manualStorageRooms != 0 {
		t.Fatalf("pending storage orders after placement = %d, want 0", w.manualStorageRooms)
	}
	var found bool
	for _, task := range w.projects[0].tasks {
		if task.terrain == Storage {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("storage room project has no storage-container task")
	}
}
