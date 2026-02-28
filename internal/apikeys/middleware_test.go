package apikeys

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsReadOnly(t *testing.T) {
	tests := []struct {
		name     string
		info     AuthInfo
		readOnly bool
	}{
		{"project key", AuthInfo{Method: "apikey", KeyType: "project"}, true},
		{"general key", AuthInfo{Method: "apikey", KeyType: "general"}, false},
		{"basic auth", AuthInfo{Method: "basic"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.IsReadOnly(); got != tt.readOnly {
				t.Fatalf("IsReadOnly() = %v, want %v", got, tt.readOnly)
			}
		})
	}
}

func TestFromContextWithValue(t *testing.T) {
	info := &AuthInfo{Method: "basic"}
	ctx := context.WithValue(context.Background(), contextKey{}, info)
	got := FromContext(ctx)
	if got == nil || got.Method != "basic" {
		t.Fatalf("expected basic auth info, got %v", got)
	}
}

func TestFromContextWithoutValue(t *testing.T) {
	got := FromContext(context.Background())
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestAuthenticateAPIKey(t *testing.T) {
	s := newTestStore(t)
	res, _ := s.CreateAPIKey("test", "general", nil)

	handler := Authenticate(s, "admin", "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := FromContext(r.Context())
		if info == nil {
			t.Fatal("expected auth info in context")
		}
		if info.Method != "apikey" || info.KeyType != "general" {
			t.Fatalf("unexpected: method=%s type=%s", info.Method, info.KeyType)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", res.FullKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAuthenticateBasicAuth(t *testing.T) {
	s := newTestStore(t)

	handler := Authenticate(s, "admin", "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := FromContext(r.Context())
		if info == nil || info.Method != "basic" {
			t.Fatal("expected basic auth info")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAuthenticateNoCredentials(t *testing.T) {
	s := newTestStore(t)

	handler := Authenticate(s, "admin", "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("expected WWW-Authenticate header")
	}
}

func TestAuthenticateInvalidAPIKey(t *testing.T) {
	s := newTestStore(t)

	handler := Authenticate(s, "admin", "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "sk_invalid_key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
