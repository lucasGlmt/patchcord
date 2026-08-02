package plugins

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/lucasglmt/patchcord/internal/connectors"
)

// SupervisorConfig controls the Plugin Supervisor's health check and
// restart policy (vision document, section 8.4).
type SupervisorConfig struct {
	// HealthCheckInterval is how often a running plugin's health is
	// checked. Defaults to 10s when zero.
	HealthCheckInterval time.Duration
	// HealthCheckTimeout bounds each individual health check call.
	// Defaults to 2s when zero.
	HealthCheckTimeout time.Duration
	// MaxRestarts is how many times a plugin is relaunched after a crash
	// or a failed health check before it is quarantined. Defaults to 3
	// when zero; a negative value disables restarts entirely.
	MaxRestarts int
	// RestartDelay is the fixed delay observed before each restart
	// attempt. Defaults to 1s when zero.
	RestartDelay time.Duration
}

const (
	defaultHealthCheckInterval = 10 * time.Second
	defaultHealthCheckTimeout  = 2 * time.Second
	defaultMaxRestarts         = 3
	defaultRestartDelay        = 1 * time.Second
)

func (c SupervisorConfig) withDefaults() SupervisorConfig {
	if c.HealthCheckInterval <= 0 {
		c.HealthCheckInterval = defaultHealthCheckInterval
	}
	if c.HealthCheckTimeout <= 0 {
		c.HealthCheckTimeout = defaultHealthCheckTimeout
	}
	if c.MaxRestarts == 0 {
		c.MaxRestarts = defaultMaxRestarts
	}
	if c.RestartDelay <= 0 {
		c.RestartDelay = defaultRestartDelay
	}
	return c
}

// Supervisor launches every plugin recorded in the catalog and keeps them
// running for as long as the agent does: it detects crashes and
// unresponsive plugins via periodic health checks, restarts them up to a
// bounded number of attempts, and quarantines (stops retrying) a plugin
// that keeps failing. Once started, it also serves as the agent's entry
// point for actually invoking an action (ExecuteAction), routing the call
// to whichever running plugin currently contributes it.
//
// A plugin failure — however it manifests — is always contained here and
// never propagated to the agent: the non-negotiable that a crashed plugin
// must never take the agent down holds because the Supervisor is the only
// thing watching plugin processes.
type Supervisor struct {
	cfg    SupervisorConfig
	logger *slog.Logger

	wg      sync.WaitGroup
	cancel  context.CancelFunc
	mu      sync.Mutex
	running map[string]*runningPlugin // keyed by plugin id; absent while quarantined
}

// runningPlugin pairs a launched process with the catalog entry it was
// launched from, so ExecuteAction can find which running plugin currently
// contributes a given action.
type runningPlugin struct {
	entry CatalogEntry
	proc  *Process
}

// NewSupervisor creates a Supervisor. Call Start to launch the catalog's
// plugins and begin supervising them.
func NewSupervisor(cfg SupervisorConfig, logger *slog.Logger) *Supervisor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Supervisor{
		cfg:     cfg.withDefaults(),
		logger:  logger,
		running: make(map[string]*runningPlugin),
	}
}

// Start launches every plugin in the catalog and begins supervising them.
// A plugin that fails to launch is logged and skipped, exactly like a
// plugin that exhausts its restart attempts: Start itself never fails
// because of a plugin.
func (s *Supervisor) Start(ctx context.Context, db *sql.DB) error {
	entries, err := List(ctx, db)
	if err != nil {
		return err
	}

	supervisorCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	for _, entry := range entries {
		proc, ok := s.launchAndHandshake(ctx, entry)
		if !ok {
			continue
		}

		s.mu.Lock()
		s.running[entry.PluginID] = &runningPlugin{entry: entry, proc: proc}
		s.mu.Unlock()

		s.wg.Add(1)
		go s.watch(supervisorCtx, entry)
	}

	return nil
}

// Stop stops supervising every plugin and terminates them, bounded by ctx.
// It waits for all supervision goroutines to finish before returning.
func (s *Supervisor) Stop(ctx context.Context) {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, rp := range s.running {
		if err := rp.proc.Close(ctx); err != nil {
			s.logger.Error("close plugin", slog.String("plugin_id", id), slog.String("error", err.Error()))
		}
	}
	s.running = make(map[string]*runningPlugin)
}

// ExecuteAction runs actionID on whichever currently running plugin
// contributes it, passing connector as the resolved connector bound to it
// (nil if none). It implements internal/runs's ActionExecutor interface,
// letting the workflow runner invoke actions without knowing anything
// about plugin processes, transport, or supervision.
func (s *Supervisor) ExecuteAction(ctx context.Context, actionID string, input map[string]any, connector *connectors.ResolvedConnector) (map[string]any, error) {
	s.mu.Lock()
	var rp *runningPlugin
	for _, candidate := range s.running {
		for _, action := range candidate.entry.Actions {
			if action == actionID {
				rp = candidate
				break
			}
		}
		if rp != nil {
			break
		}
	}
	s.mu.Unlock()

	if rp == nil {
		return nil, fmt.Errorf("action %q is not currently available (no running plugin contributes it)", actionID)
	}

	return ExecuteAction(ctx, rp.proc.Client, actionID, input, connector)
}

// TestConnector attempts to reach the external system connector describes,
// via whichever currently running plugin declares its type. ok/message is
// the connector test's own outcome — the returned error means the test
// could not even be attempted (no running plugin declares that connector
// type, or the plugin that does returns codes.Unimplemented because it
// doesn't support testing).
func (s *Supervisor) TestConnector(ctx context.Context, connector *connectors.ResolvedConnector) (ok bool, message string, err error) {
	if connector == nil {
		return false, "", fmt.Errorf("test connector requires a connector")
	}

	s.mu.Lock()
	var rp *runningPlugin
	for _, candidate := range s.running {
		for _, connectorType := range candidate.entry.Connectors {
			if connectorType == connector.Type {
				rp = candidate
				break
			}
		}
		if rp != nil {
			break
		}
	}
	s.mu.Unlock()

	if rp == nil {
		return false, "", fmt.Errorf("connector type %q is not currently available (no running plugin declares it)", connector.Type)
	}

	return TestConnector(ctx, rp.proc.Client, connector)
}

// launchAndHandshake launches entry's executable and validates it with a
// handshake. On any failure it logs the reason and returns ok=false; it
// never returns an error, since a bad plugin must never stop the agent or
// its other plugins from starting.
func (s *Supervisor) launchAndHandshake(ctx context.Context, entry CatalogEntry) (proc *Process, ok bool) {
	proc, err := Launch(ctx, entry.ExecutablePath, DefaultReadyTimeout)
	if err != nil {
		s.logger.Error("launch plugin", slog.String("plugin_id", entry.PluginID), slog.String("error", err.Error()))
		return nil, false
	}

	if _, err := Handshake(ctx, proc.Client); err != nil {
		s.logger.Error("handshake plugin", slog.String("plugin_id", entry.PluginID), slog.String("error", err.Error()))
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = proc.Close(closeCtx)
		cancel()
		return nil, false
	}

	s.logger.Info("plugin launched", slog.String("plugin_id", entry.PluginID))
	return proc, true
}

// watch supervises one plugin for the rest of the agent's run: it reacts
// to the process exiting unexpectedly and to periodic health checks,
// restarting the plugin (bounded by cfg.MaxRestarts) or quarantining it.
func (s *Supervisor) watch(ctx context.Context, entry CatalogEntry) {
	defer s.wg.Done()

	restarts := 0
	ticker := time.NewTicker(s.cfg.HealthCheckInterval)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		rp := s.running[entry.PluginID]
		s.mu.Unlock()
		if rp == nil {
			// Quarantined by a previous iteration.
			return
		}
		proc := rp.proc

		select {
		case <-ctx.Done():
			return

		case <-proc.Exited():
			s.logger.Error("plugin crashed",
				slog.String("plugin_id", entry.PluginID),
				slog.Any("error", proc.ExitErr()))
			if !s.restart(ctx, entry, &restarts) {
				return
			}

		case <-ticker.C:
			healthCtx, cancel := context.WithTimeout(ctx, s.cfg.HealthCheckTimeout)
			resp, err := proc.HealthClient.Check(healthCtx, &grpc_health_v1.HealthCheckRequest{})
			cancel()

			if err == nil && resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING {
				continue
			}

			s.logger.Warn("plugin health check failed",
				slog.String("plugin_id", entry.PluginID),
				slog.Any("error", err),
				slog.String("status", resp.GetStatus().String()))

			closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = proc.Close(closeCtx)
			closeCancel()

			if !s.restart(ctx, entry, &restarts) {
				return
			}
		}
	}
}

// restart waits cfg.RestartDelay, then relaunches entry's plugin. It
// returns false once cfg.MaxRestarts has been exhausted, after
// quarantining the plugin (removing it from the running set so it is not
// retried again this run) — the caller must stop watching in that case.
func (s *Supervisor) restart(ctx context.Context, entry CatalogEntry, restarts *int) bool {
	for {
		if *restarts >= s.cfg.MaxRestarts {
			s.logger.Error("plugin quarantined after repeated failures",
				slog.String("plugin_id", entry.PluginID),
				slog.Int("restarts", *restarts))
			s.mu.Lock()
			delete(s.running, entry.PluginID)
			s.mu.Unlock()
			return false
		}

		*restarts++
		s.logger.Info("restarting plugin",
			slog.String("plugin_id", entry.PluginID),
			slog.Int("attempt", *restarts),
			slog.Int("max_restarts", s.cfg.MaxRestarts))

		select {
		case <-time.After(s.cfg.RestartDelay):
		case <-ctx.Done():
			return false
		}

		proc, ok := s.launchAndHandshake(ctx, entry)
		if !ok {
			// launchAndHandshake already logged the reason; loop back and
			// try again, subject to the MaxRestarts check above.
			continue
		}

		s.mu.Lock()
		s.running[entry.PluginID] = &runningPlugin{entry: entry, proc: proc}
		s.mu.Unlock()
		return true
	}
}
