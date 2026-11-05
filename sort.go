package main

import (
	"cmp"
	"strings"
)

// sortKey is which field the list is sorted by.
type sortKey int

const (
	byTitle sortKey = iota
	byYear
	byDuration
	byStudio
	numSortKeys // not a real key: the number of keys, used to cycle through them
)

func (k sortKey) String() string {
	return [...]string{"title", "year", "duration", "studio"}[k]
}

// compareMovies orders two movies by key, then by title to break ties.
// It returns a negative number if a comes first, positive if b does, 0 if equal.
func compareMovies(a, b Movie, key sortKey) int {
	var c int
	switch key {
	case byYear:
		c = cmp.Compare(a.Year, b.Year)
	case byDuration:
		c = cmp.Compare(a.Duration, b.Duration)
	case byStudio:
		c = cmp.Compare(strings.ToLower(a.Studio), strings.ToLower(b.Studio))
	}
	if c == 0 {
		c = cmp.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	}
	return c
}
