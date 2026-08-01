package plugins

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

// DefaultReadyTimeout bounds how long Launch waits for a plugin process to
// report its listen address before giving up.
const DefaultReadyTimeout = 10 * time.Second

// readyMessage is the single line a plugin process prints to its own stdout
// once its gRPC server is listening on a local TCP port, telling the agent
// where to dial it.
type readyMessage struct {
	Address string `json:"address"`
}

// Process is a running plugin subprocess the agent has connected to.
//
// Its lifetime is independent of the context passed to Launch: that
// context only bounds the launch attempt itself (waiting for the plugin to
// report ready). Once Launch returns successfully, the process keeps
// running until Close is called or it exits on its own — which is exactly
// what the Plugin Supervisor needs to detect and react to a crash.
type Process struct {
	cmd          *exec.Cmd
	conn         *grpc.ClientConn
	Client       pluginv1.PluginServiceClient
	HealthClient grpc_health_v1.HealthClient

	exited  chan struct{}
	waitErr error
}

// Launch starts the plugin binary at path, waits for it to report its gRPC
// listen address on stdout, and dials it. It does not perform the protocol
// handshake itself; call Handshake with the returned Process's Client.
//
// ctx only bounds this launch attempt: if it is cancelled before the
// plugin reports ready, the partially-started process is killed and Launch
// returns ctx's error. It has no effect on the process once Launch has
// returned successfully.
func Launch(ctx context.Context, path string, readyTimeout time.Duration) (*Process, error) {
	if readyTimeout <= 0 {
		readyTimeout = DefaultReadyTimeout
	}

	cmd := exec.Command(path)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("attach stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start plugin process: %w", err)
	}

	ready, err := readReadyMessage(ctx, stdout, readyTimeout)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}

	conn, err := grpc.NewClient(
		ready.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("dial plugin at %s: %w", ready.Address, err)
	}

	proc := &Process{
		cmd:          cmd,
		conn:         conn,
		Client:       pluginv1.NewPluginServiceClient(conn),
		HealthClient: grpc_health_v1.NewHealthClient(conn),
		exited:       make(chan struct{}),
	}

	go func() {
		proc.waitErr = cmd.Wait()
		close(proc.exited)
	}()

	return proc, nil
}

// readReadyMessage reads the plugin's first stdout line and parses it as a
// readyMessage, failing if none arrives before ctx is done or timeout
// elapses.
func readReadyMessage(ctx context.Context, stdout io.Reader, timeout time.Duration) (*readyMessage, error) {
	type result struct {
		line []byte
		err  error
	}

	lineCh := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadBytes('\n')
		lineCh <- result{line, err}
	}()

	select {
	case r := <-lineCh:
		if r.err != nil {
			return nil, fmt.Errorf("read plugin bootstrap line: %w", r.err)
		}
		var ready readyMessage
		if err := json.Unmarshal(r.line, &ready); err != nil {
			return nil, fmt.Errorf("parse plugin bootstrap line %q: %w", r.line, err)
		}
		if ready.Address == "" {
			return nil, fmt.Errorf("plugin bootstrap line %q is missing an address", r.line)
		}
		return &ready, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("plugin launch cancelled: %w", ctx.Err())
	case <-time.After(timeout):
		return nil, fmt.Errorf("plugin did not report its listen address within %s", timeout)
	}
}

// Exited returns a channel that is closed once the plugin process has
// exited, whether cleanly (via Close) or unexpectedly (a crash). Use
// ExitErr after it closes to find out which.
func (p *Process) Exited() <-chan struct{} {
	return p.exited
}

// ExitErr reports why the process exited. It is only meaningful once the
// channel returned by Exited is closed.
func (p *Process) ExitErr() error {
	return p.waitErr
}

// Close closes the connection to the plugin, terminates its process if it
// isn't already gone, and waits for it to exit, bounded by ctx.
//
// Closing the connection alone would not be enough: the plugin's gRPC
// server keeps running regardless of whether a client is attached, so the
// process must be killed explicitly.
func (p *Process) Close(ctx context.Context) error {
	closeErr := p.conn.Close()

	select {
	case <-p.exited:
		// Already gone — most likely a crash the Supervisor is reacting
		// to. Nothing left to kill.
	default:
		_ = p.cmd.Process.Kill()
	}

	select {
	case <-p.exited:
	case <-ctx.Done():
		return fmt.Errorf("plugin process did not exit before context was done: %w", ctx.Err())
	}

	if closeErr != nil {
		return fmt.Errorf("close plugin connection: %w", closeErr)
	}
	return nil
}
