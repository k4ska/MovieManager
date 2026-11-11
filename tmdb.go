package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
)

// TMDB (themoviedb.org) knows about the film, not your copy of it: "studio"
// comes from the production companies and "language" is the film's original
// language, so check both against the DVD before saving.
//
// Each movie gets one genre (from the list in genres.go), one studio and one
// language: the main one.

const tmdbAPI = "https://api.themoviedb.org/3"

// tmdbResult is one search hit, enough to tell similar titles apart.
type tmdbResult struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"` // "2001-07-20", or "" if unknown
}

// year is the release year, or 0 if TMDB doesn't know it.
func (r tmdbResult) year() int {
	if len(r.ReleaseDate) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(r.ReleaseDate[:4])
	return y
}

// tmdbGet fetches a TMDB API path and decodes the JSON reply into out.
func tmdbGet(path string, query url.Values, out any) error {
	token, err := tmdbToken()
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", tmdbAPI+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("TMDB: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Message string `json:"status_message"`
		}
		json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Message == "" {
			apiErr.Message = resp.Status
		}
		return fmt.Errorf("TMDB: %s", apiErr.Message)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// searchTMDB looks up films by title, most relevant first.
func searchTMDB(title string) ([]tmdbResult, error) {
	var reply struct {
		Results []tmdbResult `json:"results"`
	}
	err := tmdbGet("/search/movie", url.Values{"query": {title}}, &reply)
	return reply.Results, err
}

// tmdbMovie fetches the details of one film and turns them into a Movie
// (without an ID: the sheet hands that out when the movie is added).
func tmdbMovie(id int) (Movie, error) {
	type named struct {
		Name        string `json:"name"`
		EnglishName string `json:"english_name"` // only languages have these two
		Code        string `json:"iso_639_1"`
	}
	var d struct {
		tmdbResult
		Runtime          int     `json:"runtime"`
		Genres           []named `json:"genres"`
		Companies        []named `json:"production_companies"`
		Languages        []named `json:"spoken_languages"`
		OriginalLanguage string  `json:"original_language"` // a code like "ja"
	}
	if err := tmdbGet("/movie/"+strconv.Itoa(id), url.Values{}, &d); err != nil {
		return Movie{}, err
	}

	mv := Movie{Title: d.Title, Year: d.year(), Duration: d.Runtime}
	var tmdbGenres []string
	for _, g := range d.Genres {
		tmdbGenres = append(tmdbGenres, g.Name)
	}
	if g := pickGenre(tmdbGenres, d.OriginalLanguage); g != "" {
		mv.Genres = []string{g}
	}
	// TMDB lists the main studio first.
	if len(d.Companies) > 0 {
		mv.Studio = d.Companies[0].Name
	}
	// The original language is only given as a code, so look up its name among
	// the spoken languages. If it isn't there, use the first spoken language.
	if len(d.Languages) > 0 {
		lang := d.Languages[0]
		if i := slices.IndexFunc(d.Languages, func(l named) bool { return l.Code == d.OriginalLanguage }); i >= 0 {
			lang = d.Languages[i]
		}
		mv.Languages = []string{lang.EnglishName}
	}
	return mv, nil
}
