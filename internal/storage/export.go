package storage

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Export JSONL record types.
const (
	RecordMeta       = "meta"
	RecordPage       = "page"
	RecordLink       = "link"
	RecordRobots     = "robots"
	RecordSitemap    = "sitemap"
	RecordSitemapURL = "sitemap_url"
)

// ExportFormatVersion is the current export format version.
const ExportFormatVersion = 1

// exportRecord is a typed JSONL line.
type exportRecord struct {
	Type    string          `json:"t"`
	Version int             `json:"v,omitempty"`
	Session *exportSession  `json:"session,omitempty"`
	Data    json.RawMessage `json:"d,omitempty"`
}

type exportSession struct {
	ID           string    `json:"id"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Status       string    `json:"status"`
	SeedURLs     []string  `json:"seed_urls"`
	Config       string    `json:"config"`
	PagesCrawled uint64    `json:"pages_crawled"`
	UserAgent    string    `json:"user_agent"`
	ProjectID    *string   `json:"project_id,omitempty"`
}

type exportPage struct {
	URL              string            `json:"url"`
	FinalURL         string            `json:"final_url"`
	StatusCode       uint16            `json:"status_code"`
	ContentType      string            `json:"content_type"`
	Title            string            `json:"title"`
	TitleLength      uint16            `json:"title_length"`
	Canonical        string            `json:"canonical"`
	CanonicalIsSelf  bool              `json:"canonical_is_self"`
	IsIndexable      bool              `json:"is_indexable"`
	IndexReason      string            `json:"index_reason"`
	MetaRobots       string            `json:"meta_robots"`
	MetaDescription  string            `json:"meta_description"`
	MetaDescLength   uint16            `json:"meta_desc_length"`
	MetaKeywords     string            `json:"meta_keywords"`
	H1               []string          `json:"h1"`
	H2               []string          `json:"h2"`
	H3               []string          `json:"h3"`
	H4               []string          `json:"h4"`
	H5               []string          `json:"h5"`
	H6               []string          `json:"h6"`
	WordCount        uint32            `json:"word_count"`
	InternalLinksOut uint32            `json:"internal_links_out"`
	ExternalLinksOut uint32            `json:"external_links_out"`
	ImagesCount      uint16            `json:"images_count"`
	ImagesNoAlt      uint16            `json:"images_no_alt"`
	Hreflang         []HreflangRow     `json:"hreflang,omitempty"`
	Lang             string            `json:"lang"`
	OGTitle          string            `json:"og_title"`
	OGDescription    string            `json:"og_description"`
	OGImage          string            `json:"og_image"`
	SchemaTypes      []string          `json:"schema_types,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	RedirectChain    []RedirectHopRow  `json:"redirect_chain,omitempty"`
	BodySize         uint64            `json:"body_size"`
	FetchDurationMs  uint64            `json:"fetch_duration_ms"`
	ContentEncoding  string            `json:"content_encoding"`
	XRobotsTag       string            `json:"x_robots_tag"`
	Error            string            `json:"error"`
	Depth            uint16            `json:"depth"`
	FoundOn          string            `json:"found_on"`
	PageRank         float64           `json:"pagerank"`
	BodyHTML         string            `json:"body_html,omitempty"`
	BodyTruncated    bool              `json:"body_truncated"`
	CrawledAt        time.Time         `json:"crawled_at"`
}

type exportLink struct {
	SourceURL  string    `json:"source_url"`
	TargetURL  string    `json:"target_url"`
	AnchorText string    `json:"anchor_text"`
	Rel        string    `json:"rel"`
	IsInternal bool      `json:"is_internal"`
	Tag        string    `json:"tag"`
	CrawledAt  time.Time `json:"crawled_at"`
}

type exportRobots struct {
	Host       string    `json:"host"`
	StatusCode uint16    `json:"status_code"`
	Content    string    `json:"content"`
	FetchedAt  time.Time `json:"fetched_at"`
}

type exportSitemap struct {
	URL        string    `json:"url"`
	Type       string    `json:"type"`
	URLCount   uint32    `json:"url_count"`
	ParentURL  string    `json:"parent_url"`
	StatusCode uint16    `json:"status_code"`
	FetchedAt  time.Time `json:"fetched_at"`
}

type exportSitemapURL struct {
	SitemapURL string `json:"sitemap_url"`
	Loc        string `json:"loc"`
	LastMod    string `json:"lastmod"`
	ChangeFreq string `json:"changefreq"`
	Priority   string `json:"priority"`
}

const exportPageBatch = 5000
const exportLinkBatch = 10000

// ExportSession streams a session's data as gzipped JSONL to w.
func (s *Store) ExportSession(ctx context.Context, sessionID string, w io.Writer, includeHTML bool) error {
	sess, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("getting session: %w", err)
	}

	gz := gzip.NewWriter(w)
	defer gz.Close()
	enc := json.NewEncoder(gz)

	// Meta line
	if err := enc.Encode(exportRecord{
		Type:    RecordMeta,
		Version: ExportFormatVersion,
		Session: &exportSession{
			ID:           sess.ID,
			StartedAt:    sess.StartedAt,
			FinishedAt:   sess.FinishedAt,
			Status:       sess.Status,
			SeedURLs:     sess.SeedURLs,
			Config:       sess.Config,
			PagesCrawled: sess.PagesCrawled,
			UserAgent:    sess.UserAgent,
			ProjectID:    sess.ProjectID,
		},
	}); err != nil {
		return fmt.Errorf("writing meta: %w", err)
	}

	// Pages (batched direct query with optional body_html)
	if err := s.exportPages(ctx, enc, sessionID, includeHTML); err != nil {
		return fmt.Errorf("exporting pages: %w", err)
	}

	// Links (batched direct query)
	if err := s.exportLinks(ctx, enc, sessionID); err != nil {
		return fmt.Errorf("exporting links: %w", err)
	}

	// Robots
	if err := s.exportRobots(ctx, enc, sessionID); err != nil {
		return fmt.Errorf("exporting robots: %w", err)
	}

	// Sitemaps
	if err := s.exportSitemaps(ctx, enc, sessionID); err != nil {
		return fmt.Errorf("exporting sitemaps: %w", err)
	}

	// Sitemap URLs
	if err := s.exportSitemapURLs(ctx, enc, sessionID); err != nil {
		return fmt.Errorf("exporting sitemap urls: %w", err)
	}

	return gz.Close()
}

func (s *Store) exportPages(ctx context.Context, enc *json.Encoder, sessionID string, includeHTML bool) error {
	htmlCol := "''"
	if includeHTML {
		htmlCol = "body_html"
	}

	query := fmt.Sprintf(`
		SELECT url, final_url, status_code, content_type,
			title, title_length, canonical, canonical_is_self, is_indexable, index_reason,
			meta_robots, meta_description, meta_desc_length, meta_keywords,
			h1, h2, h3, h4, h5, h6,
			word_count, internal_links_out, external_links_out,
			images_count, images_no_alt, hreflang,
			lang, og_title, og_description, og_image, schema_types,
			headers, redirect_chain, body_size, fetch_duration_ms,
			content_encoding, x_robots_tag,
			error, depth, found_on, pagerank, %s, body_truncated, crawled_at
		FROM crawlobserver.pages
		WHERE crawl_session_id = ?
		ORDER BY url
		LIMIT ? OFFSET ?`, htmlCol)

	for offset := 0; ; offset += exportPageBatch {
		rows, err := s.conn.Query(ctx, query, sessionID, exportPageBatch, offset)
		if err != nil {
			return fmt.Errorf("querying pages at offset %d: %w", offset, err)
		}

		count := 0
		for rows.Next() {
			var p exportPage
			var hreflangRaw []map[string]interface{}
			var chainRaw []map[string]interface{}

			if err := rows.Scan(
				&p.URL, &p.FinalURL, &p.StatusCode, &p.ContentType,
				&p.Title, &p.TitleLength, &p.Canonical, &p.CanonicalIsSelf, &p.IsIndexable, &p.IndexReason,
				&p.MetaRobots, &p.MetaDescription, &p.MetaDescLength, &p.MetaKeywords,
				&p.H1, &p.H2, &p.H3, &p.H4, &p.H5, &p.H6,
				&p.WordCount, &p.InternalLinksOut, &p.ExternalLinksOut,
				&p.ImagesCount, &p.ImagesNoAlt, &hreflangRaw,
				&p.Lang, &p.OGTitle, &p.OGDescription, &p.OGImage, &p.SchemaTypes,
				&p.Headers, &chainRaw, &p.BodySize, &p.FetchDurationMs,
				&p.ContentEncoding, &p.XRobotsTag,
				&p.Error, &p.Depth, &p.FoundOn, &p.PageRank, &p.BodyHTML, &p.BodyTruncated, &p.CrawledAt,
			); err != nil {
				rows.Close()
				return fmt.Errorf("scanning page: %w", err)
			}

			// Convert ClickHouse tuples to typed structs
			p.Hreflang = make([]HreflangRow, len(hreflangRaw))
			for i, m := range hreflangRaw {
				p.Hreflang[i] = HreflangRow{
					Lang: fmt.Sprint(m["lang"]),
					URL:  fmt.Sprint(m["url"]),
				}
			}
			p.RedirectChain = make([]RedirectHopRow, len(chainRaw))
			for i, m := range chainRaw {
				p.RedirectChain[i] = RedirectHopRow{
					URL: fmt.Sprint(m["url"]),
				}
				if sc, ok := m["status_code"]; ok {
					if v, ok := sc.(uint16); ok {
						p.RedirectChain[i].StatusCode = v
					}
				}
			}

			data, err := json.Marshal(p)
			if err != nil {
				rows.Close()
				return fmt.Errorf("marshaling page: %w", err)
			}
			if err := enc.Encode(exportRecord{Type: RecordPage, Data: data}); err != nil {
				rows.Close()
				return fmt.Errorf("writing page: %w", err)
			}
			count++
		}
		rows.Close()

		if count < exportPageBatch {
			break
		}
	}
	return nil
}

func (s *Store) exportLinks(ctx context.Context, enc *json.Encoder, sessionID string) error {
	query := `
		SELECT source_url, target_url, anchor_text, rel, is_internal, tag, crawled_at
		FROM crawlobserver.links
		WHERE crawl_session_id = ?
		ORDER BY source_url, target_url
		LIMIT ? OFFSET ?`

	for offset := 0; ; offset += exportLinkBatch {
		rows, err := s.conn.Query(ctx, query, sessionID, exportLinkBatch, offset)
		if err != nil {
			return fmt.Errorf("querying links at offset %d: %w", offset, err)
		}

		count := 0
		for rows.Next() {
			var l exportLink
			if err := rows.Scan(&l.SourceURL, &l.TargetURL, &l.AnchorText, &l.Rel, &l.IsInternal, &l.Tag, &l.CrawledAt); err != nil {
				rows.Close()
				return fmt.Errorf("scanning link: %w", err)
			}
			data, err := json.Marshal(l)
			if err != nil {
				rows.Close()
				return fmt.Errorf("marshaling link: %w", err)
			}
			if err := enc.Encode(exportRecord{Type: RecordLink, Data: data}); err != nil {
				rows.Close()
				return fmt.Errorf("writing link: %w", err)
			}
			count++
		}
		rows.Close()

		if count < exportLinkBatch {
			break
		}
	}
	return nil
}

func (s *Store) exportRobots(ctx context.Context, enc *json.Encoder, sessionID string) error {
	rows, err := s.conn.Query(ctx, `
		SELECT host, status_code, content, fetched_at
		FROM crawlobserver.robots_txt FINAL
		WHERE crawl_session_id = ?
		ORDER BY host`, sessionID)
	if err != nil {
		return fmt.Errorf("querying robots: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r exportRobots
		if err := rows.Scan(&r.Host, &r.StatusCode, &r.Content, &r.FetchedAt); err != nil {
			return fmt.Errorf("scanning robots: %w", err)
		}
		data, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("marshaling robots: %w", err)
		}
		if err := enc.Encode(exportRecord{Type: RecordRobots, Data: data}); err != nil {
			return fmt.Errorf("writing robots: %w", err)
		}
	}
	return nil
}

func (s *Store) exportSitemaps(ctx context.Context, enc *json.Encoder, sessionID string) error {
	rows, err := s.conn.Query(ctx, `
		SELECT url, type, url_count, parent_url, status_code, fetched_at
		FROM crawlobserver.sitemaps FINAL
		WHERE crawl_session_id = ?
		ORDER BY url`, sessionID)
	if err != nil {
		return fmt.Errorf("querying sitemaps: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sm exportSitemap
		if err := rows.Scan(&sm.URL, &sm.Type, &sm.URLCount, &sm.ParentURL, &sm.StatusCode, &sm.FetchedAt); err != nil {
			return fmt.Errorf("scanning sitemap: %w", err)
		}
		data, err := json.Marshal(sm)
		if err != nil {
			return fmt.Errorf("marshaling sitemap: %w", err)
		}
		if err := enc.Encode(exportRecord{Type: RecordSitemap, Data: data}); err != nil {
			return fmt.Errorf("writing sitemap: %w", err)
		}
	}
	return nil
}

func (s *Store) exportSitemapURLs(ctx context.Context, enc *json.Encoder, sessionID string) error {
	query := `
		SELECT sitemap_url, loc, lastmod, changefreq, priority
		FROM crawlobserver.sitemap_urls FINAL
		WHERE crawl_session_id = ?
		ORDER BY sitemap_url, loc
		LIMIT ? OFFSET ?`

	for offset := 0; ; offset += exportLinkBatch {
		rows, err := s.conn.Query(ctx, query, sessionID, exportLinkBatch, offset)
		if err != nil {
			return fmt.Errorf("querying sitemap urls at offset %d: %w", offset, err)
		}

		count := 0
		for rows.Next() {
			var su exportSitemapURL
			if err := rows.Scan(&su.SitemapURL, &su.Loc, &su.LastMod, &su.ChangeFreq, &su.Priority); err != nil {
				rows.Close()
				return fmt.Errorf("scanning sitemap url: %w", err)
			}
			data, err := json.Marshal(su)
			if err != nil {
				rows.Close()
				return fmt.Errorf("marshaling sitemap url: %w", err)
			}
			if err := enc.Encode(exportRecord{Type: RecordSitemapURL, Data: data}); err != nil {
				rows.Close()
				return fmt.Errorf("writing sitemap url: %w", err)
			}
			count++
		}
		rows.Close()

		if count < exportLinkBatch {
			break
		}
	}
	return nil
}
