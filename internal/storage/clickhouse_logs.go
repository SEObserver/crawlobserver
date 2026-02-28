package storage

import (
	"context"
	"fmt"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

// InsertLogs batch inserts application log rows.
func (s *Store) InsertLogs(ctx context.Context, logs []applog.LogRow) error {
	if len(logs) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO crawlobserver.application_logs (timestamp, level, component, message, context)`)
	if err != nil {
		return fmt.Errorf("preparing log batch: %w", err)
	}
	for _, l := range logs {
		if err := batch.Append(l.Timestamp, l.Level, l.Component, l.Message, l.Context); err != nil {
			return fmt.Errorf("appending log row: %w", err)
		}
	}
	return batch.Send()
}

// ListLogs returns paginated application logs with optional filters.
func (s *Store) ListLogs(ctx context.Context, limit, offset int, level, component, search string) ([]applog.LogRow, int, error) {
	where := "1=1"
	args := []any{}
	if level != "" {
		where += " AND level = ?"
		args = append(args, level)
	}
	if component != "" {
		where += " AND component = ?"
		args = append(args, component)
	}
	if search != "" {
		where += " AND message ILIKE ?"
		args = append(args, "%"+search+"%")
	}

	// Count
	var total uint64
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	if err := s.conn.QueryRow(ctx, "SELECT count() FROM crawlobserver.application_logs WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting logs: %w", err)
	}

	// Query
	q := fmt.Sprintf("SELECT timestamp, level, component, message, context FROM crawlobserver.application_logs WHERE %s ORDER BY timestamp DESC LIMIT %d OFFSET %d", where, limit, offset)
	rows, err := s.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying logs: %w", err)
	}
	defer rows.Close()

	var results []applog.LogRow
	for rows.Next() {
		var r applog.LogRow
		if err := rows.Scan(&r.Timestamp, &r.Level, &r.Component, &r.Message, &r.Context); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, int(total), nil
}

// ExportLogs returns all logs (up to 7 days per TTL) for JSONL export.
func (s *Store) ExportLogs(ctx context.Context) ([]applog.LogRow, error) {
	rows, err := s.conn.Query(ctx, `SELECT timestamp, level, component, message, context FROM crawlobserver.application_logs ORDER BY timestamp DESC`)
	if err != nil {
		return nil, fmt.Errorf("exporting logs: %w", err)
	}
	defer rows.Close()

	var results []applog.LogRow
	for rows.Next() {
		var r applog.LogRow
		if err := rows.Scan(&r.Timestamp, &r.Level, &r.Component, &r.Message, &r.Context); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}
