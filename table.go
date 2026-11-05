package main

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// noSort marks a column you can't sort by.
const noSort sortKey = -1

// columnGap is the space between two columns.
const columnGap = 2

// column describes one column of the movie table.
type column struct {
	title string
	sort  sortKey // the sort key this column shows, or noSort
	width int     // fixed width, or the minimum width of a flexible column
	flex  int     // share of the leftover width; 0 means the width is fixed
	right bool    // right-align, for numbers
	value func(Movie) string
}

var columns = []column{
	{title: "Title", sort: byTitle, width: 10, flex: 5, value: func(mv Movie) string { return mv.Title }},
	{title: "Year", sort: byYear, width: 4, value: func(mv Movie) string { return strconv.Itoa(mv.Year) }},
	{title: "Genre", sort: noSort, width: 8, flex: 4, value: func(mv Movie) string { return strings.Join(mv.Genres, ", ") }},
	{title: "Studio", sort: byStudio, width: 8, flex: 3, value: func(mv Movie) string { return mv.Studio }},
	{title: "Language", sort: noSort, width: 8, flex: 2, value: func(mv Movie) string { return strings.Join(mv.Languages, ", ") }},
	{title: "Duration", sort: byDuration, width: 5, right: true, value: func(mv Movie) string { return formatDuration(mv.Duration) }},
}

// columnWidths decides how wide each column is for a table of totalWidth.
// Fixed columns get their width; flexible ones share whatever is left.
func columnWidths(totalWidth, numberWidth int) []int {
	widths := make([]int, len(columns))
	used := numberWidth + columnGap*len(columns)
	totalFlex := 0

	for i, c := range columns {
		// Leave room for the title plus a sort arrow (" ↑").
		widths[i] = max(c.width, lipgloss.Width(c.title)+2)
		used += widths[i]
		totalFlex += c.flex
	}

	leftover := totalWidth - used
	if leftover <= 0 || totalFlex == 0 {
		return widths // too narrow: everything stays at its minimum
	}

	given := 0
	for i, c := range columns {
		extra := leftover * c.flex / totalFlex
		widths[i] += extra
		given += extra
	}
	widths[0] += leftover - given // rounding leftovers go to the title
	return widths
}

// tableView renders the column headers and the visible rows of the movie table.
func (m model) tableView(width int) string {
	switch {
	case len(m.movies) == 0 && m.syncing:
		return "Loading movies…"
	case len(m.movies) == 0:
		return "No movies yet. Press R to try again."
	case len(m.visible) == 0:
		return "No matches."
	}

	numberWidth := len(strconv.Itoa(len(m.visible)))
	widths := columnWidths(width, numberWidth)

	// Header row, with an arrow on the column we're sorting by.
	headers := make([]string, len(columns))
	for i, c := range columns {
		headers[i] = c.title
		if c.sort == m.sortBy {
			headers[i] += " " + m.sortArrow()
		}
	}
	lines := []string{columnHeaderStyle.Render(formatRow("#", headers, numberWidth, widths, width))}

	end := min(m.offset+m.visibleRows(), len(m.visible))
	for i := m.offset; i < end; i++ {
		movie := m.movies[m.visible[i]]
		cells := make([]string, len(columns))
		for j, c := range columns {
			cells[j] = c.value(movie)
		}

		row := formatRow(strconv.Itoa(i+1), cells, numberWidth, widths, width)
		if i == m.cursor {
			row = selectedStyle.Render(row)
		}
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

// formatRow lays out a row number and cells in their columns, padded to
// exactly width so a highlighted row fills the whole line.
func formatRow(number string, cells []string, numberWidth int, widths []int, width int) string {
	var b strings.Builder
	b.WriteString(pad(number, numberWidth, true))
	for i, cell := range cells {
		b.WriteString(strings.Repeat(" ", columnGap))
		b.WriteString(pad(cell, widths[i], columns[i].right))
	}
	return pad(b.String(), width, false)
}

// pad fits s into exactly width columns: cut with "…" if too long, filled
// with spaces if too short.
func pad(s string, width int, alignRight bool) string {
	s = ansi.Truncate(s, width, "…")
	fill := strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
	if alignRight {
		return fill + s
	}
	return s + fill
}
