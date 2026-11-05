package main

import (
	"fmt"
	"os"
	"slices"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// mode is what the keyboard is currently controlling.
type mode int

const (
	browsing  mode = iota // moving around the list
	searching             // typing in the search box
	filtering             // picking genres and languages
)

// model holds all the state of the app.
type model struct {
	movies  []Movie
	visible []int // indexes into movies that pass the search and filters, in sorted order
	cursor  int   // position in visible of the selected movie
	offset  int   // position in visible of the first row on screen (for scrolling)
	mode    mode

	search textinput.Model

	filterOptions []filterOption
	activeFilters map[filterOption]bool
	filterCursor  int

	sortBy      sortKey
	sortReverse bool

	syncing  bool      // a background fetch of the sheet is running
	syncedAt time.Time // when the movies were last fetched (zero if never)
	syncErr  error     // why the last sync failed, if it did
	flash    error     // shown in the help bar until the next key press

	width, height int // terminal size, set by tea.WindowSizeMsg
}

func newModel(movies []Movie) model {
	search := textinput.New()
	search.Prompt = "/ "
	search.Placeholder = "title, genre, studio, language…"
	styles := search.Styles()
	styles.Focused.Prompt = styles.Focused.Prompt.Foreground(accent)
	search.SetStyles(styles)

	m := model{
		movies:        movies,
		search:        search,
		filterOptions: buildFilterOptions(movies),
		activeFilters: map[filterOption]bool{},
	}
	m.refresh()
	return m
}

// Init runs once at startup: fetch fresh movies and start the clock.
func (m model) Init() tea.Cmd {
	return tea.Batch(syncCmd(), tickCmd())
}

// Update reacts to events (key presses, window resizes, ...) and returns the new model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.SetWidth(m.width / 2)
		m.scrollToCursor()
		return m, nil

	case syncDoneMsg:
		m.syncing = false
		m.syncedAt = msg.syncedAt
		m.syncErr = nil
		m.flash = msg.cacheErr // usually nil
		m.setMovies(msg.movies)
		return m, nil

	case syncFailedMsg:
		m.syncing = false
		m.syncErr = msg.err
		m.flash = msg.err
		return m, nil

	case tickMsg:
		return m, tickCmd() // nothing to change: just redraw, and schedule the next tick

	case tea.KeyPressMsg:
		m.flash = nil
		switch m.mode {
		case searching:
			return m.updateSearch(msg)
		case filtering:
			return m.updateFilter(msg)
		default:
			return m.updateBrowse(msg)
		}
	}

	// Other messages (like the cursor blinking) belong to the search box.
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	return m, cmd
}

// updateBrowse handles key presses while moving around the list.
func (m model) updateBrowse(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "/":
		m.mode = searching
		return m, m.search.Focus()
	case "f":
		m.mode = filtering
	case "R":
		if !m.syncing {
			m.syncing = true
			return m, syncCmd()
		}
	case "s":
		m.sortBy = (m.sortBy + 1) % numSortKeys
		m.refresh()
	case "S":
		m.sortReverse = !m.sortReverse
		m.refresh()
	case "esc":
		m.search.Reset()
		clear(m.activeFilters)
		m.refresh()
	case "j", "down":
		m.cursor = min(m.cursor+1, len(m.visible)-1)
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.visible) - 1
	}

	m.cursor = max(m.cursor, 0) // the list may be empty
	m.scrollToCursor()
	return m, nil
}

// refresh rebuilds visible from the search, filters and sort order. The
// selected movie stays selected if it's still in the list.
func (m *model) refresh() {
	id, ok := m.selectedID()
	m.rebuild()
	m.selectID(id, ok)
}

// setMovies replaces the whole collection, e.g. after a sync, keeping the
// selection, search and filters.
func (m *model) setMovies(movies []Movie) {
	id, ok := m.selectedID() // must happen before visible stops matching movies
	m.movies = movies

	m.filterOptions = buildFilterOptions(movies)
	m.filterCursor = min(m.filterCursor, max(len(m.filterOptions)-1, 0))
	for opt := range m.activeFilters {
		if !slices.Contains(m.filterOptions, opt) {
			delete(m.activeFilters, opt) // that genre or language no longer exists
		}
	}

	m.rebuild()
	m.selectID(id, ok)
}

// rebuild recomputes visible: the movies that pass the search and filters, sorted.
func (m *model) rebuild() {
	query := m.searchQuery()
	m.visible = nil
	for i, movie := range m.movies {
		if movie.matches(query) && m.matchesFilters(movie) {
			m.visible = append(m.visible, i)
		}
	}

	slices.SortStableFunc(m.visible, func(a, b int) int {
		ma, mb := m.movies[a], m.movies[b]
		// Unknown durations go last in both directions.
		if m.sortBy == byDuration && (ma.Duration == 0) != (mb.Duration == 0) {
			if ma.Duration == 0 {
				return 1
			}
			return -1
		}
		c := compareMovies(ma, mb, m.sortBy)
		if m.sortReverse {
			c = -c
		}
		return c
	})

}

// selectID moves the cursor to the movie with this ID, or to the top if it's
// not in the list.
func (m *model) selectID(id int, ok bool) {
	m.cursor = 0
	if ok {
		if i := slices.IndexFunc(m.visible, func(idx int) bool { return m.movies[idx].ID == id }); i >= 0 {
			m.cursor = i
		}
	}
	m.scrollToCursor()
}

// selectedID returns the ID of the movie under the cursor, if there is one.
func (m model) selectedID() (int, bool) {
	movie, ok := m.selected()
	return movie.ID, ok
}

// selected returns the movie under the cursor, if there is one.
func (m model) selected() (Movie, bool) {
	if m.cursor >= len(m.visible) {
		return Movie{}, false
	}
	return m.movies[m.visible[m.cursor]], true
}

// visibleRows is how many movies fit in the table, below its header row.
func (m model) visibleRows() int {
	return max(m.height-chromeHeight-panelStyle.GetVerticalFrameSize()-1, 1)
}

// scrollToCursor moves the scroll offset just enough to keep the cursor on screen.
func (m *model) scrollToCursor() {
	m.offset = min(m.offset, m.cursor)
	if m.cursor >= m.offset+m.visibleRows() {
		m.offset = m.cursor - m.visibleRows() + 1
	}
}

func main() {
	// Start with the movies from last time; Init fetches fresh ones in the background.
	cache, err := loadCache()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: ignoring cache:", err)
	}

	m := newModel(cache.Movies)
	m.syncedAt = cache.SyncedAt
	m.syncing = true

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
