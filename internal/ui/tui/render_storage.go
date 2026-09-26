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
	b.WriteString(labelStyle.Render(fmt.Sprintf("DETAILS"+divider+"STORAGE (%d)", len(storages))))
	b.WriteByte('\n')
	for i := start; i < end; i++ {
		storage := storages[i]
		marker := "•"
		if i == sel {
			marker = "›"
		}
		used, total := storageUsage(storage.Inventory)
		line := fmt.Sprintf("%s %s (%d,%d)  %d/%d slots", marker, m.storageLabel(storage),
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
	b.WriteString(titleStyle.Render(fmt.Sprintf("Storage %s (%d,%d)", m.storageLabel(storage), storage.Pos.X, storage.Pos.Y)))
	b.WriteString("\n")
	b.WriteString(statStyle.Render(fmt.Sprintf("%d/%d slots used"+divider+"%d/%d item capacity",
		used, total, storageItemCount(storage.Inventory), total*sim.MaxStackSize)))
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("CONTENTS"))
	b.WriteByte('\n')
	for _, line := range lines {
		b.WriteString(cells.Truncate(line, inner))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(labelStyle.Render("OWNED BY"))
	b.WriteByte('\n')
	for _, line := range m.ledgerLines(storage.Ledger) {
		b.WriteString(cells.Truncate(line, inner))
		b.WriteByte('\n')
	}
	return sidebarStyle.Width(width - borderCells).Height(rows - borderCells).MaxHeight(rows).Render(b.String())
}

func (m Model) renderCursorInspector() string {
	_, rows := m.viewportTiles()

	var b strings.Builder
	b.WriteString(labelStyle.Render("INSPECT"))
	b.WriteString(fmt.Sprintf("\n(%d,%d)\n%s", m.cursor.X, m.cursor.Y, m.terrainLabel(m.cursor)))
	if f, ok := m.latest.FixtureAt(m.cursor); ok && m.latest.ExploredAt(m.cursor) {
		access := f.Access.String()
		if f.Access == sim.AccessPaid {
			access = fmt.Sprintf("paid, %v a use", f.Price)
		}
		b.WriteString(fmt.Sprintf("\nOwner: %s\nAccess: %s", m.ownerLabel(f.Owner), access))
	}
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

// ledgerLines renders a container's ledger: who owns how many of what.
func (m Model) ledgerLines(ledger []sim.LedgerLine) []string {
	if len(ledger) == 0 {
		return []string{"nobody (empty)"}
	}
	lines := make([]string, 0, len(ledger))
	for _, l := range ledger {
		lines = append(lines, fmt.Sprintf("%s: %s ×%d", m.ownerLabel(l.Owner), l.Item, l.Count))
	}
	return lines
}

// storageLabel names a container by what it is to the colony: a shared chest,
// or someone's crash-pod locker.
func (m Model) storageLabel(st sim.StorageView) string {
	if st.Terrain == sim.Scumhouse {
		return "scumhouse"
	}
	if f, ok := m.latest.FixtureAt(st.Pos); ok && f.Access == sim.AccessPrivate {
		return m.ownerLabel(f.Owner) + "'s locker"
	}
	return "chest"
}
