package customtests

import (
	"context"
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
	profile := &TestProfile{
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
	profile := &TestProfile{
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
	profile := &TestProfile{
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

func TestIsClickHouseNative(t *testing.T) {
	nativeTypes := []RuleType{StringContains, StringNotContains, RegexMatch, RegexNotMatch, HeaderExists, HeaderNotExists, HeaderContains, HeaderRegex}
	for _, rt := range nativeTypes {
		if !rt.IsClickHouseNative() {
			t.Errorf("%s should be ClickHouse-native", rt)
		}
	}
	cssTypes := []RuleType{CSSExists, CSSNotExists, CSSExtractText, CSSExtractAttr}
	for _, rt := range cssTypes {
		if rt.IsClickHouseNative() {
			t.Errorf("%s should NOT be ClickHouse-native", rt)
		}
	}
}
