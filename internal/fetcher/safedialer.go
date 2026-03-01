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
var privateRanges = []*net.IPNet{
	mustParseCIDR("127.0.0.0/8"),    // IPv4 loopback
	mustParseCIDR("10.0.0.0/8"),     // RFC1918
	mustParseCIDR("172.16.0.0/12"),  // RFC1918
	mustParseCIDR("192.168.0.0/16"), // RFC1918
	mustParseCIDR("169.254.0.0/16"), // Link-local
	mustParseCIDR("0.0.0.0/8"),      // Current network
	mustParseCIDR("::1/128"),        // IPv6 loopback
	mustParseCIDR("fc00::/7"),       // IPv6 unique local
	mustParseCIDR("fe80::/10"),      // IPv6 link-local
}

func mustParseCIDR(s string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR %q: %v", s, err))
	}
	return ipNet
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

// DialOptions configures network-level behavior for the safe dialer.
type DialOptions struct {
	SourceIP        string
	ForceIPv4       bool
	AllowPrivateIPs bool
}

// SafeDialContext returns a DialContext function that blocks connections to private IPs
// after DNS resolution (anti DNS-rebinding). When allowPrivate is true, no filtering is applied.
func SafeDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return SafeDialContextWithOpts(DialOptions{AllowPrivateIPs: allowPrivate})
}

// SafeDialContextWithOpts returns a DialContext function with full network options:
// source IP binding, IPv4-only mode, and private IP filtering.
func SafeDialContextWithOpts(opts DialOptions) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if opts.SourceIP != "" {
		if ip := net.ParseIP(opts.SourceIP); ip != nil {
			dialer.LocalAddr = &net.TCPAddr{IP: ip}
		}
	}
	if opts.AllowPrivateIPs && !opts.ForceIPv4 {
		return dialer.DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if opts.ForceIPv4 {
			network = "tcp4"
		}

		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		// If host is already an IP literal, check directly
		if ip := net.ParseIP(host); ip != nil {
			if opts.ForceIPv4 && ip.To4() == nil {
				return nil, fmt.Errorf("IPv6 address %s rejected (force_ipv4 enabled)", ip)
			}
			if !opts.AllowPrivateIPs && IsPrivateIP(ip) {
				return nil, fmt.Errorf("%w: %s", ErrPrivateIP, ip)
			}
			return dialer.DialContext(ctx, network, addr)
		}

		// DNS resolution
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}

		// Filter IPv4-only if requested
		if opts.ForceIPv4 {
			filtered := ips[:0]
			for _, ip := range ips {
				if ip.IP.To4() != nil {
					filtered = append(filtered, ip)
				}
			}
			ips = filtered
		}

		if len(ips) == 0 {
			return nil, fmt.Errorf("no addresses found for host %s", host)
		}

		if !opts.AllowPrivateIPs {
			for _, ip := range ips {
				if IsPrivateIP(ip.IP) {
					return nil, fmt.Errorf("%w: %s resolves to %s", ErrPrivateIP, host, ip.IP)
				}
			}
		}

		// Dial the first resolved IP to prevent DNS rebinding
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
}
