package fetcher

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	client      *http.Client
	userAgent   string
	maxBodySize int64
}

// New creates a new Fetcher.
func New(userAgent string, timeout time.Duration, maxBodySize int64) *Fetcher {
	f := &Fetcher{
		userAgent:   userAgent,
		maxBodySize: maxBodySize,
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout:  15 * time.Second,
		MaxIdleConnsPerHost:    2,
		MaxResponseHeaderBytes: 1 << 20, // 1MB
		IdleConnTimeout:        90 * time.Second,
	}

	f.client = &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			// Track redirect hops via context
			if tracker, ok := req.Context().Value(redirectTrackerKey{}).(*redirectTracker); ok {
				prev := via[len(via)-1]
				if req.Response != nil {
					tracker.mu.Lock()
					tracker.chain = append(tracker.chain, RedirectHop{
						URL:        prev.URL.String(),
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

// Fetch retrieves a URL and returns the result with redirect chain.
func (f *Fetcher) Fetch(targetURL string, depth int, foundOn string) *FetchResult {
	result := &FetchResult{
		URL:     targetURL,
		Depth:   depth,
		FoundOn: foundOn,
	}

	start := time.Now()

	// Create redirect tracker for this request
	tracker := &redirectTracker{}
	ctx := context.WithValue(context.Background(), redirectTrackerKey{}, tracker)

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("creating request: %s", err)
		result.Duration = time.Since(start)
		return result
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := f.client.Do(req)
	if err != nil {
		result.Error = categorizeError(err)
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.FinalURL = resp.Request.URL.String()
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

// categorizeError classifies a fetch error into a category string.
func categorizeError(err error) string {
	if err == nil {
		return ""
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
