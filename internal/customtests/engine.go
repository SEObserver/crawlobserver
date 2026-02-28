package customtests

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

const (
	maxExtractLen = 500
	maxExtractAll = 20
)

// PageHTMLRow mirrors storage.PageHTMLRow to avoid circular imports.
type PageHTMLRow struct {
	URL  string
	HTML string
}

// StorageInterface is the subset of storage.Store needed by the engine.
type StorageInterface interface {
	RunCustomTestsSQL(ctx context.Context, sessionID string, rules []TestRule) (map[string]map[string]string, error)
	StreamPagesHTML(ctx context.Context, sessionID string) (<-chan PageHTMLRow, error)
}

// RunTests executes all rules from a ruleset against a crawl session.
func RunTests(ctx context.Context, store StorageInterface, sessionID string, ruleset *Ruleset) (*TestRunResult, error) {
	var chRules, goRules []TestRule
	for _, r := range ruleset.Rules {
		if r.Type.IsClickHouseNative() {
			chRules = append(chRules, r)
		} else {
			goRules = append(goRules, r)
		}
	}

	// results: url → ruleID → value
	merged := make(map[string]map[string]string)

	// 1. Run ClickHouse-native rules
	if len(chRules) > 0 {
		chResults, err := store.RunCustomTestsSQL(ctx, sessionID, chRules)
		if err != nil {
			return nil, err
		}
		for url, m := range chResults {
			if merged[url] == nil {
				merged[url] = make(map[string]string)
			}
			for k, v := range m {
				merged[url][k] = v
			}
		}
	}

	// 2. Run Go-evaluated rules by streaming HTML
	if len(goRules) > 0 {
		ch, err := store.StreamPagesHTML(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		for row := range ch {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(row.HTML))
			if err != nil {
				continue
			}
			// Parse HTML node once, shared between goquery and htmlquery
			var htmlNode *html.Node
			if doc.Selection != nil && len(doc.Selection.Nodes) > 0 {
				htmlNode = doc.Selection.Nodes[0]
			}
			if merged[row.URL] == nil {
				merged[row.URL] = make(map[string]string)
			}
			for _, r := range goRules {
				merged[row.URL][r.ID] = evalGoRule(doc, htmlNode, row.HTML, r)
			}
		}
	}

	// Build result
	result := &TestRunResult{
		RulesetID:   ruleset.ID,
		RulesetName: ruleset.Name,
		SessionID:   sessionID,
		TotalPages:  len(merged),
		Rules:       ruleset.Rules,
		Summary:     make(map[string]int),
	}

	for url, m := range merged {
		result.Pages = append(result.Pages, PageTestResult{URL: url, Results: m})
	}

	// Compute summary
	for _, r := range ruleset.Rules {
		count := 0
		for _, p := range result.Pages {
			v := p.Results[r.ID]
			if v == "pass" || (v != "fail" && v != "") {
				count++
			}
		}
		result.Summary[r.ID] = count
	}

	if result.Pages == nil {
		result.Pages = []PageTestResult{}
	}

	return result, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func joinExtracted(items []string) string {
	if len(items) <= maxExtractAll {
		return strings.Join(items, " | ")
	}
	extra := len(items) - maxExtractAll
	return strings.Join(items[:maxExtractAll], " | ") + fmt.Sprintf(" … (+%d more)", extra)
}

func evalGoRule(doc *goquery.Document, htmlNode *html.Node, rawHTML string, r TestRule) string {
	switch r.Type {
	// --- Existing CSS rules ---
	case CSSExists:
		if doc.Find(r.Value).Length() > 0 {
			return "pass"
		}
		return "fail"
	case CSSNotExists:
		if doc.Find(r.Value).Length() == 0 {
			return "pass"
		}
		return "fail"
	case CSSExtractText:
		return truncate(doc.Find(r.Value).First().Text(), maxExtractLen)
	case CSSExtractAttr:
		return truncate(doc.Find(r.Value).First().AttrOr(r.Extra, ""), maxExtractLen)

	// --- CSS extract all ---
	case CSSExtractAllText:
		var items []string
		doc.Find(r.Value).Each(func(_ int, s *goquery.Selection) {
			items = append(items, truncate(s.Text(), maxExtractLen))
		})
		return joinExtracted(items)
	case CSSExtractAllAttr:
		var items []string
		doc.Find(r.Value).Each(func(_ int, s *goquery.Selection) {
			if v, ok := s.Attr(r.Extra); ok {
				items = append(items, truncate(v, maxExtractLen))
			}
		})
		return joinExtracted(items)

	// --- Regex extract ---
	case RegexExtract:
		re, err := regexp.Compile(r.Value)
		if err != nil {
			return ""
		}
		m := re.FindStringSubmatch(rawHTML)
		if len(m) > 1 {
			return truncate(m[1], maxExtractLen)
		}
		if len(m) == 1 {
			return truncate(m[0], maxExtractLen)
		}
		return ""
	case RegexExtractAll:
		re, err := regexp.Compile(r.Value)
		if err != nil {
			return ""
		}
		matches := re.FindAllStringSubmatch(rawHTML, -1)
		var items []string
		for _, m := range matches {
			if len(m) > 1 {
				items = append(items, truncate(m[1], maxExtractLen))
			} else {
				items = append(items, truncate(m[0], maxExtractLen))
			}
		}
		return joinExtracted(items)

	// --- XPath extract ---
	case XPathExtract:
		if htmlNode == nil {
			return ""
		}
		node, err := htmlquery.Query(htmlNode, r.Value)
		if err != nil || node == nil {
			return ""
		}
		return truncate(htmlquery.InnerText(node), maxExtractLen)
	case XPathExtractAll:
		if htmlNode == nil {
			return ""
		}
		nodes, err := htmlquery.QueryAll(htmlNode, r.Value)
		if err != nil {
			return ""
		}
		var items []string
		for _, n := range nodes {
			items = append(items, truncate(htmlquery.InnerText(n), maxExtractLen))
		}
		return joinExtracted(items)
	}
	return ""
}
