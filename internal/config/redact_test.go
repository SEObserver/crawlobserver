package config

import (
	"strings"
	"testing"
	"time"
)

// Headers are free text a person typed, so they are dropped from the session
// snapshot rather than trusted to the key-name rules: a header named
// Authorization matches none of them.
func TestSessionConfigJSON_DropsRequestHeaders(t *testing.T) {
	cfg := &Config{}
	cfg.Crawler.UserAgent = "TestBot/1.0"
	cfg.Crawler.Headers = map[string]string{
		"Signature-Agent": `"https://example.com/"`,
		"Authorization":   "Bearer a-real-credential",
	}

	got, err := SessionConfigJSON(cfg)
	if err != nil {
		t.Fatalf("SessionConfigJSON: %v", err)
	}
	for _, unwanted := range []string{"Signature-Agent", "Authorization", "a-real-credential"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("session config contains %q, want it dropped:\n%s", unwanted, got)
		}
	}
	// The rest of the crawler config still has to be there.
	if !strings.Contains(got, "TestBot/1.0") {
		t.Errorf("session config lost the user agent:\n%s", got)
	}
}

// A header set in config.yaml reached the browser pool unchecked before this,
// where none of net/http's protections apply. Configuration is now refused at
// load, naming the header, rather than misbehaving at crawl time.
func TestValidate_RefusesBadConfiguredHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		wantIn  string
	}{
		{"a reserved name", map[string]string{"User-Agent": "Foo/1"}, "user agent setting"},
		{"a value that ends its field", map[string]string{"Signature": "a\r\nX-Injected: 1"}, "control character"},
		{"a name that is not a token", map[string]string{"X Signature": "v"}, "not allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(validConfigWithHeaders(tt.headers))
			if err == nil {
				t.Fatalf("validate(%v) = nil, want an error", tt.headers)
			}
			if !strings.Contains(err.Error(), "crawler.headers") {
				t.Errorf("error = %q, want it to name the setting", err)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantIn)
			}
		})
	}

	// Asserted on what the error says rather than on it being nil: the rest of
	// validate has requirements of its own that have nothing to do with
	// headers, and enumerating them here would make this test fail the day one
	// of them changes.
	ok := validConfigWithHeaders(map[string]string{
		"Signature-Agent": `"https://example.com/"`,
	})
	if err := validate(ok); err != nil && strings.Contains(err.Error(), "crawler.headers") {
		t.Errorf("validate(signature headers) complained about the headers: %v", err)
	}
}

// validConfigWithHeaders builds a config whose crawler section is sound, so
// that a complaint about crawler.headers can only come from the headers.
func validConfigWithHeaders(headers map[string]string) *Config {
	cfg := &Config{}
	cfg.Crawler.Workers = 1
	cfg.Crawler.Timeout = time.Second
	cfg.Crawler.MaxBodySize = 1024
	cfg.Crawler.UserAgent = "TestBot/1.0"
	cfg.Crawler.Headers = headers
	cfg.Storage.BatchSize = 1
	cfg.Storage.FlushInterval = time.Second
	cfg.Server.Port = 8899
	return cfg
}
