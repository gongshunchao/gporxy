package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gproxy/internal/control"
)

func TestRunRejectsMissingConfigFlag(t *testing.T) {
	err := Run([]string{"run"})
	if err == nil {
		t.Fatal("expected missing config error")
	}

	if !strings.Contains(err.Error(), "-c is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReloadRejectsMissingSocketServer(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("control:\n  socket: /tmp/missing.sock\nrules: []\n"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := Run([]string{"reload", "-c", configPath})
	if err == nil {
		t.Fatal("expected reload failure")
	}
}

func TestStatusRejectsMissingConfigFlag(t *testing.T) {
	err := Run([]string{"status"})
	if err == nil {
		t.Fatal("expected missing config error")
	}

	if !strings.Contains(err.Error(), "-c is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatStatusOutput(t *testing.T) {
	status := control.Status{
		State:            "running",
		SocketPath:       "/tmp/gproxy.sock",
		RuleCount:        5,
		TCPListenerCount: 3,
		UDPListenerCount: 2,
	}

	got := formatStatus(status)
	want := "state: running\nsocket: /tmp/gproxy.sock\nrules: 5\ntcp listeners: 3\nudp listeners: 2\n"
	if got != want {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestReloadUsesGeneralizedControlClient(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "gproxy.sock")
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	if err := os.WriteFile(configPath, []byte("control:\n  socket: "+socketPath+"\nrules: []\n"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	requests := make(chan control.Request, 1)
	server, err := control.NewServer(socketPath, func(_ context.Context, req control.Request) control.Response {
		requests <- req
		return control.Response{OK: true}
	})
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	defer func() { _ = server.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if err := RunContext(ctx, []string{"reload", "-c", configPath}); err != nil {
		t.Fatalf("reload command failed: %v", err)
	}

	select {
	case req := <-requests:
		if req.Command != control.CommandReload {
			t.Fatalf("unexpected command: %q", req.Command)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for reload request")
	}
}
