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

func (echoAction) ID() string           { return "test.echo@1" }
func (echoAction) Description() string  { return "Echoes its input back as output." }
func (echoAction) InputSchema() Schema  { return Schema{"type": "object"} }
func (echoAction) OutputSchema() Schema { return Schema{"type": "object"} }

func (echoAction) Run(_ context.Context, input ActionInput, _ *ConnectorConfig) (ActionOutput, error) {
	return ActionOutput(input), nil
}

type failingAction struct{}

func (failingAction) ID() string           { return "test.fail@1" }
func (failingAction) Description() string  { return "Always fails." }
func (failingAction) InputSchema() Schema  { return Schema{"type": "object"} }
func (failingAction) OutputSchema() Schema { return Schema{"type": "object"} }

func (failingAction) Run(context.Context, ActionInput, *ConnectorConfig) (ActionOutput, error) {
	return nil, errors.New("boom")
}

// stubTester is a ConnectorTester whose outcome is fixed per test.
type stubTester struct {
	err error
	got ConnectorConfig
}

func (s *stubTester) TestConnector(_ context.Context, connector ConnectorConfig) error {
	s.got = connector
	return s.err
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
		{
			name: "accepts a recognized permission",
			plugin: Plugin{
				Manifest:    Manifest{ID: "io.example.test", Version: "1.0.0"},
				Permissions: []Permission{PermissionNetworkOutbound},
			},
		},
		{
			name: "rejects an unrecognized permission",
			plugin: Plugin{
				Manifest:    Manifest{ID: "io.example.test", Version: "1.0.0"},
				Permissions: []Permission{"filesystem.read"},
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

// TestServe_ReturnsNewServerErrorWithoutBlocking exercises Serve's own
// "srv, err := newServer(plugin)" branch — the only part of Serve that
// returns without binding a real listener or blocking forever, so it's the
// only part of Serve a test can call directly and expect back. The rest of
// Serve (net.Listen on a real port, registering the gRPC/health services,
// printing the ready message, then grpcServer.Serve blocking until the
// process is killed) is exercised indirectly by every example plugin
// process the agent actually launches (internal/plugins.Supervisor's
// tests) rather than in-process here, the same reason
// internal/runtime.Agent.Run and internal/scheduler.Runner.Run don't reach
// 100% either: it has no listener/context hook a test could use to
// unblock it cleanly.
func TestServe_ReturnsNewServerErrorWithoutBlocking(t *testing.T) {
	err := Serve(Plugin{Manifest: Manifest{Version: "1.0.0"}}) // missing ID
	if err == nil {
		t.Fatal("expected an error for a plugin with no manifest id, got nil")
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
			name:            "accepts an agent that supports this plugin's protocol version",
			agentProtocol:   protocolVersion,
			wantProtocol:    protocolVersion,
			wantPluginID:    "io.example.test",
			wantActionCount: 1,
		},
		{
			name:          "rejects an agent that supports no protocol version",
			agentProtocol: 0,
			wantErr:       true,
		},
		{
			name:          "rejects an agent whose supported version is below this plugin's",
			agentProtocol: protocolVersion - 1,
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

func TestServer_Handshake_ReportsConnectors(t *testing.T) {
	srv, err := newServer(Plugin{
		Manifest: Manifest{ID: "io.example.test", Version: "1.0.0"},
		Actions:  []Action{echoAction{}},
		Connectors: []Connector{
			{Type: "http.connection@1", Description: "An HTTP base URL.", ConfigSchema: Schema{"type": "object"}},
		},
	})
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}
	client := dialServer(t, srv)

	resp, err := client.Handshake(context.Background(), &pluginv1.HandshakeRequest{ProtocolVersion: protocolVersion})
	if err != nil {
		t.Fatalf("Handshake() error = %v", err)
	}

	gotConnectors := resp.GetContributes().GetConnectors()
	if len(gotConnectors) != 1 || gotConnectors[0].GetType() != "http.connection@1" {
		t.Fatalf("Connectors = %v, want [http.connection@1]", gotConnectors)
	}
	if gotConnectors[0].GetDescription() == "" {
		t.Fatal("Connectors[0].Description is empty, want the plugin's description")
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

func TestServer_TestConnector(t *testing.T) {
	connectorProto := func(t *testing.T) *pluginv1.ConnectorConfig {
		t.Helper()
		configStruct, err := structpb.NewStruct(map[string]any{"host": "db.internal"})
		if err != nil {
			t.Fatalf("build config struct: %v", err)
		}
		return &pluginv1.ConnectorConfig{Type: "postgresql.connection@1", Config: configStruct}
	}

	t.Run("returns Unimplemented when the plugin sets no Tester", func(t *testing.T) {
		srv, err := newServer(Plugin{
			Manifest: Manifest{ID: "io.example.test", Version: "1.0.0"},
		})
		if err != nil {
			t.Fatalf("newServer() error = %v", err)
		}
		client := dialServer(t, srv)

		_, err = client.TestConnector(context.Background(), &pluginv1.TestConnectorRequest{Connector: connectorProto(t)})
		if status.Code(err) != codes.Unimplemented {
			t.Fatalf("status code = %v, want %v", status.Code(err), codes.Unimplemented)
		}
	})

	t.Run("returns InvalidArgument when no connector is given", func(t *testing.T) {
		srv, err := newServer(Plugin{
			Manifest: Manifest{ID: "io.example.test", Version: "1.0.0"},
			Tester:   &stubTester{},
		})
		if err != nil {
			t.Fatalf("newServer() error = %v", err)
		}
		client := dialServer(t, srv)

		_, err = client.TestConnector(context.Background(), &pluginv1.TestConnectorRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("status code = %v, want %v", status.Code(err), codes.InvalidArgument)
		}
	})

	t.Run("reports Ok=true on success and forwards the connector", func(t *testing.T) {
		tester := &stubTester{}
		srv, err := newServer(Plugin{
			Manifest: Manifest{ID: "io.example.test", Version: "1.0.0"},
			Tester:   tester,
		})
		if err != nil {
			t.Fatalf("newServer() error = %v", err)
		}
		client := dialServer(t, srv)

		resp, err := client.TestConnector(context.Background(), &pluginv1.TestConnectorRequest{Connector: connectorProto(t)})
		if err != nil {
			t.Fatalf("TestConnector() error = %v", err)
		}
		if !resp.GetOk() {
			t.Fatalf("Ok = %v, want true", resp.GetOk())
		}
		if tester.got.Type != "postgresql.connection@1" || tester.got.Config["host"] != "db.internal" {
			t.Fatalf("tester received %+v, want the decoded connector", tester.got)
		}
	})

	t.Run("reports Ok=false and the error message on failure, without an RPC error", func(t *testing.T) {
		srv, err := newServer(Plugin{
			Manifest: Manifest{ID: "io.example.test", Version: "1.0.0"},
			Tester:   &stubTester{err: errors.New("connection refused")},
		})
		if err != nil {
			t.Fatalf("newServer() error = %v", err)
		}
		client := dialServer(t, srv)

		resp, err := client.TestConnector(context.Background(), &pluginv1.TestConnectorRequest{Connector: connectorProto(t)})
		if err != nil {
			t.Fatalf("TestConnector() error = %v, want nil (a failed test is a legitimate result)", err)
		}
		if resp.GetOk() {
			t.Fatal("Ok = true, want false")
		}
		if resp.GetMessage() != "connection refused" {
			t.Fatalf("Message = %q, want %q", resp.GetMessage(), "connection refused")
		}
	})
}

func TestConnectorConfigFromProto(t *testing.T) {
	t.Run("returns nil for a nil input", func(t *testing.T) {
		if got := connectorConfigFromProto(nil); got != nil {
			t.Fatalf("connectorConfigFromProto(nil) = %v, want nil", got)
		}
	})

	t.Run("decodes type, config and secrets", func(t *testing.T) {
		configStruct, err := structpb.NewStruct(map[string]any{"host": "db.internal"})
		if err != nil {
			t.Fatalf("build config struct: %v", err)
		}
		secretsStruct, err := structpb.NewStruct(map[string]any{"password": "s3cr3t"})
		if err != nil {
			t.Fatalf("build secrets struct: %v", err)
		}

		got := connectorConfigFromProto(&pluginv1.ConnectorConfig{
			Type:    "postgresql.connection@1",
			Config:  configStruct,
			Secrets: secretsStruct,
		})

		if got.Type != "postgresql.connection@1" {
			t.Fatalf("Type = %q, want %q", got.Type, "postgresql.connection@1")
		}
		if got.Config["host"] != "db.internal" {
			t.Fatalf("Config[host] = %v, want %q", got.Config["host"], "db.internal")
		}
		if got.Secrets["password"] != "s3cr3t" {
			t.Fatalf("Secrets[password] = %v, want %q", got.Secrets["password"], "s3cr3t")
		}
	})
}
