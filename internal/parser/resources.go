package parser

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/SEObserver/crawlobserver/internal/normalizer"
)

// PageResource represents a CSS, JS, font, or icon resource referenced by a page.
type PageResource struct {
	URL          string
	ResourceType string // "css", "js", "font", "icon"
	IsInternal   bool
}

// ExtractResources extracts external resource references (CSS, JS, fonts, icons) from the document.
func ExtractResources(doc *goquery.Document, baseURL *url.URL) []PageResource {
	seen := make(map[string]bool)
	var resources []PageResource

	add := func(href, resType string) {
		href = strings.TrimSpace(href)
		if href == "" || strings.HasPrefix(strings.ToLower(href), "data:") {
			return
		}
		resolved, err := normalizer.Resolve(baseURL.String(), href)
		if err != nil {
			return
		}
		key := resolved + "|" + resType
		if seen[key] {
			return
		}
		seen[key] = true
		resources = append(resources, PageResource{
			URL:          resolved,
			ResourceType: resType,
			IsInternal:   isInternal(baseURL, resolved),
		})
	}

	// <link> tags
	doc.Find("link[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		rel, _ := s.Attr("rel")
		rel = strings.ToLower(strings.TrimSpace(rel))

		switch {
		case rel == "stylesheet":
			add(href, "css")
		case rel == "icon" || rel == "shortcut icon" || rel == "apple-touch-icon":
			add(href, "icon")
		case rel == "preload":
			as, _ := s.Attr("as")
			switch strings.ToLower(strings.TrimSpace(as)) {
			case "style":
				add(href, "css")
			case "script":
				add(href, "js")
			case "font":
				add(href, "font")
			}
		}
	})

	// <script src> (external only)
	doc.Find("script[src]").Each(func(_ int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		add(src, "js")
	})

	return resources
}
