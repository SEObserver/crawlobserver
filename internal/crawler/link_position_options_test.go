package crawler

import (
	"strings"
	"testing"

	"github.com/SEObserver/crawlobserver/internal/config"
)

func TestInvalidLinkPositionLimitRejectedBeforeStorage(t *testing.T) {
	m := NewManager(&config.Config{}, nil)
	for _, limit := range []int{0, -1} {
		req := &CrawlRequest{Seeds: []string{"https://example.com"}, MaxLinkPositionsPerPage: &limit}
		_, startErr := m.StartCrawl(*req)
		_, resumeErr := m.ResumeCrawl("missing", req)
		_, retryErr := m.RetryFailed("missing", req)
		for _, err := range []error{startErr, resumeErr, retryErr} {
			if err == nil || !strings.Contains(err.Error(), "max_link_positions_per_page") {
				t.Fatalf("limit %d: error = %v", limit, err)
			}
		}
	}
}

func TestSavedLinkPositionLimitAndLegacyDefault(t *testing.T) {
	running := &config.Config{Crawler: config.CrawlerConfig{StoreLinkPosition: true, MaxLinkPositionsPerPage: 1000, Headers: map[string]string{"Signature": "current"}}}
	for _, tc := range []struct {
		json    string
		limit   int
		enabled bool
	}{
		{`{"Crawler":{"Workers":3}}`, 1000, true},
		{`{"Crawler":{"StoreLinkPosition":false,"MaxLinkPositionsPerPage":25,"Headers":{"Signature":"old"}}}`, 25, false},
	} {
		got, err := decodeSavedConfig(tc.json, running)
		if err != nil {
			t.Fatal(err)
		}
		if got.Crawler.MaxLinkPositionsPerPage != tc.limit || got.Crawler.StoreLinkPosition != tc.enabled {
			t.Errorf("restored limit=%d enabled=%v", got.Crawler.MaxLinkPositionsPerPage, got.Crawler.StoreLinkPosition)
		}
		if running.Crawler.Headers["Signature"] != "current" {
			t.Fatal("decoding a snapshot changed running credentials")
		}
	}
}
