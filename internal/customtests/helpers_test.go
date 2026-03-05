package customtests

import (
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 5, "hello…"},
		{"empty string", "", 10, ""},
		{"single char truncation", "ab", 1, "a…"},
		{"zero max", "hello", 0, "…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestTruncate_MaxExtractLen(t *testing.T) {
	// Test with the actual constant used in the package
	short := strings.Repeat("x", maxExtractLen)
	got := truncate(short, maxExtractLen)
	if got != short {
		t.Errorf("string of exactly maxExtractLen should not be truncated")
	}

	long := strings.Repeat("x", maxExtractLen+1)
	got = truncate(long, maxExtractLen)
	if len(got) > maxExtractLen+len("…") {
		t.Errorf("truncated length = %d, want at most %d", len(got), maxExtractLen+len("…"))
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("truncated string should end with ellipsis")
	}
}

// ---------------------------------------------------------------------------
// joinExtracted
// ---------------------------------------------------------------------------

func TestJoinExtracted(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  string
	}{
		{"nil", nil, ""},
		{"empty slice", []string{}, ""},
		{"single item", []string{"a"}, "a"},
		{"two items", []string{"a", "b"}, "a | b"},
		{"three items", []string{"a", "b", "c"}, "a | b | c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinExtracted(tt.items)
			if got != tt.want {
				t.Errorf("joinExtracted(%v) = %q, want %q", tt.items, got, tt.want)
			}
		})
	}
}

func TestJoinExtracted_ExceedsLimit(t *testing.T) {
	// Create exactly maxExtractAll items - should NOT have truncation indicator
	items := make([]string, maxExtractAll)
	for i := range items {
		items[i] = fmt.Sprintf("item%d", i)
	}
	got := joinExtracted(items)
	if strings.Contains(got, "more)") {
		t.Errorf("expected no truncation for %d items, got %q", maxExtractAll, got)
	}

	// Create maxExtractAll+5 items - SHOULD have truncation indicator
	extra := make([]string, maxExtractAll+5)
	for i := range extra {
		extra[i] = fmt.Sprintf("item%d", i)
	}
	got = joinExtracted(extra)
	if !strings.Contains(got, "(+5 more)") {
		t.Errorf("expected '+5 more' indicator, got %q", got)
	}
	// Should contain items 0 through maxExtractAll-1
	if !strings.Contains(got, "item0") {
		t.Error("expected item0 in output")
	}
	if !strings.Contains(got, fmt.Sprintf("item%d", maxExtractAll-1)) {
		t.Errorf("expected item%d in output", maxExtractAll-1)
	}
}

// ---------------------------------------------------------------------------
// IsClickHouseNative (additional edge cases)
// ---------------------------------------------------------------------------

func TestIsClickHouseNative_UnknownType(t *testing.T) {
	unknown := RuleType("unknown_rule_type")
	if unknown.IsClickHouseNative() {
		t.Error("unknown rule type should not be ClickHouse-native")
	}
}

func TestIsClickHouseNative_EmptyType(t *testing.T) {
	empty := RuleType("")
	if empty.IsClickHouseNative() {
		t.Error("empty rule type should not be ClickHouse-native")
	}
}

// ---------------------------------------------------------------------------
// evalGoRule edge cases
// ---------------------------------------------------------------------------

func TestEvalGoRule_CSSNotExists_Match(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body><div>no heading</div></body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-edge",
		Name: "Edge",
		Rules: []TestRule{
			{ID: "r1", Type: CSSNotExists, Name: "No h1", Value: "h1"},
		},
	}

	result, err := RunTests(nil, store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pages[0].Results["r1"] != "pass" {
		t.Errorf("expected pass for missing h1, got %q", result.Pages[0].Results["r1"])
	}
}

func TestEvalGoRule_CSSNotExists_Fail(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body><h1>present</h1></body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-edge",
		Name: "Edge",
		Rules: []TestRule{
			{ID: "r1", Type: CSSNotExists, Name: "No h1", Value: "h1"},
		},
	}

	result, err := RunTests(nil, store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pages[0].Results["r1"] != "fail" {
		t.Errorf("expected fail for present h1, got %q", result.Pages[0].Results["r1"])
	}
}

func TestEvalGoRule_RegexExtract_NoMatch(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body>no match here</body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-nomatch",
		Name: "NoMatch",
		Rules: []TestRule{
			{ID: "r1", Type: RegexExtract, Name: "GTM", Value: `GTM-([A-Z0-9]+)`},
		},
	}

	result, err := RunTests(nil, store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pages[0].Results["r1"] != "" {
		t.Errorf("expected empty for no regex match, got %q", result.Pages[0].Results["r1"])
	}
}

func TestEvalGoRule_RegexExtract_InvalidRegex(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body>content</body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-badregex",
		Name: "BadRegex",
		Rules: []TestRule{
			{ID: "r1", Type: RegexExtract, Name: "Bad", Value: `[invalid`},
			{ID: "r2", Type: RegexExtractAll, Name: "Bad all", Value: `[invalid`},
		},
	}

	result, err := RunTests(nil, store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pages[0].Results["r1"] != "" {
		t.Errorf("expected empty for invalid regex, got %q", result.Pages[0].Results["r1"])
	}
	if result.Pages[0].Results["r2"] != "" {
		t.Errorf("expected empty for invalid regex all, got %q", result.Pages[0].Results["r2"])
	}
}

func TestEvalGoRule_RegexExtract_NoCapture(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body>GTM-ABC123</body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-nocap",
		Name: "NoCapture",
		Rules: []TestRule{
			// Regex without capture group - should return full match
			{ID: "r1", Type: RegexExtract, Name: "GTM", Value: `GTM-[A-Z0-9]+`},
			{ID: "r2", Type: RegexExtractAll, Name: "All GTM", Value: `GTM-[A-Z0-9]+`},
		},
	}

	result, err := RunTests(nil, store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pages[0].Results["r1"] != "GTM-ABC123" {
		t.Errorf("expected full match 'GTM-ABC123', got %q", result.Pages[0].Results["r1"])
	}
	if result.Pages[0].Results["r2"] != "GTM-ABC123" {
		t.Errorf("expected full match 'GTM-ABC123', got %q", result.Pages[0].Results["r2"])
	}
}

func TestEvalGoRule_XPathExtract_NilHTMLNode(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			// completely empty HTML may produce no nodes
			{URL: "https://example.com/", HTML: ""},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-nil",
		Name: "NilNode",
		Rules: []TestRule{
			{ID: "r1", Type: XPathExtract, Name: "Title", Value: "//title"},
			{ID: "r2", Type: XPathExtractAll, Name: "All p", Value: "//p"},
		},
	}

	result, err := RunTests(nil, store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With empty HTML, these should return empty strings
	if result.Pages[0].Results["r1"] != "" {
		t.Errorf("expected empty for xpath on empty html, got %q", result.Pages[0].Results["r1"])
	}
}

func TestEvalGoRule_CSSExtractAttr_MissingAttr(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://example.com/", HTML: `<html><body><a class="link">text</a></body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-noattr",
		Name: "NoAttr",
		Rules: []TestRule{
			// href attribute does not exist on this <a>
			{ID: "r1", Type: CSSExtractAttr, Name: "href", Value: "a.link", Extra: "href"},
		},
	}

	result, err := RunTests(nil, store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Pages[0].Results["r1"] != "" {
		t.Errorf("expected empty for missing attr, got %q", result.Pages[0].Results["r1"])
	}
}

// ---------------------------------------------------------------------------
// RunTests summary computation
// ---------------------------------------------------------------------------

func TestRunTests_SummaryComputation(t *testing.T) {
	store := &mockStorage{
		htmlPages: []PageHTMLRow{
			{URL: "https://a.com/", HTML: `<html><body><h1>A</h1></body></html>`},
			{URL: "https://b.com/", HTML: `<html><body><p>B no heading</p></body></html>`},
			{URL: "https://c.com/", HTML: `<html><body><h1>C</h1></body></html>`},
		},
	}
	ruleset := &Ruleset{
		ID:   "p-summary",
		Name: "Summary",
		Rules: []TestRule{
			{ID: "r1", Type: CSSExists, Name: "Has h1", Value: "h1"},
			{ID: "r2", Type: CSSExtractText, Name: "Title text", Value: "h1"},
		},
	}

	result, err := RunTests(nil, store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", result.TotalPages)
	}

	// r1: CSSExists h1 - 2 pages have h1 (a.com and c.com), 1 does not (b.com)
	if result.Summary["r1"] != 2 {
		t.Errorf("Summary[r1] = %d, want 2", result.Summary["r1"])
	}

	// r2: CSSExtractText h1 - extracted values "A" and "C" count as passes,
	// empty string for b.com does NOT count (v != "fail" && v != "" -> false for empty)
	if result.Summary["r2"] != 2 {
		t.Errorf("Summary[r2] = %d, want 2", result.Summary["r2"])
	}

	// Verify RulesetID and SessionID propagation
	if result.RulesetID != "p-summary" {
		t.Errorf("RulesetID = %q, want %q", result.RulesetID, "p-summary")
	}
	if result.SessionID != "s1" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "s1")
	}
	if result.RulesetName != "Summary" {
		t.Errorf("RulesetName = %q, want %q", result.RulesetName, "Summary")
	}
}

func TestRunTests_EmptyRuleset(t *testing.T) {
	store := &mockStorage{}
	ruleset := &Ruleset{
		ID:    "p-empty",
		Name:  "Empty",
		Rules: []TestRule{},
	}

	result, err := RunTests(nil, store, "s1", ruleset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalPages != 0 {
		t.Errorf("TotalPages = %d, want 0", result.TotalPages)
	}
	if len(result.Pages) != 0 {
		t.Errorf("len(Pages) = %d, want 0", len(result.Pages))
	}
}
