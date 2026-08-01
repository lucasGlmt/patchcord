package cli

import "testing"

func TestNewRootCommand_HasServeSubcommand(t *testing.T) {
	root := NewRootCommand()

	serve, _, err := root.Find([]string{"serve"})
	if err != nil {
		t.Fatalf("Find(serve) error = %v", err)
	}
	if serve.Name() != "serve" {
		t.Fatalf("found command %q, want %q", serve.Name(), "serve")
	}

	listenFlag := serve.Flags().Lookup("listen")
	if listenFlag == nil {
		t.Fatal("serve command has no --listen flag")
	}
	if listenFlag.DefValue != defaultListenAddr {
		t.Fatalf("--listen default = %q, want %q", listenFlag.DefValue, defaultListenAddr)
	}

	dataDirFlag := serve.Flags().Lookup("data-dir")
	if dataDirFlag == nil {
		t.Fatal("serve command has no --data-dir flag")
	}
	if dataDirFlag.DefValue != defaultDataDir {
		t.Fatalf("--data-dir default = %q, want %q", dataDirFlag.DefValue, defaultDataDir)
	}
}
