package interlinking

import (
	"context"
	"sort"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

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

	// Sort by similarity descending and cap at max
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Similarity > filtered[j].Similarity
	})
	if len(filtered) > opts.MaxOpportunities {
		filtered = filtered[:opts.MaxOpportunities]
	}

	// Build storage rows
	opps := make([]storage.InterlinkingOpportunity, len(filtered))
	for i, p := range filtered {
		src := corpus.Docs[p.SourceIdx]
		tgt := corpus.Docs[p.TargetIdx]
		opps[i] = storage.InterlinkingOpportunity{
			CrawlSessionID:  opts.SessionID,
			SourceURL:       src.URL,
			TargetURL:       tgt.URL,
			Similarity:      p.Similarity,
			Method:          opts.Method,
			SourceTitle:     src.Title,
			TargetTitle:     tgt.Title,
			SourcePageRank:  src.PageRank,
			TargetPageRank:  tgt.PageRank,
			SourceWordCount: src.WordCount,
			TargetWordCount: tgt.WordCount,
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
