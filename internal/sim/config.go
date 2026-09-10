package sim

import "time"

// Config holds every tunable knob for a simulation run in one place, so
// balancing the game means editing values here rather than hunting through the
// systems. Zero values are not meaningful; use DefaultConfig and adjust.
type Config struct {
	// World shape.
	Width, Height int

	// Seed makes a run reproducible. Same seed + same code => same game.
	Seed int64

	// Starting population.
	StartColonists int
	StartAliens    int

	// Timing.
	TicksPerSecond int // default simulation speed
	LogSize        int // how many recent events to retain

	// Colonist stats.
	ColonistHP         int
	MineTicks          int // ticks of work to excavate one Rock tile
	BuildTicks         int // ticks of work to raise one Wall
	FacilityBuildTicks int // ticks of work to build a nutrient pod or toilet
	BuildChance        int // percent chance an idle colonist builds a wall vs. mines
	FleeRadius         int // flee when an alien is within this many tiles

	// Needs. One NeedSpec per NeedKind, indexed by that kind.
	Needs                [numNeeds]NeedSpec
	StarveDamage         int // HP lost per tick while a Fatal need sits at Max
	ColonistsPerFacility int // desired colonists served by each facility (min 1 built)
	RestTicks            int // ticks an idle colonist rests before re-checking for work
	StuckLimit           int // ticks a colonist waits on a blocked path before abandoning the job

	// Alien stats.
	AlienHP       int
	AlienDamage   int // HP removed per bite
	AlienBiteRest int // cooldown ticks between bites
	AlienSlowness int // alien acts once every N ticks (>=1); higher is slower
}

// DefaultConfig returns a balanced starting point for a playable scaffold.
func DefaultConfig() Config {
	return Config{
		Width:              80,
		Height:             40,
		Seed:               time.Now().UnixNano(),
		StartColonists:     6,
		StartAliens:        3,
		TicksPerSecond:     8,
		LogSize:            64,
		ColonistHP:         40,
		MineTicks:          6,
		BuildTicks:         8,
		FacilityBuildTicks: 12,
		BuildChance:        25,
		FleeRadius:         5,

		StarveDamage:         1,
		ColonistsPerFacility: 3,
		RestTicks:            10,
		StuckLimit:           8,
		Needs: [numNeeds]NeedSpec{
			NeedFood: {
				Name: "food", Rise: 2, SeekAt: 650, Max: 1000,
				Facility: NutrientPod, UseTicks: 18, Fatal: true,
			},
			NeedBladder: {
				Name: "bladder", Rise: 3, SeekAt: 600, Max: 1000,
				Facility: Toilet, UseTicks: 10, Fatal: false,
			},
		},
		AlienHP:       30,
		AlienDamage:   6,
		AlienBiteRest: 3,
		AlienSlowness: 2,
	}
}

// tickInterval converts a ticks-per-second rate into a sleep duration, clamped
// to something sane.
func tickInterval(ticksPerSecond int) time.Duration {
	if ticksPerSecond < 1 {
		ticksPerSecond = 1
	}
	if ticksPerSecond > 60 {
		ticksPerSecond = 60
	}
	return time.Second / time.Duration(ticksPerSecond)
}
