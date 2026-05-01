//go:build integration

package storage

import (
	"context"
	"testing"
	"time"
)

func TestSessionAudit_ContentCountsOnlyHTML2xxPages(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sid := "dddddddd-eeee-ffff-0000-444444444444"
	t.Cleanup(func() { cleanupRedirectTestSession(t, s, sid) })
	cleanupRedirectTestSession(t, s, sid)

	now := time.Now()
	pages := []PageRow{
		{CrawlSessionID: sid, URL: "https://example.com/html-ok-1", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Duplicate Title", TitleLength: 15, MetaDescription: "Short meta", MetaDescLength: 10, H1: []string{"H1"}, WordCount: 80, CrawledAt: now},
		{CrawlSessionID: sid, URL: "https://example.com/html-ok-2", FinalURL: "", StatusCode: 200, ContentType: "text/html", Title: "Duplicate Title", TitleLength: 15, MetaDescription: "Another short meta", MetaDescLength: 18, H1: []string{"H1"}, WordCount: 120, CrawledAt: now},
		{CrawlSessionID: sid, URL: "https://example.com/html-404", FinalURL: "", StatusCode: 404, ContentType: "text/html", Title: "", TitleLength: 0, MetaDescription: "", MetaDescLength: 0, H1: nil, WordCount: 0, CrawledAt: now},
		{CrawlSessionID: sid, URL: "https://example.com/file.pdf", FinalURL: "", StatusCode: 200, ContentType: "application/pdf", Title: "", TitleLength: 0, MetaDescription: "", MetaDescLength: 0, H1: nil, WordCount: 0, CrawledAt: now},
	}
	insertTestPages(t, s, sid, pages)

	audit, err := s.SessionAudit(ctx, sid)
	if err != nil {
		t.Fatalf("SessionAudit: %v", err)
	}

	if audit.Content.Total != 2 {
		t.Fatalf("content.Total: expected 2 HTML 2xx pages, got %d", audit.Content.Total)
	}
	if audit.Content.HTMLPages != 2 {
		t.Fatalf("content.HTMLPages: expected 2, got %d", audit.Content.HTMLPages)
	}
	if audit.Content.TitleDuplicates != 1 {
		t.Fatalf("content.TitleDuplicates: expected 1 duplicate among HTML 2xx pages, got %d", audit.Content.TitleDuplicates)
	}
	if audit.Content.TitleMissing != 0 {
		t.Fatalf("content.TitleMissing: expected 0 after excluding HTML 404/PDF, got %d", audit.Content.TitleMissing)
	}
	if audit.Content.H1Missing != 0 {
		t.Fatalf("content.H1Missing: expected 0 after excluding HTML 404/PDF, got %d", audit.Content.H1Missing)
	}
}
