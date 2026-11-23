// Command zvintedwatch is the ZVintedWatch entry point.
//
// It runs the monitoring engine, the Telegram notifier and the web interface
// in a single process.
//
//	zvintedwatch [run] [flags]          start the application (default)
//	zvintedwatch reset-password [flags] replace the web interface password
//	zvintedwatch debug search <input>   run one search and print the listings
//	zvintedwatch debug item <url>       fetch and print a listing description
//	zvintedwatch debug enrich <url>     print the extra details of a listing (store...)
//	zvintedwatch version                print version information
//
// Run "zvintedwatch run -h" to list the flags.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/app"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/config"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/version"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/web"
)

func main() {
	args := os.Args[1:]
	cmd := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}

	var err error
	switch cmd {
	case "run":
		err = runCmd(args)
	case "reset-password":
		err = resetPasswordCmd(args)
	case "debug":
		err = debugCmd(args)
	case "version", "--version":
		fmt.Println(version.String())
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`ZVintedWatch — get Telegram alerts for new second-hand listings.

Usage:
  zvintedwatch [run] [flags]            start the application (default command)
  zvintedwatch reset-password [flags]   set a new random password for the web interface
  zvintedwatch debug search <url|words> run one search and print the listings
  zvintedwatch debug item <listing url> print the description of a listing
  zvintedwatch debug enrich <url>       print the extra details of a listing (e.g. store)
  zvintedwatch version                  print version information

Run "zvintedwatch run -h" for the list of flags.
`)
}

func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	finish := config.Register(fs)
	_ = fs.Parse(args)
	cfg, err := finish()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer a.Close()

	printBanner(cfg, a.GeneratedPassword)
	return a.Run(ctx)
}

func printBanner(cfg config.Config, generatedPassword string) {
	line := strings.Repeat("─", 64)
	fmt.Println(line)
	fmt.Printf(" %s %s\n", version.AppName, version.Version)
	fmt.Printf(" Web interface: %s\n", cfg.BrowserURL())
	if !cfg.IsLoopback() {
		fmt.Printf(" Listening on %s (reachable from other devices)\n", cfg.Listen)
	}
	fmt.Printf(" Data folder:   %s\n", cfg.DataDir)
	if generatedPassword != "" {
		fmt.Println()
		fmt.Println(" A password protects the web interface because it is reachable")
		fmt.Println(" from the network. Log in with this password and change it in")
		fmt.Println(" Settings → Security:")
		fmt.Printf("\n     %s\n", generatedPassword)
	}
	fmt.Println(" Press Ctrl+C to stop.")
	fmt.Println(line)
}

func resetPasswordCmd(args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ExitOnError)
	finish := config.Register(fs)
	_ = fs.Parse(args)
	cfg, err := finish()
	if err != nil {
		return err
	}
	if _, err := os.Stat(cfg.DatabasePath()); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no database found in %s (use -data-dir)", cfg.DataDir)
	}
	st, err := store.Open(cfg.DatabasePath())
	if err != nil {
		return err
	}
	defer st.Close()
	pw, err := web.ResetPassword(context.Background(), st)
	if err != nil {
		return err
	}
	fmt.Printf("New password: %s\nRestart ZVintedWatch if it is running; existing sessions are logged out.\n", pw)
	return nil
}
