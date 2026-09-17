package renderer

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// RenderResult holds the outcome of rendering a URL with Chrome.
type RenderResult struct {
	RenderedHTML   string
	RenderDuration time.Duration
	JSErrors       []string
	Error          error

	// Core Web Vitals
	CWVMeasured bool
	CWVLCP      float64 // Largest Contentful Paint (ms)
	CWVCLS      float64 // Cumulative Layout Shift
	CWVTTFB     float64 // Time to First Byte (ms)
}

// Render navigates to the given URL in a headless Chrome page, waits for
// the page to stabilise, and returns the rendered HTML.
func (p *Pool) Render(ctx context.Context, url string) *RenderResult {
	start := time.Now()
	result := &RenderResult{}

	page, err := p.Acquire()
	if err != nil {
		result.Error = fmt.Errorf("acquire page: %w", err)
		result.RenderDuration = time.Since(start)
		return result
	}
	defer p.Release(page)

	page = page.Context(ctx)

	// Block heavy resources to speed up rendering, and carry the crawl's
	// headers to this site's own requests.
	var blocked []proto.NetworkResourceType
	if p.opts.BlockResources {
		blocked = []proto.NetworkResourceType{
			proto.NetworkResourceTypeImage,
			proto.NetworkResourceTypeFont,
			proto.NetworkResourceTypeMedia,
		}
	}
	if router := requestRouter(page, url, blocked, p.opts.ExtraHeaders); router != nil {
		go router.Run()
		defer router.Stop()
	}

	// Collect JS console errors
	var jsErrors []string
	var jsErrorsMu sync.Mutex
	wait := page.EachEvent(func(e *proto.RuntimeExceptionThrown) bool {
		if e.ExceptionDetails != nil && e.ExceptionDetails.Text != "" {
			jsErrorsMu.Lock()
			jsErrors = append(jsErrors, e.ExceptionDetails.Text)
			jsErrorsMu.Unlock()
		}
		return false // keep listening
	})
	_ = wait // we don't block on this; just let it collect in background

	// Navigate
	err = page.Navigate(url)
	if err != nil {
		result.Error = fmt.Errorf("navigate: %w", err)
		result.RenderDuration = time.Since(start)
		return result
	}

	// Wait for page load
	err = page.WaitLoad()
	if err != nil {
		result.Error = fmt.Errorf("wait load: %w", err)
		result.RenderDuration = time.Since(start)
		return result
	}

	// Extra stabilisation wait for JS frameworks to finish rendering
	time.Sleep(500 * time.Millisecond)

	// Extract rendered HTML
	html, err := page.HTML()
	if err != nil {
		result.Error = fmt.Errorf("extract html: %w", err)
		result.RenderDuration = time.Since(start)
		return result
	}

	result.RenderedHTML = html
	result.RenderDuration = time.Since(start)

	jsErrorsMu.Lock()
	result.JSErrors = jsErrors
	jsErrorsMu.Unlock()

	if strings.TrimSpace(html) == "" {
		result.Error = fmt.Errorf("empty rendered HTML")
	}

	return result
}

// RenderWithCWV is like Render but also measures Core Web Vitals (lab data).
// It does NOT block images (LCP depends on them) and uses the Chrome DevTools
// Protocol (PerformanceTimeline + Performance.getMetrics) instead of injected JS
// for reliable measurement:
//   - TTFB: from Performance.getMetrics navigation timing
//   - LCP:  from PerformanceTimeline "largest-contentful-paint" events
//   - CLS:  from PerformanceTimeline "layout-shift" events
func (p *Pool) RenderWithCWV(ctx context.Context, url string) *RenderResult {
	start := time.Now()
	result := &RenderResult{}

	page, err := p.Acquire()
	if err != nil {
		result.Error = fmt.Errorf("acquire page: %w", err)
		result.RenderDuration = time.Since(start)
		return result
	}
	defer p.Release(page)

	page = page.Context(ctx)

	// Block fonts and media but NOT images (LCP needs images), and carry the
	// crawl's headers to this site's own requests.
	var blocked []proto.NetworkResourceType
	if p.opts.BlockResources {
		blocked = []proto.NetworkResourceType{
			proto.NetworkResourceTypeFont,
			proto.NetworkResourceTypeMedia,
		}
	}
	if router := requestRouter(page, url, blocked, p.opts.ExtraHeaders); router != nil {
		go router.Run()
		defer router.Stop()
	}

	// Enable Performance metrics collection (for TTFB)
	_ = proto.PerformanceEnable{}.Call(page)

	// Enable PerformanceTimeline for LCP and CLS events (via CDP, not JS)
	_ = proto.PerformanceTimelineEnable{
		EventTypes: []string{"largest-contentful-paint", "layout-shift"},
	}.Call(page)

	// Collect JS console errors + CWV timeline events
	var jsErrors []string
	var jsErrorsMu sync.Mutex
	var lcpMs float64
	var clsTotal float64
	var cwvMu sync.Mutex

	wait := page.EachEvent(
		func(e *proto.RuntimeExceptionThrown) bool {
			if e.ExceptionDetails != nil && e.ExceptionDetails.Text != "" {
				jsErrorsMu.Lock()
				jsErrors = append(jsErrors, e.ExceptionDetails.Text)
				jsErrorsMu.Unlock()
			}
			return false
		},
		func(e *proto.PerformanceTimelineTimelineEventAdded) bool {
			ev := e.Event
			cwvMu.Lock()
			defer cwvMu.Unlock()
			if ev.LcpDetails != nil {
				// LCP: take the latest (largest) entry; Chrome may emit multiple.
				// Use RenderTime if available (more accurate), else LoadTime.
				ms := float64(ev.LcpDetails.RenderTime)
				if ms == 0 {
					ms = float64(ev.LcpDetails.LoadTime)
				}
				if ms > lcpMs {
					lcpMs = ms
				}
			}
			if ev.LayoutShiftDetails != nil && !ev.LayoutShiftDetails.HadRecentInput {
				clsTotal += ev.LayoutShiftDetails.Value
			}
			return false
		},
	)
	_ = wait

	// Navigate
	err = page.Navigate(url)
	if err != nil {
		result.Error = fmt.Errorf("navigate: %w", err)
		result.RenderDuration = time.Since(start)
		return result
	}

	// Wait for page load
	err = page.WaitLoad()
	if err != nil {
		result.Error = fmt.Errorf("wait load: %w", err)
		result.RenderDuration = time.Since(start)
		return result
	}

	// Extra stabilisation: wait for late LCP candidates and layout shifts.
	time.Sleep(500 * time.Millisecond)

	// TTFB: navigation timing API is reliable — use a single JS call.
	// (Unlike LCP/CLS, the Navigation Timing API is synchronous and complete.)
	var ttfbMs float64
	ttfbObj, evalErr := page.Eval(`performance.getEntriesByType('navigation')[0]?.responseStart || 0`)
	if evalErr == nil && ttfbObj != nil {
		ttfbMs = ttfbObj.Value.Num()
	}

	// Collect LCP/CLS from CDP timeline events.
	cwvMu.Lock()
	finalLCP := lcpMs
	finalCLS := clsTotal
	cwvMu.Unlock()

	// CDP LcpDetails.RenderTime/LoadTime are TimeSinceEpoch (seconds since Unix epoch).
	// Convert to ms relative to navigation start (performance.timeOrigin).
	if finalLCP > 1e9 {
		originObj, originErr := page.Eval(`performance.timeOrigin`)
		if originErr == nil && originObj != nil {
			navOriginMs := originObj.Value.Num()
			if navOriginMs > 0 {
				finalLCP = (finalLCP * 1000) - navOriginMs
			}
		}
	}

	result.CWVMeasured = true
	result.CWVLCP = finalLCP
	result.CWVCLS = finalCLS
	result.CWVTTFB = ttfbMs

	// Extract rendered HTML
	html, err := page.HTML()
	if err != nil {
		result.Error = fmt.Errorf("extract html: %w", err)
		result.RenderDuration = time.Since(start)
		return result
	}

	result.RenderedHTML = html
	result.RenderDuration = time.Since(start)

	jsErrorsMu.Lock()
	result.JSErrors = jsErrors
	jsErrorsMu.Unlock()

	if strings.TrimSpace(html) == "" {
		result.Error = fmt.Errorf("empty rendered HTML")
	}

	return result
}

// requestRouter installs the page's request interception.
//
// It serves two purposes that have to share one router, since rod allows a
// single handler per pattern: dropping resource types the crawl does not need,
// and attaching the crawl's headers to same-host requests only.
//
// The host test is the point. A signature identifies the crawler to one site;
// attaching it to every request a page makes would hand it to each third-party
// script, tag and iframe the page loads, which can replay it against that site
// until it expires. Chromium's own setExtraHTTPHeaders cannot make this
// distinction — it stamps every request — which is why the headers are applied
// here instead.
func requestRouter(page *rod.Page, pageURL string, blocked []proto.NetworkResourceType, headers map[string]string) *rod.HijackRouter {
	if len(blocked) == 0 && len(headers) == 0 {
		return nil
	}

	host := hostOf(pageURL)
	blockedSet := make(map[proto.NetworkResourceType]bool, len(blocked))
	for _, t := range blocked {
		blockedSet[t] = true
	}

	router := page.HijackRequests()
	router.MustAdd("*", func(h *rod.Hijack) {
		if blockedSet[h.Request.Type()] {
			h.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}
		if len(headers) == 0 || host == "" || !sameHost(h.Request.URL().Hostname(), host) {
			h.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}
		h.ContinueRequest(&proto.FetchContinueRequest{
			Headers: mergedHeaders(h, headers),
		})
	})
	return router
}

// mergedHeaders returns the request's own headers with the crawl's headers
// added. CDP replaces the whole set when continueRequest carries headers, so
// the originals have to be carried across or the request loses them.
func mergedHeaders(h *rod.Hijack, extra map[string]string) []*proto.FetchHeaderEntry {
	merged := make(map[string]string)
	for name, values := range h.Request.Req().Header {
		if len(values) > 0 {
			merged[name] = values[0]
		}
	}
	for name, value := range extra {
		merged[name] = value
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]*proto.FetchHeaderEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, &proto.FetchHeaderEntry{Name: name, Value: merged[name]})
	}
	return entries
}

// hostOf returns the hostname of a URL, or "" when it cannot be read — in
// which case no headers are attached, since the safe answer to "is this the
// site we are crawling?" is no.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// sameHost reports whether a request belongs to the site being crawled. A
// subdomain is not the same host: a signature bound to www.example.com is not
// meant for static.example.com, which may well be a third-party bucket.
func sameHost(requestHost, pageHost string) bool {
	return requestHost != "" && strings.EqualFold(requestHost, pageHost)
}
