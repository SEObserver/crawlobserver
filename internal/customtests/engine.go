package customtests

import (
	"context"
	"strings"

	"github.com/PuerkitoBio/goquery"
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

// RunTests executes all rules from a profile against a crawl session.
func RunTests(ctx context.Context, store StorageInterface, sessionID string, profile *TestProfile) (*TestRunResult, error) {
	var chRules, cssRules []TestRule
	for _, r := range profile.Rules {
		if r.Type.IsClickHouseNative() {
			chRules = append(chRules, r)
		} else {
			cssRules = append(cssRules, r)
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

	// 2. Run CSS rules by streaming HTML
	if len(cssRules) > 0 {
		ch, err := store.StreamPagesHTML(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		for row := range ch {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(row.HTML))
			if err != nil {
				continue
			}
			if merged[row.URL] == nil {
				merged[row.URL] = make(map[string]string)
			}
			for _, r := range cssRules {
				merged[row.URL][r.ID] = evalCSS(doc, r)
			}
		}
	}

	// Build result
	result := &TestRunResult{
		ProfileID:   profile.ID,
		ProfileName: profile.Name,
		SessionID:   sessionID,
		TotalPages:  len(merged),
		Rules:       profile.Rules,
		Summary:     make(map[string]int),
	}

	for url, m := range merged {
		result.Pages = append(result.Pages, PageTestResult{URL: url, Results: m})
	}

	// Compute summary
	for _, r := range profile.Rules {
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

func evalCSS(doc *goquery.Document, r TestRule) string {
	sel := doc.Find(r.Value)
	switch r.Type {
	case CSSExists:
		if sel.Length() > 0 {
			return "pass"
		}
		return "fail"
	case CSSNotExists:
		if sel.Length() == 0 {
			return "pass"
		}
		return "fail"
	case CSSExtractText:
		return sel.First().Text()
	case CSSExtractAttr:
		return sel.First().AttrOr(r.Extra, "")
	}
	return ""
}
