package seobserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(handler http.HandlerFunc) (*Client, *httptest.Server) {
	ts := httptest.NewServer(handler)
	c := NewClient("test-key", "1.2.3")
	c.baseURL = ts.URL
	return c, ts
}

func jsonResp(status string, data interface{}) []byte {
	d, _ := json.Marshal(data)
	resp := apiResponse{Status: status, Data: json.RawMessage(d)}
	b, _ := json.Marshal(resp)
	return b
}

func TestDoRequest_SetsUserAgent(t *testing.T) {
	var gotUA string
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Write(jsonResp("ok", nil))
	})
	defer ts.Close()

	c.get(context.Background(), "test")
	if gotUA != "CrawlObserver-API/1.2.3" {
		t.Errorf("got User-Agent %q, want %q", gotUA, "CrawlObserver-API/1.2.3")
	}
}

func TestDoRequest_SetsAPIKeyHeader(t *testing.T) {
	var gotKey string
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-SEObserver-key")
		w.Write(jsonResp("ok", nil))
	})
	defer ts.Close()

	c.get(context.Background(), "test")
	if gotKey != "test-key" {
		t.Errorf("got API key %q, want %q", gotKey, "test-key")
	}
}

func TestDoRequest_401(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`unauthorized`))
	})
	defer ts.Close()

	_, meta, err := c.get(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if meta.StatusCode != 401 {
		t.Errorf("meta.StatusCode = %d, want 401", meta.StatusCode)
	}
}

func TestDoRequest_Non200(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`internal error`))
	})
	defer ts.Close()

	_, _, err := c.get(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestDoRequest_InvalidJSON(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	})
	defer ts.Close()

	_, _, err := c.get(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDoRequest_StatusNotOK(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{Status: "error", Message: "quota exceeded"}
		b, _ := json.Marshal(resp)
		w.Write(b)
	})
	defer ts.Close()

	_, _, err := c.get(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for status != ok")
	}
}

func TestDoRequest_MetaCapture(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write(jsonResp("ok", nil))
	})
	defer ts.Close()

	_, meta, err := c.get(context.Background(), "some/path")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Endpoint != "some/path" {
		t.Errorf("meta.Endpoint = %q, want %q", meta.Endpoint, "some/path")
	}
	if meta.Method != "GET" {
		t.Errorf("meta.Method = %q, want %q", meta.Method, "GET")
	}
	if meta.StatusCode != 200 {
		t.Errorf("meta.StatusCode = %d, want 200", meta.StatusCode)
	}
}

func TestDoRequest_ResponseBodyTruncation(t *testing.T) {
	largeBody := make([]byte, maxResponseBodyLog+100)
	for i := range largeBody {
		largeBody[i] = 'x'
	}
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write(largeBody)
	})
	defer ts.Close()

	// Will fail JSON parsing but meta should still capture truncated body
	_, meta, _ := c.get(context.Background(), "test")
	if len(meta.ResponseBody) != maxResponseBodyLog {
		t.Errorf("ResponseBody len = %d, want %d", len(meta.ResponseBody), maxResponseBodyLog)
	}
}

func TestGetDomainMetrics(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		metrics := []DomainMetrics{{BacklinksTotal: 1000, DomainRank: 55.5}}
		w.Write(jsonResp("ok", metrics))
	})
	defer ts.Close()

	m, meta, err := c.GetDomainMetrics(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if m.BacklinksTotal != 1000 {
		t.Errorf("BacklinksTotal = %d, want 1000", m.BacklinksTotal)
	}
	if m.DomainRank != 55.5 {
		t.Errorf("DomainRank = %f, want 55.5", m.DomainRank)
	}
	if meta.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", meta.StatusCode)
	}
}

func TestGetDomainMetrics_EmptyResult(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write(jsonResp("ok", []DomainMetrics{}))
	})
	defer ts.Close()

	m, _, err := c.GetDomainMetrics(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if m.BacklinksTotal != 0 {
		t.Errorf("expected zero-value metrics for empty result")
	}
}

func TestFetchBacklinks(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		// Majestic-style response with PascalCase field names
		raw := []map[string]interface{}{
			{
				"SourceURL":           "https://a.com/page",
				"TargetURL":           "https://b.com/",
				"AnchorText":          "click here",
				"LinkType":            "TextLink",
				"SourceTrustFlow":     42.0,
				"SourceCitationFlow":  38.0,
				"ACRank":              7.0,
				"FlagNoFollow":        1,
				"FirstIndexedDate":    "2025-01-15",
				"LastSeenDate":        "2025-06-01",
			},
		}
		w.Write(jsonResp("ok", raw))
	})
	defer ts.Close()

	rows, _, err := c.FetchBacklinks(context.Background(), "example.com", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	bl := rows[0]
	if bl.SourceURL != "https://a.com/page" {
		t.Errorf("SourceURL = %q", bl.SourceURL)
	}
	if bl.SourceDomain != "a.com" {
		t.Errorf("SourceDomain = %q, want a.com", bl.SourceDomain)
	}
	if bl.TrustFlow != 42 {
		t.Errorf("TrustFlow = %f, want 42", bl.TrustFlow)
	}
	if bl.CitationFlow != 38 {
		t.Errorf("CitationFlow = %f, want 38", bl.CitationFlow)
	}
	if bl.AnchorText != "click here" {
		t.Errorf("AnchorText = %q", bl.AnchorText)
	}
	if !bl.Nofollow {
		t.Error("expected Nofollow=true")
	}
}

func TestFetchRefDomains(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		doms := []RefDomain{{Domain: "ref.com", BacklinkCount: 5}}
		w.Write(jsonResp("ok", doms))
	})
	defer ts.Close()

	rows, _, err := c.FetchRefDomains(context.Background(), "example.com", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Domain != "ref.com" {
		t.Errorf("unexpected result: %+v", rows)
	}
}

func TestFetchAnchors(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		anchors := []Anchor{{AnchorText: "click here", BacklinkCount: 10}}
		w.Write(jsonResp("ok", anchors))
	})
	defer ts.Close()

	rows, _, err := c.FetchAnchors(context.Background(), "example.com", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].AnchorText != "click here" {
		t.Errorf("unexpected result: %+v", rows)
	}
}

func TestFetchRankings(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		rankings := []Ranking{{Keyword: "seo", Position: 3, SearchVolume: 5000}}
		w.Write(jsonResp("ok", rankings))
	})
	defer ts.Close()

	rows, _, err := c.FetchRankings(context.Background(), "example.com", "us", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Keyword != "seo" {
		t.Errorf("unexpected result: %+v", rows)
	}
}

func TestFetchTopPages(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		// Verify correct endpoint and query param
		if r.URL.Path != "/backlinks/pages.json" {
			t.Errorf("path = %q, want /backlinks/pages.json", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("limit param = %q, want 10", r.URL.Query().Get("limit"))
		}
		// Verify body is array of items
		var items []map[string]string
		json.NewDecoder(r.Body).Decode(&items)
		if len(items) != 1 || items[0]["item_type"] != "domain" || items[0]["item_value"] != "example.com" {
			t.Errorf("unexpected body: %+v", items)
		}

		// Return response with mixed types (like real API)
		raw := []map[string]interface{}{
			{
				"URL":                          "https://example.com/page",
				"Title":                        "Test",
				"TrustFlow":                    float64(30),
				"CitationFlow":                 float64(20),
				"ExtBackLinks":                 float64(500),
				"RefDomains":                   float64(100),
				"OutLinks":                     "42", // string in API
				"TopicalTrustFlow_Topic_0":     "Business",
				"TopicalTrustFlow_Value_0":     float64(25),
				"TopicalTrustFlow_Topic_1":     "Tech",
				"TopicalTrustFlow_Value_1":     float64(15),
				"TopicalTrustFlow_Value_2":     "",  // empty string TTF value
				"Language":                     "en",
			},
		}
		w.Write(jsonResp("ok", raw))
	})
	defer ts.Close()

	pages, _, err := c.FetchTopPages(context.Background(), "example.com", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	if pages[0].TrustFlow != 30 {
		t.Errorf("TrustFlow = %d, want 30", pages[0].TrustFlow)
	}
	if pages[0].CitationFlow != 20 {
		t.Errorf("CitationFlow = %d, want 20", pages[0].CitationFlow)
	}
	if pages[0].ExtBackLinks != 500 {
		t.Errorf("ExtBackLinks = %d, want 500", pages[0].ExtBackLinks)
	}
	if len(pages[0].TopicalTrustFlow) != 2 {
		t.Fatalf("got %d TTF entries, want 2", len(pages[0].TopicalTrustFlow))
	}
	if pages[0].TopicalTrustFlow[0].Topic != "Business" {
		t.Errorf("TTF[0].Topic = %q, want %q", pages[0].TopicalTrustFlow[0].Topic, "Business")
	}
}

func TestParseRawTopPage(t *testing.T) {
	tests := []struct {
		name     string
		raw      map[string]interface{}
		wantTTFs int
		wantURL  string
	}{
		{
			name:     "no TTF",
			raw:      map[string]interface{}{"URL": "https://a.com"},
			wantTTFs: 0,
			wantURL:  "https://a.com",
		},
		{
			name: "some TTF with mixed types",
			raw: map[string]interface{}{
				"URL":                      "https://a.com",
				"TopicalTrustFlow_Topic_0": "Arts",
				"TopicalTrustFlow_Value_0": float64(10),
				"TopicalTrustFlow_Topic_3": "Science",
				"TopicalTrustFlow_Value_3": float64(5),
				"TopicalTrustFlow_Value_5": "", // empty string = skip
			},
			wantTTFs: 2,
			wantURL:  "https://a.com",
		},
		{
			name: "OutLinks as string",
			raw: map[string]interface{}{
				"URL":      "https://b.com",
				"OutLinks": "123",
			},
			wantTTFs: 0,
			wantURL:  "https://b.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRawTopPage(tt.raw)
			if len(got.TopicalTrustFlow) != tt.wantTTFs {
				t.Errorf("got %d TTFs, want %d", len(got.TopicalTrustFlow), tt.wantTTFs)
			}
			if got.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tt.wantURL)
			}
		})
	}
}

func TestFetchVisibilityHistory(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		points := []VisibilityPoint{{Date: "2025-01-01", Visibility: 12.5, KeywordsCount: 100}}
		w.Write(jsonResp("ok", points))
	})
	defer ts.Close()

	rows, _, err := c.FetchVisibilityHistory(context.Background(), "example.com", "us")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Visibility != 12.5 {
		t.Errorf("unexpected result: %+v", rows)
	}
}

func TestDoRequest_CancelledContext(t *testing.T) {
	c, ts := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write(jsonResp("ok", nil))
	})
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.get(ctx, "test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
