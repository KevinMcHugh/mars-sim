package sim

import "testing"

// A colonist standing on an access tile of a facility is not a queue for it:
// at its own pod's door it would otherwise see its own bunk as taken and
// flood the whole room looking for another.
func TestAColonistDoesNotQueueBehindItself(t *testing.T) {
	w := propertyWorld(t)
	bed := Point{10, 6}
	w.SetTerrain(bed, Bed)
	w.refreshSpatial()
	e := w.spawn(Colonist, Point{10, 7})
	all := func(Point) bool { return true }
	if w.facilityCongested(e, bed, all) {
		t.Fatal("a colonist on the bed's access tile counted itself as a queue")
	}
	other := w.spawn(Colonist, Point{11, 7})
	if !w.facilityCongested(other, bed, all) {
		t.Fatal("another colonist on the access tile should count")
	}
}
