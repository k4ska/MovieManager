package main

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// count is one bar in a chart: a label and how many movies have it.
type count struct {
	label string
	n     int
}

// collectionStats is everything shown on the stats screen.
type collectionStats struct {
	movies         int
	totalMinutes   int
	withDuration   int // movies that have a duration filled in
	oldest, newest int
	genres         []count
	decades        []count
	studios        []count
}

func computeStats(movies []Movie) collectionStats {
	s := collectionStats{movies: len(movies)}
	genres := map[string]int{}
	decades := map[string]int{}
	studios := map[string]int{}

	for i, mv := range movies {
		if mv.Duration > 0 {
			s.totalMinutes += mv.Duration
			s.withDuration++
		}
		if i == 0 || mv.Year < s.oldest {
			s.oldest = mv.Year
		}
		s.newest = max(s.newest, mv.Year)

		for _, g := range mv.Genres {
			genres[g]++ // a movie with three genres counts once in each
		}
		decades[fmt.Sprintf("%ds", mv.Year/10*10)]++
		if mv.Studio != "" {
			studios[mv.Studio]++
		}
	}

	s.genres = byCount(genres)
	s.studios = byCount(studios)
	s.decades = byCount(decades)
	slices.SortFunc(s.decades, func(a, b count) int { return cmp.Compare(a.label, b.label) }) // chronological
	return s
}

// byCount turns a map of counts into a list, biggest first, ties alphabetical.
func byCount(counts map[string]int) []count {
	var list []count
	for label, n := range counts {
		list = append(list, count{label, n})
	}
	slices.SortFunc(list, func(a, b count) int {
		if c := cmp.Compare(b.n, a.n); c != 0 {
			return c
		}
		return cmp.Compare(a.label, b.label)
	})
	return list
}

// updateStats handles key presses on the stats screen.
func (m model) updateStats(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "t", "q":
		m.mode = browsing
	}
	return m, nil
}

// statsView renders the stats screen for the movies currently in the list.
func (m model) statsView(width, height int) string {
	movies := make([]Movie, len(m.visible))
	for i, idx := range m.visible {
		movies[i] = m.movies[idx]
	}
	s := computeStats(movies)

	scope := "all " + plural(s.movies, "movie")
	if len(m.visible) != len(m.movies) {
		scope = fmt.Sprintf("%d of %d movies", len(m.visible), len(m.movies))
	}
	lines := []string{titleStyle.Render("Stats") + labelStyle.Render(" · "+scope), ""}
	if s.movies == 0 {
		return strings.Join(append(lines, "No movies to count."), "\n")
	}

	// Headline numbers.
	runtime := formatDuration(s.totalMinutes)
	if runtime == "" {
		runtime = "–"
	}
	runtimeNote := "runtime"
	if days := s.totalMinutes / (24 * 60); days >= 1 {
		runtimeNote += " · " + plural(days, "day")
	}
	average := "–"
	if s.withDuration > 0 {
		average = formatDuration(s.totalMinutes / s.withDuration)
	}
	years := strconv.Itoa(s.oldest)
	if s.newest != s.oldest {
		years += "–" + strconv.Itoa(s.newest)
	}

	tiles := []string{
		statTile(strconv.Itoa(s.movies), "movies"),
		statTile(runtime, runtimeNote),
		statTile(average, "average"),
		statTile(years, "years"),
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, tiles...)
	if lipgloss.Width(row) > width { // too narrow: two rows of two
		row = lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Top, tiles[:2]...),
			lipgloss.JoinHorizontal(lipgloss.Top, tiles[2:]...))
	}
	lines = append(lines, strings.Split(row, "\n")...)
	if missing := s.movies - s.withDuration; missing > 0 {
		lines = append(lines, labelStyle.Render(fmt.Sprintf("%s without a duration are not counted in runtime", plural(missing, "movie"))))
	}
	lines = append(lines, "")

	// Bar charts: side by side if there's room, otherwise stacked.
	chartHeight := height - len(lines)
	charts := []struct {
		title  string
		counts []count
	}{
		{"Genre", s.genres},
		{"Decade", s.decades},
		{"Studio", s.studios},
	}

	const minChartWidth = 26
	if width >= len(charts)*minChartWidth+(len(charts)-1)*columnGap*2 {
		chartWidth := (width - (len(charts)-1)*columnGap*2) / len(charts)
		var rendered []string
		for i, c := range charts {
			chart := barChart(c.title, c.counts, chartWidth, chartHeight)
			if i > 0 {
				chart = lipgloss.NewStyle().PaddingLeft(columnGap * 2).Render(chart)
			}
			rendered = append(rendered, chart)
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, rendered...))
	} else {
		perChart := max((chartHeight-len(charts)+1)/len(charts), 3)
		for i, c := range charts {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, barChart(c.title, c.counts, width, perChart))
		}
	}
	return strings.Join(lines, "\n")
}

// statTile renders one headline number with a small label under it.
func statTile(value, label string) string {
	return lipgloss.NewStyle().PaddingRight(4).Render(
		statValueStyle.Render(value) + "\n" + labelStyle.Render(label))
}

// barChart renders a titled horizontal bar chart in at most height lines.
// Bars are scaled to the biggest count and drawn with eighth-blocks, so
// small differences are still visible.
func barChart(title string, counts []count, width, height int) string {
	lines := []string{columnHeaderStyle.Render(title)}
	if len(counts) == 0 {
		return strings.Join(append(lines, labelStyle.Render("none")), "\n")
	}

	// If not everything fits, show the top ones and say how many are hidden.
	shown, hidden := counts, 0
	if room := max(height-1, 1); len(counts) > room {
		shown = counts[:max(room-1, 1)]
		hidden = len(counts) - len(shown)
	}

	labelWidth, biggest := 0, 0
	for _, c := range shown {
		labelWidth = max(labelWidth, lipgloss.Width(c.label))
		biggest = max(biggest, c.n)
	}
	labelWidth = min(labelWidth, width/2)
	numberWidth := len(strconv.Itoa(biggest))
	barWidth := max(width-labelWidth-numberWidth-2, 1)

	for _, c := range shown {
		bar := barStyle.Render(drawBar(c.n, biggest, barWidth))
		gap := strings.Repeat(" ", barWidth-lipgloss.Width(bar)+1)
		lines = append(lines, pad(c.label, labelWidth, false)+" "+bar+gap+pad(strconv.Itoa(c.n), numberWidth, true))
	}
	if hidden > 0 {
		lines = append(lines, labelStyle.Render(fmt.Sprintf("+ %d more", hidden)))
	}
	return strings.Join(lines, "\n")
}

// drawBar draws a bar of n/biggest of width, using ▏▎▍▌▋▊▉ for the last
// partial cell so the length is accurate to 1/8 of a character.
func drawBar(n, biggest, width int) string {
	eighths := n * width * 8 / biggest
	bar := strings.Repeat("█", eighths/8)
	if rest := eighths % 8; rest > 0 {
		bar += string([]rune(" ▏▎▍▌▋▊▉")[rest])
	}
	return bar
}
