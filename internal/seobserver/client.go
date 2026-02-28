package seobserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client for the SEObserver API.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewClient creates a new SEObserver API client.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: "https://api1.seobserver.com",
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// apiResponse is the common response wrapper from SEObserver.
type apiResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*apiResponse, error) {
	url := c.baseURL + "/" + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-SEObserver-key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("unauthorized: invalid API key")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	var result apiResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if result.Status != "ok" {
		return nil, fmt.Errorf("API error: %s", result.Message)
	}
	return &result, nil
}

func (c *Client) postJSON(ctx context.Context, path string, payload interface{}) (*apiResponse, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling payload: %w", err)
	}
	return c.doRequest(ctx, "POST", path, strings.NewReader(string(b)))
}

func (c *Client) get(ctx context.Context, path string) (*apiResponse, error) {
	return c.doRequest(ctx, "GET", path, nil)
}

// --- Domain Metrics ---

type DomainMetrics struct {
	BacklinksTotal  int64   `json:"backlinks"`
	RefDomainsTotal int64   `json:"refdomains"`
	DomainRank      float64 `json:"domain_rank"`
	OrganicKeywords int64   `json:"organic_keywords"`
	OrganicTraffic  int64   `json:"organic_traffic"`
	OrganicCost     float64 `json:"organic_cost"`
}

// GetDomainMetrics fetches domain-level metrics via backlinks/metrics.json.
func (c *Client) GetDomainMetrics(ctx context.Context, domain string) (*DomainMetrics, error) {
	items := []map[string]string{{"item_type": "domain", "item_value": domain}}
	resp, err := c.postJSON(ctx, "backlinks/metrics.json", map[string]interface{}{"items": items})
	if err != nil {
		return nil, err
	}
	var rows []DomainMetrics
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("parsing metrics: %w", err)
	}
	if len(rows) == 0 {
		return &DomainMetrics{}, nil
	}
	return &rows[0], nil
}

// --- Backlinks ---

type Backlink struct {
	SourceURL    string  `json:"source_url"`
	TargetURL    string  `json:"target_url"`
	AnchorText   string  `json:"anchor"`
	SourceDomain string  `json:"source_domain"`
	LinkType     string  `json:"type"`
	DomainRank   float64 `json:"domain_rank"`
	PageRank     float64 `json:"page_rank"`
	Nofollow     bool    `json:"nofollow"`
	FirstSeen    string  `json:"first_seen"`
	LastSeen     string  `json:"last_seen"`
}

// FetchBacklinks fetches top backlinks via backlinks/top.json.
func (c *Client) FetchBacklinks(ctx context.Context, domain string, limit int) ([]Backlink, error) {
	payload := map[string]interface{}{
		"item":  domain,
		"limit": limit,
	}
	resp, err := c.postJSON(ctx, "backlinks/top.json", payload)
	if err != nil {
		return nil, err
	}
	var rows []Backlink
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("parsing backlinks: %w", err)
	}
	return rows, nil
}

// --- Referring Domains ---

type RefDomain struct {
	Domain       string  `json:"domain"`
	BacklinkCount int64  `json:"backlinks"`
	DomainRank   float64 `json:"domain_rank"`
	FirstSeen    string  `json:"first_seen"`
	LastSeen     string  `json:"last_seen"`
}

// FetchRefDomains fetches referring domains via backlinks/refdomains.json.
func (c *Client) FetchRefDomains(ctx context.Context, domain string, limit int) ([]RefDomain, error) {
	payload := map[string]interface{}{
		"item":  domain,
		"limit": limit,
	}
	resp, err := c.postJSON(ctx, "backlinks/refdomains.json", payload)
	if err != nil {
		return nil, err
	}
	var rows []RefDomain
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("parsing refdomains: %w", err)
	}
	return rows, nil
}

// --- Anchors ---

type Anchor struct {
	AnchorText    string `json:"anchor"`
	BacklinkCount int64  `json:"backlinks"`
	RefDomains    int64  `json:"refdomains"`
}

// FetchAnchors fetches anchor text distribution via backlinks/anchors.json.
func (c *Client) FetchAnchors(ctx context.Context, domain string, limit int) ([]Anchor, error) {
	payload := map[string]interface{}{
		"item":  domain,
		"limit": limit,
	}
	resp, err := c.postJSON(ctx, "backlinks/anchors.json", payload)
	if err != nil {
		return nil, err
	}
	var rows []Anchor
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("parsing anchors: %w", err)
	}
	return rows, nil
}

// --- Rankings ---

type Ranking struct {
	Keyword      string  `json:"keyword"`
	Position     uint16  `json:"position"`
	URL          string  `json:"url"`
	SearchVolume int64   `json:"search_volume"`
	CPC          float64 `json:"cpc"`
	Traffic      float64 `json:"traffic"`
	TrafficPct   float64 `json:"traffic_pct"`
}

// FetchRankings fetches organic keyword rankings via organic_keywords/index.json.
func (c *Client) FetchRankings(ctx context.Context, domain, base string, limit, offset int) ([]Ranking, error) {
	path := fmt.Sprintf("organic_keywords/index.json?domain=%s&base=%s&limit=%d&offset=%d", domain, base, limit, offset)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var rows []Ranking
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("parsing rankings: %w", err)
	}
	return rows, nil
}

// --- Visibility History ---

type VisibilityPoint struct {
	Date          string  `json:"date"`
	Visibility    float64 `json:"visibility"`
	KeywordsCount int64   `json:"keywords_count"`
}

// FetchVisibilityHistory fetches organic visibility history.
func (c *Client) FetchVisibilityHistory(ctx context.Context, domain, base string) ([]VisibilityPoint, error) {
	items := []map[string]string{{"item_type": "domain", "item_value": domain}}
	payload := map[string]interface{}{
		"items": items,
		"base":  base,
	}
	resp, err := c.postJSON(ctx, "organic_keywords/visibility_history.json", payload)
	if err != nil {
		return nil, err
	}
	var rows []VisibilityPoint
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, fmt.Errorf("parsing visibility history: %w", err)
	}
	return rows, nil
}
