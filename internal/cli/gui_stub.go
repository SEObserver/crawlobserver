//go:build !desktop

package cli

// gui command is not available in non-desktop builds.
// Build with: go build -tags desktop ./cmd/crawlobserver
// Or use: wails build
