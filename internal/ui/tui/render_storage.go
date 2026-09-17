package tui

import (
	"fmt"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"

	"github.com/charmbracelet/lipgloss"
)

const storageListWidth = 32

// renderStorage draws the storage tab of the details panel. Up/down navigates
// containers in deterministic snapshot order.
func (m Model) renderStorage() string {
	header := m.renderHeader()
	footer := m.footerLine("DETAILS  ↑↓/jk select container  tab/esc map  s spawn  b build  space pause  q quit")
	rows := m.rosterRows()
	if len(m.latest.Storages) == 0 {
		body := sidebarStyle.Width(m.termW - borderCells).Height(rows - borderCells).
			Render("STORAGE\n\nNo storage containers have been built.\n\nPress b, then r, to order one.")
		return strings.Join([]string{header, body, footer}, "\n")
	}

	sel := clamp(m.storageSelected, 0, len(m.latest.Storages)-1)
	listWidth, detailWidth := m.splitPanels(storageListWidth)
	body := m.renderStorageList(m.latest.Storages, sel, rows, listWidth)
	if detailWidth > 0 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body,
			strings.Repeat(" ", panelGap),
			m.renderStorageDetail(m.latest.Storages[sel], rows, detailWidth))
	}
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) renderStorageList(storages []sim.StorageView, sel, rows, width int) string {
	inner := panelInner(width)
	capacity := max(1, rows-3)
	start := max(0, sel-capacity+1)
	end := min(len(storages), start+capacity)

	var b strings.Builder
	b.WriteString(labelStyle.Render(fmt.Sprintf("DETAILS · STORAGE (%d)", len(storages))))
	b.WriteByte('\n')
	for i := start; i < end; i++ {
		storage := storages[i]
		marker := "•"
		if i == sel {
			marker = "›"
		}
		used, total := storageUsage(storage.Inventory)
		line := fmt.Sprintf("%s chest (%d,%d)  %d/%d slots", marker,
			storage.Pos.X, storage.Pos.Y, used, total)
		if i == sel {
			b.WriteString(rosterSelStyle.Render(cells.Truncate(line, inner)))
		} else {
			b.WriteString(cells.Truncate(line, inner))
		}
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return sidebarStyle.Width(width - borderCells).Height(rows - borderCells).MaxHeight(rows).Render(b.String())
}

func (m Model) renderStorageDetail(storage sim.StorageView, rows, width int) string {
	inner := panelInner(width)
	used, total := storageUsage(storage.Inventory)
	lines := storageContentLines(storage.Inventory)

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Storage chest (%d,%d)", storage.Pos.X, storage.Pos.Y)))
	b.WriteString("\n")
	b.WriteString(statStyle.Render(fmt.Sprintf("%d/%d slots used · %d/%d item capacity",
		used, total, storageItemCount(storage.Inventory), total*sim.MaxStackSize)))
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("CONTENTS"))
	b.WriteByte('\n')
	for _, line := range lines {
		b.WriteString(cells.Truncate(line, inner))
		b.WriteByte('\n')
	}
	return sidebarStyle.Width(width - borderCells).Height(rows - borderCells).MaxHeight(rows).Render(b.String())
}

func (m Model) renderCursorInspector() string {
	_, rows := m.viewportTiles()
	tile := m.latest.TileAt(m.cursor)

	var b strings.Builder
	b.WriteString(labelStyle.Render("INSPECT"))
	b.WriteString(fmt.Sprintf("\n(%d,%d)\n%s", m.cursor.X, m.cursor.Y, tile.Terrain))
	if i := m.storageIndexAt(m.cursor); i >= 0 {
		storage := m.latest.Storages[i]
		used, total := storageUsage(storage.Inventory)
		b.WriteString(fmt.Sprintf("\n\n%d/%d slots used\n", used, total))
		b.WriteString(labelStyle.Render("CONTENTS"))
		b.WriteByte('\n')
		content := storageContentLines(storage.Inventory)
		maxContent := max(1, rows-10)
		if len(content) > maxContent {
			hidden := len(content) - maxContent + 1
			content = append(content[:maxContent-1], fmt.Sprintf("… %d more stacks", hidden))
		}
		b.WriteString(strings.Join(content, "\n"))
		b.WriteString("\n\nEnter: open storage details")
	}
	return sidebarStyle.Width(sidebarWidth - borderCells).Height(rows - borderCells).Render(b.String())
}

func storageUsage(inv sim.StorageInventory) (used, total int) {
	for _, stack := range inv {
		if stack.Count > 0 {
			used++
		}
	}
	return used, len(inv)
}

func storageItemCount(inv sim.StorageInventory) int {
	n := 0
	for _, stack := range inv {
		n += stack.Count
	}
	return n
}

func storageContentLines(inv sim.StorageInventory) []string {
	var lines []string
	for i, stack := range inv {
		if stack.Count > 0 {
			lines = append(lines, fmt.Sprintf("%2d  %s ×%d", i+1, stack.Kind, stack.Count))
		}
	}
	if len(lines) == 0 {
		return []string{"empty"}
	}
	return lines
}
