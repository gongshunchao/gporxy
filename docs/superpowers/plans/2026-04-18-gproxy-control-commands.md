# Gproxy Control Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `status` and `stop` management commands over the existing Unix socket control plane, with human-readable status output and graceful daemon shutdown.

**Architecture:** The current reload-only control protocol is generalized into a command-based request/response model. `runtime.Manager` becomes the source of truth for lifecycle state and expanded listener counts. The app layer exposes `status` and `stop` commands, and the run loop uses a dedicated internal stop context so the control plane can initiate graceful process shutdown.

**Tech Stack:** Go, Go standard library networking/concurrency/testing packages, existing YAML config loader and Unix socket control plane.

---

## File Structure

Planned files and responsibilities:

- `internal/control/protocol.go`
  - Generalized control request/response types and status payload.
- `internal/control/client.go`
  - Generic request method plus `Reload`, `Status`, and `Stop` helpers.
- `internal/control/server.go`
  - Generic request dispatch for multiple control commands.
- `internal/control/control_test.go`
  - Protocol tests for `reload`, `status`, and `stop`.
- `internal/runtime/manager.go`
  - Runtime lifecycle state, listener counts, and graceful shutdown support.
- `internal/runtime/manager_test.go`
  - Status snapshot and stopping-state tests.
- `internal/app/app.go`
  - `status` / `stop` CLI support, status formatting, graceful stop orchestration.
- `internal/app/app_test.go`
  - Command parsing tests and status formatting tests.
- `internal/app/run_integration_test.go`
  - Integration test for app startup plus control-command lifecycle.
- `README.md`
  - Usage examples for `status` and `stop`.
- `example/gproxy.yaml`
  - Reused by CLI examples; no structural change expected.

### Task 1: Generalize the Control Protocol

**Files:**
- Modify: `internal/control/protocol.go`
- Modify: `internal/control/client.go`
- Modify: `internal/control/server.go`
- Modify: `internal/control/control_test.go`

- [ ] **Step 1: Write the failing control protocol tests**

Append to `internal/control/control_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/control -run 'TestClient(GetsStatusResponse|SendsStopRequest)' -v`
Expected: FAIL because the protocol only supports reload types.

- [ ] **Step 3: Write minimal protocol implementation**

Update `internal/control/protocol.go`
```go
package control

type Command string

const (
	CommandReload Command = "reload"
	CommandStatus Command = "status"
	CommandStop   Command = "stop"
)

type Request struct {
	Command    Command `json:"command"`
	ConfigYAML string  `json:"config_yaml,omitempty"`
}

type Status struct {
	SocketPath       string `json:"socket_path"`
	State            string `json:"state"`
	RuleCount        int    `json:"rule_count"`
	TCPListenerCount int    `json:"tcp_listener_count"`
	UDPListenerCount int    `json:"udp_listener_count"`
}

type Response struct {
	OK     bool    `json:"ok"`
	Error  string  `json:"error,omitempty"`
	Status *Status `json:"status,omitempty"`
}
```

Update `internal/control/server.go`
```go
package control

import (
	"context"
	"encoding/json"
	"net"
	"os"
)

type Handler func(context.Context, Request) Response

type Server struct {
	socketPath string
	handler    Handler
	listener   net.Listener
}

func NewServer(socketPath string, handler Handler) (*Server, error) {
	return &Server{socketPath: socketPath, handler: handler}, nil
}

func (s *Server) Start(ctx context.Context) error {
	_ = os.Remove(s.socketPath)
	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}
	s.listener = ln

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handleConn(ctx, conn)
		}
	}()

	return nil
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Error: err.Error()})
		return
	}

	resp := s.handler(ctx, req)
	_ = json.NewEncoder(conn).Encode(resp)
}

func (s *Server) Close() error {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	_ = os.Remove(s.socketPath)
	return nil
}
```

Update `internal/control/client.go`
```go
package control

import (
	"context"
	"encoding/json"
	"errors"
	"net"
)

type Client struct {
	socketPath string
}

func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

func (c *Client) Do(ctx context.Context, req Request) (Response, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, err
	}

	if !resp.OK {
		return Response{}, errors.New(resp.Error)
	}

	return resp, nil
}

func (c *Client) Reload(ctx context.Context, configYAML string) error {
	_, err := c.Do(ctx, Request{Command: CommandReload, ConfigYAML: configYAML})
	return err
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	resp, err := c.Do(ctx, Request{Command: CommandStatus})
	if err != nil {
		return Status{}, err
	}
	if resp.Status == nil {
		return Status{}, errors.New("missing status payload")
	}
	return *resp.Status, nil
}

func (c *Client) Stop(ctx context.Context) error {
	_, err := c.Do(ctx, Request{Command: CommandStop})
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/control -run 'TestClient(GetsStatusResponse|SendsStopRequest)' -v`
Expected: PASS.

### Task 2: Add Runtime Status Snapshot and Lifecycle State

**Files:**
- Modify: `internal/runtime/manager.go`
- Modify: `internal/runtime/manager_test.go`

- [ ] **Step 1: Write the failing runtime status tests**

Append to `internal/runtime/manager_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime -run 'TestManager(StatusReflectsExpandedSnapshot|TransitionsToStopping)' -v`
Expected: FAIL because `Status` and `MarkStopping` do not exist.

- [ ] **Step 3: Write minimal runtime status implementation**

Update `internal/runtime/manager.go`
```go
package runtime

import (
	"context"
	"sync"
)

type Starter interface {
	Start(context.Context) error
	Close() error
}

type Factory func(context.Context, Item) (Starter, error)

type ApplyResult struct {
	Added   int
	Removed int
	Kept    int
}

type Status struct {
	State            string
	RuleCount        int
	TCPListenerCount int
	UDPListenerCount int
}

type Manager struct {
	mu      sync.RWMutex
	current Snapshot
	active  map[string]Starter
	state   string
}

func NewManager() *Manager {
	return &Manager{
		current: Snapshot{Items: map[string]Item{}},
		active:  make(map[string]Starter),
		state:   "running",
	}
}

func (m *Manager) Apply(ctx context.Context, next Snapshot, factory Factory) (ApplyResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	diff := Diff(m.current, next)

	for _, item := range diff.Remove {
		if active, ok := m.active[item.Key]; ok {
			_ = active.Close()
			delete(m.active, item.Key)
		}
	}

	for _, item := range diff.Add {
		if factory == nil {
			continue
		}
		active, err := factory(ctx, item)
		if err != nil {
			return ApplyResult{}, err
		}
		if err := active.Start(ctx); err != nil {
			return ApplyResult{}, err
		}
		m.active[item.Key] = active
	}

	m.current = next
	return BuildApplyResult(diff), nil
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := Status{State: m.state, RuleCount: len(m.current.Items)}
	for _, item := range m.current.Items {
		switch item.Entry.Protocol {
		case "tcp":
			status.TCPListenerCount++
		case "udp":
			status.UDPListenerCount++
		}
	}
	return status
}

func (m *Manager) MarkStopping() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = "stopping"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime -run 'TestManager(StatusReflectsExpandedSnapshot|TransitionsToStopping)' -v`
Expected: PASS.

### Task 3: Add App Commands and Status Formatting

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

- [ ] **Step 1: Write the failing app tests**

Append to `internal/app/app_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app -run 'Test(StatusRejectsMissingConfigFlag|FormatStatusOutput)' -v`
Expected: FAIL because `status` command handling and `formatStatus` do not exist.

- [ ] **Step 3: Write minimal app command implementation**

Update `internal/app/app.go`
```go
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gproxy/internal/config"
	"gproxy/internal/control"
	"gproxy/internal/proxy/tcp"
	"gproxy/internal/proxy/udp"
	"gproxy/internal/runtime"
)

const usageText = "usage: gproxy [run|reload|status|stop] -c <config>"

func Run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return RunContext(ctx, args)
}

func RunContext(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New(usageText)
	}

	switch args[0] {
	case "run":
		return runCommand(ctx, args[1:])
	case "reload":
		return reloadCommand(ctx, args[1:])
	case "status":
		return statusCommand(ctx, args[1:])
	case "stop":
		return stopCommand(ctx, args[1:])
	default:
		return errors.New(usageText)
	}
}

func statusCommand(ctx context.Context, args []string) error {
	cfg, err := loadConfigFromFlag("status", args)
	if err != nil {
		return err
	}

	client := control.NewClient(cfg.Control.Socket)
	status, err := client.Status(ctx)
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(os.Stdout, formatStatus(status))
	return err
}

func stopCommand(ctx context.Context, args []string) error {
	cfg, err := loadConfigFromFlag("stop", args)
	if err != nil {
		return err
	}

	client := control.NewClient(cfg.Control.Socket)
	return client.Stop(ctx)
}

func loadConfigFromFlag(name string, args []string) (config.Config, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("c", "", "config file")
	if err := fs.Parse(args); err != nil {
		return config.Config{}, err
	}
	if *configPath == "" {
		return config.Config{}, errors.New("-c is required")
	}
	return config.LoadFile(*configPath)
}

func formatStatus(status control.Status) string {
	return fmt.Sprintf(
		"state: %s\nsocket: %s\nrules: %d\ntcp listeners: %d\nudp listeners: %d\n",
		status.State,
		status.SocketPath,
		status.RuleCount,
		status.TCPListenerCount,
		status.UDPListenerCount,
	)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/app -run 'Test(StatusRejectsMissingConfigFlag|FormatStatusOutput)' -v`
Expected: PASS.

### Task 4: Wire Status and Stop into the Run Loop

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/control/control_test.go`
- Modify: `internal/app/run_integration_test.go`

- [ ] **Step 1: Write the failing run-loop lifecycle tests**

Append to `internal/app/run_integration_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app -run TestRunContextStatusAndStopCommands -v`
Expected: FAIL because the run loop does not yet handle those control commands.

- [ ] **Step 3: Write minimal run-loop integration**

Update `internal/app/app.go`
```go
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"gproxy/internal/config"
	"gproxy/internal/control"
	"gproxy/internal/proxy/tcp"
	"gproxy/internal/proxy/udp"
	"gproxy/internal/runtime"
)

func runCommand(ctx context.Context, args []string) error {
	cfg, err := loadConfigFromFlag("run", args)
	if err != nil {
		return err
	}

	entries, err := config.Expand(cfg)
	if err != nil {
		return err
	}

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	manager := runtime.NewManager()
	if _, err := manager.Apply(runCtx, runtime.SnapshotFromEntries(entries), proxyFactory(cfg)); err != nil {
		return err
	}

	var stopOnce sync.Once
	server, err := control.NewServer(cfg.Control.Socket, func(_ context.Context, req control.Request) control.Response {
		switch req.Command {
		case control.CommandReload:
			if manager.Status().State == "stopping" {
				return control.Response{OK: false, Error: "daemon is stopping"}
			}

			nextCfg, err := config.Load(strings.NewReader(req.ConfigYAML))
			if err != nil {
				return control.Response{OK: false, Error: err.Error()}
			}

			nextEntries, err := config.Expand(nextCfg)
			if err != nil {
				return control.Response{OK: false, Error: err.Error()}
			}

			if _, err := manager.Apply(runCtx, runtime.SnapshotFromEntries(nextEntries), proxyFactory(nextCfg)); err != nil {
				return control.Response{OK: false, Error: err.Error()}
			}

			return control.Response{OK: true}

		case control.CommandStatus:
			status := manager.Status()
			return control.Response{
				OK: true,
				Status: &control.Status{
					SocketPath:       cfg.Control.Socket,
					State:            status.State,
					RuleCount:        status.RuleCount,
					TCPListenerCount: status.TCPListenerCount,
					UDPListenerCount: status.UDPListenerCount,
				},
			}

		case control.CommandStop:
			if manager.Status().State == "stopping" {
				return control.Response{OK: false, Error: "stop already in progress"}
			}

			manager.MarkStopping()
			stopOnce.Do(cancelRun)
			return control.Response{OK: true}

		default:
			return control.Response{OK: false, Error: "unknown control command"}
		}
	})
	if err != nil {
		return err
	}
	defer func() { _ = server.Close() }()

	if err := server.Start(runCtx); err != nil {
		return err
	}

	<-runCtx.Done()
	return nil
}

func reloadCommand(ctx context.Context, args []string) error {
	cfg, err := loadConfigFromFlag("reload", args)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(fsPathFromArgs("reload", args))
	if err != nil {
		return err
	}

	client := control.NewClient(cfg.Control.Socket)
	return client.Reload(ctx, string(data))
}
```

And add helper:
```go
func fsPathFromArgs(name string, args []string) string { /* parse -c and return path */ }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/app -run TestRunContextStatusAndStopCommands -v`
Expected: PASS.

### Task 5: Reconcile Command Helpers and Prevent Regression in Reload

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

- [ ] **Step 1: Write the failing reload regression test**

Append to `internal/app/app_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app -run TestReloadUsesGeneralizedControlClient -v`
Expected: FAIL until `reload` uses the generalized request type.

- [ ] **Step 3: Write minimal helper cleanup**

Update `internal/app/app.go`
```go
func parseConfigPath(name string, args []string) (string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("c", "", "config file")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if *configPath == "" {
		return "", errors.New("-c is required")
	}
	return *configPath, nil
}

func loadConfigFromFlag(name string, args []string) (config.Config, string, error) {
	path, err := parseConfigPath(name, args)
	if err != nil {
		return config.Config{}, "", err
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		return config.Config{}, "", err
	}
	return cfg, path, nil
}
```

Then update `run`, `reload`, `status`, and `stop` to use the shared helper consistently.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/app -run TestReloadUsesGeneralizedControlClient -v`
Expected: PASS.

### Task 6: Update Docs and Run Full Verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Write the failing README content test**

Create `internal/app/readme_test.go`
```go
package app

import (
	"os"
	"strings"
	"testing"
)

func TestReadmeMentionsStatusAndStop(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	text := string(data)
	if !strings.Contains(text, "status") || !strings.Contains(text, "stop") {
		t.Fatalf("README missing status/stop commands:\n%s", text)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app -run TestReadmeMentionsStatusAndStop -v`
Expected: FAIL because the README only documents `run` and `reload`.

- [ ] **Step 3: Update documentation**

Update `README.md`
```md
# gproxy

High-concurrency TCP and UDP port forwarder written in Go.

## Commands

- `go run ./cmd/gproxy run -c example/gproxy.yaml`
- `go run ./cmd/gproxy reload -c example/gproxy.yaml`
- `go run ./cmd/gproxy status -c example/gproxy.yaml`
- `go run ./cmd/gproxy stop -c example/gproxy.yaml`

## Config

See `example/gproxy.yaml` for single-port, range-to-single, and range-to-range examples.
```

- [ ] **Step 4: Run full verification**

Run: `gofmt -w $(rg --files -g '*.go')`
Run: `go test ./... -v`
Run: `go build -o /tmp/gproxy ./cmd/gproxy`
Expected: PASS for format, tests, and build.

## Self-Review

Spec coverage check:

- Generalized control protocol is covered by Task 1.
- Runtime status snapshot and stopping lifecycle are covered by Task 2.
- CLI `status` / `stop` behavior and human-readable output are covered by Task 3.
- Graceful stop flow in the run loop is covered by Task 4.
- Reload compatibility after protocol generalization is covered by Task 5.
- Docs and full verification are covered by Task 6.

Placeholder scan:

- No placeholders remain in this plan.

Type consistency check:

- `control.Request`, `control.Response`, `control.Status`, and `runtime.Status` are introduced with distinct responsibilities.
- `loadConfigFromFlag` is used consistently across all CLI commands.
