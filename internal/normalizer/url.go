package normalizer

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/purell"
)

// Flags used for URL normalization
const normalizationFlags = purell.FlagLowercaseScheme |
	purell.FlagLowercaseHost |
	purell.FlagUppercaseEscapes |
	purell.FlagDecodeUnnecessaryEscapes |
	purell.FlagRemoveDefaultPort |
	purell.FlagRemoveEmptyQuerySeparator |
	purell.FlagRemoveFragment |
	purell.FlagRemoveDuplicateSlashes |
	purell.FlagSortQuery

// trackingParams are URL parameters used for tracking that should be removed.
var trackingParams = map[string]struct{}{
	"utm_source":   {},
	"utm_medium":   {},
	"utm_campaign": {},
	"utm_term":     {},
	"utm_content":  {},
	"fbclid":       {},
	"gclid":        {},
	"mc_cid":       {},
	"mc_eid":       {},
}

// EnsureScheme prepends "http://" to a URL if it has no scheme.
// This is intended for seed URLs entered by users (e.g. "blog.axe-net.fr").
func EnsureScheme(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return rawURL
	}
	if !strings.Contains(rawURL, "://") {
		return "http://" + rawURL
	}
	return rawURL
}

// Normalize normalizes a URL string for deduplication.
func Normalize(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil
	}

	normalized, err := purell.NormalizeURLString(rawURL, normalizationFlags)
	if err != nil {
		return "", err
	}

	normalized = removeTrackingParams(normalized)

	// Ensure root URLs have a trailing slash so that "https://example.com"
	// and "https://example.com/" are treated as the same page (they are,
	// per RFC 3986 — the empty path at the root is equivalent to "/").
	// Without this, a bare-host seed and any relative link resolved by
	// url.ResolveReference (which produces "host/") create two separate
	// dedup entries for what is a single page.
	if u, perr := url.Parse(normalized); perr == nil && u.Host != "" && u.Path == "" {
		u.Path = "/"
		normalized = u.String()
	}

	return normalized, nil
}

// Resolve resolves a relative URL against a base URL and normalizes the result.
func Resolve(base, ref string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	resolved := baseURL.ResolveReference(refURL).String()
	return Normalize(resolved)
}

func removeTrackingParams(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	changed := false
	for param := range trackingParams {
		if q.Has(param) {
			q.Del(param)
			changed = true
		}
	}
	if !changed {
		return rawURL
	}
	u.RawQuery = q.Encode()
	return u.String()
}
