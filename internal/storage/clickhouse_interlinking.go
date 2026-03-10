package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

// DeleteInterlinkingOpportunities removes all opportunities for a session.
func (s *Store) DeleteInterlinkingOpportunities(ctx context.Context, sessionID string) error {
	return s.conn.Exec(ctx,
		`ALTER TABLE crawlobserver.interlinking_opportunities DELETE WHERE crawl_session_id = ?`,
		sessionID)
}

// InsertInterlinkingOpportunities batch-inserts interlinking opportunities.
func (s *Store) InsertInterlinkingOpportunities(ctx context.Context, sessionID string, opps []InterlinkingOpportunity) error {
	if len(opps) == 0 {
		return nil
	}

	now := time.Now()
	const chunkSize = 1000
	for i := 0; i < len(opps); i += chunkSize {
		end := i + chunkSize
		if end > len(opps) {
			end = len(opps)
		}
		batch, err := s.conn.PrepareBatch(ctx,
			`INSERT INTO crawlobserver.interlinking_opportunities (
				crawl_session_id, source_url, target_url, similarity, method,
				source_title, target_title, source_pagerank, target_pagerank,
				source_word_count, target_word_count, computed_at)`)
		if err != nil {
			return fmt.Errorf("preparing interlinking batch: %w", err)
		}
		for _, o := range opps[i:end] {
			if err := batch.Append(
				sessionID, o.SourceURL, o.TargetURL, o.Similarity, o.Method,
				o.SourceTitle, o.TargetTitle, o.SourcePageRank, o.TargetPageRank,
				o.SourceWordCount, o.TargetWordCount, now,
			); err != nil {
				return fmt.Errorf("appending interlinking row: %w", err)
			}
		}
		if err := batch.Send(); err != nil {
			return fmt.Errorf("sending interlinking batch: %w", err)
		}
	}
	return nil
}

// ListInterlinkingOpportunities returns paginated interlinking opportunities.
func (s *Store) ListInterlinkingOpportunities(ctx context.Context, sessionID string, limit, offset int, filters []ParsedFilter, sort *SortParam) ([]InterlinkingOpportunity, int, error) {
	whereExtra, args, err := BuildWhereClause(filters)
	if err != nil {
		return nil, 0, err
	}

	where := "crawl_session_id = ?"
	baseArgs := []interface{}{sessionID}
	if whereExtra != "" {
		where += " AND " + whereExtra
		baseArgs = append(baseArgs, args...)
	}

	// Count
	var total uint64
	countArgs := make([]interface{}, len(baseArgs))
	copy(countArgs, baseArgs)
	if err := s.conn.QueryRow(ctx,
		`SELECT count() FROM crawlobserver.interlinking_opportunities WHERE `+where,
		countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting interlinking opportunities: %w", err)
	}

	orderBy := BuildOrderByClause(sort, "similarity DESC")
	queryArgs := make([]interface{}, len(baseArgs))
	copy(queryArgs, baseArgs)
	queryArgs = append(queryArgs, limit, offset)

	rows, err := s.conn.Query(ctx,
		`SELECT source_url, target_url, similarity, method,
			source_title, target_title, source_pagerank, target_pagerank,
			source_word_count, target_word_count
		FROM crawlobserver.interlinking_opportunities
		WHERE `+where+orderBy+` LIMIT ? OFFSET ?`,
		queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying interlinking opportunities: %w", err)
	}
	defer rows.Close()

	var result []InterlinkingOpportunity
	for rows.Next() {
		var o InterlinkingOpportunity
		o.CrawlSessionID = sessionID
		if err := rows.Scan(
			&o.SourceURL, &o.TargetURL, &o.Similarity, &o.Method,
			&o.SourceTitle, &o.TargetTitle, &o.SourcePageRank, &o.TargetPageRank,
			&o.SourceWordCount, &o.TargetWordCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning interlinking opportunity: %w", err)
		}
		result = append(result, o)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating interlinking opportunities: %w", err)
	}
	return result, int(total), nil
}

// LoadInternalLinkSet loads all internal links as a set of (source, target) pairs.
func (s *Store) LoadInternalLinkSet(ctx context.Context, sessionID string) (map[[2]string]struct{}, error) {
	rows, err := s.conn.Query(ctx,
		`SELECT source_url, target_url FROM crawlobserver.links
		WHERE crawl_session_id = ? AND is_internal = true`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying internal links: %w", err)
	}
	defer rows.Close()

	linkSet := make(map[[2]string]struct{})
	for rows.Next() {
		var src, tgt string
		if err := rows.Scan(&src, &tgt); err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}
		linkSet[[2]string{src, tgt}] = struct{}{}
	}
	return linkSet, rows.Err()
}

// LoadPageMetadata loads title, lang, pagerank, word_count, canonical for all pages in a session.
func (s *Store) LoadPageMetadata(ctx context.Context, sessionID string) (map[string]PageMetadata, error) {
	rows, err := s.conn.Query(ctx,
		`SELECT url, title, lang, pagerank, word_count, canonical, canonical_is_self
		FROM crawlobserver.pages
		WHERE crawl_session_id = ? AND status_code = 200
		  AND (final_url = '' OR final_url = url)`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying page metadata: %w", err)
	}
	defer rows.Close()

	meta := make(map[string]PageMetadata)
	for rows.Next() {
		var rawURL, title, lang, canonical string
		var pagerank float64
		var wordCount uint32
		var canonicalSelf bool
		if err := rows.Scan(&rawURL, &title, &lang, &pagerank, &wordCount, &canonical, &canonicalSelf); err != nil {
			return nil, fmt.Errorf("scanning page metadata: %w", err)
		}
		meta[rawURL] = PageMetadata{
			Title:         title,
			Lang:          lang,
			PageRank:      pagerank,
			WordCount:     wordCount,
			Canonical:     canonical,
			CanonicalSelf: canonicalSelf,
		}
	}
	return meta, rows.Err()
}

// LoadPageRankGraph loads the full link graph for a session, suitable for PageRank computation.
func (s *Store) LoadPageRankGraph(ctx context.Context, sessionID string) (*PageRankGraph, error) {
	if !isValidUUID(sessionID) {
		return nil, fmt.Errorf("invalid session ID: %s", sessionID)
	}

	// 1. Load all URLs and assign IDs
	urlRows, err := s.conn.Query(ctx,
		`SELECT url FROM crawlobserver.pages WHERE crawl_session_id = ?`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying URLs: %w", err)
	}
	defer urlRows.Close()

	urlToID := make(map[string]uint32)
	var idToURL []string
	for urlRows.Next() {
		var u string
		if err := urlRows.Scan(&u); err != nil {
			return nil, fmt.Errorf("scanning URL: %w", err)
		}
		urlToID[u] = uint32(len(idToURL))
		idToURL = append(idToURL, u)
	}
	if err := urlRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating URLs: %w", err)
	}

	n := uint32(len(idToURL))
	if n == 0 {
		return &PageRankGraph{}, nil
	}

	// 2. Build temp ID table (same pattern as ComputePageRank)
	idTable := fmt.Sprintf("crawlobserver.tmp_urlids_sim_%s", strings.ReplaceAll(sessionID, "-", ""))
	if err := s.conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", idTable)); err != nil {
		applog.Warnf("storage", "pre-cleanup temp table %s: %v", idTable, err)
	}
	if err := s.conn.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (url String, id UInt32) ENGINE = Join(ANY, LEFT, url)", idTable)); err != nil {
		return nil, fmt.Errorf("creating URL ID table: %w", err)
	}
	defer func() {
		if err := s.conn.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", idTable)); err != nil {
			applog.Warnf("storage", "cleanup temp table %s: %v", idTable, err)
		}
	}()

	const idChunk = 10000
	for i := 0; i < int(n); i += idChunk {
		end := i + idChunk
		if end > int(n) {
			end = int(n)
		}
		batch, err := s.conn.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s (url, id)", idTable))
		if err != nil {
			return nil, fmt.Errorf("preparing URL ID batch: %w", err)
		}
		for j := i; j < end; j++ {
			if err := batch.Append(idToURL[j], uint32(j)); err != nil {
				return nil, fmt.Errorf("appending URL ID: %w", err)
			}
		}
		if err := batch.Send(); err != nil {
			return nil, fmt.Errorf("sending URL ID batch: %w", err)
		}
	}

	// 3. Load total outlink counts
	totalOutLinks := make([]uint32, n)
	countRows, err := s.conn.Query(ctx, fmt.Sprintf(`
		SELECT
			joinGet('%s', 'id', source_url) AS src_id,
			toUInt32(uniqExact(target_url)) AS total_outlinks
		FROM crawlobserver.links
		WHERE crawl_session_id = ?
			AND source_url IN (SELECT url FROM %s)
			AND source_url != target_url
		GROUP BY src_id`, idTable, idTable), sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying total outlink counts: %w", err)
	}
	defer countRows.Close()

	for countRows.Next() {
		var srcID, cnt uint32
		if err := countRows.Scan(&srcID, &cnt); err != nil {
			return nil, fmt.Errorf("scanning outlink count: %w", err)
		}
		totalOutLinks[srcID] = cnt
	}

	// 4. Load internal dofollow links
	linkRows, err := s.conn.Query(ctx, fmt.Sprintf(`
		SELECT
			joinGet('%s', 'id', source_url) AS src_id,
			joinGet('%s', 'id', target_url) AS tgt_id
		FROM crawlobserver.links
		WHERE crawl_session_id = ? AND is_internal = true
			AND NOT hasAny(splitByString(' ', lower(rel)), ['nofollow', 'sponsored', 'ugc'])
			AND source_url IN (SELECT url FROM %s)
			AND target_url IN (SELECT url FROM %s)
		GROUP BY src_id, tgt_id
		HAVING src_id != tgt_id`, idTable, idTable, idTable, idTable), sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying links: %w", err)
	}
	defer linkRows.Close()

	outLinks := make([][]uint32, n)
	for linkRows.Next() {
		var srcID, tgtID uint32
		if err := linkRows.Scan(&srcID, &tgtID); err != nil {
			return nil, fmt.Errorf("scanning link IDs: %w", err)
		}
		outLinks[srcID] = append(outLinks[srcID], tgtID)
	}

	return &PageRankGraph{
		N:             n,
		OutLinks:      outLinks,
		TotalOutLinks: totalOutLinks,
		URLToID:       urlToID,
		IDToURL:       idToURL,
	}, nil
}

// InsertSimulation stores simulation metadata and per-page results.
func (s *Store) InsertSimulation(ctx context.Context, sessionID string, simID string, virtualLinks []VirtualLink, results []SimulationResultRow, meta SimulationMeta) error {
	now := time.Now()

	// Insert simulation metadata
	batch, err := s.conn.PrepareBatch(ctx,
		`INSERT INTO crawlobserver.interlinking_simulations (
			id, crawl_session_id, virtual_links_count, pages_improved, pages_declined,
			avg_diff, max_diff, computed_at)`)
	if err != nil {
		return fmt.Errorf("preparing simulation meta batch: %w", err)
	}
	if err := batch.Append(simID, sessionID, meta.VirtualLinksCount, meta.PagesImproved, meta.PagesDeclined, meta.AvgDiff, meta.MaxDiff, now); err != nil {
		return fmt.Errorf("appending simulation meta: %w", err)
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("sending simulation meta: %w", err)
	}

	// Insert results in chunks
	const chunkSize = 5000
	for i := 0; i < len(results); i += chunkSize {
		end := i + chunkSize
		if end > len(results) {
			end = len(results)
		}
		resBatch, err := s.conn.PrepareBatch(ctx,
			`INSERT INTO crawlobserver.interlinking_simulation_results (
				simulation_id, crawl_session_id, url, pagerank_before, pagerank_after, pagerank_diff, computed_at)`)
		if err != nil {
			return fmt.Errorf("preparing simulation results batch: %w", err)
		}
		for _, r := range results[i:end] {
			if err := resBatch.Append(simID, sessionID, r.URL, r.PageRankBefore, r.PageRankAfter, r.PageRankDiff, now); err != nil {
				return fmt.Errorf("appending simulation result: %w", err)
			}
		}
		if err := resBatch.Send(); err != nil {
			return fmt.Errorf("sending simulation results batch: %w", err)
		}
	}

	return nil
}

// ListSimulations returns all simulations for a session.
func (s *Store) ListSimulations(ctx context.Context, sessionID string) ([]SimulationMeta, error) {
	rows, err := s.conn.Query(ctx,
		`SELECT id, crawl_session_id, virtual_links_count, pages_improved, pages_declined,
			avg_diff, max_diff, computed_at
		FROM crawlobserver.interlinking_simulations
		WHERE crawl_session_id = ?
		ORDER BY computed_at DESC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying simulations: %w", err)
	}
	defer rows.Close()

	var result []SimulationMeta
	for rows.Next() {
		var m SimulationMeta
		if err := rows.Scan(&m.ID, &m.CrawlSessionID, &m.VirtualLinksCount, &m.PagesImproved, &m.PagesDeclined, &m.AvgDiff, &m.MaxDiff, &m.ComputedAt); err != nil {
			return nil, fmt.Errorf("scanning simulation: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// GetSimulation returns metadata for a single simulation.
func (s *Store) GetSimulation(ctx context.Context, sessionID, simID string) (*SimulationMeta, error) {
	var m SimulationMeta
	err := s.conn.QueryRow(ctx,
		`SELECT id, crawl_session_id, virtual_links_count, pages_improved, pages_declined,
			avg_diff, max_diff, computed_at
		FROM crawlobserver.interlinking_simulations
		WHERE crawl_session_id = ? AND id = ?`, sessionID, simID).Scan(
		&m.ID, &m.CrawlSessionID, &m.VirtualLinksCount, &m.PagesImproved, &m.PagesDeclined, &m.AvgDiff, &m.MaxDiff, &m.ComputedAt)
	if err != nil {
		return nil, fmt.Errorf("querying simulation: %w", err)
	}
	return &m, nil
}

// ListSimulationResults returns paginated simulation results.
func (s *Store) ListSimulationResults(ctx context.Context, sessionID, simID string, limit, offset int, filters []ParsedFilter, sort *SortParam) ([]SimulationResultRow, int, error) {
	whereExtra, args, err := BuildWhereClause(filters)
	if err != nil {
		return nil, 0, err
	}

	where := "crawl_session_id = ? AND simulation_id = ?"
	baseArgs := []interface{}{sessionID, simID}
	if whereExtra != "" {
		where += " AND " + whereExtra
		baseArgs = append(baseArgs, args...)
	}

	var total uint64
	countArgs := make([]interface{}, len(baseArgs))
	copy(countArgs, baseArgs)
	if err := s.conn.QueryRow(ctx,
		`SELECT count() FROM crawlobserver.interlinking_simulation_results WHERE `+where,
		countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting simulation results: %w", err)
	}

	orderBy := BuildOrderByClause(sort, "pagerank_diff DESC")
	queryArgs := make([]interface{}, len(baseArgs))
	copy(queryArgs, baseArgs)
	queryArgs = append(queryArgs, limit, offset)

	rows, err := s.conn.Query(ctx,
		`SELECT url, pagerank_before, pagerank_after, pagerank_diff
		FROM crawlobserver.interlinking_simulation_results
		WHERE `+where+orderBy+` LIMIT ? OFFSET ?`,
		queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying simulation results: %w", err)
	}
	defer rows.Close()

	var result []SimulationResultRow
	for rows.Next() {
		var r SimulationResultRow
		if err := rows.Scan(&r.URL, &r.PageRankBefore, &r.PageRankAfter, &r.PageRankDiff); err != nil {
			return nil, 0, fmt.Errorf("scanning simulation result: %w", err)
		}
		result = append(result, r)
	}
	return result, int(total), rows.Err()
}
