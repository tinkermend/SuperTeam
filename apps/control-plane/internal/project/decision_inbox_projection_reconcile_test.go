package project

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSweepOrphanDecisionInboxProjectionsCancelsStaleTaskLinkedDecision(t *testing.T) {
	repo := newMemoryRepository()
	openInbox := map[uuid.UUID]bool{}
	repo.openInboxDecisionIDs = openInbox
	inbox := &fakeDecisionInboxProjector{openBySource: openInbox}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	taskID := uuid.New()
	decisionID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID, Status: ProjectStatusRunning,
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID: taskID, TenantID: tenantID, ProjectID: projectID,
		Title: "采集数据", Status: ProjectTaskStatusFailed,
	})
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID: decisionID, TenantID: tenantID, ProjectID: projectID,
		ProjectTaskID: &taskID, TargetUserID: ownerID,
		DecisionType: "project_task_runtime_recovery",
		TitleSnapshot: "采集数据", StatusSnapshot: "pending",
	})

	n, err := service.SweepOrphanDecisionInboxProjections(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "cancelled", repo.decisionRequests[0].StatusSnapshot)
	require.Len(t, inbox.resolutions, 1)
	require.Empty(t, inbox.upserts)
}

func TestSweepOrphanDecisionInboxProjectionsReprojectsWaitingHumanDecision(t *testing.T) {
	repo := newMemoryRepository()
	openInbox := map[uuid.UUID]bool{}
	repo.openInboxDecisionIDs = openInbox
	inbox := &fakeDecisionInboxProjector{openBySource: openInbox}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	taskID := uuid.New()
	decisionID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID, Status: ProjectStatusRunning,
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID: taskID, TenantID: tenantID, ProjectID: projectID,
		Title: "等待人工", Status: ProjectTaskStatusWaitingHuman,
	})
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID: decisionID, TenantID: tenantID, ProjectID: projectID,
		ProjectTaskID: &taskID, TargetUserID: ownerID,
		DecisionType: "project_task_clarification",
		TitleSnapshot: "等待人工", StatusSnapshot: "pending",
	})

	n, err := service.SweepOrphanDecisionInboxProjections(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "pending", repo.decisionRequests[0].StatusSnapshot)
	require.Len(t, inbox.upserts, 1)
	require.True(t, openInbox[decisionID])

	// Second sweep: open projection exists → no work.
	n, err = service.SweepOrphanDecisionInboxProjections(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Len(t, inbox.upserts, 1)
}

func TestSweepOrphanDecisionInboxProjectionsCancelsCastingWhenApprovalMissing(t *testing.T) {
	repo := newMemoryRepository()
	openInbox := map[uuid.UUID]bool{}
	repo.openInboxDecisionIDs = openInbox
	inbox := &fakeDecisionInboxProjector{openBySource: openInbox}
	approvals := &fakeApprovalResolver{contextPayloadErr: errors.New("approval gone")}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	demandID := uuid.New()
	eventID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID, Status: ProjectStatusRunning,
	}
	repo.demands = append(repo.demands, ProjectDemand{
		ID: demandID, TenantID: tenantID, ProjectID: projectID,
		Status: ProjectDemandStatusFailed, Title: "failed demand",
	})
	repo.events = append(repo.events, ProjectEvent{
		ID: eventID, TenantID: tenantID, ProjectID: projectID,
		EventType: ProjectEventDecisionRequested,
		Payload:   map[string]any{"demand_id": demandID.String(), "decision_type": DecisionTypeCastingExpansion},
	})
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID: decisionID, TenantID: tenantID, ProjectID: projectID,
		ApprovalRequestID: approvalID, TargetUserID: ownerID,
		DecisionType: DecisionTypeCastingExpansion,
		TitleSnapshot: "扩编请求", StatusSnapshot: "pending",
		CreatedEventID: &eventID,
	})

	n, err := service.SweepOrphanDecisionInboxProjections(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "cancelled", repo.decisionRequests[0].StatusSnapshot)
}

func TestRequestCastingExpansionCompensatesWhenInboxUpsertFails(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{upsertErr: errors.New("inbox down")}
	approvals := &fakeApprovalResolver{
		contextPayloads: map[uuid.UUID]map[string]any{},
	}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	demandID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID, Status: ProjectStatusRunning,
	}
	templateKey := "software_delivery"
	repo.demands = append(repo.demands, ProjectDemand{
		ID: demandID, TenantID: tenantID, ProjectID: projectID,
		Status: ProjectDemandStatusExecuting, Title: "executing",
		ScenarioTemplateKey: &templateKey,
	})

	_, err = service.RequestCastingExpansion(context.Background(), RequestCastingExpansionRequest{
		TenantID:            tenantID,
		ProjectID:           projectID,
		DemandID:            demandID,
		SuggestedRoleKey:    "legal_reviewer",
		Reason:              "need legal",
		ScenarioTemplateKey: templateKey,
		ActorType:           "test",
		ActorID:             "test",
	})
	require.Error(t, err)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "cancelled", repo.decisionRequests[0].StatusSnapshot,
		"failed inbox projection must not leave a pending SoT decision")
}
