package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Basic ANSI colors (0-15) come from your terminal theme, so the app
// matches whatever colors your terminal uses.
var (
	accent = lipgloss.Color("4") // blue
	subtle = lipgloss.Color("8") // gray

	headerStyle       = lipgloss.NewStyle().Bold(true).Foreground(accent).Padding(0, 1)
	countStyle        = lipgloss.NewStyle().Foreground(subtle).Padding(0, 1)
	panelStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(subtle).Padding(0, 1)
	selectedStyle     = lipgloss.NewStyle().Background(accent).Foreground(lipgloss.Color("0")).Bold(true)
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(accent)
	labelStyle        = lipgloss.NewStyle().Foreground(subtle)
	columnHeaderStyle = lipgloss.NewStyle().Bold(true)
	statValueStyle    = lipgloss.NewStyle().Bold(true)
	barStyle          = lipgloss.NewStyle().Foreground(accent)
	helpStyle         = lipgloss.NewStyle().Foreground(subtle).Padding(0, 1)
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Padding(0, 1) // red
	noticeStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Padding(0, 1) // green
)

// Lines used by the header and help bar; the panels get the rest of the height.
const chromeHeight = 2

func (m model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("loading…") // we don't know the terminal size yet
	}

	count := plural(len(m.movies), "movie")
	if len(m.visible) != len(m.movies) {
		count = fmt.Sprintf("%d/%s", len(m.visible), count)
	}
	if summary := m.activeFilterSummary(); summary != "" {
		count += " · " + summary
	}
	left := headerStyle.Render("MovieManager") + countStyle.Render(count)
	if m.mode == searching || m.search.Value() != "" {
		left += " " + m.search.View()
	}
	header := spaceBetween(left, m.syncStatus(), m.width)

	// The table takes the full width; the filter panel takes ~20% when open.
	panelHeight := m.height - chromeHeight
	innerHeight := panelHeight - panelStyle.GetVerticalFrameSize()
	tableWidth, filterWidth := m.width, 0
	if m.mode == filtering {
		filterWidth = max(m.width/5, 24)
		tableWidth = m.width - filterWidth
	}

	tableInner := tableWidth - panelStyle.GetHorizontalFrameSize()
	content := m.tableView(tableInner)
	if m.mode == viewingStats {
		content = m.statsView(tableInner, innerHeight)
	}
	if m.isAdding() {
		content = m.addView(tableInner, innerHeight)
	}
	panels := panelStyle.Width(tableWidth).Height(panelHeight).Render(
		fitBlock(content, tableInner, innerHeight))
	if m.mode == filtering {
		filterInner := filterWidth - panelStyle.GetHorizontalFrameSize()
		filters := panelStyle.Width(filterWidth).Height(panelHeight).Render(
			fitBlock(m.filterView(innerHeight), filterInner, innerHeight))
		panels = lipgloss.JoinHorizontal(lipgloss.Top, panels, filters)
	}

	help := helpStyle.Render(m.helpText())
	if m.notice != "" {
		help = noticeStyle.Render("✓ " + m.notice)
	}
	if m.mode == confirmingDelete {
		help = errorStyle.Render(fmt.Sprintf("Delete %q (%d) from the sheet? y/n", m.deleting.Title, m.deleting.Year))
	}
	if m.flash != nil {
		help = errorStyle.Render("⚠ " + m.flash.Error())
	}
	help = ansi.Truncate(help, m.width, "…")

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left,
		header,
		panels,
		help,
	))
	v.AltScreen = true
	return v
}

// sortArrow shows the sort direction: ↑ for A→Z / low→high, ↓ for the reverse.
func (m model) sortArrow() string {
	if m.sortReverse {
		return "↓"
	}
	return "↑"
}

// formatDuration turns minutes into "2h4m". Unknown (0) shows as empty.
func formatDuration(minutes int) string {
	h, m := minutes/60, minutes%60
	switch {
	case minutes <= 0:
		return ""
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh%dm", h, m)
	}
}

// helpText lists the keys that work in the current mode.
func (m model) helpText() string {
	switch m.mode {
	case searching:
		return "type to filter · ↑/↓ move · enter keep · esc clear"
	case filtering:
		return "j/k move · space toggle · c clear all · enter/esc done"
	case viewingStats:
		return "t/esc back · stats follow your current search and filters"
	case addingSearch:
		if m.review.active {
			return "enter search TMDB · tab fill in by hand · ctrl+x skip movie · esc stop review"
		}
		return "enter search TMDB · tab fill in by hand · esc cancel"
	case addingPick:
		if m.review.active {
			return "j/k move · enter use this one · tab fill in by hand · ctrl+x skip movie · esc edit search"
		}
		return "j/k move · enter use this one · tab fill in by hand · esc back"
	case addingForm:
		if m.review.active {
			return "ctrl+s save & next · ctrl+o sheet/TMDB value · ctrl+x skip · ctrl+t other match · tab/↑/↓ move · esc stop"
		}
		if m.formCursor == fieldGenre {
			return "→ complete genre · ctrl+n/p other matches · tab/↑/↓ move · ctrl+s save · esc cancel"
		}
		return "tab/↑/↓ move · ctrl+s save (or enter on the last field) · esc cancel"
	}
	help := "j/k move · / search · f filter · s sort · S reverse · a add · d delete · u/U review · t stats · R refresh · q quit"
	if m.search.Value() != "" || m.activeFilterSummary() != "" {
		help = "esc clear · " + help
	}
	return help
}

// fitBlock cuts s to at most height lines of at most width columns, so the
// content never wraps or overflows its panel.
func fitBlock(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	lines = lines[:min(len(lines), max(height, 0))]
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(lines, "\n")
}

// plural formats a count with its noun: "1 movie", "3 movies".
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// syncStatus describes where the movies came from, e.g. "synced 5m ago".
func (m model) syncStatus() string {
	last := "never synced"
	if !m.syncedAt.IsZero() {
		last = "synced " + timeAgo(m.syncedAt)
	}
	switch {
	case m.saving:
		return countStyle.Render("saving…")
	case m.syncing:
		return countStyle.Render("syncing…")
	case m.syncErr != nil:
		return errorStyle.Render("sync failed · " + last)
	default:
		return countStyle.Render(last)
	}
}

// spaceBetween puts left and right at opposite ends of a line of the given
// width. If they don't both fit, left is cut short.
func spaceBetween(left, right string, width int) string {
	room := width - lipgloss.Width(right) - 1
	left = ansi.Truncate(left, max(room, 0), "…")
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	return left + strings.Repeat(" ", max(gap, 0)) + right
}
