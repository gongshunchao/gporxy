package udp

import (
	"context"
	"net"
	"testing"
	"time"

	"gproxy/internal/config"
)

func TestProxyForwardsUDPDatagrams(t *testing.T) {
	target, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target: %v", err)
	}
	defer target.Close()

	go func() {
		buf := make([]byte, 1024)
		n, addr, err := target.ReadFrom(buf)
		if err != nil {
			return
		}
		_, _ = target.WriteTo(buf[:n], addr)
	}()

	proxy, err := New(config.Entry{
		Name:     "udp",
		Protocol: "udp",
		Listen:   "127.0.0.1:0",
		Target:   target.LocalAddr().String(),
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() { _ = proxy.Close() }()

	client, err := net.Dial("udp", proxy.ListenAddr())
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("pong")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	buf := make([]byte, 4)
	if _, err := client.Read(buf); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if string(buf) != "pong" {
		t.Fatalf("unexpected reply: %q", string(buf))
	}
}

func TestProxyCleansUpIdleSessions(t *testing.T) {
	target, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target: %v", err)
	}
	defer target.Close()

	proxy, err := New(config.Entry{
		Name:     "udp",
		Protocol: "udp",
		Listen:   "127.0.0.1:0",
		Target:   target.LocalAddr().String(),
	}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() { _ = proxy.Close() }()

	client, err := net.Dial("udp", proxy.ListenAddr())
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("x")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if got := proxy.SessionCount(); got != 0 {
		t.Fatalf("expected idle sessions to be removed, got %d", got)
	}
}
