package cli

import (
	"fmt"
	"log"

	"github.com/SEObserver/seocrawler/internal/updater"
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
		fmt.Printf("seocrawler %s\n", updater.Version)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)

	updateCmd.Flags().Bool("check", false, "Only check, don't install")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	checkOnly, _ := cmd.Flags().GetBool("check")

	log.Printf("Current version: %s", updater.Version)
	log.Println("Checking for updates...")

	release, hasUpdate, err := updater.CheckUpdate()
	if err != nil {
		return fmt.Errorf("checking updates: %w", err)
	}

	if !hasUpdate {
		log.Println("Already up to date.")
		return nil
	}

	log.Printf("New version available: %s", release.TagName)

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

	log.Printf("Updated to %s. Please restart seocrawler.", release.TagName)
	return nil
}
