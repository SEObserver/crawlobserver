package interlinking

import (
	_ "embed"
	"strings"
)

//go:embed stopwords_en.txt
var stopwordsEnRaw string

//go:embed stopwords_fr.txt
var stopwordsFrRaw string

var (
	stopwordsEN map[string]struct{}
	stopwordsFR map[string]struct{}
)

func init() {
	stopwordsEN = parseStopwords(stopwordsEnRaw)
	stopwordsFR = parseStopwords(stopwordsFrRaw)
}

func parseStopwords(raw string) map[string]struct{} {
	m := make(map[string]struct{})
	for _, line := range strings.Split(raw, "\n") {
		w := strings.TrimSpace(strings.ToLower(line))
		if w != "" && !strings.HasPrefix(w, "#") {
			m[w] = struct{}{}
		}
	}
	return m
}

// stopwordsFor returns the stopword set for the given language code.
func stopwordsFor(lang string) map[string]struct{} {
	lang = strings.ToLower(lang)
	if strings.HasPrefix(lang, "fr") {
		return stopwordsFR
	}
	return stopwordsEN
}
