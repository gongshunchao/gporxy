package runtime

import (
	"context"
	"testing"
	"time"

	"gproxy/internal/config"
)

func TestManagerApplyReportsDiffCounts(t *testing.T) {
	manager := NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	first := SnapshotFromEntries([]config.Entry{
		{Name: "one", Protocol: "tcp", Listen: "127.0.0.1:0", Target: "127.0.0.1:1"},
	})

	second := SnapshotFromEntries([]config.Entry{
		{Name: "one", Protocol: "tcp", Listen: "127.0.0.1:0", Target: "127.0.0.1:1"},
		{Name: "two", Protocol: "udp", Listen: "127.0.0.1:0", Target: "127.0.0.1:2"},
	})

	if _, err := manager.Apply(ctx, first, nil); err != nil {
		t.Fatalf("first Apply returned error: %v", err)
	}

	result, err := manager.Apply(ctx, second, nil)
	if err != nil {
		t.Fatalf("second Apply returned error: %v", err)
	}

	if result.Added != 1 || result.Removed != 0 || result.Kept != 1 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
}

func TestManagerStatusReflectsExpandedSnapshot(t *testing.T) {
	manager := NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	next := SnapshotFromEntries([]config.Entry{
		{Name: "one", Protocol: "tcp", Listen: "127.0.0.1:1000", Target: "127.0.0.1:2000"},
		{Name: "two", Protocol: "udp", Listen: "127.0.0.1:1001", Target: "127.0.0.1:2001"},
		{Name: "three", Protocol: "tcp", Listen: "127.0.0.1:1002", Target: "127.0.0.1:2002"},
	})

	if _, err := manager.Apply(ctx, next, nil); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	status := manager.Status()
	if status.State != "running" {
		t.Fatalf("unexpected state: %q", status.State)
	}

	if status.RuleCount != 3 || status.TCPListenerCount != 2 || status.UDPListenerCount != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestManagerTransitionsToStopping(t *testing.T) {
	manager := NewManager()

	manager.MarkStopping()

	status := manager.Status()
	if status.State != "stopping" {
		t.Fatalf("unexpected state: %q", status.State)
	}
}
