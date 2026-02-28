package storage

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/SEObserver/crawlobserver/internal/customtests"
)

// safeRuleID matches only alphanumeric, hyphens, underscores (safe for SQL identifiers).
var safeRuleID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// PageHTMLRow is a url+html pair streamed from ClickHouse.
type PageHTMLRow struct {
	URL  string
	HTML string
}

// chEscapeString escapes a string for safe embedding in a ClickHouse single-quoted literal.
// Backslashes must be escaped first, then single quotes.
func chEscapeString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

// buildRuleExpr returns a ClickHouse SQL expression for a single rule.
func buildRuleExpr(r customtests.TestRule) string {
	v := chEscapeString(r.Value)
	ex := chEscapeString(r.Extra)
	switch r.Type {
	case customtests.StringContains:
		return fmt.Sprintf("position(body_html, '%s') > 0", v)
	case customtests.StringNotContains:
		return fmt.Sprintf("position(body_html, '%s') = 0", v)
	case customtests.RegexMatch:
		return fmt.Sprintf("match(body_html, '%s')", v)
	case customtests.RegexNotMatch:
		return fmt.Sprintf("NOT match(body_html, '%s')", v)
	case customtests.HeaderExists:
		return fmt.Sprintf("mapContains(headers, '%s')", v)
	case customtests.HeaderNotExists:
		return fmt.Sprintf("NOT mapContains(headers, '%s')", v)
	case customtests.HeaderContains:
		return fmt.Sprintf("mapContains(headers, '%s') AND position(headers['%s'], '%s') > 0", v, v, ex)
	case customtests.HeaderRegex:
		return fmt.Sprintf("mapContains(headers, '%s') AND match(headers['%s'], '%s')", v, v, ex)
	default:
		return "0"
	}
}

// RunCustomTestsSQL runs ClickHouse-native test rules as a single query.
func (s *Store) RunCustomTestsSQL(ctx context.Context, sessionID string, rules []customtests.TestRule) (map[string]map[string]string, error) {
	if len(rules) == 0 {
		return map[string]map[string]string{}, nil
	}

	var selects []string
	selects = append(selects, "url")
	for _, r := range rules {
		if !safeRuleID.MatchString(r.ID) {
			return nil, fmt.Errorf("invalid rule ID: %q", r.ID)
		}
		selects = append(selects, fmt.Sprintf("(%s) AS `%s`", buildRuleExpr(r), r.ID))
	}

	query := fmt.Sprintf("SELECT %s FROM crawlobserver.pages WHERE crawl_session_id = {sessionID:String}",
		strings.Join(selects, ", "))

	rows, err := s.conn.Query(ctx, query, clickhouse.Named("sessionID", sessionID))
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
				log.Printf("StreamPagesHTML scan error: %v", err)
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
