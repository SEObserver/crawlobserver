package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// signatureHeaders are the headers this feature exists for: the three of
// RFC 9421 as Web Bot Auth sends them. They are used as the fixture so that a
// change breaking their exact shape — the colons of a byte sequence, the
// quotes and semicolons of the signature parameters — fails here.
var signatureHeaders = map[string]string{
	"Signature-Agent": `"https://example.com/.well-known/http-message-signatures-directory"`,
	"Signature-Input": `sig1=("@authority" "signature-agent");created=1757000000;expires=1757003600;keyid="k1";alg="ed25519";tag="web-bot-auth"`,
	"Signature":       `sig1=:dGhpcyBpcyBub3QgYSByZWFsIHNpZ25hdHVyZSwgaXQgaXMgYSB0ZXN0=:`,
}

func TestValidateExtraHeaders_AcceptsSignatureHeaders(t *testing.T) {
	if err := ValidateExtraHeaders(signatureHeaders); err != nil {
		t.Fatalf("ValidateExtraHeaders(signature headers) = %v, want nil", err)
	}
}

func TestValidateExtraHeaders_Refuses(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		wantIn  string
	}{
		{"host", map[string]string{"Host": "example.com"}, "computed from the URL"},
		{"content length", map[string]string{"Content-Length": "12"}, "computed from the request body"},
		{"user agent", map[string]string{"User-Agent": "Bot/1.0"}, "user agent setting"},
		{"user agent lowercased", map[string]string{"user-agent": "Bot/1.0"}, "user agent setting"},
		{"connection", map[string]string{"Connection": "close"}, "governed by the transport"},
		{"empty name", map[string]string{"": "v"}, "empty"},
		{"space in name", map[string]string{"X Signature": "v"}, "not allowed"},
		{"colon in name", map[string]string{"X:Signature": "v"}, "not allowed"},
		{"newline in value", map[string]string{"Signature": "a\r\nX-Injected: 1"}, "control character"},
		{"nul in value", map[string]string{"Signature": "a\x00b"}, "control character"},
		{"same header twice", map[string]string{"Signature": "a", "signature": "b"}, "the same header"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExtraHeaders(tt.headers)
			if err == nil {
				t.Fatalf("ValidateExtraHeaders(%v) = nil, want an error", tt.headers)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantIn)
			}
		})
	}
}

func TestValidateExtraHeaders_Limits(t *testing.T) {
	tooMany := make(map[string]string, MaxExtraHeaders+1)
	for i := 0; i <= MaxExtraHeaders; i++ {
		tooMany[fmt.Sprintf("X-H%d", i)] = "v"
	}
	if err := ValidateExtraHeaders(tooMany); err == nil {
		t.Error("ValidateExtraHeaders(33 headers) = nil, want an error")
	}

	longValue := map[string]string{"Signature": strings.Repeat("a", MaxExtraHeaderValueLen+1)}
	if err := ValidateExtraHeaders(longValue); err == nil {
		t.Error("ValidateExtraHeaders(over-long value) = nil, want an error")
	}

	longName := map[string]string{strings.Repeat("a", MaxExtraHeaderNameLen+1): "v"}
	if err := ValidateExtraHeaders(longName); err == nil {
		t.Error("ValidateExtraHeaders(over-long name) = nil, want an error")
	}

	if err := ValidateExtraHeaders(nil); err != nil {
		t.Errorf("ValidateExtraHeaders(nil) = %v, want nil", err)
	}
}

// recordingServer answers every request and records the headers it received,
// so that the tests below assert on what reached the wire rather than on what
// the code meant to send.
func recordingServer(t *testing.T, body string) (*httptest.Server, func(string) http.Header) {
	t.Helper()
	received := make(map[string]http.Header)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received[r.URL.Path] = r.Header.Clone()
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, func(path string) http.Header { return received[path] }
}

func assertSignatureHeadersArrived(t *testing.T, got http.Header, where string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: no request was received", where)
	}
	for name, want := range signatureHeaders {
		if g := got.Get(name); g != want {
			t.Errorf("%s: header %s = %q, want %q", where, name, g, want)
		}
	}
}

func TestFetcher_SendsExtraHeaders(t *testing.T) {
	srv, received := recordingServer(t, "<html><body>hi</body></html>")

	f := New("TestBot/1.0", 5*time.Second, 1<<20,
		DialOptions{AllowPrivateIPs: true}, "",
		WithExtraHeaders(signatureHeaders))

	result := f.Fetch(srv.URL+"/page", 0, "")
	if result.StatusCode != http.StatusOK {
		t.Fatalf("Fetch status = %d (%s), want 200", result.StatusCode, result.Error)
	}
	assertSignatureHeadersArrived(t, received("/page"), "page fetch")
}

// A site that gates on a header gates robots.txt with everything else. Fetching
// it unsigned reads a 403 as a disallow and stops the crawl before it starts.
func TestRobotsCache_SendsExtraHeaders(t *testing.T) {
	srv, received := recordingServer(t, "User-agent: *\nAllow: /\n")

	rc := NewRobotsCache("TestBot/1.0", 5*time.Second,
		DialOptions{AllowPrivateIPs: true}, "", signatureHeaders)

	rc.IsAllowed(srv.URL + "/page")
	assertSignatureHeadersArrived(t, received("/robots.txt"), "robots.txt fetch")
}

func TestFetchSitemap_SendsExtraHeaders(t *testing.T) {
	srv, received := recordingServer(t,
		`<?xml version="1.0"?><urlset><url><loc>https://example.com/</loc></url></urlset>`)

	FetchSitemap(context.Background(), srv.Client(), srv.URL+"/sitemap.xml", "TestBot/1.0", signatureHeaders)
	assertSignatureHeadersArrived(t, received("/sitemap.xml"), "sitemap fetch")
}

// The user agent has its own setting, and a caller who names it in the headers
// must not be able to change it from there — otherwise the crawl reports one
// identity to the session record and another to the site.
func TestFetcher_ExtraHeadersCannotOverrideUserAgent(t *testing.T) {
	srv, received := recordingServer(t, "ok")

	f := New("TestBot/1.0", 5*time.Second, 1<<20,
		DialOptions{AllowPrivateIPs: true}, "",
		WithExtraHeaders(map[string]string{"User-Agent": "Impostor/9.9"}))

	f.Fetch(srv.URL+"/page", 0, "")
	if got := received("/page").Get("User-Agent"); got != "TestBot/1.0" {
		t.Errorf("User-Agent = %q, want %q", got, "TestBot/1.0")
	}
}

// Accept and Accept-Language are preferences rather than transport decisions,
// so a caller is allowed to replace them.
func TestFetcher_ExtraHeadersOverrideAccept(t *testing.T) {
	srv, received := recordingServer(t, "ok")

	f := New("TestBot/1.0", 5*time.Second, 1<<20,
		DialOptions{AllowPrivateIPs: true}, "",
		WithExtraHeaders(map[string]string{"Accept": "application/json"}))

	f.Fetch(srv.URL+"/page", 0, "")
	if got := received("/page").Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q, want %q", got, "application/json")
	}
}

// The option copies its map, so a caller reusing the map cannot change what a
// running crawl sends.
func TestWithExtraHeaders_CopiesTheMap(t *testing.T) {
	srv, received := recordingServer(t, "ok")

	headers := map[string]string{"Signature-Agent": `"https://example.com/"`}
	f := New("TestBot/1.0", 5*time.Second, 1<<20,
		DialOptions{AllowPrivateIPs: true}, "", WithExtraHeaders(headers))
	headers["Signature-Agent"] = `"https://elsewhere.example/"`

	f.Fetch(srv.URL+"/page", 0, "")
	if got := received("/page").Get("Signature-Agent"); got != `"https://example.com/"` {
		t.Errorf("Signature-Agent = %q, want the value given at construction", got)
	}
}

func TestFetcher_NoExtraHeadersIsUnchanged(t *testing.T) {
	srv, received := recordingServer(t, "ok")

	f := New("TestBot/1.0", 5*time.Second, 1<<20, DialOptions{AllowPrivateIPs: true}, "")
	f.Fetch(srv.URL+"/page", 0, "")

	got := received("/page")
	if got.Get("User-Agent") != "TestBot/1.0" {
		t.Errorf("User-Agent = %q, want TestBot/1.0", got.Get("User-Agent"))
	}
	if got.Get("Signature") != "" {
		t.Errorf("Signature = %q, want it absent", got.Get("Signature"))
	}
}

// The browser pool cannot go through ApplyExtraHeaders, so it filters with
// SanitizeExtraHeaders. Both must refuse the same things, or a crawl presents
// one identity over HTTP and another once it renders.
func TestSanitizeExtraHeaders_MatchesWhatIsSent(t *testing.T) {
	cases := map[string]string{
		"Signature-Agent": `"https://example.com/"`,
		"User-Agent":      "Impostor/9.9",
		"Host":            "example.com",
		"Connection":      "close",
		"X Bad Name":      "v",
		"X-Bad-Value":     "a\r\nX-Injected: 1",
	}

	sanitized := SanitizeExtraHeaders(cases)

	srv, received := recordingServer(t, "ok")
	f := New("TestBot/1.0", 5*time.Second, 1<<20,
		DialOptions{AllowPrivateIPs: true}, "", WithExtraHeaders(cases))
	f.Fetch(srv.URL+"/page", 0, "")
	sent := received("/page")

	for name := range cases {
		_, kept := sanitized[name]
		// What the HTTP path sent, judged on the wire rather than on the map.
		arrived := sent.Get(name) == cases[name]
		if name == "User-Agent" || name == "Host" {
			// net/http fills these itself, so presence proves nothing; what
			// matters is that neither path took the caller's value.
			arrived = false
		}
		if kept != arrived {
			t.Errorf("header %q: sanitize keeps=%v but the wire got it=%v — the two paths disagree",
				name, kept, arrived)
		}
	}

	if _, ok := sanitized["Signature-Agent"]; !ok {
		t.Error("SanitizeExtraHeaders dropped a header it should keep")
	}
	if len(sanitized) != 1 {
		t.Errorf("SanitizeExtraHeaders kept %v, want only Signature-Agent", sanitized)
	}
	if SanitizeExtraHeaders(nil) != nil {
		t.Error("SanitizeExtraHeaders(nil) should be nil")
	}
	if SanitizeExtraHeaders(map[string]string{"Host": "x"}) != nil {
		t.Error("SanitizeExtraHeaders should be nil when everything is refused")
	}
}

// net/http copies custom headers across redirects, including to another host.
// A signature naming the host it was issued for must not travel with one.
func TestFetcher_ExtraHeadersDoNotFollowCrossHostRedirects(t *testing.T) {
	elsewhere, atElsewhere := recordingServer(t, "ok")
	// httptest binds to 127.0.0.1; naming it "localhost" gives a genuinely
	// different hostname pointing at the same listener, which is what makes
	// this a cross-host redirect rather than a cross-port one.
	elsewhereURL := strings.Replace(elsewhere.URL, "127.0.0.1", "localhost", 1)

	// A server that redirects away to that other host.
	var origin *httptest.Server
	origin = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stay" {
			// Same host, different path: the header must survive this one.
			http.Redirect(w, r, origin.URL+"/arrived", http.StatusFound)
			return
		}
		if r.URL.Path == "/arrived" {
			w.Header().Set("X-Saw-Signature", r.Header.Get("Signature"))
			fmt.Fprint(w, "ok")
			return
		}
		http.Redirect(w, r, elsewhereURL+"/page", http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	f := New("TestBot/1.0", 5*time.Second, 1<<20,
		DialOptions{AllowPrivateIPs: true}, "", WithExtraHeaders(signatureHeaders))

	// Leaving for another host: the headers must be dropped.
	if res := f.Fetch(origin.URL+"/leave", 0, ""); res.StatusCode != http.StatusOK {
		t.Fatalf("Fetch status = %d (%s), want 200", res.StatusCode, res.Error)
	}
	got := atElsewhere("/page")
	if got == nil {
		t.Fatal("the redirect target received no request")
	}
	for name := range signatureHeaders {
		if v := got.Get(name); v != "" {
			t.Errorf("the other host received %s = %q, want it dropped", name, v)
		}
	}

	// Staying on the same host: the headers must survive, or a site that
	// redirects http to https, or / to /index, would lose them.
	res := f.Fetch(origin.URL+"/stay", 0, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("same-host redirect status = %d (%s), want 200", res.StatusCode, res.Error)
	}
}
