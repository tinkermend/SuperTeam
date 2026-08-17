package projectcoordination

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
)

func TestSweepAutoRecheckRecoveryGatesReleasesHealedRuntime(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	decisionID := uuid.New()
	ruleID := uuid.New()
	actorID := uuid.New()

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:               demandID,
			TenantID:         tenantID,
			ProjectID:        projectID,
			SourceType:       project.DemandSourceAutomation,
			SubmittedByUserID: actorID,
			SourceRefs: map[string]any{
				"automation_rule_id":                     ruleID.String(),
				project.AutonomyTierSnapshotSourceRefKey: autonomyTierFullAuto,
			},
		},
		projectRecord: project.Project{
			ID:                   projectID,
			TenantID:             tenantID,
			WorkspaceReadyStatus: project.WorkspaceReadyStatusReady,
			CoordinationPolicy:   map[string]any{},
		},
		task: project.ProjectTask{
			ID:        taskID,
			TenantID:  tenantID,
			ProjectID: projectID,
			DemandID:  &demandID,
			Status:    project.ProjectTaskStatusWaitingHuman,
		},
		pendingRecovery: []project.DecisionRequest{{
			ID:             decisionID,
			TenantID:       tenantID,
			ProjectID:      projectID,
			ProjectTaskID:  &taskID,
			DecisionType:   "project_task_runtime_recovery",
			TitleSnapshot:  "执行工作区未就绪",
			StatusSnapshot: "pending",
		}},
	}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithAutomationAutonomyLookup(&stubAutonomyLookup{tier: autonomyTierFullAuto, actor: actorID}).
		WithGateDecisionResolver(resolver)

	n, err := store.SweepAutoRecheckRecoveryGates(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Len(t, resolver.reqs, 1)
	require.Equal(t, "approved", resolver.reqs[0].Decision)
	require.Equal(t, true, resolver.reqs[0].Payload["auto_recheck"])
}

func TestSweepAutoRecheckSkipsWhenWorkspaceStillBroken(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	decisionID := uuid.New()
	ruleID := uuid.New()

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:         demandID,
			TenantID:   tenantID,
			ProjectID:  projectID,
			SourceType: project.DemandSourceAutomation,
			SourceRefs: map[string]any{
				"automation_rule_id":                     ruleID.String(),
				project.AutonomyTierSnapshotSourceRefKey: autonomyTierFullAuto,
			},
		},
		projectRecord: project.Project{
			ID:                   projectID,
			TenantID:             tenantID,
			WorkspaceReadyStatus: project.WorkspaceReadyStatusError,
		},
		task: project.ProjectTask{ID: taskID, DemandID: &demandID, ProjectID: projectID},
		pendingRecovery: []project.DecisionRequest{{
			ID: decisionID, TenantID: tenantID, ProjectID: projectID, ProjectTaskID: &taskID,
			DecisionType: "project_task_runtime_recovery", StatusSnapshot: "pending",
		}},
	}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithAutomationAutonomyLookup(&stubAutonomyLookup{tier: autonomyTierFullAuto, actor: uuid.New()}).
		WithGateDecisionResolver(resolver)

	n, err := store.SweepAutoRecheckRecoveryGates(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Empty(t, resolver.reqs)
}
