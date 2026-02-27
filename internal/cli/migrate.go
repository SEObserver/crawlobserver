package cli

import (
	"fmt"

	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Create or update ClickHouse tables",
	RunE:  runMigrate,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	store, cleanup, err := setupClickHouse(cfg, "default")
	if err != nil {
		return err
	}
	defer store.Close()
	defer cleanup()

	return nil
}
