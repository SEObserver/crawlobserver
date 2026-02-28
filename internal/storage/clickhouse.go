package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Store manages ClickHouse connections and operations.
type Store struct {
	conn driver.Conn
}

// NewStore creates a new ClickHouse store.
func NewStore(host string, port int, database, username, password string) (*Store, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", host, port)},
		Auth: clickhouse.Auth{
			Database: database,
			Username: username,
			Password: password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time":                 60,
			"max_memory_usage":                   4000000000, // 4GB per query
			"max_bytes_before_external_group_by": 500000000,  // 500MB, then spill to disk
			"max_bytes_before_external_sort":     500000000,  // 500MB, then spill to disk
			"join_algorithm":                     "auto",     // auto-select hash or partial_merge based on memory
		},
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to ClickHouse: %w", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("pinging ClickHouse: %w", err)
	}

	return &Store{conn: conn}, nil
}

// Migrate runs all DDL migrations.
func (s *Store) Migrate(ctx context.Context) error {
	for i, m := range Migrations {
		if m.Fn != nil {
			if err := m.Fn(ctx, s.conn); err != nil {
				return fmt.Errorf("migration %d (%s): %w", i+1, m.Name, err)
			}
		} else {
			if err := s.conn.Exec(ctx, m.DDL); err != nil {
				return fmt.Errorf("migration %d (%s): %w", i+1, m.Name, err)
			}
		}
	}
	return nil
}

// Close closes the ClickHouse connection.
func (s *Store) Close() error {
	return s.conn.Close()
}
