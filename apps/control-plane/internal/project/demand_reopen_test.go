package project

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestReopenProjectDemandForReplanning proves the deliberate failed→planning_pending
// exception used by the planning_gap restaffed path: it moves a failed demand back
// into planning_pending (bypassing the forward-only rank guard AdvanceProjectDemandStatus
// enforces) and records a demand.replanning_reopened audit event, while refusing to
// reopen a demand that is not currently failed.
func TestReopenProjectDemandForReplanning(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	repo := newMemoryRepository()
	repo.demands = append(repo.demands, ProjectDemand{
		ID:        demandID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "补员后重开",
		Status:    ProjectDemandStatusFailed,
	})

	updated, err := repo.ReopenProjectDemandForReplanning(context.Background(), tenantID, demandID)
	require.NoError(t, err)
	require.Equal(t, ProjectDemandStatusPlanningPending, updated.Status)
	require.Equal(t, projectID, updated.ProjectID)

	// The demand row itself moved back to planning_pending.
	current, err := repo.GetProjectDemand(context.Background(), tenantID, demandID)
	require.NoError(t, err)
	require.Equal(t, ProjectDemandStatusPlanningPending, current.Status)

	// A demand-scoped audit event marks the reopen.
	var reopened *ProjectEvent
	for i := range repo.events {
		if repo.events[i].EventType == ProjectEventDemandReplanningReopened {
			reopened = &repo.events[i]
		}
	}
	require.NotNil(t, reopened, "expected a demand.replanning_reopened event")
	require.Equal(t, demandID.String(), reopened.Payload["demand_id"])
}

func TestReopenProjectDemandForReplanningRejectsNonFailedDemand(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	repo := newMemoryRepository()
	repo.demands = append(repo.demands, ProjectDemand{
		ID:        demandID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "执行中",
		Status:    ProjectDemandStatusExecuting,
	})

	_, err := repo.ReopenProjectDemandForReplanning(context.Background(), tenantID, demandID)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrProjectConflict))

	current, err := repo.GetProjectDemand(context.Background(), tenantID, demandID)
	require.NoError(t, err)
	require.Equal(t, ProjectDemandStatusExecuting, current.Status)
	require.Empty(t, eventsOfType(repo.events, ProjectEventDemandReplanningReopened))
}

func eventsOfType(events []ProjectEvent, eventType ProjectEventType) []ProjectEvent {
	var result []ProjectEvent
	for _, event := range events {
		if event.EventType == eventType {
			result = append(result, event)
		}
	}
	return result
}
