package sim

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
)

// A save file is the whole World: every tile, entity, project, order and
// cache, and the state of every RNG stream, so a loaded game plays on exactly
// as the saved one would have. See docs/save-load.md.
//
// The file is two text lines and a gzip stream:
//
//	mars-sim save
//	{"format":1,"commit":"…","layout":"…","seed":…,"tick":…,…}
//	<gzip: the Config, then the World, in the save codec (savecodec.go)>
//
// The header is plain JSON so `head -2` says which build wrote a save and
// when in the game it was taken, without loading it.

// SaveFileExtension is the extension frontends give save files.
const SaveFileExtension = ".marssave"

const saveMagic = "mars-sim save"

// saveFormat versions the file's framing (the header and the codec's stream
// grammar), not the World's layout; SaveInfo.Layout covers that. Bump it when
// either changes shape.
const saveFormat = 1

// SaveInfo is a save file's header.
type SaveInfo struct {
	Format int `json:"format"`
	// Commit is the git revision of the build that wrote the save (see
	// BuildCommit), for tracing a save back to the code that made it.
	Commit string `json:"commit"`
	// Layout fingerprints the World's type graph. A save loads only into a
	// build with the same layout; see saveLayout.
	Layout string `json:"layout"`
	Seed   int64  `json:"seed"`
	Tick   int    `json:"tick"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// TicksPerSecond and Paused are the engine's, as they were when saved;
	// a loaded engine starts the same way.
	TicksPerSecond int  `json:"tps"`
	Paused         bool `json:"paused"`
}

// buildCommit can be set at link time (-ldflags "-X
// github.com/kevinmchugh/mars-sim/internal/sim.buildCommit=<sha>") for a
// build made outside a git checkout. Otherwise BuildCommit reads the revision
// Go stamps into binaries built with `go build` inside one.
var buildCommit string

// BuildCommit returns the git commit this binary was built from, with
// "-dirty" appended if the tree had uncommitted changes, or "unknown" when
// there is no record of it (go run and go test do not stamp one).
func BuildCommit() string { return buildCommitOnce() }

var buildCommitOnce = sync.OnceValue(func() string {
	if buildCommit != "" {
		return buildCommit
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "unknown"
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
})

// currentSaveLayout is saveLayout for this build's Config and World.
var currentSaveLayout = sync.OnceValue(func() string {
	return saveLayout(reflect.TypeFor[Config](), reflect.TypeFor[World]())
})

// Save writes the game to out. It reads the World, so it must run on the
// goroutine that drives the engine: call it between Advance calls, or, while
// Run owns the engine, send a SaveGame command instead.
func (e *Engine) Save(out io.Writer) error {
	return e.world.save(out, SaveInfo{TicksPerSecond: e.tps, Paused: e.paused})
}

// Config returns the settings the engine's world runs with: for a loaded
// game, the ones it was saved with, not the host's. Call it before Run or
// Advance; after that the world belongs to the engine's goroutine.
func (e *Engine) Config() Config { return e.world.cfg }

// SaveGame asks a running engine to save itself. The engine writes the game
// to To between ticks and then sends the result on Done, which should be
// buffered so the engine never waits on the frontend.
type SaveGame struct {
	To   io.Writer
	Done chan<- error
}

func (SaveGame) isCommand() {}

func (w *World) save(out io.Writer, info SaveInfo) error {
	info.Format = saveFormat
	info.Commit = BuildCommit()
	info.Layout = currentSaveLayout()
	info.Seed, info.Tick = w.cfg.Seed, w.tick
	info.Width, info.Height = w.Width, w.Height
	header, err := json.Marshal(info)
	if err != nil {
		return err
	}

	// The Config goes first, on its own, because loading needs it to build
	// the World the rest is read into (see LoadEngine).
	cfgEnc := newSaveEncoder()
	cfg := w.cfg
	if err := cfgEnc.encodeRoot(&cfg); err != nil {
		return fmt.Errorf("save: config%w", err)
	}
	worldEnc := newSaveEncoder()
	if err := worldEnc.encodeRoot(w); err != nil {
		return fmt.Errorf("save: world%w", err)
	}

	bw := bufio.NewWriter(out)
	fmt.Fprintf(bw, "%s\n%s\n", saveMagic, header)
	zw := gzip.NewWriter(bw)
	var n [2 * 10]byte
	lens := appendUvarints(n[:0], uint64(len(cfgEnc.buf)), uint64(len(worldEnc.buf)))
	for _, b := range [][]byte{lens, cfgEnc.buf, worldEnc.buf} {
		if _, err := zw.Write(b); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return bw.Flush()
}

func appendUvarints(b []byte, xs ...uint64) []byte {
	e := saveEncoder{buf: b}
	for _, x := range xs {
		e.uvarint(x)
	}
	return e.buf
}

// ReadSaveInfo reads a save file's header without loading the game.
func ReadSaveInfo(in io.Reader) (SaveInfo, error) {
	info, _, err := readSaveHeader(bufio.NewReader(in))
	return info, err
}

func readSaveHeader(br *bufio.Reader) (SaveInfo, *bufio.Reader, error) {
	var info SaveInfo
	magic, err := br.ReadString('\n')
	if err != nil || strings.TrimSuffix(magic, "\n") != saveMagic {
		return info, nil, errors.New("not a mars-sim save file")
	}
	line, err := br.ReadBytes('\n')
	if err != nil {
		return info, nil, errSaveTruncated
	}
	if err := json.Unmarshal(line, &info); err != nil {
		return info, nil, fmt.Errorf("save: unreadable header: %w", err)
	}
	return info, br, nil
}

// LoadEngine reads a save file and returns an engine holding the game, paused
// or running and at the speed it was saved at. The game is exactly the one
// saved, including every RNG stream, so it plays on as the original would
// have. A host that shares live tiles calls ShareLiveTiles on the result as it
// would on a new engine.
//
// A save only loads into a build whose World has the same layout as the one
// that wrote it; mars-sim keeps no compatibility with older layouts, and the
// error names both commits.
func LoadEngine(in io.Reader) (*Engine, SaveInfo, error) {
	info, br, err := readSaveHeader(bufio.NewReader(in))
	if err != nil {
		return nil, info, err
	}
	if info.Format != saveFormat {
		return nil, info, fmt.Errorf("save is format %d, written by commit %s; this build (commit %s) reads format %d",
			info.Format, info.Commit, BuildCommit(), saveFormat)
	}
	if info.Layout != currentSaveLayout() {
		return nil, info, fmt.Errorf("save was written by commit %s, whose world layout (%s) differs from this build's "+
			"(commit %s, layout %s); saves do not load across layout changes", info.Commit, info.Layout, BuildCommit(), currentSaveLayout())
	}
	zr, err := gzip.NewReader(br)
	if err != nil {
		return nil, info, fmt.Errorf("save: %w", err)
	}
	body, err := io.ReadAll(zr)
	if err != nil {
		return nil, info, fmt.Errorf("save: %w", err)
	}
	w, err := loadWorld(body)
	if err != nil {
		return nil, info, err
	}
	return &Engine{
		world:  w,
		cmds:   make(chan Command, 32),
		tps:    max(info.TicksPerSecond, 1),
		paused: info.Paused,
	}, info, nil
}

// loadWorld rebuilds a World from a save's body: the Config, then the World.
// Save files come from players, so a corrupt one that slips past the
// decoder's checks into a panic is reported as an error, not a crash.
func loadWorld(body []byte) (w *World, err error) {
	defer func() {
		if r := recover(); r != nil {
			w, err = nil, fmt.Errorf("save: corrupt file (%v)", r)
		}
	}()
	d := newSaveDecoder(body)
	cfgLen, err := d.uvarint()
	if err != nil {
		return nil, err
	}
	worldLen, err := d.uvarint()
	if err != nil {
		return nil, err
	}
	if rest := uint64(len(body) - d.pos); cfgLen > rest || worldLen != rest-cfgLen {
		return nil, errSaveTruncated
	}
	cfgBytes := body[d.pos : d.pos+int(cfgLen)]
	worldBytes := body[d.pos+int(cfgLen):]

	var cfg Config
	if err := newSaveDecoder(cfgBytes).decodeRoot(&cfg); err != nil {
		return nil, fmt.Errorf("save: config%w", err)
	}
	// newWorld gives the fields the save leaves out (save:"-") their
	// values: the event subscribers above all, closures over this world.
	// Everything else it sets is overwritten by the decode.
	w = newWorld(cfg, newPCG(cfg.Seed))
	if err := newSaveDecoder(worldBytes).decodeRoot(w); err != nil {
		return nil, fmt.Errorf("save: world%w", err)
	}
	w.afterLoad()
	return w, nil
}

// afterLoad restores what the save codec cannot write: closures and the
// fields tagged save:"-". Every new func field reachable from the World needs
// a line here (TestSaveFuncFieldsAreRebound fails until it has one).
func (w *World) afterLoad() {
	for kind, f := range w.fields {
		if f != nil {
			f.seed, f.goal = facilitySeed(w, Terrain(kind)), facilityGoal(w, Terrain(kind))
		}
	}
	if w.frontier != nil {
		w.frontier.seed, w.frontier.goal = frontierSeed(w), frontierGoal(w)
	}
	if w.gen != nil && !w.cfg.FogOfWar {
		w.preview = newChunkPreview(w.cfg)
	}
}

// SaveBytes is Save into memory, for hosts (the browser) that hand the file
// over as a buffer.
func (e *Engine) SaveBytes() ([]byte, error) {
	var b bytes.Buffer
	if err := e.Save(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
