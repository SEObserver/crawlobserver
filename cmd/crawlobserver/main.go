package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"

	"github.com/SEObserver/crawlobserver/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		if runtime.GOOS == "windows" {
			fmt.Fprintln(os.Stderr, "\nPress Enter to exit...")
			bufio.NewReader(os.Stdin).ReadBytes('\n')
		}
		os.Exit(1)
	}
}
