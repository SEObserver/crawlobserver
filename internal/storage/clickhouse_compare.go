package storage

import (
	"context"
	"fmt"
)

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
			FROM crawlobserver.pages FINAL WHERE crawl_session_id = ? AND `+notRedirectedFilter+`
		) a
		FULL OUTER JOIN (
			SELECT url, status_code, title, canonical, is_indexable, meta_description, h1, depth, word_count, pagerank
			FROM crawlobserver.pages FINAL WHERE crawl_session_id = ? AND `+notRedirectedFilter+`
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
			FROM crawlobserver.pages FINAL b
			LEFT JOIN crawlobserver.pages FINAL a ON a.url = b.url AND a.crawl_session_id = ? AND (a.final_url = '' OR a.final_url = a.url)
			WHERE b.crawl_session_id = ? AND a.url = '' AND (b.final_url = '' OR b.final_url = b.url)
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
			FROM crawlobserver.pages FINAL a
			LEFT JOIN crawlobserver.pages FINAL b ON a.url = b.url AND b.crawl_session_id = ? AND (b.final_url = '' OR b.final_url = b.url)
			WHERE a.crawl_session_id = ? AND b.url = '' AND (a.final_url = '' OR a.final_url = a.url)
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
			FROM crawlobserver.pages FINAL a
			INNER JOIN crawlobserver.pages FINAL b ON a.url = b.url AND b.crawl_session_id = ? AND (b.final_url = '' OR b.final_url = b.url)
			WHERE a.crawl_session_id = ? AND (a.final_url = '' OR a.final_url = a.url) AND (
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
