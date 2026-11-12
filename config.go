package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// configDir is ~/.config/moviemanager on Linux (or $XDG_CONFIG_HOME). It holds
// the secrets the app needs for writing to the sheet and looking up movies:
//
//	sheet                 the Google Sheet's URL or ID (required)
//	service-account.json  Google service account key (for adding/deleting)
//	tmdb-token            TMDB API Read Access Token (for auto-filling)
func configDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "moviemanager"), nil
}

// configFile returns the path of a file in configDir.
func configFile(name string) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// readConfigFile reads a file from configDir, with a helpful error if it's missing.
func readConfigFile(name, what string) ([]byte, error) {
	path, err := configFile(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%s not set up: put it in %s", what, path)
	}
	return data, err
}

// tmdbToken returns the TMDB token from $TMDB_TOKEN or the tmdb-token file.
func tmdbToken() (string, error) {
	if token := strings.TrimSpace(os.Getenv("TMDB_TOKEN")); token != "" {
		return token, nil
	}
	data, err := readConfigFile("tmdb-token", "TMDB token")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// sheetSetting returns the spreadsheet ID from $MOVIEMANAGER_SHEET or the
// sheet file. Either may hold the bare ID or the whole URL from the browser.
func sheetSetting() (string, error) {
	value := strings.TrimSpace(os.Getenv("MOVIEMANAGER_SHEET"))
	if value == "" {
		data, err := readConfigFile("sheet", "Google Sheet")
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(string(data))
	}
	id := spreadsheetIDFrom(value)
	if id == "" {
		return "", fmt.Errorf("no spreadsheet ID in %q", value)
	}
	return id, nil
}

// spreadsheetIDFrom picks the ID out of a sheet URL like
// https://docs.google.com/spreadsheets/d/<ID>/edit#gid=0, or returns value
// as-is if it isn't a URL.
func spreadsheetIDFrom(value string) string {
	_, rest, found := strings.Cut(value, "/d/")
	if !found {
		return value
	}
	id, _, _ := strings.Cut(rest, "/")
	return id
}
