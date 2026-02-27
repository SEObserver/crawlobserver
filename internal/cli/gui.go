//go:build desktop

package cli

import (
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/SEObserver/seocrawler/internal/apikeys"
	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/server"
	"github.com/spf13/cobra"
	"github.com/wailsapp/wails/v2"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed appicon.png
var appIcon []byte

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Start the desktop GUI",
	Long:  `Start the native desktop application with embedded web UI.`,
	RunE:  runGUI,
}

func init() {
	rootCmd.AddCommand(guiCmd)

	// Make "gui" the default command when no subcommand is given (double-click .app)
	defaultCmd := guiCmd
	originalRun := rootCmd.RunE
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if originalRun != nil {
			return originalRun(cmd, args)
		}
		return defaultCmd.RunE(cmd, args)
	}
}

func runGUI(cmd *cobra.Command, args []string) error {
	// Ensure data directory exists for GUI mode (macOS launches .app with cwd=/)
	dataDir, err := appDataDir()
	if err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Resolve relative paths to the app data directory
	if !filepath.IsAbs(cfg.Server.SQLitePath) {
		cfg.Server.SQLitePath = filepath.Join(dataDir, cfg.Server.SQLitePath)
	}

	store, cleanup, err := setupClickHouse(cfg, cfg.ClickHouse.Database)
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

	// In GUI mode, use a random available port to avoid conflicts
	// (the user never sees this port — Wails proxies everything)
	guiPort, err := findFreePort()
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}
	cfg.Server.Port = guiPort
	log.Printf("GUI mode: using internal HTTP port %d", guiPort)

	srv := server.New(cfg, store, keyStore)
	srv.NoBrowserOpen = true

	// Start the HTTP server in the background
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for server to be ready
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", guiPort)
	waitForServer(serverURL, 10*time.Second)

	// Build reverse proxy to our localhost server
	target, _ := url.Parse(serverURL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// If auth is configured, inject it into proxied requests
	var authHeader string
	if cfg.Server.Username != "" && cfg.Server.Password != "" {
		creds := base64.StdEncoding.EncodeToString([]byte(cfg.Server.Username + ":" + cfg.Server.Password))
		authHeader = "Basic " + creds
	}

	proxyMiddleware := assetserver.ChainMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authHeader != "" {
				r.Header.Set("Authorization", authHeader)
			}
			proxy.ServeHTTP(w, r)
		})
	})

	appName := "SEOCrawler"
	if cfg.Theme.AppName != "" {
		appName = cfg.Theme.AppName
	}

	err = wails.Run(&options.App{
		Title:     appName,
		Width:     1440,
		Height:    900,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Middleware: proxyMiddleware,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: false,
			About: &mac.AboutInfo{
				Title:   appName,
				Message: "SEO Crawler & Analyzer",
				Icon:    appIcon,
			},
		},
		OnDomReady: func(ctx context.Context) {
			// Add top padding for the hidden-inset title bar (macOS traffic lights)
			wailsRuntime.WindowExecJS(ctx, `document.documentElement.style.setProperty('--topbar-height','36px')`)
		},
		OnShutdown: func(ctx context.Context) {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			srv.Stop(shutdownCtx)
		},
	})

	return err
}

func waitForServer(url string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/api/health")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Println("Warning: server may not be ready")
}

// findFreePort asks the OS for an available port.
func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// appDataDir returns ~/Library/Application Support/SEOCrawler (macOS) or equivalent,
// creating it if needed.
func appDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "Application Support", "SEOCrawler")
	return dir, os.MkdirAll(dir, 0755)
}
