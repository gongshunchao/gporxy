# gproxy

High-concurrency TCP and UDP port forwarder written in Go.

## Commands

- `go run ./cmd/gproxy run -c example/gproxy.yaml`
- `go run ./cmd/gproxy reload -c example/gproxy.yaml`
- `go run ./cmd/gproxy status -c example/gproxy.yaml`
- `go run ./cmd/gproxy stop -c example/gproxy.yaml`

## Config

See `example/gproxy.yaml` for single-port, range-to-single, and range-to-range examples.
