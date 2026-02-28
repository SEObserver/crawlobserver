package storage

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

const importPageBatch = 1000
const importLinkBatch = 5000

// ImportSession reads a gzipped JSONL stream and inserts the session with a new UUID.
func (s *Store) ImportSession(ctx context.Context, r io.Reader) (*CrawlSession, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close()

	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 0, 4*1024*1024), 16*1024*1024) // up to 16 MB per line for body_html

	// First line must be meta
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading meta line: %w", err)
		}
		return nil, fmt.Errorf("empty export file")
	}

	var meta exportRecord
	if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
		return nil, fmt.Errorf("decoding meta line: %w", err)
	}
	if meta.Type != RecordMeta || meta.Session == nil {
		return nil, fmt.Errorf("first line must be meta record with session data")
	}
	if meta.Version > ExportFormatVersion {
		return nil, fmt.Errorf("unsupported export format version %d (max %d)", meta.Version, ExportFormatVersion)
	}

	newID := uuid.New().String()
	sess := &CrawlSession{
		ID:           newID,
		StartedAt:    meta.Session.StartedAt,
		FinishedAt:   meta.Session.FinishedAt,
		Status:       "imported",
		SeedURLs:     meta.Session.SeedURLs,
		Config:       meta.Session.Config,
		PagesCrawled: meta.Session.PagesCrawled,
		UserAgent:    meta.Session.UserAgent,
		ProjectID:    meta.Session.ProjectID,
	}

	if err := s.InsertSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("inserting session: %w", err)
	}

	var pageBuf []PageRow
	var linkBuf []LinkRow
	var robotsBuf []RobotsRow
	var sitemapBuf []SitemapRow
	var sitemapURLBuf []SitemapURLRow

	flushPages := func() error {
		if len(pageBuf) == 0 {
			return nil
		}
		if err := s.InsertPages(ctx, pageBuf); err != nil {
			return fmt.Errorf("inserting pages batch: %w", err)
		}
		pageBuf = pageBuf[:0]
		return nil
	}
	flushLinks := func() error {
		if len(linkBuf) == 0 {
			return nil
		}
		if err := s.InsertLinks(ctx, linkBuf); err != nil {
			return fmt.Errorf("inserting links batch: %w", err)
		}
		linkBuf = linkBuf[:0]
		return nil
	}

	for scanner.Scan() {
		var rec exportRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("decoding record: %w", err)
		}

		switch rec.Type {
		case RecordPage:
			var p exportPage
			if err := json.Unmarshal(rec.Data, &p); err != nil {
				return nil, fmt.Errorf("decoding page: %w", err)
			}
			pageBuf = append(pageBuf, pageFromExport(newID, &p))
			if len(pageBuf) >= importPageBatch {
				if err := flushPages(); err != nil {
					return nil, err
				}
			}

		case RecordLink:
			var l exportLink
			if err := json.Unmarshal(rec.Data, &l); err != nil {
				return nil, fmt.Errorf("decoding link: %w", err)
			}
			linkBuf = append(linkBuf, linkFromExport(newID, &l))
			if len(linkBuf) >= importLinkBatch {
				if err := flushLinks(); err != nil {
					return nil, err
				}
			}

		case RecordRobots:
			var r exportRobots
			if err := json.Unmarshal(rec.Data, &r); err != nil {
				return nil, fmt.Errorf("decoding robots: %w", err)
			}
			robotsBuf = append(robotsBuf, RobotsRow{
				CrawlSessionID: newID,
				Host:           r.Host,
				StatusCode:     r.StatusCode,
				Content:        r.Content,
				FetchedAt:      r.FetchedAt,
			})

		case RecordSitemap:
			var sm exportSitemap
			if err := json.Unmarshal(rec.Data, &sm); err != nil {
				return nil, fmt.Errorf("decoding sitemap: %w", err)
			}
			sitemapBuf = append(sitemapBuf, SitemapRow{
				CrawlSessionID: newID,
				URL:            sm.URL,
				Type:           sm.Type,
				URLCount:       sm.URLCount,
				ParentURL:      sm.ParentURL,
				StatusCode:     sm.StatusCode,
				FetchedAt:      sm.FetchedAt,
			})

		case RecordSitemapURL:
			var su exportSitemapURL
			if err := json.Unmarshal(rec.Data, &su); err != nil {
				return nil, fmt.Errorf("decoding sitemap_url: %w", err)
			}
			sitemapURLBuf = append(sitemapURLBuf, SitemapURLRow{
				CrawlSessionID: newID,
				SitemapURL:     su.SitemapURL,
				Loc:            su.Loc,
				LastMod:        su.LastMod,
				ChangeFreq:     su.ChangeFreq,
				Priority:       su.Priority,
			})

		case RecordMeta:
			// ignore duplicate meta lines
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning records: %w", err)
	}

	// Flush remaining buffers
	if err := flushPages(); err != nil {
		return nil, err
	}
	if err := flushLinks(); err != nil {
		return nil, err
	}
	if err := s.InsertRobotsData(ctx, robotsBuf); err != nil {
		return nil, fmt.Errorf("inserting robots: %w", err)
	}
	if err := s.InsertSitemaps(ctx, sitemapBuf); err != nil {
		return nil, fmt.Errorf("inserting sitemaps: %w", err)
	}
	if err := s.InsertSitemapURLs(ctx, sitemapURLBuf); err != nil {
		return nil, fmt.Errorf("inserting sitemap urls: %w", err)
	}

	return sess, nil
}

func pageFromExport(sessionID string, p *exportPage) PageRow {
	hreflang := p.Hreflang
	if hreflang == nil {
		hreflang = []HreflangRow{}
	}
	chain := p.RedirectChain
	if chain == nil {
		chain = []RedirectHopRow{}
	}
	headers := p.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	h1 := p.H1
	if h1 == nil {
		h1 = []string{}
	}
	h2 := p.H2
	if h2 == nil {
		h2 = []string{}
	}
	h3 := p.H3
	if h3 == nil {
		h3 = []string{}
	}
	h4 := p.H4
	if h4 == nil {
		h4 = []string{}
	}
	h5 := p.H5
	if h5 == nil {
		h5 = []string{}
	}
	h6 := p.H6
	if h6 == nil {
		h6 = []string{}
	}
	schemaTypes := p.SchemaTypes
	if schemaTypes == nil {
		schemaTypes = []string{}
	}

	return PageRow{
		CrawlSessionID:  sessionID,
		URL:             p.URL,
		FinalURL:        p.FinalURL,
		StatusCode:      p.StatusCode,
		ContentType:     p.ContentType,
		Title:           p.Title,
		TitleLength:     p.TitleLength,
		Canonical:       p.Canonical,
		CanonicalIsSelf: p.CanonicalIsSelf,
		IsIndexable:     p.IsIndexable,
		IndexReason:     p.IndexReason,
		MetaRobots:      p.MetaRobots,
		MetaDescription: p.MetaDescription,
		MetaDescLength:  p.MetaDescLength,
		MetaKeywords:    p.MetaKeywords,
		H1:              h1,
		H2:              h2,
		H3:              h3,
		H4:              h4,
		H5:              h5,
		H6:              h6,
		WordCount:       p.WordCount,
		InternalLinksOut: p.InternalLinksOut,
		ExternalLinksOut: p.ExternalLinksOut,
		ImagesCount:     p.ImagesCount,
		ImagesNoAlt:     p.ImagesNoAlt,
		Hreflang:        hreflang,
		Lang:            p.Lang,
		OGTitle:         p.OGTitle,
		OGDescription:   p.OGDescription,
		OGImage:         p.OGImage,
		SchemaTypes:     schemaTypes,
		Headers:         headers,
		RedirectChain:   chain,
		BodySize:        p.BodySize,
		FetchDurationMs: p.FetchDurationMs,
		ContentEncoding: p.ContentEncoding,
		XRobotsTag:      p.XRobotsTag,
		Error:           p.Error,
		Depth:           p.Depth,
		FoundOn:         p.FoundOn,
		PageRank:        p.PageRank,
		BodyHTML:        p.BodyHTML,
		BodyTruncated:   p.BodyTruncated,
		CrawledAt:       p.CrawledAt,
	}
}

func linkFromExport(sessionID string, l *exportLink) LinkRow {
	crawledAt := l.CrawledAt
	if crawledAt.IsZero() {
		crawledAt = time.Now()
	}
	return LinkRow{
		CrawlSessionID: sessionID,
		SourceURL:      l.SourceURL,
		TargetURL:      l.TargetURL,
		AnchorText:     l.AnchorText,
		Rel:            l.Rel,
		IsInternal:     l.IsInternal,
		Tag:            l.Tag,
		CrawledAt:      crawledAt,
	}
}
