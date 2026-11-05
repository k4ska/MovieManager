package main

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// filterOption is one checkbox in the filter picker.
type filterOption struct {
	kind  string // "Genre" or "Language"
	value string // e.g. "Animation"
}

// buildFilterOptions collects every genre and language used in the collection,
// sorted alphabetically, genres first.
func buildFilterOptions(movies []Movie) []filterOption {
	var genres, languages []string
	for _, movie := range movies {
		genres = append(genres, movie.Genres...)
		languages = append(languages, movie.Languages...)
	}

	var options []filterOption
	for _, g := range unique(genres) {
		options = append(options, filterOption{"Genre", g})
	}
	for _, l := range unique(languages) {
		options = append(options, filterOption{"Language", l})
	}
	return options
}

// unique returns the sorted distinct values of list.
func unique(list []string) []string {
	slices.Sort(list)
	return slices.Compact(list)
}

// updateFilter handles key presses while the filter picker is open.
func (m model) updateFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "enter", "f", "q":
		m.mode = browsing
	case "j", "down":
		m.filterCursor = min(m.filterCursor+1, len(m.filterOptions)-1)
	case "k", "up":
		m.filterCursor = max(m.filterCursor-1, 0)
	case "space", "x":
		if len(m.filterOptions) > 0 {
			opt := m.filterOptions[m.filterCursor]
			m.activeFilters[opt] = !m.activeFilters[opt]
			m.refresh()
		}
	case "c":
		clear(m.activeFilters)
		m.refresh()
	}
	return m, nil
}

// matchesFilters reports whether movie passes the checked filters. Within one
// kind any checked value is enough (Animation OR Comedy); across kinds all
// must match (Animation AND Estonian).
func (m model) matchesFilters(movie Movie) bool {
	return matchesKind(m.activeFilters, "Genre", movie.Genres) &&
		matchesKind(m.activeFilters, "Language", movie.Languages)
}

func matchesKind(active map[filterOption]bool, kind string, values []string) bool {
	anyChecked := false
	for opt, on := range active {
		if !on || opt.kind != kind {
			continue
		}
		anyChecked = true
		if slices.Contains(values, opt.value) {
			return true
		}
	}
	return !anyChecked // nothing checked for this kind means no restriction
}

// activeFilterSummary describes the checked filters, e.g. "Animation, Estonian".
func (m model) activeFilterSummary() string {
	var names []string
	for _, opt := range m.filterOptions { // filterOptions is sorted, so the summary is too
		if m.activeFilters[opt] {
			names = append(names, opt.value)
		}
	}
	return strings.Join(names, ", ")
}

// filterView renders the filter picker shown in the right-hand panel.
func (m model) filterView(height int) string {
	lines := []string{titleStyle.Render("Filter"), ""}
	cursorLine := 0
	lastKind := ""

	for i, opt := range m.filterOptions {
		if opt.kind != lastKind {
			if lastKind != "" {
				lines = append(lines, "")
			}
			lines = append(lines, labelStyle.Render(opt.kind))
			lastKind = opt.kind
		}

		box := "[ ] "
		if m.activeFilters[opt] {
			box = "[x] "
		}
		line := box + opt.value
		if i == m.filterCursor {
			line = selectedStyle.Render(line)
			cursorLine = len(lines)
		}
		lines = append(lines, line)
	}

	// Scroll so the cursor stays visible in small terminals.
	start := max(cursorLine-height+1, 0)
	end := min(start+height, len(lines))
	return strings.Join(lines[start:end], "\n")
}
