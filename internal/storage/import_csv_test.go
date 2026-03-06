package storage

import (
	"strings"
	"testing"
	"time"
)

func TestDetectCSVSource_AddressBased(t *testing.T) {
	data := []byte("Address,Status Code,Content,Title 1\nhttp://example.com,200,text/html,Test\n")
	source, headers, _, err := detectCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if source != CSVSourceAddressBased {
		t.Errorf("expected address-based, got %s", source)
	}
	if headers[0] != "Address" {
		t.Errorf("expected first header Address, got %s", headers[0])
	}
}

func TestDetectCSVSource_AddressBasedRow2Header(t *testing.T) {
	// Row 1 is metadata, row 2 has actual headers.
	data := []byte("SEO Crawler Export,14.3\nAddress,Status Code,Content\nhttp://example.com,200,text/html\n")
	source, headers, _, err := detectCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if source != CSVSourceAddressBased {
		t.Errorf("expected address-based, got %s", source)
	}
	if headers[0] != "Address" {
		t.Errorf("expected first header Address, got %s", headers[0])
	}
}

func TestDetectCSVSource_URLBased(t *testing.T) {
	data := []byte("URL,Page Title,HTTP Status Code,Word Count\nhttps://example.com,Home,200,500\n")
	source, headers, _, err := detectCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if source != CSVSourceURLBased {
		t.Errorf("expected url-based, got %s", source)
	}
	if headers[0] != "URL" {
		t.Errorf("expected first header URL, got %s", headers[0])
	}
}

func TestDetectCSVSource_Unknown(t *testing.T) {
	data := []byte("col1,col2,col3\na,b,c\n")
	_, _, _, err := detectCSV(data)
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestDetectCSVSource_SemicolonDelimiter(t *testing.T) {
	data := []byte("Address;Status Code;Content;Title 1\nhttp://example.com;200;text/html;Test\n")
	source, _, _, err := detectCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if source != CSVSourceAddressBased {
		t.Errorf("expected address-based, got %s", source)
	}
}

func TestDetectCSVSource_BOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	data := append(bom, []byte("Address,Status Code,Content\nhttp://example.com,200,text/html\n")...)
	data = stripBOM(data)
	source, _, _, err := detectCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	if source != CSVSourceAddressBased {
		t.Errorf("expected address-based, got %s", source)
	}
}

func TestCsvRowToPage_AddressBased(t *testing.T) {
	headers := strings.Split("Address,Status Code,Content,Title 1,Title 1 Length,Meta Description 1,Meta Description 1 Length,Meta Keywords 1,Canonical Link Element 1,Indexability,Indexability Status,Meta Robots 1,X-Robots-Tag 1,H1-1,H1-2,H2-1,H2-2,Word Count,Unique Outlinks,Unique External Outlinks,Size,Response Time,Crawl Depth,Link Score,Language,Hash,Redirect URI,Images", ",")
	colIdx := make(map[string]int)
	for i, h := range headers {
		colIdx[h] = i
	}

	record := []string{
		"https://example.com/page",      // Address
		"200",                           // Status Code
		"text/html",                     // Content
		"My Title",                      // Title 1
		"8",                             // Title 1 Length
		"My description",               // Meta Description 1
		"14",                            // Meta Description 1 Length
		"kw1, kw2",                      // Meta Keywords 1
		"https://example.com/page",      // Canonical Link Element 1
		"Indexable",                     // Indexability
		"",                              // Indexability Status
		"index, follow",                 // Meta Robots 1
		"",                              // X-Robots-Tag 1
		"Main Heading",                  // H1-1
		"Second H1",                     // H1-2
		"Sub Heading",                   // H2-1
		"",                              // H2-2
		"1500",                          // Word Count
		"42",                            // Unique Outlinks
		"5",                             // Unique External Outlinks
		"65000",                         // Size
		"0.532",                         // Response Time (seconds)
		"2",                             // Crawl Depth
		"75",                            // Link Score (0-100)
		"en",                            // Language
		"1a2b3c4d",                      // Hash
		"",                              // Redirect URI
		"3",                             // Images
	}

	now := fixedTime()
	p := csvRowToPage(CSVSourceAddressBased, colIdx, record, "sess-1", now)

	assertEqual(t, "URL", p.URL, "https://example.com/page")
	assertEqualUint16(t, "StatusCode", p.StatusCode, 200)
	assertEqual(t, "ContentType", p.ContentType, "text/html")
	assertEqual(t, "Title", p.Title, "My Title")
	assertEqualUint16(t, "TitleLength", p.TitleLength, 8)
	assertEqual(t, "MetaDescription", p.MetaDescription, "My description")
	assertEqualUint16(t, "MetaDescLength", p.MetaDescLength, 14)
	assertEqual(t, "MetaKeywords", p.MetaKeywords, "kw1, kw2")
	assertEqual(t, "Canonical", p.Canonical, "https://example.com/page")
	if !p.CanonicalIsSelf {
		t.Error("expected CanonicalIsSelf=true")
	}
	if !p.IsIndexable {
		t.Error("expected IsIndexable=true")
	}
	assertEqual(t, "MetaRobots", p.MetaRobots, "index, follow")
	if len(p.H1) != 2 || p.H1[0] != "Main Heading" || p.H1[1] != "Second H1" {
		t.Errorf("H1 mismatch: %v", p.H1)
	}
	if len(p.H2) != 1 || p.H2[0] != "Sub Heading" {
		t.Errorf("H2 mismatch: %v", p.H2)
	}
	assertEqualUint32(t, "WordCount", p.WordCount, 1500)
	assertEqualUint32(t, "InternalLinksOut", p.InternalLinksOut, 42)
	assertEqualUint32(t, "ExternalLinksOut", p.ExternalLinksOut, 5)
	if p.BodySize != 65000 {
		t.Errorf("BodySize: expected 65000, got %d", p.BodySize)
	}
	if p.FetchDurationMs != 532 {
		t.Errorf("FetchDurationMs: expected 532, got %d", p.FetchDurationMs)
	}
	assertEqualUint16(t, "Depth", p.Depth, 2)
	if p.PageRank != 0.75 {
		t.Errorf("PageRank: expected 0.75, got %f", p.PageRank)
	}
	assertEqual(t, "Lang", p.Lang, "en")
	if p.ContentHash != 0x1a2b3c4d {
		t.Errorf("ContentHash: expected 0x1a2b3c4d, got %x", p.ContentHash)
	}
	assertEqualUint16(t, "ImagesCount", p.ImagesCount, 3)
}

func TestCsvRowToPage_URLBased(t *testing.T) {
	headers := strings.Split("URL,HTTP Status Code,Page Title,Title Length,Meta Description,Meta Description Length,Canonical URL,H1,H2,Word Count,Crawl Depth,URL Rank (UR),Internal Outlinks,External Outlinks,Meta Robots,X-Robots-Tag,Redirect URL", ",")
	colIdx := make(map[string]int)
	for i, h := range headers {
		colIdx[h] = i
	}

	record := []string{
		"https://example.com",     // URL
		"200",                     // HTTP Status Code
		"Home Page",               // Page Title
		"9",                       // Title Length
		"Welcome to our site",    // Meta Description
		"19",                      // Meta Description Length
		"https://example.com",    // Canonical URL
		"Welcome",                 // H1
		"About",                   // H2
		"800",                     // Word Count
		"0",                       // Crawl Depth
		"50",                      // URL Rank (UR)
		"30",                      // Internal Outlinks
		"10",                      // External Outlinks
		"index, follow",           // Meta Robots
		"",                        // X-Robots-Tag
		"",                        // Redirect URL
	}

	now := fixedTime()
	p := csvRowToPage(CSVSourceURLBased, colIdx, record, "sess-2", now)

	assertEqual(t, "URL", p.URL, "https://example.com")
	assertEqualUint16(t, "StatusCode", p.StatusCode, 200)
	assertEqual(t, "Title", p.Title, "Home Page")
	assertEqualUint16(t, "TitleLength", p.TitleLength, 9)
	assertEqual(t, "MetaDescription", p.MetaDescription, "Welcome to our site")
	assertEqualUint16(t, "MetaDescLength", p.MetaDescLength, 19)
	assertEqual(t, "Canonical", p.Canonical, "https://example.com")
	if !p.CanonicalIsSelf {
		t.Error("expected CanonicalIsSelf=true")
	}
	if len(p.H1) != 1 || p.H1[0] != "Welcome" {
		t.Errorf("H1 mismatch: %v", p.H1)
	}
	if len(p.H2) != 1 || p.H2[0] != "About" {
		t.Errorf("H2 mismatch: %v", p.H2)
	}
	assertEqualUint32(t, "WordCount", p.WordCount, 800)
	assertEqualUint16(t, "Depth", p.Depth, 0)
	if p.PageRank != 0.50 {
		t.Errorf("PageRank: expected 0.50, got %f", p.PageRank)
	}
	assertEqualUint32(t, "InternalLinksOut", p.InternalLinksOut, 30)
	assertEqualUint32(t, "ExternalLinksOut", p.ExternalLinksOut, 10)
	assertEqual(t, "MetaRobots", p.MetaRobots, "index, follow")
}

func TestCsvRowToPage_MissingColumns(t *testing.T) {
	// Only Address and Status Code — everything else should be zero-valued.
	colIdx := map[string]int{"Address": 0, "Status Code": 1}
	record := []string{"https://example.com/test", "404"}

	p := csvRowToPage(CSVSourceAddressBased, colIdx, record, "sess-3", fixedTime())

	assertEqual(t, "URL", p.URL, "https://example.com/test")
	assertEqualUint16(t, "StatusCode", p.StatusCode, 404)
	assertEqual(t, "Title", p.Title, "")
	assertEqualUint32(t, "WordCount", p.WordCount, 0)
	if p.PageRank != 0 {
		t.Errorf("PageRank: expected 0, got %f", p.PageRank)
	}
}

func TestCsvRowToPage_ResponseTimeConversion(t *testing.T) {
	colIdx := map[string]int{"Address": 0, "Response Time": 1}
	record := []string{"https://example.com", "1.234"}

	p := csvRowToPage(CSVSourceAddressBased, colIdx, record, "sess-4", fixedTime())
	if p.FetchDurationMs != 1234 {
		t.Errorf("FetchDurationMs: expected 1234, got %d", p.FetchDurationMs)
	}
}

func TestCsvRowToPage_LinkScoreConversion(t *testing.T) {
	colIdx := map[string]int{"Address": 0, "Link Score": 1}
	record := []string{"https://example.com", "100"}

	p := csvRowToPage(CSVSourceAddressBased, colIdx, record, "sess-5", fixedTime())
	if p.PageRank != 1.0 {
		t.Errorf("PageRank: expected 1.0, got %f", p.PageRank)
	}
}

func TestCsvRowToPage_CanonicalIsSelf(t *testing.T) {
	colIdx := map[string]int{"Address": 0, "Canonical Link Element 1": 1}

	// Canonical == URL → true
	p := csvRowToPage(CSVSourceAddressBased, colIdx, []string{"https://a.com", "https://a.com"}, "s", fixedTime())
	if !p.CanonicalIsSelf {
		t.Error("expected CanonicalIsSelf=true when canonical == URL")
	}

	// Canonical != URL → false
	p = csvRowToPage(CSVSourceAddressBased, colIdx, []string{"https://a.com", "https://b.com"}, "s", fixedTime())
	if p.CanonicalIsSelf {
		t.Error("expected CanonicalIsSelf=false when canonical != URL")
	}

	// No canonical → false
	p = csvRowToPage(CSVSourceAddressBased, colIdx, []string{"https://a.com", ""}, "s", fixedTime())
	if p.CanonicalIsSelf {
		t.Error("expected CanonicalIsSelf=false when canonical is empty")
	}
}

func TestCsvRowToPage_EmptyURL(t *testing.T) {
	colIdx := map[string]int{"Address": 0, "Status Code": 1}
	record := []string{"", "200"}

	p := csvRowToPage(CSVSourceAddressBased, colIdx, record, "sess-6", fixedTime())
	if p.URL != "" {
		t.Errorf("expected empty URL, got %s", p.URL)
	}
}

func TestStripBOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	input := append(bom, []byte("hello")...)
	result := stripBOM(input)
	if string(result) != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}

	// No BOM — unchanged.
	noBom := []byte("hello")
	result = stripBOM(noBom)
	if string(result) != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestIdentifySource(t *testing.T) {
	tests := []struct {
		headers []string
		want    CSVSource
		ok      bool
	}{
		{[]string{"Address", "Status Code"}, CSVSourceAddressBased, true},
		{[]string{"URL", "Page Title"}, CSVSourceURLBased, true},
		{[]string{"URL", "HTTP Status Code"}, CSVSourceURLBased, true},
		{[]string{"url", "page_title"}, "", false}, // case-sensitive
		{[]string{"foo", "bar"}, "", false},
	}

	for _, tt := range tests {
		got, ok := identifySource(tt.headers)
		if ok != tt.ok || got != tt.want {
			t.Errorf("identifySource(%v) = (%s, %v), want (%s, %v)", tt.headers, got, ok, tt.want, tt.ok)
		}
	}
}

// --- helpers ---

func fixedTime() time.Time {
	return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
}

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: expected %q, got %q", field, want, got)
	}
}

func assertEqualUint16(t *testing.T, field string, got, want uint16) {
	t.Helper()
	if got != want {
		t.Errorf("%s: expected %d, got %d", field, want, got)
	}
}

func assertEqualUint32(t *testing.T, field string, got, want uint32) {
	t.Helper()
	if got != want {
		t.Errorf("%s: expected %d, got %d", field, want, got)
	}
}
