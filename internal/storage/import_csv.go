package storage

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CSVSource identifies the CSV format variant.
type CSVSource string

const (
	CSVSourceAddressBased CSVSource = "address-based"
	CSVSourceURLBased     CSVSource = "url-based"
)

// CSVImportResult holds the outcome of a CSV import.
type CSVImportResult struct {
	Session     *CrawlSession `json:"session"`
	RowsImported int          `json:"rows_imported"`
	RowsSkipped  int          `json:"rows_skipped"`
}

// ImportCSVSession reads a CSV file, auto-detects the source, maps columns
// to PageRow, creates a session, and batch-inserts pages.
func (s *Store) ImportCSVSession(ctx context.Context, r io.Reader, projectID string) (*CSVImportResult, error) {
	// Read all data so we can retry with different delimiters.
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading CSV data: %w", err)
	}

	// Strip UTF-8 BOM if present.
	raw = stripBOM(raw)

	// Detect delimiter and source.
	source, headers, csvReader, err := detectCSV(raw)
	if err != nil {
		return nil, err
	}

	// Build column index map.
	colIdx := make(map[string]int, len(headers))
	for i, h := range headers {
		colIdx[h] = i
	}

	now := time.Now()
	newID := uuid.New().String()

	var pageBuf []PageRow
	var rowsImported, rowsSkipped int
	var firstURL string

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

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed rows.
			rowsSkipped++
			continue
		}

		page := csvRowToPage(source, colIdx, record, newID, now)
		if page.URL == "" {
			rowsSkipped++
			continue
		}

		if firstURL == "" {
			firstURL = page.URL
		}

		rowsImported++
		pageBuf = append(pageBuf, page)
		if len(pageBuf) >= importPageBatch {
			if err := flushPages(); err != nil {
				return nil, err
			}
		}
	}

	// Flush remaining.
	if err := flushPages(); err != nil {
		return nil, err
	}

	// Extract seed domain from first URL.
	var seedURLs []string
	if firstURL != "" {
		if u, err := url.Parse(firstURL); err == nil && u.Host != "" {
			seedURLs = []string{u.Scheme + "://" + u.Host}
		}
	}

	configJSON := fmt.Sprintf(`{"source":%q,"import_type":"csv"}`, source)

	var projPtr *string
	if projectID != "" {
		projPtr = &projectID
	}

	sess := &CrawlSession{
		ID:           newID,
		StartedAt:    now,
		FinishedAt:   now,
		Status:       "imported",
		SeedURLs:     seedURLs,
		Config:       configJSON,
		PagesCrawled: uint64(rowsImported),
		ProjectID:    projPtr,
	}

	if err := s.InsertSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("inserting session: %w", err)
	}

	return &CSVImportResult{
		Session:      sess,
		RowsImported: rowsImported,
		RowsSkipped:  rowsSkipped,
	}, nil
}

// stripBOM removes the UTF-8 BOM prefix if present.
func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// detectCSV auto-detects the CSV source and returns headers + a reader positioned after the header row.
func detectCSV(data []byte) (CSVSource, []string, *csv.Reader, error) {
	// Try comma first.
	source, headers, reader, err := tryDetect(data, ',')
	if err == nil {
		return source, headers, reader, nil
	}

	// Retry with semicolon.
	source, headers, reader, err = tryDetect(data, ';')
	if err == nil {
		return source, headers, reader, nil
	}

	return "", nil, nil, fmt.Errorf("unrecognized CSV format: headers do not match any supported crawler export")
}

func tryDetect(data []byte, delimiter rune) (CSVSource, []string, *csv.Reader, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delimiter
	r.LazyQuotes = true
	r.FieldsPerRecord = -1 // variable

	row1, err := r.Read()
	if err != nil {
		return "", nil, nil, err
	}

	// Check row 1 for known headers.
	if source, ok := identifySource(row1); ok {
		return source, row1, r, nil
	}

	// SF sometimes puts metadata in row 1 — check row 2.
	row2, err := r.Read()
	if err != nil {
		return "", nil, nil, fmt.Errorf("no recognizable headers")
	}

	if source, ok := identifySource(row2); ok {
		return source, row2, r, nil
	}

	return "", nil, nil, fmt.Errorf("no recognizable headers")
}

func identifySource(headers []string) (CSVSource, bool) {
	headerSet := make(map[string]bool, len(headers))
	for _, h := range headers {
		headerSet[strings.TrimSpace(h)] = true
	}

	// Address-based format: uses "Address" column for URLs.
	if headerSet["Address"] {
		return CSVSourceAddressBased, true
	}

	// URL-based format: uses "URL" + "Page Title" or "HTTP Status Code".
	if headerSet["URL"] && (headerSet["Page Title"] || headerSet["HTTP Status Code"]) {
		return CSVSourceURLBased, true
	}

	return "", false
}

// csvRowToPage maps a CSV row to a PageRow based on the detected source.
func csvRowToPage(source CSVSource, colIdx map[string]int, record []string, sessionID string, crawledAt time.Time) PageRow {
	get := func(col string) string {
		if idx, ok := colIdx[col]; ok && idx < len(record) {
			return strings.TrimSpace(record[idx])
		}
		return ""
	}

	p := PageRow{
		CrawlSessionID: sessionID,
		CrawledAt:       crawledAt,
		H1:              []string{},
		H2:              []string{},
		H3:              []string{},
		H4:              []string{},
		H5:              []string{},
		H6:              []string{},
		Hreflang:        []HreflangRow{},
		SchemaTypes:     []string{},
		Headers:         map[string]string{},
		RedirectChain:   []RedirectHopRow{},
	}

	switch source {
	case CSVSourceAddressBased:
		p.URL = get("Address")
		p.StatusCode = parseUint16(get("Status Code"))
		p.ContentType = get("Content")
		p.Title = get("Title 1")
		p.TitleLength = parseUint16(get("Title 1 Length"))
		p.MetaDescription = get("Meta Description 1")
		p.MetaDescLength = parseUint16(get("Meta Description 1 Length"))
		p.MetaKeywords = get("Meta Keywords 1")
		p.Canonical = get("Canonical Link Element 1")
		p.IsIndexable = get("Indexability") == "Indexable"
		p.IndexReason = get("Indexability Status")
		p.MetaRobots = get("Meta Robots 1")
		p.XRobotsTag = get("X-Robots-Tag 1")
		p.WordCount = parseUint32(get("Word Count"))
		p.InternalLinksOut = parseUint32(get("Unique Outlinks"))
		p.ExternalLinksOut = parseUint32(get("Unique External Outlinks"))
		p.BodySize = parseUint64(get("Size"))
		p.Depth = parseUint16(get("Crawl Depth"))
		p.Lang = get("Language")
		p.FinalURL = get("Redirect URI")
		p.ImagesCount = parseUint16(get("Images"))

		// Response Time: seconds → milliseconds
		if rt := get("Response Time"); rt != "" {
			if f, err := strconv.ParseFloat(rt, 64); err == nil {
				p.FetchDurationMs = uint64(math.Round(f * 1000))
			}
		}

		// Link Score: 0-100 → 0-1
		if ls := get("Link Score"); ls != "" {
			if f, err := strconv.ParseFloat(ls, 64); err == nil {
				p.PageRank = f / 100.0
			}
		}

		// Hash: hex string → uint64
		if h := get("Hash"); h != "" {
			if v, err := strconv.ParseUint(h, 16, 64); err == nil {
				p.ContentHash = v
			}
		}

		// H1/H2 (multiple columns)
		p.H1 = collectMulti(get, "H1-1", "H1-2")
		p.H2 = collectMulti(get, "H2-1", "H2-2")

	case CSVSourceURLBased:
		p.URL = get("URL")
		p.StatusCode = parseUint16(get("HTTP Status Code"))
		p.Title = get("Page Title")
		p.TitleLength = parseUint16(get("Title Length"))
		p.MetaDescription = get("Meta Description")
		p.MetaDescLength = parseUint16(get("Meta Description Length"))
		p.Canonical = get("Canonical URL")
		p.WordCount = parseUint32(get("Word Count"))
		p.Depth = parseUint16(get("Crawl Depth"))
		p.InternalLinksOut = parseUint32(get("Internal Outlinks"))
		p.ExternalLinksOut = parseUint32(get("External Outlinks"))
		p.MetaRobots = get("Meta Robots")
		p.XRobotsTag = get("X-Robots-Tag")
		p.FinalURL = get("Redirect URL")

		// URL Rank: 0-100 → 0-1
		if ur := get("URL Rank (UR)"); ur != "" {
			if f, err := strconv.ParseFloat(ur, 64); err == nil {
				p.PageRank = f / 100.0
			}
		}

		// H1/H2 single columns
		if h1 := get("H1"); h1 != "" {
			p.H1 = []string{h1}
		}
		if h2 := get("H2"); h2 != "" {
			p.H2 = []string{h2}
		}
	}

	// Compute CanonicalIsSelf.
	if p.Canonical != "" && p.URL != "" {
		p.CanonicalIsSelf = p.Canonical == p.URL
	}

	return p
}

// collectMulti gathers non-empty values from multiple columns.
func collectMulti(get func(string) string, cols ...string) []string {
	var result []string
	for _, col := range cols {
		if v := get(col); v != "" {
			result = append(result, v)
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

func parseUint16(s string) uint16 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseUint(s, 10, 16)
	return uint16(v)
}

func parseUint32(s string) uint32 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseUint(s, 10, 32)
	return uint32(v)
}

func parseUint64(s string) uint64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}
