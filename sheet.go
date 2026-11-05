package main

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const sheetURL = "https://docs.google.com/spreadsheets/d/1HAICfDGCxaM9qrBxQwcZJKLi4AOIqPXcV_yqybJI2kM/export?format=csv&gid=0"

type Movie struct {
	ID        int
	Title     string
	Year      int
	Genres    []string
	Studio    string
	Languages []string
	Duration  int // in minutes; 0 if unknown
}

// fetchMovies downloads the sheet as CSV and turns each row into a Movie.
func fetchMovies(url string) ([]Movie, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching sheet: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching sheet: unexpected status %s", resp.Status)
	}

	rows, err := csv.NewReader(resp.Body).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("sheet is empty")
	}

	// Map column names to their index, so the sheet's column order doesn't matter.
	col := map[string]int{}
	for i, name := range rows[0] {
		col[strings.TrimSpace(name)] = i
	}
	for _, name := range []string{"id", "title", "year", "genre", "studio", "language", "duration"} {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("missing column %q", name)
		}
	}

	var movies []Movie
	for i, row := range rows[1:] {
		line := i + 2 // row number as shown in the sheet (header is row 1)

		idCell := strings.TrimSpace(row[col["id"]])
		if idCell == "" {
			continue // skip blank rows
		}
		id, err := strconv.Atoi(idCell)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid id %q", line, idCell)
		}

		yearCell := strings.TrimSpace(row[col["year"]])
		year, err := strconv.Atoi(yearCell)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid year %q", line, yearCell)
		}

		// Duration is optional: an empty cell means we don't know it yet.
		var duration int
		if durationCell := strings.TrimSpace(row[col["duration"]]); durationCell != "" {
			duration, err = strconv.Atoi(durationCell)
			if err != nil {
				return nil, fmt.Errorf("row %d: invalid duration %q (use minutes, e.g. 124)", line, durationCell)
			}
		}

		movies = append(movies, Movie{
			ID:        id,
			Title:     strings.TrimSpace(row[col["title"]]),
			Year:      year,
			Genres:    splitList(row[col["genre"]]),
			Studio:    strings.TrimSpace(row[col["studio"]]),
			Languages: splitList(row[col["language"]]),
			Duration:  duration,
		})
	}

	return movies, nil
}

// splitList turns "Animation, Sci-Fi" into ["Animation", "Sci-Fi"].
func splitList(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
