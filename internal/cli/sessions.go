package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/storage"
	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List crawl sessions",
	RunE:  runSessions,
}

func init() {
	rootCmd.AddCommand(sessionsCmd)
}

func runSessions(cmd *cobra.Command, args []string) error {
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

	sessions, err := store.ListSessions(context.Background())
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No crawl sessions found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tPAGES\tSTARTED\tSEEDS")
	fmt.Fprintln(w, "--\t------\t-----\t-------\t-----")

	for _, s := range sessions {
		seedsStr := ""
		if len(s.SeedURLs) > 0 {
			seedsStr = s.SeedURLs[0]
			if len(s.SeedURLs) > 1 {
				seedsStr += fmt.Sprintf(" (+%d)", len(s.SeedURLs)-1)
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			s.ID,
			s.Status,
			s.PagesCrawled,
			s.StartedAt.Format("2006-01-02 15:04:05"),
			seedsStr,
		)
	}
	w.Flush()
	return nil
}
