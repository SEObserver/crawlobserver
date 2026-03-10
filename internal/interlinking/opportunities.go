package interlinking

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

// computeOpportunityScore returns a score that peaks at similarity=0.5
// (complementary content in the same cluster) and drops to 0 at similarity=0 and 1.
func computeOpportunityScore(similarity, srcPR, tgtPR float64, srcWC, tgtWC uint32) float64 {
	// Bell curve: peaks at similarity=0.5, zero at 0 and 1
	relevance := 4.0 * similarity * (1.0 - similarity)
	// Higher when source PR > target PR (juice flows down)
	srcLog := math.Log(1.0 + srcPR)
	tgtLog := math.Max(1.0, math.Log(1.0+tgtPR))
	prBenefit := srcLog / tgtLog
	// Penalize thin content
	minWC := math.Min(float64(srcWC), float64(tgtWC))
	contentQuality := math.Min(1.0, minWC/500.0)
	return relevance * prBenefit * contentQuality
}

// classifyPair returns "cannibalization" for very similar pages, "opportunity" otherwise.
func classifyPair(similarity float64) string {
	if similarity >= 0.85 {
		return "cannibalization"
	}
	return "opportunity"
}

// ComputeOpportunitiesOptions controls the interlinking analysis.
type ComputeOpportunitiesOptions struct {
	SessionID           string
	Method              string  // "tfidf"
	SimilarityThreshold float64 // default 0.3
	MaxOpportunities    int     // default 1000
	MinCommonTerms      int     // default 3
}

// OpportunityStore is the subset of storage needed by the opportunity finder.
type OpportunityStore interface {
	StreamPagesHTML(ctx context.Context, sessionID string) (<-chan storage.PageHTMLRow, error)
	LoadInternalLinkSet(ctx context.Context, sessionID string) (map[[2]string]struct{}, error)
	LoadPageMetadata(ctx context.Context, sessionID string) (map[string]storage.PageMetadata, error)
	DeleteInterlinkingOpportunities(ctx context.Context, sessionID string) error
	InsertInterlinkingOpportunities(ctx context.Context, sessionID string, opps []storage.InterlinkingOpportunity) error
}

// ComputeOpportunities runs the full interlinking analysis pipeline:
// 1. Stream HTML → extract content → build TF-IDF corpus
// 2. Find similar pairs above threshold
// 3. Filter out pairs that already have an internal link
// 4. Enrich with metadata and store results
func ComputeOpportunities(ctx context.Context, store OpportunityStore, opts ComputeOpportunitiesOptions) error {
	start := time.Now()

	if opts.SimilarityThreshold == 0 {
		opts.SimilarityThreshold = 0.3
	}
	if opts.MaxOpportunities == 0 {
		opts.MaxOpportunities = 1000
	}
	if opts.MinCommonTerms == 0 {
		opts.MinCommonTerms = 3
	}
	if opts.Method == "" {
		opts.Method = "tfidf"
	}

	// Load page metadata (title, lang, pagerank, word_count, canonical)
	applog.Info("interlinking", "Loading page metadata...")
	pageMeta, err := store.LoadPageMetadata(ctx, opts.SessionID)
	if err != nil {
		return err
	}
	applog.Infof("interlinking", "Loaded metadata for %d pages", len(pageMeta))

	// Deduplicate DUST: skip non-canonical variants and tracking-param duplicates
	skipURLs := DeduplicatePages(pageMeta)
	if len(skipURLs) > 0 {
		applog.Infof("interlinking", "Deduplicating: %d DUST URLs skipped (canonical/tracking params)", len(skipURLs))
	}

	// Stream HTML and build corpus (filtering out DUST)
	applog.Info("interlinking", "Building TF-IDF corpus...")
	htmlCh, err := store.StreamPagesHTML(ctx, opts.SessionID)
	if err != nil {
		return err
	}

	// Wrap channel to filter out DUST URLs and non-200 pages (redirects, errors).
	// pageMeta only contains status_code=200 pages, so this also excludes redirects.
	filteredCh := make(chan storage.PageHTMLRow, 64)
	go func() {
		defer close(filteredCh)
		for row := range htmlCh {
			if skipURLs[row.URL] {
				continue
			}
			if _, ok := pageMeta[row.URL]; !ok {
				continue // redirect or error page — not in metadata
			}
			filteredCh <- row
		}
	}()

	corpus, err := BuildCorpus(filteredCh, pageMeta)
	if err != nil {
		return err
	}
	applog.Infof("interlinking", "Corpus built: %d documents, %d vocab terms in %s",
		len(corpus.Docs), len(corpus.Vocab), time.Since(start))

	if len(corpus.Docs) < 2 {
		applog.Info("interlinking", "Not enough documents for similarity analysis")
		return nil
	}

	// Find similar pairs
	applog.Info("interlinking", "Finding similar pairs...")
	pairs := FindSimilarPairs(corpus, opts.SimilarityThreshold, opts.MinCommonTerms)
	applog.Infof("interlinking", "Found %d similar pairs above threshold %.2f", len(pairs), opts.SimilarityThreshold)

	if len(pairs) == 0 {
		return nil
	}

	// Load existing internal links to filter out already-linked pairs
	applog.Info("interlinking", "Loading existing internal links...")
	linkSet, err := store.LoadInternalLinkSet(ctx, opts.SessionID)
	if err != nil {
		return err
	}
	applog.Infof("interlinking", "Loaded %d existing internal links", len(linkSet))

	// Filter: remove pairs where a link already exists in either direction
	var filtered []SimilarPair
	for _, p := range pairs {
		srcURL := corpus.Docs[p.SourceIdx].URL
		tgtURL := corpus.Docs[p.TargetIdx].URL
		if _, ok := linkSet[[2]string{srcURL, tgtURL}]; ok {
			continue
		}
		if _, ok := linkSet[[2]string{tgtURL, srcURL}]; ok {
			continue
		}
		filtered = append(filtered, p)
	}
	applog.Infof("interlinking", "%d opportunities after filtering existing links", len(filtered))

	// Classify each pair and compute opportunity score
	type scoredPair struct {
		pair     SimilarPair
		score    float64
		category string
	}
	var opportunities, cannibalization []scoredPair
	for _, p := range filtered {
		src := corpus.Docs[p.SourceIdx]
		tgt := corpus.Docs[p.TargetIdx]
		cat := classifyPair(p.Similarity)
		score := computeOpportunityScore(p.Similarity, src.PageRank, tgt.PageRank, src.WordCount, tgt.WordCount)
		sp := scoredPair{pair: p, score: score, category: cat}
		if cat == "cannibalization" {
			cannibalization = append(cannibalization, sp)
		} else {
			opportunities = append(opportunities, sp)
		}
	}

	// Sort opportunities by score DESC, cannibalization by similarity DESC
	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].score > opportunities[j].score
	})
	sort.Slice(cannibalization, func(i, j int) bool {
		return cannibalization[i].pair.Similarity > cannibalization[j].pair.Similarity
	})

	// Merge and cap at MaxOpportunities
	all := append(opportunities, cannibalization...)
	if len(all) > opts.MaxOpportunities {
		all = all[:opts.MaxOpportunities]
	}

	// Build storage rows
	opps := make([]storage.InterlinkingOpportunity, len(all))
	for i, sp := range all {
		src := corpus.Docs[sp.pair.SourceIdx]
		tgt := corpus.Docs[sp.pair.TargetIdx]
		opps[i] = storage.InterlinkingOpportunity{
			CrawlSessionID:   opts.SessionID,
			SourceURL:        src.URL,
			TargetURL:        tgt.URL,
			Similarity:       sp.pair.Similarity,
			Method:           opts.Method,
			SourceTitle:      src.Title,
			TargetTitle:      tgt.Title,
			SourcePageRank:   src.PageRank,
			TargetPageRank:   tgt.PageRank,
			SourceWordCount:  src.WordCount,
			TargetWordCount:  tgt.WordCount,
			OpportunityScore: sp.score,
			Category:         sp.category,
		}
	}

	// Delete previous results and insert new ones
	if err := store.DeleteInterlinkingOpportunities(ctx, opts.SessionID); err != nil {
		return err
	}
	if err := store.InsertInterlinkingOpportunities(ctx, opts.SessionID, opps); err != nil {
		return err
	}

	applog.Infof("interlinking", "Stored %d interlinking opportunities in %s", len(opps), time.Since(start))
	return nil
}
