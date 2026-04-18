package runtime

import (
	"testing"

	"gproxy/internal/config"
)

func TestDiffDetectsAddedRemovedAndKeptEntries(t *testing.T) {
	current := SnapshotFromEntries([]config.Entry{
		{Name: "keep", Protocol: "tcp", Listen: "0.0.0.0:8080", Target: "10.0.0.2:80"},
		{Name: "remove", Protocol: "udp", Listen: "0.0.0.0:9000", Target: "10.0.0.3:9000"},
	})

	next := SnapshotFromEntries([]config.Entry{
		{Name: "keep", Protocol: "tcp", Listen: "0.0.0.0:8080", Target: "10.0.0.2:80"},
		{Name: "add", Protocol: "udp", Listen: "0.0.0.0:9001", Target: "10.0.0.4:9001"},
	})

	diff := Diff(current, next)

	if len(diff.Keep) != 1 || diff.Keep[0].Key != "tcp|0.0.0.0:8080|10.0.0.2:80" {
		t.Fatalf("unexpected keep diff: %+v", diff.Keep)
	}

	if len(diff.Remove) != 1 || diff.Remove[0].Key != "udp|0.0.0.0:9000|10.0.0.3:9000" {
		t.Fatalf("unexpected remove diff: %+v", diff.Remove)
	}

	if len(diff.Add) != 1 || diff.Add[0].Key != "udp|0.0.0.0:9001|10.0.0.4:9001" {
		t.Fatalf("unexpected add diff: %+v", diff.Add)
	}
}
