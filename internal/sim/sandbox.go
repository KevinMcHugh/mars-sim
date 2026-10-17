package sim

import "fmt"

// Place puts a new creature of Kind on the tile At: the sandbox's one tool
// (Config.Sandbox; see docs/sandbox.md). Species is an alien's species, an
// index into the roster (Snapshot.AlienSpecies), or -1 for one drawn at
// random; other kinds ignore it. Outside a sandbox, or on a tile that is not
// free, discovered floor, the engine refuses and says why in the log.
type Place struct {
	Kind    Kind
	At      Point
	Species int
}

func (Place) isCommand() {}

// place carries out a Place command. A placed alien is awake from the first
// tick, since it stands on floor the colony knows. A placed colonist arrives
// as a ship's passenger would (see outfitPlaced), minus the ship.
func (w *World) place(c Place) {
	if why := w.placeRefusal(c); why != "" {
		w.logEventAt(LogNote, c.At, why)
		return
	}
	species := c.Species
	if c.Kind == Alien && species < 0 {
		// An ordinary gameplay draw, as for any alien spawn (see spawn).
		species = w.rng.IntN(len(w.alienSpecies))
	}
	e := w.spawnAs(c.Kind, c.At, species)
	if e.Kind == Colonist {
		w.outfitPlaced(e)
	}
	w.logEventAt(LogArrival, c.At, fmt.Sprintf("Sandbox: %s is placed here.", w.placedNoun(e)))
}

// placeRefusal is why a Place cannot go ahead, as a log line, or "" when it
// can. Only free floor the colony has discovered takes a creature: a tile in
// a cavern nobody has found would hold a dormant creature the colony cannot
// see or reach (see dormant), which is no use to a sandbox.
func (w *World) placeRefusal(c Place) string {
	if !w.cfg.Sandbox {
		return "Creatures can only be placed by hand in a sandbox game."
	}
	if c.Kind < 0 || c.Kind >= numKinds {
		return fmt.Sprintf("There is no creature kind %d to place.", c.Kind)
	}
	what := withArticle(c.Kind.String())
	if c.Kind == Alien {
		if len(w.alienSpecies) == 0 {
			return "This world has no alien species to place."
		}
		if c.Species >= len(w.alienSpecies) {
			return fmt.Sprintf("This world has no alien species %d.", c.Species)
		}
	}
	l := w.layerIn(c.At)
	switch {
	case l == nil:
		return fmt.Sprintf("Can't place %s off the map, or on a level the colony hasn't reached.", what)
	case !w.Walkable(c.At):
		return fmt.Sprintf("Can't place %s on %s: only on open floor.", what, l.tiles.at(c.At.X, c.At.Y).Terrain)
	case !w.discovered(c.At):
		return fmt.Sprintf("Can't place %s in a cavern the colony hasn't found.", what)
	case w.occupied(c.At):
		return fmt.Sprintf("Can't place %s there: something already stands on that tile.", what)
	}
	return ""
}

// placedNoun names a creature just placed, for the log: a colonist by name,
// an alien by its species' form ("a hostile grub"), anything else by kind.
func (w *World) placedNoun(e *Entity) string {
	switch {
	case e.Kind == Colonist:
		return e.displayName()
	case e.Kind == Alien:
		return w.alienNounFor(e)
	}
	return withArticle(e.Kind.String())
}

// outfitPlaced gives a placed colonist what a ship's passenger lands with,
// as far as it can go without a ship: a background and an employer, the
// meals its locker would have held carried in its pockets (as many as fit),
// and its gun if its rare item is one. A chicken or a cat needs the ship's
// trough or hold, so it stays behind. Without the meals a sandbox colonist
// starved inside a day: with no ship there is no locker, and an empty
// sandbox has no kitchen or silo to buy from.
func (w *World) outfitPlaced(e *Entity) {
	w.rollBackground(e)
	w.rollEmployer(e)
	n := w.arrivalMeals(e.ID)
	for n > 0 && !e.Inventory.CanAdd(Meal, n) {
		n--
	}
	if n > 0 {
		e.Inventory.Add(Meal, n)
	}
	if w.arrivalRareItem(e.ID) == rareGun {
		e.Inventory.Add(w.arrivalGun(e.ID), 1)
	}
}
