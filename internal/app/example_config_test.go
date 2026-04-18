package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExampleConfigExists(t *testing.T) {
	path := filepath.Join("..", "..", "example", "gproxy.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected example config to exist: %v", err)
	}
}
