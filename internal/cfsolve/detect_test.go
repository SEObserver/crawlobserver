package cfsolve

import "testing"

func TestIsCFChallenge(t *testing.T) {
	cfHeaders := map[string]string{
		"Server": "cloudflare",
		"Cf-Ray": "abc123",
	}
	cfBody := []byte(`<html><head><title>Just a moment...</title></head><body>
		<script src="/cdn-cgi/challenge-platform/scripts/abc.js"></script>
		<script>window._cf_chl_opt={}</script>
	</body></html>`)

	tests := []struct {
		name    string
		status  int
		headers map[string]string
		body    []byte
		want    bool
	}{
		{
			name:    "real CF challenge",
			status:  403,
			headers: cfHeaders,
			body:    cfBody,
			want:    true,
		},
		{
			name:    "CF challenge with 503",
			status:  503,
			headers: cfHeaders,
			body:    cfBody,
			want:    true,
		},
		{
			name:    "normal 403 no CF headers",
			status:  403,
			headers: map[string]string{"Server": "nginx"},
			body:    []byte("Forbidden"),
			want:    false,
		},
		{
			name:    "CF headers but 200 status",
			status:  200,
			headers: cfHeaders,
			body:    cfBody,
			want:    false,
		},
		{
			name:    "CF headers but no body markers",
			status:  403,
			headers: cfHeaders,
			body:    []byte("<html>Access Denied</html>"),
			want:    false,
		},
		{
			name:    "CF-Mitigated header only",
			status:  403,
			headers: map[string]string{"Cf-Mitigated": "challenge"},
			body:    cfBody,
			want:    true,
		},
		{
			name:    "empty body",
			status:  403,
			headers: cfHeaders,
			body:    nil,
			want:    false,
		},
		{
			name:    "404 with CF",
			status:  404,
			headers: cfHeaders,
			body:    cfBody,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCFChallenge(tt.status, tt.headers, tt.body)
			if got != tt.want {
				t.Errorf("IsCFChallenge() = %v, want %v", got, tt.want)
			}
		})
	}
}
