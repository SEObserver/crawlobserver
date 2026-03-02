package storage

import (
	"context"
	"fmt"
	"strings"
)

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

// PageBody holds a page URL and its raw HTML body for reprocessing.
type PageBody struct {
	URL      string
	BodyHTML string
}

// GetPageBodies reads URL + body_html for a session in batches.
func (s *Store) GetPageBodies(ctx context.Context, sessionID string, limit, offset int) ([]PageBody, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT url, body_html
		FROM crawlobserver.pages
		WHERE crawl_session_id = ?
			AND status_code >= 200 AND status_code < 300
			AND length(body_html) > 0
		ORDER BY url
		LIMIT ? OFFSET ?`,
		sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying page bodies: %w", err)
	}
	defer rows.Close()

	var results []PageBody
	for rows.Next() {
		var p PageBody
		if err := rows.Scan(&p.URL, &p.BodyHTML); err != nil {
			return nil, fmt.Errorf("scanning page body: %w", err)
		}
		results = append(results, p)
	}
	return results, nil
}
