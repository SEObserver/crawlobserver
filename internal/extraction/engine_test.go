package extraction

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate_ShortString(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Errorf("truncate(short) = %q, want %q", got, "hello")
	}
}

func TestTruncate_ExactLength(t *testing.T) {
	s := strings.Repeat("a", 500)
	got := truncate(s, 500)
	if got != s {
		t.Errorf("truncate(exact) length = %d, want 500", len(got))
	}
}

func TestTruncate_LongString(t *testing.T) {
	s := strings.Repeat("x", 600)
	got := truncate(s, 500)
	want := strings.Repeat("x", 500) + "\xe2\x80\xa6" // UTF-8 for "…"
	if got != want {
		t.Errorf("truncate(long): got length %d, want %d", len(got), len(want))
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("truncate(long) should end with ellipsis")
	}
}

func TestTruncate_EmptyString(t *testing.T) {
	got := truncate("", 10)
	if got != "" {
		t.Errorf("truncate(empty) = %q, want %q", got, "")
	}
}

// ---------------------------------------------------------------------------
// joinExtracted
// ---------------------------------------------------------------------------

func TestJoinExtracted_Empty(t *testing.T) {
	got := joinExtracted(nil)
	if got != "" {
		t.Errorf("joinExtracted(nil) = %q, want %q", got, "")
	}
	got = joinExtracted([]string{})
	if got != "" {
		t.Errorf("joinExtracted(empty) = %q, want %q", got, "")
	}
}

func TestJoinExtracted_FewItems(t *testing.T) {
	items := []string{"a", "b", "c"}
	got := joinExtracted(items)
	want := "a | b | c"
	if got != want {
		t.Errorf("joinExtracted(few) = %q, want %q", got, want)
	}
}

func TestJoinExtracted_SingleItem(t *testing.T) {
	got := joinExtracted([]string{"only"})
	if got != "only" {
		t.Errorf("joinExtracted(single) = %q, want %q", got, "only")
	}
}

func TestJoinExtracted_ExactlyMaxItems(t *testing.T) {
	items := make([]string, maxExtractAll)
	for i := range items {
		items[i] = fmt.Sprintf("item%d", i)
	}
	got := joinExtracted(items)
	// Should NOT contain "more"
	if strings.Contains(got, "more") {
		t.Errorf("joinExtracted(20 items) should not contain overflow marker, got %q", got)
	}
	want := strings.Join(items, " | ")
	if got != want {
		t.Errorf("joinExtracted(20 items) = %q, want %q", got, want)
	}
}

func TestJoinExtracted_OverflowItems(t *testing.T) {
	items := make([]string, 25)
	for i := range items {
		items[i] = fmt.Sprintf("v%d", i)
	}
	got := joinExtracted(items)
	wantSuffix := fmt.Sprintf(" … (+%d more)", 25-maxExtractAll)
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("joinExtracted(25 items) should end with %q, got %q", wantSuffix, got)
	}
	// First 20 items should be present
	for i := 0; i < maxExtractAll; i++ {
		if !strings.Contains(got, fmt.Sprintf("v%d", i)) {
			t.Errorf("joinExtracted(25 items) missing item v%d", i)
		}
	}
}

func TestJoinExtracted_21Items(t *testing.T) {
	items := make([]string, 21)
	for i := range items {
		items[i] = "x"
	}
	got := joinExtracted(items)
	if !strings.Contains(got, "(+1 more)") {
		t.Errorf("joinExtracted(21 items) should contain (+1 more), got %q", got)
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — edge cases
// ---------------------------------------------------------------------------

func TestRunExtractors_NilBody(t *testing.T) {
	ext := []Extractor{{Type: CSSExtractText, Name: "title", Selector: "h1"}}
	got := RunExtractors(nil, "http://example.com", "sess1", ext, testTime)
	if got != nil {
		t.Errorf("RunExtractors(nil body) = %v, want nil", got)
	}
}

func TestRunExtractors_EmptyBody(t *testing.T) {
	ext := []Extractor{{Type: CSSExtractText, Name: "title", Selector: "h1"}}
	got := RunExtractors([]byte{}, "http://example.com", "sess1", ext, testTime)
	if got != nil {
		t.Errorf("RunExtractors(empty body) = %v, want nil", got)
	}
}

func TestRunExtractors_EmptyExtractors(t *testing.T) {
	body := []byte("<h1>Hello</h1>")
	got := RunExtractors(body, "http://example.com", "sess1", nil, testTime)
	if got != nil {
		t.Errorf("RunExtractors(nil extractors) = %v, want nil", got)
	}
	got = RunExtractors(body, "http://example.com", "sess1", []Extractor{}, testTime)
	if got != nil {
		t.Errorf("RunExtractors(empty extractors) = %v, want nil", got)
	}
}

func TestRunExtractors_NoMatch(t *testing.T) {
	body := []byte("<p>no heading here</p>")
	ext := []Extractor{{Type: CSSExtractText, Name: "title", Selector: "h1"}}
	got := RunExtractors(body, "http://example.com", "sess1", ext, testTime)
	// h1 doesn't exist so Text() returns "" and the row is skipped
	if len(got) != 0 {
		t.Errorf("RunExtractors(no match) returned %d rows, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — CSSExtractText
// ---------------------------------------------------------------------------

func TestRunExtractors_CSSExtractText(t *testing.T) {
	body := []byte("<html><body><h1>Hello World</h1><h1>Second</h1></body></html>")
	ext := []Extractor{
		{Type: CSSExtractText, Name: "heading", Selector: "h1"},
	}
	rows := RunExtractors(body, "http://example.com/page", "sess1", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("CSSExtractText: got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.Value != "Hello World" {
		t.Errorf("CSSExtractText: value = %q, want %q", r.Value, "Hello World")
	}
	if r.CrawlSessionID != "sess1" {
		t.Errorf("CSSExtractText: session = %q, want %q", r.CrawlSessionID, "sess1")
	}
	if r.URL != "http://example.com/page" {
		t.Errorf("CSSExtractText: url = %q", r.URL)
	}
	if r.ExtractorName != "heading" {
		t.Errorf("CSSExtractText: name = %q, want %q", r.ExtractorName, "heading")
	}
	if !r.CrawledAt.Equal(testTime) {
		t.Errorf("CSSExtractText: crawledAt = %v, want %v", r.CrawledAt, testTime)
	}
}

func TestRunExtractors_CSSExtractText_Truncation(t *testing.T) {
	longText := strings.Repeat("A", 600)
	body := []byte(fmt.Sprintf("<h1>%s</h1>", longText))
	ext := []Extractor{
		{Type: CSSExtractText, Name: "long", Selector: "h1"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("CSSExtractText truncation: got %d rows, want 1", len(rows))
	}
	if !strings.HasSuffix(rows[0].Value, "…") {
		t.Error("CSSExtractText truncation: expected ellipsis suffix")
	}
	// The prefix should be maxExtractLen bytes of "A"
	if !strings.HasPrefix(rows[0].Value, strings.Repeat("A", maxExtractLen)) {
		t.Error("CSSExtractText truncation: unexpected prefix")
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — CSSExtractAttr
// ---------------------------------------------------------------------------

func TestRunExtractors_CSSExtractAttr(t *testing.T) {
	body := []byte(`<html><body><a href="/link1">First</a><a href="/link2">Second</a></body></html>`)
	ext := []Extractor{
		{Type: CSSExtractAttr, Name: "first_link", Selector: "a", Attribute: "href"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("CSSExtractAttr: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "/link1" {
		t.Errorf("CSSExtractAttr: value = %q, want %q", rows[0].Value, "/link1")
	}
}

func TestRunExtractors_CSSExtractAttr_MissingAttribute(t *testing.T) {
	body := []byte(`<a class="btn">click</a>`)
	ext := []Extractor{
		{Type: CSSExtractAttr, Name: "link", Selector: "a", Attribute: "href"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	// AttrOr returns "" when attribute missing; empty values are skipped
	if len(rows) != 0 {
		t.Errorf("CSSExtractAttr missing attr: got %d rows, want 0", len(rows))
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — CSSExtractAllText
// ---------------------------------------------------------------------------

func TestRunExtractors_CSSExtractAllText(t *testing.T) {
	body := []byte(`<ul><li>Alpha</li><li>Beta</li><li>Gamma</li></ul>`)
	ext := []Extractor{
		{Type: CSSExtractAllText, Name: "items", Selector: "li"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("CSSExtractAllText: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "Alpha | Beta | Gamma" {
		t.Errorf("CSSExtractAllText: value = %q, want %q", rows[0].Value, "Alpha | Beta | Gamma")
	}
}

func TestRunExtractors_CSSExtractAllText_NoMatches(t *testing.T) {
	body := []byte(`<p>nothing</p>`)
	ext := []Extractor{
		{Type: CSSExtractAllText, Name: "items", Selector: "li"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 0 {
		t.Errorf("CSSExtractAllText no matches: got %d rows, want 0", len(rows))
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — CSSExtractAllAttr
// ---------------------------------------------------------------------------

func TestRunExtractors_CSSExtractAllAttr(t *testing.T) {
	body := []byte(`<a href="/a">A</a><a href="/b">B</a><a href="/c">C</a>`)
	ext := []Extractor{
		{Type: CSSExtractAllAttr, Name: "links", Selector: "a", Attribute: "href"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("CSSExtractAllAttr: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "/a | /b | /c" {
		t.Errorf("CSSExtractAllAttr: value = %q, want %q", rows[0].Value, "/a | /b | /c")
	}
}

func TestRunExtractors_CSSExtractAllAttr_SomeMissing(t *testing.T) {
	body := []byte(`<a href="/a">A</a><a>B</a><a href="/c">C</a>`)
	ext := []Extractor{
		{Type: CSSExtractAllAttr, Name: "links", Selector: "a", Attribute: "href"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("CSSExtractAllAttr some missing: got %d rows, want 1", len(rows))
	}
	// The middle <a> has no href so it's skipped
	if rows[0].Value != "/a | /c" {
		t.Errorf("CSSExtractAllAttr some missing: value = %q, want %q", rows[0].Value, "/a | /c")
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — RegexExtract
// ---------------------------------------------------------------------------

func TestRunExtractors_RegexExtract_WithGroup(t *testing.T) {
	body := []byte(`<meta name="price" content="$42.99">`)
	ext := []Extractor{
		{Type: RegexExtract, Name: "price", Selector: `content="\$([0-9.]+)"`},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("RegexExtract with group: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "42.99" {
		t.Errorf("RegexExtract with group: value = %q, want %q", rows[0].Value, "42.99")
	}
}

func TestRunExtractors_RegexExtract_WithoutGroup(t *testing.T) {
	body := []byte(`Status: 200 OK`)
	ext := []Extractor{
		{Type: RegexExtract, Name: "status", Selector: `\d+ OK`},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("RegexExtract without group: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "200 OK" {
		t.Errorf("RegexExtract without group: value = %q, want %q", rows[0].Value, "200 OK")
	}
}

func TestRunExtractors_RegexExtract_NoMatch(t *testing.T) {
	body := []byte(`nothing useful here`)
	ext := []Extractor{
		{Type: RegexExtract, Name: "missing", Selector: `\d+`},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 0 {
		t.Errorf("RegexExtract no match: got %d rows, want 0", len(rows))
	}
}

func TestRunExtractors_RegexExtract_InvalidRegex(t *testing.T) {
	body := []byte(`some text`)
	ext := []Extractor{
		{Type: RegexExtract, Name: "bad", Selector: `[invalid`},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 0 {
		t.Errorf("RegexExtract invalid regex: got %d rows, want 0", len(rows))
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — RegexExtractAll
// ---------------------------------------------------------------------------

func TestRunExtractors_RegexExtractAll(t *testing.T) {
	body := []byte(`price: $10, price: $20, price: $30`)
	ext := []Extractor{
		{Type: RegexExtractAll, Name: "prices", Selector: `\$(\d+)`},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("RegexExtractAll: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "10 | 20 | 30" {
		t.Errorf("RegexExtractAll: value = %q, want %q", rows[0].Value, "10 | 20 | 30")
	}
}

func TestRunExtractors_RegexExtractAll_WithoutGroup(t *testing.T) {
	body := []byte(`codes: ABC, DEF, GHI`)
	ext := []Extractor{
		{Type: RegexExtractAll, Name: "codes", Selector: `[A-Z]{3}`},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("RegexExtractAll without group: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "ABC | DEF | GHI" {
		t.Errorf("RegexExtractAll without group: value = %q, want %q", rows[0].Value, "ABC | DEF | GHI")
	}
}

func TestRunExtractors_RegexExtractAll_Overflow(t *testing.T) {
	// Generate 25 matches to trigger overflow
	parts := make([]string, 25)
	for i := range parts {
		parts[i] = fmt.Sprintf("N%d", i)
	}
	body := []byte(strings.Join(parts, " "))
	ext := []Extractor{
		{Type: RegexExtractAll, Name: "nums", Selector: `N(\d+)`},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("RegexExtractAll overflow: got %d rows, want 1", len(rows))
	}
	if !strings.Contains(rows[0].Value, "(+5 more)") {
		t.Errorf("RegexExtractAll overflow: value = %q, expected overflow marker", rows[0].Value)
	}
}

func TestRunExtractors_RegexExtractAll_InvalidRegex(t *testing.T) {
	body := []byte(`some text`)
	ext := []Extractor{
		{Type: RegexExtractAll, Name: "bad", Selector: `[invalid`},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 0 {
		t.Errorf("RegexExtractAll invalid regex: got %d rows, want 0", len(rows))
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — XPathExtract
// ---------------------------------------------------------------------------

func TestRunExtractors_XPathExtract(t *testing.T) {
	body := []byte(`<html><body><h1>XPath Title</h1></body></html>`)
	ext := []Extractor{
		{Type: XPathExtract, Name: "title", Selector: "//h1"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("XPathExtract: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "XPath Title" {
		t.Errorf("XPathExtract: value = %q, want %q", rows[0].Value, "XPath Title")
	}
}

func TestRunExtractors_XPathExtract_NoMatch(t *testing.T) {
	body := []byte(`<html><body><p>No heading</p></body></html>`)
	ext := []Extractor{
		{Type: XPathExtract, Name: "title", Selector: "//h1"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 0 {
		t.Errorf("XPathExtract no match: got %d rows, want 0", len(rows))
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — XPathExtractAll
// ---------------------------------------------------------------------------

func TestRunExtractors_XPathExtractAll(t *testing.T) {
	body := []byte(`<html><body><ul><li>One</li><li>Two</li><li>Three</li></ul></body></html>`)
	ext := []Extractor{
		{Type: XPathExtractAll, Name: "items", Selector: "//li"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("XPathExtractAll: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "One | Two | Three" {
		t.Errorf("XPathExtractAll: value = %q, want %q", rows[0].Value, "One | Two | Three")
	}
}

func TestRunExtractors_XPathExtractAll_NoMatch(t *testing.T) {
	body := []byte(`<html><body><p>nothing</p></body></html>`)
	ext := []Extractor{
		{Type: XPathExtractAll, Name: "items", Selector: "//li"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 0 {
		t.Errorf("XPathExtractAll no match: got %d rows, want 0", len(rows))
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — URLPattern
// ---------------------------------------------------------------------------

func TestRunExtractors_URLPattern_Match(t *testing.T) {
	body := []byte(`<h1>Matched</h1>`)
	ext := []Extractor{
		{Type: CSSExtractText, Name: "title", Selector: "h1", URLPattern: "http://example.com/*"},
	}
	rows := RunExtractors(body, "http://example.com/page", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("URLPattern match: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "Matched" {
		t.Errorf("URLPattern match: value = %q, want %q", rows[0].Value, "Matched")
	}
}

func TestRunExtractors_URLPattern_Mismatch(t *testing.T) {
	body := []byte(`<h1>Skipped</h1>`)
	ext := []Extractor{
		{Type: CSSExtractText, Name: "title", Selector: "h1", URLPattern: "http://other.com/*"},
	}
	rows := RunExtractors(body, "http://example.com/page", "s", ext, testTime)
	if len(rows) != 0 {
		t.Errorf("URLPattern mismatch: got %d rows, want 0", len(rows))
	}
}

func TestRunExtractors_URLPattern_Empty(t *testing.T) {
	// Empty URLPattern means "match all"
	body := []byte(`<h1>Always</h1>`)
	ext := []Extractor{
		{Type: CSSExtractText, Name: "title", Selector: "h1", URLPattern: ""},
	}
	rows := RunExtractors(body, "http://example.com/anything", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("URLPattern empty: got %d rows, want 1", len(rows))
	}
	if rows[0].Value != "Always" {
		t.Errorf("URLPattern empty: value = %q, want %q", rows[0].Value, "Always")
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — multiple extractors
// ---------------------------------------------------------------------------

func TestRunExtractors_MultipleExtractors(t *testing.T) {
	body := []byte(`<html><body><h1>Title</h1><p class="desc">Description</p><a href="/link">Link</a></body></html>`)
	exts := []Extractor{
		{Type: CSSExtractText, Name: "heading", Selector: "h1"},
		{Type: CSSExtractText, Name: "description", Selector: "p.desc"},
		{Type: CSSExtractAttr, Name: "link", Selector: "a", Attribute: "href"},
	}
	rows := RunExtractors(body, "http://example.com", "sess42", exts, testTime)
	if len(rows) != 3 {
		t.Fatalf("Multiple extractors: got %d rows, want 3", len(rows))
	}

	expected := map[string]string{
		"heading":     "Title",
		"description": "Description",
		"link":        "/link",
	}
	for _, r := range rows {
		want, ok := expected[r.ExtractorName]
		if !ok {
			t.Errorf("Unexpected extractor name: %q", r.ExtractorName)
			continue
		}
		if r.Value != want {
			t.Errorf("Extractor %q: value = %q, want %q", r.ExtractorName, r.Value, want)
		}
		if r.CrawlSessionID != "sess42" {
			t.Errorf("Extractor %q: sessionID = %q, want %q", r.ExtractorName, r.CrawlSessionID, "sess42")
		}
	}
}

func TestRunExtractors_MixedMatchAndSkip(t *testing.T) {
	body := []byte(`<h1>Found</h1>`)
	exts := []Extractor{
		{Type: CSSExtractText, Name: "found", Selector: "h1"},
		{Type: CSSExtractText, Name: "missing", Selector: "h2"}, // no match, skipped
		{Type: CSSExtractAttr, Name: "noattr", Selector: "h1", Attribute: "data-x"}, // no attr, skipped
	}
	rows := RunExtractors(body, "http://example.com", "s", exts, testTime)
	if len(rows) != 1 {
		t.Fatalf("Mixed match/skip: got %d rows, want 1", len(rows))
	}
	if rows[0].ExtractorName != "found" {
		t.Errorf("Mixed match/skip: name = %q, want %q", rows[0].ExtractorName, "found")
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — unknown extractor type
// ---------------------------------------------------------------------------

func TestRunExtractors_UnknownType(t *testing.T) {
	body := []byte(`<h1>Test</h1>`)
	ext := []Extractor{
		{Type: ExtractorType("unknown_type"), Name: "test", Selector: "h1"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 0 {
		t.Errorf("Unknown type: got %d rows, want 0", len(rows))
	}
}

// ---------------------------------------------------------------------------
// RunExtractors — CSSExtractAllText with overflow (>20 items)
// ---------------------------------------------------------------------------

func TestRunExtractors_CSSExtractAllText_Overflow(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("<ul>")
	for i := 0; i < 25; i++ {
		sb.WriteString(fmt.Sprintf("<li>item%d</li>", i))
	}
	sb.WriteString("</ul>")
	body := []byte(sb.String())

	ext := []Extractor{
		{Type: CSSExtractAllText, Name: "items", Selector: "li"},
	}
	rows := RunExtractors(body, "http://example.com", "s", ext, testTime)
	if len(rows) != 1 {
		t.Fatalf("CSSExtractAllText overflow: got %d rows, want 1", len(rows))
	}
	if !strings.Contains(rows[0].Value, "(+5 more)") {
		t.Errorf("CSSExtractAllText overflow: value = %q, expected overflow marker", rows[0].Value)
	}
	// Verify first and last of the 20 included items
	if !strings.HasPrefix(rows[0].Value, "item0") {
		t.Errorf("CSSExtractAllText overflow: expected to start with item0, got %q", rows[0].Value)
	}
	if !strings.Contains(rows[0].Value, "item19") {
		t.Errorf("CSSExtractAllText overflow: expected item19 to be present")
	}
}
