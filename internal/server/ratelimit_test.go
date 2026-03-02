package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SEObserver/crawlobserver/internal/config"
)

func newTestRateLimiter(rps float64, burst int, authRPM int) *rateLimitMiddleware {
	return newRateLimitMiddleware(config.RateLimitConfig{
		Enabled:            true,
		RequestsPerSecond:  rps,
		Burst:              burst,
		AuthRequestsPerMin: authRPM,
	})
}

func TestRateLimitAllowsBelowLimit(t *testing.T) {
	rl := newTestRateLimiter(100, 10, 60)
	defer rl.Stop()

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rl.Handler(ok)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
}

func TestRateLimitBlocksAboveLimit(t *testing.T) {
	rl := newTestRateLimiter(1, 1, 60)
	defer rl.Stop()

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rl.Handler(ok)

	// First request uses the burst
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rec.Code)
	}

	// Second request immediately should be blocked (burst=1, 1 rps)
	req2 := httptest.NewRequest("GET", "/api/test", nil)
	req2.RemoteAddr = "1.2.3.4:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rec2.Code)
	}
}

func TestRateLimitRetryAfterHeader(t *testing.T) {
	rl := newTestRateLimiter(1, 1, 60)
	defer rl.Stop()

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust burst
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "5.5.5.5:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Trigger 429
	req2 := httptest.NewRequest("GET", "/api/test", nil)
	req2.RemoteAddr = "5.5.5.5:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429")
	}
}

func TestRateLimitAuthEndpointStricter(t *testing.T) {
	// High general limit, very low auth limit
	rl := newTestRateLimiter(100, 50, 1)
	defer rl.Stop()

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First auth request ok (burst allows 1)
	req := httptest.NewRequest("POST", "/api/api-keys", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first auth request: expected 200, got %d", rec.Code)
	}

	// Second auth request should be blocked by auth limiter
	req2 := httptest.NewRequest("POST", "/api/api-keys", nil)
	req2.RemoteAddr = "9.9.9.9:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second auth request: expected 429, got %d", rec2.Code)
	}
}

func TestRateLimitPerIPIsolation(t *testing.T) {
	rl := newTestRateLimiter(1, 1, 60)
	defer rl.Stop()

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IP 1 exhausts its burst
	req1 := httptest.NewRequest("GET", "/api/test", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// IP 2 should still work
	req2 := httptest.NewRequest("GET", "/api/test", nil)
	req2.RemoteAddr = "10.0.0.2:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("different IP should not be rate limited, got %d", rec2.Code)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name     string
		remote   string
		xff      string
		xri      string
		expected string
	}{
		{"remote addr", "1.2.3.4:5678", "", "", "1.2.3.4"},
		{"xff ignored", "1.2.3.4:5678", "9.8.7.6, 5.4.3.2", "", "1.2.3.4"},
		{"xri ignored", "1.2.3.4:5678", "", "9.9.9.9", "1.2.3.4"},
		{"xff and xri ignored", "1.2.3.4:5678", "8.8.8.8", "9.9.9.9", "1.2.3.4"},
		{"no port", "1.2.3.4", "", "", "1.2.3.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remote
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}
			got := clientIP(req)
			if got != tt.expected {
				t.Fatalf("clientIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestIsAuthEndpoint(t *testing.T) {
	tests := []struct {
		method   string
		path     string
		expected bool
	}{
		{"POST", "/api/api-keys", true},
		{"DELETE", "/api/api-keys/abc", true},
		{"GET", "/api/api-keys", false},
		{"POST", "/api/projects", true},
		{"PUT", "/api/projects/abc", true},
		{"DELETE", "/api/projects/abc", true},
		{"GET", "/api/projects", false},
		{"POST", "/api/crawl", false},
		{"GET", "/api/sessions", false},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			got := isAuthEndpoint(tt.method, tt.path)
			if got != tt.expected {
				t.Fatalf("isAuthEndpoint(%s, %s) = %v, want %v", tt.method, tt.path, got, tt.expected)
			}
		})
	}
}
