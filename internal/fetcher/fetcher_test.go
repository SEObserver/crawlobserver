package fetcher

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchBasic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != "TestBot/1.0" {
			t.Errorf("unexpected User-Agent: %s", ua)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Custom", "test-value")
		fmt.Fprint(w, "<html><head><title>Test</title></head><body>Hello</body></html>")
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, true)
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", result.StatusCode)
	}
	if !strings.Contains(result.ContentType, "text/html") {
		t.Errorf("expected text/html content type, got %s", result.ContentType)
	}
	if !result.IsHTML() {
		t.Error("expected IsHTML() to be true")
	}
	if result.Headers["X-Custom"] != "test-value" {
		t.Errorf("expected X-Custom header, got %s", result.Headers["X-Custom"])
	}
	if !strings.Contains(string(result.Body), "<title>Test</title>") {
		t.Error("expected body to contain title")
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestFetchRedirectChain(t *testing.T) {
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/middle", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/middle", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/end", http.StatusFound)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body>Final</body></html>")
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, true)
	result := f.Fetch(server.URL+"/start", 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", result.StatusCode)
	}
	if !strings.HasSuffix(result.FinalURL, "/end") {
		t.Errorf("expected final URL to end with /end, got %s", result.FinalURL)
	}
	if len(result.RedirectChain) != 2 {
		t.Errorf("expected 2 redirect hops, got %d", len(result.RedirectChain))
	}
}

func TestFetchBodySizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Write 2KB of data
		fmt.Fprint(w, strings.Repeat("x", 2048))
	}))
	defer server.Close()

	// Limit to 1KB
	f := New("TestBot/1.0", 10*time.Second, 1024, true)
	result := f.Fetch(server.URL, 0, "")

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.BodySize != 1024 {
		t.Errorf("expected body size 1024, got %d", result.BodySize)
	}
}

func TestFetchTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		fmt.Fprint(w, "slow")
	}))
	defer server.Close()

	f := New("TestBot/1.0", 100*time.Millisecond, 10*1024*1024, true)
	result := f.Fetch(server.URL, 0, "")

	if result.Error == "" {
		t.Error("expected timeout error")
	}
}

func TestFetch404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	}))
	defer server.Close()

	f := New("TestBot/1.0", 10*time.Second, 10*1024*1024, true)
	result := f.Fetch(server.URL+"/missing", 0, "")

	if result.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", result.StatusCode)
	}
}
