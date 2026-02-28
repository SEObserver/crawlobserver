package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/report"
	"github.com/SEObserver/crawlobserver/internal/storage"
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate reports from crawl data",
}

var externalLinksCmd = &cobra.Command{
	Use:   "external-links",
	Short: "Report on external links found during crawling",
	RunE:  runExternalLinks,
}

func init() {
	rootCmd.AddCommand(reportCmd)
	reportCmd.AddCommand(externalLinksCmd)

	externalLinksCmd.Flags().String("format", "table", "Output format: table, csv")
	externalLinksCmd.Flags().String("session", "", "Filter by session ID")
}

func runExternalLinks(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	store, err := storage.NewStore(
		cfg.ClickHouse.Host,
		cfg.ClickHouse.Port,
		cfg.ClickHouse.Database,
		cfg.ClickHouse.Username,
		cfg.ClickHouse.Password,
	)
	if err != nil {
		return fmt.Errorf("connecting to ClickHouse: %w", err)
	}
	defer store.Close()

	sessionID, _ := cmd.Flags().GetString("session")
	format, _ := cmd.Flags().GetString("format")

	links, err := store.ExternalLinks(context.Background(), sessionID)
	if err != nil {
		return fmt.Errorf("fetching external links: %w", err)
	}

	if len(links) == 0 {
		fmt.Println("No external links found.")
		return nil
	}

	return report.WriteExternalLinks(os.Stdout, links, format)
}
