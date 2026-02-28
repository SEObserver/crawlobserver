package cli

import (
	"fmt"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/updater"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for updates and self-update",
	RunE:  runUpdate,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("crawlobserver %s\n", updater.Version)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)

	updateCmd.Flags().Bool("check", false, "Only check, don't install")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	checkOnly, _ := cmd.Flags().GetBool("check")

	applog.Infof("cli", "Current version: %s", updater.Version)
	applog.Info("cli", "Checking for updates...")

	release, hasUpdate, err := updater.CheckUpdate()
	if err != nil {
		return fmt.Errorf("checking updates: %w", err)
	}

	if !hasUpdate {
		applog.Info("cli", "Already up to date.")
		return nil
	}

	applog.Infof("cli", "New version available: %s", release.TagName)

	if checkOnly {
		fmt.Printf("Update available: %s\nDownload: %s\n", release.TagName, release.HTMLURL)
		return nil
	}

	tmpPath, err := updater.DownloadUpdate(release)
	if err != nil {
		return fmt.Errorf("downloading update: %w", err)
	}

	if err := updater.SelfUpdate(tmpPath); err != nil {
		return fmt.Errorf("installing update: %w", err)
	}

	applog.Infof("cli", "Updated to %s. Please restart crawlobserver.", release.TagName)
	return nil
}
