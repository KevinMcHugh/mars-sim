package sim

import "fmt"

// OwnerKind says what kind of party an Owner is. Everything in the economy —
// wallets, ledger lines, fixtures — names its holder with an Owner, so a new
// kind of holder is one constant here plus its handling in transfer and the
// access checks. See docs/money.md and docs/property.md.
type OwnerKind uint8

const (
	// OwnerNone is nobody: abandoned property anyone may claim. It can hold
	// goods but never money — a dollar with no account behind it would leak
	// out of the conservation invariant.
	OwnerNone OwnerKind = iota
	// OwnerCommunity is the colony itself. Its money is the treasury, and what
	// it owns everyone may use.
	OwnerCommunity
	// OwnerColonist is one colonist, named by Owner.ID.
	OwnerColonist
	// ownerOrder is an open market order holding its own escrow — the money
	// a bid has put up, or the goods an ask has set aside on a ledger — named
	// by its OrderID in Owner.ID. Unexported: nothing outside the market
	// should ever pay one or own through one. See market.go.
	ownerOrder
	// TODO: OwnerOrganization — an organization could own property. Undefined
	// for now; see docs/economy.md.
)

// Owner names who holds money or property. ID is meaningful only for
// OwnerColonist; the other kinds leave it zero, so two Owners compare equal
// with == exactly when they name the same party.
type Owner struct {
	Kind OwnerKind
	ID   EntityID
}

var (
	// Nobody is the owner of abandoned property.
	Nobody = Owner{Kind: OwnerNone}
	// Community is the colony as a whole.
	Community = Owner{Kind: OwnerCommunity}
)

// ColonistOwner names one colonist as an owner.
func ColonistOwner(id EntityID) Owner {
	return Owner{Kind: OwnerColonist, ID: id}
}

// less orders owners deterministically: nobody, then the community, then
// colonists by ID. Anything that lists owners (ledger lines, the market view)
// sorts with this rather than trusting a map's iteration order.
func (o Owner) less(p Owner) bool {
	if o.Kind != p.Kind {
		return o.Kind < p.Kind
	}
	return o.ID < p.ID
}

// Order reports whether this owner is an open market order holding goods in
// escrow, and which one: the "for sale" line of a ledger.
func (o Owner) Order() (OrderID, bool) {
	return OrderID(o.ID), o.Kind == ownerOrder
}

func (o Owner) String() string {
	switch o.Kind {
	case OwnerNone:
		return "nobody"
	case OwnerCommunity:
		return "the colony"
	case OwnerColonist:
		return fmt.Sprintf("colonist #%d", o.ID)
	case ownerOrder:
		return fmt.Sprintf("order #%d", o.ID)
	default:
		return "unknown"
	}
}
