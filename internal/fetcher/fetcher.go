package fetcher

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// redirectTrackerKey is used to store redirect tracking data in request context.
type redirectTrackerKey struct{}

// redirectTracker tracks redirect hops for a single request.
type redirectTracker struct {
	mu    sync.Mutex
	chain []RedirectHop
}

// Fetcher performs HTTP requests with redirect chain tracking.
type Fetcher struct {
	client       *http.Client
	userAgent    string
	maxBodySize  int64
	extraHeaders map[string]string
}

// Option configures a Fetcher at construction.
type Option func(*Fetcher)

// WithExtraHeaders sets headers added to every request the Fetcher makes.
// The map is copied, so the caller may reuse it.
func WithExtraHeaders(headers map[string]string) Option {
	return func(f *Fetcher) {
		if len(headers) == 0 {
			return
		}
		f.extraHeaders = make(map[string]string, len(headers))
		for k, v := range headers {
			f.extraHeaders[k] = v
		}
	}
}

// New creates a new Fetcher. When tlsProfile is non-empty, the transport uses
// utls to mimic the chosen browser's TLS fingerprint.
func New(userAgent string, timeout time.Duration, maxBodySize int64, dialOpts DialOptions, tlsProfile TLSProfile, opts ...Option) *Fetcher {
	f := &Fetcher{
		userAgent:   userAgent,
		maxBodySize: maxBodySize,
	}
	for _, opt := range opts {
		opt(f)
	}

	dialFn := SafeDialContextWithOpts(dialOpts)
	transport := &http.Transport{
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  15 * time.Second,
		MaxIdleConnsPerHost:    2,
		MaxResponseHeaderBytes: 1 << 20, // 1MB
		IdleConnTimeout:        90 * time.Second,
		DialContext:            dialFn,
	}

	var rt http.RoundTripper = transport
	if tlsProfile != "" {
		rt = utlsTransport(tlsProfile, dialFn, transport)
	}

	allowPrivate := dialOpts.AllowPrivateIPs
	f.client = &http.Client{
		Timeout:   timeout,
		Transport: rt,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			// net/http copies every header across a redirect except a short
			// list of its own — Authorization, Cookie and the like. A crawl
			// header is often a signature naming the host it was issued for,
			// so carrying it to another host would hand it to whoever the
			// redirect points at. Dropped here, where the new host is known.
			if len(via) > 0 && !strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) {
				for name := range f.extraHeaders {
					req.Header.Del(name)
				}
			}
			// SSRF: block redirects to private IP literals
			if !allowPrivate {
				if ip := net.ParseIP(req.URL.Hostname()); ip != nil && IsPrivateIP(ip) {
					return fmt.Errorf("%w: redirect to %s", ErrPrivateIP, req.URL.Hostname())
				}
			}
			// Track redirect hops via context
			if tracker, ok := req.Context().Value(redirectTrackerKey{}).(*redirectTracker); ok {
				prev := via[len(via)-1]
				if req.Response != nil {
					tracker.mu.Lock()
					tracker.chain = append(tracker.chain, RedirectHop{
						URL:        stripDefaultPort(prev.URL.String()),
						StatusCode: req.Response.StatusCode,
					})
					tracker.mu.Unlock()
				}
			}
			return nil
		},
	}

	return f
}

// Client returns the underlying HTTP client.
func (f *Fetcher) Client() *http.Client {
	return f.client
}

// SetCookieJar sets the cookie jar on the underlying HTTP client.
// This allows cookies (e.g. cf_clearance) to persist across requests.
func (f *Fetcher) SetCookieJar(jar http.CookieJar) {
	f.client.Jar = jar
}

// UserAgent returns the User-Agent string used by this Fetcher.
func (f *Fetcher) UserAgent() string {
	return f.userAgent
}

// Fetch retrieves a URL and returns the result with redirect chain.
func (f *Fetcher) Fetch(targetURL string, depth int, foundOn string) *FetchResult {
	return f.FetchWithContext(context.Background(), targetURL, depth, foundOn)
}

// FetchWithContext retrieves a URL using the provided context for cancellation.
func (f *Fetcher) FetchWithContext(ctx context.Context, targetURL string, depth int, foundOn string) *FetchResult {
	result := &FetchResult{
		URL:     targetURL,
		Depth:   depth,
		FoundOn: foundOn,
	}

	start := time.Now()

	// Create redirect tracker for this request
	tracker := &redirectTracker{}
	ctx = context.WithValue(ctx, redirectTrackerKey{}, tracker)

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("creating request: %s", err)
		result.Duration = time.Since(start)
		return result
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	ApplyExtraHeaders(req, f.extraHeaders)

	resp, err := f.client.Do(req)
	if err != nil {
		result.Error = CategorizeError(err)
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.FinalURL = stripDefaultPort(resp.Request.URL.String())
	result.RedirectChain = tracker.chain
	result.ContentType = resp.Header.Get("Content-Type")

	// Copy response headers
	result.Headers = make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		result.Headers[k] = strings.Join(v, ", ")
	}

	// Content-Type early check: skip body for non-HTML responses
	if !isHTMLContentType(result.ContentType) {
		result.Duration = time.Since(start)
		return result
	}

	// Read body with size limit
	limitedReader := io.LimitReader(resp.Body, f.maxBodySize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		result.Error = fmt.Sprintf("reading body: %s", err)
		result.Duration = time.Since(start)
		return result
	}

	if int64(len(body)) > f.maxBodySize {
		body = body[:f.maxBodySize]
		result.BodyTruncated = true
	}

	result.Body = body
	result.BodySize = int64(len(body))
	result.Duration = time.Since(start)

	return result
}

// isHTMLContentType checks if a Content-Type header indicates HTML content.
func isHTMLContentType(ct string) bool {
	lower := strings.ToLower(ct)
	return ct == "" || strings.Contains(lower, "text/html") || strings.Contains(lower, "application/xhtml+xml")
}

// IsHTML checks if the FetchResult contains HTML content.
func (r *FetchResult) IsHTML() bool {
	ct := strings.ToLower(r.ContentType)
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml+xml")
}

// CategorizeError classifies a fetch error into a category string.
func CategorizeError(err error) string {
	if err == nil {
		return ""
	}

	// Check for SSRF block
	if errors.Is(err, ErrPrivateIP) {
		return "ssrf_blocked"
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return "dns_not_found"
		}
		if dnsErr.IsTimeout {
			return "dns_timeout"
		}
		return "dns_not_found"
	}

	// Check for connection errors
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			return "connection_refused"
		}
	}

	// Check for TLS errors
	var tlsErr *tls.CertificateVerificationError
	if errors.As(err, &tlsErr) {
		return "tls_error"
	}

	// Check for timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	// net.Error timeout check
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}

	return fmt.Sprintf("fetch_error: %s", err)
}

// stripDefaultPort removes :443 from HTTPS URLs and :80 from HTTP URLs.
func stripDefaultPort(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		parsed.Host = parsed.Hostname()
		return parsed.String()
	}
	return rawURL
}
