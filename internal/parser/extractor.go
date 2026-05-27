package parser

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/SEObserver/crawlobserver/internal/htmlutil"
)

func extractTitle(doc *goquery.Document) string {
	return strings.TrimSpace(doc.Find("title").First().Text())
}

func extractCanonical(doc *goquery.Document) string {
	var canonical string
	doc.Find("link").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if htmlutil.AttrTokenContains(s, "rel", "canonical") {
			canonical, _ = htmlutil.Attr(s, "href")
			return false
		}
		return true
	})
	return strings.TrimSpace(canonical)
}

func extractMetaContent(doc *goquery.Document, name string) string {
	var content string
	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		n, _ := htmlutil.Attr(s, "name")
		if strings.EqualFold(n, name) {
			content, _ = htmlutil.Attr(s, "content")
		}
	})
	return strings.TrimSpace(content)
}

func extractHeadings(doc *goquery.Document, tag string) []string {
	var headings []string
	doc.Find(tag).Each(func(_ int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			headings = append(headings, text)
		}
	})
	return headings
}
