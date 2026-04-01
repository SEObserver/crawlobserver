package storage

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CriticalTables lists the non-regenerable tables that must be backed up separately.
var CriticalTables = []string{
	"gsc_analytics",
	"gsc_inspection",
	"provider_domain_metrics",
	"provider_backlinks",
	"provider_refdomains",
	"provider_rankings",
	"provider_visibility",
	"provider_top_pages",
	"provider_data",
}

// ExportCriticalTables exports each non-regenerable table to a separate gzipped JSONL file.
// Files are written as <dir>/<table>_<timestamp>.jsonl.gz.
// Errors on individual tables are accumulated — the function exports as many tables as possible.
func (s *Store) ExportCriticalTables(ctx context.Context, dir string, retain int) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating export dir: %w", err)
	}

	ts := time.Now().Format("20060102T150405")

	var errs []string
	for _, table := range CriticalTables {
		if err := s.exportCriticalTable(ctx, dir, table, ts); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", table, err))
		}
	}

	// Prune old exports
	if retain > 0 {
		for _, table := range CriticalTables {
			pruneTableExports(dir, table, retain)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("export errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (s *Store) exportCriticalTable(ctx context.Context, dir, table, ts string) error {
	// Check if table has any data
	var count uint64
	if err := s.conn.QueryRow(ctx, fmt.Sprintf("SELECT count() FROM %s", table)).Scan(&count); err != nil {
		// Table might not exist yet — skip silently
		return nil
	}
	if count == 0 {
		return nil
	}

	filename := fmt.Sprintf("%s_%s.jsonl.gz", table, ts)
	path := filepath.Join(dir, filename)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	enc := json.NewEncoder(gw)

	// Stream all rows without LIMIT/OFFSET (deterministic, no missed/duplicated rows).
	// The driver handles streaming natively — rows are fetched in blocks.
	query := fmt.Sprintf("SELECT * FROM %s", table)
	rows, err := s.conn.Query(ctx, query)
	if err != nil {
		os.Remove(path)
		return fmt.Errorf("querying: %w", err)
	}
	defer rows.Close()

	colNames := rows.Columns()

	for rows.Next() {
		// Use interface{} for all columns — the driver handles type
		// conversion including Nullable, LowCardinality, Array, Tuple, etc.
		values := make([]interface{}, len(colNames))
		for i := range values {
			values[i] = new(interface{})
		}
		if err := rows.Scan(values...); err != nil {
			os.Remove(path)
			return fmt.Errorf("scanning row: %w", err)
		}

		row := make(map[string]interface{}, len(colNames))
		for i, name := range colNames {
			// Dereference the *interface{}
			row[name] = *(values[i].(*interface{}))
		}
		if err := enc.Encode(row); err != nil {
			os.Remove(path)
			return fmt.Errorf("encoding row: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		os.Remove(path)
		return fmt.Errorf("iterating rows: %w", err)
	}

	return nil
}

// ImportCriticalTable reads a JSONL file and inserts rows into the named table.
func (s *Store) ImportCriticalTable(ctx context.Context, table string, r io.Reader) error {
	dec := json.NewDecoder(r)
	var batch []map[string]interface{}
	const batchSize = 1000

	for {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decoding JSONL: %w", err)
		}
		batch = append(batch, row)

		if len(batch) >= batchSize {
			if err := s.insertCriticalBatch(ctx, table, batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		return s.insertCriticalBatch(ctx, table, batch)
	}
	return nil
}

func (s *Store) insertCriticalBatch(ctx context.Context, table string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	// Build column list from first row
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table, strings.Join(cols, ", "), strings.Join(placeholders, ", "))

	batch, err := s.conn.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("preparing batch: %w", err)
	}

	for _, row := range rows {
		values := make([]interface{}, len(cols))
		for i, col := range cols {
			values[i] = row[col]
		}
		if err := batch.Append(values...); err != nil {
			return fmt.Errorf("appending row: %w", err)
		}
	}

	return batch.Send()
}

// pruneTableExports keeps only the most recent N exports for a given table.
func pruneTableExports(dir, table string, keep int) {
	prefix := table + "_"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var matches []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".jsonl.gz") {
			matches = append(matches, e.Name())
		}
	}

	// Sort ascending (oldest first by name since timestamp is embedded)
	sort.Strings(matches)

	if len(matches) <= keep {
		return
	}
	for _, name := range matches[:len(matches)-keep] {
		os.Remove(filepath.Join(dir, name))
	}
}
