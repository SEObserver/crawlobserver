package cfsolve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNullResolver(t *testing.T) {
	r := &NullResolver{}
	defer r.Close()

	res, err := r.Solve(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Solved {
		t.Fatal("NullResolver should return Solved=false")
	}
	if len(res.Cookies) != 0 {
		t.Fatal("NullResolver should return no cookies")
	}
}

func TestAPIResolver_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		var req apiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.URL != "https://example.com/challenge" {
			t.Errorf("unexpected URL: %s", req.URL)
		}

		resp := apiResponse{
			Solved: true,
			Cookies: []apiCookie{
				{Name: "cf_clearance", Value: "abc123", Domain: ".example.com", Path: "/", Secure: true, HTTPOnly: true},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	r := NewAPIResolver(srv.URL, "test-key", 5*time.Second)
	res, err := r.Solve(context.Background(), "https://example.com/challenge")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Solved {
		t.Fatal("expected Solved=true")
	}
	if len(res.Cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(res.Cookies))
	}
	if res.Cookies[0].Name != "cf_clearance" || res.Cookies[0].Value != "abc123" {
		t.Errorf("unexpected cookie: %+v", res.Cookies[0])
	}
}

func TestAPIResolver_Unsolved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(apiResponse{Solved: false})
	}))
	defer srv.Close()

	r := NewAPIResolver(srv.URL, "", 5*time.Second)
	res, err := r.Solve(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Solved {
		t.Fatal("expected Solved=false")
	}
}

func TestAPIResolver_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewAPIResolver(srv.URL, "", 5*time.Second)
	_, err := r.Solve(context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestAPIResolver_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	r := NewAPIResolver(srv.URL, "", 10*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := r.Solve(ctx, "https://example.com")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
