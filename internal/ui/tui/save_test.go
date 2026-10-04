package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	tea "github.com/charmbracelet/bubbletea"
)

// ctrl+s saves the running game to a file in the working directory that
// loads back, and the footer says where it went.
func TestCtrlSSavesTheGame(t *testing.T) {
	t.Chdir(t.TempDir())
	eng := sim.NewEngine(sim.DefaultConfig())
	snaps := eng.Subscribe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Run(ctx)

	var m tea.Model = New(eng, snaps)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m, _ = m.Update(snapshotMsg{snap: <-snaps})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("ctrl+s returned no command")
	}
	msg, ok := cmd().(savedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("save: %+v", msg)
	}
	f, err := os.Open(msg.path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, _, err := sim.LoadEngine(f); err != nil {
		t.Fatalf("the saved file does not load: %v", err)
	}
	m, _ = m.Update(msg)
	if out := m.View(); !strings.Contains(out, "saved "+msg.path) {
		t.Errorf("footer does not report the save:\n%s", out)
	}
}
