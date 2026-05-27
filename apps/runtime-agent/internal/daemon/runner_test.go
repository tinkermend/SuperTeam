package daemon

import "testing"

func TestRunnerSnapshotUsesConfiguredNodeID(t *testing.T) {
	runner, err := NewRunner(Config{NodeID: "node-1"})
	if err != nil {
		t.Fatalf("expected runner to be created: %v", err)
	}

	snapshot := runner.Snapshot()

	if snapshot.NodeID != "node-1" {
		t.Fatalf("expected node ID node-1, got %q", snapshot.NodeID)
	}

	if snapshot.Status != "idle" {
		t.Fatalf("expected status idle, got %q", snapshot.Status)
	}
}

func TestRunnerRequiresNodeID(t *testing.T) {
	_, err := NewRunner(Config{})

	if err == nil {
		t.Fatal("expected missing node ID to fail")
	}
}

