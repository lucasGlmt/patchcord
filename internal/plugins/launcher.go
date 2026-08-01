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
type Process struct {
	cmd    *exec.Cmd
	conn   *grpc.ClientConn
	Client pluginv1.PluginServiceClient
}

// Launch starts the plugin binary at path, waits for it to report its gRPC
// listen address on stdout, and dials it. It does not perform the protocol
// handshake itself; call Handshake with the returned Process's Client.
func Launch(ctx context.Context, path string, readyTimeout time.Duration) (*Process, error) {
	if readyTimeout <= 0 {
		readyTimeout = DefaultReadyTimeout
	}

	cmd := exec.CommandContext(ctx, path)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("attach stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start plugin process: %w", err)
	}

	ready, err := readReadyMessage(stdout, readyTimeout)
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

	return &Process{
		cmd:    cmd,
		conn:   conn,
		Client: pluginv1.NewPluginServiceClient(conn),
	}, nil
}

// readReadyMessage reads the plugin's first stdout line and parses it as a
// readyMessage, failing if none arrives within timeout.
func readReadyMessage(stdout io.Reader, timeout time.Duration) (*readyMessage, error) {
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
	case <-time.After(timeout):
		return nil, fmt.Errorf("plugin did not report its listen address within %s", timeout)
	}
}

// Close closes the connection to the plugin, terminates its process, and
// waits for it to exit, bounded by ctx.
//
// Closing the connection alone would not be enough: the plugin's gRPC
// server keeps running regardless of whether a client is attached, so the
// process must be killed explicitly. A graceful shutdown negotiated over
// the protocol itself is Plugin Supervisor work for a later phase.
func (p *Process) Close(ctx context.Context) error {
	closeErr := p.conn.Close()
	killErr := p.cmd.Process.Kill()

	done := make(chan struct{})
	go func() {
		_ = p.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return fmt.Errorf("plugin process did not exit before context was done: %w", ctx.Err())
	}

	if closeErr != nil {
		return fmt.Errorf("close plugin connection: %w", closeErr)
	}
	if killErr != nil {
		return fmt.Errorf("kill plugin process: %w", killErr)
	}
	return nil
}
