package main

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const sheetID = 0 // the "gid" of the tab with the movies: 0 is the first tab

// spreadsheetID is the long ID from the sheet's URL. main sets it at startup
// from the config (see sheetSetting), before anything talks to the sheet.
var spreadsheetID string

// sheetURL is the public CSV export of the movie tab.
func sheetURL() string {
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv&gid=%d", spreadsheetID, sheetID)
}

type Movie struct {
	ID        int      `json:"id"`
	Title     string   `json:"title"`
	Year      int      `json:"year"`
	Genres    []string `json:"genres"`
	Studio    string   `json:"studio"`
	Languages []string `json:"languages"`
	Duration  int      `json:"duration"` // in minutes; 0 if unknown
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
	if err := checkColumns(col); err != nil {
		return nil, err
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

// sheetColumns are the header names the sheet must have, in any order.
var sheetColumns = []string{"id", "title", "year", "genre", "studio", "language", "duration"}

// checkColumns reports the first of sheetColumns missing from col.
func checkColumns(col map[string]int) error {
	for _, name := range sheetColumns {
		if _, ok := col[name]; !ok {
			return fmt.Errorf("missing column %q", name)
		}
	}
	return nil
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
