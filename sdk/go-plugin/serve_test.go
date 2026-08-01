package patchcord

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

type echoAction struct{}

func (echoAction) ID() string { return "test.echo@1" }

func (echoAction) Run(_ context.Context, input ActionInput) (ActionOutput, error) {
	return ActionOutput(input), nil
}

type failingAction struct{}

func (failingAction) ID() string { return "test.fail@1" }

func (failingAction) Run(context.Context, ActionInput) (ActionOutput, error) {
	return nil, errors.New("boom")
}

func TestNewServer(t *testing.T) {
	tests := []struct {
		name    string
		plugin  Plugin
		wantErr bool
	}{
		{
			name: "accepts a valid plugin",
			plugin: Plugin{
				Manifest: Manifest{ID: "io.example.test", Version: "1.0.0"},
				Actions:  []Action{echoAction{}},
			},
		},
		{
			name:    "rejects a missing id",
			plugin:  Plugin{Manifest: Manifest{Version: "1.0.0"}},
			wantErr: true,
		},
		{
			name:    "rejects a missing version",
			plugin:  Plugin{Manifest: Manifest{ID: "io.example.test"}},
			wantErr: true,
		},
		{
			name: "rejects duplicate action ids",
			plugin: Plugin{
				Manifest: Manifest{ID: "io.example.test", Version: "1.0.0"},
				Actions:  []Action{echoAction{}, echoAction{}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newServer(tt.plugin)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("newServer() error = %v", err)
			}
		})
	}
}

// dialServer starts an in-memory gRPC server backed by srv and returns a
// client connected to it.
func dialServer(t *testing.T, srv *server) pluginv1.PluginServiceClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	pluginv1.RegisterPluginServiceServer(grpcServer, srv)

	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(grpcServer.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return pluginv1.NewPluginServiceClient(conn)
}

func TestServer_Handshake(t *testing.T) {
	tests := []struct {
		name            string
		agentProtocol   uint32
		wantErr         bool
		wantProtocol    uint32
		wantPluginID    string
		wantActionCount int
	}{
		{
			name:            "negotiates the plugin's protocol version when the agent supports it",
			agentProtocol:   1,
			wantProtocol:    1,
			wantPluginID:    "io.example.test",
			wantActionCount: 1,
		},
		{
			name:          "rejects an agent that supports no protocol version",
			agentProtocol: 0,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, err := newServer(Plugin{
				Manifest: Manifest{ID: "io.example.test", Version: "1.0.0"},
				Actions:  []Action{echoAction{}},
			})
			if err != nil {
				t.Fatalf("newServer() error = %v", err)
			}
			client := dialServer(t, srv)

			resp, err := client.Handshake(context.Background(), &pluginv1.HandshakeRequest{
				ProtocolVersion: tt.agentProtocol,
			})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Handshake() error = %v", err)
			}
			if resp.GetProtocolVersion() != tt.wantProtocol {
				t.Fatalf("ProtocolVersion = %d, want %d", resp.GetProtocolVersion(), tt.wantProtocol)
			}
			if resp.GetPluginId() != tt.wantPluginID {
				t.Fatalf("PluginId = %q, want %q", resp.GetPluginId(), tt.wantPluginID)
			}
			if len(resp.GetContributes().GetActions()) != tt.wantActionCount {
				t.Fatalf("len(Actions) = %d, want %d", len(resp.GetContributes().GetActions()), tt.wantActionCount)
			}
		})
	}
}

func TestServer_ExecuteAction(t *testing.T) {
	srv, err := newServer(Plugin{
		Manifest: Manifest{ID: "io.example.test", Version: "1.0.0"},
		Actions:  []Action{echoAction{}, failingAction{}},
	})
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	client := dialServer(t, srv)

	t.Run("runs a known action and returns its output", func(t *testing.T) {
		input, err := structpb.NewStruct(map[string]any{"value": "hello"})
		if err != nil {
			t.Fatalf("build input: %v", err)
		}

		resp, err := client.ExecuteAction(context.Background(), &pluginv1.ExecuteActionRequest{
			Action: "test.echo@1",
			Input:  input,
		})
		if err != nil {
			t.Fatalf("ExecuteAction() error = %v", err)
		}
		if got := resp.GetOutput().AsMap()["value"]; got != "hello" {
			t.Fatalf("output[value] = %v, want %q", got, "hello")
		}
	})

	t.Run("returns NotFound for an unknown action", func(t *testing.T) {
		_, err := client.ExecuteAction(context.Background(), &pluginv1.ExecuteActionRequest{
			Action: "test.unknown@1",
		})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("status code = %v, want %v", status.Code(err), codes.NotFound)
		}
	})

	t.Run("returns Internal when the action fails", func(t *testing.T) {
		_, err := client.ExecuteAction(context.Background(), &pluginv1.ExecuteActionRequest{
			Action: "test.fail@1",
		})
		if status.Code(err) != codes.Internal {
			t.Fatalf("status code = %v, want %v", status.Code(err), codes.Internal)
		}
	})
}
