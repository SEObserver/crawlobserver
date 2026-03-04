package storage

import (
	"context"
	"fmt"
	"time"
)

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
			source_domain, link_type, domain_rank, page_rank, source_ttf_topic, nofollow,
			first_seen, last_seen, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing provider_backlinks batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		if err := batch.Append(
			projectID, r.Provider, r.Domain, r.SourceURL, r.TargetURL, r.AnchorText,
			r.SourceDomain, r.LinkType, r.TrustFlow, r.CitationFlow, r.SourceTTFTopic, r.Nofollow,
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

func (s *Store) ProviderBacklinks(ctx context.Context, projectID, provider string, limit, offset int, filters []ParsedFilter, sort *SortParam) ([]ProviderBacklinkRow, int, error) {
	baseWhere := `project_id = ? AND provider = ?`
	args := []interface{}{projectID, provider}

	whereExtra, filterArgs, err := BuildWhereClause(filters)
	if err != nil {
		return nil, 0, fmt.Errorf("building backlinks filter clause: %w", err)
	}
	if whereExtra != "" {
		baseWhere += " AND " + whereExtra
		args = append(args, filterArgs...)
	}

	var total uint64
	if err := s.conn.QueryRow(ctx, fmt.Sprintf(`
		SELECT count() FROM crawlobserver.provider_backlinks FINAL
		WHERE %s`, baseWhere), args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting provider backlinks: %w", err)
	}

	orderClause := BuildOrderByClause(sort, "domain_rank DESC")
	query := fmt.Sprintf(`
		SELECT provider, domain, source_url, target_url, anchor_text, source_domain, link_type,
			domain_rank, page_rank, source_ttf_topic, nofollow, first_seen, last_seen, fetched_at
		FROM crawlobserver.provider_backlinks FINAL
		WHERE %s
		%s
		LIMIT ? OFFSET ?`, baseWhere, orderClause)
	queryArgs := append(args[:len(args):len(args)], limit, offset)

	rows, err := s.conn.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying provider backlinks: %w", err)
	}
	defer rows.Close()

	var result []ProviderBacklinkRow
	for rows.Next() {
		var r ProviderBacklinkRow
		if err := rows.Scan(&r.Provider, &r.Domain, &r.SourceURL, &r.TargetURL, &r.AnchorText,
			&r.SourceDomain, &r.LinkType, &r.TrustFlow, &r.CitationFlow, &r.SourceTTFTopic, &r.Nofollow,
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

func (s *Store) InsertProviderTopPages(ctx context.Context, projectID string, rows []ProviderTopPageRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.provider_top_pages (
			project_id, provider, domain, url, title, trust_flow, citation_flow,
			ext_backlinks, ref_domains, topical_trust_flow, language, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing provider_top_pages batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		ttf := make([][]interface{}, len(r.TopicalTrustFlow))
		for i, t := range r.TopicalTrustFlow {
			ttf[i] = []interface{}{t.Topic, t.Value}
		}
		if err := batch.Append(
			projectID, r.Provider, r.Domain, r.URL, r.Title,
			r.TrustFlow, r.CitationFlow, r.ExtBackLinks, r.RefDomains,
			ttf, r.Language, now,
		); err != nil {
			return fmt.Errorf("appending provider_top_pages row: %w", err)
		}
	}
	return batch.Send()
}

func (s *Store) ProviderTopPages(ctx context.Context, projectID, provider string, limit, offset int) ([]ProviderTopPageRow, int, error) {
	var total uint64
	if err := s.conn.QueryRow(ctx, `
		SELECT count() FROM crawlobserver.provider_top_pages FINAL
		WHERE project_id = ? AND provider = ?`, projectID, provider).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting provider top pages: %w", err)
	}

	rows, err := s.conn.Query(ctx, `
		SELECT provider, domain, url, title, trust_flow, citation_flow,
			ext_backlinks, ref_domains, topical_trust_flow, language, fetched_at
		FROM crawlobserver.provider_top_pages FINAL
		WHERE project_id = ? AND provider = ?
		ORDER BY trust_flow DESC
		LIMIT ? OFFSET ?`, projectID, provider, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying provider top pages: %w", err)
	}
	defer rows.Close()

	var result []ProviderTopPageRow
	for rows.Next() {
		var r ProviderTopPageRow
		var ttfRaw [][]interface{}
		if err := rows.Scan(&r.Provider, &r.Domain, &r.URL, &r.Title,
			&r.TrustFlow, &r.CitationFlow, &r.ExtBackLinks, &r.RefDomains,
			&ttfRaw, &r.Language, &r.FetchedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning provider top page row: %w", err)
		}
		for _, pair := range ttfRaw {
			if len(pair) == 2 {
				topic, _ := pair[0].(string)
				value, _ := pair[1].(uint8)
				r.TopicalTrustFlow = append(r.TopicalTrustFlow, TopicalTF{Topic: topic, Value: value})
			}
		}
		result = append(result, r)
	}
	if result == nil {
		result = []ProviderTopPageRow{}
	}
	return result, int(total), nil
}

func (s *Store) InsertProviderAPICalls(ctx context.Context, rows []ProviderAPICallRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.provider_api_calls (
			project_id, provider, endpoint, method, status_code, duration_ms,
			rows_returned, response_body, error, called_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing provider_api_calls batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.ProjectID, r.Provider, r.Endpoint, r.Method, r.StatusCode,
			r.DurationMs, r.RowsReturned, r.ResponseBody, r.Error, r.CalledAt,
		); err != nil {
			return fmt.Errorf("appending provider_api_calls row: %w", err)
		}
	}
	return batch.Send()
}

func (s *Store) ProviderAPICalls(ctx context.Context, projectID, provider string, limit, offset int) ([]ProviderAPICallRow, int, error) {
	var total uint64
	if err := s.conn.QueryRow(ctx, `
		SELECT count() FROM crawlobserver.provider_api_calls
		WHERE project_id = ? AND provider = ?`, projectID, provider).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting provider api calls: %w", err)
	}

	rows, err := s.conn.Query(ctx, `
		SELECT project_id, provider, endpoint, method, status_code, duration_ms,
			rows_returned, response_body, error, called_at
		FROM crawlobserver.provider_api_calls
		WHERE project_id = ? AND provider = ?
		ORDER BY called_at DESC
		LIMIT ? OFFSET ?`, projectID, provider, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying provider api calls: %w", err)
	}
	defer rows.Close()

	var result []ProviderAPICallRow
	for rows.Next() {
		var r ProviderAPICallRow
		if err := rows.Scan(&r.ProjectID, &r.Provider, &r.Endpoint, &r.Method,
			&r.StatusCode, &r.DurationMs, &r.RowsReturned, &r.ResponseBody,
			&r.Error, &r.CalledAt); err != nil {
			return nil, 0, fmt.Errorf("scanning provider api call row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []ProviderAPICallRow{}
	}
	return result, int(total), nil
}

// --- Unified provider_data methods ---

func (s *Store) InsertProviderData(ctx context.Context, projectID string, rows []ProviderDataRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.provider_data (
			project_id, provider, data_type, domain, item_url,
			trust_flow, citation_flow, domain_rank, ext_backlinks, ref_domains,
			str_data, num_data, fetched_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing provider_data batch: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		strData := r.StrData
		if strData == nil {
			strData = map[string]string{}
		}
		numData := r.NumData
		if numData == nil {
			numData = map[string]float64{}
		}
		if err := batch.Append(
			projectID, r.Provider, r.DataType, r.Domain, r.ItemURL,
			r.TrustFlow, r.CitationFlow, r.DomainRank, r.ExtBacklinks, r.RefDomains,
			strData, numData, now,
		); err != nil {
			return fmt.Errorf("appending provider_data row: %w", err)
		}
	}
	return batch.Send()
}

func (s *Store) ProviderData(ctx context.Context, projectID, provider, dataType string, limit, offset int, filters []ParsedFilter, sort *SortParam) ([]ProviderDataRow, int, error) {
	baseWhere := `project_id = ? AND provider = ? AND data_type = ?`
	baseArgs := []interface{}{projectID, provider, dataType}

	whereExtra, filterArgs, err := BuildWhereClause(filters)
	if err != nil {
		return nil, 0, fmt.Errorf("building filter clause: %w", err)
	}
	fullWhere := baseWhere
	allArgs := append([]interface{}{}, baseArgs...)
	if whereExtra != "" {
		fullWhere += " AND " + whereExtra
		allArgs = append(allArgs, filterArgs...)
	}

	var total uint64
	countQ := `SELECT count() FROM crawlobserver.provider_data FINAL WHERE ` + fullWhere
	if err := s.conn.QueryRow(ctx, countQ, allArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting provider_data: %w", err)
	}

	query := `
		SELECT provider, data_type, domain, item_url,
			trust_flow, citation_flow, domain_rank, ext_backlinks, ref_domains,
			str_data, num_data, fetched_at
		FROM crawlobserver.provider_data FINAL
		WHERE ` + fullWhere + BuildOrderByClause(sort, "trust_flow DESC") + ` LIMIT ? OFFSET ?`
	queryArgs := append(append([]interface{}{}, allArgs...), limit, offset)

	rows, err := s.conn.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying provider_data: %w", err)
	}
	defer rows.Close()

	var result []ProviderDataRow
	for rows.Next() {
		var r ProviderDataRow
		if err := rows.Scan(&r.Provider, &r.DataType, &r.Domain, &r.ItemURL,
			&r.TrustFlow, &r.CitationFlow, &r.DomainRank, &r.ExtBacklinks, &r.RefDomains,
			&r.StrData, &r.NumData, &r.FetchedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning provider_data row: %w", err)
		}
		result = append(result, r)
	}
	if result == nil {
		result = []ProviderDataRow{}
	}
	return result, int(total), nil
}

func (s *Store) ProviderDataAge(ctx context.Context, projectID, provider, dataType string) (time.Time, error) {
	var fetchedAt time.Time
	err := s.conn.QueryRow(ctx, `
		SELECT max(fetched_at)
		FROM crawlobserver.provider_data
		WHERE project_id = ? AND provider = ? AND data_type = ?`,
		projectID, provider, dataType).Scan(&fetchedAt)
	if err != nil {
		return time.Time{}, err
	}
	return fetchedAt, nil
}

func (s *Store) DeleteProviderData(ctx context.Context, projectID, provider string) error {
	tables := []string{
		"provider_domain_metrics",
		"provider_backlinks",
		"provider_refdomains",
		"provider_rankings",
		"provider_visibility",
		"provider_top_pages",
		"provider_data",
	}
	for _, table := range tables {
		q := fmt.Sprintf("ALTER TABLE crawlobserver.%s DELETE WHERE project_id = ? AND provider = ?", table)
		if err := s.conn.Exec(ctx, q, projectID, provider); err != nil {
			return fmt.Errorf("deleting from %s: %w", table, err)
		}
	}
	return nil
}
