// Package runtime manages the Patchcord agent's lifecycle: opening its
// database, launching its installed plugins, binding its local HTTP API,
// and shutting everything down cleanly on cancellation.
package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/lucasglmt/patchcord/internal/api"
	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/migrations"
)

const defaultShutdownTimeout = 10 * time.Second

// Config holds the settings needed to run the agent.
type Config struct {
	// ListenAddr is the local address the HTTP API binds to, e.g. "127.0.0.1:7331".
	ListenAddr string
	// DataDir holds the agent's SQLite database, created if it doesn't exist.
	DataDir string
	// ShutdownTimeout bounds how long in-flight requests are given to
	// complete once shutdown starts. Defaults to 10s when zero.
	ShutdownTimeout time.Duration
}

// Agent supervises the agent's database, HTTP API and installed plugins for
// the duration of one run.
type Agent struct {
	cfg      Config
	logger   *slog.Logger
	server   *http.Server
	listener net.Listener
	db       *sql.DB
	plugins  []runningPlugin
}

// runningPlugin pairs a launched plugin process with the catalog id it was
// launched for, so shutdown can log which plugin failed to stop cleanly.
type runningPlugin struct {
	id   string
	proc *plugins.Process
}

// NewAgent opens and migrates the agent's database, then binds its listen
// address and prepares its HTTP server. It returns an error if either step
// fails.
func NewAgent(cfg Config, logger *slog.Logger) (*Agent, error) {
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}

	db, err := persistence.Open(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := persistence.Migrate(context.Background(), db, migrations.FS, logger); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("bind listen address %q: %w", cfg.ListenAddr, err)
	}

	running := launchInstalledPlugins(context.Background(), db, logger)

	return &Agent{
		cfg:      cfg,
		logger:   logger,
		server:   &http.Server{Handler: api.NewRouter(api.Deps{DB: db})},
		listener: listener,
		db:       db,
		plugins:  running,
	}, nil
}

// launchInstalledPlugins launches every plugin recorded in the catalog and
// completes its handshake. A plugin that fails to launch or handshake is
// logged and skipped: per the non-negotiable that a plugin failure must
// never take the agent down, this never fails agent startup.
func launchInstalledPlugins(ctx context.Context, db *sql.DB, logger *slog.Logger) []runningPlugin {
	entries, err := plugins.List(ctx, db)
	if err != nil {
		logger.Error("list installed plugins", slog.String("error", err.Error()))
		return nil
	}

	running := make([]runningPlugin, 0, len(entries))
	for _, entry := range entries {
		proc, err := plugins.Launch(ctx, entry.ExecutablePath, plugins.DefaultReadyTimeout)
		if err != nil {
			logger.Error("launch plugin", slog.String("plugin_id", entry.PluginID), slog.String("error", err.Error()))
			continue
		}

		if _, err := plugins.Handshake(ctx, proc.Client); err != nil {
			logger.Error("handshake plugin", slog.String("plugin_id", entry.PluginID), slog.String("error", err.Error()))
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = proc.Close(closeCtx)
			cancel()
			continue
		}

		logger.Info("plugin launched", slog.String("plugin_id", entry.PluginID))
		running = append(running, runningPlugin{id: entry.PluginID, proc: proc})
	}

	return running
}

// Addr returns the actual address the agent is listening on. It is only
// meaningful after NewAgent has succeeded.
func (a *Agent) Addr() string {
	return a.listener.Addr().String()
}

// Run serves the agent's HTTP API until ctx is cancelled, then shuts it down
// gracefully within the configured shutdown timeout before terminating its
// installed plugins and closing the database. It blocks until the agent has
// fully stopped.
func (a *Agent) Run(ctx context.Context) error {
	defer func() {
		for _, rp := range a.plugins {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := rp.proc.Close(closeCtx); err != nil {
				a.logger.Error("close plugin", slog.String("plugin_id", rp.id), slog.String("error", err.Error()))
			}
			cancel()
		}

		if err := a.db.Close(); err != nil {
			a.logger.Error("close database", slog.String("error", err.Error()))
		}
	}()

	serveErr := make(chan error, 1)

	go func() {
		a.logger.Info("agent starting", slog.String("addr", a.Addr()))
		err := a.server.Serve(a.listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()

	if err := a.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	if err := <-serveErr; err != nil {
		return fmt.Errorf("http server: %w", err)
	}

	a.logger.Info("agent stopped")
	return nil
}
