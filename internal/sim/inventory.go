package sim

const (
	// InventorySlotCount is the number of stacks a colonist can carry.
	InventorySlotCount = 8
	// StorageInventorySlotCount gives one storage container six times a
	// colonist's carrying capacity.
	StorageInventorySlotCount = 6 * InventorySlotCount
	// MaxStackSize is the largest number of items one inventory slot can hold.
	MaxStackSize = 64
)

// ItemKind identifies a resource that can be carried in an inventory.
type ItemKind uint8

const (
	ItemNone ItemKind = iota
	RawRock
	IronOre
	WaterIce
	// UraniumOre is mined from a uranium-bearing deposit. Carrying it keeps
	// its owner under a dose the whole time (see mutation.go): it is the one
	// item that acts on the colonist holding it.
	UraniumOre
	Clay
	// Pistol and Shotgun are combat weapons: a colonist carrying one fights an
	// alien that gets close instead of only fleeing. See combat.go and
	// docs/combat.md.
	Pistol
	Shotgun
	// Viscera is a gore stain scrubbed off the floor, and Corpse is a body
	// carried off it. Both are refuse: a colonist gathers them while cleaning
	// and they exist only to be destroyed in an incinerator. See cleaning.go
	// and docs/sanitation.md.
	Viscera
	Corpse
)

func (k ItemKind) String() string {
	switch k {
	case RawRock:
		return "raw rock"
	case IronOre:
		return "iron ore"
	case WaterIce:
		return "water ice"
	case UraniumOre:
		return "uranium ore"
	case Clay:
		return "clay"
	case Pistol:
		return "pistol"
	case Shotgun:
		return "shotgun"
	case Viscera:
		return "viscera"
	case Corpse:
		return "corpse"
	default:
		return "empty"
	}
}

// isWeapon reports whether an item kind is a wieldable combat weapon.
func (k ItemKind) isWeapon() bool {
	return k == Pistol || k == Shotgun
}

// bestWeapon returns the most effective weapon in inv, or ItemNone if the
// colonist is unarmed. A shotgun beats a pistol; ties within a kind don't
// matter since only its presence is checked.
func bestWeapon(inv Inventory) ItemKind {
	best := ItemNone
	for _, stack := range inv {
		if stack.Count == 0 || !stack.Kind.isWeapon() {
			continue
		}
		if stack.Kind == Shotgun {
			return Shotgun // nothing beats it
		}
		best = stack.Kind
	}
	return best
}

// isRefuse reports whether an item kind is waste bound for an incinerator. It
// is the one place that decides what burns, so a future kind of trash (spoiled
// rations, alien remains) joins the cleaning loop by being listed here.
func (k ItemKind) isRefuse() bool {
	return k == Viscera || k == Corpse
}

// isStorableMaterial reports whether an item belongs in general colony storage.
// Weapons stay equipped and refuse keeps its dedicated incinerator route.
func (k ItemKind) isStorableMaterial() bool {
	switch k {
	case RawRock, IronOre, WaterIce, UraniumOre, Clay:
		return true
	default:
		return false
	}
}

// Has reports whether the inventory holds at least one item of a kind. Used by
// the per-tick uranium-exposure check; it is a Count in disguise rather than
// its own loop, which over eight slots costs nothing and keeps one answer to
// "how much of this is in here".
func (inv *Inventory) Has(kind ItemKind) bool {
	return inv.Count(kind) > 0
}

// ItemStack is one homogeneous inventory slot. Empty slots have Count zero and
// ItemNone as their kind.
type ItemStack struct {
	Kind  ItemKind
	Count int
}

// Inventory is a colonist's fixed set of carrying slots.
type Inventory [InventorySlotCount]ItemStack

// StorageInventory is the fixed set of stacks held by one storage container.
type StorageInventory [StorageInventorySlotCount]ItemStack

// StorageContainer is the state attached to one Storage terrain tile. Position
// lives here rather than in StorageInventory so snapshots and future hauling
// jobs can identify a container without scanning the terrain grid.
type StorageContainer struct {
	Pos       Point
	Inventory StorageInventory
}

// RemoveAll empties every stack of a kind and returns how many items were in
// them. Used by the incinerator, which destroys a hauler's whole load at once
// rather than item by item.
func (inv *Inventory) RemoveAll(kind ItemKind) int {
	return removeAllStacks(inv[:], kind)
}

func removeAllStacks(stacks []ItemStack, kind ItemKind) int {
	if kind == ItemNone {
		return 0
	}
	removed := 0
	for i := range stacks {
		if stacks[i].Kind != kind || stacks[i].Count == 0 {
			continue
		}
		removed += stacks[i].Count
		stacks[i] = ItemStack{}
	}
	return removed
}

// Count returns how many items of a kind the inventory holds across all stacks.
func (inv *Inventory) Count(kind ItemKind) int {
	return countStacks(inv[:], kind)
}

// CanAdd reports whether all quantity items can fit without changing the
// inventory.
func (inv *Inventory) CanAdd(kind ItemKind, quantity int) bool {
	return canAddStacks(inv[:], kind, quantity)
}

func countStacks(stacks []ItemStack, kind ItemKind) int {
	if kind == ItemNone {
		return 0
	}
	n := 0
	for _, stack := range stacks {
		if stack.Kind == kind {
			n += stack.Count
		}
	}
	return n
}

func canAddStacks(stacks []ItemStack, kind ItemKind, quantity int) bool {
	if kind == ItemNone || quantity < 0 {
		return false
	}
	if quantity == 0 {
		return true
	}
	capacity := 0
	for _, stack := range stacks {
		switch {
		case stack.Count == 0:
			capacity += MaxStackSize
		case stack.Kind == kind:
			capacity += MaxStackSize - stack.Count
		}
		if capacity >= quantity {
			return true
		}
	}
	return false
}

// Add stores all quantity items, filling existing matching stacks before empty
// slots. It returns false and leaves the inventory unchanged if they do not all
// fit.
func (inv *Inventory) Add(kind ItemKind, quantity int) bool {
	return addStacks(inv[:], kind, quantity)
}

func addStacks(stacks []ItemStack, kind ItemKind, quantity int) bool {
	if !canAddStacks(stacks, kind, quantity) {
		return false
	}
	remaining := quantity
	for i := range stacks {
		stack := &stacks[i]
		if stack.Kind != kind || stack.Count == 0 || stack.Count == MaxStackSize {
			continue
		}
		add := min(remaining, MaxStackSize-stack.Count)
		stack.Count += add
		remaining -= add
		if remaining == 0 {
			return true
		}
	}
	for i := range stacks {
		stack := &stacks[i]
		if stack.Count != 0 {
			continue
		}
		add := min(remaining, MaxStackSize)
		*stack = ItemStack{Kind: kind, Count: add}
		remaining -= add
		if remaining == 0 {
			return true
		}
	}
	return true
}

// AddAll stores several item stacks as one transaction. If the complete set
// does not fit, it returns false without changing the inventory.
func (inv *Inventory) AddAll(stacks ...ItemStack) bool {
	next := *inv
	for _, stack := range stacks {
		if stack.Count < 0 || (stack.Count > 0 && !next.Add(stack.Kind, stack.Count)) {
			return false
		}
	}
	*inv = next
	return true
}

// CanAddAll reports whether a heterogeneous group of stacks fits without
// changing the inventory.
func (inv *Inventory) CanAddAll(stacks ...ItemStack) bool {
	next := *inv
	return next.AddAll(stacks...)
}

// storableStacks returns the material stacks a colonist can deposit in a chest.
// The returned values are a copy, suitable for an atomic StorageInventory.AddAll.
func (inv *Inventory) storableStacks() []ItemStack {
	stacks := make([]ItemStack, 0, len(inv))
	for _, stack := range inv {
		if stack.Count > 0 && stack.Kind.isStorableMaterial() {
			stacks = append(stacks, stack)
		}
	}
	return stacks
}

// removeStorable clears all general materials after a successful deposit.
func (inv *Inventory) removeStorable() {
	for i := range inv {
		if inv[i].Kind.isStorableMaterial() {
			inv[i] = ItemStack{}
		}
	}
}

// Count returns how many items of a kind the container holds.
func (inv *StorageInventory) Count(kind ItemKind) int {
	return countStacks(inv[:], kind)
}

// RemoveAll empties every stack of a kind and returns the number removed.
func (inv *StorageInventory) RemoveAll(kind ItemKind) int {
	return removeAllStacks(inv[:], kind)
}

// CanAdd reports whether all quantity items fit without changing the container.
func (inv *StorageInventory) CanAdd(kind ItemKind, quantity int) bool {
	return canAddStacks(inv[:], kind, quantity)
}

// Add stores all quantity items, or leaves the container unchanged if they do
// not fit.
func (inv *StorageInventory) Add(kind ItemKind, quantity int) bool {
	return addStacks(inv[:], kind, quantity)
}

// AddAll stores heterogeneous stacks as one transaction.
func (inv *StorageInventory) AddAll(stacks ...ItemStack) bool {
	next := *inv
	for _, stack := range stacks {
		if stack.Count < 0 || (stack.Count > 0 && !next.Add(stack.Kind, stack.Count)) {
			return false
		}
	}
	*inv = next
	return true
}

// CanAddAll reports whether heterogeneous stacks fit without mutation.
func (inv *StorageInventory) CanAddAll(stacks ...ItemStack) bool {
	next := *inv
	return next.AddAll(stacks...)
}

// miningYield returns the complete inventory award for excavating a tile.
func miningYield(tile Tile) []ItemStack {
	yield := []ItemStack{{Kind: RawRock, Count: 1}}
	switch tile.Composition {
	case IronBearingRock:
		yield = append(yield, ItemStack{Kind: IronOre, Count: 1})
	case WaterIceBearingRock:
		yield = append(yield, ItemStack{Kind: WaterIce, Count: 1})
	case UraniumBearingRock:
		yield = append(yield, ItemStack{Kind: UraniumOre, Count: 1})
	case ClayBearingRock:
		yield = append(yield, ItemStack{Kind: Clay, Count: 1})
	}
	return yield
}
