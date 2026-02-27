package cli

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	chmanaged "github.com/SEObserver/seocrawler/internal/clickhouse"
	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/storage"
)

// setupClickHouse connects to ClickHouse, auto-detecting or managing a subprocess as needed.
// Returns the store, a cleanup function (stops managed CH if applicable), and any error.
func setupClickHouse(cfg *config.Config, connectDB string) (*storage.Store, func(), error) {
	noop := func() {}

	mode := cfg.ClickHouse.Mode
	if mode == "" {
		mode = detectMode(cfg)
	}

	switch mode {
	case "external":
		log.Printf("Using external ClickHouse at %s:%d", cfg.ClickHouse.Host, cfg.ClickHouse.Port)
		store, err := storage.NewStore(
			cfg.ClickHouse.Host,
			cfg.ClickHouse.Port,
			connectDB,
			cfg.ClickHouse.Username,
			cfg.ClickHouse.Password,
		)
		if err != nil {
			return nil, noop, fmt.Errorf("connecting to external ClickHouse: %w", err)
		}
		return store, noop, nil

	case "managed":
		dataDir := cfg.ClickHouse.DataDir
		if dataDir == "" {
			dataDir = chmanaged.DefaultDataDir()
		}

		binaryPath := chmanaged.FindBinary(cfg.ClickHouse.BinaryPath, dataDir)
		if binaryPath == "" {
			log.Println("No ClickHouse binary found, downloading...")
			var err error
			binaryPath, err = chmanaged.DownloadBinary(dataDir)
			if err != nil {
				return nil, noop, fmt.Errorf("downloading ClickHouse: %w", err)
			}
		}

		srv := chmanaged.NewManagedServer(dataDir)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := srv.Start(ctx, binaryPath); err != nil {
			return nil, noop, fmt.Errorf("starting managed ClickHouse: %w", err)
		}

		cleanup := func() { srv.Stop() }

		store, err := storage.NewStore(
			"127.0.0.1",
			srv.TCPPort(),
			connectDB,
			"default",
			"",
		)
		if err != nil {
			srv.Stop()
			return nil, noop, fmt.Errorf("connecting to managed ClickHouse: %w", err)
		}

		return store, cleanup, nil

	default:
		return nil, noop, fmt.Errorf("unknown clickhouse.mode: %q", mode)
	}
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
