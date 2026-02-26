package parser

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/SEObserver/seocrawler/internal/normalizer"
	"golang.org/x/net/publicsuffix"
)

// Link represents an extracted link from a page.
type Link struct {
	TargetURL  string
	AnchorText string
	Rel        string
	IsInternal bool
	Tag        string // "a", "link", "area", etc.
}

func extractLinks(doc *goquery.Document, baseURL *url.URL) []Link {
	var links []Link

	doc.Find("a[href], area[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}

		href = strings.TrimSpace(href)

		// Skip non-HTTP links
		if isNonHTTP(href) {
			return
		}

		resolved, err := normalizer.Resolve(baseURL.String(), href)
		if err != nil {
			return
		}

		rel, _ := s.Attr("rel")
		tag := goquery.NodeName(s)

		links = append(links, Link{
			TargetURL:  resolved,
			AnchorText: strings.TrimSpace(s.Text()),
			Rel:        strings.TrimSpace(rel),
			IsInternal: isInternal(baseURL, resolved),
			Tag:        tag,
		})
	})

	return links
}

func isInternal(baseURL *url.URL, targetURL string) bool {
	target, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	baseDomain, err1 := publicsuffix.EffectiveTLDPlusOne(baseURL.Hostname())
	targetDomain, err2 := publicsuffix.EffectiveTLDPlusOne(target.Hostname())
	if err1 != nil || err2 != nil {
		return strings.EqualFold(baseURL.Hostname(), target.Hostname())
	}
	return strings.EqualFold(baseDomain, targetDomain)
}

func isNonHTTP(href string) bool {
	lower := strings.ToLower(href)
	return strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "tel:") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "#")
}
