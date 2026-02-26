package cli

import (
	"context"
	"fmt"
	"log"

	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/storage"
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

	store, err := storage.NewStore(
		cfg.ClickHouse.Host,
		cfg.ClickHouse.Port,
		"default", // connect to default db first for CREATE DATABASE
		cfg.ClickHouse.Username,
		cfg.ClickHouse.Password,
	)
	if err != nil {
		return fmt.Errorf("connecting to ClickHouse: %w", err)
	}
	defer store.Close()

	log.Println("Running migrations...")
	if err := store.Migrate(context.Background()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	log.Println("Migrations complete.")
	return nil
}
