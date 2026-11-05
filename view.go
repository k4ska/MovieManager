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

	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent).Padding(0, 1)
	countStyle    = lipgloss.NewStyle().Foreground(subtle).Padding(0, 1)
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(subtle).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Background(accent).Foreground(lipgloss.Color("0")).Bold(true)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(accent)
	labelStyle    = lipgloss.NewStyle().Foreground(subtle).Width(10)
	helpStyle     = lipgloss.NewStyle().Foreground(subtle).Padding(0, 1)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Padding(0, 1) // red
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
	arrow := "↑"
	if m.sortReverse {
		arrow = "↓"
	}
	status := count + " · sort: " + m.sortBy.String() + " " + arrow
	if summary := m.activeFilterSummary(); summary != "" {
		status += " · " + summary
	}
	left := headerStyle.Render("MovieManager") + countStyle.Render(status)
	if m.mode == searching || m.search.Value() != "" {
		left += " " + m.search.View()
	}
	header := spaceBetween(left, m.syncStatus(), m.width)

	panelHeight := m.height - chromeHeight
	listWidth := max(m.width*2/5, 30)
	detailWidth := m.width - listWidth

	innerHeight := panelHeight - panelStyle.GetVerticalFrameSize()
	list := panelStyle.Width(listWidth).Height(panelHeight).Render(m.listView(listWidth - panelStyle.GetHorizontalFrameSize()))
	right := m.detailView()
	if m.mode == filtering {
		right = m.filterView(innerHeight)
	}
	right = fitBlock(right, detailWidth-panelStyle.GetHorizontalFrameSize(), innerHeight)
	details := panelStyle.Width(detailWidth).Height(panelHeight).Render(right)

	help := helpStyle.Render(m.helpText())
	if m.flash != nil {
		help = errorStyle.Render("⚠ " + m.flash.Error())
	}
	help = ansi.Truncate(help, m.width, "…")

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left,
		header,
		lipgloss.JoinHorizontal(lipgloss.Top, list, details),
		help,
	))
	v.AltScreen = true
	return v
}

// listView renders the visible slice of the movie list, one movie per line.
func (m model) listView(width int) string {
	switch {
	case len(m.movies) == 0 && m.syncing:
		return "Loading movies…"
	case len(m.movies) == 0:
		return "No movies yet. Press R to try again."
	case len(m.visible) == 0:
		return "No matches."
	}

	var rows []string
	end := min(m.offset+m.visibleRows(), len(m.visible))

	for i := m.offset; i < end; i++ {
		movie := m.movies[m.visible[i]]
		// Title on the left, year (or duration, when sorting by it) on the right.
		extra := fmt.Sprintf("%d", movie.Year)
		if m.sortBy == byDuration {
			extra = formatDuration(movie.Duration)
		}
		title := ansi.Truncate(movie.Title, width-lipgloss.Width(extra)-1, "…")
		gap := width - lipgloss.Width(title) - lipgloss.Width(extra)
		row := title + strings.Repeat(" ", max(gap, 1)) + extra

		if i == m.cursor {
			row = selectedStyle.Render(row)
		}
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}

// detailView renders everything we know about the selected movie.
func (m model) detailView() string {
	movie, ok := m.selected()
	if !ok {
		return ""
	}

	field := func(label, value string) string {
		return labelStyle.Render(label) + value
	}

	return strings.Join([]string{
		titleStyle.Render(movie.Title),
		"",
		field("Year", fmt.Sprintf("%d", movie.Year)),
		field("Genre", strings.Join(movie.Genres, ", ")),
		field("Studio", movie.Studio),
		field("Language", strings.Join(movie.Languages, ", ")),
		field("Duration", formatDuration(movie.Duration)),
	}, "\n")
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
	}
	help := "j/k move · / search · f filter · s sort · S reverse · R refresh · q quit"
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
