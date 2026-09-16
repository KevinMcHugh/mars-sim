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
