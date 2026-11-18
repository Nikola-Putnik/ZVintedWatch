// Package config resolves the bootstrap configuration of ZVintedWatch:
// where to listen and where to keep data. Everything else (alerts, Telegram,
// network settings, password) is edited in the web interface and stored in
// the database.
//
// Precedence: command-line flags, then environment variables, then defaults.
package config

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config is the bootstrap configuration.
type Config struct {
	// Listen is the address of the web interface (host:port).
	Listen string
	// DataDir holds the database and the log file.
	DataDir string
	// OpenBrowser opens the web interface in the default browser at startup.
	OpenBrowser bool
	// Debug enables debug-level logging.
	Debug bool
}

// Environment variables recognised by Parse.
const (
	EnvListen  = "ZVW_LISTEN"
	EnvDataDir = "ZVW_DATA_DIR"
	EnvDebug   = "ZVW_DEBUG"
)

// DefaultListen only accepts connections from the local machine.
const DefaultListen = "127.0.0.1:8080"

// Register declares the common flags on fs and returns a function that
// finalizes the Config once fs.Parse has run.
func Register(fs *flag.FlagSet) func() (Config, error) {
	listen := fs.String("listen", envOr(EnvListen, DefaultListen),
		"address of the web interface; use 0.0.0.0:8080 to allow other devices (env "+EnvListen+")")
	dataDir := fs.String("data-dir", os.Getenv(EnvDataDir),
		"directory for the database and logs (default: a 'data' folder next to the executable; env "+EnvDataDir+")")
	open := fs.Bool("open", runtime.GOOS == "windows", "open the web interface in the browser at startup")
	debug := fs.Bool("debug", os.Getenv(EnvDebug) != "", "enable debug logging (env "+EnvDebug+")")

	return func() (Config, error) {
		c := Config{Listen: strings.TrimSpace(*listen), DataDir: *dataDir, OpenBrowser: *open, Debug: *debug}
		if _, _, err := net.SplitHostPort(c.Listen); err != nil {
			return c, fmt.Errorf("invalid -listen address %q: %w", c.Listen, err)
		}
		if c.DataDir == "" {
			c.DataDir = DefaultDataDir()
		}
		abs, err := filepath.Abs(c.DataDir)
		if err != nil {
			return c, err
		}
		c.DataDir = abs
		return c, nil
	}
}

// DefaultDataDir returns a "data" folder next to the executable, which keeps
// the application portable (copy the folder to move it). When running through
// "go run" the executable lives in a temporary build directory, so the
// current directory is used instead.
func DefaultDataDir() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		if !strings.Contains(dir, "go-build") && !strings.HasPrefix(dir, os.TempDir()) {
			return filepath.Join(dir, "data")
		}
	}
	return "data"
}

// DatabasePath is the SQLite database location.
func (c Config) DatabasePath() string { return filepath.Join(c.DataDir, "zvintedwatch.db") }

// LogPath is the log file location.
func (c Config) LogPath() string { return filepath.Join(c.DataDir, "zvintedwatch.log") }

// IsLoopback reports whether the web interface only accepts local connections.
func (c Config) IsLoopback() bool {
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// BrowserURL is the address to open in a browser.
func (c Config) BrowserURL() string {
	host, port, _ := net.SplitHostPort(c.Listen)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
