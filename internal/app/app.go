// Package app wires every component of ZVintedWatch together and runs them.
//
// To add a marketplace, implement provider.Provider in its own package and
// add it to NewRegistry.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/Nikola-Putnik/ZVintedWatch/internal/activity"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/config"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/engine"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/httpx"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/notify/telegram"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider/cashconverters"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/provider/vinted"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/store"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/version"
	"github.com/Nikola-Putnik/ZVintedWatch/internal/web"
)

// NewRegistry returns every available marketplace provider. To add a site,
// implement provider.Provider in its own package and add it here: the engine,
// the web interface and the debug command pick it up automatically.
func NewRegistry(f *httpx.Fetcher) *provider.Registry {
	return provider.NewRegistry(
		vinted.New(f),
		cashconverters.New(f),
	)
}

// App is a fully wired ZVintedWatch instance.
type App struct {
	cfg      config.Config
	logger   *slog.Logger
	logFile  io.Closer
	store    *store.Store
	bus      *activity.Bus
	fetcher  *httpx.Fetcher
	registry *provider.Registry
	engine   *engine.Engine
	telegram *telegram.Service
	server   *web.Server

	// GeneratedPassword is set when a password was created at startup
	// because the interface listens on the network.
	GeneratedPassword string
}

// New opens the data directory and builds every component.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	logger, logFile, err := newLogger(cfg)
	if err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, logger: logger, logFile: logFile}

	a.store, err = store.Open(cfg.DatabasePath())
	if err != nil {
		a.Close()
		return nil, err
	}
	a.bus = activity.NewBus(1500, logger)
	a.fetcher = httpx.New(a.bus, nil)

	a.registry = NewRegistry(a.fetcher)

	for _, p := range a.registry.All() {
		id := p.Info().ID
		settings := httpx.DefaultSettings()
		if _, err := a.store.GetSetting(ctx, web.NetworkSettingsKey(id), &settings); err != nil {
			a.Close()
			return nil, err
		}
		if err := a.fetcher.Configure(id, settings); err != nil {
			logger.Warn("invalid network settings, using defaults", "provider", id, "err", err)
			_ = a.fetcher.Configure(id, httpx.DefaultSettings())
		}
	}

	a.telegram = telegram.NewService(a.store, a.bus, "")
	if err := a.telegram.Load(ctx); err != nil {
		a.Close()
		return nil, err
	}
	a.engine = engine.New(a.store, a.registry, a.bus, a.telegram, engine.Options{})

	auth, generated, err := web.NewAuth(ctx, a.store, cfg.IsLoopback())
	if err != nil {
		a.Close()
		return nil, err
	}
	a.GeneratedPassword = generated

	a.server = web.New(web.Deps{
		Store: a.store, Bus: a.bus, Fetcher: a.fetcher, Registry: a.registry, Engine: a.engine,
		Telegram: a.telegram, Auth: auth, Logger: logger, Loopback: cfg.IsLoopback(),
	})
	return a, nil
}

// Run starts the engine and the web server and blocks until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	if err := a.engine.Reload(ctx); err != nil {
		return err
	}

	ln, err := net.Listen("tcp", a.cfg.Listen)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w (is another instance running? use -listen to pick another port)", a.cfg.Listen, err)
	}
	srv := &http.Server{
		Handler:           a.server,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No write timeout: the live console keeps a streaming connection open.
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	engineDone := make(chan struct{})
	go func() {
		a.engine.Run(runCtx)
		close(engineDone)
	}()
	go a.forwardHealthNotices(runCtx)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	a.bus.Publish(activity.Event{Kind: activity.KindSystem, Message: fmt.Sprintf(
		"%s %s started — web interface on %s, data in %s", version.AppName, version.Version,
		a.cfg.BrowserURL(), a.cfg.DataDir)})
	if a.cfg.OpenBrowser {
		openBrowser(a.cfg.BrowserURL())
	}

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	a.logger.Info("shutting down")
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	_ = srv.Shutdown(shutdownCtx)
	cancel()
	<-engineDone
	return nil
}

// forwardHealthNotices sends cooldown notices to the Telegram system
// destination, when one is configured.
func (a *App) forwardHealthNotices(ctx context.Context) {
	events, cancel := a.bus.Subscribe(64)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			if e.Kind == activity.KindHealth {
				name := e.Provider
				if p, found := a.registry.Get(e.Provider); found {
					name = p.Info().Name
				}
				a.telegram.SystemNotice(name + ": " + e.Message)
			}
		}
	}
}

// Close releases every resource.
func (a *App) Close() {
	if a.telegram != nil {
		a.telegram.Close()
	}
	if a.fetcher != nil {
		a.fetcher.Close()
	}
	if a.store != nil {
		a.store.Close()
	}
	if a.logFile != nil {
		a.logFile.Close()
	}
}

// Logger returns the application logger.
func (a *App) Logger() *slog.Logger { return a.logger }

// Store returns the database (used by command-line tools).
func (a *App) Store() *store.Store { return a.store }

// newLogger logs to the console and to a log file in the data directory.
// The file is rotated once it exceeds 10 MB (one backup is kept).
func newLogger(cfg config.Config) (*slog.Logger, io.Closer, error) {
	path := cfg.LogPath()
	if info, err := os.Stat(path); err == nil && info.Size() > 10<<20 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	h := slog.NewTextHandler(io.MultiWriter(os.Stdout, f), &slog.HandlerOptions{Level: level})
	return slog.New(h), f, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
