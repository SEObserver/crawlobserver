package crawler

import (
	"encoding/json"
	"time"

	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/storage"
	"github.com/google/uuid"
)

// Session represents a single crawl session lifecycle.
type Session struct {
	ID        string
	StartedAt time.Time
	SeedURLs  []string
	Config    *config.Config
	Status    string
	Pages     uint64
	ProjectID *string
}

// NewSession creates a new crawl session.
func NewSession(seeds []string, cfg *config.Config) *Session {
	return &Session{
		ID:        uuid.New().String(),
		StartedAt: time.Now(),
		SeedURLs:  seeds,
		Config:    cfg,
		Status:    "running",
	}
}

// ToStorageRow converts a Session to a storage model.
func (s *Session) ToStorageRow() *storage.CrawlSession {
	configJSON, _ := json.Marshal(s.Config)
	return &storage.CrawlSession{
		ID:           s.ID,
		StartedAt:    s.StartedAt,
		Status:       s.Status,
		SeedURLs:     s.SeedURLs,
		Config:       string(configJSON),
		PagesCrawled: s.Pages,
		UserAgent:    s.Config.Crawler.UserAgent,
		ProjectID:    s.ProjectID,
	}
}
