package storage

import (
	"context"
	"fmt"

	"github.com/SEObserver/crawlobserver/internal/schema"
)

// InsertStructuredData batch inserts structured data validation items.
func (s *Store) InsertStructuredData(ctx context.Context, items []schema.StructuredDataItem) error {
	if len(items) == 0 {
		return nil
	}

	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO crawlobserver.structured_data_items (
			crawl_session_id, url, schema_type, json_ld,
			errors, warnings, is_valid, source, crawled_at
		)`)
	if err != nil {
		return fmt.Errorf("preparing structured_data batch: %w", err)
	}

	for _, item := range items {
		if err := batch.Append(
			item.CrawlSessionID,
			item.URL,
			item.SchemaType,
			item.JSONLD,
			item.Errors,
			item.Warnings,
			item.IsValid,
			item.Source,
			item.CrawledAt,
		); err != nil {
			return fmt.Errorf("appending structured_data row: %w", err)
		}
	}

	return batch.Send()
}

// GetStructuredData retrieves structured data items for a specific URL in a session.
func (s *Store) GetStructuredData(ctx context.Context, sessionID, url string) ([]schema.StructuredDataItem, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT crawl_session_id, url, schema_type, json_ld,
			errors, warnings, is_valid, source, crawled_at
		FROM crawlobserver.structured_data_items FINAL
		WHERE crawl_session_id = ? AND url = ?
		ORDER BY crawled_at DESC`, sessionID, url)
	if err != nil {
		return nil, fmt.Errorf("querying structured_data: %w", err)
	}
	defer rows.Close()

	var items []schema.StructuredDataItem
	for rows.Next() {
		var item schema.StructuredDataItem
		if err := rows.Scan(
			&item.CrawlSessionID,
			&item.URL,
			&item.SchemaType,
			&item.JSONLD,
			&item.Errors,
			&item.Warnings,
			&item.IsValid,
			&item.Source,
			&item.CrawledAt,
		); err != nil {
			return nil, fmt.Errorf("scanning structured_data row: %w", err)
		}
		items = append(items, item)
	}

	return items, rows.Err()
}
