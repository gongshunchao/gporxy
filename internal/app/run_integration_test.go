package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunContextStartsControlSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "gproxy.sock")
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	content := "control:\n  socket: " + socketPath + "\n  udp_session_idle_timeout: 1s\nrules: []\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- RunContext(ctx, []string{"run", "-c", configPath})
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("RunContext returned error: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for RunContext to exit")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("control socket was not created")
}

func TestRunContextStatusAndStopCommands(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "gproxy.sock")
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	content := "control:\n  socket: " + socketPath + "\n  udp_session_idle_timeout: 1s\nrules: []\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- RunContext(ctx, []string{"run", "-c", configPath})
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	statusCtx, statusCancel := context.WithTimeout(context.Background(), time.Second)
	defer statusCancel()

	if err := RunContext(statusCtx, []string{"status", "-c", configPath}); err != nil {
		t.Fatalf("status command failed: %v", err)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()

	if err := RunContext(stopCtx, []string{"stop", "-c", configPath}); err != nil {
		t.Fatalf("stop command failed: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run context returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run context shutdown")
	}
}
