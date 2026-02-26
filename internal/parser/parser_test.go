package parser

import (
	"testing"
)

const testHTML = `<!DOCTYPE html>
<html>
<head>
	<title>Test Page Title</title>
	<meta name="description" content="Test meta description">
	<meta name="robots" content="index, follow">
	<link rel="canonical" href="https://example.com/page">
</head>
<body>
	<h1>Main Heading</h1>
	<h2>Sub Heading 1</h2>
	<h2>Sub Heading 2</h2>
	<h3>Sub Sub Heading</h3>
	<p>Some text with a <a href="/internal-link">internal link</a>.</p>
	<p><a href="https://external.com/page" rel="nofollow">External Link</a></p>
	<p><a href="/relative/path">Relative Link</a></p>
	<p><a href="mailto:test@example.com">Email</a></p>
	<p><a href="javascript:void(0)">JS Link</a></p>
	<p><a href="tel:+1234567890">Phone</a></p>
	<p><a href="#section">Anchor</a></p>
</body>
</html>`

func TestParse(t *testing.T) {
	data, err := Parse([]byte(testHTML), "https://example.com/page")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if data.Title != "Test Page Title" {
		t.Errorf("Title = %q, want %q", data.Title, "Test Page Title")
	}

	if data.Canonical != "https://example.com/page" {
		t.Errorf("Canonical = %q, want %q", data.Canonical, "https://example.com/page")
	}

	if data.MetaRobots != "index, follow" {
		t.Errorf("MetaRobots = %q, want %q", data.MetaRobots, "index, follow")
	}

	if data.MetaDescription != "Test meta description" {
		t.Errorf("MetaDescription = %q, want %q", data.MetaDescription, "Test meta description")
	}

	if len(data.H1) != 1 || data.H1[0] != "Main Heading" {
		t.Errorf("H1 = %v, want [Main Heading]", data.H1)
	}

	if len(data.H2) != 2 {
		t.Errorf("H2 count = %d, want 2", len(data.H2))
	}

	if len(data.H3) != 1 {
		t.Errorf("H3 count = %d, want 1", len(data.H3))
	}
}

func TestParseLinks(t *testing.T) {
	data, err := Parse([]byte(testHTML), "https://example.com/page")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Should have 3 links: /internal-link, https://external.com/page, /relative/path
	// mailto:, javascript:, tel:, and # should be filtered out
	if len(data.Links) != 3 {
		t.Fatalf("Links count = %d, want 3, links: %+v", len(data.Links), data.Links)
	}

	// Check internal link
	found := false
	for _, l := range data.Links {
		if l.AnchorText == "internal link" {
			found = true
			if !l.IsInternal {
				t.Error("expected /internal-link to be internal")
			}
			if l.Tag != "a" {
				t.Errorf("expected tag 'a', got %q", l.Tag)
			}
		}
	}
	if !found {
		t.Error("internal link not found")
	}

	// Check external link
	found = false
	for _, l := range data.Links {
		if l.AnchorText == "External Link" {
			found = true
			if l.IsInternal {
				t.Error("expected external link to not be internal")
			}
			if l.Rel != "nofollow" {
				t.Errorf("expected rel=nofollow, got %q", l.Rel)
			}
		}
	}
	if !found {
		t.Error("external link not found")
	}
}

func TestParseEmptyHTML(t *testing.T) {
	data, err := Parse([]byte("<html></html>"), "https://example.com")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if data.Title != "" {
		t.Errorf("expected empty title, got %q", data.Title)
	}
	if len(data.Links) != 0 {
		t.Errorf("expected 0 links, got %d", len(data.Links))
	}
}
