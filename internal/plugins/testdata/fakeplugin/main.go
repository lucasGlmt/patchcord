// Command fakeplugin is a minimal stand-in Patchcord plugin used only by
// internal/plugins tests to exercise Launch, Handshake and the Supervisor
// against real processes, ahead of the actual plugin SDK.
//
// Its behavior is controlled by environment variables so a single binary
// can play every role the tests need:
//
//   - FAKE_PLUGIN_MODE=silent: never reports ready (launch timeout).
//   - FAKE_PLUGIN_MODE=garbage: reports a non-JSON bootstrap line.
//   - FAKE_PLUGIN_MODE=unhealthy: serves normally but reports NOT_SERVING.
//   - FAKE_PLUGIN_MODE=crash-once: crashes shortly after starting, but
//     only the first time; a marker file (FAKE_PLUGIN_MARKER_FILE) records
//     that it already crashed once, so a restart behaves normally.
//   - FAKE_PLUGIN_MODE=always-crash: crashes shortly after starting, every
//     single time, to exercise quarantine after repeated failures.
//   - unset: serves normally and stays healthy.
//
// FAKE_PLUGIN_LAUNCH_COUNTER_FILE, if set, is appended one byte every time
// the process starts, so a test can observe how many times it was
// (re)launched.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

const crashDelay = 150 * time.Millisecond

type server struct {
	pluginv1.UnimplementedPluginServiceServer
}

func (server) Handshake(context.Context, *pluginv1.HandshakeRequest) (*pluginv1.HandshakeResponse, error) {
	return &pluginv1.HandshakeResponse{
		ProtocolVersion: 1,
		PluginId:        "io.patchcord.fake",
		PluginVersion:   "0.0.1",
	}, nil
}

func main() {
	recordLaunch()

	mode := os.Getenv("FAKE_PLUGIN_MODE")

	switch mode {
	case "silent":
		time.Sleep(time.Hour)
		return
	case "garbage":
		fmt.Println("not json")
		time.Sleep(time.Hour)
		return
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	pluginv1.RegisterPluginServiceServer(grpcServer, server{})

	healthServer := health.NewServer()
	if mode == "unhealthy" {
		healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	} else {
		healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	}
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	ready, err := json.Marshal(struct {
		Address string `json:"address"`
	}{Address: listener.Addr().String()})
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal ready message:", err)
		os.Exit(1)
	}
	fmt.Println(string(ready))

	if mode == "always-crash" || (mode == "crash-once" && !alreadyCrashedOnce()) {
		go func() { _ = grpcServer.Serve(listener) }()
		time.Sleep(crashDelay)
		os.Exit(1)
	}

	_ = grpcServer.Serve(listener)
}

// recordLaunch appends one byte to FAKE_PLUGIN_LAUNCH_COUNTER_FILE, if
// set, so a test can count how many times this binary has been started.
func recordLaunch() {
	path := os.Getenv("FAKE_PLUGIN_LAUNCH_COUNTER_FILE")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString("x")
}

// alreadyCrashedOnce reports whether this process has crashed before, by
// checking (and creating) a marker file at FAKE_PLUGIN_MARKER_FILE.
func alreadyCrashedOnce() bool {
	path := os.Getenv("FAKE_PLUGIN_MARKER_FILE")
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	_ = os.WriteFile(path, []byte("crashed"), 0o644)
	return false
}
