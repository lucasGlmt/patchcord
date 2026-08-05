// Package runtime manages the Patchcord agent's lifecycle: opening its
// database, launching and supervising its installed plugins, binding its
// local HTTP API, and shutting everything down cleanly on cancellation.
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
	"github.com/lucasglmt/patchcord/internal/auth"
	"github.com/lucasglmt/patchcord/internal/persistence"
	"github.com/lucasglmt/patchcord/internal/plugins"
	"github.com/lucasglmt/patchcord/internal/scheduler"
	"github.com/lucasglmt/patchcord/internal/secrets"
	"github.com/lucasglmt/patchcord/migrations"
)

const defaultShutdownTimeout = 10 * time.Second

// Config holds the settings needed to run the agent.
type Config struct {
	// ListenAddr is the local address the HTTP API binds to, e.g. "127.0.0.1:7331".
	ListenAddr string
	// DataDir holds the agent's SQLite database, created if it doesn't exist.
	DataDir string
	// SecretsMasterKeyFile points to the file holding the base64 AES-256
	// master key for the "file" secret store. Left empty, "file" secret
	// references simply don't resolve on this agent — see
	// secrets.BuildStore and ADR-0040.
	SecretsMasterKeyFile string
	// AppsDirectoryListingEnabled turns on GET /apps/, an index page listing
	// every installed application. Left false (the default), that route
	// keeps returning a plain 404 — see ADR-0061.
	AppsDirectoryListingEnabled bool
	// ShutdownTimeout bounds how long in-flight requests are given to
	// complete once shutdown starts. Defaults to 10s when zero.
	ShutdownTimeout time.Duration
}

// Agent supervises the agent's database, HTTP API and installed plugins for
// the duration of one run.
type Agent struct {
	cfg        Config
	logger     *slog.Logger
	server     *http.Server
	listener   net.Listener
	db         *sql.DB
	supervisor *plugins.Supervisor
	cancelRuns context.CancelFunc
}

// NewAgent opens and migrates the agent's database, launches and starts
// supervising its installed plugins, then binds its listen address and
// prepares its HTTP server. It returns an error if the database or the
// listen address cannot be set up; a plugin that fails to launch is logged
// and skipped, never fails agent startup (see plugins.Supervisor).
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

	secretStore, err := secrets.BuildStore(cfg.DataDir, cfg.SecretsMasterKeyFile)
	if err != nil {
		_ = listener.Close()
		_ = db.Close()
		return nil, fmt.Errorf("build secrets store: %w", err)
	}

	// Seeds Patchcord's bundled reference plugins into the catalog the very
	// first time the agent runs against this data directory (ADR-0059) —
	// a no-op on every call after that, so it must run before Start reads
	// the catalog.
	if err := plugins.SeedEmbedded(context.Background(), db, cfg.DataDir, logger); err != nil {
		_ = listener.Close()
		_ = db.Close()
		return nil, fmt.Errorf("seed embedded plugins: %w", err)
	}

	supervisor := plugins.NewSupervisor(plugins.SupervisorConfig{}, logger)
	if err := supervisor.Start(context.Background(), db); err != nil {
		_ = listener.Close()
		_ = db.Close()
		return nil, fmt.Errorf("start plugin supervisor: %w", err)
	}

	// runCtx is the base context every HTTP-triggered background run
	// (see internal/api's handleRunWorkflow) derives from — never a single
	// request's own context, which is gone the moment its response is
	// written. Cancelled during Run's shutdown sequence so an in-flight
	// background run is recorded Cancelled instead of left running against
	// plugins that are about to be torn down by supervisor.Stop.
	runCtx, cancelRuns := context.WithCancel(context.Background())

	// The scheduler fires "schedule"-triggered workflows (ADR-0035) the same
	// way handleRunWorkflow fires a manual one — under runCtx, so it shares
	// the same shutdown-cancellation behavior.
	go scheduler.NewRunner(db, supervisor, logger, secretStore).Run(runCtx)

	return &Agent{
		cfg:    cfg,
		logger: logger,
		server: &http.Server{
			Handler: api.NewRouter(api.Deps{
				DB:                          db,
				Executor:                    supervisor,
				RunCtx:                      runCtx,
				Logger:                      logger,
				Sessions:                    auth.NewStore(),
				ConnectorTester:             supervisor,
				Secrets:                     secretStore,
				AppsDirectoryListingEnabled: cfg.AppsDirectoryListingEnabled,
			}),
			// ReadHeaderTimeout bounds how long a client may take sending
			// its request headers — standard Go hardening against a
			// slow-header (Slowloris-style) client tying up a handler
			// goroutine indefinitely; it does not touch reading the body
			// or writing the response. IdleTimeout bounds how long a
			// keep-alive connection may sit idle between requests. Neither
			// is a ReadTimeout/WriteTimeout: GET /v1/runs/{id}/events
			// (see internal/api/events.go) deliberately holds its response
			// open, streaming Server-Sent Events for as long as a run
			// takes — a blanket WriteTimeout would sever that connection
			// mid-run.
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		listener:   listener,
		db:         db,
		supervisor: supervisor,
		cancelRuns: cancelRuns,
	}, nil
}

// Addr returns the actual address the agent is listening on. It is only
// meaningful after NewAgent has succeeded.
func (a *Agent) Addr() string {
	return a.listener.Addr().String()
}

// Run serves the agent's HTTP API until ctx is cancelled, then shuts it down
// gracefully within the configured shutdown timeout before stopping its
// plugin supervisor and closing the database. It blocks until the agent has
// fully stopped.
func (a *Agent) Run(ctx context.Context) error {
	defer func() {
		// Cancel any HTTP-triggered background run (internal/api's
		// handleRunWorkflow) before tearing down the plugins it depends on,
		// so it is recorded Cancelled rather than failing loudly against
		// processes that are mid-shutdown, or left as an orphaned goroutine.
		a.cancelRuns()

		stopCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
		a.supervisor.Stop(stopCtx)
		cancel()

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
