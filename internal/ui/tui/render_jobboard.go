package tui

import (
	"fmt"
	"strings"

	"github.com/kevinmchugh/mars-sim/internal/sim"

	"github.com/charmbracelet/lipgloss"
)

// jobListWidth mirrors rosterListWidth: room for a project name, tick counts,
// a task-progress fraction, and the selection marker, plus panel chrome.
const jobListWidth = 40

func (m Model) renderJobs() string {
	header := m.renderHeader()
	footer := helpStyle.Render("↑↓/jk select  f facility room  d dormitory  tab/esc map  space pause  q quit")

	projects := m.latest.Projects
	rows := m.termH - headerRows - footerRows
	if rows < minRows {
		rows = minRows
	}

	if len(projects) == 0 {
		body := m.renderNoProjects(rows)
		return strings.Join([]string{header, body, footer}, "\n")
	}

	sel := clamp(m.jobSelected, 0, len(projects)-1)
	list := m.renderProjectList(projects, sel, rows)
	detail := m.renderProjectDetail(projects[sel], rows)
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, " ", detail)
	return strings.Join([]string{header, body, footer}, "\n")
}

// renderNoProjects shows any manual orders still waiting for a build site, so
// the board is not just a blank "nothing queued" screen while colonists dig
// toward room.
func (m Model) renderNoProjects(rows int) string {
	var b strings.Builder
	b.WriteString("No jobs queued.\n")
	if n := m.latest.PendingFacilityRooms; n > 0 {
		b.WriteString(fmt.Sprintf("%d facility room order(s) waiting for a build site.\n", n))
	}
	if n := m.latest.PendingDormitories; n > 0 {
		b.WriteString(fmt.Sprintf("%d dormitory order(s) waiting for a build site.\n", n))
	}
	b.WriteString("\nPress f or d to queue a facility room or dormitory.")
	return sidebarStyle.Width(m.termW - 2).Height(rows - 2).Render(b.String())
}

// renderProjectList draws the scrolling list of queued projects, keeping the
// selection in view.
func (m Model) renderProjectList(projects []sim.ProjectView, sel, rows int) string {
	capacity := (rows - 3) / 3 // box borders + heading, with three lines per project
	if capacity < 1 {
		capacity = 1
	}
	start := 0
	if sel >= capacity {
		start = sel - capacity + 1
	}
	end := start + capacity
	if end > len(projects) {
		end = len(projects)
	}

	var b strings.Builder
	b.WriteString(labelStyle.Render(fmt.Sprintf("JOB BOARD (%d)", len(projects))))
	b.WriteByte('\n')
	for i := start; i < end; i++ {
		p := projects[i]
		nameLine := truncate(p.Name, jobListWidth-4)
		progressLine := fmt.Sprintf("%d/%d tasks · %d assigned", p.TasksDone(), len(p.Tasks), len(p.Assignees()))
		queuedLine := fmt.Sprintf("queued t%d · %d ticks ago", p.QueuedTick, m.latest.Tick-p.QueuedTick)
		marker := "•"
		if i == sel {
			marker = "›"
			b.WriteString(rosterSelStyle.Render(marker + " " + nameLine))
		} else {
			b.WriteString(marker + " " + nameLine)
		}
		b.WriteByte('\n')
		b.WriteString("  " + truncate(progressLine, jobListWidth-4) + "\n")
		b.WriteString("  " + truncate(queuedLine, jobListWidth-4))
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return sidebarStyle.Width(jobListWidth - 2).Height(rows - 2).MaxHeight(rows - 2).Render(b.String())
}

// renderProjectDetail draws the inspector for one project: its queued time,
// overall progress, and every task with its builder (if any).
func (m Model) renderProjectDetail(p sim.ProjectView, rows int) string {
	width := m.termW - jobListWidth - 3
	if width < 24 {
		width = 24
	}

	names := m.colonistNames()

	var b strings.Builder
	b.WriteString(titleStyle.Render(p.Name) + "\n")
	b.WriteString(statStyle.Render(fmt.Sprintf(
		"queued tick %d · %d ticks ago", p.QueuedTick, m.latest.Tick-p.QueuedTick,
	)) + "\n\n")

	remaining := p.TasksRemaining()
	b.WriteString(labelStyle.Render("PROGRESS") + "  " +
		fmt.Sprintf("%d/%d built · %d action(s) remaining\n", p.TasksDone(), len(p.Tasks), remaining))

	assignees := p.Assignees()
	b.WriteString(labelStyle.Render("ASSIGNED") + "  ")
	if len(assignees) == 0 {
		b.WriteString("nobody yet\n\n")
	} else {
		who := make([]string, 0, len(assignees))
		for _, id := range assignees {
			who = append(who, colonistDisplayName(names, id))
		}
		b.WriteString(truncate(strings.Join(who, ", "), width-10) + "\n\n")
	}

	b.WriteString(labelStyle.Render("TASKS") + "\n")
	for _, t := range p.Tasks {
		status := "queued"
		switch {
		case t.Done:
			status = "done"
		case t.Owner != 0:
			status = "building: " + colonistDisplayName(names, t.Owner)
		}
		line := fmt.Sprintf("(%d,%d) %s — %s", t.Pos.X, t.Pos.Y, t.Terrain, status)
		style := statStyle
		if t.Done {
			style = traitStyle
		}
		b.WriteString(style.Render(truncate(line, width-4)) + "\n")
	}

	return sidebarStyle.Width(width).Height(rows - 2).MaxHeight(rows - 2).Render(b.String())
}

// colonistDisplayName looks up a colonist's name by ID, falling back to a
// numbered label if it is not (or no longer) in the latest frame.
func colonistDisplayName(names map[sim.EntityID]string, id sim.EntityID) string {
	if name, ok := names[id]; ok {
		return name
	}
	return fmt.Sprintf("colonist #%d", id)
}
