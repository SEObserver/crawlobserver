package storage

import (
	"testing"
)

func TestNotRedirectedFilter_Constant(t *testing.T) {
	expected := "(final_url = '' OR final_url = url)"
	if notRedirectedFilter != expected {
		t.Errorf("notRedirectedFilter = %q, want %q", notRedirectedFilter, expected)
	}
}

// TestIsFollowedRedirect mirrors the SQL filter in pure Go.
// Each case is annotated with its real-world likelihood given the fetcher pipeline:
//   - url   = normalizer.Normalize() output (lowercase host, sorted query, no fragment, no //, no default port)
//   - final = resp.Request.URL.String() (Go net/http: lowercase scheme+host only)
func TestIsFollowedRedirect(t *testing.T) {
	isNotRedirected := func(url, finalURL string) bool {
		return finalURL == "" || finalURL == url
	}

	tests := []struct {
		name     string
		url      string
		finalURL string
		want     bool // true = included (not a followed redirect)
	}{
		// ── Included pages ────────────────────────────────────────────

		// Fetch failed (timeout, DNS) → FinalURL stays "". SEO: keep for error audit.
		{"empty final_url — fetch error or legacy data", "https://example.com/page", "", true},
		// No redirect occurred. The normal case (~95% of pages).
		{"final_url == url — no redirect", "https://example.com/page", "https://example.com/page", true},
		// Server 302→same URL (e.g. cookie check). Same content, keep it.
		{"self-redirect — same URL after redirect loop", "https://example.com/page", "https://example.com/page", true},

		// ── Excluded: real-world redirects ────────────────────────────

		// Classic 301 moved page. SEO: /old is just an alias, exclude.
		{"301 path change /old → /new", "https://example.com/old", "https://example.com/new", false},
		// Apache mod_dir / Nginx try_files adds trailing slash. Very common.
		{"trailing slash added by server", "https://example.com/products", "https://example.com/products/", false},
		// Next.js / some CDNs strip trailing slash. Less common but real.
		{"trailing slash removed by server", "https://example.com/products/", "https://example.com/products", false},
		// Near-universal since 2018. Seed may start as http://.
		{"http → https upgrade", "http://example.com/page", "https://example.com/page", false},
		// Multi-hop chain stored as first → final. Common (http→https→www→slash).
		{"redirect chain /a → /b → /c stored as /a → /c", "https://example.com/a", "https://example.com/c", false},
		// WWW canonicalization — daily SEO audit finding.
		{"www added", "https://example.com/page", "https://www.example.com/page", false},
		{"www removed", "https://www.example.com/page", "https://example.com/page", false},
		// Subdomain consolidation (blog.example.com → example.com/blog).
		{"subdomain redirect", "https://blog.example.com/post", "https://example.com/blog/post", false},
		// Domain migration.
		{"cross-domain redirect", "https://old-domain.com/page", "https://new-domain.com/page", false},
		// The most realistic multi-hop: http + no-www + no-slash → https + www + slash.
		{"combined http+www+slash", "http://example.com/products", "https://www.example.com/products/", false},
		// Server normalizes path case (IIS, some CMS). Path not lowercased by normalizer.
		{"path case normalized by server", "https://example.com/Page", "https://example.com/page", false},
		// Server adds session/lang param on redirect.
		{"query param added by server", "https://example.com/page", "https://example.com/page?ref=internal", false},

		// ── Excluded: theoretically possible but unlikely with our normalizer ──

		// Normalizer already lowercases host → both sides match. Defence in depth.
		{"[unlikely] domain case — normalizer lowercases both", "https://Example.COM/page", "https://example.com/page", false},
		// Normalizer strips UTM from url before crawl → server never sees them. Defence in depth.
		{"[unlikely] UTM stripped by server — normalizer already removes them", "https://example.com/page?utm_source=google", "https://example.com/page", false},
		// Normalizer sorts query → url is already sorted. Only if server re-sorts differently.
		{"[unlikely] query reordered — normalizer sorts query params", "https://example.com/page?a=1&b=2", "https://example.com/page?b=2&a=1", false},
		// Both Go net/http and normalizer encode consistently. Possible with exotic servers.
		{"[unlikely] percent-encoding mismatch", "https://example.com/my%20page", "https://example.com/my page", false},
		{"[unlikely] UTF-8 encoding mismatch", "https://example.com/caf%C3%A9", "https://example.com/café", false},
		// https→http downgrade is blocked by most clients. Almost never real.
		{"[unlikely] https → http downgrade", "https://example.com/page", "http://example.com/page", false},

		// ── Excluded: cannot happen with our normalizer pipeline ──────

		// Normalizer applies FlagRemoveFragment → url never has #fragment.
		{"[impossible] fragment stripped — normalizer removes fragments", "https://example.com/page#section", "https://example.com/page", false},
		// Normalizer applies FlagRemoveDuplicateSlashes → url never has //.
		{"[impossible] double slash — normalizer deduplicates slashes", "https://example.com//page", "https://example.com/page", false},
		// Go url.Parse resolves ./ and ../ → normalizer.Resolve cleans them.
		{"[impossible] dot segment ./ — resolved by url.Parse", "https://example.com/a/./b", "https://example.com/a/b", false},
		{"[impossible] dot-dot segment ../ — resolved by url.Parse", "https://example.com/a/x/../b", "https://example.com/a/b", false},
		// Normalizer applies FlagRemoveDefaultPort; Go omits default port too.
		{"[impossible] explicit :443 — both sides omit default port", "https://example.com/page", "https://example.com:443/page", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isNotRedirected(tc.url, tc.finalURL)
			if got != tc.want {
				t.Errorf("isNotRedirected(url=%q, final=%q) = %v, want %v", tc.url, tc.finalURL, got, tc.want)
			}
		})
	}
}

// TestNotRedirectedFilter_SQLSyntax verifies the constant is safe to concatenate in SQL.
func TestNotRedirectedFilter_SQLSyntax(t *testing.T) {
	query := "SELECT count() FROM pages WHERE crawl_session_id = ? AND " + notRedirectedFilter
	if query == "" {
		t.Fatal("query is empty")
	}
	opens := 0
	for _, c := range notRedirectedFilter {
		switch c {
		case '(':
			opens++
		case ')':
			opens--
		}
		if opens < 0 {
			t.Fatal("unbalanced parentheses: closing before opening")
		}
	}
	if opens != 0 {
		t.Errorf("unbalanced parentheses: %d unclosed", opens)
	}
	_ = "WHERE a = 1 AND " + notRedirectedFilter + " AND b = 2"
}
