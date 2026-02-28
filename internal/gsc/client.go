package gsc

import (
	"context"
	"fmt"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	searchconsole "google.golang.org/api/searchconsole/v1"
	"google.golang.org/api/option"
)

type Client struct {
	service *searchconsole.Service
}

type AnalyticsRow struct {
	Date        string
	Query       string
	Page        string
	Country     string
	Device      string
	Clicks      int64
	Impressions int64
	CTR         float64
	Position    float64
}

type InspectionRow struct {
	URL              string
	Verdict          string
	CoverageState    string
	IndexingState    string
	RobotsTxtState   string
	LastCrawlTime    time.Time
	CrawledAs        string
	CanonicalURL     string
	IsGoogleCanonical bool
	MobileUsability  string
	RichResultsItems int
}

type Property struct {
	SiteURL         string `json:"site_url"`
	PermissionLevel string `json:"permission_level"`
}

type Sitemap struct {
	Path         string `json:"path"`
	Type         string `json:"type"`
	IsPending    bool   `json:"is_pending"`
	Warnings     int64  `json:"warnings"`
	Errors       int64  `json:"errors"`
	Contents     []SitemapContent `json:"contents"`
}

type SitemapContent struct {
	Type      string `json:"type"`
	Submitted int64  `json:"submitted"`
	Indexed   int64  `json:"indexed"`
}

func OAuthConfig(cfg *config.GSCConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURI,
		Scopes:       []string{"https://www.googleapis.com/auth/webmasters.readonly"},
		Endpoint:     google.Endpoint,
	}
}

func AuthorizeURL(cfg *config.GSCConfig, state string) string {
	oauthCfg := OAuthConfig(cfg)
	return oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
}

func ExchangeCode(ctx context.Context, cfg *config.GSCConfig, code string) (*oauth2.Token, error) {
	oauthCfg := OAuthConfig(cfg)
	token, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchanging code: %w", err)
	}
	return token, nil
}

func NewClient(ctx context.Context, cfg *config.GSCConfig, token *oauth2.Token) (*Client, error) {
	oauthCfg := OAuthConfig(cfg)
	tokenSource := oauthCfg.TokenSource(ctx, token)

	svc, err := searchconsole.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, fmt.Errorf("creating searchconsole service: %w", err)
	}
	return &Client{service: svc}, nil
}

// NewClientFromTokens creates a client from raw token strings, refreshing if needed.
func NewClientFromTokens(ctx context.Context, cfg *config.GSCConfig, accessToken, refreshToken string, expiry time.Time) (*Client, *oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       expiry,
		TokenType:    "Bearer",
	}
	oauthCfg := OAuthConfig(cfg)
	tokenSource := oauthCfg.TokenSource(ctx, token)

	// This may refresh the token
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, nil, fmt.Errorf("refreshing token: %w", err)
	}

	svc, err := searchconsole.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, nil, fmt.Errorf("creating searchconsole service: %w", err)
	}
	return &Client{service: svc}, newToken, nil
}

func (c *Client) ListProperties(ctx context.Context) ([]Property, error) {
	resp, err := c.service.Sites.List().Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("listing properties: %w", err)
	}
	var props []Property
	for _, s := range resp.SiteEntry {
		props = append(props, Property{
			SiteURL:         s.SiteUrl,
			PermissionLevel: s.PermissionLevel,
		})
	}
	return props, nil
}

// BatchCallback is called after each batch of rows is fetched from the API.
// rows contains the current batch; totalSoFar is the running total of rows fetched.
type BatchCallback func(rows []AnalyticsRow, totalSoFar int) error

func (c *Client) FetchSearchAnalytics(ctx context.Context, propertyURL, startDate, endDate string, onBatch BatchCallback) (int, error) {
	startRow := int64(0)
	rowLimit := int64(25000)
	total := 0

	for {
		applog.Infof("gsc", "querying analytics startRow=%d ...", startRow)
		req := &searchconsole.SearchAnalyticsQueryRequest{
			StartDate:  startDate,
			EndDate:    endDate,
			Dimensions: []string{"date", "query", "page", "country", "device"},
			RowLimit:   rowLimit,
			StartRow:   startRow,
		}

		resp, err := c.service.Searchanalytics.Query(propertyURL, req).Context(ctx).Do()
		if err != nil {
			return total, fmt.Errorf("querying search analytics (startRow=%d): %w", startRow, err)
		}

		var batch []AnalyticsRow
		for _, row := range resp.Rows {
			if len(row.Keys) < 5 {
				continue
			}
			batch = append(batch, AnalyticsRow{
				Date:        row.Keys[0],
				Query:       row.Keys[1],
				Page:        row.Keys[2],
				Country:     row.Keys[3],
				Device:      row.Keys[4],
				Clicks:      int64(row.Clicks),
				Impressions: int64(row.Impressions),
				CTR:         row.Ctr,
				Position:    row.Position,
			})
		}
		total += len(batch)
		applog.Infof("gsc", "got %d rows (total so far: %d)", len(batch), total)

		if onBatch != nil {
			if err := onBatch(batch, total); err != nil {
				return total, fmt.Errorf("batch callback: %w", err)
			}
		}

		if len(resp.Rows) < int(rowLimit) {
			break
		}
		startRow += rowLimit
	}

	return total, nil
}

func (c *Client) FetchURLInspection(ctx context.Context, propertyURL string, urls []string) ([]InspectionRow, error) {
	var results []InspectionRow
	for _, u := range urls {
		req := &searchconsole.InspectUrlIndexRequest{
			InspectionUrl: u,
			SiteUrl:       propertyURL,
		}
		resp, err := c.service.UrlInspection.Index.Inspect(req).Context(ctx).Do()
		if err != nil {
			// Skip individual URL errors, continue with others
			results = append(results, InspectionRow{
				URL:     u,
				Verdict: fmt.Sprintf("ERROR: %v", err),
			})
			continue
		}

		row := InspectionRow{
			URL: u,
		}
		if r := resp.InspectionResult; r != nil {
			if r.IndexStatusResult != nil {
				row.Verdict = r.IndexStatusResult.Verdict
				row.CoverageState = r.IndexStatusResult.CoverageState
				row.IndexingState = r.IndexStatusResult.IndexingState
				row.RobotsTxtState = r.IndexStatusResult.RobotsTxtState
				if r.IndexStatusResult.LastCrawlTime != "" {
					if t, err := time.Parse(time.RFC3339, r.IndexStatusResult.LastCrawlTime); err == nil {
						row.LastCrawlTime = t
					}
				}
				row.CrawledAs = r.IndexStatusResult.CrawledAs
				row.CanonicalURL = r.IndexStatusResult.GoogleCanonical
				row.IsGoogleCanonical = r.IndexStatusResult.PageFetchState == "SUCCESSFUL"
			}
			if r.MobileUsabilityResult != nil {
				row.MobileUsability = r.MobileUsabilityResult.Verdict
			}
			if r.RichResultsResult != nil {
				for _, det := range r.RichResultsResult.DetectedItems {
					row.RichResultsItems += len(det.Items)
				}
			}
		}
		results = append(results, row)
	}
	return results, nil
}

func (c *Client) ListSitemaps(ctx context.Context, propertyURL string) ([]Sitemap, error) {
	resp, err := c.service.Sitemaps.List(propertyURL).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("listing sitemaps: %w", err)
	}
	var sitemaps []Sitemap
	for _, s := range resp.Sitemap {
		sm := Sitemap{
			Path:      s.Path,
			Type:      s.Type,
			IsPending: s.IsPending,
			Warnings:  s.Warnings,
			Errors:    s.Errors,
		}
		for _, c := range s.Contents {
			sm.Contents = append(sm.Contents, SitemapContent{
				Type:      c.Type,
				Submitted: c.Submitted,
				Indexed:   c.Indexed,
			})
		}
		sitemaps = append(sitemaps, sm)
	}
	return sitemaps, nil
}
