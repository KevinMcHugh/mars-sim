package sim

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

// Command is a message a frontend sends to the engine to influence the running
// simulation. Commands are applied on the engine goroutine between ticks, so
// they never race with world mutation.
type Command interface{ isCommand() }

// TogglePause pauses or resumes ticking.
type TogglePause struct{}

// SetTicksPerSecond changes the simulation speed. Values are clamped.
type SetTicksPerSecond struct{ Rate int }

// Spawn injects a new entity of the given kind at a random valid location.
// Handy for stress-testing and for player actions later.
type Spawn struct{ Kind Kind }

func (TogglePause) isCommand()       {}
func (SetTicksPerSecond) isCommand() {}
func (Spawn) isCommand()             {}

// Engine drives the simulation. It owns the World and is the only goroutine that
// touches it. Frontends interact only through Subscribe (to receive Snapshots)
// and Send (to submit Commands), which makes the sim/render split clean enough
// to hang additional frontends off the same engine.
type Engine struct {
	world  *World
	cmds   chan Command
	tps    int
	paused bool

	mu   sync.Mutex
	subs []chan *Snapshot
}

// NewEngine builds an engine with a freshly generated world.
func NewEngine(cfg Config) *Engine {
	rng := rand.New(rand.NewSource(cfg.Seed))
	w := newWorld(cfg, rng)
	generate(w)
	return &Engine{
		world: w,
		cmds:  make(chan Command, 32),
		tps:   cfg.TicksPerSecond,
	}
}

// Subscribe registers a new frontend and returns a channel of Snapshots. The
// channel has capacity 1 and the engine drops stale frames rather than blocking,
// so a slow renderer can never stall the simulation. Call before Run so the
// initial frame is not missed.
func (e *Engine) Subscribe() <-chan *Snapshot {
	ch := make(chan *Snapshot, 1)
	e.mu.Lock()
	e.subs = append(e.subs, ch)
	e.mu.Unlock()
	return ch
}

// Send submits a command. It never blocks: if the command buffer is full the
// command is dropped, which is acceptable for the interactive controls we have.
func (e *Engine) Send(cmd Command) {
	select {
	case e.cmds <- cmd:
	default:
	}
}

// Run drives the tick loop until ctx is cancelled. It is meant to be launched in
// its own goroutine: go engine.Run(ctx).
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(tickInterval(e.tps))
	defer ticker.Stop()

	// Publish the initial world so frontends have something to draw before the
	// first tick fires.
	e.publish()

	for {
		select {
		case <-ctx.Done():
			e.closeSubs()
			return

		case cmd := <-e.cmds:
			if e.apply(cmd) {
				ticker.Reset(tickInterval(e.tps))
			}

		case <-ticker.C:
			if !e.paused {
				e.world.step()
				e.publish()
			}
		}
	}
}

// apply handles one command and reports whether the tick interval changed (so
// Run can reset the ticker).
func (e *Engine) apply(cmd Command) (rateChanged bool) {
	switch c := cmd.(type) {
	case TogglePause:
		e.paused = !e.paused
		e.publish() // reflect the paused flag immediately
	case SetTicksPerSecond:
		e.tps = clamp(c.Rate, 1, 60)
		e.publish()
		return true
	case Spawn:
		e.spawn(c.Kind)
		e.publish()
	}
	return false
}

func (e *Engine) spawn(kind Kind) {
	w := e.world
	center := Point{w.Width / 2, w.Height / 2}
	switch kind {
	case Colonist:
		if p, ok := w.randomFloor(); ok {
			w.spawn(Colonist, p)
		}
	case Alien:
		if p, ok := w.randomRockFar(center, 8); ok {
			w.spawn(Alien, p)
		}
	}
}

// publish sends the current snapshot to every subscriber, replacing any frame a
// subscriber has not yet consumed so the latest state always wins.
func (e *Engine) publish() {
	snap := e.world.snapshot(e.paused, e.tps)
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ch := range e.subs {
		select {
		case ch <- snap:
		default:
			// Drop the stale frame the subscriber has not read, then post the
			// fresh one.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- snap:
			default:
			}
		}
	}
}

func (e *Engine) closeSubs() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ch := range e.subs {
		close(ch)
	}
	e.subs = nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
