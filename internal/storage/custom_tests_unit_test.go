package storage

import (
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/SEObserver/crawlobserver/internal/customtests"
)

// ---------------------------------------------------------------------------
// validateRegex
// ---------------------------------------------------------------------------

func TestValidateRegex(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantErr bool
		errMsg  string // substring expected in error message (empty = don't check)
	}{
		{
			name:    "valid simple pattern",
			pattern: `hello`,
			wantErr: false,
		},
		{
			name:    "valid complex pattern",
			pattern: `<h1[^>]*class="title"[^>]*>`,
			wantErr: false,
		},
		{
			name:    "valid character class",
			pattern: `[a-zA-Z0-9_-]+`,
			wantErr: false,
		},
		{
			name:    "valid alternation",
			pattern: `(foo|bar|baz)`,
			wantErr: false,
		},
		{
			name:    "valid empty pattern",
			pattern: ``,
			wantErr: false,
		},
		{
			name:    "valid dot star",
			pattern: `.*`,
			wantErr: false,
		},
		{
			name:    "invalid unclosed bracket",
			pattern: `[abc`,
			wantErr: true,
			errMsg:  "invalid regex pattern",
		},
		{
			name:    "invalid unclosed paren",
			pattern: `(abc`,
			wantErr: true,
			errMsg:  "invalid regex pattern",
		},
		{
			name:    "invalid bad escape",
			pattern: `\`,
			wantErr: true,
			errMsg:  "invalid regex pattern",
		},
		{
			name:    "invalid bad repetition",
			pattern: `*`,
			wantErr: true,
			errMsg:  "invalid regex pattern",
		},
		{
			name:    "pattern exactly at max length",
			pattern: strings.Repeat("a", maxRegexLen),
			wantErr: false,
		},
		{
			name:    "pattern one over max length",
			pattern: strings.Repeat("a", maxRegexLen+1),
			wantErr: true,
			errMsg:  "regex pattern too long",
		},
		{
			name:    "pattern well over max length",
			pattern: strings.Repeat("x", 5000),
			wantErr: true,
			errMsg:  "regex pattern too long",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRegex(tc.pattern)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errMsg != "" && !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("error = %q, want substring %q", err.Error(), tc.errMsg)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildRuleExpr
// ---------------------------------------------------------------------------

// namedVal is a helper to extract Name and Value from a clickhouse.Named result.
func namedVal(arg any) (string, any) {
	nv, ok := arg.(driver.NamedValue)
	if !ok {
		return "", nil
	}
	return nv.Name, nv.Value
}

func TestBuildRuleExpr_StringContains(t *testing.T) {
	r := customtests.TestRule{
		ID:    "rule-1",
		Type:  customtests.StringContains,
		Value: "canonical",
	}
	expr, err := buildRuleExpr(r, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := "position(body_html, {v0:String}) > 0"
	if expr.sql != wantSQL {
		t.Errorf("sql = %q, want %q", expr.sql, wantSQL)
	}
	if len(expr.args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(expr.args))
	}
	name, val := namedVal(expr.args[0])
	if name != "v0" {
		t.Errorf("arg name = %q, want %q", name, "v0")
	}
	if val != "canonical" {
		t.Errorf("arg value = %q, want %q", val, "canonical")
	}
}

func TestBuildRuleExpr_StringNotContains(t *testing.T) {
	r := customtests.TestRule{
		ID:    "rule-2",
		Type:  customtests.StringNotContains,
		Value: "noindex",
	}
	expr, err := buildRuleExpr(r, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := "position(body_html, {v3:String}) = 0"
	if expr.sql != wantSQL {
		t.Errorf("sql = %q, want %q", expr.sql, wantSQL)
	}
	if len(expr.args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(expr.args))
	}
	name, val := namedVal(expr.args[0])
	if name != "v3" {
		t.Errorf("arg name = %q, want %q", name, "v3")
	}
	if val != "noindex" {
		t.Errorf("arg value = %q, want %q", val, "noindex")
	}
}

func TestBuildRuleExpr_RegexMatch(t *testing.T) {
	r := customtests.TestRule{
		ID:    "regex-1",
		Type:  customtests.RegexMatch,
		Value: `<h1[^>]*>.*</h1>`,
	}
	expr, err := buildRuleExpr(r, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := "match(body_html, {v5:String})"
	if expr.sql != wantSQL {
		t.Errorf("sql = %q, want %q", expr.sql, wantSQL)
	}
	if len(expr.args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(expr.args))
	}
	name, val := namedVal(expr.args[0])
	if name != "v5" {
		t.Errorf("arg name = %q, want %q", name, "v5")
	}
	if val != `<h1[^>]*>.*</h1>` {
		t.Errorf("arg value = %q, want %q", val, `<h1[^>]*>.*</h1>`)
	}
}

func TestBuildRuleExpr_RegexMatch_InvalidPattern(t *testing.T) {
	r := customtests.TestRule{
		ID:    "bad-regex",
		Type:  customtests.RegexMatch,
		Value: `[unclosed`,
	}
	_, err := buildRuleExpr(r, 0)
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
	if !strings.Contains(err.Error(), "bad-regex") {
		t.Errorf("error should mention rule ID, got: %v", err)
	}
}

func TestBuildRuleExpr_RegexMatch_TooLong(t *testing.T) {
	r := customtests.TestRule{
		ID:    "long-regex",
		Type:  customtests.RegexMatch,
		Value: strings.Repeat("a", maxRegexLen+1),
	}
	_, err := buildRuleExpr(r, 0)
	if err == nil {
		t.Fatal("expected error for too-long regex, got nil")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("error should mention 'too long', got: %v", err)
	}
}

func TestBuildRuleExpr_RegexNotMatch(t *testing.T) {
	r := customtests.TestRule{
		ID:    "notmatch-1",
		Type:  customtests.RegexNotMatch,
		Value: `nofollow`,
	}
	expr, err := buildRuleExpr(r, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := "NOT match(body_html, {v2:String})"
	if expr.sql != wantSQL {
		t.Errorf("sql = %q, want %q", expr.sql, wantSQL)
	}
	if len(expr.args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(expr.args))
	}
	name, val := namedVal(expr.args[0])
	if name != "v2" {
		t.Errorf("arg name = %q, want %q", name, "v2")
	}
	if val != "nofollow" {
		t.Errorf("arg value = %q, want %q", val, "nofollow")
	}
}

func TestBuildRuleExpr_RegexNotMatch_InvalidPattern(t *testing.T) {
	r := customtests.TestRule{
		ID:    "bad-not-regex",
		Type:  customtests.RegexNotMatch,
		Value: `(unclosed`,
	}
	_, err := buildRuleExpr(r, 0)
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
	if !strings.Contains(err.Error(), "bad-not-regex") {
		t.Errorf("error should mention rule ID, got: %v", err)
	}
}

func TestBuildRuleExpr_HeaderExists(t *testing.T) {
	r := customtests.TestRule{
		ID:    "hdr-exists",
		Type:  customtests.HeaderExists,
		Value: "X-Robots-Tag",
	}
	expr, err := buildRuleExpr(r, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := "mapContains(headers, {v7:String})"
	if expr.sql != wantSQL {
		t.Errorf("sql = %q, want %q", expr.sql, wantSQL)
	}
	if len(expr.args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(expr.args))
	}
	name, val := namedVal(expr.args[0])
	if name != "v7" {
		t.Errorf("arg name = %q, want %q", name, "v7")
	}
	if val != "X-Robots-Tag" {
		t.Errorf("arg value = %q, want %q", val, "X-Robots-Tag")
	}
}

func TestBuildRuleExpr_HeaderNotExists(t *testing.T) {
	r := customtests.TestRule{
		ID:    "hdr-not-exists",
		Type:  customtests.HeaderNotExists,
		Value: "X-Frame-Options",
	}
	expr, err := buildRuleExpr(r, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := "NOT mapContains(headers, {v1:String})"
	if expr.sql != wantSQL {
		t.Errorf("sql = %q, want %q", expr.sql, wantSQL)
	}
	if len(expr.args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(expr.args))
	}
	name, val := namedVal(expr.args[0])
	if name != "v1" {
		t.Errorf("arg name = %q, want %q", name, "v1")
	}
	if val != "X-Frame-Options" {
		t.Errorf("arg value = %q, want %q", val, "X-Frame-Options")
	}
}

func TestBuildRuleExpr_HeaderContains(t *testing.T) {
	r := customtests.TestRule{
		ID:    "hdr-contains",
		Type:  customtests.HeaderContains,
		Value: "Content-Type",
		Extra: "text/html",
	}
	expr, err := buildRuleExpr(r, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := "mapContains(headers, {v4:String}) AND position(headers[{v4:String}], {ex4:String}) > 0"
	if expr.sql != wantSQL {
		t.Errorf("sql = %q, want %q", expr.sql, wantSQL)
	}
	if len(expr.args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(expr.args))
	}

	name0, val0 := namedVal(expr.args[0])
	if name0 != "v4" {
		t.Errorf("arg[0] name = %q, want %q", name0, "v4")
	}
	if val0 != "Content-Type" {
		t.Errorf("arg[0] value = %q, want %q", val0, "Content-Type")
	}

	name1, val1 := namedVal(expr.args[1])
	if name1 != "ex4" {
		t.Errorf("arg[1] name = %q, want %q", name1, "ex4")
	}
	if val1 != "text/html" {
		t.Errorf("arg[1] value = %q, want %q", val1, "text/html")
	}
}

func TestBuildRuleExpr_HeaderRegex(t *testing.T) {
	r := customtests.TestRule{
		ID:    "hdr-regex",
		Type:  customtests.HeaderRegex,
		Value: "X-Robots-Tag",
		Extra: `noindex|nofollow`,
	}
	expr, err := buildRuleExpr(r, 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSQL := "mapContains(headers, {v9:String}) AND match(headers[{v9:String}], {ex9:String})"
	if expr.sql != wantSQL {
		t.Errorf("sql = %q, want %q", expr.sql, wantSQL)
	}
	if len(expr.args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(expr.args))
	}

	name0, val0 := namedVal(expr.args[0])
	if name0 != "v9" {
		t.Errorf("arg[0] name = %q, want %q", name0, "v9")
	}
	if val0 != "X-Robots-Tag" {
		t.Errorf("arg[0] value = %q, want %q", val0, "X-Robots-Tag")
	}

	name1, val1 := namedVal(expr.args[1])
	if name1 != "ex9" {
		t.Errorf("arg[1] name = %q, want %q", name1, "ex9")
	}
	if val1 != `noindex|nofollow` {
		t.Errorf("arg[1] value = %q, want %q", val1, `noindex|nofollow`)
	}
}

func TestBuildRuleExpr_HeaderRegex_InvalidPattern(t *testing.T) {
	r := customtests.TestRule{
		ID:    "hdr-bad-regex",
		Type:  customtests.HeaderRegex,
		Value: "X-Custom",
		Extra: `[invalid`,
	}
	_, err := buildRuleExpr(r, 0)
	if err == nil {
		t.Fatal("expected error for invalid header regex, got nil")
	}
	if !strings.Contains(err.Error(), "hdr-bad-regex") {
		t.Errorf("error should mention rule ID, got: %v", err)
	}
}

func TestBuildRuleExpr_HeaderRegex_TooLongPattern(t *testing.T) {
	r := customtests.TestRule{
		ID:    "hdr-long-regex",
		Type:  customtests.HeaderRegex,
		Value: "X-Custom",
		Extra: strings.Repeat("a", maxRegexLen+1),
	}
	_, err := buildRuleExpr(r, 0)
	if err == nil {
		t.Fatal("expected error for too-long header regex, got nil")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("error should mention 'too long', got: %v", err)
	}
}

func TestBuildRuleExpr_DefaultCase(t *testing.T) {
	r := customtests.TestRule{
		ID:   "unknown-rule",
		Type: customtests.RuleType("nonexistent_type"),
	}
	expr, err := buildRuleExpr(r, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expr.sql != "0" {
		t.Errorf("sql = %q, want %q", expr.sql, "0")
	}
	if expr.args != nil {
		t.Errorf("args = %v, want nil", expr.args)
	}
}

func TestBuildRuleExpr_CSSType_FallsToDefault(t *testing.T) {
	// CSS types are not ClickHouse-native, should fall to default
	r := customtests.TestRule{
		ID:    "css-rule",
		Type:  customtests.CSSExists,
		Value: "div.content",
	}
	expr, err := buildRuleExpr(r, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expr.sql != "0" {
		t.Errorf("sql = %q, want %q for non-native type", expr.sql, "0")
	}
	if expr.args != nil {
		t.Errorf("args = %v, want nil for non-native type", expr.args)
	}
}

// TestBuildRuleExpr_IndexParameterization verifies that different idx values
// produce correctly numbered parameter names.
func TestBuildRuleExpr_IndexParameterization(t *testing.T) {
	tests := []struct {
		name      string
		idx       int
		ruleType  customtests.RuleType
		wantVName string
		wantEName string // empty for single-arg types
	}{
		{
			name:      "idx 0 StringContains",
			idx:       0,
			ruleType:  customtests.StringContains,
			wantVName: "v0",
		},
		{
			name:      "idx 42 StringContains",
			idx:       42,
			ruleType:  customtests.StringContains,
			wantVName: "v42",
		},
		{
			name:      "idx 10 HeaderContains",
			idx:       10,
			ruleType:  customtests.HeaderContains,
			wantVName: "v10",
			wantEName: "ex10",
		},
		{
			name:      "idx 99 HeaderRegex",
			idx:       99,
			ruleType:  customtests.HeaderRegex,
			wantVName: "v99",
			wantEName: "ex99",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := customtests.TestRule{
				ID:    "idx-test",
				Type:  tc.ruleType,
				Value: "test-value",
				Extra: "test-extra",
			}
			expr, err := buildRuleExpr(r, tc.idx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check that the SQL contains the expected parameter name
			if !strings.Contains(expr.sql, tc.wantVName+":String") {
				t.Errorf("sql = %q, expected to contain %q", expr.sql, tc.wantVName+":String")
			}

			// Verify the first named arg has the correct name
			name0, _ := namedVal(expr.args[0])
			if name0 != tc.wantVName {
				t.Errorf("arg[0] name = %q, want %q", name0, tc.wantVName)
			}

			// For two-arg types, check the extra parameter
			if tc.wantEName != "" {
				if len(expr.args) < 2 {
					t.Fatalf("expected 2 args for type %s, got %d", tc.ruleType, len(expr.args))
				}
				if !strings.Contains(expr.sql, tc.wantEName+":String") {
					t.Errorf("sql = %q, expected to contain %q", expr.sql, tc.wantEName+":String")
				}
				name1, _ := namedVal(expr.args[1])
				if name1 != tc.wantEName {
					t.Errorf("arg[1] name = %q, want %q", name1, tc.wantEName)
				}
			}
		})
	}
}

// TestBuildRuleExpr_AllTypes is a comprehensive table-driven test covering
// every supported rule type in a single table.
func TestBuildRuleExpr_AllTypes(t *testing.T) {
	tests := []struct {
		name     string
		rule     customtests.TestRule
		idx      int
		wantSQL  string
		wantArgs []driver.NamedValue
		wantErr  bool
	}{
		{
			name: "StringContains",
			rule: customtests.TestRule{ID: "sc", Type: customtests.StringContains, Value: "foo"},
			idx:  0,
			wantSQL:  "position(body_html, {v0:String}) > 0",
			wantArgs: []driver.NamedValue{{Name: "v0", Value: "foo"}},
		},
		{
			name: "StringNotContains",
			rule: customtests.TestRule{ID: "snc", Type: customtests.StringNotContains, Value: "bar"},
			idx:  1,
			wantSQL:  "position(body_html, {v1:String}) = 0",
			wantArgs: []driver.NamedValue{{Name: "v1", Value: "bar"}},
		},
		{
			name: "RegexMatch valid",
			rule: customtests.TestRule{ID: "rm", Type: customtests.RegexMatch, Value: `\d+`},
			idx:  2,
			wantSQL:  "match(body_html, {v2:String})",
			wantArgs: []driver.NamedValue{{Name: "v2", Value: `\d+`}},
		},
		{
			name:    "RegexMatch invalid",
			rule:    customtests.TestRule{ID: "rm-bad", Type: customtests.RegexMatch, Value: `[`},
			idx:     0,
			wantErr: true,
		},
		{
			name: "RegexNotMatch valid",
			rule: customtests.TestRule{ID: "rnm", Type: customtests.RegexNotMatch, Value: `^ok$`},
			idx:  3,
			wantSQL:  "NOT match(body_html, {v3:String})",
			wantArgs: []driver.NamedValue{{Name: "v3", Value: `^ok$`}},
		},
		{
			name:    "RegexNotMatch invalid",
			rule:    customtests.TestRule{ID: "rnm-bad", Type: customtests.RegexNotMatch, Value: `(`},
			idx:     0,
			wantErr: true,
		},
		{
			name: "HeaderExists",
			rule: customtests.TestRule{ID: "he", Type: customtests.HeaderExists, Value: "Content-Type"},
			idx:  4,
			wantSQL:  "mapContains(headers, {v4:String})",
			wantArgs: []driver.NamedValue{{Name: "v4", Value: "Content-Type"}},
		},
		{
			name: "HeaderNotExists",
			rule: customtests.TestRule{ID: "hne", Type: customtests.HeaderNotExists, Value: "X-Debug"},
			idx:  5,
			wantSQL:  "NOT mapContains(headers, {v5:String})",
			wantArgs: []driver.NamedValue{{Name: "v5", Value: "X-Debug"}},
		},
		{
			name: "HeaderContains",
			rule: customtests.TestRule{ID: "hc", Type: customtests.HeaderContains, Value: "Content-Type", Extra: "html"},
			idx:  6,
			wantSQL:  "mapContains(headers, {v6:String}) AND position(headers[{v6:String}], {ex6:String}) > 0",
			wantArgs: []driver.NamedValue{{Name: "v6", Value: "Content-Type"}, {Name: "ex6", Value: "html"}},
		},
		{
			name: "HeaderRegex valid",
			rule: customtests.TestRule{ID: "hr", Type: customtests.HeaderRegex, Value: "X-Robots-Tag", Extra: `noindex|nofollow`},
			idx:  7,
			wantSQL:  "mapContains(headers, {v7:String}) AND match(headers[{v7:String}], {ex7:String})",
			wantArgs: []driver.NamedValue{{Name: "v7", Value: "X-Robots-Tag"}, {Name: "ex7", Value: `noindex|nofollow`}},
		},
		{
			name:    "HeaderRegex invalid extra",
			rule:    customtests.TestRule{ID: "hr-bad", Type: customtests.HeaderRegex, Value: "X-H", Extra: `[bad`},
			idx:     0,
			wantErr: true,
		},
		{
			name: "unknown type falls to default",
			rule: customtests.TestRule{ID: "unk", Type: customtests.RuleType("unknown")},
			idx:  0,
			wantSQL:  "0",
			wantArgs: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := buildRuleExpr(tc.rule, tc.idx)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if expr.sql != tc.wantSQL {
				t.Errorf("sql = %q, want %q", expr.sql, tc.wantSQL)
			}

			if tc.wantArgs == nil {
				if expr.args != nil {
					t.Errorf("args = %v, want nil", expr.args)
				}
				return
			}

			if len(expr.args) != len(tc.wantArgs) {
				t.Fatalf("got %d args, want %d", len(expr.args), len(tc.wantArgs))
			}

			for i, wantNV := range tc.wantArgs {
				gotName, gotVal := namedVal(expr.args[i])
				if gotName != wantNV.Name {
					t.Errorf("arg[%d] name = %q, want %q", i, gotName, wantNV.Name)
				}
				if gotVal != wantNV.Value {
					t.Errorf("arg[%d] value = %v, want %v", i, gotVal, wantNV.Value)
				}
			}
		})
	}
}

// Ensure the clickhouse import is used (compile check).
var _ = clickhouse.Named
