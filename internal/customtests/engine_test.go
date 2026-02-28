package customtests

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type mockStorage struct {
	sqlResults map[string]map[string]string
	htmlPages  []PageHTMLRow
}

func (m *mockStorage) RunCustomTestsSQL(_ context.Context, _ string, rules []TestRule) (map[string]map[string]string, error) {
	if m.sqlResults != nil {
		return m.sqlResults, nil
	}
	result := make(map[string]map[string]string)
	for _, r := range rules {
		if result["https://example.com/"] == nil {
			result["https://example.com/"] = make(map[string]string)
		}
		result["https://example.com/"][r.ID] = "pass"
	}
	return result, nil
}

func (m *mockStorage) StreamPagesHTML(_ context.Context, _ string) (<-chan PageHTMLRow, error) {
	ch := make(chan PageHTMLRow, len(m.htmlPages))
	go func() {
		defer close(ch)
		for _, p := range m.htmlPages {
			ch <- p
		}
	}()
	return ch, nil
}

func TestRunTests_CHNativeOnly(t *testing.T) {
	store := &mockStorage{}
	profile := &Ruleset{
		ID:   "prof-1",
		Name: "Test",
		Rules: []TestRule{
			{ID: "r1", Type: StringContains, Name: "Has GTM", Value: "GTM-XXXX"},
			{ID: "r2", Type: HeaderExists, Name: "Has XFO", Value: "X-Frame-Options"},
		},
	}

	result, err := RunTests(context.Background(), store, "sess-1", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalPages != 1 {
		t.Errorf("expected 1 page, got %d", result.TotalPages)
	}
	if result.Summary["r1"] != 1 {
		t.Errorf("expected r1 summary=1, got %d", result.Summary["r1"])
	}
}

func TestRunTests_CSSOnly(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><head><title>Hello</title></head><body><h1>World</h1><a href="/test" class="nav">Link</a></body></html>`},
			{URL: "https://example.com/about", HTML: `<html><head><title>About</title></head><body><p>No heading</p></body></html>`},
		},
	}
	profile := &Ruleset{
		ID:   "prof-2",
		Name: "CSS Test",
		Rules: []TestRule{
			{ID: "r1", Type: CSSExists, Name: "Has h1", Value: "h1"},
			{ID: "r2", Type: CSSNotExists, Name: "No h2", Value: "h2"},
			{ID: "r3", Type: CSSExtractText, Name: "Title", Value: "title"},
			{ID: "r4", Type: CSSExtractAttr, Name: "Nav href", Value: "a.nav", Extra: "href"},
		},
	}

	result, err := RunTests(context.Background(), store, "sess-1", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalPages != 2 {
		t.Errorf("expected 2 pages, got %d", result.TotalPages)
	}

	// Find result for example.com/
	var mainPage *PageTestResult
	for i, p := range result.Pages {
		if p.URL == "https://example.com/" {
			mainPage = &result.Pages[i]
			break
		}
	}
	if mainPage == nil {
		t.Fatal("expected result for https://example.com/")
	}
	if mainPage.Results["r1"] != "pass" {
		t.Errorf("r1 (css_exists h1): expected pass, got %q", mainPage.Results["r1"])
	}
	if mainPage.Results["r2"] != "pass" {
		t.Errorf("r2 (css_not_exists h2): expected pass, got %q", mainPage.Results["r2"])
	}
	if mainPage.Results["r3"] != "Hello" {
		t.Errorf("r3 (css_extract_text title): expected 'Hello', got %q", mainPage.Results["r3"])
	}
	if mainPage.Results["r4"] != "/test" {
		t.Errorf("r4 (css_extract_attr href): expected '/test', got %q", mainPage.Results["r4"])
	}
}

func TestRunTests_MixedRules(t *testing.T) {
	store := &mockStorage{
		sqlResults: map[string]map[string]string{
			"https://example.com/": {"r1": "pass"},
		},
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body><div class="test">found</div></body></html>`},
		},
	}
	profile := &Ruleset{
		ID:   "prof-3",
		Name: "Mixed",
		Rules: []TestRule{
			{ID: "r1", Type: StringContains, Name: "Has GTM", Value: "GTM"},
			{ID: "r2", Type: CSSExists, Name: "Has .test", Value: ".test"},
		},
	}

	result, err := RunTests(context.Background(), store, "sess-1", profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var page *PageTestResult
	for i, p := range result.Pages {
		if p.URL == "https://example.com/" {
			page = &result.Pages[i]
			break
		}
	}
	if page == nil {
		t.Fatal("expected page result")
	}
	if page.Results["r1"] != "pass" {
		t.Errorf("r1: expected pass, got %q", page.Results["r1"])
	}
	if page.Results["r2"] != "pass" {
		t.Errorf("r2: expected pass, got %q", page.Results["r2"])
	}
}

func TestCSSExtractAll(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body><ul><li class="item">A</li><li class="item">B</li><li class="item">C</li></ul><a href="/1" class="link">L1</a><a href="/2" class="link">L2</a></body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-all",
		Name: "Extract All",
		Rules: []TestRule{
			{ID: "r1", Type: CSSExtractAllText, Name: "All items", Value: "li.item"},
			{ID: "r2", Type: CSSExtractAllAttr, Name: "All hrefs", Value: "a.link", Extra: "href"},
		},
	}

	result, err := RunTests(context.Background(), store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	page := result.Pages[0]
	if page.Results["r1"] != "A | B | C" {
		t.Errorf("r1: expected 'A | B | C', got %q", page.Results["r1"])
	}
	if page.Results["r2"] != "/1 | /2" {
		t.Errorf("r2: expected '/1 | /2', got %q", page.Results["r2"])
	}
}

func TestRegexExtract(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body>GTM-ABC123 and GTM-DEF456 and GTM-GHI789</body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-regex",
		Name: "Regex",
		Rules: []TestRule{
			{ID: "r1", Type: RegexExtract, Name: "First GTM", Value: `GTM-([A-Z0-9]+)`},
			{ID: "r2", Type: RegexExtractAll, Name: "All GTMs", Value: `GTM-([A-Z0-9]+)`},
		},
	}

	result, err := RunTests(context.Background(), store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	page := result.Pages[0]
	if page.Results["r1"] != "ABC123" {
		t.Errorf("r1: expected 'ABC123', got %q", page.Results["r1"])
	}
	if page.Results["r2"] != "ABC123 | DEF456 | GHI789" {
		t.Errorf("r2: expected 'ABC123 | DEF456 | GHI789', got %q", page.Results["r2"])
	}
}

func TestXPathExtract(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><head><title>XPath Test</title></head><body><div class="content"><p>First</p><p>Second</p></div></body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-xpath",
		Name: "XPath",
		Rules: []TestRule{
			{ID: "r1", Type: XPathExtract, Name: "Title", Value: `//title`},
			{ID: "r2", Type: XPathExtractAll, Name: "All p", Value: `//div[@class="content"]/p`},
		},
	}

	result, err := RunTests(context.Background(), store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	page := result.Pages[0]
	if page.Results["r1"] != "XPath Test" {
		t.Errorf("r1: expected 'XPath Test', got %q", page.Results["r1"])
	}
	if page.Results["r2"] != "First | Second" {
		t.Errorf("r2: expected 'First | Second', got %q", page.Results["r2"])
	}
}

func TestExtractTruncation(t *testing.T) {
	longText := strings.Repeat("x", 600)
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body><p>` + longText + `</p></body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-trunc",
		Name: "Truncation",
		Rules: []TestRule{
			{ID: "r1", Type: CSSExtractText, Name: "Long text", Value: "p"},
		},
	}

	result, err := RunTests(context.Background(), store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := result.Pages[0].Results["r1"]
	if len(val) > 510 { // 500 + "…" (multi-byte)
		t.Errorf("expected truncated value, got len=%d", len(val))
	}
	if !strings.HasSuffix(val, "…") {
		t.Errorf("expected truncated suffix '…', got %q", val[len(val)-5:])
	}
}

func TestExtractAllLimit(t *testing.T) {
	var items string
	for i := 0; i < 25; i++ {
		items += fmt.Sprintf(`<li class="x">item%d</li>`, i)
	}
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body><ul>` + items + `</ul></body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-limit",
		Name: "Limit",
		Rules: []TestRule{
			{ID: "r1", Type: CSSExtractAllText, Name: "Items", Value: "li.x"},
		},
	}

	result, err := RunTests(context.Background(), store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := result.Pages[0].Results["r1"]
	if !strings.Contains(val, "(+5 more)") {
		t.Errorf("expected '+5 more' suffix, got %q", val)
	}
	// Should contain first 20 items
	if !strings.Contains(val, "item0") || !strings.Contains(val, "item19") {
		t.Errorf("expected items 0-19 in result")
	}
	// Should NOT contain item20
	parts := strings.Split(val, " | ")
	for _, p := range parts[:20] {
		if p == "item20" {
			t.Errorf("item20 should not be in first 20 items")
		}
	}
}

func TestIsClickHouseNative(t *testing.T) {
	nativeTypes := []RuleType{StringContains, StringNotContains, RegexMatch, RegexNotMatch, HeaderExists, HeaderNotExists, HeaderContains, HeaderRegex}
	for _, rt := range nativeTypes {
		if !rt.IsClickHouseNative() {
			t.Errorf("%s should be ClickHouse-native", rt)
		}
	}
	goTypes := []RuleType{CSSExists, CSSNotExists, CSSExtractText, CSSExtractAttr, CSSExtractAllText, CSSExtractAllAttr, RegexExtract, RegexExtractAll, XPathExtract, XPathExtractAll}
	for _, rt := range goTypes {
		if rt.IsClickHouseNative() {
			t.Errorf("%s should NOT be ClickHouse-native", rt)
		}
	}
}
