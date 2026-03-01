package parser

import (
	"hash/fnv"
	"strings"
	"unicode"
)

// SimHash computes a 64-bit SimHash fingerprint of the visible text content.
// Near-duplicate pages will have SimHash values with low Hamming distance.
// Use bitCount(bitXor(a, b)) in ClickHouse to compare.
func SimHash(text string) uint64 {
	var v [64]int

	tokenize(text, func(token string) {
		h := hashToken(token)
		for i := 0; i < 64; i++ {
			if h&(1<<uint(i)) != 0 {
				v[i]++
			} else {
				v[i]--
			}
		}
	})

	var fingerprint uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			fingerprint |= 1 << uint(i)
		}
	}
	return fingerprint
}

// tokenize splits text into lowercased word tokens and calls fn for each.
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

func hashToken(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
