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
	RenderedHTML    string
	RenderDuration time.Duration
	JSErrors       []string
	Error          error
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
