package fetcher

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// TLSProfile selects which browser TLS fingerprint to mimic.
// Empty string means standard Go TLS (no mimicry).
type TLSProfile string

const (
	TLSChrome  TLSProfile = "chrome"
	TLSFirefox TLSProfile = "firefox"
	TLSEdge    TLSProfile = "edge"
)

// clientHelloID maps a TLSProfile to the corresponding utls ClientHelloID.
func clientHelloID(p TLSProfile) (utls.ClientHelloID, error) {
	switch p {
	case TLSChrome, TLSEdge:
		return utls.HelloChrome_Auto, nil
	case TLSFirefox:
		return utls.HelloFirefox_Auto, nil
	default:
		return utls.ClientHelloID{}, fmt.Errorf("unknown TLS profile: %q", p)
	}
}

// utlsTransport returns an http.RoundTripper that uses uTLS for the TLS
// handshake (preserving browser fingerprint) and correctly handles both
// HTTP/1.1 and HTTP/2 depending on the ALPN negotiated during the handshake.
//
// The standard http.Transport only speaks HTTP/1.x when DialTLSContext is set,
// even if the server negotiates HTTP/2 via ALPN. This causes "malformed HTTP
// response" errors. We solve this by inspecting the negotiated protocol after
// the handshake and routing to either an http2.Transport or a standard
// http.Transport accordingly.
func utlsTransport(profile TLSProfile, safeDial func(ctx context.Context, network, addr string) (net.Conn, error), baseTransport *http.Transport) http.RoundTripper {
	helloID, err := clientHelloID(profile)
	if err != nil {
		return baseTransport
	}

	dialUTLS := func(ctx context.Context, network, addr string) (net.Conn, string, error) {
		rawConn, err := safeDial(ctx, network, addr)
		if err != nil {
			return nil, "", err
		}

		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			rawConn.Close()
			return nil, "", fmt.Errorf("splitting host/port: %w", err)
		}

		tlsConn := utls.UClient(rawConn, &utls.Config{ServerName: host}, helloID)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			tlsConn.Close()
			return nil, "", fmt.Errorf("utls handshake: %w", err)
		}

		proto := tlsConn.ConnectionState().NegotiatedProtocol
		return tlsConn, proto, nil
	}

	// HTTP/2 transport for connections that negotiate h2
	h2Transport := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			conn, _, err := dialUTLS(ctx, network, addr)
			return conn, err
		},
	}

	// HTTP/1.1 transport — uses DialTLSContext for the uTLS handshake
	h1Transport := baseTransport.Clone()
	h1Transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, _, err := dialUTLS(ctx, network, addr)
		return conn, err
	}

	return &alpnSwitchTransport{
		dialUTLS: dialUTLS,
		h1:       h1Transport,
		h2:       h2Transport,
	}
}

// alpnSwitchTransport inspects the ALPN negotiated by uTLS and delegates to
// the appropriate HTTP/1.1 or HTTP/2 transport.
type alpnSwitchTransport struct {
	dialUTLS func(ctx context.Context, network, addr string) (net.Conn, string, error)
	h1       *http.Transport
	h2       *http2.Transport

	// Cache negotiated protocol per host to avoid re-dialing.
	mu       sync.Mutex
	protoMap map[string]string // host:port -> "h2" or "http/1.1"
}

func (t *alpnSwitchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	addr := req.URL.Host
	if !hasPort(addr) {
		if req.URL.Scheme == "https" {
			addr += ":443"
		} else {
			addr += ":80"
		}
	}

	// Check cache first
	t.mu.Lock()
	proto, ok := t.protoMap[addr]
	t.mu.Unlock()

	if !ok {
		// Probe the protocol by dialing
		conn, negotiated, err := t.dialUTLS(req.Context(), "tcp", addr)
		if err != nil {
			return nil, err
		}
		// Close the probe connection — the transport will open its own.
		conn.Close()

		proto = negotiated
		if proto == "" {
			proto = "http/1.1"
		}

		t.mu.Lock()
		if t.protoMap == nil {
			t.protoMap = make(map[string]string)
		}
		t.protoMap[addr] = proto
		t.mu.Unlock()
	}

	if proto == "h2" {
		return t.h2.RoundTrip(req)
	}

	return t.h1.RoundTrip(req)
}

func hasPort(host string) bool {
	_, _, err := net.SplitHostPort(host)
	return err == nil
}
