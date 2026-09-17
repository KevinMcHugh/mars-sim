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
	// UraniumOre is mined from a uranium-bearing deposit. Carrying it keeps
	// its owner under a dose the whole time (see mutation.go): it is the one
	// item that acts on the colonist holding it.
	UraniumOre
	// Pistol and Shotgun are combat weapons: a colonist carrying one fights an
	// alien that gets close instead of only fleeing. See combat.go and
	// docs/combat.md.
	Pistol
	Shotgun
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
	case Pistol:
		return "pistol"
	case Shotgun:
		return "shotgun"
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

// Has reports whether the inventory holds at least one item of a kind. Used by
// the per-tick uranium-exposure check, so it walks the eight slots rather than
// building anything.
func (inv *Inventory) Has(kind ItemKind) bool {
	if kind == ItemNone {
		return false
	}
	for _, stack := range inv {
		if stack.Kind == kind && stack.Count > 0 {
			return true
		}
	}
	return false
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
	case UraniumBearingRock:
		yield = append(yield, ItemStack{Kind: UraniumOre, Count: 1})
	}
	return yield
}
