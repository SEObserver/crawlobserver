package normalizer

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"lowercase scheme", "HTTP://Example.COM/path", "http://example.com/path", false},
		{"lowercase host", "https://EXAMPLE.COM/Path", "https://example.com/Path", false},
		{"remove default port 80", "http://example.com:80/path", "http://example.com/path", false},
		{"remove default port 443", "https://example.com:443/path", "https://example.com/path", false},
		{"keep non-default port", "http://example.com:8080/path", "http://example.com:8080/path", false},
		{"remove fragment", "https://example.com/page#section", "https://example.com/page", false},
		{"preserve trailing slash", "https://example.com/path/", "https://example.com/path/", false},
		{"remove duplicate slashes", "https://example.com//path///to", "https://example.com/path/to", false},
		{"sort query params", "https://example.com/page?z=1&a=2", "https://example.com/page?a=2&z=1", false},
		{"remove utm_source", "https://example.com/page?utm_source=google&real=1", "https://example.com/page?real=1", false},
		{"remove utm_medium", "https://example.com/?utm_medium=email", "https://example.com/", false},
		{"remove utm_campaign", "https://example.com/?utm_campaign=sale&keep=1", "https://example.com/?keep=1", false},
		{"remove fbclid", "https://example.com/?fbclid=abc123", "https://example.com/", false},
		{"remove gclid", "https://example.com/?gclid=xyz&q=test", "https://example.com/?q=test", false},
		{"remove all tracking params", "https://example.com/?utm_source=x&utm_medium=y&utm_campaign=z&utm_term=a&utm_content=b&fbclid=c&gclid=d", "https://example.com/", false},
		{"empty string", "", "", false},
		{"whitespace", "  https://example.com  ", "https://example.com/", false},
		{"bare host gets trailing slash", "https://example.com", "https://example.com/", false},
		{"bare host with port gets trailing slash", "https://example.com:8080", "https://example.com:8080/", false},
		{"non-root path is unchanged", "https://example.com/page", "https://example.com/page", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Normalize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Normalize() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureScheme(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bare domain", "blog.axe-net.fr", "http://blog.axe-net.fr"},
		{"domain with www", "www.example.com", "http://www.example.com"},
		{"domain with path", "example.com/path/to/page", "http://example.com/path/to/page"},
		{"already http", "http://example.com", "http://example.com"},
		{"already https", "https://example.com", "https://example.com"},
		{"with whitespace", "  blog.axe-net.fr  ", "http://blog.axe-net.fr"},
		{"empty string", "", ""},
		{"ftp scheme", "ftp://files.example.com", "ftp://files.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnsureScheme(tt.input)
			if got != tt.want {
				t.Errorf("EnsureScheme(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		ref     string
		want    string
		wantErr bool
	}{
		{"absolute URL", "https://example.com/page", "https://other.com/path", "https://other.com/path", false},
		{"relative path", "https://example.com/dir/page", "other", "https://example.com/dir/other", false},
		{"root relative", "https://example.com/dir/page", "/root", "https://example.com/root", false},
		{"with fragment removed", "https://example.com/page", "/other#frag", "https://example.com/other", false},
		{"with tracking params removed", "https://example.com/page", "/other?utm_source=test&real=1", "https://example.com/other?real=1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.base, tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}
