package renderer

import (
	"context"
	"fmt"
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

	// Block heavy resources to speed up rendering
	if p.opts.BlockResources {
		router := page.HijackRequests()
		router.MustAdd("*", func(h *rod.Hijack) {
			resType := h.Request.Type()
			switch resType {
			case proto.NetworkResourceTypeImage,
				proto.NetworkResourceTypeFont,
				proto.NetworkResourceTypeMedia:
				h.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			default:
				h.ContinueRequest(&proto.FetchContinueRequest{})
			}
		})
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

	// Block fonts and media but NOT images (LCP needs images)
	if p.opts.BlockResources {
		router := page.HijackRequests()
		router.MustAdd("*", func(h *rod.Hijack) {
			resType := h.Request.Type()
			switch resType {
			case proto.NetworkResourceTypeFont,
				proto.NetworkResourceTypeMedia:
				h.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			default:
				h.ContinueRequest(&proto.FetchContinueRequest{})
			}
		})
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
