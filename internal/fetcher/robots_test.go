package fetcher

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRobotsCacheAllowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `
User-agent: CrawlObserver
Disallow: /private/
Disallow: /admin/
Crawl-delay: 2

User-agent: *
Disallow: /secret/
`)
	}))
	defer server.Close()

	rc := NewRobotsCache("CrawlObserver", 5*time.Second, true, "")

	tests := []struct {
		path    string
		allowed bool
	}{
		{"/public/page", true},
		{"/private/data", false},
		{"/admin/panel", false},
		{"/", true},
		{"/about", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			url := server.URL + tt.path
			got := rc.IsAllowed(url)
			if got != tt.allowed {
				t.Errorf("IsAllowed(%s) = %v, want %v", tt.path, got, tt.allowed)
			}
		})
	}
}

func TestRobotsCacheCrawlDelay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `
User-agent: CrawlObserver
Crawl-delay: 3
Disallow: /private/
`)
	}))
	defer server.Close()

	rc := NewRobotsCache("CrawlObserver", 5*time.Second, true, "")

	delay := rc.CrawlDelay(server.URL + "/page")
	if delay != 3*time.Second {
		t.Errorf("CrawlDelay() = %v, want 3s", delay)
	}
}

func TestRobotsCacheNoRobotsTxt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	rc := NewRobotsCache("CrawlObserver", 5*time.Second, true, "")

	// Should allow everything when robots.txt returns 404
	if !rc.IsAllowed(server.URL + "/anything") {
		t.Error("expected all URLs to be allowed when robots.txt is missing")
	}
}
