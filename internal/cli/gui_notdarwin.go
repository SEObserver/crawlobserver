//go:build desktop && !darwin

package cli

import (
	"fmt"
	"unsafe"
)

func setupNativeMenu()                         {}
func installClipboardMonitor(_ unsafe.Pointer) {}
func nativeSaveFile(_, _ string) error         { return fmt.Errorf("not supported") }
