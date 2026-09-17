package renderer

import "testing"

// A signature identifies the crawler to one site. Attached to every request a
// rendered page makes, it would reach each third-party script, tag and iframe
// the page loads — each of which could replay it against that site until it
// expires. These tests pin the rule that decides who gets it.
func TestSameHost(t *testing.T) {
	tests := []struct {
		name        string
		requestHost string
		pageHost    string
		want        bool
	}{
		{"the site itself", "example.com", "example.com", true},
		{"case does not matter", "EXAMPLE.com", "example.com", true},
		{"a third-party CDN", "cdn.analytics-vendor.com", "example.com", false},
		{"a subdomain is not the same host", "static.example.com", "example.com", false},
		{"the parent is not the same host", "example.com", "www.example.com", false},
		{"a lookalike suffix", "notexample.com", "example.com", false},
		{"a lookalike prefix", "example.com.evil.test", "example.com", false},
		{"an empty request host", "", "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameHost(tt.requestHost, tt.pageHost); got != tt.want {
				t.Errorf("sameHost(%q, %q) = %v, want %v", tt.requestHost, tt.pageHost, got, tt.want)
			}
		})
	}
}

func TestHostOf(t *testing.T) {
	tests := []struct {
		rawURL string
		want   string
	}{
		{"https://example.com/page", "example.com"},
		{"https://example.com:8443/page", "example.com"},
		{"http://EXAMPLE.com/", "EXAMPLE.com"},
		{"", ""},
		{"://not a url", ""},
		{"not-a-url", ""},
	}

	for _, tt := range tests {
		t.Run(tt.rawURL, func(t *testing.T) {
			if got := hostOf(tt.rawURL); got != tt.want {
				t.Errorf("hostOf(%q) = %q, want %q", tt.rawURL, got, tt.want)
			}
		})
	}
}

// An unreadable page URL must attach nothing rather than attach to everything:
// the safe answer to "is this the site we are crawling?" is no.
func TestHostOf_UnreadableURLAttachesNothing(t *testing.T) {
	if sameHost("example.com", hostOf("://not a url")) {
		t.Error("headers would be attached although the page host could not be read")
	}
}
