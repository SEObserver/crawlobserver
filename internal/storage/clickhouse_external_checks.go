package storage

import (
	"context"
	"fmt"
)

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
