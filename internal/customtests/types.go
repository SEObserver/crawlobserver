package customtests

import "time"

// RuleType defines the kind of test rule.
type RuleType string

const (
	StringContains    RuleType = "string_contains"
	StringNotContains RuleType = "string_not_contains"
	RegexMatch        RuleType = "regex_match"
	RegexNotMatch     RuleType = "regex_not_match"
	HeaderExists      RuleType = "header_exists"
	HeaderNotExists   RuleType = "header_not_exists"
	HeaderContains    RuleType = "header_contains"
	HeaderRegex       RuleType = "header_regex"
	CSSExists         RuleType = "css_exists"
	CSSNotExists      RuleType = "css_not_exists"
	CSSExtractText    RuleType = "css_extract_text"
	CSSExtractAttr    RuleType = "css_extract_attr"
)

// IsClickHouseNative returns true if the rule can run as a ClickHouse SQL expression.
func (r RuleType) IsClickHouseNative() bool {
	switch r {
	case StringContains, StringNotContains, RegexMatch, RegexNotMatch,
		HeaderExists, HeaderNotExists, HeaderContains, HeaderRegex:
		return true
	}
	return false
}

// TestRule is a single test rule within a profile.
type TestRule struct {
	ID        string   `json:"id"`
	ProfileID string   `json:"profile_id"`
	Type      RuleType `json:"type"`
	Name      string   `json:"name"`
	Value     string   `json:"value"`
	Extra     string   `json:"extra"`
	SortOrder int      `json:"sort_order"`
}

// TestProfile groups test rules under a named profile.
type TestProfile struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Rules     []TestRule `json:"rules"`
}

// PageTestResult holds the test results for a single page.
type PageTestResult struct {
	URL     string            `json:"url"`
	Results map[string]string `json:"results"` // rule_id → "pass"/"fail"/extracted_value
}

// TestRunResult is the full output of running a test profile against a session.
type TestRunResult struct {
	ProfileID   string           `json:"profile_id"`
	ProfileName string           `json:"profile_name"`
	SessionID   string           `json:"session_id"`
	TotalPages  int              `json:"total_pages"`
	Rules       []TestRule       `json:"rules"`
	Pages       []PageTestResult `json:"pages"`
	Summary     map[string]int   `json:"summary"` // rule_id → count of passes
}
