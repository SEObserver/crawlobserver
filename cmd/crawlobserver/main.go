package main

import (
	"os"

	"github.com/SEObserver/crawlobserver/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
