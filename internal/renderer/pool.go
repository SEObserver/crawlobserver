package renderer

import (
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// PoolOptions configures the Chrome page pool.
type PoolOptions struct {
	MaxPages       int
	PageTimeout    time.Duration
	UserAgent      string
	BlockResources bool
	Headless       bool
}

func DefaultPoolOptions() PoolOptions {
	return PoolOptions{
		MaxPages:       4,
		PageTimeout:    15 * time.Second,
		UserAgent:      "",
		BlockResources: true,
		Headless:       true,
	}
}

// Pool manages a single Chrome browser with a pool of reusable pages.
type Pool struct {
	browser *rod.Browser
	pages   chan *rod.Page
	opts    PoolOptions
	mu      sync.Mutex
	closed  bool
}

// NewPool launches a headless Chrome and creates a page pool.
func NewPool(opts PoolOptions) (*Pool, error) {
	if opts.MaxPages <= 0 {
		opts.MaxPages = 4
	}
	if opts.PageTimeout <= 0 {
		opts.PageTimeout = 15 * time.Second
	}

	l := launcher.New().Headless(opts.Headless)
	controlURL, err := l.Launch()
	if err != nil {
		return nil, err
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, err
	}

	p := &Pool{
		browser: browser,
		pages:   make(chan *rod.Page, opts.MaxPages),
		opts:    opts,
	}

	return p, nil
}

// Acquire returns a page from the pool or creates a new one.
func (p *Pool) Acquire() (*rod.Page, error) {
	select {
	case page := <-p.pages:
		return page, nil
	default:
	}

	page, err := p.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, err
	}

	if p.opts.UserAgent != "" {
		err = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: p.opts.UserAgent,
		})
		if err != nil {
			page.Close()
			return nil, err
		}
	}

	return page, nil
}

// Release returns a page to the pool or closes it if the pool is full.
func (p *Pool) Release(page *rod.Page) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		page.Close()
		return
	}
	p.mu.Unlock()

	// Navigate to blank to free memory before reuse
	_ = page.Navigate("about:blank")

	select {
	case p.pages <- page:
	default:
		page.Close()
	}
}

// Close shuts down the browser and all pages.
func (p *Pool) Close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()

	close(p.pages)
	for page := range p.pages {
		page.Close()
	}
	p.browser.Close()
}
