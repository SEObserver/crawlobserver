package providers

import "time"

// ProviderConnection represents a connection to an external data provider for a project.
type ProviderConnection struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	Provider        string    `json:"provider"`
	Domain          string    `json:"domain"`
	APIKey          string    `json:"-"`
	LimitBacklinks  int       `json:"limit_backlinks"`
	LimitRefdomains int       `json:"limit_refdomains"`
	LimitRankings   int       `json:"limit_rankings"`
	LimitTopPages   int       `json:"limit_top_pages"`
	CreatedAt       time.Time `json:"created_at"`
}

const DefaultLimit = 1000

func EffectiveLimit(v int) int {
	if v <= 0 {
		return DefaultLimit
	}
	return v
}
