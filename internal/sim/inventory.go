package sim

const (
	// InventorySlotCount is the number of stacks a colonist can carry.
	InventorySlotCount = 8
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
)

func (k ItemKind) String() string {
	switch k {
	case RawRock:
		return "raw rock"
	case IronOre:
		return "iron ore"
	case WaterIce:
		return "water ice"
	default:
		return "empty"
	}
}

// ItemStack is one homogeneous inventory slot. Empty slots have Count zero and
// ItemNone as their kind.
type ItemStack struct {
	Kind  ItemKind
	Count int
}

// Inventory is a colonist's fixed set of carrying slots.
type Inventory [InventorySlotCount]ItemStack

// CanAdd reports whether all quantity items can fit without changing the
// inventory.
func (inv *Inventory) CanAdd(kind ItemKind, quantity int) bool {
	if kind == ItemNone || quantity < 0 {
		return false
	}
	if quantity == 0 {
		return true
	}
	capacity := 0
	for _, stack := range inv {
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
	if !inv.CanAdd(kind, quantity) {
		return false
	}
	remaining := quantity
	for i := range inv {
		stack := &inv[i]
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
	for i := range inv {
		stack := &inv[i]
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

// miningYield returns the complete inventory award for excavating a tile.
func miningYield(tile Tile) []ItemStack {
	yield := []ItemStack{{Kind: RawRock, Count: 1}}
	switch tile.Composition {
	case IronBearingRock:
		yield = append(yield, ItemStack{Kind: IronOre, Count: 1})
	case WaterIceBearingRock:
		yield = append(yield, ItemStack{Kind: WaterIce, Count: 1})
	}
	return yield
}
