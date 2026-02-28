package server

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SEObserver/crawlobserver/internal/config"
	"golang.org/x/time/rate"
)

type ipLimiter struct {
	general  *rate.Limiter
	auth     *rate.Limiter
	lastSeen time.Time
}

type rateLimitMiddleware struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiter
	cfg      config.RateLimitConfig
	stopCh   chan struct{}
}

func newRateLimitMiddleware(cfg config.RateLimitConfig) *rateLimitMiddleware {
	rl := &rateLimitMiddleware{
		limiters: make(map[string]*ipLimiter),
		cfg:      cfg,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

func (rl *rateLimitMiddleware) getLimiter(ip string) *ipLimiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	l, ok := rl.limiters[ip]
	if !ok {
		l = &ipLimiter{
			general:  rate.NewLimiter(rate.Limit(rl.cfg.RequestsPerSecond), rl.cfg.Burst),
			auth:     rate.NewLimiter(rate.Limit(float64(rl.cfg.AuthRequestsPerMin)/60.0), max(rl.cfg.AuthRequestsPerMin/3, 1)),
			lastSeen: time.Now(),
		}
		rl.limiters[ip] = l
	}
	l.lastSeen = time.Now()
	return l
}

func (rl *rateLimitMiddleware) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for ip, l := range rl.limiters {
				if l.lastSeen.Before(cutoff) {
					delete(rl.limiters, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *rateLimitMiddleware) Stop() {
	close(rl.stopCh)
}

func (rl *rateLimitMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		l := rl.getLimiter(ip)

		if !l.general.Allow() {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
			return
		}

		if isAuthEndpoint(r.Method, r.URL.Path) {
			if !l.auth.Allow() {
				w.Header().Set("Retry-After", "5")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{"error": "auth rate limit exceeded"})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if parts := strings.SplitN(xff, ",", 2); len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isAuthEndpoint(method, path string) bool {
	if method != "POST" && method != "PUT" && method != "DELETE" {
		return false
	}
	return strings.HasPrefix(path, "/api/api-keys") || strings.HasPrefix(path, "/api/projects")
}
