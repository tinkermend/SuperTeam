package daemon

import (
	"errors"
	"strings"
)

type Config struct {
	NodeID string
}

type Snapshot struct {
	NodeID string
	Status string
}

type Runner struct {
	nodeID string
}

func NewRunner(config Config) (*Runner, error) {
	nodeID := strings.TrimSpace(config.NodeID)
	if nodeID == "" {
		return nil, errors.New("node id is required")
	}

	return &Runner{nodeID: nodeID}, nil
}

func (runner *Runner) Snapshot() Snapshot {
	return Snapshot{
		NodeID: runner.nodeID,
		Status: "idle",
	}
}

