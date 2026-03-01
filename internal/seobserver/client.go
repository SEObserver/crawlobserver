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

// APICallMeta captures metadata about an API call for logging.
type APICallMeta struct {
	Endpoint     string
	Method       string
	StatusCode   uint16
	DurationMs   uint32
	ResponseBody string // truncated to 10KB
}

const maxResponseBodyLog = 10 * 1024

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*apiResponse, *APICallMeta, error) {
	meta := &APICallMeta{Endpoint: path, Method: method}
	start := time.Now()

	url := c.baseURL + "/" + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, meta, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("X-SEObserver-key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	meta.DurationMs = uint32(time.Since(start).Milliseconds())
	if err != nil {
		return nil, meta, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()
	meta.StatusCode = uint16(resp.StatusCode)

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, meta, fmt.Errorf("reading response: %w", err)
	}

	if len(data) <= maxResponseBodyLog {
		meta.ResponseBody = string(data)
	} else {
		meta.ResponseBody = string(data[:maxResponseBodyLog])
	}

	if resp.StatusCode == 401 {
		return nil, meta, fmt.Errorf("unauthorized: invalid API key")
	}
	if resp.StatusCode != 200 {
		return nil, meta, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	var result apiResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, meta, fmt.Errorf("parsing response: %w", err)
	}
	if result.Status != "ok" {
		return nil, meta, fmt.Errorf("API error: %s", result.Message)
	}
	return &result, meta, nil
}

func (c *Client) postJSON(ctx context.Context, path string, payload interface{}) (*apiResponse, *APICallMeta, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling payload: %w", err)
	}
	return c.doRequest(ctx, "POST", path, strings.NewReader(string(b)))
}

func (c *Client) get(ctx context.Context, path string) (*apiResponse, *APICallMeta, error) {
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
func (c *Client) GetDomainMetrics(ctx context.Context, domain string) (*DomainMetrics, *APICallMeta, error) {
	items := []map[string]string{{"item_type": "domain", "item_value": domain}}
	resp, meta, err := c.postJSON(ctx, "backlinks/metrics.json", map[string]interface{}{"items": items})
	if err != nil {
		return nil, meta, err
	}
	var rows []DomainMetrics
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, meta, fmt.Errorf("parsing metrics: %w", err)
	}
	if len(rows) == 0 {
		return &DomainMetrics{}, meta, nil
	}
	return &rows[0], meta, nil
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
func (c *Client) FetchBacklinks(ctx context.Context, domain string, limit int) ([]Backlink, *APICallMeta, error) {
	payload := map[string]interface{}{
		"item":  domain,
		"limit": limit,
	}
	resp, meta, err := c.postJSON(ctx, "backlinks/top.json", payload)
	if err != nil {
		return nil, meta, err
	}
	var rows []Backlink
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, meta, fmt.Errorf("parsing backlinks: %w", err)
	}
	return rows, meta, nil
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
func (c *Client) FetchRefDomains(ctx context.Context, domain string, limit int) ([]RefDomain, *APICallMeta, error) {
	payload := map[string]interface{}{
		"item":  domain,
		"limit": limit,
	}
	resp, meta, err := c.postJSON(ctx, "backlinks/refdomains.json", payload)
	if err != nil {
		return nil, meta, err
	}
	var rows []RefDomain
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, meta, fmt.Errorf("parsing refdomains: %w", err)
	}
	return rows, meta, nil
}

// --- Anchors ---

type Anchor struct {
	AnchorText    string `json:"anchor"`
	BacklinkCount int64  `json:"backlinks"`
	RefDomains    int64  `json:"refdomains"`
}

// FetchAnchors fetches anchor text distribution via backlinks/anchors.json.
func (c *Client) FetchAnchors(ctx context.Context, domain string, limit int) ([]Anchor, *APICallMeta, error) {
	payload := map[string]interface{}{
		"item":  domain,
		"limit": limit,
	}
	resp, meta, err := c.postJSON(ctx, "backlinks/anchors.json", payload)
	if err != nil {
		return nil, meta, err
	}
	var rows []Anchor
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, meta, fmt.Errorf("parsing anchors: %w", err)
	}
	return rows, meta, nil
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
func (c *Client) FetchRankings(ctx context.Context, domain, base string, limit, offset int) ([]Ranking, *APICallMeta, error) {
	path := fmt.Sprintf("organic_keywords/index.json?domain=%s&base=%s&limit=%d&offset=%d", domain, base, limit, offset)
	resp, meta, err := c.get(ctx, path)
	if err != nil {
		return nil, meta, err
	}
	var rows []Ranking
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, meta, fmt.Errorf("parsing rankings: %w", err)
	}
	return rows, meta, nil
}

// --- Visibility History ---

type VisibilityPoint struct {
	Date          string  `json:"date"`
	Visibility    float64 `json:"visibility"`
	KeywordsCount int64   `json:"keywords_count"`
}

// FetchVisibilityHistory fetches organic visibility history.
func (c *Client) FetchVisibilityHistory(ctx context.Context, domain, base string) ([]VisibilityPoint, *APICallMeta, error) {
	items := []map[string]string{{"item_type": "domain", "item_value": domain}}
	payload := map[string]interface{}{
		"items": items,
		"base":  base,
	}
	resp, meta, err := c.postJSON(ctx, "organic_keywords/visibility_history.json", payload)
	if err != nil {
		return nil, meta, err
	}
	var rows []VisibilityPoint
	if err := json.Unmarshal(resp.Data, &rows); err != nil {
		return nil, meta, fmt.Errorf("parsing visibility history: %w", err)
	}
	return rows, meta, nil
}

// --- Top Pages (Majestic) ---

// TopPage represents a top page with Majestic authority metrics.
type TopPage struct {
	URL              string      `json:"url"`
	Title            string      `json:"title"`
	TrustFlow        uint8       `json:"trust_flow"`
	CitationFlow     uint8       `json:"citation_flow"`
	ExtBackLinks     int64       `json:"ext_backlinks"`
	RefDomains       int64       `json:"ref_domains"`
	TopicalTrustFlow []TopicalTF `json:"topical_trust_flow"`
	Language         string      `json:"language"`
}

// TopicalTF represents a topical trust flow entry.
type TopicalTF struct {
	Topic string `json:"topic"`
	Value uint8  `json:"value"`
}

// rawTopPage is the flat JSON structure from the Majestic/SEObserver API.
type rawTopPage struct {
	URL          string `json:"url"`
	Title        string `json:"title"`
	TrustFlow    uint8  `json:"trust_flow"`
	CitationFlow uint8  `json:"citation_flow"`
	ExtBackLinks int64  `json:"ext_backlinks"`
	RefDomains   int64  `json:"ref_domains"`
	Language     string `json:"language"`
	// Flat topical TF fields (Topic_0..Topic_9, Value_0..Value_9)
	TTFTopic0 string `json:"TopicalTrustFlow_Topic_0"`
	TTFValue0 uint8  `json:"TopicalTrustFlow_Value_0"`
	TTFTopic1 string `json:"TopicalTrustFlow_Topic_1"`
	TTFValue1 uint8  `json:"TopicalTrustFlow_Value_1"`
	TTFTopic2 string `json:"TopicalTrustFlow_Topic_2"`
	TTFValue2 uint8  `json:"TopicalTrustFlow_Value_2"`
	TTFTopic3 string `json:"TopicalTrustFlow_Topic_3"`
	TTFValue3 uint8  `json:"TopicalTrustFlow_Value_3"`
	TTFTopic4 string `json:"TopicalTrustFlow_Topic_4"`
	TTFValue4 uint8  `json:"TopicalTrustFlow_Value_4"`
	TTFTopic5 string `json:"TopicalTrustFlow_Topic_5"`
	TTFValue5 uint8  `json:"TopicalTrustFlow_Value_5"`
	TTFTopic6 string `json:"TopicalTrustFlow_Topic_6"`
	TTFValue6 uint8  `json:"TopicalTrustFlow_Value_6"`
	TTFTopic7 string `json:"TopicalTrustFlow_Topic_7"`
	TTFValue7 uint8  `json:"TopicalTrustFlow_Value_7"`
	TTFTopic8 string `json:"TopicalTrustFlow_Topic_8"`
	TTFValue8 uint8  `json:"TopicalTrustFlow_Value_8"`
	TTFTopic9 string `json:"TopicalTrustFlow_Topic_9"`
	TTFValue9 uint8  `json:"TopicalTrustFlow_Value_9"`
}

func (r *rawTopPage) toTopPage() TopPage {
	tp := TopPage{
		URL:          r.URL,
		Title:        r.Title,
		TrustFlow:    r.TrustFlow,
		CitationFlow: r.CitationFlow,
		ExtBackLinks: r.ExtBackLinks,
		RefDomains:   r.RefDomains,
		Language:     r.Language,
	}
	pairs := []struct{ t string; v uint8 }{
		{r.TTFTopic0, r.TTFValue0}, {r.TTFTopic1, r.TTFValue1},
		{r.TTFTopic2, r.TTFValue2}, {r.TTFTopic3, r.TTFValue3},
		{r.TTFTopic4, r.TTFValue4}, {r.TTFTopic5, r.TTFValue5},
		{r.TTFTopic6, r.TTFValue6}, {r.TTFTopic7, r.TTFValue7},
		{r.TTFTopic8, r.TTFValue8}, {r.TTFTopic9, r.TTFValue9},
	}
	for _, p := range pairs {
		if p.t != "" {
			tp.TopicalTrustFlow = append(tp.TopicalTrustFlow, TopicalTF{Topic: p.t, Value: p.v})
		}
	}
	return tp
}

// FetchTopPages fetches top pages with Majestic authority data.
func (c *Client) FetchTopPages(ctx context.Context, domain string, limit int) ([]TopPage, *APICallMeta, error) {
	payload := map[string]interface{}{
		"item":  domain,
		"limit": limit,
	}
	resp, meta, err := c.postJSON(ctx, "backlinks/top-pages.json", payload)
	if err != nil {
		return nil, meta, err
	}
	var raw []rawTopPage
	if err := json.Unmarshal(resp.Data, &raw); err != nil {
		return nil, meta, fmt.Errorf("parsing top pages: %w", err)
	}
	pages := make([]TopPage, len(raw))
	for i, r := range raw {
		pages[i] = r.toTopPage()
	}
	return pages, meta, nil
}
