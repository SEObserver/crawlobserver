package apikeys

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// decodeCrawlHeaders reads the stored form of a project's crawl headers.
//
// It never fails: a row written before the column existed holds the empty
// string, and a row holding something unreadable is a stored value that no
// longer parses. Returning an empty map in both cases crawls without the
// headers, which is the same outcome as a project that never had any. The
// alternative — failing every read of the project — would take the whole
// project out of reach over a setting it does not need to be listed.
func decodeCrawlHeaders(stored string) map[string]string {
	if stored == "" {
		return map[string]string{}
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(stored), &headers); err != nil {
		return map[string]string{}
	}
	if headers == nil {
		return map[string]string{}
	}
	return headers
}

// SetProjectCrawlHeaders replaces the headers sent with this project's crawls.
// Passing an empty map removes them. Callers validate the headers before
// storing them; this only records what it is given.
func (s *Store) SetProjectCrawlHeaders(id string, headers map[string]string) error {
	stored := ""
	if len(headers) > 0 {
		encoded, err := json.Marshal(headers)
		if err != nil {
			return fmt.Errorf("encoding crawl headers: %w", err)
		}
		stored = string(encoded)
	}

	res, err := s.db.Exec(`UPDATE projects SET crawl_headers = ? WHERE id = ?`, stored, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("project not found")
	}
	return nil
}

// ProjectCrawlHeaders returns the headers to send with this project's crawls.
//
// A project that has none, and a project id that names nothing, both answer an
// empty map and no error: a crawl whose project was deleted mid-flight should
// carry no headers rather than fail to start. Any other database failure is
// returned, because it is the difference between a project that asked for no
// headers and one whose headers could not be read — and a crawl that silently
// goes out unsigned is diagnosed at the far end as a robots.txt refusal.
func (s *Store) ProjectCrawlHeaders(projectID string) (map[string]string, error) {
	if projectID == "" {
		return map[string]string{}, nil
	}
	var stored string
	err := s.db.QueryRow(`SELECT crawl_headers FROM projects WHERE id = ?`, projectID).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading crawl headers of project %s: %w", projectID, err)
	}
	return decodeCrawlHeaders(stored), nil
}
