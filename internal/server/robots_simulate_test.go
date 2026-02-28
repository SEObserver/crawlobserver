package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SEObserver/crawlobserver/internal/storage"
)

type simulateResponse struct {
	TotalURLs        int `json:"total_urls"`
	CurrentlyAllowed int `json:"currently_allowed"`
	CurrentlyBlocked int `json:"currently_blocked"`
	NewlyBlocked     []struct {
		URL string `json:"url"`
	} `json:"newly_blocked"`
	NewlyAllowed []struct {
		URL string `json:"url"`
	} `json:"newly_allowed"`
	Summary struct {
		WillBlock int `json:"will_block"`
		WillAllow int `json:"will_allow"`
	} `json:"summary"`
}

func doSimulate(t *testing.T, handler http.Handler, body interface{}) (*httptest.ResponseRecorder, simulateResponse) {
	t.Helper()
	req := authRequest(httptest.NewRequest("POST", "/api/sessions/sess-1/robots-simulate", jsonBody(t, body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp simulateResponse
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return rec, resp
}

func TestRobotsSimulate_DisallowAll(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)
	ms.robotsContent = &storage.RobotsRow{
		Content: "User-agent: *\nAllow: /\n",
	}
	ms.urlsByHost = map[string][]string{
		"https://example.com": {
			"https://example.com/page1",
			"https://example.com/page2",
			"https://example.com/blog/post",
		},
	}

	rec, resp := doSimulate(t, handler, map[string]string{
		"host":        "https://example.com",
		"user_agent":  "*",
		"new_content": "User-agent: *\nDisallow: /\n",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if resp.TotalURLs != 3 {
		t.Errorf("total_urls: got %d, want 3", resp.TotalURLs)
	}
	if resp.CurrentlyAllowed != 3 {
		t.Errorf("currently_allowed: got %d, want 3", resp.CurrentlyAllowed)
	}
	if resp.CurrentlyBlocked != 0 {
		t.Errorf("currently_blocked: got %d, want 0", resp.CurrentlyBlocked)
	}
	if resp.Summary.WillBlock != 3 {
		t.Errorf("will_block: got %d, want 3", resp.Summary.WillBlock)
	}
	if len(resp.NewlyBlocked) != 3 {
		t.Errorf("newly_blocked count: got %d, want 3", len(resp.NewlyBlocked))
	}
}

func TestRobotsSimulate_UnblockPath(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)
	ms.robotsContent = &storage.RobotsRow{
		Content: "User-agent: *\nDisallow: /private/\nDisallow: /admin/\n",
	}
	ms.urlsByHost = map[string][]string{
		"https://example.com": {
			"https://example.com/public",
			"https://example.com/private/secret",
			"https://example.com/private/data",
			"https://example.com/admin/dashboard",
		},
	}

	rec, resp := doSimulate(t, handler, map[string]string{
		"host":        "https://example.com",
		"user_agent":  "*",
		"new_content": "User-agent: *\nDisallow: /admin/\n",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if resp.TotalURLs != 4 {
		t.Errorf("total_urls: got %d, want 4", resp.TotalURLs)
	}
	if resp.CurrentlyAllowed != 1 {
		t.Errorf("currently_allowed: got %d, want 1", resp.CurrentlyAllowed)
	}
	if resp.CurrentlyBlocked != 3 {
		t.Errorf("currently_blocked: got %d, want 3", resp.CurrentlyBlocked)
	}
	if resp.Summary.WillAllow != 2 {
		t.Errorf("will_allow: got %d, want 2", resp.Summary.WillAllow)
	}
	if resp.Summary.WillBlock != 0 {
		t.Errorf("will_block: got %d, want 0", resp.Summary.WillBlock)
	}
}

func TestRobotsSimulate_HostWithScheme(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)
	ms.robotsContent = &storage.RobotsRow{
		Content: "User-agent: *\nAllow: /\n",
	}
	ms.urlsByHost = map[string][]string{
		"https://www.example.com": {
			"https://www.example.com/page1",
			"https://www.example.com/page2",
		},
	}

	rec, resp := doSimulate(t, handler, map[string]string{
		"host":        "https://www.example.com",
		"user_agent":  "*",
		"new_content": "User-agent: *\nDisallow: /\n",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if resp.TotalURLs != 2 {
		t.Errorf("total_urls: got %d, want 2 (host with scheme not handled correctly)", resp.TotalURLs)
	}
	if resp.Summary.WillBlock != 2 {
		t.Errorf("will_block: got %d, want 2", resp.Summary.WillBlock)
	}
}

func TestRobotsSimulate_NoChanges(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)

	content := "User-agent: *\nDisallow: /private/\n"
	ms.robotsContent = &storage.RobotsRow{Content: content}
	ms.urlsByHost = map[string][]string{
		"https://example.com": {
			"https://example.com/public",
			"https://example.com/private/secret",
		},
	}

	rec, resp := doSimulate(t, handler, map[string]string{
		"host":        "https://example.com",
		"user_agent":  "*",
		"new_content": content,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if resp.Summary.WillBlock != 0 {
		t.Errorf("will_block: got %d, want 0", resp.Summary.WillBlock)
	}
	if resp.Summary.WillAllow != 0 {
		t.Errorf("will_allow: got %d, want 0", resp.Summary.WillAllow)
	}
}

func TestRobotsSimulate_HostWithoutScheme(t *testing.T) {
	srv, handler, _ := newTestServer(t)
	ms := srv.store.(*mockStore)
	ms.robotsContent = &storage.RobotsRow{
		Content: "User-agent: *\nAllow: /\n",
	}
	ms.urlsByHost = map[string][]string{
		"https://example.com": {
			"https://example.com/page1",
		},
	}

	rec, resp := doSimulate(t, handler, map[string]string{
		"host":        "example.com",
		"user_agent":  "*",
		"new_content": "User-agent: *\nDisallow: /\n",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if resp.TotalURLs != 1 {
		t.Errorf("total_urls: got %d, want 1 (bare host should be prefixed with https://)", resp.TotalURLs)
	}
}

func TestRobotsSimulate_MissingFields(t *testing.T) {
	_, handler, _ := newTestServer(t)

	req := authRequest(httptest.NewRequest("POST", "/api/sessions/sess-1/robots-simulate",
		jsonBody(t, map[string]string{"host": "example.com"})))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing new_content, got %d", rec.Code)
	}
}
