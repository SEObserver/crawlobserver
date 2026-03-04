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
		links := []Backlink{
			{SourceURL: "https://a.com", TargetURL: "https://b.com", DomainRank: 42},
		}
		w.Write(jsonResp("ok", links))
	})
	defer ts.Close()

	rows, _, err := c.FetchBacklinks(context.Background(), "example.com", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].DomainRank != 42 {
		t.Errorf("DomainRank = %f, want 42", rows[0].DomainRank)
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
		raw := []rawTopPage{{
			URL:       "https://example.com/page",
			Title:     "Test",
			TrustFlow: 30,
			TTFTopic0: "Business",
			TTFValue0: 25,
			TTFTopic1: "Tech",
			TTFValue1: 15,
		}}
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
	if len(pages[0].TopicalTrustFlow) != 2 {
		t.Fatalf("got %d TTF entries, want 2", len(pages[0].TopicalTrustFlow))
	}
	if pages[0].TopicalTrustFlow[0].Topic != "Business" {
		t.Errorf("TTF[0].Topic = %q, want %q", pages[0].TopicalTrustFlow[0].Topic, "Business")
	}
}

func TestRawTopPage_ToTopPage(t *testing.T) {
	tests := []struct {
		name     string
		raw      rawTopPage
		wantTTFs int
	}{
		{
			name:     "no TTF",
			raw:      rawTopPage{URL: "https://a.com"},
			wantTTFs: 0,
		},
		{
			name: "some TTF",
			raw: rawTopPage{
				URL:       "https://a.com",
				TTFTopic0: "Arts",
				TTFValue0: 10,
				TTFTopic3: "Science",
				TTFValue3: 5,
			},
			wantTTFs: 2,
		},
		{
			name: "all TTF",
			raw: rawTopPage{
				TTFTopic0: "A", TTFValue0: 1,
				TTFTopic1: "B", TTFValue1: 2,
				TTFTopic2: "C", TTFValue2: 3,
				TTFTopic3: "D", TTFValue3: 4,
				TTFTopic4: "E", TTFValue4: 5,
				TTFTopic5: "F", TTFValue5: 6,
				TTFTopic6: "G", TTFValue6: 7,
				TTFTopic7: "H", TTFValue7: 8,
				TTFTopic8: "I", TTFValue8: 9,
				TTFTopic9: "J", TTFValue9: 10,
			},
			wantTTFs: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.raw.toTopPage()
			if len(got.TopicalTrustFlow) != tt.wantTTFs {
				t.Errorf("got %d TTFs, want %d", len(got.TopicalTrustFlow), tt.wantTTFs)
			}
			if got.URL != tt.raw.URL {
				t.Errorf("URL = %q, want %q", got.URL, tt.raw.URL)
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
