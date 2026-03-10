package cfsolve

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

// SolveResult holds the outcome of a Cloudflare challenge solve attempt.
type SolveResult struct {
	Solved  bool
	Cookies []*http.Cookie
	Err     error
}

// Solver resolves Cloudflare challenges via headless Chrome with stealth.
// It deduplicates concurrent solves per host and shares cookies via a jar.
type Solver struct {
	userAgent    string
	jar          *cookiejar.Jar
	solveTimeout time.Duration

	mu      sync.Mutex
	browser *rod.Browser
	solving map[string]chan struct{} // host → channel closed when solve completes
}

// New creates a Solver. The jar must be the same one used by the Fetcher.
func New(userAgent string, jar *cookiejar.Jar, solveTimeout time.Duration) *Solver {
	if solveTimeout <= 0 {
		solveTimeout = 30 * time.Second
	}
	return &Solver{
		userAgent:    userAgent,
		jar:          jar,
		solveTimeout: solveTimeout,
		solving:      make(map[string]chan struct{}),
	}
}

// Solve navigates to the challenge URL in headless Chrome, waits for
// cf_clearance to appear, and injects the cookies into the shared jar.
// Concurrent calls for the same host block on the first solve.
func (s *Solver) Solve(ctx context.Context, challengeURL string) *SolveResult {
	u, err := url.Parse(challengeURL)
	if err != nil {
		return &SolveResult{Err: fmt.Errorf("parse URL: %w", err)}
	}
	host := u.Host

	// Dedup: if a solve is already running for this host, wait for it.
	s.mu.Lock()
	if ch, ok := s.solving[host]; ok {
		s.mu.Unlock()
		select {
		case <-ch:
			// Previous solve completed — check if we got cookies.
			cookies := s.jar.Cookies(u)
			for _, c := range cookies {
				if c.Name == "cf_clearance" {
					return &SolveResult{Solved: true, Cookies: cookies}
				}
			}
			return &SolveResult{Err: fmt.Errorf("concurrent solve for %s did not yield cf_clearance", host)}
		case <-ctx.Done():
			return &SolveResult{Err: ctx.Err()}
		}
	}
	ch := make(chan struct{})
	s.solving[host] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.solving, host)
		close(ch) // unblock waiters
		s.mu.Unlock()
	}()

	return s.doSolve(ctx, challengeURL, u)
}

// doSolve performs the actual Chrome navigation and cookie extraction.
func (s *Solver) doSolve(ctx context.Context, challengeURL string, u *url.URL) *SolveResult {
	if err := s.ensureBrowser(); err != nil {
		return &SolveResult{Err: fmt.Errorf("launch browser: %w", err)}
	}

	solveCtx, cancel := context.WithTimeout(ctx, s.solveTimeout)
	defer cancel()

	// Create page manually with about:blank (stealth.Page uses empty URL which
	// can cause "Session with given id not found" in headless Chrome).
	page, err := s.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return &SolveResult{Err: fmt.Errorf("create page: %w", err)}
	}
	defer page.Close()

	// Inject stealth anti-detection scripts before any navigation
	if _, err := page.EvalOnNewDocument(stealth.JS); err != nil {
		return &SolveResult{Err: fmt.Errorf("inject stealth JS: %w", err)}
	}

	// Match the crawler's User-Agent
	if s.userAgent != "" {
		_ = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: s.userAgent,
		})
	}

	page = page.Context(solveCtx)

	if err := page.Navigate(challengeURL); err != nil {
		return &SolveResult{Err: fmt.Errorf("navigate: %w", err)}
	}

	if err := page.WaitLoad(); err != nil {
		return &SolveResult{Err: fmt.Errorf("wait load: %w", err)}
	}

	applog.Infof("cfsolve", "Page loaded for %s, waiting for challenge resolution...", u.Host)

	// Poll for cf_clearance cookie, attempting to click Turnstile checkbox
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	turnstileClicked := false

	for {
		select {
		case <-solveCtx.Done():
			// Log page title for debugging
			title, _ := page.Eval(`() => document.title`)
			titleStr := ""
			if title != nil {
				titleStr = title.Value.Str()
			}
			return &SolveResult{Err: fmt.Errorf("solve timeout for %s (page title: %q)", u.Host, titleStr)}
		case <-ticker.C:
			// Check for cf_clearance cookie
			cookies, err := page.Cookies([]string{challengeURL})
			if err != nil {
				continue
			}
			for _, c := range cookies {
				if c.Name == "cf_clearance" {
					httpCookies := rodCookiesToHTTP(cookies)
					s.jar.SetCookies(u, httpCookies)
					applog.Infof("cfsolve", "CF challenge solved for %s, got %d cookies", u.Host, len(httpCookies))
					return &SolveResult{Solved: true, Cookies: httpCookies}
				}
			}

			// Try to click Turnstile checkbox if not already done
			if !turnstileClicked {
				if clicked := s.tryClickTurnstile(page); clicked {
					turnstileClicked = true
					applog.Infof("cfsolve", "Clicked Turnstile checkbox for %s", u.Host)
				}
			}
		}
	}
}

// tryClickTurnstile attempts to find and click the Cloudflare Turnstile checkbox.
func (s *Solver) tryClickTurnstile(page *rod.Page) bool {
	// Turnstile renders inside an iframe under #turnstile-wrapper or similar
	// Try to find the iframe containing the checkbox
	iframes, err := page.Elements("iframe[src*='challenges.cloudflare.com']")
	if err != nil || len(iframes) == 0 {
		// Also try the older cf-challenge selectors
		iframes, err = page.Elements("iframe[src*='cdn-cgi/challenge-platform']")
		if err != nil || len(iframes) == 0 {
			return false
		}
	}

	for _, iframe := range iframes {
		frame, err := iframe.Frame()
		if err != nil {
			continue
		}
		// Look for the checkbox input inside the iframe
		checkbox, err := frame.Element("input[type='checkbox']")
		if err != nil {
			// Try clicking the body of the iframe — some Turnstile variants
			// don't have a visible checkbox but clicking triggers verification
			box, err := iframe.Shape()
			if err != nil || box == nil || len(box.Quads) == 0 {
				continue
			}
			center := box.OnePointInside()
			_ = page.Mouse.MoveTo(*center)
			_ = page.Mouse.Click(proto.InputMouseButtonLeft, 1)
			return true
		}
		if err := checkbox.Click(proto.InputMouseButtonLeft, 1); err == nil {
			return true
		}
	}
	return false
}

// ensureBrowser lazily starts the headless Chrome browser.
func (s *Solver) ensureBrowser() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browser != nil {
		return nil
	}

	// Use the "new" headless mode (Chrome 109+) which is less detectable
	l := launcher.New().
		Headless(false). // disable default --headless
		Set(flags.Headless, "new").
		Set("disable-blink-features", "AutomationControlled")
	controlURL, err := l.Launch()
	if err != nil {
		return err
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return err
	}
	s.browser = browser
	return nil
}

// Close shuts down the browser if it was started.
func (s *Solver) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browser != nil {
		s.browser.Close()
		s.browser = nil
	}
}

// rodCookiesToHTTP converts Rod cookies to standard http.Cookie values.
func rodCookiesToHTTP(cookies []*proto.NetworkCookie) []*http.Cookie {
	out := make([]*http.Cookie, 0, len(cookies))
	for _, c := range cookies {
		hc := &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HttpOnly: c.HTTPOnly,
		}
		if c.Expires > 0 {
			hc.Expires = c.Expires.Time()
		}
		out = append(out, hc)
	}
	return out
}
