package parser

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func extractTitle(doc *goquery.Document) string {
	return strings.TrimSpace(doc.Find("title").First().Text())
}

func extractCanonical(doc *goquery.Document) string {
	canonical, _ := doc.Find("link[rel='canonical']").First().Attr("href")
	return strings.TrimSpace(canonical)
}

func extractMetaContent(doc *goquery.Document, name string) string {
	var content string
	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		n, _ := s.Attr("name")
		if strings.EqualFold(n, name) {
			content, _ = s.Attr("content")
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
