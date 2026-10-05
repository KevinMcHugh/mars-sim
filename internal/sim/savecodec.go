package sim

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unsafe"
)

// The save codec writes an object graph -- the whole World, caches included --
// by walking it with reflection, so a field added to the World (or to anything
// it reaches) is saved without a second edit. See docs/save-load.md.
//
// The stream is a sequence of values, each written by its kind:
//
//	bool, ints, uints, floats   varints (floats as their IEEE bits)
//	string                      uvarint length, bytes
//	array, struct               each element / saved field in order
//	pointer, map, slice         uvarint 0 for nil, else an object id; an id
//	                            seen for the first time is followed by the
//	                            object's contents (a slice's: its length,
//	                            then its elements), a repeated one is not
//	interface                   registered type name ("" for nil), then value
//	func                        nothing: kept from the world loaded into
//
// Object ids keep sharing intact: two fields holding the same *Ship (or the
// same map, or the same slice -- a struct copied whole shares its slices with
// the original) load holding the same one, and a pointer back to the World is
// the World. A map's entries are written in the order of their encoded keys, so
// equal worlds always encode to equal bytes, whatever order Go's maps keep.
//
// What ids cannot capture is a pointer *into* something -- &slice[i], or two
// different slices over one backing array. A load would give each its own copy and
// quietly break whatever relied on the sharing, so the encoder records the
// memory every pointee and slice covers and refuses a graph where two overlap.

// saveSkipTag marks a struct field the codec leaves out: `save:"-"`. A skipped
// field keeps whatever value the world being loaded into already has.
const saveSkipTag = "save"

// saveInterfaceTypes names every concrete type that may sit in an interface
// field. Go cannot look a type up by name, so the loader needs this list; the
// encoder refuses an interface holding anything else rather than write a
// value nobody can read back.
var saveInterfaceTypes = map[string]reflect.Type{
	"*rand.PCG": reflect.TypeFor[*rand.PCG](),
}

var saveInterfaceNames = func() map[reflect.Type]string {
	m := make(map[reflect.Type]string, len(saveInterfaceTypes))
	for name, t := range saveInterfaceTypes {
		m[t] = name
	}
	return m
}()

// savedFields lists, per struct type, the indexes of the fields the codec
// writes: every field not tagged save:"-".
type savedFields struct{ cache map[reflect.Type][]int }

func (s *savedFields) of(t reflect.Type) []int {
	if f, ok := s.cache[t]; ok {
		return f
	}
	if s.cache == nil {
		s.cache = make(map[reflect.Type][]int)
	}
	var idx []int
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Tag.Get(saveSkipTag) != "-" {
			idx = append(idx, i)
		}
	}
	s.cache[t] = idx
	return idx
}

// memRange is the memory one pointee or slice backing array covers.
type memRange struct {
	start, end uintptr
	typ        reflect.Type
}

type saveEncoder struct {
	buf    []byte
	fields *savedFields
	// ids numbers every pointer and map written so far, by address, and
	// sliceIDs every slice, by backing array and length.
	ids      map[uintptr]saveObject
	sliceIDs map[sliceKey]uint64
	nextID   uint64
	ranges   []memRange
	// keyEnc writes map keys on their own, to sort entries by key bytes.
	// keyOnly marks that encoder: a key holding a pointer, map or interface
	// would get an id, and ids are what sorting by key bytes has to avoid.
	keyEnc  *saveEncoder
	keyOnly bool
}

type saveObject struct {
	id  uint64
	typ reflect.Type
}

type sliceKey struct {
	data uintptr
	n    int
	typ  reflect.Type
}

func newSaveEncoder() *saveEncoder {
	return &saveEncoder{fields: &savedFields{}, ids: make(map[uintptr]saveObject), sliceIDs: make(map[sliceKey]uint64)}
}

// encodeRoot writes *root, with root itself as object 1, so pointers back to
// it (a flow field's world, say) load as the root they were.
func (e *saveEncoder) encodeRoot(root any) error {
	v := reflect.ValueOf(root)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("save: root must be a non-nil pointer, got %T", root)
	}
	e.nextID++
	e.ids[v.Pointer()] = saveObject{id: e.nextID, typ: v.Type()}
	e.cover(v.Pointer(), v.Type().Elem().Size(), v.Type().Elem())
	if err := e.value(v.Elem()); err != nil {
		return err
	}
	return e.checkOverlaps()
}

func (e *saveEncoder) uvarint(x uint64) { e.buf = binary.AppendUvarint(e.buf, x) }
func (e *saveEncoder) varint(x int64)   { e.buf = binary.AppendVarint(e.buf, x) }

func (e *saveEncoder) cover(start, size uintptr, t reflect.Type) {
	if size > 0 {
		e.ranges = append(e.ranges, memRange{start, start + size, t})
	}
}

func (e *saveEncoder) value(v reflect.Value) error {
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			e.buf = append(e.buf, 1)
		} else {
			e.buf = append(e.buf, 0)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.varint(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		e.uvarint(v.Uint())
	case reflect.Float32, reflect.Float64:
		e.uvarint(math.Float64bits(v.Float()))
	case reflect.String:
		s := v.String()
		e.uvarint(uint64(len(s)))
		e.buf = append(e.buf, s...)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := e.value(v.Index(i)); err != nil {
				return fmt.Errorf("[%d]%w", i, err)
			}
		}
	case reflect.Struct:
		t := v.Type()
		for _, i := range e.fields.of(t) {
			if err := e.value(v.Field(i)); err != nil {
				return fmt.Errorf(".%s%w", t.Field(i).Name, err)
			}
		}
	case reflect.Slice:
		if e.keyOnly {
			return fmt.Errorf(": map key holds a %v", v.Type())
		}
		return e.slice(v)
	case reflect.Pointer, reflect.Map, reflect.Interface:
		if e.keyOnly {
			return fmt.Errorf(": map key holds a %v, so the entries cannot be put in a fixed order", v.Type())
		}
		return e.reference(v)
	case reflect.Func:
		// Closures cannot be written; the loaded world keeps its own (see
		// World.afterLoad).
	default:
		return fmt.Errorf(": cannot save a %v", v.Type())
	}
	return nil
}

// slice writes a slice: its id, and the first time, its length and elements.
func (e *saveEncoder) slice(v reflect.Value) error {
	if v.IsNil() {
		e.uvarint(0)
		return nil
	}
	n := v.Len()
	// Empty slices share nothing worth keeping, and many have the same
	// address (Go's zero-size base), so each gets an id of its own.
	key := sliceKey{v.Pointer(), n, v.Type()}
	if id, ok := e.sliceIDs[key]; ok && n > 0 {
		e.uvarint(id)
		return nil
	}
	e.nextID++
	if n > 0 {
		e.sliceIDs[key] = e.nextID
	}
	e.uvarint(e.nextID)
	e.uvarint(uint64(n))
	e.cover(v.Pointer(), uintptr(n)*v.Type().Elem().Size(), v.Type())
	if v.Type().Elem().Kind() == reflect.Uint8 {
		e.buf = append(e.buf, v.Bytes()...)
		return nil
	}
	for i := 0; i < n; i++ {
		if err := e.value(v.Index(i)); err != nil {
			return fmt.Errorf("[%d]%w", i, err)
		}
	}
	return nil
}

// reference writes a pointer, map or interface value.
func (e *saveEncoder) reference(v reflect.Value) error {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			e.uvarint(0)
			return nil
		}
		if !e.object(v) {
			return nil
		}
		e.cover(v.Pointer(), v.Type().Elem().Size(), v.Type().Elem())
		return e.value(v.Elem())
	case reflect.Map:
		if v.IsNil() {
			e.uvarint(0)
			return nil
		}
		if !e.object(v) {
			return nil
		}
		return e.mapEntries(v)
	case reflect.Interface:
		if v.IsNil() {
			e.uvarint(0)
			return nil
		}
		c := v.Elem()
		name, ok := saveInterfaceNames[c.Type()]
		if !ok {
			return fmt.Errorf(": %v holds %v, which is not in saveInterfaceTypes", v.Type(), c.Type())
		}
		e.uvarint(uint64(len(name)))
		e.buf = append(e.buf, name...)
		return e.value(c)
	}
	return nil
}

// object writes the id of the pointer or map v and reports whether this is
// its first appearance, in which case its contents follow.
func (e *saveEncoder) object(v reflect.Value) (first bool) {
	addr := v.Pointer()
	if o, ok := e.ids[addr]; ok && o.typ == v.Type() {
		e.uvarint(o.id)
		return false
	}
	// A pointer to a struct and one to its first field share an address;
	// giving the second its own id lets checkOverlaps name the problem.
	e.nextID++
	e.ids[addr] = saveObject{id: e.nextID, typ: v.Type()}
	e.uvarint(e.nextID)
	return true
}

// mapEntries writes a map's length and its entries in key-byte order.
func (e *saveEncoder) mapEntries(v reflect.Value) error {
	if e.keyEnc == nil {
		e.keyEnc = &saveEncoder{fields: e.fields, keyOnly: true}
	}
	type entry struct {
		key []byte
		k   reflect.Value
	}
	entries := make([]entry, 0, v.Len())
	for it := v.MapRange(); it.Next(); {
		k := it.Key()
		e.keyEnc.buf = e.keyEnc.buf[:0]
		if err := e.keyEnc.value(k); err != nil {
			return fmt.Errorf("{key}%w", err)
		}
		entries = append(entries, entry{bytes.Clone(e.keyEnc.buf), k})
	}
	sort.Slice(entries, func(i, j int) bool { return bytes.Compare(entries[i].key, entries[j].key) < 0 })
	e.uvarint(uint64(len(entries)))
	for _, en := range entries {
		e.buf = append(e.buf, en.key...)
		if err := e.value(v.MapIndex(en.k)); err != nil {
			return fmt.Errorf("{%v}%w", en.k, err)
		}
	}
	return nil
}

// checkOverlaps refuses a graph in which two pointees or slices share memory:
// the loader would give each its own copy, and the sharing would be lost.
func (e *saveEncoder) checkOverlaps() error {
	slices.SortFunc(e.ranges, func(a, b memRange) int {
		switch {
		case a.start != b.start:
			if a.start < b.start {
				return -1
			}
			return 1
		case a.end != b.end:
			if a.end > b.end {
				return -1 // the enclosing range first
			}
			return 1
		}
		return 0
	})
	for i := 1; i < len(e.ranges); i++ {
		prev, cur := e.ranges[i-1], e.ranges[i]
		if cur.start < prev.end {
			return fmt.Errorf(": a %v and a %v share memory (an interior pointer or a re-sliced slice); "+
				"a load would split them, so mark one save:\"-\" and rebuild it in afterLoad, or stop sharing", prev.typ, cur.typ)
		}
	}
	return nil
}

type saveDecoder struct {
	buf    []byte
	pos    int
	fields *savedFields
	// objects[id-1] is the pointer or map loaded under that id.
	objects []reflect.Value
}

var errSaveTruncated = errors.New("save: the file ends early (truncated or corrupt)")

func newSaveDecoder(b []byte) *saveDecoder { return &saveDecoder{buf: b, fields: &savedFields{}} }

// decodeRoot reads into *root, which plays object 1 as in encodeRoot. Fields
// the codec skips keep the values root already has.
func (d *saveDecoder) decodeRoot(root any) error {
	v := reflect.ValueOf(root)
	d.objects = append(d.objects, v)
	if err := d.value(v.Elem()); err != nil {
		return err
	}
	if d.pos != len(d.buf) {
		return fmt.Errorf("save: %d bytes left over after the world (corrupt file)", len(d.buf)-d.pos)
	}
	return nil
}

func (d *saveDecoder) uvarint() (uint64, error) {
	x, n := binary.Uvarint(d.buf[d.pos:])
	if n <= 0 {
		return 0, errSaveTruncated
	}
	d.pos += n
	return x, nil
}

func (d *saveDecoder) varint() (int64, error) {
	x, n := binary.Varint(d.buf[d.pos:])
	if n <= 0 {
		return 0, errSaveTruncated
	}
	d.pos += n
	return x, nil
}

func (d *saveDecoder) bytes(n uint64) ([]byte, error) {
	if n > uint64(len(d.buf)-d.pos) {
		return nil, errSaveTruncated
	}
	b := d.buf[d.pos : d.pos+int(n)]
	d.pos += int(n)
	return b, nil
}

// settable makes an unexported struct field writable. The codec is the
// World's own serializer, reading and writing its private state on purpose.
func settable(v reflect.Value) reflect.Value {
	if v.CanSet() {
		return v
	}
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
}

func (d *saveDecoder) value(v reflect.Value) error {
	switch v.Kind() {
	case reflect.Bool:
		b, err := d.bytes(1)
		if err != nil {
			return err
		}
		v.SetBool(b[0] != 0)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		x, err := d.varint()
		if err != nil {
			return err
		}
		v.SetInt(x)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		x, err := d.uvarint()
		if err != nil {
			return err
		}
		v.SetUint(x)
	case reflect.Float32, reflect.Float64:
		x, err := d.uvarint()
		if err != nil {
			return err
		}
		v.SetFloat(math.Float64frombits(x))
	case reflect.String:
		n, err := d.uvarint()
		if err != nil {
			return err
		}
		b, err := d.bytes(n)
		if err != nil {
			return err
		}
		v.SetString(string(b))
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := d.value(v.Index(i)); err != nil {
				return fmt.Errorf("[%d]%w", i, err)
			}
		}
	case reflect.Struct:
		t := v.Type()
		for _, i := range d.fields.of(t) {
			if err := d.value(settable(v.Field(i))); err != nil {
				return fmt.Errorf(".%s%w", t.Field(i).Name, err)
			}
		}
	case reflect.Slice:
		id, err := d.uvarint()
		if err != nil {
			return err
		}
		switch {
		case id == 0:
			v.SetZero()
			return nil
		case id <= uint64(len(d.objects)):
			obj := d.objects[id-1]
			if obj.Type() != v.Type() {
				return fmt.Errorf(": object %d is a %v, not a %v (corrupt file)", id, obj.Type(), v.Type())
			}
			v.Set(obj)
			return nil
		case id != uint64(len(d.objects))+1:
			return fmt.Errorf(": object id %d out of sequence (corrupt file)", id)
		}
		n, err := d.uvarint()
		if err != nil {
			return err
		}
		if n > uint64(len(d.buf)-d.pos) && v.Type().Elem().Size() > 0 {
			return errSaveTruncated // every element takes at least a byte
		}
		s := reflect.MakeSlice(v.Type(), int(n), int(n))
		// Registered before the elements, which may lead back to it.
		d.objects = append(d.objects, s)
		if v.Type().Elem().Kind() == reflect.Uint8 {
			b, err := d.bytes(n)
			if err != nil {
				return err
			}
			copy(s.Bytes(), b)
		} else {
			for i := 0; i < int(n); i++ {
				if err := d.value(s.Index(i)); err != nil {
					return fmt.Errorf("[%d]%w", i, err)
				}
			}
		}
		v.Set(s)
	case reflect.Pointer:
		obj, first, err := d.object(v.Type())
		if err != nil || obj.IsValid() && !first {
			if err == nil {
				v.Set(obj)
			}
			return err
		}
		if !obj.IsValid() {
			v.SetZero()
			return nil
		}
		v.Set(obj)
		return d.value(obj.Elem())
	case reflect.Map:
		obj, first, err := d.object(v.Type())
		if err != nil || obj.IsValid() && !first {
			if err == nil {
				v.Set(obj)
			}
			return err
		}
		if !obj.IsValid() {
			v.SetZero()
			return nil
		}
		v.Set(obj)
		return d.mapEntries(obj)
	case reflect.Interface:
		n, err := d.uvarint()
		if err != nil {
			return err
		}
		if n == 0 {
			v.SetZero()
			return nil
		}
		name, err := d.bytes(n)
		if err != nil {
			return err
		}
		t, ok := saveInterfaceTypes[string(name)]
		if !ok {
			return fmt.Errorf(": unknown interface type %q", name)
		}
		c := reflect.New(t).Elem()
		if err := d.value(c); err != nil {
			return err
		}
		v.Set(c)
	case reflect.Func:
		// Not saved; see the encoder.
	default:
		return fmt.Errorf(": cannot load a %v", v.Type())
	}
	return nil
}

// object reads a pointer or map id. It returns the zero Value for nil; for an
// id seen before, the object loaded under it; for a new id, a fresh object
// (registered before its contents are read, so cycles resolve) and first set.
func (d *saveDecoder) object(t reflect.Type) (obj reflect.Value, first bool, err error) {
	id, err := d.uvarint()
	if err != nil || id == 0 {
		return reflect.Value{}, false, err
	}
	switch {
	case id <= uint64(len(d.objects)):
		obj = d.objects[id-1]
		if obj.Type() != t {
			return reflect.Value{}, false, fmt.Errorf(": object %d is a %v, not a %v (corrupt file)", id, obj.Type(), t)
		}
		return obj, false, nil
	case id == uint64(len(d.objects))+1:
		if t.Kind() == reflect.Map {
			obj = reflect.MakeMap(t)
		} else {
			obj = reflect.New(t.Elem())
		}
		d.objects = append(d.objects, obj)
		return obj, true, nil
	}
	return reflect.Value{}, false, fmt.Errorf(": object id %d out of sequence (corrupt file)", id)
}

func (d *saveDecoder) mapEntries(m reflect.Value) error {
	n, err := d.uvarint()
	if err != nil {
		return err
	}
	kt, vt := m.Type().Key(), m.Type().Elem()
	for i := uint64(0); i < n; i++ {
		k := reflect.New(kt).Elem()
		if err := d.value(k); err != nil {
			return fmt.Errorf("{key}%w", err)
		}
		val := reflect.New(vt).Elem()
		if err := d.value(val); err != nil {
			return fmt.Errorf("{%v}%w", k, err)
		}
		m.SetMapIndex(k, val)
	}
	return nil
}

// saveLayout fingerprints the shape of everything reachable from the given
// types: every type's kind and, for a struct, each saved field's name and
// type. The codec has no field names in the stream, so a save only loads into
// a build with the same layout; the header carries this so a mismatch fails
// up front with both commits named, rather than as garbage halfway through.
func saveLayout(roots ...reflect.Type) string {
	fields := &savedFields{}
	seen := map[reflect.Type]bool{}
	var lines []string
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		if seen[t] {
			return
		}
		seen[t] = true
		var b strings.Builder
		fmt.Fprintf(&b, "%s %s/%s %v", t.String(), t.PkgPath(), t.Name(), t.Kind())
		switch t.Kind() {
		case reflect.Array:
			fmt.Fprintf(&b, " %d", t.Len())
			walk(t.Elem())
		case reflect.Slice, reflect.Pointer:
			walk(t.Elem())
		case reflect.Map:
			walk(t.Key())
			walk(t.Elem())
		case reflect.Struct:
			for _, i := range fields.of(t) {
				f := t.Field(i)
				fmt.Fprintf(&b, " %s:%s", f.Name, f.Type.String())
				walk(f.Type)
			}
		}
		lines = append(lines, b.String())
	}
	for _, t := range roots {
		walk(t)
	}
	for _, t := range saveInterfaceTypes {
		walk(t)
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:8])
}
