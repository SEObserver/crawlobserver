package interlinking

import (
	"testing"
)

func TestComputeOpportunityScore(t *testing.T) {
	// sim=0.5 should score higher than sim=1.0 (cannibalization)
	score05 := computeOpportunityScore(0.5, 10, 5, 500, 500)
	score10 := computeOpportunityScore(1.0, 10, 5, 500, 500)
	if score05 <= score10 {
		t.Errorf("sim=0.5 score (%.4f) should be > sim=1.0 score (%.4f)", score05, score10)
	}

	// sim=0.5 should score higher than sim=0.1 (too dissimilar)
	score01 := computeOpportunityScore(0.1, 10, 5, 500, 500)
	if score05 <= score01 {
		t.Errorf("sim=0.5 score (%.4f) should be > sim=0.1 score (%.4f)", score05, score01)
	}

	// sim=1.0 should produce zero relevance (4 * 1 * 0 = 0)
	if score10 != 0 {
		t.Errorf("sim=1.0 score should be 0, got %.4f", score10)
	}

	// sim=0.0 should also produce zero
	score00 := computeOpportunityScore(0.0, 10, 5, 500, 500)
	if score00 != 0 {
		t.Errorf("sim=0.0 score should be 0, got %.4f", score00)
	}
}

func TestComputeOpportunityScore_ThinContent(t *testing.T) {
	// Pages with low word count should score lower
	scoreNormal := computeOpportunityScore(0.5, 10, 5, 500, 500)
	scoreThin := computeOpportunityScore(0.5, 10, 5, 100, 500)
	if scoreThin >= scoreNormal {
		t.Errorf("thin content score (%.4f) should be < normal score (%.4f)", scoreThin, scoreNormal)
	}

	// Very thin content (50 words) should penalize heavily
	scoreVeryThin := computeOpportunityScore(0.5, 10, 5, 50, 500)
	if scoreVeryThin >= scoreThin {
		t.Errorf("very thin score (%.4f) should be < thin score (%.4f)", scoreVeryThin, scoreThin)
	}
}

func TestClassifyPair(t *testing.T) {
	tests := []struct {
		similarity float64
		want       string
	}{
		{0.3, "opportunity"},
		{0.5, "opportunity"},
		{0.84, "opportunity"},
		{0.85, "cannibalization"},
		{0.9, "cannibalization"},
		{1.0, "cannibalization"},
	}
	for _, tc := range tests {
		got := classifyPair(tc.similarity)
		if got != tc.want {
			t.Errorf("classifyPair(%.2f) = %q, want %q", tc.similarity, got, tc.want)
		}
	}
}
