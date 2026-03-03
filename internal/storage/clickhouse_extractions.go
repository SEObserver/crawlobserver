package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/SEObserver/crawlobserver/internal/extraction"
)

// InsertExtractions batch inserts extraction rows.
func (s *Store) InsertExtractions(ctx context.Context, rows []extraction.ExtractionRow) error {
	if len(rows) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.extractions (
			crawl_session_id, url, extractor_name, value, crawled_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing extractions batch: %w", err)
	}

	for _, r := range rows {
		if err := batch.Append(
			r.CrawlSessionID, r.URL, r.ExtractorName, r.Value, r.CrawledAt,
		); err != nil {
			return fmt.Errorf("appending extraction row: %w", err)
		}
	}

	return batch.Send()
}

// ExtractionRow for query results.
type ExtractionQueryRow struct {
	URL           string
	ExtractorName string
	Value         string
}

// GetExtractions retrieves extraction results for a session, pivoted by extractor name.
func (s *Store) GetExtractions(ctx context.Context, sessionID string, limit, offset int) (*extraction.ExtractionResult, error) {
	// Get distinct extractor names
	nameRows, err := s.conn.Query(ctx, `
		SELECT DISTINCT extractor_name
		FROM crawlobserver.extractions
		WHERE crawl_session_id = ?
		ORDER BY extractor_name`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying extractor names: %w", err)
	}

	var extractorNames []string
	for nameRows.Next() {
		var name string
		if err := nameRows.Scan(&name); err != nil {
			return nil, err
		}
		extractorNames = append(extractorNames, name)
	}
	nameRows.Close()

	// Get total distinct URLs
	var totalPages uint64
	if err := s.conn.QueryRow(ctx, `
		SELECT COUNT(DISTINCT url)
		FROM crawlobserver.extractions
		WHERE crawl_session_id = ?`, sessionID).Scan(&totalPages); err != nil {
		return nil, fmt.Errorf("counting extraction pages: %w", err)
	}

	// Get paginated URLs
	urlRows, err := s.conn.Query(ctx, `
		SELECT DISTINCT url
		FROM crawlobserver.extractions
		WHERE crawl_session_id = ?
		ORDER BY url
		LIMIT ? OFFSET ?`, sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying extraction URLs: %w", err)
	}

	var urls []string
	for urlRows.Next() {
		var u string
		if err := urlRows.Scan(&u); err != nil {
			return nil, err
		}
		urls = append(urls, u)
	}
	urlRows.Close()

	if len(urls) == 0 {
		return &extraction.ExtractionResult{
			SessionID:  sessionID,
			TotalPages: 0,
			Pages:      []extraction.PageExtraction{},
		}, nil
	}

	// Get extraction values for these URLs
	dataRows, err := s.conn.Query(ctx, `
		SELECT url, extractor_name, value
		FROM crawlobserver.extractions
		WHERE crawl_session_id = ? AND url IN (?)
		ORDER BY url, extractor_name`, sessionID, urls)
	if err != nil {
		return nil, fmt.Errorf("querying extraction data: %w", err)
	}

	pageMap := make(map[string]map[string]string)
	for dataRows.Next() {
		var url, name, value string
		if err := dataRows.Scan(&url, &name, &value); err != nil {
			return nil, err
		}
		if pageMap[url] == nil {
			pageMap[url] = make(map[string]string)
		}
		pageMap[url][name] = value
	}
	dataRows.Close()

	// Build result preserving URL order
	var pages []extraction.PageExtraction
	for _, u := range urls {
		pages = append(pages, extraction.PageExtraction{
			URL:    u,
			Values: pageMap[u],
		})
	}

	// Build minimal Extractor list for result
	var extractors []extraction.Extractor
	for _, name := range extractorNames {
		extractors = append(extractors, extraction.Extractor{Name: name})
	}

	return &extraction.ExtractionResult{
		SessionID:  sessionID,
		Extractors: extractors,
		TotalPages: int(totalPages),
		Pages:      pages,
	}, nil
}

// DeleteExtractions removes all extraction data for a session.
func (s *Store) DeleteExtractions(ctx context.Context, sessionID string) error {
	return s.conn.Exec(ctx, `
		ALTER TABLE crawlobserver.extractions
		DELETE WHERE crawl_session_id = ?`, sessionID)
}

// StreamPagesHTMLForExtraction is an alias to StreamPagesHTML for clarity in extraction context.
func (s *Store) StreamPagesHTMLForExtraction(ctx context.Context, sessionID string) (<-chan PageHTMLRow, error) {
	return s.StreamPagesHTML(ctx, sessionID)
}

// PageHTMLRowForExtraction is a helper to check if stored HTML exists.
func (s *Store) HasStoredHTML(ctx context.Context, sessionID string) (bool, error) {
	var count uint64
	err := s.conn.QueryRow(ctx, `
		SELECT count()
		FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND body_html != ''
		LIMIT 1`, sessionID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// RunExtractionsPostCrawl runs extractors against stored HTML and inserts results.
func (s *Store) RunExtractionsPostCrawl(ctx context.Context, sessionID string, extractors []extraction.Extractor) (*extraction.ExtractionResult, error) {
	// Delete existing extractions for this session
	if err := s.DeleteExtractions(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("cleaning old extractions: %w", err)
	}

	ch, err := s.StreamPagesHTML(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("streaming pages HTML: %w", err)
	}

	now := time.Now()
	var allRows []extraction.ExtractionRow
	pageCount := 0

	for row := range ch {
		if row.HTML == "" {
			continue
		}
		rows := extraction.RunExtractors([]byte(row.HTML), row.URL, sessionID, extractors, now)
		allRows = append(allRows, rows...)
		pageCount++

		// Batch insert every 1000 pages
		if len(allRows) >= 5000 {
			if err := s.InsertExtractions(ctx, allRows); err != nil {
				return nil, fmt.Errorf("inserting extractions: %w", err)
			}
			allRows = allRows[:0]
		}
	}

	// Insert remaining
	if len(allRows) > 0 {
		if err := s.InsertExtractions(ctx, allRows); err != nil {
			return nil, fmt.Errorf("inserting extractions: %w", err)
		}
	}

	// Return results
	return s.GetExtractions(ctx, sessionID, 100, 0)
}
