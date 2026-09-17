package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
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
	"host":                "computed from the URL",
	"content-length":      "computed from the request body",
	"user-agent":          "set by the user agent setting",
	"connection":          "governed by the transport",
	"keep-alive":          "governed by the transport",
	"proxy-connection":    "governed by the transport",
	"proxy-authenticate":  "governed by the proxy",
	"proxy-authorization": "governed by the proxy",
	"te":                  "governed by the transport",
	"transfer-encoding":   "governed by the transport",
	"upgrade":             "governed by the transport",
	"trailer":             "governed by the transport",
	"via":                 "managed by proxies",
	"forwarded":           "managed by proxies",
	"x-forwarded-for":     "managed by proxies",
	"x-forwarded-host":    "managed by proxies",
	"x-forwarded-proto":   "managed by proxies",
	"x-forwarded-port":    "managed by proxies",
	"x-real-ip":           "managed by proxies",
}

// HeaderPolicy binds caller-supplied headers to a fixed set of origins.
//
// A policy is immutable after construction. NewHeaderPolicy copies both the
// caller's header map and its seed origins; HeadersForURL returns a new map on
// every call. Callers may therefore safely reuse or discard their input maps,
// and may not mutate a running policy through a returned map.
//
// An origin is the URL's HTTP(S) scheme, case-insensitive host, and effective
// port. The default ports (80 and 443) are equivalent to their explicit
// spellings. Paths, queries, fragments, and userinfo do not affect matching.
// Headers are never granted to an origin merely because a request or redirect
// reached it; it must have been present in seedURLs when the policy was made.
type HeaderPolicy struct {
	headers map[string]string
	origins map[headerOrigin]struct{}
}

type headerOrigin struct {
	scheme string
	host   string
	port   int
}

// NewHeaderPolicy creates an immutable policy for headers on the supplied
// seed origins. Invalid or transport-owned headers are silently omitted; the
// configuration path should call ValidateExtraHeaders first to report such an
// error to the user.
func NewHeaderPolicy(headers map[string]string, seedURLs []string) *HeaderPolicy {
	p := &HeaderPolicy{
		headers: make(map[string]string),
		origins: make(map[headerOrigin]struct{}),
	}

	// Sort names so that a map containing differently-cased spellings has a
	// deterministic result even when a caller skipped validation. Validated
	// configuration rejects that collision before construction.
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := headers[name]
		if !sendable(name, value) {
			continue
		}
		canonical := http.CanonicalHeaderKey(name)
		if _, exists := p.headers[canonical]; exists {
			continue
		}
		p.headers[canonical] = value
	}

	for _, raw := range seedURLs {
		if origin, ok := normalizeHeaderOrigin(raw); ok {
			p.origins[origin] = struct{}{}
		}
	}
	if len(p.headers) == 0 || len(p.origins) == 0 {
		return nil
	}
	return p
}

// HeadersForURL returns a copy of the policy headers when raw belongs to an
// explicitly trusted seed origin. It returns nil for an invalid URL, a
// non-HTTP(S) URL, or an origin that was not supplied to NewHeaderPolicy.
// Modifying the returned map never changes the policy.
func (p *HeaderPolicy) HeadersForURL(raw string) map[string]string {
	if p == nil || len(p.headers) == 0 || !p.allowsURL(raw) {
		return nil
	}
	return p.copyHeaders()
}

// AllowsURL reports whether raw is one of this policy's explicitly trusted
// HTTP(S) origins. It performs no header allocation and is useful to callers
// that need the same origin decision for control flow.
func (p *HeaderPolicy) AllowsURL(raw string) bool {
	return p != nil && p.allowsURL(raw)
}

// Apply removes this policy's headers from req and adds them only when req's
// URL is an explicitly trusted origin. Existing request defaults, cookies, and
// headers unrelated to this policy are preserved.
func (p *HeaderPolicy) Apply(req *http.Request) {
	if p == nil || req == nil {
		return
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	p.Strip(req)
	for name, value := range p.HeadersForURL(requestURLString(req)) {
		req.Header.Set(name, value)
	}
}

// Strip removes every header configured by this policy from req, comparing
// names case-insensitively. It leaves User-Agent, cookies, and all other
// request state untouched. Strip is safe on a nil policy or request.
func (p *HeaderPolicy) Strip(req *http.Request) {
	if p == nil || req == nil || req.Header == nil {
		return
	}
	for configured := range p.headers {
		for actual := range req.Header {
			if strings.EqualFold(actual, configured) {
				delete(req.Header, actual)
			}
		}
	}
}

func (p *HeaderPolicy) copyHeaders() map[string]string {
	if p == nil || len(p.headers) == 0 {
		return nil
	}
	result := make(map[string]string, len(p.headers))
	for name, value := range p.headers {
		result[name] = value
	}
	return result
}

func (p *HeaderPolicy) allowsURL(raw string) bool {
	if p == nil || len(p.origins) == 0 {
		return false
	}
	origin, ok := normalizeHeaderOrigin(raw)
	if !ok {
		return false
	}
	_, ok = p.origins[origin]
	return ok
}

func requestURLString(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.String()
}

func normalizeHeaderOrigin(raw string) (headerOrigin, bool) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Host == "" {
		return headerOrigin{}, false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return headerOrigin{}, false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return headerOrigin{}, false
	}

	portText := u.Port()
	port := 0
	if portText == "" {
		if strings.HasSuffix(u.Host, ":") {
			// A trailing colon is not an omitted default port. url.Parse accepts
			// it, but treating it as a trusted origin would be surprising.
			return headerOrigin{}, false
		}
		if scheme == "http" {
			port = 80
		} else {
			port = 443
		}
	} else {
		parsedPort, parseErr := strconv.Atoi(portText)
		if parseErr != nil || parsedPort < 1 || parsedPort > 65535 {
			return headerOrigin{}, false
		}
		port = parsedPort
	}

	return headerOrigin{scheme: scheme, host: host, port: port}, true
}

// headerPolicyContextKey carries redirect trust through net/http's copied
// request chain. It is private so callers cannot accidentally grant trust.
type headerPolicyContextKey struct{}

type headerPolicyRedirectState struct {
	trusted bool
}

// policyRoundTripper applies a policy at the last point before bytes leave
// the process. This protects callers that reuse a custom http.Client whose
// redirect handling copied headers before a CheckRedirect hook ran.
type policyRoundTripper struct {
	base   http.RoundTripper
	policy *HeaderPolicy
}

func (t *policyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if t == nil || t.policy == nil {
		if t == nil || t.base == nil {
			return http.DefaultTransport.RoundTrip(req)
		}
		return t.base.RoundTrip(req)
	}
	state, ok := req.Context().Value(headerPolicyContextKey{}).(*headerPolicyRedirectState)
	if !ok || state == nil {
		// RoundTrip is the only hook available when a caller wraps a generic
		// http.Client without changing its CheckRedirect callback. Mutating the
		// request before the client builds its redirect copy lets the state
		// pointer follow that chain through net/http's original request context.
		state = &headerPolicyRedirectState{trusted: t.policy.AllowsURL(requestURLString(req))}
		*req = *req.WithContext(context.WithValue(req.Context(), headerPolicyContextKey{}, state))
	}
	if state.trusted && !t.policy.AllowsURL(requestURLString(req)) {
		state.trusted = false
	}
	clone := req.Clone(req.Context())
	if !state.trusted {
		t.policy.Strip(clone)
	} else {
		t.policy.Apply(clone)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

// WrapTransport returns a RoundTripper that applies p to every outgoing
// request. Redirect-aware callers should also use the client's policy redirect
// hook; Fetcher, RobotsCache, and sitemap helpers install both protections.
func (p *HeaderPolicy) WrapTransport(base http.RoundTripper) http.RoundTripper {
	if p == nil {
		if base == nil {
			return http.DefaultTransport
		}
		return base
	}
	return &policyRoundTripper{base: base, policy: p}
}

func preparePolicyRequest(req *http.Request, policy *HeaderPolicy) *http.Request {
	if req == nil || policy == nil {
		return req
	}
	trusted := policy.AllowsURL(requestURLString(req))
	state := &headerPolicyRedirectState{trusted: trusted}
	req = req.WithContext(context.WithValue(req.Context(), headerPolicyContextKey{}, state))
	if trusted {
		policy.Apply(req)
	} else {
		policy.Strip(req)
	}
	return req
}

func redirectChainTrusted(policy *HeaderPolicy, req *http.Request, via []*http.Request) bool {
	if policy == nil {
		return true
	}
	for _, previous := range via {
		if previous == nil || !policy.AllowsURL(requestURLString(previous)) {
			return false
		}
	}
	return req != nil && policy.AllowsURL(requestURLString(req))
}

// policyRedirectHook composes a client's existing redirect callback with the
// policy. The caller callback runs first so it can inspect or adjust the final
// target; policy enforcement runs last, after all callback mutations.
func policyRedirectHook(policy *HeaderPolicy, existing func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if existing != nil {
			if err := existing(req, via); err != nil {
				if policy != nil {
					policy.Strip(req)
				}
				return err
			}
		}
		if policy == nil {
			return nil
		}

		trusted := redirectChainTrusted(policy, req, via)
		state := &headerPolicyRedirectState{trusted: trusted}
		if len(via) > 0 && via[0] != nil {
			ctx := context.WithValue(via[0].Context(), headerPolicyContextKey{}, state)
			// net/http uses the first request's context for every subsequent
			// redirect. Updating the pointed-to request makes the sticky trust
			// decision survive a return to an allowed origin.
			*via[0] = *via[0].WithContext(ctx)
			*req = *req.WithContext(ctx)
		} else {
			*req = *req.WithContext(context.WithValue(req.Context(), headerPolicyContextKey{}, state))
		}
		if trusted {
			policy.Apply(req)
		} else {
			policy.Strip(req)
		}
		return nil
	}
}

func cloneClientWithHeaderPolicy(client *http.Client, policy *HeaderPolicy) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	if policy == nil {
		return client
	}
	copy := *client
	copy.Transport = policy.WrapTransport(client.Transport)
	copy.CheckRedirect = policyRedirectHook(policy, client.CheckRedirect)
	return &copy
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
// Carriage return, line feed, NUL, and DEL are refused because control
// characters do not belong in a configured field value and could be handled
// inconsistently by an intermediary.
func isValidHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == '\r' || c == '\n' || c == 0 || c == 0x7f {
			return false
		}
		if c < 0x20 && c != '\t' {
			return false
		}
	}
	return true
}

// SanitizeExtraHeaders returns a copy containing only headers that pass the
// crawler's syntax and transport ownership checks. New code should prefer
// NewHeaderPolicy, which also binds those headers to explicit origins.
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
// single rule behind the legacy helpers and HeaderPolicy construction.
func sendable(name, value string) bool {
	if name == "" || !isValidHeaderName(name) || !isValidHeaderValue(value) {
		return false
	}
	return reservedHeaders[strings.ToLower(name)] == ""
}

// ApplyExtraHeaders sets sanitized headers on req, replacing any the caller
// names. Deprecated: use HeaderPolicy.Apply so headers cannot cross origins.
func ApplyExtraHeaders(req *http.Request, headers map[string]string) {
	for name, value := range headers {
		if !sendable(name, value) {
			continue
		}
		req.Header.Set(name, value)
	}
}
