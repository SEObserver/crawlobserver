//go:build !desktop

package cli

import (
	"testing"
)

func TestRootDefaultsToServe(t *testing.T) {
	// In CLI builds (non-desktop), rootCmd.RunE should be set by serve_default.go init().
	// It should delegate to serveCmd.RunE when no subcommand is given.
	if rootCmd.RunE == nil {
		t.Fatal("rootCmd.RunE should not be nil — serve_default.go init() should have set it")
	}
}

func TestServeCommandRegistered(t *testing.T) {
	// Verify serveCmd is registered as a subcommand of rootCmd.
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "serve" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("serve command should be registered on rootCmd")
	}
}

func TestServeCmdHasRunE(t *testing.T) {
	if serveCmd.RunE == nil {
		t.Fatal("serveCmd.RunE should not be nil")
	}
}
