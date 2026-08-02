# Supervision & Lifecycle

The `Supervisor` (`internal/plugins/supervisor.go`) launches every plugin recorded in the catalog and keeps them running for as long as it's active: `patchcord serve` runs one for its entire lifetime, and `workflow run` / `connector test` each run one for the duration of that single command (see [CLI Overview](../cli/index.md#a-one-shot-command-does-not-talk-to-a-running-agent)). It is also the agent's entry point for actually invoking an action or testing a connector — the workflow runner and the CLI never talk to a plugin process directly, only to the Supervisor.

A plugin failure, however it manifests, is always contained here and never propagated to the agent — this is what makes non-negotiable #7 ("a plugin crash never takes down the agent") true by construction rather than by luck.

## Health checks

The SDK registers the standard gRPC health protocol (`grpc.health.v1.Health`) automatically, reporting `SERVING` as soon as the plugin's server starts. The Supervisor polls `Check()` on this same connection every `HealthCheckInterval` (default 10s), bounded by `HealthCheckTimeout` (default 2s) ([ADR-0016](../../../adr/0016-plugin-supervisor.md)). No custom health RPC was added to the protocol — reusing the gRPC standard keeps `plugin.proto` minimal.

## Crash detection

Independent of health checks: `Process.Exited()` exposes a channel closed the moment the plugin's process exits, whether cleanly or via a crash — detected through the process's own `cmd.Wait()`, not by inference from a failed health check.

## Restart and quarantine

A crash and a failed health check converge on the same policy:

1. Wait `RestartDelay` (default 1s) — a fixed delay, not exponential backoff.
2. Relaunch and re-handshake the plugin.
3. If this fails, retry, up to `MaxRestarts` (default 3) attempts.
4. Beyond that, the plugin is **quarantined**: removed from the running set for the rest of this session. It is not uninstalled from the catalog — a fresh `patchcord serve` gives every installed plugin a clean slate of restart attempts again. Quarantine state lives only in memory.

```go
type SupervisorConfig struct {
	HealthCheckInterval time.Duration // default 10s
	HealthCheckTimeout  time.Duration // default 2s
	MaxRestarts         int           // default 3; negative disables restarts
	RestartDelay        time.Duration // default 1s
}
```

## What this does not do (yet)

- No exponential backoff — a fixed delay can retry too aggressively against a slow-to-recover dependency.
- No persisted quarantine state, and no live visibility (CLI or API) into a running agent's current supervision state (which plugins are up, restarting, or quarantined) — you have to read the agent's logs.
- No graceful shutdown RPC: `Process.Close` kills the process directly (`SIGKILL`-equivalent) rather than negotiating a shutdown over the protocol.

## Verifying it works

Manual smoke test: `kill -9` a running plugin's process while `patchcord serve` is up — the Supervisor detects the crash, restarts it (or quarantines it after `MaxRestarts` attempts), and the agent itself keeps running throughout. `internal/plugins/supervisor_test.go` covers the same scenarios (crash restart, quarantine after repeated failure, health-check-triggered restart) against real subprocesses.
