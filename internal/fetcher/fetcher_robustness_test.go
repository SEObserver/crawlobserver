package fetcher

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFetchContentTypeEarlyCheck_Image(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		// Write 1MB of fake image data — should NOT be downloaded
		w.Write(make([]byte, 1024*1024))
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", result.StatusCode)
	}
	if result.ContentType != "image/png" {
		t.Errorf("expected image/png, got %s", result.ContentType)
	}
	// Body should be empty because Content-Type is not HTML
	if len(result.Body) != 0 {
		t.Errorf("expected empty body for non-HTML, got %d bytes", len(result.Body))
	}
	if result.BodySize != 0 {
		t.Errorf("expected body size 0, got %d", result.BodySize)
	}
	if result.IsHTML() {
		t.Error("expected IsHTML() to be false for image/png")
	}
}

func TestFetchContentTypeEarlyCheck_PDF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("fake pdf data"))
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if len(result.Body) != 0 {
		t.Errorf("expected empty body for PDF, got %d bytes", len(result.Body))
	}
}

func TestFetchContentTypeEarlyCheck_XHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xhtml+xml; charset=utf-8")
		fmt.Fprint(w, "<html><body>XHTML</body></html>")
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	// XHTML body SHOULD be downloaded
	if len(result.Body) == 0 {
		t.Error("expected non-empty body for XHTML")
	}
	if !result.IsHTML() {
		t.Error("expected IsHTML() to be true for XHTML")
	}
}

func TestFetchContentTypeEarlyCheck_NoContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No Content-Type header — should still download (could be HTML)
		fmt.Fprint(w, "<html><body>No CT</body></html>")
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	// Body should be downloaded when Content-Type is missing
	if len(result.Body) == 0 {
		t.Error("expected non-empty body when Content-Type is missing")
	}
}

func TestFetchBodyTruncated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, strings.Repeat("x", 2048))
	}))
	defer server.Close()

	// Limit to 1KB
	f := New("TestBot/1.0", 10*time.Second, 1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !result.BodyTruncated {
		t.Error("expected BodyTruncated to be true")
	}
	if result.BodySize != 1024 {
		t.Errorf("expected body size 1024, got %d", result.BodySize)
	}
}

func TestFetchBodyNotTruncated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, strings.Repeat("x", 512))
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.BodyTruncated {
		t.Error("expected BodyTruncated to be false")
	}
	if result.BodySize != 512 {
		t.Errorf("expected body size 512, got %d", result.BodySize)
	}
}

func TestFetchRedirectChainNoConcurrentRace(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/end", http.StatusFound)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>done</html>")
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")

	// Run concurrent fetches — with the old code this would race on f.client.CheckRedirect
	var wg sync.WaitGroup
	errs := make([]string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result := f.Fetch(server.URL+"/redir", 0, "")
			if result.Error != "" {
				errs[idx] = result.Error
				return
			}
			if len(result.RedirectChain) != 1 {
				errs[idx] = fmt.Sprintf("expected 1 redirect hop, got %d", len(result.RedirectChain))
			}
			if !strings.HasSuffix(result.FinalURL, "/end") {
				errs[idx] = fmt.Sprintf("expected final URL /end, got %s", result.FinalURL)
			}
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		if e != "" {
			t.Errorf("goroutine %d: %s", i, e)
		}
	}
}

// TestCategorizeError tests error classification.
func TestCategorizeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "dns not found",
			err:      &net.DNSError{Err: "no such host", Name: "example.com", IsNotFound: true},
			expected: "dns_not_found",
		},
		{
			name:     "dns timeout",
			err:      &net.DNSError{Err: "timeout", Name: "example.com", IsTimeout: true},
			expected: "dns_timeout",
		},
		{
			name:     "dns generic",
			err:      &net.DNSError{Err: "some dns error", Name: "example.com"},
			expected: "dns_not_found",
		},
		{
			name: "connection refused",
			err: &net.OpError{
				Op:  "dial",
				Err: errors.New("connection refused"),
			},
			expected: "connection_refused",
		},
		{
			name:     "tls error",
			err:      &tls.CertificateVerificationError{Err: errors.New("certificate invalid")},
			expected: "tls_error",
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: "timeout",
		},
		{
			name:     "generic error",
			err:      errors.New("something weird happened"),
			expected: "fetch_error: something weird happened",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CategorizeError(tt.err)
			if got != tt.expected {
				t.Errorf("CategorizeError() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCategorizeErrorTimeout(t *testing.T) {
	// Test with a real timeout from http.Client
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	f := New("TestBot/1.0", 100*time.Millisecond, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	if result.Error == "" {
		t.Fatal("expected timeout error")
	}
	// Should be categorized as timeout
	if !strings.Contains(result.Error, "timeout") {
		t.Errorf("expected error to contain 'timeout', got %q", result.Error)
	}
}

func TestIsHTMLContentType(t *testing.T) {
	tests := []struct {
		ct       string
		expected bool
	}{
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"TEXT/HTML", true},
		{"application/xhtml+xml", true},
		{"application/xhtml+xml; charset=utf-8", true},
		{"", true}, // empty means unknown, assume HTML
		{"image/png", false},
		{"application/pdf", false},
		{"application/json", false},
		{"text/plain", false},
		{"video/mp4", false},
	}

	for _, tt := range tests {
		t.Run(tt.ct, func(t *testing.T) {
			got := isHTMLContentType(tt.ct)
			if got != tt.expected {
				t.Errorf("isHTMLContentType(%q) = %v, want %v", tt.ct, got, tt.expected)
			}
		})
	}
}

func TestFetchDepthAndFoundOnPreserved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>ok</html>")
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 3, "https://parent.com/page")

	if result.Depth != 3 {
		t.Errorf("Depth = %d, want 3", result.Depth)
	}
	if result.FoundOn != "https://parent.com/page" {
		t.Errorf("FoundOn = %q, want https://parent.com/page", result.FoundOn)
	}
}

func TestFetchInvalidURL(t *testing.T) {
	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch("://invalid", 0, "")

	if result.Error == "" {
		t.Error("expected error for invalid URL")
	}
}

func TestFetchHeadersCollected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("X-Custom-Header", "test-value")
		w.Header().Set("Content-Length", "0")
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, DialOptions{AllowPrivateIPs: true}, "")
	result := f.Fetch(server.URL, 0, "")

	// Headers should be collected even for non-HTML responses
	if result.Headers == nil {
		t.Fatal("expected headers to be collected")
	}
	if result.Headers["X-Custom-Header"] != "test-value" {
		t.Errorf("X-Custom-Header = %q, want test-value", result.Headers["X-Custom-Header"])
	}
}
