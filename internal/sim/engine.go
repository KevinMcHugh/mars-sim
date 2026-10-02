package sim

import (
	"context"
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

// OrderScumhouse asks the planner to queue one scumhouse room.
type OrderScumhouse struct{}

// OrderIncubator asks the planner to queue one scum incubator room.
type OrderIncubator struct{}

// OrderFoundry asks the planner to queue one foundry: a forge and a gun bench.
type OrderFoundry struct{}

// OrderMeetingHall asks the planner to queue one meeting hall: a room of
// chairs where colonists socialize and eat.
type OrderMeetingHall struct{}

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
func (OrderScumhouse) isCommand()    {}
func (OrderIncubator) isCommand()    {}
func (OrderFoundry) isCommand()      {}
func (OrderMeetingHall) isCommand()  {}

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

	// flowShown is the flow field a frontend asked to see (ShowFlowField),
	// if flowShow is set. flowView is the last copy published, and
	// flowVersion the field version it was copied at, so a field that has
	// not changed is not copied again.
	flowShow    bool
	flowShown   FlowFieldRef
	flowView    *FlowFieldView
	flowVersion int

	// lastPublish is when the last snapshot went out; see shouldPublish.
	lastPublish time.Time

	// The tick schedule, shared by Run and Advance: ticks are interval apart
	// and the next one is due at due. See restartSchedule and runDueTick.
	interval time.Duration
	due      time.Time

	// stepping is set once Advance has been called: the host owns the loop.
	// Publishing then happens at most once per Advance, at its end, so no
	// frame is built only to be overwritten before the host sees it (see
	// requestPublish); publishWanted asks for that end-of-call frame.
	stepping      bool
	publishWanted bool

	mu   sync.Mutex
	subs []chan *Snapshot
	// liveTiles is set by ShareLiveTiles; it rules out Subscribe. It mirrors
	// the world's TileSharing so Subscribe can check it under mu without
	// touching the world, which belongs to the Run goroutine.
	liveTiles bool
	// running is set once Run or Advance starts; ShareLiveTiles is refused
	// after that.
	running bool
}

// NewEngine builds an engine with a freshly generated world.
func NewEngine(cfg Config) *Engine {
	w := newWorld(cfg, newPCG(cfg.Seed))
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
//
// Subscribe panics after ShareLiveTiles: a channel hands frames to another
// goroutine, and a live grid is only safe on the engine's own.
func (e *Engine) Subscribe() <-chan *Snapshot {
	ch := make(chan *Snapshot, 1)
	e.mu.Lock()
	if e.liveTiles {
		e.mu.Unlock()
		panic("sim: Subscribe after ShareLiveTiles; live tiles cannot cross goroutines")
	}
	e.subs = append(e.subs, ch)
	e.mu.Unlock()
	return ch
}

// ShareLiveTiles makes every later Snapshot's Tiles alias the live map
// (TilesLive) instead of a copy-on-write grid, which spares a frontend that
// reads frames on the engine's goroutine, between ticks, a second copy of the
// whole map. It is for the browser worker, whose encoder runs in the same
// thread as the engine (see docs/browser-frontend.md). The native TUI reads
// frames on its own goroutine and must keep the default.
//
// Call it before Run (or the first Advance), and never together with
// Subscribe: it panics if the engine has started (the world then belongs to
// whoever drives it) or a subscriber is already registered.
func (e *Engine) ShareLiveTiles() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		panic("sim: ShareLiveTiles after the engine started")
	}
	if len(e.subs) > 0 {
		panic("sim: ShareLiveTiles with a subscriber; live tiles cannot cross goroutines")
	}
	e.liveTiles = true
	e.world.SetTileSharing(TilesLive)
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
//
// Run and Advance share the schedule (restartSchedule, handle, runDueTick);
// Run adds only the blocking: on a timer, on commands, and on ctx.
func (e *Engine) Run(ctx context.Context) {
	if e.stepping {
		panic("sim: Run on an engine driven by Advance")
	}
	e.start()

	timer := time.NewTimer(e.interval)
	defer timer.Stop()

	for {
		// Paused, there is nothing to time: wait for a command (resuming, most
		// likely) instead of waking every interval to skip a tick.
		if e.paused {
			select {
			case <-ctx.Done():
				e.closeSubs()
				return
			case cmd := <-e.cmds:
				e.handle(cmd)
			}
			continue
		}

		// A tick that is already due runs without going to sleep first:
		// parking on a timer that has already expired still costs a goroutine
		// wakeup, and wakeups were over a fifth of one macOS profile at a few
		// hundred ticks a second. Commands and cancellation are still checked
		// between every tick, just without blocking.
		if !e.due.After(time.Now()) {
			select {
			case <-ctx.Done():
				e.closeSubs()
				return
			case cmd := <-e.cmds:
				e.handle(cmd)
				continue
			default:
			}
			e.runDueTick()
			continue
		}
		timer.Reset(time.Until(e.due))

		select {
		case <-ctx.Done():
			e.closeSubs()
			return

		case cmd := <-e.cmds:
			e.handle(cmd)

		case <-timer.C:
			// Nothing to do here: the next pass through the loop finds the
			// tick due and runs it.
		}
	}
}

// Advance is Run turned inside out, for a host that owns the event loop and
// cannot give the engine a goroutine that blocks: the browser worker (see
// docs/browser-frontend.md). Go's WASM runtime only hands control back to
// JavaScript when every goroutine is blocked, so Run ticking flat out would
// starve the worker's message handler, and pausing it would never arrive.
//
// Each call applies the commands sent since the last one, then runs the ticks
// that are due, stopping once budget has elapsed. At least one due tick runs
// per call, however small the budget, so a slow tick cannot stall the game.
// The first call also publishes the initial frame.
//
// It returns the snapshot published at the end of the call, or nil if there was
// none: a call publishes at most once, under the same frame-rate cap as Run
// (shouldPublish), or because a command asked for a fresh frame. A tick whose
// changes were not published is carried by the next frame, because
// TileChanges accumulates until a publish; so every frame reaches the host
// and TileChanges.Frame never skips. It also returns how long the host should
// wait before calling again: 0 means a tick is already due, and a negative
// wait means the engine is paused and only a command (Send, then Advance)
// will change anything.
//
// Advance never blocks, and it must be called from one goroutine only. It does
// not mix with Run.
func (e *Engine) Advance(budget time.Duration) (snap *Snapshot, wait time.Duration) {
	if !e.stepping {
		e.mu.Lock()
		byRun := e.running
		e.mu.Unlock()
		if byRun {
			panic("sim: Advance on an engine driven by Run")
		}
		e.stepping = true
		e.start()
	}
	deadline := time.Now().Add(budget)
	ticked := false
	for {
		e.drainCommands()
		if e.paused {
			wait = -1
			break
		}
		now := time.Now()
		if e.due.After(now) {
			wait = e.due.Sub(now)
			break
		}
		if ticked && !now.Before(deadline) {
			break // out of budget with a tick still due: wait stays 0
		}
		e.runDueTick()
		ticked = true
	}
	if e.publishWanted || ticked && shouldPublish(e.tps, time.Since(e.lastPublish)) {
		e.publishWanted = false
		start := time.Now()
		snap = e.publish()
		if ticked {
			e.perf.addPublish(time.Since(start))
		}
	}
	return snap, wait
}

// start marks the engine running, schedules the first tick, and publishes the
// initial world so frontends have something to draw before it fires.
func (e *Engine) start() {
	e.mu.Lock()
	e.running = true
	e.mu.Unlock()
	e.restartSchedule()
	e.requestPublish()
}

// restartSchedule puts the next tick one interval from now at the current
// rate, forgetting any backlog.
func (e *Engine) restartSchedule() {
	e.interval = tickInterval(e.tps)
	e.due = time.Now().Add(e.interval)
}

// handle applies one command. A rate change restarts the schedule at the new
// interval, and so does anything received while paused: a resume should wait
// one interval rather than burst through the time spent paused.
func (e *Engine) handle(cmd Command) {
	wasPaused := e.paused
	if e.apply(cmd) || wasPaused {
		e.restartSchedule()
	}
}

// drainCommands handles every command waiting in the buffer, without
// blocking.
func (e *Engine) drainCommands() {
	for {
		select {
		case cmd := <-e.cmds:
			e.handle(cmd)
		default:
			return
		}
	}
}

// runDueTick runs the tick that is due and schedules the next.
func (e *Engine) runDueTick() {
	e.tick()
	e.due = nextDue(e.due, time.Now(), e.interval)
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
	if !e.stepping && shouldPublish(e.tps, stepped.Sub(e.lastPublish)) {
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
// handle can restart the schedule).
func (e *Engine) apply(cmd Command) (rateChanged bool) {
	switch c := cmd.(type) {
	case TogglePause:
		e.paused = !e.paused
		e.requestPublish() // reflect the paused flag immediately
	case SetTicksPerSecond:
		e.tps = max(c.Rate, 1)
		e.requestPublish()
		return true
	case Spawn:
		e.spawn(c.Kind)
		e.requestPublish()
	case OrderFacilityRoom:
		e.world.manualFacilityRooms++
	case OrderDormitory:
		e.world.manualDormitories++
	case OrderTrashRoom:
		e.world.manualTrashRooms++
	case OrderStorageRoom:
		e.world.manualStorageRooms++
	case OrderScumhouse:
		e.world.manualScumhouses++
	case OrderFoundry:
		e.world.manualFoundries++
	case ShowFlowField:
		e.flowShow, e.flowShown = c.Show, c.Field
		e.requestPublish()
	case OrderMeetingHall:
		e.world.manualHalls++
	case OrderIncubator:
		e.world.manualIncubators++
	case CancelExcavation:
		e.world.cancelExcavation(c.ID)
		e.requestPublish()
	case PlaceColonyOrder:
		e.world.placeColonyOrder(c)
		e.requestPublish()
	case RepriceColonyOrder:
		e.world.repriceColonyOrder(c)
		e.requestPublish()
	case CancelColonyOrder:
		e.world.cancelColonyOrder(c.ID)
		e.requestPublish()
	case SuspendColonyOrders:
		e.world.suspendColonyOrders(c)
		e.requestPublish()
	case ResumeColonyOrders:
		e.world.resumeColonyOrders(c)
		e.requestPublish()
	case OrderExcavation:
		e.world.orderExcavation(c)
		e.requestPublish() // the log line and the work order show at once
	}
	return false
}

func (e *Engine) spawn(kind Kind) {
	w := e.world
	center := Point{w.Width / 2, w.Height / 2}
	switch kind {
	case Colonist:
		w.arrive(true) // every colonist comes in a crash pod
	case Alien:
		if p, ok := w.alienSpawnSite(center, 8); ok {
			w.spawn(Alien, p)
		}
	case Cat:
		if p, ok := w.randomFloor(); ok {
			w.spawn(Cat, p)
		}
	case Rat:
		if p, ok := w.randomFloor(); ok {
			w.spawn(Rat, p)
		}
	}
}

// publish sends the current snapshot to every subscriber, replacing any frame a
// subscriber has not yet consumed so the latest state always wins.
func (e *Engine) publish() *Snapshot {
	// Closing buckets here as well as in tick means a frame published while
	// paused (a spawn, a speed change) shows the pause so far as the empty
	// buckets it is, rather than a history that stopped when ticking did.
	e.perf.advance(time.Now())
	e.lastPublish = time.Now()
	snap := e.world.snapshot(e.paused, e.tps)
	snap.Perf = e.perf.samples()
	snap.FlowFields = e.world.flowFieldRefs()
	if e.flowShow {
		e.flowView, e.flowVersion = e.world.flowFieldView(e.flowShown, e.flowView, e.flowVersion)
		snap.FlowField = e.flowView
	} else {
		e.flowView = nil // let the copy go; showing it again copies afresh
	}
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
	return snap
}

// requestPublish asks for a fresh frame now. Under Run that is a publish on
// the spot; under Advance it is deferred to the end of the call, so one call
// never publishes twice and a frame is never overwritten unseen.
func (e *Engine) requestPublish() {
	if e.stepping {
		e.publishWanted = true
		return
	}
	e.publish()
}

func (e *Engine) closeSubs() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ch := range e.subs {
		close(ch)
	}
	e.subs = nil
}
