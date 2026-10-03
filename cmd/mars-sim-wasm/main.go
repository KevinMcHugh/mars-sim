//go:build js && wasm

// The browser build of the engine, for a Web Worker (see
// docs/browser-frontend.md). Build it with:
//
//	web/build-wasm.sh (then npm run dev in web/)
//
// It exports these on globalThis.marssim:
//
//	start(settings)        new game; settings is a mars-sim.yaml mapping as
//	                       JSON. Returns JSON: timings and the wire Hello.
//	advance(ms)            run the ticks due within a budget (see
//	                       sim.Engine.Advance). Returns {wait, frame, perf,
//	                       topics}: frame is a wire frame (Uint8Array) or
//	                       null; topics is JSON of the panel topics due, or
//	                       null (see wire.Topics).
//	interest(x0,y0,x1,y1)  the map region the page shows, in tiles
//	subscribe(topic)       start sending a panel's topic ("lore")
//	unsubscribe(topic)     stop sending it
//	send(command)          queue a command (JSON), applied at the next advance
//	memory()               the Go heap, as JSON
//
// The JS side owns the loop (web/public/worker.js): it calls advance, posts the
// frame if there is one, and calls again after the returned wait.
package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"syscall/js"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/wire"
)

// hostAPI versions this file's exports: bump it whenever one is added,
// removed or changes shape, together with HOST_API in web/src/sim/client.ts.
// The page checks it at start, so a mars-sim.wasm left over from an older
// build (npm run wasm not rerun after a pull) fails with a message saying so,
// instead of a panel that silently never loads. 1 was everything before
// subscribe/unsubscribe; 2 had no entity: or tile: topics; 3 no roster; 4 no log; 5 no jobs, storage, market or account:; 6 no perf or population; 7 no flow command; 8 no dig command; 9 no dig-cancel; 10 no order-place, order-reprice or order-cancel; 11 no order-suspend or order-resume; 12 no ship-move; 13 no ship-land; 14 no zone, clear or clear-cancel; 15 no recruit-roll or recruit-hire.
const hostAPI = 16

var (
	eng *sim.Engine
	enc *wire.Encoder
	// last is the newest snapshot the engine published. With live tiles its
	// terrain is the live map, valid until the next tick; advance re-encodes
	// it between ticks when the page is owed pages and no new frame came.
	last *sim.Snapshot
	// interestMoved says the view changed since the last frame, so the next
	// advance sends a frame even without a new snapshot.
	interestMoved bool
	// topics are the panels the page has open; see wire.Topics. They outlive
	// a new game: an open panel should keep receiving its data.
	topics = wire.NewTopics()
)

func main() {
	api := js.Global().Get("Object").New()
	api.Set("api", hostAPI)
	api.Set("start", js.FuncOf(func(_ js.Value, args []js.Value) any {
		settings := "{}"
		if len(args) > 0 && args[0].Type() == js.TypeString {
			settings = args[0].String()
		}
		return toJSON(start(settings))
	}))
	api.Set("advance", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if eng == nil {
			return toJSON(errorResult("advance before start"))
		}
		budget := 8 * time.Millisecond
		if len(args) > 0 && args[0].Type() == js.TypeNumber {
			budget = time.Duration(args[0].Float() * float64(time.Millisecond))
		}
		return advance(budget)
	}))
	api.Set("subscribe", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return toJSON(errorResult("subscribe needs a topic"))
		}
		if err := topics.Subscribe(args[0].String()); err != nil {
			return toJSON(errorResult(err.Error()))
		}
		return toJSON(struct{}{})
	}))
	api.Set("unsubscribe", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			topics.Unsubscribe(args[0].String())
		}
		return toJSON(struct{}{})
	}))
	api.Set("interest", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if enc == nil || len(args) < 4 {
			return toJSON(errorResult("interest needs a started engine and x0, y0, x1, y1"))
		}
		enc.SetInterest(wire.Rect{X0: args[0].Int(), Y0: args[1].Int(), X1: args[2].Int(), Y1: args[3].Int()})
		interestMoved = true
		return toJSON(struct{}{})
	}))
	api.Set("send", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if eng == nil || len(args) < 1 {
			return toJSON(errorResult("send needs a started engine and a command"))
		}
		cmd, err := parseCommand(args[0].String())
		if err != nil {
			return toJSON(errorResult(err.Error()))
		}
		eng.Send(cmd)
		return toJSON(struct{}{})
	}))
	api.Set("memory", js.FuncOf(func(js.Value, []js.Value) any {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return toJSON(memoryResult{HeapInuse: m.HeapInuse, Sys: m.Sys, NumGC: m.NumGC})
	}))
	js.Global().Set("marssim", api)
	select {} // keep the exports alive; JS drives everything from here
}

type startResult struct {
	Error string      `json:"error,omitempty"`
	Set   []string    `json:"set,omitempty"`
	GenMs float64     `json:"genMs"`
	Hello *wire.Hello `json:"hello,omitempty"`
}

// start builds a new engine from the default config plus the settings the page
// passed, which use the same keys as mars-sim.yaml (JSON is YAML, so the
// settings-file parser reads them as is).
func start(settings string) startResult {
	cfg := sim.DefaultConfig()
	set, err := sim.ApplyConfigFile(&cfg, []byte(settings), "settings")
	if err != nil {
		return startResult{Error: err.Error()}
	}
	cfg.SyncWithCognition()
	t0 := time.Now()
	eng = sim.NewEngine(cfg)
	// The encoder reads frames on this goroutine, between ticks, so the
	// copy-on-write tile grid would buy nothing. See docs/snapshot-tile-grid.md.
	eng.ShareLiveTiles()
	genMs := ms(time.Since(t0))
	enc = wire.NewEncoder()
	topics.Restart()
	last, interestMoved = nil, false
	// The first advance publishes the initial frame; take it now, so the
	// Hello can describe it and the page can set its view before any ticks.
	snap, _ := eng.Advance(0)
	last = snap
	hello := wire.NewHello(snap)
	return startResult{Set: set, GenMs: genMs, Hello: &hello}
}

// advance steps the engine and returns {wait, frame, perf} for the worker. A
// frame goes out when the engine published one, when the view moved, or when
// the last frame left pages owed; in the last case wait is 0 even while
// paused, so the worker comes straight back for the rest.
func advance(budget time.Duration) js.Value {
	snap, wait := eng.Advance(budget)
	if snap != nil {
		last = snap
	}
	out := js.Global().Get("Object").New()
	var frame []byte
	if last != nil && (snap != nil || interestMoved || enc.Owed() > 0) {
		frame = enc.Encode(last)
		interestMoved = false
	}
	if enc.Owed() > 0 {
		wait = 0
	}
	out.Set("wait", ms(wait))
	if frame == nil {
		out.Set("frame", js.Null())
	} else {
		u8 := js.Global().Get("Uint8Array").New(len(frame))
		js.CopyBytesToJS(u8, frame)
		out.Set("frame", u8)
	}
	if snap != nil && len(snap.Perf) > 0 {
		p := snap.Perf[len(snap.Perf)-1]
		out.Set("perf", toJSON(perfSample{
			Ticks:     p.Ticks,
			StepMs:    ms(p.Step),
			PublishMs: ms(p.Publish),
			MaxTickMs: ms(p.MaxTick),
		}))
	} else {
		out.Set("perf", js.Null())
	}
	// Topics go out at their own pace, from the newest snapshot, whether or
	// not this call published one: a panel opened on a paused game still
	// gets its data.
	if due := topics.Due(last, time.Now()); due != nil {
		out.Set("topics", toJSON(due))
	} else {
		out.Set("topics", js.Null())
	}
	return out
}

// perfSample is the newest closed sim.PerfSample, in milliseconds. It rides
// beside the frame rather than in it: it is the spike's measurement, not
// something the game draws.
type perfSample struct {
	Ticks     int     `json:"ticks"`
	StepMs    float64 `json:"stepMs"`
	PublishMs float64 `json:"publishMs"`
	MaxTickMs float64 `json:"maxTickMs"`
}

type memoryResult struct {
	HeapInuse uint64 `json:"heapInuse"`
	Sys       uint64 `json:"sys"`
	NumGC     uint32 `json:"numGC"`
}

// command is a sim.Command as the page sends it.
type command struct {
	Type string `json:"type"` // pause | speed | spawn | flow | dig | dig-cancel | zone | clear | clear-cancel | ship-move | ship-land | order-place | order-reprice | order-cancel | order-suspend | order-resume | recruit-roll | recruit-hire
	Rate int    `json:"rate,omitempty"`
	// Kind is what to spawn, or for zone the zone kind by name ("none"
	// unzones).
	Kind string `json:"kind,omitempty"`
	// Field is the flow field to show, an index into Hello.flowFields, or
	// -1 for none.
	Field *int `json:"field,omitempty"`
	// dig, zone and clear: the rectangle, inclusive, in tiles.
	ID int `json:"id,omitempty"` // dig-cancel, clear-cancel: the project id; order-*: the order's id; ship-move, ship-land: the ship's; recruit-hire: the set on offer
	X0 int `json:"x0,omitempty"`
	Y0 int `json:"y0,omitempty"`
	X1 int `json:"x1,omitempty"`
	Y1 int `json:"y1,omitempty"`
	// order-place, order-suspend and order-resume: the side ("bid" or
	// "ask") and the item by name; order-place also the
	// quantity, and the depot (x, y); order-place and order-reprice: the
	// price. ship-move, ship-land: the ship's (new) top-left (x, y).
	Side  string `json:"side,omitempty"`
	Item  string `json:"item,omitempty"`
	Qty   int    `json:"qty,omitempty"`
	Price int64  `json:"price,omitempty"`
	X     int    `json:"x,omitempty"`
	Y     int    `json:"y,omitempty"`
	// recruit-hire: the candidates to hire, by index into the set; none
	// turns the set away.
	Picks []int `json:"picks,omitempty"`
}

func parseCommand(s string) (sim.Command, error) {
	var c command
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil, err
	}
	switch c.Type {
	case "pause":
		return sim.TogglePause{}, nil
	case "speed":
		return sim.SetTicksPerSecond{Rate: c.Rate}, nil
	case "dig-cancel":
		return sim.CancelExcavation{ID: c.ID}, nil
	case "ship-move":
		return sim.MoveShip{Ship: c.ID, X: c.X, Y: c.Y}, nil
	case "ship-land":
		return sim.LandShip{Ship: c.ID, X: c.X, Y: c.Y}, nil
	case "recruit-roll":
		return sim.RollRecruits{}, nil
	case "recruit-hire":
		return sim.HireRecruits{Offer: c.ID, Picks: c.Picks}, nil
	case "dig":
		return sim.OrderExcavation{X0: c.X0, Y0: c.Y0, X1: c.X1, Y1: c.Y1}, nil
	case "zone":
		k, ok := sim.ParseZoneKind(c.Kind)
		if !ok {
			return nil, fmt.Errorf("unknown zone kind %q", c.Kind)
		}
		return sim.PaintZone{Kind: k, X0: c.X0, Y0: c.Y0, X1: c.X1, Y1: c.Y1}, nil
	case "clear":
		return sim.ClearArea{X0: c.X0, Y0: c.Y0, X1: c.X1, Y1: c.Y1}, nil
	case "clear-cancel":
		return sim.CancelClear{ID: c.ID}, nil
	case "order-place", "order-suspend", "order-resume":
		item, ok := sim.ParseItemKind(c.Item)
		if !ok {
			return nil, fmt.Errorf("unknown item %q", c.Item)
		}
		var side sim.Side
		switch c.Side {
		case "bid":
			side = sim.Bid
		case "ask":
			side = sim.Ask
		default:
			return nil, fmt.Errorf("unknown side %q", c.Side)
		}
		switch c.Type {
		case "order-suspend":
			return sim.SuspendColonyOrders{Side: side, Item: item}, nil
		case "order-resume":
			return sim.ResumeColonyOrders{Side: side, Item: item}, nil
		}
		return sim.PlaceColonyOrder{Side: side, Item: item, Qty: c.Qty, Price: sim.Money(c.Price),
			Depot: sim.Point{X: c.X, Y: c.Y}}, nil
	case "order-reprice":
		return sim.RepriceColonyOrder{ID: sim.OrderID(c.ID), Price: sim.Money(c.Price)}, nil
	case "order-cancel":
		return sim.CancelColonyOrder{ID: sim.OrderID(c.ID)}, nil
	case "spawn":
		for k := sim.Colonist; k <= sim.Rat; k++ {
			if k.String() == c.Kind {
				return sim.Spawn{Kind: k}, nil
			}
		}
		return nil, fmt.Errorf("unknown kind %q", c.Kind)
	case "flow":
		if c.Field == nil || *c.Field < 0 {
			return sim.ShowFlowField{}, nil
		}
		// Hello.flowFields is the first snapshot's list, and the fields
		// are fixed for a world's life, so the newest snapshot's agrees.
		if last == nil || *c.Field >= len(last.FlowFields) {
			return nil, fmt.Errorf("no flow field %d", *c.Field)
		}
		return sim.ShowFlowField{Show: true, Field: last.FlowFields[*c.Field]}, nil
	}
	return nil, fmt.Errorf("unknown command %q", c.Type)
}

type errorBody struct {
	Error string `json:"error"`
}

func errorResult(msg string) errorBody { return errorBody{Error: msg} }

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(errorResult(err.Error()))
	}
	return string(b)
}
