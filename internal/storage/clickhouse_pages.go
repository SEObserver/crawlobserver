package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/google/uuid"
)

// InsertPages batch inserts page rows.
func (s *Store) InsertPages(ctx context.Context, pages []PageRow) error {
	if len(pages) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.pages (
			crawl_session_id, url, final_url, status_code, content_type,
			title, title_length, canonical, canonical_is_self, is_indexable, index_reason,
			meta_robots, meta_description, meta_desc_length, meta_keywords,
			h1, h2, h3, h4, h5, h6,
			word_count, internal_links_out, external_links_out,
			images_count, images_no_alt, hreflang,
			lang, og_title, og_description, og_image, schema_types,
			headers, redirect_chain, body_size, fetch_duration_ms,
			content_encoding, x_robots_tag,
			error, depth, found_on, pagerank, content_hash, body_html, body_truncated, crawled_at,
			js_rendered, js_render_duration_ms, js_render_error,
			rendered_title, rendered_meta_description, rendered_h1,
			rendered_word_count, rendered_links_count, rendered_images_count,
			rendered_canonical, rendered_meta_robots, rendered_schema_types,
			rendered_body_html,
			js_changed_title, js_changed_description, js_changed_h1,
			js_changed_canonical, js_changed_content,
			js_added_links, js_added_images, js_added_schema
		)`)
	if err != nil {
		return fmt.Errorf("preparing pages batch: %w", err)
	}

	for _, p := range pages {
		// Convert redirect chain to ClickHouse tuple format
		chain := make([][]interface{}, len(p.RedirectChain))
		for i, hop := range p.RedirectChain {
			chain[i] = []interface{}{hop.URL, hop.StatusCode}
		}

		// Convert hreflang to ClickHouse tuple format
		hreflang := make([][]interface{}, len(p.Hreflang))
		for i, h := range p.Hreflang {
			hreflang[i] = []interface{}{h.Lang, h.URL}
		}

		if err := batch.Append(
			p.CrawlSessionID, p.URL, p.FinalURL, p.StatusCode, p.ContentType,
			p.Title, p.TitleLength, p.Canonical, p.CanonicalIsSelf, p.IsIndexable, p.IndexReason,
			p.MetaRobots, p.MetaDescription, p.MetaDescLength, p.MetaKeywords,
			p.H1, p.H2, p.H3, p.H4, p.H5, p.H6,
			p.WordCount, p.InternalLinksOut, p.ExternalLinksOut,
			p.ImagesCount, p.ImagesNoAlt, hreflang,
			p.Lang, p.OGTitle, p.OGDescription, p.OGImage, p.SchemaTypes,
			p.Headers, chain, p.BodySize, p.FetchDurationMs,
			p.ContentEncoding, p.XRobotsTag,
			p.Error, p.Depth, p.FoundOn, p.PageRank, p.ContentHash, p.BodyHTML, p.BodyTruncated, p.CrawledAt,
			p.JSRendered, p.JSRenderDurationMs, p.JSRenderError,
			p.RenderedTitle, p.RenderedMetaDescription, p.RenderedH1,
			p.RenderedWordCount, p.RenderedLinksCount, p.RenderedImagesCount,
			p.RenderedCanonical, p.RenderedMetaRobots, p.RenderedSchemaTypes,
			p.RenderedBodyHTML,
			p.JSChangedTitle, p.JSChangedDescription, p.JSChangedH1,
			p.JSChangedCanonical, p.JSChangedContent,
			p.JSAddedLinks, p.JSAddedImages, p.JSAddedSchema,
		); err != nil {
			return fmt.Errorf("appending page row: %w", err)
		}
	}

	return batch.Send()
}

// CountPages returns the total number of pages for a session.
func (s *Store) CountPages(ctx context.Context, sessionID string) (uint64, error) {
	var count uint64
	err := s.conn.QueryRow(ctx, `SELECT count() FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID).Scan(&count)
	return count, err
}

// ListPages retrieves pages for a session with pagination and optional filters.
func (s *Store) ListPages(ctx context.Context, sessionID string, limit, offset int, filters []ParsedFilter, sort *SortParam) ([]PageRow, error) {
	query := `
		SELECT crawl_session_id, url, final_url, status_code, content_type,
			title, title_length, canonical, canonical_is_self, is_indexable, index_reason,
			meta_robots, meta_description, meta_desc_length, meta_keywords,
			h1, h2, h3, h4, h5, h6,
			word_count, internal_links_out, external_links_out,
			images_count, images_no_alt,
			lang, og_title, og_description, og_image, schema_types,
			body_size, fetch_duration_ms, content_encoding, x_robots_tag,
			error, depth, found_on, pagerank, crawled_at,
			js_rendered, js_render_duration_ms, js_render_error,
			js_changed_title, js_changed_description, js_changed_h1,
			js_changed_canonical, js_changed_content,
			js_added_links, js_added_images, js_added_schema
		FROM crawlobserver.pages
		WHERE crawl_session_id = ?`
	args := []interface{}{sessionID}

	whereExtra, filterArgs, err := BuildWhereClause(filters)
	if err != nil {
		return nil, fmt.Errorf("building filter clause: %w", err)
	}
	if whereExtra != "" {
		query += " AND " + whereExtra
		args = append(args, filterArgs...)
	}

	query += BuildOrderByClause(sort, "crawled_at DESC") + ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying pages: %w", err)
	}
	defer rows.Close()

	var pages []PageRow
	for rows.Next() {
		var p PageRow
		if err := rows.Scan(
			&p.CrawlSessionID, &p.URL, &p.FinalURL, &p.StatusCode, &p.ContentType,
			&p.Title, &p.TitleLength, &p.Canonical, &p.CanonicalIsSelf, &p.IsIndexable, &p.IndexReason,
			&p.MetaRobots, &p.MetaDescription, &p.MetaDescLength, &p.MetaKeywords,
			&p.H1, &p.H2, &p.H3, &p.H4, &p.H5, &p.H6,
			&p.WordCount, &p.InternalLinksOut, &p.ExternalLinksOut,
			&p.ImagesCount, &p.ImagesNoAlt,
			&p.Lang, &p.OGTitle, &p.OGDescription, &p.OGImage, &p.SchemaTypes,
			&p.BodySize, &p.FetchDurationMs, &p.ContentEncoding, &p.XRobotsTag,
			&p.Error, &p.Depth, &p.FoundOn, &p.PageRank, &p.CrawledAt,
			&p.JSRendered, &p.JSRenderDurationMs, &p.JSRenderError,
			&p.JSChangedTitle, &p.JSChangedDescription, &p.JSChangedH1,
			&p.JSChangedCanonical, &p.JSChangedContent,
			&p.JSAddedLinks, &p.JSAddedImages, &p.JSAddedSchema,
		); err != nil {
			return nil, fmt.Errorf("scanning page: %w", err)
		}
		pages = append(pages, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pages: %w", err)
	}
	return pages, nil
}

// GetPage retrieves all fields for a single page (excluding body_html).
func (s *Store) GetPage(ctx context.Context, sessionID, url string) (*PageRow, error) {
	var p PageRow
	var redirectChain []map[string]interface{}
	var hreflang []map[string]interface{}

	row := s.conn.QueryRow(ctx, `
		SELECT crawl_session_id, url, final_url, status_code, content_type,
			title, title_length, canonical, canonical_is_self, is_indexable, index_reason,
			meta_robots, meta_description, meta_desc_length, meta_keywords,
			h1, h2, h3, h4, h5, h6,
			word_count, internal_links_out, external_links_out,
			images_count, images_no_alt, hreflang,
			lang, og_title, og_description, og_image, schema_types,
			headers, redirect_chain, body_size, fetch_duration_ms,
			content_encoding, x_robots_tag,
			error, depth, found_on, pagerank, crawled_at,
			js_rendered, js_render_duration_ms, js_render_error,
			rendered_title, rendered_meta_description, rendered_h1,
			rendered_word_count, rendered_links_count, rendered_images_count,
			rendered_canonical, rendered_meta_robots, rendered_schema_types,
			js_changed_title, js_changed_description, js_changed_h1,
			js_changed_canonical, js_changed_content,
			js_added_links, js_added_images, js_added_schema
		FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND url = ?
		LIMIT 1`, sessionID, url)

	if err := row.Scan(
		&p.CrawlSessionID, &p.URL, &p.FinalURL, &p.StatusCode, &p.ContentType,
		&p.Title, &p.TitleLength, &p.Canonical, &p.CanonicalIsSelf, &p.IsIndexable, &p.IndexReason,
		&p.MetaRobots, &p.MetaDescription, &p.MetaDescLength, &p.MetaKeywords,
		&p.H1, &p.H2, &p.H3, &p.H4, &p.H5, &p.H6,
		&p.WordCount, &p.InternalLinksOut, &p.ExternalLinksOut,
		&p.ImagesCount, &p.ImagesNoAlt, &hreflang,
		&p.Lang, &p.OGTitle, &p.OGDescription, &p.OGImage, &p.SchemaTypes,
		&p.Headers, &redirectChain, &p.BodySize, &p.FetchDurationMs,
		&p.ContentEncoding, &p.XRobotsTag,
		&p.Error, &p.Depth, &p.FoundOn, &p.PageRank, &p.CrawledAt,
		&p.JSRendered, &p.JSRenderDurationMs, &p.JSRenderError,
		&p.RenderedTitle, &p.RenderedMetaDescription, &p.RenderedH1,
		&p.RenderedWordCount, &p.RenderedLinksCount, &p.RenderedImagesCount,
		&p.RenderedCanonical, &p.RenderedMetaRobots, &p.RenderedSchemaTypes,
		&p.JSChangedTitle, &p.JSChangedDescription, &p.JSChangedH1,
		&p.JSChangedCanonical, &p.JSChangedContent,
		&p.JSAddedLinks, &p.JSAddedImages, &p.JSAddedSchema,
	); err != nil {
		return nil, fmt.Errorf("querying page detail: %w", err)
	}

	for _, m := range redirectChain {
		hop := RedirectHopRow{}
		if v, ok := m["url"]; ok {
			hop.URL, _ = v.(string)
		}
		if v, ok := m["status_code"]; ok {
			hop.StatusCode, _ = v.(uint16)
		}
		p.RedirectChain = append(p.RedirectChain, hop)
	}
	for _, m := range hreflang {
		h := HreflangRow{}
		if v, ok := m["lang"]; ok {
			h.Lang, _ = v.(string)
		}
		if v, ok := m["url"]; ok {
			h.URL, _ = v.(string)
		}
		p.Hreflang = append(p.Hreflang, h)
	}

	return &p, nil
}

// GetPageHTML retrieves the raw HTML for a specific page.
func (s *Store) GetPageHTML(ctx context.Context, sessionID, url string) (string, error) {
	var html string
	row := s.conn.QueryRow(ctx, `
		SELECT body_html FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND url = ? LIMIT 1`, sessionID, url)
	if err := row.Scan(&html); err != nil {
		return "", fmt.Errorf("querying page HTML: %w", err)
	}
	return html, nil
}

// PageLinksResult holds outbound links, inbound links (paginated), and counts.
type PageLinksResult struct {
	OutLinks      []LinkRow `json:"out_links"`
	InLinks       []LinkRow `json:"in_links"`
	OutLinksCount uint64    `json:"out_links_count"`
	InLinksCount  uint64    `json:"in_links_count"`
}

// GetPageLinks retrieves outbound and inbound links for a URL with pagination.
func (s *Store) GetPageLinks(ctx context.Context, sessionID, url string, outLimit, outOffset, inLimit, inOffset int) (*PageLinksResult, error) {
	result := &PageLinksResult{}

	// Counts
	countRow := s.conn.QueryRow(ctx, `
		SELECT countIf(source_url = ?), countIf(target_url = ?)
		FROM crawlobserver.links
		WHERE crawl_session_id = ? AND (source_url = ? OR target_url = ?)`,
		url, url, sessionID, url, url)
	if err := countRow.Scan(&result.OutLinksCount, &result.InLinksCount); err != nil {
		return nil, fmt.Errorf("querying link counts: %w", err)
	}

	// Outbound links (paginated)
	outRows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM crawlobserver.links
		WHERE crawl_session_id = ? AND source_url = ?
		ORDER BY target_url
		LIMIT ? OFFSET ?`, sessionID, url, outLimit, outOffset)
	if err != nil {
		return nil, fmt.Errorf("querying outbound links: %w", err)
	}
	defer outRows.Close()
	for outRows.Next() {
		var l LinkRow
		if err := outRows.Scan(&l.CrawlSessionID, &l.SourceURL, &l.TargetURL, &l.AnchorText,
			&l.Rel, &l.IsInternal, &l.Tag, &l.CrawledAt); err != nil {
			return nil, fmt.Errorf("scanning outbound link: %w", err)
		}
		result.OutLinks = append(result.OutLinks, l)
	}
	if err := outRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating outbound links: %w", err)
	}

	// Inbound links (paginated)
	inRows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM crawlobserver.links
		WHERE crawl_session_id = ? AND target_url = ?
		ORDER BY source_url
		LIMIT ? OFFSET ?`, sessionID, url, inLimit, inOffset)
	if err != nil {
		return nil, fmt.Errorf("querying inbound links: %w", err)
	}
	defer inRows.Close()
	for inRows.Next() {
		var l LinkRow
		if err := inRows.Scan(&l.CrawlSessionID, &l.SourceURL, &l.TargetURL, &l.AnchorText,
			&l.Rel, &l.IsInternal, &l.Tag, &l.CrawledAt); err != nil {
			return nil, fmt.Errorf("scanning inbound link: %w", err)
		}
		result.InLinks = append(result.InLinks, l)
	}
	if err := inRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating inbound links: %w", err)
	}

	return result, nil
}

// UncrawledURLs returns internal link targets that were discovered but not crawled in a session.
func (s *Store) UncrawledURLs(ctx context.Context, sessionID string) ([]string, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT DISTINCT target_url
		FROM crawlobserver.links
		WHERE crawl_session_id = ? AND is_internal = true
		  AND target_url NOT IN (
		    SELECT url FROM crawlobserver.pages WHERE crawl_session_id = ?
		  )
	`, sessionID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying uncrawled URLs: %w", err)
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		urls = append(urls, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating uncrawled URLs: %w", err)
	}
	return urls, nil
}

// StreamCrawledURLs streams all URLs already crawled in a session, calling fn
// for each URL. This avoids loading the entire URL list into memory (which can
// cause OOM on large sites with 1M+ pages). Returns the number of URLs streamed.
func (s *Store) StreamCrawledURLs(ctx context.Context, sessionID string, fn func(string)) (int, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT url FROM crawlobserver.pages WHERE crawl_session_id = ?
	`, sessionID)
	if err != nil {
		return 0, fmt.Errorf("querying crawled URLs: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return count, err
		}
		fn(u)
		count++
	}
	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("iterating crawled URLs: %w", err)
	}
	return count, nil
}

// FailedURLs returns URLs with status_code = 0 (fetch errors) for a session.
func (s *Store) FailedURLs(ctx context.Context, sessionID string) ([]string, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT url FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND status_code = 0`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying failed URLs: %w", err)
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		urls = append(urls, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating failed URLs: %w", err)
	}
	return urls, nil
}

// URLsByStatus returns URLs with a specific status code for a session.
func (s *Store) URLsByStatus(ctx context.Context, sessionID string, statusCode int) ([]string, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT url FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND status_code = ?`, sessionID, statusCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var urls []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		urls = append(urls, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating URLs by status: %w", err)
	}
	return urls, nil
}

// DeleteFailedPages removes pages with status_code = 0 for a session so they can be re-crawled.
func (s *Store) DeleteFailedPages(ctx context.Context, sessionID string) (int, error) {
	// Count first
	var cnt uint64
	row := s.conn.QueryRow(ctx, `
		SELECT count() FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND status_code = 0`, sessionID)
	if err := row.Scan(&cnt); err != nil {
		return 0, fmt.Errorf("counting failed pages: %w", err)
	}

	if cnt == 0 {
		return 0, nil
	}

	// Delete them
	if err := s.conn.Exec(ctx, `
		ALTER TABLE crawlobserver.pages DELETE
		WHERE crawl_session_id = ? AND status_code = 0`, sessionID); err != nil {
		return 0, fmt.Errorf("deleting failed pages: %w", err)
	}

	return int(cnt), nil
}

// DeletePagesByStatus deletes pages with a specific status code and returns the count deleted.
func (s *Store) DeletePagesByStatus(ctx context.Context, sessionID string, statusCode int) (int, error) {
	var cnt uint64
	row := s.conn.QueryRow(ctx, `
		SELECT count() FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND status_code = ?`, sessionID, statusCode)
	if err := row.Scan(&cnt); err != nil {
		return 0, fmt.Errorf("counting pages with status %d: %w", statusCode, err)
	}
	if cnt == 0 {
		return 0, nil
	}
	if err := s.conn.Exec(ctx, `
		ALTER TABLE crawlobserver.pages DELETE
		WHERE crawl_session_id = ? AND status_code = ?`, sessionID, statusCode); err != nil {
		return 0, fmt.Errorf("deleting pages with status %d: %w", statusCode, err)
	}
	return int(cnt), nil
}

// isValidUUID checks whether s is a valid UUID string.
func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// ComputePageRank computes internal PageRank for all pages in a session.
// Uses uint32 IDs for memory efficiency and iterative power method.
// URL→ID mapping is done in ClickHouse via a Join-engine temp table,
// so only uint32 pairs are transferred for the link graph.
func (s *Store) ComputePageRank(ctx context.Context, sessionID string) error {
	start := time.Now()
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}

	// 1. Load all crawled URLs and assign numeric IDs
	urlRows, err := s.conn.Query(ctx, `
		SELECT url FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("querying URLs: %w", err)
	}
	defer urlRows.Close()

	urlToID := make(map[string]uint32)
	idToURL := make([]string, 0)
	for urlRows.Next() {
		var u string
		if err := urlRows.Scan(&u); err != nil {
			return fmt.Errorf("scanning URL: %w", err)
		}
		urlToID[u] = uint32(len(idToURL))
		idToURL = append(idToURL, u)
	}
	if err := urlRows.Err(); err != nil {
		return fmt.Errorf("iterating URLs: %w", err)
	}

	n := uint32(len(idToURL))
	if n == 0 {
		return nil
	}

	applog.Infof("storage", "PageRank: loaded %d URLs in %s", n, time.Since(start))

	// 2. Build URL→ID temp table in ClickHouse for server-side ID resolution
	idTable := fmt.Sprintf("crawlobserver.tmp_urlids_%s", strings.ReplaceAll(sessionID, "-", ""))
	if err := s.conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", idTable)); err != nil {
		applog.Warnf("storage", "pre-cleanup temp table %s: %v", idTable, err)
	}
	if err := s.conn.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (url String, id UInt32) ENGINE = Join(ANY, LEFT, url)", idTable)); err != nil {
		return fmt.Errorf("creating URL ID table: %w", err)
	}
	defer func() {
		if err := s.conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", idTable)); err != nil {
			applog.Warnf("storage", "cleanup temp table %s: %v", idTable, err)
		}
	}()

	// Insert URL→ID mappings
	const idChunk = 10000
	for i := 0; i < int(n); i += idChunk {
		end := i + idChunk
		if end > int(n) {
			end = int(n)
		}
		batch, err := s.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s (url, id)", idTable))
		if err != nil {
			return fmt.Errorf("preparing URL ID batch: %w", err)
		}
		for j := i; j < end; j++ {
			if err := batch.Append(idToURL[j], uint32(j)); err != nil {
				return fmt.Errorf("appending URL ID: %w", err)
			}
		}
		if err := batch.Send(); err != nil {
			return fmt.Errorf("sending URL ID batch: %w", err)
		}
	}

	// Free the Go-side map — no longer needed
	urlToID = nil

	// 3. Load deduplicated internal links as uint32 ID pairs (resolved in ClickHouse)
	t2 := time.Now()
	linkRows, err := s.conn.Query(ctx, fmt.Sprintf(`
		SELECT
			joinGet('%s', 'id', source_url) AS src_id,
			joinGet('%s', 'id', target_url) AS tgt_id
		FROM crawlobserver.links
		WHERE crawl_session_id = ? AND is_internal = true
			AND source_url IN (SELECT url FROM %s)
			AND target_url IN (SELECT url FROM %s)
		GROUP BY src_id, tgt_id
		HAVING src_id != tgt_id`,
		idTable, idTable, idTable, idTable), sessionID)
	if err != nil {
		return fmt.Errorf("querying links: %w", err)
	}
	defer linkRows.Close()

	outLinks := make([][]uint32, n)
	var edgeCount int
	for linkRows.Next() {
		var srcID, tgtID uint32
		if err := linkRows.Scan(&srcID, &tgtID); err != nil {
			return fmt.Errorf("scanning link IDs: %w", err)
		}
		outLinks[srcID] = append(outLinks[srcID], tgtID)
		edgeCount++
	}
	if err := linkRows.Err(); err != nil {
		return fmt.Errorf("iterating link IDs: %w", err)
	}

	applog.Infof("storage", "PageRank: loaded %d unique edges in %s", edgeCount, time.Since(t2))

	// 4. PageRank iteration (power method)
	const damping = 0.85
	const iterations = 20
	const tolerance = 1e-6

	rank := make([]float64, n)
	newRank := make([]float64, n)
	initial := 1.0 / float64(n)
	for i := range rank {
		rank[i] = initial
	}

	for iter := 0; iter < iterations; iter++ {
		// Reset newRank with teleportation base
		base := (1.0 - damping) / float64(n)
		for i := range newRank {
			newRank[i] = base
		}

		// Accumulate dangling node mass (nodes with no outgoing links)
		var danglingSum float64
		for i := uint32(0); i < n; i++ {
			if len(outLinks[i]) == 0 {
				danglingSum += rank[i]
			}
		}
		danglingContrib := damping * danglingSum / float64(n)
		for i := range newRank {
			newRank[i] += danglingContrib
		}

		// Distribute rank through links
		for src := uint32(0); src < n; src++ {
			if len(outLinks[src]) == 0 {
				continue
			}
			contrib := damping * rank[src] / float64(len(outLinks[src]))
			for _, tgt := range outLinks[src] {
				newRank[tgt] += contrib
			}
		}

		// Check convergence
		var diff float64
		for i := range rank {
			d := newRank[i] - rank[i]
			if d < 0 {
				d = -d
			}
			diff += d
		}

		rank, newRank = newRank, rank

		if diff < tolerance {
			applog.Infof("storage", "PageRank converged after %d iterations (diff=%.2e)", iter+1, diff)
			break
		}
	}

	// 5. Normalize to 0-100 scale
	var maxRank float64
	for _, r := range rank {
		if r > maxRank {
			maxRank = r
		}
	}
	if maxRank > 0 {
		for i := range rank {
			rank[i] = (rank[i] / maxRank) * 100.0
		}
	}

	// 6. Write back via temp table + single mutation (avoids 100s of mutations)
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}

	tmpTable := fmt.Sprintf("crawlobserver.tmp_pagerank_%s", strings.ReplaceAll(sessionID, "-", ""))
	if err := s.conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tmpTable)); err != nil {
		return fmt.Errorf("dropping old temp pagerank table: %w", err)
	}
	if err := s.conn.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (page_url String, new_pagerank Float64) ENGINE = Join(ANY, LEFT, page_url)", tmpTable)); err != nil {
		return fmt.Errorf("creating temp pagerank table: %w", err)
	}
	defer func() {
		if err := s.conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tmpTable)); err != nil {
			applog.Warnf("storage", "cleanup temp table %s: %v", tmpTable, err)
		}
	}()

	const chunkSize = 5000
	for i := 0; i < int(n); i += chunkSize {
		end := i + chunkSize
		if end > int(n) {
			end = int(n)
		}
		batch, err := s.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s (page_url, new_pagerank)", tmpTable))
		if err != nil {
			return fmt.Errorf("preparing pagerank batch: %w", err)
		}
		for j := i; j < end; j++ {
			if err := batch.Append(idToURL[j], rank[j]); err != nil {
				return fmt.Errorf("appending to pagerank batch: %w", err)
			}
		}
		if err := batch.Send(); err != nil {
			return fmt.Errorf("sending pagerank batch: %w", err)
		}
	}

	// Use joinGet to look up pagerank from the Join-engine temp table.
	// Single mutation, no data copy, no correlated subquery.
	query := fmt.Sprintf(`ALTER TABLE crawlobserver.pages UPDATE
		pagerank = joinGet('%s', 'new_pagerank', url)
		WHERE crawl_session_id = ?
		SETTINGS mutations_sync = 1`,
		tmpTable)
	if err := s.conn.Exec(ctx, query, sessionID); err != nil {
		return fmt.Errorf("updating pagerank via joinGet: %w", err)
	}

	applog.Infof("storage", "ComputePageRank: computed for %d pages in session %s in %s", n, sessionID, time.Since(start))
	return nil
}

// RecomputeDepths runs a BFS from seed URLs and updates depth/found_on in the pages table.
// BFSResult holds the output of a BFS depth computation.
type BFSResult struct {
	Depths  map[string]uint16
	FoundOn map[string]string
}

// ComputeBFSDepths runs BFS from seedURLs over the link graph and returns
// the depth and found_on for every URL in crawledSet.
// Seeds get depth 0. Orphans (unreachable) get maxDepth+1.
func ComputeBFSDepths(seedURLs []string, crawledSet map[string]bool, adj map[string][]string) BFSResult {
	depths := make(map[string]uint16)
	foundOn := make(map[string]string)
	visited := make(map[string]bool)
	type bfsItem struct {
		url   string
		depth uint16
	}
	var queue []bfsItem

	for _, seed := range seedURLs {
		// Try the seed URL as-is and with/without trailing slash
		candidates := []string{seed}
		if strings.HasSuffix(seed, "/") {
			candidates = append(candidates, strings.TrimRight(seed, "/"))
		} else {
			candidates = append(candidates, seed+"/")
		}
		for _, c := range candidates {
			if crawledSet[c] && !visited[c] {
				visited[c] = true
				depths[c] = 0
				foundOn[c] = ""
				queue = append(queue, bfsItem{url: c, depth: 0})
			}
		}
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		for _, target := range adj[item.url] {
			if !visited[target] {
				visited[target] = true
				newDepth := item.depth + 1
				if crawledSet[target] {
					depths[target] = newDepth
					foundOn[target] = item.url
					queue = append(queue, bfsItem{url: target, depth: newDepth})
				}
			}
		}
	}

	// Assign max depth to unreachable URLs (orphans).
	// depths only contains crawled URLs, so maxDepth is accurate.
	var maxDepth uint16
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
	}
	orphanDepth := maxDepth + 1
	for u := range crawledSet {
		if _, ok := depths[u]; !ok {
			depths[u] = orphanDepth
			foundOn[u] = ""
		}
	}

	return BFSResult{Depths: depths, FoundOn: foundOn}
}

func (s *Store) RecomputeDepths(ctx context.Context, sessionID string, seedURLs []string) error {
	// 1. Get all crawled URLs
	crawledRows, err := s.conn.Query(ctx, `
		SELECT url FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("querying crawled URLs: %w", err)
	}
	defer crawledRows.Close()

	crawledSet := make(map[string]bool)
	for crawledRows.Next() {
		var u string
		if err := crawledRows.Scan(&u); err != nil {
			return fmt.Errorf("scanning crawled URL: %w", err)
		}
		crawledSet[u] = true
	}
	if err := crawledRows.Err(); err != nil {
		return fmt.Errorf("iterating crawled URLs: %w", err)
	}

	if len(crawledSet) == 0 {
		return nil
	}

	// 2. Get all internal links as adjacency list
	linkRows, err := s.conn.Query(ctx, `
		SELECT source_url, target_url FROM crawlobserver.links
		WHERE crawl_session_id = ? AND is_internal = true`, sessionID)
	if err != nil {
		return fmt.Errorf("querying links: %w", err)
	}
	defer linkRows.Close()

	adj := make(map[string][]string)
	seen := make(map[[2]string]bool)
	for linkRows.Next() {
		var src, tgt string
		if err := linkRows.Scan(&src, &tgt); err != nil {
			return fmt.Errorf("scanning link: %w", err)
		}
		key := [2]string{src, tgt}
		if !seen[key] {
			seen[key] = true
			adj[src] = append(adj[src], tgt)
		}
	}
	if err := linkRows.Err(); err != nil {
		return fmt.Errorf("iterating links: %w", err)
	}

	// 3. BFS from seed URLs
	bfsResult := ComputeBFSDepths(seedURLs, crawledSet, adj)
	depths := bfsResult.Depths
	foundOn := bfsResult.FoundOn

	// 4. Write back depths via temp table (avoids SQL injection from crawled URLs)
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}

	tmpTable := fmt.Sprintf("crawlobserver.tmp_depths_%s", strings.ReplaceAll(sessionID, "-", ""))
	if err := s.conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tmpTable)); err != nil {
		return fmt.Errorf("dropping old temp depths table: %w", err)
	}
	if err := s.conn.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (page_url String, new_depth UInt16, new_found_on String) ENGINE = Join(ANY, LEFT, page_url)", tmpTable)); err != nil {
		return fmt.Errorf("creating temp depths table: %w", err)
	}
	defer func() {
		if err := s.conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tmpTable)); err != nil {
			applog.Warnf("storage", "cleanup temp table %s: %v", tmpTable, err)
		}
	}()

	urls := make([]string, 0, len(depths))
	for u := range depths {
		urls = append(urls, u)
	}

	const chunkSize = 500
	for i := 0; i < len(urls); i += chunkSize {
		end := i + chunkSize
		if end > len(urls) {
			end = len(urls)
		}

		batch, err := s.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s (page_url, new_depth, new_found_on)", tmpTable))
		if err != nil {
			return fmt.Errorf("preparing depths batch: %w", err)
		}
		for _, u := range urls[i:end] {
			if err := batch.Append(u, depths[u], foundOn[u]); err != nil {
				return fmt.Errorf("appending to depths batch: %w", err)
			}
		}
		if err := batch.Send(); err != nil {
			return fmt.Errorf("sending depths batch: %w", err)
		}
	}

	// Use joinGet to look up depth/found_on from the Join-engine temp table.
	// Single mutation, no data copy, no correlated subquery.
	query := fmt.Sprintf(`ALTER TABLE crawlobserver.pages UPDATE
		depth = joinGet('%s', 'new_depth', url),
		found_on = joinGet('%s', 'new_found_on', url)
		WHERE crawl_session_id = ?
		SETTINGS mutations_sync = 1`,
		tmpTable, tmpTable)

	if err := s.conn.Exec(ctx, query, sessionID); err != nil {
		return fmt.Errorf("updating depths via joinGet: %w", err)
	}

	applog.Infof("storage", "RecomputeDepths: updated %d URLs for session %s", len(depths), sessionID)
	return nil
}

// ListRedirectPages retrieves pages with 3xx status codes and their inbound internal link count.
func (s *Store) ListRedirectPages(ctx context.Context, sessionID string, limit, offset int, filters []ParsedFilter, sort *SortParam) ([]RedirectPageRow, error) {
	query := `
		SELECT p.url, p.status_code, p.final_url,
			count(DISTINCT l.source_url) AS inbound_internal_links
		FROM crawlobserver.pages AS p
		LEFT JOIN crawlobserver.links AS l
			ON l.crawl_session_id = p.crawl_session_id
			AND l.target_url = p.url
			AND l.is_internal = true
		WHERE p.crawl_session_id = ?
			AND p.status_code >= 300 AND p.status_code < 400`
	args := []interface{}{sessionID}

	whereExtra, filterArgs, err := BuildWhereClause(filters)
	if err != nil {
		return nil, fmt.Errorf("building filter clause: %w", err)
	}
	if whereExtra != "" {
		query += " AND " + whereExtra
		args = append(args, filterArgs...)
	}

	query += " GROUP BY p.url, p.status_code, p.final_url"
	query += BuildOrderByClause(sort, "inbound_internal_links DESC") + ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying redirect pages: %w", err)
	}
	defer rows.Close()

	var result []RedirectPageRow
	for rows.Next() {
		var r RedirectPageRow
		if err := rows.Scan(&r.URL, &r.StatusCode, &r.FinalURL, &r.InboundInternalLinks); err != nil {
			return nil, fmt.Errorf("scanning redirect page: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating redirect pages: %w", err)
	}
	return result, nil
}

// PageRankBucket holds one histogram bucket for PageRank distribution.
type PageRankBucket struct {
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Count uint64  `json:"count"`
	AvgPR float64 `json:"avg_pr"`
}

// PageRankDistributionResult holds the full distribution response.
type PageRankDistributionResult struct {
	Buckets     []PageRankBucket `json:"buckets"`
	TotalWithPR uint64           `json:"total_with_pr"`
	Avg         float64          `json:"avg"`
	Median      float64          `json:"median"`
	P90         float64          `json:"p90"`
	P99         float64          `json:"p99"`
}

// PageRankDistribution returns a histogram of PageRank values for a session.
func (s *Store) PageRankDistribution(ctx context.Context, sessionID string, buckets int) (*PageRankDistributionResult, error) {
	if buckets <= 0 {
		buckets = 20
	}

	result := &PageRankDistributionResult{}

	// Stats + percentiles
	row := s.conn.QueryRow(ctx, `
		SELECT count(), avg(pagerank),
			quantile(0.5)(pagerank), quantile(0.9)(pagerank), quantile(0.99)(pagerank)
		FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND pagerank > 0`, sessionID)
	if err := row.Scan(&result.TotalWithPR, &result.Avg, &result.Median, &result.P90, &result.P99); err != nil {
		return nil, fmt.Errorf("querying pagerank stats: %w", err)
	}

	if result.TotalWithPR == 0 {
		result.Avg = 0
		result.Median = 0
		result.P90 = 0
		result.P99 = 0
		return result, nil
	}

	// Histogram buckets
	width := 100.0 / float64(buckets)
	distQuery := fmt.Sprintf(`
		SELECT floor(pagerank / %f) * %f AS bucket_min,
			floor(pagerank / %f) * %f + %f AS bucket_max,
			count() AS cnt,
			avg(pagerank) AS avg_pr
		FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND pagerank > 0
		GROUP BY bucket_min, bucket_max
		ORDER BY bucket_min`, width, width, width, width, width)
	rows, err := s.conn.Query(ctx, distQuery, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying pagerank distribution: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var b PageRankBucket
		if err := rows.Scan(&b.Min, &b.Max, &b.Count, &b.AvgPR); err != nil {
			return nil, fmt.Errorf("scanning bucket: %w", err)
		}
		result.Buckets = append(result.Buckets, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pagerank buckets: %w", err)
	}
	return result, nil
}

// PageRankTreemapEntry holds aggregated PageRank data for a URL directory.
type PageRankTreemapEntry struct {
	Path      string  `json:"path"`
	PageCount uint64  `json:"page_count"`
	TotalPR   float64 `json:"total_pr"`
	AvgPR     float64 `json:"avg_pr"`
	MaxPR     float64 `json:"max_pr"`
}

// PageRankTreemap returns PageRank aggregated by URL directory prefix.
func (s *Store) PageRankTreemap(ctx context.Context, sessionID string, depth, minPages int) ([]PageRankTreemapEntry, error) {
	if !isValidUUID(sessionID) {
		return nil, fmt.Errorf("invalid session ID: %s", sessionID)
	}
	if depth <= 0 {
		depth = 2
	}
	if minPages <= 0 {
		minPages = 1
	}

	query := fmt.Sprintf(`
		SELECT
			arrayStringConcat(arraySlice(splitByChar('/', replaceRegexpOne(url, '^http[s]://[^/]*', '')), 1, %d), '/') AS dir_path,
			count() AS page_count,
			sum(pagerank) AS total_pr,
			avg(pagerank) AS avg_pr,
			max(pagerank) AS max_pr
		FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND pagerank > 0
		GROUP BY dir_path
		HAVING page_count >= %d
		ORDER BY total_pr DESC
		LIMIT 200`, depth, minPages)
	rows, err := s.conn.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying pagerank treemap: %w", err)
	}
	defer rows.Close()

	var entries []PageRankTreemapEntry
	for rows.Next() {
		var e PageRankTreemapEntry
		if err := rows.Scan(&e.Path, &e.PageCount, &e.TotalPR, &e.AvgPR, &e.MaxPR); err != nil {
			return nil, fmt.Errorf("scanning treemap entry: %w", err)
		}
		if e.Path == "" {
			e.Path = "/"
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating treemap entries: %w", err)
	}
	return entries, nil
}

// PageRankTopPage holds a single page entry for the top PageRank list.
type PageRankTopPage struct {
	URL              string  `json:"url"`
	PageRank         float64 `json:"pagerank"`
	Depth            uint16  `json:"depth"`
	InternalLinksOut uint32  `json:"internal_links_out"`
	ExternalLinksOut uint32  `json:"external_links_out"`
	WordCount        uint32  `json:"word_count"`
	StatusCode       uint16  `json:"status_code"`
	Title            string  `json:"title"`
}

// PageRankTopResult holds the paginated top PageRank pages response.
type PageRankTopResult struct {
	Pages []PageRankTopPage `json:"pages"`
	Total uint64            `json:"total"`
}

// PageRankTop returns the top pages by PageRank with metadata, paginated.
func (s *Store) PageRankTop(ctx context.Context, sessionID string, limit, offset int, directory string) (*PageRankTopResult, error) {
	if limit <= 0 {
		limit = 50
	}

	result := &PageRankTopResult{}

	// Count query
	countQuery := `SELECT count() FROM crawlobserver.pages WHERE crawl_session_id = ? AND pagerank > 0`
	countArgs := []interface{}{sessionID}
	if directory != "" {
		countQuery += ` AND url LIKE ?`
		countArgs = append(countArgs, "%"+directory+"%")
	}
	row := s.conn.QueryRow(ctx, countQuery, countArgs...)
	if err := row.Scan(&result.Total); err != nil {
		return nil, fmt.Errorf("querying pagerank count: %w", err)
	}

	// Data query
	query := `SELECT url, pagerank, depth, internal_links_out, external_links_out, word_count, status_code, title
		FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND pagerank > 0`
	args := []interface{}{sessionID}
	if directory != "" {
		query += ` AND url LIKE ?`
		args = append(args, "%"+directory+"%")
	}
	query += ` ORDER BY pagerank DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying pagerank top: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p PageRankTopPage
		if err := rows.Scan(&p.URL, &p.PageRank, &p.Depth, &p.InternalLinksOut, &p.ExternalLinksOut, &p.WordCount, &p.StatusCode, &p.Title); err != nil {
			return nil, fmt.Errorf("scanning top page: %w", err)
		}
		result.Pages = append(result.Pages, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating top pages: %w", err)
	}
	return result, nil
}

// NearDuplicates finds pages with similar content using SimHash Hamming distance.
// threshold is the max Hamming distance (e.g. 3 = ≤3 bits differ out of 64).
func (s *Store) NearDuplicates(ctx context.Context, sessionID string, threshold int, limit, offset int) (*NearDuplicatesResult, error) {
	if threshold <= 0 {
		threshold = 3
	}

	// Count total pairs
	var total uint64
	err := s.conn.QueryRow(ctx, `
		SELECT count()
		FROM crawlobserver.pages AS a
		INNER JOIN crawlobserver.pages AS b
			ON a.crawl_session_id = b.crawl_session_id
			AND a.url < b.url
		WHERE a.crawl_session_id = ?
			AND a.content_hash != 0
			AND b.content_hash != 0
			AND a.status_code >= 200 AND a.status_code < 300
			AND b.status_code >= 200 AND b.status_code < 300
			AND bitCount(bitXor(a.content_hash, b.content_hash)) <= ?`,
		sessionID, threshold).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("counting near-duplicates: %w", err)
	}

	rows, err := s.conn.Query(ctx, `
		SELECT
			a.url, b.url,
			a.title, b.title,
			a.canonical, b.canonical,
			a.word_count, b.word_count,
			1.0 - (bitCount(bitXor(a.content_hash, b.content_hash)) / 64.0) AS similarity
		FROM crawlobserver.pages AS a
		INNER JOIN crawlobserver.pages AS b
			ON a.crawl_session_id = b.crawl_session_id
			AND a.url < b.url
		WHERE a.crawl_session_id = ?
			AND a.content_hash != 0
			AND b.content_hash != 0
			AND a.status_code >= 200 AND a.status_code < 300
			AND b.status_code >= 200 AND b.status_code < 300
			AND bitCount(bitXor(a.content_hash, b.content_hash)) <= ?
		ORDER BY similarity DESC
		LIMIT ? OFFSET ?`,
		sessionID, threshold, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying near-duplicates: %w", err)
	}
	defer rows.Close()

	result := &NearDuplicatesResult{Total: total, Pairs: []NearDuplicatePair{}}
	for rows.Next() {
		var p NearDuplicatePair
		if err := rows.Scan(&p.URLa, &p.URLb, &p.TitleA, &p.TitleB, &p.CanonicalA, &p.CanonicalB, &p.WordCountA, &p.WordCountB, &p.Similarity); err != nil {
			return nil, fmt.Errorf("scanning near-duplicate: %w", err)
		}
		result.Pairs = append(result.Pairs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating near-duplicates: %w", err)
	}
	return result, nil
}

// PagesWithAuthority joins crawled pages with provider top_pages (Majestic authority data).
func (s *Store) PagesWithAuthority(ctx context.Context, sessionID, projectID string, limit, offset int) ([]PageWithAuthority, int, error) {
	if !isValidUUID(sessionID) {
		return nil, 0, fmt.Errorf("invalid session ID")
	}

	var total uint64
	if err := s.conn.QueryRow(ctx, `
		SELECT count()
		FROM crawlobserver.pages FINAL AS p
		WHERE p.crawl_session_id = ? AND p.status_code >= 200 AND p.status_code < 300
	`, sessionID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting authority pages: %w", err)
	}

	rows, err := s.conn.Query(ctx, `
		SELECT p.url, p.title, p.pagerank, p.word_count, p.status_code, p.depth,
		       t.trust_flow, t.citation_flow, t.ext_backlinks, t.ref_domains
		FROM crawlobserver.pages FINAL AS p
		LEFT JOIN crawlobserver.provider_top_pages FINAL AS t
		  ON p.url = t.url AND t.project_id = ? AND t.provider = 'seobserver'
		WHERE p.crawl_session_id = ?
		  AND p.status_code >= 200 AND p.status_code < 300
		ORDER BY t.trust_flow DESC NULLS LAST
		LIMIT ? OFFSET ?
	`, projectID, sessionID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying authority pages: %w", err)
	}
	defer rows.Close()

	var result []PageWithAuthority
	for rows.Next() {
		var r PageWithAuthority
		var tf, cf uint8
		var extBL, rd int64
		if err := rows.Scan(&r.URL, &r.Title, &r.PageRank, &r.WordCount, &r.StatusCode, &r.Depth,
			&tf, &cf, &extBL, &rd); err != nil {
			return nil, 0, fmt.Errorf("scanning authority page row: %w", err)
		}
		if tf > 0 || cf > 0 {
			r.TrustFlow = &tf
			r.CitationFlow = &cf
			r.ExtBackLinks = &extBL
			r.RefDomains = &rd
		}
		result = append(result, r)
	}
	if result == nil {
		result = []PageWithAuthority{}
	}
	return result, int(total), nil
}

// weightedPRSortColumns is the whitelist of allowed sort columns for weighted PageRank.
var weightedPRSortColumns = map[string]string{
	"weighted_pr":   "weighted_pr",
	"pagerank":      "p.pagerank",
	"trust_flow":    "t.trust_flow",
	"citation_flow": "t.citation_flow",
	"ref_domains":   "t.ref_domains",
	"ext_backlinks": "t.ext_backlinks",
	"delta":         "(weighted_pr - p.pagerank)",
}

// WeightedPageRankTop returns pages ranked by a weighted PageRank that fuses internal PR with SEObserver data.
func (s *Store) WeightedPageRankTop(ctx context.Context, sessionID, projectID string, limit, offset int, directory, sort, order string) (*WeightedPageRankResult, error) {
	if limit <= 0 {
		limit = 50
	}

	result := &WeightedPageRankResult{}

	// Count pages with PR > 0
	countQuery := `SELECT count() FROM crawlobserver.pages WHERE crawl_session_id = ? AND pagerank > 0`
	countArgs := []interface{}{sessionID}
	if directory != "" {
		countQuery += ` AND url LIKE ?`
		countArgs = append(countArgs, "%"+directory+"%")
	}
	if err := s.conn.QueryRow(ctx, countQuery, countArgs...).Scan(&result.Total); err != nil {
		return nil, fmt.Errorf("counting weighted pagerank pages: %w", err)
	}

	// Main query with weighted PR calculation
	// Wrap tables in subqueries to avoid FINAL + CROSS JOIN syntax issues in ClickHouse
	query := `
		SELECT
			p.url,
			p.pagerank,
			if(t.trust_flow > 0 OR t.citation_flow > 0,
				0.40 * p.pagerank
				+ 0.25 * t.trust_flow
				+ 0.10 * t.citation_flow
				+ 0.15 * if(m.max_log_rd > 0, 100.0 * log1p(t.ref_domains) / m.max_log_rd, 0)
				+ 0.10 * if(m.max_log_bl > 0, 100.0 * log1p(t.ext_backlinks) / m.max_log_bl, 0),
				p.pagerank
			) AS weighted_pr,
			t.trust_flow,
			t.citation_flow,
			t.ext_backlinks,
			t.ref_domains,
			p.depth,
			p.internal_links_out,
			p.status_code,
			p.title,
			t.ttf_topic
		FROM (
			SELECT url, pagerank, depth, internal_links_out, status_code, title
			FROM crawlobserver.pages FINAL
			WHERE crawl_session_id = ? AND pagerank > 0
		) AS p
		CROSS JOIN (
			SELECT
				max(log1p(ext_backlinks)) AS max_log_bl,
				max(log1p(ref_domains)) AS max_log_rd
			FROM crawlobserver.provider_data
			WHERE project_id = ? AND provider = 'seobserver' AND data_type = 'top_pages'
		) AS m
		LEFT JOIN (
			SELECT trimRight(item_url, '/') AS item_url_norm, trust_flow, citation_flow, ext_backlinks, ref_domains,
				str_data['ttf_topic_0'] AS ttf_topic
			FROM crawlobserver.provider_data FINAL
			WHERE project_id = ? AND provider = 'seobserver' AND data_type = 'top_pages'
		) AS t ON trimRight(p.url, '/') = t.item_url_norm
		WHERE 1=1`

	args := []interface{}{sessionID, projectID, projectID}
	if directory != "" {
		query += ` AND p.url LIKE ?`
		args = append(args, "%"+directory+"%")
	}

	// Dynamic ORDER BY with whitelist
	orderClause := "weighted_pr DESC"
	if col, ok := weightedPRSortColumns[sort]; ok {
		dir := "DESC"
		if order == "asc" {
			dir = "ASC"
		}
		orderClause = col + " " + dir
	}
	query += ` ORDER BY ` + orderClause + ` LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying weighted pagerank: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p WeightedPageRankPage
		var tf, cf uint8
		var extBL, rd int64
		var ttfTopic string
		if err := rows.Scan(&p.URL, &p.PageRank, &p.WeightedPR, &tf, &cf, &extBL, &rd,
			&p.Depth, &p.InternalLinksOut, &p.StatusCode, &p.Title, &ttfTopic); err != nil {
			return nil, fmt.Errorf("scanning weighted pagerank row: %w", err)
		}
		if tf > 0 || cf > 0 {
			p.TrustFlow = &tf
			p.CitationFlow = &cf
			p.ExtBackLinks = &extBL
			p.RefDomains = &rd
		}
		if ttfTopic != "" {
			p.TTFTopic = &ttfTopic
		}
		result.Pages = append(result.Pages, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating weighted pagerank rows: %w", err)
	}
	return result, nil
}
