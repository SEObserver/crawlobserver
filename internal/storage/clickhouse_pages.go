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
			error, depth, found_on, pagerank, body_html, body_truncated, crawled_at
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
			p.Error, p.Depth, p.FoundOn, p.PageRank, p.BodyHTML, p.BodyTruncated, p.CrawledAt,
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
func (s *Store) ListPages(ctx context.Context, sessionID string, limit, offset int, filters []ParsedFilter) ([]PageRow, error) {
	query := `
		SELECT crawl_session_id, url, final_url, status_code, content_type,
			title, title_length, canonical, canonical_is_self, is_indexable, index_reason,
			meta_robots, meta_description, meta_desc_length, meta_keywords,
			h1, h2, h3, h4, h5, h6,
			word_count, internal_links_out, external_links_out,
			images_count, images_no_alt,
			lang, og_title, og_description, og_image, schema_types,
			body_size, fetch_duration_ms, content_encoding, x_robots_tag,
			error, depth, found_on, pagerank, crawled_at
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

	query += ` ORDER BY crawled_at DESC LIMIT ? OFFSET ?`
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
		); err != nil {
			return nil, fmt.Errorf("scanning page: %w", err)
		}
		pages = append(pages, p)
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
			error, depth, found_on, pagerank, crawled_at
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
	return urls, nil
}

// CrawledURLs returns all URLs already crawled in a session (for dedup on resume).
func (s *Store) CrawledURLs(ctx context.Context, sessionID string) ([]string, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT url FROM crawlobserver.pages WHERE crawl_session_id = ?
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying crawled URLs: %w", err)
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
	return urls, nil
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

	n := uint32(len(idToURL))
	if n == 0 {
		return nil
	}

	// 2. Load internal links as adjacency list (outgoing edges by source ID)
	linkRows, err := s.conn.Query(ctx, `
		SELECT source_url, target_url FROM crawlobserver.links
		WHERE crawl_session_id = ? AND is_internal = true`, sessionID)
	if err != nil {
		return fmt.Errorf("querying links: %w", err)
	}
	defer linkRows.Close()

	outLinks := make([][]uint32, n)
	for linkRows.Next() {
		var src, tgt string
		if err := linkRows.Scan(&src, &tgt); err != nil {
			return fmt.Errorf("scanning link: %w", err)
		}
		srcID, srcOK := urlToID[src]
		tgtID, tgtOK := urlToID[tgt]
		if srcOK && tgtOK && srcID != tgtID {
			outLinks[srcID] = append(outLinks[srcID], tgtID)
		}
	}

	// 3. Deduplicate outgoing links per node
	for i := range outLinks {
		if len(outLinks[i]) > 1 {
			seen := make(map[uint32]bool, len(outLinks[i]))
			j := 0
			for _, id := range outLinks[i] {
				if !seen[id] {
					seen[id] = true
					outLinks[i][j] = id
					j++
				}
			}
			outLinks[i] = outLinks[i][:j]
		}
	}

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
	if err := s.conn.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (page_url String, new_pagerank Float64) ENGINE = Memory", tmpTable)); err != nil {
		return fmt.Errorf("creating temp pagerank table: %w", err)
	}
	defer s.conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tmpTable))

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

	query := fmt.Sprintf(`ALTER TABLE crawlobserver.pages UPDATE
		pagerank = (SELECT new_pagerank FROM %s WHERE page_url = pages.url LIMIT 1)
		WHERE crawl_session_id = ? AND url IN (SELECT page_url FROM %s)`,
		tmpTable, tmpTable)
	if err := s.conn.Exec(ctx, query, sessionID); err != nil {
		return fmt.Errorf("updating pagerank from temp table: %w", err)
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
	if err := s.conn.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (page_url String, new_depth UInt16, new_found_on String) ENGINE = Memory", tmpTable)); err != nil {
		return fmt.Errorf("creating temp depths table: %w", err)
	}
	defer s.conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tmpTable))

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

	// Single mutation: update from temp table.
	// Use pages.url to explicitly reference the outer table column in subqueries.
	query := fmt.Sprintf(`ALTER TABLE crawlobserver.pages UPDATE
		depth = (SELECT new_depth FROM %s WHERE page_url = pages.url LIMIT 1),
		found_on = (SELECT new_found_on FROM %s WHERE page_url = pages.url LIMIT 1)
		WHERE crawl_session_id = ? AND url IN (SELECT page_url FROM %s)`,
		tmpTable, tmpTable, tmpTable)

	if err := s.conn.Exec(ctx, query, sessionID); err != nil {
		return fmt.Errorf("updating depths from temp table: %w", err)
	}

	applog.Infof("storage", "RecomputeDepths: updated %d URLs for session %s", len(depths), sessionID)
	return nil
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
	return result, nil
}
