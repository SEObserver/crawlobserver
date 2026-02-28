package storage

import (
	"context"
	"fmt"
)

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
