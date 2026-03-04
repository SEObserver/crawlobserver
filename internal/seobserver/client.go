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
	apiKey    string
	baseURL   string
	userAgent string
	http      *http.Client
}

// NewClient creates a new SEObserver API client.
func NewClient(apiKey, appVersion string) *Client {
	return &Client{
		apiKey:    apiKey,
		baseURL:   "https://api1.seobserver.com",
		userAgent: "CrawlObserver-API/" + appVersion,
		http:      &http.Client{Timeout: 60 * time.Second},
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
	req.Header.Set("User-Agent", c.userAgent)
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
	resp, meta, err := c.postJSON(ctx, "backlinks/metrics.json", items)
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
	SourceURL      string  `json:"source_url"`
	TargetURL      string  `json:"target_url"`
	AnchorText     string  `json:"anchor"`
	SourceDomain   string  `json:"source_domain"`
	LinkType       string  `json:"type"`
	TrustFlow      float64 `json:"trust_flow"`
	CitationFlow   float64 `json:"citation_flow"`
	SourceTTFTopic string  `json:"source_ttf_topic"`
	Nofollow       bool    `json:"nofollow"`
	FirstSeen      string  `json:"first_seen"`
	LastSeen       string  `json:"last_seen"`
}

// parseRawBacklink parses a single raw backlink from the Majestic-style API response.
// Field names are PascalCase and types are mixed (FlagNoFollow is int 0/1, etc.).
func parseRawBacklink(raw map[string]interface{}) Backlink {
	srcURL := jsonStr(raw, "SourceURL")
	srcDomain := ""
	if srcURL != "" {
		// Extract domain from source URL
		if idx := strings.Index(srcURL, "://"); idx >= 0 {
			rest := srcURL[idx+3:]
			if slash := strings.Index(rest, "/"); slash >= 0 {
				srcDomain = rest[:slash]
			} else {
				srcDomain = rest
			}
		}
	}
	return Backlink{
		SourceURL:      srcURL,
		TargetURL:      jsonStr(raw, "TargetURL"),
		AnchorText:     jsonStr(raw, "AnchorText"),
		SourceDomain:   srcDomain,
		LinkType:       jsonStr(raw, "LinkType"),
		TrustFlow:      jsonFloat64(raw, "SourceTrustFlow"),
		CitationFlow:   jsonFloat64(raw, "SourceCitationFlow"),
		SourceTTFTopic: jsonStr(raw, "SourceTopicalTrustFlow_Topic_0"),
		Nofollow:       jsonInt64(raw, "FlagNoFollow") != 0,
		FirstSeen:      jsonStr(raw, "FirstIndexedDate"),
		LastSeen:       jsonStr(raw, "LastSeenDate"),
	}
}

// jsonFloat64 extracts a float64 from a map, handling int and string values.
func jsonFloat64(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case string:
		return 0
	}
	return 0
}

// FetchBacklinks fetches top backlinks via backlinks/top.json.
func (c *Client) FetchBacklinks(ctx context.Context, domain string, limit int) ([]Backlink, *APICallMeta, error) {
	items := []map[string]string{{"item_type": "domain", "item_value": domain}}
	path := fmt.Sprintf("backlinks/top.json?limit=%d", limit)
	resp, meta, err := c.postJSON(ctx, path, items)
	if err != nil {
		return nil, meta, err
	}
	var rawRows []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &rawRows); err != nil {
		return nil, meta, fmt.Errorf("parsing backlinks: %w", err)
	}
	rows := make([]Backlink, len(rawRows))
	for i, raw := range rawRows {
		rows[i] = parseRawBacklink(raw)
	}
	return rows, meta, nil
}

// --- Referring Domains ---

type RefDomain struct {
	Domain        string  `json:"domain"`
	BacklinkCount int64   `json:"backlinks"`
	DomainRank    float64 `json:"domain_rank"`
	FirstSeen     string  `json:"first_seen"`
	LastSeen      string  `json:"last_seen"`
}

// FetchRefDomains fetches referring domains via backlinks/refdomains.json.
func (c *Client) FetchRefDomains(ctx context.Context, domain string, limit int) ([]RefDomain, *APICallMeta, error) {
	items := []map[string]string{{"item_type": "domain", "item_value": domain}}
	path := fmt.Sprintf("backlinks/refdomains.json?limit=%d", limit)
	resp, meta, err := c.postJSON(ctx, path, items)
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
	items := []map[string]string{{"item_type": "domain", "item_value": domain}}
	path := fmt.Sprintf("backlinks/anchors.json?limit=%d", limit)
	resp, meta, err := c.postJSON(ctx, path, items)
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
	path := fmt.Sprintf("organic_keywords/visibility_history.json?base=%s", base)
	resp, meta, err := c.postJSON(ctx, path, items)
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
	OutLinks         int64       `json:"out_links"`
	TopicalTrustFlow []TopicalTF `json:"topical_trust_flow"`
	Language         string      `json:"language"`
	LastCrawlResult  string      `json:"last_crawl_result"`
	LastCrawlDate    string      `json:"last_crawl_date"`
}

// TopicalTF represents a topical trust flow entry.
type TopicalTF struct {
	Topic string `json:"topic"`
	Value uint8  `json:"value"`
}

// ParseRawTopPage parses a single raw top page from the flexible API response.
// The API returns mixed types: TTF values can be int or empty string, OutLinks is a string.
func ParseRawTopPage(raw map[string]interface{}) TopPage {
	tp := TopPage{
		URL:             jsonStr(raw, "URL"),
		Title:           jsonStr(raw, "Title"),
		TrustFlow:       jsonUint8(raw, "TrustFlow"),
		CitationFlow:    jsonUint8(raw, "CitationFlow"),
		ExtBackLinks:    jsonInt64(raw, "ExtBackLinks"),
		RefDomains:      jsonInt64(raw, "RefDomains"),
		Language:        jsonStr(raw, "Language"),
		OutLinks:        jsonInt64(raw, "OutLinks"),
		LastCrawlResult: jsonStr(raw, "LastCrawlResult"),
		LastCrawlDate:   jsonStr(raw, "Date"),
	}
	for i := 0; i < 10; i++ {
		topicKey := fmt.Sprintf("TopicalTrustFlow_Topic_%d", i)
		valueKey := fmt.Sprintf("TopicalTrustFlow_Value_%d", i)
		topic := jsonStr(raw, topicKey)
		if topic == "" {
			continue
		}
		tp.TopicalTrustFlow = append(tp.TopicalTrustFlow, TopicalTF{
			Topic: topic,
			Value: jsonUint8(raw, valueKey),
		})
	}
	return tp
}

// jsonStr extracts a string from a map, handling nil.
func jsonStr(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// jsonUint8 extracts a uint8 from a map, handling float64 (JSON numbers) and string values.
func jsonUint8(m map[string]interface{}, key string) uint8 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return uint8(val)
	case string:
		// API sometimes returns "" for empty TTF values
		return 0
	}
	return 0
}

// jsonInt64 extracts an int64 from a map, handling float64 and string values.
func jsonInt64(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	case string:
		return 0
	}
	return 0
}

// FetchTopPages fetches top pages with Majestic authority data via backlinks/pages.json.
func (c *Client) FetchTopPages(ctx context.Context, domain string, limit int) ([]TopPage, *APICallMeta, error) {
	items := []map[string]string{{"item_type": "domain", "item_value": domain}}
	path := fmt.Sprintf("backlinks/pages.json?limit=%d", limit)
	resp, meta, err := c.postJSON(ctx, path, items)
	if err != nil {
		return nil, meta, err
	}
	var rawRows []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &rawRows); err != nil {
		return nil, meta, fmt.Errorf("parsing top pages: %w", err)
	}
	pages := make([]TopPage, len(rawRows))
	for i, raw := range rawRows {
		pages[i] = ParseRawTopPage(raw)
	}
	return pages, meta, nil
}
