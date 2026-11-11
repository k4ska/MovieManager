package main

import (
	"slices"
	"strings"
)

// genres is the fixed list a movie's genre comes from. Keeping it short and
// distinctive makes the genre filter and stats useful: every movie is exactly
// one of these.
var genres = []string{"Animation", "Anime", "Action", "Adventure", "Comedy", "Horror", "Music", "Science Fiction"}

// genreAliases are other spellings of a genre, e.g. from older rows of the sheet.
var genreAliases = map[string]string{
	"sci-fi":  "Science Fiction",
	"musical": "Music",
}

// normalizeGenre turns any capitalization or alias of a genre into its name
// in genres. ok is false if s isn't one of them.
func normalizeGenre(s string) (genre string, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if g, ok := genreAliases[s]; ok {
		return g, true
	}
	i := slices.IndexFunc(genres, func(g string) bool { return strings.ToLower(g) == s })
	if i < 0 {
		return "", false
	}
	return genres[i], true
}

// pickGenre chooses one of genres for a film from TMDB's genre names, which
// come main genre first. Being animated beats everything else, because that's
// what you'd look for on the shelf: Japanese animation is Anime, the rest is
// Animation. Otherwise it's the first TMDB genre that's in the list, or "" if
// none is (e.g. a film that's only Drama).
func pickGenre(tmdbGenres []string, originalLanguage string) string {
	if slices.Contains(tmdbGenres, "Animation") {
		if originalLanguage == "ja" {
			return "Anime"
		}
		return "Animation"
	}
	for _, name := range tmdbGenres {
		if g, ok := normalizeGenre(name); ok {
			return g
		}
	}
	return ""
}
