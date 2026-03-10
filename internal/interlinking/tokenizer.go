package interlinking

import (
	"strings"
	"unicode"
)

// tokenize splits text into lowercased word tokens and calls fn for each.
// Copied from parser/simhash.go to avoid coupling.
func tokenize(text string, fn func(string)) {
	var buf strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf.WriteRune(unicode.ToLower(r))
		} else if buf.Len() > 0 {
			fn(buf.String())
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		fn(buf.String())
	}
}

// tokenizeFiltered splits text into tokens, filtering stopwords based on language.
func tokenizeFiltered(text, lang string) []string {
	sw := stopwordsFor(lang)
	var tokens []string
	tokenize(text, func(tok string) {
		if len(tok) < 2 {
			return
		}
		if _, skip := sw[tok]; skip {
			return
		}
		tokens = append(tokens, tok)
	})
	return tokens
}
