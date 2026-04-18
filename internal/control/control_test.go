package control

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestClientSendsReloadRequest(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "gproxy.sock")
	requests := make(chan Request, 1)

	server, err := NewServer(socketPath, func(_ context.Context, req Request) Response {
		requests <- req
		return Response{OK: true}
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

	client := NewClient(socketPath)
	if err := client.Reload(ctx, "rules: []"); err != nil {
		t.Fatalf("Reload returned error: %v", err)
	}

	select {
	case req := <-requests:
		if req.Command != CommandReload || req.ConfigYAML != "rules: []" {
			t.Fatalf("unexpected request: %+v", req)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for reload request")
	}
}

func TestClientGetsStatusResponse(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "gproxy.sock")

	server, err := NewServer(socketPath, func(_ context.Context, req Request) Response {
		if req.Command != CommandStatus {
			t.Fatalf("unexpected command: %q", req.Command)
		}

		return Response{
			OK: true,
			Status: &Status{
				SocketPath:       socketPath,
				State:            "running",
				RuleCount:        4,
				TCPListenerCount: 2,
				UDPListenerCount: 2,
			},
		}
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

	client := NewClient(socketPath)
	status, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}

	if status.State != "running" || status.RuleCount != 4 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestClientSendsStopRequest(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "gproxy.sock")
	commands := make(chan Command, 1)

	server, err := NewServer(socketPath, func(_ context.Context, req Request) Response {
		commands <- req.Command
		return Response{OK: true}
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

	client := NewClient(socketPath)
	if err := client.Stop(ctx); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	select {
	case command := <-commands:
		if command != CommandStop {
			t.Fatalf("unexpected command: %q", command)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for stop request")
	}
}
