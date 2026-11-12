package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
)

// cacheFile is what we store on disk: the last movies we fetched, and when.
type cacheFile struct {
	SyncedAt time.Time `json:"synced_at"`
	Movies   []Movie   `json:"movies"`
}

// cachePath is ~/.cache/moviemanager/movies.json on Linux (or $XDG_CACHE_HOME).
func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "moviemanager", "movies.json"), nil
}

// loadCache reads the cache file. A missing file is not an error: it just
// means this is the first run.
func loadCache() (cacheFile, error) {
	var cache cacheFile
	path, err := cachePath()
	if err != nil {
		return cache, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cache, nil
	}
	if err != nil {
		return cache, err
	}

	if err := json.Unmarshal(data, &cache); err != nil {
		return cacheFile{}, fmt.Errorf("reading cache %s: %w", path, err)
	}
	return cache, nil
}

// saveCache writes the cache file. It writes to a temporary file first and
// then renames it, so a crash halfway never leaves a broken cache behind.
func saveCache(cache cacheFile) error {
	path, err := cachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Messages sent back to Update when a background sync finishes.
type (
	syncDoneMsg struct {
		movies   []Movie
		syncedAt time.Time
		cacheErr error // the sync worked, but saving the cache failed
	}
	syncFailedMsg struct{ err error }
)

// syncCmd fetches the sheet in the background and saves it to the cache.
// Bubble Tea runs the returned function in its own goroutine, so the UI
// stays responsive while it waits for the network.
func syncCmd() tea.Cmd {
	return func() tea.Msg {
		movies, err := fetchMovies(sheetURL())
		if err != nil {
			return syncFailedMsg{err}
		}
		cache := cacheFile{SyncedAt: time.Now(), Movies: movies}
		return syncDoneMsg{movies, cache.SyncedAt, saveCache(cache)}
	}
}

// tickMsg arrives once a minute so "synced 5m ago" stays up to date.
type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Minute, func(time.Time) tea.Msg { return tickMsg{} })
}

// timeAgo formats how long ago t was: "just now", "5m ago", "3h ago", "Nov 2".
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return t.Format("Jan 2")
	}
}
