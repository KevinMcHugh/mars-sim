package sim

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// encodeWorld is the save codec's view of a world: equal bytes mean equal
// state, caches and RNG streams included, since map entries are written in
// key order.
func encodeWorld(t *testing.T, w *World) []byte {
	t.Helper()
	e := newSaveEncoder()
	if err := e.encodeRoot(w); err != nil {
		t.Fatal(err)
	}
	return e.buf
}

// saveAndLoad round-trips w through a save file.
func saveAndLoad(t *testing.T, w *World) *World {
	t.Helper()
	var b bytes.Buffer
	if err := w.save(&b, SaveInfo{TicksPerSecond: 10}); err != nil {
		t.Fatal(err)
	}
	e, info, err := LoadEngine(&b)
	if err != nil {
		t.Fatal(err)
	}
	if info.Tick != w.tick || info.Seed != w.cfg.Seed {
		t.Fatalf("header says tick %d seed %d, want tick %d seed %d", info.Tick, info.Seed, w.tick, w.cfg.Seed)
	}
	return e.world
}

// A loaded game must be the saved one, not a lookalike: saving it again gives
// the same bytes.
func TestSaveLoadSaveIsIdentical(t *testing.T) {
	w := newTestWorld(t, testConfig())
	for i := 0; i < 200; i++ {
		w.step()
	}
	got := saveAndLoad(t, w)
	if !bytes.Equal(encodeWorld(t, w), encodeWorld(t, got)) {
		t.Fatalf("a loaded world encodes differently from the world that was saved: %s", saveDiff(w, got))
	}
}

// The guard docs/browser-frontend.md asked for: run N ticks, save, load, run M
// more, and the loaded game must match a straight N+M run on every tick. The
// comparison is the whole encoded world, so a cache that loads subtly wrong
// and only later decides a tie is caught, not just a colonist standing
// somewhere else. The fingerprint names the field when they part.
func TestSaveLoadPlaysOnIdentically(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cfg         func() Config
		before, run int
	}{
		{"mechanics", func() Config {
			cfg := testConfig()
			cfg.Seed = 99
			cfg.Width, cfg.Height = 80, 50
			cfg.StartColonists, cfg.StartCats, cfg.StartRats = 16, 2, 10
			cfg.CavernNestPercent = 100
			return cfg
		}, 300, 600},
		// The game's defaults: scarcity, the market, kitchens and ships.
		{"scarcity", func() Config {
			cfg := DefaultConfig()
			cfg.Seed = 7
			cfg.Width, cfg.Height = 250, 150
			cfg.StartColonists = 40
			cfg.ZoningAuto = true
			return cfg
		}, 800, 700},
	} {
		t.Run(tc.name, func(t *testing.T) {
			straight := newTestWorld(t, tc.cfg())
			for i := 0; i < tc.before; i++ {
				straight.step()
			}
			loaded := saveAndLoad(t, straight)
			for i := 0; i < tc.run; i++ {
				straight.step()
				loaded.step()
				if i%25 != 24 && i != tc.run-1 {
					continue
				}
				if bytes.Equal(encodeWorld(t, straight), encodeWorld(t, loaded)) {
					continue
				}
				fa, fb := worldFingerprint(straight), worldFingerprint(loaded)
				for _, k := range fingerprintKeys {
					if fa[k] != fb[k] {
						t.Fatalf("loaded game diverged by tick %d, field %q", straight.tick, k)
					}
				}
				t.Fatalf("loaded game's state differs by tick %d: %s", straight.tick, saveDiff(straight, loaded))
			}
		})
	}
}

// The RNG streams load as the same objects the world draws from: w.rng's
// source is rngSrc.sim, not a copy of it, so a draw moves both.
func TestSaveKeepsRNGSourcesShared(t *testing.T) {
	w := newTestWorld(t, testConfig())
	w.step()
	got := saveAndLoad(t, w)
	before, err := got.saveRNG()
	if err != nil {
		t.Fatal(err)
	}
	got.rng.Uint64()
	after, err := got.saveRNG()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before.Sim, after.Sim) {
		t.Fatal("drawing from the loaded w.rng did not advance rngSrc.sim: the load split them")
	}
}

// A func field cannot be saved, so afterLoad rebinds each one. This lists
// every func reachable from the World; a new one fails here until it is
// handled in afterLoad (or tagged save:"-" and rebuilt there) and added below.
func TestSaveFuncFieldsAreRebound(t *testing.T) {
	want := []string{"flowField.goal", "flowField.seed"}
	fields := &savedFields{}
	seen := map[reflect.Type]bool{}
	var got []string
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		if seen[t] {
			return
		}
		seen[t] = true
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(t.Elem())
		case reflect.Map:
			walk(t.Key())
			walk(t.Elem())
		case reflect.Struct:
			for _, i := range fields.of(t) {
				f := t.Field(i)
				ft := f.Type
				for ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array || ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Func {
					got = append(got, t.Name()+"."+f.Name)
				}
				walk(f.Type)
			}
		}
	}
	walk(reflect.TypeFor[World]())
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("func fields saved worlds reach: %v, want %v; rebind a new one in World.afterLoad", got, want)
	}

	w := saveAndLoad(t, newTestWorld(t, testConfig()))
	for kind, f := range append(w.fields[:], w.frontier) {
		if f != nil && (f.seed == nil || f.goal == nil) {
			t.Fatalf("flow field %d loaded without its seed or goal", kind)
		}
	}
	if len(w.subscribers) != len(newTestWorld(t, testConfig()).subscribers) {
		t.Fatal("a loaded world lost its event subscribers")
	}
}

// The codec must refuse sharing it cannot reproduce rather than split it.
func TestSaveRefusesInteriorPointers(t *testing.T) {
	type holder struct {
		Items []int
		Ptr   *int
	}
	h := &holder{Items: []int{1, 2, 3}}
	h.Ptr = &h.Items[1]
	err := newSaveEncoder().encodeRoot(h)
	if err == nil || !strings.Contains(err.Error(), "share memory") {
		t.Fatalf("encoding a pointer into a slice: err = %v, want a share-memory error", err)
	}
}

// Shared pointers, shared maps and cycles survive a round trip as sharing.
func TestSaveCodecKeepsSharing(t *testing.T) {
	type node struct {
		Next *node
		M    map[int]string
	}
	type graph struct {
		A, B *node
		M1   map[int]string
		Root *graph
	}
	m := map[int]string{1: "one", 2: "two"}
	a := &node{M: m}
	a.Next = a
	g := &graph{A: a, B: a, M1: m}
	g.Root = g
	e := newSaveEncoder()
	if err := e.encodeRoot(g); err != nil {
		t.Fatal(err)
	}
	var got graph
	if err := newSaveDecoder(e.buf).decodeRoot(&got); err != nil {
		t.Fatal(err)
	}
	if got.A != got.B || got.A.Next != got.A || got.Root != &got {
		t.Fatal("shared pointers loaded as copies")
	}
	got.M1[3] = "three"
	if got.A.M[3] != "three" {
		t.Fatal("a shared map loaded as two maps")
	}
}

func TestLoadRejectsOtherFiles(t *testing.T) {
	var good bytes.Buffer
	w := newTestWorld(t, testConfig())
	if err := w.save(&good, SaveInfo{}); err != nil {
		t.Fatal(err)
	}
	header, _, _ := strings.Cut(good.String(), "\n{")
	for name, tc := range map[string]struct {
		file string
		want string
	}{
		"not a save": {"hello", "not a mars-sim save"},
		"truncated":  {good.String()[:good.Len()/2], ""},
		"other layout": {header + "\n" + `{"format":1,"commit":"abc123","layout":"0000"}` + "\n",
			"commit abc123"},
		"other format": {header + "\n" + `{"format":99,"commit":"abc123"}` + "\n", "format 99"},
		"corrupt body": {corrupted(good.Bytes()), "save:"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := LoadEngine(strings.NewReader(tc.file))
			if err == nil {
				t.Fatal("loaded")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// corrupted flips bytes in the middle of a save's world, re-zipped so the
// damage reaches the decoder rather than stopping at gzip's checksum.
func corrupted(file []byte) string {
	_, br, err := readSaveHeader(bufio.NewReader(bytes.NewReader(file)))
	if err != nil {
		panic(err)
	}
	zr, _ := gzip.NewReader(br)
	body, _ := io.ReadAll(zr)
	for i := len(body) / 2; i < len(body)/2+64 && i < len(body); i++ {
		body[i] ^= 0xA5
	}
	head := file[:bytes.IndexByte(file[len(saveMagic)+1:], '\n')+len(saveMagic)+2]
	var out bytes.Buffer
	out.Write(head)
	zw := gzip.NewWriter(&out)
	zw.Write(body)
	zw.Close()
	return out.String()
}

// The header is readable on its own and carries the commit.
func TestSaveHeader(t *testing.T) {
	cfg := testConfig()
	e := NewEngine(cfg)
	e.Advance(0)
	b, err := e.SaveBytes()
	if err != nil {
		t.Fatal(err)
	}
	info, err := ReadSaveInfo(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if info.Commit == "" || info.Layout != currentSaveLayout() || info.Seed != cfg.Seed ||
		info.Width != cfg.Width || info.TicksPerSecond != cfg.TicksPerSecond {
		t.Fatalf("header = %+v", info)
	}
}

// A running engine saves on its own goroutine through SaveGame, and the
// loaded engine runs and publishes like a new one.
func TestSaveGameCommandWhileRunning(t *testing.T) {
	cfg := testConfig()
	cfg.TicksPerSecond = 500
	e := NewEngine(cfg)
	snaps := e.Subscribe()
	ctx, cancel := context.WithCancel(context.Background())
	go e.Run(ctx)
	<-snaps
	time.Sleep(20 * time.Millisecond)

	var b bytes.Buffer
	done := make(chan error, 1)
	e.Send(SaveGame{To: &b, Done: done})
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SaveGame never answered")
	}
	cancel()

	loaded, info, err := LoadEngine(&b)
	if err != nil {
		t.Fatal(err)
	}
	if info.Tick == 0 {
		t.Fatal("saved at tick 0: the engine never ran")
	}
	ls := loaded.Subscribe()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go loaded.Run(ctx2)
	first := <-ls
	if first.Tick != info.Tick || !first.TileChanges.All {
		t.Fatalf("loaded engine's first frame: tick %d all=%v, want tick %d with every page", first.Tick, first.TileChanges.All, info.Tick)
	}
	for s := range ls {
		if s.Tick > info.Tick {
			return
		}
	}
	t.Fatal("the loaded engine never ticked")
}

// saveDiff names the first place two worlds differ, walking them the way the
// save codec does (saved fields only, maps by key, each pointer pair once),
// so a failing round trip says where rather than only that.
func saveDiff(a, b *World) string {
	fields := &savedFields{}
	seen := map[[2]uintptr]bool{}
	var diff func(x, y reflect.Value, path string) string
	diff = func(x, y reflect.Value, path string) string {
		switch x.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice:
			if x.IsNil() != y.IsNil() {
				return path + ": nil on one side"
			}
			if x.IsNil() {
				return ""
			}
			if x.Kind() != reflect.Slice {
				k := [2]uintptr{x.Pointer(), y.Pointer()}
				if seen[k] {
					return ""
				}
				seen[k] = true
			}
		}
		switch x.Kind() {
		case reflect.Pointer:
			return diff(x.Elem(), y.Elem(), path)
		case reflect.Interface:
			if x.IsNil() != y.IsNil() {
				return path + ": nil on one side"
			}
			if x.IsNil() {
				return ""
			}
			return diff(x.Elem(), y.Elem(), path)
		case reflect.Struct:
			for _, i := range fields.of(x.Type()) {
				if d := diff(x.Field(i), y.Field(i), path+"."+x.Type().Field(i).Name); d != "" {
					return d
				}
			}
		case reflect.Slice, reflect.Array:
			if x.Len() != y.Len() {
				return fmt.Sprintf("%s: len %d vs %d", path, x.Len(), y.Len())
			}
			for i := 0; i < x.Len(); i++ {
				if d := diff(x.Index(i), y.Index(i), fmt.Sprintf("%s[%d]", path, i)); d != "" {
					return d
				}
			}
		case reflect.Map:
			if x.Len() != y.Len() {
				return fmt.Sprintf("%s: len %d vs %d", path, x.Len(), y.Len())
			}
			for it := x.MapRange(); it.Next(); {
				yv := y.MapIndex(it.Key())
				if !yv.IsValid() {
					return fmt.Sprintf("%s{%v}: missing on one side", path, it.Key())
				}
				if d := diff(it.Value(), yv, fmt.Sprintf("%s{%v}", path, it.Key())); d != "" {
					return d
				}
			}
		case reflect.Func:
		case reflect.Bool:
			if x.Bool() != y.Bool() {
				return fmt.Sprintf("%s: %v vs %v", path, x.Bool(), y.Bool())
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if x.Int() != y.Int() {
				return fmt.Sprintf("%s: %v vs %v", path, x.Int(), y.Int())
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			if x.Uint() != y.Uint() {
				return fmt.Sprintf("%s: %v vs %v", path, x.Uint(), y.Uint())
			}
		case reflect.Float32, reflect.Float64:
			if math.Float64bits(x.Float()) != math.Float64bits(y.Float()) {
				return fmt.Sprintf("%s: %v vs %v", path, x.Float(), y.Float())
			}
		case reflect.String:
			if x.String() != y.String() {
				return fmt.Sprintf("%s: %q vs %q", path, x.String(), y.String())
			}
		}
		return ""
	}
	if d := diff(reflect.ValueOf(a), reflect.ValueOf(b), "World"); d != "" {
		return d
	}
	return "every value matches, so what differs is which objects are shared"
}
