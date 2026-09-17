package crawler

import (
	"errors"
	"testing"

	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/extraction"
)

// fakeProjectStore stands for the key store, which satisfies both loaders.
type fakeProjectStore struct {
	headers map[string]map[string]string
	err     error
	calls   int
}

func (f *fakeProjectStore) GetExtractorSet(string) (*extraction.ExtractorSet, error) {
	return nil, errors.New("not used")
}

func (f *fakeProjectStore) ProjectCrawlHeaders(projectID string) (map[string]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.headers[projectID], nil
}

// loaderWithoutHeaders satisfies only ExtractorSetLoader, as a caller passing
// its own loader would.
type loaderWithoutHeaders struct{}

func (loaderWithoutHeaders) GetExtractorSet(string) (*extraction.ExtractorSet, error) {
	return nil, errors.New("not used")
}

func projectID(s string) *string { return &s }

func TestHeadersForProject(t *testing.T) {
	configured := map[string]string{"X-Configured": "yes"}
	projectHeaders := map[string]string{"Signature-Agent": `"https://example.com/"`}

	store := &fakeProjectStore{headers: map[string]map[string]string{
		"with-headers": projectHeaders,
		"no-headers":   {},
	}}
	cfg := &config.Config{}
	cfg.Crawler.Headers = configured
	m := NewManager(cfg, nil, store)

	tests := []struct {
		name    string
		project *string
		want    string
	}{
		{"a project with headers uses them", projectID("with-headers"), "Signature-Agent"},
		{"a project without falls back to the configured ones", projectID("no-headers"), "X-Configured"},
		{"an unknown project falls back", projectID("nowhere"), "X-Configured"},
		{"no project at all uses the configured ones", nil, "X-Configured"},
		{"an empty project id uses the configured ones", projectID(""), "X-Configured"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.headersForProject(tt.project, configured)
			if _, ok := got[tt.want]; !ok {
				t.Errorf("headers = %v, want the one named %q", got, tt.want)
			}
		})
	}
}

// A store that fails must not take the crawl down with it: the crawl runs with
// the configured headers, and the site says whether that is enough.
func TestHeadersForProject_StoreError(t *testing.T) {
	configured := map[string]string{"X-Configured": "yes"}
	store := &fakeProjectStore{err: errors.New("database is locked")}

	cfg := &config.Config{}
	m := NewManager(cfg, nil, store)

	got := m.headersForProject(projectID("any"), configured)
	if _, ok := got["X-Configured"]; !ok {
		t.Errorf("headers = %v, want the configured ones", got)
	}
}

// The manager discovers the header loader on the loader it is given. A loader
// that does not carry one must leave the manager working, not panicking.
func TestNewManager_LoaderWithoutHeaders(t *testing.T) {
	configured := map[string]string{"X-Configured": "yes"}
	cfg := &config.Config{}
	m := NewManager(cfg, nil, loaderWithoutHeaders{})

	if m.projectHeaders != nil {
		t.Error("projectHeaders was set from a loader that does not implement it")
	}
	got := m.headersForProject(projectID("any"), configured)
	if _, ok := got["X-Configured"]; !ok {
		t.Errorf("headers = %v, want the configured ones", got)
	}
}

func TestNewManager_NoLoaderAtAll(t *testing.T) {
	cfg := &config.Config{}
	m := NewManager(cfg, nil)

	if got := m.headersForProject(projectID("any"), nil); len(got) != 0 {
		t.Errorf("headers = %v, want none", got)
	}
}

// The headers are read whenever a crawl starts, resumes or retries, never
// restored from the session snapshot, because a signed header expires. This
// checks the store is actually consulted rather than a cached value reused.
func TestHeadersForProject_ReadsTheStoreEveryTime(t *testing.T) {
	store := &fakeProjectStore{headers: map[string]map[string]string{
		"p": {"Signature": "first"},
	}}
	cfg := &config.Config{}
	m := NewManager(cfg, nil, store)

	if got := m.headersForProject(projectID("p"), nil); got["Signature"] != "first" {
		t.Fatalf("Signature = %q, want %q", got["Signature"], "first")
	}

	// The signature is refreshed out of band, as it would be before resuming.
	store.headers["p"] = map[string]string{"Signature": "second"}

	if got := m.headersForProject(projectID("p"), nil); got["Signature"] != "second" {
		t.Errorf("Signature = %q, want the refreshed %q", got["Signature"], "second")
	}
	if store.calls != 2 {
		t.Errorf("the store was consulted %d times, want 2", store.calls)
	}
}
