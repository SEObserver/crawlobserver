package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sessions: %w", err)
	}
	return sessions, nil
}

// ListSessionsPaginated retrieves crawl sessions with pagination, optional project and search filters.
func (s *Store) ListSessionsPaginated(ctx context.Context, limit, offset int, projectID, search string) ([]CrawlSession, int, error) {
	where := " WHERE 1=1"
	var args []interface{}
	if projectID != "" {
		where += ` AND project_id = ?`
		args = append(args, projectID)
	}
	if search != "" {
		where += ` AND arrayExists(x -> x ILIKE ?, seed_urls)`
		args = append(args, "%"+search+"%")
	}

	// Count
	var total uint64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := s.conn.QueryRow(ctx,
		`SELECT count() FROM crawlobserver.crawl_sessions FINAL`+where, countArgs...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting sessions: %w", err)
	}

	// Fetch page
	query := `SELECT id, started_at, finished_at, status, seed_urls, config, pages_crawled, user_agent, project_id
		FROM crawlobserver.crawl_sessions FINAL` + where + ` ORDER BY started_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying sessions paginated: %w", err)
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
			return nil, 0, fmt.Errorf("scanning session: %w", err)
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating sessions: %w", err)
	}
	if sessions == nil {
		sessions = []CrawlSession{}
	}
	return sessions, int(total), nil
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating status codes: %w", err)
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
	if err := depthRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating depth distribution: %w", err)
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
		if err := prRows.Err(); err != nil {
			applog.Warnf("audit", "iterating top pagerank: %v", err)
		}
	}

	return stats, nil
}

// --- Audit types and method ---

// AuditContent holds content-related audit metrics.
type AuditContent struct {
	Total                uint64 `json:"total"`
	HTMLPages            uint64 `json:"html_pages"`
	TitleMissing         uint64 `json:"title_missing"`
	TitleTooLong         uint64 `json:"title_too_long"`
	TitleTooShort        uint64 `json:"title_too_short"`
	TitleDuplicates      uint64 `json:"title_duplicates"`
	MetaDescMissing      uint64 `json:"meta_desc_missing"`
	MetaDescTooLong      uint64 `json:"meta_desc_too_long"`
	MetaDescTooShort     uint64 `json:"meta_desc_too_short"`
	H1Missing            uint64 `json:"h1_missing"`
	H1Multiple           uint64 `json:"h1_multiple"`
	ThinUnder100         uint64 `json:"thin_under_100"`
	Thin100300           uint64 `json:"thin_100_300"`
	ImagesTotal          uint64 `json:"images_total"`
	ImagesNoAltTotal     uint64 `json:"images_no_alt_total"`
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
	TotalInternal        uint64           `json:"total_internal"`
	TotalExternal        uint64           `json:"total_external"`
	ExternalNofollow     uint64           `json:"external_nofollow"`
	ExternalDofollow     uint64           `json:"external_dofollow"`
	PagesNoInternalOut   uint64           `json:"pages_no_internal_out"`
	PagesHighInternalOut uint64           `json:"pages_high_internal_out"`
	PagesNoExternal      uint64           `json:"pages_no_external"`
	BrokenInternal       uint64           `json:"broken_internal"`
	TopExternalDomains   []ExternalDomain `json:"top_external_domains"`
	TopAnchors           []AnchorCount    `json:"top_anchors"`
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
	var titleDups int64
	if err := dupRow.Scan(&titleDups); err != nil {
		applog.Warnf("audit", "scan title duplicates: %v", err)
	} else if titleDups > 0 {
		content.TitleDuplicates = uint64(titleDups)
	}
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
		if err := niRows.Err(); err != nil {
			applog.Warnf("audit", "iterating noindex reasons: %v", err)
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
		if err := ctRows.Err(); err != nil {
			applog.Warnf("audit", "iterating content types: %v", err)
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
	if err := pageDistRow.Scan(&links.PagesNoInternalOut, &links.PagesHighInternalOut, &links.PagesNoExternal); err != nil {
		applog.Warnf("audit", "scan link distribution: %v", err)
	}

	// Broken internal links (LEFT ANTI JOIN to avoid in-memory hash set on large datasets)
	brokenRow := s.conn.QueryRow(ctx, `
		SELECT count(DISTINCT l.target_url)
		FROM crawlobserver.links AS l
		LEFT ANTI JOIN crawlobserver.pages AS p
			ON p.crawl_session_id = l.crawl_session_id AND p.url = l.target_url
		WHERE l.crawl_session_id = ? AND l.is_internal = true`,
		sessionID)
	if err := brokenRow.Scan(&links.BrokenInternal); err != nil {
		applog.Warnf("audit", "scan broken internal links: %v", err)
	}

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
		if err := edRows.Err(); err != nil {
			applog.Warnf("audit", "iterating external domains: %v", err)
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
		if err := anRows.Err(); err != nil {
			applog.Warnf("audit", "iterating anchors: %v", err)
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
		if err := dirRows.Err(); err != nil {
			applog.Warnf("audit", "iterating directories: %v", err)
		}
	}

	// Orphan pages (LEFT ANTI JOIN to avoid in-memory hash set on large datasets)
	orphanRow := s.conn.QueryRow(ctx, `
		SELECT count()
		FROM crawlobserver.pages AS p
		LEFT ANTI JOIN (
			SELECT DISTINCT target_url
			FROM crawlobserver.links
			WHERE crawl_session_id = ? AND is_internal = true
		) AS l ON p.url = l.target_url
		WHERE p.crawl_session_id = ?`, sessionID, sessionID)
	if err := orphanRow.Scan(&structure.OrphanPages); err != nil {
		applog.Warnf("audit", "scan orphan pages: %v", err)
	}
	result.Structure = structure

	// --- Sitemaps audit ---
	sitemaps := &AuditSitemaps{}
	smRow := s.conn.QueryRow(ctx, `
		SELECT count(DISTINCT loc) FROM crawlobserver.sitemap_urls
		WHERE crawl_session_id = ?`, sessionID)
	if err := smRow.Scan(&sitemaps.TotalSitemapURLs); err != nil {
		applog.Warnf("audit", "scan sitemap URL count: %v", err)
	}

	if sitemaps.TotalSitemapURLs > 0 {
		var inBoth uint64
		ibRow := s.conn.QueryRow(ctx, `
			SELECT count() FROM (
				SELECT DISTINCT loc FROM crawlobserver.sitemap_urls WHERE crawl_session_id = ?
			) AS sm WHERE sm.loc IN (
				SELECT url FROM crawlobserver.pages WHERE crawl_session_id = ?
			)`, sessionID, sessionID)
		if ibRow.Scan(&inBoth) == nil {
			sitemaps.InBoth = inBoth
			var totalCrawled uint64
			tcRow := s.conn.QueryRow(ctx, `SELECT count() FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID)
			if err := tcRow.Scan(&totalCrawled); err != nil {
				applog.Warnf("audit", "scan total crawled for sitemap coverage: %v", err)
			}
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
	if err := intlRow.Scan(&intl.PagesWithHreflang, &intl.PagesWithLang, &intl.PagesWithSchema); err != nil {
		applog.Warnf("audit", "scan international stats: %v", err)
	}

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
		if err := langRows.Err(); err != nil {
			applog.Warnf("audit", "iterating languages: %v", err)
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
		if err := schemaRows.Err(); err != nil {
			applog.Warnf("audit", "iterating schemas: %v", err)
		}
	}
	result.International = intl

	return result, nil
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating storage stats: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating session storage stats: %w", err)
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
	if err := pageRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating page stats: %w", err)
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
	if err := linkRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating link stats: %w", err)
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
