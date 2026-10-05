package sim

import (
	"math/bits"
	"slices"
	"sort"
)

// patchList is every scum patch in cmpScumPatch order, for growScum to draw
// the k-th from. It is a sorted list cut into blocks, so adding or removing a
// patch moves at most one block rather than the whole list, with a Fenwick
// tree over the block sizes, so finding the k-th patch and keeping the counts
// in step are both logarithmic.
//
// It was a plain sorted slice. Each new patch then moved every patch after
// it, and growth that started thousands of patches a tick (a map with room
// for many more, e.g. a world built without chunk generation) went quadratic:
// BenchmarkStepSmallColonyOnHugeMap2500 took ~10 ms a tick, and the 10000
// one never finished. A running count of the patches before each block,
// recomputed after a change, was the first fix; with growth inserting between
// draws it re-summed thousands of blocks per draw on a big map, and was most
// of the tick. See docs/scumhouse.md.
//
// The zero value is an empty list.
type patchList struct {
	blocks [][]Point // each non-empty and sorted; together, the whole list in order
	// tree is a Fenwick tree (1-based) over len(blocks[i]); rebuilt whenever a
	// block is split off or emptied, which moves the blocks after it.
	tree []int
	n    int
	// flat is the whole list in one slice while frozen is set: a draw is then
	// a plain index. freeze builds it; any change clears frozen.
	flat   []Point
	frozen bool
}

// patchBlock is how many patches a block is cut back to when it grows past
// twice that.
const patchBlock = 512

func (l *patchList) len() int { return l.n }

// blockFor is the block p belongs in: the first whose last patch is not
// before p, or the last block when p is past them all.
func (l *patchList) blockFor(p Point) int {
	i := sort.Search(len(l.blocks), func(i int) bool {
		b := l.blocks[i]
		return cmpScumPatch(b[len(b)-1], p) >= 0
	})
	return min(i, len(l.blocks)-1)
}

// insert adds p, which must not be in the list already.
func (l *patchList) insert(p Point) {
	l.n++
	l.frozen = false
	if len(l.blocks) == 0 {
		l.blocks = [][]Point{{p}}
		l.rebuild()
		return
	}
	bi := l.blockFor(p)
	b := l.blocks[bi]
	j, _ := slices.BinarySearchFunc(b, p, cmpScumPatch)
	b = slices.Insert(b, j, p)
	if len(b) > 2*patchBlock {
		tail := slices.Clone(b[patchBlock:])
		l.blocks[bi] = b[:patchBlock:patchBlock] // so a later insert here cannot write over tail's old storage
		l.blocks = slices.Insert(l.blocks, bi+1, tail)
		l.rebuild()
		return
	}
	l.blocks[bi] = b
	l.add(bi, 1)
}

// remove takes p out of the list, if it is there.
func (l *patchList) remove(p Point) {
	if len(l.blocks) == 0 {
		return
	}
	bi := l.blockFor(p)
	b := l.blocks[bi]
	j, found := slices.BinarySearchFunc(b, p, cmpScumPatch)
	if !found {
		return
	}
	l.n--
	l.frozen = false
	if b = slices.Delete(b, j, j+1); len(b) == 0 {
		l.blocks = slices.Delete(l.blocks, bi, bi+1)
		l.rebuild()
		return
	}
	l.blocks[bi] = b
	l.add(bi, -1)
}

// at is the k-th patch in order, for 0 <= k < len.
func (l *patchList) at(k int) Point {
	if l.frozen {
		return l.flat[k]
	}
	// Descend the tree for the last block whose start is <= k.
	i := 0 // 1-based position reached so far; k is left relative to it
	for step := 1 << (bits.Len(uint(len(l.blocks))) - 1); step > 0; step >>= 1 {
		if next := i + step; next <= len(l.blocks) && l.tree[next] <= k {
			i = next
			k -= l.tree[next]
		}
	}
	return l.blocks[i][k]
}

// add changes block bi's count by d.
func (l *patchList) add(bi, d int) {
	for i := bi + 1; i < len(l.tree); i += i & -i {
		l.tree[i] += d
	}
}

// rebuild recomputes the tree from the blocks, in linear time.
func (l *patchList) rebuild() {
	l.tree = slices.Grow(l.tree[:0], len(l.blocks)+1)[:len(l.blocks)+1]
	l.tree[0] = 0
	for i, b := range l.blocks {
		l.tree[i+1] = len(b)
	}
	for i := 1; i < len(l.tree); i++ {
		if j := i + i&-i; j < len(l.tree) {
			l.tree[j] += l.tree[i]
		}
	}
}

// freeze copies the list into flat for at, unless it has not changed since
// the last copy. growScum freezes it when it is about to draw without
// changing it, which on a settled map is nearly every tick: the copy is then
// rare, and every draw an index instead of a walk down the tree.
func (l *patchList) freeze() {
	if !l.frozen {
		l.flat = l.appendTo(l.flat[:0])
		l.frozen = true
	}
}

// appendTo appends every patch, in order, to dst.
func (l *patchList) appendTo(dst []Point) []Point {
	for _, b := range l.blocks {
		dst = append(dst, b...)
	}
	return dst
}
