//go:build js && wasm

// Scum Lab's browser build. It exports roll, catalog, and evaluate, and it
// does not start a world. Build with:
//
//	tools/scum-lab/build.sh
package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

func main() {
	needs, moodMax, moodMargin, traitChance := sim.LabSettings()
	api := js.Global().Get("Object").New()
	api.Set("catalog", js.FuncOf(func(js.Value, []js.Value) any {
		return mustJSON(struct {
			Traits []sim.LabTrait `json:"traits"`
			Drives  []sim.LabDrive  `json:"drives"`
		}{
			Traits: sim.LabTraits(),
			Drives:  sim.LabDrives(needs),
		})
	}))
	api.Set("roll", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return errorJSON("roll needs a seed")
		}
		return mustJSON(sim.LabRoll(int64(args[0].Int()), traitChance))
	}))
	api.Set("evaluate", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 2 {
			return errorJSON("evaluate needs cognition and situation JSON")
		}
		var cog sim.LabCognition
		var sit sim.LabSituation
		if err := json.Unmarshal([]byte(args[0].String()), &cog); err != nil {
			return errorJSON(err.Error())
		}
		if err := json.Unmarshal([]byte(args[1].String()), &sit); err != nil {
			return errorJSON(err.Error())
		}
		verdict, err := sim.LabEvaluate(needs, moodMax, moodMargin, cog, sit)
		if err != nil {
			return errorJSON(err.Error())
		}
		return mustJSON(verdict)
	}))
	js.Global().Set("scumlab", api)
	select {}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return errorJSON(err.Error())
	}
	return string(b)
}

func errorJSON(msg string) string {
	b, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{msg})
	return string(b)
}
