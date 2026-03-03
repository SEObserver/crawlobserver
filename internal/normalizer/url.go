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
