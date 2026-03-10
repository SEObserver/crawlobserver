package cfsolve

import (
	"context"
	"net/http"
)

// SolveResult holds the outcome of a Cloudflare challenge solve attempt.
type SolveResult struct {
	Solved  bool
	Cookies []*http.Cookie
	Err     error
}

// ChallengeResolver resolves Cloudflare challenges for a given URL.
type ChallengeResolver interface {
	Solve(ctx context.Context, challengeURL string) (*SolveResult, error)
	Close()
}
