package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/SEObserver/crawlobserver/internal/storage"
)

func testLinks() []storage.LinkRow {
	return []storage.LinkRow{
		{
			CrawlSessionID: "test-session",
			SourceURL:      "https://example.com/page1",
			TargetURL:      "https://external.com/link1",
			AnchorText:     "External Link 1",
			Rel:            "nofollow",
			IsInternal:     false,
			Tag:            "a",
			CrawledAt:      time.Now(),
		},
		{
			CrawlSessionID: "test-session",
			SourceURL:      "https://example.com/page2",
			TargetURL:      "https://other.com/link2",
			AnchorText:     "Other Link",
			Rel:            "",
			IsInternal:     false,
			Tag:            "a",
			CrawledAt:      time.Now(),
		},
	}
}

func TestWriteExternalLinksTable(t *testing.T) {
	var buf bytes.Buffer
	links := testLinks()

	if err := WriteExternalLinks(&buf, links, "table"); err != nil {
		t.Fatalf("WriteExternalLinks() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "SOURCE") {
		t.Error("table output should contain header")
	}
	if !strings.Contains(output, "external.com") {
		t.Error("table output should contain external link")
	}
	if !strings.Contains(output, "Total: 2") {
		t.Error("table output should contain total count")
	}
}

func TestWriteExternalLinksCSV(t *testing.T) {
	var buf bytes.Buffer
	links := testLinks()

	if err := WriteExternalLinks(&buf, links, "csv"); err != nil {
		t.Fatalf("WriteExternalLinks() error = %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 { // header + 2 data rows
		t.Errorf("CSV should have 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "source_url") {
		t.Error("CSV header should contain source_url")
	}
}

func TestWriteExternalLinksEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteExternalLinks(&buf, nil, "table"); err != nil {
		t.Fatalf("WriteExternalLinks() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Total: 0") {
		t.Error("should show 0 total for empty links")
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if truncate(short, 10) != "hello" {
		t.Error("should not truncate short strings")
	}

	long := "this is a very long string that exceeds the limit"
	result := truncate(long, 20)
	if len(result) != 20 {
		t.Errorf("truncated length = %d, want 20", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated string should end with ...")
	}
}
