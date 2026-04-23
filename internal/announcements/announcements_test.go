package announcements

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func TestPickActive_SkipsFutureAndExpired(t *testing.T) {
	now := mustTime(t, "2026-04-23T12:00:00Z")
	msgs := []Message{
		// messages[0] is future — should be skipped so messages[1] wins
		{
			ID:          "future",
			PublishedAt: "2026-04-24T00:00:00Z",
			Translations: map[string]MessageTranslation{
				"en": {Title: "Future"},
			},
		},
		{
			ID:          "active",
			PublishedAt: "2026-04-20T00:00:00Z",
			ShowUntil:   "2026-05-01T00:00:00Z",
			Translations: map[string]MessageTranslation{
				"en": {Title: "Active"},
			},
		},
	}
	got := pickActive(msgs, now)
	if got == nil {
		t.Fatal("expected an active message, got nil")
	}
	if got.ID != "active" {
		t.Errorf("expected ID=active, got %q", got.ID)
	}
}

func TestPickActive_ExpiredHidden(t *testing.T) {
	now := mustTime(t, "2026-04-23T12:00:00Z")
	msgs := []Message{
		{
			ID:          "expired",
			PublishedAt: "2026-04-01T00:00:00Z",
			ShowUntil:   "2026-04-22T00:00:00Z",
			Translations: map[string]MessageTranslation{
				"en": {Title: "Expired"},
			},
		},
	}
	if got := pickActive(msgs, now); got != nil {
		t.Errorf("expected nil for expired message, got %+v", got)
	}
}

func TestPickActive_MalformedDatesSkipped(t *testing.T) {
	now := mustTime(t, "2026-04-23T12:00:00Z")
	msgs := []Message{
		{
			ID:          "bad-date",
			PublishedAt: "not-a-date",
			Translations: map[string]MessageTranslation{
				"en": {Title: "Bad"},
			},
		},
	}
	if got := pickActive(msgs, now); got != nil {
		t.Errorf("expected nil for malformed date, got %+v", got)
	}
}

func TestPickActive_LegacyFlatSchema(t *testing.T) {
	now := mustTime(t, "2026-04-23T12:00:00Z")
	msgs := []Message{
		{
			ID:             "legacy",
			PublishedAt:    "2026-04-20T00:00:00Z",
			LegacyTitle:    "Hello",
			LegacyBody:     "World",
			LegacyCTALabel: "Click",
		},
	}
	got := pickActive(msgs, now)
	if got == nil {
		t.Fatal("expected legacy message to be normalized, got nil")
	}
	tr, ok := got.Translations["en"]
	if !ok {
		t.Fatalf("expected synthesized en translation, got %+v", got.Translations)
	}
	if tr.Title != "Hello" || tr.Body != "World" || tr.CTALabel != "Click" {
		t.Errorf("translation fields not promoted correctly: %+v", tr)
	}
	if got.LegacyTitle != "" || got.LegacyBody != "" || got.LegacyCTALabel != "" {
		t.Errorf("legacy fields should be cleared after normalize: %+v", got)
	}
	if got.DefaultLocale != "en" {
		t.Errorf("expected default_locale=en, got %q", got.DefaultLocale)
	}
}

func TestPickActive_LegacyWithExplicitDefaultLocale(t *testing.T) {
	now := mustTime(t, "2026-04-23T12:00:00Z")
	msgs := []Message{
		{
			ID:            "legacy-fr",
			PublishedAt:   "2026-04-20T00:00:00Z",
			DefaultLocale: "fr",
			LegacyTitle:   "Bonjour",
		},
	}
	got := pickActive(msgs, now)
	if got == nil {
		t.Fatal("expected legacy message to be normalized")
	}
	if _, ok := got.Translations["fr"]; !ok {
		t.Errorf("expected fr translation, got %+v", got.Translations)
	}
}

func TestPickActive_RequiresTitle(t *testing.T) {
	now := mustTime(t, "2026-04-23T12:00:00Z")
	msgs := []Message{
		{
			ID:          "no-title",
			PublishedAt: "2026-04-20T00:00:00Z",
			Translations: map[string]MessageTranslation{
				"en": {Body: "body only"},
			},
		},
	}
	if got := pickActive(msgs, now); got != nil {
		t.Errorf("expected nil for message without title, got %+v", got)
	}
}

func TestPickActive_RequiresID(t *testing.T) {
	now := mustTime(t, "2026-04-23T12:00:00Z")
	msgs := []Message{
		{
			PublishedAt: "2026-04-20T00:00:00Z",
			Translations: map[string]MessageTranslation{
				"en": {Title: "ok"},
			},
		},
	}
	if got := pickActive(msgs, now); got != nil {
		t.Errorf("expected nil for message without id, got %+v", got)
	}
}

func TestFetcher_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"messages": [
				{
					"id": "future",
					"published_at": "2099-01-01T00:00:00Z",
					"translations": {"en": {"title": "future"}}
				},
				{
					"id": "active",
					"published_at": "2020-01-01T00:00:00Z",
					"translations": {"en": {"title": "active"}}
				}
			]
		}`)
	}))
	defer srv.Close()

	f := New(srv.URL, time.Minute)
	f.now = func() time.Time { return mustTime(t, "2026-04-23T12:00:00Z") }
	f.fetchOnce(context.Background())

	msg, _ := f.Snapshot()
	if msg == nil {
		t.Fatal("expected active message, got nil")
	}
	if msg.ID != "active" {
		t.Errorf("expected ID=active, got %q", msg.ID)
	}
}

func TestFetcher_PreservesCacheOn404(t *testing.T) {
	// Feed starts healthy, then returns 404. Cache must be preserved.
	returnOK := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !returnOK {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"messages":[{"id":"m1","published_at":"2020-01-01T00:00:00Z","translations":{"en":{"title":"m1"}}}]}`)
	}))
	defer srv.Close()

	f := New(srv.URL, time.Minute)
	f.now = func() time.Time { return mustTime(t, "2026-04-23T12:00:00Z") }
	f.fetchOnce(context.Background())
	if msg, _ := f.Snapshot(); msg == nil || msg.ID != "m1" {
		t.Fatalf("expected m1 cached, got %+v", msg)
	}

	returnOK = false
	f.fetchOnce(context.Background())
	if msg, _ := f.Snapshot(); msg == nil || msg.ID != "m1" {
		t.Errorf("expected cache preserved on 404, got %+v", msg)
	}
}

func TestFetcher_EmptyFeedClearsCache(t *testing.T) {
	// Feed starts with one message, then becomes empty — cache should clear.
	payload := `{"messages":[{"id":"m1","published_at":"2020-01-01T00:00:00Z","translations":{"en":{"title":"m1"}}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer srv.Close()

	f := New(srv.URL, time.Minute)
	f.now = func() time.Time { return mustTime(t, "2026-04-23T12:00:00Z") }
	f.fetchOnce(context.Background())
	if msg, _ := f.Snapshot(); msg == nil {
		t.Fatal("expected cached message")
	}

	payload = `{"messages":[]}`
	f.fetchOnce(context.Background())
	if msg, _ := f.Snapshot(); msg != nil {
		t.Errorf("expected nil after empty feed, got %+v", msg)
	}
}
