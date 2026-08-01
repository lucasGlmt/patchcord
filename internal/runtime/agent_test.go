package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewAgent_InvalidListenAddress(t *testing.T) {
	cfg := Config{ListenAddr: "not-a-valid-address", DataDir: t.TempDir()}
	_, err := NewAgent(cfg, testLogger())
	if err == nil {
		t.Fatal("expected an error for an invalid listen address, got nil")
	}
}

func TestAgent_RunServesHealthAndShutsDownOnCancel(t *testing.T) {
	cfg := Config{ListenAddr: "127.0.0.1:0", DataDir: t.TempDir()}
	agent, err := NewAgent(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	runErr := make(chan error, 1)
	go func() {
		runErr <- agent.Run(ctx)
	}()

	healthURL := fmt.Sprintf("http://%s/v1/system/health", agent.Addr())

	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get(healthURL)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET %s: %v", healthURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Status   string `json:"status"`
		Database string `json:"database"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body.Status != "ok" || body.Database != "ok" {
		t.Fatalf("body = %+v, want status/database = ok/ok", body)
	}

	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil after clean shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return within the shutdown timeout")
	}
}
