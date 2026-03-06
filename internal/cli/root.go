package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SEObserver/crawlobserver/internal/config"
	"github.com/SEObserver/crawlobserver/internal/telemetry"
	"github.com/SEObserver/crawlobserver/internal/updater"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "crawlobserver",
	Short: "SEO crawler — extract SEO signals at scale",
	Long:  `CrawlObserver is an open-source SEO crawler that extracts SEO signals (title, canonical, headers, links) and stores them in ClickHouse.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.Load()

		// Skip interactive prompt for gui (has its own OnboardingWizard)
		if cmd.Name() == "gui" {
			return nil
		}

		// If --telemetry flag was explicitly passed, apply it directly
		if cmd.Flags().Changed("telemetry") {
			val, _ := cmd.Flags().GetBool("telemetry")
			viper.Set("telemetry.enabled", val)
			if cfg.Telemetry.AskedAt == "" {
				viper.Set("telemetry.asked_at", time.Now().UTC().Format(time.RFC3339))
			}
			_ = config.WriteConfig()
			cfg.Telemetry.Enabled = val
		} else if cfg.Telemetry.AskedAt == "" && isInteractive() {
			// First launch: ask for telemetry consent
			askTelemetryConsent()
			// Reload config after consent
			cfg, _ = config.Load()
		}

		// Init telemetry for all commands
		telemetry.Init(cfg.Telemetry.Enabled, cfg.Telemetry.InstanceID, updater.Version)
		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.PersistentFlags().Bool("telemetry", false, "Enable or disable anonymous usage analytics")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
	}

	viper.SetEnvPrefix("CRAWLOBSERVER")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Error reading config: %s\n", err)
		}
	}
}

// isInteractive returns true if stdin is a terminal.
func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// askTelemetryConsent prompts the user to opt-in to anonymous analytics.
func askTelemetryConsent() {
	fmt.Println("─────────────────────────────────────────────────")
	fmt.Println("  CrawlObserver — Anonymous Usage Analytics")
	fmt.Println()
	fmt.Println("  Help improve CrawlObserver by sharing anonymous")
	fmt.Println("  usage data (crawl count, feature usage).")
	fmt.Println()
	fmt.Println("  We NEVER collect: URLs, page content, IPs,")
	fmt.Println("  or any personal data.")
	fmt.Println()
	fmt.Print("  Enable anonymous analytics? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	enabled := input == "y" || input == "yes"

	viper.Set("telemetry.enabled", enabled)
	viper.Set("telemetry.asked_at", time.Now().UTC().Format(time.RFC3339))
	_ = config.WriteConfig()

	if enabled {
		fmt.Println("  Analytics enabled. Thank you!")
	} else {
		fmt.Println("  Analytics disabled.")
	}
	fmt.Println("─────────────────────────────────────────────────")
}
