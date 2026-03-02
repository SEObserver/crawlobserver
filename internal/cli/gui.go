//go:build desktop

package cli

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SEObserver/crawlobserver/internal/apikeys"
	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/backup"
	chmanaged "github.com/SEObserver/crawlobserver/internal/clickhouse"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/server"
	"github.com/SEObserver/crawlobserver/internal/updater"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	webview "github.com/webview/webview_go"
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

	// Point viper to writable config in app data dir (cwd is / in .app bundles)
	viper.SetConfigFile(filepath.Join(dataDir, "config.yaml"))
	_ = viper.ReadInConfig()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Resolve relative paths to the app data directory
	if !filepath.IsAbs(cfg.Server.SQLitePath) {
		cfg.Server.SQLitePath = filepath.Join(dataDir, cfg.Server.SQLitePath)
	}

	// In desktop mode, auth is unnecessary — server listens on 127.0.0.1 only
	cfg.Server.Username = ""
	cfg.Server.Password = ""

	// Find port BEFORE creating the webview (needed for the loading page's health poll)
	guiPort, err := findFreePort()
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}
	cfg.Server.Port = guiPort
	applog.Infof("cli", "GUI mode: using internal HTTP port %d", guiPort)

	serverURL := fmt.Sprintf("http://127.0.0.1:%d", guiPort)

	appName := "CrawlObserver"
	if cfg.Theme.AppName != "" {
		appName = cfg.Theme.AppName
	}

	// Create native webview window IMMEDIATELY with a loading splash screen
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle(appName)
	w.SetSize(1440, 900, webview.HintNone)
	w.SetSize(800, 600, webview.HintMin)
	w.Navigate("data:text/html," + splashHTML(guiPort))

	// Resources created by the background goroutine, protected by mutex
	var mu sync.Mutex
	var srv *server.Server
	var store interface{ Close() error }
	var keyStore *apikeys.Store
	var chCleanup func()

	// Launch setup in background goroutine
	go func() {
		setupErr := func() error {
			s, cleanup, managedCH, err := setupClickHouse(cfg, cfg.ClickHouse.Database)
			if err != nil {
				return fmt.Errorf("ClickHouse setup: %w", err)
			}

			ks, err := apikeys.NewStore(cfg.Server.SQLitePath)
			if err != nil {
				s.Close()
				cleanup()
				return fmt.Errorf("opening SQLite store: %w", err)
			}

			httpSrv := server.New(cfg, s, ks)
			httpSrv.NoBrowserOpen = true
			httpSrv.IsDesktop = true

			// Wire update status
			httpSrv.UpdateStatus = updater.NewUpdateStatus()

			// Wire backup options
			chDataDir := cfg.ClickHouse.DataDir
			if chDataDir == "" {
				chDataDir = chmanaged.DefaultDataDir()
			}
			backupDir := filepath.Join(dataDir, "backups")
			configPath := viper.ConfigFileUsed()

			httpSrv.BackupOpts = &backup.BackupOptions{
				DataDir:    chDataDir,
				SQLitePath: cfg.Server.SQLitePath,
				ConfigPath: configPath,
				BackupDir:  backupDir,
			}

			// Wire ClickHouse stop/start for backup/restore
			if managedCH != nil {
				httpSrv.StopClickHouse = func() {
					managedCH.Stop()
				}
				httpSrv.StartClickHouse = func() error {
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()
					return managedCH.Restart(ctx)
				}
			}

			// Background update check (5s after startup)
			go func() {
				time.Sleep(5 * time.Second)
				applog.Info("cli", "Checking for updates...")
				httpSrv.UpdateStatus.Check()
				snap := httpSrv.UpdateStatus.Snapshot()
				if snap.Available {
					applog.Infof("cli", "Update available: %s -> %s", snap.CurrentVersion, snap.LatestVersion)
				} else if snap.Error != "" {
					applog.Warnf("cli", "Update check error: %s", snap.Error)
				} else {
					applog.Info("cli", "Application is up to date.")
				}
			}()

			// Start the HTTP server
			go func() {
				if err := httpSrv.Start(); err != nil && err != http.ErrServerClosed {
					applog.Errorf("cli", "HTTP server error: %v", err)
				}
			}()

			// Wait for server to be ready
			waitForServer(serverURL, 10*time.Second)

			// Store references for shutdown
			mu.Lock()
			srv = httpSrv
			store = s
			keyStore = ks
			chCleanup = cleanup
			mu.Unlock()

			return nil
		}()

		if setupErr != nil {
			applog.Errorf("cli", "Setup failed: %v", setupErr)
			w.Dispatch(func() {
				w.Navigate("data:text/html," + errorHTML(setupErr.Error()))
			})
			return
		}

		// Setup succeeded — navigate to the real app
		w.Dispatch(func() {
			w.Navigate(serverURL)
		})
	}()

	// Run the webview event loop (blocks until window is closed)
	w.Run()

	// Clean shutdown after window is closed
	mu.Lock()
	localSrv := srv
	localStore := store
	localKeyStore := keyStore
	localCleanup := chCleanup
	mu.Unlock()

	if localSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		localSrv.Stop(shutdownCtx)
	}
	if localKeyStore != nil {
		localKeyStore.Close()
	}
	if localStore != nil {
		localStore.Close()
	}
	if localCleanup != nil {
		localCleanup()
	}

	return nil
}

// splashHTML returns an HTML loading page that polls /api/health.
func splashHTML(port int) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0a0a0a;color:#e0e0e0;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Helvetica,Arial,sans-serif;
display:flex;flex-direction:column;align-items:center;justify-content:center;height:100vh;overflow:hidden}
.spinner{width:48px;height:48px;border:3px solid #333;border-top-color:#f97316;border-radius:50%%;
animation:spin 0.8s linear infinite;margin-bottom:24px}
@keyframes spin{to{transform:rotate(360deg)}}
h1{font-size:18px;font-weight:500;color:#ccc;margin-bottom:8px}
p{font-size:13px;color:#888}
</style>
</head>
<body>
<div class="spinner"></div>
<h1>Starting CrawlObserver</h1>
<p>Downloading assets...</p>
<script>
(function(){
var url="http://127.0.0.1:%d";
var t=setInterval(function(){
fetch(url+"/api/health").then(function(r){
if(r.ok){clearInterval(t);window.location=url}
}).catch(function(){})
},500);
setTimeout(function(){
clearInterval(t);
document.querySelector("p").textContent="Setup is taking longer than expected. Please wait...";
},300000);
})();
</script>
</body>
</html>`, port)
}

// errorHTML returns an HTML error page for setup failures.
func errorHTML(msg string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0a0a0a;color:#e0e0e0;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Helvetica,Arial,sans-serif;
display:flex;flex-direction:column;align-items:center;justify-content:center;height:100vh;overflow:hidden;padding:40px}
.icon{font-size:48px;margin-bottom:24px}
h1{font-size:18px;font-weight:500;color:#ef4444;margin-bottom:12px}
pre{font-size:13px;color:#aaa;background:#1a1a1a;padding:16px;border-radius:8px;max-width:600px;
overflow-x:auto;white-space:pre-wrap;word-break:break-word}
</style>
</head>
<body>
<div class="icon">⚠</div>
<h1>Setup Failed</h1>
<pre>%s</pre>
</body>
</html>`, msg)
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
	applog.Warn("cli", "Server may not be ready")
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

// appDataDir returns ~/Library/Application Support/CrawlObserver (macOS) or equivalent,
// creating it if needed.
func appDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "Application Support", "CrawlObserver")
	return dir, os.MkdirAll(dir, 0755)
}
