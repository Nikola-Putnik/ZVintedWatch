//go:build windows

package main

import "golang.org/x/sys/windows"

// The console output is UTF-8; make legacy Windows consoles render it
// correctly (accents in listing titles, box-drawing characters).
func init() {
	_ = windows.SetConsoleOutputCP(65001)
}
