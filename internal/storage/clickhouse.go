package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Store manages ClickHouse connections and operations.
type Store struct {
	conn driver.Conn
}

// NewStore creates a new ClickHouse store.
func NewStore(host string, port int, database, username, password string) (*Store, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", host, port)},
		Auth: clickhouse.Auth{
			Database: database,
			Username: username,
			Password: password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to ClickHouse: %w", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("pinging ClickHouse: %w", err)
	}

	return &Store{conn: conn}, nil
}

// Migrate runs all DDL migrations.
func (s *Store) Migrate(ctx context.Context) error {
	for i, ddl := range Migrations {
		if err := s.conn.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	return nil
}

// InsertSession inserts or updates a crawl session.
func (s *Store) InsertSession(ctx context.Context, session *CrawlSession) error {
	return s.conn.Exec(ctx, `
		INSERT INTO seocrawler.crawl_sessions
		(id, started_at, finished_at, status, seed_urls, config, pages_crawled, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.StartedAt, session.FinishedAt, session.Status,
		session.SeedURLs, session.Config, session.PagesCrawled, session.UserAgent,
	)
}

// InsertPages batch inserts page rows.
func (s *Store) InsertPages(ctx context.Context, pages []PageRow) error {
	if len(pages) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO seocrawler.pages (
			crawl_session_id, url, final_url, status_code, content_type,
			title, title_length, canonical, canonical_is_self, is_indexable, index_reason,
			meta_robots, meta_description, meta_desc_length, meta_keywords,
			h1, h2, h3, h4, h5, h6,
			word_count, internal_links_out, external_links_out,
			images_count, images_no_alt, hreflang,
			lang, og_title, og_description, og_image, schema_types,
			headers, redirect_chain, body_size, fetch_duration_ms,
			content_encoding, x_robots_tag,
			error, depth, found_on, body_html, crawled_at
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
			p.Error, p.Depth, p.FoundOn, p.BodyHTML, p.CrawledAt,
		); err != nil {
			return fmt.Errorf("appending page row: %w", err)
		}
	}

	return batch.Send()
}

// InsertLinks batch inserts link rows.
func (s *Store) InsertLinks(ctx context.Context, links []LinkRow) error {
	if len(links) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO seocrawler.links (
			crawl_session_id, source_url, target_url, anchor_text, rel,
			is_internal, tag, crawled_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing links batch: %w", err)
	}

	for _, l := range links {
		if err := batch.Append(
			l.CrawlSessionID, l.SourceURL, l.TargetURL, l.AnchorText, l.Rel,
			l.IsInternal, l.Tag, l.CrawledAt,
		); err != nil {
			return fmt.Errorf("appending link row: %w", err)
		}
	}

	return batch.Send()
}

// ListSessions retrieves all crawl sessions.
func (s *Store) ListSessions(ctx context.Context) ([]CrawlSession, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT id, started_at, finished_at, status, seed_urls, config, pages_crawled, user_agent
		FROM seocrawler.crawl_sessions FINAL
		ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	var sessions []CrawlSession
	for rows.Next() {
		var sess CrawlSession
		if err := rows.Scan(
			&sess.ID, &sess.StartedAt, &sess.FinishedAt,
			&sess.Status, &sess.SeedURLs, &sess.Config,
			&sess.PagesCrawled, &sess.UserAgent,
		); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// GetSession retrieves a single crawl session by ID.
func (s *Store) GetSession(sessionID string) (*CrawlSession, error) {
	row := s.conn.QueryRow(context.Background(), `
		SELECT id, started_at, finished_at, status, seed_urls, config, pages_crawled, user_agent
		FROM seocrawler.crawl_sessions FINAL
		WHERE id = ?
	`, sessionID)

	var sess CrawlSession
	if err := row.Scan(
		&sess.ID, &sess.StartedAt, &sess.FinishedAt,
		&sess.Status, &sess.SeedURLs, &sess.Config,
		&sess.PagesCrawled, &sess.UserAgent,
	); err != nil {
		return nil, fmt.Errorf("querying session %s: %w", sessionID, err)
	}
	return &sess, nil
}

// ExternalLinks retrieves external links for a given session (or all sessions).
func (s *Store) ExternalLinks(ctx context.Context, sessionID string) ([]LinkRow, error) {
	query := `
		SELECT crawl_session_id, source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM seocrawler.links
		WHERE is_internal = false`
	args := []interface{}{}

	if sessionID != "" {
		query += ` AND crawl_session_id = ?`
		args = append(args, sessionID)
	}
	query += ` ORDER BY source_url, target_url`

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying external links: %w", err)
	}
	defer rows.Close()

	var links []LinkRow
	for rows.Next() {
		var l LinkRow
		if err := rows.Scan(
			&l.CrawlSessionID, &l.SourceURL, &l.TargetURL, &l.AnchorText,
			&l.Rel, &l.IsInternal, &l.Tag, &l.CrawledAt,
		); err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}
		links = append(links, l)
	}
	return links, nil
}

// InternalLinksPaginated retrieves internal links with pagination and optional filters.
func (s *Store) InternalLinksPaginated(ctx context.Context, sessionID string, limit, offset int, sourceFilter, targetFilter string) ([]LinkRow, error) {
	query := `
		SELECT crawl_session_id, source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM seocrawler.links
		WHERE is_internal = true AND crawl_session_id = ?`
	args := []interface{}{sessionID}

	if sourceFilter != "" {
		query += ` AND source_url LIKE ?`
		args = append(args, "%"+sourceFilter+"%")
	}
	if targetFilter != "" {
		query += ` AND target_url LIKE ?`
		args = append(args, "%"+targetFilter+"%")
	}

	query += ` ORDER BY source_url, target_url LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying internal links: %w", err)
	}
	defer rows.Close()

	var links []LinkRow
	for rows.Next() {
		var l LinkRow
		if err := rows.Scan(
			&l.CrawlSessionID, &l.SourceURL, &l.TargetURL, &l.AnchorText,
			&l.Rel, &l.IsInternal, &l.Tag, &l.CrawledAt,
		); err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}
		links = append(links, l)
	}
	return links, nil
}

// ExternalLinksPaginated retrieves external links with pagination.
func (s *Store) ExternalLinksPaginated(ctx context.Context, sessionID string, limit, offset int) ([]LinkRow, error) {
	query := `
		SELECT crawl_session_id, source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM seocrawler.links
		WHERE is_internal = false`
	args := []interface{}{}

	if sessionID != "" {
		query += ` AND crawl_session_id = ?`
		args = append(args, sessionID)
	}
	query += ` ORDER BY source_url, target_url LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying external links: %w", err)
	}
	defer rows.Close()

	var links []LinkRow
	for rows.Next() {
		var l LinkRow
		if err := rows.Scan(
			&l.CrawlSessionID, &l.SourceURL, &l.TargetURL, &l.AnchorText,
			&l.Rel, &l.IsInternal, &l.Tag, &l.CrawledAt,
		); err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}
		links = append(links, l)
	}
	return links, nil
}

// ListPages retrieves pages for a session with pagination.
func (s *Store) ListPages(ctx context.Context, sessionID string, limit, offset int) ([]PageRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, url, final_url, status_code, content_type,
			title, title_length, canonical, canonical_is_self, is_indexable, index_reason,
			meta_robots, meta_description, meta_desc_length, meta_keywords,
			h1, h2, h3, h4, h5, h6,
			word_count, internal_links_out, external_links_out,
			images_count, images_no_alt,
			lang, og_title, og_description, og_image, schema_types,
			body_size, fetch_duration_ms, content_encoding, x_robots_tag,
			error, depth, found_on, crawled_at
		FROM seocrawler.pages
		WHERE crawl_session_id = ?
		ORDER BY crawled_at DESC
		LIMIT ? OFFSET ?
	`, sessionID, limit, offset)
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
			&p.Error, &p.Depth, &p.FoundOn, &p.CrawledAt,
		); err != nil {
			return nil, fmt.Errorf("scanning page: %w", err)
		}
		pages = append(pages, p)
	}
	return pages, nil
}

// SessionStats holds aggregate stats for a crawl session.
type SessionStats struct {
	TotalPages     uint64  `json:"total_pages"`
	TotalLinks     uint64  `json:"total_links"`
	InternalLinks  uint64  `json:"internal_links"`
	ExternalLinks  uint64  `json:"external_links"`
	AvgFetchMs     float64 `json:"avg_fetch_ms"`
	ErrorCount     uint64  `json:"error_count"`
	StatusCodes    map[uint16]uint64 `json:"status_codes"`
}

// SessionStats retrieves aggregate statistics for a crawl session.
func (s *Store) SessionStats(ctx context.Context, sessionID string) (*SessionStats, error) {
	stats := &SessionStats{
		StatusCodes: make(map[uint16]uint64),
	}

	// Page stats
	row := s.conn.QueryRow(ctx, `
		SELECT count(), avg(fetch_duration_ms), countIf(error != '')
		FROM seocrawler.pages WHERE crawl_session_id = ?`, sessionID)
	if err := row.Scan(&stats.TotalPages, &stats.AvgFetchMs, &stats.ErrorCount); err != nil {
		return nil, fmt.Errorf("querying page stats: %w", err)
	}

	// Link stats
	row = s.conn.QueryRow(ctx, `
		SELECT count(), countIf(is_internal = true), countIf(is_internal = false)
		FROM seocrawler.links WHERE crawl_session_id = ?`, sessionID)
	if err := row.Scan(&stats.TotalLinks, &stats.InternalLinks, &stats.ExternalLinks); err != nil {
		return nil, fmt.Errorf("querying link stats: %w", err)
	}

	// Status code distribution
	rows, err := s.conn.Query(ctx, `
		SELECT status_code, count() FROM seocrawler.pages
		WHERE crawl_session_id = ? GROUP BY status_code ORDER BY status_code`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying status codes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var code uint16
		var cnt uint64
		if err := rows.Scan(&code, &cnt); err != nil {
			return nil, err
		}
		stats.StatusCodes[code] = cnt
	}

	return stats, nil
}

// DeleteSession deletes a crawl session and all its associated data.
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	queries := []string{
		`ALTER TABLE seocrawler.links DELETE WHERE crawl_session_id = ?`,
		`ALTER TABLE seocrawler.pages DELETE WHERE crawl_session_id = ?`,
		`ALTER TABLE seocrawler.crawl_sessions DELETE WHERE id = ?`,
	}
	for _, q := range queries {
		if err := s.conn.Exec(ctx, q, sessionID); err != nil {
			return fmt.Errorf("deleting session data: %w", err)
		}
	}
	return nil
}

// UncrawledURLs returns internal link targets that were discovered but not crawled in a session.
func (s *Store) UncrawledURLs(sessionID string) ([]string, error) {
	ctx := context.Background()
	rows, err := s.conn.Query(ctx, `
		SELECT DISTINCT target_url
		FROM seocrawler.links
		WHERE crawl_session_id = ? AND is_internal = true
		  AND target_url NOT IN (
		    SELECT url FROM seocrawler.pages WHERE crawl_session_id = ?
		  )
		LIMIT 10000
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
func (s *Store) CrawledURLs(sessionID string) ([]string, error) {
	ctx := context.Background()
	rows, err := s.conn.Query(ctx, `
		SELECT url FROM seocrawler.pages WHERE crawl_session_id = ?
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

// GetPageHTML retrieves the raw HTML for a specific page.
func (s *Store) GetPageHTML(ctx context.Context, sessionID, url string) (string, error) {
	var html string
	row := s.conn.QueryRow(ctx, `
		SELECT body_html FROM seocrawler.pages
		WHERE crawl_session_id = ? AND url = ? LIMIT 1`, sessionID, url)
	if err := row.Scan(&html); err != nil {
		return "", fmt.Errorf("querying page HTML: %w", err)
	}
	return html, nil
}

// TableStorageStats holds storage stats for a single table.
type TableStorageStats struct {
	Name        string `json:"name"`
	BytesOnDisk uint64 `json:"bytes_on_disk"`
	Rows        uint64 `json:"rows"`
}

// StorageStatsResult holds storage stats for all tables.
type StorageStatsResult struct {
	Tables []TableStorageStats `json:"tables"`
}

// StorageStats retrieves disk usage and row counts for all seocrawler tables.
func (s *Store) StorageStats(ctx context.Context) (*StorageStatsResult, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT table, sum(bytes_on_disk), sum(rows)
		FROM system.parts
		WHERE database = 'seocrawler' AND active = 1
		GROUP BY table
		ORDER BY table`)
	if err != nil {
		return nil, fmt.Errorf("querying storage stats: %w", err)
	}
	defer rows.Close()

	result := &StorageStatsResult{}
	for rows.Next() {
		var t TableStorageStats
		if err := rows.Scan(&t.Name, &t.BytesOnDisk, &t.Rows); err != nil {
			return nil, fmt.Errorf("scanning storage stats: %w", err)
		}
		result.Tables = append(result.Tables, t)
	}
	return result, nil
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
			error, depth, found_on, crawled_at
		FROM seocrawler.pages
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
		&p.Error, &p.Depth, &p.FoundOn, &p.CrawledAt,
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

// GetPageLinks retrieves outbound and inbound links for a URL.
func (s *Store) GetPageLinks(ctx context.Context, sessionID, url string) ([]LinkRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM seocrawler.links
		WHERE crawl_session_id = ? AND (source_url = ? OR target_url = ?)
		ORDER BY source_url, target_url
		LIMIT 500`, sessionID, url, url)
	if err != nil {
		return nil, fmt.Errorf("querying page links: %w", err)
	}
	defer rows.Close()

	var links []LinkRow
	for rows.Next() {
		var l LinkRow
		if err := rows.Scan(
			&l.CrawlSessionID, &l.SourceURL, &l.TargetURL, &l.AnchorText,
			&l.Rel, &l.IsInternal, &l.Tag, &l.CrawledAt,
		); err != nil {
			return nil, fmt.Errorf("scanning page link: %w", err)
		}
		links = append(links, l)
	}
	return links, nil
}

// Close closes the ClickHouse connection.
func (s *Store) Close() error {
	return s.conn.Close()
}
