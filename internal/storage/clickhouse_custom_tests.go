package storage

import (
	"context"
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/customtests"
)

// safeRuleID matches only alphanumeric, hyphens, underscores (safe for SQL identifiers).
var safeRuleID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// PageHTMLRow is a url+html pair streamed from ClickHouse.
type PageHTMLRow struct {
	URL  string
	HTML string
}

// maxRegexLen limits regex pattern length to prevent resource abuse.
const maxRegexLen = 1000

// validateRegex checks that a pattern is valid RE2 syntax and within size limits.
func validateRegex(pattern string) error {
	if len(pattern) > maxRegexLen {
		return fmt.Errorf("regex pattern too long (%d chars, max %d)", len(pattern), maxRegexLen)
	}
	if _, err := syntax.Parse(pattern, syntax.Perl); err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}
	return nil
}

// ruleExpr holds a parameterized SQL expression and its named arguments.
type ruleExpr struct {
	sql  string
	args []any
}

// buildRuleExpr returns a parameterized ClickHouse SQL expression for a single rule.
// All user-provided values are passed as named parameters, never interpolated.
func buildRuleExpr(r customtests.TestRule, idx int) (ruleExpr, error) {
	vName := fmt.Sprintf("v%d", idx)
	exName := fmt.Sprintf("ex%d", idx)

	switch r.Type {
	case customtests.StringContains:
		return ruleExpr{
			sql:  fmt.Sprintf("position(body_html, {%s:String}) > 0", vName),
			args: []any{clickhouse.Named(vName, r.Value)},
		}, nil

	case customtests.StringNotContains:
		return ruleExpr{
			sql:  fmt.Sprintf("position(body_html, {%s:String}) = 0", vName),
			args: []any{clickhouse.Named(vName, r.Value)},
		}, nil

	case customtests.RegexMatch:
		if err := validateRegex(r.Value); err != nil {
			return ruleExpr{}, fmt.Errorf("rule %q: %w", r.ID, err)
		}
		return ruleExpr{
			sql:  fmt.Sprintf("match(body_html, {%s:String})", vName),
			args: []any{clickhouse.Named(vName, r.Value)},
		}, nil

	case customtests.RegexNotMatch:
		if err := validateRegex(r.Value); err != nil {
			return ruleExpr{}, fmt.Errorf("rule %q: %w", r.ID, err)
		}
		return ruleExpr{
			sql:  fmt.Sprintf("NOT match(body_html, {%s:String})", vName),
			args: []any{clickhouse.Named(vName, r.Value)},
		}, nil

	case customtests.HeaderExists:
		return ruleExpr{
			sql:  fmt.Sprintf("mapContains(headers, {%s:String})", vName),
			args: []any{clickhouse.Named(vName, r.Value)},
		}, nil

	case customtests.HeaderNotExists:
		return ruleExpr{
			sql:  fmt.Sprintf("NOT mapContains(headers, {%s:String})", vName),
			args: []any{clickhouse.Named(vName, r.Value)},
		}, nil

	case customtests.HeaderContains:
		return ruleExpr{
			sql:  fmt.Sprintf("mapContains(headers, {%s:String}) AND position(headers[{%s:String}], {%s:String}) > 0", vName, vName, exName),
			args: []any{clickhouse.Named(vName, r.Value), clickhouse.Named(exName, r.Extra)},
		}, nil

	case customtests.HeaderRegex:
		if err := validateRegex(r.Extra); err != nil {
			return ruleExpr{}, fmt.Errorf("rule %q: %w", r.ID, err)
		}
		return ruleExpr{
			sql:  fmt.Sprintf("mapContains(headers, {%s:String}) AND match(headers[{%s:String}], {%s:String})", vName, vName, exName),
			args: []any{clickhouse.Named(vName, r.Value), clickhouse.Named(exName, r.Extra)},
		}, nil

	default:
		return ruleExpr{sql: "0", args: nil}, nil
	}
}

// RunCustomTestsSQL runs ClickHouse-native test rules as a single query.
// All user values are parameterized via named placeholders.
func (s *Store) RunCustomTestsSQL(ctx context.Context, sessionID string, rules []customtests.TestRule) (map[string]map[string]string, error) {
	if len(rules) == 0 {
		return map[string]map[string]string{}, nil
	}

	var selects []string
	var allArgs []any
	selects = append(selects, "url")
	allArgs = append(allArgs, clickhouse.Named("sessionID", sessionID))

	for i, r := range rules {
		if !safeRuleID.MatchString(r.ID) {
			return nil, fmt.Errorf("invalid rule ID: %q", r.ID)
		}
		expr, err := buildRuleExpr(r, i)
		if err != nil {
			return nil, err
		}
		selects = append(selects, fmt.Sprintf("(%s) AS `%s`", expr.sql, r.ID))
		allArgs = append(allArgs, expr.args...)
	}

	query := fmt.Sprintf("SELECT %s FROM crawlobserver.pages WHERE crawl_session_id = {sessionID:String} AND "+notRedirectedFilter,
		strings.Join(selects, ", "))

	rows, err := s.conn.Query(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("running custom tests SQL: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[string]string)
	for rows.Next() {
		// Scan url + one bool per rule
		vals := make([]interface{}, 1+len(rules))
		var url string
		vals[0] = &url
		bools := make([]bool, len(rules))
		for i := range rules {
			vals[i+1] = &bools[i]
		}
		if err := rows.Scan(vals...); err != nil {
			return nil, fmt.Errorf("scanning custom test row: %w", err)
		}
		m := make(map[string]string, len(rules))
		for i, r := range rules {
			if bools[i] {
				m[r.ID] = "pass"
			} else {
				m[r.ID] = "fail"
			}
		}
		result[url] = m
	}
	return result, nil
}

// StreamPagesHTML streams url+body_html pairs for a session.
func (s *Store) StreamPagesHTML(ctx context.Context, sessionID string) (<-chan PageHTMLRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT url, body_html FROM crawlobserver.pages
		WHERE crawl_session_id = {sessionID:String} AND body_html != ''`,
		clickhouse.Named("sessionID", sessionID))
	if err != nil {
		return nil, fmt.Errorf("streaming pages HTML: %w", err)
	}

	ch := make(chan PageHTMLRow, 64)
	go func() {
		defer close(ch)
		defer rows.Close()
		for rows.Next() {
			var r PageHTMLRow
			if err := rows.Scan(&r.URL, &r.HTML); err != nil {
				applog.Errorf("storage", "StreamPagesHTML scan error: %v", err)
				return
			}
			select {
			case ch <- r:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
