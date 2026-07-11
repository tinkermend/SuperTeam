package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestNoopCoordinatorSignalClientTerminateProjectCoordinator(t *testing.T) {
	t.Parallel()

	err := NoopCoordinatorSignalClient{}.TerminateProjectCoordinator(context.Background(), TerminateProjectCoordinatorSignal{
		TenantID:   uuid.New(),
		ProjectID:  uuid.New(),
		WorkflowID: "project-coordinator:test",
		Reason:     "project deleted",
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
