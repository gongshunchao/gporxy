package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestMainWithoutArgsFailsWithUsage(t *testing.T) {
	cmd := exec.Command("go", "run", ".")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected command to fail without args")
	}

	if !strings.Contains(string(output), "usage: gproxy [run|reload] -c <config>") {
		t.Fatalf("unexpected output: %s", string(output))
	}
}
