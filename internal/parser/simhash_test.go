package parser

import (
	"math/bits"
	"testing"
)

// ---------------------------------------------------------------------------
// SimHash
// ---------------------------------------------------------------------------

func TestSimHash_Determinism(t *testing.T) {
	text := "the quick brown fox jumps over the lazy dog"
	h1 := SimHash(text)
	h2 := SimHash(text)
	if h1 != h2 {
		t.Fatalf("SimHash not deterministic: got %d and %d for the same input", h1, h2)
	}
}

func TestSimHash_EmptyText(t *testing.T) {
	h := SimHash("")
	if h != 0 {
		t.Fatalf("SimHash of empty text should be 0, got %d", h)
	}
}

func TestSimHash_SingleWord(t *testing.T) {
	h := SimHash("hello")
	if h == 0 {
		t.Fatal("SimHash of a single word should be non-zero")
	}
}

func TestSimHash_SimilarTexts_LowHammingDistance(t *testing.T) {
	a := SimHash("the quick brown fox jumps over the lazy dog")
	b := SimHash("the quick brown fox leaps over the lazy dog")

	dist := bits.OnesCount64(a ^ b)
	// Similar texts should have a low Hamming distance. A threshold of 10
	// is generous; in practice the distance is typically much smaller.
	if dist > 10 {
		t.Fatalf("expected low Hamming distance for similar texts, got %d (a=%064b b=%064b)", dist, a, b)
	}
}

func TestSimHash_DifferentTexts_HighHammingDistance(t *testing.T) {
	a := SimHash("hello world")
	b := SimHash("completely different unrelated text")

	dist := bits.OnesCount64(a ^ b)
	// Very different texts should have a high Hamming distance (close to 32
	// for independent random 64-bit values). We use a conservative lower
	// bound of 10 to avoid flaky tests.
	if dist < 10 {
		t.Fatalf("expected high Hamming distance for different texts, got %d (a=%064b b=%064b)", dist, a, b)
	}
}

// ---------------------------------------------------------------------------
// tokenize
// ---------------------------------------------------------------------------

func collectTokens(text string) []string {
	var tokens []string
	tokenize(text, func(tok string) {
		tokens = append(tokens, tok)
	})
	return tokens
}

func TestTokenize_SimpleWords(t *testing.T) {
	tokens := collectTokens("hello world")
	want := []string{"hello", "world"}
	assertTokens(t, tokens, want)
}

func TestTokenize_PunctuationStripped(t *testing.T) {
	tokens := collectTokens("hello, world!")
	want := []string{"hello", "world"}
	assertTokens(t, tokens, want)
}

func TestTokenize_UnicodeLetters(t *testing.T) {
	tokens := collectTokens("café résumé")
	want := []string{"café", "résumé"}
	assertTokens(t, tokens, want)
}

func TestTokenize_DigitsIncluded(t *testing.T) {
	tokens := collectTokens("test123 foo")
	want := []string{"test123", "foo"}
	assertTokens(t, tokens, want)
}

func TestTokenize_Lowercased(t *testing.T) {
	tokens := collectTokens("Hello WORLD")
	want := []string{"hello", "world"}
	assertTokens(t, tokens, want)
}

func TestTokenize_EmptyString(t *testing.T) {
	tokens := collectTokens("")
	if len(tokens) != 0 {
		t.Fatalf("expected no tokens for empty string, got %v", tokens)
	}
}

func TestTokenize_OnlyPunctuation(t *testing.T) {
	tokens := collectTokens("!@#$%^&*()")
	if len(tokens) != 0 {
		t.Fatalf("expected no tokens for only-punctuation string, got %v", tokens)
	}
}

// ---------------------------------------------------------------------------
// hashToken
// ---------------------------------------------------------------------------

func TestHashToken_Determinism(t *testing.T) {
	h1 := hashToken("hello")
	h2 := hashToken("hello")
	if h1 != h2 {
		t.Fatalf("hashToken not deterministic: got %d and %d for the same input", h1, h2)
	}
}

func TestHashToken_DifferentInputs(t *testing.T) {
	h1 := hashToken("hello")
	h2 := hashToken("world")
	if h1 == h2 {
		t.Fatalf("hashToken produced same value for different inputs: %d", h1)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertTokens(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("token count mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d] mismatch: got %q, want %q", i, got[i], want[i])
		}
	}
}
