package storage

import (
	"context"
	"fmt"
	"log"
	"strings"
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
		(id, started_at, finished_at, status, seed_urls, config, pages_crawled, user_agent, project_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.StartedAt, session.FinishedAt, session.Status,
		session.SeedURLs, session.Config, session.PagesCrawled, session.UserAgent,
		session.ProjectID,
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

// ListSessions retrieves crawl sessions, optionally filtered by project ID.
func (s *Store) ListSessions(ctx context.Context, projectID ...string) ([]CrawlSession, error) {
	query := `
		SELECT id, started_at, finished_at, status, seed_urls, config, pages_crawled, user_agent, project_id
		FROM seocrawler.crawl_sessions FINAL`
	var args []interface{}
	if len(projectID) > 0 && projectID[0] != "" {
		query += ` WHERE project_id = ?`
		args = append(args, projectID[0])
	}
	query += ` ORDER BY started_at DESC`

	rows, err := s.conn.Query(ctx, query, args...)
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
			&sess.PagesCrawled, &sess.UserAgent, &sess.ProjectID,
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
		SELECT id, started_at, finished_at, status, seed_urls, config, pages_crawled, user_agent, project_id
		FROM seocrawler.crawl_sessions FINAL
		WHERE id = ?
	`, sessionID)

	var sess CrawlSession
	if err := row.Scan(
		&sess.ID, &sess.StartedAt, &sess.FinishedAt,
		&sess.Status, &sess.SeedURLs, &sess.Config,
		&sess.PagesCrawled, &sess.UserAgent, &sess.ProjectID,
	); err != nil {
		return nil, fmt.Errorf("querying session %s: %w", sessionID, err)
	}
	return &sess, nil
}

// UpdateSessionProject re-inserts a session with a new project_id (ReplacingMergeTree pattern).
func (s *Store) UpdateSessionProject(ctx context.Context, sessionID string, projectID *string) error {
	sess, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}
	sess.ProjectID = projectID
	return s.InsertSession(ctx, sess)
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
func (s *Store) InternalLinksPaginated(ctx context.Context, sessionID string, limit, offset int, filters []ParsedFilter) ([]LinkRow, error) {
	query := `
		SELECT crawl_session_id, source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM seocrawler.links
		WHERE is_internal = true AND crawl_session_id = ?`
	args := []interface{}{sessionID}

	whereExtra, filterArgs, err := BuildWhereClause(filters)
	if err != nil {
		return nil, fmt.Errorf("building filter clause: %w", err)
	}
	if whereExtra != "" {
		query += " AND " + whereExtra
		args = append(args, filterArgs...)
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

// ExternalLinksPaginated retrieves external links with pagination and optional filters.
func (s *Store) ExternalLinksPaginated(ctx context.Context, sessionID string, limit, offset int, filters []ParsedFilter) ([]LinkRow, error) {
	query := `
		SELECT crawl_session_id, source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM seocrawler.links
		WHERE is_internal = false`
	args := []interface{}{}

	if sessionID != "" {
		query += ` AND crawl_session_id = ?`
		args = append(args, sessionID)
	}

	whereExtra, filterArgs, err := BuildWhereClause(filters)
	if err != nil {
		return nil, fmt.Errorf("building filter clause: %w", err)
	}
	if whereExtra != "" {
		query += " AND " + whereExtra
		args = append(args, filterArgs...)
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
		FROM seocrawler.pages
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

// PageRankEntry holds a URL and its PageRank score.
type PageRankEntry struct {
	URL      string  `json:"url"`
	PageRank float64 `json:"pagerank"`
}

// SessionStats holds aggregate stats for a crawl session.
type SessionStats struct {
	TotalPages        uint64            `json:"total_pages"`
	TotalLinks        uint64            `json:"total_links"`
	InternalLinks     uint64            `json:"internal_links"`
	ExternalLinks     uint64            `json:"external_links"`
	AvgFetchMs        float64           `json:"avg_fetch_ms"`
	ErrorCount        uint64            `json:"error_count"`
	StatusCodes       map[uint16]uint64 `json:"status_codes"`
	DepthDistribution map[uint16]uint64 `json:"depth_distribution"`
	PagesPerSecond    float64           `json:"pages_per_second"`
	CrawlDurationSec  float64           `json:"crawl_duration_sec"`
	TopPageRank       []PageRankEntry   `json:"top_pagerank"`
}

// SessionStats retrieves aggregate statistics for a crawl session.
func (s *Store) SessionStats(ctx context.Context, sessionID string) (*SessionStats, error) {
	stats := &SessionStats{
		StatusCodes:       make(map[uint16]uint64),
		DepthDistribution: make(map[uint16]uint64),
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

	// Depth distribution
	depthRows, err := s.conn.Query(ctx, `
		SELECT depth, count() FROM seocrawler.pages
		WHERE crawl_session_id = ? GROUP BY depth ORDER BY depth`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying depth distribution: %w", err)
	}
	defer depthRows.Close()
	for depthRows.Next() {
		var depth uint16
		var cnt uint64
		if err := depthRows.Scan(&depth, &cnt); err != nil {
			return nil, err
		}
		stats.DepthDistribution[depth] = cnt
	}

	// Crawl duration and pages/sec
	var startedAt, finishedAt time.Time
	durRow := s.conn.QueryRow(ctx, `
		SELECT started_at, finished_at
		FROM seocrawler.crawl_sessions FINAL
		WHERE id = ?`, sessionID)
	if err := durRow.Scan(&startedAt, &finishedAt); err == nil {
		if !finishedAt.IsZero() && finishedAt.After(startedAt) {
			stats.CrawlDurationSec = finishedAt.Sub(startedAt).Seconds()
		}
		if stats.CrawlDurationSec > 0 {
			stats.PagesPerSecond = float64(stats.TotalPages) / stats.CrawlDurationSec
		}
	}

	// Top PageRank
	prRows, err := s.conn.Query(ctx, `
		SELECT url, pagerank FROM seocrawler.pages
		WHERE crawl_session_id = ? AND pagerank > 0
		ORDER BY pagerank DESC LIMIT 20`, sessionID)
	if err == nil {
		defer prRows.Close()
		for prRows.Next() {
			var e PageRankEntry
			if err := prRows.Scan(&e.URL, &e.PageRank); err == nil {
				stats.TopPageRank = append(stats.TopPageRank, e)
			}
		}
	}

	return stats, nil
}

// DeleteSession deletes a crawl session and all its associated data.
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	queries := []string{
		`ALTER TABLE seocrawler.links DELETE WHERE crawl_session_id = ?`,
		`ALTER TABLE seocrawler.pages DELETE WHERE crawl_session_id = ?`,
		`ALTER TABLE seocrawler.robots_txt DELETE WHERE crawl_session_id = ?`,
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

// GlobalSessionStats holds aggregated stats for a single session.
type GlobalSessionStats struct {
	SessionID  string  `json:"session_id"`
	TotalPages uint64  `json:"total_pages"`
	TotalLinks uint64  `json:"total_links"`
	ErrorCount uint64  `json:"error_count"`
	AvgFetchMs float64 `json:"avg_fetch_ms"`
}

// GlobalStats retrieves aggregated stats per session across all data.
func (s *Store) GlobalStats(ctx context.Context) ([]GlobalSessionStats, *StorageStatsResult, error) {
	// 1. Page stats per session
	pageRows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, count(), countIf(error != ''), avg(fetch_duration_ms)
		FROM seocrawler.pages
		GROUP BY crawl_session_id`)
	if err != nil {
		return nil, nil, fmt.Errorf("querying global page stats: %w", err)
	}
	defer pageRows.Close()

	statsMap := map[string]*GlobalSessionStats{}
	for pageRows.Next() {
		var gs GlobalSessionStats
		if err := pageRows.Scan(&gs.SessionID, &gs.TotalPages, &gs.ErrorCount, &gs.AvgFetchMs); err != nil {
			return nil, nil, fmt.Errorf("scanning global page stats: %w", err)
		}
		statsMap[gs.SessionID] = &gs
	}

	// 2. Link counts per session
	linkRows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, count()
		FROM seocrawler.links
		GROUP BY crawl_session_id`)
	if err != nil {
		return nil, nil, fmt.Errorf("querying global link stats: %w", err)
	}
	defer linkRows.Close()

	for linkRows.Next() {
		var sid string
		var cnt uint64
		if err := linkRows.Scan(&sid, &cnt); err != nil {
			return nil, nil, fmt.Errorf("scanning global link stats: %w", err)
		}
		if gs, ok := statsMap[sid]; ok {
			gs.TotalLinks = cnt
		} else {
			statsMap[sid] = &GlobalSessionStats{SessionID: sid, TotalLinks: cnt}
		}
	}

	// 3. Storage stats
	storage, err := s.StorageStats(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("querying storage for global stats: %w", err)
	}

	result := make([]GlobalSessionStats, 0, len(statsMap))
	for _, gs := range statsMap {
		result = append(result, *gs)
	}
	return result, storage, nil
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

// PageLinksResult holds outbound links, inbound links (paginated), and counts.
type PageLinksResult struct {
	OutLinks      []LinkRow `json:"out_links"`
	InLinks       []LinkRow `json:"in_links"`
	OutLinksCount uint64    `json:"out_links_count"`
	InLinksCount  uint64    `json:"in_links_count"`
}

// GetPageLinks retrieves outbound and inbound links for a URL with inbound pagination.
func (s *Store) GetPageLinks(ctx context.Context, sessionID, url string, inLimit, inOffset int) (*PageLinksResult, error) {
	result := &PageLinksResult{}

	// Counts
	countRow := s.conn.QueryRow(ctx, `
		SELECT countIf(source_url = ?), countIf(target_url = ?)
		FROM seocrawler.links
		WHERE crawl_session_id = ? AND (source_url = ? OR target_url = ?)`,
		url, url, sessionID, url, url)
	if err := countRow.Scan(&result.OutLinksCount, &result.InLinksCount); err != nil {
		return nil, fmt.Errorf("querying link counts: %w", err)
	}

	// Outbound links (all, capped at 1000)
	outRows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM seocrawler.links
		WHERE crawl_session_id = ? AND source_url = ?
		ORDER BY target_url
		LIMIT 1000`, sessionID, url)
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
		FROM seocrawler.links
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

// ComputePageRank computes internal PageRank for all pages in a session.
// Uses uint32 IDs for memory efficiency and iterative power method.
func (s *Store) ComputePageRank(ctx context.Context, sessionID string) error {
	// 1. Load all crawled URLs and assign numeric IDs
	urlRows, err := s.conn.Query(ctx, `
		SELECT url FROM seocrawler.pages WHERE crawl_session_id = ?`, sessionID)
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
		SELECT source_url, target_url FROM seocrawler.links
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
			log.Printf("PageRank converged after %d iterations (diff=%.2e)", iter+1, diff)
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

	// 6. Write back to ClickHouse in chunks
	const chunkSize = 500
	for i := 0; i < int(n); i += chunkSize {
		end := i + chunkSize
		if end > int(n) {
			end = int(n)
		}

		var cases []string
		var quotedURLs []string
		for j := i; j < end; j++ {
			escapedURL := strings.ReplaceAll(idToURL[j], "'", "\\'")
			cases = append(cases, fmt.Sprintf("url = '%s', %.6f", escapedURL, rank[j]))
			quotedURLs = append(quotedURLs, fmt.Sprintf("'%s'", escapedURL))
		}

		query := fmt.Sprintf(`ALTER TABLE seocrawler.pages UPDATE
			pagerank = multiIf(%s, pagerank)
			WHERE crawl_session_id = '%s' AND url IN (%s)`,
			strings.Join(cases, ", "),
			strings.ReplaceAll(sessionID, "'", "\\'"),
			strings.Join(quotedURLs, ", "))

		if err := s.conn.Exec(ctx, query); err != nil {
			return fmt.Errorf("updating pagerank chunk %d: %w", i/chunkSize, err)
		}
	}

	log.Printf("ComputePageRank: computed for %d pages in session %s", n, sessionID)
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
		SELECT url FROM seocrawler.pages WHERE crawl_session_id = ?`, sessionID)
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
		SELECT source_url, target_url FROM seocrawler.links
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
	result := ComputeBFSDepths(seedURLs, crawledSet, adj)
	depths := result.Depths
	foundOn := result.FoundOn

	// 4. Build UPDATE mutations in chunks
	urls := make([]string, 0, len(depths))
	for u := range depths {
		urls = append(urls, u)
	}

	const chunkSize = 100
	for i := 0; i < len(urls); i += chunkSize {
		end := i + chunkSize
		if end > len(urls) {
			end = len(urls)
		}
		chunk := urls[i:end]

		// Build multiIf for depth
		var depthCases []string
		var foundOnCases []string
		for _, u := range chunk {
			escapedURL := strings.ReplaceAll(u, "'", "\\'")
			depthCases = append(depthCases, fmt.Sprintf("url = '%s', %d", escapedURL, depths[u]))
			parent := foundOn[u]
			escapedParent := strings.ReplaceAll(parent, "'", "\\'")
			foundOnCases = append(foundOnCases, fmt.Sprintf("url = '%s', '%s'", escapedURL, escapedParent))
		}

		depthExpr := fmt.Sprintf("multiIf(%s, depth)", strings.Join(depthCases, ", "))
		foundOnExpr := fmt.Sprintf("multiIf(%s, found_on)", strings.Join(foundOnCases, ", "))

		// Build URL list for WHERE
		var quotedURLs []string
		for _, u := range chunk {
			escapedURL := strings.ReplaceAll(u, "'", "\\'")
			quotedURLs = append(quotedURLs, fmt.Sprintf("'%s'", escapedURL))
		}

		query := fmt.Sprintf(`ALTER TABLE seocrawler.pages UPDATE
			depth = %s,
			found_on = %s
			WHERE crawl_session_id = '%s' AND url IN (%s)`,
			depthExpr, foundOnExpr,
			strings.ReplaceAll(sessionID, "'", "\\'"),
			strings.Join(quotedURLs, ", "))

		if err := s.conn.Exec(ctx, query); err != nil {
			return fmt.Errorf("updating depths chunk %d: %w", i/chunkSize, err)
		}
	}

	log.Printf("RecomputeDepths: updated %d URLs for session %s", len(depths), sessionID)
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
		FROM seocrawler.pages
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
		FROM seocrawler.pages
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
		FROM seocrawler.pages
		WHERE crawl_session_id = '%s' AND pagerank > 0
		GROUP BY dir_path
		HAVING page_count >= %d
		ORDER BY total_pr DESC
		LIMIT 200`, depth, strings.ReplaceAll(sessionID, "'", "\\'"), minPages)
	rows, err := s.conn.Query(ctx, query)
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
	countQuery := `SELECT count() FROM seocrawler.pages WHERE crawl_session_id = ? AND pagerank > 0`
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
		FROM seocrawler.pages
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

// FailedURLs returns URLs with status_code = 0 (fetch errors) for a session.
func (s *Store) FailedURLs(sessionID string) ([]string, error) {
	ctx := context.Background()
	rows, err := s.conn.Query(ctx, `
		SELECT url FROM seocrawler.pages
		WHERE crawl_session_id = ? AND status_code = 0
		LIMIT 10000`, sessionID)
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

// DeleteFailedPages removes pages with status_code = 0 for a session so they can be re-crawled.
func (s *Store) DeleteFailedPages(ctx context.Context, sessionID string) (int, error) {
	// Count first
	var cnt uint64
	row := s.conn.QueryRow(ctx, `
		SELECT count() FROM seocrawler.pages
		WHERE crawl_session_id = ? AND status_code = 0`, sessionID)
	if err := row.Scan(&cnt); err != nil {
		return 0, fmt.Errorf("counting failed pages: %w", err)
	}

	if cnt == 0 {
		return 0, nil
	}

	// Delete them
	if err := s.conn.Exec(ctx, `
		ALTER TABLE seocrawler.pages DELETE
		WHERE crawl_session_id = ? AND status_code = 0`, sessionID); err != nil {
		return 0, fmt.Errorf("deleting failed pages: %w", err)
	}

	return int(cnt), nil
}

// InsertRobotsData batch inserts robots.txt rows.
func (s *Store) InsertRobotsData(ctx context.Context, rows []RobotsRow) error {
	if len(rows) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO seocrawler.robots_txt (
			crawl_session_id, host, status_code, content, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing robots batch: %w", err)
	}

	for _, r := range rows {
		if err := batch.Append(r.CrawlSessionID, r.Host, r.StatusCode, r.Content, r.FetchedAt); err != nil {
			return fmt.Errorf("appending robots row: %w", err)
		}
	}

	return batch.Send()
}

// GetRobotsHosts returns all hosts with robots.txt data for a session (without content).
func (s *Store) GetRobotsHosts(ctx context.Context, sessionID string) ([]RobotsRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, host, status_code, '' AS content, fetched_at
		FROM seocrawler.robots_txt FINAL
		WHERE crawl_session_id = ?
		ORDER BY host`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying robots hosts: %w", err)
	}
	defer rows.Close()

	var result []RobotsRow
	for rows.Next() {
		var r RobotsRow
		if err := rows.Scan(&r.CrawlSessionID, &r.Host, &r.StatusCode, &r.Content, &r.FetchedAt); err != nil {
			return nil, fmt.Errorf("scanning robots host: %w", err)
		}
		result = append(result, r)
	}
	return result, nil
}

// GetRobotsContent returns the full robots.txt content for a specific host in a session.
func (s *Store) GetRobotsContent(ctx context.Context, sessionID, host string) (*RobotsRow, error) {
	row := s.conn.QueryRow(ctx, `
		SELECT crawl_session_id, host, status_code, content, fetched_at
		FROM seocrawler.robots_txt FINAL
		WHERE crawl_session_id = ? AND host = ?
		LIMIT 1`, sessionID, host)

	var r RobotsRow
	if err := row.Scan(&r.CrawlSessionID, &r.Host, &r.StatusCode, &r.Content, &r.FetchedAt); err != nil {
		return nil, fmt.Errorf("querying robots content: %w", err)
	}
	return &r, nil
}

// InsertSitemaps inserts sitemap rows.
func (s *Store) InsertSitemaps(ctx context.Context, rows []SitemapRow) error {
	if len(rows) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO seocrawler.sitemaps (
			crawl_session_id, url, type, url_count, parent_url, status_code, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing sitemaps batch: %w", err)
	}

	for _, r := range rows {
		if err := batch.Append(r.CrawlSessionID, r.URL, r.Type, r.URLCount, r.ParentURL, r.StatusCode, r.FetchedAt); err != nil {
			return fmt.Errorf("appending sitemap row: %w", err)
		}
	}

	return batch.Send()
}

// InsertSitemapURLs inserts sitemap URL rows.
func (s *Store) InsertSitemapURLs(ctx context.Context, rows []SitemapURLRow) error {
	if len(rows) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO seocrawler.sitemap_urls (
			crawl_session_id, sitemap_url, loc, lastmod, changefreq, priority
		)`)
	if err != nil {
		return fmt.Errorf("preparing sitemap_urls batch: %w", err)
	}

	for _, r := range rows {
		if err := batch.Append(r.CrawlSessionID, r.SitemapURL, r.Loc, r.LastMod, r.ChangeFreq, r.Priority); err != nil {
			return fmt.Errorf("appending sitemap_url row: %w", err)
		}
	}

	return batch.Send()
}

// GetSitemaps returns all sitemaps for a session.
func (s *Store) GetSitemaps(ctx context.Context, sessionID string) ([]SitemapRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, url, type, url_count, parent_url, status_code, fetched_at
		FROM seocrawler.sitemaps FINAL
		WHERE crawl_session_id = ?
		ORDER BY url`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying sitemaps: %w", err)
	}
	defer rows.Close()

	var result []SitemapRow
	for rows.Next() {
		var r SitemapRow
		if err := rows.Scan(&r.CrawlSessionID, &r.URL, &r.Type, &r.URLCount, &r.ParentURL, &r.StatusCode, &r.FetchedAt); err != nil {
			return nil, fmt.Errorf("scanning sitemap: %w", err)
		}
		result = append(result, r)
	}
	return result, nil
}

// GetSitemapURLs returns paginated URLs from a specific sitemap.
func (s *Store) GetSitemapURLs(ctx context.Context, sessionID, sitemapURL string, limit, offset int) ([]SitemapURLRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, sitemap_url, loc, lastmod, changefreq, priority
		FROM seocrawler.sitemap_urls FINAL
		WHERE crawl_session_id = ? AND sitemap_url = ?
		ORDER BY loc
		LIMIT ? OFFSET ?`, sessionID, sitemapURL, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying sitemap urls: %w", err)
	}
	defer rows.Close()

	var result []SitemapURLRow
	for rows.Next() {
		var r SitemapURLRow
		if err := rows.Scan(&r.CrawlSessionID, &r.SitemapURL, &r.Loc, &r.LastMod, &r.ChangeFreq, &r.Priority); err != nil {
			return nil, fmt.Errorf("scanning sitemap url: %w", err)
		}
		result = append(result, r)
	}
	return result, nil
}

// Close closes the ClickHouse connection.
func (s *Store) Close() error {
	return s.conn.Close()
}
