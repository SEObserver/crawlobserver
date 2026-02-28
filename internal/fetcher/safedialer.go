package fetcher

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// ErrPrivateIP is returned when a dial attempts to connect to a private/reserved IP.
var ErrPrivateIP = errors.New("connection to private/reserved IP address is blocked")

// privateRanges contains all RFC-defined private and reserved IPv4/IPv6 ranges.
var privateRanges []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // Link-local
		"0.0.0.0/8",      // Current network
		"::1/128",         // IPv6 loopback
		"fc00::/7",        // IPv6 unique local
		"fe80::/10",       // IPv6 link-local
	} {
		_, ipNet, _ := net.ParseCIDR(cidr)
		privateRanges = append(privateRanges, ipNet)
	}
}

// IsPrivateIP checks if an IP address belongs to a private or reserved range.
func IsPrivateIP(ip net.IP) bool {
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

// SafeDialContext returns a DialContext function that blocks connections to private IPs
// after DNS resolution (anti DNS-rebinding). When allowPrivate is true, no filtering is applied.
func SafeDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if allowPrivate {
		return dialer.DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		// If host is already an IP literal, check directly
		if ip := net.ParseIP(host); ip != nil {
			if IsPrivateIP(ip) {
				return nil, fmt.Errorf("%w: %s", ErrPrivateIP, ip)
			}
			return dialer.DialContext(ctx, network, addr)
		}

		// DNS resolution
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no addresses found for host %s", host)
		}

		for _, ip := range ips {
			if IsPrivateIP(ip.IP) {
				return nil, fmt.Errorf("%w: %s resolves to %s", ErrPrivateIP, host, ip.IP)
			}
		}

		// Dial the first resolved IP to prevent DNS rebinding
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
}
