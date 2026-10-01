package wire

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

var update = flag.Bool("update", false, "rewrite the golden frames in testdata")

// The golden frames pin the byte layout, and they are what the JS decoder's
// test (web/wire/decode.test.mjs) reads: each NAME.bin is a frame and
// NAME.json is what decoding it must give. A layout change shows up here as a
// byte diff; regenerate with
//
//	go test ./internal/wire -run Golden -update
//
// bump Version, and update web/wire/decode.js in the same change.
func TestGoldenFrames(t *testing.T) {
	e := NewEncoder()
	e.SetInterest(everything())
	snap := fixture(true)
	first := bytes.Clone(e.Encode(snap))

	changed := *snap
	changed.Tick = 1300
	changed.Paused = true
	changed.TileChanges = sim.TileChanges{Frame: 2, Pages: []int{snap.Tiles.PageIndex(sim.Point{X: 70, Y: 10})}, Refuse: true}
	// The toilet field, shown: three reached tiles, one of them a goal.
	changed.FlowFields = []sim.FlowFieldRef{{Facility: sim.NutrientPod}, {Facility: sim.Toilet}, {Frontier: true}}
	changed.FlowField = sim.NewFlowFieldView(sim.FlowFieldRef{Facility: sim.Toilet}, snap.Width, snap.Height,
		map[sim.Point]int32{{X: 7, Y: 0}: 0, {X: 8, Y: 1}: 1, {X: 100, Y: 5}: 40})
	second := bytes.Clone(e.Encode(&changed))

	capped := NewEncoder()
	capped.MaxPages = 4
	capped.SetInterest(everything())
	third := bytes.Clone(capped.Encode(fixture(false)))

	for name, frame := range map[string][]byte{"first": first, "changed": second, "capped": third} {
		bin := filepath.Join("testdata", name+".bin")
		want := filepath.Join("testdata", name+".json")
		got, err := json.MarshalIndent(decode(t, frame), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, '\n')
		if *update {
			if err := os.WriteFile(bin, frame, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(want, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		oldBin, err := os.ReadFile(bin)
		if err != nil {
			t.Fatalf("%v (run with -update to create it)", err)
		}
		if !bytes.Equal(oldBin, frame) {
			t.Errorf("%s: frame bytes changed. A new sim.Stats field or a renumbered enum changes them too: "+
				"regenerate with go test ./internal/wire -run Golden -update. A layout change also bumps Version "+
				"and updates web/wire/decode.js", name)
		}
		oldJSON, err := os.ReadFile(want)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(oldJSON, got) {
			t.Errorf("%s: decoded frame changed:\n%s", name, got)
		}
	}

}
