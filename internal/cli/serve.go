package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/server"
	"github.com/SEObserver/crawlobserver/internal/telemetry"
	"github.com/SEObserver/crawlobserver/internal/updater"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server",
	Long:  `Start the web server and open the browser UI for browsing crawl results.`,
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().Int("port", 0, "Port for the web server")
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if port, _ := cmd.Flags().GetInt("port"); port > 0 {
		cfg.Server.Port = port
	}

	// Windows without external ClickHouse → guided setup mode
	mode := cfg.ClickHouse.Mode
	if mode == "" {
		mode = detectMode(cfg)
	}
	if runtime.GOOS == "windows" && mode == "managed" {
		return runServeSetupMode(cfg)
	}

	store, cleanup, _, err := setupClickHouse(cfg, cfg.ClickHouse.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	defer cleanup()

	keyStore, err := apikeys.NewStore(cfg.Server.SQLitePath)
	if err != nil {
		return fmt.Errorf("opening SQLite store: %w", err)
	}
	defer keyStore.Close()

	srv := server.New(cfg, store, keyStore)
	srv.UpdateStatus = updater.NewUpdateStatus()

	// Background update check
	go func() {
		time.Sleep(3 * time.Second)
		srv.UpdateStatus.Check()
		snap := srv.UpdateStatus.Snapshot()
		if snap.Available {
			applog.Infof("cli", "Update available: %s -> %s  (run 'crawlobserver update' to install)", snap.CurrentVersion, snap.LatestVersion)
		}
	}()

	defer telemetry.Close()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		applog.Info("cli", "Shutting down web server...")
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		srv.Stop(ctx)
	}()

	return srv.Start()
}

// runServeSetupMode starts the server in setup mode on Windows when no ClickHouse is available.
// The onboarding wizard guides the user through installing ClickHouse via Docker or WSL.
// A background goroutine polls for ClickHouse availability and transitions to ready automatically.
func runServeSetupMode(cfg *config.Config) error {
	applog.Info("cli", "Windows detected without ClickHouse — starting in setup mode")

	srv := server.NewSetupServer(cfg)
	srv.UpdateStatus = updater.NewUpdateStatus()

	var (
		mu        sync.Mutex
		cleanupFn func()
	)

	// Background goroutine: poll for ClickHouse, then setup
	go func() {
		for {
			if detectMode(cfg) == "external" {
				break
			}
			time.Sleep(3 * time.Second)
		}

		applog.Info("cli", "ClickHouse detected, completing setup...")

		store, cleanup, _, err := setupClickHouse(cfg, cfg.ClickHouse.Database)
		if err != nil {
			applog.Errorf("cli", "ClickHouse setup failed: %v", err)
			return
		}

		keyStore, err := apikeys.NewStore(cfg.Server.SQLitePath)
		if err != nil {
			store.Close()
			cleanup()
			applog.Errorf("cli", "SQLite store failed: %v", err)
			return
		}

		mu.Lock()
		cleanupFn = func() {
			keyStore.Close()
			store.Close()
			cleanup()
		}
		mu.Unlock()

		srv.TransitionToReady(store, keyStore)
		applog.Init(store)
		srv.SetDownloadProgress(server.SetupProgress{Percent: 100})

		// Background update check
		go func() {
			time.Sleep(5 * time.Second)
			srv.UpdateStatus.Check()
		}()

		applog.Info("cli", "Setup complete — server is ready")
	}()

	defer telemetry.Close()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		applog.Info("cli", "Shutting down web server...")
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		srv.Stop(ctx)
	}()

	err := srv.Start()

	mu.Lock()
	if cleanupFn != nil {
		cleanupFn()
	}
	mu.Unlock()

	return err
}
