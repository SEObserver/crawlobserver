//go:build !desktop

package cli

import "github.com/spf13/cobra"

func init() {
	// Make "serve" the default command when no subcommand is given (CLI builds).
	// Desktop builds use gui.go instead, which defaults to "gui".
	defaultCmd := serveCmd
	originalRun := rootCmd.RunE
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if originalRun != nil {
			return originalRun(cmd, args)
		}
		return defaultCmd.RunE(cmd, args)
	}
}
