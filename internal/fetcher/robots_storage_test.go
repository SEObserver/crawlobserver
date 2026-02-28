package fetcher

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRobotsCacheEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "User-agent: *\nDisallow: /private/\n")
	}))
	defer server.Close()

	rc := NewRobotsCache("TestBot", 5*time.Second, true, "")

	// Trigger fetch by calling IsAllowed
	rc.IsAllowed(server.URL + "/page")

	entries := rc.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	for host, entry := range entries {
		if host != server.URL {
			t.Errorf("expected host %q, got %q", server.URL, host)
		}
		if entry.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", entry.StatusCode)
		}
		if entry.Content == "" {
			t.Error("expected non-empty content")
		}
		if entry.FetchedAt.IsZero() {
			t.Error("expected non-zero FetchedAt")
		}
		// Verify content is the actual robots.txt
		if entry.Content != "User-agent: *\nDisallow: /private/\n" {
			t.Errorf("unexpected content: %q", entry.Content)
		}
	}
}

func TestRobotsCacheEntriesMultipleHosts(t *testing.T) {
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "User-agent: *\nDisallow: /a/\n")
	}))
	defer s1.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "User-agent: *\nDisallow: /b/\n")
	}))
	defer s2.Close()

	rc := NewRobotsCache("TestBot", 5*time.Second, true, "")

	rc.IsAllowed(s1.URL + "/page")
	rc.IsAllowed(s2.URL + "/page")

	entries := rc.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	e1 := entries[s1.URL]
	if e1 == nil {
		t.Fatal("missing entry for s1")
	}
	if e1.Content != "User-agent: *\nDisallow: /a/\n" {
		t.Errorf("s1 content = %q", e1.Content)
	}

	e2 := entries[s2.URL]
	if e2 == nil {
		t.Fatal("missing entry for s2")
	}
	if e2.Content != "User-agent: *\nDisallow: /b/\n" {
		t.Errorf("s2 content = %q", e2.Content)
	}
}

func TestRobotsCacheEntriesEmpty(t *testing.T) {
	rc := NewRobotsCache("TestBot", 5*time.Second, true, "")
	entries := rc.Entries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestRobotsCacheEntries404Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	}))
	defer server.Close()

	rc := NewRobotsCache("TestBot", 5*time.Second, true, "")
	rc.IsAllowed(server.URL + "/page")

	entries := rc.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	for _, entry := range entries {
		if entry.StatusCode != 404 {
			t.Errorf("expected status 404, got %d", entry.StatusCode)
		}
		// Content should be empty on non-200
		if entry.Content != "" {
			t.Errorf("expected empty content for 404, got %q", entry.Content)
		}
	}
}

func TestRobotsCacheEntriesIsCopy(t *testing.T) {
	rc := NewRobotsCache("TestBot", 5*time.Second, true, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "User-agent: *\nAllow: /\n")
	}))
	defer server.Close()

	rc.IsAllowed(server.URL + "/page")

	entries1 := rc.Entries()
	entries2 := rc.Entries()

	// Modifying entries1 should not affect entries2
	for k := range entries1 {
		delete(entries1, k)
	}
	if len(entries2) != 1 {
		t.Error("Entries() should return independent copies")
	}
}

func TestRobotsCacheIsAllowedWithContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `User-agent: TestBot
Disallow: /admin/
Disallow: /api/

User-agent: *
Disallow: /secret/
`)
	}))
	defer server.Close()

	rc := NewRobotsCache("TestBot", 5*time.Second, true, "")

	// TestBot-specific rules
	if rc.IsAllowed(server.URL + "/admin/panel") {
		t.Error("/admin/panel should be disallowed for TestBot")
	}
	if rc.IsAllowed(server.URL + "/api/data") {
		t.Error("/api/data should be disallowed for TestBot")
	}
	if !rc.IsAllowed(server.URL + "/public/page") {
		t.Error("/public/page should be allowed for TestBot")
	}

	// Verify content is stored correctly
	entries := rc.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	for _, entry := range entries {
		if entry.StatusCode != 200 {
			t.Errorf("status = %d, want 200", entry.StatusCode)
		}
		if !contains(entry.Content, "Disallow: /admin/") {
			t.Error("content should contain admin disallow rule")
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
