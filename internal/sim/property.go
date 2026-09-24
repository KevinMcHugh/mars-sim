package sim

import "sort"

// Access is who may use a placed fixture. See docs/property.md.
type Access uint8

const (
	// AccessCommunal: anyone may use it. Everything the colony builds is
	// communal, which is how the game behaved before fixtures had owners.
	AccessCommunal Access = iota
	// AccessPrivate: only the owner may use it.
	AccessPrivate
	// TODO: AccessPaid — anyone may use it for a per-use charge to the owner.
	// Arrives with labor orders (economy phase E5); see docs/economy.md.
)

func (a Access) String() string {
	switch a {
	case AccessCommunal:
		return "communal"
	case AccessPrivate:
		return "private"
	default:
		return "unknown"
	}
}

// Fixture is the ownership record for one placed structure a colonist can
// walk up to and use: a pod, toilet, bed, incinerator, or storage container.
// Terrain says what the tile is; the fixture says whose it is and who may use
// it. Like storageContainers, fixtures are sparse — only these tiles have one —
// so ownership adds nothing to the page-shared tile grid.
type Fixture struct {
	Pos     Point
	Terrain Terrain
	Owner   Owner
	Access  Access
}

// isFixtureTerrain reports whether a terrain kind gets a Fixture record: the
// placed structures that are used from an adjacent tile. Walls are built but
// not used, so nobody needs to own one yet.
func isFixtureTerrain(t Terrain) bool {
	switch t {
	case NutrientPod, Toilet, Bed, Incinerator, Storage, Scumhouse:
		return true
	default:
		return false
	}
}

// hasDepot reports whether a terrain kind carries a storage container: a chest,
// or a scumhouse's store of inputs and meals.
func hasDepot(t Terrain) bool {
	return t == Storage || t == Scumhouse
}

// placeFixture records a newly placed fixture tile. SetTerrain calls it for
// every fixture terrain, so everything the colony builds starts out owned by
// the community and open to all; setFixtureOwner is how anything else changes
// that.
func (w *World) placeFixture(p Point, t Terrain) {
	w.fixtures[p] = &Fixture{Pos: p, Terrain: t, Owner: Community, Access: AccessCommunal}
	w.fixtureRev++
}

// dropFixture forgets the fixture at p when its terrain is replaced.
func (w *World) dropFixture(p Point) {
	f := w.fixtures[p]
	if f == nil {
		return
	}
	if f.Access != AccessCommunal {
		w.restrictedFixtures[f.Terrain]--
	}
	delete(w.fixtures, p)
	w.fixtureRev++
}

// setFixtureOwner changes who owns the fixture at p and who may use it,
// reporting false if there is no fixture there. A change in whether a fixture
// is communal changes which tiles the shared flow field for its terrain may
// lead to, so that field is marked stale.
func (w *World) setFixtureOwner(p Point, owner Owner, access Access) bool {
	f := w.fixtures[p]
	if f == nil {
		return false
	}
	wasRestricted, restricted := f.Access != AccessCommunal, access != AccessCommunal
	f.Owner, f.Access = owner, access
	if wasRestricted != restricted {
		if restricted {
			w.restrictedFixtures[f.Terrain]++
		} else {
			w.restrictedFixtures[f.Terrain]--
		}
		if field := w.fields[f.Terrain]; field != nil {
			// The fixture's access tiles stop (or start) being goals of the
			// shared field. Like a terrain change, that reaches the field on
			// its next read, as a repair around this tile.
			field.touch(p)
		}
	}
	w.fixtureRev++
	return true
}

// communalFixture reports whether anyone may use the tile at p. A tile with no
// fixture record is not restricted by ownership, so it counts as communal.
func (w *World) communalFixture(p Point) bool {
	f := w.fixtures[p]
	return f == nil || f.Access == AccessCommunal
}

// canUseFixture reports whether e may use the fixture at p: anyone may use a
// communal one, and only its owner a private one. Rats never own anything.
func (w *World) canUseFixture(e *Entity, p Point) bool {
	f := w.fixtures[p]
	if f == nil || f.Access == AccessCommunal {
		return true
	}
	return e.Kind == Colonist && f.Owner == ColonistOwner(e.ID)
}

// facilityReachable reports whether e can reach a facility of kind that it is
// allowed to use. The shared flow field only ever leads to communal fixtures
// (see facilitySeed), so it answers for those; a private fixture e owns is
// checked separately, and only when some fixture of this kind is restricted,
// so a colony with no private property pays nothing extra for it.
func (w *World) facilityReachable(e *Entity, kind Terrain) bool {
	if field := w.facilityField(kind); field != nil && field.at(e.Pos) >= 0 {
		return true
	}
	if w.restrictedFixtures[kind] == 0 {
		return false
	}
	room := w.roomOf(e.Pos)
	for p := range w.facilityTiles[kind] {
		if !w.communalFixture(p) && w.canUseFixture(e, p) && w.taskReachable(p, room) {
			return true
		}
	}
	return false
}

// LedgerLine is one owner's holding of one item kind in a depot.
type LedgerLine struct {
	Owner Owner
	Item  ItemKind
	Count int
}

// credit records that owner now holds n more of kind in this container. The
// caller must already have added the physical items: the ledger says whose
// they are, never whether they exist. Lines stay sorted by owner then item, so
// the ledger reads the same whatever order deposits arrived in.
func (c *StorageContainer) credit(owner Owner, kind ItemKind, n int) {
	if n <= 0 {
		return
	}
	i := sort.Search(len(c.Ledger), func(i int) bool {
		l := c.Ledger[i]
		return !(l.Owner.less(owner) || (l.Owner == owner && l.Item < kind))
	})
	if i < len(c.Ledger) && c.Ledger[i].Owner == owner && c.Ledger[i].Item == kind {
		c.Ledger[i].Count += n
		return
	}
	c.Ledger = append(c.Ledger, LedgerLine{})
	copy(c.Ledger[i+1:], c.Ledger[i:])
	c.Ledger[i] = LedgerLine{Owner: owner, Item: kind, Count: n}
}

// debit takes n of kind out of the container on owner's account: the physical
// items and the ledger line together, or neither. It refuses to take more than
// owner's line holds, so one owner can never withdraw another's goods.
func (c *StorageContainer) debit(owner Owner, kind ItemKind, n int) bool {
	if n <= 0 {
		return n == 0
	}
	for i := range c.Ledger {
		l := &c.Ledger[i]
		if l.Owner != owner || l.Item != kind {
			continue
		}
		if l.Count < n || !c.Inventory.Remove(kind, n) {
			return false
		}
		l.Count -= n
		if l.Count == 0 {
			c.Ledger = append(c.Ledger[:i], c.Ledger[i+1:]...)
		}
		return true
	}
	return false
}

// held returns how many of kind owner holds in this container.
func (c *StorageContainer) held(owner Owner, kind ItemKind) int {
	for _, l := range c.Ledger {
		if l.Owner == owner && l.Item == kind {
			return l.Count
		}
	}
	return 0
}

// ledgerBalanced reports whether, for every item kind, the ledger lines add up
// to exactly what is physically in the container. It is the depot ledger's one
// invariant; tests check it rather than production code, which keeps it by
// crediting at the same moment it adds.
func (c *StorageContainer) ledgerBalanced() bool {
	var byKind [numItemKinds]int
	for _, l := range c.Ledger {
		byKind[l.Item] += l.Count
	}
	for k := ItemKind(1); k < numItemKinds; k++ {
		if byKind[k] != c.Inventory.Count(k) {
			return false
		}
	}
	return true
}

// FixtureView is an immutable copy of one fixture's ownership, for display.
type FixtureView struct {
	Pos     Point
	Terrain Terrain
	Owner   Owner
	Access  Access
}

// publishedFixtures returns the fixtures sorted by position for a snapshot.
// The list is rebuilt only when a fixture changed since the last frame — which
// is rare next to how often frames are published — and the previous slice is
// handed back otherwise. That is safe because a published slice is never
// written again: a change allocates a new one.
func (w *World) publishedFixtures() []FixtureView {
	if w.snapFixtures != nil && w.snapFixtureRev == w.fixtureRev {
		return w.snapFixtures
	}
	out := make([]FixtureView, 0, len(w.fixtures))
	for _, f := range w.fixtures {
		out = append(out, FixtureView{Pos: f.Pos, Terrain: f.Terrain, Owner: f.Owner, Access: f.Access})
	}
	sort.Slice(out, func(i, j int) bool { return lessPoint(out[i].Pos, out[j].Pos) })
	w.snapFixtures, w.snapFixtureRev = out, w.fixtureRev
	return out
}
