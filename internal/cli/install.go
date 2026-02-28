package cli

import (
	"fmt"

	"github.com/SEObserver/crawlobserver/internal/applog"
	chmanaged "github.com/SEObserver/crawlobserver/internal/clickhouse"
	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/spf13/cobra"
)

var installClickhouseCmd = &cobra.Command{
	Use:   "install-clickhouse",
	Short: "Download and install the ClickHouse binary",
	Long:  `Downloads the ClickHouse binary for the current platform. Useful for pre-provisioning offline environments.`,
	RunE:  runInstallClickhouse,
}

func init() {
	rootCmd.AddCommand(installClickhouseCmd)
}

func runInstallClickhouse(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	dataDir := cfg.ClickHouse.DataDir
	if dataDir == "" {
		dataDir = chmanaged.DefaultDataDir()
	}

	// Check if already installed
	existing := chmanaged.FindBinary(cfg.ClickHouse.BinaryPath, dataDir)
	if existing != "" {
		applog.Infof("cli", "ClickHouse binary already found at: %s", existing)
		applog.Info("cli", "Re-downloading to update...")
	}

	binPath, err := chmanaged.DownloadBinary(dataDir)
	if err != nil {
		return fmt.Errorf("installing ClickHouse: %w", err)
	}

	applog.Infof("cli", "ClickHouse installed at: %s", binPath)
	return nil
}
