package crawler

import "fmt"

// Validate before touching storage, especially before deleting failed pages
// for a retry. Omission inherits the running or saved configuration.
func validateLinkPositionLimit(req *CrawlRequest) error {
	if req != nil && req.MaxLinkPositionsPerPage != nil && *req.MaxLinkPositionsPerPage < 1 {
		return fmt.Errorf("max_link_positions_per_page must be >= 1")
	}
	return nil
}
