package cfsolve

import (
	"bytes"
	"strings"
)

// cfBodyMarkers are byte patterns found in Cloudflare challenge pages.
var cfBodyMarkers = [][]byte{
	[]byte("/cdn-cgi/challenge-platform"),
	[]byte("cf_chl_opt"),
	[]byte("Just a moment"),
}

// IsCFChallenge returns true if the response looks like a Cloudflare challenge page.
// It requires all three conditions: status 403/503, a CF header, and a body marker.
func IsCFChallenge(statusCode int, headers map[string]string, body []byte) bool {
	if statusCode != 403 && statusCode != 503 {
		return false
	}
	if !hasCFHeader(headers) {
		return false
	}
	return hasCFBodyMarker(body)
}

// hasCFHeader checks for any Cloudflare-specific response header.
func hasCFHeader(headers map[string]string) bool {
	if _, ok := headers["Cf-Ray"]; ok {
		return true
	}
	if _, ok := headers["Cf-Mitigated"]; ok {
		return true
	}
	if server, ok := headers["Server"]; ok {
		if strings.Contains(strings.ToLower(server), "cloudflare") {
			return true
		}
	}
	return false
}

// hasCFBodyMarker checks for Cloudflare challenge markers in the response body.
func hasCFBodyMarker(body []byte) bool {
	for _, marker := range cfBodyMarkers {
		if bytes.Contains(body, marker) {
			return true
		}
	}
	return false
}
