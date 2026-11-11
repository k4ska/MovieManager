package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Adding a movie goes through up to three screens:
//
//	addingSearch  type a title          (enter: search TMDB, tab: skip TMDB)
//	addingPick    pick the right film   (enter: fetch its details)
//	addingForm    check and edit fields (ctrl+s: save to the sheet)
//
// Deleting asks for confirmation in the help bar (confirmingDelete).

// formField is one row of the add form.
type formField struct {
	label string
	input textinput.Model

	// When reviewing, the field's value in the sheet and on TMDB, shown next
	// to the input. ctrl+o switches the input between them.
	sheet, tmdb string
}

var formLabels = []string{"Title", "Year", "Genre", "Studio", "Language", "Duration"}

// Indexes into the form, in the same order as formLabels.
const (
	fieldTitle = iota
	fieldYear
	fieldGenre
	fieldStudio
	fieldLanguage
	fieldDuration
)

// formValues are mv's fields as text, in the order of formLabels. Unknown
// numbers are "".
func formValues(mv Movie) []string {
	values := []string{
		mv.Title,
		"",
		strings.Join(mv.Genres, ", "),
		mv.Studio,
		strings.Join(mv.Languages, ", "),
		"",
	}
	if mv.Year > 0 {
		values[fieldYear] = strconv.Itoa(mv.Year)
	}
	if mv.Duration > 0 {
		values[fieldDuration] = strconv.Itoa(mv.Duration)
	}
	return values
}

func newForm(mv Movie) []formField {
	values := formValues(mv)
	placeholders := []string{"", "e.g. 2001", strings.Join(genres, ", "), "", "e.g. English", "minutes, e.g. 124"}

	form := make([]formField, len(formLabels))
	for i, label := range formLabels {
		in := textinput.New()
		in.Prompt = ""
		in.Placeholder = placeholders[i]
		in.SetValue(values[i])
		form[i] = formField{label: label, input: in}
	}

	// The genre box completes what you type from the genre list: → accepts
	// (tab and ↑/↓ already move between fields), ctrl+n/ctrl+p cycle.
	g := &form[fieldGenre].input
	g.ShowSuggestions = true
	g.SetSuggestions(genres)
	g.KeyMap.AcceptSuggestion.SetKeys("right")
	g.KeyMap.NextSuggestion.SetKeys("ctrl+n")
	g.KeyMap.PrevSuggestion.SetKeys("ctrl+p")
	return form
}

// formMovie checks the form and turns it into a Movie.
func formMovie(form []formField) (Movie, error) {
	value := func(i int) string { return strings.TrimSpace(form[i].input.Value()) }

	mv := Movie{
		Title:     value(fieldTitle),
		Studio:    value(fieldStudio),
		Languages: splitList(value(fieldLanguage)),
	}
	if mv.Title == "" {
		return mv, errors.New("title is required")
	}
	genre, ok := normalizeGenre(value(fieldGenre))
	if !ok {
		return mv, errors.New("genre must be one of: " + strings.Join(genres, ", "))
	}
	mv.Genres = []string{genre}
	year, err := strconv.Atoi(value(fieldYear))
	if err != nil {
		return mv, errors.New("year must be a number, e.g. 2001")
	}
	mv.Year = year
	if d := value(fieldDuration); d != "" {
		if mv.Duration, err = strconv.Atoi(d); err != nil {
			return mv, errors.New("duration must be in minutes, e.g. 124")
		}
	}
	return mv, nil
}

// Messages sent back to Update when background work finishes.
type (
	tmdbSearchMsg struct {
		results []tmdbResult
		err     error
	}
	tmdbMovieMsg struct {
		movie Movie
		err   error
	}
	movieAddedMsg struct {
		movie Movie
		err   error
	}
	movieDeletedMsg struct {
		movie Movie
		err   error
	}
)

func searchTMDBCmd(title string) tea.Cmd {
	return func() tea.Msg {
		results, err := searchTMDB(title)
		return tmdbSearchMsg{results, err}
	}
}

func tmdbMovieCmd(id int) tea.Cmd {
	return func() tea.Msg {
		mv, err := tmdbMovie(id)
		return tmdbMovieMsg{mv, err}
	}
}

func addMovieCmd(mv Movie) tea.Cmd {
	return func() tea.Msg {
		c, err := newSheetsClient(context.Background())
		if err != nil {
			return movieAddedMsg{mv, err}
		}
		mv, err = c.addMovie(mv)
		return movieAddedMsg{mv, err}
	}
}

func deleteMovieCmd(mv Movie) tea.Cmd {
	return func() tea.Msg {
		c, err := newSheetsClient(context.Background())
		if err != nil {
			return movieDeletedMsg{mv, err}
		}
		return movieDeletedMsg{mv, c.deleteMovie(mv)}
	}
}

// isAdding reports whether one of the add screens is showing.
func (m model) isAdding() bool {
	return m.mode == addingSearch || m.mode == addingPick || m.mode == addingForm
}

// startAdding opens the title box of the add flow.
func (m model) startAdding() (tea.Model, tea.Cmd) {
	m.mode = addingSearch
	m.loading = ""
	m.addTitle.Reset()
	return m, m.addTitle.Focus()
}

// openForm shows the add form, filled in with mv.
func (m model) openForm(mv Movie, fromTMDB bool) (tea.Model, tea.Cmd) {
	m.mode = addingForm
	m.fromTMDB = fromTMDB
	m.loading = ""
	m.addTitle.Blur()
	m.form = newForm(mv)
	m.formCursor = 0
	return m, m.form[0].input.Focus()
}

// updateAddSearch handles key presses in the title box.
func (m model) updateAddSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.review.active {
			return m.endReview("Review stopped")
		}
		m.mode = browsing
		m.addTitle.Blur()
		return m, nil
	case "enter":
		if title := strings.TrimSpace(m.addTitle.Value()); title != "" && m.loading == "" {
			m.loading = "Searching TMDB…"
			return m, searchTMDBCmd(title)
		}
		return m, nil
	case "tab":
		return m.fillInByHand()
	case "ctrl+x":
		if m.review.active {
			return m.skipReview()
		}
	}

	var cmd tea.Cmd
	m.addTitle, cmd = m.addTitle.Update(msg)
	return m, cmd
}

// fillInByHand skips TMDB and opens the form without its values.
func (m model) fillInByHand() (tea.Model, tea.Cmd) {
	if m.review.active {
		return m.openReviewForm(Movie{})
	}
	return m.openForm(Movie{Title: strings.TrimSpace(m.addTitle.Value())}, false)
}

// updateAddPick handles key presses in the list of TMDB results.
func (m model) updateAddPick(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = addingSearch
		m.loading = ""
		return m, m.addTitle.Focus()
	case "tab":
		return m.fillInByHand()
	case "ctrl+x":
		if m.review.active {
			return m.skipReview()
		}
	case "j", "down":
		m.resultCursor = min(m.resultCursor+1, len(m.results)-1)
	case "k", "up":
		m.resultCursor = max(m.resultCursor-1, 0)
	case "enter":
		if m.loading == "" {
			m.loading = "Fetching details…"
			return m, tmdbMovieCmd(m.results[m.resultCursor].ID)
		}
	}
	return m, nil
}

// updateAddForm handles key presses in the add form.
func (m model) updateAddForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	move := func(by int) (tea.Model, tea.Cmd) {
		m.form[m.formCursor].input.Blur()
		m.formCursor = (m.formCursor + by + len(m.form)) % len(m.form)
		return m, m.form[m.formCursor].input.Focus()
	}

	if m.review.active {
		switch msg.String() {
		case "esc":
			return m.endReview("Review stopped")
		case "ctrl+o":
			return m.swapFormValue()
		case "ctrl+x":
			return m.skipReview()
		case "ctrl+t": // pick another TMDB match, or search again
			m.form[m.formCursor].input.Blur()
			if len(m.results) > 0 {
				m.mode = addingPick
				return m, nil
			}
			m.mode = addingSearch
			return m, m.addTitle.Focus()
		}
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = browsing
		return m, nil
	case "tab", "down":
		return move(1)
	case "shift+tab", "up":
		return move(-1)
	case "enter":
		if m.formCursor < len(m.form)-1 {
			return move(1)
		}
		return m.saveForm()
	case "ctrl+s":
		return m.saveForm()
	}

	var cmd tea.Cmd
	m.form[m.formCursor].input, cmd = m.form[m.formCursor].input.Update(msg)
	return m, cmd
}

// saveForm checks the form and, if it's fine, starts adding it to the sheet.
func (m model) saveForm() (tea.Model, tea.Cmd) {
	if m.review.active {
		return m.saveReview()
	}
	if m.saving {
		return m, nil
	}
	mv, err := formMovie(m.form)
	if err != nil {
		m.flash = err
		return m, nil
	}
	m.saving = true
	return m, addMovieCmd(mv)
}

// startDeleting asks whether to delete the selected movie.
func (m model) startDeleting() (tea.Model, tea.Cmd) {
	if mv, ok := m.selected(); ok && !m.saving {
		m.mode = confirmingDelete
		m.deleting = mv
	}
	return m, nil
}

// updateConfirmDelete handles the y/n answer to "delete this movie?".
func (m model) updateConfirmDelete(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.mode = browsing
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y":
		m.saving = true
		return m, deleteMovieCmd(m.deleting)
	}
	return m, nil // anything else cancels
}

// updateEdits handles the results of TMDB lookups and sheet changes.
// Results that arrive after you've left their screen are dropped.
func (m model) updateEdits(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tmdbSearchMsg:
		if m.mode != addingSearch || m.loading == "" {
			return m, nil
		}
		m.loading = ""
		if m.review.active {
			return m.reviewResults(msg)
		}
		title := strings.TrimSpace(m.addTitle.Value())
		switch {
		case msg.err != nil:
			m.flash = msg.err // e.g. no token yet: you can still fill it in by hand
			return m.openForm(Movie{Title: title}, false)
		case len(msg.results) == 0:
			m.flash = fmt.Errorf("no TMDB matches for %q: fill it in by hand", title)
			return m.openForm(Movie{Title: title}, false)
		}
		m.mode = addingPick
		m.addTitle.Blur()
		m.results = msg.results
		m.resultCursor = 0

	case tmdbMovieMsg:
		if m.mode != addingPick || m.loading == "" {
			return m, nil
		}
		m.loading = ""
		if msg.err != nil {
			m.flash = msg.err
			return m, nil
		}
		if m.review.active {
			return m.openReviewForm(msg.movie)
		}
		return m.openForm(msg.movie, true)

	case movieAddedMsg:
		m.saving = false
		if msg.err != nil {
			m.flash = msg.err
			return m, nil
		}
		if m.mode == addingForm {
			m.mode = browsing
		}
		m.setMovies(append(slices.Clone(m.movies), msg.movie))
		m.selectID(msg.movie.ID, true)
		return m, tea.Batch(m.showNotice(fmt.Sprintf("Added %q", msg.movie.Title)), m.resync())

	case movieUpdatedMsg:
		return m.movieUpdated(msg)

	case movieDeletedMsg:
		m.saving = false
		if msg.err != nil {
			m.flash = msg.err
			return m, nil
		}
		m.setMovies(slices.DeleteFunc(slices.Clone(m.movies), func(mv Movie) bool { return mv.ID == msg.movie.ID }))
		return m, tea.Batch(m.showNotice(fmt.Sprintf("Deleted %q", msg.movie.Title)), m.resync())
	}
	return m, nil
}

// noticeDuration is how long a notice stays in the help bar.
const noticeDuration = 4 * time.Second

// clearNoticeMsg arrives noticeDuration after notice number id was shown.
type clearNoticeMsg struct{ id int }

// showNotice puts text in the help bar and starts the timer that clears it.
func (m *model) showNotice(text string) tea.Cmd {
	m.notice = text
	m.noticeID++
	id := m.noticeID
	return tea.Tick(noticeDuration, func(time.Time) tea.Msg { return clearNoticeMsg{id} })
}

// resync fetches the sheet again after a change, so the cache matches it.
func (m *model) resync() tea.Cmd {
	if m.syncing {
		return nil
	}
	m.syncing = true
	return syncCmd()
}

// addView draws the current add screen inside the main panel.
func (m model) addView(width, height int) string {
	heading := "Add a movie"
	if m.review.active {
		heading = m.reviewHeading()
	}
	lines := []string{titleStyle.Render(heading), ""}

	switch m.mode {
	case addingSearch:
		lines = append(lines, m.addTitle.View())

	case addingPick:
		heading := fmt.Sprintf("TMDB matches for %q:", m.addTitle.Value())
		if m.review.active {
			heading = fmt.Sprintf("TMDB matches for %q (the sheet says %d):", m.addTitle.Value(), m.review.sheet.Year)
		}
		lines = append(lines, labelStyle.Render(heading), "")
		// Scroll so the cursor stays visible, below the lines above.
		room := max(height-len(lines)-2, 1)
		start := max(m.resultCursor-room+1, 0)
		for i := start; i < min(start+room, len(m.results)); i++ {
			r := m.results[i]
			line := r.Title
			if y := r.year(); y > 0 {
				line += fmt.Sprintf(" (%d)", y)
			}
			if r.OriginalTitle != "" && r.OriginalTitle != r.Title {
				line += " · " + r.OriginalTitle
			}
			if i == m.resultCursor {
				line = selectedStyle.Render(pad(line, width, false))
			}
			lines = append(lines, line)
		}

	case addingForm:
		inputWidth := max(width-13, 10) // label, gap, and a column for the cursor
		if m.review.active {
			inputWidth = max((width-13)/2, 10) // the other half shows the sheet's and TMDB's values
		}
		for i, f := range m.form {
			f.input.SetWidth(inputWidth)
			label := labelStyle.Render(pad(f.label, 10, false))
			if i == m.formCursor {
				label = titleStyle.Render(pad(f.label, 10, false))
			}
			line := label + "  " + f.input.View()
			if m.review.active {
				line = pad(line, 13+inputWidth, false) + "  " + reviewHint(f)
			}
			lines = append(lines, line)
		}
		if m.fromTMDB {
			lines = append(lines, "", labelStyle.Render("Filled in from TMDB, which describes the film, not the disc: check the studio and languages."))
		}
	}

	if m.loading != "" {
		lines = append(lines, "", labelStyle.Render(m.loading))
	}
	return strings.Join(lines, "\n")
}
