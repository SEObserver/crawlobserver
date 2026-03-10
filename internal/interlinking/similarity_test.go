package interlinking

import (
	"testing"
)

func TestCosineSimilarityClampedToOne(t *testing.T) {
	// Create two documents with identical term vectors but slightly different norms
	// that could produce sim > 1.0 due to floating point
	a := &Document{
		TermFreqs: map[uint32]float64{0: 1.0, 1: 1.0},
		Norm:      1.4142135623730950, // slightly less than sqrt(2)
	}
	b := &Document{
		TermFreqs: map[uint32]float64{0: 1.0, 1: 1.0},
		Norm:      1.4142135623730950,
	}

	sim := cosineSimilarity(a, b)
	if sim > 1.0 {
		t.Errorf("cosineSimilarity should be clamped to 1.0, got %v", sim)
	}

	// Normal case: identical vectors should give exactly 1.0
	if sim < 0.999 {
		t.Errorf("identical vectors should have similarity ~1.0, got %v", sim)
	}
}

func TestCosineSimilarityZeroNorm(t *testing.T) {
	a := &Document{TermFreqs: map[uint32]float64{0: 1.0}, Norm: 0}
	b := &Document{TermFreqs: map[uint32]float64{0: 1.0}, Norm: 1.0}

	sim := cosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("zero norm should give similarity 0, got %v", sim)
	}
}
