package cli

import (
	"context"
	"fmt"
	"log"

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

	store, cleanup, err := setupClickHouse(cfg, "default") // connect to default db first for CREATE DATABASE
	if err != nil {
		return err
	}
	defer store.Close()
	defer cleanup()

	log.Println("Running migrations...")
	if err := store.Migrate(context.Background()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	log.Println("Migrations complete.")
	return nil
}
