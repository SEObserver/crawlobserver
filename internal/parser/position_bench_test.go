package parser

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// benchPage builds the 120-link page used by the focused extraction benchmark.
func benchPage() string {
	return benchPageWithLinks(120)
}

// benchPageWithLinks builds a deterministic template page with exactly n
// successfully extractable links. Keeping all links in one list makes the
// sibling-index work visible in the benchmark while the surrounding markup
// keeps the full Parse benchmark representative of a normal page.
func benchPageWithLinks(n int) string {
	var b strings.Builder
	b.Grow(n * 80)
	b.WriteString(`<!DOCTYPE html><html><head><title>Benchmark</title></head><body class="page-id-1274 single">`)
	b.WriteString(`<header id="masthead" class="site-header"><h1>Benchmark page</h1></header>`)
	b.WriteString(`<nav class="primary-menu"><ul>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<li class="menu-item"><a href="/link/%d">Link %d</a></li>`, i, i)
	}
	b.WriteString(`</ul></nav>`)
	b.WriteString(`<main id="content"><article class="post"><div class="entry-content">Some body copy around the links.</div></article></main>`)
	b.WriteString(`<aside class="sidebar"><div class="widget related-items">Related links</div></aside>`)
	b.WriteString(`<footer class="site-footer"><div class="footer-columns">Footer content</div></footer></body></html>`)
	return b.String()
}

func benchmarkExtractLinks(b *testing.B, opts Options) {
	page := benchPage()
	base, err := url.Parse("https://example.com/page")
	if err != nil {
		b.Fatalf("url.Parse() error = %v", err)
	}
	doc := docFromHTML(page)

	links := extractLinks(doc, base, opts)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractLinks(doc, base, opts)
	}
	b.StopTimer()

	// The retained cost of the feature is the XPath string kept per link;
	// everything else it records is fixed-size.
	xpathBytes := 0
	for _, l := range links {
		xpathBytes += len(l.XPath)
	}
	b.ReportMetric(float64(xpathBytes)/float64(len(links)), "xpath-B/link")
}

func BenchmarkExtractLinks(b *testing.B) {
	benchmarkExtractLinks(b, Options{LinkPosition: false})
}

func BenchmarkExtractLinksWithPosition(b *testing.B) {
	benchmarkExtractLinks(b, Options{LinkPosition: true})
}

// BenchmarkParseLinkPositions measures the complete parser path, including
// document parsing and all SEO extractors, for the requested page sizes. The
// capped case is the production default: only the first 1,000 links receive
// position metadata, while the all case is useful for comparing the cost of
// retaining that metadata for every link.
func BenchmarkParseLinkPositions(b *testing.B) {
	for _, linkCount := range []int{120, 1000, 10000} {
		body := []byte(benchPageWithLinks(linkCount))
		for _, tc := range []struct {
			name string
			opts Options
		}{
			{name: "off", opts: Options{LinkPosition: false}},
			{name: "capped-1000", opts: Options{LinkPosition: true, MaxLinkPositions: DefaultMaxLinkPositions}},
			{name: "all", opts: Options{LinkPosition: true, MaxLinkPositions: linkCount}},
		} {
			b.Run(fmt.Sprintf("links=%d/%s", linkCount, tc.name), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				b.ReportMetric(float64(linkCount), "links/page")
				for i := 0; i < b.N; i++ {
					if _, err := ParseWithOptions(body, "https://example.com/page", tc.opts); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
