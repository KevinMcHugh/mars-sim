package sim

import "testing"

// A published view reads exactly what the field does, everywhere, and is
// reused rather than copied again while the field has not changed.
func TestFlowFieldViewMatchesField(t *testing.T) {
	cfg := testConfig()
	cfg.Seed = 5
	w := newTestWorld(t, cfg)
	for i := 0; i < 200; i++ {
		w.step()
	}
	refs := w.flowFieldRefs()
	if last := refs[len(refs)-1]; !last.Frontier {
		t.Fatalf("frontier field not listed last: %+v", refs)
	}
	for _, r := range refs {
		v, ver := w.flowFieldView(r, nil, 0)
		if v == nil {
			t.Fatalf("no view for %s", r.Name())
		}
		f := w.flowFieldFor(r)
		var goals int
		var maxD int32
		for y := 0; y < w.Height; y++ {
			for x := 0; x < w.Width; x++ {
				p := Point{x, y}
				want := f.at(p)
				if got := v.At(p); got != want {
					t.Fatalf("%s at %v: view %d, field %d", r.Name(), p, got, want)
				}
				if want == 0 {
					goals++
				}
				maxD = max(maxD, want)
			}
		}
		if v.Goals != goals || v.Max != maxD {
			t.Fatalf("%s: goals/max %d/%d, want %d/%d", r.Name(), v.Goals, v.Max, goals, maxD)
		}
		if again, _ := w.flowFieldView(r, v, ver); again != v {
			t.Fatalf("%s: unchanged field was copied again", r.Name())
		}
	}
	if v := (*FlowFieldView)(nil); v.At(Point{}) != -1 {
		t.Fatal("nil view should read as unreachable")
	}
}

// Showing a field freshens it outside a colonist's turn. That must change
// only when the work happens, never a distance anyone reads: a world whose
// fields are all published every tick stays in lockstep with one nobody
// watches.
func TestFlowFieldViewKeepsRunDeterministic(t *testing.T) {
	mk := func() *World {
		cfg := testConfig()
		cfg.Seed = 99
		cfg.Width, cfg.Height = 80, 50
		cfg.StartColonists, cfg.StartCats, cfg.StartRats = 16, 2, 10
		return newTestWorld(t, cfg)
	}
	a, b := mk(), mk()
	for i := 0; i < 800; i++ {
		a.step()
		b.step()
		for _, r := range b.flowFieldRefs() {
			b.flowFieldView(r, nil, 0)
		}
		fa, fb := worldFingerprint(a), worldFingerprint(b)
		for _, k := range fingerprintKeys {
			if fa[k] != fb[k] {
				t.Fatalf("watched world diverged at tick %d, field %q:\n  A: %s\n  B: %s",
					a.tick, k, fa[k], fb[k])
			}
		}
	}
}

// The engine publishes the field a frontend asked for, and stops when told.
func TestShowFlowFieldPublishes(t *testing.T) {
	cfg := testConfig()
	e := NewEngine(cfg)
	snap := e.publish()
	if snap.FlowField != nil || len(snap.FlowFields) == 0 {
		t.Fatalf("before asking: FlowField %v, %d fields listed", snap.FlowField, len(snap.FlowFields))
	}
	frontier := FlowFieldRef{Frontier: true}
	e.apply(ShowFlowField{Show: true, Field: frontier})
	snap = e.publish()
	if snap.FlowField == nil || snap.FlowField.Field != frontier {
		t.Fatalf("frontier not published: %+v", snap.FlowField)
	}
	if again := e.publish(); again.FlowField != snap.FlowField {
		t.Fatal("unchanged field copied for the next frame")
	}
	e.apply(ShowFlowField{})
	if snap = e.publish(); snap.FlowField != nil {
		t.Fatal("field still published after ShowFlowField{}")
	}
}
