package plugins

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

type fakePluginServer struct {
	pluginv1.UnimplementedPluginServiceServer

	response *pluginv1.HandshakeResponse
	err      error

	executeResponse *pluginv1.ExecuteActionResponse
	executeErr      error
}

func (f *fakePluginServer) Handshake(context.Context, *pluginv1.HandshakeRequest) (*pluginv1.HandshakeResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func (f *fakePluginServer) ExecuteAction(context.Context, *pluginv1.ExecuteActionRequest) (*pluginv1.ExecuteActionResponse, error) {
	if f.executeErr != nil {
		return nil, f.executeErr
	}
	return f.executeResponse, nil
}

// dialFakePlugin starts an in-memory gRPC server backed by srv and returns a
// client connected to it, so handshake logic can be tested without spawning
// a real plugin process.
func dialFakePlugin(t *testing.T, srv *fakePluginServer) pluginv1.PluginServiceClient {
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

func TestHandshake(t *testing.T) {
	tests := []struct {
		name     string
		response *pluginv1.HandshakeResponse
		rpcErr   error
		wantErr  bool
		want     *Manifest
	}{
		{
			name: "accepts a compatible manifest",
			response: &pluginv1.HandshakeResponse{
				ProtocolVersion: 1,
				PluginId:        "io.patchcord.example-text",
				PluginVersion:   "1.0.0",
				Contributes: &pluginv1.Contributions{
					Actions: []string{"text.uppercase@1"},
				},
				Permissions: []string{"network.outbound"},
			},
			want: &Manifest{
				ProtocolVersion: 1,
				PluginID:        "io.patchcord.example-text",
				PluginVersion:   "1.0.0",
				Actions:         []string{"text.uppercase@1"},
				Permissions:     []string{"network.outbound"},
			},
		},
		{
			name: "rejects a protocol version higher than the agent supports",
			response: &pluginv1.HandshakeResponse{
				ProtocolVersion: 2,
				PluginId:        "io.patchcord.example-text",
				PluginVersion:   "1.0.0",
			},
			wantErr: true,
		},
		{
			name: "rejects a zero protocol version",
			response: &pluginv1.HandshakeResponse{
				PluginId:      "io.patchcord.example-text",
				PluginVersion: "1.0.0",
			},
			wantErr: true,
		},
		{
			name: "rejects a manifest without a plugin id",
			response: &pluginv1.HandshakeResponse{
				ProtocolVersion: 1,
				PluginVersion:   "1.0.0",
			},
			wantErr: true,
		},
		{
			name: "rejects a manifest without a plugin version",
			response: &pluginv1.HandshakeResponse{
				ProtocolVersion: 1,
				PluginId:        "io.patchcord.example-text",
			},
			wantErr: true,
		},
		{
			name:    "propagates an RPC error",
			rpcErr:  errors.New("boom"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := dialFakePlugin(t, &fakePluginServer{response: tt.response, err: tt.rpcErr})

			got, err := Handshake(context.Background(), client)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Handshake() error = %v", err)
			}

			if got.ProtocolVersion != tt.want.ProtocolVersion ||
				got.PluginID != tt.want.PluginID ||
				got.PluginVersion != tt.want.PluginVersion ||
				!slices.Equal(got.Connectors, tt.want.Connectors) ||
				!slices.Equal(got.Actions, tt.want.Actions) ||
				!slices.Equal(got.Permissions, tt.want.Permissions) {
				t.Fatalf("Handshake() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
