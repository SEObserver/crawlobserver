package apikeys

import (
	"context"
	"crypto/subtle"
	"net/http"
)

type contextKey struct{}

type AuthInfo struct {
	Method    string  // "basic" | "apikey"
	KeyType   string  // "general" | "project" (only for apikey)
	ProjectID *string // non-nil only for project keys
}

func (a *AuthInfo) IsReadOnly() bool {
	return a.Method == "apikey" && a.KeyType == "project"
}

func FromContext(ctx context.Context) *AuthInfo {
	if v, ok := ctx.Value(contextKey{}).(*AuthInfo); ok {
		return v
	}
	return nil
}

func Authenticate(keyStore *Store, basicUser, basicPass string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try API key first
			if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
				result := keyStore.ValidateKey(apiKey)
				if result == nil {
					http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
					return
				}
				info := &AuthInfo{
					Method:    "apikey",
					KeyType:   result.Type,
					ProjectID: result.ProjectID,
				}
				ctx := context.WithValue(r.Context(), contextKey{}, info)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Fall back to basic auth
			if basicUser != "" && basicPass != "" {
				user, pass, ok := r.BasicAuth()
				if ok &&
					subtle.ConstantTimeCompare([]byte(user), []byte(basicUser)) == 1 &&
					subtle.ConstantTimeCompare([]byte(pass), []byte(basicPass)) == 1 {
					info := &AuthInfo{Method: "basic"}
					ctx := context.WithValue(r.Context(), contextKey{}, info)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			w.Header().Set("WWW-Authenticate", `Basic realm="CrawlObserver"`)
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		})
	}
}
