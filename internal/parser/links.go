package parser

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/SEObserver/crawlobserver/internal/htmlutil"
	"github.com/SEObserver/crawlobserver/internal/normalizer"
	"golang.org/x/net/publicsuffix"
)

// Link represents an extracted link from a page.
type Link struct {
	TargetURL  string
	AnchorText string
	Rel        string
	IsInternal bool
	Tag        string // "a", "link", "area", etc.

	// Where the link sits in the document. These describe the markup around
	// the link and nothing else: what the link is for is left to the caller.
	// They are zero when extraction is disabled or the per-page limit has
	// been reached; see Options.LinkPosition and Options.MaxLinkPositions.
	Landmark       string // "main", "article", "nav", "header", "footer", "aside", or "" when the link is in none of them
	XPath          string // absolute path of the link element, e.g. /html/body/nav/ul/li[2]/a
	Depth          uint16 // number of element ancestors above the link
	DocumentIndex  uint32 // 0-based rank among the page's extracted links, in document order
	BlockSignature uint64 // identifies the block the link sits in, see blockSignature
}

func extractLinks(doc *goquery.Document, baseURL *url.URL, opts Options) []Link {
	var links []Link
	var xpathCache *xpathIndexCache
	if opts.LinkPosition {
		xpathCache = newXPathIndexCache()
	}
	maxLinkPositions := effectiveMaxLinkPositions(opts.MaxLinkPositions)

	doc.Find("a, area").Each(func(_ int, s *goquery.Selection) {
		href, exists := htmlutil.Attr(s, "href")
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

		rel, _ := htmlutil.Attr(s, "rel")
		tag := goquery.NodeName(s)

		link := Link{
			TargetURL:  resolved,
			AnchorText: strings.TrimSpace(s.Text()),
			Rel:        strings.TrimSpace(rel),
			IsInternal: isInternal(baseURL, resolved),
			Tag:        tag,
		}

		// Position metadata is deliberately bounded independently from link
		// extraction. The link remains in the result so crawling, counts and
		// PageRank still see the complete page, while expensive position work is
		// reserved for the first successfully extracted links.
		if opts.LinkPosition && len(links) < maxLinkPositions {
			n := s.Nodes[0]
			link.Landmark = landmarkOf(n)
			link.XPath = nodeXPathWithCache(n, xpathCache)
			link.Depth = nodeDepth(n)
			link.DocumentIndex = uint32(len(links))
			link.BlockSignature = blockSignature(n)
		}

		links = append(links, link)
	})

	return links
}

func effectiveMaxLinkPositions(max int) int {
	if max <= 0 {
		return DefaultMaxLinkPositions
	}
	return max
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
