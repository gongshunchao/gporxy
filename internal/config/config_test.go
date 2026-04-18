package config

import (
	"strings"
	"testing"
)

func TestParseEndpointRangeSinglePort(t *testing.T) {
	got, err := ParseEndpointRange("127.0.0.1:8080")
	if err != nil {
		t.Fatalf("ParseEndpointRange returned error: %v", err)
	}

	if got.Host != "127.0.0.1" || got.StartPort != 8080 || got.EndPort != 8080 {
		t.Fatalf("unexpected endpoint range: %+v", got)
	}
}

func TestParseEndpointRangePortRange(t *testing.T) {
	got, err := ParseEndpointRange("0.0.0.0:10000-10002")
	if err != nil {
		t.Fatalf("ParseEndpointRange returned error: %v", err)
	}

	if got.Host != "0.0.0.0" || got.StartPort != 10000 || got.EndPort != 10002 {
		t.Fatalf("unexpected endpoint range: %+v", got)
	}
}

func TestParseEndpointRangeRejectsMalformedValue(t *testing.T) {
	_, err := ParseEndpointRange("127.0.0.1")
	if err == nil {
		t.Fatal("expected malformed endpoint error")
	}
}

func TestLoadParsesYAML(t *testing.T) {
	src := `
control:
  socket: /tmp/gproxy.sock
  udp_session_idle_timeout: 30s
rules:
  - name: tcp-single
    protocol: tcp
    listen: 127.0.0.1:8080
    target: 10.0.0.2:80
`

	cfg, err := Load(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Control.Socket != "/tmp/gproxy.sock" {
		t.Fatalf("unexpected socket: %s", cfg.Control.Socket)
	}

	if len(cfg.Rules) != 1 || cfg.Rules[0].Name != "tcp-single" {
		t.Fatalf("unexpected rules: %+v", cfg.Rules)
	}
}

func TestValidateRejectsMismatchedRangeLengths(t *testing.T) {
	cfg := Config{
		Control: Control{Socket: "/tmp/gproxy.sock"},
		Rules: []Rule{
			{
				Name:     "bad",
				Protocol: "tcp",
				Listen:   "0.0.0.0:10000-10002",
				Target:   "10.0.0.2:20000-20001",
			},
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected range length validation error")
	}
}

func TestExpandRangeToSingle(t *testing.T) {
	cfg := Config{
		Control: Control{Socket: "/tmp/gproxy.sock"},
		Rules: []Rule{
			{
				Name:     "fan-in",
				Protocol: "udp",
				Listen:   "0.0.0.0:30000-30002",
				Target:   "10.0.0.3:40000",
			},
		},
	}

	entries, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand returned error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].Listen != "0.0.0.0:30000" || entries[2].Target != "10.0.0.3:40000" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestExpandRangeToRange(t *testing.T) {
	cfg := Config{
		Control: Control{Socket: "/tmp/gproxy.sock"},
		Rules: []Rule{
			{
				Name:     "pair",
				Protocol: "tcp",
				Listen:   "0.0.0.0:10000-10002",
				Target:   "10.0.0.2:20000-20002",
			},
		},
	}

	entries, err := Expand(cfg)
	if err != nil {
		t.Fatalf("Expand returned error: %v", err)
	}

	want := []Entry{
		{Name: "pair", Protocol: "tcp", Listen: "0.0.0.0:10000", Target: "10.0.0.2:20000"},
		{Name: "pair", Protocol: "tcp", Listen: "0.0.0.0:10001", Target: "10.0.0.2:20001"},
		{Name: "pair", Protocol: "tcp", Listen: "0.0.0.0:10002", Target: "10.0.0.2:20002"},
	}

	for i := range want {
		if entries[i] != want[i] {
			t.Fatalf("entry %d mismatch: got %+v want %+v", i, entries[i], want[i])
		}
	}
}

func TestValidateRejectsDuplicateListeners(t *testing.T) {
	cfg := Config{
		Control: Control{Socket: "/tmp/gproxy.sock"},
		Rules: []Rule{
			{Name: "one", Protocol: "tcp", Listen: "0.0.0.0:8080", Target: "10.0.0.2:80"},
			{Name: "two", Protocol: "tcp", Listen: "0.0.0.0:8080", Target: "10.0.0.3:80"},
		},
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected duplicate listener error")
	}
}
