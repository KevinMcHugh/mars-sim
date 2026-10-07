package sim

import (
	"testing"
)

// zoneWorld is a world with manual zoning, no colonists, and nothing but one
// open, explored chamber: x 4..35, y 4..19 on the 40x24 test map.
func zoneWorld(t *testing.T) *World {
	t.Helper()
	cfg := testConfig()
	cfg.ZoningAuto = false
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			w.SetTerrain(Point{x, y, LandingLevel}, Rock)
		}
	}
	for y := 4; y <= 19; y++ {
		for x := 4; x <= 35; x++ {
			w.SetTerrain(Point{x, y, LandingLevel}, Floor)
		}
	}
	w.refreshSpatial()
	return w
}

// buildAll raises every task of a project at once, as if its builders had.
func buildAll(w *World, p *project) {
	for _, t := range p.tasks {
		if t.terrain != Floor {
			w.SetTerrain(t.pos, t.terrain)
		}
	}
	w.pruneProjects()
	w.refreshSpatial()
}

// clearAll takes down every tile of a clearing project at once.
func clearAll(w *World, p *project) {
	for _, t := range p.tasks {
		w.clearTile(t.pos)
		if t.order != nil && w.workOrders[t.order.ID] == t.order {
			w.closeWork(t.order)
		}
	}
	w.pruneProjects()
	w.refreshSpatial()
}

func lastProject(t *testing.T, w *World, name string) *project {
	t.Helper()
	for i := len(w.projects) - 1; i >= 0; i-- {
		if w.projects[i].name == name {
			return w.projects[i]
		}
	}
	t.Fatalf("no %s project; have %v", name, projectNames(w.projects))
	return nil
}

func zonedTiles(w *World, k ZoneKind) int { return w.zoneTiles[k] }

// With manual zoning the colony builds nothing it has no zone for, and says
// what it is waiting on; once a zone is drawn it builds there and only there.
func TestManualZoningBuildsOnlyInsideAZoneOfTheRightKind(t *testing.T) {
	w := zoneWorld(t)
	w.manualDormitories = 1
	w.planRooms()
	if len(w.projects) != 0 {
		t.Fatalf("planned %v with no zones drawn", projectNames(w.projects))
	}
	if got := w.zoneWaiting(); len(got) != 1 || got[0] != Bed {
		t.Fatalf("waiting on %v, want bunks", got)
	}

	// A storage zone is no place for a dormitory.
	if !w.paintZone(PaintZone{Kind: ZoneStorage, X0: 6, Y0: 6, X1: 16, Y1: 14}) {
		t.Fatal("storage zone refused")
	}
	w.planRooms()
	if len(w.projects) != 0 {
		t.Fatalf("planned %v in a storage zone", projectNames(w.projects))
	}

	if !w.paintZone(PaintZone{Kind: ZoneResidence, X0: 20, Y0: 6, X1: 30, Y1: 14}) {
		t.Fatal("residence zone refused")
	}
	w.planRooms()
	p := lastProject(t, w, dormRoom.name)
	for _, task := range p.tasks {
		if z := w.zoneAt(task.pos); z != ZoneResidence {
			t.Errorf("dormitory task at %v is zoned %v", task.pos, z)
		}
	}
	if s := p.structure; s == nil || s.zone != ZoneResidence || w.structureName(s) != "dormitory" {
		t.Fatalf("dormitory project's structure = %+v", s)
	}
	if w.manualDormitories != 0 {
		t.Fatalf("%d dormitories still on order", w.manualDormitories)
	}
}

// Zones never overlap: painting over another kind replaces it where they
// meet, and two rectangles of one kind side by side make one L-shaped zone.
func TestPaintingAZoneReplacesWhatItCovers(t *testing.T) {
	w := zoneWorld(t)
	w.paintZone(PaintZone{Kind: ZoneResidence, X0: 6, Y0: 6, X1: 15, Y1: 10})   // 10x5
	w.paintZone(PaintZone{Kind: ZoneStorage, X0: 13, Y0: 6, X1: 20, Y1: 10})    // takes 3 columns
	w.paintZone(PaintZone{Kind: ZoneResidence, X0: 6, Y0: 11, X1: 8, Y1: 15})   // the L's foot
	w.paintZone(PaintZone{Kind: ZoneResidence, X0: 20, Y0: 10, X1: 20, Y1: 10}) // takes one tile back
	if got, want := zonedTiles(w, ZoneResidence), 7*5+3*5+1; got != want {
		t.Errorf("residence covers %d tiles, want %d", got, want)
	}
	if got, want := zonedTiles(w, ZoneStorage), 8*5-1; got != want {
		t.Errorf("storage covers %d tiles, want %d", got, want)
	}
	if w.zoneAt(Point{14, 8, LandingLevel}) != ZoneStorage || w.zoneAt(Point{12, 8, LandingLevel}) != ZoneResidence || w.zoneAt(Point{20, 10, LandingLevel}) != ZoneResidence {
		t.Error("the later paint did not win where the zones met")
	}
	w.paintZone(PaintZone{Kind: NoZone, X0: 0, Y0: 0, X1: 39, Y1: 23})
	if zonedTiles(w, ZoneResidence)+zonedTiles(w, ZoneStorage) != 0 || zonedTiles(w, NoZone) != w.Width*w.Height {
		t.Errorf("unzoning everything left %d residence, %d storage", zonedTiles(w, ZoneResidence), zonedTiles(w, ZoneStorage))
	}

	// The published runs read back the same ground, merged across columns.
	w.paintZone(PaintZone{Kind: ZoneProduction, X0: 5, Y0: 7, X1: 9, Y1: 7})
	w.paintZone(PaintZone{Kind: ZoneProduction, X0: 10, Y0: 7, X1: 12, Y1: 7})
	if runs := w.publishedZones(); len(runs) != 1 || runs[0] != (ZoneRun{Y: 7, X0: 5, X1: 12, Kind: ZoneProduction}) {
		t.Errorf("runs = %+v", runs)
	}
}

// A dormitory built in residence has to go when its ground is zoned for
// storage: the paint buys a clearing order for every tile of it, and once
// it is down the structure is forgotten and its doorway freed.
func TestRezoningUnderAStructureOrdersItCleared(t *testing.T) {
	w := zoneWorld(t)
	w.paintZone(PaintZone{Kind: ZoneResidence, X0: 20, Y0: 6, X1: 30, Y1: 14})
	w.manualDormitories = 1
	w.planRooms()
	room := lastProject(t, w, dormRoom.name)
	s := room.structure
	buildAll(w, room)
	built := 0
	for _, p := range s.tiles {
		if isBuilt(w.TerrainAt(p)) {
			built++
		}
	}
	if built == 0 || !w.doorTiles[s.doors[0]] {
		t.Fatalf("%d tiles built, door reserved %v", built, w.doorTiles[s.doors[0]])
	}

	before := w.treasury
	// Storage over just the dormitory's corner still takes the whole room.
	if !w.paintZone(PaintZone{Kind: ZoneStorage, X0: s.x1, Y0: s.y1, X1: s.x1 + 3, Y1: s.y1 + 3}) {
		t.Fatalf("rezone refused: %v", w.log.tail(1))
	}
	clearing := lastProject(t, w, ClearingName)
	if len(clearing.tasks) != built {
		t.Fatalf("clearing %d tiles, want the dormitory's %d", len(clearing.tasks), built)
	}
	if cost := Money(built) * Money(w.cfg.WageDemolish); w.treasury != before-cost {
		t.Fatalf("treasury %v, was %v, clearing costs %v", w.treasury, before, cost)
	}
	if !loggedContaining(w, "dormitory") {
		t.Error("the log does not say what is being cleared")
	}
	clearAll(w, clearing)
	if w.structures[s.id] != nil {
		t.Error("the dormitory is still registered after it was cleared")
	}
	if w.doorTiles[s.doors[0]] {
		t.Error("the cleared dormitory's doorway is still reserved")
	}
	assertMoneyConserved(t, w)
}

// Unzoning ground leaves whatever stood on it unzoned, so it is cleared too.
func TestUnzoningOrdersWhatStoodThereCleared(t *testing.T) {
	w := zoneWorld(t)
	w.paintZone(PaintZone{Kind: ZoneStorage, X0: 18, Y0: 6, X1: 30, Y1: 14})
	w.manualStorageRooms = 1
	w.planRooms()
	room := lastProject(t, w, storageRoom.name)
	buildAll(w, room)
	w.paintZone(PaintZone{Kind: NoZone, X0: 0, Y0: 0, X1: 39, Y1: 23})
	clearing := lastProject(t, w, ClearingName)
	for _, task := range clearing.tasks {
		if !isBuilt(w.TerrainAt(task.pos)) {
			t.Errorf("clearing task at %v on %v", task.pos, w.TerrainAt(task.pos))
		}
	}
	if len(clearing.tasks) == 0 {
		t.Fatal("unzoning ordered nothing cleared")
	}
}

// Zoning is free but its work is not: a paint the treasury cannot pay for
// changes nothing at all.
func TestARezoneTheTreasuryCannotPayForChangesNothing(t *testing.T) {
	w := zoneWorld(t)
	w.paintZone(PaintZone{Kind: ZoneResidence, X0: 20, Y0: 6, X1: 30, Y1: 14})
	w.manualDormitories = 1
	w.planRooms()
	buildAll(w, lastProject(t, w, dormRoom.name))
	c := w.spawn(Colonist, Point{8, 8, LandingLevel})
	w.transfer(Community, ColonistOwner(c.ID), w.treasury)

	if w.paintZone(PaintZone{Kind: ZoneStorage, X0: 20, Y0: 6, X1: 30, Y1: 14}) {
		t.Fatal("an empty treasury paid to clear a dormitory")
	}
	if w.zoneAt(Point{25, 10, LandingLevel}) != ZoneResidence {
		t.Error("the refused paint changed the zone anyway")
	}
	// Painting where nothing needs doing is still free.
	if !w.paintZone(PaintZone{Kind: ZoneStorage, X0: 6, Y0: 6, X1: 10, Y1: 10}) {
		t.Error("a free paint was refused")
	}
	assertMoneyConserved(t, w)
}

// Rock the colony has seen inside a new zone is dug out, paid like an
// excavation order.
func TestAZoneOverRockOrdersItDugOut(t *testing.T) {
	w := zoneWorld(t)
	// Reveal a band of rock above the chamber.
	for x := 6; x <= 12; x++ {
		w.revealAround(Point{x, 3, LandingLevel})
	}
	c := PaintZone{Kind: ZoneProduction, X0: 6, Y0: 2, X1: 12, Y1: 6}
	rock := len(w.unmarkedRock(6, 2, 12, 6))
	if rock == 0 {
		t.Fatal("no seen rock in the zone")
	}
	if !w.paintZone(c) {
		t.Fatalf("zone refused: %v", w.log.tail(1))
	}
	dig := lastProject(t, w, ExcavationName)
	if len(dig.tasks) != rock {
		t.Fatalf("digging %d tiles, want %d", len(dig.tasks), rock)
	}
	for _, task := range dig.tasks {
		if w.zoneAt(task.pos) != ZoneProduction {
			t.Errorf("dig at %v outside the zone", task.pos)
		}
	}
}

// The ground round a colony ship is residence, held for as long as the ship
// stands: painting skips it. Clearing the ship lets it go, and its lockers'
// goods move to a chest, still their owners'.
func TestShipsHoldTheirGroundAsResidence(t *testing.T) {
	cfg := testConfig()
	cfg.ZoningAuto = false
	cfg.StartColonists, cfg.StartAliens = 2, 0
	cfg.ShipCapacity = 1 // two ships, side by side
	w := newTestWorld(t, cfg)
	if len(w.ships) != 2 {
		t.Fatalf("%d ships landed", len(w.ships))
	}
	ship := w.ships[0].structure
	if ship == nil || ship.ship == nil || ship.zone != ZoneResidence || w.structureName(ship) != "colony ship" {
		t.Fatalf("the first ship's structure = %+v", ship)
	}
	for _, p := range ship.lock {
		if w.zoneAt(p) != ZoneResidence || !w.zoneLocked(p) {
			t.Fatalf("%v round a ship is %v, locked %v", p, w.zoneAt(p), w.zoneLocked(p))
		}
	}
	w.paintZone(PaintZone{Kind: ZoneStorage, X0: 0, Y0: 0, X1: w.Width - 1, Y1: w.Height - 1})
	for _, p := range ship.lock {
		if w.zoneAt(p) != ZoneResidence {
			t.Fatalf("painting storage took %v from a ship", p)
		}
	}
	if !loggedContaining(w, "stay residence") {
		t.Error("the log does not say the ships' ground was kept")
	}

	// A communal chest elsewhere takes the locker's meals.
	chest := Point{1, 1, LandingLevel}
	w.SetTerrain(chest, Storage)
	owner := w.entities[w.ships[0].Colonists[0]]
	locker, ok := w.lockerOf(owner)
	if !ok {
		t.Fatal("the passenger has no locker")
	}
	meals := w.storageContainers[locker].held(ColonistOwner(owner.ID), Meal)
	if meals == 0 {
		t.Fatal("the locker landed empty")
	}
	if !w.clearArea(ClearArea{X0: ship.x0, Y0: ship.y0, X1: ship.x1, Y1: ship.y1}) {
		t.Fatalf("clearing the ship refused: %v", w.log.tail(1))
	}
	clearAll(w, lastProject(t, w, ClearingName))
	if w.structures[ship.id] != nil || w.ships[0].structure != nil {
		t.Fatal("the cleared ship is still registered")
	}
	for _, d := range ship.doors {
		if w.doorTiles[d] {
			t.Fatalf("the cleared ship's door step %v is still reserved", d)
		}
	}
	// Ground the other ship holds too (their walkways meet) stays held.
	other := map[Point]bool{}
	if s := w.ships[1].structure; s != nil {
		for _, p := range s.lock {
			other[p] = true
		}
	}
	for _, p := range ship.lock {
		if w.zoneLocked(p) != other[p] {
			t.Fatalf("%v held %v after the ship went; the other ship holds it: %v", p, w.zoneLocked(p), other[p])
		}
	}
	if got := w.storageContainers[chest].held(ColonistOwner(owner.ID), Meal); got != meals {
		t.Errorf("the chest holds %d of the owner's meals, want %d", got, meals)
	}
	if !w.storageContainers[chest].ledgerBalanced() {
		t.Error("the chest's ledger does not balance")
	}
}

// A ship moved before the first tick takes its residence hold with it.
func TestAMovedShipTakesItsGroundWithIt(t *testing.T) {
	cfg := testConfig()
	cfg.ZoningAuto = false
	cfg.StartColonists, cfg.StartAliens = 1, 0
	cfg.Width, cfg.Height = 80, 40
	w := newTestWorld(t, cfg)
	sh := w.ships[0]
	before := sh.structure
	o := sh.Origin
	if !w.moveShip(MoveShip{Ship: sh.ID, X: o.X + 12, Y: o.Y}) {
		t.Skip("no room to move the ship")
	}
	if w.structures[before.id] != nil {
		t.Error("the ship's old structure is still registered")
	}
	for _, p := range before.lock {
		if sh.near(p) {
			continue // under the ship where it landed again
		}
		if w.zoneLocked(p) || w.zoneAt(p) != NoZone {
			t.Fatalf("%v at the old site is still %v, held %v", p, w.zoneAt(p), w.zoneLocked(p))
		}
	}
	if sh.structure == nil || sh.structure == before {
		t.Fatal("the moved ship was not registered again")
	}
	for _, p := range sh.structure.lock {
		if !w.zoneLocked(p) || w.zoneAt(p) != ZoneResidence {
			t.Fatalf("%v at the new site is not held as residence", p)
		}
	}
}

// A depot that comes down closes its market and moves its goods to another
// chest, each line still its owner's; an ask resting there is withdrawn first.
func TestClearingADepotMovesItsGoodsWithTheirOwners(t *testing.T) {
	w := zoneWorld(t)
	a, b := w.spawn(Colonist, Point{8, 8, LandingLevel}), w.spawn(Colonist, Point{9, 8, LandingLevel})
	from, to := Point{10, 10, LandingLevel}, Point{20, 10, LandingLevel}
	w.SetTerrain(from, Storage)
	w.SetTerrain(to, Storage)
	src := w.storageContainers[from]
	for _, l := range []struct {
		e *Entity
		n int
	}{{a, 3}, {b, 5}} {
		src.Inventory.Add(Meal, l.n)
		src.credit(ColonistOwner(l.e.ID), Meal, l.n)
	}
	ask, _ := w.post(Ask, Meal, 2, 5, ColonistOwner(a.ID), from, 0)
	if ask == nil || w.orders[ask.ID] == nil {
		t.Fatal("no ask resting at the depot")
	}
	w.demolish(from)
	if w.orders[ask.ID] != nil {
		t.Error("the ask at the cleared depot is still open")
	}
	dst := w.storageContainers[to]
	if dst.held(ColonistOwner(a.ID), Meal) != 3 || dst.held(ColonistOwner(b.ID), Meal) != 5 || !dst.ledgerBalanced() {
		t.Errorf("moved ledger %+v", dst.Ledger)
	}
	if w.storageContainers[from] != nil || w.TerrainAt(from) != Floor {
		t.Error("the depot is still there")
	}
}

// Taking a wall down opens the way at once: the shared flow field routes
// through the gap, and a colonist already walking the long way round drops
// its route to plan a shorter one.
func TestClearingAWallReroutesColonists(t *testing.T) {
	w := zoneWorld(t)
	// A wall across the chamber at x=20, open only at the bottom row.
	for y := 4; y <= 18; y++ {
		w.SetTerrain(Point{20, y, LandingLevel}, Wall)
	}
	toilet := Point{10, 5, LandingLevel}
	w.SetTerrain(toilet, Toilet)
	w.refreshSpatial()
	f := w.facilityField(Toilet)
	at := Point{30, 5, LandingLevel}
	f.ensureFresh()
	long := f.at(at)
	if long <= 0 {
		t.Fatalf("toilet unreachable from %v before clearing (%d)", at, long)
	}
	walker := w.spawn(Colonist, at)
	route, ok := w.pathToAdjacent(walker.Pos, toilet)
	if !ok {
		t.Fatal("no route round the wall")
	}
	walker.path, walker.pathAt, walker.pathGoal = route, 0, toilet

	w.tick++
	w.clearTile(Point{20, 5, LandingLevel})
	w.refreshSpatial()
	f.ensureFresh()
	if short := f.at(at); short <= 0 || short >= long {
		t.Fatalf("field distance %d after clearing, was %d", short, long)
	}
	if len(walker.path) != 0 {
		t.Error("the colonist kept its route round the wall")
	}
}

// Colonists take a clearing order like any other paid work, and are paid
// for it: every tile comes down and nothing stays escrowed.
func TestColonistsClearAnAreaAndArePaid(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	// A short wall in the open cavern: three floor tiles in a row, with
	// floor above and below so the wall seals nothing off, nearest the map
	// center.
	var wall []Point
	cx, cy := w.Width/2, w.Height/2
	for r := 0; r < w.Height/2 && wall == nil; r++ {
		for _, y := range []int{cy - r, cy + r} {
			for x := cx - 8; x <= cx+8 && wall == nil; x++ {
				ok := true
				for dx := 0; dx < 3 && ok; dx++ {
					for dy := -1; dy <= 1; dy++ {
						p := Point{x + dx, y + dy, LandingLevel}
						if w.TerrainAt(p) != Floor || w.occupied(p) || w.zoneLocked(p) || w.doorTiles[p] {
							ok = false
						}
					}
				}
				if ok {
					wall = []Point{{x, y, LandingLevel}, {x + 1, y, LandingLevel}, {x + 2, y, LandingLevel}}
				}
			}
		}
	}
	if wall == nil {
		t.Fatal("no open floor to wall off")
	}
	for _, p := range wall {
		w.SetTerrain(p, Wall)
	}
	w.refreshSpatial()
	wallets := w.moneyInCirculation() - w.treasury
	minX, maxX := wall[0].X, wall[len(wall)-1].X
	if !w.clearArea(ClearArea{X0: minX, Y0: wall[0].Y, X1: maxX, Y1: wall[0].Y}) {
		t.Fatalf("refused: %v", w.log.tail(1))
	}
	for i := 0; i < 4000 && w.workEscrowed() > 0; i++ {
		w.step()
	}
	for _, p := range wall {
		if w.TerrainAt(p) != Floor {
			t.Errorf("%v is still %v", p, w.TerrainAt(p))
		}
	}
	if w.workEscrowed() != 0 {
		t.Fatalf("%v still escrowed", w.workEscrowed())
	}
	if w.moneyInCirculation()-w.treasury <= wallets {
		t.Error("nobody was paid for the clearing")
	}
	assertMoneyConserved(t, w)
}

// A clearing order can be cancelled; what it held goes back to the treasury.
func TestAClearingOrderCanBeCancelled(t *testing.T) {
	w := zoneWorld(t)
	for x := 10; x <= 14; x++ {
		w.SetTerrain(Point{x, 10, LandingLevel}, Wall)
	}
	before := w.treasury
	w.clearArea(ClearArea{X0: 10, Y0: 10, X1: 14, Y1: 10})
	p := lastProject(t, w, ClearingName)
	if !w.cancelClearing(p.id) {
		t.Fatal("cancel refused")
	}
	if w.treasury != before || w.workEscrowed() != 0 || w.TerrainAt(Point{12, 10, LandingLevel}) != Wall {
		t.Fatalf("treasury %v (was %v), escrow %v, wall %v", w.treasury, before, w.workEscrowed(), w.TerrainAt(Point{12, 10, LandingLevel}))
	}
}

// Clearing a room under construction calls it off and refunds it.
func TestClearingARisingRoomCallsItOff(t *testing.T) {
	w := zoneWorld(t)
	w.paintZone(PaintZone{Kind: ZoneResidence, X0: 20, Y0: 6, X1: 30, Y1: 14})
	w.manualDormitories = 1
	w.planRooms()
	room := lastProject(t, w, dormRoom.name)
	s := room.structure
	if !w.clearArea(ClearArea{X0: s.x0, Y0: s.y0, X1: s.x1, Y1: s.y1}) {
		t.Fatalf("refused: %v", w.log.tail(1))
	}
	if w.hasProject(room) || w.workEscrowed() != 0 {
		t.Fatalf("room still planned %v, escrow %v", w.hasProject(room), w.workEscrowed())
	}
	if w.structures[s.id] != nil {
		t.Error("a room never built is still registered")
	}
	assertMoneyConserved(t, w)
}

// With zoning-auto the colony zones each room it marks out for the room's
// kind, and keeps other kinds of room off it.
func TestAutoZoningZonesWhatTheColonyBuilds(t *testing.T) {
	w := zoneWorld(t)
	w.cfg.ZoningAuto = true
	w.manualDormitories = 1
	w.planRooms()
	room := lastProject(t, w, dormRoom.name)
	for _, p := range room.structure.area {
		if w.zoneAt(p) != ZoneResidence {
			t.Fatalf("%v of an auto-planned dormitory is zoned %v", p, w.zoneAt(p))
		}
	}
	// A player's zone is used first.
	w.paintZone(PaintZone{Kind: ZoneStorage, X0: 24, Y0: 8, X1: 34, Y1: 18})
	w.manualStorageRooms = 1
	w.cfg.MaxConcurrentProjects = 4
	buildAll(w, room)
	w.planRooms()
	silo := lastProject(t, w, storageRoom.name)
	for _, p := range silo.structure.area {
		if w.zoneAt(p) != ZoneStorage || p.X < 24 || p.Y < 8 {
			t.Fatalf("the storage room's %v is outside the player's storage zone", p)
		}
	}
}

// A manual-zoning colony the player zones builds its scumhouse, and builds
// every room inside a zone of its kind.
func TestAZonedColonyBuildsInItsZones(t *testing.T) {
	if testing.Short() {
		t.Skip("long run")
	}
	cfg := DefaultConfig()
	cfg.Seed, cfg.TraitChance, cfg.StartAliens, cfg.CavernNestPercent = 42, 0, 0, 0
	cfg.Width, cfg.Height, cfg.StartColonists = 80, 50, 6
	w := newTestWorld(t, cfg)
	if w.cfg.ZoningAuto {
		t.Fatal("the game's default is not manual zoning")
	}
	cx, cy := w.Width/2, w.Height/2
	for _, z := range []PaintZone{
		{Kind: ZoneProduction, X0: cx - 18, Y0: cy - 10, X1: cx - 4, Y1: cy - 1},
		{Kind: ZoneStorage, X0: cx - 3, Y0: cy - 10, X1: cx + 5, Y1: cy - 1},
		{Kind: ZoneResidence, X0: cx + 6, Y0: cy - 10, X1: cx + 20, Y1: cy - 1},
	} {
		if !w.paintZone(z) {
			t.Fatalf("zone %v refused: %v", z.Kind, w.log.tail(1))
		}
	}
	for i := 0; i < 6000; i++ {
		w.step()
	}
	if w.countTerrain(Scumhouse) == 0 {
		t.Fatalf("no scumhouse by tick %d; waiting on %v; log %v", w.tick, w.zoneWaiting(), w.log.tail(5))
	}
	var built []string
	for _, s := range w.sortedStructures() {
		if s.ship == nil {
			built = append(built, w.structureName(s))
		}
	}
	t.Logf("tick %d: %d starved; built %v; waiting on %v", w.tick, w.starved, built, w.zoneWaiting())
	for _, s := range w.sortedStructures() {
		for _, p := range s.area {
			if z := w.zoneAt(p); z != s.zone {
				t.Fatalf("%s %d stands on %v zoned %v", w.structureName(s), s.id, p, z)
			}
		}
	}
}

// One seed and the same commands make the same game, zoning included.
func TestZoningIsDeterministic(t *testing.T) {
	run := func() string {
		cfg := DefaultConfig()
		cfg.Seed, cfg.StartAliens = 3, 0
		cfg.Width, cfg.Height, cfg.StartColonists = 80, 50, 6
		w := newTestWorld(t, cfg)
		cx, cy := w.Width/2, w.Height/2
		w.paintZone(PaintZone{Kind: ZoneProduction, X0: cx - 15, Y0: cy - 9, X1: cx - 1, Y1: cy - 1})
		w.paintZone(PaintZone{Kind: ZoneResidence, X0: cx, Y0: cy - 9, X1: cx + 15, Y1: cy - 1})
		for i := 0; i < 1500; i++ {
			w.step()
			if i == 700 {
				w.paintZone(PaintZone{Kind: ZoneStorage, X0: cx - 4, Y0: cy - 9, X1: cx + 4, Y1: cy - 1})
			}
		}
		return goldenHash(w)
	}
	if a, b := run(), run(); a != b {
		t.Fatalf("two runs differ:\n%s\n%s", a, b)
	}
}

// Every fixture kind names a zone kind that exists, every room recipe takes
// its zone from its fixtures, and a chest's zone follows its role.
func TestEveryFixtureHasAZone(t *testing.T) {
	for _, k := range FixtureKinds() {
		if z := FixtureZone(k); z == NoZone || z >= numZoneKinds {
			t.Errorf("%s belongs in zone %v", k, z)
		}
	}
	for _, k := range []Terrain{NutrientPod, Toilet, Bed, Incinerator, Storage, Scumhouse, Forge, GunBench, Chair, Incubator, Trough} {
		if FixtureZone(k) == NoZone {
			t.Errorf("fixture %s has no zone", k)
		}
	}
	for _, tc := range []struct {
		r    roomRecipe
		want ZoneKind
	}{
		{lifeSupportRoom, ZoneResidence}, {toiletRoom, ZoneResidence}, {dormRoom, ZoneResidence},
		{hallRoom, ZoneResidence}, {houseRoom, ZoneResidence}, {storageRoom, ZoneStorage},
		{scumhouseRoom, ZoneProduction}, {incubatorRoom, ZoneProduction}, {trashRoom, ZoneProduction},
		{foundryRoom, ZoneProduction},
	} {
		if got := tc.r.zone(); got != tc.want {
			t.Errorf("recipe %q is zoned %v, want %v", tc.r.name, got, tc.want)
		}
	}
	for _, k := range ZoneKinds() {
		if got, ok := ParseZoneKind(k.String()); !ok || got != k || k.Color() == "" {
			t.Errorf("zone kind %v does not round-trip or has no colour", k)
		}
	}

	w := newTestWorld(t, testConfig())
	stove, pantry, chest := Point{3, 3, LandingLevel}, Point{5, 3, LandingLevel}, Point{9, 9, LandingLevel}
	w.pantryOf[stove], w.pantryHouse[pantry] = pantry, stove
	if z := w.fixtureZone(pantry, Storage); z != ZoneProduction {
		t.Errorf("a stove's pantry is %v, want production", z)
	}
	if z := w.fixtureZone(chest, Storage); z != ZoneStorage {
		t.Errorf("a chest on its own is %v, want storage", z)
	}
}
