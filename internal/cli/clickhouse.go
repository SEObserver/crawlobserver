package cli

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	chmanaged "github.com/SEObserver/crawlobserver/internal/clickhouse"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

// setupClickHouse connects to ClickHouse, auto-detecting or managing a subprocess as needed.
// It auto-runs migrations and returns a store connected to the crawlobserver database.
// Returns the store, a cleanup function (stops managed CH if applicable),
// the ManagedServer (nil for external mode), and any error.
func setupClickHouse(cfg *config.Config, connectDB string) (*storage.Store, func(), *chmanaged.ManagedServer, error) {
	noop := func() {}

	mode := cfg.ClickHouse.Mode
	if mode == "" {
		mode = detectMode(cfg)
	}

	var host, username, password string
	var port int
	cleanup := noop
	var managed *chmanaged.ManagedServer

	switch mode {
	case "external":
		applog.Infof("cli", "Using external ClickHouse at %s:%d", cfg.ClickHouse.Host, cfg.ClickHouse.Port)
		host = cfg.ClickHouse.Host
		port = cfg.ClickHouse.Port
		username = cfg.ClickHouse.Username
		password = cfg.ClickHouse.Password

	case "managed":
		dataDir := cfg.ClickHouse.DataDir
		if dataDir == "" {
			dataDir = chmanaged.DefaultDataDir()
		}

		binaryPath := chmanaged.FindBinary(cfg.ClickHouse.BinaryPath, dataDir)
		if binaryPath == "" {
			applog.Info("cli", "No ClickHouse binary found, downloading...")
			var err error
			binaryPath, err = chmanaged.DownloadBinary(dataDir)
			if err != nil {
				return nil, noop, nil, fmt.Errorf("downloading ClickHouse: %w", err)
			}
		}

		srv := chmanaged.NewManagedServer(dataDir)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := srv.Start(ctx, binaryPath); err != nil {
			return nil, noop, nil, fmt.Errorf("starting managed ClickHouse: %w", err)
		}

		host = "127.0.0.1"
		port = srv.TCPPort()
		username = "default"
		password = ""
		cleanup = func() { srv.Stop() }
		managed = srv

	default:
		return nil, noop, nil, fmt.Errorf("unknown clickhouse.mode: %q", mode)
	}

	// Auto-migrate: connect to default db, run migrations, then connect to target db
	if connectDB != "default" {
		initStore, err := storage.NewStore(host, port, "default", username, password)
		if err != nil {
			cleanup()
			return nil, noop, nil, fmt.Errorf("connecting for migrations: %w", err)
		}
		applog.Info("cli", "Running auto-migrations...")
		if err := initStore.Migrate(context.Background()); err != nil {
			initStore.Close()
			cleanup()
			return nil, noop, nil, fmt.Errorf("auto-migration: %w", err)
		}
		initStore.Close()
	}

	store, err := storage.NewStore(host, port, connectDB, username, password)
	if err != nil {
		cleanup()
		return nil, noop, nil, fmt.Errorf("connecting to ClickHouse: %w", err)
	}

	// If connecting to default (migrate command), run migrations on this store directly
	if connectDB == "default" {
		applog.Info("cli", "Running migrations...")
		if err := store.Migrate(context.Background()); err != nil {
			store.Close()
			cleanup()
			return nil, noop, nil, fmt.Errorf("migration: %w", err)
		}
		applog.Info("cli", "Migrations complete.")
	}

	return store, cleanup, managed, nil
}

// detectMode auto-detects whether to use external or managed mode.
func detectMode(cfg *config.Config) string {
	addr := net.JoinHostPort(cfg.ClickHouse.Host, strconv.Itoa(cfg.ClickHouse.Port))
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err == nil {
		conn.Close()
		return "external"
	}
	return "managed"
}
