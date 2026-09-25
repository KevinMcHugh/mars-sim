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

// SetTicksPerSecond changes the simulation speed. Rates below 1 are raised to 1;
// there is no upper cap.
type SetTicksPerSecond struct{ Rate int }

// Spawn injects a new entity of the given kind at a random valid location.
// Handy for stress-testing and for player actions later.
type Spawn struct{ Kind Kind }

// OrderFacilityRoom asks the planner to queue one life-support room.
type OrderFacilityRoom struct{}

// OrderDormitory asks the planner to queue one dormitory.
type OrderDormitory struct{}

// OrderTrashRoom asks the planner to queue one trash room (an incinerator to
// burn refuse in). The planner queues one on its own once there is refuse to
// clean up; this is how a player gets one built ahead of the first death.
type OrderTrashRoom struct{}

// OrderStorageRoom asks the planner to place one storage container in a small
// purpose-built room.
type OrderStorageRoom struct{}

func (TogglePause) isCommand()       {}
func (SetTicksPerSecond) isCommand() {}
func (Spawn) isCommand()             {}
func (OrderFacilityRoom) isCommand() {}
func (OrderDormitory) isCommand()    {}
func (OrderTrashRoom) isCommand()    {}
func (OrderStorageRoom) isCommand()  {}

// Engine drives the simulation. It owns the World and is the only goroutine that
// touches it. Frontends interact only through Subscribe (to receive Snapshots)
// and Send (to submit Commands), which makes the sim/render split clean enough
// to hang additional frontends off the same engine.
type Engine struct {
	world  *World
	cmds   chan Command
	tps    int
	paused bool
	perf   perfRecorder

	// lastPublish is when the last snapshot went out; see shouldPublish.
	lastPublish time.Time

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
//
// Ticks are paced against fixed deadlines rather than a time.Ticker. A ticker
// drops every tick its receiver was not ready for, so a wakeup that arrives
// late — routine on macOS, which coalesces timers to save power — costs a
// whole tick, and the achieved rate sinks to whatever cadence the OS actually
// delivers instead of the one asked for. Deadlines remember when each tick was
// due, so a late wakeup is made up by ticking again straight away.
func (e *Engine) Run(ctx context.Context) {
	interval := tickInterval(e.tps)
	due := time.Now().Add(interval)
	timer := time.NewTimer(interval)
	defer timer.Stop()

	// Publish the initial world so frontends have something to draw before the
	// first tick fires.
	e.publish()

	for {
		// Paused, there is nothing to time: wait for a command (resuming, most
		// likely) instead of waking every interval to skip a tick.
		if e.paused {
			select {
			case <-ctx.Done():
				e.closeSubs()
				return
			case cmd := <-e.cmds:
				e.apply(cmd)
				interval = tickInterval(e.tps)
				due = time.Now().Add(interval)
			}
			continue
		}

		// A tick that is already due runs without going to sleep first:
		// parking on a timer that has already expired still costs a goroutine
		// wakeup, and wakeups were over a fifth of one macOS profile at a few
		// hundred ticks a second. Commands and cancellation are still checked
		// between every tick, just without blocking.
		if !due.After(time.Now()) {
			select {
			case <-ctx.Done():
				e.closeSubs()
				return
			case cmd := <-e.cmds:
				if e.apply(cmd) {
					interval = tickInterval(e.tps)
					due = time.Now().Add(interval)
				}
				continue
			default:
			}
			e.tick()
			due = nextDue(due, time.Now(), interval)
			continue
		}
		timer.Reset(time.Until(due))

		select {
		case <-ctx.Done():
			e.closeSubs()
			return

		case cmd := <-e.cmds:
			if e.apply(cmd) {
				interval = tickInterval(e.tps)
				due = time.Now().Add(interval)
			}

		case <-timer.C:
			// Nothing to do here: the next pass through the loop finds the
			// tick due and runs it.
		}
	}
}

// maxTickLag is how far behind schedule the engine may fall before it stops
// trying to catch up. Within it, missed ticks are run back to back; beyond it
// (the machine cannot simulate this fast, or the process was suspended) the
// schedule restarts from now, so the sim runs flat out rather than bursting
// through a backlog it will never clear.
const maxTickLag = 250 * time.Millisecond

// nextDue returns when the tick after one due at due should run, given the
// time is now: one interval later, or now if that has already slipped more
// than maxTickLag into the past.
func nextDue(due, now time.Time, interval time.Duration) time.Time {
	next := due.Add(interval)
	if now.Sub(next) > maxTickLag {
		return now
	}
	return next
}

// tick advances the world one step, publishes it if a frontend could use a
// new frame yet, and records how long each half took for the Perf screen.
func (e *Engine) tick() {
	start := time.Now()
	e.perf.advance(start)
	e.world.step()
	stepped := time.Now()
	var published time.Duration
	if shouldPublish(e.tps, stepped.Sub(e.lastPublish)) {
		e.publish()
		published = time.Since(stepped)
	}
	e.perf.record(stepped.Sub(start), published)
}

// maxPublishRate caps how many snapshots a second the engine publishes. The
// TUI redraws at 30 fps and the one-slot subscription keeps only the newest
// frame, so at a few hundred ticks a second nearly every snapshot was built
// only to be thrown away, and every send woke the frontend's goroutine. A
// snapshot is not cheap on a big colony, and at high rates publishing was a
// third of the engine's time.
const maxPublishRate = 60

// shouldPublish reports whether a tick should publish, given the tick rate and
// how long it has been since the last snapshot. At or below maxPublishRate
// every tick publishes, so a slow game never skips a frame to timer jitter;
// above it, a tick publishes once a publish interval has passed.
func shouldPublish(tps int, sinceLast time.Duration) bool {
	return tps <= maxPublishRate || sinceLast >= time.Second/maxPublishRate
}

// apply handles one command and reports whether the tick interval changed (so
// Run can reset the ticker).
func (e *Engine) apply(cmd Command) (rateChanged bool) {
	switch c := cmd.(type) {
	case TogglePause:
		e.paused = !e.paused
		e.publish() // reflect the paused flag immediately
	case SetTicksPerSecond:
		e.tps = max(c.Rate, 1)
		e.publish()
		return true
	case Spawn:
		e.spawn(c.Kind)
		e.publish()
	case OrderFacilityRoom:
		e.world.manualFacilityRooms++
	case OrderDormitory:
		e.world.manualDormitories++
	case OrderTrashRoom:
		e.world.manualTrashRooms++
	case OrderStorageRoom:
		e.world.manualStorageRooms++
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
		if p, ok := w.alienSpawnSite(center, 8); ok {
			w.spawn(Alien, p)
		}
	case Cat:
		if p, ok := w.randomFloor(); ok {
			w.spawn(Cat, p)
		}
	case Mouse:
		if p, ok := w.randomFloor(); ok {
			w.spawn(Mouse, p)
		}
	}
}

// publish sends the current snapshot to every subscriber, replacing any frame a
// subscriber has not yet consumed so the latest state always wins.
func (e *Engine) publish() {
	// Closing buckets here as well as in tick means a frame published while
	// paused (a spawn, a speed change) shows the pause so far as the empty
	// buckets it is, rather than a history that stopped when ticking did.
	e.perf.advance(time.Now())
	e.lastPublish = time.Now()
	snap := e.world.snapshot(e.paused, e.tps)
	snap.Perf = e.perf.samples()
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
