package sim

import "testing"

func storageBehaviorWorld(t *testing.T, withStorage bool) (*World, *Entity, Point) {
	t.Helper()
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	for y := 5; y <= 15; y++ {
		for x := 5; x <= 22; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	chest := Point{20, 10}
	if withStorage {
		w.SetTerrain(chest, Storage)
	}
	w.refreshSpatial()
	return w, w.spawn(Colonist, Point{10, 10}), chest
}

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

func TestFullColonistSeeksStorageAndUnloadsMaterials(t *testing.T) {
	w, colonist, chest := storageBehaviorWorld(t, true)
	colonist.Inventory[0] = ItemStack{Kind: Pistol, Count: 1}
	for i := 1; i < len(colonist.Inventory); i++ {
		colonist.Inventory[i] = ItemStack{Kind: RawRock, Count: MaxStackSize}
	}

	w.assignWorkJob(colonist)
	if colonist.Job != JobStore || colonist.Target != chest {
		t.Fatalf("full colonist job = %v at %v, want storage at %v",
			colonist.Job, colonist.Target, chest)
	}
	for i := 0; i < 30 && w.storageContainers[chest].Inventory.Count(RawRock) == 0; i++ {
		w.colonistTurn(colonist)
	}

	if got := colonist.Inventory.Count(RawRock); got != 0 {
		t.Fatalf("colonist retained %d raw rock after unloading", got)
	}
	if got := colonist.Inventory.Count(Pistol); got != 1 {
		t.Fatalf("storage removed equipped pistol: count = %d", got)
	}
	if got := w.storageContainers[chest].Inventory.Count(RawRock); got != 7*MaxStackSize {
		t.Fatalf("stored raw rock = %d, want %d", got, 7*MaxStackSize)
	}
}

func TestFullInventoryMotivatesStorageConstruction(t *testing.T) {
	w, colonist, _ := storageBehaviorWorld(t, false)
	for i := range colonist.Inventory {
		colonist.Inventory[i] = ItemStack{Kind: RawRock, Count: MaxStackSize}
	}
	// Satisfy fatal life-support planning so storage is the next automatic
	// priority rather than being masked by the colony's more urgent need.
	w.SetTerrain(Point{7, 7}, NutrientPod)
	w.SetTerrain(Point{8, 7}, Toilet)

	w.planRooms()
	if len(w.projects) != 1 || w.projects[0].name != "storage room" {
		t.Fatalf("full inventory planned projects = %#v, want one storage room", w.projects)
	}

	w.assignWorkJob(colonist)
	if colonist.Job != JobBuild || colonist.task == nil {
		t.Fatalf("full colonist did not help build storage: job=%v task=%#v", colonist.Job, colonist.task)
	}
}

func TestStorageCanBreakAFullInventoryProjectDeadlock(t *testing.T) {
	w, colonist, _ := storageBehaviorWorld(t, false)
	for i := range colonist.Inventory {
		colonist.Inventory[i] = ItemStack{Kind: RawRock, Count: MaxStackSize}
	}
	// One unfinished project fills this one-colonist world's normal concurrency
	// allowance. Its dig task cannot be completed by the full worker.
	w.projects = []*project{{
		id: 1, name: "dormitory",
		tasks: []*buildTask{{pos: Point{2, 2}, terrain: Floor, phase: roomDigPhase}},
	}}

	w.planRooms()
	if len(w.projects) != 2 || w.projects[1].name != "storage room" {
		t.Fatalf("project-cap deadlock planned %#v, want an additional storage room", w.projects)
	}
}
