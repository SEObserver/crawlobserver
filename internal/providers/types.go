package providers

import "time"

// ProviderConnection represents a connection to an external data provider for a project.
type ProviderConnection struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Provider  string    `json:"provider"`
	Domain    string    `json:"domain"`
	APIKey    string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}
