package fetcher

import (
	"net"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		// IPv4 loopback
		{"127.0.0.1", true},
		{"127.255.255.255", true},

		// RFC1918 - 10.0.0.0/8
		{"10.0.0.1", true},
		{"10.255.255.255", true},

		// RFC1918 - 172.16.0.0/12
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.15.255.255", false}, // just below range
		{"172.32.0.0", false},    // just above range

		// RFC1918 - 192.168.0.0/16
		{"192.168.0.1", true},
		{"192.168.255.255", true},

		// Link-local / cloud metadata
		{"169.254.0.1", true},
		{"169.254.169.254", true},

		// Current network
		{"0.0.0.0", true},
		{"0.255.255.255", true},

		// IPv6 loopback
		{"::1", true},

		// IPv6 unique local
		{"fc00::1", true},
		{"fdff::1", true},

		// IPv6 link-local
		{"fe80::1", true},

		// Public IPs
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},
		{"192.167.255.255", false},
		{"11.0.0.1", false},
		{"2607:f8b0:4004:800::200e", false}, // Google IPv6
	}

	for _, tc := range tests {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("could not parse IP: %s", tc.ip)
		}
		got := IsPrivateIP(ip)
		if got != tc.private {
			t.Errorf("IsPrivateIP(%s) = %v, want %v", tc.ip, got, tc.private)
		}
	}
}

func TestSafeDialContext_AllowPrivate(t *testing.T) {
	dial := SafeDialContext(true)
	if dial == nil {
		t.Fatal("SafeDialContext(true) returned nil")
	}
}

func TestSafeDialContext_BlockPrivate(t *testing.T) {
	dial := SafeDialContext(false)
	if dial == nil {
		t.Fatal("SafeDialContext(false) returned nil")
	}

	// Try to dial a private IP literal — should be blocked
	_, err := dial(t.Context(), "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("expected error dialing 127.0.0.1, got nil")
	}
	if !isPrivateIPError(err) {
		t.Errorf("expected ErrPrivateIP, got: %v", err)
	}

	// Try 10.0.0.1
	_, err = dial(t.Context(), "tcp", "10.0.0.1:80")
	if err == nil {
		t.Fatal("expected error dialing 10.0.0.1, got nil")
	}
	if !isPrivateIPError(err) {
		t.Errorf("expected ErrPrivateIP, got: %v", err)
	}

	// Try metadata endpoint
	_, err = dial(t.Context(), "tcp", "169.254.169.254:80")
	if err == nil {
		t.Fatal("expected error dialing 169.254.169.254, got nil")
	}
	if !isPrivateIPError(err) {
		t.Errorf("expected ErrPrivateIP, got: %v", err)
	}

	// IPv6 loopback
	_, err = dial(t.Context(), "tcp", "[::1]:80")
	if err == nil {
		t.Fatal("expected error dialing ::1, got nil")
	}
	if !isPrivateIPError(err) {
		t.Errorf("expected ErrPrivateIP, got: %v", err)
	}
}

func isPrivateIPError(err error) bool {
	return err != nil && (err.Error() == ErrPrivateIP.Error() || len(err.Error()) > len(ErrPrivateIP.Error()) && err.Error()[:len(ErrPrivateIP.Error())] == ErrPrivateIP.Error())
}
