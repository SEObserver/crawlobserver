package server

import (
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		isZero  bool
	}{
		{
			name:  "date only",
			input: "2025-06-15",
			want:  time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "datetime without timezone",
			input: "2025-06-15T14:30:00",
			want:  time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:  "RFC3339",
			input: "2025-06-15T14:30:00Z",
			want:  time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:  "RFC3339 with offset",
			input: "2025-06-15T14:30:00+02:00",
			want:  time.Date(2025, 6, 15, 14, 30, 0, 0, time.FixedZone("", 2*3600)),
		},
		{
			name:   "empty string",
			input:  "",
			isZero: true,
		},
		{
			name:   "invalid format",
			input:  "not-a-date",
			isZero: true,
		},
		{
			name:   "partial date",
			input:  "2025-06",
			isZero: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDate(tt.input)
			if tt.isZero {
				if !got.IsZero() {
					t.Errorf("parseDate(%q) = %v, want zero time", tt.input, got)
				}
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("parseDate(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
