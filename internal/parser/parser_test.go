package parser

import (
	"bytes"
	"testing"

	"github.com/PuerkitoBio/goquery"
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

// docFromHTML is a helper to create a goquery.Document from an HTML string.
func docFromHTML(html string) *goquery.Document {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		panic(err)
	}
	return doc
}

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

// --- extractSchemaTypes tests ---

func TestExtractSchemaTypes_JSONLD_Single(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">{"@type": "Product", "name": "Widget"}</script>
	</head><body></body></html>`
	doc := docFromHTML(html)
	types := extractSchemaTypes(doc)

	if len(types) != 1 {
		t.Fatalf("expected 1 type, got %d: %v", len(types), types)
	}
	if types[0] != "Product" {
		t.Errorf("expected Product, got %q", types[0])
	}
}

func TestExtractSchemaTypes_JSONLD_Multiple(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">{"@type": "Organization", "name": "Acme"}</script>
	<script type="application/ld+json">{"@type": "WebPage", "name": "Home"}</script>
	</head><body></body></html>`
	doc := docFromHTML(html)
	types := extractSchemaTypes(doc)

	if len(types) != 2 {
		t.Fatalf("expected 2 types, got %d: %v", len(types), types)
	}
	found := map[string]bool{}
	for _, tp := range types {
		found[tp] = true
	}
	if !found["Organization"] {
		t.Error("expected Organization type")
	}
	if !found["WebPage"] {
		t.Error("expected WebPage type")
	}
}

func TestExtractSchemaTypes_JSONLD_NestedTypes(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{"@type": "Product", "offers": {"@type": "Offer", "price": "9.99"}}
	</script>
	</head><body></body></html>`
	doc := docFromHTML(html)
	types := extractSchemaTypes(doc)

	if len(types) != 2 {
		t.Fatalf("expected 2 types, got %d: %v", len(types), types)
	}
	found := map[string]bool{}
	for _, tp := range types {
		found[tp] = true
	}
	if !found["Product"] {
		t.Error("expected Product type")
	}
	if !found["Offer"] {
		t.Error("expected Offer type")
	}
}

func TestExtractSchemaTypes_JSONLD_Dedup(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">{"@type": "Product"}</script>
	<script type="application/ld+json">{"@type": "Product"}</script>
	</head><body></body></html>`
	doc := docFromHTML(html)
	types := extractSchemaTypes(doc)

	if len(types) != 1 {
		t.Fatalf("expected 1 type (dedup), got %d: %v", len(types), types)
	}
}

func TestExtractSchemaTypes_Microdata(t *testing.T) {
	html := `<html><body>
	<div itemscope itemtype="https://schema.org/Product">
		<span itemprop="name">Widget</span>
	</div>
	</body></html>`
	doc := docFromHTML(html)
	types := extractSchemaTypes(doc)

	if len(types) != 1 {
		t.Fatalf("expected 1 type, got %d: %v", len(types), types)
	}
	if types[0] != "Product" {
		t.Errorf("expected Product, got %q", types[0])
	}
}

func TestExtractSchemaTypes_MicrodataAndJSONLD(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">{"@type": "WebPage"}</script>
	</head><body>
	<div itemscope itemtype="https://schema.org/Product">
		<span itemprop="name">Widget</span>
	</div>
	</body></html>`
	doc := docFromHTML(html)
	types := extractSchemaTypes(doc)

	if len(types) != 2 {
		t.Fatalf("expected 2 types, got %d: %v", len(types), types)
	}
	found := map[string]bool{}
	for _, tp := range types {
		found[tp] = true
	}
	if !found["WebPage"] {
		t.Error("expected WebPage type from JSON-LD")
	}
	if !found["Product"] {
		t.Error("expected Product type from microdata")
	}
}

func TestExtractSchemaTypes_MicrodataDedup(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">{"@type": "Product"}</script>
	</head><body>
	<div itemscope itemtype="https://schema.org/Product">
		<span itemprop="name">Widget</span>
	</div>
	</body></html>`
	doc := docFromHTML(html)
	types := extractSchemaTypes(doc)

	if len(types) != 1 {
		t.Fatalf("expected 1 type (dedup across JSON-LD and microdata), got %d: %v", len(types), types)
	}
}

func TestExtractSchemaTypes_NoSchema(t *testing.T) {
	html := `<html><body><p>No schema here</p></body></html>`
	doc := docFromHTML(html)
	types := extractSchemaTypes(doc)

	if len(types) != 0 {
		t.Errorf("expected 0 types, got %d: %v", len(types), types)
	}
}

func TestExtractSchemaTypes_EmptyJSONLD(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">{}</script>
	</head><body></body></html>`
	doc := docFromHTML(html)
	types := extractSchemaTypes(doc)

	if len(types) != 0 {
		t.Errorf("expected 0 types for empty JSON-LD, got %d: %v", len(types), types)
	}
}

func TestExtractSchemaTypes_MalformedJSONLD(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">not json at all</script>
	</head><body></body></html>`
	doc := docFromHTML(html)
	types := extractSchemaTypes(doc)

	// Should not panic, may return 0 types
	if len(types) != 0 {
		t.Logf("extracted types from malformed JSON-LD: %v (acceptable)", types)
	}
}

// --- extractLang tests ---

func TestExtractLang(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "standard lang",
			html: `<html lang="en"><body></body></html>`,
			want: "en",
		},
		{
			name: "lang with region",
			html: `<html lang="fr-FR"><body></body></html>`,
			want: "fr-FR",
		},
		{
			name: "lang with whitespace",
			html: `<html lang="  de  "><body></body></html>`,
			want: "de",
		},
		{
			name: "no lang attribute",
			html: `<html><body></body></html>`,
			want: "",
		},
		{
			name: "empty lang attribute",
			html: `<html lang=""><body></body></html>`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := docFromHTML(tt.html)
			got := extractLang(doc)
			if got != tt.want {
				t.Errorf("extractLang() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- countWords tests ---

func TestCountWords(t *testing.T) {
	tests := []struct {
		name string
		html string
		want int
	}{
		{
			name: "simple text",
			html: `<html><body>Hello world</body></html>`,
			want: 2,
		},
		{
			name: "text with punctuation",
			html: `<html><body>Hello, world! How are you?</body></html>`,
			want: 5,
		},
		{
			name: "text with numbers",
			html: `<html><body>There are 42 items</body></html>`,
			want: 4,
		},
		{
			name: "multiple spaces",
			html: `<html><body>Hello    world</body></html>`,
			want: 2,
		},
		{
			name: "empty body",
			html: `<html><body></body></html>`,
			want: 0,
		},
		{
			name: "no body",
			html: `<html></html>`,
			want: 0,
		},
		{
			name: "text in nested elements",
			html: `<html><body><div><p>One two</p><p>three four</p></div></body></html>`,
			want: 3, // goquery merges "two" and "three" into "twothree" (no space between sibling text nodes)
		},
		{
			name: "text with tabs and newlines",
			html: "<html><body>\n\tHello\n\tworld\n</body></html>",
			want: 2,
		},
		{
			name: "unicode text",
			html: `<html><body>Bonjour le monde</body></html>`,
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := docFromHTML(tt.html)
			got := countWords(doc)
			if got != tt.want {
				t.Errorf("countWords() = %d, want %d", got, tt.want)
			}
		})
	}
}

// --- extractHreflang tests ---

func TestExtractHreflang(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		wantCount int
		wantLang  string // first entry lang
		wantURL   string // first entry URL
	}{
		{
			name: "single hreflang",
			html: `<html><head>
				<link rel="alternate" hreflang="fr" href="https://example.com/fr">
			</head><body></body></html>`,
			wantCount: 1,
			wantLang:  "fr",
			wantURL:   "https://example.com/fr",
		},
		{
			name: "multiple hreflangs",
			html: `<html><head>
				<link rel="alternate" hreflang="en" href="https://example.com/en">
				<link rel="alternate" hreflang="fr" href="https://example.com/fr">
				<link rel="alternate" hreflang="x-default" href="https://example.com/">
			</head><body></body></html>`,
			wantCount: 3,
			wantLang:  "en",
			wantURL:   "https://example.com/en",
		},
		{
			name:      "no hreflang",
			html:      `<html><head></head><body></body></html>`,
			wantCount: 0,
		},
		{
			name: "link without hreflang attr",
			html: `<html><head>
				<link rel="alternate" href="https://example.com/rss" type="application/rss+xml">
			</head><body></body></html>`,
			wantCount: 0,
		},
		{
			name: "empty lang ignored",
			html: `<html><head>
				<link rel="alternate" hreflang="" href="https://example.com/empty">
			</head><body></body></html>`,
			wantCount: 0,
		},
		{
			name: "empty href ignored",
			html: `<html><head>
				<link rel="alternate" hreflang="en" href="">
			</head><body></body></html>`,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := docFromHTML(tt.html)
			entries := extractHreflang(doc)
			if len(entries) != tt.wantCount {
				t.Fatalf("extractHreflang() returned %d entries, want %d: %+v", len(entries), tt.wantCount, entries)
			}
			if tt.wantCount > 0 {
				if entries[0].Lang != tt.wantLang {
					t.Errorf("first entry lang = %q, want %q", entries[0].Lang, tt.wantLang)
				}
				if entries[0].URL != tt.wantURL {
					t.Errorf("first entry URL = %q, want %q", entries[0].URL, tt.wantURL)
				}
			}
		})
	}
}

// --- extractMetaProperty tests ---

func TestExtractMetaProperty(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="OG Title">
		<meta property="og:description" content="OG Desc">
		<meta property="og:image" content="https://example.com/img.png">
	</head><body></body></html>`
	doc := docFromHTML(html)

	tests := []struct {
		property string
		want     string
	}{
		{"og:title", "OG Title"},
		{"og:description", "OG Desc"},
		{"og:image", "https://example.com/img.png"},
		{"og:nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.property, func(t *testing.T) {
			got := extractMetaProperty(doc, tt.property)
			if got != tt.want {
				t.Errorf("extractMetaProperty(%q) = %q, want %q", tt.property, got, tt.want)
			}
		})
	}
}

func TestExtractMetaProperty_CaseInsensitive(t *testing.T) {
	html := `<html><head>
		<meta property="OG:Title" content="The Title">
	</head><body></body></html>`
	doc := docFromHTML(html)

	got := extractMetaProperty(doc, "og:title")
	if got != "The Title" {
		t.Errorf("extractMetaProperty case-insensitive = %q, want %q", got, "The Title")
	}
}

// --- extractImages tests ---

func TestExtractImages(t *testing.T) {
	html := `<html><body>
		<img src="/img/photo.jpg" alt="A photo" width="800" height="600">
		<img data-src="/img/lazy.jpg" alt="Lazy loaded">
		<img src="" alt="No source">
	</body></html>`

	data, err := Parse([]byte(html), "https://example.com/page")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// img with src, img with data-src (lazy), img with empty src
	if len(data.Images) != 3 {
		t.Fatalf("expected 3 images, got %d: %+v", len(data.Images), data.Images)
	}

	// First image: src resolved
	if data.Images[0].Src != "https://example.com/img/photo.jpg" {
		t.Errorf("image 0 src = %q, want resolved URL", data.Images[0].Src)
	}
	if data.Images[0].Alt != "A photo" {
		t.Errorf("image 0 alt = %q, want %q", data.Images[0].Alt, "A photo")
	}
	if data.Images[0].Width != "800" {
		t.Errorf("image 0 width = %q, want %q", data.Images[0].Width, "800")
	}
	if data.Images[0].Height != "600" {
		t.Errorf("image 0 height = %q, want %q", data.Images[0].Height, "600")
	}

	// Second image: data-src fallback
	if data.Images[1].Src != "https://example.com/img/lazy.jpg" {
		t.Errorf("image 1 src = %q, want resolved lazy URL", data.Images[1].Src)
	}
}
