package crawler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SEObserver/crawlobserver/internal/config"
)

func TestEngineHeadersStayOnOriginalSeeds(t *testing.T) {
	for _, crossHost := range []bool{false, true} {
		t.Run(fmt.Sprintf("cross_host=%v", crossHost), func(t *testing.T) {
			seen := make(chan http.Header, 10)
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen <- r.Header.Clone()
				fmt.Fprint(w, "User-agent: *\nAllow: /\n")
			}))
			defer other.Close()
			otherURL := other.URL
			if crossHost {
				otherURL = strings.Replace(otherURL, "127.0.0.1", "localhost", 1)
			}
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/sitemap.xml" {
					fmt.Fprintf(w, `<sitemapindex><sitemap><loc>%s/child.xml</loc></sitemap></sitemapindex>`, otherURL)
					return
				}
				http.Redirect(w, r, otherURL+r.URL.Path, http.StatusFound)
			}))
			defer origin.Close()
			e := engineWithHeaders(map[string]string{"Signature": "secret", "Authorization": "Bearer secret"})
			defer e.cancel()
			e.SessionID([]string{origin.URL})
			assertNoHeaders := func() {
				t.Helper()
				select {
				case got := <-seen:
					for _, name := range []string{"Signature", "Authorization"} {
						if got.Get(name) != "" {
							t.Errorf("untrusted origin received %s", name)
						}
					}
				default:
					t.Fatal("request did not reach the other origin")
				}
			}
			e.fetch.Fetch(origin.URL+"/page", 0, "")
			assertNoHeaders()
			e.robots.IsAllowed(origin.URL + "/page")
			assertNoHeaders()
			e.retrieveSitemaps([]string{origin.URL + "/sitemap.xml"})
			assertNoHeaders()
			e.retrieveSitemaps([]string{otherURL + "/direct-sitemap.xml"})
			assertNoHeaders()
			// Both a redirect and an URL classified as SEO-internal must pass
			// the same stricter credential boundary.
			e.resourceCh = make(chan resourceCheckItem, 2)
			e.resourceCh <- resourceCheckItem{URL: origin.URL + "/asset.css", IsInternal: true}
			e.resourceCh <- resourceCheckItem{URL: otherURL + "/asset.css", IsInternal: true}
			close(e.resourceCh)
			e.resourceCheckWorker()
			assertNoHeaders()
			assertNoHeaders()
			// Resuming with discovered URLs cannot make those URLs trusted.
			e.ResumeSession("resume-id", []string{origin.URL})
			e.fetch.Fetch(otherURL+"/retry", 0, "")
			assertNoHeaders()
		})
	}
}

func TestEngineReloadsProjectHeadersBeforeStarting(t *testing.T) {
	srv, received := headerRecordingServer(t, "ok")
	store := &fakeProjectStore{headers: map[string]map[string]string{"p": {"Signature": "old"}}}
	m := NewManager(&config.Config{}, nil, store)
	e := engineWithHeaders(map[string]string{"Signature": "old"})
	defer e.cancel()
	e.SessionID([]string{srv.URL})
	e.session.ProjectID = projectID("p")
	m.bindProjectHeaders(e)

	// Models a queued engine: its project changes after engine construction.
	store.headers["p"] = map[string]string{"Signature": "new"}
	if err := e.prepareRequestHeaders(); err != nil {
		t.Fatal(err)
	}
	e.fetch.Fetch(srv.URL+"/page", 0, "")
	if got := received("/page").Get("Signature"); got != "new" {
		t.Fatalf("Signature = %q, want new", got)
	}

	// A failed read must stop initCrawl before it creates storage buffers or
	// performs requests. A nil store makes accidental continuation fail here.
	store.err = errors.New("database unavailable")
	if err := e.initCrawl([]string{srv.URL}); err == nil {
		t.Fatal("expected header load failure")
	}
	if e.session.Status != "error" {
		t.Fatalf("status = %q, want error", e.session.Status)
	}
}
