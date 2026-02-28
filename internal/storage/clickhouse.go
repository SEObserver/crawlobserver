package storage

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/customtests"
	"github.com/google/uuid"
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
	for i, m := range Migrations {
		if m.Fn != nil {
			if err := m.Fn(ctx, s.conn); err != nil {
				return fmt.Errorf("migration %d (%s): %w", i+1, m.Name, err)
			}
		} else {
			if err := s.conn.Exec(ctx, m.DDL); err != nil {
				return fmt.Errorf("migration %d (%s): %w", i+1, m.Name, err)
			}
		}
	}
	return nil
}

// InsertSession inserts or updates a crawl session.
func (s *Store) InsertSession(ctx context.Context, session *CrawlSession) error {
	return s.conn.Exec(ctx, `
		INSERT INTO crawlobserver.crawl_sessions
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

// InsertLinks batch inserts link rows.
func (s *Store) InsertLinks(ctx context.Context, links []LinkRow) error {
	if len(links) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.links (
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

// CountPages returns the total number of pages for a session.
func (s *Store) CountPages(ctx context.Context, sessionID string) (uint64, error) {
	var count uint64
	err := s.conn.QueryRow(ctx, `SELECT count() FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID).Scan(&count)
	return count, err
}

// ListSessions retrieves crawl sessions, optionally filtered by project ID.
func (s *Store) ListSessions(ctx context.Context, projectID ...string) ([]CrawlSession, error) {
	query := `
		SELECT id, started_at, finished_at, status, seed_urls, config, pages_crawled, user_agent, project_id
		FROM crawlobserver.crawl_sessions FINAL`
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
func (s *Store) GetSession(ctx context.Context, sessionID string) (*CrawlSession, error) {
	row := s.conn.QueryRow(ctx, `
		SELECT id, started_at, finished_at, status, seed_urls, config, pages_crawled, user_agent, project_id
		FROM crawlobserver.crawl_sessions FINAL
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
	sess, err := s.GetSession(ctx, sessionID)
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
		FROM crawlobserver.links
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
		FROM crawlobserver.links
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
		FROM crawlobserver.links
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
		FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID)
	if err := row.Scan(&stats.TotalPages, &stats.AvgFetchMs, &stats.ErrorCount); err != nil {
		return nil, fmt.Errorf("querying page stats: %w", err)
	}

	// Link stats
	row = s.conn.QueryRow(ctx, `
		SELECT count(), countIf(is_internal = true), countIf(is_internal = false)
		FROM crawlobserver.links WHERE crawl_session_id = ?`, sessionID)
	if err := row.Scan(&stats.TotalLinks, &stats.InternalLinks, &stats.ExternalLinks); err != nil {
		return nil, fmt.Errorf("querying link stats: %w", err)
	}

	// Status code distribution
	rows, err := s.conn.Query(ctx, `
		SELECT status_code, count() FROM crawlobserver.pages
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
		SELECT depth, count() FROM crawlobserver.pages
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
		FROM crawlobserver.crawl_sessions FINAL
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
		SELECT url, pagerank FROM crawlobserver.pages
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

// --- Audit types and method ---

// AuditContent holds content-related audit metrics.
type AuditContent struct {
	Total               uint64 `json:"total"`
	HTMLPages           uint64 `json:"html_pages"`
	TitleMissing        uint64 `json:"title_missing"`
	TitleTooLong        uint64 `json:"title_too_long"`
	TitleTooShort       uint64 `json:"title_too_short"`
	TitleDuplicates     uint64 `json:"title_duplicates"`
	MetaDescMissing     uint64 `json:"meta_desc_missing"`
	MetaDescTooLong     uint64 `json:"meta_desc_too_long"`
	MetaDescTooShort    uint64 `json:"meta_desc_too_short"`
	H1Missing           uint64 `json:"h1_missing"`
	H1Multiple          uint64 `json:"h1_multiple"`
	ThinUnder100        uint64 `json:"thin_under_100"`
	Thin100300          uint64 `json:"thin_100_300"`
	ImagesTotal         uint64 `json:"images_total"`
	ImagesNoAltTotal    uint64 `json:"images_no_alt_total"`
	PagesWithImagesNoAlt uint64 `json:"pages_with_images_no_alt"`
}

// NoindexReason is a reason + count for non-indexable pages.
type NoindexReason struct {
	Reason string `json:"reason"`
	Count  uint64 `json:"count"`
}

// ContentTypeCount is a content type + count.
type ContentTypeCount struct {
	ContentType string `json:"content_type"`
	Count       uint64 `json:"count"`
}

// AuditTechnical holds technical audit metrics.
type AuditTechnical struct {
	Indexable           uint64             `json:"indexable"`
	NonIndexable        uint64             `json:"non_indexable"`
	CanonicalSelf       uint64             `json:"canonical_self"`
	CanonicalOther      uint64             `json:"canonical_other"`
	CanonicalMissing    uint64             `json:"canonical_missing"`
	HasRedirect         uint64             `json:"has_redirect"`
	RedirectChainsOver2 uint64             `json:"redirect_chains_over_2"`
	ResponseFast        uint64             `json:"response_fast"`
	ResponseOK          uint64             `json:"response_ok"`
	ResponseSlow        uint64             `json:"response_slow"`
	ResponseVerySlow    uint64             `json:"response_very_slow"`
	ErrorPages          uint64             `json:"error_pages"`
	NoindexReasons      []NoindexReason    `json:"noindex_reasons"`
	ContentTypes        []ContentTypeCount `json:"content_types"`
}

// ExternalDomain is a domain + link count.
type ExternalDomain struct {
	Domain string `json:"domain"`
	Count  uint64 `json:"count"`
}

// AnchorCount is an anchor text + count.
type AnchorCount struct {
	Anchor string `json:"anchor"`
	Count  uint64 `json:"count"`
}

// AuditLinks holds link audit metrics.
type AuditLinks struct {
	TotalInternal       uint64           `json:"total_internal"`
	TotalExternal       uint64           `json:"total_external"`
	ExternalNofollow    uint64           `json:"external_nofollow"`
	ExternalDofollow    uint64           `json:"external_dofollow"`
	PagesNoInternalOut  uint64           `json:"pages_no_internal_out"`
	PagesHighInternalOut uint64          `json:"pages_high_internal_out"`
	PagesNoExternal     uint64           `json:"pages_no_external"`
	BrokenInternal      uint64           `json:"broken_internal"`
	TopExternalDomains  []ExternalDomain `json:"top_external_domains"`
	TopAnchors          []AnchorCount    `json:"top_anchors"`
}

// DirectoryCount is a URL directory prefix + count.
type DirectoryCount struct {
	Directory string `json:"directory"`
	Count     uint64 `json:"count"`
}

// AuditStructure holds site structure audit metrics.
type AuditStructure struct {
	Directories []DirectoryCount `json:"directories"`
	OrphanPages uint64           `json:"orphan_pages"`
}

// AuditSitemaps holds sitemap coverage audit metrics.
type AuditSitemaps struct {
	InBoth           uint64 `json:"in_both"`
	CrawledOnly      uint64 `json:"crawled_only"`
	SitemapOnly      uint64 `json:"sitemap_only"`
	TotalSitemapURLs uint64 `json:"total_sitemap_urls"`
}

// LangCount is a language + count.
type LangCount struct {
	Lang  string `json:"lang"`
	Count uint64 `json:"count"`
}

// SchemaCount is a schema type + count.
type SchemaCount struct {
	SchemaType string `json:"schema_type"`
	Count      uint64 `json:"count"`
}

// AuditInternational holds international/schema audit metrics.
type AuditInternational struct {
	PagesWithHreflang  uint64        `json:"pages_with_hreflang"`
	PagesWithLang      uint64        `json:"pages_with_lang"`
	PagesWithSchema    uint64        `json:"pages_with_schema"`
	LangDistribution   []LangCount   `json:"lang_distribution"`
	SchemaDistribution []SchemaCount `json:"schema_distribution"`
}

// AuditResult is the combined audit result.
type AuditResult struct {
	Content       *AuditContent       `json:"content"`
	Technical     *AuditTechnical     `json:"technical"`
	Links         *AuditLinks         `json:"links"`
	Structure     *AuditStructure     `json:"structure"`
	Sitemaps      *AuditSitemaps      `json:"sitemaps"`
	International *AuditInternational `json:"international"`
}

// SessionAudit computes a comprehensive SEO audit for a crawl session.
func (s *Store) SessionAudit(ctx context.Context, sessionID string) (*AuditResult, error) {
	result := &AuditResult{}

	// --- Content audit ---
	content := &AuditContent{}
	row := s.conn.QueryRow(ctx, `
		SELECT count() AS total,
			countIf(content_type LIKE '%html%') AS html_pages,
			countIf(title = '') AS title_missing,
			countIf(title_length > 60) AS title_too_long,
			countIf(title_length > 0 AND title_length < 30) AS title_too_short,
			countIf(meta_description = '') AS meta_desc_missing,
			countIf(meta_desc_length > 160) AS meta_desc_too_long,
			countIf(meta_desc_length > 0 AND meta_desc_length < 70) AS meta_desc_too_short,
			countIf(length(h1) = 0) AS h1_missing,
			countIf(length(h1) > 1) AS h1_multiple,
			countIf(word_count < 100) AS thin_under_100,
			countIf(word_count >= 100 AND word_count < 300) AS thin_100_300,
			sum(images_count) AS images_total,
			sum(images_no_alt) AS images_no_alt_total,
			countIf(images_no_alt > 0) AS pages_with_images_no_alt
		FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID)
	if err := row.Scan(
		&content.Total, &content.HTMLPages,
		&content.TitleMissing, &content.TitleTooLong, &content.TitleTooShort,
		&content.MetaDescMissing, &content.MetaDescTooLong, &content.MetaDescTooShort,
		&content.H1Missing, &content.H1Multiple,
		&content.ThinUnder100, &content.Thin100300,
		&content.ImagesTotal, &content.ImagesNoAltTotal, &content.PagesWithImagesNoAlt,
	); err != nil {
		return nil, fmt.Errorf("audit content: %w", err)
	}

	// Title duplicates
	dupRow := s.conn.QueryRow(ctx, `
		SELECT sum(cnt - 1) FROM (
			SELECT title, count() AS cnt FROM crawlobserver.pages
			WHERE crawl_session_id = ? AND title != ''
			GROUP BY title HAVING cnt > 1
		)`, sessionID)
	_ = dupRow.Scan(&content.TitleDuplicates) // ignore error (may be 0 rows)
	result.Content = content

	// --- Technical audit ---
	tech := &AuditTechnical{}
	techRow := s.conn.QueryRow(ctx, `
		SELECT
			countIf(is_indexable = true) AS indexable,
			countIf(is_indexable = false) AS non_indexable,
			countIf(canonical_is_self = true) AS canonical_self,
			countIf(canonical != '' AND canonical_is_self = false) AS canonical_other,
			countIf(canonical = '') AS canonical_missing,
			countIf(length(redirect_chain) > 0) AS has_redirect,
			countIf(length(redirect_chain) > 2) AS redirect_chains_over_2,
			countIf(fetch_duration_ms < 200) AS response_fast,
			countIf(fetch_duration_ms >= 200 AND fetch_duration_ms < 500) AS response_ok,
			countIf(fetch_duration_ms >= 500 AND fetch_duration_ms < 1000) AS response_slow,
			countIf(fetch_duration_ms >= 1000) AS response_very_slow,
			countIf(error != '') AS error_pages
		FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID)
	if err := techRow.Scan(
		&tech.Indexable, &tech.NonIndexable,
		&tech.CanonicalSelf, &tech.CanonicalOther, &tech.CanonicalMissing,
		&tech.HasRedirect, &tech.RedirectChainsOver2,
		&tech.ResponseFast, &tech.ResponseOK, &tech.ResponseSlow, &tech.ResponseVerySlow,
		&tech.ErrorPages,
	); err != nil {
		return nil, fmt.Errorf("audit technical: %w", err)
	}

	// Noindex reasons
	niRows, err := s.conn.Query(ctx, `
		SELECT index_reason, count() AS cnt FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND is_indexable = false AND index_reason != ''
		GROUP BY index_reason ORDER BY cnt DESC`, sessionID)
	if err == nil {
		defer niRows.Close()
		for niRows.Next() {
			var nr NoindexReason
			if err := niRows.Scan(&nr.Reason, &nr.Count); err == nil {
				tech.NoindexReasons = append(tech.NoindexReasons, nr)
			}
		}
	}

	// Content types
	ctRows, err := s.conn.Query(ctx, `
		SELECT content_type, count() AS cnt FROM crawlobserver.pages
		WHERE crawl_session_id = ?
		GROUP BY content_type ORDER BY cnt DESC LIMIT 20`, sessionID)
	if err == nil {
		defer ctRows.Close()
		for ctRows.Next() {
			var ct ContentTypeCount
			if err := ctRows.Scan(&ct.ContentType, &ct.Count); err == nil {
				tech.ContentTypes = append(tech.ContentTypes, ct)
			}
		}
	}
	result.Technical = tech

	// --- Links audit ---
	links := &AuditLinks{}
	linkRow := s.conn.QueryRow(ctx, `
		SELECT
			countIf(is_internal = true) AS total_internal,
			countIf(is_internal = false) AS total_external,
			countIf(is_internal = false AND rel LIKE '%nofollow%') AS external_nofollow,
			countIf(is_internal = false AND (rel = '' OR rel NOT LIKE '%nofollow%')) AS external_dofollow
		FROM crawlobserver.links WHERE crawl_session_id = ?`, sessionID)
	if err := linkRow.Scan(&links.TotalInternal, &links.TotalExternal, &links.ExternalNofollow, &links.ExternalDofollow); err != nil {
		return nil, fmt.Errorf("audit links: %w", err)
	}

	// Pages link distribution
	pageDistRow := s.conn.QueryRow(ctx, `
		SELECT
			countIf(internal_links_out = 0) AS pages_no_internal_out,
			countIf(internal_links_out > 100) AS pages_high_internal_out,
			countIf(external_links_out = 0) AS pages_no_external
		FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID)
	_ = pageDistRow.Scan(&links.PagesNoInternalOut, &links.PagesHighInternalOut, &links.PagesNoExternal)

	// Broken internal links
	brokenRow := s.conn.QueryRow(ctx, `
		SELECT count(DISTINCT target_url) FROM crawlobserver.links
		WHERE crawl_session_id = ? AND is_internal = true
		AND target_url NOT IN (SELECT url FROM crawlobserver.pages WHERE crawl_session_id = ?)`,
		sessionID, sessionID)
	_ = brokenRow.Scan(&links.BrokenInternal)

	// Top external domains
	edRows, err := s.conn.Query(ctx, `
		SELECT domain(target_url) AS d, count() AS cnt FROM crawlobserver.links
		WHERE crawl_session_id = ? AND is_internal = false
		GROUP BY d ORDER BY cnt DESC LIMIT 20`, sessionID)
	if err == nil {
		defer edRows.Close()
		for edRows.Next() {
			var ed ExternalDomain
			if err := edRows.Scan(&ed.Domain, &ed.Count); err == nil {
				links.TopExternalDomains = append(links.TopExternalDomains, ed)
			}
		}
	}

	// Top anchor texts
	anRows, err := s.conn.Query(ctx, `
		SELECT anchor_text, count() AS cnt FROM crawlobserver.links
		WHERE crawl_session_id = ? AND is_internal = true AND anchor_text != ''
		GROUP BY anchor_text ORDER BY cnt DESC LIMIT 20`, sessionID)
	if err == nil {
		defer anRows.Close()
		for anRows.Next() {
			var ac AnchorCount
			if err := anRows.Scan(&ac.Anchor, &ac.Count); err == nil {
				links.TopAnchors = append(links.TopAnchors, ac)
			}
		}
	}
	result.Links = links

	// --- Structure audit ---
	structure := &AuditStructure{}

	// Directories (by URL path prefix up to 2nd segment)
	dirRows, err := s.conn.Query(ctx, `
		SELECT
			concat('/', arrayStringConcat(arraySlice(splitByChar('/', pathFull(url)), 2, 1), '/'), '/') AS dir,
			count() AS cnt
		FROM crawlobserver.pages
		WHERE crawl_session_id = ?
		GROUP BY dir ORDER BY cnt DESC LIMIT 50`, sessionID)
	if err == nil {
		defer dirRows.Close()
		for dirRows.Next() {
			var dc DirectoryCount
			if err := dirRows.Scan(&dc.Directory, &dc.Count); err == nil {
				structure.Directories = append(structure.Directories, dc)
			}
		}
	}

	// Orphan pages (not targeted by any internal link)
	orphanRow := s.conn.QueryRow(ctx, `
		SELECT count() FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND url NOT IN (
			SELECT DISTINCT target_url FROM crawlobserver.links
			WHERE crawl_session_id = ? AND is_internal = true
		)`, sessionID, sessionID)
	_ = orphanRow.Scan(&structure.OrphanPages)
	result.Structure = structure

	// --- Sitemaps audit ---
	sitemaps := &AuditSitemaps{}
	smRow := s.conn.QueryRow(ctx, `
		SELECT count(DISTINCT url) FROM crawlobserver.sitemap_urls
		WHERE crawl_session_id = ?`, sessionID)
	_ = smRow.Scan(&sitemaps.TotalSitemapURLs)

	if sitemaps.TotalSitemapURLs > 0 {
		covRow := s.conn.QueryRow(ctx, `
			SELECT
				countIf(in_crawl AND in_sitemap) AS in_both,
				countIf(in_crawl AND NOT in_sitemap) AS crawled_only,
				countIf(NOT in_crawl AND in_sitemap) AS sitemap_only
			FROM (
				SELECT
					url AS u,
					url IN (SELECT url FROM crawlobserver.pages WHERE crawl_session_id = ?) AS in_crawl,
					1 AS in_sitemap
				FROM crawlobserver.sitemap_urls WHERE crawl_session_id = ?
				UNION ALL
				SELECT
					url AS u,
					1 AS in_crawl,
					url IN (SELECT url FROM crawlobserver.sitemap_urls WHERE crawl_session_id = ?) AS in_sitemap
				FROM crawlobserver.pages WHERE crawl_session_id = ?
			) GROUP BY u
			HAVING 1`, sessionID, sessionID, sessionID, sessionID)
		// Simplified: just do two counts
		_ = covRow.Scan(&sitemaps.InBoth, &sitemaps.CrawledOnly, &sitemaps.SitemapOnly)
	}
	// Simpler coverage approach
	if sitemaps.TotalSitemapURLs > 0 {
		var inBoth uint64
		ibRow := s.conn.QueryRow(ctx, `
			SELECT count() FROM (
				SELECT DISTINCT url FROM crawlobserver.sitemap_urls WHERE crawl_session_id = ?
			) AS sm WHERE sm.url IN (
				SELECT url FROM crawlobserver.pages WHERE crawl_session_id = ?
			)`, sessionID, sessionID)
		if ibRow.Scan(&inBoth) == nil {
			sitemaps.InBoth = inBoth
			var totalCrawled uint64
			tcRow := s.conn.QueryRow(ctx, `SELECT count() FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID)
			_ = tcRow.Scan(&totalCrawled)
			sitemaps.CrawledOnly = totalCrawled - inBoth
			sitemaps.SitemapOnly = sitemaps.TotalSitemapURLs - inBoth
		}
	}
	result.Sitemaps = sitemaps

	// --- International audit ---
	intl := &AuditInternational{}
	intlRow := s.conn.QueryRow(ctx, `
		SELECT
			countIf(length(hreflang) > 0) AS pages_with_hreflang,
			countIf(lang != '') AS pages_with_lang,
			countIf(length(schema_types) > 0) AS pages_with_schema
		FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID)
	_ = intlRow.Scan(&intl.PagesWithHreflang, &intl.PagesWithLang, &intl.PagesWithSchema)

	// Lang distribution
	langRows, err := s.conn.Query(ctx, `
		SELECT lang, count() AS cnt FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND lang != ''
		GROUP BY lang ORDER BY cnt DESC LIMIT 20`, sessionID)
	if err == nil {
		defer langRows.Close()
		for langRows.Next() {
			var lc LangCount
			if err := langRows.Scan(&lc.Lang, &lc.Count); err == nil {
				intl.LangDistribution = append(intl.LangDistribution, lc)
			}
		}
	}

	// Schema distribution
	schemaRows, err := s.conn.Query(ctx, `
		SELECT arrayJoin(schema_types) AS st, count() AS cnt FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND length(schema_types) > 0
		GROUP BY st ORDER BY cnt DESC LIMIT 20`, sessionID)
	if err == nil {
		defer schemaRows.Close()
		for schemaRows.Next() {
			var sc SchemaCount
			if err := schemaRows.Scan(&sc.SchemaType, &sc.Count); err == nil {
				intl.SchemaDistribution = append(intl.SchemaDistribution, sc)
			}
		}
	}
	result.International = intl

	return result, nil
}

// CompareStats retrieves side-by-side stats for two sessions.
func (s *Store) CompareStats(ctx context.Context, sessionA, sessionB string) (*CompareStatsResult, error) {
	statsA, err := s.SessionStats(ctx, sessionA)
	if err != nil {
		return nil, fmt.Errorf("stats for session A: %w", err)
	}
	statsB, err := s.SessionStats(ctx, sessionB)
	if err != nil {
		return nil, fmt.Errorf("stats for session B: %w", err)
	}
	return &CompareStatsResult{
		SessionA: sessionA,
		SessionB: sessionB,
		StatsA:   statsA,
		StatsB:   statsB,
	}, nil
}

// ComparePages returns paginated page diffs between two sessions.
func (s *Store) ComparePages(ctx context.Context, sessionA, sessionB, diffType string, limit, offset int) (*PageDiffResult, error) {
	result := &PageDiffResult{}

	// Count totals for all three diff types
	countRow := s.conn.QueryRow(ctx, `
		SELECT
			countIf(b.url != '' AND a.url = '') AS added,
			countIf(a.url != '' AND b.url = '') AS removed,
			countIf(a.url != '' AND b.url != '' AND (
				a.status_code != b.status_code OR
				a.title != b.title OR
				a.canonical != b.canonical OR
				a.is_indexable != b.is_indexable OR
				a.meta_description != b.meta_description OR
				a.h1[1] != b.h1[1] OR
				a.depth != b.depth OR
				(abs(toInt64(a.word_count) - toInt64(b.word_count)) > 50 AND abs(toInt64(a.word_count) - toInt64(b.word_count)) > toInt64(a.word_count) / 10) OR
				abs(a.pagerank - b.pagerank) > 0.01
			)) AS changed
		FROM (
			SELECT url, status_code, title, canonical, is_indexable, meta_description, h1, depth, word_count, pagerank
			FROM crawlobserver.pages WHERE crawl_session_id = ?
		) a
		FULL OUTER JOIN (
			SELECT url, status_code, title, canonical, is_indexable, meta_description, h1, depth, word_count, pagerank
			FROM crawlobserver.pages WHERE crawl_session_id = ?
		) b USING (url)`, sessionA, sessionB)
	if err := countRow.Scan(&result.TotalAdded, &result.TotalRemoved, &result.TotalChanged); err != nil {
		return nil, fmt.Errorf("counting page diffs: %w", err)
	}

	// Paginated query based on diffType
	var query string
	switch diffType {
	case "added":
		query = `
			SELECT b.url,
				0, '', '', false, 0, 0, 0, '', '',
				b.status_code, b.title, b.canonical, b.is_indexable, b.word_count, b.depth, b.pagerank, b.meta_description, b.h1[1]
			FROM crawlobserver.pages b
			LEFT JOIN crawlobserver.pages a ON a.url = b.url AND a.crawl_session_id = ?
			WHERE b.crawl_session_id = ? AND a.url = ''
			ORDER BY b.url
			LIMIT ? OFFSET ?`
		rows, err := s.conn.Query(ctx, query, sessionA, sessionB, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("querying added pages: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var p PageDiffRow
			p.DiffType = "added"
			if err := rows.Scan(
				&p.URL,
				&p.StatusCodeA, &p.TitleA, &p.CanonicalA, &p.IsIndexableA, &p.WordCountA, &p.DepthA, &p.PageRankA, &p.MetaDescriptionA, &p.H1A,
				&p.StatusCodeB, &p.TitleB, &p.CanonicalB, &p.IsIndexableB, &p.WordCountB, &p.DepthB, &p.PageRankB, &p.MetaDescriptionB, &p.H1B,
			); err != nil {
				return nil, fmt.Errorf("scanning added page: %w", err)
			}
			result.Pages = append(result.Pages, p)
		}

	case "removed":
		query = `
			SELECT a.url,
				a.status_code, a.title, a.canonical, a.is_indexable, a.word_count, a.depth, a.pagerank, a.meta_description, a.h1[1],
				0, '', '', false, 0, 0, 0, '', ''
			FROM crawlobserver.pages a
			LEFT JOIN crawlobserver.pages b ON a.url = b.url AND b.crawl_session_id = ?
			WHERE a.crawl_session_id = ? AND b.url = ''
			ORDER BY a.url
			LIMIT ? OFFSET ?`
		rows, err := s.conn.Query(ctx, query, sessionB, sessionA, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("querying removed pages: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var p PageDiffRow
			p.DiffType = "removed"
			if err := rows.Scan(
				&p.URL,
				&p.StatusCodeA, &p.TitleA, &p.CanonicalA, &p.IsIndexableA, &p.WordCountA, &p.DepthA, &p.PageRankA, &p.MetaDescriptionA, &p.H1A,
				&p.StatusCodeB, &p.TitleB, &p.CanonicalB, &p.IsIndexableB, &p.WordCountB, &p.DepthB, &p.PageRankB, &p.MetaDescriptionB, &p.H1B,
			); err != nil {
				return nil, fmt.Errorf("scanning removed page: %w", err)
			}
			result.Pages = append(result.Pages, p)
		}

	case "changed":
		query = `
			SELECT a.url,
				a.status_code, a.title, a.canonical, a.is_indexable, a.word_count, a.depth, a.pagerank, a.meta_description, a.h1[1],
				b.status_code, b.title, b.canonical, b.is_indexable, b.word_count, b.depth, b.pagerank, b.meta_description, b.h1[1]
			FROM crawlobserver.pages a
			INNER JOIN crawlobserver.pages b ON a.url = b.url AND b.crawl_session_id = ?
			WHERE a.crawl_session_id = ? AND (
				a.status_code != b.status_code OR
				a.title != b.title OR
				a.canonical != b.canonical OR
				a.is_indexable != b.is_indexable OR
				a.meta_description != b.meta_description OR
				a.h1[1] != b.h1[1] OR
				a.depth != b.depth OR
				(abs(toInt64(a.word_count) - toInt64(b.word_count)) > 50 AND abs(toInt64(a.word_count) - toInt64(b.word_count)) > toInt64(a.word_count) / 10) OR
				abs(a.pagerank - b.pagerank) > 0.01
			)
			ORDER BY a.url
			LIMIT ? OFFSET ?`
		rows, err := s.conn.Query(ctx, query, sessionB, sessionA, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("querying changed pages: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var p PageDiffRow
			p.DiffType = "changed"
			if err := rows.Scan(
				&p.URL,
				&p.StatusCodeA, &p.TitleA, &p.CanonicalA, &p.IsIndexableA, &p.WordCountA, &p.DepthA, &p.PageRankA, &p.MetaDescriptionA, &p.H1A,
				&p.StatusCodeB, &p.TitleB, &p.CanonicalB, &p.IsIndexableB, &p.WordCountB, &p.DepthB, &p.PageRankB, &p.MetaDescriptionB, &p.H1B,
			); err != nil {
				return nil, fmt.Errorf("scanning changed page: %w", err)
			}
			result.Pages = append(result.Pages, p)
		}
	}

	return result, nil
}

// CompareLinks returns paginated link diffs between two sessions.
func (s *Store) CompareLinks(ctx context.Context, sessionA, sessionB, diffType string, limit, offset int) (*LinkDiffResult, error) {
	result := &LinkDiffResult{}

	// Count totals
	countRow := s.conn.QueryRow(ctx, `
		SELECT
			countIf(b.source_url != '' AND a.source_url = '') AS added,
			countIf(a.source_url != '' AND b.source_url = '') AS removed
		FROM (
			SELECT source_url, target_url
			FROM crawlobserver.links WHERE crawl_session_id = ? AND is_internal = true
		) a
		FULL OUTER JOIN (
			SELECT source_url, target_url
			FROM crawlobserver.links WHERE crawl_session_id = ? AND is_internal = true
		) b USING (source_url, target_url)`, sessionA, sessionB)
	if err := countRow.Scan(&result.TotalAdded, &result.TotalRemoved); err != nil {
		return nil, fmt.Errorf("counting link diffs: %w", err)
	}

	switch diffType {
	case "added":
		rows, err := s.conn.Query(ctx, `
			SELECT b.source_url, b.target_url, b.anchor_text
			FROM crawlobserver.links b
			LEFT JOIN crawlobserver.links a
				ON a.source_url = b.source_url AND a.target_url = b.target_url
				AND a.crawl_session_id = ? AND a.is_internal = true
			WHERE b.crawl_session_id = ? AND b.is_internal = true AND a.source_url = ''
			ORDER BY b.source_url, b.target_url
			LIMIT ? OFFSET ?`, sessionA, sessionB, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("querying added links: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var l LinkDiffRow
			l.DiffType = "added"
			if err := rows.Scan(&l.SourceURL, &l.TargetURL, &l.AnchorText); err != nil {
				return nil, fmt.Errorf("scanning added link: %w", err)
			}
			result.Links = append(result.Links, l)
		}

	case "removed":
		rows, err := s.conn.Query(ctx, `
			SELECT a.source_url, a.target_url, a.anchor_text
			FROM crawlobserver.links a
			LEFT JOIN crawlobserver.links b
				ON a.source_url = b.source_url AND a.target_url = b.target_url
				AND b.crawl_session_id = ? AND b.is_internal = true
			WHERE a.crawl_session_id = ? AND a.is_internal = true AND b.source_url = ''
			ORDER BY a.source_url, a.target_url
			LIMIT ? OFFSET ?`, sessionB, sessionA, limit, offset)
		if err != nil {
			return nil, fmt.Errorf("querying removed links: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var l LinkDiffRow
			l.DiffType = "removed"
			if err := rows.Scan(&l.SourceURL, &l.TargetURL, &l.AnchorText); err != nil {
				return nil, fmt.Errorf("scanning removed link: %w", err)
			}
			result.Links = append(result.Links, l)
		}
	}

	return result, nil
}

// DeleteSession deletes a crawl session and all its associated data.
// Uses DROP PARTITION for instant deletion on partitioned tables.
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	// Drop partition on data tables (partitioned by crawl_session_id)
	dataTables := []string{"pages", "links", "robots_txt", "sitemaps", "sitemap_urls", "external_link_checks", "page_resource_checks", "page_resource_refs"}
	for _, table := range dataTables {
		q := fmt.Sprintf("ALTER TABLE crawlobserver.%s DROP PARTITION ID ?", table)
		if err := s.conn.Exec(ctx, q, sessionID); err != nil {
			return fmt.Errorf("dropping partition on %s: %w", table, err)
		}
	}

	// crawl_sessions is not partitioned by session, use regular DELETE
	if err := s.conn.Exec(ctx, `ALTER TABLE crawlobserver.crawl_sessions DELETE WHERE id = ?`, sessionID); err != nil {
		return fmt.Errorf("deleting session row: %w", err)
	}

	return nil
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

// StorageStats retrieves disk usage and row counts for all crawlobserver tables.
func (s *Store) StorageStats(ctx context.Context) (*StorageStatsResult, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT table, sum(bytes_on_disk), sum(rows)
		FROM system.parts
		WHERE database = 'crawlobserver' AND active = 1
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

// SessionStorageStats returns bytes on disk per crawl session,
// computed from system.parts partitions across all data tables.
func (s *Store) SessionStorageStats(ctx context.Context) (map[string]uint64, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT partition AS session_id, sum(bytes_on_disk) AS bytes
		FROM system.parts
		WHERE database = 'crawlobserver' AND active = 1 AND table != 'crawl_sessions'
		GROUP BY partition`)
	if err != nil {
		return nil, fmt.Errorf("querying session storage stats: %w", err)
	}
	defer rows.Close()

	result := make(map[string]uint64)
	for rows.Next() {
		var sessionID string
		var bytes uint64
		if err := rows.Scan(&sessionID, &bytes); err != nil {
			return nil, fmt.Errorf("scanning session storage stats: %w", err)
		}
		result[sessionID] = bytes
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
		FROM crawlobserver.pages
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
		FROM crawlobserver.links
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
		FROM crawlobserver.links
		WHERE crawl_session_id = ? AND (source_url = ? OR target_url = ?)`,
		url, url, sessionID, url, url)
	if err := countRow.Scan(&result.OutLinksCount, &result.InLinksCount); err != nil {
		return nil, fmt.Errorf("querying link counts: %w", err)
	}

	// Outbound links (all, capped at 1000)
	outRows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM crawlobserver.links
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

// isValidUUID checks whether s is a valid UUID string.
func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// ComputePageRank computes internal PageRank for all pages in a session.
// Uses uint32 IDs for memory efficiency and iterative power method.
func (s *Store) ComputePageRank(ctx context.Context, sessionID string) error {
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

	// 6. Write back to ClickHouse via batched ALTER TABLE UPDATE with multiIf
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}

	const chunkSize = 500
	for i := 0; i < int(n); i += chunkSize {
		end := i + chunkSize
		if end > int(n) {
			end = int(n)
		}

		// Build multiIf(url = ?, val, url = ?, val, ..., pagerank)
		// Args order must match ? order in query: multiIf args, then sessionID, then IN list
		var multiIfArgs []interface{}
		var inArgs []interface{}
		var conditions []string
		var urlList []string
		for j := i; j < end; j++ {
			conditions = append(conditions, "url = ?, ?")
			multiIfArgs = append(multiIfArgs, idToURL[j], rank[j])
			urlList = append(urlList, "?")
			inArgs = append(inArgs, idToURL[j])
		}

		multiIfExpr := "multiIf(" + strings.Join(conditions, ", ") + ", pagerank)"
		inList := strings.Join(urlList, ", ")

		query := fmt.Sprintf(`ALTER TABLE crawlobserver.pages UPDATE
			pagerank = %s
			WHERE crawl_session_id = ? AND url IN (%s)`,
			multiIfExpr, inList)

		var args []interface{}
		args = append(args, multiIfArgs...)
		args = append(args, sessionID)
		args = append(args, inArgs...)

		if err := s.conn.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("updating pagerank batch at offset %d: %w", i, err)
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
	result := ComputeBFSDepths(seedURLs, crawledSet, adj)
	depths := result.Depths
	foundOn := result.FoundOn

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

// InsertRobotsData batch inserts robots.txt rows.
func (s *Store) InsertRobotsData(ctx context.Context, rows []RobotsRow) error {
	if len(rows) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.robots_txt (
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
		FROM crawlobserver.robots_txt FINAL
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
		FROM crawlobserver.robots_txt FINAL
		WHERE crawl_session_id = ? AND host = ?
		LIMIT 1`, sessionID, host)

	var r RobotsRow
	if err := row.Scan(&r.CrawlSessionID, &r.Host, &r.StatusCode, &r.Content, &r.FetchedAt); err != nil {
		return nil, fmt.Errorf("querying robots content: %w", err)
	}
	return &r, nil
}

// GetURLsByHost returns all distinct URLs for a given host in a session.
func (s *Store) GetURLsByHost(ctx context.Context, sessionID, host string) ([]string, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT DISTINCT url
		FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND url LIKE ?
		ORDER BY url`, sessionID, host+"/%")
	if err != nil {
		return nil, fmt.Errorf("querying urls by host: %w", err)
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("scanning url: %w", err)
		}
		urls = append(urls, u)
	}
	return urls, nil
}

// InsertSitemaps inserts sitemap rows.
func (s *Store) InsertSitemaps(ctx context.Context, rows []SitemapRow) error {
	if len(rows) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.sitemaps (
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
		INSERT INTO crawlobserver.sitemap_urls (
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
		FROM crawlobserver.sitemaps FINAL
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
		FROM crawlobserver.sitemap_urls FINAL
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

// --- GSC Analytics & Inspection ---

type GSCQueryRow struct {
	Query       string  `json:"query"`
	Clicks      uint64  `json:"clicks"`
	Impressions uint64  `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

type GSCPageRow struct {
	Page        string  `json:"page"`
	Clicks      uint64  `json:"clicks"`
	Impressions uint64  `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

type GSCCountryRow struct {
	Country     string  `json:"country"`
	Clicks      uint64  `json:"clicks"`
	Impressions uint64  `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

type GSCDeviceRow struct {
	Device      string  `json:"device"`
	Clicks      uint64  `json:"clicks"`
	Impressions uint64  `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

type GSCOverviewStats struct {
	TotalClicks      uint64  `json:"total_clicks"`
	TotalImpressions uint64  `json:"total_impressions"`
	AvgCTR           float64 `json:"avg_ctr"`
	AvgPosition      float64 `json:"avg_position"`
	DateMin          string  `json:"date_min"`
	DateMax          string  `json:"date_max"`
	TotalQueries     uint64  `json:"total_queries"`
	TotalPages       uint64  `json:"total_pages"`
}

type GSCTimelineRow struct {
	Date        string `json:"date"`
	Clicks      uint64 `json:"clicks"`
	Impressions uint64 `json:"impressions"`
}

type GSCInspectionRow struct {
	URL               string `json:"url"`
	Verdict           string `json:"verdict"`
	CoverageState     string `json:"coverage_state"`
	IndexingState     string `json:"indexing_state"`
	RobotsTxtState    string `json:"robots_txt_state"`
	LastCrawlTime     string `json:"last_crawl_time"`
	CrawledAs         string `json:"crawled_as"`
	CanonicalURL      string `json:"canonical_url"`
	IsGoogleCanonical bool   `json:"is_google_canonical"`
	MobileUsability   string `json:"mobile_usability"`
	RichResultsItems  uint16 `json:"rich_results_items"`
}

func (s *Store) InsertGSCAnalytics(ctx context.Context, projectID string, rows []GSCAnalyticsInsertRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.gsc_analytics (
			project_id, date, query, page, country, device,
			clicks, impressions, ctr, position, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing gsc_analytics batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			projectID, r.Date, r.Query, r.Page, r.Country, r.Device,
			r.Clicks, r.Impressions, r.CTR, r.Position, now,
		); err != nil {
			return fmt.Errorf("appending gsc_analytics row: %w", err)
		}
	}
	return batch.Send()
}

// GSCAnalyticsInsertRow is the input row for batch inserts.
type GSCAnalyticsInsertRow struct {
	Date        time.Time
	Query       string
	Page        string
	Country     string
	Device      string
	Clicks      uint32
	Impressions uint32
	CTR         float32
	Position    float32
}

func (s *Store) InsertGSCInspection(ctx context.Context, projectID string, rows []GSCInspectionInsertRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.gsc_inspection (
			project_id, url, verdict, coverage_state, indexing_state, robots_txt_state,
			last_crawl_time, crawled_as, canonical_url, is_google_canonical,
			mobile_usability, rich_results_items, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing gsc_inspection batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			projectID, r.URL, r.Verdict, r.CoverageState, r.IndexingState, r.RobotsTxtState,
			r.LastCrawlTime, r.CrawledAs, r.CanonicalURL, r.IsGoogleCanonical,
			r.MobileUsability, r.RichResultsItems, now,
		); err != nil {
			return fmt.Errorf("appending gsc_inspection row: %w", err)
		}
	}
	return batch.Send()
}

type GSCInspectionInsertRow struct {
	URL               string
	Verdict           string
	CoverageState     string
	IndexingState     string
	RobotsTxtState    string
	LastCrawlTime     time.Time
	CrawledAs         string
	CanonicalURL      string
	IsGoogleCanonical bool
	MobileUsability   string
	RichResultsItems  uint16
}

func (s *Store) GSCOverview(ctx context.Context, projectID string) (*GSCOverviewStats, error) {
	var stats GSCOverviewStats
	err := s.conn.QueryRow(ctx, `
		SELECT
			sum(clicks), sum(impressions),
			if(sum(impressions) > 0, sum(clicks) / sum(impressions), 0),
			if(sum(impressions) > 0, sum(position * impressions) / sum(impressions), 0),
			toString(min(date)), toString(max(date)),
			uniqExact(query), uniqExact(page)
		FROM crawlobserver.gsc_analytics FINAL
		WHERE project_id = ?`, projectID).Scan(
		&stats.TotalClicks, &stats.TotalImpressions,
		&stats.AvgCTR, &stats.AvgPosition,
		&stats.DateMin, &stats.DateMax,
		&stats.TotalQueries, &stats.TotalPages,
	)
	if err != nil {
		return nil, fmt.Errorf("querying gsc overview: %w", err)
	}
	return &stats, nil
}

func (s *Store) GSCTopQueries(ctx context.Context, projectID string, limit, offset int) ([]GSCQueryRow, int, error) {
	var total uint64
	if err := s.conn.QueryRow(ctx, `
		SELECT uniqExact(query) FROM crawlobserver.gsc_analytics FINAL WHERE project_id = ?`, projectID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting gsc queries: %w", err)
	}

	rows, err := s.conn.Query(ctx, `
		SELECT query, sum(clicks), sum(impressions),
			if(sum(impressions) > 0, sum(clicks) / sum(impressions), 0),
			if(sum(impressions) > 0, sum(position * impressions) / sum(impressions), 0)
		FROM crawlobserver.gsc_analytics FINAL
		WHERE project_id = ?
		GROUP BY query
		ORDER BY sum(clicks) DESC
		LIMIT ? OFFSET ?`, projectID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying gsc top queries: %w", err)
	}
	defer rows.Close()

	var result []GSCQueryRow
	for rows.Next() {
		var r GSCQueryRow
		if err := rows.Scan(&r.Query, &r.Clicks, &r.Impressions, &r.CTR, &r.Position); err != nil {
			return nil, 0, fmt.Errorf("scanning gsc query row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []GSCQueryRow{}
	}
	return result, int(total), nil
}

func (s *Store) GSCTopPages(ctx context.Context, projectID string, limit, offset int) ([]GSCPageRow, int, error) {
	var total uint64
	if err := s.conn.QueryRow(ctx, `
		SELECT uniqExact(page) FROM crawlobserver.gsc_analytics FINAL WHERE project_id = ?`, projectID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting gsc pages: %w", err)
	}

	rows, err := s.conn.Query(ctx, `
		SELECT page, sum(clicks), sum(impressions),
			if(sum(impressions) > 0, sum(clicks) / sum(impressions), 0),
			if(sum(impressions) > 0, sum(position * impressions) / sum(impressions), 0)
		FROM crawlobserver.gsc_analytics FINAL
		WHERE project_id = ?
		GROUP BY page
		ORDER BY sum(clicks) DESC
		LIMIT ? OFFSET ?`, projectID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying gsc top pages: %w", err)
	}
	defer rows.Close()

	var result []GSCPageRow
	for rows.Next() {
		var r GSCPageRow
		if err := rows.Scan(&r.Page, &r.Clicks, &r.Impressions, &r.CTR, &r.Position); err != nil {
			return nil, 0, fmt.Errorf("scanning gsc page row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []GSCPageRow{}
	}
	return result, int(total), nil
}

func (s *Store) GSCByCountry(ctx context.Context, projectID string) ([]GSCCountryRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT country, sum(clicks), sum(impressions),
			if(sum(impressions) > 0, sum(clicks) / sum(impressions), 0),
			if(sum(impressions) > 0, sum(position * impressions) / sum(impressions), 0)
		FROM crawlobserver.gsc_analytics FINAL
		WHERE project_id = ?
		GROUP BY country
		ORDER BY sum(clicks) DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("querying gsc by country: %w", err)
	}
	defer rows.Close()

	var result []GSCCountryRow
	for rows.Next() {
		var r GSCCountryRow
		if err := rows.Scan(&r.Country, &r.Clicks, &r.Impressions, &r.CTR, &r.Position); err != nil {
			return nil, fmt.Errorf("scanning gsc country row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []GSCCountryRow{}
	}
	return result, nil
}

func (s *Store) GSCByDevice(ctx context.Context, projectID string) ([]GSCDeviceRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT device, sum(clicks), sum(impressions),
			if(sum(impressions) > 0, sum(clicks) / sum(impressions), 0),
			if(sum(impressions) > 0, sum(position * impressions) / sum(impressions), 0)
		FROM crawlobserver.gsc_analytics FINAL
		WHERE project_id = ?
		GROUP BY device
		ORDER BY sum(clicks) DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("querying gsc by device: %w", err)
	}
	defer rows.Close()

	var result []GSCDeviceRow
	for rows.Next() {
		var r GSCDeviceRow
		if err := rows.Scan(&r.Device, &r.Clicks, &r.Impressions, &r.CTR, &r.Position); err != nil {
			return nil, fmt.Errorf("scanning gsc device row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []GSCDeviceRow{}
	}
	return result, nil
}

func (s *Store) GSCTimeline(ctx context.Context, projectID string) ([]GSCTimelineRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT toString(date), sum(clicks), sum(impressions)
		FROM crawlobserver.gsc_analytics FINAL
		WHERE project_id = ?
		GROUP BY date
		ORDER BY date`, projectID)
	if err != nil {
		return nil, fmt.Errorf("querying gsc timeline: %w", err)
	}
	defer rows.Close()

	var result []GSCTimelineRow
	for rows.Next() {
		var r GSCTimelineRow
		if err := rows.Scan(&r.Date, &r.Clicks, &r.Impressions); err != nil {
			return nil, fmt.Errorf("scanning gsc timeline row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []GSCTimelineRow{}
	}
	return result, nil
}

func (s *Store) GSCInspectionResults(ctx context.Context, projectID string, limit, offset int) ([]GSCInspectionRow, int, error) {
	var total uint64
	if err := s.conn.QueryRow(ctx, `
		SELECT count() FROM crawlobserver.gsc_inspection FINAL WHERE project_id = ?`, projectID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting gsc inspections: %w", err)
	}

	rows, err := s.conn.Query(ctx, `
		SELECT url, verdict, coverage_state, indexing_state, robots_txt_state,
			toString(last_crawl_time), crawled_as, canonical_url, is_google_canonical,
			mobile_usability, rich_results_items
		FROM crawlobserver.gsc_inspection FINAL
		WHERE project_id = ?
		ORDER BY url
		LIMIT ? OFFSET ?`, projectID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying gsc inspections: %w", err)
	}
	defer rows.Close()

	var result []GSCInspectionRow
	for rows.Next() {
		var r GSCInspectionRow
		if err := rows.Scan(&r.URL, &r.Verdict, &r.CoverageState, &r.IndexingState,
			&r.RobotsTxtState, &r.LastCrawlTime, &r.CrawledAs, &r.CanonicalURL,
			&r.IsGoogleCanonical, &r.MobileUsability, &r.RichResultsItems); err != nil {
			return nil, 0, fmt.Errorf("scanning gsc inspection row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []GSCInspectionRow{}
	}
	return result, int(total), nil
}

func (s *Store) DeleteGSCData(ctx context.Context, projectID string) error {
	if err := s.conn.Exec(ctx, `ALTER TABLE crawlobserver.gsc_analytics DELETE WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("deleting gsc analytics: %w", err)
	}
	if err := s.conn.Exec(ctx, `ALTER TABLE crawlobserver.gsc_inspection DELETE WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("deleting gsc inspection: %w", err)
	}
	return nil
}

// --- Custom Tests ---

// PageHTMLRow is a url+html pair streamed from ClickHouse.
type PageHTMLRow struct {
	URL  string
	HTML string
}

// buildRuleExpr returns a ClickHouse SQL expression for a single rule.
func buildRuleExpr(r customtests.TestRule) string {
	v := strings.ReplaceAll(r.Value, "'", "\\'")
	ex := strings.ReplaceAll(r.Extra, "'", "\\'")
	switch r.Type {
	case customtests.StringContains:
		return fmt.Sprintf("position(body_html, '%s') > 0", v)
	case customtests.StringNotContains:
		return fmt.Sprintf("position(body_html, '%s') = 0", v)
	case customtests.RegexMatch:
		return fmt.Sprintf("match(body_html, '%s')", v)
	case customtests.RegexNotMatch:
		return fmt.Sprintf("NOT match(body_html, '%s')", v)
	case customtests.HeaderExists:
		return fmt.Sprintf("mapContains(headers, '%s')", v)
	case customtests.HeaderNotExists:
		return fmt.Sprintf("NOT mapContains(headers, '%s')", v)
	case customtests.HeaderContains:
		return fmt.Sprintf("mapContains(headers, '%s') AND position(headers['%s'], '%s') > 0", v, v, ex)
	case customtests.HeaderRegex:
		return fmt.Sprintf("mapContains(headers, '%s') AND match(headers['%s'], '%s')", v, v, ex)
	default:
		return "0"
	}
}

// RunCustomTestsSQL runs ClickHouse-native test rules as a single query.
func (s *Store) RunCustomTestsSQL(ctx context.Context, sessionID string, rules []customtests.TestRule) (map[string]map[string]string, error) {
	if len(rules) == 0 {
		return map[string]map[string]string{}, nil
	}

	var selects []string
	selects = append(selects, "url")
	for _, r := range rules {
		selects = append(selects, fmt.Sprintf("(%s) AS `%s`", buildRuleExpr(r), r.ID))
	}

	query := fmt.Sprintf("SELECT %s FROM crawlobserver.pages WHERE crawl_session_id = {sessionID:String}",
		strings.Join(selects, ", "))

	rows, err := s.conn.Query(ctx, query, clickhouse.Named("sessionID", sessionID))
	if err != nil {
		return nil, fmt.Errorf("running custom tests SQL: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[string]string)
	for rows.Next() {
		// Scan url + one bool per rule
		vals := make([]interface{}, 1+len(rules))
		var url string
		vals[0] = &url
		bools := make([]bool, len(rules))
		for i := range rules {
			vals[i+1] = &bools[i]
		}
		if err := rows.Scan(vals...); err != nil {
			return nil, fmt.Errorf("scanning custom test row: %w", err)
		}
		m := make(map[string]string, len(rules))
		for i, r := range rules {
			if bools[i] {
				m[r.ID] = "pass"
			} else {
				m[r.ID] = "fail"
			}
		}
		result[url] = m
	}
	return result, nil
}

// StreamPagesHTML streams url+body_html pairs for a session.
func (s *Store) StreamPagesHTML(ctx context.Context, sessionID string) (<-chan PageHTMLRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT url, body_html FROM crawlobserver.pages
		WHERE crawl_session_id = {sessionID:String} AND body_html != ''`,
		clickhouse.Named("sessionID", sessionID))
	if err != nil {
		return nil, fmt.Errorf("streaming pages HTML: %w", err)
	}

	ch := make(chan PageHTMLRow, 64)
	go func() {
		defer close(ch)
		defer rows.Close()
		for rows.Next() {
			var r PageHTMLRow
			if err := rows.Scan(&r.URL, &r.HTML); err != nil {
				log.Printf("StreamPagesHTML scan error: %v", err)
				return
			}
			select {
			case ch <- r:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// InsertExternalLinkChecks batch-inserts external link check results.
func (s *Store) InsertExternalLinkChecks(ctx context.Context, checks []ExternalLinkCheck) error {
	if len(checks) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.external_link_checks (
			crawl_session_id, url, status_code, error, content_type,
			redirect_url, response_time_ms, checked_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing external_link_checks batch: %w", err)
	}

	for _, c := range checks {
		if err := batch.Append(
			c.CrawlSessionID, c.URL, c.StatusCode, c.Error, c.ContentType,
			c.RedirectURL, c.ResponseTimeMs, c.CheckedAt,
		); err != nil {
			return fmt.Errorf("appending external_link_check row: %w", err)
		}
	}

	return batch.Send()
}

// GetExternalLinkChecks returns paginated external link check results for a session.
func (s *Store) GetExternalLinkChecks(ctx context.Context, sessionID string, limit, offset int, filters []ParsedFilter) ([]ExternalLinkCheck, error) {
	where, args, err := BuildWhereClause(filters)
	if err != nil {
		return nil, err
	}

	query := `SELECT crawl_session_id, url, status_code, error, content_type,
		redirect_url, response_time_ms, checked_at
		FROM crawlobserver.external_link_checks
		WHERE crawl_session_id = ?`
	queryArgs := []interface{}{sessionID}

	if where != "" {
		query += " AND " + where
		queryArgs = append(queryArgs, args...)
	}

	query += " ORDER BY status_code ASC, url ASC LIMIT ? OFFSET ?"
	queryArgs = append(queryArgs, limit, offset)

	rows, err := s.conn.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("querying external_link_checks: %w", err)
	}
	defer rows.Close()

	var results []ExternalLinkCheck
	for rows.Next() {
		var c ExternalLinkCheck
		if err := rows.Scan(&c.CrawlSessionID, &c.URL, &c.StatusCode, &c.Error,
			&c.ContentType, &c.RedirectURL, &c.ResponseTimeMs, &c.CheckedAt); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	return results, nil
}

// GetExternalLinkCheckDomains returns aggregated external check stats per domain.
func (s *Store) GetExternalLinkCheckDomains(ctx context.Context, sessionID string, limit, offset int, filters []ParsedFilter) ([]ExternalDomainCheck, error) {
	where, args, err := BuildWhereClause(filters)
	if err != nil {
		return nil, err
	}

	query := `SELECT domain(url) AS domain,
		count() AS total_urls,
		countIf(status_code >= 200 AND status_code < 300) AS ok,
		countIf(status_code >= 300 AND status_code < 400) AS redirects,
		countIf(status_code >= 400 AND status_code < 500) AS client_errors,
		countIf(status_code >= 500) AS server_errors,
		countIf(status_code = 0) AS unreachable,
		toUInt32(avg(response_time_ms)) AS avg_response_ms
		FROM crawlobserver.external_link_checks
		WHERE crawl_session_id = ?`
	queryArgs := []interface{}{sessionID}

	if where != "" {
		query += " AND " + where
		queryArgs = append(queryArgs, args...)
	}

	query += " GROUP BY domain ORDER BY total_urls DESC LIMIT ? OFFSET ?"
	queryArgs = append(queryArgs, limit, offset)

	rows, err := s.conn.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("querying external_link_check domains: %w", err)
	}
	defer rows.Close()

	var results []ExternalDomainCheck
	for rows.Next() {
		var d ExternalDomainCheck
		if err := rows.Scan(&d.Domain, &d.TotalURLs, &d.OK, &d.Redirects,
			&d.ClientErrors, &d.ServerErrors, &d.Unreachable, &d.AvgResponseMs); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, nil
}

// InsertPageResourceChecks batch inserts page resource check results.
func (s *Store) InsertPageResourceChecks(ctx context.Context, checks []PageResourceCheck) error {
	if len(checks) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.page_resource_checks (
			crawl_session_id, url, resource_type, is_internal,
			status_code, error, content_type, redirect_url,
			response_time_ms, checked_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing page_resource_checks batch: %w", err)
	}
	for _, c := range checks {
		if err := batch.Append(
			c.CrawlSessionID, c.URL, c.ResourceType, c.IsInternal,
			c.StatusCode, c.Error, c.ContentType, c.RedirectURL,
			c.ResponseTimeMs, c.CheckedAt,
		); err != nil {
			return fmt.Errorf("appending page_resource_check row: %w", err)
		}
	}
	return batch.Send()
}

// InsertPageResourceRefs batch inserts page-to-resource references.
func (s *Store) InsertPageResourceRefs(ctx context.Context, refs []PageResourceRef) error {
	if len(refs) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.page_resource_refs (
			crawl_session_id, page_url, resource_url, resource_type, is_internal
		)`)
	if err != nil {
		return fmt.Errorf("preparing page_resource_refs batch: %w", err)
	}
	for _, r := range refs {
		if err := batch.Append(
			r.CrawlSessionID, r.PageURL, r.ResourceURL, r.ResourceType, r.IsInternal,
		); err != nil {
			return fmt.Errorf("appending page_resource_ref row: %w", err)
		}
	}
	return batch.Send()
}

// GetPageResourceChecks returns paginated resource checks with page_count from refs.
func (s *Store) GetPageResourceChecks(ctx context.Context, sessionID string, limit, offset int, filters []ParsedFilter) ([]PageResourceCheck, error) {
	where, args, err := BuildWhereClause(filters)
	if err != nil {
		return nil, err
	}

	query := `SELECT c.crawl_session_id, c.url, c.resource_type, c.is_internal,
		c.status_code, c.error, c.content_type, c.redirect_url,
		c.response_time_ms, c.checked_at,
		count(DISTINCT r.page_url) AS page_count
		FROM crawlobserver.page_resource_checks AS c
		LEFT JOIN crawlobserver.page_resource_refs AS r
			ON c.crawl_session_id = r.crawl_session_id AND c.url = r.resource_url
		WHERE c.crawl_session_id = ?`
	queryArgs := []interface{}{sessionID}

	if where != "" {
		// Prefix unqualified columns with c.
		prefixed := prefixColumns(where)
		query += " AND " + prefixed
		queryArgs = append(queryArgs, args...)
	}

	query += " GROUP BY c.crawl_session_id, c.url, c.resource_type, c.is_internal, c.status_code, c.error, c.content_type, c.redirect_url, c.response_time_ms, c.checked_at"
	query += " ORDER BY c.status_code ASC, c.url ASC LIMIT ? OFFSET ?"
	queryArgs = append(queryArgs, limit, offset)

	rows, err := s.conn.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("querying page_resource_checks: %w", err)
	}
	defer rows.Close()

	var results []PageResourceCheck
	for rows.Next() {
		var c PageResourceCheck
		if err := rows.Scan(&c.CrawlSessionID, &c.URL, &c.ResourceType, &c.IsInternal,
			&c.StatusCode, &c.Error, &c.ContentType, &c.RedirectURL,
			&c.ResponseTimeMs, &c.CheckedAt, &c.PageCount); err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	return results, nil
}

// prefixColumns adds "c." prefix to known column names in a WHERE clause fragment.
func prefixColumns(where string) string {
	cols := []string{"url", "resource_type", "is_internal", "status_code", "content_type", "error"}
	result := where
	for _, col := range cols {
		result = strings.ReplaceAll(result, col+" ", "c."+col+" ")
	}
	return result
}

// GetPageResourceTypeSummary returns aggregated stats per resource type.
func (s *Store) GetPageResourceTypeSummary(ctx context.Context, sessionID string) ([]ResourceTypeSummary, error) {
	query := `SELECT
		resource_type,
		count() AS total,
		countIf(is_internal = true) AS internal,
		countIf(is_internal = false) AS external,
		countIf(status_code >= 200 AND status_code < 400) AS ok,
		countIf(status_code = 0 OR status_code >= 400) AS errors
		FROM crawlobserver.page_resource_checks
		WHERE crawl_session_id = ?
		GROUP BY resource_type
		ORDER BY total DESC`

	rows, err := s.conn.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying page_resource_checks summary: %w", err)
	}
	defer rows.Close()

	var results []ResourceTypeSummary
	for rows.Next() {
		var r ResourceTypeSummary
		if err := rows.Scan(&r.ResourceType, &r.Total, &r.Internal, &r.External, &r.OK, &r.Errors); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// GetExpiredDomains returns registrable domains where all external checks failed with DNS errors.
func (s *Store) GetExpiredDomains(ctx context.Context, sessionID string, limit, offset int) (*ExpiredDomainsResult, error) {
	// Step 1: Count total expired domains
	countQuery := `SELECT count() FROM (
		SELECT cutToFirstSignificantSubdomain(url) AS reg_domain
		FROM crawlobserver.external_link_checks
		WHERE crawl_session_id = ?
		GROUP BY reg_domain
		HAVING countIf(NOT (error = 'dns_not_found' OR error ILIKE '%no such host%')) = 0
	)`
	var total uint64
	if err := s.conn.QueryRow(ctx, countQuery, sessionID).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting expired domains: %w", err)
	}

	if total == 0 {
		return &ExpiredDomainsResult{Domains: []ExpiredDomain{}, Total: 0}, nil
	}

	// Step 2: Get paginated expired domains
	domainsQuery := `SELECT cutToFirstSignificantSubdomain(url) AS reg_domain,
		count() AS dead_urls
		FROM crawlobserver.external_link_checks
		WHERE crawl_session_id = ?
		GROUP BY reg_domain
		HAVING countIf(NOT (error = 'dns_not_found' OR error ILIKE '%no such host%')) = 0
		ORDER BY dead_urls DESC
		LIMIT ? OFFSET ?`

	rows, err := s.conn.Query(ctx, domainsQuery, sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying expired domains: %w", err)
	}
	defer rows.Close()

	var domains []ExpiredDomain
	var regDomains []string
	for rows.Next() {
		var d ExpiredDomain
		if err := rows.Scan(&d.RegistrableDomain, &d.DeadURLsChecked); err != nil {
			return nil, err
		}
		d.Sources = []ExpiredDomainSource{}
		domains = append(domains, d)
		regDomains = append(regDomains, d.RegistrableDomain)
	}

	if len(domains) == 0 {
		return &ExpiredDomainsResult{Domains: []ExpiredDomain{}, Total: total}, nil
	}

	// Step 3: Get source URLs from links table
	sourcesQuery := `SELECT DISTINCT source_url, target_url,
		cutToFirstSignificantSubdomain(target_url) AS reg_domain
		FROM crawlobserver.links
		WHERE crawl_session_id = ?
		AND is_internal = false
		AND cutToFirstSignificantSubdomain(target_url) IN (?)
		ORDER BY reg_domain, source_url`

	srcRows, err := s.conn.Query(ctx, sourcesQuery, sessionID, regDomains)
	if err != nil {
		return nil, fmt.Errorf("querying expired domain sources: %w", err)
	}
	defer srcRows.Close()

	sourceMap := make(map[string][]ExpiredDomainSource)
	for srcRows.Next() {
		var sourceURL, targetURL, regDomain string
		if err := srcRows.Scan(&sourceURL, &targetURL, &regDomain); err != nil {
			return nil, err
		}
		sourceMap[regDomain] = append(sourceMap[regDomain], ExpiredDomainSource{
			SourceURL: sourceURL,
			TargetURL: targetURL,
		})
	}

	// Attach sources to domains
	for i := range domains {
		if sources, ok := sourceMap[domains[i].RegistrableDomain]; ok {
			domains[i].Sources = sources
		}
	}

	return &ExpiredDomainsResult{Domains: domains, Total: total}, nil
}

// InsertLogs batch inserts application log rows.
func (s *Store) InsertLogs(ctx context.Context, logs []applog.LogRow) error {
	if len(logs) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO crawlobserver.application_logs (timestamp, level, component, message, context)`)
	if err != nil {
		return fmt.Errorf("preparing log batch: %w", err)
	}
	for _, l := range logs {
		if err := batch.Append(l.Timestamp, l.Level, l.Component, l.Message, l.Context); err != nil {
			return fmt.Errorf("appending log row: %w", err)
		}
	}
	return batch.Send()
}

// ListLogs returns paginated application logs with optional filters.
func (s *Store) ListLogs(ctx context.Context, limit, offset int, level, component, search string) ([]applog.LogRow, int, error) {
	where := "1=1"
	args := []any{}
	if level != "" {
		where += " AND level = ?"
		args = append(args, level)
	}
	if component != "" {
		where += " AND component = ?"
		args = append(args, component)
	}
	if search != "" {
		where += " AND message ILIKE ?"
		args = append(args, "%"+search+"%")
	}

	// Count
	var total uint64
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	if err := s.conn.QueryRow(ctx, "SELECT count() FROM crawlobserver.application_logs WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting logs: %w", err)
	}

	// Query
	q := fmt.Sprintf("SELECT timestamp, level, component, message, context FROM crawlobserver.application_logs WHERE %s ORDER BY timestamp DESC LIMIT %d OFFSET %d", where, limit, offset)
	rows, err := s.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying logs: %w", err)
	}
	defer rows.Close()

	var results []applog.LogRow
	for rows.Next() {
		var r applog.LogRow
		if err := rows.Scan(&r.Timestamp, &r.Level, &r.Component, &r.Message, &r.Context); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, int(total), nil
}

// ExportLogs returns all logs (up to 7 days per TTL) for JSONL export.
func (s *Store) ExportLogs(ctx context.Context) ([]applog.LogRow, error) {
	rows, err := s.conn.Query(ctx, `SELECT timestamp, level, component, message, context FROM crawlobserver.application_logs ORDER BY timestamp DESC`)
	if err != nil {
		return nil, fmt.Errorf("exporting logs: %w", err)
	}
	defer rows.Close()

	var results []applog.LogRow
	for rows.Next() {
		var r applog.LogRow
		if err := rows.Scan(&r.Timestamp, &r.Level, &r.Component, &r.Message, &r.Context); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// --- Provider Data Methods ---

func (s *Store) InsertProviderDomainMetrics(ctx context.Context, projectID string, rows []ProviderDomainMetricsRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.provider_domain_metrics (
			project_id, provider, domain, backlinks_total, refdomains_total, domain_rank,
			organic_keywords, organic_traffic, organic_cost, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing provider_domain_metrics batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			projectID, r.Provider, r.Domain, r.BacklinksTotal, r.RefDomainsTotal, r.DomainRank,
			r.OrganicKeywords, r.OrganicTraffic, r.OrganicCost, now,
		); err != nil {
			return fmt.Errorf("appending provider_domain_metrics row: %w", err)
		}
	}
	return batch.Send()
}

func (s *Store) InsertProviderBacklinks(ctx context.Context, projectID string, rows []ProviderBacklinkRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.provider_backlinks (
			project_id, provider, domain, source_url, target_url, anchor_text,
			source_domain, link_type, domain_rank, page_rank, nofollow,
			first_seen, last_seen, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing provider_backlinks batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			projectID, r.Provider, r.Domain, r.SourceURL, r.TargetURL, r.AnchorText,
			r.SourceDomain, r.LinkType, r.DomainRank, r.PageRank, r.Nofollow,
			r.FirstSeen, r.LastSeen, now,
		); err != nil {
			return fmt.Errorf("appending provider_backlinks row: %w", err)
		}
	}
	return batch.Send()
}

func (s *Store) InsertProviderRefDomains(ctx context.Context, projectID string, rows []ProviderRefDomainRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.provider_refdomains (
			project_id, provider, domain, ref_domain, backlink_count, domain_rank,
			first_seen, last_seen, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing provider_refdomains batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			projectID, r.Provider, r.Domain, r.RefDomain, r.BacklinkCount, r.DomainRank,
			r.FirstSeen, r.LastSeen, now,
		); err != nil {
			return fmt.Errorf("appending provider_refdomains row: %w", err)
		}
	}
	return batch.Send()
}

func (s *Store) InsertProviderRankings(ctx context.Context, projectID string, rows []ProviderRankingRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.provider_rankings (
			project_id, provider, domain, keyword, url, search_base,
			position, search_volume, cpc, traffic, traffic_pct, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing provider_rankings batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			projectID, r.Provider, r.Domain, r.Keyword, r.URL, r.SearchBase,
			r.Position, r.SearchVolume, r.CPC, r.Traffic, r.TrafficPct, now,
		); err != nil {
			return fmt.Errorf("appending provider_rankings row: %w", err)
		}
	}
	return batch.Send()
}

func (s *Store) InsertProviderVisibility(ctx context.Context, projectID string, rows []ProviderVisibilityRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.provider_visibility (
			project_id, provider, domain, search_base, date, visibility, keywords_count, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing provider_visibility batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			projectID, r.Provider, r.Domain, r.SearchBase, r.Date, r.Visibility, r.KeywordsCount, now,
		); err != nil {
			return fmt.Errorf("appending provider_visibility row: %w", err)
		}
	}
	return batch.Send()
}

func (s *Store) ProviderDomainMetrics(ctx context.Context, projectID, provider string) (*ProviderDomainMetricsRow, error) {
	var r ProviderDomainMetricsRow
	err := s.conn.QueryRow(ctx, `
		SELECT provider, domain, backlinks_total, refdomains_total, domain_rank,
			organic_keywords, organic_traffic, organic_cost, fetched_at
		FROM crawlobserver.provider_domain_metrics FINAL
		WHERE project_id = ? AND provider = ?
		LIMIT 1`, projectID, provider).Scan(
		&r.Provider, &r.Domain, &r.BacklinksTotal, &r.RefDomainsTotal, &r.DomainRank,
		&r.OrganicKeywords, &r.OrganicTraffic, &r.OrganicCost, &r.FetchedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) ProviderBacklinks(ctx context.Context, projectID, provider string, limit, offset int) ([]ProviderBacklinkRow, int, error) {
	var total uint64
	if err := s.conn.QueryRow(ctx, `
		SELECT count() FROM crawlobserver.provider_backlinks FINAL
		WHERE project_id = ? AND provider = ?`, projectID, provider).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting provider backlinks: %w", err)
	}

	rows, err := s.conn.Query(ctx, `
		SELECT provider, domain, source_url, target_url, anchor_text, source_domain, link_type,
			domain_rank, page_rank, nofollow, first_seen, last_seen, fetched_at
		FROM crawlobserver.provider_backlinks FINAL
		WHERE project_id = ? AND provider = ?
		ORDER BY domain_rank DESC
		LIMIT ? OFFSET ?`, projectID, provider, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying provider backlinks: %w", err)
	}
	defer rows.Close()

	var result []ProviderBacklinkRow
	for rows.Next() {
		var r ProviderBacklinkRow
		if err := rows.Scan(&r.Provider, &r.Domain, &r.SourceURL, &r.TargetURL, &r.AnchorText,
			&r.SourceDomain, &r.LinkType, &r.DomainRank, &r.PageRank, &r.Nofollow,
			&r.FirstSeen, &r.LastSeen, &r.FetchedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning provider backlink row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []ProviderBacklinkRow{}
	}
	return result, int(total), nil
}

func (s *Store) ProviderRefDomains(ctx context.Context, projectID, provider string, limit, offset int) ([]ProviderRefDomainRow, int, error) {
	var total uint64
	if err := s.conn.QueryRow(ctx, `
		SELECT count() FROM crawlobserver.provider_refdomains FINAL
		WHERE project_id = ? AND provider = ?`, projectID, provider).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting provider refdomains: %w", err)
	}

	rows, err := s.conn.Query(ctx, `
		SELECT provider, domain, ref_domain, backlink_count, domain_rank, first_seen, last_seen, fetched_at
		FROM crawlobserver.provider_refdomains FINAL
		WHERE project_id = ? AND provider = ?
		ORDER BY backlink_count DESC
		LIMIT ? OFFSET ?`, projectID, provider, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying provider refdomains: %w", err)
	}
	defer rows.Close()

	var result []ProviderRefDomainRow
	for rows.Next() {
		var r ProviderRefDomainRow
		if err := rows.Scan(&r.Provider, &r.Domain, &r.RefDomain, &r.BacklinkCount, &r.DomainRank,
			&r.FirstSeen, &r.LastSeen, &r.FetchedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning provider refdomain row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []ProviderRefDomainRow{}
	}
	return result, int(total), nil
}

func (s *Store) ProviderRankings(ctx context.Context, projectID, provider string, limit, offset int) ([]ProviderRankingRow, int, error) {
	var total uint64
	if err := s.conn.QueryRow(ctx, `
		SELECT count() FROM crawlobserver.provider_rankings FINAL
		WHERE project_id = ? AND provider = ?`, projectID, provider).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting provider rankings: %w", err)
	}

	rows, err := s.conn.Query(ctx, `
		SELECT provider, domain, keyword, url, search_base, position,
			search_volume, cpc, traffic, traffic_pct, fetched_at
		FROM crawlobserver.provider_rankings FINAL
		WHERE project_id = ? AND provider = ?
		ORDER BY traffic DESC
		LIMIT ? OFFSET ?`, projectID, provider, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying provider rankings: %w", err)
	}
	defer rows.Close()

	var result []ProviderRankingRow
	for rows.Next() {
		var r ProviderRankingRow
		if err := rows.Scan(&r.Provider, &r.Domain, &r.Keyword, &r.URL, &r.SearchBase, &r.Position,
			&r.SearchVolume, &r.CPC, &r.Traffic, &r.TrafficPct, &r.FetchedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning provider ranking row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []ProviderRankingRow{}
	}
	return result, int(total), nil
}

func (s *Store) ProviderVisibilityHistory(ctx context.Context, projectID, provider string) ([]ProviderVisibilityRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT provider, domain, search_base, date, visibility, keywords_count, fetched_at
		FROM crawlobserver.provider_visibility FINAL
		WHERE project_id = ? AND provider = ?
		ORDER BY date ASC`, projectID, provider)
	if err != nil {
		return nil, fmt.Errorf("querying provider visibility: %w", err)
	}
	defer rows.Close()

	var result []ProviderVisibilityRow
	for rows.Next() {
		var r ProviderVisibilityRow
		if err := rows.Scan(&r.Provider, &r.Domain, &r.SearchBase, &r.Date, &r.Visibility,
			&r.KeywordsCount, &r.FetchedAt); err != nil {
			return nil, fmt.Errorf("scanning provider visibility row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []ProviderVisibilityRow{}
	}
	return result, nil
}

func (s *Store) DeleteProviderData(ctx context.Context, projectID, provider string) error {
	tables := []string{
		"provider_domain_metrics",
		"provider_backlinks",
		"provider_refdomains",
		"provider_rankings",
		"provider_visibility",
	}
	for _, table := range tables {
		q := fmt.Sprintf("ALTER TABLE crawlobserver.%s DELETE WHERE project_id = ? AND provider = ?", table)
		if err := s.conn.Exec(ctx, q, projectID, provider); err != nil {
			return fmt.Errorf("deleting from %s: %w", table, err)
		}
	}
	return nil
}

// Close closes the ClickHouse connection.
func (s *Store) Close() error {
	return s.conn.Close()
}
