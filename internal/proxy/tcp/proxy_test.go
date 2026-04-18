package tcp

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"gproxy/internal/config"
)

func TestProxyForwardsSingleTCPConnection(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target: %v", err)
	}
	defer target.Close()

	go func() {
		conn, err := target.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

	proxy, err := New(config.Entry{
		Name:     "tcp",
		Protocol: "tcp",
		Listen:   "127.0.0.1:0",
		Target:   target.Addr().String(),
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() { _ = proxy.Close() }()

	conn, err := net.DialTimeout("tcp", proxy.ListenAddr(), time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if string(buf) != "ping" {
		t.Fatalf("unexpected response: %q", string(buf))
	}
}
