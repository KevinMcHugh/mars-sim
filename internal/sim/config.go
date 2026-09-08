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
	ColonistHP  int
	MineTicks   int // ticks of work to excavate one Rock tile
	BuildTicks  int // ticks of work to raise one Wall
	BuildChance int // percent chance an idle colonist builds vs. mines
	FleeRadius  int // flee when an alien is within this many tiles

	// Alien stats.
	AlienHP       int
	AlienDamage   int // HP removed per bite
	AlienBiteRest int // cooldown ticks between bites
	AlienSlowness int // alien acts once every N ticks (>=1); higher is slower
}

// DefaultConfig returns a balanced starting point for a playable scaffold.
func DefaultConfig() Config {
	return Config{
		Width:          80,
		Height:         40,
		Seed:           time.Now().UnixNano(),
		StartColonists: 6,
		StartAliens:    3,
		TicksPerSecond: 8,
		LogSize:        64,
		ColonistHP:     40,
		MineTicks:      6,
		BuildTicks:     8,
		BuildChance:    25,
		FleeRadius:     5,
		AlienHP:        30,
		AlienDamage:    6,
		AlienBiteRest:  3,
		AlienSlowness:  2,
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
