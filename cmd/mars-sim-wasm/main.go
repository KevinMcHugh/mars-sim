//go:build js && wasm

// The browser build of the engine, for a Web Worker (see
// docs/browser-frontend.md). Build it with:
//
//	web/spike/build.sh
//
// It exports these on globalThis.marssim:
//
//	start(settings)        new game; settings is a mars-sim.yaml mapping as
//	                       JSON. Returns JSON: timings and the wire Hello.
//	advance(ms)            run the ticks due within a budget (see
//	                       sim.Engine.Advance). Returns {wait, frame, perf}:
//	                       frame is a wire frame (Uint8Array) or null.
//	interest(x0,y0,x1,y1)  the map region the page shows, in tiles
//	send(command)          queue a command (JSON), applied at the next advance
//	memory()               the Go heap, as JSON
//
// The JS side owns the loop (web/spike/worker.js): it calls advance, posts the
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
)

func main() {
	api := js.Global().Get("Object").New()
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
	Type string `json:"type"` // pause | speed | spawn
	Rate int    `json:"rate,omitempty"`
	Kind string `json:"kind,omitempty"`
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
	case "spawn":
		for k := sim.Colonist; k <= sim.Rat; k++ {
			if k.String() == c.Kind {
				return sim.Spawn{Kind: k}, nil
			}
		}
		return nil, fmt.Errorf("unknown kind %q", c.Kind)
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
