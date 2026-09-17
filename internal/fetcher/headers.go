package fetcher

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Limits on caller-supplied headers. They are generous enough for the
// signature headers of RFC 9421, whose Signature-Input can run long, and tight
// enough that a malformed configuration cannot grow a request without bound.
const (
	MaxExtraHeaders         = 32
	MaxExtraHeaderNameLen   = 128
	MaxExtraHeaderValueLen  = 8192
	maxExtraHeadersTotalLen = 32 * 1024
)

// reservedHeaders are refused from caller-supplied headers.
//
// Host and Content-Length are computed by net/http from the request itself, so
// setting them here would either be ignored or produce a request that
// contradicts what is actually sent. The hop-by-hop headers govern the
// connection rather than the message, and the transport owns them. User-Agent
// is refused because it has its own setting, and accepting it in two places
// would leave no answer to which one wins.
var reservedHeaders = map[string]string{
	"host":              "computed from the URL",
	"content-length":    "computed from the request body",
	"user-agent":        "set by the user agent setting",
	"connection":        "governed by the transport",
	"transfer-encoding": "governed by the transport",
	"upgrade":           "governed by the transport",
	"trailer":           "governed by the transport",
}

// ValidateExtraHeaders reports whether headers may be sent on crawl requests.
//
// It returns an error naming the offending header, since these values are
// entered by hand and a caller who mistypes one deserves to be told which.
func ValidateExtraHeaders(headers map[string]string) error {
	if len(headers) > MaxExtraHeaders {
		return fmt.Errorf("too many headers: %d, maximum is %d", len(headers), MaxExtraHeaders)
	}

	// Sorted so that a configuration with several faults always reports the
	// same one, rather than a different name on every run.
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	total := 0
	seen := make(map[string]string, len(headers))
	for _, name := range names {
		value := headers[name]

		if name == "" {
			return fmt.Errorf("header name is empty")
		}
		if len(name) > MaxExtraHeaderNameLen {
			return fmt.Errorf("header %q: name is longer than %d characters", name, MaxExtraHeaderNameLen)
		}
		if !isValidHeaderName(name) {
			return fmt.Errorf("header %q: name contains a character that is not allowed", name)
		}
		if len(value) > MaxExtraHeaderValueLen {
			return fmt.Errorf("header %q: value is longer than %d characters", name, MaxExtraHeaderValueLen)
		}
		if !isValidHeaderValue(value) {
			return fmt.Errorf("header %q: value contains a control character", name)
		}

		canonical := strings.ToLower(name)
		if reason := reservedHeaders[canonical]; reason != "" {
			return fmt.Errorf("header %q cannot be set: it is %s", name, reason)
		}
		// Two spellings of one header reach the wire as a single header, so the
		// pair would silently lose one value. Refusing says which two collide.
		if other, ok := seen[canonical]; ok {
			return fmt.Errorf("headers %q and %q are the same header", other, name)
		}
		seen[canonical] = name

		total += len(name) + len(value)
	}

	if total > maxExtraHeadersTotalLen {
		return fmt.Errorf("headers are %d characters in total, maximum is %d", total, maxExtraHeadersTotalLen)
	}
	return nil
}

// isValidHeaderName reports whether name is a token as RFC 9110 defines it.
func isValidHeaderName(name string) bool {
	for i := 0; i < len(name); i++ {
		if !isTokenChar(name[i]) {
			return false
		}
	}
	return true
}

func isTokenChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}

// isValidHeaderValue reports whether value can be sent as a field value.
// Carriage return, line feed and NUL are refused because they would end the
// field early and let a value write headers of its own.
func isValidHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == '\r' || c == '\n' || c == 0 {
			return false
		}
		if c < 0x20 && c != '\t' {
			return false
		}
	}
	return true
}

// SanitizeExtraHeaders returns the headers that may be sent, dropping any the
// crawler refuses to set.
//
// It exists so that a consumer which cannot go through ApplyExtraHeaders — the
// browser pool sets its headers through the DevTools protocol, where none of
// net/http's protections apply — filters by the same rule rather than one of
// its own. Two filters would drift, and the drift would show up as a crawl
// presenting one identity over HTTP and another once it rendered.
func SanitizeExtraHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	safe := make(map[string]string, len(headers))
	for name, value := range headers {
		if sendable(name, value) {
			safe[name] = value
		}
	}
	if len(safe) == 0 {
		return nil
	}
	return safe
}

// sendable reports whether a header may go out on a crawl request. It is the
// single rule behind both ApplyExtraHeaders and SanitizeExtraHeaders.
func sendable(name, value string) bool {
	if name == "" || !isValidHeaderName(name) || !isValidHeaderValue(value) {
		return false
	}
	return reservedHeaders[strings.ToLower(name)] == ""
}

// ApplyExtraHeaders sets headers on req, replacing any the caller names.
//
// Exported because the crawl engine checks a page's own resources with a
// client of its own, and those requests have to carry what the page carried.
//
// It runs after the defaults so that a caller can override Accept or
// Accept-Language, which are preferences rather than transport decisions.
// Invalid headers are skipped rather than sent: they are refused when they are
// stored, and a request is not the place to discover one that slipped through.
func ApplyExtraHeaders(req *http.Request, headers map[string]string) {
	for name, value := range headers {
		if !sendable(name, value) {
			continue
		}
		req.Header.Set(name, value)
	}
}
