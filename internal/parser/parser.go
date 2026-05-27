package parser

import (
	"bytes"
	"net/url"
	"strings"
	"unicode"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/SEObserver/crawlobserver/internal/htmlutil"
)

// PageData holds all extracted SEO signals from a page.
type PageData struct {
	Title           string
	Canonical       string
	MetaRobots      string
	MetaDescription string
	MetaKeywords    string
	H1              []string
	H2              []string
	H3              []string
	H4              []string
	H5              []string
	H6              []string
	Links           []Link
	Images          []Image
	Hreflang        []HreflangEntry
	Lang            string
	OGTitle         string
	OGDescription   string
	OGImage         string
	SchemaTypes     []string
	JSONLDBlocks    []string // raw JSON-LD block contents
	WordCount       int
	ContentHash     uint64 // SimHash fingerprint of visible body text
	Resources       []PageResource
}

// Image represents an image found on a page.
type Image struct {
	Src    string
	Alt    string
	Width  string
	Height string
}

// HreflangEntry represents a hreflang link.
type HreflangEntry struct {
	Lang string
	URL  string
}

// Parse parses HTML body and extracts SEO signals.
func Parse(body []byte, pageURL string) (*PageData, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	baseURL, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}

	data := &PageData{}
	data.Title = extractTitle(doc)
	data.Canonical = extractCanonical(doc)
	data.MetaRobots = extractMetaContent(doc, "robots")
	data.MetaDescription = extractMetaContent(doc, "description")
	data.MetaKeywords = extractMetaContent(doc, "keywords")
	data.H1 = extractHeadings(doc, "h1")
	data.H2 = extractHeadings(doc, "h2")
	data.H3 = extractHeadings(doc, "h3")
	data.H4 = extractHeadings(doc, "h4")
	data.H5 = extractHeadings(doc, "h5")
	data.H6 = extractHeadings(doc, "h6")
	data.Links = extractLinks(doc, baseURL)
	data.Images = extractImages(doc, baseURL)
	data.Hreflang = extractHreflang(doc)
	data.Lang = extractLang(doc)
	data.OGTitle = extractMetaProperty(doc, "og:title")
	data.OGDescription = extractMetaProperty(doc, "og:description")
	data.OGImage = extractMetaProperty(doc, "og:image")
	data.SchemaTypes = extractSchemaTypes(doc)
	data.JSONLDBlocks = extractJSONLDBlocks(doc)
	data.WordCount = countWords(doc)
	data.ContentHash = SimHash(ExtractMainContent(body, baseURL))
	data.Resources = ExtractResources(doc, baseURL)

	return data, nil
}

// countWords counts visible text words in the body.
func countWords(doc *goquery.Document) int {
	text := doc.Find("body").Text()
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inWord {
				count++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	return count
}

// extractImages extracts all images from the page.
func extractImages(doc *goquery.Document, baseURL *url.URL) []Image {
	var images []Image
	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		src, _ := htmlutil.Attr(s, "src")
		if src == "" {
			src, _ = htmlutil.Attr(s, "data-src") // lazy loading
		}
		alt, _ := htmlutil.Attr(s, "alt")
		width, _ := htmlutil.Attr(s, "width")
		height, _ := htmlutil.Attr(s, "height")

		if src != "" {
			// Resolve relative URLs
			if resolved, err := baseURL.Parse(src); err == nil {
				src = resolved.String()
			}
		}

		images = append(images, Image{
			Src:    src,
			Alt:    strings.TrimSpace(alt),
			Width:  width,
			Height: height,
		})
	})
	return images
}

// extractHreflang extracts hreflang annotations.
func extractHreflang(doc *goquery.Document) []HreflangEntry {
	var entries []HreflangEntry
	doc.Find("link").Each(func(_ int, s *goquery.Selection) {
		if !htmlutil.AttrTokenContains(s, "rel", "alternate") {
			return
		}
		lang, _ := htmlutil.Attr(s, "hreflang")
		href, _ := htmlutil.Attr(s, "href")
		if lang != "" && href != "" {
			entries = append(entries, HreflangEntry{
				Lang: strings.TrimSpace(lang),
				URL:  strings.TrimSpace(href),
			})
		}
	})
	return entries
}

// extractLang extracts the html lang attribute.
func extractLang(doc *goquery.Document) string {
	lang, _ := htmlutil.Attr(doc.Find("html").First(), "lang")
	return strings.TrimSpace(lang)
}

// extractMetaProperty extracts content from meta property tags (OpenGraph).
func extractMetaProperty(doc *goquery.Document, property string) string {
	var content string
	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		p, _ := htmlutil.Attr(s, "property")
		if strings.EqualFold(p, property) {
			content, _ = htmlutil.Attr(s, "content")
		}
	})
	return strings.TrimSpace(content)
}

// extractJSONLDBlocks extracts the raw text content of each <script type="application/ld+json"> block.
func extractJSONLDBlocks(doc *goquery.Document) []string {
	var blocks []string
	doc.Find("script").Each(func(_ int, s *goquery.Selection) {
		if !htmlutil.AttrMediaTypeEqual(s, "type", "application/ld+json") {
			return
		}
		text := strings.TrimSpace(s.Text())
		if text != "" {
			blocks = append(blocks, text)
		}
	})
	return blocks
}

// extractSchemaTypes extracts schema.org types from JSON-LD.
func extractSchemaTypes(doc *goquery.Document) []string {
	seen := make(map[string]bool)
	var types []string

	// JSON-LD
	doc.Find("script").Each(func(_ int, s *goquery.Selection) {
		if !htmlutil.AttrMediaTypeEqual(s, "type", "application/ld+json") {
			return
		}
		text := s.Text()
		// Simple extraction of @type values
		for _, part := range strings.Split(text, "\"@type\"") {
			if len(part) < 3 {
				continue
			}
			// Find the value after the colon
			idx := strings.IndexByte(part, '"')
			if idx < 0 {
				continue
			}
			rest := part[idx+1:]
			end := strings.IndexByte(rest, '"')
			if end > 0 && end < 100 {
				t := rest[:end]
				if t != "" && !seen[t] {
					seen[t] = true
					types = append(types, t)
				}
			}
		}
	})

	// Microdata
	doc.Find("*").Each(func(_ int, s *goquery.Selection) {
		itemtype, ok := htmlutil.Attr(s, "itemtype")
		if !ok {
			return
		}
		// Extract type name from URL like "https://schema.org/Product"
		if idx := strings.LastIndex(itemtype, "/"); idx >= 0 {
			t := itemtype[idx+1:]
			if t != "" && !seen[t] {
				seen[t] = true
				types = append(types, t)
			}
		}
	})

	return types
}

// ExtractMainContent uses Mozilla Readability to extract the main article text,
// stripping navigation, sidebars, footers, and other boilerplate.
// This produces a much better SimHash signal than raw body text.
// Falls back to full body text if Readability fails (e.g. non-article pages).
func ExtractMainContent(body []byte, pageURL *url.URL) string {
	article, err := readability.FromReader(bytes.NewReader(body), pageURL)
	if err != nil || article.Node == nil {
		return ""
	}
	var buf strings.Builder
	if err := article.RenderText(&buf); err != nil {
		return ""
	}
	text := buf.String()
	// Readability can return very short text for non-article pages (homepages, etc.).
	// If the extracted content is too short, it's not meaningful for SimHash.
	if len(text) < 100 {
		return ""
	}
	return text
}
