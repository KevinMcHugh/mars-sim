package tui

import (
	"fmt"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	tea "github.com/charmbracelet/bubbletea"
)

// snapshotMsg carries a new world frame from the engine into the Bubble Tea
// update loop.
type snapshotMsg struct{ snap *sim.Snapshot }

// viewMode selects which screen the UI is showing.
type viewMode int

const (
	modeMap    viewMode = iota // the cavern map (default)
	modeRoster                 // the colonist roster and inspector
	modeJobs                   // the job board: queued projects and their tasks
)

// menuKind selects an open pick-one prompt, if any. Opening a menu (via `s` or
// `b`) captures keypresses instead of routing them to the current screen:
// up/down move the highlighted option, enter submits it, a shortcut letter
// jumps to and submits an option directly, and esc cancels without sending a
// command.
type menuKind int

const (
	menuNone   menuKind = iota
	menuSpawn           // pick an entity kind to spawn
	menuBuild           // pick a room kind to queue
	menuFilter          // toggle the roster's dead/non-human filters
)

// menuItem is one selectable option in a spawn/build menu.
type menuItem struct {
	key   string // shortcut key that jumps to and submits this option directly
	label string
}

var spawnMenuItems = []menuItem{
	{"c", "colonist"},
	{"a", "alien"},
	{"x", "cat"},
	{"m", "mouse"},
}

var buildMenuItems = []menuItem{
	{"f", "facility room"},
	{"d", "dormitory"},
}

// filterMenuItems are the roster's toggleable filters. Unlike the spawn/build
// pickers, each item is a checkbox: pressing its key (or enter on the
// highlighted one) flips it without closing the menu, since toggling more
// than one at a time is the normal case.
var filterMenuItems = []menuItem{
	{"d", "dead"},
	{"n", "non-human"},
}

// Model is the Bubble Tea model. It is a pure consumer of the engine: it draws
// the latest Snapshot and forwards key presses to the engine as Commands. It
// holds no game state of its own beyond the camera, the current screen, and the
// last frame.
type Model struct {
	eng   *sim.Engine
	snaps <-chan *sim.Snapshot

	latest *sim.Snapshot

	termW, termH int
	cam          sim.Point // world coordinate shown at the map's top-left
	camReady     bool

	mode        viewMode
	selected    int      // roster: index into the ID-sorted entity list
	jobSelected int      // job board: index into the queued project list
	menu        menuKind // an open spawn/build/filter picker, if any

	// spawnCursor / buildCursor / filterCursor are each menu's highlighted
	// option index. They persist across opens (and across submits/toggles),
	// so e.g. spawning three mice is s, [navigate to mouse], enter, then just
	// s, enter, s, enter.
	spawnCursor  int
	buildCursor  int
	filterCursor int

	// showDead / showNonHuman are the roster's filters, toggled from the
	// filter menu (see filterMenuItems). Both default off so the roster's
	// out-of-the-box view is unchanged: living colonists only.
	showDead     bool
	showNonHuman bool

	quitting bool

	// cache carries rendering work between frames; see renderCache.
	cache *renderCache
}

// New builds a Model bound to an engine and its snapshot channel.
func New(eng *sim.Engine, snaps <-chan *sim.Snapshot) Model {
	return Model{eng: eng, snaps: snaps, cache: newRenderCache()}
}

// Init starts listening for snapshots.
func (m Model) Init() tea.Cmd {
	return waitSnap(m.snaps)
}

// waitSnap blocks on the snapshot channel and turns the next frame into a
// message. Re-issued after every frame so the UI keeps following the sim.
func waitSnap(ch <-chan *sim.Snapshot) tea.Cmd {
	return func() tea.Msg {
		s, ok := <-ch
		if !ok {
			return nil // engine shut down
		}
		return snapshotMsg{s}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termW, m.termH = msg.Width, msg.Height
		return m, nil

	case snapshotMsg:
		if msg.snap == nil {
			m.quitting = true
			return m, tea.Quit
		}
		m.latest = msg.snap
		if !m.camReady {
			m.centerCamera()
			m.camReady = true
		}
		return m, waitSnap(m.snaps)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.menu != menuNone {
		return m.handleMenuKey(msg)
	}
	// Keys that mean the same thing on every screen.
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case " ":
		m.eng.Send(sim.TogglePause{})
		return m, nil
	case "+", "=":
		m.eng.Send(sim.SetTicksPerSecond{Rate: m.currentTPS() + 2})
		return m, nil
	case "-", "_":
		m.eng.Send(sim.SetTicksPerSecond{Rate: m.currentTPS() - 2})
		return m, nil
	case "tab":
		switch m.mode {
		case modeMap:
			m.mode = modeRoster
		case modeRoster:
			m.mode = modeJobs
		default:
			m.mode = modeMap
		}
		return m, nil
	case "s":
		m.menu = menuSpawn
		return m, nil
	case "b":
		m.menu = menuBuild
		return m, nil
	}
	switch m.mode {
	case modeRoster:
		return m.handleRosterKey(msg)
	case modeJobs:
		return m.handleJobsKey(msg)
	default:
		return m.handleMapKey(msg)
	}
}

// handleMenuKey resolves an open spawn/build/filter picker. The filter menu
// is a set of checkboxes rather than a one-shot pick, so it gets its own
// handler (handleFilterMenuKey) instead of the submit-and-close flow below.
func (m Model) handleMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.menu == menuFilter {
		return m.handleFilterMenuKey(msg)
	}
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.menu = menuNone
		return m, nil
	}

	items := m.menuItems()
	switch msg.String() {
	case "up", "k":
		m.setMenuCursor(wrapCursor(m.menuCursor()-1, len(items)))
		return m, nil
	case "down", "j":
		m.setMenuCursor(wrapCursor(m.menuCursor()+1, len(items)))
		return m, nil
	case "enter":
		m.submitMenuItem(m.menuCursor())
		m.menu = menuNone
		return m, nil
	}
	for i, it := range items {
		if it.key == msg.String() {
			m.setMenuCursor(i)
			m.submitMenuItem(i)
			m.menu = menuNone
			return m, nil
		}
	}
	return m, nil
}

// handleFilterMenuKey resolves the open filter menu: up/down move the
// highlighted checkbox, enter or space toggles it, a shortcut letter toggles
// its item directly wherever the highlight is, and esc closes the menu.
// Unlike handleMenuKey's spawn/build flow, toggling never closes the menu —
// setting both filters in one visit is the normal case.
func (m Model) handleFilterMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.menu = menuNone
		return m, nil
	case "up", "k":
		m.filterCursor = wrapCursor(m.filterCursor-1, len(filterMenuItems))
		return m, nil
	case "down", "j":
		m.filterCursor = wrapCursor(m.filterCursor+1, len(filterMenuItems))
		return m, nil
	case "enter", " ":
		m.toggleFilter(filterMenuItems[m.filterCursor].key)
		return m, nil
	}
	for i, it := range filterMenuItems {
		if it.key == msg.String() {
			m.filterCursor = i
			m.toggleFilter(it.key)
			return m, nil
		}
	}
	return m, nil
}

// toggleFilter flips the named filter's on/off state.
func (m *Model) toggleFilter(key string) {
	switch key {
	case "d":
		m.showDead = !m.showDead
	case "n":
		m.showNonHuman = !m.showNonHuman
	}
}

// filterOn reports the named filter's current on/off state.
func (m Model) filterOn(key string) bool {
	switch key {
	case "d":
		return m.showDead
	case "n":
		return m.showNonHuman
	default:
		return false
	}
}

// wrapCursor keeps a menu selection cycling within [0, n).
func wrapCursor(i, n int) int {
	if n <= 0 {
		return 0
	}
	return ((i % n) + n) % n
}

// menuItems returns the open menu's options, or nil if none is open.
func (m Model) menuItems() []menuItem {
	switch m.menu {
	case menuSpawn:
		return spawnMenuItems
	case menuBuild:
		return buildMenuItems
	default:
		return nil
	}
}

// menuCursor returns the open menu's highlighted index.
func (m Model) menuCursor() int {
	switch m.menu {
	case menuSpawn:
		return m.spawnCursor
	case menuBuild:
		return m.buildCursor
	default:
		return 0
	}
}

// setMenuCursor moves the open menu's highlight, remembering it per menu kind
// so it reopens on the same selection next time.
func (m *Model) setMenuCursor(i int) {
	switch m.menu {
	case menuSpawn:
		m.spawnCursor = i
	case menuBuild:
		m.buildCursor = i
	}
}

// submitMenuItem sends the command for the open menu's i-th option.
func (m Model) submitMenuItem(i int) {
	items := m.menuItems()
	if i < 0 || i >= len(items) {
		return
	}
	switch m.menu {
	case menuSpawn:
		switch items[i].key {
		case "c":
			m.eng.Send(sim.Spawn{Kind: sim.Colonist})
		case "a":
			m.eng.Send(sim.Spawn{Kind: sim.Alien})
		case "x":
			m.eng.Send(sim.Spawn{Kind: sim.Cat})
		case "m":
			m.eng.Send(sim.Spawn{Kind: sim.Mouse})
		}
	case menuBuild:
		switch items[i].key {
		case "f":
			m.eng.Send(sim.OrderFacilityRoom{})
		case "d":
			m.eng.Send(sim.OrderDormitory{})
		}
	}
}

// menuPrompt describes the open spawn/build/filter picker for the footer, if
// any, bracketing the highlighted option.
func (m Model) menuPrompt() (string, bool) {
	if m.menu == menuFilter {
		return m.filterPrompt(), true
	}
	items := m.menuItems()
	if items == nil {
		return "", false
	}
	label := "spawn"
	if m.menu == menuBuild {
		label = "build"
	}
	cursor := m.menuCursor()
	parts := make([]string, len(items))
	for i, it := range items {
		text := it.key + " " + it.label
		if i == cursor {
			text = "[" + text + "]"
		}
		parts[i] = text
	}
	return label + ":  " + strings.Join(parts, "   ") + "   ↑↓ select  enter confirm  esc cancel", true
}

// filterPrompt describes the open filter menu for the footer: each checkbox's
// current on/off state, with the highlighted one bracketed.
func (m Model) filterPrompt() string {
	parts := make([]string, len(filterMenuItems))
	for i, it := range filterMenuItems {
		state := "off"
		if m.filterOn(it.key) {
			state = "on"
		}
		text := fmt.Sprintf("%s %s: %s", it.key, it.label, state)
		if i == m.filterCursor {
			text = "[" + text + "]"
		}
		parts[i] = text
	}
	return "filter:  " + strings.Join(parts, "   ") + "   ↑↓ select  enter/space toggle  esc close"
}

// handleMapKey handles keys specific to the map screen.
func (m Model) handleMapKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.quitting = true
		return m, tea.Quit

	case "left", "h":
		m.panCamera(-4, 0)
	case "right", "l":
		m.panCamera(4, 0)
	case "up", "k":
		m.panCamera(0, -2)
	case "down", "j":
		m.panCamera(0, 2)
	}
	return m, nil
}

// handleRosterKey handles keys specific to the roster screen: moving the
// selection, opening the filter menu, and returning to the map.
func (m Model) handleRosterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeMap
	case "up", "k":
		m.selected--
	case "down", "j":
		m.selected++
	case "home", "g":
		m.selected = 0
	case "f":
		m.menu = menuFilter
		return m, nil
	}
	m.selected = m.clampSelection(m.selected)
	return m, nil
}

// clampSelection keeps a roster index within the current filtered entity list.
func (m Model) clampSelection(i int) int {
	n := m.rosterCount()
	if n == 0 {
		return 0
	}
	return clamp(i, 0, n-1)
}

// handleJobsKey handles keys specific to the job board screen: moving the
// selection, queueing new work, and returning to the map.
func (m Model) handleJobsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeMap
	case "up", "k":
		m.jobSelected--
	case "down", "j":
		m.jobSelected++
	case "home", "g":
		m.jobSelected = 0
	}
	m.jobSelected = m.clampJobSelection(m.jobSelected)
	return m, nil
}

// clampJobSelection keeps a job board index within the current project list.
func (m Model) clampJobSelection(i int) int {
	n := m.projectCount()
	if n == 0 {
		return 0
	}
	return clamp(i, 0, n-1)
}

// projectCount returns how many projects are queued in the latest frame.
func (m Model) projectCount() int {
	if m.latest == nil {
		return 0
	}
	return len(m.latest.Projects)
}

// rosterCount returns how many entities the roster currently shows, honoring
// the dead/non-human filters (see rosterEntries in render_roster.go).
func (m Model) rosterCount() int {
	return len(m.rosterEntries())
}

func (m Model) View() string {
	if m.quitting {
		return "Signing off Mars Colony. Stay wary of the tunnels.\n"
	}
	return m.render()
}

func (m Model) currentTPS() int {
	if m.latest != nil {
		return m.latest.TicksPerSecond
	}
	return 8
}

// centerCamera points the viewport at the middle of the world.
func (m *Model) centerCamera() {
	if m.latest == nil {
		return
	}
	cols, rows := m.viewportTiles()
	m.cam = sim.Point{
		X: m.latest.Width/2 - cols/2,
		Y: m.latest.Height/2 - rows/2,
	}
	m.clampCamera()
}

func (m *Model) panCamera(dx, dy int) {
	m.cam = m.cam.Add(dx, dy)
	m.clampCamera()
}

// clampCamera keeps the viewport within the world bounds.
func (m *Model) clampCamera() {
	if m.latest == nil {
		return
	}
	cols, rows := m.viewportTiles()
	maxX := max(0, m.latest.Width-cols)
	maxY := max(0, m.latest.Height-rows)
	m.cam.X = clamp(m.cam.X, 0, maxX)
	m.cam.Y = clamp(m.cam.Y, 0, maxY)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
