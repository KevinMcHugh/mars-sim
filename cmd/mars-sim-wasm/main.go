//go:build js && wasm

// The browser build of the engine, for a Web Worker. This is the spike from
// docs/browser-frontend.md: it runs a headless world and reports stats and
// timings, with no map or wire format yet. Build it with:
//
//	web/spike/build.sh
//
// It exports four functions on globalThis.marssim, each taking and returning
// JSON strings so the boundary stays one call per slice:
//
//	start(settings)  new game; settings is a mars-sim.yaml mapping as JSON
//	advance(ms)      run the ticks due within a budget; see sim.Engine.Advance
//	send(command)    queue a command, applied at the next advance
//	memory()         the Go heap, for the spike's memory readout
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
)

var eng *sim.Engine

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
		snap, wait := eng.Advance(budget)
		return toJSON(advanceResult{Wait: ms(wait), Frame: frameOf(snap)})
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
	Error  string   `json:"error,omitempty"`
	Set    []string `json:"set,omitempty"`
	GenMs  float64  `json:"genMs"`
	Width  int      `json:"width"`
	Height int      `json:"height"`
	Seed   int64    `json:"seed"`
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
	// The worker encodes frames on this goroutine, between ticks, so the
	// copy-on-write tile grid would buy nothing. See docs/snapshot-tile-grid.md.
	eng.ShareLiveTiles()
	return startResult{
		Set:    set,
		GenMs:  ms(time.Since(t0)),
		Width:  cfg.Width,
		Height: cfg.Height,
		Seed:   cfg.Seed,
	}
}

type advanceResult struct {
	// Wait is how long to wait before the next advance, in milliseconds:
	// 0 means call again now, negative means paused until a command.
	Wait  float64 `json:"wait"`
	Frame *frame  `json:"frame,omitempty"`
}

// frame is the spike's stand-in for the wire format: the header a real frame
// would carry, and nothing colony-sized. The point is to measure the engine,
// not the encoding.
type frame struct {
	Tick      int         `json:"tick"`
	Paused    bool        `json:"paused"`
	TPS       int         `json:"tps"`
	Stats     sim.Stats   `json:"stats"`
	Entities  int         `json:"entities"`
	TileFrame uint64      `json:"tileFrame"`
	TileAll   bool        `json:"tileAll"`
	TilePages []int       `json:"tilePages,omitempty"`
	Perf      *perfSample `json:"perf,omitempty"`
}

// perfSample is the newest closed sim.PerfSample, in milliseconds.
type perfSample struct {
	Ticks     int     `json:"ticks"`
	StepMs    float64 `json:"stepMs"`
	PublishMs float64 `json:"publishMs"`
	MaxTickMs float64 `json:"maxTickMs"`
	BucketMs  float64 `json:"bucketMs"`
}

func frameOf(s *sim.Snapshot) *frame {
	if s == nil {
		return nil
	}
	f := &frame{
		Tick:      s.Tick,
		Paused:    s.Paused,
		TPS:       s.TicksPerSecond,
		Stats:     s.Stats,
		Entities:  len(s.Entities),
		TileFrame: s.TileChanges.Frame,
		TileAll:   s.TileChanges.All,
		TilePages: s.TileChanges.Pages,
	}
	if n := len(s.Perf); n > 0 {
		p := s.Perf[n-1]
		f.Perf = &perfSample{
			Ticks:     p.Ticks,
			StepMs:    ms(p.Step),
			PublishMs: ms(p.Publish),
			MaxTickMs: ms(p.MaxTick),
			BucketMs:  ms(sim.PerfBucket),
		}
	}
	return f
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
