package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProjectHeadersAccessAndValidation(t *testing.T) {
	_, handler, store := newTestServer(t)
	t.Cleanup(func() { store.Close() })
	project, err := store.CreateProject("protected")
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.CreateAPIKey("reader", "project", &project.ID)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/projects/" + project.ID + "/crawl-headers"
	request := func(method, path, body, auth string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		switch auth {
		case "admin":
			authRequest(req)
		case "project":
			req.Header.Set("X-API-Key", key.FullKey)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	const payload = `{"headers":{"Authorization":"Bearer header-secret","Signature":"opaque-signature"}}`
	if rec := request("PUT", endpoint, payload, "admin"); rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request("GET", endpoint, "", "admin"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "header-secret") {
		t.Fatalf("admin read: %d %s", rec.Code, rec.Body.String())
	}
	for _, method := range []string{"GET", "PUT"} {
		for _, auth := range []string{"", "project"} {
			rec := request(method, endpoint, payload, auth)
			want := http.StatusUnauthorized
			if auth == "project" {
				want = http.StatusForbidden
			}
			if rec.Code != want || strings.Contains(rec.Body.String(), "header-secret") {
				t.Errorf("%s auth %q: %d %s", method, auth, rec.Code, rec.Body.String())
			}
		}
	}
	for _, path := range []string{"/api/projects", "/api/projects?limit=10"} {
		rec := request("GET", path, "", "project")
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "header-secret") {
			t.Errorf("listing: %d %s", rec.Code, rec.Body.String())
		}
	}
	if rec := request("PUT", endpoint, `{"headers":{"Signature":"bad\r\nInjected: yes"}}`, "admin"); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid headers accepted: %d", rec.Code)
	}
	if rec := request("GET", endpoint, "", "admin"); !strings.Contains(rec.Body.String(), "header-secret") {
		t.Fatal("rejected update changed stored headers")
	}
	if rec := request("PUT", endpoint, `{"headers":{}}`, "admin"); rec.Code != http.StatusOK {
		t.Fatalf("clear: %d", rec.Code)
	}
	if rec := request("GET", endpoint, "", "admin"); strings.Contains(rec.Body.String(), "header-secret") {
		t.Fatal("cleared headers still returned")
	}
}
