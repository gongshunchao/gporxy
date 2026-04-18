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

func runCommand(ctx context.Context, args []string) error {
	cfg, _, err := loadConfigFromFlag("run", args)
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
			manager.StopAccepting()
			go func() {
				timer := time.NewTimer(stopGracePeriod(cfg))
				defer timer.Stop()

				select {
				case <-timer.C:
				case <-ctx.Done():
				}

				manager.CloseAll()
				cancelRun()
			}()
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
	manager.CloseAll()
	return nil
}

func reloadCommand(ctx context.Context, args []string) error {
	cfg, path, err := loadConfigFromFlag("reload", args)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	client := control.NewClient(cfg.Control.Socket)
	return client.Reload(ctx, string(data))
}

func statusCommand(ctx context.Context, args []string) error {
	cfg, _, err := loadConfigFromFlag("status", args)
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
	cfg, _, err := loadConfigFromFlag("stop", args)
	if err != nil {
		return err
	}

	client := control.NewClient(cfg.Control.Socket)
	return client.Stop(ctx)
}

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

func proxyFactory(cfg config.Config) runtime.Factory {
	idleTimeout := cfg.Control.UDPSessionIdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = 30 * time.Second
	}

	return func(_ context.Context, item runtime.Item) (runtime.Starter, error) {
		switch item.Entry.Protocol {
		case "tcp":
			return tcp.New(item.Entry)
		case "udp":
			return udp.New(item.Entry, idleTimeout)
		default:
			return nil, errors.New("unsupported protocol")
		}
	}
}

func stopGracePeriod(cfg config.Config) time.Duration {
	if cfg.Control.UDPSessionIdleTimeout > 0 {
		return cfg.Control.UDPSessionIdleTimeout
	}
	return 30 * time.Second
}
