package sim

// WorldEvent is something that happened in the world that other systems may
// react to. It is the backbone of the reactive design: producers emit events
// and derived systems (the job board, and more to come) update from them
// instead of rescanning the world each tick.
//
// The bus is deliberately tiny and synchronous — handlers run on the engine
// goroutine during the emitting call, so there is no locking and no ordering
// surprise. Boxing a value into the WorldEvent interface allocates, so emit
// only on real changes (SetTerrain already no-ops when terrain is unchanged).
type WorldEvent interface{ isWorldEvent() }

// TileChanged reports that a single tile's terrain changed. Old and New let a
// handler tell digging (Rock->Floor) from building (Floor->Wall/facility)
// without re-reading the map.
type TileChanged struct {
	Pos      Point
	Old, New Terrain
}

func (TileChanged) isWorldEvent() {}

// subscribe registers a handler to receive every future event.
func (w *World) subscribe(fn func(WorldEvent)) {
	w.subscribers = append(w.subscribers, fn)
}

// emit delivers an event to every subscriber, in registration order.
func (w *World) emit(e WorldEvent) {
	for _, fn := range w.subscribers {
		fn(e)
	}
}
