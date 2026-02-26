package fetcher

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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

	f.client = &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
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

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("creating request: %s", err)
		result.Duration = time.Since(start)
		return result
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	// Track redirects
	var chain []RedirectHop
	f.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		// req.Response is the redirect response that caused this new request
		prev := via[len(via)-1]
		if req.Response != nil {
			chain = append(chain, RedirectHop{
				URL:        prev.URL.String(),
				StatusCode: req.Response.StatusCode,
			})
		}
		return nil
	}

	resp, err := f.client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("fetching: %s", err)
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.FinalURL = resp.Request.URL.String()
	result.RedirectChain = chain
	result.ContentType = resp.Header.Get("Content-Type")

	// Copy response headers
	result.Headers = make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		result.Headers[k] = strings.Join(v, ", ")
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
	}

	result.Body = body
	result.BodySize = int64(len(body))
	result.Duration = time.Since(start)

	return result
}

// IsHTML checks if the FetchResult contains HTML content.
func (r *FetchResult) IsHTML() bool {
	ct := strings.ToLower(r.ContentType)
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml+xml")
}
