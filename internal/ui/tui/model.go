package tui

import (
	"github.com/kevinmchugh/mars-sim/internal/sim"

	tea "github.com/charmbracelet/bubbletea"
)

// snapshotMsg carries a new world frame from the engine into the Bubble Tea
// update loop.
type snapshotMsg struct{ snap *sim.Snapshot }

// Model is the Bubble Tea model. It is a pure consumer of the engine: it draws
// the latest Snapshot and forwards key presses to the engine as Commands. It
// holds no game state of its own beyond the camera and the last frame.
type Model struct {
	eng   *sim.Engine
	snaps <-chan *sim.Snapshot

	latest *sim.Snapshot

	termW, termH int
	cam          sim.Point // world coordinate shown at the map's top-left
	camReady     bool

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
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		m.quitting = true
		return m, tea.Quit

	case " ":
		m.eng.Send(sim.TogglePause{})

	case "+", "=":
		m.eng.Send(sim.SetTicksPerSecond{Rate: m.currentTPS() + 2})
	case "-", "_":
		m.eng.Send(sim.SetTicksPerSecond{Rate: m.currentTPS() - 2})

	case "c":
		m.eng.Send(sim.Spawn{Kind: sim.Colonist})
	case "a":
		m.eng.Send(sim.Spawn{Kind: sim.Alien})

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
