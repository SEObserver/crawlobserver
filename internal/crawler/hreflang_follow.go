package crawler

import (
	"github.com/SEObserver/crawlobserver/internal/fetcher"
	"github.com/SEObserver/crawlobserver/internal/frontier"
	"github.com/SEObserver/crawlobserver/internal/normalizer"
	"github.com/SEObserver/crawlobserver/internal/parser"
)

// enqueueHreflangAlternates adds the page's hreflang alternates to the frontier
// when the crawl asked for it. Language versions reachable only through a
// script-driven language switcher carry no <a href> the link extractor could
// follow, but their <link rel="alternate" hreflang> annotations still name
// them.
//
// Alternates go through the same scope, exclusion and depth checks as links
// and sit one level below the page that declares them, so a crawl scoped to
// one host keeps ignoring alternates hosted elsewhere and max_depth bounds
// them like any other discovery. They are not recorded as links: the
// annotations themselves are already stored with the page.
//
// It returns the number of alternates actually queued, for tests and logging.
func (e *Engine) enqueueHreflangAlternates(result *fetcher.FetchResult, alternates []parser.HreflangEntry) int {
	if !e.followHreflang || e.sitemapOnly || len(alternates) == 0 {
		return 0
	}
	newDepth := result.Depth + 1
	if e.cfg.Crawler.MaxDepth > 0 && newDepth > e.cfg.Crawler.MaxDepth {
		return 0
	}
	// Relative hrefs resolve against the final URL, like links do. The frontier
	// already knows the requested URL, but not the final one after redirects,
	// so a self-referencing alternate on a redirected page is compared with a
	// normalized copy of it.
	base := result.FinalURL
	if base == "" {
		base = result.URL
	}
	self := result.URL
	if normalizedFinal, err := normalizer.Normalize(base); err == nil {
		self = normalizedFinal
	}
	added := 0
	for _, alt := range alternates {
		target, err := normalizer.Resolve(base, alt.URL)
		if err != nil {
			continue
		}
		if target == self || target == result.URL {
			continue
		}
		if !e.isInScope(target) || e.isExcluded(target) {
			continue
		}
		priority := newDepth
		if e.sitemapURLSet[target] && priority > 1 {
			priority = 1
		}
		if e.front.Add(frontier.CrawlURL{
			URL:      target,
			Priority: priority,
			Depth:    newDepth,
			FoundOn:  result.URL,
		}) {
			added++
		}
	}
	return added
}
