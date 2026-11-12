package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/oauth2/google"
)

// Reading uses the public CSV export (see sheet.go), but changing the sheet
// needs the Sheets API, signed in as a Google service account that the sheet
// is shared with (as an Editor).

const sheetsAPI = "https://sheets.googleapis.com/v4/spreadsheets/"

// sheetsClient talks to the Sheets API as the service account.
type sheetsClient struct {
	http *http.Client
}

func newSheetsClient(ctx context.Context) (*sheetsClient, error) {
	key, err := readConfigFile("service-account.json", "Google service account key")
	if err != nil {
		return nil, err
	}
	conf, err := google.JWTConfigFromJSON(key, "https://www.googleapis.com/auth/spreadsheets")
	if err != nil {
		return nil, fmt.Errorf("reading service account key: %w", err)
	}
	return &sheetsClient{conf.Client(ctx)}, nil
}

// call sends a request to the Sheets API and decodes the JSON reply into out
// (unless out is nil).
func (c *sheetsClient) call(method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, sheetsAPI+spreadsheetID+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sheets API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Google explains what went wrong in {"error": {"message": "..."}}.
		var apiErr struct {
			Error struct{ Message string } `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Error.Message == "" {
			apiErr.Error.Message = resp.Status
		}
		return fmt.Errorf("sheets API: %s", apiErr.Error.Message)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// tabName is the name of the movie tab, quoted for A1 ranges, e.g. 'DVDs'.
// The values API wants tabs by name, so we look up the name of sheetID.
func (c *sheetsClient) tabName() (string, error) {
	var info struct {
		Sheets []struct {
			Properties struct {
				SheetID int    `json:"sheetId"`
				Title   string `json:"title"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	if err := c.call("GET", "?fields=sheets.properties(sheetId,title)", nil, &info); err != nil {
		return "", err
	}
	for _, s := range info.Sheets {
		if s.Properties.SheetID == sheetID {
			// Quote the name in case it has spaces; a ' inside is written ''.
			return "'" + strings.ReplaceAll(s.Properties.Title, "'", "''") + "'", nil
		}
	}
	return "", fmt.Errorf("sheet has no tab with gid %d", sheetID)
}

// sheetRows is the current contents of the movie tab, straight from the API.
type sheetRows struct {
	tab  string         // quoted name of the tab, from tabName
	rows [][]string     // rows[0] is the header; rows[i] is sheet row i+1
	col  map[string]int // column name -> index, like in fetchMovies
}

func (c *sheetsClient) readRows() (sheetRows, error) {
	tab, err := c.tabName()
	if err != nil {
		return sheetRows{}, err
	}
	var reply struct {
		Values [][]string `json:"values"`
	}
	// Starting at A1 makes rows[i] line up with sheet row i+1, blank rows included.
	if err := c.call("GET", "/values/"+url.PathEscape(tab+"!A1:ZZ"), nil, &reply); err != nil {
		return sheetRows{}, err
	}
	if len(reply.Values) == 0 {
		return sheetRows{}, fmt.Errorf("sheet is empty")
	}

	col := map[string]int{}
	for i, name := range reply.Values[0] {
		col[strings.TrimSpace(name)] = i
	}
	if err := checkColumns(col); err != nil {
		return sheetRows{}, err
	}
	return sheetRows{tab, reply.Values, col}, nil
}

// cell returns a trimmed cell, or "" if the row is shorter than that (the API
// leaves out empty cells at the end of a row).
func (s sheetRows) cell(row int, name string) string {
	r, i := s.rows[row], s.col[name]
	if i >= len(r) {
		return ""
	}
	return strings.TrimSpace(r[i])
}

// addMovie appends mv as a new row at the bottom of the sheet, giving it the
// next free ID. It returns the movie with that ID filled in.
func (c *sheetsClient) addMovie(mv Movie) (Movie, error) {
	s, err := c.readRows()
	if err != nil {
		return mv, err
	}

	// Pick the ID from the sheet itself, not our cached copy, in case someone
	// added a row by hand since the last sync.
	mv.ID = 1
	for i := 1; i < len(s.rows); i++ {
		if id, err := strconv.Atoi(s.cell(i, "id")); err == nil {
			mv.ID = max(mv.ID, id+1)
		}
	}

	// Build the row in the sheet's column order; other columns are left empty.
	row := make([]any, len(s.rows[0]))
	for i := range row {
		row[i] = ""
	}
	row[s.col["id"]] = mv.ID
	for name, value := range cellValues(mv) {
		row[s.col[name]] = value
	}

	// RAW stores exactly what we send, so a title starting with "=" stays text.
	path := "/values/" + url.PathEscape(s.tab) + ":append?valueInputOption=RAW&insertDataOption=INSERT_ROWS"
	body := map[string]any{"values": [][]any{row}}
	return mv, c.call("POST", path, body, nil)
}

// cellValues are the cells of mv by column name, except the id. Numbers are
// sent as numbers so the sheet stores them as numbers.
func cellValues(mv Movie) map[string]any {
	values := map[string]any{
		"title":    mv.Title,
		"year":     mv.Year,
		"genre":    strings.Join(mv.Genres, ", "),
		"studio":   mv.Studio,
		"language": strings.Join(mv.Languages, ", "),
		"duration": "",
	}
	if mv.Duration > 0 {
		values["duration"] = mv.Duration
	}
	return values
}

// findRow returns the index in s.rows of mv's row. It checks the title too,
// so we never change the wrong row if the sheet changed since the last sync.
func (s sheetRows) findRow(mv Movie) (int, error) {
	want := strconv.Itoa(mv.ID)
	for i := 1; i < len(s.rows); i++ {
		if s.cell(i, "id") != want {
			continue
		}
		if title := s.cell(i, "title"); title != mv.Title {
			return 0, fmt.Errorf("row %d has id %d but title %q, not %q: press R to refresh", i+1, mv.ID, title, mv.Title)
		}
		return i, nil
	}
	return 0, fmt.Errorf("no row with id %d in the sheet: press R to refresh", mv.ID)
}

// updateMovie overwrites the row of old with the values of mv (same ID). Only
// the movie's own cells are written, so any extra columns stay as they are.
func (c *sheetsClient) updateMovie(old, mv Movie) error {
	s, err := c.readRows()
	if err != nil {
		return err
	}
	i, err := s.findRow(old)
	if err != nil {
		return err
	}

	var data []any
	for name, value := range cellValues(mv) {
		cell := fmt.Sprintf("%s!%s%d", s.tab, columnLetter(s.col[name]), i+1) // e.g. 'DVDs'!C5
		data = append(data, map[string]any{"range": cell, "values": [][]any{{value}}})
	}
	body := map[string]any{"valueInputOption": "RAW", "data": data}
	return c.call("POST", "/values:batchUpdate", body, nil)
}

// columnLetter turns a column index into its letters: 0 is A, 26 is AA.
func columnLetter(i int) string {
	letters := ""
	for i++; i > 0; i = (i - 1) / 26 {
		letters = string(rune('A'+(i-1)%26)) + letters
	}
	return letters
}

// deleteMovie removes the row of mv from the sheet.
func (c *sheetsClient) deleteMovie(mv Movie) error {
	s, err := c.readRows()
	if err != nil {
		return err
	}
	i, err := s.findRow(mv)
	if err != nil {
		return err
	}
	// Row indexes in the API start at 0, and rows[0] is sheet row 1.
	body := map[string]any{"requests": []any{map[string]any{
		"deleteDimension": map[string]any{"range": map[string]any{
			"sheetId": sheetID, "dimension": "ROWS", "startIndex": i, "endIndex": i + 1,
		}},
	}}}
	return c.call("POST", ":batchUpdate", body, nil)
}
