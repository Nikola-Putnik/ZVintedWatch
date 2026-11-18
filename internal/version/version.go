// Package version exposes build metadata injected at link time.
//
// Release builds set these variables with -ldflags, for example:
//
//	go build -ldflags "-X github.com/Nikola-Putnik/ZVintedWatch/internal/version.Version=v1.2.3"
package version

import "fmt"

var (
	// Version is the semantic version of the build ("dev" for local builds).
	Version = "dev"
	// Commit is the short git commit hash the binary was built from.
	Commit = "none"
	// Date is the build date in RFC 3339 format.
	Date = "unknown"
)

// AppName is the human-readable name of the application.
const AppName = "ZVintedWatch"

// String returns a one-line description of the build.
func String() string {
	return fmt.Sprintf("%s %s (commit %s, built %s)", AppName, Version, Commit, Date)
}
