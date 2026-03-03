//go:build desktop && !darwin

package cli

import "unsafe"

func setupNativeMenu()                          {}
func installClipboardMonitor(_ unsafe.Pointer)  {}
