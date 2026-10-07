package sim

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// patchList agrees with a plain sorted slice through enough inserts and
// removals to split blocks and empty them again.
func TestPatchListMatchesSortedSlice(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	var l patchList
	var want []Point
	in := map[Point]bool{}
	check := func(step int) {
		t.Helper()
		if l.len() != len(want) {
			t.Fatalf("step %d: len %d, want %d", step, l.len(), len(want))
		}
		if got := l.appendTo(nil); !slices.Equal(got, want) {
			t.Fatalf("step %d: list out of step with the sorted slice", step)
		}
		for range 20 {
			if len(want) == 0 {
				break
			}
			k := r.IntN(len(want))
			if got := l.at(k); got != want[k] {
				t.Fatalf("step %d: at(%d) = %v, want %v", step, k, got, want[k])
			}
		}
	}
	for step := range 20000 {
		p := Point{r.IntN(300), r.IntN(300), LandingLevel}
		// Mostly inserts for the first half, mostly removals after.
		grow := r.IntN(10) < 8
		if step >= 10000 {
			grow = !grow
		}
		if grow && !in[p] {
			l.insert(p)
			in[p] = true
			i, _ := slices.BinarySearchFunc(want, p, cmpScumPatch)
			want = slices.Insert(want, i, p)
		} else if !grow && len(want) > 0 {
			q := want[r.IntN(len(want))]
			l.remove(q)
			l.remove(q) // a second remove is a no-op
			delete(in, q)
			i, _ := slices.BinarySearchFunc(want, q, cmpScumPatch)
			want = slices.Delete(want, i, i+1)
		}
		if step%500 == 0 {
			check(step)
		}
		if step%700 == 0 {
			l.freeze() // the next check reads the flat copy, until a change thaws it
			check(step)
		}
	}
	check(20000)
	if len(l.blocks) < 2 && len(want) > 2*patchBlock {
		t.Fatalf("%d patches in %d block(s): blocks never split", len(want), len(l.blocks))
	}
}
