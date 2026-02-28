package fetcher

import (
	"context"
	"fmt"
	"net"

	utls "github.com/refraction-networking/utls"
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

// utlsDialTLSContext returns a DialTLSContext function that performs a TCP dial
// via safeDial (preserving SSRF protection), then upgrades the connection using
// utls with the chosen browser fingerprint.
func utlsDialTLSContext(profile TLSProfile, safeDial func(ctx context.Context, network, addr string) (net.Conn, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	helloID, err := clientHelloID(profile)
	if err != nil {
		// Fall back to standard Go TLS if profile is invalid — should not happen
		// because callers validate, but be defensive.
		return nil
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// 1. TCP dial through the safe dialer (DNS resolution + private IP check)
		rawConn, err := safeDial(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		// Extract SNI hostname from addr (host:port)
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			rawConn.Close()
			return nil, fmt.Errorf("splitting host/port: %w", err)
		}

		// 2. Wrap with utls for browser-like TLS handshake
		tlsConn := utls.UClient(rawConn, &utls.Config{ServerName: host}, helloID)

		if err := tlsConn.HandshakeContext(ctx); err != nil {
			tlsConn.Close()
			return nil, fmt.Errorf("utls handshake: %w", err)
		}

		return tlsConn, nil
	}
}
