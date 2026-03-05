package storage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestExportRecordMarshalMeta(t *testing.T) {
	sess := &exportSession{
		ID:           "sess-123",
		StartedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		FinishedAt:   time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC),
		Status:       "completed",
		SeedURLs:     []string{"https://example.com"},
		Config:       `{"workers":4}`,
		PagesCrawled: 42,
		UserAgent:    "TestBot/1.0",
	}
	rec := exportRecord{
		Type:    RecordMeta,
		Version: ExportFormatVersion,
		Session: sess,
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded exportRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Type != RecordMeta {
		t.Errorf("Type = %q, want %q", decoded.Type, RecordMeta)
	}
	if decoded.Version != ExportFormatVersion {
		t.Errorf("Version = %d, want %d", decoded.Version, ExportFormatVersion)
	}
	if decoded.Session == nil {
		t.Fatal("Session should not be nil")
	}
	if decoded.Session.ID != "sess-123" {
		t.Errorf("Session.ID = %q, want %q", decoded.Session.ID, "sess-123")
	}
	if decoded.Session.PagesCrawled != 42 {
		t.Errorf("Session.PagesCrawled = %d, want 42", decoded.Session.PagesCrawled)
	}
	if decoded.Session.UserAgent != "TestBot/1.0" {
		t.Errorf("Session.UserAgent = %q, want %q", decoded.Session.UserAgent, "TestBot/1.0")
	}
	if len(decoded.Session.SeedURLs) != 1 || decoded.Session.SeedURLs[0] != "https://example.com" {
		t.Errorf("Session.SeedURLs = %v, want [https://example.com]", decoded.Session.SeedURLs)
	}
}

func TestExportRecordMarshalMetaWithProjectID(t *testing.T) {
	pid := "proj-456"
	sess := &exportSession{
		ID:        "sess-789",
		ProjectID: &pid,
	}
	rec := exportRecord{
		Type:    RecordMeta,
		Version: ExportFormatVersion,
		Session: sess,
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded exportRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Session.ProjectID == nil {
		t.Fatal("Session.ProjectID should not be nil")
	}
	if *decoded.Session.ProjectID != "proj-456" {
		t.Errorf("Session.ProjectID = %q, want %q", *decoded.Session.ProjectID, "proj-456")
	}
}

func TestExportRecordMarshalMetaWithoutProjectID(t *testing.T) {
	sess := &exportSession{
		ID: "sess-no-project",
	}
	rec := exportRecord{
		Type:    RecordMeta,
		Version: ExportFormatVersion,
		Session: sess,
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Verify project_id is omitted from JSON (omitempty)
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	sessMap := raw["session"].(map[string]interface{})
	if _, ok := sessMap["project_id"]; ok {
		t.Error("project_id should be omitted when nil")
	}
}

func TestExportPageMarshalRoundtrip(t *testing.T) {
	p := exportPage{
		URL:              "https://example.com/page",
		FinalURL:         "https://example.com/page",
		StatusCode:       200,
		ContentType:      "text/html",
		Title:            "Test Page",
		TitleLength:      9,
		Canonical:        "https://example.com/page",
		CanonicalIsSelf:  true,
		IsIndexable:      true,
		MetaDescription:  "A test page",
		MetaDescLength:   11,
		H1:               []string{"Main Heading"},
		H2:               []string{"Sub 1", "Sub 2"},
		WordCount:        150,
		InternalLinksOut: 5,
		ExternalLinksOut: 2,
		ImagesCount:      3,
		ImagesNoAlt:      1,
		Lang:             "en",
		SchemaTypes:      []string{"Article", "WebPage"},
		Headers:          map[string]string{"Server": "nginx"},
		BodySize:         1024,
		FetchDurationMs:  250,
		Depth:            1,
		FoundOn:          "https://example.com/",
		PageRank:         0.85,
		CrawledAt:        time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded exportPage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.URL != p.URL {
		t.Errorf("URL = %q, want %q", decoded.URL, p.URL)
	}
	if decoded.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", decoded.StatusCode)
	}
	if decoded.IsIndexable != true {
		t.Errorf("IsIndexable = %v, want true", decoded.IsIndexable)
	}
	if decoded.PageRank != 0.85 {
		t.Errorf("PageRank = %f, want 0.85", decoded.PageRank)
	}
	if len(decoded.H1) != 1 || decoded.H1[0] != "Main Heading" {
		t.Errorf("H1 = %v, want [Main Heading]", decoded.H1)
	}
	if len(decoded.H2) != 2 {
		t.Errorf("H2 len = %d, want 2", len(decoded.H2))
	}
	if len(decoded.SchemaTypes) != 2 {
		t.Errorf("SchemaTypes len = %d, want 2", len(decoded.SchemaTypes))
	}
	if decoded.Headers["Server"] != "nginx" {
		t.Errorf("Headers[Server] = %q, want %q", decoded.Headers["Server"], "nginx")
	}
}

func TestExportPageWithHreflangAndRedirectChain(t *testing.T) {
	p := exportPage{
		URL:        "https://example.com/page",
		StatusCode: 301,
		Hreflang: []HreflangRow{
			{Lang: "en", URL: "https://example.com/en"},
			{Lang: "fr", URL: "https://example.com/fr"},
		},
		RedirectChain: []RedirectHopRow{
			{URL: "https://example.com/old", StatusCode: 301},
			{URL: "https://example.com/page", StatusCode: 200},
		},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded exportPage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(decoded.Hreflang) != 2 {
		t.Fatalf("Hreflang len = %d, want 2", len(decoded.Hreflang))
	}
	if decoded.Hreflang[0].Lang != "en" {
		t.Errorf("Hreflang[0].Lang = %q, want %q", decoded.Hreflang[0].Lang, "en")
	}
	if decoded.Hreflang[1].URL != "https://example.com/fr" {
		t.Errorf("Hreflang[1].URL = %q, want %q", decoded.Hreflang[1].URL, "https://example.com/fr")
	}

	if len(decoded.RedirectChain) != 2 {
		t.Fatalf("RedirectChain len = %d, want 2", len(decoded.RedirectChain))
	}
	if decoded.RedirectChain[0].StatusCode != 301 {
		t.Errorf("RedirectChain[0].StatusCode = %d, want 301", decoded.RedirectChain[0].StatusCode)
	}
}

func TestExportPageOmitEmptyFields(t *testing.T) {
	p := exportPage{
		URL:        "https://example.com/page",
		StatusCode: 200,
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}

	// hreflang, schema_types, headers, redirect_chain should be omitted when nil
	if _, ok := raw["hreflang"]; ok {
		t.Error("hreflang should be omitted when nil")
	}
	if _, ok := raw["schema_types"]; ok {
		t.Error("schema_types should be omitted when nil")
	}
	if _, ok := raw["headers"]; ok {
		t.Error("headers should be omitted when nil")
	}
	if _, ok := raw["redirect_chain"]; ok {
		t.Error("redirect_chain should be omitted when nil")
	}
}

func TestExportPageBodyHTMLOmitEmpty(t *testing.T) {
	p := exportPage{
		URL:        "https://example.com/page",
		StatusCode: 200,
		BodyHTML:   "<html><body>Hello</body></html>",
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded exportPage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.BodyHTML != "<html><body>Hello</body></html>" {
		t.Errorf("BodyHTML = %q, want <html><body>Hello</body></html>", decoded.BodyHTML)
	}

	// BodyHTML empty should be omitted
	p2 := exportPage{URL: "https://example.com/page2"}
	data2, err := json.Marshal(p2)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data2, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	if _, ok := raw["body_html"]; ok {
		t.Error("body_html should be omitted when empty")
	}
}

func TestExportLinkMarshalRoundtrip(t *testing.T) {
	l := exportLink{
		SourceURL:  "https://example.com/page",
		TargetURL:  "https://example.com/other",
		AnchorText: "Click here",
		Rel:        "nofollow",
		IsInternal: true,
		Tag:        "a",
		CrawledAt:  time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded exportLink
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.SourceURL != l.SourceURL {
		t.Errorf("SourceURL = %q, want %q", decoded.SourceURL, l.SourceURL)
	}
	if decoded.TargetURL != l.TargetURL {
		t.Errorf("TargetURL = %q, want %q", decoded.TargetURL, l.TargetURL)
	}
	if decoded.AnchorText != "Click here" {
		t.Errorf("AnchorText = %q, want %q", decoded.AnchorText, "Click here")
	}
	if decoded.Rel != "nofollow" {
		t.Errorf("Rel = %q, want %q", decoded.Rel, "nofollow")
	}
	if !decoded.IsInternal {
		t.Error("IsInternal = false, want true")
	}
	if decoded.Tag != "a" {
		t.Errorf("Tag = %q, want %q", decoded.Tag, "a")
	}
}

func TestExportRobotsMarshalRoundtrip(t *testing.T) {
	r := exportRobots{
		Host:       "example.com",
		StatusCode: 200,
		Content:    "User-agent: *\nDisallow: /admin/",
		FetchedAt:  time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded exportRobots
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Host != "example.com" {
		t.Errorf("Host = %q, want %q", decoded.Host, "example.com")
	}
	if decoded.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", decoded.StatusCode)
	}
	if decoded.Content != "User-agent: *\nDisallow: /admin/" {
		t.Errorf("Content mismatch")
	}
}

func TestExportSitemapMarshalRoundtrip(t *testing.T) {
	sm := exportSitemap{
		URL:        "https://example.com/sitemap.xml",
		Type:       "index",
		URLCount:   100,
		ParentURL:  "",
		StatusCode: 200,
		FetchedAt:  time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(sm)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded exportSitemap
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.URL != "https://example.com/sitemap.xml" {
		t.Errorf("URL = %q, want sitemap URL", decoded.URL)
	}
	if decoded.Type != "index" {
		t.Errorf("Type = %q, want %q", decoded.Type, "index")
	}
	if decoded.URLCount != 100 {
		t.Errorf("URLCount = %d, want 100", decoded.URLCount)
	}
}

func TestExportSitemapURLMarshalRoundtrip(t *testing.T) {
	su := exportSitemapURL{
		SitemapURL: "https://example.com/sitemap.xml",
		Loc:        "https://example.com/page1",
		LastMod:    "2025-01-01",
		ChangeFreq: "weekly",
		Priority:   "0.8",
	}

	data, err := json.Marshal(su)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded exportSitemapURL
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.SitemapURL != su.SitemapURL {
		t.Errorf("SitemapURL = %q, want %q", decoded.SitemapURL, su.SitemapURL)
	}
	if decoded.Loc != "https://example.com/page1" {
		t.Errorf("Loc = %q, want page1 URL", decoded.Loc)
	}
	if decoded.LastMod != "2025-01-01" {
		t.Errorf("LastMod = %q, want %q", decoded.LastMod, "2025-01-01")
	}
	if decoded.ChangeFreq != "weekly" {
		t.Errorf("ChangeFreq = %q, want %q", decoded.ChangeFreq, "weekly")
	}
	if decoded.Priority != "0.8" {
		t.Errorf("Priority = %q, want %q", decoded.Priority, "0.8")
	}
}

func TestExportRecordDataField(t *testing.T) {
	// Test that a page can be embedded as Data in an exportRecord
	p := exportPage{
		URL:        "https://example.com/test",
		StatusCode: 200,
		Title:      "Test",
	}
	pData, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal page: %v", err)
	}

	rec := exportRecord{
		Type: RecordPage,
		Data: pData,
	}

	encoded, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal record: %v", err)
	}

	var decoded exportRecord
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal record: %v", err)
	}
	if decoded.Type != RecordPage {
		t.Errorf("Type = %q, want %q", decoded.Type, RecordPage)
	}
	if decoded.Version != 0 {
		t.Errorf("Version = %d, want 0 (omitempty)", decoded.Version)
	}

	var decodedPage exportPage
	if err := json.Unmarshal(decoded.Data, &decodedPage); err != nil {
		t.Fatalf("Unmarshal page data: %v", err)
	}
	if decodedPage.URL != "https://example.com/test" {
		t.Errorf("page URL = %q, want test URL", decodedPage.URL)
	}
	if decodedPage.Title != "Test" {
		t.Errorf("page Title = %q, want %q", decodedPage.Title, "Test")
	}
}

func TestExportRecordConstants(t *testing.T) {
	if RecordMeta != "meta" {
		t.Errorf("RecordMeta = %q, want %q", RecordMeta, "meta")
	}
	if RecordPage != "page" {
		t.Errorf("RecordPage = %q, want %q", RecordPage, "page")
	}
	if RecordLink != "link" {
		t.Errorf("RecordLink = %q, want %q", RecordLink, "link")
	}
	if RecordRobots != "robots" {
		t.Errorf("RecordRobots = %q, want %q", RecordRobots, "robots")
	}
	if RecordSitemap != "sitemap" {
		t.Errorf("RecordSitemap = %q, want %q", RecordSitemap, "sitemap")
	}
	if RecordSitemapURL != "sitemap_url" {
		t.Errorf("RecordSitemapURL = %q, want %q", RecordSitemapURL, "sitemap_url")
	}
	if ExportFormatVersion != 1 {
		t.Errorf("ExportFormatVersion = %d, want 1", ExportFormatVersion)
	}
}

func TestExportRecordJSONFieldNames(t *testing.T) {
	rec := exportRecord{
		Type:    RecordMeta,
		Version: 1,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Check JSON field names match the expected short forms
	if _, ok := raw["t"]; !ok {
		t.Error("expected JSON field 't' for Type")
	}
	if _, ok := raw["v"]; !ok {
		t.Error("expected JSON field 'v' for Version")
	}

	// session and data should be omitted when nil
	if _, ok := raw["session"]; ok {
		t.Error("session should be omitted when nil")
	}
	if _, ok := raw["d"]; ok {
		t.Error("d should be omitted when nil/empty")
	}
}
