package sim

import (
	"fmt"
	"math/rand"
	"sort"

	"gopkg.in/yaml.v3"
)

// DirectorFileName is the schedule file mars-sim looks for in the working
// directory, alongside mars-sim.yaml. It is optional: a run with no file (or
// an empty schedule list) plays with the director doing nothing, exactly as
// the game did before it existed. See docs/director.md.
const DirectorFileName = "director.yaml"

// OccurrenceKind identifies a major happening the director can schedule.
type OccurrenceKind uint8

const (
	OccMousePlague OccurrenceKind = iota
	OccAlienSwarm
	OccSupplyDrop
)

func (k OccurrenceKind) String() string {
	switch k {
	case OccMousePlague:
		return "mouse-plague"
	case OccAlienSwarm:
		return "alien-swarm"
	case OccSupplyDrop:
		return "supply-drop"
	default:
		return "unknown"
	}
}

func parseOccurrenceKind(s string) (OccurrenceKind, bool) {
	switch s {
	case "mouse-plague":
		return OccMousePlague, true
	case "alien-swarm":
		return OccAlienSwarm, true
	case "supply-drop":
		return OccSupplyDrop, true
	default:
		return 0, false
	}
}

// Occurrence is one candidate thing the director can make happen: a kind plus
// the parameters that kind reads. Only the fields a given Kind actually uses
// are meaningful — Count for a plague/swarm, Pistols/Shotguns for a supply
// drop — the rest sit at zero.
type Occurrence struct {
	Kind     OccurrenceKind
	Count    int // OccMousePlague: mice spawned. OccAlienSwarm: aliens spawned.
	Pistols  int // OccSupplyDrop only.
	Shotguns int // OccSupplyDrop only.
}

// Schedule is one entry from director.yaml: a tick window, and the candidate
// Occurrences that might happen in it. At world creation exactly one
// candidate is chosen — flat (uniform) odds across the list — to fire at a
// tick drawn uniformly from [EarliestTick, LatestTick].
//
// The window and the candidates are deliberately two separate lists rather
// than one flat table of "event, tick": that is what lets a schedule say
// "something happens between tick 1000 and 1500" without committing, at the
// point the window is authored, to which of several things it will be.
type Schedule struct {
	Name         string
	EarliestTick int
	LatestTick   int
	Occurrences  []Occurrence
}

// scheduledEvent is a Schedule resolved down to one concrete firing.
// Resolution (which candidate, which tick) happens once, off the simulation
// RNG stream, at world creation — see resolveSchedules.
type scheduledEvent struct {
	Tick       int
	Name       string
	Occurrence Occurrence
}

// resolveSchedules rolls every Schedule down to a single scheduledEvent and
// returns them sorted by tick (ties keep schedule order). It draws from rng
// — the simulation stream, not the personality one — because which creatures
// spawn and when is gameplay, not flavor: the same seed must produce the same
// director script every run. See docs/personality.md for the stream split.
func resolveSchedules(schedules []Schedule, rng *rand.Rand) []scheduledEvent {
	if len(schedules) == 0 {
		return nil
	}
	events := make([]scheduledEvent, len(schedules))
	for i, s := range schedules {
		occ := s.Occurrences[rng.Intn(len(s.Occurrences))]
		tick := s.EarliestTick
		if span := s.LatestTick - s.EarliestTick; span > 0 {
			tick += rng.Intn(span + 1)
		}
		events[i] = scheduledEvent{Tick: tick, Name: s.Name, Occurrence: occ}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Tick < events[j].Tick })
	return events
}

// runDirector fires every scheduled event whose tick has arrived. The queue
// is sorted by tick at resolution time, so directorNext only ever advances.
func (w *World) runDirector() {
	for w.directorNext < len(w.directorQueue) && w.directorQueue[w.directorNext].Tick <= w.tick {
		w.fireOccurrence(w.directorQueue[w.directorNext])
		w.directorNext++
	}
}

func (w *World) fireOccurrence(ev scheduledEvent) {
	switch ev.Occurrence.Kind {
	case OccMousePlague:
		w.fireMousePlague(ev)
	case OccAlienSwarm:
		w.fireAlienSwarm(ev)
	case OccSupplyDrop:
		w.fireSupplyDrop(ev)
	}
}

// fireMousePlague drops a wave of mice onto open floor, the same way starting
// mice are placed in generate().
func (w *World) fireMousePlague(ev scheduledEvent) {
	spawned := 0
	for i := 0; i < ev.Occurrence.Count; i++ {
		p, ok := w.randomFloor()
		if !ok {
			break // the colony has no open floor left; stop rather than loop forever
		}
		w.spawn(Mouse, p)
		spawned++
	}
	w.log.add(fmt.Sprintf("%s: a plague of mice pours into the colony (%d mice).", ev.Name, spawned))
}

// fireAlienSwarm drops a wave of aliens into the rock around the colony, the
// same way the starting aliens are placed in generate() and Engine.spawn.
func (w *World) fireAlienSwarm(ev scheduledEvent) {
	center := Point{w.Width / 2, w.Height / 2}
	spawned := 0
	var first *Entity
	for i := 0; i < ev.Occurrence.Count; i++ {
		p, ok := w.randomRockFar(center, 8)
		if !ok {
			break
		}
		a := w.spawn(Alien, p)
		if first == nil {
			first = a
		}
		spawned++
	}
	// With more than one species rolled for this world (AlienSpeciesCount),
	// a swarm can be a mix; naming it after whichever spawned first is a
	// deliberate simplification for one flavor line rather than a precise
	// species-by-species breakdown.
	noun := "aliens"
	if first != nil {
		noun = w.alienPluralFor(first)
	}
	w.log.add(fmt.Sprintf("%s: a swarm of %s burrows toward the colony (%d).", ev.Name, noun, spawned))
}

// fireSupplyDrop hands out weapons to living colonists, the same way the
// colony ship's starting firearms are issued in equipColonyShip: spread
// across distinct colonists rather than piled onto one, drawn in random
// order so who gets armed is not predictable from ID. A colonist with no
// room in their inventory is simply skipped; any weapon that cannot be
// placed is lost rather than left to accumulate somewhere with no holder.
func (w *World) fireSupplyDrop(ev scheduledEvent) {
	recipients := w.livingColonistsShuffled()
	issue := func(kind ItemKind, count int) int {
		issued := 0
		for _, e := range recipients {
			if issued >= count {
				break
			}
			if e.Inventory.Add(kind, 1) {
				issued++
			}
		}
		return issued
	}
	shotguns := issue(Shotgun, ev.Occurrence.Shotguns)
	pistols := issue(Pistol, ev.Occurrence.Pistols)
	w.log.add(fmt.Sprintf("%s: a supply drop lands (%d shotguns, %d pistols).", ev.Name, shotguns, pistols))
}

// livingColonistsShuffled returns every living colonist in random order. IDs
// are sorted before the shuffle so the draw is reproducible from the seed —
// Go's map iteration order is not (see the same pattern in
// entityIDsSorted).
func (w *World) livingColonistsShuffled() []*Entity {
	ids := make([]EntityID, 0, len(w.kindEntities[Colonist]))
	for id := range w.kindEntities[Colonist] {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	w.rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	out := make([]*Entity, len(ids))
	for i, id := range ids {
		out[i] = w.entities[id]
	}
	return out
}

// ---- director.yaml -----------------------------------------------------------

type rawOccurrence struct {
	Kind     string `yaml:"kind"`
	Count    int    `yaml:"count"`
	Pistols  int    `yaml:"pistols"`
	Shotguns int    `yaml:"shotguns"`
}

type rawSchedule struct {
	Name         string          `yaml:"name"`
	EarliestTick int             `yaml:"earliest_tick"`
	LatestTick   int             `yaml:"latest_tick"`
	Occurrences  []rawOccurrence `yaml:"occurrences"`
}

type rawSchedules struct {
	Schedules []rawSchedule `yaml:"schedules"`
}

// LoadSchedules parses a director.yaml document into the director's schedule
// list. name is used in error messages.
func LoadSchedules(data []byte, name string) ([]Schedule, error) {
	var raw rawSchedules
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(raw.Schedules) == 0 {
		return nil, nil
	}
	schedules := make([]Schedule, len(raw.Schedules))
	for i, rs := range raw.Schedules {
		s, err := rs.toSchedule()
		if err != nil {
			label := rs.Name
			if label == "" {
				label = fmt.Sprintf("#%d", i)
			}
			return nil, fmt.Errorf("%s: schedule %s: %w", name, label, err)
		}
		schedules[i] = s
	}
	return schedules, nil
}

func (rs rawSchedule) toSchedule() (Schedule, error) {
	if rs.Name == "" {
		return Schedule{}, fmt.Errorf("name is required")
	}
	if rs.EarliestTick < 0 {
		return Schedule{}, fmt.Errorf("earliest_tick cannot be negative (got %d)", rs.EarliestTick)
	}
	if rs.LatestTick < rs.EarliestTick {
		return Schedule{}, fmt.Errorf("latest_tick (%d) cannot be before earliest_tick (%d)", rs.LatestTick, rs.EarliestTick)
	}
	if len(rs.Occurrences) == 0 {
		return Schedule{}, fmt.Errorf("needs at least one occurrence")
	}
	occs := make([]Occurrence, len(rs.Occurrences))
	for i, ro := range rs.Occurrences {
		kind, ok := parseOccurrenceKind(ro.Kind)
		if !ok {
			return Schedule{}, fmt.Errorf("occurrence %d: unknown kind %q (want mouse-plague, alien-swarm or supply-drop)", i, ro.Kind)
		}
		switch kind {
		case OccMousePlague, OccAlienSwarm:
			if ro.Count < 1 {
				return Schedule{}, fmt.Errorf("occurrence %d: %s needs count >= 1 (got %d)", i, kind, ro.Count)
			}
		case OccSupplyDrop:
			if ro.Pistols < 0 || ro.Shotguns < 0 {
				return Schedule{}, fmt.Errorf("occurrence %d: supply-drop quantities cannot be negative", i)
			}
			if ro.Pistols == 0 && ro.Shotguns == 0 {
				return Schedule{}, fmt.Errorf("occurrence %d: supply-drop needs at least one pistol or shotgun", i)
			}
		}
		occs[i] = Occurrence{Kind: kind, Count: ro.Count, Pistols: ro.Pistols, Shotguns: ro.Shotguns}
	}
	return Schedule{Name: rs.Name, EarliestTick: rs.EarliestTick, LatestTick: rs.LatestTick, Occurrences: occs}, nil
}
