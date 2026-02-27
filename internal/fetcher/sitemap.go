package fetcher

import (
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"strings"
)

const maxSitemapSize = 10 * 1024 * 1024 // 10MB
const maxTotalSitemaps = 50

// SitemapURL represents a single URL entry in a sitemap.
type SitemapURL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod"`
	ChangeFreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

// SitemapEntry represents a fetched sitemap (index or urlset).
type SitemapEntry struct {
	URL        string
	Type       string // "index" or "urlset"
	StatusCode int
	URLs       []SitemapURL
	Sitemaps   []string // child sitemap URLs if index
}

// XML structures for parsing

type xmlSitemapIndex struct {
	XMLName  xml.Name           `xml:"sitemapindex"`
	Sitemaps []xmlSitemapLoc    `xml:"sitemap"`
}

type xmlSitemapLoc struct {
	Loc string `xml:"loc"`
}

type xmlURLSet struct {
	XMLName xml.Name       `xml:"urlset"`
	URLs    []xmlURLEntry  `xml:"url"`
}

type xmlURLEntry struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod"`
	ChangeFreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

// FetchSitemap fetches and parses a single sitemap URL.
func FetchSitemap(client *http.Client, sitemapURL, userAgent string) SitemapEntry {
	entry := SitemapEntry{URL: sitemapURL}

	req, err := http.NewRequest("GET", sitemapURL, nil)
	if err != nil {
		return entry
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return entry
	}
	defer resp.Body.Close()

	entry.StatusCode = resp.StatusCode
	if resp.StatusCode != 200 {
		return entry
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSitemapSize))
	if err != nil {
		return entry
	}

	content := string(body)

	// Detect type: sitemapindex vs urlset
	if strings.Contains(content, "<sitemapindex") {
		entry.Type = "index"
		var idx xmlSitemapIndex
		if err := xml.Unmarshal(body, &idx); err == nil {
			for _, s := range idx.Sitemaps {
				loc := strings.TrimSpace(s.Loc)
				if loc != "" {
					entry.Sitemaps = append(entry.Sitemaps, loc)
				}
			}
		}
	} else if strings.Contains(content, "<urlset") {
		entry.Type = "urlset"
		var urlset xmlURLSet
		if err := xml.Unmarshal(body, &urlset); err == nil {
			for _, u := range urlset.URLs {
				entry.URLs = append(entry.URLs, SitemapURL{
					Loc:        strings.TrimSpace(u.Loc),
					LastMod:    strings.TrimSpace(u.LastMod),
					ChangeFreq: strings.TrimSpace(u.ChangeFreq),
					Priority:   strings.TrimSpace(u.Priority),
				})
			}
		}
	}

	return entry
}

// DiscoverSitemaps fetches all given sitemap URLs, recursing into indexes.
// Returns at most maxTotalSitemaps entries.
func DiscoverSitemaps(client *http.Client, userAgent string, sitemapURLs []string) []SitemapEntry {
	var results []SitemapEntry
	seen := make(map[string]bool)

	var queue []string
	for _, u := range sitemapURLs {
		if !seen[u] {
			seen[u] = true
			queue = append(queue, u)
		}
	}

	for len(queue) > 0 && len(results) < maxTotalSitemaps {
		url := queue[0]
		queue = queue[1:]

		log.Printf("Fetching sitemap: %s", url)
		entry := FetchSitemap(client, url, userAgent)
		results = append(results, entry)

		// If it's an index, enqueue children
		if entry.Type == "index" {
			for _, child := range entry.Sitemaps {
				if !seen[child] && len(results)+len(queue) < maxTotalSitemaps {
					seen[child] = true
					queue = append(queue, child)
				}
			}
		}
	}

	return results
}
