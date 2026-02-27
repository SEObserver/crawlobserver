package fetcher

import (
	"time"
)

// RedirectHop represents a single hop in a redirect chain.
type RedirectHop struct {
	URL        string
	StatusCode int
}

// FetchResult contains the result of fetching a URL.
type FetchResult struct {
	URL           string
	FinalURL      string
	StatusCode    int
	ContentType   string
	Headers       map[string]string
	Body          []byte
	BodySize      int64
	BodyTruncated bool
	RedirectChain []RedirectHop
	Duration      time.Duration
	Error         string
	Depth         int
	FoundOn       string
}
