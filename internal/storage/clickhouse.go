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
			title, canonical, meta_robots, meta_description,
			h1, h2, h3, h4, h5, h6,
			headers, redirect_chain, body_size, fetch_duration_ms,
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

		if err := batch.Append(
			p.CrawlSessionID, p.URL, p.FinalURL, p.StatusCode, p.ContentType,
			p.Title, p.Canonical, p.MetaRobots, p.MetaDescription,
			p.H1, p.H2, p.H3, p.H4, p.H5, p.H6,
			p.Headers, chain, p.BodySize, p.FetchDurationMs,
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
		FROM seocrawler.crawl_sessions
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

// Close closes the ClickHouse connection.
func (s *Store) Close() error {
	return s.conn.Close()
}
