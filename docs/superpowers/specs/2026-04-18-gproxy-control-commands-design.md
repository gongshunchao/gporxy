# Gproxy Control Commands Design Spec

## Summary

This design extends `gproxy` with two new management commands:

- `gproxy status -c config.yaml`
- `gproxy stop -c config.yaml`

Both commands reuse the existing local Unix socket control plane. `status` returns a human-readable runtime summary. `stop` performs a graceful shutdown: stop accepting new traffic first, allow existing TCP connections and UDP sessions to drain naturally, then exit.

## Goals

- Add `status` and `stop` commands without introducing a second control channel.
- Keep the CLI consistent with the existing `run` and `reload` workflow.
- Return concise human-readable status output.
- Implement graceful stop semantics instead of immediate process termination.
- Preserve the current local-only Unix socket management model.

## Non-Goals

- No JSON output mode in this change.
- No per-rule or per-listener detailed status output in this change.
- No remote admin endpoint.
- No PID-file based management.

## User-Facing CLI

The CLI becomes:

```bash
gproxy run -c config.yaml
gproxy reload -c config.yaml
gproxy status -c config.yaml
gproxy stop -c config.yaml
```

Behavior:

- `status` reads the config file, extracts `control.socket`, queries the running daemon, and prints a concise summary.
- `stop` reads the config file, extracts `control.socket`, sends a graceful stop request, waits for acknowledgement, and exits.

## Control Plane Extension

The current control plane only supports a reload request with one request type. It will be generalized into a command-based request/response protocol carried over the same Unix socket.

Request fields:

- `command`
  - one of `reload`, `status`, `stop`
- `config_yaml`
  - used only by `reload`

Response fields:

- `ok`
- `error`
- `status`
  - populated only for `status`

Status payload fields:

- `socket_path`
- `rule_count`
- `tcp_listener_count`
- `udp_listener_count`
- `state`
  - `running`
  - `stopping`

The control server remains local-only and keeps JSON encoding over Unix sockets.

## Runtime State Model

The daemon needs a lightweight runtime summary that can be queried without traversing deep listener internals.

`runtime.Manager` will maintain:

- current expanded snapshot
- count of active TCP listeners
- count of active UDP listeners
- current lifecycle state
  - `running`
  - `stopping`

These values must be safe to read concurrently from the control-plane handler while listeners are starting, stopping, or being reloaded.

## Status Command Output

The `status` command prints concise human-readable text. Example:

```text
state: running
socket: /tmp/gproxy.sock
rules: 14
tcp listeners: 8
udp listeners: 6
```

If the daemon is unreachable, the command returns an error and a non-zero exit code.

## Stop Semantics

`stop` must be graceful.

Stop sequence:

1. Mark daemon lifecycle state as `stopping`.
2. Stop accepting new control requests except the in-flight `stop` request being processed.
3. Stop accepting new TCP connections by closing listeners.
4. Stop accepting new UDP packets by closing listeners.
5. Allow already-established TCP relays to complete naturally.
6. Allow existing UDP sessions to drain until their normal idle timeout or until shutdown cleanup completes.
7. Exit the process cleanly.

This matches the user's requirement that shutdown should stop new traffic first and let active traffic finish where practical.

## Listener Behavior During Stop

TCP:

- Closing the listener prevents new accepts.
- Existing connection relay goroutines continue until both sides finish or error.

UDP:

- Closing the inbound packet listener prevents new client packets from being accepted.
- Existing per-client outbound session goroutines may continue until their sockets close naturally.
- Shutdown cleanup may close remaining idle or completed sessions after listener shutdown.

## Status Accuracy Rules

The reported counts should reflect the active expanded snapshot, not the original YAML rule count before expansion.

Definitions:

- `rule_count`
  - total expanded forwarding entries currently active in the runtime snapshot
- `tcp_listener_count`
  - active TCP listeners created from that snapshot
- `udp_listener_count`
  - active UDP listeners created from that snapshot

This makes the output operationally meaningful because it reflects real listening endpoints.

## Reload Interaction

Reload semantics remain unchanged:

- unchanged listeners stay alive
- removed listeners stop accepting new traffic
- added listeners start

Status must reflect the post-reload snapshot after a successful reload.

If the daemon is already `stopping`, it should reject `reload` with a clear error instead of attempting another reconfiguration.

## Error Handling

`status` errors:

- socket missing
- daemon not running
- decode failure
- malformed response

`stop` errors:

- socket missing
- daemon not running
- stop already in progress
- control request failure

Errors should be returned to the CLI and printed plainly.

## Testing Scope

This change should add tests for:

- control protocol handling of `status`
- control protocol handling of `stop`
- app command parsing for `status` and `stop`
- human-readable status output formatting
- runtime manager status snapshot reporting
- graceful stop closing listeners while allowing process shutdown

## Implementation Constraints

- Reuse the existing Unix socket control plane.
- Keep the protocol simple and backward-compatible only within this codebase; there is no external compatibility requirement yet.
- Avoid adding heavy observability or metrics systems just to support `status`.
- Keep the status payload intentionally small.
