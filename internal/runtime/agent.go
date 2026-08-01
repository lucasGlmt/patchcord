// Package runtime manages the Patchcord agent's lifecycle: opening its
// database, binding its local HTTP API, and shutting both down cleanly on
// cancellation.
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

// Agent supervises the agent's database and HTTP API for the duration of
// one run.
type Agent struct {
	cfg      Config
	logger   *slog.Logger
	server   *http.Server
	listener net.Listener
	db       *sql.DB
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

	return &Agent{
		cfg:      cfg,
		logger:   logger,
		server:   &http.Server{Handler: api.NewRouter(api.Deps{DB: db})},
		listener: listener,
		db:       db,
	}, nil
}

// Addr returns the actual address the agent is listening on. It is only
// meaningful after NewAgent has succeeded.
func (a *Agent) Addr() string {
	return a.listener.Addr().String()
}

// Run serves the agent's HTTP API until ctx is cancelled, then shuts it down
// gracefully within the configured shutdown timeout before closing the
// database. It blocks until the agent has fully stopped.
func (a *Agent) Run(ctx context.Context) error {
	defer func() {
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
