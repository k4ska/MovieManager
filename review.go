package main

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Reviewing goes through movies one by one, looks each up on TMDB and shows
// the sheet's values next to TMDB's, so you can make the collection
// consistent. It reuses the add screens (addingSearch, addingPick,
// addingForm); m.review.active tells them they're part of a review.
//
//	u  review the selected movie
//	U  review every movie in the list (so search and filters pick which)

// review is the state of a review in progress.
type review struct {
	active bool
	queue  []int // IDs of the movies to review, in list order
	pos    int   // index in queue of the movie being reviewed
	sheet  Movie // that movie as it is in the sheet

	updated, unchanged, skipped int
}

// movieUpdatedMsg is sent back to Update when a change to a row is saved.
type movieUpdatedMsg struct {
	movie Movie
	err   error
}

func updateMovieCmd(old, mv Movie) tea.Cmd {
	return func() tea.Msg {
		c, err := newSheetsClient(context.Background())
		if err != nil {
			return movieUpdatedMsg{mv, err}
		}
		return movieUpdatedMsg{mv, c.updateMovie(old, mv)}
	}
}

// startReview reviews the movies with these IDs, in order.
func (m model) startReview(ids []int) (tea.Model, tea.Cmd) {
	if m.saving || len(ids) == 0 {
		return m, nil
	}
	m.review = review{active: true, queue: ids}
	return m.reviewCurrent()
}

// reviewCurrent looks up the movie at m.review.pos on TMDB, or finishes the
// review if there are no movies left.
func (m model) reviewCurrent() (tea.Model, tea.Cmd) {
	for ; m.review.pos < len(m.review.queue); m.review.pos++ {
		id := m.review.queue[m.review.pos]
		i := slices.IndexFunc(m.movies, func(mv Movie) bool { return mv.ID == id })
		if i < 0 {
			continue // deleted since the review started
		}
		m.review.sheet = m.movies[i]
		m.selectID(id, true) // so the table shows where you are when the review ends

		m.mode = addingSearch
		m.results = nil
		m.addTitle.SetValue(m.review.sheet.Title)
		m.loading = "Searching TMDB…"
		return m, tea.Batch(m.addTitle.Focus(), searchTMDBCmd(m.review.sheet.Title))
	}
	return m.endReview("Review done")
}

// nextReview moves on to the next movie.
func (m model) nextReview() (tea.Model, tea.Cmd) {
	m.review.pos++
	return m.reviewCurrent()
}

// skipReview leaves the current movie as it is and moves on.
func (m model) skipReview() (tea.Model, tea.Cmd) {
	if m.saving {
		return m, nil
	}
	m.review.skipped++
	return m.nextReview()
}

// endReview goes back to the list and sums up what happened.
func (m model) endReview(how string) (tea.Model, tea.Cmd) {
	r := m.review
	m.review = review{}
	m.mode = browsing
	m.loading = ""
	m.addTitle.Blur()

	summary := fmt.Sprintf("%s: %d updated, %d already right, %d skipped", how, r.updated, r.unchanged, r.skipped)
	cmds := []tea.Cmd{m.showNotice(summary)}
	if r.updated > 0 {
		cmds = append(cmds, m.resync())
	}
	return m, tea.Batch(cmds...)
}

// reviewResults handles TMDB's search results for the movie being reviewed.
// If exactly the right year is among them, that one is used straight away.
func (m model) reviewResults(msg tmdbSearchMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.flash = msg.err
		return m, nil
	case len(msg.results) == 0:
		m.flash = fmt.Errorf("no TMDB matches for %q: edit the title and press enter, or tab to fill in by hand", m.addTitle.Value())
		return m, nil
	}

	m.mode = addingPick
	m.addTitle.Blur()
	m.results = msg.results
	m.resultCursor = 0

	sameYear := slices.IndexFunc(m.results, func(r tmdbResult) bool { return r.year() == m.review.sheet.Year })
	if sameYear >= 0 {
		m.resultCursor = sameYear
		m.loading = "Fetching details…"
		return m, tmdbMovieCmd(m.results[sameYear].ID)
	}
	return m, nil
}

// openReviewForm shows the sheet's and TMDB's values side by side, with the
// input filled in from whichever is the better default for that field.
// tmdb is empty when filling in by hand.
func (m model) openReviewForm(tmdb Movie) (tea.Model, tea.Cmd) {
	next, cmd := m.openForm(Movie{}, false)
	m = next.(model)

	sheet := m.review.sheet
	sheetValues, tmdbValues := formValues(sheet), formValues(tmdb)
	for i := range m.form {
		f := &m.form[i]
		f.sheet, f.tmdb = sheetValues[i], tmdbValues[i]

		// The title and language are your disc's (e.g. an Estonian title and
		// dub), so they stay; for the rest TMDB is the better source.
		value := cmp.Or(f.tmdb, f.sheet)
		if i == fieldTitle || i == fieldLanguage {
			value = cmp.Or(f.sheet, f.tmdb)
		}
		if i == fieldGenre && f.tmdb == "" {
			value = "" // TMDB had none of our genres: use the sheet's, if it's one of them
			for _, g := range sheet.Genres {
				if genre, ok := normalizeGenre(g); ok {
					value = genre
					break
				}
			}
		}
		f.input.SetValue(value)
	}
	return m, cmd
}

// swapFormValue switches the focused input between the sheet's and TMDB's value.
func (m model) swapFormValue() (tea.Model, tea.Cmd) {
	f := &m.form[m.formCursor]
	if f.input.Value() == f.sheet {
		f.input.SetValue(f.tmdb)
	} else {
		f.input.SetValue(f.sheet)
	}
	f.input.CursorEnd()
	return m, nil
}

// saveReview saves the form over the movie being reviewed, or just moves on
// if nothing changed.
func (m model) saveReview() (tea.Model, tea.Cmd) {
	if m.saving {
		return m, nil
	}
	mv, err := formMovie(m.form)
	if err != nil {
		m.flash = err
		return m, nil
	}
	old := m.review.sheet
	mv.ID = old.ID
	if sameMovie(mv, old) {
		m.review.unchanged++
		return m.nextReview()
	}
	m.saving = true
	return m, updateMovieCmd(old, mv)
}

// movieUpdated handles a saved change: it updates our copy of the movie and
// moves on to the next one.
func (m model) movieUpdated(msg movieUpdatedMsg) (tea.Model, tea.Cmd) {
	m.saving = false
	if msg.err != nil {
		m.flash = msg.err
		return m, nil
	}
	movies := slices.Clone(m.movies)
	if i := slices.IndexFunc(movies, func(mv Movie) bool { return mv.ID == msg.movie.ID }); i >= 0 {
		movies[i] = msg.movie
	}
	m.setMovies(movies)

	if !m.review.active {
		return m, nil // the review was stopped while saving
	}
	m.review.updated++
	return m.nextReview()
}

// sameMovie reports whether a and b have the same values in every column.
func sameMovie(a, b Movie) bool {
	return a.Title == b.Title && a.Year == b.Year && a.Studio == b.Studio && a.Duration == b.Duration &&
		slices.Equal(a.Genres, b.Genres) && slices.Equal(a.Languages, b.Languages)
}

// reviewHeading is the title of the add screens during a review, e.g.
// "Review from TMDB · 3/50 · Brave".
func (m model) reviewHeading() string {
	return fmt.Sprintf("Review from TMDB · %d/%d · %s", m.review.pos+1, len(m.review.queue), m.review.sheet.Title)
}

// reviewHint is what's shown next to a form input during a review.
func reviewHint(f formField) string {
	orDash := func(s string) string { return cmp.Or(strings.TrimSpace(s), "—") }
	return labelStyle.Render("sheet: " + orDash(f.sheet) + "  ·  TMDB: " + orDash(f.tmdb))
}
