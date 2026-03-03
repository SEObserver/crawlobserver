package extraction

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

const (
	maxExtractLen = 500
	maxExtractAll = 20
)

// RunExtractors runs all extractors against a page body and returns rows for insertion.
func RunExtractors(body []byte, url, sessionID string, extractors []Extractor, crawledAt time.Time) []ExtractionRow {
	if len(body) == 0 || len(extractors) == 0 {
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil
	}

	var htmlNode *html.Node
	if len(doc.Nodes) > 0 {
		htmlNode = doc.Nodes[0]
	}

	rawHTML := string(body)
	var rows []ExtractionRow

	for _, ext := range extractors {
		if ext.URLPattern != "" {
			matched, _ := filepath.Match(ext.URLPattern, url)
			if !matched {
				continue
			}
		}

		value := evalExtractor(doc, htmlNode, rawHTML, ext)
		if value == "" {
			continue
		}

		rows = append(rows, ExtractionRow{
			CrawlSessionID: sessionID,
			URL:            url,
			ExtractorName:  ext.Name,
			Value:          value,
			CrawledAt:      crawledAt,
		})
	}

	return rows
}

func evalExtractor(doc *goquery.Document, htmlNode *html.Node, rawHTML string, ext Extractor) string {
	switch ext.Type {
	case CSSExtractText:
		return truncate(doc.Find(ext.Selector).First().Text(), maxExtractLen)
	case CSSExtractAttr:
		return truncate(doc.Find(ext.Selector).First().AttrOr(ext.Attribute, ""), maxExtractLen)
	case CSSExtractAllText:
		var items []string
		doc.Find(ext.Selector).Each(func(_ int, s *goquery.Selection) {
			items = append(items, truncate(s.Text(), maxExtractLen))
		})
		return joinExtracted(items)
	case CSSExtractAllAttr:
		var items []string
		doc.Find(ext.Selector).Each(func(_ int, s *goquery.Selection) {
			if v, ok := s.Attr(ext.Attribute); ok {
				items = append(items, truncate(v, maxExtractLen))
			}
		})
		return joinExtracted(items)
	case RegexExtract:
		re, err := regexp.Compile(ext.Selector)
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
		re, err := regexp.Compile(ext.Selector)
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
	case XPathExtract:
		if htmlNode == nil {
			return ""
		}
		node, err := htmlquery.Query(htmlNode, ext.Selector)
		if err != nil || node == nil {
			return ""
		}
		return truncate(htmlquery.InnerText(node), maxExtractLen)
	case XPathExtractAll:
		if htmlNode == nil {
			return ""
		}
		nodes, err := htmlquery.QueryAll(htmlNode, ext.Selector)
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func joinExtracted(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) <= maxExtractAll {
		return strings.Join(items, " | ")
	}
	extra := len(items) - maxExtractAll
	return strings.Join(items[:maxExtractAll], " | ") + fmt.Sprintf(" … (+%d more)", extra)
}
