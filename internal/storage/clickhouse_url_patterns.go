package storage

import (
	"context"
	"fmt"
)

// URLPattern groups pages by URL path template (first N segments).
type URLPattern struct {
	Pattern      string  `json:"pattern"`
	Total        uint64  `json:"total"`
	Indexable    uint64  `json:"indexable"`
	NonIndexable uint64  `json:"non_indexable"`
	WithParams   uint64  `json:"with_params"`
	AvgPageRank  float64 `json:"avg_pagerank"`
	Status200    uint64  `json:"status_200"`
	Status3xx    uint64  `json:"status_3xx"`
	Status4xx    uint64  `json:"status_4xx"`
	StatusOther  uint64  `json:"status_other"`
}

// URLParam describes a query-string parameter and its prevalence.
type URLParam struct {
	Param        string `json:"param"`
	Occurrences  uint64 `json:"occurrences"`
	UniqueURLs   uint64 `json:"unique_urls"`
	Indexable    uint64 `json:"indexable"`
	NonIndexable uint64 `json:"non_indexable"`
}

// URLDirectory groups pages by directory path at a configurable depth.
type URLDirectory struct {
	Path       string  `json:"path"`
	Total      uint64  `json:"total"`
	Indexable  uint64  `json:"indexable"`
	WithParams uint64  `json:"with_params"`
	AvgPR      float64 `json:"avg_pr"`
	Errors     uint64  `json:"errors"`
}

// URLHost groups pages by hostname.
type URLHost struct {
	Host      string  `json:"host"`
	Total     uint64  `json:"total"`
	Indexable uint64  `json:"indexable"`
	Status200 uint64  `json:"status_200"`
	Errors    uint64  `json:"errors"`
	AvgPR     float64 `json:"avg_pr"`
}

// URLPatterns returns pages grouped by the first `depth` path segments.
func (s *Store) URLPatterns(ctx context.Context, sessionID string, depth int) ([]URLPattern, error) {
	if !isValidUUID(sessionID) {
		return nil, fmt.Errorf("invalid session ID: %s", sessionID)
	}
	if depth < 1 {
		depth = 2
	}
	if depth > 5 {
		depth = 5
	}

	query := fmt.Sprintf(`
		SELECT
			concat('/',
				arrayStringConcat(
					arraySlice(
						splitByChar('/', pathFull(url)),
						2, %d
					), '/'
				),
			'/') AS pattern,
			count() AS total,
			countIf(is_indexable) AS indexable,
			countIf(NOT is_indexable) AS non_indexable,
			countIf(length(extractURLParameterNames(url)) > 0) AS with_params,
			avg(if(pagerank > 0, pagerank, 0)) AS avg_pagerank,
			countIf(status_code >= 200 AND status_code < 300) AS status_200,
			countIf(status_code >= 300 AND status_code < 400) AS status_3xx,
			countIf(status_code >= 400 AND status_code < 500) AS status_4xx,
			countIf(status_code = 0 OR status_code >= 500) AS status_other
		FROM crawlobserver.pages FINAL
		WHERE crawl_session_id = ? AND `+notRedirectedFilter+`
		GROUP BY pattern
		ORDER BY total DESC
		LIMIT 200`, depth)

	rows, err := s.conn.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying url patterns: %w", err)
	}
	defer rows.Close()

	var results []URLPattern
	for rows.Next() {
		var p URLPattern
		if err := rows.Scan(&p.Pattern, &p.Total, &p.Indexable, &p.NonIndexable,
			&p.WithParams, &p.AvgPageRank, &p.Status200, &p.Status3xx,
			&p.Status4xx, &p.StatusOther); err != nil {
			return nil, fmt.Errorf("scanning url pattern: %w", err)
		}
		if p.Pattern == "" {
			p.Pattern = "/"
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// URLParams returns the most frequent query-string parameters.
func (s *Store) URLParams(ctx context.Context, sessionID string, limit int) ([]URLParam, error) {
	if !isValidUUID(sessionID) {
		return nil, fmt.Errorf("invalid session ID: %s", sessionID)
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT
			arrayJoin(extractURLParameterNames(url)) AS param,
			count() AS occurrences,
			uniq(replaceRegexpOne(url, '\\?.*', '')) AS unique_urls,
			countIf(is_indexable) AS indexable,
			countIf(NOT is_indexable) AS non_indexable
		FROM crawlobserver.pages FINAL
		WHERE crawl_session_id = ? AND `+notRedirectedFilter+`
		GROUP BY param
		ORDER BY occurrences DESC
		LIMIT %d`, limit)

	rows, err := s.conn.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying url params: %w", err)
	}
	defer rows.Close()

	var results []URLParam
	for rows.Next() {
		var p URLParam
		if err := rows.Scan(&p.Param, &p.Occurrences, &p.UniqueURLs,
			&p.Indexable, &p.NonIndexable); err != nil {
			return nil, fmt.Errorf("scanning url param: %w", err)
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// URLDirectories returns pages grouped by directory path at a configurable depth.
func (s *Store) URLDirectories(ctx context.Context, sessionID string, depth, minPages int) ([]URLDirectory, error) {
	if !isValidUUID(sessionID) {
		return nil, fmt.Errorf("invalid session ID: %s", sessionID)
	}
	if depth < 1 {
		depth = 2
	}
	if depth > 5 {
		depth = 5
	}
	if minPages < 1 {
		minPages = 1
	}

	query := fmt.Sprintf(`
		SELECT
			arrayStringConcat(
				arraySlice(
					splitByChar('/', replaceRegexpOne(url, '^http[s]://[^/]*', '')),
					1, %d
				), '/'
			) AS dir_path,
			count() AS total,
			countIf(is_indexable) AS indexable,
			countIf(length(extractURLParameterNames(url)) > 0) AS with_params,
			avg(if(pagerank > 0, pagerank, 0)) AS avg_pr,
			countIf(status_code >= 400 OR status_code = 0) AS errors
		FROM crawlobserver.pages FINAL
		WHERE crawl_session_id = ? AND `+notRedirectedFilter+`
		GROUP BY dir_path
		HAVING total >= %d
		ORDER BY total DESC
		LIMIT 200`, depth, minPages)

	rows, err := s.conn.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying url directories: %w", err)
	}
	defer rows.Close()

	var results []URLDirectory
	for rows.Next() {
		var d URLDirectory
		if err := rows.Scan(&d.Path, &d.Total, &d.Indexable,
			&d.WithParams, &d.AvgPR, &d.Errors); err != nil {
			return nil, fmt.Errorf("scanning url directory: %w", err)
		}
		if d.Path == "" {
			d.Path = "/"
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// URLHosts returns pages grouped by hostname.
func (s *Store) URLHosts(ctx context.Context, sessionID string) ([]URLHost, error) {
	if !isValidUUID(sessionID) {
		return nil, fmt.Errorf("invalid session ID: %s", sessionID)
	}

	query := `
		SELECT
			domain(url) AS host,
			count() AS total,
			countIf(is_indexable) AS indexable,
			countIf(status_code >= 200 AND status_code < 300) AS status_200,
			countIf(status_code >= 400 OR status_code = 0) AS errors,
			avg(if(pagerank > 0, pagerank, 0)) AS avg_pr
		FROM crawlobserver.pages FINAL
		WHERE crawl_session_id = ? AND ` + notRedirectedFilter + `
		GROUP BY host
		ORDER BY total DESC
		LIMIT 100`

	rows, err := s.conn.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying url hosts: %w", err)
	}
	defer rows.Close()

	var results []URLHost
	for rows.Next() {
		var h URLHost
		if err := rows.Scan(&h.Host, &h.Total, &h.Indexable,
			&h.Status200, &h.Errors, &h.AvgPR); err != nil {
			return nil, fmt.Errorf("scanning url host: %w", err)
		}
		results = append(results, h)
	}
	return results, rows.Err()
}
