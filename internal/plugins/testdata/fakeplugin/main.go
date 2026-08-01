// Command fakeplugin is a minimal stand-in Patchcord plugin used only by
// internal/plugins tests to exercise Launch and Handshake against a real
// process, ahead of the actual plugin SDK.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"

	pluginv1 "github.com/lucasglmt/patchcord/api/plugin/v1"
)

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
	switch os.Getenv("FAKE_PLUGIN_MODE") {
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

	ready, err := json.Marshal(struct {
		Address string `json:"address"`
	}{Address: listener.Addr().String()})
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal ready message:", err)
		os.Exit(1)
	}
	fmt.Println(string(ready))

	_ = grpcServer.Serve(listener)
}
