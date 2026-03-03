package extraction

import "time"

// ExtractorType defines the kind of extraction.
type ExtractorType string

const (
	CSSExtractText    ExtractorType = "css_extract_text"
	CSSExtractAttr    ExtractorType = "css_extract_attr"
	CSSExtractAllText ExtractorType = "css_extract_all_text"
	CSSExtractAllAttr ExtractorType = "css_extract_all_attr"
	RegexExtract      ExtractorType = "regex_extract"
	RegexExtractAll   ExtractorType = "regex_extract_all"
	XPathExtract      ExtractorType = "xpath_extract"
	XPathExtractAll   ExtractorType = "xpath_extract_all"
)

// Extractor is a single extraction rule within an extractor set.
type Extractor struct {
	ID         string        `json:"id"`
	SetID      string        `json:"set_id"`
	Type       ExtractorType `json:"type"`
	Name       string        `json:"name"`
	Selector   string        `json:"selector"`
	Attribute  string        `json:"attribute"`
	URLPattern string        `json:"url_pattern"`
	SortOrder  int           `json:"sort_order"`
}

// ExtractorSet groups extractors under a named set.
type ExtractorSet struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
	Extractors     []Extractor `json:"extractors"`
	ExtractorCount int         `json:"extractor_count,omitempty"`
}

// ExtractionRow is a single extracted value ready for ClickHouse insertion.
type ExtractionRow struct {
	CrawlSessionID string
	URL            string
	ExtractorName  string
	Value          string
	CrawledAt      time.Time
}

// PageExtraction holds extraction results for a single page.
type PageExtraction struct {
	URL    string            `json:"url"`
	Values map[string]string `json:"values"`
}

// ExtractionResult is the full output of running extractors against a session.
type ExtractionResult struct {
	SessionID  string           `json:"session_id"`
	Extractors []Extractor      `json:"extractors"`
	TotalPages int              `json:"total_pages"`
	Pages      []PageExtraction `json:"pages"`
}
