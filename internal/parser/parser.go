package parser

import (
	"bytes"
	"net/url"

	"github.com/PuerkitoBio/goquery"
)

// PageData holds all extracted SEO signals from a page.
type PageData struct {
	Title           string
	Canonical       string
	MetaRobots      string
	MetaDescription string
	H1              []string
	H2              []string
	H3              []string
	H4              []string
	H5              []string
	H6              []string
	Links           []Link
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
	data.H1 = extractHeadings(doc, "h1")
	data.H2 = extractHeadings(doc, "h2")
	data.H3 = extractHeadings(doc, "h3")
	data.H4 = extractHeadings(doc, "h4")
	data.H5 = extractHeadings(doc, "h5")
	data.H6 = extractHeadings(doc, "h6")
	data.Links = extractLinks(doc, baseURL)

	return data, nil
}
