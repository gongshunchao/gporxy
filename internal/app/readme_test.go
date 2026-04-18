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
