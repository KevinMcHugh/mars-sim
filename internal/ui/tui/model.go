package tui

import (
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
// `b`) captures the next keypress as a selection instead of routing it to the
// current screen; `esc` cancels without sending a command.
type menuKind int

const (
	menuNone  menuKind = iota
	menuSpawn          // pick an entity kind to spawn
	menuBuild          // pick a room kind to queue
)

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
	selected    int      // roster: index into the ID-sorted colonist list
	jobSelected int      // job board: index into the queued project list
	menu        menuKind // an open spawn/build picker, if any

	quitting bool
}

// New builds a Model bound to an engine and its snapshot channel.
func New(eng *sim.Engine, snaps <-chan *sim.Snapshot) Model {
	return Model{eng: eng, snaps: snaps}
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

// handleMenuKey resolves an open spawn/build picker: a recognized selection
// key sends the command and closes the menu; esc cancels; anything else is
// ignored so the prompt stays open until the user answers it.
func (m Model) handleMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.menu = menuNone
		return m, nil
	}
	switch m.menu {
	case menuSpawn:
		switch msg.String() {
		case "c":
			m.eng.Send(sim.Spawn{Kind: sim.Colonist})
		case "a":
			m.eng.Send(sim.Spawn{Kind: sim.Alien})
		case "x":
			m.eng.Send(sim.Spawn{Kind: sim.Cat})
		case "m":
			m.eng.Send(sim.Spawn{Kind: sim.Mouse})
		default:
			return m, nil
		}
	case menuBuild:
		switch msg.String() {
		case "f":
			m.eng.Send(sim.OrderFacilityRoom{})
		case "d":
			m.eng.Send(sim.OrderDormitory{})
		default:
			return m, nil
		}
	}
	m.menu = menuNone
	return m, nil
}

// menuPrompt describes the open spawn/build picker for the footer, if any.
func (m Model) menuPrompt() (string, bool) {
	switch m.menu {
	case menuSpawn:
		return "spawn:  c colonist   a alien   x cat   m mouse   esc cancel", true
	case menuBuild:
		return "build:  f facility room   d dormitory   esc cancel", true
	default:
		return "", false
	}
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
// selection and returning to the map.
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
	}
	m.selected = m.clampSelection(m.selected)
	return m, nil
}

// clampSelection keeps a roster index within the current colonist list.
func (m Model) clampSelection(i int) int {
	n := m.colonistCount()
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

// colonistCount returns how many colonists are in the latest frame.
func (m Model) colonistCount() int {
	if m.latest == nil {
		return 0
	}
	n := 0
	for _, e := range m.latest.Entities {
		if e.Kind == sim.Colonist {
			n++
		}
	}
	return n
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
