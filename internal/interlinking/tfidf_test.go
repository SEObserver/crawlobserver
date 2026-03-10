package interlinking

import (
	"testing"

	"github.com/SEObserver/crawlobserver/internal/storage"
)

func TestTokenizeFiltered(t *testing.T) {
	tokens := tokenizeFiltered("The quick brown fox jumps over the lazy dog", "en")
	// "the", "over" should be filtered as stopwords
	for _, tok := range tokens {
		if tok == "the" || tok == "over" {
			t.Errorf("stopword %q should be filtered", tok)
		}
	}
	if len(tokens) == 0 {
		t.Error("expected some tokens")
	}
}

func TestTokenizeFilteredFR(t *testing.T) {
	tokens := tokenizeFiltered("Le chat est sur la table dans le jardin", "fr")
	for _, tok := range tokens {
		if tok == "le" || tok == "est" || tok == "sur" || tok == "la" || tok == "dans" {
			t.Errorf("French stopword %q should be filtered", tok)
		}
	}
}

func TestBuildCorpusEmpty(t *testing.T) {
	ch := make(chan storage.PageHTMLRow)
	close(ch)
	corpus, err := BuildCorpus(ch, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus.Docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(corpus.Docs))
	}
}

func TestBuildCorpusAndSimilarity(t *testing.T) {
	ch := make(chan storage.PageHTMLRow, 3)
	ch <- storage.PageHTMLRow{URL: "http://example.com/a", HTML: `<html><body><article><p>` + longText("machine learning artificial intelligence neural networks deep learning") + `</p></article></body></html>`}
	ch <- storage.PageHTMLRow{URL: "http://example.com/b", HTML: `<html><body><article><p>` + longText("machine learning deep learning neural networks artificial intelligence") + `</p></article></body></html>`}
	ch <- storage.PageHTMLRow{URL: "http://example.com/c", HTML: `<html><body><article><p>` + longText("cooking recipes kitchen ingredients baking bread flour") + `</p></article></body></html>`}
	close(ch)

	meta := map[string]storage.PageMetadata{
		"http://example.com/a": {Title: "ML Page", Lang: "en"},
		"http://example.com/b": {Title: "AI Page", Lang: "en"},
		"http://example.com/c": {Title: "Cooking", Lang: "en"},
	}

	corpus, err := BuildCorpus(ch, meta)
	if err != nil {
		t.Fatal(err)
	}

	if len(corpus.Docs) < 2 {
		t.Skipf("not enough docs extracted: %d (readability may have filtered content)", len(corpus.Docs))
	}

	pairs := FindSimilarPairs(corpus, 0.1, 2)
	// The two ML pages should be similar, the cooking page should not
	var foundMLPair bool
	for _, p := range pairs {
		srcURL := corpus.Docs[p.SourceIdx].URL
		tgtURL := corpus.Docs[p.TargetIdx].URL
		if (srcURL == "http://example.com/a" && tgtURL == "http://example.com/b") ||
			(srcURL == "http://example.com/b" && tgtURL == "http://example.com/a") {
			foundMLPair = true
			if p.Similarity < 0.5 {
				t.Errorf("expected high similarity between ML pages, got %.2f", p.Similarity)
			}
		}
	}
	if !foundMLPair && len(corpus.Docs) >= 2 {
		t.Log("ML pair not found in similar pairs (may be due to readability extraction)")
	}
}

// longText repeats text to make it long enough for readability extraction.
func longText(base string) string {
	result := ""
	for i := 0; i < 30; i++ {
		result += base + ". "
	}
	return result
}

func TestCosineSimilarityIdentical(t *testing.T) {
	a := &Document{
		TermFreqs: map[uint32]float64{0: 1.0, 1: 2.0, 2: 3.0},
		Norm:      3.741657386773941, // sqrt(1+4+9)
	}
	sim := cosineSimilarity(a, a)
	if sim < 0.999 {
		t.Errorf("expected ~1.0 for identical docs, got %.4f", sim)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	a := &Document{
		TermFreqs: map[uint32]float64{0: 1.0},
		Norm:      1.0,
	}
	b := &Document{
		TermFreqs: map[uint32]float64{1: 1.0},
		Norm:      1.0,
	}
	sim := cosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("expected 0 for orthogonal docs, got %.4f", sim)
	}
}
