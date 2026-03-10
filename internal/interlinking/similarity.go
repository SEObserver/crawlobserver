package interlinking

// SimilarPair represents two documents with high cosine similarity.
type SimilarPair struct {
	SourceIdx  int
	TargetIdx  int
	Similarity float64
}

// FindSimilarPairs finds document pairs above a similarity threshold.
// Uses an inverted index on high-IDF terms to avoid N² comparisons.
func FindSimilarPairs(corpus *Corpus, threshold float64, minCommonTerms int) []SimilarPair {
	if len(corpus.Docs) < 2 {
		return nil
	}

	// Build inverted index: termID → list of doc indices
	// Only index terms with above-median IDF (discriminative terms)
	inverted := make(map[uint32][]int)
	for docIdx, doc := range corpus.Docs {
		for termID := range doc.TermFreqs {
			inverted[termID] = append(inverted[termID], docIdx)
		}
	}

	// For each doc, collect candidate docs sharing enough terms
	type pairKey struct{ a, b int }
	candidateCounts := make(map[pairKey]int)

	for _, docIndices := range inverted {
		// Skip very common terms (posting list too long = not discriminative)
		if len(docIndices) > len(corpus.Docs)/5+1 {
			continue
		}
		for i := 0; i < len(docIndices); i++ {
			for j := i + 1; j < len(docIndices); j++ {
				key := pairKey{docIndices[i], docIndices[j]}
				candidateCounts[key]++
			}
		}
	}

	// Compute exact cosine similarity for qualifying candidates
	var pairs []SimilarPair
	for key, count := range candidateCounts {
		if count < minCommonTerms {
			continue
		}

		sim := cosineSimilarity(&corpus.Docs[key.a], &corpus.Docs[key.b])
		if sim >= threshold {
			pairs = append(pairs, SimilarPair{
				SourceIdx:  key.a,
				TargetIdx:  key.b,
				Similarity: sim,
			})
		}
	}

	return pairs
}

// cosineSimilarity computes cosine similarity between two sparse TF-IDF vectors.
func cosineSimilarity(a, b *Document) float64 {
	if a.Norm == 0 || b.Norm == 0 {
		return 0
	}

	// Iterate the smaller map for efficiency
	small, large := a.TermFreqs, b.TermFreqs
	if len(small) > len(large) {
		small, large = large, small
	}

	var dot float64
	for id, wA := range small {
		if wB, ok := large[id]; ok {
			dot += wA * wB
		}
	}

	return dot / (a.Norm * b.Norm)
}
