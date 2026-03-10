package cfsolve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// APIResolver resolves Cloudflare challenges via an external HTTP API.
type APIResolver struct {
	apiURL  string
	apiKey  string
	client  *http.Client
}

type apiRequest struct {
	URL string `json:"url"`
}

type apiCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httponly"`
}

type apiResponse struct {
	Solved  bool        `json:"solved"`
	Cookies []apiCookie `json:"cookies"`
}

// NewAPIResolver creates an APIResolver targeting the given endpoint.
func NewAPIResolver(apiURL, apiKey string, timeout time.Duration) *APIResolver {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &APIResolver{
		apiURL: apiURL,
		apiKey: apiKey,
		client: &http.Client{Timeout: timeout},
	}
}

func (a *APIResolver) Solve(ctx context.Context, challengeURL string) (*SolveResult, error) {
	body, err := json.Marshal(apiRequest{URL: challengeURL})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	result := &SolveResult{Solved: apiResp.Solved}
	for _, c := range apiResp.Cookies {
		result.Cookies = append(result.Cookies, &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HttpOnly: c.HTTPOnly,
		})
	}
	return result, nil
}

func (a *APIResolver) Close() {}
