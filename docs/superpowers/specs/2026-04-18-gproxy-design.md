# Gproxy Design Spec

## Summary

`gproxy` is a Go-based high-concurrency port forwarding tool for standard TCP and UDP proxying. It supports single-port forwarding, port-range-to-single-port forwarding, and one-to-one port-range forwarding. Configuration is defined in YAML and can be reloaded through a management command.

The first version is optimized for predictable latency and concurrency stability rather than rich control-plane features. The implementation avoids unnecessary abstractions in the data path, expands port ranges at config-load time, and applies incremental reloads to minimize connection churn.

## Goals

- Implement TCP and UDP port forwarding in Go.
- Support `host:port -> host:port`.
- Support `host:port-range -> host:port`.
- Support `host:port-range -> host:port-range`.
- Use YAML as the configuration format.
- Provide a command-triggered reload flow.
- Keep added latency within a few milliseconds while prioritizing high-concurrency stability.
- Preserve existing active traffic when a reload does not require those flows to be terminated.

## Non-Goals

- No reverse tunnel or NAT traversal behavior.
- No HTTP control plane in v1.
- No ACL, authentication, rate limiting, or metrics endpoint in v1.
- No protocol-aware proxying beyond raw TCP and UDP forwarding.
- No automatic file-watch reload in v1.

## User-Facing CLI

Two commands are included in the first version:

```bash
gproxy run -c config.yaml
gproxy reload -c config.yaml
```

`run` starts the forwarding process and the local control endpoint. `reload` parses the given YAML file and sends the new configuration to the running process.

## Configuration Model

The YAML file contains a local control section and a list of forwarding rules.

Example:

```yaml
control:
  socket: /tmp/gproxy.sock
  udp_session_idle_timeout: 30s

rules:
  - name: tcp-range-1to1
    protocol: tcp
    listen: 0.0.0.0:10000-10010
    target: 10.0.0.2:20000-20010

  - name: udp-range-fan-in
    protocol: udp
    listen: 0.0.0.0:30000-30010
    target: 10.0.0.3:40000

  - name: tcp-single
    protocol: tcp
    listen: 127.0.0.1:8080
    target: 192.168.1.20:80
```

## Address Grammar

The `listen` and `target` fields use one of these forms:

- `host:port`
- `host:start-end`

Examples:

- `0.0.0.0:8080`
- `127.0.0.1:10000-10010`
- `example.internal:443`

The host is always explicit. There is no syntax for omitting the host in v1.

## Supported Mapping Semantics

The proxy supports exactly these combinations:

1. Single to single
   - `A:p -> B:q`
2. Range to single
   - `A:p1-p2 -> B:q`
3. Range to range
   - `A:p1-p2 -> B:q1-q2`

Semantics:

- Single to single forwards one listening port to one target port.
- Range to single forwards every source port in the range to the same target port.
- Range to range is one-to-one by offset.

Example of one-to-one offset mapping:

- `0.0.0.0:10000-10002 -> 10.0.0.2:20000-20002`
- Expanded result:
  - `10000 -> 20000`
  - `10001 -> 20001`
  - `10002 -> 20002`

## Validation Rules

Configuration load fails if any of the following is true:

- `protocol` is not `tcp` or `udp`
- `listen` is malformed
- `target` is malformed
- A port value is outside `1-65535`
- A range start is greater than its end
- `listen` is a single port and `target` is a range
- Both sides are ranges but the sizes differ
- Two expanded rules conflict on the same `protocol + listen host + listen port`
- A rule name is duplicated
- The control socket path is empty

Validation happens before any runtime state is changed. A failed reload leaves the current running configuration untouched.

## Internal Runtime Model

At startup or reload:

1. Parse YAML into config structs.
2. Validate all rules.
3. Expand range rules into concrete single-port forwarding entries.
4. Build an immutable runtime config snapshot.
5. Diff the new snapshot against the currently active snapshot.
6. Start newly required listeners.
7. Stop removed listeners.
8. Keep unchanged listeners and their live traffic untouched.

The data plane only handles concrete single-port forwarding entries. No port-range parsing is allowed on the hot path.

## Proposed Package Layout

```text
cmd/gproxy
internal/config
internal/control
internal/proxy
internal/proxy/tcp
internal/proxy/udp
internal/runtime
```

Responsibilities:

- `cmd/gproxy`: CLI entrypoint and subcommands
- `internal/config`: YAML parsing, address/range parsing, validation, expansion
- `internal/control`: local Unix socket control protocol for reload
- `internal/runtime`: active config snapshot and diff/apply logic
- `internal/proxy/tcp`: TCP listeners and connection bridging
- `internal/proxy/udp`: UDP listeners, per-client session tracking, cleanup

## Control Plane Design

The running process exposes a local Unix socket defined by `control.socket`, for example `/tmp/gproxy.sock`.

`gproxy reload -c config.yaml` performs the following:

1. Read and parse the YAML file.
2. Validate and expand it locally.
3. Serialize the normalized config snapshot.
4. Connect to the Unix socket.
5. Send a reload request with the full new config.
6. Receive a success or failure response.

This keeps reload deterministic and avoids giving the running process a second responsibility of reading arbitrary files on demand.

The control plane is local-only in v1. No TCP, HTTP, or remote admin endpoint is exposed.

## TCP Data Plane

Each expanded TCP forwarding entry owns one `net.Listener`.

Accept loop behavior:

1. Accept inbound client connection.
2. Dial target TCP address.
3. Start bidirectional relay between client and target.
4. Close both sides when copy finishes or a fatal error occurs.

Implementation requirements:

- Use `io.CopyBuffer` in both directions.
- Use a shared `sync.Pool` for relay buffers.
- Avoid wrapping connections in extra layers unless required for correctness.
- Keep per-connection state minimal.

Reload behavior for TCP:

- If a rule is unchanged, the existing listener remains.
- If a rule is removed, the listener stops accepting new connections.
- Existing active bridged connections continue until they end naturally.
- If a rule changes, treat it as remove old plus add new.

## UDP Data Plane

Each expanded UDP forwarding entry owns one long-lived `net.PacketConn` for inbound traffic.

Because UDP is connectionless, the proxy maintains a per-client session table keyed by client source address. Each session contains:

- the original client address
- a dedicated outbound `UDPConn` connected to the target
- last activity timestamp

Session flow:

1. A datagram arrives from client `C`.
2. Look up session for `C`.
3. If no session exists, create one connected to the configured target.
4. Forward the datagram to the target through the session socket.
5. A dedicated read loop on the session socket forwards target responses back to client `C`.
6. If the session is idle past `udp_session_idle_timeout`, close and remove it.

Implementation requirements:

- Use sharded session maps to reduce lock contention.
- Reuse read buffers where practical.
- Avoid per-packet allocations beyond what Go networking APIs require.
- Bound session lifetime with idle timeout cleanup.

Reload behavior for UDP:

- If a rule is unchanged, keep the existing listener and active sessions.
- If a rule is removed, stop accepting new packets on that listener.
- Existing sessions may drain naturally until idle timeout.
- If a rule changes, treat it as remove old plus add new.

This preserves stability while avoiding abrupt interruption of active client flows.

## Performance Strategy

The primary performance target is stable high concurrency with low operational jitter. The design choices for v1 are:

- Pre-expand all range rules during config load.
- Use O(1) forwarding entry lookup by concrete listener identity.
- Reuse buffers for TCP relay.
- Keep UDP listeners long-lived.
- Use sharded locking for UDP session tables.
- Apply incremental reloads instead of full restart.
- Avoid deep abstraction layers in the data plane.

The explicit tradeoff is that very large configured ranges create many listeners. This is acceptable in v1 because it keeps the hot path simple and predictable.

## Error Handling

Startup errors:

- Invalid config
- Control socket bind failure
- Listener bind failure

Startup should fail fast and return a non-zero exit code if required listeners cannot be established.

Reload errors:

- Invalid replacement config
- Control socket unavailable
- Any failure while preparing the new snapshot

The running process must reject the reload and continue operating on the previous snapshot if the new config cannot be applied safely.

Per-connection and per-session errors:

- TCP relay copy errors are treated as connection termination events.
- UDP send/read errors terminate the affected session, not the whole listener.
- Unexpected runtime errors should be logged with rule identity and peer address when available.

## Logging

Logging in v1 should be structured enough to support debugging without adding significant overhead.

Minimum fields:

- rule name
- protocol
- listen address
- target address
- error message

Log categories:

- startup
- reload
- listener lifecycle
- configuration validation failures
- fatal per-connection or per-session errors

Per-packet logging is explicitly out of scope.

## Testing Strategy

The implementation should include both unit tests and integration tests.

Unit tests:

- parse single-port address
- parse range address
- reject malformed addresses
- reject invalid ranges
- expand range-to-single correctly
- expand range-to-range correctly
- reject mismatched range lengths
- detect duplicate listener conflicts

TCP integration tests:

- single-port forwarding
- range-to-single forwarding
- range-to-range forwarding
- removed listener stops new connections after reload
- unchanged listener keeps established connections across reload

UDP integration tests:

- single-port request-response forwarding
- range-to-single forwarding
- range-to-range forwarding
- sessions isolated by client address
- idle sessions cleaned up after timeout
- reload preserves unchanged listener behavior

Control plane tests:

- reload command reaches running daemon
- invalid reload leaves old config active
- socket path mismatch returns clear error

## Implementation Constraints

- Use Go standard library wherever possible.
- Prefer straightforward concurrency patterns over clever abstractions.
- Keep the codebase small and focused.
- Do not optimize with unsafe or platform-specific syscalls in v1.
- Preserve a path for later extension with `status` and `stop` management commands.

## Future Extensions

Possible follow-up features after v1:

- `status` command
- `stop` command
- metrics export
- listener-level connection limits
- ACLs
- optional HTTP control plane

These are intentionally excluded from the first version to keep the implementation centered on correctness, reload safety, and stable performance.
