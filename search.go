package main

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// updateSearch handles key presses while the search box is focused.
func (m model) updateSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc": // stop searching and clear the search text
		m.mode = browsing
		m.search.Blur()
		m.search.Reset()
		m.refresh()
		return m, nil
	case "enter": // stop searching but keep the search text
		m.mode = browsing
		m.search.Blur()
		return m, nil
	case "up", "down", "ctrl+c": // still allow moving and quitting while typing
		return m.updateBrowse(msg)
	}

	// Everything else is typing: let the text input handle it.
	before := m.search.Value()
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	if m.search.Value() != before {
		m.refresh()
	}
	return m, cmd
}

// searchQuery is the search text, lowercased for case-insensitive matching.
func (m model) searchQuery() string {
	return strings.ToLower(m.search.Value())
}

// matches reports whether every word in query appears in one of the movie's
// fields, so "animation estonian" finds Estonian-dubbed cartoons.
// query must already be lowercase.
func (mv Movie) matches(query string) bool {
	fields := []string{mv.Title, mv.Studio, strconv.Itoa(mv.Year)}
	fields = append(fields, mv.Genres...)
	fields = append(fields, mv.Languages...)

	for word := range strings.FieldsSeq(query) {
		found := slices.ContainsFunc(fields, func(f string) bool {
			return strings.Contains(strings.ToLower(f), word)
		})
		if !found {
			return false
		}
	}
	return true
}
