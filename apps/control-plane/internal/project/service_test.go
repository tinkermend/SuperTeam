package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	runtimepkg "github.com/superteam/control-plane/internal/runtime"
)

func TestCreateProjectRequiresHumanOwnerAndCreatesEvents(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, runtimeNodeID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "payment-gateway-stability",
		Goal:             "修复超时链路并形成验收报告",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{
			{PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID, ProjectRole: ProjectRoleOwner, DisplayNameSnapshot: "王佩"},
			{PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: employeeID, ProjectRole: ProjectRoleExecutor, DisplayNameSnapshot: "后端执行 A", Settings: map[string]any{"concurrency_slots": float64(2)}},
		},
		RuntimeNodeIDs: []uuid.UUID{runtimeNodeID},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Project.Status != ProjectStatusRunning {
		t.Fatalf("expected running project, got %s", created.Project.Status)
	}
	if created.Project.CoordinationStatus != "registered" {
		t.Fatalf("expected registered coordination status, got %s", created.Project.CoordinationStatus)
	}
	if !strings.HasPrefix(created.Project.CoordinationWorkflowID, "project-coordinator:") {
		t.Fatalf("expected coordination workflow id, got %q", created.Project.CoordinationWorkflowID)
	}
	if repo.eventTypes[0] != ProjectEventCreated || repo.eventTypes[1] != ProjectEventConfigChanged {
		t.Fatalf("expected create/config events, got %#v", repo.eventTypes)
	}
	for _, member := range created.Members {
		if member.ProjectRole == ProjectRole("coordinator") {
			t.Fatal("coordinator must not be represented as a project member")
		}
	}
}

func TestRuntimeWritebackProjectTaskStatusesIncludeQueued(t *testing.T) {
	require.ElementsMatch(t, []string{"assigned", "queued", "running"}, runtimeWritebackProjectTaskStatuses())
	require.True(t, projectTaskAcceptsRuntimeWriteback("queued"))
}

func TestGetProjectRuntimeReadinessReportsMissingPlacement(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "runtime placement readiness",
		Goal:             "dispatch only after runtime placement",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:                  uuid.New(),
		TenantID:            tenantID,
		ProjectID:           projectID,
		PrincipalType:       PrincipalTypeDigitalEmployee,
		PrincipalID:         employeeID,
		ProjectRole:         ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("Codex executor"),
	}}
	service.SetDigitalEmployeePlanningProfileSource(&fakeProjectPlanningProfileSource{
		records: map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{
			employeeID: {ProviderType: "codex"},
		},
	})

	readiness, err := service.GetProjectRuntimeReadiness(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Equal(t, ProjectRuntimePlacementStatusMissing, readiness.PlacementStatus)
	require.Contains(t, readinessBlockingCodes(readiness.BlockingReasons), "runtime_placement_missing")
	require.Contains(t, readinessActionCodes(readiness.NextActions), "bind_runtime")
	require.Equal(t, []string{"codex"}, readiness.RequiredProviderTypes)
	require.Len(t, readiness.EmployeeReadiness, 1)
	require.Equal(t, employeeID, readiness.EmployeeReadiness[0].DigitalEmployeeID)
	require.True(t, readiness.EmployeeReadiness[0].CanPlan)
	require.False(t, readiness.EmployeeReadiness[0].CanDispatch)
}

func TestAddProjectRuntimeNodeRecordsEventAndReadiness(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	actorID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	nodeID := "local-dev-node"
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "runtime placement readiness",
		Goal:             "dispatch through codex runtime",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:                  uuid.New(),
		TenantID:            tenantID,
		ProjectID:           projectID,
		PrincipalType:       PrincipalTypeDigitalEmployee,
		PrincipalID:         employeeID,
		ProjectRole:         ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("Codex executor"),
	}}
	service.SetDigitalEmployeePlanningProfileSource(&fakeProjectPlanningProfileSource{
		records: map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{
			employeeID: {ProviderType: "codex"},
		},
	})
	service.SetProjectRuntimeNodeReader(&fakeProjectRuntimeNodeReader{
		nodes: []runtimepkg.NodeRecord{{
			ID:                 runtimeNodeID,
			TenantID:           tenantID,
			NodeID:             nodeID,
			Name:               "Local Dev Node",
			Status:             string(runtimepkg.NodeStatusOnline),
			MaxSlots:           2,
			CurrentLoad:        1,
			LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
			SupportedProviders: []byte(`["codex"]`),
		}},
		capabilities: map[uuid.UUID][]runtimepkg.RuntimeCapability{
			runtimeNodeID: {{
				ID:            uuid.New(),
				TenantID:      tenantID,
				RuntimeNodeID: runtimeNodeID,
				ProviderType:  "codex",
				Available:     true,
				Status:        "available",
				HealthStatus:  "healthy",
			}},
		},
		connected: map[string]bool{nodeID: true},
	})

	node, err := service.AddProjectRuntimeNode(context.Background(), ModifyProjectRuntimeNodeRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		RuntimeNodeID: runtimeNodeID,
		ActorUserID:   actorID,
		Reason:        "  bind for dispatch  ",
	})
	require.NoError(t, err)
	require.Equal(t, runtimeNodeID, node.RuntimeNodeID)
	require.Equal(t, ProvisionStatusUnprovisioned, node.ProvisionStatus)
	require.Equal(t, ProjectEventRuntimePlacementUpdated, repo.eventTypes[len(repo.eventTypes)-1])

	// Bind-only: no disk supply yet → workspace_pending (spec 2026-08-12 P1).
	readiness, err := service.GetProjectRuntimeReadiness(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Equal(t, ProjectRuntimePlacementStatusWorkspacePending, readiness.PlacementStatus)

	// After admin provision confirm, readiness becomes ready.
	service.SetRuntimeWorkspaceCommander(&recordingWorkspaceCommander{
		connected: true,
		nodes: map[uuid.UUID]struct {
			tenantID uuid.UUID
			nodeID   string
		}{
			runtimeNodeID: {tenantID: tenantID, nodeID: nodeID},
		},
	})
	provisioned, err := service.ProvisionWorkspaceOnNode(context.Background(), ModifyProjectRuntimeNodeRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		RuntimeNodeID: runtimeNodeID,
		ActorUserID:   actorID,
		Reason:        "confirm supply",
	})
	require.NoError(t, err)
	require.Equal(t, ProvisionStatusProvisioned, provisioned.ProvisionStatus)

	readiness, err = service.GetProjectRuntimeReadiness(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Equal(t, ProjectRuntimePlacementStatusReady, readiness.PlacementStatus)
	require.NotNil(t, readiness.RuntimeNodeID)
	require.Equal(t, runtimeNodeID, *readiness.RuntimeNodeID)
	require.True(t, readiness.CommandChannelConnected)
	require.Equal(t, []string{"codex"}, readiness.RequiredProviderTypes)
	require.Contains(t, readiness.ProviderCapabilities, "codex")
	require.Len(t, readiness.EmployeeReadiness, 1)
	require.True(t, readiness.EmployeeReadiness[0].CanPlan)
	require.True(t, readiness.EmployeeReadiness[0].CanDispatch)
	require.Equal(t, "codex", readiness.EmployeeReadiness[0].ProviderType)
}

func TestGetProjectRuntimeReadinessBlocksDispatchForPendingEmployeeFacts(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	nodeID := "local-dev-node"
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "runtime placement readiness",
		Goal:             "block dispatch until employee config is ready",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:                  uuid.New(),
		TenantID:            tenantID,
		ProjectID:           projectID,
		PrincipalType:       PrincipalTypeDigitalEmployee,
		PrincipalID:         employeeID,
		ProjectRole:         ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("Codex executor"),
	}}
	// Readiness is driven by the runtime eligibility set (project_runtime_nodes),
	// not by project_placements — register the same node there too so this
	// fixture represents a project that is actually dispatch-eligible under the
	// new model, not just one with a legacy placement row.
	if _, err := repo.InsertProjectRuntimeNode(context.Background(), tenantID, projectID, runtimeNodeID, true, "create"); err != nil {
		t.Fatalf("insert project runtime node: %v", err)
	}
	service.SetDigitalEmployeePlanningProfileSource(&fakeProjectPlanningProfileSource{
		records: map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{
			employeeID: {
				DigitalEmployeeID: employeeID,
				ProviderType:      "codex",
				ExecutionStatus:   "unavailable",
			},
		},
	})
	service.SetProjectRuntimeNodeReader(&fakeProjectRuntimeNodeReader{
		nodes: []runtimepkg.NodeRecord{{
			ID:                 runtimeNodeID,
			TenantID:           tenantID,
			NodeID:             nodeID,
			Name:               "Local Dev Node",
			Status:             string(runtimepkg.NodeStatusOnline),
			MaxSlots:           2,
			CurrentLoad:        0,
			LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
			SupportedProviders: []byte(`["codex"]`),
		}},
		capabilities: map[uuid.UUID][]runtimepkg.RuntimeCapability{
			runtimeNodeID: {{
				ID:            uuid.New(),
				TenantID:      tenantID,
				RuntimeNodeID: runtimeNodeID,
				ProviderType:  "codex",
				Available:     true,
				Status:        "available",
				HealthStatus:  "healthy",
			}},
		},
		connected: map[string]bool{nodeID: true},
	})

	readiness, err := service.GetProjectRuntimeReadiness(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Equal(t, ProjectRuntimePlacementStatusWorkspacePending, readiness.PlacementStatus)
	require.Contains(t, readinessBlockingCodes(readiness.BlockingReasons), "employee_workspace_pending")
	require.Contains(t, readinessActionCodes(readiness.NextActions), "prepare_employee_workspace")
	require.Len(t, readiness.EmployeeReadiness, 1)
	require.Equal(t, employeeID, readiness.EmployeeReadiness[0].DigitalEmployeeID)
	require.True(t, readiness.EmployeeReadiness[0].CanPlan)
	require.False(t, readiness.EmployeeReadiness[0].CanDispatch)
	require.Equal(t, "employee_workspace_pending", readiness.EmployeeReadiness[0].ReasonCode)
	require.Equal(t, "employee execution workspace or provider is not ready", readiness.EmployeeReadiness[0].ReasonMessage)
}

func TestGetProjectRuntimeReadinessBlocksProviderTypeMissingEmployee(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	nodeID := "local-dev-node"
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "runtime placement readiness",
		Goal:             "block readiness when employee provider is missing",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:                  uuid.New(),
		TenantID:            tenantID,
		ProjectID:           projectID,
		PrincipalType:       PrincipalTypeDigitalEmployee,
		PrincipalID:         employeeID,
		ProjectRole:         ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("No provider executor"),
	}}
	// Readiness is driven by the runtime eligibility set (project_runtime_nodes),
	// not by project_placements — register the same node there too so this
	// fixture represents a project that is actually dispatch-eligible under the
	// new model, not just one with a legacy placement row.
	if _, err := repo.InsertProjectRuntimeNode(context.Background(), tenantID, projectID, runtimeNodeID, true, "create"); err != nil {
		t.Fatalf("insert project runtime node: %v", err)
	}
	service.SetDigitalEmployeePlanningProfileSource(&fakeProjectPlanningProfileSource{
		records: map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{
			employeeID: {DigitalEmployeeID: employeeID},
		},
	})
	service.SetProjectRuntimeNodeReader(&fakeProjectRuntimeNodeReader{
		nodes: []runtimepkg.NodeRecord{{
			ID:                 runtimeNodeID,
			TenantID:           tenantID,
			NodeID:             nodeID,
			Name:               "Local Dev Node",
			Status:             string(runtimepkg.NodeStatusOnline),
			MaxSlots:           2,
			CurrentLoad:        0,
			LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
			SupportedProviders: []byte(`["codex"]`),
		}},
		capabilities: map[uuid.UUID][]runtimepkg.RuntimeCapability{},
		connected:    map[string]bool{nodeID: true},
	})

	readiness, err := service.GetProjectRuntimeReadiness(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Equal(t, ProjectRuntimePlacementStatusWorkspacePending, readiness.PlacementStatus)
	require.Contains(t, readinessBlockingCodes(readiness.BlockingReasons), "provider_type_missing")
	require.Contains(t, readinessActionCodes(readiness.NextActions), "configure_employee_provider")
	require.Len(t, readiness.EmployeeReadiness, 1)
	require.False(t, readiness.EmployeeReadiness[0].CanPlan)
	require.False(t, readiness.EmployeeReadiness[0].CanDispatch)
	require.Equal(t, "provider_type_missing", readiness.EmployeeReadiness[0].ReasonCode)
}

func TestAvailableProviderCapabilitiesAcceptsHealthyRuntimeStatus(t *testing.T) {
	capabilities := []runtimepkg.RuntimeCapability{
		{
			ProviderType: "codex",
			Available:    true,
			Status:       "healthy",
			HealthStatus: "healthy",
		},
		{
			ProviderType: "opencode",
			Available:    true,
			Status:       "unavailable",
			HealthStatus: "healthy",
		},
		{
			ProviderType: "claude-code",
			Available:    true,
			Status:       "available",
			HealthStatus: "unhealthy",
		},
		{
			ProviderType: "pi",
			Available:    false,
			Status:       "healthy",
			HealthStatus: "healthy",
		},
	}

	require.Equal(t, []string{"codex"}, availableProviderCapabilities(capabilities, nil))
}

func TestGetProjectRuntimeReadinessDoesNotFallbackOverExplicitUnavailableCapability(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	nodeID := "local-dev-node"
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "runtime placement readiness",
		Goal:             "respect explicit provider capability health",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:                  uuid.New(),
		TenantID:            tenantID,
		ProjectID:           projectID,
		PrincipalType:       PrincipalTypeDigitalEmployee,
		PrincipalID:         employeeID,
		ProjectRole:         ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("Codex executor"),
	}}
	// Readiness is driven by the runtime eligibility set (project_runtime_nodes),
	// not by project_placements — register the same node there too so this
	// fixture represents a project that is actually dispatch-eligible under the
	// new model, not just one with a legacy placement row.
	if _, err := repo.InsertProjectRuntimeNode(context.Background(), tenantID, projectID, runtimeNodeID, true, "create"); err != nil {
		t.Fatalf("insert project runtime node: %v", err)
	}
	service.SetDigitalEmployeePlanningProfileSource(&fakeProjectPlanningProfileSource{
		records: map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{
			employeeID: {DigitalEmployeeID: employeeID, ProviderType: "codex"},
		},
	})
	service.SetProjectRuntimeNodeReader(&fakeProjectRuntimeNodeReader{
		nodes: []runtimepkg.NodeRecord{{
			ID:                 runtimeNodeID,
			TenantID:           tenantID,
			NodeID:             nodeID,
			Name:               "Local Dev Node",
			Status:             string(runtimepkg.NodeStatusOnline),
			MaxSlots:           2,
			CurrentLoad:        0,
			LastHeartbeatAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
			SupportedProviders: []byte(`["codex"]`),
		}},
		capabilities: map[uuid.UUID][]runtimepkg.RuntimeCapability{
			runtimeNodeID: {{
				ID:            uuid.New(),
				TenantID:      tenantID,
				RuntimeNodeID: runtimeNodeID,
				ProviderType:  "codex",
				Available:     false,
				Status:        "unavailable",
				HealthStatus:  "unhealthy",
			}},
		},
		connected: map[string]bool{nodeID: true},
	})

	readiness, err := service.GetProjectRuntimeReadiness(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Equal(t, ProjectRuntimePlacementStatusProviderUnavailable, readiness.PlacementStatus)
	require.NotContains(t, readiness.ProviderCapabilities, "codex")
	require.Contains(t, readinessBlockingCodes(readiness.BlockingReasons), "provider_unavailable")
	require.Len(t, readiness.EmployeeReadiness, 1)
	require.False(t, readiness.EmployeeReadiness[0].CanDispatch)
	require.Equal(t, "provider_unavailable", readiness.EmployeeReadiness[0].ReasonCode)
}

func TestAddProjectRuntimeNodeRejectsRuntimeNodeOutsideTenant(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	foreignTenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	actorID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "runtime placement readiness",
		Goal:             "reject foreign runtime placement",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	service.SetProjectRuntimeNodeReader(&fakeProjectRuntimeNodeReader{
		nodes: []runtimepkg.NodeRecord{{
			ID:       runtimeNodeID,
			TenantID: foreignTenantID,
			NodeID:   "foreign-node",
			Name:     "Foreign Node",
			Status:   string(runtimepkg.NodeStatusOnline),
		}},
	})

	node, err := service.AddProjectRuntimeNode(context.Background(), ModifyProjectRuntimeNodeRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		RuntimeNodeID: runtimeNodeID,
		ActorUserID:   actorID,
		Reason:        "foreign node",
	})

	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Nil(t, node)
	require.Empty(t, repo.projectRuntimeNodes)
	require.NotContains(t, repo.eventTypes, ProjectEventRuntimePlacementUpdated)
}

func TestGetExecutionTraceGroupsEventsByAttempt(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	summaryID := uuid.New()
	now := time.Now().UTC()
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "核对证据链",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:            attemptID,
		TenantID:      tenantID,
		ProjectTaskID: taskID,
		AttemptNo:     1,
		Status:        ProjectTaskAttemptStatusSucceeded,
		StartedAt:     &now,
		FinishedAt:    &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "证据链完整",
		EvidenceRefs:        []any{map[string]any{"ref": "evidence://1"}},
		ArtifactRefs:        []any{map[string]any{"ref": "artifact://1"}},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: false,
		CreatedAt:           now,
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents, ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        &taskID,
		ProjectTaskAttemptID: &attemptID,
		EventType:            ExecutionLedgerEventAttemptStarted,
		SourceType:           "project_task_attempt",
		SourceID:             attemptID.String(),
		ActorType:            "runtime_node",
		InputSummary:         strPtr("Runtime started attempt"),
		OccurredAt:           now,
		CreatedAt:            now,
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents, ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        &taskID,
		ProjectTaskAttemptID: &attemptID,
		EventType:            ExecutionLedgerEventSummaryCreated,
		SourceType:           "project_execution_summary",
		SourceID:             summaryID.String(),
		ActorType:            "system",
		OutputSummary:        strPtr("证据链完整"),
		OccurredAt:           now.Add(time.Second),
		CreatedAt:            now.Add(time.Second),
	})

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Equal(t, projectID, trace.ProjectID)
	require.Equal(t, int32(1), trace.Summary.AttemptCount)
	require.Equal(t, int32(1), trace.Summary.ArtifactRefCount)
	require.Equal(t, int32(1), trace.Summary.EvidenceRefCount)
	require.Len(t, trace.Attempts, 1)
	require.Equal(t, attemptID, trace.Attempts[0].AttemptID)
	require.Len(t, trace.Attempts[0].Events, 2)
	require.NotNil(t, trace.Attempts[0].Summary)
	require.Equal(t, summaryID, trace.Attempts[0].Summary.ExecutionSummaryID)
}

func TestGetExecutionTraceProjectsSessionResumeFromAttemptPacket(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	now := time.Now().UTC()
	status := "skipped"
	label := "已开新会话 · 原会话过期"
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "接续任务",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:            attemptID,
		TenantID:      tenantID,
		ProjectTaskID: taskID,
		AttemptNo:     1,
		Status:        ProjectTaskAttemptStatusSucceeded,
		StartedAt:     &now,
		FinishedAt:    &now,
		CreatedAt:     now,
		UpdatedAt:     now,
		ExecutionContextPacket: map[string]any{
			"session_resume_status": status,
			"session_resume_label":  label,
		},
	})

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Len(t, trace.Attempts, 1)
	require.NotNil(t, trace.Attempts[0].SessionResumeStatus)
	require.Equal(t, status, *trace.Attempts[0].SessionResumeStatus)
	require.NotNil(t, trace.Attempts[0].SessionResumeLabel)
	require.Equal(t, label, *trace.Attempts[0].SessionResumeLabel)
}

func TestSessionResumeFieldsFromExecutionContext(t *testing.T) {
	status, label := sessionResumeFieldsFromExecutionContext(nil)
	require.Nil(t, status)
	require.Nil(t, label)

	status, label = sessionResumeFieldsFromExecutionContext(map[string]any{
		"session_resume_status": "  resumed  ",
		"session_resume_label":  " 已接上上次会话 ",
	})
	require.NotNil(t, status)
	require.Equal(t, "resumed", *status)
	require.NotNil(t, label)
	require.Equal(t, "已接上上次会话", *label)
}

func TestGetExecutionTraceAttachesCapabilityProjection(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	skillID := uuid.New()
	mcpID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Name: "p"}
	repo.tasks = []ProjectTask{{
		ID: taskID, TenantID: tenantID, ProjectID: projectID, Title: "t", Status: ProjectTaskStatusCompleted,
	}}
	repo.projectTaskAttempts = []ProjectTaskAttempt{{
		ID:            attemptID,
		TenantID:      tenantID,
		ProjectTaskID: taskID,
		AttemptNo:     1,
		Status:        "succeeded",
	}}
	payload, _ := json.Marshal(map[string]any{
		"skills": []any{
			map[string]any{
				"skill_id": skillID.String(), "skill_key": "linux", "source_scope": "project", "version": "1",
			},
		},
		"mcp_servers": []any{
			map[string]any{
				"server_id": mcpID.String(), "server_key": "gh", "name": "GitHub", "source_scope": "dependency_closure",
				"url": "https://should-not-leak.example",
			},
		},
		"environment": []any{map[string]any{"name": "TOKEN", "value": "sekrit"}},
		"metadata": map[string]any{
			"skill_conflicts": []any{
				map[string]any{"slug": "linux", "source": "project_binding", "winning_skill_id": skillID.String(), "winning_source": "project", "dropped_source": "employee"},
			},
		},
	})
	repo.capabilityProjectionPayloads = map[uuid.UUID][]byte{attemptID: payload}
	repo.skillNamesByID = map[uuid.UUID]string{skillID: "Linux 排障"}
	repo.projectTaskAttestations = []ProjectTaskAttestation{{
		TenantID:  tenantID,
		AttemptID: attemptID,
		Metadata: map[string]any{
			"skill_conflicts": []any{
				map[string]any{"slug": "beta", "source": "workspace_native"},
			},
		},
	}}

	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
	})
	if err != nil {
		t.Fatalf("GetExecutionTrace: %v", err)
	}
	if len(trace.Attempts) != 1 || trace.Attempts[0].CapabilityProjection == nil {
		t.Fatalf("missing projection: %#v", trace.Attempts)
	}
	snap := trace.Attempts[0].CapabilityProjection
	if !snap.Available || snap.Summary.SkillCount != 1 || snap.Summary.MCPCount != 1 || snap.Summary.ConflictCount != 2 {
		t.Fatalf("snap summary %#v available=%v", snap.Summary, snap.Available)
	}
	if snap.Skills[0].SkillName != "Linux 排障" || snap.Skills[0].SourceScope != "project" {
		t.Fatalf("skill %#v", snap.Skills[0])
	}
	if snap.MCPServers[0].SourceScope != "dependency_closure" {
		t.Fatalf("mcp %#v", snap.MCPServers[0])
	}
	raw, _ := json.Marshal(snap)
	if strings.Contains(string(raw), "sekrit") || strings.Contains(string(raw), "should-not-leak") {
		t.Fatalf("secret leaked: %s", raw)
	}
}

func TestGetExecutionTraceCapabilityProjectionUnavailableWithoutPayload(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	attemptID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID}
	taskID := uuid.New()
	repo.tasks = []ProjectTask{{
		ID: taskID, TenantID: tenantID, ProjectID: projectID, Title: "t", Status: ProjectTaskStatusRunning,
	}}
	repo.projectTaskAttempts = []ProjectTaskAttempt{{
		ID: attemptID, TenantID: tenantID, ProjectTaskID: taskID, AttemptNo: 1, Status: "running",
	}}
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{TenantID: tenantID, ProjectID: projectID})
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if trace.Attempts[0].CapabilityProjection == nil || trace.Attempts[0].CapabilityProjection.Available {
		t.Fatalf("expected unavailable snapshot, got %#v", trace.Attempts[0].CapabilityProjection)
	}
}

func TestGetExecutionTraceDoesNotFallbackSummaryWhenTaskHasMatchedSummaryEvent(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	firstAttemptID := uuid.New()
	secondAttemptID := uuid.New()
	summaryID := uuid.New()
	now := time.Now().UTC()
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "核对证据链",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts,
		ProjectTaskAttempt{
			ID:            firstAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     1,
			Status:        ProjectTaskAttemptStatusFailed,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		ProjectTaskAttempt{
			ID:            secondAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     2,
			Status:        ProjectTaskAttemptStatusSucceeded,
			CreatedAt:     now.Add(time.Second),
			UpdatedAt:     now.Add(time.Second),
		},
	)
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "第二次尝试证据链完整",
		EvidenceRefs:        []any{map[string]any{"ref": "evidence://2"}},
		ArtifactRefs:        []any{map[string]any{"ref": "artifact://2"}},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: true,
		CreatedAt:           now.Add(2 * time.Second),
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents, ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        &taskID,
		ProjectTaskAttemptID: &secondAttemptID,
		EventType:            ExecutionLedgerEventSummaryCreated,
		SourceType:           "project_execution_summary",
		SourceID:             summaryID.String(),
		ActorType:            "system",
		OutputSummary:        strPtr("第二次尝试证据链完整"),
		OccurredAt:           now.Add(3 * time.Second),
		CreatedAt:            now.Add(3 * time.Second),
	})

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Len(t, trace.Attempts, 2)
	require.Equal(t, firstAttemptID, trace.Attempts[0].AttemptID)
	require.Nil(t, trace.Attempts[0].Summary)
	require.Equal(t, secondAttemptID, trace.Attempts[1].AttemptID)
	require.NotNil(t, trace.Attempts[1].Summary)
	require.Equal(t, summaryID, trace.Attempts[1].Summary.ExecutionSummaryID)
	require.Equal(t, int32(1), trace.Summary.ArtifactRefCount)
	require.Equal(t, int32(1), trace.Summary.EvidenceRefCount)
	require.Equal(t, int32(1), trace.Summary.HumanReviewRequiredCount)
}

func TestGetExecutionTraceRequestsThousandExecutionSummaries(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	_, err = service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Equal(t, int32(1000), repo.lastExecutionSummariesLimit)
	require.Equal(t, int32(0), repo.lastExecutionSummariesOffset)
}

func TestGetExecutionTraceUsesSummaryMappingOutsideVisibleEventFilter(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	firstAttemptID := uuid.New()
	secondAttemptID := uuid.New()
	summaryID := uuid.New()
	now := time.Now().UTC()
	eventType := ExecutionLedgerEventProviderEvent
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "核对执行事件",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts,
		ProjectTaskAttempt{
			ID:            firstAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     1,
			Status:        ProjectTaskAttemptStatusFailed,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		ProjectTaskAttempt{
			ID:            secondAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     2,
			Status:        ProjectTaskAttemptStatusSucceeded,
			CreatedAt:     now.Add(time.Second),
			UpdatedAt:     now.Add(time.Second),
		},
	)
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "第二次尝试完成",
		EvidenceRefs:        []any{map[string]any{"ref": "evidence://summary-filtered"}},
		ArtifactRefs:        []any{map[string]any{"ref": "artifact://summary-filtered"}},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: false,
		CreatedAt:           now.Add(2 * time.Second),
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents,
		ExecutionLedgerEvent{
			ID:                   uuid.New(),
			TenantID:             tenantID,
			ProjectID:            projectID,
			ProjectTaskID:        &taskID,
			ProjectTaskAttemptID: &secondAttemptID,
			EventType:            ExecutionLedgerEventProviderEvent,
			SourceType:           "provider_session_event",
			SourceID:             uuid.NewString(),
			ActorType:            "provider",
			OutputSummary:        strPtr("visible provider event"),
			OccurredAt:           now.Add(3 * time.Second),
			CreatedAt:            now.Add(3 * time.Second),
		},
		ExecutionLedgerEvent{
			ID:                   uuid.New(),
			TenantID:             tenantID,
			ProjectID:            projectID,
			ProjectTaskID:        &taskID,
			ProjectTaskAttemptID: &secondAttemptID,
			EventType:            ExecutionLedgerEventSummaryCreated,
			SourceType:           "project_execution_summary",
			SourceID:             summaryID.String(),
			ActorType:            "system",
			OutputSummary:        strPtr("hidden by visible filter"),
			OccurredAt:           now.Add(4 * time.Second),
			CreatedAt:            now.Add(4 * time.Second),
		},
	)

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: &eventType,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Len(t, trace.Attempts, 2)
	require.Nil(t, trace.Attempts[0].Summary)
	require.NotNil(t, trace.Attempts[1].Summary)
	require.Equal(t, summaryID, trace.Attempts[1].Summary.ExecutionSummaryID)
	require.Empty(t, trace.Attempts[0].Events)
	require.Len(t, trace.Attempts[1].Events, 1)
	require.Equal(t, ExecutionLedgerEventProviderEvent, trace.Attempts[1].Events[0].EventType)
	require.Len(t, repo.executionLedgerEventListRequests, 2)
	require.Equal(t, &eventType, repo.executionLedgerEventListRequests[0].EventType)
	require.NotNil(t, repo.executionLedgerEventListRequests[1].EventType)
	require.Equal(t, ExecutionLedgerEventSummaryCreated, *repo.executionLedgerEventListRequests[1].EventType)
	require.Nil(t, repo.executionLedgerEventListRequests[1].ErrorFamily)
	require.Equal(t, int32(1000), repo.executionLedgerEventListRequests[1].Limit)
	require.Equal(t, int32(0), repo.executionLedgerEventListRequests[1].Offset)
}

func TestGetExecutionTraceFallbackSummaryAttachesToLatestAttempt(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	firstAttemptID := uuid.New()
	secondAttemptID := uuid.New()
	summaryID := uuid.New()
	startedAt := time.Now().UTC()
	finishedAt := startedAt.Add(5 * time.Second)
	secondStartedAt := startedAt.Add(10 * time.Second)
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "生成验收报告",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts,
		ProjectTaskAttempt{
			ID:            firstAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     1,
			Status:        ProjectTaskAttemptStatusFailed,
			StartedAt:     &startedAt,
			FinishedAt:    &finishedAt,
			CreatedAt:     startedAt,
			UpdatedAt:     finishedAt,
		},
		ProjectTaskAttempt{
			ID:            secondAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     2,
			Status:        ProjectTaskAttemptStatusSucceeded,
			StartedAt:     &secondStartedAt,
			CreatedAt:     secondStartedAt,
			UpdatedAt:     secondStartedAt,
		},
	)
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "最新尝试完成报告",
		EvidenceRefs:        []any{map[string]any{"ref": "evidence://latest"}},
		ArtifactRefs:        []any{map[string]any{"ref": "artifact://latest"}},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: false,
		CreatedAt:           startedAt.Add(11 * time.Second),
	})

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Len(t, trace.Attempts, 2)
	require.Equal(t, firstAttemptID, trace.Attempts[0].AttemptID)
	require.Nil(t, trace.Attempts[0].Summary)
	require.Equal(t, secondAttemptID, trace.Attempts[1].AttemptID)
	require.NotNil(t, trace.Attempts[1].Summary)
	require.Equal(t, summaryID, trace.Attempts[1].Summary.ExecutionSummaryID)
}

func TestGetExecutionTraceDeduplicatesAggregateRefsAcrossSummaryEventAndSummary(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	summaryID := uuid.New()
	now := time.Now().UTC()
	artifactRef := map[string]any{"ref": "artifact://same"}
	evidenceRef := map[string]any{"ref": "evidence://same"}
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "去重引用计数",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:            attemptID,
		TenantID:      tenantID,
		ProjectTaskID: taskID,
		AttemptNo:     1,
		Status:        ProjectTaskAttemptStatusSucceeded,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "引用相同",
		EvidenceRefs:        []any{evidenceRef},
		ArtifactRefs:        []any{artifactRef},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: false,
		CreatedAt:           now,
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents, ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        &taskID,
		ProjectTaskAttemptID: &attemptID,
		EventType:            ExecutionLedgerEventSummaryCreated,
		SourceType:           "project_execution_summary",
		SourceID:             summaryID.String(),
		ActorType:            "system",
		ArtifactRefs:         []any{artifactRef},
		EvidenceRefs:         []any{evidenceRef},
		OccurredAt:           now,
		CreatedAt:            now,
	})

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), trace.Summary.ArtifactRefCount)
	require.Equal(t, int32(1), trace.Summary.EvidenceRefCount)
	require.Len(t, trace.Attempts, 1)
	require.Len(t, trace.Attempts[0].Events, 1)
	require.Len(t, trace.Attempts[0].Events[0].ArtifactRefs, 1)
	require.Len(t, trace.Attempts[0].Summary.ArtifactRefs, 1)
}

func TestBuildProjectTaskExecutionPacketIncludesDependenciesAndHumanDecisions(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	blockerID := uuid.New()
	decisionID := uuid.New()
	task := ProjectTask{
		ID:              taskID,
		TenantID:        tenantID,
		ProjectID:       projectID,
		Title:           "执行上线检查",
		Status:          ProjectTaskStatusPlanned,
		ExpectedOutputs: []any{"deployment_report"},
		InputRequirements: map[string]any{
			"environment": "staging",
		},
		HandoffContract: map[string]any{
			"completion_path": "project_task_attempt_writeback",
		},
		BlockedByTaskIDs: []uuid.UUID{blockerID},
	}
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: blockerID,
		Conclusion:    "依赖任务已完成，产出 staging 检查清单。",
		EvidenceRefs:  []any{"evidence://staging-checklist"},
	})
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:             decisionID,
		TenantID:       tenantID,
		ProjectID:      projectID,
		ProjectTaskID:  &taskID,
		DecisionType:   "approval_required",
		StatusSnapshot: ProjectTaskStatusWaitingHuman,
	})

	packet, err := service.BuildProjectTaskExecutionPacket(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, "v1", packet.Version)
	require.Equal(t, taskID.String(), packet.ProjectTaskID)
	require.Contains(t, packet.ExpectedOutputs, "deployment_report")
	require.Len(t, packet.DependencyOutputs, 1)
	require.Equal(t, "evidence://staging-checklist", packet.DependencyOutputs[0].EvidenceRefs[0])
	require.Len(t, packet.HumanDecisionRefs, 1)
}

func TestRecordAttemptContextUpdateRoutesContractChangeToReplan(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:               taskID,
		TenantID:         tenantID,
		ProjectID:        projectID,
		Status:           ProjectTaskStatusRunning,
		CurrentAttemptID: &attemptID,
	})

	update, err := service.RecordAttemptContextUpdate(context.Background(), RecordAttemptContextUpdateRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskID,
		AttemptID:     &attemptID,
		UpdateKind:    "requirement_changed",
		Payload:       map[string]any{"new_scope": "include production"},
	})
	require.NoError(t, err)
	require.Equal(t, ContextUpdateDeliveryCancelAndReplan, update.DeliveryMode)
}

func TestProjectTaskLivenessProjectionExplainsNextAction(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	waitingReason := HumanWaitReasonMissingContext
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:               taskID,
		TenantID:         tenantID,
		ProjectID:        projectID,
		Status:           ProjectTaskStatusWaitingHuman,
		CurrentAttemptID: &attemptID,
		WaitingReason:    &waitingReason,
	})

	items, err := service.ListProjectTaskLiveness(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, ProjectTaskLivenessWaitingHuman, items[0].Liveness)
	require.Equal(t, "human response", items[0].NextAction)
	require.Equal(t, HumanWaitReasonMissingContext, items[0].Reason)
}

// waiting_reason / waiting_request_id 是粘性列，任务进终态时写侧不清（见
// UpdateProjectTaskStatus）。读侧必须按状态收敛，否则会投影出 is_terminal=true
// 却又带着"在等某个决策"的自相矛盾状态。
func TestProjectTaskLivenessDropsStickyWaitingPointerOnTerminalTasks(t *testing.T) {
	for _, status := range []string{
		ProjectTaskStatusCompleted,
		ProjectTaskStatusFailed,
		ProjectTaskStatusCancelled,
		"done",
		"success",
	} {
		t.Run(status, func(t *testing.T) {
			repo := newMemoryRepository()
			service, err := NewService(repo)
			require.NoError(t, err)
			tenantID := uuid.New()
			projectID := uuid.New()
			waitingReason := HumanWaitReasonAcceptanceRequired
			waitingRequestID := uuid.New()
			repo.tasks = append(repo.tasks, ProjectTask{
				ID:               uuid.New(),
				TenantID:         tenantID,
				ProjectID:        projectID,
				Status:           status,
				WaitingReason:    &waitingReason,
				WaitingRequestID: &waitingRequestID,
			})

			items, err := service.ListProjectTaskLiveness(context.Background(), tenantID, projectID)
			require.NoError(t, err)
			require.Len(t, items, 1)
			require.Equal(t, ProjectTaskLivenessTerminal, items[0].Liveness)
			require.Nil(t, items[0].WaitingRequestID, "终态任务不得再带等待中的决策 id")
			require.Empty(t, items[0].Reason, "终态任务不得再带等待原因")
		})
	}
}

// 反向钉住：仍在等人的任务必须照常带出决策 id，别把守卫做成一刀切。
func TestProjectTaskLivenessKeepsWaitingPointerWhileWaitingHuman(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	waitingReason := HumanWaitReasonAcceptanceRequired
	waitingRequestID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:               uuid.New(),
		TenantID:         tenantID,
		ProjectID:        projectID,
		Status:           ProjectTaskStatusWaitingHuman,
		WaitingReason:    &waitingReason,
		WaitingRequestID: &waitingRequestID,
	})

	items, err := service.ListProjectTaskLiveness(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, ProjectTaskLivenessWaitingHuman, items[0].Liveness)
	require.NotNil(t, items[0].WaitingRequestID)
	require.Equal(t, waitingRequestID, *items[0].WaitingRequestID)
}

func TestQueueProjectTaskCreatesAttemptAndMovesTaskToQueued(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "实现幂等写回",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	})

	result, err := service.QueueProjectTask(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        taskID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: &runID,
		RuntimeTaskID:        &runtimeTaskID,
		RuntimeNodeID:        &runtimeNodeID,
		IdempotencyKey:       "project-task:" + taskID.String() + ":attempt:1:queue",
		LeaseToken:           "lease-token-1",
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, result.Task.Status)
	require.NotNil(t, result.Task.CurrentAttemptID)
	require.Equal(t, runID, *result.Task.DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *result.Task.RuntimeTaskID)
	require.Equal(t, int32(1), result.Attempt.AttemptNo)
	require.Equal(t, ProjectTaskAttemptStatusQueued, result.Attempt.Status)
	require.Equal(t, runID, *result.Attempt.DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *result.Attempt.RuntimeTaskID)
	require.Equal(t, runtimeNodeID, *result.Attempt.RuntimeNodeID)
	require.Equal(t, "lease-token-1", result.Attempt.LeaseToken)
	require.Equal(t, "v1", result.Attempt.ExecutionContextPacketVersion)
	require.Equal(t, taskID.String(), result.Attempt.ExecutionContextPacket["project_task_id"])
	require.Equal(t, "实现幂等写回", result.Attempt.ExecutionContextPacket["title"])
	require.Equal(t, "project_coordinator", result.Event.ActorType)
	require.Equal(t, taskID.String(), result.Event.ActorID)
	require.Equal(t, result.Attempt.ID.String(), result.Event.Payload["project_task_attempt_id"])
	require.Equal(t, ProjectTaskStatusQueued, result.Event.Payload["project_task_status"])
	require.Equal(t, runID.String(), result.Event.Payload["digital_employee_run_id"])
	require.Equal(t, runtimeTaskID.String(), result.Event.Payload["runtime_task_id"])
	require.Equal(t, runtimeNodeID.String(), result.Event.Payload["runtime_node_id"])
}

func TestQueueProjectTaskCreatesAttemptWithEmployeeAndProviderAuditFacts(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "记录执行身份",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	})

	result, err := service.QueueProjectTask(context.Background(), QueueProjectTaskRequest{
		TenantID:                      tenantID,
		ProjectID:                     projectID,
		ProjectTaskID:                 taskID,
		ProjectTaskAttemptID:          &attemptID,
		DigitalEmployeeID:             employeeID,
		ProviderType:                  "codex",
		DigitalEmployeeRunID:          &runID,
		RuntimeTaskID:                 &runtimeTaskID,
		RuntimeNodeID:                 &runtimeNodeID,
		IdempotencyKey:                "project-task:" + taskID.String() + ":attempt:1:audit",
		LeaseToken:                    "lease-token-1",
		ExecutionContextPacket:        map[string]any{"project_id": projectID.String()},
		ExecutionContextPacketVersion: "v1",
	})
	require.NoError(t, err)
	require.NotNil(t, result.Attempt.DigitalEmployeeID)
	require.Equal(t, employeeID, *result.Attempt.DigitalEmployeeID)
	require.NotNil(t, result.Attempt.ProviderType)
	require.Equal(t, "codex", *result.Attempt.ProviderType)

	readBack, err := repo.GetProjectTaskAttempt(context.Background(), tenantID, attemptID)
	require.NoError(t, err)
	require.NotNil(t, readBack.DigitalEmployeeID)
	require.Equal(t, employeeID, *readBack.DigitalEmployeeID)
	require.NotNil(t, readBack.ProviderType)
	require.Equal(t, "codex", *readBack.ProviderType)
}

func TestStartProjectTaskAttemptAdvancesRunning(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)

	started, err := service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-start-1"),
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusRunning, started.Status)
	require.Equal(t, ProjectTaskStatusRunning, repo.tasks[0].Status)
	require.NotNil(t, started.StartedAt)
	require.NotNil(t, started.RenewedAt)
}

func TestStartProjectTaskAttemptRetriesUntilQueuedAttemptVisible(t *testing.T) {
	baseRepo := newMemoryRepository()
	fixture := newProjectTaskAttemptServiceFixture(baseRepo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	repo := &delayedAttemptReadinessRepository{
		memoryRepository:    baseRepo,
		staleProjectTaskID:  fixture.taskID,
		staleReadsRemaining: 1,
	}
	service, err := NewService(repo)
	require.NoError(t, err)

	started, err := service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-start-race"),
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusRunning, started.Status)
	require.Zero(t, repo.staleReadsRemaining)
}

func TestStartProjectTaskAttemptWritesLedgerEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	_, err = service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("start-ledger"),
	})
	require.NoError(t, err)
	require.Len(t, repo.executionLedgerEvents, 1)
	require.Equal(t, ExecutionLedgerEventAttemptStarted, repo.executionLedgerEvents[0].EventType)
	require.Equal(t, fixture.attemptID, *repo.executionLedgerEvents[0].ProjectTaskAttemptID)
}

func TestStartProjectTaskAttemptIgnoresLedgerWriteFailure(t *testing.T) {
	repo := newMemoryRepository()
	repo.createExecutionLedgerEventErr = fmt.Errorf("ledger unavailable")
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)

	started, err := service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("start-ledger-error"),
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusRunning, started.Status)
	require.Equal(t, ProjectTaskStatusRunning, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusRunning, repo.projectTaskAttempts[0].Status)
	require.Empty(t, repo.executionLedgerEvents)
}

func TestStartProjectTaskAttemptRejectsWrongLeaseToken(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	req := fixture.runtimeRequest("attempt-start-1")
	req.LeaseToken = "wrong-token"

	_, err = service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: req,
	})

	require.ErrorIs(t, err, ErrProjectConflict)
}

func TestRenewProjectTaskAttemptLeaseUpdatesExpiry(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	expiresAt := time.Now().UTC().Add(5 * time.Minute)

	err = service.RenewProjectTaskAttemptLease(context.Background(), RenewProjectTaskAttemptLeaseRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-lease-1"),
		LeaseExpiresAt:                   &expiresAt,
	})

	require.NoError(t, err)
	require.NotNil(t, repo.projectTaskAttempts[0].LeaseExpiresAt)
	require.True(t, repo.projectTaskAttempts[0].LeaseExpiresAt.Equal(expiresAt))
	require.NotNil(t, repo.projectTaskAttempts[0].RenewedAt)
}

func TestCompleteProjectTaskAttemptCreatesSummaryAndCompletesTask(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-1"),
		Conclusion:                       "done",
		EvidenceRefs:                     []any{"s3://bucket/report.md"},
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusSucceeded, repo.projectTaskAttempts[0].Status)
	require.Contains(t, repo.eventTypes, ProjectEventTaskCompleted)
}

func TestCompleteProjectTaskAttemptStoresStructuredResultContract(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "完成分析",
		AcceptanceResults: []TaskResultAcceptanceResult{
			{
				Criterion:    "输出结论",
				Status:       TaskResultCriterionStatusPassed,
				EvidenceRefs: []string{"artifact:report"},
			},
		},
		EvidenceRefs: []TaskResultRef{{Type: "report", Ref: "artifact:report"}},
		ArtifactRefs: []TaskResultRef{{Type: "markdown", Ref: "artifact:analysis-report"}},
		Verification: []TaskResultVerification{{Type: "command", Status: TaskResultVerificationStatusPassed, Summary: "命令通过"}},
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-with-result"),
		Conclusion:                       "legacy conclusion",
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	require.Equal(t, "完成分析", summary.Conclusion)
	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultStatusCompleted, results[0].ResultStatus)
	require.Equal(t, TaskResultDecisionCompleteAccepted, results[0].Decision)
	require.Equal(t, "accepted", results[0].ValidationStatus)
	require.NotNil(t, results[0].AttemptID)
	require.Equal(t, fixture.attemptID, *results[0].AttemptID)
	require.NotNil(t, results[0].ExecutionSummaryID)
	require.Equal(t, summary.ID, *results[0].ExecutionSummaryID)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestCompleteProjectTaskAttemptLegacyCompletionStoresAcceptedLatestResult(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	completeReq := CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-legacy-result"),
		Conclusion:                       "done with legacy evidence",
		EvidenceRefs:                     []any{"s3://bucket/report.md"},
		ArtifactRefs:                     []any{"artifact:analysis-report"},
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), completeReq)

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusSucceeded, repo.projectTaskAttempts[0].Status)
	require.Equal(t, 1, coordinator.completedSignals)
	require.Equal(t, fixture.taskID, coordinator.lastCompleted.ProjectTaskID)
	require.Equal(t, summary.ID, coordinator.lastCompleted.ExecutionSummaryID)
	require.Equal(t, repo.projects[fixture.projectID].CoordinationWorkflowID, coordinator.lastCompleted.WorkflowID)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, TaskResultContractFromLegacyCompletion(completeReq), results[0].Contract)
	require.Equal(t, TaskResultStatusCompleted, results[0].ResultStatus)
	require.Equal(t, TaskResultDecisionCompleteAccepted, results[0].Decision)
	require.Equal(t, "accepted", results[0].ValidationStatus)
	require.True(t, ProjectTaskResultAcceptedForDependencyUnlock(results[0]))
	require.NotNil(t, results[0].AttemptID)
	require.Equal(t, fixture.attemptID, *results[0].AttemptID)
	require.NotNil(t, results[0].ExecutionSummaryID)
	require.Equal(t, summary.ID, *results[0].ExecutionSummaryID)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
	require.NotNil(t, results[0].CreatedEventID)
	require.Equal(t, *results[0].CreatedEventID, coordinator.lastCompleted.CompletedEventID)
}

func TestCompleteProjectTaskAttemptResultContractHumanReviewRoutesToWaitingHuman(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	// §5.2: a live downstream dependent is required for the provider
	// HumanReviewRequest to open downstream_release.
	addFixtureDownstreamDependency(repo.memoryRepository, fixture)
	contract := validCompletedTaskResultContract()
	contract.HumanReviewRequest = &TaskResultHumanReviewRequest{
		Reason:     "需要负责人确认验收口径",
		Prompt:     "请确认是否接受该结果",
		Options:    []string{"accept", "request_revision"},
		RequiredBy: "human_owner",
		ReviewType: "acceptance",
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-contract-human-review"),
		Conclusion:                       "legacy conclusion",
		RequiresHumanReview:              false,
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingRequestID)
	require.Equal(t, ProjectTaskAttemptStatusSucceeded, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, fixture.taskID, *repo.decisionRequests[0].ProjectTaskID)
	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultDecisionWaitingHumanReview, results[0].Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestResolveProjectTaskHumanWaitAcceptanceApproveAppendsAcceptedLatestResult(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture, summary, decision, waitingResult, originalContract := completeProjectTaskAttemptIntoHumanReviewResult(t, service, repo, "human-review-accept-result")

	task, err := service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionApprove,
		ResponseSummary: "验收通过，证据完整",
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusCompleted, task.Status)
	require.NotNil(t, task.LatestTaskResultID)
	require.NotEqual(t, waitingResult.ID, *task.LatestTaskResultID)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)

	acceptedResult := requireProjectTaskResultByID(t, results, *task.LatestTaskResultID)
	oldResult := requireProjectTaskResultByID(t, results, waitingResult.ID)
	require.Equal(t, TaskResultDecisionWaitingHumanReview, oldResult.Decision)
	require.Nil(t, oldResult.DecisionRequestID)
	require.Equal(t, TaskResultStatusCompleted, acceptedResult.ResultStatus)
	require.Equal(t, "accepted", acceptedResult.ValidationStatus)
	require.Equal(t, TaskResultDecisionCompleteAccepted, acceptedResult.Decision)
	require.NotNil(t, acceptedResult.AttemptID)
	require.Equal(t, fixture.attemptID, *acceptedResult.AttemptID)
	require.NotNil(t, acceptedResult.ExecutionSummaryID)
	require.Equal(t, summary.ID, *acceptedResult.ExecutionSummaryID)
	require.NotNil(t, acceptedResult.CreatedEventID)
	require.NotEqual(t, *oldResult.CreatedEventID, *acceptedResult.CreatedEventID)
	require.NotNil(t, acceptedResult.DecisionRequestID)
	require.Equal(t, decision.ID, *acceptedResult.DecisionRequestID)
	require.Equal(t, originalContract.Summary, acceptedResult.Contract.Summary)
	require.Equal(t, originalContract.HumanReviewRequest, acceptedResult.Contract.HumanReviewRequest)
	require.Len(t, acceptedResult.Contract.AcceptanceResults, 1)
	require.Equal(t, "验收通过，证据完整", acceptedResult.Contract.AcceptanceResults[0].HumanAcceptedReason)
}

func TestResolveProjectTaskHumanWaitAcceptanceApproveSignalsCoordinatorCompleted(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture, summary, _, _, _ := completeProjectTaskAttemptIntoHumanReviewResult(t, service, repo, "human-review-accept-signal")
	initialCompletedEvents := countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted)

	task, err := service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionApprove,
		ResponseSummary: "验收通过",
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusCompleted, task.Status)
	require.Equal(t, 1, coordinator.completedSignals)
	require.Equal(t, fixture.taskID, coordinator.lastCompleted.ProjectTaskID)
	require.Equal(t, summary.ID, coordinator.lastCompleted.ExecutionSummaryID)
	require.Equal(t, repo.projects[fixture.projectID].CoordinationWorkflowID, coordinator.lastCompleted.WorkflowID)
	require.Equal(t, initialCompletedEvents+1, countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted))
	completedEvent := lastProjectEventOfType(t, repo.events, ProjectEventTaskCompleted)
	require.Equal(t, completedEvent.ID, coordinator.lastCompleted.CompletedEventID)
}

func TestResolveProjectTaskHumanWaitAcceptanceDecisionLinkFailureLeavesTaskWaitingAndLatestUnaccepted(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture, _, _, waitingResult, _ := completeProjectTaskAttemptIntoHumanReviewResult(t, service, repo, "human-review-link-failure")
	linkErr := errors.New("decision link unavailable")
	repo.linkProjectTaskResultDecisionRequestErr = linkErr

	_, err = service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionApprove,
		ResponseSummary: "验收通过",
	})

	require.ErrorIs(t, err, linkErr)
	require.Equal(t, 0, coordinator.completedSignals)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.LatestTaskResultID)
	require.Equal(t, waitingResult.ID, *task.LatestTaskResultID)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	latest := requireProjectTaskResultByID(t, results, *task.LatestTaskResultID)
	require.Equal(t, TaskResultDecisionWaitingHumanReview, latest.Decision)
	require.False(t, ProjectTaskResultAcceptedForDependencyUnlock(latest))
	// 补偿动作必须让任务**可重试**：终态写回会清空等待指针，只还原 status 的话
	// waiting_reason 为空，approve 守卫会让人类永远点不动"验收通过"。
	require.NotNil(t, task.WaitingReason, "回滚后必须还原 waiting_reason，否则重试永久 409")
	require.Equal(t, HumanWaitReasonAcceptanceRequired, *task.WaitingReason)
	require.NotNil(t, task.WaitingRequestID, "回滚后必须还原 waiting_request_id")
}

// 承接上一条：故障排除后人类再点一次"验收通过"必须真的能过。
// 这是 code review 揪出的回归——终态写回清空等待指针后，补偿动作只还原 status，
// 重试会被 approve 守卫永久挡成 ErrProjectConflict，任务卡死在 waiting_human。
func TestResolveProjectTaskHumanWaitRetrySucceedsAfterDecisionLinkFailureRollback(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture, _, _, _, _ := completeProjectTaskAttemptIntoHumanReviewResult(t, service, repo, "human-review-retry-after-rollback")
	linkErr := errors.New("decision link unavailable")
	repo.linkProjectTaskResultDecisionRequestErr = linkErr

	request := ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionApprove,
		ResponseSummary: "验收通过",
	}

	_, err = service.ResolveProjectTaskHumanWait(context.Background(), request)
	require.ErrorIs(t, err, linkErr)

	// 故障恢复后重试。
	repo.linkProjectTaskResultDecisionRequestErr = nil
	task, err := service.ResolveProjectTaskHumanWait(context.Background(), request)

	require.NoError(t, err, "回滚后重试必须能成功，而不是被 approve 守卫永久拒绝")
	require.Equal(t, ProjectTaskStatusCompleted, task.Status)
	require.Equal(t, 1, coordinator.completedSignals)
}

func TestResolveProjectTaskHumanWaitAcceptanceLatestLinkFailureStillRestoresTaskWaiting(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture, _, _, waitingResult, _ := completeProjectTaskAttemptIntoHumanReviewResult(t, service, repo, "human-review-latest-link-failure")
	linkErr := errors.New("latest result link unavailable")
	repo.linkProjectTaskLatestResultErr = linkErr
	repo.linkProjectTaskLatestResultErrAfter = repo.linkProjectTaskLatestResultCalls

	_, err = service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionApprove,
		ResponseSummary: "验收通过",
	})

	require.ErrorIs(t, err, linkErr)
	require.Contains(t, err.Error(), "rollback human accepted task result")
	require.Equal(t, 0, coordinator.completedSignals)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.LatestTaskResultID)
	require.Equal(t, waitingResult.ID, *task.LatestTaskResultID)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	latest := requireProjectTaskResultByID(t, results, *task.LatestTaskResultID)
	require.Equal(t, TaskResultDecisionWaitingHumanReview, latest.Decision)
	require.False(t, ProjectTaskResultAcceptedForDependencyUnlock(latest))
}

func TestResolveProjectTaskHumanWaitResultReviewNonApproveDoesNotAcceptResultOrSignal(t *testing.T) {
	tests := []struct {
		name           string
		resolution     string
		expectedStatus string
	}{
		{name: "resume_same_task", resolution: HumanWaitResolutionResumeSameTask, expectedStatus: ProjectTaskStatusQueued},
		{name: "cancel_and_replan", resolution: HumanWaitResolutionCancelAndReplan, expectedStatus: ProjectTaskStatusCancelled},
		{name: "cancel_without_replan", resolution: HumanWaitResolutionCancelWithoutPlan, expectedStatus: ProjectTaskStatusCancelled},
		{name: "mark_failed", resolution: HumanWaitResolutionMarkFailed, expectedStatus: ProjectTaskStatusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newProjectTaskResultMemoryRepository()
			coordinator := &fakeCoordinatorSignalClient{}
			service, err := NewServiceWithCoordinator(repo, coordinator)
			require.NoError(t, err)
			fixture, _, _, waitingResult, _ := completeProjectTaskAttemptIntoHumanReviewResult(t, service, repo, "human-review-"+tt.name)

			task, err := service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
				TenantID:        fixture.tenantID,
				ProjectID:       fixture.projectID,
				ProjectTaskID:   fixture.taskID,
				ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
				Resolution:      tt.resolution,
				ResponseSummary: "继续处理",
			})

			require.NoError(t, err)
			require.Equal(t, tt.expectedStatus, task.Status)
			require.Equal(t, 0, coordinator.completedSignals)
			results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
				TenantID:      fixture.tenantID,
				ProjectID:     fixture.projectID,
				ProjectTaskID: fixture.taskID,
				Limit:         10,
			})
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Equal(t, waitingResult.ID, results[0].ID)
			require.Equal(t, TaskResultDecisionWaitingHumanReview, results[0].Decision)
			require.NotEqual(t, TaskResultDecisionCompleteAccepted, results[0].Decision)
		})
	}
}

func TestCompleteProjectTaskAttemptRevisionNeededResultContractRoutesToWaitingHuman(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := TaskResultContract{
		Status:  TaskResultStatusRevisionNeeded,
		Summary: "需要负责人确认修订范围",
		RevisionRequest: &TaskResultRevisionRequest{
			Reason:                 "验收口径需要补充",
			RecommendedTaskSummary: "补充缺失证据后重试当前任务",
			RequestedChanges:       []string{"补充验收证据"},
		},
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-revision-result"),
		Conclusion:                       "runtime posted explicit revision result",
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	require.Empty(t, repo.executionSummaries)
	require.Equal(t, "需要负责人确认修订范围", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingReason)
	require.Equal(t, HumanWaitReasonClarification, *repo.tasks[0].WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_clarification", repo.decisionRequests[0].DecisionType)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, contract, result.Contract)
	require.Equal(t, TaskResultDecisionRevisionAttempt, result.Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, result.ID, *repo.tasks[0].LatestTaskResultID)
}

func TestCompleteProjectTaskAttemptBlockedResultContractRoutesToWaitingHuman(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := TaskResultContract{
		Status:  TaskResultStatusBlocked,
		Summary: "缺少客户授权",
		Blocker: &TaskResultBlocker{
			Reason:           "permission_required",
			ResolutionPrompt: "请负责人补充客户系统访问授权",
			RequiredBy:       "human_owner",
			ContextRefs:      []TaskResultRef{{Kind: "missing_context", Ref: "customer-permission"}},
		},
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-blocked-result"),
		Conclusion:                       "runtime posted explicit blocked result",
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	require.Empty(t, repo.executionSummaries)
	require.Equal(t, "缺少客户授权", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingReason)
	require.Equal(t, HumanWaitReasonPermissionRequired, *repo.tasks[0].WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_permission", repo.decisionRequests[0].DecisionType)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, contract, result.Contract)
	require.Equal(t, TaskResultDecisionBlockedWaitingHuman, result.Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, result.ID, *repo.tasks[0].LatestTaskResultID)
}

func TestCompleteProjectTaskAttemptRetryableFailedResultContractQueuesRetry(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	maxAttempts := int32(3)
	repo.tasks[0].AttemptCount = 1
	repo.tasks[0].MaxAttempts = &maxAttempts
	retryable := true
	contract := TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: "Provider 中途退出",
		Failure: &TaskResultFailure{
			// B 层(provider 真跑过)才允许自动重排;A 层平台启动类失败一律等人。
			ErrorFamily:            FailureFamilyTransientProvider,
			Retryable:              &retryable,
			RecoveryRecommendation: "retry_original_attempt",
			Message:                "provider exited mid-run",
		},
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-failed-retryable-result"),
		Conclusion:                       "runtime posted explicit retryable failure result",
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	require.Empty(t, repo.executionSummaries)
	require.Equal(t, "Provider 中途退出", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusQueued, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].CurrentAttemptID)
	require.NotEqual(t, fixture.attemptID, *repo.tasks[0].CurrentAttemptID)
	require.Equal(t, int32(2), repo.tasks[0].AttemptCount)
	require.Equal(t, ProjectTaskAttemptStatusFailed, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.projectTaskAttempts, 2)
	require.Equal(t, ProjectTaskAttemptStatusQueued, repo.projectTaskAttempts[1].Status)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, contract, result.Contract)
	require.Equal(t, TaskResultDecisionFailedRetryable, result.Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, result.ID, *repo.tasks[0].LatestTaskResultID)
}

func TestCompleteProjectTaskAttemptReplanResultContractWaitsWithPlanInvalid(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	maxAttempts := int32(3)
	repo.tasks[0].AttemptCount = 1
	repo.tasks[0].MaxAttempts = &maxAttempts
	retryable := true
	contract := TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: "当前计划无法继续",
		Failure: &TaskResultFailure{
			ErrorFamily:            FailureFamilyPlanInvalid,
			Retryable:              &retryable,
			RecoveryRecommendation: "request_replan",
			Message:                "dependency graph changed",
		},
		ReplanRequest: &TaskResultReplanRequest{
			Reason:      "依赖关系已变化",
			Scope:       "project",
			Constraints: []string{"保留已有结果"},
		},
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-replan-result"),
		Conclusion:                       "runtime posted explicit replan result",
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	require.Empty(t, repo.executionSummaries)
	require.Equal(t, "当前计划无法继续", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingReason)
	require.Equal(t, HumanWaitReasonPlanInvalid, *repo.tasks[0].WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, contract, result.Contract)
	require.Equal(t, TaskResultDecisionReplanRequested, result.Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, result.ID, *repo.tasks[0].LatestTaskResultID)
}

func TestCompleteProjectTaskAttemptNonRetryableFailedResultContractSignalsFailure(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	retryable := false
	contract := TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: "输出契约无法解析",
		Failure: &TaskResultFailure{
			ErrorFamily:            FailureFamilyNonRetryableExecution,
			Retryable:              &retryable,
			RecoveryRecommendation: "manual_recovery_required",
			Message:                "missing required result fields",
		},
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-failed-final-result"),
		Conclusion:                       "runtime posted explicit final failure result",
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	require.Empty(t, repo.executionSummaries)
	require.Equal(t, "输出契约无法解析", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusFailed, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusFailed, repo.projectTaskAttempts[0].Status)
	require.Equal(t, "输出契约无法解析", *repo.projectTaskAttempts[0].FailureMessage)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 1, coordinator.failedSignals)

	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, contract, result.Contract)
	require.Equal(t, TaskResultDecisionFailedRecovery, result.Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, result.ID, *repo.tasks[0].LatestTaskResultID)
}

func TestCompleteProjectTaskAttemptCancelledResultContractTerminalizesWithoutFailureSignal(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := TaskResultContract{
		Status:  TaskResultStatusCancelled,
		Summary: "业务负责人取消当前任务",
		Cancellation: &TaskResultCancellation{
			Reason:      "需求已取消",
			CancelledBy: "human_owner",
		},
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-cancelled-result"),
		Conclusion:                       "runtime posted explicit cancellation result",
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	require.Empty(t, repo.executionSummaries)
	require.Equal(t, "业务负责人取消当前任务", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusCancelled, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusFailed, repo.projectTaskAttempts[0].Status)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, contract, result.Contract)
	require.Equal(t, TaskResultDecisionCancelledTerminal, result.Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, result.ID, *repo.tasks[0].LatestTaskResultID)
}

func requireSingleProjectTaskResult(t *testing.T, repo *projectTaskResultMemoryRepository, fixture projectTaskAttemptServiceFixture) ProjectTaskResult {
	t.Helper()
	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	return results[0]
}

func TestCompleteProjectTaskAttemptInvalidResultContractRecordsRejectedResultAndWaitsHuman(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := validCompletedTaskResultContract()
	contract.Summary = ""
	initialSummaryCount := len(repo.executionSummaries)
	initialLedgerCount := len(repo.executionLedgerEvents)

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-invalid-result"),
		Conclusion:                       "legacy conclusion",
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingReason)
	require.Equal(t, HumanWaitReasonClarification, *repo.tasks[0].WaitingReason)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_clarification", repo.decisionRequests[0].DecisionType)
	require.Len(t, repo.executionSummaries, initialSummaryCount)
	require.Len(t, repo.executionLedgerEvents, initialLedgerCount+1)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, TaskResultDecisionValidationFailed, results[0].Decision)
	require.Equal(t, "rejected", results[0].ValidationStatus)
	require.Contains(t, results[0].ValidationErrors, "summary_required")
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestSubmitProjectTaskAttemptResultCompletedInvalidContractRecordsRejectedResultAndWaitsHuman(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := validCompletedTaskResultContract()
	contract.Summary = ""

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-completed-invalid"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingReason)
	require.Equal(t, HumanWaitReasonClarification, *repo.tasks[0].WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_clarification", repo.decisionRequests[0].DecisionType)
	require.Empty(t, repo.executionSummaries)

	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, TaskResultDecisionValidationFailed, result.Decision)
	require.Equal(t, "rejected", result.ValidationStatus)
	require.Contains(t, result.ValidationErrors, "summary_required")
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, result.ID, *repo.tasks[0].LatestTaskResultID)
}

func TestSubmitProjectTaskAttemptResultInvalidRevisionContractRecordsRejectedResultAndWaitsHuman(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := TaskResultContract{
		Status:          TaskResultStatusRevisionNeeded,
		Summary:         "需要修订但缺少原因",
		RevisionRequest: &TaskResultRevisionRequest{},
	}

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-revision-invalid"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, "需要修订但缺少原因", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingReason)
	require.Equal(t, HumanWaitReasonClarification, *repo.tasks[0].WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_clarification", repo.decisionRequests[0].DecisionType)
	require.Empty(t, repo.executionSummaries)

	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, TaskResultDecisionValidationFailed, result.Decision)
	require.Equal(t, "rejected", result.ValidationStatus)
	require.Contains(t, result.ValidationErrors, "revision_reason_required")
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, result.ID, *repo.tasks[0].LatestTaskResultID)
}

func TestCompleteProjectTaskAttemptResultLinkFailureRollsBackTerminalWriteback(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	repo.linkProjectTaskLatestResultErr = ErrProjectConflict
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := validCompletedTaskResultContract()
	initialTask := repo.tasks[0]
	initialAttempt := repo.projectTaskAttempts[0]
	initialSummaryCount := len(repo.executionSummaries)
	initialEventCount := len(repo.events)
	initialLedgerCount := len(repo.executionLedgerEvents)

	_, err = service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-link-fails"),
		Conclusion:                       "legacy conclusion",
		ResultContract:                   &contract,
	})

	require.ErrorIs(t, err, ErrProjectConflict)
	require.Equal(t, initialTask.Status, repo.tasks[0].Status)
	require.Equal(t, initialTask.CurrentAttemptID, repo.tasks[0].CurrentAttemptID)
	require.Nil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, initialAttempt.Status, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.executionSummaries, initialSummaryCount)
	require.Len(t, repo.events, initialEventCount)
	require.Len(t, repo.executionLedgerEvents, initialLedgerCount)
	require.Empty(t, repo.projectTaskResults)
}

func TestCompleteProjectTaskAttemptAcceptanceResultLinkFailureRollsBackTerminalWriteback(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	repo.linkProjectTaskLatestResultErr = ErrProjectConflict
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	// §5.2 downstream_release gate: a human-review signal (high risk) AND a
	// non-terminal downstream dependency open the acceptance writeback path.
	highRisk := "high"
	repo.tasks[0].RiskLevel = &highRisk
	downstreamID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        downstreamID,
		TenantID:  repo.tasks[0].TenantID,
		ProjectID: repo.tasks[0].ProjectID,
		Title:     "下游任务",
		Status:    ProjectTaskStatusQueued,
	})
	repo.taskDependents = map[uuid.UUID][]uuid.UUID{repo.tasks[0].ID: {downstreamID}}
	contract := validCompletedTaskResultContract()
	initialTask := repo.tasks[0]
	initialAttempt := repo.projectTaskAttempts[0]
	initialSummaryCount := len(repo.executionSummaries)
	initialEventCount := len(repo.events)
	initialLedgerCount := len(repo.executionLedgerEvents)
	initialDecisionCount := len(repo.decisionRequests)

	_, err = service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-acceptance-link-fails"),
		Conclusion:                       "legacy conclusion",
		ResultContract:                   &contract,
	})

	require.ErrorIs(t, err, ErrProjectConflict)
	require.Equal(t, initialTask.Status, repo.tasks[0].Status)
	require.Equal(t, initialTask.CurrentAttemptID, repo.tasks[0].CurrentAttemptID)
	require.Nil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, initialAttempt.Status, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.executionSummaries, initialSummaryCount)
	require.Len(t, repo.events, initialEventCount)
	require.Len(t, repo.executionLedgerEvents, initialLedgerCount)
	require.Len(t, repo.decisionRequests, initialDecisionCount)
	require.Empty(t, repo.projectTaskResults)
}

func TestSubmitProjectTaskAttemptResultUsesRealServiceAndStoresContract(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := validCompletedTaskResultContract()
	contract.Summary = "结构化结果"

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-real-service"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, "结构化结果", summary.Conclusion)
	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
}

func TestSubmitProjectTaskAttemptResultCompletedRejectsUnknownRuntimeAttestationRef(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	repo.tasks[0].HandoffContract = map[string]any{"requires_runtime_attestation": true}
	contract := completedContractWithRuntimeAttestationRef("attestation:missing")

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-missing-attestation"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, TaskResultDecisionValidationFailed, result.Decision)
	require.Equal(t, "rejected", result.ValidationStatus)
	require.Contains(t, result.ValidationErrors, "verification_attestation_ref_not_found")
}

func TestSubmitProjectTaskAttemptResultCompletedRejectsOtherAttemptRuntimeAttestationRef(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	repo.tasks[0].HandoffContract = map[string]any{"requires_runtime_attestation": true}
	otherAttemptID := uuid.New()
	attestationRef := "attestation:project-task-attempt:" + otherAttemptID.String() + ":attestation:provider_terminal:cmd-1"
	repo.projectTaskAttestations = append(repo.projectTaskAttestations, projectTaskAttestationForFixture(fixture, otherAttemptID, attestationRef, *repo.tasks[0].AssignedDigitalEmployeeID))

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-wrong-attestation"),
		ResultContract:                   completedContractWithRuntimeAttestationRef(attestationRef),
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, TaskResultDecisionValidationFailed, result.Decision)
	require.Equal(t, "rejected", result.ValidationStatus)
	require.Contains(t, result.ValidationErrors, "verification_attestation_ref_wrong_attempt")
}

func TestSubmitProjectTaskAttemptResultCompletedAcceptsOwnedRuntimeAttestationRef(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	repo.tasks[0].HandoffContract = map[string]any{"requires_runtime_attestation": true}
	attestationRef := "attestation:project-task-attempt:" + fixture.attemptID.String() + ":attestation:provider_terminal:cmd-1"
	repo.projectTaskAttestations = append(repo.projectTaskAttestations, projectTaskAttestationForFixture(fixture, fixture.attemptID, attestationRef, *repo.tasks[0].AssignedDigitalEmployeeID))

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-owned-attestation"),
		ResultContract:                   completedContractWithRuntimeAttestationRef(attestationRef),
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusSucceeded, repo.projectTaskAttempts[0].Status)
	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, TaskResultDecisionCompleteAccepted, result.Decision)
	require.Equal(t, "accepted", result.ValidationStatus)
}

// demandCriterionSnapshotTaskKey is the planned task key demandCriterion
// SnapshotFixture assigns to the fixture task and names in each criterion's
// SatisfiedBy, mirroring real Task-4 decomposition data where automated_test
// criteria always name their satisfying tasks and every decomposed task
// carries its planned key.
const demandCriterionSnapshotTaskKey = "task-under-test"

// demandCriterionSnapshotFixture appends one demand_acceptance_criteria
// snapshot row (Task 4) scoped to the fixture's tenant/project and the given
// demand/plan-revision, links the fixture's task to that demand/revision so
// demandAcceptanceCriteriaSnapshot can find it, and puts the task's planned
// key into the criterion's SatisfiedBy (automated_test only — human_judgment
// criteria have empty SatisfiedBy in real data, planner never assigns them a
// satisfying task).
func demandCriterionSnapshotFixture(repo *projectTaskResultMemoryRepository, fixture projectTaskAttemptServiceFixture, demandID, planRevisionID uuid.UUID, criterionID, statement, verificationMethod string) {
	taskKey := demandCriterionSnapshotTaskKey
	repo.tasks[0].DemandID = &demandID
	repo.tasks[0].AcceptedPlanRevisionID = &planRevisionID
	repo.tasks[0].PlannedTaskKey = &taskKey
	var satisfiedBy []string
	if verificationMethod == "automated_test" {
		satisfiedBy = []string{taskKey}
	}
	repo.demandAcceptanceCriteria = append(repo.demandAcceptanceCriteria, DemandAcceptanceCriterion{
		TenantID:           fixture.tenantID,
		ProjectID:          fixture.projectID,
		DemandID:           demandID,
		PlanRevisionID:     planRevisionID,
		CriterionID:        criterionID,
		Statement:          statement,
		VerificationMethod: verificationMethod,
		Severity:           "blocking",
		SatisfiedBy:        satisfiedBy,
	})
}

func TestRecordResultProjectsCriterionVerdicts(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	employeeID := *repo.tasks[0].AssignedDigitalEmployeeID
	demandID := uuid.New()
	planRevisionID := uuid.New()
	demandCriterionSnapshotFixture(repo, fixture, demandID, planRevisionID, "c1", "结论可复核", "automated_test")
	// The runtime minted a real succeeded attestation for THIS attempt at
	// writeback (the employee never sees it and cannot echo it into the
	// acceptance_result). The server verifies its existence and attaches its ref.
	attestationRef := "attestation:project-task-attempt:" + fixture.attemptID.String() + ":cmd-1"
	repo.projectTaskAttestations = append(repo.projectTaskAttestations, projectTaskAttestationForFixture(fixture, fixture.attemptID, attestationRef, employeeID))

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-verdict-projection"),
		ResultContract: TaskResultContract{
			Status:  TaskResultStatusCompleted,
			Summary: "完成分析",
			AcceptanceResults: []TaskResultAcceptanceResult{
				{
					CriterionID: "c1",
					Status:      TaskResultCriterionStatusPassed,
					// No self-reported attestation ref: the employee can't know it.
				},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)

	verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), fixture.tenantID, demandID, planRevisionID)
	require.NoError(t, err)
	require.Len(t, verdicts, 1)
	verdict := verdicts[0]
	require.Equal(t, "c1", verdict.CriterionID)
	require.Equal(t, "satisfied", verdict.Verdict)
	require.Equal(t, "executor", verdict.JudgeType)
	require.Equal(t, employeeID, verdict.JudgeID)
	require.NotNil(t, verdict.ProjectTaskID)
	require.Equal(t, fixture.taskID, *verdict.ProjectTaskID)
	// The server attached the real attestation ref it verified for the attempt.
	require.Equal(t, []string{attestationRef}, verdict.EvidenceRefs)
}

// TestRecordResultProjectsCriterionVerdictsStatementOnlyMatch covers the
// matching resolution from Task 4's review: the contract gate that requires
// an AcceptanceResult per requiredAcceptanceCriteria keys on statement text
// (stringsFromCriterionMap), so an employee may legitimately echo only the
// statement and never populate CriterionID. Projection must still resolve
// that result against the right snapshot criterion via the statement fallback.
func TestRecordResultProjectsCriterionVerdictsStatementOnlyMatch(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	demandID := uuid.New()
	planRevisionID := uuid.New()
	demandCriterionSnapshotFixture(repo, fixture, demandID, planRevisionID, "c1", "结论可复核", "automated_test")
	attestationRef := "attestation:project-task-attempt:" + fixture.attemptID.String() + ":cmd-1"
	repo.projectTaskAttestations = append(repo.projectTaskAttestations, projectTaskAttestationForFixture(fixture, fixture.attemptID, attestationRef, *repo.tasks[0].AssignedDigitalEmployeeID))

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-verdict-statement-match"),
		ResultContract: TaskResultContract{
			Status:  TaskResultStatusCompleted,
			Summary: "完成分析",
			AcceptanceResults: []TaskResultAcceptanceResult{
				{
					// No CriterionID: employee echoed the statement only.
					Criterion: "结论可复核",
					Status:    TaskResultCriterionStatusPassed,
				},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)

	verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), fixture.tenantID, demandID, planRevisionID)
	require.NoError(t, err)
	require.Len(t, verdicts, 1)
	require.Equal(t, "c1", verdicts[0].CriterionID)
	require.Equal(t, "satisfied", verdicts[0].Verdict)
}

// TestProjectionScopedBySatisfiedBy is the cross-task collision guard: both
// projection and attestation tightening must only consider snapshot criteria
// whose SatisfiedBy names THIS task's planned key. Criterion-b belongs to a
// different task (task-b) and shares statement text with the employee's
// statement-echo result; without scoping, the statement fallback would match
// it — rejecting this task for task-b's missing attestation and/or projecting
// a verdict task-b never earned.
func TestProjectionScopedBySatisfiedBy(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	demandID := uuid.New()
	planRevisionID := uuid.New()
	// Own criterion: satisfied_by this task's planned key (set by the fixture).
	demandCriterionSnapshotFixture(repo, fixture, demandID, planRevisionID, "c-a", "结论可复核（本任务）", "automated_test")
	// Foreign criterion: similar statement, satisfied_by a DIFFERENT task.
	repo.demandAcceptanceCriteria = append(repo.demandAcceptanceCriteria, DemandAcceptanceCriterion{
		TenantID:           fixture.tenantID,
		ProjectID:          fixture.projectID,
		DemandID:           demandID,
		PlanRevisionID:     planRevisionID,
		CriterionID:        "c-b",
		Statement:          "结论可复核",
		VerificationMethod: "automated_test",
		Severity:           "blocking",
		SatisfiedBy:        []string{"task-b"},
	})
	// A real runtime attestation backs this attempt (server-verified, not echoed).
	attestationRef := "attestation:project-task-attempt:" + fixture.attemptID.String() + ":cmd-1"
	repo.projectTaskAttestations = append(repo.projectTaskAttestations, projectTaskAttestationForFixture(fixture, fixture.attemptID, attestationRef, *repo.tasks[0].AssignedDigitalEmployeeID))

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-satisfied-by-scoping"),
		ResultContract: TaskResultContract{
			Status:  TaskResultStatusCompleted,
			Summary: "完成分析",
			AcceptanceResults: []TaskResultAcceptanceResult{
				{
					// Own criterion, judged normally; server verifies the attempt's
					// real attestation.
					CriterionID: "c-a",
					Status:      TaskResultCriterionStatusPassed,
				},
				{
					// Statement echo that textually matches FOREIGN criterion
					// c-b, with no attestation ref: must neither trip
					// tightening for c-b nor project a verdict onto it.
					Criterion: "结论可复核",
					Status:    TaskResultCriterionStatusPassed,
				},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)
	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, TaskResultDecisionCompleteAccepted, result.Decision)
	require.Equal(t, "accepted", result.ValidationStatus)
	require.NotContains(t, result.ValidationErrors, "acceptance_result_attestation_required:c-b")

	verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), fixture.tenantID, demandID, planRevisionID)
	require.NoError(t, err)
	require.Len(t, verdicts, 1)
	require.Equal(t, "c-a", verdicts[0].CriterionID)
	require.Equal(t, "satisfied", verdicts[0].Verdict)
}

func TestAutomatedCriterionRequiresAttestationEvidence(t *testing.T) {
	t.Run("no attestation anywhere rejects with error code", func(t *testing.T) {
		repo := newProjectTaskResultMemoryRepository()
		service, err := NewService(repo)
		require.NoError(t, err)
		fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
		demandID := uuid.New()
		planRevisionID := uuid.New()
		demandCriterionSnapshotFixture(repo, fixture, demandID, planRevisionID, "c1", "结论可复核", "automated_test")

		// No attestation record exists for the attempt and verification[] carries
		// none: a green light not backed by any real execution record.
		summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
			ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-verdict-missing-attestation"),
			ResultContract: TaskResultContract{
				Status:  TaskResultStatusCompleted,
				Summary: "完成分析",
				AcceptanceResults: []TaskResultAcceptanceResult{
					{
						CriterionID:  "c1",
						Status:       TaskResultCriterionStatusPassed,
						EvidenceRefs: []string{"artifact:report"},
					},
				},
			},
		})

		require.NoError(t, err)
		require.Equal(t, fixture.taskID, summary.ProjectTaskID)
		require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
		result := requireSingleProjectTaskResult(t, repo, fixture)
		require.Equal(t, TaskResultDecisionValidationFailed, result.Decision)
		require.Equal(t, "rejected", result.ValidationStatus)
		require.Contains(t, result.ValidationErrors, "acceptance_result_attestation_required:c1")

		verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), fixture.tenantID, demandID, planRevisionID)
		require.NoError(t, err)
		require.Empty(t, verdicts)
	})

	t.Run("employee-forged attestation ref without a real record still rejects", func(t *testing.T) {
		// Anti-forgery: the employee CANNOT satisfy the gate by self-reporting an
		// attestation-shaped string in acceptance_results — only a real
		// server-side attestation record (or verification[] backed by one) counts.
		repo := newProjectTaskResultMemoryRepository()
		service, err := NewService(repo)
		require.NoError(t, err)
		fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
		demandID := uuid.New()
		planRevisionID := uuid.New()
		demandCriterionSnapshotFixture(repo, fixture, demandID, planRevisionID, "c1", "结论可复核", "automated_test")
		forgedRef := "attestation:project-task-attempt:" + fixture.attemptID.String() + ":cmd-1"

		_, err = service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
			ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-verdict-forged-attestation"),
			ResultContract: TaskResultContract{
				Status:  TaskResultStatusCompleted,
				Summary: "完成分析",
				AcceptanceResults: []TaskResultAcceptanceResult{
					{
						CriterionID:  "c1",
						Status:       TaskResultCriterionStatusPassed,
						EvidenceRefs: []string{forgedRef}, // self-reported, no backing record
					},
				},
			},
		})

		require.NoError(t, err)
		require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
		result := requireSingleProjectTaskResult(t, repo, fixture)
		require.Equal(t, "rejected", result.ValidationStatus)
		require.Contains(t, result.ValidationErrors, "acceptance_result_attestation_required:c1")

		verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), fixture.tenantID, demandID, planRevisionID)
		require.NoError(t, err)
		require.Empty(t, verdicts)
	})

	t.Run("real attestation record for the attempt passes and projects with attached ref", func(t *testing.T) {
		repo := newProjectTaskResultMemoryRepository()
		service, err := NewService(repo)
		require.NoError(t, err)
		fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
		demandID := uuid.New()
		planRevisionID := uuid.New()
		demandCriterionSnapshotFixture(repo, fixture, demandID, planRevisionID, "c1", "结论可复核", "automated_test")
		attestationRef := "attestation:project-task-attempt:" + fixture.attemptID.String() + ":cmd-1"
		repo.projectTaskAttestations = append(repo.projectTaskAttestations, projectTaskAttestationForFixture(fixture, fixture.attemptID, attestationRef, *repo.tasks[0].AssignedDigitalEmployeeID))

		summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
			ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-verdict-with-attestation"),
			ResultContract: TaskResultContract{
				Status:  TaskResultStatusCompleted,
				Summary: "完成分析",
				AcceptanceResults: []TaskResultAcceptanceResult{
					{
						CriterionID: "c1",
						Status:      TaskResultCriterionStatusPassed,
					},
				},
			},
		})

		require.NoError(t, err)
		require.Equal(t, fixture.taskID, summary.ProjectTaskID)
		require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)
		result := requireSingleProjectTaskResult(t, repo, fixture)
		require.Equal(t, TaskResultDecisionCompleteAccepted, result.Decision)
		require.Equal(t, "accepted", result.ValidationStatus)

		verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), fixture.tenantID, demandID, planRevisionID)
		require.NoError(t, err)
		require.Len(t, verdicts, 1)
		require.Equal(t, "satisfied", verdicts[0].Verdict)
		require.Equal(t, []string{attestationRef}, verdicts[0].EvidenceRefs)
	})

	t.Run("verification[] attestation ref backed by a record passes and attaches that ref", func(t *testing.T) {
		repo := newProjectTaskResultMemoryRepository()
		service, err := NewService(repo)
		require.NoError(t, err)
		fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
		demandID := uuid.New()
		planRevisionID := uuid.New()
		demandCriterionSnapshotFixture(repo, fixture, demandID, planRevisionID, "c1", "结论可复核", "automated_test")
		attestationRef := "attestation:project-task-attempt:" + fixture.attemptID.String() + ":provider_terminal:cmd-1"
		repo.projectTaskAttestations = append(repo.projectTaskAttestations, projectTaskAttestationForFixture(fixture, fixture.attemptID, attestationRef, *repo.tasks[0].AssignedDigitalEmployeeID))

		contract := TaskResultContract{
			Status:  TaskResultStatusCompleted,
			Summary: "完成分析",
			AcceptanceResults: []TaskResultAcceptanceResult{
				{CriterionID: "c1", Status: TaskResultCriterionStatusPassed},
			},
			Verification: []TaskResultVerification{{
				Type:         "command",
				Status:       TaskResultVerificationStatusPassed,
				Summary:      "命令通过",
				EvidenceRefs: []TaskResultRef{{Kind: "attestation", Ref: attestationRef}},
			}},
		}

		_, err = service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
			ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-verdict-verification-attestation"),
			ResultContract:                   contract,
		})

		require.NoError(t, err)
		require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)
		verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), fixture.tenantID, demandID, planRevisionID)
		require.NoError(t, err)
		require.Len(t, verdicts, 1)
		require.Equal(t, "satisfied", verdicts[0].Verdict)
		require.Equal(t, []string{attestationRef}, verdicts[0].EvidenceRefs)
	})
}

func TestHumanJudgmentSelfReportIgnored(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	demandID := uuid.New()
	planRevisionID := uuid.New()
	demandCriterionSnapshotFixture(repo, fixture, demandID, planRevisionID, "c2", "业务判断达标", "human_judgment")
	// Put this task into the human_judgment criterion's SatisfiedBy (planner
	// permits it: "satisfied_by may be empty" for human_judgment, i.e. it is
	// optional, not forbidden). This keeps the criterion inside
	// criteriaSatisfiedByTask's scope so the test exercises the actual
	// human_judgment ignore branch, not the SatisfiedBy scoping filter.
	repo.demandAcceptanceCriteria[len(repo.demandAcceptanceCriteria)-1].SatisfiedBy = []string{demandCriterionSnapshotTaskKey}

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-human-judgment-self-report"),
		ResultContract: TaskResultContract{
			Status:  TaskResultStatusCompleted,
			Summary: "完成分析",
			AcceptanceResults: []TaskResultAcceptanceResult{
				{
					CriterionID: "c2",
					Status:      TaskResultCriterionStatusPassed,
					// No attestation ref: human_judgment is never tightened,
					// and a self-report against it must not be projected.
				},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)

	verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), fixture.tenantID, demandID, planRevisionID)
	require.NoError(t, err)
	require.Empty(t, verdicts)
}

// TestProjectionSkippedWithoutSnapshot is the legacy guard: a task whose
// demand was never decomposed under Task 4 (no demand_acceptance_criteria
// rows, here modeled by leaving DemandID/AcceptedPlanRevisionID unset,
// mirroring pre-rollout data) must see byte-identical behavior to before this
// task — no attestation tightening, no verdict projection — even though the
// AcceptanceResult below would have failed tightening had a snapshot existed.
func TestProjectionSkippedWithoutSnapshot(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	require.Nil(t, repo.tasks[0].DemandID)
	require.Nil(t, repo.tasks[0].AcceptedPlanRevisionID)

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-no-snapshot"),
		ResultContract: TaskResultContract{
			Status:  TaskResultStatusCompleted,
			Summary: "完成分析",
			AcceptanceResults: []TaskResultAcceptanceResult{
				{
					CriterionID: "c1",
					Status:      TaskResultCriterionStatusPassed,
					// No attestation ref: would fail tightening if any
					// snapshot criterion existed, but none does.
				},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)
	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, TaskResultDecisionCompleteAccepted, result.Decision)
	require.Equal(t, "accepted", result.ValidationStatus)
	require.Empty(t, repo.demandCriterionVerdicts)
}

// TestNotApplicableAutomatedCriterionDoesNotDeadlock is the regression guard
// for the convergence-gate deadlock: an executor returning a contract-valid
// not_applicable acceptance result (with the required human reason + evidence)
// for an injected automated_test blocking criterion must PROJECT a verdict —
// now with the third verdict value not_applicable — so the gate treats that
// criterion as released rather than counting a verdict-less blocking criterion
// as permanently unsatisfied. The mandatory human_judgment fallback remains the
// only pending criterion, so the demand can still complete once the human signs
// it — no permanent stuck.
func TestNotApplicableAutomatedCriterionDoesNotDeadlock(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	employeeID := *repo.tasks[0].AssignedDigitalEmployeeID
	demandID := uuid.New()
	planRevisionID := uuid.New()
	// Injected automated_test blocking criterion the executor judges N/A.
	demandCriterionSnapshotFixture(repo, fixture, demandID, planRevisionID, "auto-check", "自动测试通过", "automated_test")
	// The mandatory human_judgment fallback backstop ("人类负责人确认交付符合需求意图").
	repo.demandAcceptanceCriteria = append(repo.demandAcceptanceCriteria, DemandAcceptanceCriterion{
		TenantID:           fixture.tenantID,
		ProjectID:          fixture.projectID,
		DemandID:           demandID,
		PlanRevisionID:     planRevisionID,
		CriterionID:        "human-fallback",
		Statement:          "人类负责人确认交付符合需求意图",
		VerificationMethod: "human_judgment",
		Severity:           "blocking",
	})

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-not-applicable-no-deadlock"),
		ResultContract: TaskResultContract{
			Status:  TaskResultStatusCompleted,
			Summary: "完成分析",
			AcceptanceResults: []TaskResultAcceptanceResult{
				{
					CriterionID:         "auto-check",
					Status:              TaskResultCriterionStatusNotApplicable,
					HumanAcceptedReason: "该自动检查不适用于本次交付范围",
					EvidenceRefs:        []string{"artifact:na-rationale"},
				},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	// The task completes cleanly — not_applicable is not attestation-gated.
	require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)
	result := requireSingleProjectTaskResult(t, repo, fixture)
	require.Equal(t, "accepted", result.ValidationStatus)

	// The not_applicable verdict is now PROJECTED (previously dropped, which is
	// what stranded the gate).
	verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), fixture.tenantID, demandID, planRevisionID)
	require.NoError(t, err)
	require.Len(t, verdicts, 1)
	require.Equal(t, "auto-check", verdicts[0].CriterionID)
	require.Equal(t, "not_applicable", verdicts[0].Verdict)
	require.Equal(t, "executor", verdicts[0].JudgeType)
	require.Equal(t, employeeID, verdicts[0].JudgeID)
	require.Equal(t, "该自动检查不适用于本次交付范围", verdicts[0].Reason)

	// Gate: the not_applicable automated criterion is released; only the unsigned
	// human_judgment fallback remains pending. The demand is NOT deadlocked.
	criteria, err := repo.ListDemandAcceptanceCriteria(context.Background(), fixture.tenantID, demandID, planRevisionID)
	require.NoError(t, err)
	pending := ResolveUnsatisfiedBlockingCriteria(criteria, verdicts)
	require.Equal(t, []string{"human-fallback"}, pending)
}

// TestResolveUnsatisfiedBlockingCriteriaReleasesNotApplicable pins the gate
// resolver matrix for the third verdict value: satisfied and not_applicable
// both RELEASE a blocking criterion; unsatisfied and no-verdict both BLOCK; and
// a human verdict still overrides an executor not_applicable in both directions.
func TestResolveUnsatisfiedBlockingCriteriaReleasesNotApplicable(t *testing.T) {
	criteria := []DemandAcceptanceCriterion{{CriterionID: "a", Severity: "blocking"}}
	verdict := func(v, judge string) []DemandCriterionVerdict {
		return []DemandCriterionVerdict{{CriterionID: "a", Verdict: v, JudgeType: judge}}
	}
	cases := []struct {
		name     string
		verdicts []DemandCriterionVerdict
		released bool
	}{
		{"executor satisfied releases", verdict("satisfied", "executor"), true},
		{"executor not_applicable releases", verdict("not_applicable", "executor"), true},
		{"executor unsatisfied blocks", verdict("unsatisfied", "executor"), false},
		{"no verdict blocks", nil, false},
		{"human satisfied overrides executor not_applicable (released)", []DemandCriterionVerdict{
			{CriterionID: "a", Verdict: "not_applicable", JudgeType: "executor"},
			{CriterionID: "a", Verdict: "satisfied", JudgeType: "human"},
		}, true},
		{"human unsatisfied overrides executor not_applicable (blocks)", []DemandCriterionVerdict{
			{CriterionID: "a", Verdict: "not_applicable", JudgeType: "executor"},
			{CriterionID: "a", Verdict: "unsatisfied", JudgeType: "human"},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pending := ResolveUnsatisfiedBlockingCriteria(criteria, tc.verdicts)
			if tc.released {
				require.Empty(t, pending)
			} else {
				require.Equal(t, []string{"a"}, pending)
			}
		})
	}
}

func TestStringRefIsAttestation(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		want bool
	}{
		{"attestation prefix", "attestation:project-task-attempt:x:cmd-1", true},
		{"attestation prefix with leading/trailing whitespace", "  attestation:cmd-1  ", true},
		{"no prefix", "artifact:report", false},
		{"empty", "", false},
		{"prefix without colon", "attestationcmd-1", false},
		{"case mismatch is not normalized", "ATTESTATION:cmd-1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, stringRefIsAttestation(tc.ref))
		})
	}
}

func TestSubmitProjectTaskAttemptResultRevisionNeededWaitsForHumanAndKeepsLatestResult(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := TaskResultContract{
		Status:  TaskResultStatusRevisionNeeded,
		Summary: "需要负责人确认修订范围",
		RevisionRequest: &TaskResultRevisionRequest{
			Reason:                 "验收口径需要补充",
			RecommendedTaskSummary: "补充缺失证据后重试当前任务",
			RequestedChanges:       []string{"补充验收证据"},
		},
	}

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-revision"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, "需要负责人确认修订范围", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingReason)
	require.Equal(t, HumanWaitReasonClarification, *repo.tasks[0].WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_clarification", repo.decisionRequests[0].DecisionType)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultDecisionRevisionAttempt, results[0].Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestSubmitProjectTaskAttemptResultRevisionTaskWaitsWithPlanInvalidAndKeepsLatestResult(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := TaskResultContract{
		Status:  TaskResultStatusRevisionNeeded,
		Summary: "任务契约需要调整后再执行",
		RevisionRequest: &TaskResultRevisionRequest{
			Reason:                 "原交接契约已不适配",
			RecommendedTaskSummary: "调整交接契约后重新执行",
			ContractChanged:        true,
			RequestedChanges:       []string{"调整验收标准"},
		},
	}

	_, err = service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-revision-task"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingReason)
	require.Equal(t, HumanWaitReasonPlanInvalid, *repo.tasks[0].WaitingReason)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, TaskResultDecisionRevisionTask, results[0].Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestSubmitProjectTaskAttemptResultBlockedWaitsForHumanAndKeepsLatestResult(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := TaskResultContract{
		Status:  TaskResultStatusBlocked,
		Summary: "缺少客户授权",
		Blocker: &TaskResultBlocker{
			Reason:           "permission_required",
			ResolutionPrompt: "请负责人补充客户系统访问授权",
			RequiredBy:       "human_owner",
			ContextRefs:      []TaskResultRef{{Kind: "missing_context", Ref: "customer-permission"}},
		},
	}

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-blocked"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, "缺少客户授权", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingReason)
	require.Equal(t, HumanWaitReasonPermissionRequired, *repo.tasks[0].WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_permission", repo.decisionRequests[0].DecisionType)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultDecisionBlockedWaitingHuman, results[0].Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestSubmitProjectTaskAttemptResultBlockedResolvableUpstreamSignalsCoordinatorWithoutHumanWait(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	repo.tasks[0].InputRequirements = map[string]any{"required_inputs": []any{"load_test_report"}}
	contract := TaskResultContract{
		Status:  TaskResultStatusBlocked,
		Summary: "缺少上游产出 load_test_report",
		Blocker: &TaskResultBlocker{
			Reason:           "缺少 load_test_report 数据",
			ResolutionPrompt: "需要上游任务补充 load_test_report",
			RequiredBy:       "upstream_producer",
			MissingInputs:    []string{"load_test_report"},
		},
	}

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-blocked-resolvable-upstream"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, "缺少上游产出 load_test_report", summary.Conclusion)
	// Unlike blocked_waiting_human, this decision must not open a human decision:
	// the platform resolves it autonomously via CreateUpstreamSupplementTasks.
	require.Empty(t, repo.decisionRequests)
	require.Equal(t, 1, coordinator.completedSignals)
	require.Equal(t, fixture.taskID, coordinator.lastCompleted.ProjectTaskID)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultDecisionBlockedResolvableUpstream, results[0].Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestSubmitProjectTaskAttemptResultRetryableFailedQueuesRetryAndKeepsLatestResult(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	maxAttempts := int32(3)
	repo.tasks[0].AttemptCount = 1
	repo.tasks[0].MaxAttempts = &maxAttempts
	retryable := true
	contract := TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: "Provider 中途退出",
		Failure: &TaskResultFailure{
			// B 层(provider 真跑过)才允许自动重排;A 层平台启动类失败一律等人。
			ErrorFamily:            FailureFamilyTransientProvider,
			Retryable:              &retryable,
			RecoveryRecommendation: "retry_original_attempt",
			Message:                "provider exited mid-run",
		},
	}

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-failed-retryable"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, "Provider 中途退出", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusQueued, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].CurrentAttemptID)
	require.NotEqual(t, fixture.attemptID, *repo.tasks[0].CurrentAttemptID)
	require.Equal(t, int32(2), repo.tasks[0].AttemptCount)
	require.Equal(t, ProjectTaskAttemptStatusFailed, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.projectTaskAttempts, 2)
	require.Equal(t, ProjectTaskAttemptStatusQueued, repo.projectTaskAttempts[1].Status)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultDecisionFailedRetryable, results[0].Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestSubmitProjectTaskAttemptResultReplanRequestWaitsWithPlanInvalidAndKeepsLatestResult(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	maxAttempts := int32(3)
	repo.tasks[0].AttemptCount = 1
	repo.tasks[0].MaxAttempts = &maxAttempts
	retryable := true
	contract := TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: "当前计划无法继续",
		Failure: &TaskResultFailure{
			ErrorFamily:            FailureFamilyPlanInvalid,
			Retryable:              &retryable,
			RecoveryRecommendation: "request_replan",
			Message:                "dependency graph changed",
		},
		ReplanRequest: &TaskResultReplanRequest{
			Reason:      "依赖关系已变化",
			Scope:       "project",
			Constraints: []string{"保留已有结果"},
		},
	}

	_, err = service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-replan"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingReason)
	require.Equal(t, HumanWaitReasonPlanInvalid, *repo.tasks[0].WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultDecisionReplanRequested, results[0].Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestSubmitProjectTaskAttemptResultNonRetryableFailedSignalsFailureAndKeepsLatestResult(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	retryable := false
	contract := TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: "输出契约无法解析",
		Failure: &TaskResultFailure{
			ErrorFamily:            FailureFamilyNonRetryableExecution,
			Retryable:              &retryable,
			RecoveryRecommendation: "manual_recovery_required",
			Message:                "missing required result fields",
		},
	}

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-failed-final"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, "输出契约无法解析", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusFailed, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusFailed, repo.projectTaskAttempts[0].Status)
	require.Equal(t, "输出契约无法解析", *repo.projectTaskAttempts[0].FailureMessage)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 1, coordinator.failedSignals)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultDecisionFailedRecovery, results[0].Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestSubmitProjectTaskAttemptResultCancelledTerminalizesWithoutFailureSignalAndKeepsLatestResult(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := TaskResultContract{
		Status:  TaskResultStatusCancelled,
		Summary: "业务负责人取消当前任务",
		Cancellation: &TaskResultCancellation{
			Reason:      "需求已取消",
			CancelledBy: "human_owner",
		},
	}

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-cancelled"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, "业务负责人取消当前任务", summary.Conclusion)
	require.Equal(t, ProjectTaskStatusCancelled, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusFailed, repo.projectTaskAttempts[0].Status)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, 0, coordinator.failedSignals)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultDecisionCancelledTerminal, results[0].Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func validCompletedTaskResultContract() TaskResultContract {
	return TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "完成分析",
		AcceptanceResults: []TaskResultAcceptanceResult{
			{
				Criterion:    "输出结论",
				Status:       TaskResultCriterionStatusPassed,
				EvidenceRefs: []string{"artifact:report"},
			},
		},
		EvidenceRefs: []TaskResultRef{{Type: "report", Ref: "artifact:report"}},
		ArtifactRefs: []TaskResultRef{{Type: "markdown", Ref: "artifact:analysis-report"}},
		Verification: []TaskResultVerification{{Type: "command", Status: TaskResultVerificationStatusPassed, Summary: "命令通过"}},
	}
}

func completedContractWithRuntimeAttestationRef(attestationRef string) TaskResultContract {
	contract := validCompletedTaskResultContract()
	contract.Verification = []TaskResultVerification{{
		Type:    "command",
		Status:  TaskResultVerificationStatusPassed,
		Summary: "命令通过",
		EvidenceRefs: []TaskResultRef{{
			Kind: "attestation",
			Type: "runtime_command",
			Ref:  attestationRef,
		}},
	}}
	return contract
}

func projectTaskAttestationForFixture(fixture projectTaskAttemptServiceFixture, attemptID uuid.UUID, attestationRef string, digitalEmployeeID uuid.UUID) ProjectTaskAttestation {
	now := time.Now().UTC()
	idempotencyKey := strings.TrimPrefix(attestationRef, "attestation:")
	return ProjectTaskAttestation{
		ID:                uuid.New(),
		TenantID:          fixture.tenantID,
		ProjectID:         fixture.projectID,
		ProjectTaskID:     fixture.taskID,
		AttemptID:         attemptID,
		RuntimeNodeID:     fixture.nodeID,
		DigitalEmployeeID: digitalEmployeeID,
		AttestationType:   "provider_terminal",
		Status:            ProjectTaskAttestationStatusSucceeded,
		IdempotencyKey:    idempotencyKey,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func completeProjectTaskAttemptIntoHumanReviewResult(t *testing.T, service *Service, repo *projectTaskResultMemoryRepository, idempotencyKey string) (projectTaskAttemptServiceFixture, ExecutionSummary, DecisionRequest, ProjectTaskResult, TaskResultContract) {
	t.Helper()

	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	// §5.2: downstream_release only opens when a live dependent needs this output;
	// wire one so the provider HumanReviewRequest below actually gates.
	addFixtureDownstreamDependency(repo.memoryRepository, fixture)
	contract := validCompletedTaskResultContract()
	contract.HumanReviewRequest = &TaskResultHumanReviewRequest{
		Reason:     "需要负责人确认验收口径",
		Prompt:     "请确认是否接受该结果",
		Options:    []string{"accept", "request_revision"},
		RequiredBy: "human_owner",
		ReviewType: "acceptance",
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest(idempotencyKey),
		Conclusion:                       "legacy conclusion",
		ResultContract:                   &contract,
	})
	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingRequestID)
	require.Len(t, repo.decisionRequests, 1)
	decision := repo.decisionRequests[0]
	require.Equal(t, "project_task_acceptance", decision.DecisionType)
	require.Equal(t, decision.ID, *repo.tasks[0].WaitingRequestID)

	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, TaskResultDecisionWaitingHumanReview, results[0].Decision)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
	return fixture, *summary, decision, results[0], contract
}

func requireProjectTaskResultByID(t *testing.T, results []ProjectTaskResult, id uuid.UUID) ProjectTaskResult {
	t.Helper()
	for _, result := range results {
		if result.ID == id {
			return result
		}
	}
	t.Fatalf("project task result %s not found in %#v", id, results)
	return ProjectTaskResult{}
}

func lastProjectEventOfType(t *testing.T, events []ProjectEvent, eventType ProjectEventType) ProjectEvent {
	t.Helper()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].EventType == eventType {
			return events[i]
		}
	}
	t.Fatalf("project event type %s not found in %#v", eventType, events)
	return ProjectEvent{}
}

func TestCompleteProjectTaskAttemptWritesLedgerEvents(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	_, err = service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("complete-ledger"),
		Conclusion:                       "验收证据已生成",
		EvidenceRefs:                     []any{map[string]any{"ref": "evidence://complete"}},
		ArtifactRefs:                     []any{map[string]any{"ref": "artifact://complete"}},
		ConfidenceFactors:                map[string]any{"verified": true},
		MissingInformation:               []any{},
		RecommendedNextAction:            "进入验收",
	})
	require.NoError(t, err)
	requireLedgerEventTypes(t, repo.executionLedgerEvents, ExecutionLedgerEventAttemptCompleted, ExecutionLedgerEventSummaryCreated)
}

func TestCompleteProjectTaskAttemptAcceptanceBeforeCompletedWritesLedgerEvents(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, nil, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	high := "high"
	repo.tasks[0].RiskLevel = &high
	// §5.2: high-risk gates only with a live downstream dependent.
	addFixtureDownstreamDependency(repo.memoryRepository, fixture)

	_, err = service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("complete-high-risk-1"),
		Conclusion:                       "候选结果已完成",
	})

	require.NoError(t, err)
	task := repo.tasks[0]
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.WaitingReason)
	require.Equal(t, HumanWaitReasonAcceptanceRequired, *task.WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusSucceeded, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_acceptance", repo.decisionRequests[0].DecisionType)
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, repo.decisionRequests[0].ID, inbox.upserts[0].ID)
	require.Equal(t, repo.decisionRequests[0].ProjectTaskID, inbox.upserts[0].ProjectTaskID)
	require.Len(t, repo.executionLedgerEvents, 2)
	completedEvent := repo.executionLedgerEvents[0]
	summaryEvent := repo.executionLedgerEvents[1]
	require.Equal(t, ExecutionLedgerEventAttemptCompleted, completedEvent.EventType)
	require.Equal(t, fixture.attemptID.String(), completedEvent.SourceID)
	require.Equal(t, "project_task_attempt:"+fixture.attemptID.String()+":attempt.completed", completedEvent.IdempotencyKey)
	require.Equal(t, true, completedEvent.Metadata["requires_human_review"])
	require.Equal(t, ExecutionLedgerEventSummaryCreated, summaryEvent.EventType)
	require.Equal(t, repo.executionSummaries[0].ID.String(), summaryEvent.SourceID)
	require.Equal(t, "project_execution_summary:"+repo.executionSummaries[0].ID.String()+":summary.created", summaryEvent.IdempotencyKey)
	require.Equal(t, true, summaryEvent.Metadata["requires_human_review"])
}

func TestResolveProjectTaskHumanWaitAcceptanceApprovedCompletesTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusWaitingHuman, ProjectTaskAttemptStatusSucceeded)
	reason := HumanWaitReasonAcceptanceRequired
	repo.tasks[0].WaitingReason = &reason
	waitingRequestID := uuid.New()
	repo.tasks[0].WaitingRequestID = &waitingRequestID

	task, err := service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionApprove,
		ResponseSummary: "验收通过",
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusCompleted, task.Status)
}

func TestResolveProjectTaskHumanWaitResumeSameTaskCreatesQueuedAttempt(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusWaitingHuman, ProjectTaskAttemptStatusWaitingHuman)
	reason := HumanWaitReasonMissingContext
	repo.tasks[0].WaitingReason = &reason
	repo.tasks[0].AttemptCount = 1

	task, err := service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionResumeSameTask,
		ResponseSummary: "已补充上下文",
		ContextRefs:     []any{"customer_scope"},
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, task.Status)
	require.NotEqual(t, fixture.attemptID, *task.CurrentAttemptID)
	require.Equal(t, int32(2), task.AttemptCount)
}

func TestResolveProjectTaskHumanWaitMarkFailedFailsTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusWaitingHuman, ProjectTaskAttemptStatusWaitingHuman)
	reason := HumanWaitReasonClarification
	repo.tasks[0].WaitingReason = &reason

	task, err := service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionMarkFailed,
		ResponseSummary: "无法继续",
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusFailed, task.Status)
}

func TestFailProjectTaskAttemptFailsTaskAndAttempt(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	retryable := true

	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-fail-1"),
		FailureSummary:                   "provider crashed",
		FailureFamily:                    "runtime_agent_failure",
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusFailed, task.Status)
	require.Equal(t, ProjectTaskAttemptStatusFailed, repo.projectTaskAttempts[0].Status)
	require.Equal(t, "runtime_agent_failure", *repo.projectTaskAttempts[0].FailureFamily)
	require.Equal(t, "provider crashed", *repo.projectTaskAttempts[0].FailureMessage)
	require.Equal(t, ProjectEventTaskFailed, repo.eventTypes[len(repo.eventTypes)-1])
}

func TestFailProjectTaskAttemptWritesLedgerEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	retryable := true

	_, err = service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-ledger"),
		FailureSummary:                   "provider crashed",
		FailureFamily:                    "runtime_agent_failure",
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Len(t, repo.executionLedgerEvents, 1)
	event := repo.executionLedgerEvents[0]
	require.Equal(t, ExecutionLedgerEventAttemptFailed, event.EventType)
	require.Equal(t, fixture.attemptID, *event.ProjectTaskAttemptID)
	require.Equal(t, "runtime_agent_failure", *event.ErrorFamily)
	require.Equal(t, "provider crashed", *event.ErrorMessage)
	require.NotNil(t, event.Retryable)
	require.True(t, *event.Retryable)
}

func TestFailProjectTaskAttemptTransientProviderSchedulesRetry(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	// Prior dispatch binding must not leak onto the retry attempt.
	oldRunID := uuid.New()
	oldRuntimeTaskID := uuid.New()
	repo.projectTaskAttempts[0].DigitalEmployeeRunID = &oldRunID
	repo.projectTaskAttempts[0].RuntimeTaskID = &oldRuntimeTaskID
	repo.tasks[0].DigitalEmployeeRunID = &oldRunID
	repo.tasks[0].RuntimeTaskID = &oldRuntimeTaskID
	maxAttempts := int32(3)
	repo.tasks[0].AttemptCount = 1
	repo.tasks[0].MaxAttempts = &maxAttempts
	retryable := true

	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-transient-1"),
		FailureSummary:                   "provider exited mid-run",
		FailureFamily:                    FailureFamilyTransientProvider,
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, task.Status)
	require.NotNil(t, task.CurrentAttemptID)
	require.NotEqual(t, fixture.attemptID, *task.CurrentAttemptID)
	require.Equal(t, int32(2), task.AttemptCount)
	require.Nil(t, task.DigitalEmployeeRunID, "retry must clear task-level run binding")
	require.Nil(t, task.RuntimeTaskID, "retry must clear task-level runtime_task binding")
	retryAttempt, err := repo.GetProjectTaskAttempt(context.Background(), fixture.tenantID, *task.CurrentAttemptID)
	require.NoError(t, err)
	require.Nil(t, retryAttempt.DigitalEmployeeRunID, "retry attempt must not reuse prior digital_employee_run_id")
	require.Nil(t, retryAttempt.RuntimeTaskID, "retry attempt must not reuse prior runtime_task_id")
	require.Len(t, repo.executionLedgerEvents, 1)
	require.Equal(t, (*task.CurrentAttemptID).String(), repo.executionLedgerEvents[0].Metadata["retry_project_task_attempt_id"])
	require.Equal(t, 1, coordinator.retrySignals, "requeue must wake coordinator for redispatch")
	require.Equal(t, task.ID, coordinator.lastRetry.ProjectTaskID)
	require.Equal(t, 0, coordinator.failedSignals, "must not open human-recovery signal on auto-retry")
}

// A 层(平台启动类:派发/拉起/租约/启动超时/runtime 短暂不可用)失败一律等人,
// 系统绝不自愈重排——人在恢复卡上点了才重来。这条不成立时,节点抖动会让任务
// 在一个可能仍然坏的落点上反复自动重跑。
func TestFailProjectTaskAttemptTransientRuntimeWaitsForHumanInsteadOfAutoRetry(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	maxAttempts := int32(3)
	repo.tasks[0].AttemptCount = 1
	repo.tasks[0].MaxAttempts = &maxAttempts
	retryable := true

	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-transient-runtime-1"),
		FailureSummary:                   "runtime node restarted",
		FailureFamily:                    FailureFamilyTransientRuntime,
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status,
		"预算未用尽也不得自动重排:A 层失败要人来判断落点是否还能用")
	require.Equal(t, int32(1), task.AttemptCount, "等人期间不得消耗重试预算")
	require.Equal(t, 0, coordinator.retrySignals, "A 层失败不得唤醒协调线程重排")
	require.Equal(t, ProjectTaskAttemptStatusLost, repo.projectTaskAttempts[0].Status)
}

func TestFailProjectTaskAttemptWaitingHumanUsesPrimaryNonNoiseAttribution(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	maxAttempts := int32(1)
	repo.tasks[0].AttemptCount = 1
	repo.tasks[0].MaxAttempts = &maxAttempts

	// Prior real provider failure (attempt 0 in fixture history).
	priorFamily := FailureFamilyTransientProvider
	priorMsg := "provider exited without a terminal event"
	priorCode := "PROVIDER_NO_TERMINAL_EVENT"
	repo.projectTaskAttempts = append([]ProjectTaskAttempt{{
		ID:             uuid.New(),
		TenantID:       fixture.tenantID,
		ProjectTaskID:  fixture.taskID,
		AttemptNo:      1,
		Status:         ProjectTaskAttemptStatusFailed,
		FailureFamily:  &priorFamily,
		FailureMessage: &priorMsg,
		ErrorCode:      &priorCode,
		LeaseToken:     "prior-lease",
		IdempotencyKey: "prior-fail",
		CreatedAt:      time.Now().UTC().Add(-time.Minute),
		UpdatedAt:      time.Now().UTC().Add(-time.Minute),
	}}, repo.projectTaskAttempts...)
	// Current fixture attempt is #2 and is about to be marked lost by watchdog.
	repo.projectTaskAttempts[1].AttemptNo = 2

	retryable := true
	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-noise-cover"),
		FailureSummary:                   "Runtime did not acknowledge project task attempt start before deadline",
		FailureFamily:                    FailureFamilyTransientRuntime,
		Retryable:                        &retryable,
		ErrorCode:                        "",
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.Len(t, inbox.upserts, 1)
	require.NotNil(t, inbox.upserts[0].SummarySnapshot)
	summary := *inbox.upserts[0].SummarySnapshot
	require.Contains(t, summary, "执行器启动或运行失败")
	require.Contains(t, summary, "PROVIDER_NO_TERMINAL_EVENT")
	require.Contains(t, summary, "provider exited without a terminal event")
	require.NotContains(t, summary, "Runtime did not acknowledge")
}

func TestFailProjectTaskAttemptRetryExhaustionMovesToWaitingHuman(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	maxAttempts := int32(3)
	repo.tasks[0].AttemptCount = 3
	repo.tasks[0].MaxAttempts = &maxAttempts
	retryable := true

	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-exhausted-1"),
		FailureSummary:                   "provider timed out repeatedly",
		FailureFamily:                    FailureFamilyTimeout,
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.WaitingReason)
	require.Equal(t, HumanWaitReasonClarification, *task.WaitingReason)
	require.Equal(t, fixture.attemptID, *task.CurrentAttemptID)
	require.Equal(t, int32(3), task.AttemptCount)
	require.Equal(t, ProjectTaskAttemptStatusTimedOut, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.executionLedgerEvents, 1)
	require.NotContains(t, repo.executionLedgerEvents[0].Metadata, "retry_project_task_attempt_id")
}

// TestFailProjectTaskAttemptWaitingHumanEmitsHumanTask pins the sister-F1 fix
// (handoff §6): a failed attempt that parks the task waiting_human must create a
// decision request AND project it to the inbox, so the human has an actionable
// card instead of a silently blocked task.
func TestFailProjectTaskAttemptWaitingHumanEmitsHumanTask(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)

	// ApprovalRequired (retryable unset) routes to waiting_human via
	// projectTaskFailureAction.
	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-approval-required"),
		FailureSummary:                   "需要负责人批准高风险动作",
		FailureFamily:                    FailureFamilyApprovalRequired,
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.Len(t, repo.decisionRequests, 1, "waiting_human must create a decision request (sister-F1)")
	require.Equal(t, "project_task_approval", repo.decisionRequests[0].DecisionType)
	require.Equal(t, fixture.taskID, *repo.decisionRequests[0].ProjectTaskID)
	require.NotNil(t, task.WaitingRequestID, "waiting_request_id must link the decision (a144f12d class)")
	require.Equal(t, repo.decisionRequests[0].ID, *task.WaitingRequestID)
	require.NotNil(t, repo.tasks[0].WaitingRequestID)
	require.Equal(t, repo.decisionRequests[0].ID, *repo.tasks[0].WaitingRequestID)
	require.Len(t, inbox.upserts, 1, "the decision must be projected to the inbox")
	require.Equal(t, repo.decisionRequests[0].ID, inbox.upserts[0].ID)
}

func TestFailProjectTaskAttemptNonRetryableExecutionFailsTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	retryable := false

	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-non-retryable-1"),
		FailureSummary:                   "output contract cannot be parsed",
		FailureFamily:                    FailureFamilyNonRetryableExecution,
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusFailed, task.Status)
	require.Equal(t, ProjectTaskAttemptStatusFailed, repo.projectTaskAttempts[0].Status)
	require.Equal(t, "output contract cannot be parsed", *repo.projectTaskAttempts[0].FailureMessage)
}

func TestProjectTaskDispatchRecoveryActionSchedulesRetryForRetryableFailure(t *testing.T) {
	task := ProjectTask{
		ID:          uuid.New(),
		Status:      ProjectTaskStatusPlanned,
		MaxAttempts: serviceTestInt32Ptr(3),
	}
	event := ProjectEvent{
		ID:        uuid.New(),
		EventType: ProjectEventTaskDispatchFailed,
		Payload: map[string]any{
			"retryable": true,
			"error":     "runtime node is not connected",
		},
	}

	// A-layer: no automatic re-dispatch; human recovery card only.
	action := projectTaskDispatchRecoveryAction(task, event, 1, 0, 0)

	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, action.Action)
	require.Equal(t, FailureFamilyTransientRuntime, action.FailureFamily)
	require.True(t, action.Retryable)
	require.Equal(t, HumanWaitReasonRuntimeRecovery, action.WaitingReason)
}

func TestProjectTaskDispatchRecoveryActionMovesNonRetryableFailureToWaitingHuman(t *testing.T) {
	task := ProjectTask{
		ID:          uuid.New(),
		Status:      ProjectTaskStatusPlanned,
		MaxAttempts: serviceTestInt32Ptr(3),
	}
	event := ProjectEvent{
		ID:        uuid.New(),
		EventType: ProjectEventTaskDispatchFailed,
		Payload: map[string]any{
			"retryable": false,
			"error":     "invalid run input",
		},
	}

	action := projectTaskDispatchRecoveryAction(task, event, 1, 0, 0)

	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, action.Action)
	require.Equal(t, FailureFamilyInvalidContract, action.FailureFamily)
	require.False(t, action.Retryable)
	require.Equal(t, HumanWaitReasonPlanInvalid, action.WaitingReason)
}

func TestProjectTaskDispatchRecoveryActionMovesRetryExhaustionToWaitingHuman(t *testing.T) {
	task := ProjectTask{
		ID:          uuid.New(),
		Status:      ProjectTaskStatusPlanned,
		MaxAttempts: serviceTestInt32Ptr(3),
	}
	event := ProjectEvent{
		ID:        uuid.New(),
		EventType: ProjectEventTaskDispatchFailed,
		Payload: map[string]any{
			"retryable": true,
			"error":     "runtime node is not connected",
		},
	}

	// count=2 exceeds MaxDispatchAutoRetries(1) with no prior human approval.
	action := projectTaskDispatchRecoveryAction(task, event, 2, 0, 0)

	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, action.Action)
	require.Equal(t, FailureFamilyTransientRuntime, action.FailureFamily)
	require.Equal(t, HumanWaitReasonRuntimeRecovery, action.WaitingReason)
}

func TestProjectTaskDispatchRecoveryActionFailsAfterHumanRedispatchBudget(t *testing.T) {
	task := ProjectTask{
		ID:          uuid.New(),
		Status:      ProjectTaskStatusPlanned,
		MaxAttempts: serviceTestInt32Ptr(3),
	}
	event := ProjectEvent{
		ID:        uuid.New(),
		EventType: ProjectEventTaskDispatchFailed,
		Payload: map[string]any{
			"retryable": true,
			"error":     "runtime node is not connected",
		},
	}
	action := projectTaskDispatchRecoveryAction(task, event, 2, 1, 0)
	require.Equal(t, ProjectTaskRecoveryActionFailed, action.Action)
	require.Contains(t, action.TerminalReason, "人类恢复")
}

type dispatchRecoveryFixture struct {
	tenantID       uuid.UUID
	projectID      uuid.UUID
	taskID         uuid.UUID
	failureEventID uuid.UUID
}

func newDispatchRecoveryFixture(repo *memoryRepository, taskStatus string, attemptCount, maxAttempts int32, retryable bool) dispatchRecoveryFixture {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	now := time.Now().UTC()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "Dispatch recovery",
		Goal:             "Recover dispatch",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "dispatch recovery task",
		Status:                    taskStatus,
		AssignedDigitalEmployeeID: &employeeID,
		AttemptCount:              attemptCount,
		MaxAttempts:               &maxAttempts,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	})
	eventID := uuid.New()
	repo.events = append(repo.events, ProjectEvent{
		ID:        eventID,
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventTaskDispatchFailed,
		ActorType: "project_coordinator",
		ActorID:   taskID.String(),
		Payload: map[string]any{
			"project_task_id": taskID.String(),
			"retryable":       retryable,
			"error":           map[bool]string{true: "runtime node is not connected", false: "invalid run input"}[retryable],
		},
		CreatedAt: now,
	})
	return dispatchRecoveryFixture{tenantID: tenantID, projectID: projectID, taskID: taskID, failureEventID: eventID}
}

func TestRecoverProjectTaskDispatchFailureOpensHumanCardWithoutAutoRetry(t *testing.T) {
	// A-layer dispatch failure: never auto-retry; open recovery card for human click.
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newDispatchRecoveryFixture(repo, ProjectTaskStatusPlanned, 0, 3, true)

	result, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:       fixture.tenantID,
		ProjectID:      fixture.projectID,
		ProjectTaskID:  fixture.taskID,
		FailureEventID: fixture.failureEventID,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, result.Action)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.Nil(t, task.RetryNotBefore, "must not schedule auto re-dispatch")
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, "project_task_recovery", inbox.upserts[0].DecisionType)
}

func TestRecoverProjectTaskDispatchFailureCreatesWaitingHumanDecision(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newDispatchRecoveryFixture(repo, ProjectTaskStatusPlanned, 0, 3, false)

	result, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:       fixture.tenantID,
		ProjectID:      fixture.projectID,
		ProjectTaskID:  fixture.taskID,
		FailureEventID: fixture.failureEventID,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, result.Action)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.WaitingRequestID)
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, "project_task_recovery", inbox.upserts[0].DecisionType)
}

func TestRecoverProjectTaskDispatchFailureNoopsWhenDecisionPending(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newDispatchRecoveryFixture(repo, ProjectTaskStatusPlanned, 0, 3, false)

	first, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, first.Action)

	second, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionNoop, second.Action)
	require.Len(t, inbox.upserts, 1)
	require.Len(t, repo.decisionRequests, 1)
}

func TestRecoverProjectTaskDispatchFailureExhaustsRetriesByFailureCount(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	// Fixture already has 1 dispatch_failed; add one more so count=2 > MaxDispatchAutoRetries(1).
	fixture := newDispatchRecoveryFixture(repo, ProjectTaskStatusPlanned, 0, 3, true)
	repo.events = append(repo.events, ProjectEvent{
		ID:        uuid.New(),
		TenantID:  fixture.tenantID,
		ProjectID: fixture.projectID,
		EventType: ProjectEventTaskDispatchFailed,
		ActorType: "project_coordinator",
		ActorID:   fixture.taskID.String(),
		Payload: map[string]any{
			"project_task_id": fixture.taskID.String(),
			"retryable":       true,
			"error":           "runtime node is not connected",
		},
		CreatedAt: time.Now().UTC(),
	})

	result, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionWaitingHuman, result.Action)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
}

// Second dispatch failure after one human recovery approval must fail the task
// instead of minting another recovery card (human redispatch budget = 1).
func TestRecoverProjectTaskDispatchFailureFailsAfterHumanRedispatchBudget(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newDispatchRecoveryFixture(repo, ProjectTaskStatusPlanned, 0, 3, true)
	// Prior human recovery already approved once for this task.
	taskID := fixture.taskID
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:             uuid.New(),
		TenantID:       fixture.tenantID,
		ProjectID:      fixture.projectID,
		ProjectTaskID:  &taskID,
		DecisionType:   "project_task_recovery",
		TitleSnapshot:  "prior recovery",
		StatusSnapshot: "approved",
	})
	// Exhaust auto-retry: second dispatch failure.
	repo.events = append(repo.events, ProjectEvent{
		ID:        uuid.New(),
		TenantID:  fixture.tenantID,
		ProjectID: fixture.projectID,
		EventType: ProjectEventTaskDispatchFailed,
		ActorType: "project_coordinator",
		ActorID:   fixture.taskID.String(),
		Payload: map[string]any{
			"project_task_id": fixture.taskID.String(),
			"retryable":       true,
			"error":           "runtime node is not connected",
		},
		CreatedAt: time.Now().UTC(),
	})

	result, err := service.RecoverProjectTaskDispatchFailure(context.Background(), RecoverProjectTaskDispatchFailureRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskRecoveryActionFailed, result.Action)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusFailed, task.Status)
	require.Empty(t, inbox.upserts, "must not mint another recovery card")
}

func TestProjectTaskDispatchRetryAvailableNeverAutos(t *testing.T) {
	task := ProjectTask{}
	require.False(t, projectTaskDispatchRetryAvailable(task, 1, 3), "A-layer: no auto re-dispatch")
	require.False(t, projectTaskDispatchRetryAvailable(task, 0, 3))
	require.Equal(t, int64(0), MaxDispatchAutoRetries)
}

func TestRecoverStaleQueuedProjectTaskAttemptOpensHumanCard(t *testing.T) {
	// Runtime never acked start (A-layer): no auto-requeue; human recovery only.
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	repo.tasks[0].MaxAttempts = serviceTestInt32Ptr(3)

	result, err := service.RecoverStaleQueuedProjectTaskAttempt(context.Background(), RecoverProjectTaskAttemptRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		AttemptID:     fixture.attemptID,
		FailureFamily: FailureFamilyRuntimeStartTimeout,
		Summary:       "Runtime did not acknowledge start before deadline",
		Now:           time.Now().UTC(),
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, result.Status)
	require.Equal(t, ProjectTaskAttemptStatusLost, repo.projectTaskAttempts[0].Status)
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, "project_task_recovery", inbox.upserts[0].DecisionType)
}

func TestRecoverLostProjectTaskAttemptMovesToWaitingHumanWhenRetryExhausted(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	repo.tasks[0].AttemptCount = 1
	repo.tasks[0].MaxAttempts = serviceTestInt32Ptr(1)

	result, err := service.RecoverLostProjectTaskAttempt(context.Background(), RecoverProjectTaskAttemptRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		AttemptID:     fixture.attemptID,
		FailureFamily: FailureFamilyRuntimeLeaseLost,
		Summary:       "Runtime lease expired",
		Now:           time.Now().UTC(),
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, result.Status)
	require.Equal(t, ProjectTaskAttemptStatusLost, repo.projectTaskAttempts[0].Status)
	require.Len(t, inbox.upserts, 1)
}

func TestSweepStaleQueuedProjectTaskAttemptsRecoversCandidates(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	now := time.Now().UTC()
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	repo.tasks[0].MaxAttempts = serviceTestInt32Ptr(3)
	repo.projectTaskAttempts[0].CreatedAt = now.Add(-10 * time.Minute)

	result, err := service.SweepStaleQueuedProjectTaskAttempts(context.Background(), SweepProjectTaskAttemptRecoveryRequest{
		TenantID: fixture.tenantID,
		Now:      now,
		Limit:    10,
	})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{fixture.attemptID}, result.RecoveredAttemptIDs)
	require.Equal(t, []uuid.UUID{fixture.taskID}, result.RecoveredTaskIDs)
}

func TestSweepExpiredRunningProjectTaskAttemptsRecoversCandidates(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	now := time.Now().UTC()
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	repo.tasks[0].MaxAttempts = serviceTestInt32Ptr(3)
	expiresAt := now.Add(-time.Minute)
	repo.projectTaskAttempts[0].LeaseExpiresAt = &expiresAt

	result, err := service.SweepExpiredRunningProjectTaskAttempts(context.Background(), SweepProjectTaskAttemptRecoveryRequest{
		TenantID: fixture.tenantID,
		Now:      now,
		Limit:    10,
	})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{fixture.attemptID}, result.RecoveredAttemptIDs)
	require.Equal(t, []uuid.UUID{fixture.taskID}, result.RecoveredTaskIDs)
}

func TestWaitHumanProjectTaskAttemptMovesTaskAndCreatesDecisionRequest(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, nil, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	employeeID := *repo.tasks[0].AssignedDigitalEmployeeID

	task, err := service.WaitHumanProjectTaskAttempt(context.Background(), WaitHumanProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-wait-human-1"),
		DigitalEmployeeID:                employeeID,
		Reason:                           HumanWaitReasonMissingContext,
		Summary:                          "Need customer scope",
		MissingContextRefs:               []any{"customer_scope"},
		SuggestedResolutionOptions:       []string{HumanWaitResolutionResumeSameTask},
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.WaitingReason)
	require.Equal(t, HumanWaitReasonMissingContext, *task.WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_missing_context", repo.decisionRequests[0].DecisionType)
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, repo.decisionRequests[0].ID, inbox.upserts[0].ID)
	require.Equal(t, repo.decisionRequests[0].ProjectTaskID, inbox.upserts[0].ProjectTaskID)
	require.Equal(t, ProjectEventTaskWaitingHuman, repo.eventTypes[len(repo.eventTypes)-1])
}

func TestMemoryRepositoryRecordPreDispatchGateResultReturnsLinkedGateWithoutOverwrite(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "linked gate replay",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	retryAfter := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)
	checkedAt := time.Date(2026, 6, 21, 12, 45, 0, 0, time.UTC)
	idempotencyKey := "gate:" + task.ID.String() + ":attempt:1:linked-replay"
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     idempotencyKey,
		DispatchToken:      "dispatch-token-linked-replay",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          checkedAt,
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "failed",
		}},
		Blockers: []PreDispatchGateBlocker{{
			Key:       "runtime.slot_unavailable",
			Severity:  "transient",
			Retryable: true,
		}},
		HumanActionRequest: HumanActionRequest{
			"action": "wait_for_runtime_slot",
		},
		RetryAfter: &retryAfter,
	})
	require.NoError(t, err)
	queued, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        task.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: ptrUUIDValue(uuid.New()),
		RuntimeTaskID:        ptrUUIDValue(uuid.New()),
		RuntimeNodeID:        ptrUUIDValue(uuid.New()),
		IdempotencyKey:       "project-task:" + task.ID.String() + ":attempt:1:linked-replay",
		LeaseToken:           "lease-token-linked-replay",
		DispatchGateResultID: &gate.ID,
	})
	require.NoError(t, err)

	replayed, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     idempotencyKey,
		DispatchToken:      "dispatch-token-linked-replay",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          checkedAt.Add(time.Minute),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
		Blockers: nil,
		HumanActionRequest: HumanActionRequest{
			"action": "dispatch_now",
		},
		RetryAfter: nil,
	})
	require.NoError(t, err)

	require.Equal(t, gate.ID, replayed.ID)
	require.Equal(t, PreDispatchGateStatusRetryLater, replayed.Status)
	require.NotNil(t, replayed.RetryAfter)
	require.Equal(t, retryAfter, *replayed.RetryAfter)
	require.Len(t, replayed.Blockers, 1)
	require.Equal(t, "failed", replayed.Checks[0].Status)
	require.Equal(t, HumanActionRequest{"action": "wait_for_runtime_slot"}, replayed.HumanActionRequest)
	require.NotNil(t, replayed.AttemptID)
	require.Equal(t, queued.Attempt.ID, *replayed.AttemptID)
}

func TestMemoryRepositoryRecordLinkedPreDispatchGateReplayDoesNotMoveLatest(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "linked gate replay keeps latest",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	checkedAt := time.Date(2026, 6, 21, 15, 0, 0, 0, time.UTC)
	gateAKey := "gate:" + task.ID.String() + ":attempt:1:linked-latest-a"
	gateA, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     gateAKey,
		DispatchToken:      "dispatch-token-linked-latest-a",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          checkedAt,
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
	})
	require.NoError(t, err)
	_, err = repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        task.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: ptrUUIDValue(uuid.New()),
		RuntimeTaskID:        ptrUUIDValue(uuid.New()),
		RuntimeNodeID:        ptrUUIDValue(uuid.New()),
		IdempotencyKey:       "project-task:" + task.ID.String() + ":attempt:1:linked-latest-a",
		LeaseToken:           "lease-token-linked-latest-a",
		DispatchGateResultID: &gateA.ID,
	})
	require.NoError(t, err)
	gateB, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          2,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "gate:" + task.ID.String() + ":attempt:2:latest-b",
		DispatchToken:      "dispatch-token-latest-b",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          checkedAt.Add(time.Minute),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "failed",
		}},
	})
	require.NoError(t, err)
	latestBeforeReplay, err := repo.GetProjectTask(context.Background(), tenantID, task.ID)
	require.NoError(t, err)
	require.NotNil(t, latestBeforeReplay.LatestDispatchGateResultID)
	require.Equal(t, gateB.ID, *latestBeforeReplay.LatestDispatchGateResultID)

	replayed, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     gateAKey,
		DispatchToken:      "dispatch-token-linked-latest-a",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          checkedAt.Add(2 * time.Minute),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, gateA.ID, replayed.ID)

	latestAfterReplay, err := repo.GetProjectTask(context.Background(), tenantID, task.ID)
	require.NoError(t, err)
	require.NotNil(t, latestAfterReplay.LatestDispatchGateResultID)
	require.Equal(t, gateB.ID, *latestAfterReplay.LatestDispatchGateResultID)
}

func TestMemoryRepositoryRecordPreDispatchGateResultWithInvalidProjectIsAtomic(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	wrongProjectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "invalid project gate replay",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "gate:" + task.ID.String() + ":attempt:1:invalid-project",
		DispatchToken:      "dispatch-token-invalid-project",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          time.Date(2026, 6, 21, 14, 0, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "failed",
		}},
		Blockers: []PreDispatchGateBlocker{{
			Key:       "runtime.slot_unavailable",
			Severity:  "transient",
			Retryable: true,
		}},
		HumanActionRequest: HumanActionRequest{
			"action": "wait_for_runtime_slot",
		},
	})
	require.NoError(t, err)
	originalTask := repo.tasks[0]
	originalGate := repo.dispatchGateResults[0]

	_, err = repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          wrongProjectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     gate.IdempotencyKey,
		DispatchToken:      "dispatch-token-invalid-project-replay",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          time.Date(2026, 6, 21, 14, 5, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
		HumanActionRequest: HumanActionRequest{
			"action": "dispatch_now",
		},
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, originalGate, repo.dispatchGateResults[0])
	require.Equal(t, originalTask, repo.tasks[0])

	_, err = repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          wrongProjectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          2,
		DispatchReason:     DispatchReasonRetry,
		IdempotencyKey:     "gate:" + task.ID.String() + ":attempt:2:invalid-project",
		DispatchToken:      "dispatch-token-invalid-project-new",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          time.Date(2026, 6, 21, 14, 10, 0, 0, time.UTC),
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, originalGate, repo.dispatchGateResults[0])
	require.Equal(t, originalTask, repo.tasks[0])
}

func TestMemoryRepositoryGetPreDispatchGateResultByKeyScopesProject(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	wrongProjectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "gate lookup project scope",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	idempotencyKey := "gate:" + task.ID.String() + ":attempt:1:project-scope"
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     idempotencyKey,
		DispatchToken:      "dispatch-token-project-scope",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          time.Date(2026, 6, 21, 14, 20, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "failed",
		}},
	})
	require.NoError(t, err)

	found, err := repo.GetPreDispatchGateResultByKey(context.Background(), tenantID, projectID, task.ID, idempotencyKey)
	require.NoError(t, err)
	require.Equal(t, gate.ID, found.ID)

	_, err = repo.GetPreDispatchGateResultByKey(context.Background(), tenantID, wrongProjectID, task.ID, idempotencyKey)
	require.ErrorIs(t, err, ErrProjectNotFound)
}

func TestMemoryRepositoryLinkPreDispatchGateAttemptRejectsWrongTaskAndUpdatesAttempt(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskA, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "gate task A",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	taskB, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "gate task B",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      taskA.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "gate:" + taskA.ID.String() + ":attempt:1:wrong-attempt",
		DispatchToken:      "dispatch-token-wrong-attempt",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          time.Date(2026, 6, 21, 13, 15, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
	})
	require.NoError(t, err)
	wrongAttempt, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        taskB.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: ptrUUIDValue(uuid.New()),
		RuntimeTaskID:        ptrUUIDValue(uuid.New()),
		RuntimeNodeID:        ptrUUIDValue(uuid.New()),
		IdempotencyKey:       "project-task:" + taskB.ID.String() + ":attempt:1:wrong-attempt",
		LeaseToken:           "lease-token-wrong-attempt",
	})
	require.NoError(t, err)

	_, err = repo.LinkPreDispatchGateAttempt(context.Background(), LinkPreDispatchGateAttemptRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskA.ID,
		GateResultID:  gate.ID,
		AttemptID:     wrongAttempt.Attempt.ID,
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Nil(t, repo.dispatchGateResults[0].AttemptID)
	require.Nil(t, repo.projectTaskAttempts[0].DispatchGateResultID)

	correctAttempt, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        taskA.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: ptrUUIDValue(uuid.New()),
		RuntimeTaskID:        ptrUUIDValue(uuid.New()),
		RuntimeNodeID:        ptrUUIDValue(uuid.New()),
		IdempotencyKey:       "project-task:" + taskA.ID.String() + ":attempt:1:correct-attempt",
		LeaseToken:           "lease-token-correct-attempt",
	})
	require.NoError(t, err)

	linked, err := repo.LinkPreDispatchGateAttempt(context.Background(), LinkPreDispatchGateAttemptRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskA.ID,
		GateResultID:  gate.ID,
		AttemptID:     correctAttempt.Attempt.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, linked.AttemptID)
	require.Equal(t, correctAttempt.Attempt.ID, *linked.AttemptID)
	linkedAttempt, err := repo.GetProjectTaskAttempt(context.Background(), tenantID, correctAttempt.Attempt.ID)
	require.NoError(t, err)
	require.NotNil(t, linkedAttempt.DispatchGateResultID)
	require.Equal(t, gate.ID, *linkedAttempt.DispatchGateResultID)
}

func TestQueueProjectTaskWithInvalidDispatchGateResultIsAtomic(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "invalid gate queue",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "gate:" + task.ID.String() + ":attempt:1:valid",
		DispatchToken:      "dispatch-token-valid",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          time.Date(2026, 6, 21, 14, 30, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
	})
	require.NoError(t, err)
	originalTask := repo.tasks[0]
	originalGate := repo.dispatchGateResults[0]
	missingGateID := uuid.New()

	_, err = repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        task.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: ptrUUIDValue(uuid.New()),
		RuntimeTaskID:        ptrUUIDValue(uuid.New()),
		RuntimeNodeID:        ptrUUIDValue(uuid.New()),
		IdempotencyKey:       "project-task:" + task.ID.String() + ":attempt:1:missing-gate",
		LeaseToken:           "lease-token-missing-gate",
		DispatchGateResultID: &missingGateID,
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Empty(t, repo.events)
	require.Empty(t, repo.projectTaskAttempts)
	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, originalGate, repo.dispatchGateResults[0])
	require.Equal(t, gate.ID, *repo.tasks[0].LatestDispatchGateResultID)
	require.Equal(t, originalTask, repo.tasks[0])
}

func TestMemoryRepositoryMoveProjectTaskToWaitingHumanForPreDispatchGateRequiresExistingGate(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "missing gate wait human",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	originalTask := repo.tasks[0]

	_, err = repo.MoveProjectTaskToWaitingHumanForPreDispatchGate(context.Background(), MoveProjectTaskToWaitingHumanForPreDispatchGateRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: task.ID,
		GateResultID:  uuid.New(),
		WaitingReason: HumanWaitReasonClarification,
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Equal(t, originalTask, repo.tasks[0])
}

func TestMemoryRepositoryLinkPreDispatchGateDecisionRequestRejectsWrongTask(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskA, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "gate decision task A",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	taskB, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "gate decision task B",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      taskA.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "gate:" + taskA.ID.String() + ":attempt:1:wrong-decision",
		DispatchToken:      "dispatch-token-wrong-decision",
		Status:             PreDispatchGateStatusWaitingHuman,
		CheckedAt:          time.Date(2026, 6, 21, 13, 30, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "risk.approval",
			Status: "failed",
		}},
	})
	require.NoError(t, err)
	decision, err := repo.CreateDecisionRequest(context.Background(), CreateDecisionRequestRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: uuid.New(),
		ProjectTaskID:     &taskB.ID,
		TargetUserID:      uuid.New(),
		DecisionType:      "pre_dispatch_gate_review",
		TitleSnapshot:     "Review task B",
		StatusSnapshot:    "pending",
	})
	require.NoError(t, err)

	_, err = repo.LinkPreDispatchGateDecisionRequest(context.Background(), LinkPreDispatchGateDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ProjectTaskID:     taskA.ID,
		GateResultID:      gate.ID,
		DecisionRequestID: decision.ID,
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Nil(t, repo.dispatchGateResults[0].DecisionRequestID)
	require.Nil(t, repo.decisionRequests[0].DispatchGateResultID)
}

func TestWaitHumanProjectTaskAttemptWritesLedgerEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	employeeID := *repo.tasks[0].AssignedDigitalEmployeeID

	_, err = service.WaitHumanProjectTaskAttempt(context.Background(), WaitHumanProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("wait-human-ledger"),
		DigitalEmployeeID:                employeeID,
		Reason:                           HumanWaitReasonMissingContext,
		Summary:                          "Need customer scope",
		MissingContextRefs:               []any{"customer_scope"},
		SuggestedResolutionOptions:       []string{HumanWaitResolutionResumeSameTask},
	})

	require.NoError(t, err)
	require.Len(t, repo.executionLedgerEvents, 1)
	event := repo.executionLedgerEvents[0]
	require.Equal(t, ExecutionLedgerEventAttemptWaitingHuman, event.EventType)
	require.Equal(t, fixture.attemptID, *event.ProjectTaskAttemptID)
	require.Equal(t, "Need customer scope", *event.OutputSummary)
}

func TestProjectTaskAttemptRejectsWrongRuntimeNode(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	req := fixture.runtimeRequest("attempt-lease-1")
	req.RuntimeNodeID = uuid.New()

	err = service.RenewProjectTaskAttemptLease(context.Background(), RenewProjectTaskAttemptLeaseRequest{
		ProjectTaskAttemptRuntimeRequest: req,
	})

	require.ErrorIs(t, err, ErrProjectConflict)
}

type projectTaskAttemptServiceFixture struct {
	tenantID  uuid.UUID
	projectID uuid.UUID
	taskID    uuid.UUID
	attemptID uuid.UUID
	nodeID    uuid.UUID
	lease     string
}

func (f projectTaskAttemptServiceFixture) runtimeRequest(idempotencyKey string) ProjectTaskAttemptRuntimeRequest {
	return ProjectTaskAttemptRuntimeRequest{
		TenantID:       f.tenantID,
		AttemptID:      f.attemptID,
		ProjectTaskID:  f.taskID,
		RuntimeNodeID:  f.nodeID,
		LeaseToken:     f.lease,
		IdempotencyKey: idempotencyKey,
	}
}

func newProjectTaskAttemptServiceFixture(repo *memoryRepository, taskStatus, attemptStatus string) projectTaskAttemptServiceFixture {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	employeeID := uuid.New()
	nodeID := uuid.New()
	lease := "lease-token-1"
	now := time.Now().UTC()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "Runtime closure",
		Goal:                   "Close task through attempts",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "attempt writeback",
		Status:                    taskStatus,
		AssignedDigitalEmployeeID: &employeeID,
		CurrentAttemptID:          &attemptID,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:             attemptID,
		TenantID:       tenantID,
		ProjectTaskID:  taskID,
		AttemptNo:      1,
		Status:         attemptStatus,
		RuntimeNodeID:  &nodeID,
		LeaseToken:     lease,
		IdempotencyKey: "project-task:" + taskID.String() + ":attempt:1:queue",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	return projectTaskAttemptServiceFixture{
		tenantID:  tenantID,
		projectID: projectID,
		taskID:    taskID,
		attemptID: attemptID,
		nodeID:    nodeID,
		lease:     lease,
	}
}

// addFixtureDownstreamDependency wires a non-terminal downstream task depending on
// the fixture task so §5.2's downstream_release gate can fire — the human-review /
// acceptance machinery only intercepts a completed task when a live dependent
// still needs the human to vouch for its output (leaf tasks fold into demand
// acceptance instead). Tests that assert the gate opened must set up a dependent.
func addFixtureDownstreamDependency(repo *memoryRepository, fixture projectTaskAttemptServiceFixture) {
	downstreamID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        downstreamID,
		TenantID:  fixture.tenantID,
		ProjectID: fixture.projectID,
		Title:     "下游依赖任务",
		Status:    ProjectTaskStatusQueued,
	})
	if repo.taskDependents == nil {
		repo.taskDependents = map[uuid.UUID][]uuid.UUID{}
	}
	repo.taskDependents[fixture.taskID] = append(repo.taskDependents[fixture.taskID], downstreamID)
}

type delayedAttemptReadinessRepository struct {
	*memoryRepository
	staleProjectTaskID  uuid.UUID
	staleReadsRemaining int
}

func (r *delayedAttemptReadinessRepository) GetProjectTask(ctx context.Context, tenantID, taskID uuid.UUID) (ProjectTask, error) {
	task, err := r.memoryRepository.GetProjectTask(ctx, tenantID, taskID)
	if err != nil {
		return ProjectTask{}, err
	}
	if task.ID == r.staleProjectTaskID && r.staleReadsRemaining > 0 {
		r.staleReadsRemaining--
		task.Status = ProjectTaskStatusWaitingHuman
		task.CurrentAttemptID = nil
		return task, nil
	}
	return task, nil
}

func requireLedgerEventTypes(t *testing.T, events []ExecutionLedgerEvent, expected ...string) {
	t.Helper()
	actual := make([]string, 0, len(events))
	for _, event := range events {
		actual = append(actual, event.EventType)
	}
	for _, eventType := range expected {
		require.Contains(t, actual, eventType)
	}
}

func TestQueueProjectTaskReplaysIdempotencyKey(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "验证幂等重放",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	})
	req := QueueProjectTaskRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		IdempotencyKey:    "project-task:" + taskID.String() + ":attempt:1:queue",
		LeaseToken:        "lease-token-1",
	}

	first, err := service.QueueProjectTask(context.Background(), req)
	require.NoError(t, err)
	second, err := service.QueueProjectTask(context.Background(), req)
	require.NoError(t, err)

	require.Equal(t, first.Attempt.ID, second.Attempt.ID)
	require.Equal(t, first.Task.CurrentAttemptID, second.Task.CurrentAttemptID)
	require.Equal(t, ProjectTaskStatusQueued, second.Task.Status)
	require.Len(t, repo.events, 1)
	require.Len(t, repo.projectTaskAttempts, 1)
}

func TestQueueProjectTaskRejectsIdempotencyKeyForDifferentTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	firstTaskID := uuid.New()
	secondTaskID := uuid.New()
	repo.tasks = append(repo.tasks,
		ProjectTask{
			ID:                        firstTaskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "首次排队任务",
			Status:                    ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			CreatedAt:                 time.Now().UTC(),
			UpdatedAt:                 time.Now().UTC(),
		},
		ProjectTask{
			ID:                        secondTaskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "冲突排队任务",
			Status:                    ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			CreatedAt:                 time.Now().UTC(),
			UpdatedAt:                 time.Now().UTC(),
		},
	)
	idempotencyKey := "project-task:" + firstTaskID.String() + ":attempt:1:queue"
	_, err = service.QueueProjectTask(context.Background(), QueueProjectTaskRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ProjectTaskID:     firstTaskID,
		DigitalEmployeeID: employeeID,
		IdempotencyKey:    idempotencyKey,
		LeaseToken:        "lease-token-1",
	})
	require.NoError(t, err)

	_, err = service.QueueProjectTask(context.Background(), QueueProjectTaskRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ProjectTaskID:     secondTaskID,
		DigitalEmployeeID: employeeID,
		IdempotencyKey:    idempotencyKey,
		LeaseToken:        "lease-token-2",
	})
	require.ErrorIs(t, err, ErrProjectConflict)
	require.Len(t, repo.events, 1)
	require.Len(t, repo.projectTaskAttempts, 1)
}

func TestQueueProjectTaskRejectsInvalidCurrentStatus(t *testing.T) {
	for _, status := range []string{ProjectTaskStatusRunning, ProjectTaskStatusCompleted} {
		t.Run(status, func(t *testing.T) {
			repo := newMemoryRepository()
			service, err := NewService(repo)
			require.NoError(t, err)

			tenantID := uuid.New()
			projectID := uuid.New()
			taskID := uuid.New()
			employeeID := uuid.New()
			repo.tasks = append(repo.tasks, ProjectTask{
				ID:                        taskID,
				TenantID:                  tenantID,
				ProjectID:                 projectID,
				Title:                     "状态冲突任务",
				Status:                    status,
				AssignedDigitalEmployeeID: &employeeID,
				CreatedAt:                 time.Now().UTC(),
				UpdatedAt:                 time.Now().UTC(),
			})

			_, err = service.QueueProjectTask(context.Background(), QueueProjectTaskRequest{
				TenantID:          tenantID,
				ProjectID:         projectID,
				ProjectTaskID:     taskID,
				DigitalEmployeeID: employeeID,
				IdempotencyKey:    "project-task:" + taskID.String() + ":attempt:1:queue",
				LeaseToken:        "lease-token-1",
			})
			require.ErrorIs(t, err, ErrProjectConflict)
			require.Empty(t, repo.events)
			require.Empty(t, repo.projectTaskAttempts)
		})
	}
}

func TestCreateProjectRejectsUnauthorizedTeamScope(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	teamID := uuid.New()

	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		TeamID:           &teamID,
		ActorUserID:      actorID,
		Name:             "unauthorized-team-project",
		Goal:             "验证团队授权边界",
		HumanOwnerUserID: ownerID,
	})
	if !errors.Is(err, ErrUnauthorizedProjectTeamScope) {
		t.Fatalf("expected unauthorized team scope error, got %v", err)
	}
	assertNoCreateProjectSideEffects(t, repo, coordinator)
}

func TestCreateProjectAllowsAuthorizedTeamScope(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	teamID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.authorizeProjectTeamScope(tenantID, actorID, teamID)
	stubProjectRuntimeNodeReader(service, tenantID, runtimeNodeID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		TeamID:           &teamID,
		ActorUserID:      actorID,
		Name:             "authorized-team-project",
		Goal:             "验证授权通过",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{{
			PrincipalType:       PrincipalTypeTeam,
			PrincipalID:         teamID,
			ProjectRole:         ProjectRoleObserver,
			DisplayNameSnapshot: "研发团队",
		}},
		RuntimeNodeIDs: []uuid.UUID{runtimeNodeID},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Project.TeamID == nil || *created.Project.TeamID != teamID {
		t.Fatalf("expected team id %s, got %#v", teamID, created.Project.TeamID)
	}
}

func TestCreateProjectRejectsUnauthorizedMemberOnlyTeamScope(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	teamID := uuid.New()

	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Name:             "unauthorized-member-project",
		Goal:             "验证成员团队授权边界",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{{
			PrincipalType:       PrincipalTypeTeam,
			PrincipalID:         teamID,
			ProjectRole:         ProjectRoleObserver,
			DisplayNameSnapshot: "研发团队",
		}},
	})
	if !errors.Is(err, ErrUnauthorizedProjectTeamScope) {
		t.Fatalf("expected unauthorized team scope error, got %v", err)
	}
	assertNoCreateProjectSideEffects(t, repo, coordinator)
}

func TestCreateProjectAllowsAuthorizedMemberOnlyTeamScope(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	teamID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.authorizeProjectTeamScope(tenantID, actorID, teamID)
	stubProjectRuntimeNodeReader(service, tenantID, runtimeNodeID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Name:             "authorized-member-project",
		Goal:             "验证成员团队授权通过",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{{
			PrincipalType:       PrincipalTypeTeam,
			PrincipalID:         teamID,
			ProjectRole:         ProjectRoleObserver,
			DisplayNameSnapshot: "研发团队",
		}},
		RuntimeNodeIDs: []uuid.UUID{runtimeNodeID},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Project.TeamID != nil {
		t.Fatalf("expected no top-level team id, got %#v", created.Project.TeamID)
	}
}

func TestCreateProjectWithoutTeamScopeSucceedsWithoutAuthorizer(t *testing.T) {
	backing := newMemoryRepository()
	service, err := NewService(&repositoryWithoutProjectTeamScopeAuthorizer{Repository: backing})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, runtimeNodeID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Name:             "no-team-project",
		Goal:             "验证无团队路径不要求授权器",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{{
			PrincipalType:       PrincipalTypeDigitalEmployee,
			PrincipalID:         employeeID,
			ProjectRole:         ProjectRoleExecutor,
			DisplayNameSnapshot: "后端执行 A",
		}},
		RuntimeNodeIDs: []uuid.UUID{runtimeNodeID},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Project.TeamID != nil {
		t.Fatalf("expected no team id, got %#v", created.Project.TeamID)
	}
}

func TestCreateProjectRequiresRuntimeNodes(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)
	tenantID := uuid.New()
	ownerID := uuid.New()

	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "missing-runtime-nodes",
		Goal:             "验证运行节点资格集必填",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   nil,
	})
	require.ErrorIs(t, err, ErrProjectRuntimeNodesRequired)
	assertNoCreateProjectSideEffects(t, repo, coordinator)
}

func TestCreateProjectPersistsRuntimeNodes(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	ownerID := uuid.New()
	nodeA := uuid.New()
	nodeB := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, nodeA, nodeB)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "with-runtime-nodes",
		Goal:             "验证运行节点资格集写入并可读取",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   []uuid.UUID{nodeA, nodeB},
	})
	require.NoError(t, err)

	nodes, err := service.ListProjectRuntimeNodes(context.Background(), tenantID, created.Project.ID)
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	gotIDs := []uuid.UUID{nodes[0].RuntimeNodeID, nodes[1].RuntimeNodeID}
	require.ElementsMatch(t, []uuid.UUID{nodeA, nodeB}, gotIDs)
}

func TestCreateProjectRequiresMandatoryFields(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         uuid.New(),
		ActorUserID:      uuid.New(),
		Name:             "missing-goal",
		HumanOwnerUserID: uuid.New(),
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}
}

func TestCreateProjectAcceptsNullableRepoBinding(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	ownerID := uuid.New()
	credentialRef := "  git-credential:primary  "
	runtimeNodeID := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, runtimeNodeID)

	unbound, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "no-repo-binding",
		Goal:             "验证仓库绑定可为空",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   []uuid.UUID{runtimeNodeID},
	})
	require.NoError(t, err)
	require.Equal(t, ProjectRepoBindingStatusUnbound, unbound.Project.RepoBinding.Status)
	require.Empty(t, unbound.Project.RepoBinding.URL)
	require.Empty(t, unbound.Project.RepoBinding.DefaultBranch)
	require.Nil(t, unbound.Project.RepoBinding.GitCredentialRef)
	require.Empty(t, unbound.Project.RepoBinding.Scope)

	bound, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "with-repo-binding",
		Goal:             "验证仓库绑定归一化",
		HumanOwnerUserID: ownerID,
		RuntimeNodeIDs:   []uuid.UUID{runtimeNodeID},
		RepoBinding: &ProjectRepoBindingInput{
			URL:              "  https://github.com/acme/superteam.git  ",
			DefaultBranch:    "  main  ",
			GitCredentialRef: &credentialRef,
			Scope:            []string{" apps/control-plane ", "", "apps/web", "apps/control-plane"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, ProjectRepoBindingStatusBound, bound.Project.RepoBinding.Status)
	require.Equal(t, "https://github.com/acme/superteam.git", bound.Project.RepoBinding.URL)
	require.Equal(t, "main", bound.Project.RepoBinding.DefaultBranch)
	require.NotNil(t, bound.Project.RepoBinding.GitCredentialRef)
	require.Equal(t, "git-credential:primary", *bound.Project.RepoBinding.GitCredentialRef)
	require.Equal(t, []string{"apps/control-plane", "apps/web"}, bound.Project.RepoBinding.Scope)
}

func TestCreateProjectRejectsPartialRepoBinding(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	require.NoError(t, err)
	tenantID := uuid.New()
	ownerID := uuid.New()

	for _, tc := range []struct {
		name        string
		repoBinding *ProjectRepoBindingInput
	}{
		{
			name: "missing default branch",
			repoBinding: &ProjectRepoBindingInput{
				URL: "https://github.com/acme/superteam.git",
			},
		},
		{
			name: "missing url",
			repoBinding: &ProjectRepoBindingInput{
				DefaultBranch: "main",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.CreateProject(context.Background(), CreateProjectRequest{
				TenantID:         tenantID,
				ActorUserID:      ownerID,
				Name:             "partial-repo-binding",
				Goal:             "验证仓库绑定必须完整",
				HumanOwnerUserID: ownerID,
				RepoBinding:      tc.repoBinding,
			})
			require.ErrorIs(t, err, ErrInvalidProject)
		})
	}
}

func TestCreateProjectRejectsCoordinatorMemberRole(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         uuid.New(),
		ActorUserID:      uuid.New(),
		Name:             "demo-project",
		Goal:             "目标",
		HumanOwnerUserID: uuid.New(),
		Members: []ProjectMemberInput{{
			PrincipalType: PrincipalTypeDigitalEmployee,
			PrincipalID:   uuid.New(),
			ProjectRole:   ProjectRole("coordinator"),
		}},
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid member error, got %v", err)
	}
}

func TestCreateProjectValidatesRolePrincipalTypes(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, tc := range []struct {
		name          string
		principalType PrincipalType
		role          ProjectRole
	}{
		{name: "owner must be human", principalType: PrincipalTypeDigitalEmployee, role: ProjectRoleOwner},
		{name: "executor must be digital employee", principalType: PrincipalTypeHumanUser, role: ProjectRoleExecutor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.CreateProject(context.Background(), CreateProjectRequest{
				TenantID:         uuid.New(),
				ActorUserID:      uuid.New(),
				Name:             "demo-project",
				Goal:             "目标",
				HumanOwnerUserID: uuid.New(),
				Members: []ProjectMemberInput{{
					PrincipalType: tc.principalType,
					PrincipalID:   uuid.New(),
					ProjectRole:   tc.role,
				}},
			})
			if !errors.Is(err, ErrInvalidProjectMember) {
				t.Fatalf("expected invalid member error, got %v", err)
			}
		})
	}
}

func TestSubmitDemandRecordsDemandAndEventWithoutAutoCreatingTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Name:             "customer-runtime-acceptance",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	seedHumanOwnerMember(repo, repo.projects[projectID].TenantID, projectID, ownerID)
	seedDigitalExecutorMember(repo, repo.projects[projectID].TenantID, projectID, uuid.New())

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          repo.projects[projectID].TenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
		SourceType:        DemandSourceManual,
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.Status != ProjectDemandStatusPlanningPending {
		t.Fatalf("expected planning pending demand, got %s", demand.Status)
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("service must not create project tasks from demand directly")
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventDemandSubmitted {
		t.Fatalf("expected demand event only, got %#v", repo.eventTypes)
	}
}

func TestSubmitDemandRejectsUnknownScenarioTemplateKey(t *testing.T) {
	newFixture := func(t *testing.T) (*Service, *memoryRepository, uuid.UUID, uuid.UUID, uuid.UUID) {
		t.Helper()
		repo := newMemoryRepository()
		service, err := NewService(repo)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
		projectID := uuid.New()
		ownerID := uuid.New()
		repo.projects[projectID] = Project{
			ID:               projectID,
			TenantID:         tenantID,
			Name:             "demand-template-key",
			Status:           ProjectStatusRunning,
			HumanOwnerUserID: ownerID,
		}
		seedHumanOwnerMember(repo, tenantID, projectID, ownerID)
		seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())
		service.SetScenarioTemplateResolver(stubScenarioTemplateResolver{bindings: map[string]ScenarioTemplateBinding{
			"ops_analysis": {Key: "ops_analysis", Name: "运维分析", Status: "active"},
			"retired":      {Key: "retired", Name: "退役", Status: "disabled"},
		}})
		return service, repo, tenantID, projectID, ownerID
	}

	t.Run("unknown key rejected", func(t *testing.T) {
		service, _, tenantID, projectID, ownerID := newFixture(t)
		key := "nope"
		_, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
			TenantID:            tenantID,
			ProjectID:           projectID,
			SubmittedByUserID:   ownerID,
			Title:               "验证未知模板键被拒绝",
			SourceType:          DemandSourceManual,
			ScenarioTemplateKey: &key,
		})
		if !errors.Is(err, ErrInvalidProject) {
			t.Fatalf("expected ErrInvalidProject, got %v", err)
		}
	})

	t.Run("disabled key rejected", func(t *testing.T) {
		service, _, tenantID, projectID, ownerID := newFixture(t)
		key := "retired"
		_, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
			TenantID:            tenantID,
			ProjectID:           projectID,
			SubmittedByUserID:   ownerID,
			Title:               "验证禁用模板键被拒绝",
			SourceType:          DemandSourceManual,
			ScenarioTemplateKey: &key,
		})
		if !errors.Is(err, ErrInvalidProject) {
			t.Fatalf("expected ErrInvalidProject, got %v", err)
		}
	})

	t.Run("active key accepted and persisted", func(t *testing.T) {
		service, _, tenantID, projectID, ownerID := newFixture(t)
		key := " ops_analysis "
		demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
			TenantID:            tenantID,
			ProjectID:           projectID,
			SubmittedByUserID:   ownerID,
			Title:               "验证有效模板键落库",
			SourceType:          DemandSourceManual,
			ScenarioTemplateKey: &key,
		})
		if err != nil {
			t.Fatalf("submit demand: %v", err)
		}
		if demand.ScenarioTemplateKey == nil || *demand.ScenarioTemplateKey != "ops_analysis" {
			t.Fatalf("expected trimmed bound key, got %#v", demand.ScenarioTemplateKey)
		}
	})

	t.Run("no key keeps today's behavior", func(t *testing.T) {
		service, _, tenantID, projectID, ownerID := newFixture(t)
		demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
			TenantID:          tenantID,
			ProjectID:         projectID,
			SubmittedByUserID: ownerID,
			Title:             "验证缺省模板键",
			SourceType:        DemandSourceManual,
		})
		if err != nil {
			t.Fatalf("submit demand: %v", err)
		}
		if demand.ScenarioTemplateKey != nil {
			t.Fatalf("expected nil key, got %#v", demand.ScenarioTemplateKey)
		}
	})
}

func TestSubmitDemandCoordinationMode(t *testing.T) {
	newFixture := func() (*memoryRepository, *fakeCoordinatorSignalClient, *Service, uuid.UUID, uuid.UUID) {
		repo := newMemoryRepository()
		coordinator := &fakeCoordinatorSignalClient{}
		service, err := NewServiceWithCoordinator(repo, coordinator)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		projectID := uuid.New()
		tenantID := uuid.New()
		ownerID := uuid.New()
		repo.projects[projectID] = Project{
			ID:                     projectID,
			TenantID:               tenantID,
			Name:                   "customer-runtime-acceptance",
			Status:                 ProjectStatusRunning,
			HumanOwnerUserID:       ownerID,
			CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
			CoordinationStatus:     "registered",
		}
		seedHumanOwnerMember(repo, tenantID, projectID, ownerID)
		seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())
		return repo, coordinator, service, tenantID, projectID
	}

	t.Run("absent defaults to plan", func(t *testing.T) {
		repo, coordinator, service, tenantID, projectID := newFixture()
		ownerID := repo.projects[projectID].HumanOwnerUserID

		demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
			TenantID:          tenantID,
			ProjectID:         projectID,
			SubmittedByUserID: ownerID,
			Title:             "验证 Runtime 连接",
		})
		require.NoError(t, err)
		require.Equal(t, CoordinationModePlan, demand.CoordinationMode)
		require.Len(t, repo.demands, 1)
		require.Equal(t, CoordinationModePlan, repo.demands[0].CoordinationMode)
		require.Equal(t, 1, coordinator.demandSignals)
	})

	t.Run("explicit loop", func(t *testing.T) {
		repo, coordinator, service, tenantID, projectID := newFixture()
		ownerID := repo.projects[projectID].HumanOwnerUserID

		demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
			TenantID:          tenantID,
			ProjectID:         projectID,
			SubmittedByUserID: ownerID,
			Title:             "验证 Runtime 连接",
			CoordinationMode:  CoordinationModeLoop,
		})
		require.NoError(t, err)
		require.Equal(t, CoordinationModeLoop, demand.CoordinationMode)
		require.Len(t, repo.demands, 1)
		require.Equal(t, CoordinationModeLoop, repo.demands[0].CoordinationMode)
		require.Equal(t, 1, coordinator.demandSignals)
	})

	t.Run("invalid value rejected before persistence or signal", func(t *testing.T) {
		repo, coordinator, service, tenantID, projectID := newFixture()
		ownerID := repo.projects[projectID].HumanOwnerUserID

		demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
			TenantID:          tenantID,
			ProjectID:         projectID,
			SubmittedByUserID: ownerID,
			Title:             "验证 Runtime 连接",
			CoordinationMode:  "banana",
		})
		require.ErrorIs(t, err, ErrInvalidCoordinationMode)
		require.Nil(t, demand)
		require.Len(t, repo.demands, 0)
		require.Equal(t, 0, coordinator.demandSignals)
	})
}

func TestGetDemandLaunchDetailAggregatesDemandFacts(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "customer-runtime-acceptance",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)
	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID, Title: "审查 PR",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	job := CoordinationJob{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, TriggerEventID: demand.CreatedEventID, JobType: "demand_route", Status: "running"}
	inputJob := CoordinationJob{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, JobType: "demand_route", Status: "running", InputSnapshotRef: map[string]any{"demand_id": demand.ID.String()}}
	repo.coordinationJobs = append(repo.coordinationJobs, job, inputJob)
	task := ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, DemandID: &demand.ID, Title: "审查 PR", Status: "pending"}
	repo.tasks = append(repo.tasks, task)
	summary := ExecutionSummary{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ProjectTaskID: task.ID, DigitalEmployeeID: ownerID, Conclusion: "已完成审查"}
	unrelatedSummary := ExecutionSummary{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ProjectTaskID: uuid.New(), DigitalEmployeeID: ownerID, Conclusion: "其他需求结果"}
	repo.executionSummaries = append(repo.executionSummaries, summary, unrelatedSummary)
	repo.routeDecisions = append(repo.routeDecisions, RouteDecision{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, CoordinationJobID: job.ID, DemandID: &demand.ID, Reason: "按能力分派"})
	decisionRequest := DecisionRequest{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, CoordinationJobID: &job.ID, TargetUserID: ownerID, DecisionType: "route_review", TitleSnapshot: "确认路由", StatusSnapshot: "pending"}
	taskDecisionRequest := DecisionRequest{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ProjectTaskID: &task.ID, TargetUserID: ownerID, DecisionType: "task_review", TitleSnapshot: "确认任务", StatusSnapshot: "pending"}
	repo.decisionRequests = append(repo.decisionRequests, decisionRequest, taskDecisionRequest)
	demandResourceType := "project_demand"
	demandResourceID := demand.ID.String()
	demandResourceEvent := ProjectEvent{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ResourceType: &demandResourceType, ResourceID: &demandResourceID, EventType: ProjectEventDemandSubmitted, ActorType: "human_user", ActorID: ownerID.String(), Payload: map[string]any{}}
	taskPayloadEvent := ProjectEvent{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, EventType: ProjectEventTaskDispatched, ActorType: "workflow", ActorID: job.ID.String(), Payload: map[string]any{"project_task_id": task.ID.String()}}
	decisionPayloadEvent := ProjectEvent{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, EventType: ProjectEventDecisionRequested, ActorType: "workflow", ActorID: job.ID.String(), Payload: map[string]any{"decision_request_id": decisionRequest.ID.String()}}
	repo.events = append(repo.events, demandResourceEvent, taskPayloadEvent, decisionPayloadEvent)
	for i := 0; i < 120; i++ {
		unrelatedDemandID := uuid.New()
		unrelatedJobID := uuid.New()
		unrelatedTaskID := uuid.New()
		unrelatedDecisionID := uuid.New()
		later := time.Now().UTC().Add(time.Duration(i+1) * time.Minute)
		repo.coordinationJobs = append(repo.coordinationJobs, CoordinationJob{ID: unrelatedJobID, TenantID: tenantID, ProjectID: projectID, JobType: "demand_route", Status: "running", InputSnapshotRef: map[string]any{"demand_id": unrelatedDemandID.String()}, CreatedAt: later})
		repo.tasks = append(repo.tasks, ProjectTask{ID: unrelatedTaskID, TenantID: tenantID, ProjectID: projectID, DemandID: &unrelatedDemandID, Title: "其他任务", Status: "pending", UpdatedAt: later})
		repo.routeDecisions = append(repo.routeDecisions, RouteDecision{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, CoordinationJobID: unrelatedJobID, DemandID: &unrelatedDemandID, Reason: "其他路由", CreatedAt: later})
		repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{ID: unrelatedDecisionID, TenantID: tenantID, ProjectID: projectID, CoordinationJobID: &unrelatedJobID, ProjectTaskID: &unrelatedTaskID, TargetUserID: ownerID, DecisionType: "route_review", TitleSnapshot: "其他决策", StatusSnapshot: "pending", CreatedAt: later})
		unrelatedTaskResourceID := unrelatedTaskID.String()
		repo.events = append(repo.events, ProjectEvent{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, SequenceNumber: int64(1000 + i), EventType: ProjectEventTaskDispatched, ActorType: "workflow", ActorID: unrelatedJobID.String(), ResourceID: &unrelatedTaskResourceID, Payload: map[string]any{"project_task_id": unrelatedTaskID.String()}, CreatedAt: later})
	}

	detail, err := service.GetDemandLaunchDetail(context.Background(), tenantID, demand.ID)
	if err != nil {
		t.Fatalf("launch detail: %v", err)
	}
	if detail.Demand.ID != demand.ID || detail.Project.ID != projectID {
		t.Fatalf("unexpected demand/project: %#v", detail)
	}
	if detail.Reviewer == nil || detail.Reviewer.ReviewerUserID != ownerID {
		t.Fatalf("expected reviewer preference in launch detail: %#v", detail.Reviewer)
	}
	if len(detail.CoordinationJobs) != 2 || len(detail.RouteDecisions) != 1 || len(detail.ProjectTasks) != 1 || len(detail.DecisionRequests) != 2 {
		t.Fatalf("expected related facts, got %#v", detail)
	}
	if len(detail.ExecutionSummaries) != 1 || detail.ExecutionSummaries[0].ID != summary.ID {
		t.Fatalf("expected task-scoped execution summary in launch detail, got %#v", detail.ExecutionSummaries)
	}
	if len(detail.RecentEvents) != 4 {
		t.Fatalf("expected demand event in launch detail: %#v", detail.RecentEvents)
	}
	eventIDs := map[uuid.UUID]struct{}{}
	for _, event := range detail.RecentEvents {
		eventIDs[event.ID] = struct{}{}
	}
	if _, ok := eventIDs[*demand.CreatedEventID]; !ok {
		t.Fatalf("expected created demand event in launch detail: %#v", detail.RecentEvents)
	}
	if _, ok := eventIDs[demandResourceEvent.ID]; !ok {
		t.Fatalf("expected demand resource event in launch detail: %#v", detail.RecentEvents)
	}
	if _, ok := eventIDs[taskPayloadEvent.ID]; !ok {
		t.Fatalf("expected task payload event in launch detail: %#v", detail.RecentEvents)
	}
	if _, ok := eventIDs[decisionPayloadEvent.ID]; !ok {
		t.Fatalf("expected decision payload event in launch detail: %#v", detail.RecentEvents)
	}
}

func TestGetProjectTaskGraphRequiresFilterAndDoesNotApplyHiddenLimit(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	repo := &taskGraphLimitRepository{memoryRepository: newMemoryRepository()}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected missing graph filter to be invalid, got %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("expected invalid graph request not to call repository, got %d calls", repo.calls)
	}

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if len(graph.Nodes) != 55 {
		t.Fatalf("expected complete demand graph, got %d nodes", len(graph.Nodes))
	}
	if graph.Edges == nil || graph.Employees == nil || graph.Runs == nil || graph.ExecutionSummaries == nil || graph.RecentEvents == nil || graph.DecisionRequests == nil {
		t.Fatalf("expected non-nil graph sidecar slices: %#v", graph)
	}
	if repo.lastReq.Limit != 0 || repo.lastReq.Offset != 0 {
		t.Fatalf("expected graph service not to apply hidden pagination, got limit=%d offset=%d", repo.lastReq.Limit, repo.lastReq.Offset)
	}
}

func TestGetProjectTaskGraphBuildsStageSummariesWhenRepositoryOmitsThem(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	stageOne := int32(1)
	stageTwo := int32(2)
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Nodes: []ProjectTaskGraphNode{
				{Task: ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "入口", Status: "completed", StageIndex: &stageOne}},
				{Task: ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "巡检", Status: "running", StageIndex: &stageTwo}},
				{Task: ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "审批", Status: "waiting_human", StageIndex: &stageTwo, RequiresHumanApproval: true}},
			},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if len(graph.StageSummaries) != 2 {
		t.Fatalf("expected two stage summaries, got %#v", graph.StageSummaries)
	}
	if graph.StageSummaries[0].StageIndex != 1 || graph.StageSummaries[0].CompletedNodes != 1 {
		t.Fatalf("unexpected first stage summary: %#v", graph.StageSummaries[0])
	}
	if graph.StageSummaries[1].StageIndex != 2 || graph.StageSummaries[1].RunningNodes != 1 || graph.StageSummaries[1].WaitingHumanNodes != 1 {
		t.Fatalf("unexpected second stage summary: %#v", graph.StageSummaries[1])
	}
}

func TestGetProjectTaskGraphEnrichesEmployeeIdentityWhenLookupIsSet(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	digitalEmployeeID := uuid.New()
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Nodes:     []ProjectTaskGraphNode{},
			Employees: []ProjectTaskGraphEmployee{{DigitalEmployeeID: digitalEmployeeID}},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	lookup := &fakeDigitalEmployeeIdentityLookup{
		identities: map[uuid.UUID]DigitalEmployeeIdentity{
			digitalEmployeeID: {
				Role: "代码审查员",
				AvatarAsset: &ProjectTaskGraphEmployeeAvatarAsset{
					ID:           "avatar-1",
					Label:        "Adventurer 1",
					ThumbnailURL: "https://example.com/avatar-1-thumb.png",
				},
			},
		},
	}
	service.SetDigitalEmployeeIdentityLookup(lookup)

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if len(graph.Employees) != 1 {
		t.Fatalf("expected one graph employee, got %#v", graph.Employees)
	}
	employee := graph.Employees[0]
	if employee.EmployeeRole != "代码审查员" {
		t.Fatalf("expected enriched employee role, got %q", employee.EmployeeRole)
	}
	if employee.AvatarAsset == nil || employee.AvatarAsset.ID != "avatar-1" || employee.AvatarAsset.Label != "Adventurer 1" || employee.AvatarAsset.ThumbnailURL != "https://example.com/avatar-1-thumb.png" {
		t.Fatalf("expected enriched avatar asset, got %#v", employee.AvatarAsset)
	}
	if len(lookup.calls) != 1 || lookup.calls[0] != digitalEmployeeID {
		t.Fatalf("expected lookup called once for employee, got %#v", lookup.calls)
	}
}

func TestGetProjectTaskGraphSkipsEnrichmentWhenLookupIsUnset(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	digitalEmployeeID := uuid.New()
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Nodes:     []ProjectTaskGraphNode{},
			Employees: []ProjectTaskGraphEmployee{{DigitalEmployeeID: digitalEmployeeID}},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if len(graph.Employees) != 1 {
		t.Fatalf("expected one graph employee, got %#v", graph.Employees)
	}
	if graph.Employees[0].EmployeeRole != "" || graph.Employees[0].AvatarAsset != nil {
		t.Fatalf("expected no enrichment without lookup, got %#v", graph.Employees[0])
	}
}

func TestGetProjectTaskGraphIgnoresEmployeeIdentityLookupErrors(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	digitalEmployeeID := uuid.New()
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Nodes:     []ProjectTaskGraphNode{},
			Employees: []ProjectTaskGraphEmployee{{DigitalEmployeeID: digitalEmployeeID}},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.SetDigitalEmployeeIdentityLookup(&fakeDigitalEmployeeIdentityLookup{
		err: map[uuid.UUID]error{digitalEmployeeID: errors.New("employee not found")},
	})

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if len(graph.Employees) != 1 {
		t.Fatalf("expected one graph employee, got %#v", graph.Employees)
	}
	if graph.Employees[0].EmployeeRole != "" || graph.Employees[0].AvatarAsset != nil {
		t.Fatalf("expected failed enrichment to leave employee unchanged, got %#v", graph.Employees[0])
	}
}

func TestGetProjectTaskGraphPropagatesEmployeeIdentityContextCancellation(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	digitalEmployeeID := uuid.New()
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Nodes:     []ProjectTaskGraphNode{},
			Employees: []ProjectTaskGraphEmployee{{DigitalEmployeeID: digitalEmployeeID}},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.SetDigitalEmployeeIdentityLookup(&fakeDigitalEmployeeIdentityLookup{
		err: map[uuid.UUID]error{digitalEmployeeID: context.Canceled},
	})

	_, err = service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestListWorkflowInstancesNormalizesPaginationAndStatusPriority(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	repo := &workflowInstanceServiceRepository{
		memoryRepository: newMemoryRepository(),
		items: []WorkflowInstanceSummary{{
			DemandID:          demandID,
			ProjectID:         projectID,
			ProjectName:       "支付巡检",
			Title:             "定位支付成功率下降",
			SubmittedByUserID: actorID,
			Status:            WorkflowInstanceStatusUnknown,
			Progress: WorkflowInstanceProgress{
				TotalNodes:        3,
				CompletedNodes:    1,
				RunningNodes:      1,
				BlockedNodes:      1,
				WaitingHumanNodes: 1,
			},
		}},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	items, err := service.ListWorkflowInstances(context.Background(), ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       0,
		Offset:      -4,
	})
	if err != nil {
		t.Fatalf("list workflow instances: %v", err)
	}
	if repo.lastReq.Limit != 20 || repo.lastReq.Offset != 0 {
		t.Fatalf("expected normalized pagination, got limit=%d offset=%d", repo.lastReq.Limit, repo.lastReq.Offset)
	}
	if len(items) != 1 {
		t.Fatalf("expected one workflow instance, got %#v", items)
	}
	if items[0].Status != WorkflowInstanceStatusWaitingHuman {
		t.Fatalf("expected waiting_human to outrank running and planning, got %#v", items[0])
	}
}

func TestListWorkflowInstancesRejectsMissingActor(t *testing.T) {
	tenantID := uuid.New()
	repo := &workflowInstanceServiceRepository{memoryRepository: newMemoryRepository()}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.ListWorkflowInstances(context.Background(), ListWorkflowInstancesRequest{
		TenantID: tenantID,
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("expected invalid request not to call repository, got %d calls", repo.calls)
	}
}

func TestListWorkflowInstancesKeepsOptionalReadModelFieldsAndSortsAttentionFirst(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	waitingDemandID := uuid.New()
	runningDemandID := uuid.New()
	completedDemandID := uuid.New()
	dueAt := time.Now().UTC().Add(15 * time.Minute)
	remaining := int32(900)
	repo := &workflowInstanceServiceRepository{
		memoryRepository: newMemoryRepository(),
		items: []WorkflowInstanceSummary{
			{
				DemandID:          completedDemandID,
				ProjectID:         uuid.New(),
				ProjectName:       "归档项目",
				Title:             "复盘归档",
				SubmittedByUserID: actorID,
				Status:            WorkflowInstanceStatusCompleted,
				UpdatedAt:         time.Now().UTC().Add(-2 * time.Minute),
				Progress: WorkflowInstanceProgress{
					TotalNodes:     2,
					CompletedNodes: 2,
				},
			},
			{
				DemandID:          runningDemandID,
				ProjectID:         uuid.New(),
				ProjectName:       "运行项目",
				Title:             "服务巡检",
				SubmittedByUserID: actorID,
				Status:            WorkflowInstanceStatusRunning,
				UpdatedAt:         time.Now().UTC().Add(-1 * time.Minute),
				Progress: WorkflowInstanceProgress{
					TotalNodes:   3,
					RunningNodes: 1,
				},
			},
			{
				DemandID:          waitingDemandID,
				ProjectID:         uuid.New(),
				ProjectName:       "支付项目",
				Title:             "支付成功率下降",
				SubmittedByUserID: actorID,
				Status:            WorkflowInstanceStatusUnknown,
				UpdatedAt:         time.Now().UTC().Add(-3 * time.Minute),
				Progress: WorkflowInstanceProgress{
					TotalNodes:        5,
					CompletedNodes:    2,
					RunningNodes:      1,
					BlockedNodes:      1,
					WaitingHumanNodes: 1,
					PlannedNodes:      1,
					FailedNodes:       0,
					CancelledNodes:    0,
				},
				CurrentBlocker: &WorkflowInstanceCurrentBlocker{
					Type:  "decision_request",
					Title: "等待人工审批回滚方案",
				},
				Priority: &WorkflowInstancePriority{
					Value:  "p1",
					Label:  "P1",
					Source: "source_refs.priority",
				},
				Risk: &WorkflowInstanceRisk{
					Level:  "high",
					Label:  "高风险",
					Source: "project_tasks.risk_level",
				},
				SLA: &WorkflowInstanceSLA{
					DueAt:            &dueAt,
					RemainingSeconds: &remaining,
					Breached:         false,
					Label:            "剩余 15 分钟",
					Source:           "source_refs.sla_due_at",
				},
				RecentEvent: &WorkflowInstanceRecentEvent{
					EventType:  string(ProjectEventDecisionRequested),
					Summary:    "已创建恢复决策请求",
					OccurredAt: time.Now().UTC().Add(-30 * time.Second),
				},
			},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	items, err := service.ListWorkflowInstances(context.Background(), ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
	})
	if err != nil {
		t.Fatalf("list workflow instances: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected three workflow instances, got %#v", items)
	}
	if items[0].DemandID != waitingDemandID {
		t.Fatalf("expected waiting-human workflow first, got %#v", items)
	}
	if items[0].Status != WorkflowInstanceStatusWaitingHuman {
		t.Fatalf("expected waiting_human status, got %s", items[0].Status)
	}
	if items[0].Priority == nil || items[0].Priority.Label != "P1" {
		t.Fatalf("expected priority field to survive service normalization: %#v", items[0].Priority)
	}
	if items[0].Risk == nil || items[0].Risk.Level != "high" {
		t.Fatalf("expected risk field to survive service normalization: %#v", items[0].Risk)
	}
	if items[0].SLA == nil || items[0].SLA.RemainingSeconds == nil || *items[0].SLA.RemainingSeconds != 900 {
		t.Fatalf("expected SLA field to survive service normalization: %#v", items[0].SLA)
	}
	if items[0].RecentEvent == nil || items[0].RecentEvent.EventType != string(ProjectEventDecisionRequested) {
		t.Fatalf("expected recent event field to survive service normalization: %#v", items[0].RecentEvent)
	}
}

func TestSubmitDemandPersistsPrimaryOwnerFallbackWhenReviewerOmitted(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", Content: "统计 PR 并分派审查",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}

	if demand.ReviewerPreference == nil {
		t.Fatalf("expected reviewer preference on demand: %#v", demand)
	}
	if demand.ReviewerPreference.ReviewerUserID != ownerID {
		t.Fatalf("expected owner %s, got %#v", ownerID, demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectHumanOwnerFallback {
		t.Fatalf("unexpected reviewer reason: %#v", demand.ReviewerPreference)
	}
	if demand.SourceRefs["reviewer_user_id"] != ownerID.String() {
		t.Fatalf("expected owner persisted in source refs: %#v", demand.SourceRefs)
	}
}

func TestSubmitDemandPersistsExplicitReviewerSelectionReason(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	reviewerName := "审查负责人"
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, DisplayNameSnapshot: &reviewerName, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", ReviewerUserID: &reviewerID,
		ReviewerSelectionReason: ReviewerSelectionProjectReviewerDefault,
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}

	if demand.ReviewerPreference == nil {
		t.Fatalf("expected reviewer preference on demand: %#v", demand)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectReviewerDefault {
		t.Fatalf("expected explicit reason to be preserved, got %#v", demand.ReviewerPreference)
	}
	if demand.SourceRefs["reviewer_selection_reason"] != string(ReviewerSelectionProjectReviewerDefault) {
		t.Fatalf("expected reviewer reason persisted in source refs: %#v", demand.SourceRefs)
	}
	if demand.SourceRefs["reviewer_display_name"] != reviewerName {
		t.Fatalf("expected reviewer display name persisted in source refs: %#v", demand.SourceRefs)
	}
}

func TestSubmitDemandRejectsInvalidReviewerSelectionReason(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", ReviewerUserID: &reviewerID,
		ReviewerSelectionReason: ReviewerSelectionReason("invalid_reason"),
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid project member, got %v", err)
	}
}

func TestSubmitDemandDiscardsSpoofedReviewerSourceRefs(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", ReviewerUserID: &reviewerID,
		SourceRefs: map[string]any{
			"reviewer_display_name": "Spoofed",
			"reviewer_user_id":      "bad",
			"external_ticket":       "T-1",
		},
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}

	if demand.SourceRefs["reviewer_user_id"] != reviewerID.String() {
		t.Fatalf("expected canonical reviewer id, got source refs: %#v", demand.SourceRefs)
	}
	if _, ok := demand.SourceRefs["reviewer_display_name"]; ok {
		t.Fatalf("expected spoofed display name to be discarded: %#v", demand.SourceRefs)
	}
	if demand.SourceRefs["external_ticket"] != "T-1" {
		t.Fatalf("expected non-reviewer source ref to remain: %#v", demand.SourceRefs)
	}
}

func TestSubmitDemandFallsBackToHumanOwnerWhenNoReviewer(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
		ProjectRole: ProjectRoleOwner, Status: "active",
	}}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "补充证据",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.ReviewerPreference == nil || demand.ReviewerPreference.ReviewerUserID != ownerID {
		t.Fatalf("expected owner fallback preference: %#v", demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectHumanOwnerFallback {
		t.Fatalf("expected owner fallback reason, got %#v", demand.ReviewerPreference)
	}
}

func TestSubmitDemandRequiresActiveHumanOwnerMemberForFallback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		members []ProjectMember
	}{
		{name: "missing owner member"},
		{
			name: "inactive owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeHumanUser,
				ProjectRole:   ProjectRoleOwner,
				Status:        "inactive",
			}},
		},
		{
			name: "digital owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeDigitalEmployee,
				ProjectRole:   ProjectRoleOwner,
				Status:        "active",
			}},
		},
		{
			name: "observer owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeHumanUser,
				ProjectRole:   ProjectRoleObserver,
				Status:        "active",
			}},
		},
		{
			name: "executor owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeHumanUser,
				ProjectRole:   ProjectRoleExecutor,
				Status:        "active",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tenantID := uuid.New()
			projectID := uuid.New()
			ownerID := uuid.New()
			repo := newMemoryRepository()
			repo.projects[projectID] = Project{
				ID:               projectID,
				TenantID:         tenantID,
				Status:           ProjectStatusRunning,
				HumanOwnerUserID: ownerID,
			}
			for _, member := range tc.members {
				member.ID = uuid.New()
				member.TenantID = tenantID
				member.ProjectID = projectID
				member.PrincipalID = ownerID
				repo.members[projectID] = append(repo.members[projectID], member)
			}
			// 通过数字员工门禁,才能验证负责人回落逻辑。
			if !projectHasActiveDigitalEmployee(repo.members[projectID]) {
				seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())
			}
			service, err := NewService(repo)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
				TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
				Title: "补充证据",
			})
			if !errors.Is(err, ErrInvalidProjectMember) {
				t.Fatalf("expected invalid project member, got %v", err)
			}
		})
	}
}

func TestReviewerPreferenceFromSourceRefsRestoresDisplayName(t *testing.T) {
	reviewerID := uuid.New()
	preference := reviewerPreferenceFromSourceRefs(map[string]any{
		"reviewer_user_id":            reviewerID.String(),
		"reviewer_selection_reason":   string(ReviewerSelectionProjectReviewerDefault),
		"reviewer_project_role":       string(ProjectRoleReviewer),
		"reviewer_resolved_from_rule": true,
		"reviewer_display_name":       "审查负责人",
	})

	if preference == nil {
		t.Fatal("expected reviewer preference")
	}
	if preference.DisplayName == nil || *preference.DisplayName != "审查负责人" {
		t.Fatalf("expected display name restored, got %#v", preference)
	}
	if preference.ReviewerUserID != reviewerID || preference.SelectionReason != ReviewerSelectionProjectReviewerDefault || preference.ProjectRole != ProjectRoleReviewer || !preference.ResolvedFromRule {
		t.Fatalf("unexpected reviewer preference: %#v", preference)
	}
}

func TestSubmitDemandRejectsDigitalEmployeeReviewer(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	digitalEmployeeID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: digitalEmployeeID,
			ProjectRole: ProjectRoleExecutor, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "需要审核", ReviewerUserID: &digitalEmployeeID,
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid project member, got %v", err)
	}
}

func TestSubmitDemandFallsBackToPrimaryOwnerWhenMultipleReviewersExist(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
		ProjectRole: ProjectRoleOwner, Status: "active",
	}}
	for range 2 {
		repo.members[projectID] = append(repo.members[projectID], ProjectMember{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: uuid.New(),
			ProjectRole: ProjectRoleReviewer, Status: "active",
		})
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "多审核人项目",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.ReviewerPreference == nil || demand.ReviewerPreference.ReviewerUserID != ownerID {
		t.Fatalf("expected owner fallback preference: %#v", demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectHumanOwnerFallback {
		t.Fatalf("expected owner fallback reason, got %#v", demand.ReviewerPreference)
	}
}

func TestProjectGovernanceCreatesEvidenceAndProjectEvent(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: actorID}

	evidence, err := service.CreateEvidenceRef(context.Background(), CreateEvidenceRefServiceRequest{
		TenantID: tenantID, ProjectID: projectID, ActorType: "human_user", ActorID: actorID,
		EvidenceType: "test_result", Title: "回归测试结果", SourceType: "artifact",
		SourceRef: "s3://bucket/reports/regression.json", SubmittedByType: "human_user", SubmittedByID: &actorID,
	})
	if err != nil {
		t.Fatalf("create evidence: %v", err)
	}
	if evidence.VerificationStatus != EvidenceVerificationStatusSubmitted {
		t.Fatalf("expected submitted evidence, got %s", evidence.VerificationStatus)
	}
	if repo.eventTypes[len(repo.eventTypes)-1] != ProjectEventEvidenceLinked {
		t.Fatalf("expected evidence event, got %#v", repo.eventTypes)
	}
}

func TestProjectAcceptanceRequiresHumanOwnerAndFinalReport(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}

	_, err = service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID: tenantID, ProjectID: projectID, AcceptedByUserID: otherUserID,
		Status: "accepted", Conclusion: "通过", EvidenceRefIDs: []uuid.UUID{uuid.New()}, ReportRefIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, ErrInvalidProjectAcceptance) {
		t.Fatalf("expected invalid acceptance actor, got %v", err)
	}
}

func TestProjectGovernanceEvidenceFailureDoesNotLeaveSuccessEvent(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	repo.createEvidenceRefErr = fmt.Errorf("evidence store unavailable")
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: actorID}

	_, err = service.CreateEvidenceRef(context.Background(), CreateEvidenceRefServiceRequest{
		TenantID: tenantID, ProjectID: projectID, ActorType: "human_user", ActorID: actorID,
		EvidenceType: "test_result", Title: "回归测试结果", SourceType: "artifact",
		SourceRef: "s3://bucket/reports/regression.json", SubmittedByType: "human_user", SubmittedByID: &actorID,
	})
	if err == nil {
		t.Fatal("expected evidence write error")
	}
	if countProjectEvents(repo.eventTypes, ProjectEventEvidenceLinked) != 0 {
		t.Fatalf("expected no success event after evidence failure, got %#v", repo.eventTypes)
	}
}

func TestProjectPatchEvidencePreservesOrClearsMetadata(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: ownerID}
	evidence, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceType: "test_result", Title: "回归测试结果",
		SourceType: "artifact", SourceRef: "s3://bucket/reports/regression.json",
		SubmittedByType: "human_user", SubmittedByID: &ownerID, VerificationStatus: EvidenceVerificationStatusSubmitted,
		Metadata: map[string]any{"suite": "regression", "passed": true},
	})
	if err != nil {
		t.Fatalf("seed evidence: %v", err)
	}

	updated, err := service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: evidence.ID, ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusVerified,
	})
	if err != nil {
		t.Fatalf("patch evidence with omitted metadata: %v", err)
	}
	if updated.VerificationStatus != EvidenceVerificationStatusVerified || updated.Metadata["suite"] != "regression" || updated.Metadata["passed"] != true {
		t.Fatalf("expected omitted metadata to keep existing values, got %#v", updated)
	}

	cleared, err := service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: evidence.ID, ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusRejected,
		Metadata:           map[string]any{},
	})
	if err != nil {
		t.Fatalf("patch evidence with empty metadata: %v", err)
	}
	if cleared.VerificationStatus != EvidenceVerificationStatusRejected || len(cleared.Metadata) != 0 {
		t.Fatalf("expected explicit empty metadata to clear values, got %#v", cleared)
	}
}

func TestProjectPatchEvidenceEventFailureRollsBackStatusAndMetadata(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: ownerID}
	evidence, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceType: "test_result", Title: "回归测试结果",
		SourceType: "artifact", SourceRef: "s3://bucket/reports/regression.json",
		SubmittedByType: "human_user", SubmittedByID: &ownerID, VerificationStatus: EvidenceVerificationStatusSubmitted,
		Metadata: map[string]any{"suite": "regression"},
	})
	if err != nil {
		t.Fatalf("seed evidence: %v", err)
	}
	repo.appendProjectEventErr = errors.New("event store unavailable")

	_, err = service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: evidence.ID, ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusVerified,
		Metadata:           map[string]any{"suite": "smoke"},
	})
	if err == nil {
		t.Fatal("expected event write failure")
	}
	if repo.evidenceRefs[0].VerificationStatus != EvidenceVerificationStatusSubmitted || repo.evidenceRefs[0].Metadata["suite"] != "regression" {
		t.Fatalf("expected evidence update rolled back, got %#v", repo.evidenceRefs[0])
	}
	if countProjectEvents(repo.eventTypes, ProjectEventEvidenceVerified) != 0 {
		t.Fatalf("expected no verification event after rollback, got %#v", repo.eventTypes)
	}
}

func TestProjectGovernanceMissingRecordsReturnNotFound(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: ownerID}

	_, err = service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: uuid.New(), ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusVerified,
	})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing evidence not found, got %v", err)
	}
	record, err := service.GetAcceptance(context.Background(), tenantID, projectID)
	if err != nil {
		t.Fatalf("expected no error for existing project without acceptance, got %v", err)
	}
	if record != nil {
		t.Fatalf("expected nil acceptance record for existing project without acceptance, got %#v", record)
	}
	_, err = service.GetAcceptance(context.Background(), tenantID, uuid.New())
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing project not found, got %v", err)
	}
	_, err = service.GetConfigRevision(context.Background(), tenantID, projectID, uuid.New())
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing config revision not found, got %v", err)
	}
}

func TestProjectBudgetSummaryRequiresExistingProject(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.GetBudgetSummary(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing project budget summary to return not found, got %v", err)
	}
}

func TestProjectAcceptanceRejectsMissingEvidenceOrReportRefs(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}

	_, err = service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID: tenantID, ProjectID: projectID, AcceptedByUserID: ownerID,
		Status: "accepted", Conclusion: "通过", EvidenceRefIDs: []uuid.UUID{uuid.New()}, ReportRefIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, ErrInvalidProjectAcceptance) {
		t.Fatalf("expected invalid acceptance refs, got %v", err)
	}
	if countProjectEvents(repo.eventTypes, ProjectEventAcceptanceSubmitted) != 0 {
		t.Fatalf("expected no acceptance event for invalid refs, got %#v", repo.eventTypes)
	}
}

func TestProjectAcceptanceSucceedsWithExistingEvidenceAndReportRefs(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	evidence, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		EvidenceType:       "test_result",
		Title:              "回归测试结果",
		SourceType:         "artifact",
		SourceRef:          "s3://bucket/reports/regression.json",
		SubmittedByType:    "human_user",
		SubmittedByID:      &ownerID,
		VerificationStatus: EvidenceVerificationStatusSubmitted,
	})
	if err != nil {
		t.Fatalf("seed evidence: %v", err)
	}
	report, err := repo.CreateReportRef(context.Background(), CreateReportRefRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		ReportType:      "final_report",
		Title:           "验收报告",
		ObjectRef:       "s3://bucket/reports/final.md",
		Format:          "markdown",
		GeneratedByType: "human_user",
		GeneratedByID:   &ownerID,
	})
	if err != nil {
		t.Fatalf("seed report: %v", err)
	}

	acceptance, err := service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID: tenantID, ProjectID: projectID, AcceptedByUserID: ownerID,
		Status: "accepted", Conclusion: "通过", EvidenceRefIDs: []uuid.UUID{evidence.ID}, ReportRefIDs: []uuid.UUID{report.ID},
	})
	if err != nil {
		t.Fatalf("create acceptance: %v", err)
	}
	if acceptance.CreatedEventID == nil || countProjectEvents(repo.eventTypes, ProjectEventAcceptanceSubmitted) != 1 {
		t.Fatalf("expected acceptance event and record link, events=%#v acceptance=%#v", repo.eventTypes, acceptance)
	}
}

func TestProjectArchivePreviewCountsAllPages(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}

	for i := 0; i < 105; i++ {
		_, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
			TenantID:           tenantID,
			ProjectID:          projectID,
			EvidenceType:       "test_result",
			Title:              fmt.Sprintf("证据 %d", i),
			SourceType:         "artifact",
			SourceRef:          fmt.Sprintf("s3://bucket/evidence/%d.json", i),
			SubmittedByType:    "human_user",
			SubmittedByID:      &ownerID,
			VerificationStatus: EvidenceVerificationStatusSubmitted,
		})
		if err != nil {
			t.Fatalf("seed evidence: %v", err)
		}
	}
	for i := 0; i < 103; i++ {
		_, err := repo.CreateArtifactRef(context.Background(), CreateArtifactRefRequest{
			TenantID:        tenantID,
			ProjectID:       projectID,
			ArtifactType:    "execution_log",
			Title:           fmt.Sprintf("工件 %d", i),
			ObjectRef:       fmt.Sprintf("s3://bucket/artifacts/%d.log", i),
			RetentionStatus: "locked",
		})
		if err != nil {
			t.Fatalf("seed artifact: %v", err)
		}
	}
	for i := 0; i < 102; i++ {
		_, err := repo.CreateReportRef(context.Background(), CreateReportRefRequest{
			TenantID:        tenantID,
			ProjectID:       projectID,
			ReportType:      "final_report",
			Title:           fmt.Sprintf("报告 %d", i),
			ObjectRef:       fmt.Sprintf("s3://bucket/reports/%d.md", i),
			Format:          "markdown",
			GeneratedByType: "human_user",
			GeneratedByID:   &ownerID,
		})
		if err != nil {
			t.Fatalf("seed report: %v", err)
		}
	}

	preview, err := service.BuildArchivePreview(context.Background(), tenantID, projectID)
	if err != nil {
		t.Fatalf("build archive preview: %v", err)
	}
	if preview.EvidenceCount != 105 || preview.ArtifactCount != 103 || preview.ReportCount != 102 {
		t.Fatalf("expected full counts, got evidence=%d artifact=%d report=%d", preview.EvidenceCount, preview.ArtifactCount, preview.ReportCount)
	}
}

func TestArchiveSnapshotLocksReferencedArtifactsBeforeArchiving(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	lockEventID := uuid.New()
	locker := &fakeArchiveArtifactLocker{eventID: &lockEventID}
	service, err := NewServiceWithArchiveArtifactLocker(repo, locker)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	artifactID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	repo.artifactRefs = append(repo.artifactRefs, ProjectArtifactRef{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ArtifactID: &artifactID,
		ObjectRef: "s3://bucket/report.md", Title: "最终报告",
	})
	snapshot, err := service.CreateArchiveSnapshot(context.Background(), CreateArchiveSnapshotServiceRequest{
		TenantID: tenantID, ProjectID: projectID, CreatedByUserID: ownerID,
		SnapshotType: "final_archive", Summary: "验收通过后归档", ObjectRef: "s3://bucket/archive/project.json",
	})
	if err != nil {
		t.Fatalf("archive snapshot: %v", err)
	}
	if snapshot.Status != "archived" {
		t.Fatalf("expected archived snapshot, got %s", snapshot.Status)
	}
	if len(locker.artifactIDs) != 1 || locker.artifactIDs[0] != artifactID {
		t.Fatalf("expected artifact lock, got %#v", locker.artifactIDs)
	}
	if snapshot.RetentionLockEventID == nil || *snapshot.RetentionLockEventID != lockEventID {
		t.Fatalf("expected retention lock event id %s, got %#v", lockEventID, snapshot.RetentionLockEventID)
	}
	if repo.projects[projectID].Status != ProjectStatusArchived {
		t.Fatalf("expected project archived after retention lock, got %s", repo.projects[projectID].Status)
	}
}

func TestArchiveSnapshotStaysPendingWhenArtifactLockFails(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	locker := &fakeArchiveArtifactLocker{err: errors.New("retention store unavailable")}
	service, err := NewServiceWithArchiveArtifactLocker(repo, locker)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	artifactID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	repo.artifactRefs = append(repo.artifactRefs, ProjectArtifactRef{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ArtifactID: &artifactID,
		ObjectRef: "s3://bucket/report.md", Title: "最终报告",
	})

	snapshot, err := service.CreateArchiveSnapshot(context.Background(), CreateArchiveSnapshotServiceRequest{
		TenantID: tenantID, ProjectID: projectID, CreatedByUserID: ownerID,
		SnapshotType: "final_archive", Summary: "验收通过后归档", ObjectRef: "s3://bucket/archive/project.json",
	})
	if err != nil {
		t.Fatalf("archive snapshot should return pending state without error: %v", err)
	}
	if snapshot.Status != "archive_pending_retention" {
		t.Fatalf("expected retention pending snapshot, got %s", snapshot.Status)
	}
	if repo.projects[projectID].Status == ProjectStatusArchived {
		t.Fatalf("project must not be archived when retention lock fails")
	}
	if len(repo.archiveSnapshots) != 1 || repo.archiveSnapshots[0].Status != "archive_pending_retention" {
		t.Fatalf("expected persisted pending snapshot, got %#v", repo.archiveSnapshots)
	}
}

func TestArchiveSnapshotReturnsArchiveProjectErrorAfterSuccessfulLock(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	repo.archiveProjectErr = errors.New("archive update failed")
	locker := &fakeArchiveArtifactLocker{}
	service, err := NewServiceWithArchiveArtifactLocker(repo, locker)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	artifactID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	repo.artifactRefs = append(repo.artifactRefs, ProjectArtifactRef{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ArtifactID: &artifactID,
		ObjectRef: "s3://bucket/report.md", Title: "最终报告",
	})

	_, err = service.CreateArchiveSnapshot(context.Background(), CreateArchiveSnapshotServiceRequest{
		TenantID: tenantID, ProjectID: projectID, CreatedByUserID: ownerID,
		SnapshotType: "final_archive", Summary: "验收通过后归档", ObjectRef: "s3://bucket/archive/project.json",
	})
	if !errors.Is(err, repo.archiveProjectErr) {
		t.Fatalf("expected archive project error, got %v", err)
	}
	if len(repo.archiveSnapshots) != 0 {
		t.Fatalf("expected archived snapshot to roll back after archive project failure, got %#v", repo.archiveSnapshots)
	}
	if countProjectEvents(repo.eventTypes, ProjectEventArchiveSnapshotCreated) != 0 {
		t.Fatalf("expected archive snapshot event to roll back after archive project failure, got %#v", repo.eventTypes)
	}
	if repo.projects[projectID].Status == ProjectStatusArchived {
		t.Fatalf("project must not be marked archived when repository archive update fails")
	}
}

func TestSubmitDemandSignalsProjectCoordinatorInV1(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "customer-runtime-acceptance",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)
	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.Status != ProjectDemandStatusPlanningPending {
		t.Fatalf("expected planning pending demand, got %s", demand.Status)
	}
	if coordinator.demandSignals != 1 {
		t.Fatalf("expected one DemandSubmitted signal, got %d", coordinator.demandSignals)
	}
	if coordinator.ensureSignals != 1 {
		t.Fatalf("expected coordinator to be ensured before demand signal, got %d", coordinator.ensureSignals)
	}
	if coordinator.lastDemand.DemandID != demand.ID || coordinator.lastDemand.CreatedEventID == uuid.Nil {
		t.Fatalf("unexpected demand signal: %#v", coordinator.lastDemand)
	}
}

func TestSubmitDemandRecordsRetryableWorkflowSignalFailure(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{demandSignalErr: errors.New("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "customer-runtime-acceptance",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)
	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err == nil {
		t.Fatal("expected signal error")
	}
	if len(repo.eventTypes) != 3 || repo.eventTypes[1] != ProjectEventWorkflowSignaled || repo.eventTypes[2] != ProjectEventWorkflowCoordinationFailed {
		t.Fatalf("expected workflow signal failure event, got %#v", repo.eventTypes)
	}
	payload := lastProjectEventOfType(t, repo.events, ProjectEventWorkflowSignaled).Payload
	if payload["signal_name"] != "DemandSubmitted" || payload["status"] != "failed" || payload["retryable"] != true {
		t.Fatalf("unexpected workflow signal payload: %#v", payload)
	}
	if payload["demand_id"] == "" || payload["error"] == "" {
		t.Fatalf("expected retry payload to include demand id and error: %#v", payload)
	}
}

// TestSubmitDemandRejectsWithoutDigitalEmployee: 空数字员工池不得提交需求、
// 不得启动规划——在入口硬失败,避免规划器胡填员工 ID 后再开 planning_failed 卡。
func TestSubmitDemandRejectsWithoutDigitalEmployee(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "empty-pool-project",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	// 仅人类负责人,不挂数字员工——门禁应拦截。
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if !errors.Is(err, ErrProjectRequiresDigitalEmployee) {
		t.Fatalf("expected ErrProjectRequiresDigitalEmployee, got %v", err)
	}
	if err.Error() != "项目至少包含一个数字员工" {
		t.Fatalf("expected Chinese message, got %q", err.Error())
	}
	// 不得写 demand / 不得 signal coordinator。
	if len(repo.demands) != 0 {
		t.Fatalf("expected no demand created, got %d", len(repo.demands))
	}
	if coordinator.demandSignals != 0 {
		t.Fatalf("expected no DemandSubmitted signal, got %d", coordinator.demandSignals)
	}
	// 也不得落 demand.submitted 事件(门禁在事件之前)。
	if countProjectEvents(repo.eventTypes, ProjectEventDemandSubmitted) != 0 {
		t.Fatalf("expected no demand.submitted event, got %#v", repo.eventTypes)
	}
}

func TestSubmitDemandAcceptsWithActiveDigitalEmployee(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "staffed-project",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)
	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "有员工时应能提交",
		Content:           "内容",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand == nil || demand.ID == uuid.Nil {
		t.Fatal("expected demand")
	}
	if coordinator.demandSignals != 1 {
		t.Fatalf("expected DemandSubmitted signal, got %d", coordinator.demandSignals)
	}
}

func TestProjectHasActiveDigitalEmployeeIgnoresInactiveAndHumans(t *testing.T) {
	members := []ProjectMember{
		{PrincipalType: PrincipalTypeHumanUser, Status: "active"},
		{PrincipalType: PrincipalTypeDigitalEmployee, Status: "removed"},
		{PrincipalType: PrincipalTypeDigitalEmployee, Status: "inactive"},
	}
	if projectHasActiveDigitalEmployee(members) {
		t.Fatal("expected false for no active digital employee")
	}
	members = append(members, ProjectMember{PrincipalType: PrincipalTypeDigitalEmployee, Status: "active"})
	if !projectHasActiveDigitalEmployee(members) {
		t.Fatal("expected true with one active digital employee")
	}
}

func TestRetryWorkflowSignalReplaysFailedDemandSignal(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{demandSignalErr: fmt.Errorf("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "customer-runtime-acceptance",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)
	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err == nil {
		t.Fatal("expected first signal error")
	}
	failedEvent := lastProjectEventOfType(t, repo.events, ProjectEventWorkflowSignaled)
	coordinator.demandSignalErr = nil

	event, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedEvent.ID,
		ActorID:   ownerID,
	})
	if err != nil {
		t.Fatalf("retry workflow signal: %v", err)
	}
	if repo.demands[0].CreatedEventID == nil {
		t.Fatalf("expected demand created event id: %#v", repo.demands[0])
	}
	if coordinator.demandSignals != 2 || coordinator.lastDemand.DemandID != repo.demands[0].ID || coordinator.lastDemand.CreatedEventID != *repo.demands[0].CreatedEventID {
		t.Fatalf("expected demand signal replay, count=%d signal=%#v demand=%#v", coordinator.demandSignals, coordinator.lastDemand, repo.demands[0])
	}
	if coordinator.ensureSignals != 2 {
		t.Fatalf("expected coordinator ensure before initial and retried demand signals, got %d", coordinator.ensureSignals)
	}
	if event.EventType != ProjectEventWorkflowSignaled || event.Payload["signal_name"] != "DemandSubmitted" || event.Payload["status"] != "sent" || event.Payload["retry_of_event_id"] != failedEvent.ID.String() {
		t.Fatalf("unexpected retry event: %#v", event)
	}
}

func TestParseEvidenceRefElement(t *testing.T) {
	if _, ok := parseEvidenceRefElement(map[string]any{"summary": "no ref"}); ok {
		t.Fatalf("expected element without source ref to be skipped")
	}
	if _, ok := parseEvidenceRefElement(42); ok {
		t.Fatalf("expected non string/map element to be skipped")
	}
	strParsed, ok := parseEvidenceRefElement("s3://bucket/report.md")
	if !ok || strParsed.SourceRef != "s3://bucket/report.md" || strParsed.Title != "s3://bucket/report.md" ||
		strParsed.EvidenceType != "execution_evidence" || strParsed.SourceType != "runtime_output" {
		t.Fatalf("unexpected string parse: %#v ok=%v", strParsed, ok)
	}
	mapParsed, ok := parseEvidenceRefElement(map[string]any{
		"ref": "doc-1", "title": "需求摘要文档", "summary": "v1.0", "type": "document", "source_type": "workspace_file",
	})
	if !ok || mapParsed.SourceRef != "doc-1" || mapParsed.Title != "需求摘要文档" || mapParsed.Summary != "v1.0" ||
		mapParsed.EvidenceType != "document" || mapParsed.SourceType != "workspace_file" {
		t.Fatalf("unexpected map parse: %#v ok=%v", mapParsed, ok)
	}
}

func TestParseArtifactRefElement(t *testing.T) {
	if _, ok := parseArtifactRefElement(map[string]any{"title": "no ref"}); ok {
		t.Fatalf("expected element without object ref to be skipped")
	}
	strParsed, ok := parseArtifactRefElement("artifact-1")
	if !ok || strParsed.ObjectRef != "artifact-1" || strParsed.Title != "artifact-1" || strParsed.ArtifactType != "execution_artifact" {
		t.Fatalf("unexpected string parse: %#v ok=%v", strParsed, ok)
	}
	mapParsed, ok := parseArtifactRefElement(map[string]any{"id": "plan-0.1", "title": "任务计划草案", "type": "plan", "content_type": "text/markdown"})
	if !ok || mapParsed.ObjectRef != "plan-0.1" || mapParsed.Title != "任务计划草案" || mapParsed.ArtifactType != "plan" || mapParsed.ContentType != "text/markdown" {
		t.Fatalf("unexpected map parse: %#v ok=%v", mapParsed, ok)
	}
}

func TestCompleteProjectTaskWritesSummaryAndSignalsCoordinator(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:              tenantID,
		RuntimeNodeID:         runtimeNodeID,
		ProjectTaskID:         taskID,
		DigitalEmployeeID:     employeeID,
		Conclusion:            "证据充分",
		EvidenceRefs:          []any{"s3://bucket/report.md"},
		ArtifactRefs:          []any{"artifact-1"},
		ConfidenceFactors:     map[string]any{"tests": "passed"},
		RecommendedNextAction: "提交负责人验收",
	})
	if err != nil {
		t.Fatalf("complete project task: %v", err)
	}
	if summary.ProjectTaskID != taskID || summary.DigitalEmployeeID != employeeID {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.CreatedEventID == nil {
		t.Fatalf("expected summary to reference created event: %#v", summary)
	}
	if repo.tasks[0].Status != "completed" {
		t.Fatalf("expected task completed, got %s", repo.tasks[0].Status)
	}
	if coordinator.completedSignals != 1 || coordinator.lastCompleted.ExecutionSummaryID != summary.ID {
		t.Fatalf("expected completed signal for summary, got count=%d signal=%#v", coordinator.completedSignals, coordinator.lastCompleted)
	}
	if countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected one completed event, got %#v", repo.eventTypes)
	}
	if countProjectEvents(repo.eventTypes, ProjectEventEvidenceLinked) != 1 || countProjectEvents(repo.eventTypes, ProjectEventArtifactLinked) != 1 {
		t.Fatalf("expected evidence+artifact materialization events, got %#v", repo.eventTypes)
	}
}

func TestCompleteProjectTaskRejectsMissingRequiredEvidence(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "evidence_refs"},
		HandoffContract: map[string]any{},
	})

	_, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
		EvidenceRefs:      nil,
	})

	require.ErrorIs(t, err, ErrInvalidProjectEvidence)
	require.Equal(t, "assigned", repo.tasks[0].Status)
	require.Len(t, repo.executionSummaries, 0)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, []ProjectEventType{ProjectEventTaskContractMissing}, repo.eventTypes)
	require.Equal(t, task.ID.String(), repo.events[0].Payload["project_task_id"])
	require.Equal(t, []any{"evidence_refs"}, repo.events[0].Payload["missing_outputs"])
}

func TestCompleteProjectTaskWithRequiredOutputsContractWritesSummaryAndSignalsCoordinator(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "evidence_refs", "artifact_refs", "recommended_next_action"},
		HandoffContract: map[string]any{},
	})

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:              task.TenantID,
		RuntimeNodeID:         runtimeNodeID,
		ProjectTaskID:         task.ID,
		DigitalEmployeeID:     *task.AssignedDigitalEmployeeID,
		Conclusion:            "完成",
		EvidenceRefs:          []any{"evidence://report"},
		ArtifactRefs:          []any{"artifact://patch"},
		RecommendedNextAction: "提交验收",
	})

	require.NoError(t, err)
	require.Equal(t, task.ID, summary.ProjectTaskID)
	require.Equal(t, "completed", repo.tasks[0].Status)
	require.Len(t, repo.executionSummaries, 1)
	require.Equal(t, 1, coordinator.completedSignals)
	require.Equal(t, ProjectEventTaskCompleted, repo.eventTypes[0])
}

func TestCompleteProjectTaskMissingInformationContractRequiresExplicitArray(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "missing_information"},
		HandoffContract: map[string]any{},
	})

	_, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.ErrorIs(t, err, ErrInvalidProjectEvidence)
	require.Equal(t, []ProjectEventType{ProjectEventTaskContractMissing}, repo.eventTypes)
	require.Equal(t, []any{"missing_information"}, repo.events[0].Payload["missing_outputs"])
	require.Len(t, repo.executionSummaries, 0)
	require.Equal(t, 0, coordinator.completedSignals)

	repo.eventTypes = nil
	repo.events = nil
	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:            task.TenantID,
		RuntimeNodeID:       runtimeNodeID,
		ProjectTaskID:       task.ID,
		DigitalEmployeeID:   *task.AssignedDigitalEmployeeID,
		Conclusion:          "完成",
		MissingInformation:  []any{},
		ConfidenceFactors:   map[string]any{"contract": "explicit_empty_missing_information"},
		RequiresHumanReview: false,
	})

	require.NoError(t, err)
	require.Equal(t, task.ID, summary.ProjectTaskID)
	require.Equal(t, ProjectEventTaskCompleted, repo.eventTypes[0])
}

func TestCompleteProjectTaskWorkProductsContractRequiresBoundRunProducts(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "work_products"},
		HandoffContract: map[string]any{},
	})

	_, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.ErrorIs(t, err, ErrInvalidProjectEvidence)
	require.Equal(t, []ProjectEventType{ProjectEventTaskContractMissing}, repo.eventTypes)
	require.Equal(t, []any{"work_products"}, repo.events[0].Payload["missing_outputs"])
	require.Len(t, repo.executionSummaries, 0)
	require.Equal(t, 0, coordinator.completedSignals)

	repo.eventTypes = nil
	repo.events = nil
	repo.projectTaskRunWorkProducts[*task.DigitalEmployeeRunID] = []any{map[string]any{"ref": "wp://analysis", "title": "分析报告"}}
	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.NoError(t, err)
	require.Equal(t, task.ID, summary.ProjectTaskID)
	require.Equal(t, ProjectEventTaskCompleted, repo.eventTypes[0])
}

func TestCompleteProjectTaskHandoffContractRequiredRefsMissingCustomRefFails(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary"},
		HandoffContract: map[string]any{"required_refs": []any{"wp://analysis", "evidence://report"}},
	})
	repo.projectTaskRunWorkProducts[*task.DigitalEmployeeRunID] = []any{map[string]any{"ref": "wp://draft", "title": "草稿"}}

	_, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
		EvidenceRefs:      []any{"evidence://report"},
	})

	require.ErrorIs(t, err, ErrInvalidProjectEvidence)
	require.Len(t, repo.executionSummaries, 0)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, []ProjectEventType{ProjectEventTaskContractMissing}, repo.eventTypes)
	require.Equal(t, []any{"wp://analysis"}, repo.events[0].Payload["missing_handoff_refs"])
}

func TestCompleteProjectTaskHandoffContractRequiredRefsMatchWorkProductFields(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary"},
		HandoffContract: map[string]any{"required_refs": []any{"wp://analysis", "分析报告", "report"}},
	})
	repo.projectTaskRunWorkProducts[*task.DigitalEmployeeRunID] = []any{map[string]any{
		"ref":   "wp://analysis",
		"title": "分析报告",
		"type":  "report",
	}}

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.NoError(t, err)
	require.Equal(t, task.ID, summary.ProjectTaskID)
	require.Equal(t, "completed", repo.tasks[0].Status)
	require.Len(t, repo.executionSummaries, 1)
	require.Equal(t, 1, coordinator.completedSignals)
	require.Equal(t, ProjectEventTaskCompleted, repo.eventTypes[0])
}

func TestCompleteProjectTaskContractMissingEventAppendFailureReturnsAppendError(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "evidence_refs"},
		HandoffContract: map[string]any{},
	})
	appendErr := fmt.Errorf("event store unavailable")
	repo.appendProjectEventErr = appendErr

	_, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.ErrorIs(t, err, appendErr)
	require.Len(t, repo.executionSummaries, 0)
	require.Equal(t, "assigned", repo.tasks[0].Status)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Len(t, repo.eventTypes, 0)
}

func TestCompleteProjectTaskEmptyLegacyContractStillCompletes(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{})

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.NoError(t, err)
	require.Equal(t, task.ID, summary.ProjectTaskID)
	require.Equal(t, "completed", repo.tasks[0].Status)
	require.Len(t, repo.executionSummaries, 1)
	require.Equal(t, 1, coordinator.completedSignals)
	require.Equal(t, ProjectEventTaskCompleted, repo.eventTypes[0])
}

func newProjectServiceWritebackFixture(t *testing.T, taskOverrides ProjectTask) (*Service, *memoryRepository, *fakeCoordinatorSignalClient, ProjectTask, uuid.UUID) {
	t.Helper()

	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	task := ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
		ExpectedOutputs:           taskOverrides.ExpectedOutputs,
		HandoffContract:           taskOverrides.HandoffContract,
	}
	if task.HandoffContract == nil {
		task.HandoffContract = map[string]any{}
	}
	repo.tasks = append(repo.tasks, task)
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)
	task = repo.tasks[0]
	return service, repo, coordinator, task, runtimeNodeID
}

func TestBindProjectTaskRunEnablesCompleteProjectTaskWriteback(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "planned",
		AssignedDigitalEmployeeID: &employeeID,
	})

	bound, err := repo.BindProjectTaskRun(context.Background(), BindProjectTaskRunRequest{
		TenantID:             tenantID,
		ProjectTaskID:        taskID,
		DigitalEmployeeRunID: runID,
		RuntimeTaskID:        runtimeTaskID,
		CurrentStatuses:      []string{"planned", "pending"},
	})
	if err != nil {
		t.Fatalf("bind project task run: %v", err)
	}
	repo.projectTaskRunRuntimeNodes[taskID] = runtimeNodeID
	if bound.Status != "assigned" || bound.DigitalEmployeeRunID == nil || *bound.DigitalEmployeeRunID != runID ||
		bound.RuntimeTaskID == nil || *bound.RuntimeTaskID != runtimeTaskID {
		t.Fatalf("expected assigned run binding, got %#v", bound)
	}

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "证据充分",
	})
	if err != nil {
		t.Fatalf("complete project task: %v", err)
	}
	if summary.ProjectTaskID != taskID || summary.DigitalEmployeeID != employeeID {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if repo.tasks[0].Status != "completed" {
		t.Fatalf("expected task completed after runtime writeback, got %s", repo.tasks[0].Status)
	}
	if coordinator.completedSignals != 1 {
		t.Fatalf("expected coordinator completion signal, got %d", coordinator.completedSignals)
	}
}

func TestRetryWorkflowSignalReplaysCompletedTaskWithoutDuplicateWriteback(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{completedSignalErr: fmt.Errorf("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "证据充分",
	})
	if err == nil {
		t.Fatal("expected first completed signal error")
	}
	if len(repo.executionSummaries) != 1 {
		t.Fatalf("expected one summary after failed signal, got %d", len(repo.executionSummaries))
	}
	completedEvents := countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted)
	if completedEvents != 1 {
		t.Fatalf("expected one completed event after failed signal, got %d events=%#v", completedEvents, repo.eventTypes)
	}
	failedSignalEvent := lastProjectEventOfType(t, repo.events, ProjectEventWorkflowSignaled)
	coordinator.completedSignalErr = nil

	retryEvent, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedSignalEvent.ID,
		ActorID:   repo.projects[projectID].HumanOwnerUserID,
	})
	if err != nil {
		t.Fatalf("retry completed workflow signal: %v", err)
	}
	if coordinator.completedSignals != 2 || coordinator.lastCompleted.ProjectTaskID != taskID || coordinator.lastCompleted.ExecutionSummaryID != repo.executionSummaries[0].ID {
		t.Fatalf("expected completed signal replay, count=%d signal=%#v summary=%#v", coordinator.completedSignals, coordinator.lastCompleted, repo.executionSummaries[0])
	}
	if len(repo.executionSummaries) != 1 {
		t.Fatalf("expected retry not to create duplicate summary, got %d", len(repo.executionSummaries))
	}
	if countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected retry not to create duplicate completed event, events=%#v", repo.eventTypes)
	}
	if retryEvent.Payload["status"] != "sent" || retryEvent.Payload["retry_of_event_id"] != failedSignalEvent.ID.String() {
		t.Fatalf("unexpected retry event payload: %#v", retryEvent.Payload)
	}
}

func TestProjectCoordinationBackendE2ESimulation(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{demandSignalErr: fmt.Errorf("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "E2E 仿真项目",
		Goal:                   "验证需求、Runtime 写回和 Workflow signal 重试闭环",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)
	seedDigitalExecutorMember(repo, tenantID, projectID, uuid.New())

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 执行回写",
		Content:           "模拟 Temporal 短暂不可用后的重试恢复",
	})
	if err == nil {
		t.Fatal("expected demand signal failure")
	}
	if len(repo.demands) != 1 || countProjectEvents(repo.eventTypes, ProjectEventDemandSubmitted) != 1 {
		t.Fatalf("expected one persisted demand before retry, demands=%d events=%#v", len(repo.demands), repo.eventTypes)
	}
	failedDemandSignalEvent := lastProjectEventOfType(t, repo.events, ProjectEventWorkflowSignaled)
	if failedDemandSignalEvent.EventType != ProjectEventWorkflowSignaled || failedDemandSignalEvent.Payload["signal_name"] != "DemandSubmitted" || failedDemandSignalEvent.Payload["status"] != "failed" {
		t.Fatalf("expected retryable demand signal failure event, got %#v", failedDemandSignalEvent)
	}
	demandCoordinationFailedEvent := lastProjectEventOfType(t, repo.events, ProjectEventWorkflowCoordinationFailed)
	if demandCoordinationFailedEvent.Payload["reason_code"] != "workflow_signal_failed" || demandCoordinationFailedEvent.Payload["recommended_action"] != "inspect_workflow_signal_failure" {
		t.Fatalf("unexpected projected demand coordination failure event: %#v", demandCoordinationFailedEvent.Payload)
	}

	coordinator.demandSignalErr = nil
	retryDemandEvent, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedDemandSignalEvent.ID,
		ActorID:   ownerID,
	})
	if err != nil {
		t.Fatalf("retry demand workflow signal: %v", err)
	}
	if retryDemandEvent.Payload["status"] != "sent" || retryDemandEvent.Payload["retry_of_event_id"] != failedDemandSignalEvent.ID.String() {
		t.Fatalf("unexpected demand retry event payload: %#v", retryDemandEvent.Payload)
	}
	if coordinator.demandSignals != 2 || len(repo.demands) != 1 || countProjectEvents(repo.eventTypes, ProjectEventDemandSubmitted) != 1 {
		t.Fatalf("expected demand retry to only resend signal, signals=%d demands=%d events=%#v", coordinator.demandSignals, len(repo.demands), repo.eventTypes)
	}

	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理执行证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "错误 Runtime 尝试写回",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected wrong runtime rejection, got %v", err)
	}
	if repo.tasks[0].Status != "assigned" || len(repo.executionSummaries) != 0 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 0 {
		t.Fatalf("expected rejected runtime writeback to have no side effects, task=%#v summaries=%d events=%#v", repo.tasks[0], len(repo.executionSummaries), repo.eventTypes)
	}

	coordinator.completedSignalErr = fmt.Errorf("temporal unavailable")
	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:              tenantID,
		RuntimeNodeID:         runtimeNodeID,
		ProjectTaskID:         taskID,
		DigitalEmployeeID:     employeeID,
		Conclusion:            "证据充分",
		EvidenceRefs:          []any{"s3://bucket/e2e-report.md"},
		ArtifactRefs:          []any{"artifact-runtime-log"},
		ConfidenceFactors:     map[string]any{"tests": "passed"},
		RecommendedNextAction: "提交负责人验收",
	})
	if err == nil {
		t.Fatal("expected completed task signal failure")
	}
	if repo.tasks[0].Status != "completed" || len(repo.executionSummaries) != 1 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected successful writeback before signal retry, task=%#v summaries=%d events=%#v", repo.tasks[0], len(repo.executionSummaries), repo.eventTypes)
	}
	failedCompletedSignalEvent := lastProjectEventOfType(t, repo.events, ProjectEventWorkflowSignaled)
	if failedCompletedSignalEvent.EventType != ProjectEventWorkflowSignaled || failedCompletedSignalEvent.Payload["signal_name"] != "EmployeeTaskCompleted" || failedCompletedSignalEvent.Payload["status"] != "failed" {
		t.Fatalf("expected retryable completed signal failure event, got %#v", failedCompletedSignalEvent)
	}

	coordinator.completedSignalErr = nil
	retryCompletedEvent, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedCompletedSignalEvent.ID,
		ActorID:   ownerID,
	})
	if err != nil {
		t.Fatalf("retry completed workflow signal: %v", err)
	}
	if retryCompletedEvent.Payload["status"] != "sent" || retryCompletedEvent.Payload["retry_of_event_id"] != failedCompletedSignalEvent.ID.String() {
		t.Fatalf("unexpected completed retry event payload: %#v", retryCompletedEvent.Payload)
	}
	if coordinator.completedSignals != 2 || coordinator.lastCompleted.ExecutionSummaryID != repo.executionSummaries[0].ID {
		t.Fatalf("expected completed signal replay, signals=%d last=%#v summary=%#v", coordinator.completedSignals, coordinator.lastCompleted, repo.executionSummaries[0])
	}
	if len(repo.executionSummaries) != 1 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected completed retry not to duplicate facts, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}

	demands, err := service.ListProjectDemands(context.Background(), tenantID, projectID, 50, 0)
	if err != nil {
		t.Fatalf("list demands: %v", err)
	}
	summaries, err := service.ListExecutionSummaries(context.Background(), tenantID, projectID, 50, 0)
	if err != nil {
		t.Fatalf("list execution summaries: %v", err)
	}
	events, err := service.ListProjectEvents(context.Background(), tenantID, projectID, 50, 0)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(demands) != 1 || len(summaries) != 1 || countProjectEvents(projectEventTypes(events), ProjectEventWorkflowSignaled) != 4 || countProjectEvents(projectEventTypes(events), ProjectEventWorkflowCoordinationFailed) != 2 {
		t.Fatalf("unexpected API-facing read model: demands=%d summaries=%d events=%#v", len(demands), len(summaries), projectEventTypes(events))
	}
}

func TestProjectTaskWritebackRequiresRuntimeNodeIdentity(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "证据充分",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected runtime identity rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}
}

func TestProjectTaskWritebackRequiresDigitalEmployeeRunBinding(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "未绑定运行记录",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected missing run binding rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}
}

func TestBindProjectTaskRunRejectsSameRunDifferentRuntimeTask(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	runID := uuid.New()
	originalRuntimeTaskID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                   taskID,
		TenantID:             tenantID,
		ProjectID:            projectID,
		Title:                "整理证据",
		Status:               "assigned",
		DigitalEmployeeRunID: &runID,
		RuntimeTaskID:        &originalRuntimeTaskID,
	})

	_, err := repo.BindProjectTaskRun(context.Background(), BindProjectTaskRunRequest{
		TenantID:             tenantID,
		ProjectTaskID:        taskID,
		DigitalEmployeeRunID: runID,
		RuntimeTaskID:        uuid.New(),
		CurrentStatuses:      []string{"pending", "running"},
	})
	if !errors.Is(err, ErrProjectConflict) {
		t.Fatalf("expected project conflict, got %v", err)
	}
	if repo.tasks[0].RuntimeTaskID == nil || *repo.tasks[0].RuntimeTaskID != originalRuntimeTaskID {
		t.Fatalf("expected runtime task id to remain unchanged, got %#v", repo.tasks[0].RuntimeTaskID)
	}
}

func TestCompleteProjectTaskRejectsTerminalReplay(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "completed",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "重复完成",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected terminal replay rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 || coordinator.completedSignals != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v signals=%d", len(repo.executionSummaries), repo.eventTypes, coordinator.completedSignals)
	}
}

func TestCompleteProjectTaskRejectsConcurrentTerminalTransitionBeforeSideEffects(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	completed := "completed"
	repo.taskStatusBeforeUpdate = &completed
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "并发完成",
	})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected conditional status update rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 || coordinator.completedSignals != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v signals=%d", len(repo.executionSummaries), repo.eventTypes, coordinator.completedSignals)
	}
}

func TestCompleteProjectTaskRollsBackStatusWhenSummaryCreationFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.createExecutionSummaryErr = fmt.Errorf("summary unavailable")
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "写摘要失败",
	})
	if err == nil {
		t.Fatal("expected summary creation error")
	}
	if repo.tasks[0].Status != "assigned" || len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 || coordinator.completedSignals != 0 {
		t.Fatalf("expected rollback before side effects, task=%#v summaries=%d events=%#v signals=%d", repo.tasks[0], len(repo.executionSummaries), repo.eventTypes, coordinator.completedSignals)
	}
}

func TestFailProjectTaskRollsBackStatusWhenEventAppendFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.appendProjectEventErr = fmt.Errorf("event store unavailable")
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "running",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.FailProjectTask(context.Background(), FailProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		FailureSummary:    "工具链失败",
	})
	if err == nil {
		t.Fatal("expected event append error")
	}
	if repo.tasks[0].Status != "running" || len(repo.eventTypes) != 0 || coordinator.failedSignals != 0 {
		t.Fatalf("expected rollback before side effects, task=%#v events=%#v signals=%d", repo.tasks[0], repo.eventTypes, coordinator.failedSignals)
	}
}

func TestRequestProjectTaskTransferRollsBackStatusWhenTransferCreationFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.createTransferRequestErr = fmt.Errorf("transfer store unavailable")
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.RequestProjectTaskTransfer(context.Background(), RequestProjectTaskTransferRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Reason:            "上下文不足",
	})
	if err == nil {
		t.Fatal("expected transfer creation error")
	}
	if repo.tasks[0].Status != "assigned" || len(repo.transferRequests) != 0 || len(repo.eventTypes) != 0 || coordinator.transferSignals != 0 {
		t.Fatalf("expected rollback before side effects, task=%#v transfers=%d events=%#v signals=%d", repo.tasks[0], len(repo.transferRequests), repo.eventTypes, coordinator.transferSignals)
	}
}

func TestCompleteProjectTaskRejectsWrongRuntimeWhenRunIsBound(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runID := uuid.New()
	expectedRuntimeNodeID := uuid.New()
	repo.projectTaskRunRuntimeNodes[taskID] = expectedRuntimeNodeID
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
		DigitalEmployeeRunID:      &runID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "错误 Runtime 写回",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected wrong runtime rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}
}

func TestRequestProjectTaskTransferRejectsWaitingHumanTask(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "等待负责人确认",
		Status:                    "waiting_human",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.RequestProjectTaskTransfer(context.Background(), RequestProjectTaskTransferRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Reason:            "上下文不足",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected waiting human transfer rejection, got %v", err)
	}
	if len(repo.transferRequests) != 0 || len(repo.eventTypes) != 0 || coordinator.transferSignals != 0 {
		t.Fatalf("expected rejection before side effects, transfers=%d events=%#v signals=%d", len(repo.transferRequests), repo.eventTypes, coordinator.transferSignals)
	}
}

func TestRequestProjectTaskTransferMovesTaskToWaitingHuman(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	transfer, err := service.RequestProjectTaskTransfer(context.Background(), RequestProjectTaskTransferRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Reason:            "上下文不足",
	})
	if err != nil {
		t.Fatalf("request transfer: %v", err)
	}
	if transfer.Status != "requested" || repo.tasks[0].Status != "waiting_human" {
		t.Fatalf("expected transfer to pause task, transfer=%#v task=%#v", transfer, repo.tasks[0])
	}
	if coordinator.transferSignals != 1 {
		t.Fatalf("expected transfer signal, got %d", coordinator.transferSignals)
	}
}

func TestResolveDecisionUsesApprovalAndSignalsCoordinator(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "需要负责人确认",
		StatusSnapshot:    "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
		Comment:           "同意",
		Payload:           map[string]any{"source": "console"},
	})
	if err != nil {
		t.Fatalf("resolve decision: %v", err)
	}
	if resolved.StatusSnapshot != "approved" {
		t.Fatalf("expected approved projection, got %s", resolved.StatusSnapshot)
	}
	if approvals.calls != 1 || approvals.last.ApprovalRequestID != approvalID || approvals.last.Decision != "approved" {
		t.Fatalf("expected approval resolver call, got count=%d last=%#v", approvals.calls, approvals.last)
	}
	if approvals.last.Payload["source"] != "console" {
		t.Fatalf("expected approval payload to be preserved, got %#v", approvals.last.Payload)
	}
	if coordinator.decisionSignals != 1 || coordinator.lastDecision.DecisionRequestID != decisionID || coordinator.lastDecision.ResolvedEventID == uuid.Nil {
		t.Fatalf("expected decision signal, got count=%d signal=%#v", coordinator.decisionSignals, coordinator.lastDecision)
	}
	if coordinator.lastDecision.Payload["source"] != "console" {
		t.Fatalf("expected decision signal payload to be preserved, got %#v", coordinator.lastDecision.Payload)
	}
}

// TestResolveDecisionIdempotentSelfHealsFeishuCards: 同 decision 再次 resolve
// 时必须再调 EnsureDecisionCardsTerminal,用于补上首次竞态漏掉的 card_update。
func TestResolveDecisionIdempotentSelfHealsFeishuCards(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "demo", Goal: "g",
		Status: ProjectStatusRunning, HumanOwnerUserID: actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	now := time.Now().UTC()
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID: decisionID, TenantID: tenantID, ProjectID: projectID,
		TargetUserID: actorID, DecisionType: "route_review",
		TitleSnapshot: "需要负责人确认", StatusSnapshot: "approved",
		ResolvedAt: &now,
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID: tenantID, ProjectID: projectID, DecisionRequestID: decisionID,
		DecidedByUserID: actorID, Decision: "approved", Comment: "再点一次",
	})
	if err != nil {
		t.Fatalf("idempotent resolve: %v", err)
	}
	if resolved.StatusSnapshot != "approved" {
		t.Fatalf("status=%s", resolved.StatusSnapshot)
	}
	if len(repo.ensureDecisionCardsTerminalCalls) != 1 {
		t.Fatalf("expected EnsureDecisionCardsTerminal once, got %d", len(repo.ensureDecisionCardsTerminalCalls))
	}
	call := repo.ensureDecisionCardsTerminalCalls[0]
	if call.Decision.ID != decisionID || call.ResolvedBy != actorID || call.Comment != "再点一次" {
		t.Fatalf("unexpected ensure call %#v", call)
	}
	// 幂等路径不得再 signal coordinator / resolve approval。
	if coordinator.decisionSignals != 0 {
		t.Fatalf("idempotent path must not re-signal, got %d", coordinator.decisionSignals)
	}
	if approvals.calls != 0 {
		t.Fatalf("idempotent path must not re-resolve approval, got %d", approvals.calls)
	}
}

// TestResolveDecisionTerminalMismatchReprojectsInbox: a decision already
// terminal under a different verb (e.g. bulk-cancelled) must still re-project
// the inbox so a stale open card closes when the human taps 同意/驳回.
func TestResolveDecisionTerminalMismatchReprojectsInbox(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "demo", Goal: "g",
		Status: ProjectStatusRunning, HumanOwnerUserID: actorID,
	}
	now := time.Now().UTC()
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID: decisionID, TenantID: tenantID, ProjectID: projectID,
		TargetUserID: actorID, DecisionType: "project_task_recovery",
		TitleSnapshot: "Create gate-e2e-risk.txt", StatusSnapshot: "cancelled",
		ResolvedAt: &now,
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID: tenantID, ProjectID: projectID, DecisionRequestID: decisionID,
		DecidedByUserID: actorID, Decision: "approved",
	})
	if err != nil {
		t.Fatalf("terminal mismatch resolve: %v", err)
	}
	if resolved.StatusSnapshot != "cancelled" {
		t.Fatalf("must keep real terminal snapshot cancelled, got %s", resolved.StatusSnapshot)
	}
	if len(inbox.resolutions) != 1 {
		t.Fatalf("expected inbox re-project once, got %d", len(inbox.resolutions))
	}
	if inbox.resolutions[0].StatusSnapshot != "cancelled" {
		t.Fatalf("inbox projection must carry cancelled, got %s", inbox.resolutions[0].StatusSnapshot)
	}
}

func TestResolveDecisionThreadsTargetExitDeliverableToSignal(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "plan_review",
		TitleSnapshot:     "确认项目计划版本",
		StatusSnapshot:    "pending",
	})

	_, err = service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:              tenantID,
		ProjectID:             projectID,
		DecisionRequestID:     decisionID,
		DecidedByUserID:       actorID,
		Decision:              "rejected",
		Comment:               "改选出口",
		TargetExitDeliverable: "  branch_ref  ",
	})
	if err != nil {
		t.Fatalf("resolve decision: %v", err)
	}
	if coordinator.decisionSignals != 1 {
		t.Fatalf("expected decision signal, got count=%d", coordinator.decisionSignals)
	}
	if coordinator.lastDecision.TargetExitDeliverable != "branch_ref" {
		t.Fatalf("expected target exit deliverable to be threaded and trimmed, got %q", coordinator.lastDecision.TargetExitDeliverable)
	}
}

func TestResolveDecisionAcceptsRequestChangesForPlanReview(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	planRevisionID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.planRevisions = append(repo.planRevisions, PlanRevision{
		ID:        planRevisionID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Status:    PlanRevisionStatusPendingReview,
		Payload: map[string]any{
			"available_exits": []any{
				map[string]any{"deliverable": "review_verdict", "label": "审查通过"},
				map[string]any{"deliverable": "branch_ref", "label": "分支就绪"},
			},
		},
	})
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		PlanRevisionID:    &planRevisionID,
		TargetUserID:      actorID,
		DecisionType:      "plan_review",
		TitleSnapshot:     "确认项目计划版本",
		StatusSnapshot:    "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:              tenantID,
		ProjectID:             projectID,
		DecisionRequestID:     decisionID,
		DecidedByUserID:       actorID,
		Decision:              PlanReviewDecisionRequestChanges,
		Comment:               "改选出口",
		TargetExitDeliverable: "branch_ref",
	})
	if err != nil {
		t.Fatalf("resolve decision with request_changes: %v", err)
	}
	if resolved.StatusSnapshot != PlanReviewDecisionRequestChanges {
		t.Fatalf("expected request_changes projection, got %s", resolved.StatusSnapshot)
	}
	if approvals.calls != 1 || approvals.last.Decision != PlanReviewDecisionRequestChanges {
		t.Fatalf("expected approval resolver to receive request_changes untouched, got count=%d last=%#v", approvals.calls, approvals.last)
	}
	if coordinator.decisionSignals != 1 || coordinator.lastDecision.Decision != PlanReviewDecisionRequestChanges {
		t.Fatalf("expected decision signal with request_changes untouched, got count=%d signal=%#v", coordinator.decisionSignals, coordinator.lastDecision)
	}
	if coordinator.lastDecision.TargetExitDeliverable != "branch_ref" {
		t.Fatalf("expected target exit deliverable threaded, got %q", coordinator.lastDecision.TargetExitDeliverable)
	}
}

func TestResolveDecisionRejectsRequestChangesForNonPlanReview(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "project_acceptance",
		TitleSnapshot:     "项目验收确认",
		StatusSnapshot:    "pending",
	})

	_, err = service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          PlanReviewDecisionRequestChanges,
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected ErrInvalidProject for request_changes on non-plan_review decision, got %v", err)
	}
	if coordinator.decisionSignals != 0 || approvals.calls != 0 {
		t.Fatalf("expected no side effects, got signals=%d approvals=%d", coordinator.decisionSignals, approvals.calls)
	}
	stored, err := s_findDecisionForTest(repo, tenantID, projectID, decisionID)
	if err != nil {
		t.Fatalf("reload decision: %v", err)
	}
	if stored.StatusSnapshot != "pending" {
		t.Fatalf("expected decision to stay pending, got %s", stored.StatusSnapshot)
	}
}

// TestResolveDecisionAcceptsRestaffedForPlanningGap drives the real ResolveDecision
// chain for a planning_gap decision's happy path: restaffed resolves the approval
// untouched and signals the coordinator with the restaffed vocabulary and the
// correct decision id (the coordinator then reopens+replans the demand recorded in
// the approval request's ContextPayload demand_id).
func TestResolveDecisionAcceptsRestaffedForPlanningGap(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      DecisionTypePlanningGap,
		TitleSnapshot:     "规划缺口：项目员工池无法满足审查独立性约束",
		StatusSnapshot:    "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          PlanningGapDecisionRestaffed,
		Comment:           "已补充员工",
		Payload:           map[string]any{"demand_id": demandID.String()},
	})
	if err != nil {
		t.Fatalf("resolve decision with restaffed: %v", err)
	}
	if resolved.StatusSnapshot != PlanningGapDecisionRestaffed {
		t.Fatalf("expected restaffed projection, got %s", resolved.StatusSnapshot)
	}
	if approvals.calls != 1 || approvals.last.Decision != PlanningGapDecisionRestaffed {
		t.Fatalf("expected approval resolver to receive restaffed untouched, got count=%d last=%#v", approvals.calls, approvals.last)
	}
	if approvals.last.ApprovalRequestID != approvalID {
		t.Fatalf("expected approval %s resolved, got %s", approvalID, approvals.last.ApprovalRequestID)
	}
	if coordinator.decisionSignals != 1 || coordinator.lastDecision.Decision != PlanningGapDecisionRestaffed {
		t.Fatalf("expected decision signal with restaffed untouched, got count=%d signal=%#v", coordinator.decisionSignals, coordinator.lastDecision)
	}
	if coordinator.lastDecision.DecisionRequestID != decisionID {
		t.Fatalf("expected signal to carry decision %s, got %s", decisionID, coordinator.lastDecision.DecisionRequestID)
	}
}

// TestResolveDecisionRejectsRestaffedForNonPlanningGap proves restaffed is
// planning_gap vocabulary only: any other pending decision type must reject it as
// ErrInvalidProject with zero side effects (no approval call, no coordinator
// signal) and stay pending. Mirrors the request_changes narrowing test above.
func TestResolveDecisionRejectsRestaffedForNonPlanningGap(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	planRevisionID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		PlanRevisionID:    &planRevisionID,
		TargetUserID:      actorID,
		DecisionType:      "plan_review",
		TitleSnapshot:     "确认项目计划版本",
		StatusSnapshot:    "pending",
	})

	_, err = service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          PlanningGapDecisionRestaffed,
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected ErrInvalidProject for restaffed on non-planning_gap decision, got %v", err)
	}
	if coordinator.decisionSignals != 0 || approvals.calls != 0 {
		t.Fatalf("expected no side effects, got signals=%d approvals=%d", coordinator.decisionSignals, approvals.calls)
	}
	stored, err := s_findDecisionForTest(repo, tenantID, projectID, decisionID)
	if err != nil {
		t.Fatalf("reload decision: %v", err)
	}
	if stored.StatusSnapshot != "pending" {
		t.Fatalf("expected decision to stay pending, got %s", stored.StatusSnapshot)
	}
}

// TestResolveDecisionRejectsGenericVocabularyForPlanningGap proves planning_gap's
// vocabulary is closed in both directions: the generic approved (and by the same
// gate needs_more_evidence) is not meaningful on a planning gap — only restaffed
// and rejected are — so it rejects as ErrInvalidProject with zero side effects.
func TestResolveDecisionRejectsGenericVocabularyForPlanningGap(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      DecisionTypePlanningGap,
		TitleSnapshot:     "规划缺口：项目员工池无法满足审查独立性约束",
		StatusSnapshot:    "pending",
	})

	_, err = service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected ErrInvalidProject for approved on planning_gap decision, got %v", err)
	}
	if coordinator.decisionSignals != 0 || approvals.calls != 0 {
		t.Fatalf("expected no side effects, got signals=%d approvals=%d", coordinator.decisionSignals, approvals.calls)
	}
	stored, err := s_findDecisionForTest(repo, tenantID, projectID, decisionID)
	if err != nil {
		t.Fatalf("reload decision: %v", err)
	}
	if stored.StatusSnapshot != "pending" {
		t.Fatalf("expected decision to stay pending, got %s", stored.StatusSnapshot)
	}
}

// TestResolveExemptedCreatesExemptionRecord drives the real ResolveDecision chain
// for a planning_gap decision resolved "exempted": the constraint_kind/roles are
// read from the approval request's ContextPayload gap (recorded at detection time,
// not supplied by the resolving caller), a first-class exemption record is
// persisted before the approval/signal side effects, and the demand is reopened +
// replanned exactly like restaffed (same coordinator signal, same decision
// vocabulary carried through).
func TestResolveExemptedCreatesExemptionRecord(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      DecisionTypePlanningGap,
		TitleSnapshot:     "规划缺口：项目员工池无法满足审查独立性约束",
		StatusSnapshot:    "pending",
	})
	approvals.contextPayloads = map[uuid.UUID]map[string]any{
		approvalID: {
			"demand_id": demandID.String(),
			"diagnosis": "项目员工池无法满足审查独立性约束",
			"gap": map[string]any{
				"constraint_kind": "role_independence",
				"roles":           []any{"reviewer", "developer"},
			},
		},
	}

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          PlanningGapDecisionExempted,
		Comment:           "改选更浅出口，豁免独立性约束",
	})
	if err != nil {
		t.Fatalf("resolve decision with exempted: %v", err)
	}
	if resolved.StatusSnapshot != PlanningGapDecisionExempted {
		t.Fatalf("expected exempted projection, got %s", resolved.StatusSnapshot)
	}
	if len(repo.demandConstraintExemptions) != 1 {
		t.Fatalf("expected 1 exemption record, got %d", len(repo.demandConstraintExemptions))
	}
	exemption := repo.demandConstraintExemptions[0]
	if exemption.TenantID != tenantID || exemption.ProjectID != projectID || exemption.DemandID != demandID {
		t.Fatalf("unexpected exemption scope: %#v", exemption)
	}
	if exemption.ConstraintKind != "role_independence" {
		t.Fatalf("expected constraint_kind role_independence, got %s", exemption.ConstraintKind)
	}
	if len(exemption.Roles) != 2 || exemption.Roles[0] != "reviewer" || exemption.Roles[1] != "developer" {
		t.Fatalf("expected roles [reviewer developer], got %#v", exemption.Roles)
	}
	if exemption.GrantedByUserID != actorID {
		t.Fatalf("expected granted_by %s, got %s", actorID, exemption.GrantedByUserID)
	}
	if exemption.DecisionRequestID == nil || *exemption.DecisionRequestID != decisionID {
		t.Fatalf("expected decision_request_id %s, got %#v", decisionID, exemption.DecisionRequestID)
	}
	if approvals.calls != 1 || approvals.last.Decision != PlanningGapDecisionExempted {
		t.Fatalf("expected approval resolver to receive exempted, got count=%d last=%#v", approvals.calls, approvals.last)
	}
	if coordinator.decisionSignals != 1 || coordinator.lastDecision.Decision != PlanningGapDecisionExempted {
		t.Fatalf("expected decision signal with exempted, got count=%d signal=%#v", coordinator.decisionSignals, coordinator.lastDecision)
	}
	if coordinator.lastDecision.DecisionRequestID != decisionID {
		t.Fatalf("expected signal to carry decision %s, got %s", decisionID, coordinator.lastDecision.DecisionRequestID)
	}
}

// TestResolveExemptedRejectsMissingGapPayload proves exempted cannot be resolved
// on a planning_gap decision whose approval request carries no structured gap
// (e.g. a legacy or non-structural no-suitable-employee diagnosis) — there is
// nothing to exempt. It must fail ErrInvalidProject with zero side effects: no
// exemption record, no approval resolve, no coordinator signal, decision stays
// pending.
func TestResolveExemptedRejectsMissingGapPayload(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      DecisionTypePlanningGap,
		TitleSnapshot:     "规划缺口：无适合员工",
		StatusSnapshot:    "pending",
	})
	approvals.contextPayloads = map[uuid.UUID]map[string]any{
		approvalID: {
			"demand_id": demandID.String(),
			"diagnosis": "项目没有可参与规划的数字员工",
		},
	}

	_, err = service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          PlanningGapDecisionExempted,
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected ErrInvalidProject for exempted with missing gap payload, got %v", err)
	}
	if len(repo.demandConstraintExemptions) != 0 {
		t.Fatalf("expected no exemption record, got %d", len(repo.demandConstraintExemptions))
	}
	if coordinator.decisionSignals != 0 || approvals.calls != 0 {
		t.Fatalf("expected no side effects, got signals=%d approvals=%d", coordinator.decisionSignals, approvals.calls)
	}
	stored, err := s_findDecisionForTest(repo, tenantID, projectID, decisionID)
	if err != nil {
		t.Fatalf("reload decision: %v", err)
	}
	if stored.StatusSnapshot != "pending" {
		t.Fatalf("expected decision to stay pending, got %s", stored.StatusSnapshot)
	}
}

func s_findDecisionForTest(repo *memoryRepository, tenantID, projectID, decisionID uuid.UUID) (DecisionRequest, error) {
	decisions, err := repo.ListDecisionRequests(context.Background(), tenantID, projectID, 100, 0)
	if err != nil {
		return DecisionRequest{}, err
	}
	for _, d := range decisions {
		if d.ID == decisionID {
			return d, nil
		}
	}
	return DecisionRequest{}, errors.New("decision not found")
}

func TestResolveDecisionRejectsUnknownTargetExitDeliverable(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	planRevisionID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.planRevisions = append(repo.planRevisions, PlanRevision{
		ID:        planRevisionID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Status:    PlanRevisionStatusPendingReview,
		Payload: map[string]any{
			"available_exits": []any{
				map[string]any{"deliverable": "review_verdict", "label": "审查通过"},
				map[string]any{"deliverable": "branch_ref", "label": "分支就绪"},
			},
		},
	})
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		PlanRevisionID:    &planRevisionID,
		TargetUserID:      actorID,
		DecisionType:      "plan_review",
		TitleSnapshot:     "确认项目计划版本",
		StatusSnapshot:    "pending",
	})

	_, err = service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:              tenantID,
		ProjectID:             projectID,
		DecisionRequestID:     decisionID,
		DecidedByUserID:       actorID,
		Decision:              PlanReviewDecisionRequestChanges,
		Comment:               "改选出口",
		TargetExitDeliverable: "not_a_real_exit",
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected ErrInvalidProject for target_exit_deliverable not in available_exits, got %v", err)
	}
	if coordinator.decisionSignals != 0 || approvals.calls != 0 {
		t.Fatalf("expected no side effects for bogus target_exit_deliverable, got signals=%d approvals=%d", coordinator.decisionSignals, approvals.calls)
	}
	stored, err := s_findDecisionForTest(repo, tenantID, projectID, decisionID)
	if err != nil {
		t.Fatalf("reload decision: %v", err)
	}
	if stored.StatusSnapshot != "pending" {
		t.Fatalf("expected decision to stay pending, got %s", stored.StatusSnapshot)
	}
}

func TestResolveDecisionRejectsTargetExitDeliverableForUnboundPlanRevision(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	// No PlanRevisionID linkage on the decision — legacy/unbound plan review.
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "plan_review",
		TitleSnapshot:     "确认项目计划版本",
		StatusSnapshot:    "pending",
	})

	_, err = service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:              tenantID,
		ProjectID:             projectID,
		DecisionRequestID:     decisionID,
		DecidedByUserID:       actorID,
		Decision:              PlanReviewDecisionRequestChanges,
		Comment:               "改选出口",
		TargetExitDeliverable: "branch_ref",
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected ErrInvalidProject for target_exit_deliverable on unbound plan revision, got %v", err)
	}
	if coordinator.decisionSignals != 0 || approvals.calls != 0 {
		t.Fatalf("expected no side effects for unbound plan revision, got signals=%d approvals=%d", coordinator.decisionSignals, approvals.calls)
	}
}

func TestResolveDecisionRejectsUnknownDecisionValue(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:             decisionID,
		TenantID:       tenantID,
		ProjectID:      projectID,
		TargetUserID:   actorID,
		DecisionType:   "plan_review",
		TitleSnapshot:  "确认项目计划版本",
		StatusSnapshot: "pending",
	})

	_, err = service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "definitely_not_a_decision",
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected ErrInvalidProject for unknown decision, got %v", err)
	}
	if coordinator.decisionSignals != 0 || approvals.calls != 0 {
		t.Fatalf("expected no side effects for invalid decision, got signals=%d approvals=%d", coordinator.decisionSignals, approvals.calls)
	}
}

func TestMemoryRepositoryDecisionRequestCarriesPlanRevisionID(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	planRevisionID := uuid.New()
	repo := &memoryRepository{}

	decision, err := repo.CreateDecisionRequest(context.Background(), CreateDecisionRequestRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: uuid.New(),
		PlanRevisionID:    &planRevisionID,
		TargetUserID:      uuid.New(),
		DecisionType:      "plan_review",
		TitleSnapshot:     "确认项目计划版本",
		StatusSnapshot:    "pending",
	})

	require.NoError(t, err)
	require.NotNil(t, decision.PlanRevisionID)
	require.Equal(t, planRevisionID, *decision.PlanRevisionID)
}

func TestResolveDecisionSkipsApprovalResolverForProjectOnlyDecision(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:             decisionID,
		TenantID:       tenantID,
		ProjectID:      projectID,
		TargetUserID:   actorID,
		DecisionType:   "project_task_acceptance",
		TitleSnapshot:  "任务验收",
		StatusSnapshot: "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	})

	require.NoError(t, err)
	require.Equal(t, "approved", resolved.StatusSnapshot)
	require.Equal(t, 0, approvals.calls)
	require.Equal(t, 1, coordinator.decisionSignals)
	require.Equal(t, uuid.Nil, coordinator.lastDecision.ApprovalRequestID)
}

func TestResolveDecisionApprovedProjectTaskAcceptanceCompletesWaitingTask(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusWaitingHuman, ProjectTaskAttemptStatusSucceeded)
	actorID := repo.projects[fixture.projectID].HumanOwnerUserID
	decisionID := uuid.New()
	reason := HumanWaitReasonAcceptanceRequired
	repo.tasks[0].WaitingReason = &reason
	repo.tasks[0].WaitingRequestID = &decisionID
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:             decisionID,
		TenantID:       fixture.tenantID,
		ProjectID:      fixture.projectID,
		ProjectTaskID:  &fixture.taskID,
		TargetUserID:   actorID,
		DecisionType:   "project_task_acceptance",
		TitleSnapshot:  "任务验收",
		StatusSnapshot: "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          fixture.tenantID,
		ProjectID:         fixture.projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
		Comment:           "验收通过",
	})

	require.NoError(t, err)
	require.Equal(t, "approved", resolved.StatusSnapshot)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusCompleted, task.Status)
	require.Equal(t, 0, approvals.calls)
	require.Equal(t, 1, coordinator.decisionSignals)
}

func TestResolveDecisionIsIdempotentForSameResolvedDecision(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	resolvedEventID := uuid.New()
	resolvedAt := time.Now().UTC()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusArchived,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "project_acceptance",
		TitleSnapshot:     "验收项目交付",
		StatusSnapshot:    "approved",
		ResolvedEventID:   &resolvedEventID,
		ResolvedAt:        &resolvedAt,
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	})

	if err != nil {
		t.Fatalf("resolve decision replay: %v", err)
	}
	if resolved.ID != decisionID || resolved.StatusSnapshot != "approved" || resolved.ResolvedEventID == nil || *resolved.ResolvedEventID != resolvedEventID {
		t.Fatalf("expected existing resolved decision, got %#v", resolved)
	}
	if approvals.calls != 0 || coordinator.decisionSignals != 0 || len(repo.events) != 0 {
		t.Fatalf("expected idempotent replay without side effects, approvals=%d signals=%d events=%d", approvals.calls, coordinator.decisionSignals, len(repo.events))
	}
}

func TestRetryWorkflowSignalReplaysHumanDecisionPayload(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	approvalID := uuid.New()
	decisionID := uuid.New()
	resolvedEventID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	failedEvent, err := repo.AppendProjectEvent(context.Background(), AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventWorkflowSignaled,
		ActorType: "control_plane",
		ActorID:   "project_service",
		Summary:   "Workflow signal 状态已记录",
		Payload: map[string]any{
			"signal_name":         "HumanDecisionSubmitted",
			"status":              "failed",
			"retryable":           true,
			"approval_request_id": approvalID.String(),
			"decision_request_id": decisionID.String(),
			"resolved_event_id":   resolvedEventID.String(),
			"decision":            "approved",
			"payload":             map[string]any{"recovery_action": "retry"},
		},
	})
	if err != nil {
		t.Fatalf("seed failed workflow signal: %v", err)
	}

	event, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedEvent.ID,
		ActorID:   ownerID,
	})

	if err != nil {
		t.Fatalf("retry human decision workflow signal: %v", err)
	}
	if coordinator.decisionSignals != 1 || coordinator.lastDecision.DecisionRequestID != decisionID || coordinator.lastDecision.ResolvedEventID != resolvedEventID {
		t.Fatalf("expected human decision signal replay, count=%d signal=%#v", coordinator.decisionSignals, coordinator.lastDecision)
	}
	if coordinator.lastDecision.Payload["recovery_action"] != "retry" {
		t.Fatalf("expected human decision payload replay, got %#v", coordinator.lastDecision.Payload)
	}
	if event.EventType != ProjectEventWorkflowSignaled || event.Payload["status"] != "sent" || event.Payload["retry_of_event_id"] != failedEvent.ID.String() {
		t.Fatalf("unexpected retry event: %#v", event)
	}
}

func TestResolveDecisionProjectsInboxResolution(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, coordinator, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "需要负责人确认",
		StatusSnapshot:    "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	})
	if err != nil {
		t.Fatalf("resolve decision: %v", err)
	}
	if len(inbox.resolutions) != 1 || inbox.resolutions[0].ID != decisionID || inbox.resolutions[0].StatusSnapshot != "approved" || inbox.resolutions[0].ResolvedEventID == nil {
		t.Fatalf("expected inbox resolution projection, got %#v", inbox.resolutions)
	}
	if resolved.ID != decisionID || coordinator.decisionSignals != 1 {
		t.Fatalf("expected resolved decision and coordinator signal, resolved=%#v signals=%d", resolved, coordinator.decisionSignals)
	}

	projectionErr := errors.New("inbox unavailable")
	failingRepo := newMemoryRepository()
	failingCoordinator := &fakeCoordinatorSignalClient{}
	failingApprovals := &fakeApprovalResolver{}
	failingInbox := &fakeDecisionInboxProjector{resolveErr: projectionErr}
	failingService, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(failingRepo, failingCoordinator, failingApprovals, failingInbox, nil)
	if err != nil {
		t.Fatalf("new failing service: %v", err)
	}
	failingProjectID := uuid.New()
	failingDecisionID := uuid.New()
	failingRepo.projects[failingProjectID] = Project{
		ID:                     failingProjectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + failingProjectID.String(),
	}
	failingRepo.decisionRequests = append(failingRepo.decisionRequests, DecisionRequest{
		ID:                failingDecisionID,
		TenantID:          tenantID,
		ProjectID:         failingProjectID,
		ApprovalRequestID: uuid.New(),
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "需要负责人确认",
		StatusSnapshot:    "pending",
	})
	if _, err := failingService.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         failingProjectID,
		DecisionRequestID: failingDecisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	}); !errors.Is(err, projectionErr) {
		t.Fatalf("expected projector error, got %v", err)
	}
	if failingCoordinator.decisionSignals != 0 {
		t.Fatalf("expected no coordinator signal after projection failure, got %d", failingCoordinator.decisionSignals)
	}
}

func TestResolveDecisionFindsDecisionBeyondFirstPage(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	targetDecisionID := uuid.New()
	targetApprovalID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "demo-project",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	for i := 0; i < 100; i++ {
		repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
			ID:                uuid.New(),
			TenantID:          tenantID,
			ProjectID:         projectID,
			ApprovalRequestID: uuid.New(),
			TargetUserID:      actorID,
			DecisionType:      "route_review",
			TitleSnapshot:     "较新的决策",
			StatusSnapshot:    "pending",
			CreatedAt:         time.Now().UTC().Add(time.Duration(i+1) * time.Minute),
		})
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                targetDecisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: targetApprovalID,
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "较早的决策",
		StatusSnapshot:    "pending",
		CreatedAt:         time.Now().UTC().Add(-time.Hour),
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: targetDecisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	})
	if err != nil {
		t.Fatalf("resolve older decision: %v", err)
	}
	if resolved.ID != targetDecisionID || approvals.last.ApprovalRequestID != targetApprovalID {
		t.Fatalf("expected target decision to resolve, decision=%#v approval=%#v", resolved, approvals.last)
	}
}

// demandAcceptanceSignFixture wires a demand parked at acceptance_pending
// with a snapshotted plan revision, a pending demand_acceptance decision
// (TargetUserID == ownerID == the project's human_owner) and the given
// criterion snapshot — everything Service.SignDemandCriterionVerdict's
// preconditions expect. Returned IDs let each test seed additional verdicts
// or diverge a single field (actor, demand status, criterion method) to
// exercise one precondition at a time.
type demandAcceptanceSignFixture struct {
	tenantID   uuid.UUID
	projectID  uuid.UUID
	demandID   uuid.UUID
	revisionID uuid.UUID
	ownerID    uuid.UUID
	decisionID uuid.UUID
	approvalID uuid.UUID
}

func setupDemandAcceptanceSignFixture(repo *memoryRepository, criteria []DemandAcceptanceCriterion) demandAcceptanceSignFixture {
	f := demandAcceptanceSignFixture{
		tenantID:   uuid.New(),
		projectID:  uuid.New(),
		demandID:   uuid.New(),
		revisionID: uuid.New(),
		ownerID:    uuid.New(),
		decisionID: uuid.New(),
		approvalID: uuid.New(),
	}
	repo.projects[f.projectID] = Project{
		ID:               f.projectID,
		TenantID:         f.tenantID,
		Name:             "验收项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: f.ownerID,
	}
	repo.demands = append(repo.demands, ProjectDemand{
		ID:                f.demandID,
		TenantID:          f.tenantID,
		ProjectID:         f.projectID,
		SubmittedByUserID: f.ownerID,
		Title:             "需要验收的需求",
		SourceType:        DemandSourceManual,
		Status:            ProjectDemandStatusAcceptancePending,
	})
	repo.planRevisions = append(repo.planRevisions, PlanRevision{
		ID:             f.revisionID,
		TenantID:       f.tenantID,
		ProjectID:      f.projectID,
		DemandID:       f.demandID,
		RevisionNumber: 1,
		Status:         PlanRevisionStatusAccepted,
	})
	revisionID := f.revisionID
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                f.decisionID,
		TenantID:          f.tenantID,
		ProjectID:         f.projectID,
		ApprovalRequestID: f.approvalID,
		PlanRevisionID:    &revisionID,
		TargetUserID:      f.ownerID,
		DecisionType:      DecisionTypeDemandAcceptance,
		TitleSnapshot:     "需求验收：需要验收的需求",
		StatusSnapshot:    "pending",
	})
	for i := range criteria {
		criteria[i].TenantID = f.tenantID
		criteria[i].ProjectID = f.projectID
		criteria[i].DemandID = f.demandID
		criteria[i].PlanRevisionID = f.revisionID
	}
	repo.demandAcceptanceCriteria = append(repo.demandAcceptanceCriteria, criteria...)
	return f
}

// TestCloseDemandCancelsPlanningZombie pins spec §5.5 close_demand: an eligible
// human can cancel a demand stuck in planning (the F6 zombie), it becomes
// cancelled with a demand.cancelled audit event, and ineligible actors are
// rejected.
func TestCloseDemandCancelsPlanningZombie(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "僵尸项目",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.demands = append(repo.demands, ProjectDemand{
		ID:        demandID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "永远规划中的僵尸需求",
		Status:    ProjectDemandStatusPlanningPending,
	})

	// Ineligible actor is rejected before any state change.
	if _, err := service.CloseDemand(context.Background(), CloseDemandRequest{
		TenantID: tenantID, DemandID: demandID, ActorUserID: uuid.New(), Reason: "非成员",
	}); !errors.Is(err, ErrProjectDecisionForbidden) {
		t.Fatalf("expected ErrProjectDecisionForbidden for non-member, got %v", err)
	}

	updated, err := service.CloseDemand(context.Background(), CloseDemandRequest{
		TenantID: tenantID, DemandID: demandID, ActorUserID: ownerID, Reason: "规划挂死，关闭",
	})
	if err != nil {
		t.Fatalf("close demand: %v", err)
	}
	if updated.Status != ProjectDemandStatusCancelled {
		t.Fatalf("expected demand cancelled, got %s", updated.Status)
	}
	ev := lastProjectEventOfType(t, repo.events, ProjectEventDemandCancelled)
	if ev.Payload["demand_id"] != demandID.String() || ev.Payload["previous_status"] != string(ProjectDemandStatusPlanningPending) {
		t.Fatalf("unexpected cancel event payload: %#v", ev.Payload)
	}

	// Idempotent second close.
	again, err := service.CloseDemand(context.Background(), CloseDemandRequest{
		TenantID: tenantID, DemandID: demandID, ActorUserID: ownerID,
	})
	if err != nil || again.Status != ProjectDemandStatusCancelled {
		t.Fatalf("expected idempotent cancel, got status=%v err=%v", again.Status, err)
	}
}

// TestProjectTaskRequiresAcceptanceGatesOnDownstreamDependency pins spec §5.2 /
// F4: a completed task opens downstream_release ONLY with a human-review signal
// AND a live (non-terminal) downstream dependency. Leaf tasks never gate; a
// terminal downstream does not gate; and RequiresHumanApproval alone never
// post-gates (it already gated pre-dispatch).
func TestProjectTaskRequiresAcceptanceGatesOnDownstreamDependency(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	high := "high"
	blocker := ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, RiskLevel: &high, Status: ProjectTaskStatusRunning}
	repo.tasks = append(repo.tasks, blocker)
	req := CompleteProjectTaskAttemptRequest{}

	// leaf high-risk task, no downstream → no gate
	got, err := service.projectTaskRequiresAcceptance(context.Background(), blocker, req)
	require.NoError(t, err)
	require.False(t, got, "leaf high-risk task must not open downstream_release (§5.2)")

	// non-terminal downstream → gate
	downstream := ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Status: ProjectTaskStatusQueued}
	repo.tasks = append(repo.tasks, downstream)
	repo.taskDependents = map[uuid.UUID][]uuid.UUID{blocker.ID: {downstream.ID}}
	got, err = service.projectTaskRequiresAcceptance(context.Background(), blocker, req)
	require.NoError(t, err)
	require.True(t, got, "high-risk task with a live downstream must open downstream_release")

	// terminal downstream → no gate
	repo.tasks[1].Status = ProjectTaskStatusCompleted
	got, err = service.projectTaskRequiresAcceptance(context.Background(), blocker, req)
	require.NoError(t, err)
	require.False(t, got, "a fully terminal downstream must not gate")

	// F4: RequiresHumanApproval alone (no review signal) must not post-gate even
	// with a live downstream — it already fired the pre-dispatch gate.
	repo.tasks[1].Status = ProjectTaskStatusQueued
	approvalOnly := ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, RequiresHumanApproval: true, Status: ProjectTaskStatusRunning}
	repo.tasks = append(repo.tasks, approvalOnly)
	repo.taskDependents[approvalOnly.ID] = []uuid.UUID{downstream.ID}
	got, err = service.projectTaskRequiresAcceptance(context.Background(), approvalOnly, req)
	require.NoError(t, err)
	require.False(t, got, "RequiresHumanApproval alone must not post-gate (F4 de-double-gate)")
}

func blockingHumanJudgmentCriterion(criterionID, statement string) DemandAcceptanceCriterion {
	return DemandAcceptanceCriterion{
		CriterionID:        criterionID,
		Statement:          statement,
		VerificationMethod: demandCriterionVerificationMethodHumanJudgment,
		Severity:           demandAcceptanceCriterionSeverityBlocking,
	}
}

func TestListDemandAcceptanceCriteriaDetailResolvesEffectiveVerdictsAndSummaries(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, NoopCoordinatorSignalClient{}, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	taskID := uuid.New()
	c1 := blockingHumanJudgmentCriterion("c1", "第一条判据")
	c1.SatisfiedBy = []string{taskID.String()}
	c2 := blockingHumanJudgmentCriterion("c2", "第二条判据")
	c3 := DemandAcceptanceCriterion{
		CriterionID:        "c3",
		Statement:          "自动判据",
		VerificationMethod: "automated_test",
		Severity:           demandAcceptanceCriterionSeverityBlocking,
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{c1, c2, c3})

	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:            uuid.New(),
		TenantID:      f.tenantID,
		ProjectID:     f.projectID,
		ProjectTaskID: taskID,
		Conclusion:    "已交付并通过测试",
		CreatedAt:     time.Now().UTC(),
	})

	execTaskC1 := uuid.New()
	execTaskC3 := uuid.New()
	repo.demandCriterionVerdicts = append(repo.demandCriterionVerdicts,
		// c1 executor satisfied — must be overridden by the later human verdict.
		DemandCriterionVerdict{
			ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, DemandID: f.demandID, PlanRevisionID: f.revisionID,
			CriterionID: "c1", Verdict: "satisfied", JudgeType: "executor", ProjectTaskID: &execTaskC1,
			EvidenceRefs: []string{"attestation:executor-c1"},
		},
		// c1 human unsatisfied — precedence over executor.
		DemandCriterionVerdict{
			ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, DemandID: f.demandID, PlanRevisionID: f.revisionID,
			CriterionID: "c1", Verdict: "unsatisfied", JudgeType: "human", JudgeID: f.ownerID, Reason: "未达标",
		},
		// c3 executor satisfied — no human verdict, executor stands.
		DemandCriterionVerdict{
			ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, DemandID: f.demandID, PlanRevisionID: f.revisionID,
			CriterionID: "c3", Verdict: "satisfied", JudgeType: "executor", ProjectTaskID: &execTaskC3,
			EvidenceRefs: []string{"attestation:executor-c3"},
		},
	)

	detail, err := service.ListDemandAcceptanceCriteriaDetail(context.Background(), f.tenantID, f.demandID)
	if err != nil {
		t.Fatalf("list acceptance criteria: %v", err)
	}
	if detail.DemandStatus != ProjectDemandStatusAcceptancePending {
		t.Fatalf("expected demand_status acceptance_pending, got %s", detail.DemandStatus)
	}
	if len(detail.Criteria) != 3 {
		t.Fatalf("expected 3 criteria in snapshot order, got %d", len(detail.Criteria))
	}

	// c1: human unsatisfied overrides executor satisfied; task_summaries resolved.
	got1 := detail.Criteria[0]
	if got1.CriterionID != "c1" {
		t.Fatalf("expected first criterion c1, got %s", got1.CriterionID)
	}
	if got1.Verdict == nil || *got1.Verdict != "unsatisfied" {
		t.Fatalf("expected c1 effective verdict unsatisfied, got %v", got1.Verdict)
	}
	if got1.JudgeType == nil || *got1.JudgeType != "human" {
		t.Fatalf("expected c1 judge_type human, got %v", got1.JudgeType)
	}
	if len(got1.TaskSummaries) != 1 || got1.TaskSummaries[0].TaskID != taskID.String() || got1.TaskSummaries[0].Summary != "已交付并通过测试" {
		t.Fatalf("expected c1 task_summaries to resolve latest conclusion, got %#v", got1.TaskSummaries)
	}

	// c2: no verdict at all → nil verdict/judge, empty task summaries.
	got2 := detail.Criteria[1]
	if got2.Verdict != nil || got2.JudgeType != nil {
		t.Fatalf("expected c2 unresolved verdict/judge, got verdict=%v judge=%v", got2.Verdict, got2.JudgeType)
	}
	if len(got2.TaskSummaries) != 0 {
		t.Fatalf("expected c2 no task summaries, got %#v", got2.TaskSummaries)
	}

	// c3: executor satisfied stands; evidence surfaced.
	got3 := detail.Criteria[2]
	if got3.Verdict == nil || *got3.Verdict != "satisfied" {
		t.Fatalf("expected c3 effective verdict satisfied, got %v", got3.Verdict)
	}
	if got3.JudgeType == nil || *got3.JudgeType != "executor" {
		t.Fatalf("expected c3 judge_type executor, got %v", got3.JudgeType)
	}
	if len(got3.EvidenceRefs) != 1 || got3.EvidenceRefs[0] != "attestation:executor-c3" {
		t.Fatalf("expected c3 evidence_refs from executor verdict, got %#v", got3.EvidenceRefs)
	}
}

// TestListDemandAcceptanceCriteriaDetailResolvesTaskSummariesByPlannedKey proves
// the real-chain identity of satisfied_by: Task 4 decomposition stores each
// satisfying task's planned_task_key (e.g. "develop"), NOT its UUID. The panel
// must map planned_task_key → task UUID for the demand's tasks, then surface
// that task's real conclusion (anti-rubber-stamp evidence) — otherwise the
// "查看满足任务产出" panel is永远空.
func TestListDemandAcceptanceCriteriaDetailResolvesTaskSummariesByPlannedKey(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, NoopCoordinatorSignalClient{}, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	plannedKey := "develop"
	c1 := DemandAcceptanceCriterion{
		CriterionID:        "c1",
		Statement:          "变更以 branch+commit 交付",
		VerificationMethod: "automated_test",
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{plannedKey}, // planned_task_key, not a UUID
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{c1})

	// The demand's real task carries the planned key and a real UUID.
	taskID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:             taskID,
		TenantID:       f.tenantID,
		ProjectID:      f.projectID,
		DemandID:       &f.demandID,
		PlannedTaskKey: &plannedKey,
		Title:          "开发任务",
		Status:         ProjectTaskStatusCompleted,
		UpdatedAt:      time.Now().UTC(),
	})
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:            uuid.New(),
		TenantID:      f.tenantID,
		ProjectID:     f.projectID,
		ProjectTaskID: taskID,
		Conclusion:    "已按 branch+commit 交付并通过验证",
		CreatedAt:     time.Now().UTC(),
	})

	detail, err := service.ListDemandAcceptanceCriteriaDetail(context.Background(), f.tenantID, f.demandID)
	if err != nil {
		t.Fatalf("list acceptance criteria: %v", err)
	}
	require.Len(t, detail.Criteria, 1)
	got := detail.Criteria[0]
	require.Len(t, got.TaskSummaries, 1)
	require.Equal(t, taskID.String(), got.TaskSummaries[0].TaskID)
	require.Equal(t, "已按 branch+commit 交付并通过验证", got.TaskSummaries[0].Summary)
}

// TestListDemandAcceptanceCriteriaDetailSurfacesDeclaredDeliverables proves the
// v2 §4 P2 deep-link: a satisfied task's retrievable declared deliverables ride
// under its TaskSummary so the human can preview them beside the sign button;
// non-declared, non-retrievable, and other tasks' artifacts are excluded.
func TestListDemandAcceptanceCriteriaDetailSurfacesDeclaredDeliverables(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, NoopCoordinatorSignalClient{}, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	plannedKey := "develop"
	c1 := DemandAcceptanceCriterion{
		CriterionID:        "c1",
		Statement:          "报告已交付",
		VerificationMethod: "human_judgment",
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{plannedKey},
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{c1})

	taskID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:             taskID,
		TenantID:       f.tenantID,
		ProjectID:      f.projectID,
		DemandID:       &f.demandID,
		PlannedTaskKey: &plannedKey,
		Title:          "开发任务",
		Status:         ProjectTaskStatusCompleted,
		UpdatedAt:      time.Now().UTC(),
	})
	htmlType := "text/html"
	var size int64 = 462
	declaredID := uuid.New()
	repo.artifactRefs = append(repo.artifactRefs,
		ProjectArtifactRef{
			ID: declaredID, TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: &taskID,
			ArtifactType: "declared", Title: "report.html", ObjectRef: "artifacts/t/sha256/aa",
			ContentType: &htmlType, SizeBytes: &size,
		},
		// 兜底附件不是 declared,不应出现。
		ProjectArtifactRef{
			ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: &taskID,
			ArtifactType: "execution_output", Title: "notes.md", ObjectRef: "artifacts/t/sha256/bb",
		},
		// declared 但非内容寻址(不可取回),排除。
		ProjectArtifactRef{
			ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: &taskID,
			ArtifactType: "declared", Title: "external", ObjectRef: "https://x/y",
		},
		// 别的任务的 declared,不串到本判据。
		ProjectArtifactRef{
			ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: ptrUUIDValue(uuid.New()),
			ArtifactType: "declared", Title: "other.html", ObjectRef: "artifacts/t/sha256/cc",
		},
	)

	detail, err := service.ListDemandAcceptanceCriteriaDetail(context.Background(), f.tenantID, f.demandID)
	if err != nil {
		t.Fatalf("list acceptance criteria: %v", err)
	}
	require.Len(t, detail.Criteria, 1)
	require.Len(t, detail.Criteria[0].TaskSummaries, 1)
	deliverables := detail.Criteria[0].TaskSummaries[0].Deliverables
	require.Len(t, deliverables, 1, "only the retrievable declared deliverable of this task")
	require.Equal(t, declaredID.String(), deliverables[0].ArtifactRefID)
	require.Equal(t, "report.html", deliverables[0].Title)
	require.Equal(t, "text/html", deliverables[0].ContentType)
	require.NotNil(t, deliverables[0].SizeBytes)
	require.Equal(t, int64(462), *deliverables[0].SizeBytes)
}

func TestListDemandAcceptanceCriteriaDetailReturnsEmptyWhenNoOpenRevision(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, NoopCoordinatorSignalClient{}, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: uuid.New()}
	repo.demands = append(repo.demands, ProjectDemand{
		ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: ProjectDemandStatusExecuting,
	})

	detail, err := service.ListDemandAcceptanceCriteriaDetail(context.Background(), tenantID, demandID)
	if err != nil {
		t.Fatalf("list acceptance criteria: %v", err)
	}
	if detail.DemandStatus != ProjectDemandStatusExecuting {
		t.Fatalf("expected demand_status executing, got %s", detail.DemandStatus)
	}
	if len(detail.Criteria) != 0 {
		t.Fatalf("expected no criteria for legacy demand without open revision, got %d", len(detail.Criteria))
	}
}

func TestSignDemandCriterionVerdictReturnsProgressWhenCriteriaRemain(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, NoopCoordinatorSignalClient{}, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "第一条判据"),
		blockingHumanJudgmentCriterion("c2", "第二条判据"),
	})

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "satisfied",
		Reason:      "已核实",
	})
	if err != nil {
		t.Fatalf("sign criterion: %v", err)
	}
	if result.DemandStatus != ProjectDemandStatusAcceptancePending {
		t.Fatalf("expected demand to stay acceptance_pending, got %s", result.DemandStatus)
	}
	if result.Signed != 1 || result.Total != 2 || result.Remaining != 1 {
		t.Fatalf("expected progress 1/2 remaining 1, got signed=%d total=%d remaining=%d", result.Signed, result.Total, result.Remaining)
	}
	if len(repo.demandCriterionVerdicts) != 1 {
		t.Fatalf("expected one verdict row, got %d", len(repo.demandCriterionVerdicts))
	}
	demand, _ := repo.GetProjectDemand(context.Background(), f.tenantID, f.demandID)
	if demand.Status != ProjectDemandStatusAcceptancePending {
		t.Fatalf("expected repository demand to stay acceptance_pending, got %s", demand.Status)
	}
	decision, _ := repo.GetDecisionRequest(context.Background(), f.tenantID, f.projectID, f.decisionID)
	if decision.StatusSnapshot != "pending" {
		t.Fatalf("expected decision to stay pending, got %s", decision.StatusSnapshot)
	}
	if approvals.calls != 0 {
		t.Fatalf("expected no approval resolution while criteria remain, got %d calls", approvals.calls)
	}
}

func TestSignDemandCriterionVerdictCompletesDemandWhenAllBlockingSatisfied(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "唯一一条判据"),
	})

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "satisfied",
	})
	if err != nil {
		t.Fatalf("sign criterion: %v", err)
	}
	if result.DemandStatus != ProjectDemandStatusCompleted {
		t.Fatalf("expected demand completed, got %s", result.DemandStatus)
	}
	if result.Signed != 1 || result.Total != 1 || result.Remaining != 0 {
		t.Fatalf("expected progress 1/1 remaining 0, got signed=%d total=%d remaining=%d", result.Signed, result.Total, result.Remaining)
	}
	demand, _ := repo.GetProjectDemand(context.Background(), f.tenantID, f.demandID)
	if demand.Status != ProjectDemandStatusCompleted {
		t.Fatalf("expected repository demand completed, got %s", demand.Status)
	}
	if approvals.calls != 1 || approvals.last.ApprovalRequestID != f.approvalID || approvals.last.Decision != "approved" {
		t.Fatalf("expected approval resolved approved, got calls=%d last=%#v", approvals.calls, approvals.last)
	}
	decision, _ := repo.GetDecisionRequest(context.Background(), f.tenantID, f.projectID, f.decisionID)
	if decision.StatusSnapshot != "approved" {
		t.Fatalf("expected decision resolved approved, got %s", decision.StatusSnapshot)
	}
	if len(inbox.resolutions) != 1 {
		t.Fatalf("expected inbox resolution, got %d", len(inbox.resolutions))
	}
	completedEvent := lastProjectEventOfType(t, repo.events, ProjectEventDemandAcceptanceCompleted)
	if completedEvent.Payload["demand_id"] != f.demandID.String() {
		t.Fatalf("unexpected completed event payload: %#v", completedEvent.Payload)
	}
	// Fix B: this was the project's only demand, now terminal → project acceptance
	// review opened rather than the project being left stuck running.
	assertProjectAcceptanceReviewOpened(t, repo, approvals, f.projectID)
}

// TestSignDemandCriterionVerdictAlsoCloseProjectArchivesWithoutClosureCard pins
// §5.3「通过并结项」: when also_close_project=true and the signed demand is the
// last non-terminal demand, the service archives + writes an acceptance record
// and does NOT open a project_acceptance / closure_confirm card.
func TestSignDemandCriterionVerdictAlsoCloseProjectArchivesWithoutClosureCard(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "唯一一条判据"),
	})

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:         f.tenantID,
		DemandID:         f.demandID,
		ActorUserID:      f.ownerID,
		CriterionID:      "c1",
		Verdict:          "satisfied",
		AlsoCloseProject: true,
	})
	if err != nil {
		t.Fatalf("sign with also_close_project: %v", err)
	}
	if result.DemandStatus != ProjectDemandStatusCompleted {
		t.Fatalf("expected demand completed, got %s", result.DemandStatus)
	}
	projectRecord, _ := repo.GetProject(context.Background(), f.tenantID, f.projectID)
	if projectRecord.Status != ProjectStatusArchived {
		t.Fatalf("expected project archived, got %s", projectRecord.Status)
	}
	if approvals.createCalls != 0 {
		t.Fatalf("expected no closure_confirm approval created, got createCalls=%d", approvals.createCalls)
	}
	for _, d := range repo.decisionRequests {
		if d.ProjectID == f.projectID && d.DecisionType == "project_acceptance" && d.StatusSnapshot == "pending" {
			t.Fatalf("expected no pending project_acceptance decision, found %#v", d)
		}
	}
	foundAcceptanceEvent := false
	for _, ev := range repo.events {
		if ev.EventType == ProjectEventAcceptanceSubmitted {
			foundAcceptanceEvent = true
			if payload, ok := ev.Payload["also_close_project"].(bool); !ok || !payload {
				t.Fatalf("expected also_close_project=true on acceptance event, got %#v", ev.Payload)
			}
			break
		}
	}
	if !foundAcceptanceEvent {
		t.Fatalf("expected acceptance.submitted event after also_close_project")
	}
}

func TestSignDemandCriterionVerdictAlsoCloseProjectNoopsWhenOtherDemandsOpen(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "唯一一条判据"),
	})
	// A second still-open demand means the project is not ready to close.
	otherDemandID := uuid.New()
	repo.demands = append(repo.demands, ProjectDemand{
		ID: otherDemandID, TenantID: f.tenantID, ProjectID: f.projectID,
		Title: "另一条开放需求", Status: ProjectDemandStatusExecuting, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:         f.tenantID,
		DemandID:         f.demandID,
		ActorUserID:      f.ownerID,
		CriterionID:      "c1",
		Verdict:          "satisfied",
		AlsoCloseProject: true,
	})
	if err != nil {
		t.Fatalf("sign with also_close_project while other demand open: %v", err)
	}
	if result.DemandStatus != ProjectDemandStatusCompleted {
		t.Fatalf("expected demand completed, got %s", result.DemandStatus)
	}
	projectRecord, _ := repo.GetProject(context.Background(), f.tenantID, f.projectID)
	if projectRecord.Status == ProjectStatusArchived {
		t.Fatalf("project must not archive while another demand is open, got %s", projectRecord.Status)
	}
	if approvals.createCalls != 0 {
		t.Fatalf("expected no premature closure card, got createCalls=%d", approvals.createCalls)
	}
}

// TestResolveDecisionDemandAcceptanceApprovedSignsAllHumanCriteria pins the F1
// fix: resolving a demand_acceptance decision "approved" (the inbox one-tap card
// path) must sign every pending human-signable criterion and converge the
// demand, NOT resolve into the dead-end pre-dispatch gate. Automated criteria are
// left to their executor verdicts.
func TestResolveDecisionDemandAcceptanceApprovedSignsAllHumanCriteria(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "第一条人类判据"),
		blockingHumanJudgmentCriterion("c2", "第二条人类判据"),
		{
			CriterionID:        "auto",
			Statement:          "自动判据",
			VerificationMethod: "automated_test",
			Severity:           demandAcceptanceCriterionSeverityBlocking,
		},
	})
	execTask := uuid.New()
	repo.demandCriterionVerdicts = append(repo.demandCriterionVerdicts, DemandCriterionVerdict{
		ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, DemandID: f.demandID, PlanRevisionID: f.revisionID,
		CriterionID: "auto", Verdict: "satisfied", JudgeType: "executor", ProjectTaskID: &execTask,
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          f.tenantID,
		ProjectID:         f.projectID,
		DecisionRequestID: f.decisionID,
		DecidedByUserID:   f.ownerID,
		Decision:          "approved",
	})
	if err != nil {
		t.Fatalf("resolve demand_acceptance approved: %v", err)
	}
	if resolved.StatusSnapshot != "approved" {
		t.Fatalf("expected decision approved, got %s", resolved.StatusSnapshot)
	}
	demand, _ := repo.GetProjectDemand(context.Background(), f.tenantID, f.demandID)
	if demand.Status != ProjectDemandStatusCompleted {
		t.Fatalf("expected demand completed via one-tap sign-all, got %s", demand.Status)
	}
	humanSigned := 0
	for _, v := range repo.demandCriterionVerdicts {
		if v.JudgeType == demandCriterionJudgeTypeHuman {
			humanSigned++
		}
	}
	if humanSigned != 2 {
		t.Fatalf("expected 2 human verdicts (automated criterion untouched), got %d", humanSigned)
	}
	if len(inbox.resolutions) == 0 {
		t.Fatalf("expected the inbox demand_acceptance card to be resolved")
	}
}

// TestResolveDecisionDemandAcceptanceRejectedFailsDemand pins the reject arm of
// the F1 fix: "rejected" signs the first pending human-signable criterion
// unsatisfied, failing the demand and resolving the decision.
func TestResolveDecisionDemandAcceptanceRejectedFailsDemand(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "会被驳回的判据"),
		blockingHumanJudgmentCriterion("c2", "尚未签署的判据"),
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          f.tenantID,
		ProjectID:         f.projectID,
		DecisionRequestID: f.decisionID,
		DecidedByUserID:   f.ownerID,
		Decision:          "rejected",
		Comment:           "证据不足",
	})
	if err != nil {
		t.Fatalf("resolve demand_acceptance rejected: %v", err)
	}
	if resolved.StatusSnapshot != "rejected" {
		t.Fatalf("expected decision rejected, got %s", resolved.StatusSnapshot)
	}
	demand, _ := repo.GetProjectDemand(context.Background(), f.tenantID, f.demandID)
	if demand.Status != ProjectDemandStatusFailed {
		t.Fatalf("expected demand failed, got %s", demand.Status)
	}
}

// TestResolveDecisionDemandAcceptanceClosesOrphanOnFailedDemand: a still-pending
// demand_acceptance card whose demand is already failed (and/or whose plan
// revision is superseded) must force-close and project the inbox instead of
// returning "projection not applied" while leaving the card open.
func TestResolveDecisionDemandAcceptanceClosesOrphanOnFailedDemand(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "过期判据"),
	})
	// Demand already terminal; plan revision superseded — the card is orphaned.
	for i := range repo.demands {
		if repo.demands[i].ID == f.demandID {
			repo.demands[i].Status = ProjectDemandStatusFailed
		}
	}
	for i := range repo.planRevisions {
		if repo.planRevisions[i].ID == f.revisionID {
			repo.planRevisions[i].Status = PlanRevisionStatusSuperseded
		}
	}

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          f.tenantID,
		ProjectID:         f.projectID,
		DecisionRequestID: f.decisionID,
		DecidedByUserID:   f.ownerID,
		Decision:          "approved",
		Comment:           "清理过期验收卡",
	})
	if err != nil {
		t.Fatalf("orphan demand_acceptance resolve: %v", err)
	}
	if resolved.StatusSnapshot != "rejected" {
		t.Fatalf("failed demand orphan should resolve as rejected, got %s", resolved.StatusSnapshot)
	}
	if len(inbox.resolutions) == 0 {
		t.Fatalf("expected inbox projection for orphan acceptance card")
	}
	if isPendingDecisionStatus(inbox.resolutions[len(inbox.resolutions)-1].StatusSnapshot) {
		t.Fatalf("inbox projection must not stay pending")
	}
}

func TestSignDemandCriterionVerdictRejectsWithStructuredEvent(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, NoopCoordinatorSignalClient{}, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "会被驳回的判据"),
		blockingHumanJudgmentCriterion("c2", "尚未签署的判据"),
	})

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "unsatisfied",
		Reason:      "证据不足",
	})
	if err != nil {
		t.Fatalf("sign criterion: %v", err)
	}
	if result.DemandStatus != ProjectDemandStatusFailed {
		t.Fatalf("expected demand failed, got %s", result.DemandStatus)
	}
	demand, _ := repo.GetProjectDemand(context.Background(), f.tenantID, f.demandID)
	if demand.Status != ProjectDemandStatusFailed {
		t.Fatalf("expected repository demand failed even with c2 unsigned, got %s", demand.Status)
	}
	if approvals.calls != 1 || approvals.last.Decision != "rejected" {
		t.Fatalf("expected approval resolved rejected, got calls=%d last=%#v", approvals.calls, approvals.last)
	}
	decision, _ := repo.GetDecisionRequest(context.Background(), f.tenantID, f.projectID, f.decisionID)
	if decision.StatusSnapshot != "rejected" {
		t.Fatalf("expected decision resolved rejected, got %s", decision.StatusSnapshot)
	}
	rejectedEvent := lastProjectEventOfType(t, repo.events, ProjectEventDemandAcceptanceRejected)
	if rejectedEvent.Payload["criterion_id"] != "c1" || rejectedEvent.Payload["statement"] != "会被驳回的判据" || rejectedEvent.Payload["reason"] != "证据不足" {
		t.Fatalf("unexpected rejected event payload: %#v", rejectedEvent.Payload)
	}
	// Fix C: demand_id present for consumer symmetry with the completed event.
	if rejectedEvent.Payload["demand_id"] != f.demandID.String() {
		t.Fatalf("expected demand_id in rejected event payload, got %#v", rejectedEvent.Payload)
	}
	// Fix B: this was the project's only demand, now terminal → project acceptance
	// review opened (project running→acceptance, project_acceptance decision created).
	assertProjectAcceptanceReviewOpened(t, repo, approvals, f.projectID)
}

func TestSignDemandCriterionVerdictRejectsUnauthorizedSigner(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "判据"),
	})

	_, err = service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: uuid.New(), // neither the decision's target user nor the project owner
		CriterionID: "c1",
		Verdict:     "satisfied",
	})
	if !errors.Is(err, ErrProjectDecisionForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
	if len(repo.demandCriterionVerdicts) != 0 {
		t.Fatalf("expected no verdict written for unauthorized signer, got %d", len(repo.demandCriterionVerdicts))
	}
}

func addActiveHumanMember(repo *memoryRepository, tenantID, projectID uuid.UUID, role ProjectRole) uuid.UUID {
	memberID := uuid.New()
	repo.members[projectID] = append(repo.members[projectID], ProjectMember{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   memberID,
		ProjectRole:   role,
		Status:        "active",
	})
	return memberID
}

func TestSignDemandCriterionVerdictAllowsProjectHumanMember(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "判据"),
	})
	memberID := addActiveHumanMember(repo, f.tenantID, f.projectID, ProjectRoleReviewer)

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: memberID,
		CriterionID: "c1",
		Verdict:     "satisfied",
	})
	if err != nil {
		t.Fatalf("expected project human member to sign (any-of-N), got %v", err)
	}
	if result == nil {
		t.Fatalf("expected sign result for eligible member")
	}
	if len(repo.demandCriterionVerdicts) != 1 {
		t.Fatalf("expected verdict written by member, got %d", len(repo.demandCriterionVerdicts))
	}
}

func TestSignDemandCriterionVerdictRejectsInactiveOrNonHumanMember(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "判据"),
	})
	inactiveID := uuid.New()
	employeeID := uuid.New()
	repo.members[f.projectID] = append(repo.members[f.projectID],
		ProjectMember{ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, PrincipalType: PrincipalTypeHumanUser, PrincipalID: inactiveID, ProjectRole: ProjectRoleReviewer, Status: "removed"},
		ProjectMember{ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: employeeID, ProjectRole: ProjectRoleExecutor, Status: "active"},
	)
	for _, actorID := range []uuid.UUID{inactiveID, employeeID} {
		_, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
			TenantID:    f.tenantID,
			DemandID:    f.demandID,
			ActorUserID: actorID,
			CriterionID: "c1",
			Verdict:     "satisfied",
		})
		if !errors.Is(err, ErrProjectDecisionForbidden) {
			t.Fatalf("expected forbidden for actor %s, got %v", actorID, err)
		}
	}
}

func TestResolveDecisionAllowsProjectHumanMember(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "demo-project", Goal: "目标",
		Status: ProjectStatusRunning, HumanOwnerUserID: ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID: decisionID, TenantID: tenantID, ProjectID: projectID,
		ApprovalRequestID: uuid.New(), TargetUserID: ownerID,
		DecisionType: "route_review", TitleSnapshot: "需要确认", StatusSnapshot: "pending",
	})
	memberID := addActiveHumanMember(repo, tenantID, projectID, ProjectRoleReviewer)

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID: tenantID, ProjectID: projectID, DecisionRequestID: decisionID,
		DecidedByUserID: memberID, Decision: "approved", Comment: "同意",
	})
	if err != nil {
		t.Fatalf("expected project human member to resolve (any-of-N), got %v", err)
	}
	if resolved.StatusSnapshot != "approved" {
		t.Fatalf("expected approved, got %s", resolved.StatusSnapshot)
	}
}

func TestResolveDecisionRejectsNonMemberActor(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "demo-project", Goal: "目标",
		Status: ProjectStatusRunning, HumanOwnerUserID: ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID: decisionID, TenantID: tenantID, ProjectID: projectID,
		ApprovalRequestID: uuid.New(), TargetUserID: ownerID,
		DecisionType: "route_review", TitleSnapshot: "需要确认", StatusSnapshot: "pending",
	})

	_, err = service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID: tenantID, ProjectID: projectID, DecisionRequestID: decisionID,
		DecidedByUserID: uuid.New(), Decision: "approved",
	})
	if !errors.Is(err, ErrProjectDecisionForbidden) {
		t.Fatalf("expected forbidden for non-member, got %v", err)
	}
	if approvals.calls != 0 {
		t.Fatalf("expected no approval side effect, got %d", approvals.calls)
	}
}

func TestProjectAcceptanceAllowsProjectHumanMember(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	memberID := addActiveHumanMember(repo.memoryRepository, tenantID, projectID, ProjectRoleReviewer)
	evidence, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceType: "test_result", Title: "回归测试结果",
		SourceType: "artifact", SourceRef: "s3://bucket/reports/regression.json",
		SubmittedByType: "human_user", SubmittedByID: &ownerID,
		VerificationStatus: EvidenceVerificationStatusSubmitted,
	})
	if err != nil {
		t.Fatalf("seed evidence: %v", err)
	}
	report, err := repo.CreateReportRef(context.Background(), CreateReportRefRequest{
		TenantID: tenantID, ProjectID: projectID, ReportType: "final_report", Title: "验收报告",
		ObjectRef: "s3://bucket/reports/final.md", Format: "markdown",
		GeneratedByType: "human_user", GeneratedByID: &ownerID,
	})
	if err != nil {
		t.Fatalf("seed report: %v", err)
	}

	if _, err := service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID: tenantID, ProjectID: projectID, AcceptedByUserID: memberID,
		Status: "accepted", Conclusion: "通过", EvidenceRefIDs: []uuid.UUID{evidence.ID}, ReportRefIDs: []uuid.UUID{report.ID},
	}); err != nil {
		t.Fatalf("expected project human member acceptance (any-of-N), got %v", err)
	}
}

func TestSignDemandCriterionVerdictRejectsNonHumanJudgmentCriterion(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		{CriterionID: "c1", Statement: "自动化判据", VerificationMethod: "automated_test", Severity: demandAcceptanceCriterionSeverityBlocking},
	})

	_, err = service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "satisfied",
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error for non human_judgment criterion, got %v", err)
	}
}

func TestSignDemandCriterionVerdictDuplicateSameValueIsIdempotent(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, NoopCoordinatorSignalClient{}, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "第一条判据"),
		blockingHumanJudgmentCriterion("c2", "第二条判据"),
	})
	signReq := SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "satisfied",
		Reason:      "已核实",
	}
	if _, err := service.SignDemandCriterionVerdict(context.Background(), signReq); err != nil {
		t.Fatalf("first sign: %v", err)
	}

	result, err := service.SignDemandCriterionVerdict(context.Background(), signReq)
	if err != nil {
		t.Fatalf("expected idempotent replay to succeed, got %v", err)
	}
	if result.Signed != 1 || result.Total != 2 || result.Remaining != 1 {
		t.Fatalf("expected unchanged progress 1/2, got signed=%d total=%d remaining=%d", result.Signed, result.Total, result.Remaining)
	}
	if len(repo.demandCriterionVerdicts) != 1 {
		t.Fatalf("expected no duplicate verdict row, got %d", len(repo.demandCriterionVerdicts))
	}
}

func TestSignDemandCriterionVerdictDuplicateDifferentValueConflicts(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, NoopCoordinatorSignalClient{}, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "第一条判据"),
		blockingHumanJudgmentCriterion("c2", "第二条判据"),
	})
	if _, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "satisfied",
	}); err != nil {
		t.Fatalf("first sign: %v", err)
	}

	_, err = service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "unsatisfied",
	})
	if !errors.Is(err, ErrProjectConflict) {
		t.Fatalf("expected conflict for re-judgement, got %v", err)
	}
	if len(repo.demandCriterionVerdicts) != 1 {
		t.Fatalf("expected the original verdict row to be untouched, got %d", len(repo.demandCriterionVerdicts))
	}
	demand, _ := repo.GetProjectDemand(context.Background(), f.tenantID, f.demandID)
	if demand.Status != ProjectDemandStatusAcceptancePending {
		t.Fatalf("expected demand to stay acceptance_pending after rejected re-judgement, got %s", demand.Status)
	}
}

func TestSignDemandCriterionVerdictRequiresAcceptancePendingStatus(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "判据"),
	})
	for i := range repo.demands {
		if repo.demands[i].ID == f.demandID {
			repo.demands[i].Status = ProjectDemandStatusExecuting
		}
	}

	_, err = service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "satisfied",
	})
	if !errors.Is(err, ErrProjectConflict) {
		t.Fatalf("expected conflict for non acceptance_pending demand, got %v", err)
	}
}

// assertProjectAcceptanceReviewOpened verifies Fix B: when the last demand of a
// (running) project converges terminal, the service opens the project acceptance
// review — project transitions running→acceptance and a pending project_acceptance
// decision exists with demand-first title/context (never the opaque "验收项目交付").
func assertProjectAcceptanceReviewOpened(t *testing.T, repo *memoryRepository, approvals *fakeApprovalResolver, projectID uuid.UUID) {
	t.Helper()
	if repo.projects[projectID].Status != ProjectStatusAcceptance {
		t.Fatalf("expected project status acceptance, got %s", repo.projects[projectID].Status)
	}
	if approvals.createCalls != 1 || approvals.lastCreate.DecisionType != "project_acceptance" {
		t.Fatalf("expected one project_acceptance approval created, got calls=%d last=%#v", approvals.createCalls, approvals.lastCreate)
	}
	if strings.Contains(approvals.lastCreate.Title, "验收项目交付") {
		t.Fatalf("project_acceptance title must not use opaque project-only copy, got %q", approvals.lastCreate.Title)
	}
	if !strings.HasPrefix(approvals.lastCreate.Title, "结项确认 · ") {
		t.Fatalf("expected project-first title prefix 结项确认 · , got %q", approvals.lastCreate.Title)
	}
	if approvals.lastCreate.ContextPayload == nil {
		t.Fatalf("expected ContextPayload on project_acceptance approval")
	}
	if _, ok := approvals.lastCreate.ContextPayload["demands"]; !ok {
		t.Fatalf("expected demands in ContextPayload, got %#v", approvals.lastCreate.ContextPayload)
	}
	if _, ok := approvals.lastCreate.ContextPayload["primary_demand_id"]; !ok {
		t.Fatalf("expected primary_demand_id in ContextPayload, got %#v", approvals.lastCreate.ContextPayload)
	}
	found := false
	for _, d := range repo.decisionRequests {
		if d.ProjectID == projectID && d.DecisionType == "project_acceptance" && d.StatusSnapshot == "pending" {
			found = true
			if strings.Contains(d.TitleSnapshot, "验收项目交付") || !strings.HasPrefix(d.TitleSnapshot, "结项确认 · ") {
				t.Fatalf("expected project-first decision title, got %q", d.TitleSnapshot)
			}
		}
	}
	if !found {
		t.Fatalf("expected a pending project_acceptance decision for project %s", projectID)
	}
}

// TestSignDemandCriterionVerdictHealsPartialFailureToTerminal covers Fix A: a
// prior attempt wrote the verdict but died before advancing/resolving, leaving
// the demand stuck acceptance_pending with its decision still pending. Re-signing
// the same value must re-run convergence and heal to completed — not early-return
// with the demand still stuck.
func TestSignDemandCriterionVerdictHealsPartialFailureToTerminal(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "唯一一条判据"),
	})
	// Simulate the partial-failure state: the verdict row exists (a prior attempt
	// wrote it) but the demand is still acceptance_pending and the decision still
	// pending (that attempt died before advance/resolve).
	if err := repo.CreateDemandCriterionVerdict(context.Background(), CreateDemandCriterionVerdictRequest{
		TenantID:       f.tenantID,
		ProjectID:      f.projectID,
		DemandID:       f.demandID,
		PlanRevisionID: f.revisionID,
		CriterionID:    "c1",
		Verdict:        "satisfied",
		JudgeType:      "human",
		JudgeID:        f.ownerID,
	}); err != nil {
		t.Fatalf("seed partial verdict: %v", err)
	}

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "satisfied",
	})
	if err != nil {
		t.Fatalf("re-sign to heal: %v", err)
	}
	if result.DemandStatus != ProjectDemandStatusCompleted {
		t.Fatalf("expected healed demand completed, got %s", result.DemandStatus)
	}
	if demand, _ := repo.GetProjectDemand(context.Background(), f.tenantID, f.demandID); demand.Status != ProjectDemandStatusCompleted {
		t.Fatalf("expected repository demand healed to completed, got %s", demand.Status)
	}
	decision, _ := repo.GetDecisionRequest(context.Background(), f.tenantID, f.projectID, f.decisionID)
	if decision.StatusSnapshot != "approved" {
		t.Fatalf("expected decision resolved approved on heal, got %s", decision.StatusSnapshot)
	}
	// No duplicate verdict row despite the re-sign.
	if count := countHumanVerdicts(repo.demandCriterionVerdicts, "c1"); count != 1 {
		t.Fatalf("expected exactly one human verdict for c1, got %d", count)
	}
	assertProjectAcceptanceReviewOpened(t, repo, approvals, f.projectID)
}

// TestSignDemandCriterionVerdictReconcilesAlreadyCompletedDemand covers Fix A's
// other partial-failure shape: a prior attempt advanced the demand to completed
// but died before resolving the decision. A retry (or a late duplicate sign)
// finds the demand terminal and must reconcile — resolve the still-pending
// decision to match — rather than 409-ing on the acceptance_pending precondition.
func TestSignDemandCriterionVerdictReconcilesAlreadyCompletedDemand(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "唯一一条判据"),
	})
	// Verdict written + demand already advanced to completed, but decision left pending.
	if err := repo.CreateDemandCriterionVerdict(context.Background(), CreateDemandCriterionVerdictRequest{
		TenantID: f.tenantID, ProjectID: f.projectID, DemandID: f.demandID, PlanRevisionID: f.revisionID,
		CriterionID: "c1", Verdict: "satisfied", JudgeType: "human", JudgeID: f.ownerID,
	}); err != nil {
		t.Fatalf("seed verdict: %v", err)
	}
	for i := range repo.demands {
		if repo.demands[i].ID == f.demandID {
			repo.demands[i].Status = ProjectDemandStatusCompleted
		}
	}

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "satisfied",
	})
	if err != nil {
		t.Fatalf("reconcile completed demand: %v", err)
	}
	if result.DemandStatus != ProjectDemandStatusCompleted {
		t.Fatalf("expected completed, got %s", result.DemandStatus)
	}
	decision, _ := repo.GetDecisionRequest(context.Background(), f.tenantID, f.projectID, f.decisionID)
	if decision.StatusSnapshot != "approved" {
		t.Fatalf("expected pending decision reconciled to approved, got %s", decision.StatusSnapshot)
	}
	if approvals.calls != 1 || approvals.last.Decision != "approved" {
		t.Fatalf("expected approval reconciled approved, got calls=%d last=%#v", approvals.calls, approvals.last)
	}
	assertProjectAcceptanceReviewOpened(t, repo, approvals, f.projectID)
}

// TestSignDemandCriterionVerdictDoesNotOpenReviewWhileOtherDemandsRemain covers
// the Fix B guard: signing one demand terminal must NOT open the project
// acceptance review while a sibling demand is still non-terminal.
func TestSignDemandCriterionVerdictDoesNotOpenReviewWhileOtherDemandsRemain(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, NoopCoordinatorSignalClient{}, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		blockingHumanJudgmentCriterion("c1", "唯一一条判据"),
	})
	// A sibling demand in the same project is still executing.
	repo.demands = append(repo.demands, ProjectDemand{
		ID:        uuid.New(),
		TenantID:  f.tenantID,
		ProjectID: f.projectID,
		Title:     "并行需求",
		Status:    ProjectDemandStatusExecuting,
	})

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     "satisfied",
	})
	if err != nil {
		t.Fatalf("sign criterion: %v", err)
	}
	if result.DemandStatus != ProjectDemandStatusCompleted {
		t.Fatalf("expected this demand completed, got %s", result.DemandStatus)
	}
	if repo.projects[f.projectID].Status != ProjectStatusRunning {
		t.Fatalf("expected project to stay running while sibling demand executes, got %s", repo.projects[f.projectID].Status)
	}
	if approvals.createCalls != 0 {
		t.Fatalf("expected no project_acceptance review opened, got %d create calls", approvals.createCalls)
	}
}

func countHumanVerdicts(verdicts []DemandCriterionVerdict, criterionID string) int {
	count := 0
	for _, v := range verdicts {
		if v.CriterionID == criterionID && v.JudgeType == "human" && v.ProjectTaskID == nil {
			count++
		}
	}
	return count
}

func TestUpdateConfigRejectsArchivedProject(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         uuid.New(),
		Name:             "已归档项目",
		Status:           ProjectStatusArchived,
		HumanOwnerUserID: uuid.New(),
	}
	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    repo.projects[projectID].TenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "新名称",
	})
	if !errors.Is(err, ErrProjectArchived) {
		t.Fatalf("expected archived error, got %v", err)
	}
}

func TestArchiveProjectBlocksWhenActiveTasksRemain(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "仍有任务",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        uuid.New(),
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "执行中",
		Status:    "running",
	})
	_, err = service.ArchiveProject(context.Background(), tenantID, projectID, actorID)
	if !errors.Is(err, ErrProjectArchiveBlocked) {
		t.Fatalf("expected archive blocked, got %v", err)
	}
	if repo.projects[projectID].Status == ProjectStatusArchived {
		t.Fatal("project must stay running when archive is blocked")
	}
}

func TestArchiveProjectAllowsTerminalTasksOnly(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "可归档",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	repo.tasks = append(repo.tasks,
		ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "完成", Status: "completed"},
		ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "失败", Status: "failed"},
		ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "取消", Status: "cancelled"},
	)
	got, err := service.ArchiveProject(context.Background(), tenantID, projectID, actorID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if got.Status != ProjectStatusArchived {
		t.Fatalf("expected archived, got %s", got.Status)
	}
	if got.ArchivedAt == nil {
		t.Fatal("expected archived_at")
	}
	found := false
	for _, et := range repo.eventTypes {
		if et == ProjectEventArchived {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected project.archived event, got %#v", repo.eventTypes)
	}
}

func TestArchiveProjectBlocksOpenDemands(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "有未结需求",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	repo.demands = append(repo.demands, ProjectDemand{
		ID:        uuid.New(),
		TenantID:  tenantID,
		ProjectID: projectID,
		Status:    ProjectDemandStatusExecuting,
	})
	_, err = service.ArchiveProject(context.Background(), tenantID, projectID, actorID)
	var blocked *ProjectArchiveBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected archive blocked error, got %v", err)
	}
	if len(blocked.Blockers) != 1 || blocked.Blockers[0].Code != "open_demands" {
		t.Fatalf("unexpected blockers: %#v", blocked.Blockers)
	}
}

func TestArchiveProjectBlocksPendingDecisions(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "有待决",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	repo.deletePreviewCounts = ProjectDeleteWarnings{PendingDecisionCount: 2}
	_, err = service.ArchiveProject(context.Background(), tenantID, projectID, actorID)
	var blocked *ProjectArchiveBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected archive blocked error, got %v", err)
	}
	found := false
	for _, b := range blocked.Blockers {
		if b.Code == "pending_decisions" && b.Count == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pending_decisions blocker, got %#v", blocked.Blockers)
	}
}

func TestBuildArchivePreviewCanArchiveWithMissingEvidenceWarning(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "空材料",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	preview, err := service.BuildArchivePreview(context.Background(), tenantID, projectID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !preview.CanArchive {
		t.Fatalf("expected can_archive true, blockers=%#v", preview.Blockers)
	}
	found := false
	for _, w := range preview.Warnings {
		if w.Code == "missing_evidence" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing_evidence warning, got %#v", preview.Warnings)
	}
	for _, code := range preview.BlockedReasons {
		if code == "missing_final_report" {
			t.Fatalf("missing_final_report should merge into missing_evidence, got %#v", preview.BlockedReasons)
		}
	}
}

func TestTryCloseProjectFromDemandSignOffDefersOnPendingDecision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "结项延后",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	repo.demands = append(repo.demands, ProjectDemand{
		ID:        uuid.New(),
		TenantID:  tenantID,
		ProjectID: projectID,
		Status:    ProjectDemandStatusCompleted,
	})
	repo.deletePreviewCounts = ProjectDeleteWarnings{PendingDecisionCount: 1}
	closed, err := service.tryCloseProjectFromDemandSignOff(context.Background(), tenantID, projectID, actorID, "验收通过")
	if err != nil {
		t.Fatalf("expected no error bubble, got %v", err)
	}
	if closed {
		t.Fatal("expected closed=false when pending decisions block archive")
	}
	if repo.projects[projectID].Status == ProjectStatusArchived {
		t.Fatal("project must stay running")
	}
	found := false
	for _, ev := range repo.events {
		if ev.EventType == ProjectEventArchiveAutoCloseDeferred {
			found = true
			break
		}
	}
	if !found {
		// memory repo may track eventTypes instead of full events
		for _, et := range repo.eventTypes {
			if et == ProjectEventArchiveAutoCloseDeferred {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected auto_close_deferred event, events=%#v types=%#v", repo.events, repo.eventTypes)
	}
}

func TestArchiveProjectAllowsEmptyProjectWithNoDemands(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "空项目可归档",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	// seed minimal evidence+report so missing_evidence is only a warning
	repo.evidenceRefs = append(repo.evidenceRefs, ProjectEvidenceRef{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID})
	repo.reportRefs = append(repo.reportRefs, ProjectReportRef{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID})
	preview, err := service.BuildArchivePreview(context.Background(), tenantID, projectID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !preview.CanArchive {
		t.Fatalf("empty project should be archivable, blockers=%#v", preview.Blockers)
	}
	project, err := service.ArchiveProject(context.Background(), tenantID, projectID, actorID)
	if err != nil {
		t.Fatalf("archive empty project: %v", err)
	}
	if project.Status != ProjectStatusArchived {
		t.Fatalf("expected archived, got %s", project.Status)
	}
}

func TestUnarchiveProjectRestoresRunning(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	now := time.Now().UTC()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "已归档",
		Status:           ProjectStatusArchived,
		ArchivedAt:       &now,
		HumanOwnerUserID: actorID,
	}
	got, err := service.UnarchiveProject(context.Background(), tenantID, projectID, actorID)
	if err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if got.Status != ProjectStatusRunning {
		t.Fatalf("expected running, got %s", got.Status)
	}
	if got.ArchivedAt != nil {
		t.Fatalf("expected archived_at cleared, got %#v", got.ArchivedAt)
	}
	_, err = service.UnarchiveProject(context.Background(), tenantID, projectID, actorID)
	if !errors.Is(err, ErrProjectNotArchived) {
		t.Fatalf("expected not-archived on second unarchive, got %v", err)
	}
	found := false
	for _, et := range repo.eventTypes {
		if et == ProjectEventUnarchived {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected project.unarchived event, got %#v", repo.eventTypes)
	}
}

func TestUpdateProjectConfigCreatesRevision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "old-project",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	updated, err := service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "old-project",
		Goal:        "新目标",
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if updated.Name != "old-project" {
		t.Fatalf("expected project name unchanged, got %q", updated.Name)
	}
	if updated.Goal != "新目标" {
		t.Fatalf("expected updated goal, got %q", updated.Goal)
	}
	if len(repo.revisions) != 1 {
		t.Fatalf("expected config revision, got %d", len(repo.revisions))
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventConfigChanged {
		t.Fatalf("expected config changed event, got %#v", repo.eventTypes)
	}
}

func TestUpdateProjectConfigAllowsDisplayNameChange(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "旧展示名",
		DirectoryName:    "old-project",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	updated, err := service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: ownerID,
		Name:        "新展示名",
	})
	if err != nil {
		t.Fatalf("update display name: %v", err)
	}
	if updated.Name != "新展示名" {
		t.Fatalf("expected display name updated, got %q", updated.Name)
	}
	if updated.DirectoryName != "old-project" {
		t.Fatalf("directory name must stay immutable, got %q", updated.DirectoryName)
	}
}

func TestUpdateProjectConfigPreservesAndClearsRepoBinding(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	credentialRef := "git-credential:primary"
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "with-repo-binding",
		Goal:             "验证更新语义",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
		RepoBinding: ProjectRepoBinding{
			Status:           ProjectRepoBindingStatusBound,
			URL:              "https://github.com/acme/superteam.git",
			DefaultBranch:    "main",
			GitCredentialRef: &credentialRef,
			Scope:            []string{"apps/control-plane"},
		},
	}

	preserved, err := service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: ownerID,
		Name:        "with-repo-binding",
		Goal:        "验证更新语义-改目标",
	})
	require.NoError(t, err)
	require.Equal(t, "with-repo-binding", preserved.Name)
	require.Equal(t, ProjectRepoBindingStatusBound, preserved.RepoBinding.Status)
	require.Equal(t, "https://github.com/acme/superteam.git", preserved.RepoBinding.URL)
	require.Equal(t, []string{"apps/control-plane"}, preserved.RepoBinding.Scope)

	cleared, err := service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: ownerID,
		RepoBinding: &ProjectRepoBindingInput{},
	})
	require.NoError(t, err)
	require.Equal(t, ProjectRepoBindingStatusUnbound, cleared.RepoBinding.Status)
	require.Empty(t, cleared.RepoBinding.URL)
	require.Empty(t, cleared.RepoBinding.DefaultBranch)
	require.Nil(t, cleared.RepoBinding.GitCredentialRef)
	require.Empty(t, cleared.RepoBinding.Scope)
}

func TestCreateProjectTaskAttestationDefaultsProviderAuthModeAndPersistsRuntimeMetadata(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	projectTaskID := uuid.New()
	attemptID := uuid.New()
	runtimeNodeID := uuid.New()
	digitalEmployeeID := uuid.New()

	attestation, err := service.CreateProjectTaskAttestation(context.Background(), CreateProjectTaskAttestationRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		ProjectTaskID:             projectTaskID,
		AttemptID:                 attemptID,
		RuntimeNodeID:             runtimeNodeID,
		DigitalEmployeeID:         digitalEmployeeID,
		CapabilityManifestVersion: "cap-manifest:v3",
		AttestationType:           "provider_start",
		Status:                    ProjectTaskAttestationStatusSucceeded,
		CommandArgv:               []any{"codex", "exec"},
		ExitCode:                  serviceTestInt32Ptr(0),
		DurationMs:                serviceTestInt64Ptr(1234),
		StdoutSha256:              serviceTestStringPtr("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		StderrSha256:              serviceTestStringPtr("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		ArtifactRefs:              []any{map[string]any{"type": "log", "ref": "artifact:stdout"}},
		ArtifactHashes:            map[string]any{"artifact:stdout": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Metadata:                  map[string]any{"workspace_mode": "branch"},
		IdempotencyKey:            "attestation-1",
	})
	require.NoError(t, err)
	require.NotNil(t, attestation)
	require.Equal(t, tenantID, attestation.TenantID)
	require.Equal(t, digitalEmployeeID, attestation.DigitalEmployeeID)
	require.Equal(t, "cap-manifest:v3", attestation.CapabilityManifestVersion)
	require.Equal(t, ProjectTaskAttestationProviderAuthModeHost, attestation.ProviderAuthMode)
	require.Equal(t, "attestation-1", attestation.IdempotencyKey)
	require.Len(t, repo.projectTaskAttestations, 1)
	require.Equal(t, ProjectTaskAttestationProviderAuthModeHost, repo.projectTaskAttestations[0].ProviderAuthMode)
}

func TestCreateProjectTaskAttestationRedactsRuntimeLocalPathMetadata(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	attestation, err := service.CreateProjectTaskAttestation(context.Background(), CreateProjectTaskAttestationRequest{
		TenantID:          uuid.New(),
		ProjectID:         uuid.New(),
		ProjectTaskID:     uuid.New(),
		AttemptID:         uuid.New(),
		RuntimeNodeID:     uuid.New(),
		DigitalEmployeeID: uuid.New(),
		AttestationType:   "provider_start",
		Status:            ProjectTaskAttestationStatusSucceeded,
		Metadata: map[string]any{
			"workspace_ref":  "project-workspace:project/task/attempt",
			"agent_home_dir": "/srv/runtime/employees/emp-1",
			"nested": map[string]any{
				"workspace_path": "/srv/runtime/workspaces/project",
				"keep":           "value",
			},
			"items": []any{
				map[string]any{
					"mcp_config_path": "/srv/runtime/workspaces/project/.superteam/mcp.json",
					"keep":            "list-value",
				},
			},
		},
		IdempotencyKey: "attestation-redaction",
	})
	require.NoError(t, err)

	require.NotContains(t, attestation.Metadata, "agent_home_dir")
	require.Equal(t, "project-workspace:project/task/attempt", attestation.Metadata["workspace_ref"])
	nested := attestation.Metadata["nested"].(map[string]any)
	require.NotContains(t, nested, "workspace_path")
	require.Equal(t, "value", nested["keep"])
	items := attestation.Metadata["items"].([]any)
	item := items[0].(map[string]any)
	require.NotContains(t, item, "mcp_config_path")
	require.Equal(t, "list-value", item["keep"])
	require.NotContains(t, repo.projectTaskAttestations[0].Metadata, "agent_home_dir")
}

func serviceTestStringPtr(value string) *string {
	return &value
}

func serviceTestInt32Ptr(value int32) *int32 {
	return &value
}

func serviceTestInt64Ptr(value int64) *int64 {
	return &value
}

func TestCreateProjectTaskAttestationRejectsMissingRequiredMetadata(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	require.NoError(t, err)
	base := CreateProjectTaskAttestationRequest{
		TenantID:                  uuid.New(),
		ProjectID:                 uuid.New(),
		ProjectTaskID:             uuid.New(),
		AttemptID:                 uuid.New(),
		RuntimeNodeID:             uuid.New(),
		DigitalEmployeeID:         uuid.New(),
		CapabilityManifestVersion: "cap-manifest:v3",
		AttestationType:           "provider_start",
		Status:                    ProjectTaskAttestationStatusSucceeded,
		IdempotencyKey:            "attestation-1",
	}

	for _, tc := range []struct {
		name string
		mut  func(*CreateProjectTaskAttestationRequest)
	}{
		{name: "digital employee", mut: func(req *CreateProjectTaskAttestationRequest) { req.DigitalEmployeeID = uuid.Nil }},
		{name: "provider auth mode", mut: func(req *CreateProjectTaskAttestationRequest) { req.ProviderAuthMode = "unknown" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			tc.mut(&req)
			_, err := service.CreateProjectTaskAttestation(context.Background(), req)
			require.ErrorIs(t, err, ErrInvalidProjectEvidence)
		})
	}
}

func TestCreateProjectTaskAttestationAcceptsOptionalCapabilityManifestAndExplicitAuthModes(t *testing.T) {
	for _, mode := range []ProjectTaskAttestationProviderAuthMode{
		ProjectTaskAttestationProviderAuthModeEmployee,
		ProjectTaskAttestationProviderAuthModeExplicitCredential,
	} {
		t.Run(string(mode), func(t *testing.T) {
			repo := newMemoryRepository()
			service, err := NewService(repo)
			require.NoError(t, err)

			attestation, err := service.CreateProjectTaskAttestation(context.Background(), CreateProjectTaskAttestationRequest{
				TenantID:          uuid.New(),
				ProjectID:         uuid.New(),
				ProjectTaskID:     uuid.New(),
				AttemptID:         uuid.New(),
				RuntimeNodeID:     uuid.New(),
				DigitalEmployeeID: uuid.New(),
				ProviderAuthMode:  mode,
				AttestationType:   "provider_start",
				Status:            ProjectTaskAttestationStatusSucceeded,
				IdempotencyKey:    "attestation-" + string(mode),
			})

			require.NoError(t, err)
			require.Empty(t, attestation.CapabilityManifestVersion)
			require.Equal(t, mode, attestation.ProviderAuthMode)
		})
	}
}

func TestProjectTaskAttemptBudgetHeartbeatTripsWallClock(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{ID: taskID, TenantID: tenantID, ProjectID: projectID, Status: ProjectTaskStatusRunning})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID: attemptID, TenantID: tenantID, ProjectTaskID: taskID, Status: ProjectTaskAttemptStatusRunning,
		BudgetWallClockLimitSec: serviceTestInt32Ptr(10),
	})

	result, err := service.RecordProjectTaskAttemptBudgetHeartbeat(context.Background(), RecordProjectTaskAttemptBudgetHeartbeatRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        taskID,
		AttemptID:            attemptID,
		ConsumedWallClockSec: 11,
	})

	require.NoError(t, err)
	require.True(t, result.Tripped)
	require.Equal(t, "wall_clock_exceeded", result.TripReason)
	require.Equal(t, int32(11), result.Attempt.BudgetConsumedWallClockSec)
	require.NotNil(t, result.Attempt.BudgetTrippedAt)
	require.NotNil(t, result.Attempt.BudgetTripReason)
	require.Equal(t, "wall_clock_exceeded", *result.Attempt.BudgetTripReason)
}

func TestUpdateProjectConfigRecordsRetryableWorkflowSignalFailure(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{policySignalErr: errors.New("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "old-project",
		Goal:                   "旧目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}

	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "old-project",
		Goal:        "新目标",
	})
	if err == nil {
		t.Fatal("expected signal error")
	}
	if len(repo.eventTypes) != 3 || repo.eventTypes[1] != ProjectEventWorkflowSignaled || repo.eventTypes[2] != ProjectEventWorkflowCoordinationFailed {
		t.Fatalf("expected workflow signal failure event, got %#v", repo.eventTypes)
	}
	payload := lastProjectEventOfType(t, repo.events, ProjectEventWorkflowSignaled).Payload
	if payload["signal_name"] != "ProjectPolicyChanged" || payload["status"] != "failed" || payload["retryable"] != true {
		t.Fatalf("unexpected workflow signal payload: %#v", payload)
	}
	if payload["changed_event_id"] == "" || payload["error"] == "" {
		t.Fatalf("expected retry payload to include event id and error: %#v", payload)
	}
}

func TestUpdateProjectConfigRejectsMissingIDs(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, tc := range []struct {
		name string
		req  UpdateProjectConfigRequest
	}{
		{name: "tenant", req: UpdateProjectConfigRequest{ProjectID: uuid.New(), ActorUserID: uuid.New()}},
		{name: "project", req: UpdateProjectConfigRequest{TenantID: uuid.New(), ActorUserID: uuid.New()}},
		{name: "actor", req: UpdateProjectConfigRequest{TenantID: uuid.New(), ProjectID: uuid.New()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.UpdateProjectConfig(context.Background(), tc.req)
			if !errors.Is(err, ErrInvalidProject) {
				t.Fatalf("expected invalid project error, got %v", err)
			}
		})
	}
}

func TestUpdateProjectConfigWithoutMembersPreservesExistingMembers(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	memberID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "old-project",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: memberID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   memberID,
		ProjectRole:   ProjectRoleOwner,
		Status:        "active",
	}}

	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        " old-project ",
		Goal:        "新目标",
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if got := repo.projects[projectID].Name; got != "old-project" {
		t.Fatalf("expected name unchanged, got %q", got)
	}
	if len(repo.members[projectID]) != 1 {
		t.Fatalf("expected members to be preserved, got %d", len(repo.members[projectID]))
	}
}

func TestUpdateProjectConfigRejectsUnauthorizedTeamMembers(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	actorUserID := uuid.New()
	allowedTeamID := uuid.New()
	unauthorizedTeamID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "旧项目",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: actorUserID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeTeam,
		PrincipalID:   allowedTeamID,
		ProjectRole:   ProjectRoleObserver,
		Status:        "active",
	}}
	repo.authorizeProjectTeamScope(tenantID, actorUserID, allowedTeamID)

	updatedMembers := []ProjectMemberInput{{
		PrincipalType: PrincipalTypeTeam,
		PrincipalID:   unauthorizedTeamID,
		ProjectRole:   ProjectRoleObserver,
	}}
	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: actorUserID,
		Members:     &updatedMembers,
	})
	if !errors.Is(err, ErrUnauthorizedProjectTeamScope) {
		t.Fatalf("expected unauthorized team scope, got %v", err)
	}
	if got := repo.members[projectID][0].PrincipalID; got != allowedTeamID {
		t.Fatalf("expected existing members unchanged, got %s", got)
	}
}

func TestReplaceProjectMembersRequiresActorAndRecordsEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "demo-project",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	_, err = service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.Nil, nil)
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}

	members, err := service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.New(), []ProjectMemberInput{
		{
			PrincipalType: PrincipalTypeHumanUser,
			PrincipalID:   uuid.New(),
			ProjectRole:   ProjectRoleOwner,
		},
		{
			PrincipalType: PrincipalTypeDigitalEmployee,
			PrincipalID:   uuid.New(),
			ProjectRole:   ProjectRoleExecutor,
		},
	})
	if err != nil {
		t.Fatalf("replace members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected two members, got %d", len(members))
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventConfigChanged {
		t.Fatalf("expected config changed event, got %#v", repo.eventTypes)
	}
	if got := repo.events[0].Payload["member_count"]; got != 2 {
		t.Fatalf("expected member_count payload, got %#v", got)
	}
}

func TestListProjectRunSummariesService(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	lastActivity := time.Now().Add(-time.Hour)
	repo.runSummaries = []ProjectRunSummary{{
		ProjectID:            projectID,
		Name:                 "运行带项目",
		Status:               ProjectStatusRunning,
		RunningCount:         2,
		WaitingHumanCount:    1,
		FailedCount:          3,
		OpenDecisionCount:    4,
		EvidencePendingCount: 5,
		// 宽口径 1 与 orphan 口径 0 故意不同：两者串错会被下面的断言抓到。
		WaitingHumanUnlinkedCount: 0,
		LastActivityAt:            &lastActivity,
	}}
	repo.completedTodayCount = 7

	if _, err := service.ListProjectRunSummaries(context.Background(), ListProjectRunSummariesRequest{}); err == nil {
		t.Fatalf("expected nil tenant to be rejected")
	}

	result, err := service.ListProjectRunSummaries(context.Background(), ListProjectRunSummariesRequest{TenantID: tenantID, Limit: 500})
	if err != nil {
		t.Fatalf("list project run summaries: %v", err)
	}
	if repo.lastListRunSummariesReq.TenantID != tenantID || repo.lastListRunSummariesReq.Limit != 500 {
		t.Fatalf("expected limit 500 (run-summary max) with tenant passthrough, got %#v", repo.lastListRunSummariesReq)
	}
	// 超过 500 封顶
	if _, err := service.ListProjectRunSummaries(context.Background(), ListProjectRunSummariesRequest{TenantID: tenantID, Limit: 999}); err != nil {
		t.Fatalf("list project run summaries over max: %v", err)
	}
	if repo.lastListRunSummariesReq.Limit != 500 {
		t.Fatalf("expected limit clamped to 500, got %d", repo.lastListRunSummariesReq.Limit)
	}
	if len(result.Items) != 1 || result.Items[0].ProjectID != projectID || result.Items[0].RunningCount != 2 {
		t.Fatalf("expected repository summaries passthrough, got %#v", result.Items)
	}
	if result.Items[0].OpenDecisionCount != 4 || result.Items[0].EvidencePendingCount != 5 || result.Items[0].FailedCount != 3 {
		t.Fatalf("expected new count columns passthrough, got open=%d evidence=%d failed=%d",
			result.Items[0].OpenDecisionCount, result.Items[0].EvidencePendingCount, result.Items[0].FailedCount)
	}
	// 宽口径(大屏「待人工」)与 orphan 口径(项目首页「等人」)是两个字段，串用会让
	// 大屏计数塌成 orphan 数。此处用不同取值锁死映射不得互换。
	if result.Items[0].WaitingHumanCount != 1 || result.Items[0].WaitingHumanUnlinkedCount != 0 {
		t.Fatalf("expected waiting_human wide=1 / unlinked=0 passthrough, got wide=%d unlinked=%d",
			result.Items[0].WaitingHumanCount, result.Items[0].WaitingHumanUnlinkedCount)
	}
	if result.TodayCompletedRunCount != 7 {
		t.Fatalf("expected tenant-wide today completed count 7, got %d", result.TodayCompletedRunCount)
	}
}

func TestListPaginationIsNormalized(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "demo-project",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	if _, err := service.ListProjects(context.Background(), ListProjectsRequest{TenantID: tenantID, Limit: 200, Offset: -5}); err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if repo.lastListProjects.Limit != 100 || repo.lastListProjects.Offset != 0 {
		t.Fatalf("expected projects pagination 100/0, got %d/%d", repo.lastListProjects.Limit, repo.lastListProjects.Offset)
	}
	if _, err := service.ListProjectEvents(context.Background(), tenantID, projectID, 0, -1); err != nil {
		t.Fatalf("list events: %v", err)
	}
	if repo.lastEventsLimit != 50 || repo.lastEventsOffset != 0 {
		t.Fatalf("expected events pagination 50/0, got %d/%d", repo.lastEventsLimit, repo.lastEventsOffset)
	}
	if _, err := service.ListProjectDemands(context.Background(), tenantID, projectID, 101, -2); err != nil {
		t.Fatalf("list demands: %v", err)
	}
	if repo.lastDemandsLimit != 100 || repo.lastDemandsOffset != 0 {
		t.Fatalf("expected demands pagination 100/0, got %d/%d", repo.lastDemandsLimit, repo.lastDemandsOffset)
	}
	// 概览已不再拉任务页（active_tasks 字段退役，计数走聚合），只剩事件分页需要钉住。
	repo.lastTasksLimit = -1
	if _, err := service.GetOverview(context.Background(), tenantID, projectID); err != nil {
		t.Fatalf("get overview: %v", err)
	}
	if repo.lastEventsLimit != 20 || repo.lastEventsOffset != 0 {
		t.Fatalf("expected overview events pagination 20/0, got %d/%d", repo.lastEventsLimit, repo.lastEventsOffset)
	}
	if repo.lastTasksLimit != -1 {
		t.Fatalf("overview 不应再查询任务页，却调用了 ListProjectTasks(limit=%d)", repo.lastTasksLimit)
	}
}

// 概览计数必须来自全表聚合，不能受 ListProjectTasks 的 20 条分页窗口影响；
// 且 cancelled 属终态，不算 active。
func TestGetOverviewTaskSummaryCountsBeyondTaskPage(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "demo-project",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	// 25 条任务 > 概览任务页的 20 条上限。
	statuses := []string{}
	for i := 0; i < 18; i++ {
		statuses = append(statuses, "completed")
	}
	statuses = append(statuses, "running", "running", "waiting_human", "planned", "failed", "cancelled", "cancelled")
	for i, status := range statuses {
		repo.tasks = append(repo.tasks, ProjectTask{
			ID:        uuid.New(),
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     fmt.Sprintf("task-%d", i),
			Status:    status,
		})
	}

	overview, err := service.GetOverview(context.Background(), tenantID, projectID)
	require.NoError(t, err)

	require.Equal(t, 25, overview.TaskSummary.TotalTasks)
	require.Equal(t, 18, overview.TaskSummary.CompletedTasks)
	require.Equal(t, 1, overview.TaskSummary.FailedTasks)
	require.Equal(t, 2, overview.TaskSummary.CancelledTasks)
	require.Equal(t, 2, overview.TaskSummary.RunningTasks)
	require.Equal(t, 1, overview.TaskSummary.PendingHumanTasks)
	// active = running(2) + waiting_human(1) + planned(1)，cancelled 不计入。
	require.Equal(t, 4, overview.TaskSummary.ActiveTasks)
	require.Equal(t,
		overview.TaskSummary.TotalTasks,
		overview.TaskSummary.ActiveTasks+overview.TaskSummary.CompletedTasks+
			overview.TaskSummary.FailedTasks+overview.TaskSummary.CancelledTasks,
	)
}

func TestDeleteProjectBlockedByActiveTask(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "阻断项目",
		Goal:                   "不可删除",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.deleteBlockers = []ProjectDeleteBlocker{{
		Type:   "project_task",
		ID:     uuid.New().String(),
		Status: "running",
		Title:  "执行中的任务",
	}}

	err = service.DeleteProject(context.Background(), DeleteProjectRequest{
		TenantID: tenantID, ProjectID: projectID, ActorUserID: actorID,
	})
	require.ErrorIs(t, err, ErrProjectDeleteBlocked)
	var blocked *ProjectDeleteBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Len(t, blocked.Blockers, 1)
	require.Equal(t, 0, coordinator.terminateSignals)
	require.Nil(t, repo.projects[projectID].DeletedAt)
	require.Len(t, repo.deleteAuditEvents, 0)
}

func TestDeleteProjectTerminatesThenCascades(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	workflowID := "project-coordinator:" + projectID.String()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "可删除项目",
		Goal:                   "清理",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: workflowID,
	}
	repo.deleteCascadeResult = ProjectDeleteCascadeResult{
		MemberCount: 2, TaskCount: 3, DecisionCount: 1, ApprovalCount: 1,
		InboxCount: 1, RuntimeNodeCount: 1, AffinityCount: 1,
	}

	err = service.DeleteProject(context.Background(), DeleteProjectRequest{
		TenantID: tenantID, ProjectID: projectID, ActorUserID: actorID,
	})
	require.NoError(t, err)
	require.Equal(t, 1, coordinator.terminateSignals)
	require.Equal(t, tenantID, coordinator.lastTerminate.TenantID)
	require.Equal(t, projectID, coordinator.lastTerminate.ProjectID)
	require.Equal(t, workflowID, coordinator.lastTerminate.WorkflowID)
	require.Equal(t, "project deleted", coordinator.lastTerminate.Reason)
	require.NotNil(t, repo.projects[projectID].DeletedAt)
	require.Equal(t, "terminated", repo.projects[projectID].CoordinationStatus)
	require.Len(t, repo.deleteAuditEvents, 1)
	require.Equal(t, actorID, repo.deleteAuditEvents[0].ActorUserID)
	require.Equal(t, projectID, repo.deleteAuditEvents[0].Project.ID)
	require.Equal(t, 2, repo.deleteAuditEvents[0].CascadeResult.MemberCount)
}

func TestDeleteProjectAbortsWhenTerminateFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{terminateSignalErr: errors.New("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "终止失败项目",
		Goal:                   "不可级联",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}

	err = service.DeleteProject(context.Background(), DeleteProjectRequest{
		TenantID: tenantID, ProjectID: projectID, ActorUserID: actorID,
	})
	require.Error(t, err)
	require.Equal(t, 1, coordinator.terminateSignals)
	require.Nil(t, repo.projects[projectID].DeletedAt)
	require.Len(t, repo.deleteAuditEvents, 0)
}

func TestDeleteProjectAbortsWhenAuditFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "审计失败项目",
		Goal:                   "不可级联",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.deleteAuditEventErr = errors.New("audit insert failed")

	err = service.DeleteProject(context.Background(), DeleteProjectRequest{
		TenantID: tenantID, ProjectID: projectID, ActorUserID: actorID,
	})
	require.Error(t, err)
	require.Equal(t, 1, coordinator.terminateSignals)
	require.Nil(t, repo.projects[projectID].DeletedAt)
	require.Len(t, repo.deleteAuditEvents, 0)
}

func TestGetProjectDeletePreviewIncludesWarnings(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "预览项目",
		Goal:             "查看警告",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}
	repo.deletePreviewCounts = ProjectDeleteWarnings{
		PendingDecisionCount:       2,
		WaitingHumanTaskCount:      1,
		OpenInboxCount:             3,
		ActiveMemberCount:          5,
		DigitalEmployeeMemberCount: 2,
		RuntimeNodeBindingCount:    1,
		AffinityCount:              2,
	}

	preview, err := service.GetProjectDeletePreview(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Equal(t, projectID, preview.ProjectID)
	require.Equal(t, "预览项目", preview.ProjectName)
	require.True(t, preview.CanDelete)
	require.Empty(t, preview.Blockers)
	require.Equal(t, int32(2), preview.Warnings.PendingDecisionCount)
	require.Equal(t, int32(1), preview.Warnings.WaitingHumanTaskCount)
	require.Equal(t, int32(3), preview.Warnings.OpenInboxCount)
	require.Equal(t, "删除将取消待审批并解除成员与 Runtime 绑定。", preview.Message)

	repo.deleteBlockers = []ProjectDeleteBlocker{{
		Type: "project_task", ID: uuid.New().String(), Status: "running", Title: "活跃任务",
	}}
	preview, err = service.GetProjectDeletePreview(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.False(t, preview.CanDelete)
	require.Len(t, preview.Blockers, 1)
	require.Contains(t, preview.Message, "存在活跃执行时不可删除")
}

type memoryRepository struct {
	projects                         map[uuid.UUID]Project
	consumedTokens                   map[uuid.UUID]int64
	members                          map[uuid.UUID][]ProjectMember
	tasks                            []ProjectTask
	taskDependents                   map[uuid.UUID][]uuid.UUID
	projectTaskAttempts              []ProjectTaskAttempt
	dispatchGateResults              []PreDispatchGateResult
	events                           []ProjectEvent
	eventTypes                       []ProjectEventType
	demands                          []ProjectDemand
	revisions                        []ProjectConfigRevision
	coordinationJobs                 []CoordinationJob
	routeDecisions                   []RouteDecision
	executionSummaries               []ExecutionSummary
	executionLedgerEvents            []ExecutionLedgerEvent
	projectTaskResults               []ProjectTaskResult
	projectTaskAttestations          []ProjectTaskAttestation
	capabilityProjectionPayloads     map[uuid.UUID][]byte // attempt_id -> start_session payload
	skillNamesByID                   map[uuid.UUID]string
	transferRequests                 []TransferRequest
	decisionRequests                 []DecisionRequest
	planRevisions                    []PlanRevision
	contextUpdates                   []ProjectTaskAttemptContextUpdate
	evidenceRefs                     []ProjectEvidenceRef
	artifactRefs                     []ProjectArtifactRef
	reportRefs                       []ProjectReportRef
	budgetLedger                     []ProjectBudgetLedgerEntry
	acceptanceRecords                []ProjectAcceptanceRecord
	archiveSnapshots                 []ProjectArchiveSnapshot
	demandConstraintExemptions       []DemandConstraintExemption
	demandAcceptanceCriteria         []DemandAcceptanceCriterion
	demandCriterionVerdicts          []DemandCriterionVerdict
	createDemandCriterionVerdictErr  error
	projectTeamScopes                map[uuid.UUID]map[uuid.UUID]map[uuid.UUID]bool
	lastListProjects                 ListProjectsRequest
	lastListRunSummariesReq          ListProjectRunSummariesRequest
	runSummaries                     []ProjectRunSummary
	completedTodayCount              int32
	lastTasksLimit                   int32
	lastTasksOffset                  int32
	lastEventsLimit                  int32
	lastEventsOffset                 int32
	lastDemandsLimit                 int32
	lastDemandsOffset                int32
	lastExecutionSummariesLimit      int32
	lastExecutionSummariesOffset     int32
	executionLedgerEventListRequests []GetExecutionTraceRequest

	taskStatusBeforeUpdate           *string
	appendProjectEventErr            error
	createExecutionSummaryErr        error
	createExecutionLedgerEventErr    error
	createTransferRequestErr         error
	archiveProjectErr                error
	projectTaskRunRuntimeNodes       map[uuid.UUID]uuid.UUID
	projectTaskRunWorkProducts       map[uuid.UUID][]any
	projectRuntimeNodes              map[uuid.UUID][]ProjectRuntimeNode
	projectEmployeeNodeAffinity      map[uuid.UUID]map[uuid.UUID]ProjectEmployeeNodeAffinity
	deleteBlockers                   []ProjectDeleteBlocker
	deletePreviewCounts              ProjectDeleteWarnings
	deleteCascadeResult              ProjectDeleteCascadeResult
	deleteAuditEvents                []ProjectDeleteAuditEventParams
	deleteAuditEventErr              error
	ensureDecisionCardsTerminalCalls []ensureDecisionCardsTerminalCall
	workspaceDeleteRequests         []WorkspaceDeleteRequest
	workspaceDeleteAuditEvents      []map[string]any
	// openInboxDecisionIDs tracks which decision IDs currently have an open
	// inbox projection in memory tests (shared with fakeDecisionInboxProjector).
	openInboxDecisionIDs map[uuid.UUID]bool
}

type ensureDecisionCardsTerminalCall struct {
	Decision   DecisionRequest
	ResolvedBy uuid.UUID
	Comment    string
}

func (r *memoryRepository) CreateDemandConstraintExemption(ctx context.Context, req CreateDemandConstraintExemptionRequest) error {
	for _, existing := range r.demandConstraintExemptions {
		if existing.TenantID == req.TenantID && existing.DemandID == req.DemandID && existing.ConstraintKind == req.ConstraintKind {
			return nil // idempotent: mirrors the UNIQUE(tenant_id, demand_id, constraint_kind) ON CONFLICT DO NOTHING
		}
	}
	r.demandConstraintExemptions = append(r.demandConstraintExemptions, DemandConstraintExemption{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		DemandID:          req.DemandID,
		ConstraintKind:    req.ConstraintKind,
		Roles:             req.Roles,
		GrantedByUserID:   req.GrantedByUserID,
		DecisionRequestID: req.DecisionRequestID,
		CreatedAt:         time.Now().UTC(),
	})
	return nil
}

func (r *memoryRepository) ListDemandConstraintExemptions(ctx context.Context, tenantID, demandID uuid.UUID) ([]DemandConstraintExemption, error) {
	result := make([]DemandConstraintExemption, 0)
	for _, exemption := range r.demandConstraintExemptions {
		if exemption.TenantID == tenantID && exemption.DemandID == demandID {
			result = append(result, exemption)
		}
	}
	return result, nil
}

func (r *memoryRepository) CreateDemandAcceptanceCriteria(ctx context.Context, reqs []CreateDemandAcceptanceCriterionRequest) error {
	for _, req := range reqs {
		exists := false
		for _, existing := range r.demandAcceptanceCriteria {
			if existing.TenantID == req.TenantID && existing.DemandID == req.DemandID &&
				existing.PlanRevisionID == req.PlanRevisionID && existing.CriterionID == req.CriterionID {
				exists = true
				break
			}
		}
		if exists {
			continue // idempotent: mirrors UNIQUE(tenant_id, demand_id, plan_revision_id, criterion_id) ON CONFLICT DO NOTHING
		}
		r.demandAcceptanceCriteria = append(r.demandAcceptanceCriteria, DemandAcceptanceCriterion{
			ID:                 uuid.New(),
			TenantID:           req.TenantID,
			ProjectID:          req.ProjectID,
			DemandID:           req.DemandID,
			PlanRevisionID:     req.PlanRevisionID,
			CriterionID:        req.CriterionID,
			Statement:          req.Statement,
			VerificationMethod: req.VerificationMethod,
			Severity:           req.Severity,
			SatisfiedBy:        append([]string(nil), req.SatisfiedBy...),
			CreatedAt:          time.Now().UTC(),
		})
	}
	return nil
}

func (r *memoryRepository) ListDemandAcceptanceCriteria(ctx context.Context, tenantID, demandID, planRevisionID uuid.UUID) ([]DemandAcceptanceCriterion, error) {
	result := make([]DemandAcceptanceCriterion, 0)
	for _, criterion := range r.demandAcceptanceCriteria {
		if criterion.TenantID == tenantID && criterion.DemandID == demandID && criterion.PlanRevisionID == planRevisionID {
			result = append(result, criterion)
		}
	}
	return result, nil
}

// CreateDemandCriterionVerdict mirrors PgRepository's partial-unique ON
// CONFLICT DO NOTHING idempotency for the executor-projection path
// (project_task_id set): a repeat write for the same
// tenant/demand/plan_revision/criterion/task is a no-op, not a duplicate row.
func (r *memoryRepository) CreateDemandCriterionVerdict(ctx context.Context, req CreateDemandCriterionVerdictRequest) error {
	if r.createDemandCriterionVerdictErr != nil {
		return r.createDemandCriterionVerdictErr
	}
	if req.ProjectTaskID != nil {
		for _, existing := range r.demandCriterionVerdicts {
			if existing.TenantID == req.TenantID && existing.DemandID == req.DemandID &&
				existing.PlanRevisionID == req.PlanRevisionID && existing.CriterionID == req.CriterionID &&
				existing.ProjectTaskID != nil && *existing.ProjectTaskID == *req.ProjectTaskID {
				return nil
			}
		}
	}
	r.demandCriterionVerdicts = append(r.demandCriterionVerdicts, DemandCriterionVerdict{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		ProjectID:      req.ProjectID,
		DemandID:       req.DemandID,
		PlanRevisionID: req.PlanRevisionID,
		CriterionID:    req.CriterionID,
		Verdict:        req.Verdict,
		JudgeType:      req.JudgeType,
		JudgeID:        req.JudgeID,
		Reason:         req.Reason,
		EvidenceRefs:   append([]string(nil), req.EvidenceRefs...),
		ProjectTaskID:  req.ProjectTaskID,
		CreatedAt:      time.Now().UTC(),
	})
	return nil
}

func (r *memoryRepository) CreateAdversarialVerdict(ctx context.Context, req CreateAdversarialVerdictRequest) error {
	for i, existing := range r.demandCriterionVerdicts {
		if existing.TenantID == req.TenantID && existing.DemandID == req.DemandID &&
			existing.PlanRevisionID == req.PlanRevisionID && existing.CriterionID == req.CriterionID &&
			existing.ProjectTaskID == nil && existing.JudgeType == "adversarial" {
			r.demandCriterionVerdicts[i].Verdict = req.Verdict
			r.demandCriterionVerdicts[i].Reason = req.Reason
			r.demandCriterionVerdicts[i].EvidenceRefs = append([]string(nil), req.EvidenceRefs...)
			return nil
		}
	}
	r.demandCriterionVerdicts = append(r.demandCriterionVerdicts, DemandCriterionVerdict{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		ProjectID:      req.ProjectID,
		DemandID:       req.DemandID,
		PlanRevisionID: req.PlanRevisionID,
		CriterionID:    req.CriterionID,
		Verdict:        req.Verdict,
		JudgeType:      "adversarial",
		JudgeID:        req.JudgeID,
		Reason:         req.Reason,
		EvidenceRefs:   append([]string(nil), req.EvidenceRefs...),
		ProjectTaskID:  nil,
		CreatedAt:      time.Now().UTC(),
	})
	return nil
}

func (r *memoryRepository) CreateReviewGateVerdict(ctx context.Context, req CreateReviewGateVerdictRequest) error {
	for i, existing := range r.demandCriterionVerdicts {
		if existing.TenantID == req.TenantID && existing.DemandID == req.DemandID &&
			existing.PlanRevisionID == req.PlanRevisionID && existing.CriterionID == req.CriterionID &&
			existing.ProjectTaskID == nil && existing.JudgeType == "review_gate" {
			r.demandCriterionVerdicts[i].Verdict = req.Verdict
			r.demandCriterionVerdicts[i].Reason = req.Reason
			r.demandCriterionVerdicts[i].EvidenceRefs = append([]string(nil), req.EvidenceRefs...)
			return nil
		}
	}
	r.demandCriterionVerdicts = append(r.demandCriterionVerdicts, DemandCriterionVerdict{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		ProjectID:      req.ProjectID,
		DemandID:       req.DemandID,
		PlanRevisionID: req.PlanRevisionID,
		CriterionID:    req.CriterionID,
		Verdict:        req.Verdict,
		JudgeType:      "review_gate",
		JudgeID:        req.JudgeID,
		Reason:         req.Reason,
		EvidenceRefs:   append([]string(nil), req.EvidenceRefs...),
		ProjectTaskID:  nil,
		CreatedAt:      time.Now().UTC(),
	})
	return nil
}

func (r *memoryRepository) CreateAdversarialJudgements(ctx context.Context, reqs []CreateAdversarialJudgementRequest) error {
	return nil
}

// ListAdversarialJudgements: memoryRepository's CreateAdversarialJudgements is
// a no-op stub (no test in this package currently exercises the per-lens
// write path), so there is nothing to read back here. Real read-back is
// covered by TestListAdversarialJudgementsReadsBack against PgRepository.
func (r *memoryRepository) ListAdversarialJudgements(ctx context.Context, tenantID, demandID, planRevisionID uuid.UUID) ([]DemandAdversarialJudgement, error) {
	return nil, nil
}

func (r *memoryRepository) ListDemandCriterionVerdicts(ctx context.Context, tenantID, demandID, planRevisionID uuid.UUID) ([]DemandCriterionVerdict, error) {
	result := make([]DemandCriterionVerdict, 0)
	for _, verdict := range r.demandCriterionVerdicts {
		if verdict.TenantID == tenantID && verdict.DemandID == demandID && verdict.PlanRevisionID == planRevisionID {
			result = append(result, verdict)
		}
	}
	return result, nil
}

func (r *memoryRepository) GetProjectEmployeeNodeAffinity(ctx context.Context, tenantID, projectID, digitalEmployeeID uuid.UUID) (ProjectEmployeeNodeAffinity, error) {
	if byEmployee, ok := r.projectEmployeeNodeAffinity[projectID]; ok {
		if affinity, ok := byEmployee[digitalEmployeeID]; ok {
			return affinity, nil
		}
	}
	return ProjectEmployeeNodeAffinity{}, ErrProjectNotFound
}

func (r *memoryRepository) UpsertProjectEmployeeNodeAffinity(ctx context.Context, tenantID, projectID, digitalEmployeeID, runtimeNodeID uuid.UUID) (ProjectEmployeeNodeAffinity, error) {
	if r.projectEmployeeNodeAffinity == nil {
		r.projectEmployeeNodeAffinity = map[uuid.UUID]map[uuid.UUID]ProjectEmployeeNodeAffinity{}
	}
	if r.projectEmployeeNodeAffinity[projectID] == nil {
		r.projectEmployeeNodeAffinity[projectID] = map[uuid.UUID]ProjectEmployeeNodeAffinity{}
	}
	now := time.Now().UTC()
	existing, ok := r.projectEmployeeNodeAffinity[projectID][digitalEmployeeID]
	affinity := ProjectEmployeeNodeAffinity{
		ID:                existing.ID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: digitalEmployeeID,
		RuntimeNodeID:     runtimeNodeID,
		LastRunAt:         &now,
		CreatedAt:         existing.CreatedAt,
		UpdatedAt:         now,
	}
	if !ok {
		affinity.ID = uuid.New()
		affinity.CreatedAt = now
	}
	r.projectEmployeeNodeAffinity[projectID][digitalEmployeeID] = affinity
	return affinity, nil
}

type projectTaskResultMemoryRepository struct {
	*memoryRepository
	recordProjectTaskResultErr              error
	linkProjectTaskLatestResultErr          error
	linkProjectTaskLatestResultErrAfter     int
	linkProjectTaskLatestResultCalls        int
	linkProjectTaskResultDecisionRequestErr error
}

type repositoryWithoutProjectTeamScopeAuthorizer struct {
	Repository
}

type taskGraphLimitRepository struct {
	*memoryRepository
	calls   int
	lastReq GetProjectTaskGraphRequest
	graph   ProjectTaskGraph
}

type fakeDigitalEmployeeIdentityLookup struct {
	identities map[uuid.UUID]DigitalEmployeeIdentity
	err        map[uuid.UUID]error
	calls      []uuid.UUID
}

type fakeProjectPlanningProfileSource struct {
	records map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord
	err     error
}

type fakeProjectRuntimeNodeReader struct {
	nodes        []runtimepkg.NodeRecord
	capabilities map[uuid.UUID][]runtimepkg.RuntimeCapability
	connected    map[string]bool
}

type workflowInstanceServiceRepository struct {
	*memoryRepository
	calls   int
	lastReq ListWorkflowInstancesRequest
	items   []WorkflowInstanceSummary
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		projects:                   map[uuid.UUID]Project{},
		consumedTokens:             map[uuid.UUID]int64{},
		members:                    map[uuid.UUID][]ProjectMember{},
		projectTeamScopes:          map[uuid.UUID]map[uuid.UUID]map[uuid.UUID]bool{},
		projectTaskRunRuntimeNodes: map[uuid.UUID]uuid.UUID{},
		projectTaskRunWorkProducts: map[uuid.UUID][]any{},
	}
}

func newProjectTaskResultMemoryRepository() *projectTaskResultMemoryRepository {
	return &projectTaskResultMemoryRepository{memoryRepository: newMemoryRepository()}
}

func ptrUUIDValue(id uuid.UUID) *uuid.UUID {
	return &id
}

func (r *memoryRepository) authorizeProjectTeamScope(tenantID, userID, teamID uuid.UUID) {
	if r.projectTeamScopes == nil {
		r.projectTeamScopes = map[uuid.UUID]map[uuid.UUID]map[uuid.UUID]bool{}
	}
	if r.projectTeamScopes[tenantID] == nil {
		r.projectTeamScopes[tenantID] = map[uuid.UUID]map[uuid.UUID]bool{}
	}
	if r.projectTeamScopes[tenantID][userID] == nil {
		r.projectTeamScopes[tenantID][userID] = map[uuid.UUID]bool{}
	}
	r.projectTeamScopes[tenantID][userID][teamID] = true
}

func (r *memoryRepository) CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error) {
	return r.projectTeamScopes[tenantID][userID][teamID], nil
}

func assertNoCreateProjectSideEffects(t *testing.T, repo *memoryRepository, coordinator *fakeCoordinatorSignalClient) {
	t.Helper()
	if len(repo.projects) != 0 || len(repo.members) != 0 || len(repo.events) != 0 || len(repo.eventTypes) != 0 || coordinator.ensureSignals != 0 {
		t.Fatalf("expected rejection before side effects, projects=%d members=%d events=%d eventTypes=%#v ensureSignals=%d", len(repo.projects), len(repo.members), len(repo.events), repo.eventTypes, coordinator.ensureSignals)
	}
}

func (r *taskGraphLimitRepository) GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (ProjectTaskGraph, error) {
	r.calls++
	r.lastReq = req
	if r.graph.Nodes != nil {
		return r.graph, nil
	}
	count := 55
	if req.Limit > 0 && int(req.Limit) < count {
		count = int(req.Limit)
	}
	nodes := make([]ProjectTaskGraphNode, 0, count)
	for i := 0; i < count; i++ {
		nodes = append(nodes, ProjectTaskGraphNode{
			Task: ProjectTask{
				ID:        uuid.New(),
				TenantID:  req.TenantID,
				ProjectID: req.ProjectID,
				Title:     fmt.Sprintf("graph task %02d", i+1),
				Status:    "planned",
			},
		})
	}
	return ProjectTaskGraph{
		Nodes:              nodes,
		Edges:              []ProjectTaskGraphEdge{},
		Employees:          []ProjectTaskGraphEmployee{},
		Runs:               []ProjectTaskGraphRun{},
		ExecutionSummaries: []ExecutionSummary{},
		RecentEvents:       []ProjectEvent{},
		DecisionRequests:   []DecisionRequest{},
		StageSummaries:     []ProjectTaskGraphStageSummary{},
	}, nil
}

func (f *fakeDigitalEmployeeIdentityLookup) GetDigitalEmployeeIdentity(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeIdentity, error) {
	f.calls = append(f.calls, digitalEmployeeID)
	if err, ok := f.err[digitalEmployeeID]; ok {
		return DigitalEmployeeIdentity{}, err
	}
	return f.identities[digitalEmployeeID], nil
}

func (f *fakeProjectPlanningProfileSource) PlanningProfileRecords(ctx context.Context, tenantID, projectID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{}
	for _, employeeID := range employeeIDs {
		if record, ok := f.records[employeeID]; ok {
			out[employeeID] = record
		}
	}
	return out, nil
}

func (r *fakeProjectRuntimeNodeReader) ListRuntimeNodesForTenant(ctx context.Context, params runtimepkg.ListRuntimeNodesForTenantParams) ([]runtimepkg.NodeRecord, error) {
	out := make([]runtimepkg.NodeRecord, 0, len(r.nodes))
	for _, node := range r.nodes {
		if node.TenantID != params.TenantID {
			continue
		}
		if params.Status.Valid && node.Status != params.Status.String {
			continue
		}
		out = append(out, node)
	}
	return out, nil
}

func (r *fakeProjectRuntimeNodeReader) ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]runtimepkg.RuntimeCapability, error) {
	for _, node := range r.nodes {
		if node.TenantID == tenantID && node.NodeID == nodeID {
			return append([]runtimepkg.RuntimeCapability(nil), r.capabilities[node.ID]...), nil
		}
	}
	return nil, nil
}

func (r *fakeProjectRuntimeNodeReader) IsConnected(nodeID string) bool {
	return r.connected[nodeID]
}

// stubProjectRuntimeNodeReader registers the given runtime node ids as
// belonging to tenantID, so CreateProject's requireRuntimeNodeForTenant
// eligibility check accepts them.
func stubProjectRuntimeNodeReader(service *Service, tenantID uuid.UUID, runtimeNodeIDs ...uuid.UUID) {
	nodes := make([]runtimepkg.NodeRecord, 0, len(runtimeNodeIDs))
	for _, id := range runtimeNodeIDs {
		nodes = append(nodes, runtimepkg.NodeRecord{ID: id, TenantID: tenantID})
	}
	service.SetProjectRuntimeNodeReader(&fakeProjectRuntimeNodeReader{nodes: nodes})
}

func readinessBlockingCodes(reasons []ProjectReadinessReason) []string {
	codes := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		codes = append(codes, reason.Code)
	}
	return codes
}

func readinessActionCodes(actions []ProjectReadinessAction) []string {
	codes := make([]string, 0, len(actions))
	for _, action := range actions {
		codes = append(codes, action.Code)
	}
	return codes
}

func (r *workflowInstanceServiceRepository) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	r.calls++
	r.lastReq = req
	return append([]WorkflowInstanceSummary(nil), r.items...), nil
}

func (r *memoryRepository) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	return []WorkflowInstanceSummary{}, nil
}

func cloneProjects(projects map[uuid.UUID]Project) map[uuid.UUID]Project {
	cloned := make(map[uuid.UUID]Project, len(projects))
	for id, project := range projects {
		cloned[id] = project
	}
	return cloned
}

func strPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func nonEmptyString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func paginateTestSlice[T any](items []T, limit, offset int32) []T {
	start := int(offset)
	if start > len(items) {
		return []T{}
	}
	end := start + int(limit)
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func seedHumanOwnerMember(repo *memoryRepository, tenantID, projectID, ownerID uuid.UUID) {
	repo.members[projectID] = append(repo.members[projectID], ProjectMember{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   ownerID,
		ProjectRole:   ProjectRoleOwner,
		Status:        "active",
	})
}

// seedDigitalExecutorMember adds an active digital employee executor so
// SubmitDemand's hard gate (project requires ≥1 digital employee) passes.
func seedDigitalExecutorMember(repo *memoryRepository, tenantID, projectID, employeeID uuid.UUID) {
	if employeeID == uuid.Nil {
		employeeID = uuid.New()
	}
	repo.members[projectID] = append(repo.members[projectID], ProjectMember{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeDigitalEmployee,
		PrincipalID:   employeeID,
		ProjectRole:   ProjectRoleExecutor,
		Status:        "active",
	})
}

func bindTaskToRuntimeRun(repo *memoryRepository, taskIndex int, runtimeNodeID uuid.UUID) uuid.UUID {
	runID := uuid.New()
	repo.tasks[taskIndex].DigitalEmployeeRunID = &runID
	repo.projectTaskRunRuntimeNodes[repo.tasks[taskIndex].ID] = runtimeNodeID
	return runID
}

func (r *memoryRepository) CreateProject(ctx context.Context, req CreateProjectRequest, projectID uuid.UUID, workflowID string) (Project, error) {
	ready := req.WorkspaceReadyStatus
	if ready == "" {
		ready = WorkspaceReadyStatusReady
	}
	dirName := strings.TrimSpace(req.DirectoryName)
	if dirName == "" {
		dirName = req.Name
	}
	project := Project{
		ID:                     projectID,
		TenantID:               req.TenantID,
		TeamID:                 req.TeamID,
		Name:                   req.Name,
		DirectoryName:          dirName,
		Description:            strPtrOrNil(req.Description),
		Goal:                   req.Goal,
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       req.HumanOwnerUserID,
		CoordinationWorkflowID: workflowID,
		CoordinationStatus:     "registered",
		CoordinationPolicy:     req.CoordinationPolicy,
		RepoBinding:            repoBindingFromInput(req.RepoBinding),
		ScenarioTemplateKey:    req.ScenarioTemplateKey,
		WorkspaceReadyStatus:   ready,
		// 与 pg_repository 对齐：ownership 是持久字段，fake 丢掉它会让
		// attach 相关回归静默通过。
		WorkspaceOwnership: workspaceOwnershipFromRecord(string(req.WorkspaceOwnership)),
	}
	if ready == WorkspaceReadyStatusReady {
		now := time.Now().UTC()
		project.WorkspaceReadyAt = &now
	}
	r.projects[project.ID] = project
	return project, nil
}

func (r *memoryRepository) SetProjectWorkspaceReady(_ context.Context, tenantID, projectID uuid.UUID, status WorkspaceReadyStatus, primaryNodeID *uuid.UUID, readyError *string) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	switch status {
	case WorkspaceReadyStatusReady:
		if project.WorkspaceReadyStatus != WorkspaceReadyStatusPending &&
			project.WorkspaceReadyStatus != WorkspaceReadyStatusError &&
			project.WorkspaceReadyStatus != WorkspaceReadyStatusReady &&
			project.WorkspaceReadyStatus != "" {
			return Project{}, ErrProjectNotFound
		}
	case WorkspaceReadyStatusError:
		if project.WorkspaceReadyStatus != WorkspaceReadyStatusPending && project.WorkspaceReadyStatus != "" {
			return Project{}, ErrProjectNotFound
		}
	case WorkspaceReadyStatusPending:
		if project.WorkspaceReadyStatus != WorkspaceReadyStatusPending &&
			project.WorkspaceReadyStatus != WorkspaceReadyStatusError &&
			project.WorkspaceReadyStatus != WorkspaceReadyStatusReady &&
			project.WorkspaceReadyStatus != "" {
			return Project{}, ErrProjectNotFound
		}
	}
	project.WorkspaceReadyStatus = status
	project.PrimaryRuntimeNodeID = primaryNodeID
	project.WorkspaceReadyError = readyError
	if status == WorkspaceReadyStatusReady && project.WorkspaceReadyAt == nil {
		now := time.Now().UTC()
		project.WorkspaceReadyAt = &now
	}
	if status == WorkspaceReadyStatusError || status == WorkspaceReadyStatusPending {
		// keep WorkspaceReadyAt as-is for pending/error transitions
	}
	r.projects[projectID] = project
	return project, nil
}

func (r *memoryRepository) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	return project, nil
}

func (r *memoryRepository) SumProjectConsumedTokens(ctx context.Context, tenantID, projectID uuid.UUID) (int64, error) {
	return r.consumedTokens[projectID], nil
}

func (r *memoryRepository) SetProjectBudgetTokenLimit(ctx context.Context, tenantID, projectID uuid.UUID, limit *int64) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	project.BudgetTokenLimit = limit
	r.projects[projectID] = project
	return project, nil
}

func (r *memoryRepository) GetProjectForDelete(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID || project.DeletedAt != nil {
		return Project{}, ErrProjectNotFound
	}
	return project, nil
}

func (r *memoryRepository) ListProjectDeleteBlockers(_ context.Context, _, _ uuid.UUID) ([]ProjectDeleteBlocker, error) {
	return append([]ProjectDeleteBlocker(nil), r.deleteBlockers...), nil
}

func (r *memoryRepository) GetProjectDeletePreviewCounts(_ context.Context, _, _ uuid.UUID) (ProjectDeleteWarnings, error) {
	return r.deletePreviewCounts, nil
}

func (r *memoryRepository) SoftDeleteProjectCascade(_ context.Context, params SoftDeleteProjectCascadeParams) (ProjectDeleteCascadeResult, error) {
	project, ok := r.projects[params.ProjectID]
	if !ok || project.TenantID != params.TenantID || project.DeletedAt != nil {
		return ProjectDeleteCascadeResult{}, ErrProjectNotFound
	}
	if params.ActorUserID != uuid.Nil {
		if err := r.deleteAuditEventErr; err != nil {
			return ProjectDeleteCascadeResult{}, err
		}
	}
	deletedAt := params.DeletedAt.UTC()
	project.DeletedAt = &deletedAt
	project.CoordinationStatus = "terminated"
	r.projects[params.ProjectID] = project
	cascade := r.deleteCascadeResult
	if params.ActorUserID != uuid.Nil {
		r.deleteAuditEvents = append(r.deleteAuditEvents, ProjectDeleteAuditEventParams{
			TenantID:      params.TenantID,
			ActorUserID:   params.ActorUserID,
			Project:       params.Project,
			CascadeResult: cascade,
			DeletedAt:     params.DeletedAt,
		})
	}
	return cascade, nil
}

func (r *memoryRepository) EnqueueWorkspaceDeleteRequest(_ context.Context, params EnqueueWorkspaceDeleteRequestParams) (WorkspaceDeleteRequest, error) {
	for _, existing := range r.workspaceDeleteRequests {
		if existing.ProjectID == params.ProjectID && existing.RuntimeNodeID == params.RuntimeNodeID && existing.Status == WorkspaceDeleteRequestStatusPending {
			return existing, nil
		}
	}
	ownership := params.Ownership
	if ownership == "" {
		ownership = WorkspaceOwnershipPlatformManaged
	}
	item := WorkspaceDeleteRequest{
		ID:             uuid.New(),
		TenantID:       params.TenantID,
		ProjectID:      params.ProjectID,
		RuntimeNodeID:  params.RuntimeNodeID,
		DirectoryName:  params.DirectoryName,
		NodeIDSnapshot: params.NodeIDSnapshot,
		Ownership:      ownership,
		RepoSummary:    params.RepoSummary,
		Status:         WorkspaceDeleteRequestStatusPending,
		RequestedBy:    params.RequestedBy,
		RequestedAt:    time.Now().UTC(),
	}
	if strings.TrimSpace(params.Reason) != "" {
		reason := strings.TrimSpace(params.Reason)
		item.Reason = &reason
	}
	r.workspaceDeleteRequests = append(r.workspaceDeleteRequests, item)
	return item, nil
}

func (r *memoryRepository) GetWorkspaceDeleteRequest(_ context.Context, tenantID, requestID uuid.UUID) (WorkspaceDeleteRequest, error) {
	for _, item := range r.workspaceDeleteRequests {
		if item.TenantID == tenantID && item.ID == requestID {
			return item, nil
		}
	}
	return WorkspaceDeleteRequest{}, ErrProjectWorkspaceDeleteRequestNotFound
}

func (r *memoryRepository) ListPendingWorkspaceDeleteRequests(_ context.Context, tenantID uuid.UUID) ([]WorkspaceDeleteRequest, error) {
	out := make([]WorkspaceDeleteRequest, 0)
	for _, item := range r.workspaceDeleteRequests {
		if item.TenantID == tenantID && item.Status == WorkspaceDeleteRequestStatusPending {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *memoryRepository) ListStalePendingWorkspaceDeleteRequests(_ context.Context, staleBefore time.Time) ([]WorkspaceDeleteRequest, error) {
	out := make([]WorkspaceDeleteRequest, 0)
	for _, item := range r.workspaceDeleteRequests {
		if item.Status == WorkspaceDeleteRequestStatusPending && item.RequestedAt.Before(staleBefore) {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *memoryRepository) CountPendingWorkspaceDeleteByDirectoryName(_ context.Context, directoryName string) (int32, error) {
	var count int32
	for _, item := range r.workspaceDeleteRequests {
		if item.DirectoryName == directoryName && item.Status == WorkspaceDeleteRequestStatusPending {
			count++
		}
	}
	return count, nil
}

func (r *memoryRepository) ConfirmWorkspaceDeleteRequest(_ context.Context, tenantID, requestID, actorUserID uuid.UUID) (WorkspaceDeleteRequest, error) {
	for i, item := range r.workspaceDeleteRequests {
		if item.TenantID == tenantID && item.ID == requestID && item.Status == WorkspaceDeleteRequestStatusPending {
			now := time.Now().UTC()
			item.Status = WorkspaceDeleteRequestStatusConfirmed
			item.ResolvedBy = &actorUserID
			item.ResolvedAt = &now
			r.workspaceDeleteRequests[i] = item
			return item, nil
		}
	}
	return WorkspaceDeleteRequest{}, ErrProjectWorkspaceDeleteRequestNotFound
}

func (r *memoryRepository) RejectWorkspaceDeleteRequest(_ context.Context, tenantID, requestID, actorUserID uuid.UUID, reason string) (WorkspaceDeleteRequest, error) {
	for i, item := range r.workspaceDeleteRequests {
		if item.TenantID == tenantID && item.ID == requestID && item.Status == WorkspaceDeleteRequestStatusPending {
			now := time.Now().UTC()
			item.Status = WorkspaceDeleteRequestStatusRejected
			item.ResolvedBy = &actorUserID
			item.ResolvedAt = &now
			if strings.TrimSpace(reason) != "" {
				r := strings.TrimSpace(reason)
				item.Reason = &r
			}
			r.workspaceDeleteRequests[i] = item
			return item, nil
		}
	}
	return WorkspaceDeleteRequest{}, ErrProjectWorkspaceDeleteRequestNotFound
}

func (r *memoryRepository) ResolveOrphanWorkspaceDeleteReminders(_ context.Context) error {
	return nil
}

func (r *memoryRepository) CreateWorkspaceDeleteAuditEvent(_ context.Context, tenantID, actorUserID uuid.UUID, action string, request WorkspaceDeleteRequest, extra map[string]any) error {
	payload := map[string]any{
		"tenant_id": tenantID.String(),
		"actor_id":  actorUserID.String(),
		"action":    action,
		"request":   request.ID.String(),
	}
	for k, v := range extra {
		payload[k] = v
	}
	r.workspaceDeleteAuditEvents = append(r.workspaceDeleteAuditEvents, payload)
	return nil
}

func (r *memoryRepository) ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error) {
	r.lastListProjects = req
	projects := make([]Project, 0, len(r.projects))
	for _, project := range r.projects {
		if project.TenantID != req.TenantID {
			continue
		}
		if req.Status != nil && project.Status != *req.Status {
			continue
		}
		if req.Query != "" && !strings.Contains(project.Name, req.Query) && !strings.Contains(project.Goal, req.Query) {
			continue
		}
		projects = append(projects, project)
	}
	return projects, nil
}

func (r *memoryRepository) ListProjectRunSummaries(ctx context.Context, req ListProjectRunSummariesRequest) ([]ProjectRunSummary, error) {
	r.lastListRunSummariesReq = req
	return r.runSummaries, nil
}

func (r *memoryRepository) CountTaskRunsCompletedToday(ctx context.Context, tenantID uuid.UUID) (int32, error) {
	return r.completedTodayCount, nil
}

func (r *memoryRepository) UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (Project, error) {
	project, ok := r.projects[req.ProjectID]
	if !ok || project.TenantID != req.TenantID {
		return Project{}, ErrProjectNotFound
	}
	if req.Name != "" {
		project.Name = req.Name
	}
	if req.Description != "" {
		project.Description = strPtrOrNil(req.Description)
	}
	if req.Goal != "" {
		project.Goal = req.Goal
	}
	if req.HumanOwnerUserID != uuid.Nil {
		project.HumanOwnerUserID = req.HumanOwnerUserID
	}
	if req.CoordinationPolicy != nil {
		project.CoordinationPolicy = req.CoordinationPolicy
	}
	if req.RepoBinding != nil {
		project.RepoBinding = repoBindingFromInput(req.RepoBinding)
	}
	r.projects[project.ID] = project
	return project, nil
}

func repoBindingFromInput(input *ProjectRepoBindingInput) ProjectRepoBinding {
	if input == nil {
		return unboundProjectRepoBinding()
	}
	if strings.TrimSpace(input.URL) == "" && strings.TrimSpace(input.DefaultBranch) == "" && trimmedStringPtr(input.GitCredentialRef) == nil && len(normalizeProjectRepoBindingScope(input.Scope)) == 0 {
		return unboundProjectRepoBinding()
	}
	return ProjectRepoBinding{
		Status:           ProjectRepoBindingStatusBound,
		URL:              input.URL,
		DefaultBranch:    input.DefaultBranch,
		GitCredentialRef: input.GitCredentialRef,
		Scope:            append([]string(nil), input.Scope...),
	}
}

func (r *memoryRepository) ArchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	if r.archiveProjectErr != nil {
		return Project{}, r.archiveProjectErr
	}
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	now := time.Now()
	project.Status = ProjectStatusArchived
	project.ArchivedAt = &now
	r.projects[projectID] = project
	return project, nil
}

func (r *memoryRepository) UnarchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	if project.Status != ProjectStatusArchived && project.ArchivedAt == nil {
		return Project{}, ErrProjectNotFound
	}
	project.Status = ProjectStatusRunning
	project.ArchivedAt = nil
	r.projects[projectID] = project
	return project, nil
}

func (r *memoryRepository) TransitionProjectStatus(ctx context.Context, tenantID, projectID uuid.UUID, fromStatuses []string, toStatus string) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	for _, from := range fromStatuses {
		if project.Status == ProjectStatus(from) {
			project.Status = ProjectStatus(toStatus)
			r.projects[projectID] = project
			return project, nil
		}
	}
	return Project{}, ErrProjectNotFound
}

func (r *memoryRepository) AreAllProjectDemandsTerminal(ctx context.Context, tenantID, projectID uuid.UUID) (bool, error) {
	nonTerminal, err := r.CountNonTerminalProjectDemands(ctx, tenantID, projectID)
	if err != nil {
		return false, err
	}
	if nonTerminal > 0 {
		return false, nil
	}
	count := 0
	for _, demand := range r.demands {
		if demand.TenantID == tenantID && demand.ProjectID == projectID {
			count++
		}
	}
	return count > 0, nil
}

func (r *memoryRepository) CountNonTerminalProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID) (int64, error) {
	terminal := map[ProjectDemandStatus]bool{
		ProjectDemandStatusCompleted: true,
		ProjectDemandStatusFailed:    true,
		ProjectDemandStatusCancelled: true,
	}
	var nonTerminal int64
	for _, demand := range r.demands {
		if demand.TenantID == tenantID && demand.ProjectID == projectID && !terminal[demand.Status] {
			nonTerminal++
		}
	}
	return nonTerminal, nil
}

func (r *memoryRepository) SetProjectHumanOwners(ctx context.Context, tenantID, projectID uuid.UUID, ownerIDs []uuid.UUID) error {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return ErrProjectNotFound
	}
	if len(ownerIDs) == 0 {
		return ErrProjectRequiresHumanOwner
	}
	project.HumanOwnerUserIDs = ownerIDs
	project.HumanOwnerUserID = ownerIDs[0]
	r.projects[projectID] = project
	return nil
}

func (r *memoryRepository) ReplaceProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return nil, ErrProjectNotFound
	}
	mapped := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		mapped = append(mapped, ProjectMember{
			ID:                  uuid.New(),
			TenantID:            tenantID,
			ProjectID:           projectID,
			PrincipalType:       member.PrincipalType,
			PrincipalID:         member.PrincipalID,
			ProjectRole:         member.ProjectRole,
			DisplayNameSnapshot: strPtrOrNil(member.DisplayNameSnapshot),
			Status:              "active",
			Settings:            member.Settings,
		})
	}
	r.members[projectID] = mapped
	return mapped, nil
}

func (r *memoryRepository) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error) {
	members := r.members[projectID]
	filtered := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		if member.TenantID == tenantID {
			filtered = append(filtered, member)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error) {
	r.lastTasksLimit = limit
	r.lastTasksOffset = offset
	filtered := make([]ProjectTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID && (status == nil || task.Status == *status) && task.DismissedAt == nil {
			filtered = append(filtered, task)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

// 与真实查询同口径：全量聚合、排除 dismissed、cancelled 属终态不计入 active。
// 刻意不复用 ListProjectTasks，否则会把"计数受分页窗口影响"的缺陷复制进夹具。
func (r *memoryRepository) GetProjectTaskStatusCounts(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectTaskSummary, error) {
	summary := ProjectTaskSummary{}
	for _, task := range r.tasks {
		if task.TenantID != tenantID || task.ProjectID != projectID || task.DismissedAt != nil {
			continue
		}
		summary.TotalTasks++
		if isActiveTasksGateStatus(task.Status) {
			summary.ActiveTasks++
		}
		switch ClassifyProjectTaskPortfolioBucket(task.Status, task.RequiresHumanApproval) {
		case PortfolioBucketPending:
			summary.PendingTasks++
		case PortfolioBucketQueued:
			summary.QueuedTasks++
		case PortfolioBucketRunning:
			summary.RunningTasks++
		case PortfolioBucketWaitingHuman:
			summary.PendingHumanTasks++
		case PortfolioBucketBlocked:
			summary.BlockedTasks++
		case PortfolioBucketFailed:
			summary.FailedTasks++
		case PortfolioBucketCompleted:
			summary.CompletedTasks++
		case PortfolioBucketCancelled:
			summary.CancelledTasks++
		case PortfolioBucketOther:
			summary.OtherTasks++
		}
	}
	return summary, nil
}

func (r *memoryRepository) GetProjectPortfolio(ctx context.Context, req GetProjectPortfolioRequest) (ProjectPortfolioResponse, error) {
	// Minimal in-memory implementation for service-layer unit tests.
	visible := make([]Project, 0)
	for _, p := range r.projects {
		if p.TenantID != req.TenantID {
			continue
		}
		if req.MineOnly {
			isOwner := p.HumanOwnerUserID == req.ActorUserID
			for _, id := range p.HumanOwnerUserIDs {
				if id == req.ActorUserID {
					isOwner = true
					break
				}
			}
			isMember := false
			for _, m := range r.members[p.ID] {
				if m.PrincipalType == PrincipalTypeHumanUser && m.PrincipalID == req.ActorUserID && m.Status == "active" {
					isMember = true
					break
				}
			}
			if !isOwner && !isMember {
				continue
			}
		}
		visible = append(visible, p)
	}

	statusCounts := map[string]int{
		"draft": 0, "configuring": 0, "running": 0,
		"paused": 0, "acceptance": 0, "archived": 0,
	}
	activeTasks := ProjectTaskPortfolioCounts{}
	for _, p := range visible {
		statusCounts[string(p.Status)]++
		if p.Status == ProjectStatusArchived {
			continue
		}
		for _, task := range r.tasks {
			if task.TenantID != req.TenantID || task.ProjectID != p.ID || task.DismissedAt != nil {
				continue
			}
			activeTasks.Total++
			switch ClassifyProjectTaskPortfolioBucket(task.Status, task.RequiresHumanApproval) {
			case PortfolioBucketPending:
				activeTasks.Pending++
			case PortfolioBucketQueued:
				activeTasks.Queued++
			case PortfolioBucketRunning:
				activeTasks.Running++
			case PortfolioBucketWaitingHuman:
				activeTasks.WaitingHuman++
			case PortfolioBucketBlocked:
				activeTasks.Blocked++
			case PortfolioBucketFailed:
				activeTasks.Failed++
			case PortfolioBucketCompleted:
				activeTasks.Completed++
			case PortfolioBucketCancelled:
				activeTasks.Cancelled++
			case PortfolioBucketOther:
				activeTasks.Other++
			}
		}
	}

	filtered := make([]Project, 0, len(visible))
	for _, p := range visible {
		if req.Query != "" && !strings.Contains(p.Name, req.Query) && !strings.Contains(p.Goal, req.Query) {
			continue
		}
		if len(req.ProjectStatuses) > 0 {
			ok := false
			for _, s := range req.ProjectStatuses {
				if string(p.Status) == s {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		if req.OwnerUserID != nil {
			match := p.HumanOwnerUserID == *req.OwnerUserID
			for _, id := range p.HumanOwnerUserIDs {
				if id == *req.OwnerUserID {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		if req.TaskState != "" {
			has := false
			for _, task := range r.tasks {
				if task.ProjectID != p.ID || task.DismissedAt != nil {
					continue
				}
				if string(ClassifyProjectTaskPortfolioBucket(task.Status, task.RequiresHumanApproval)) == req.TaskState {
					has = true
					break
				}
			}
			if !has {
				continue
			}
		}
		filtered = append(filtered, p)
	}

	total := int32(len(filtered))
	start := int(req.Offset)
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + int(req.Limit)
	if end > len(filtered) {
		end = len(filtered)
	}
	page := filtered[start:end]
	items := make([]ProjectPortfolioItem, 0, len(page))
	for _, p := range page {
		tc := ProjectTaskPortfolioCounts{}
		openDecisions := 0
		waitingUnlinked := 0
		for _, task := range r.tasks {
			if task.ProjectID != p.ID || task.DismissedAt != nil {
				continue
			}
			tc.Total++
			bucket := ClassifyProjectTaskPortfolioBucket(task.Status, task.RequiresHumanApproval)
			switch bucket {
			case PortfolioBucketPending:
				tc.Pending++
			case PortfolioBucketQueued:
				tc.Queued++
			case PortfolioBucketRunning:
				tc.Running++
			case PortfolioBucketWaitingHuman:
				tc.WaitingHuman++
				// orphan check
				linked := false
				for _, d := range r.decisionRequests {
					if d.ProjectTaskID != nil && *d.ProjectTaskID == task.ID {
						st := strings.ToLower(d.StatusSnapshot)
						if st == "pending" || st == "waiting" || st == "requested" || st == "open" {
							linked = true
							break
						}
					}
				}
				if !linked {
					waitingUnlinked++
				}
			case PortfolioBucketBlocked:
				tc.Blocked++
			case PortfolioBucketFailed:
				tc.Failed++
			case PortfolioBucketCompleted:
				tc.Completed++
			case PortfolioBucketCancelled:
				tc.Cancelled++
			case PortfolioBucketOther:
				tc.Other++
			}
		}
		for _, d := range r.decisionRequests {
			if d.ProjectID == p.ID {
				st := strings.ToLower(d.StatusSnapshot)
				if st == "pending" || st == "waiting" || st == "requested" || st == "open" {
					openDecisions++
				}
			}
		}
		item := ProjectPortfolioItem{
			Project: ProjectPortfolioProject{
				ID: p.ID, Name: p.Name, Goal: p.Goal, Status: p.Status,
				HumanOwnerUserID: p.HumanOwnerUserID, HumanOwnerUserIDs: p.HumanOwnerUserIDs,
				CoordinationStatus: p.CoordinationStatus, UpdatedAt: p.UpdatedAt,
			},
			TaskCounts: tc,
			Attention: ProjectPortfolioAttention{
				OpenDecisionCount:         openDecisions,
				WaitingHumanUnlinkedCount: waitingUnlinked,
				CoordinationAnomaly:       coordinationAnomaly(p.Status, p.CoordinationStatus, p.ArchivedAt != nil),
			},
		}
		if p.HumanOwnerUserID != uuid.Nil {
			item.Owner = &ProjectPortfolioOwner{ID: p.HumanOwnerUserID, DisplayName: ""}
		}
		items = append(items, item)
	}

	return ProjectPortfolioResponse{
		Summary: ProjectPortfolioSummary{
			TotalProjects:       len(visible),
			ProjectStatusCounts: statusCounts,
			ActiveTaskCounts:    activeTasks,
		},
		Items: items,
		Pagination: ProjectPortfolioPagination{
			Limit: req.Limit, Offset: req.Offset, Total: total,
			HasMore: int64(req.Offset)+int64(len(items)) < int64(total),
		},
	}, nil
}

func (r *memoryRepository) DismissProjectTask(ctx context.Context, tenantID, projectID, taskID, actorUserID uuid.UUID) (ProjectTask, error) {
	now := time.Now().UTC()
	for i := range r.tasks {
		task := &r.tasks[i]
		if task.TenantID != tenantID || task.ProjectID != projectID || task.ID != taskID {
			continue
		}
		if task.DismissedAt != nil {
			return *task, nil
		}
		if task.Status != "failed" && task.Status != "cancelled" {
			return ProjectTask{}, ErrProjectTaskNotDismissible
		}
		task.DismissedAt = &now
		task.DismissedBy = &actorUserID
		task.UpdatedAt = now
		return *task, nil
	}
	return ProjectTask{}, ErrProjectTaskNotDismissible
}

func (r *memoryRepository) AppendProjectEvent(ctx context.Context, event AppendProjectEventRequest) (ProjectEvent, error) {
	if r.appendProjectEventErr != nil {
		return ProjectEvent{}, r.appendProjectEventErr
	}
	projectEvent := ProjectEvent{
		ID:             uuid.New(),
		TenantID:       event.TenantID,
		ProjectID:      event.ProjectID,
		SequenceNumber: int64(len(r.events) + 1),
		EventType:      event.EventType,
		ActorType:      event.ActorType,
		ActorID:        event.ActorID,
		ResourceType:   event.ResourceType,
		ResourceID:     event.ResourceID,
		Summary:        strPtrOrNil(event.Summary),
		Payload:        event.Payload,
	}
	r.events = append(r.events, projectEvent)
	r.eventTypes = append(r.eventTypes, event.EventType)
	return projectEvent, nil
}

func (r *memoryRepository) InsertProjectRuntimeNode(ctx context.Context, tenantID, projectID, runtimeNodeID uuid.UUID, provisioned bool, provisionSource string) (ProjectRuntimeNode, error) {
	if r.projectRuntimeNodes == nil {
		r.projectRuntimeNodes = map[uuid.UUID][]ProjectRuntimeNode{}
	}
	for _, existing := range r.projectRuntimeNodes[projectID] {
		if existing.RuntimeNodeID == runtimeNodeID {
			return existing, nil
		}
	}
	status := ProvisionStatusUnprovisioned
	var provisionedAt *time.Time
	var source *string
	if provisioned {
		status = ProvisionStatusProvisioned
		now := time.Now().UTC()
		provisionedAt = &now
		s := provisionSource
		if s == "" {
			s = "create"
		}
		source = &s
	}
	node := ProjectRuntimeNode{
		ID:              uuid.New(),
		TenantID:        tenantID,
		ProjectID:       projectID,
		RuntimeNodeID:   runtimeNodeID,
		ProvisionStatus: status,
		ProvisionedAt:   provisionedAt,
		ProvisionSource: source,
		CreatedAt:       time.Now().UTC(),
	}
	r.projectRuntimeNodes[projectID] = append(r.projectRuntimeNodes[projectID], node)
	return node, nil
}

func (r *memoryRepository) MarkProjectRuntimeNodeProvisioned(ctx context.Context, tenantID, projectID, runtimeNodeID uuid.UUID, provisionSource string) (ProjectRuntimeNode, error) {
	if r.projectRuntimeNodes == nil {
		return ProjectRuntimeNode{}, ErrProjectNotFound
	}
	nodes := r.projectRuntimeNodes[projectID]
	for i, node := range nodes {
		if node.TenantID == tenantID && node.RuntimeNodeID == runtimeNodeID {
			now := time.Now().UTC()
			node.ProvisionStatus = ProvisionStatusProvisioned
			node.ProvisionedAt = &now
			s := provisionSource
			if s == "" {
				s = "confirm"
			}
			node.ProvisionSource = &s
			nodes[i] = node
			r.projectRuntimeNodes[projectID] = nodes
			return node, nil
		}
	}
	return ProjectRuntimeNode{}, ErrProjectNotFound
}

func (r *memoryRepository) ListProjectRuntimeNodes(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectRuntimeNode, error) {
	items := make([]ProjectRuntimeNode, 0, len(r.projectRuntimeNodes[projectID]))
	for _, node := range r.projectRuntimeNodes[projectID] {
		if node.TenantID == tenantID {
			items = append(items, node)
		}
	}
	return items, nil
}

func (r *memoryRepository) RemoveProjectRuntimeNode(ctx context.Context, tenantID, projectID, runtimeNodeID uuid.UUID) error {
	kept := make([]ProjectRuntimeNode, 0, len(r.projectRuntimeNodes[projectID]))
	for _, node := range r.projectRuntimeNodes[projectID] {
		if node.TenantID == tenantID && node.RuntimeNodeID == runtimeNodeID {
			continue
		}
		kept = append(kept, node)
	}
	r.projectRuntimeNodes[projectID] = kept
	return nil
}

func (r *memoryRepository) GetProjectEvent(ctx context.Context, tenantID, projectID, eventID uuid.UUID) (ProjectEvent, error) {
	for _, event := range r.events {
		if event.ID == eventID && event.TenantID == tenantID && event.ProjectID == projectID {
			return event, nil
		}
	}
	return ProjectEvent{}, ErrProjectNotFound
}

func (r *memoryRepository) GetProjectEventByTypeAndActor(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (ProjectEvent, error) {
	for i := len(r.events) - 1; i >= 0; i-- {
		event := r.events[i]
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == eventType && event.ActorID == actorID {
			return event, nil
		}
	}
	return ProjectEvent{}, ErrProjectNotFound
}

func (r *memoryRepository) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error) {
	r.lastEventsLimit = limit
	r.lastEventsOffset = offset
	filtered := make([]ProjectEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID {
			filtered = append(filtered, event)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].SequenceNumber > filtered[j].SequenceNumber
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) CreateProjectDemand(ctx context.Context, req SubmitProjectDemandRequest, status ProjectDemandStatus, createdEventID *uuid.UUID) (ProjectDemand, error) {
	demand := ProjectDemand{
		ID:                  uuid.New(),
		TenantID:            req.TenantID,
		ProjectID:           req.ProjectID,
		SubmittedByUserID:   req.SubmittedByUserID,
		Title:               req.Title,
		Content:             strPtrOrNil(req.Content),
		SourceType:          req.SourceType,
		SourceRefs:          req.SourceRefs,
		Attachments:         req.Attachments,
		ReviewerPreference:  reviewerPreferenceFromSourceRefs(req.SourceRefs),
		Status:              status,
		CreatedEventID:      createdEventID,
		CoordinationMode:    req.CoordinationMode,
		ScenarioTemplateKey: req.ScenarioTemplateKey,
	}
	r.demands = append(r.demands, demand)
	return demand, nil
}

func (r *memoryRepository) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error) {
	r.lastDemandsLimit = limit
	r.lastDemandsOffset = offset
	filtered := make([]ProjectDemand, 0, len(r.demands))
	for _, demand := range r.demands {
		if demand.TenantID == tenantID && demand.ProjectID == projectID {
			filtered = append(filtered, demand)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) ListProjectDemandsForConsole(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error) {
	filtered, err := r.ListProjectDemands(ctx, tenantID, projectID, limit, offset)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if !filtered[i].UpdatedAt.Equal(filtered[j].UpdatedAt) {
			return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
		}
		ri, rj := consoleDemandStatusRank(filtered[i].Status), consoleDemandStatusRank(filtered[j].Status)
		if ri != rj {
			return ri < rj
		}
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return filtered, nil
}

func (r *memoryRepository) ListProjectDemandOpenDecisionCounts(ctx context.Context, tenantID, projectID uuid.UUID) ([]DemandDossierSiblingPending, error) {
	demands, err := r.ListProjectDemands(ctx, tenantID, projectID, 0, 0)
	if err != nil {
		return nil, err
	}
	demandByTask := make(map[uuid.UUID]uuid.UUID)
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID && task.DemandID != nil {
			demandByTask[task.ID] = *task.DemandID
		}
	}
	demandByRevision := make(map[uuid.UUID]uuid.UUID)
	for _, revision := range r.planRevisions {
		if revision.TenantID == tenantID && revision.ProjectID == projectID {
			demandByRevision[revision.ID] = revision.DemandID
		}
	}
	counts := map[uuid.UUID]int{}
	for _, decision := range r.decisionRequests {
		if decision.TenantID != tenantID || decision.ProjectID != projectID {
			continue
		}
		if !isOpenDecisionStatus(decision.StatusSnapshot) {
			continue
		}
		switch {
		case decision.ProjectTaskID != nil:
			if demandID, ok := demandByTask[*decision.ProjectTaskID]; ok {
				counts[demandID]++
			}
		case decision.PlanRevisionID != nil:
			if demandID, ok := demandByRevision[*decision.PlanRevisionID]; ok {
				counts[demandID]++
			}
		}
	}
	siblings := make([]DemandDossierSiblingPending, 0, len(demands))
	for _, demand := range demands {
		siblings = append(siblings, DemandDossierSiblingPending{
			DemandID:      demand.ID,
			OpenDecisions: counts[demand.ID],
			DemandTitle:   demand.Title,
			DemandStatus:  string(demand.Status),
		})
	}
	return siblings, nil
}

func consoleDemandStatusRank(status ProjectDemandStatus) int {
	switch status {
	case ProjectDemandStatusCompleted, ProjectDemandStatusFailed, ProjectDemandStatusCancelled, ProjectDemandStatusPlanningFailed:
		return 1
	default:
		return 0
	}
}

func (r *memoryRepository) CreateConfigRevision(ctx context.Context, req UpdateProjectConfigRequest, project Project, eventID uuid.UUID) (ProjectConfigRevision, error) {
	revision := ProjectConfigRevision{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		RevisionNumber:  int32(len(r.revisions) + 1),
		ConfigSnapshot:  map[string]any{"name": project.Name, "status": string(project.Status)},
		ChangeSummary:   strPtrOrNil("项目配置已更新"),
		CreatedByUserID: req.ActorUserID,
		CreatedEventID:  &eventID,
	}
	r.revisions = append(r.revisions, revision)
	return revision, nil
}

func (r *memoryRepository) GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (ProjectDemand, error) {
	for _, demand := range r.demands {
		if demand.ID == demandID && demand.TenantID == tenantID {
			return demand, nil
		}
	}
	return ProjectDemand{}, ErrProjectNotFound
}

func (r *memoryRepository) ReopenProjectDemandForReplanning(ctx context.Context, tenantID, demandID uuid.UUID) (ProjectDemand, error) {
	for i := range r.demands {
		if r.demands[i].ID == demandID && r.demands[i].TenantID == tenantID {
			if r.demands[i].Status != ProjectDemandStatusFailed {
				return ProjectDemand{}, fmt.Errorf("demand %s is not in failed state (current=%s): %w", demandID, r.demands[i].Status, ErrProjectConflict)
			}
			r.demands[i].Status = ProjectDemandStatusPlanningPending
			if _, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
				TenantID:     tenantID,
				ProjectID:    r.demands[i].ProjectID,
				EventType:    ProjectEventDemandReplanningReopened,
				ActorType:    "project_coordinator",
				ActorID:      "project_coordinator",
				ResourceType: strPtr("project_demand"),
				ResourceID:   strPtr(demandID.String()),
				Summary:      "规划缺口补员后重开需求重新规划",
				Payload:      map[string]any{"demand_id": demandID.String()},
			}); err != nil {
				return ProjectDemand{}, err
			}
			return r.demands[i], nil
		}
	}
	return ProjectDemand{}, ErrProjectNotFound
}

func (r *memoryRepository) CloseProjectDemand(ctx context.Context, tenantID, demandID, actorUserID uuid.UUID, reason string) (ProjectDemand, error) {
	for i := range r.demands {
		if r.demands[i].ID == demandID && r.demands[i].TenantID == tenantID {
			status := r.demands[i].Status
			if status == ProjectDemandStatusCancelled {
				return r.demands[i], nil
			}
			if status == ProjectDemandStatusCompleted || status == ProjectDemandStatusFailed {
				return ProjectDemand{}, fmt.Errorf("demand %s already terminal (%s): %w", demandID, status, ErrProjectConflict)
			}
			r.demands[i].Status = ProjectDemandStatusCancelled
			if _, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
				TenantID:     tenantID,
				ProjectID:    r.demands[i].ProjectID,
				EventType:    ProjectEventDemandCancelled,
				ActorType:    "human_user",
				ActorID:      actorUserID.String(),
				ResourceType: strPtr("project_demand"),
				ResourceID:   strPtr(demandID.String()),
				Summary:      "需求已关闭",
				Payload:      map[string]any{"demand_id": demandID.String(), "reason": reason, "previous_status": string(status)},
			}); err != nil {
				return ProjectDemand{}, err
			}
			return r.demands[i], nil
		}
	}
	return ProjectDemand{}, ErrProjectNotFound
}

func (r *memoryRepository) AdvanceProjectDemandStatus(ctx context.Context, tenantID, projectID, demandID uuid.UUID, target ProjectDemandStatus) error {
	for i := range r.demands {
		if r.demands[i].ID == demandID && r.demands[i].TenantID == tenantID {
			if ProjectDemandStatusCanAdvance(r.demands[i].Status, target) {
				r.demands[i].Status = target
			}
			return nil
		}
	}
	return nil
}

func (r *memoryRepository) GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTask, error) {
	for _, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) GetProjectTaskInProject(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID) (ProjectTask, error) {
	for _, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID && task.ProjectID == projectID {
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) GetProjectTaskRunRuntimeNodeID(ctx context.Context, tenantID, projectTaskID, runID uuid.UUID) (uuid.UUID, error) {
	runtimeNodeID, ok := r.projectTaskRunRuntimeNodes[projectTaskID]
	if !ok {
		return uuid.Nil, ErrProjectNotFound
	}
	return runtimeNodeID, nil
}

func (r *memoryRepository) GetProjectTaskRunWorkProducts(ctx context.Context, tenantID, runID uuid.UUID) ([]any, error) {
	workProducts, ok := r.projectTaskRunWorkProducts[runID]
	if !ok {
		return []any{}, nil
	}
	return workProducts, nil
}

func (r *memoryRepository) CreateCoordinationJob(ctx context.Context, req CreateCoordinationJobRequest) (CoordinationJob, error) {
	job := CoordinationJob{
		ID:               uuid.New(),
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		WorkflowID:       req.WorkflowID,
		TriggerEventID:   req.TriggerEventID,
		JobType:          req.JobType,
		Status:           req.Status,
		InputSnapshotRef: req.InputSnapshotRef,
		OutputEventIDs:   []any{},
		CreatedAt:        time.Now().UTC(),
	}
	r.coordinationJobs = append(r.coordinationJobs, job)
	return job, nil
}

func (r *memoryRepository) FinishCoordinationJob(ctx context.Context, req FinishCoordinationJobRequest) (CoordinationJob, error) {
	for index, job := range r.coordinationJobs {
		if job.ID == req.ID && job.TenantID == req.TenantID {
			now := time.Now().UTC()
			job.Status = req.Status
			job.OutputEventIDs = req.OutputEventIDs
			job.FinishedAt = &now
			r.coordinationJobs[index] = job
			return job, nil
		}
	}
	return CoordinationJob{}, ErrProjectNotFound
}

func (r *memoryRepository) ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error) {
	filtered := make([]CoordinationJob, 0, len(r.coordinationJobs))
	for _, job := range r.coordinationJobs {
		if job.TenantID == tenantID && job.ProjectID == projectID {
			filtered = append(filtered, job)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) ListDemandLaunchCoordinationJobs(ctx context.Context, tenantID, projectID, demandID uuid.UUID, createdEventID *uuid.UUID, limit int32) ([]CoordinationJob, error) {
	candidates := make([]CoordinationJob, 0, len(r.coordinationJobs))
	for _, job := range r.coordinationJobs {
		if job.TenantID == tenantID && job.ProjectID == projectID {
			candidates = append(candidates, job)
		}
	}
	filtered := filterJobsForDemand(candidates, ProjectDemand{ID: demandID, CreatedEventID: createdEventID})
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, 0), nil
}

func (r *memoryRepository) CreateRouteDecision(ctx context.Context, req CreateRouteDecisionRequest) (RouteDecision, error) {
	decision := RouteDecision{
		ID:                          uuid.New(),
		TenantID:                    req.TenantID,
		ProjectID:                   req.ProjectID,
		CoordinationJobID:           req.CoordinationJobID,
		DemandID:                    req.DemandID,
		CandidateDigitalEmployeeIDs: req.CandidateDigitalEmployeeIDs,
		SelectedDigitalEmployeeIDs:  req.SelectedDigitalEmployeeIDs,
		Reason:                      req.Reason,
		InputRequirements:           req.InputRequirements,
		ExpectedOutputs:             req.ExpectedOutputs,
		BudgetEstimate:              req.BudgetEstimate,
		RequiresHumanReview:         req.RequiresHumanReview,
		CreatedEventID:              req.CreatedEventID,
		CreatedAt:                   time.Now().UTC(),
	}
	r.routeDecisions = append(r.routeDecisions, decision)
	return decision, nil
}

func (r *memoryRepository) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error) {
	filtered := make([]RouteDecision, 0, len(r.routeDecisions))
	for _, decision := range r.routeDecisions {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			filtered = append(filtered, decision)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) ListDemandLaunchRouteDecisions(ctx context.Context, tenantID, projectID, demandID uuid.UUID, limit int32) ([]RouteDecision, error) {
	candidates := make([]RouteDecision, 0, len(r.routeDecisions))
	for _, decision := range r.routeDecisions {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			candidates = append(candidates, decision)
		}
	}
	filtered := filterRoutesForDemand(candidates, demandID)
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, 0), nil
}

func (r *memoryRepository) CreatePlanRevision(ctx context.Context, req CreatePlanRevisionRequest) (PlanRevision, error) {
	return PlanRevision{}, ErrProjectNotFound
}

func (r *memoryRepository) GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (PlanRevision, error) {
	for _, revision := range r.planRevisions {
		if revision.ID == revisionID && revision.TenantID == tenantID && revision.ProjectID == projectID {
			return revision, nil
		}
	}
	return PlanRevision{}, ErrProjectNotFound
}

func (r *memoryRepository) ListPlanRevisions(ctx context.Context, req ListPlanRevisionsRequest) ([]PlanRevision, error) {
	result := make([]PlanRevision, 0, len(r.planRevisions))
	for _, revision := range r.planRevisions {
		if revision.TenantID != req.TenantID || revision.ProjectID != req.ProjectID {
			continue
		}
		if req.DemandID != nil && revision.DemandID != *req.DemandID {
			continue
		}
		result = append(result, revision)
	}
	return paginateTestSlice(result, req.Limit, req.Offset), nil
}

func (r *memoryRepository) ListPlanRevisionsForDemand(ctx context.Context, tenantID, projectID, demandID uuid.UUID) ([]PlanRevision, error) {
	result := make([]PlanRevision, 0)
	for _, revision := range r.planRevisions {
		if revision.TenantID == tenantID && revision.ProjectID == projectID && revision.DemandID == demandID {
			result = append(result, revision)
		}
	}
	return result, nil
}

func (r *memoryRepository) AcceptPlanRevision(ctx context.Context, req AcceptPlanRevisionRequest) (PlanRevision, error) {
	return PlanRevision{}, ErrProjectNotFound
}

func (r *memoryRepository) CancelStalePlanReviewDecisionsForDemand(ctx context.Context, tenantID, projectID, demandID, exceptRevisionID uuid.UUID) ([]DecisionRequest, error) {
	return nil, nil
}

func (r *memoryRepository) RejectPlanRevision(ctx context.Context, req RejectPlanRevisionRequest) (PlanRevision, error) {
	return PlanRevision{}, ErrProjectNotFound
}

func (r *memoryRepository) CreateProjectTask(ctx context.Context, req CreateProjectTaskRequest) (ProjectTask, error) {
	maxAttempts := EffectiveProjectTaskMaxAttempts(req.MaxAttempts, 0)
	task := ProjectTask{
		ID:                        uuid.New(),
		TenantID:                  req.TenantID,
		ProjectID:                 req.ProjectID,
		DemandID:                  req.DemandID,
		Title:                     req.Title,
		Summary:                   strPtrOrNil(req.Summary),
		Status:                    req.Status,
		AssignedDigitalEmployeeID: req.AssignedDigitalEmployeeID,
		RiskLevel:                 strPtrOrNil(req.RiskLevel),
		RequiresHumanApproval:     req.RequiresHumanApproval,
		MaxAttempts:               &maxAttempts,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	}
	r.tasks = append(r.tasks, task)
	return task, nil
}

func (r *memoryRepository) ListDemandLaunchProjectTasks(ctx context.Context, tenantID, projectID, demandID uuid.UUID, limit int32) ([]ProjectTask, error) {
	candidates := make([]ProjectTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID {
			candidates = append(candidates, task)
		}
	}
	filtered := filterTasksForDemand(candidates, demandID)
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})
	return paginateTestSlice(filtered, limit, 0), nil
}

// 接续链的内存实现：先沿 continues_demand_id 上溯到链头，再自链头向下展开。
// 与 SQL 版同口径(spec §4.1)：从链上任一成员出发返回同一条链，链头在前。
func (r *memoryRepository) ListProjectDemandContinuationChain(ctx context.Context, tenantID, demandID uuid.UUID, maxDepth int32) ([]ProjectDemand, error) {
	byID := map[uuid.UUID]ProjectDemand{}
	for _, demand := range r.demands {
		if demand.TenantID == tenantID {
			byID[demand.ID] = demand
		}
	}
	current, ok := byID[demandID]
	if !ok {
		return nil, ErrProjectNotFound
	}
	head := current
	seen := map[uuid.UUID]bool{head.ID: true}
	for depth := int32(0); depth < maxDepth; depth++ {
		if head.ContinuesDemandID == nil {
			break
		}
		parent, ok := byID[*head.ContinuesDemandID]
		if !ok || seen[parent.ID] {
			break
		}
		seen[parent.ID] = true
		head = parent
	}
	chain := []ProjectDemand{head}
	frontier := []ProjectDemand{head}
	visited := map[uuid.UUID]bool{head.ID: true}
	for depth := int32(0); depth < maxDepth && len(frontier) > 0; depth++ {
		next := make([]ProjectDemand, 0, len(frontier))
		for _, parent := range frontier {
			children := make([]ProjectDemand, 0)
			for _, candidate := range byID {
				if candidate.ContinuesDemandID != nil && *candidate.ContinuesDemandID == parent.ID && !visited[candidate.ID] {
					children = append(children, candidate)
				}
			}
			sort.SliceStable(children, func(i, j int) bool {
				return children[i].CreatedAt.Before(children[j].CreatedAt)
			})
			for _, child := range children {
				visited[child.ID] = true
				chain = append(chain, child)
				next = append(next, child)
			}
		}
		frontier = next
	}
	return chain, nil
}

func (r *memoryRepository) CountProjectDemandContinuationDepth(ctx context.Context, tenantID, demandID uuid.UUID, maxDepth int32) (int32, error) {
	byID := map[uuid.UUID]ProjectDemand{}
	for _, demand := range r.demands {
		if demand.TenantID == tenantID {
			byID[demand.ID] = demand
		}
	}
	current, ok := byID[demandID]
	if !ok {
		return 0, ErrProjectNotFound
	}
	seen := map[uuid.UUID]bool{current.ID: true}
	depth := int32(0)
	for depth < maxDepth {
		if current.ContinuesDemandID == nil {
			break
		}
		parent, ok := byID[*current.ContinuesDemandID]
		if !ok || seen[parent.ID] {
			break
		}
		seen[parent.ID] = true
		current = parent
		depth++
	}
	return depth, nil
}

func (r *memoryRepository) createProjectTaskAttempt(req QueueProjectTaskRequest, attemptNo int32, eventID *uuid.UUID) ProjectTaskAttempt {
	version := nonEmptyString(req.ExecutionContextPacketVersion, "v1")
	packet := req.ExecutionContextPacket
	if packet == nil {
		packet = map[string]any{}
	}
	packet = cloneMap(packet)
	attemptID := uuid.New()
	if req.ProjectTaskAttemptID != nil {
		attemptID = *req.ProjectTaskAttemptID
	}
	attempt := ProjectTaskAttempt{
		ID:                            attemptID,
		TenantID:                      req.TenantID,
		ProjectTaskID:                 req.ProjectTaskID,
		AttemptNo:                     attemptNo,
		Status:                        ProjectTaskAttemptStatusQueued,
		DigitalEmployeeID:             &req.DigitalEmployeeID,
		ProviderType:                  strPtrOrNil(req.ProviderType),
		DigitalEmployeeRunID:          req.DigitalEmployeeRunID,
		RuntimeTaskID:                 req.RuntimeTaskID,
		RuntimeNodeID:                 req.RuntimeNodeID,
		ExecutionContextPacket:        packet,
		ExecutionContextPacketVersion: version,
		LeaseToken:                    req.LeaseToken,
		LeaseExpiresAt:                req.LeaseExpiresAt,
		IdempotencyKey:                req.IdempotencyKey,
		DispatchGateResultID:          req.DispatchGateResultID,
		CreatedEventID:                eventID,
		CreatedAt:                     time.Now().UTC(),
		UpdatedAt:                     time.Now().UTC(),
	}
	r.projectTaskAttempts = append(r.projectTaskAttempts, attempt)
	return attempt
}

func (r *memoryRepository) replayQueueProjectTaskAttempt(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, bool, error) {
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != req.TenantID || attempt.IdempotencyKey != req.IdempotencyKey {
			continue
		}
		if attempt.ProjectTaskID != req.ProjectTaskID {
			return QueueProjectTaskResult{}, true, ErrProjectConflict
		}
		task, err := r.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
		if err != nil {
			return QueueProjectTaskResult{}, true, err
		}
		if task.ProjectID != req.ProjectID {
			return QueueProjectTaskResult{}, true, ErrProjectNotFound
		}
		var event ProjectEvent
		if attempt.CreatedEventID != nil {
			for _, candidate := range r.events {
				if candidate.TenantID == req.TenantID && candidate.ProjectID == req.ProjectID && candidate.ID == *attempt.CreatedEventID {
					event = candidate
					break
				}
			}
		}
		return QueueProjectTaskResult{Task: task, Attempt: attempt, Event: event}, true, nil
	}
	return QueueProjectTaskResult{}, false, nil
}

func (r *memoryRepository) QueueProjectTaskWithAttempt(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, error) {
	if result, replayed, err := r.replayQueueProjectTaskAttempt(ctx, req); replayed || err != nil {
		return result, err
	}
	for index, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusPlanned && task.Status != ProjectTaskStatusWaitingHuman {
			return QueueProjectTaskResult{}, ErrProjectConflict
		}
		if task.AssignedDigitalEmployeeID != nil && *task.AssignedDigitalEmployeeID != req.DigitalEmployeeID {
			return QueueProjectTaskResult{}, ErrProjectTaskForbidden
		}
		gateIndex := -1
		if req.DispatchGateResultID != nil {
			for candidateIndex, gate := range r.dispatchGateResults {
				if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == *req.DispatchGateResultID {
					if gate.AttemptID != nil && (req.ProjectTaskAttemptID == nil || *gate.AttemptID != *req.ProjectTaskAttemptID) {
						return QueueProjectTaskResult{}, ErrProjectNotFound
					}
					gateIndex = candidateIndex
					break
				}
			}
			if gateIndex == -1 {
				return QueueProjectTaskResult{}, ErrProjectNotFound
			}
		}
		event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventTaskDispatched,
			ActorType:    "project_coordinator",
			ActorID:      req.ProjectTaskID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(req.ProjectTaskID.String()),
			Summary:      "项目任务已排队",
			Payload:      queueProjectTaskEventPayload(req, uuid.Nil, task.AttemptCount+1),
		})
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		attempt := r.createProjectTaskAttempt(req, task.AttemptCount+1, &event.ID)
		event.Payload["project_task_attempt_id"] = attempt.ID.String()
		r.events[len(r.events)-1] = event
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusQueued
		task.CurrentAttemptID = &attempt.ID
		task.DigitalEmployeeRunID = req.DigitalEmployeeRunID
		task.RuntimeTaskID = req.RuntimeTaskID
		if req.DispatchGateResultID != nil {
			task.LatestDispatchGateResultID = req.DispatchGateResultID
			gate := r.dispatchGateResults[gateIndex]
			gate.AttemptID = &attempt.ID
			gate.UpdatedAt = now
			r.dispatchGateResults[gateIndex] = gate
		}
		task.AttemptCount++
		task.RetryNotBefore = nil
		task.WaitingReason = nil
		task.WaitingRequestID = nil
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return QueueProjectTaskResult{Task: task, Attempt: attempt, Event: event}, nil
	}
	return QueueProjectTaskResult{}, ErrProjectNotFound
}

func (r *memoryRepository) RecordPreDispatchGateResult(ctx context.Context, req RecordPreDispatchGateResultRequest) (PreDispatchGateResult, error) {
	now := time.Now().UTC()
	checkedAt := req.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = now
	}
	taskExists := false
	for _, task := range r.tasks {
		if task.TenantID == req.TenantID && task.ProjectID == req.ProjectID && task.ID == req.ProjectTaskID {
			taskExists = true
			break
		}
	}
	if !taskExists {
		return PreDispatchGateResult{}, ErrProjectNotFound
	}
	for index, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectTaskID == req.ProjectTaskID && gate.IdempotencyKey == req.IdempotencyKey {
			if gate.ProjectID != req.ProjectID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			if gate.AttemptID != nil || gate.DecisionRequestID != nil {
				return gate, nil
			}
			gate.Status = req.Status
			gate.CheckedAt = checkedAt
			gate.Checks = append([]PreDispatchGateCheck(nil), req.Checks...)
			gate.Blockers = append([]PreDispatchGateBlocker(nil), req.Blockers...)
			gate.HumanActionRequest = HumanActionRequest(cloneMap(map[string]any(req.HumanActionRequest)))
			gate.RetryAfter = req.RetryAfter
			if gate.CreatedEventID == nil {
				gate.CreatedEventID = req.CreatedEventID
			}
			gate.UpdatedAt = now
			r.dispatchGateResults[index] = gate
			if err := r.markLatestDispatchGate(req.TenantID, req.ProjectID, req.ProjectTaskID, gate.ID); err != nil {
				return PreDispatchGateResult{}, err
			}
			return gate, nil
		}
	}
	gate := PreDispatchGateResult{
		ID:                     uuid.New(),
		TenantID:               req.TenantID,
		ProjectID:              req.ProjectID,
		ProjectTaskID:          req.ProjectTaskID,
		AcceptedPlanRevisionID: req.AcceptedPlanRevisionID,
		PlannedTaskKey:         req.PlannedTaskKey,
		SelectedEmployeeID:     req.SelectedEmployeeID,
		AttemptNo:              req.AttemptNo,
		DispatchReason:         req.DispatchReason,
		IdempotencyKey:         req.IdempotencyKey,
		DispatchToken:          req.DispatchToken,
		Status:                 req.Status,
		CheckedAt:              checkedAt,
		Checks:                 append([]PreDispatchGateCheck(nil), req.Checks...),
		Blockers:               append([]PreDispatchGateBlocker(nil), req.Blockers...),
		HumanActionRequest:     HumanActionRequest(cloneMap(map[string]any(req.HumanActionRequest))),
		RetryAfter:             req.RetryAfter,
		CreatedEventID:         req.CreatedEventID,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	r.dispatchGateResults = append(r.dispatchGateResults, gate)
	if err := r.markLatestDispatchGate(req.TenantID, req.ProjectID, req.ProjectTaskID, gate.ID); err != nil {
		return PreDispatchGateResult{}, err
	}
	return gate, nil
}

func (r *memoryRepository) GetPreDispatchGateResult(ctx context.Context, tenantID, projectID, gateResultID uuid.UUID) (PreDispatchGateResult, error) {
	for _, gate := range r.dispatchGateResults {
		if gate.TenantID == tenantID && gate.ProjectID == projectID && gate.ID == gateResultID {
			return gate, nil
		}
	}
	return PreDispatchGateResult{}, ErrProjectNotFound
}

func (r *memoryRepository) GetPreDispatchGateResultByKey(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID, idempotencyKey string) (PreDispatchGateResult, error) {
	for _, gate := range r.dispatchGateResults {
		if gate.TenantID == tenantID && gate.ProjectID == projectID && gate.ProjectTaskID == projectTaskID && gate.IdempotencyKey == idempotencyKey {
			return gate, nil
		}
	}
	return PreDispatchGateResult{}, ErrProjectNotFound
}

func (r *memoryRepository) ListPreDispatchGateResults(ctx context.Context, req ListPreDispatchGateResultsRequest) ([]PreDispatchGateResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	filtered := make([]PreDispatchGateResult, 0, len(r.dispatchGateResults))
	for _, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID {
			filtered = append(filtered, gate)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) LinkPreDispatchGateAttempt(ctx context.Context, req LinkPreDispatchGateAttemptRequest) (PreDispatchGateResult, error) {
	gateIndex := -1
	for index, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == req.GateResultID {
			if gate.AttemptID != nil && *gate.AttemptID != req.AttemptID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			gateIndex = index
			break
		}
	}
	if gateIndex == -1 {
		return PreDispatchGateResult{}, ErrProjectNotFound
	}
	attemptIndex := -1
	for index, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == req.TenantID && attempt.ProjectTaskID == req.ProjectTaskID && attempt.ID == req.AttemptID {
			if attempt.DispatchGateResultID != nil && *attempt.DispatchGateResultID != req.GateResultID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			attemptIndex = index
			break
		}
	}
	if attemptIndex == -1 {
		return PreDispatchGateResult{}, ErrProjectNotFound
	}
	now := time.Now().UTC()
	gate := r.dispatchGateResults[gateIndex]
	gate.AttemptID = &req.AttemptID
	gate.UpdatedAt = now
	r.dispatchGateResults[gateIndex] = gate
	attempt := r.projectTaskAttempts[attemptIndex]
	attempt.DispatchGateResultID = &req.GateResultID
	attempt.UpdatedAt = now
	r.projectTaskAttempts[attemptIndex] = attempt
	return gate, nil
}

func (r *memoryRepository) LinkPreDispatchGateDecisionRequest(ctx context.Context, req LinkPreDispatchGateDecisionRequest) (PreDispatchGateResult, error) {
	decisionIndex := -1
	for index, decision := range r.decisionRequests {
		if decision.TenantID == req.TenantID && decision.ProjectID == req.ProjectID && decision.ID == req.DecisionRequestID {
			if decision.ProjectTaskID == nil || *decision.ProjectTaskID != req.ProjectTaskID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			if decision.DispatchGateResultID != nil && *decision.DispatchGateResultID != req.GateResultID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			decisionIndex = index
			break
		}
	}
	if decisionIndex == -1 {
		return PreDispatchGateResult{}, ErrProjectNotFound
	}
	gateIndex := -1
	for index, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == req.GateResultID {
			if gate.DecisionRequestID != nil && *gate.DecisionRequestID != req.DecisionRequestID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			gateIndex = index
			break
		}
	}
	if gateIndex == -1 {
		return PreDispatchGateResult{}, ErrProjectNotFound
	}
	now := time.Now().UTC()
	gate := r.dispatchGateResults[gateIndex]
	gate.DecisionRequestID = &req.DecisionRequestID
	gate.UpdatedAt = now
	r.dispatchGateResults[gateIndex] = gate
	decision := r.decisionRequests[decisionIndex]
	decision.DispatchGateResultID = &req.GateResultID
	decision.UpdatedAt = now
	r.decisionRequests[decisionIndex] = decision
	return gate, nil
}

func (r *memoryRepository) MoveProjectTaskToWaitingHumanForPreDispatchGate(ctx context.Context, req MoveProjectTaskToWaitingHumanForPreDispatchGateRequest) (ProjectTask, error) {
	gateExists := false
	for _, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == req.GateResultID {
			gateExists = true
			break
		}
	}
	if !gateExists {
		return ProjectTask{}, ErrProjectNotFound
	}
	for index, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusPlanned && task.Status != ProjectTaskStatusWaitingHuman {
			return ProjectTask{}, ErrProjectNotFound
		}
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusWaitingHuman
		task.WaitingReason = strPtrOrNil(req.WaitingReason)
		if req.DecisionRequestID != uuid.Nil {
			task.WaitingRequestID = &req.DecisionRequestID
		} else {
			task.WaitingRequestID = nil
		}
		task.LatestDispatchGateResultID = &req.GateResultID
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) markLatestDispatchGate(tenantID, projectID, projectTaskID, gateResultID uuid.UUID) error {
	for index, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID && task.ID == projectTaskID {
			task.LatestDispatchGateResultID = &gateResultID
			task.UpdatedAt = time.Now().UTC()
			r.tasks[index] = task
			return nil
		}
	}
	return ErrProjectNotFound
}

func (r *memoryRepository) GetProjectTaskAttempt(ctx context.Context, tenantID, attemptID uuid.UUID) (ProjectTaskAttempt, error) {
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == tenantID && attempt.ID == attemptID {
			return attempt, nil
		}
	}
	return ProjectTaskAttempt{}, ErrProjectNotFound
}

func (r *memoryRepository) UpdateProjectTaskAttemptBudgetHeartbeat(ctx context.Context, req RecordProjectTaskAttemptBudgetHeartbeatRequest, tripReason string) (ProjectTaskAttempt, error) {
	for index, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == req.TenantID && attempt.ProjectTaskID == req.ProjectTaskID && attempt.ID == req.AttemptID {
			now := time.Now().UTC()
			attempt.BudgetLastHeartbeatAt = &now
			if req.ConsumedWallClockSec > attempt.BudgetConsumedWallClockSec {
				attempt.BudgetConsumedWallClockSec = req.ConsumedWallClockSec
			}
			if req.ConsumedTokens > attempt.BudgetConsumedTokens {
				attempt.BudgetConsumedTokens = req.ConsumedTokens
			}
			if tripReason != "" && attempt.BudgetTrippedAt == nil {
				attempt.BudgetTrippedAt = &now
				attempt.BudgetTripReason = &tripReason
			}
			r.projectTaskAttempts[index] = attempt
			return attempt, nil
		}
	}
	return ProjectTaskAttempt{}, ErrProjectNotFound
}

func (r *memoryRepository) GetCurrentProjectTaskAttempt(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTaskAttempt, error) {
	task, err := r.GetProjectTask(ctx, tenantID, projectTaskID)
	if err != nil {
		return ProjectTaskAttempt{}, err
	}
	if task.CurrentAttemptID == nil {
		return ProjectTaskAttempt{}, ErrProjectNotFound
	}
	return r.GetProjectTaskAttempt(ctx, tenantID, *task.CurrentAttemptID)
}

func (r *memoryRepository) RecordProjectTaskAttemptContextUpdate(ctx context.Context, req RecordProjectTaskAttemptContextUpdateRepositoryRequest) (ProjectTaskAttemptContextUpdate, error) {
	update := ProjectTaskAttemptContextUpdate{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		ProjectTaskID:  req.ProjectTaskID,
		AttemptID:      req.AttemptID,
		UpdateKind:     req.UpdateKind,
		Payload:        cloneMap(req.Payload),
		DeliveryMode:   req.DeliveryMode,
		CreatedEventID: req.CreatedEventID,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	r.contextUpdates = append(r.contextUpdates, update)
	return update, nil
}

func (r *memoryRepository) DecomposeAcceptedPlanRevision(ctx context.Context, req DecomposeAcceptedPlanRevisionRequest) (DecomposeAcceptedPlanRevisionResult, error) {
	return DecomposeAcceptedPlanRevisionResult{}, ErrProjectTaskGraphPending
}

func (r *memoryRepository) UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID, currentStatuses []string) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			if r.taskStatusBeforeUpdate != nil {
				task.Status = *r.taskStatusBeforeUpdate
				r.tasks[index] = task
				r.taskStatusBeforeUpdate = nil
			}
			if !containsString(currentStatuses, task.Status) {
				return ProjectTask{}, ErrProjectNotFound
			}
			task.Status = status
			// 与真实 UpdateProjectTaskStatus 同口径：进终态清空等待指针。
			// 夹具不照做，就复现不出"补偿动作没还原指针 → 重试永久 409"。
			if isTerminalProjectTaskStatus(status) {
				task.WaitingReason = nil
				task.WaitingRequestID = nil
			}
			task.UpdatedAt = time.Now().UTC()
			r.tasks[index] = task
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

// 与真实 RestoreProjectTaskHumanWait 同口径：退回 waiting_human 并还原等待指针。
func (r *memoryRepository) RestoreProjectTaskHumanWait(ctx context.Context, tenantID, projectTaskID uuid.UUID, waitingReason *string, waitingRequestID *uuid.UUID) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.ID != projectTaskID || task.TenantID != tenantID {
			continue
		}
		if task.Status != ProjectTaskStatusCompleted {
			return ProjectTask{}, ErrProjectNotFound
		}
		task.Status = ProjectTaskStatusWaitingHuman
		task.WaitingReason = waitingReason
		task.WaitingRequestID = waitingRequestID
		task.UpdatedAt = time.Now().UTC()
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) BindProjectTaskRun(ctx context.Context, req BindProjectTaskRunRequest) (ProjectTask, error) {
	for i, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.DigitalEmployeeRunID != nil || task.RuntimeTaskID != nil {
			if task.DigitalEmployeeRunID != nil && *task.DigitalEmployeeRunID == req.DigitalEmployeeRunID &&
				task.RuntimeTaskID != nil && *task.RuntimeTaskID == req.RuntimeTaskID {
				return task, nil
			}
			return ProjectTask{}, ErrProjectConflict
		}
		allowed := false
		for _, status := range req.CurrentStatuses {
			if task.Status == status {
				allowed = true
				break
			}
		}
		if !allowed {
			return ProjectTask{}, ErrProjectConflict
		}
		task.Status = "assigned"
		task.DigitalEmployeeRunID = &req.DigitalEmployeeRunID
		task.RuntimeTaskID = &req.RuntimeTaskID
		task.UpdatedAt = time.Now().UTC()
		r.tasks[i] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) BindProjectTaskAttemptRun(ctx context.Context, req BindProjectTaskAttemptRunRequest) (ProjectTaskAttemptRunBindingResult, error) {
	attemptIndex := -1
	for index, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == req.TenantID && attempt.ProjectTaskID == req.ProjectTaskID && attempt.ID == req.AttemptID {
			attemptIndex = index
			break
		}
	}
	if attemptIndex == -1 {
		return ProjectTaskAttemptRunBindingResult{}, ErrProjectNotFound
	}
	attempt := r.projectTaskAttempts[attemptIndex]
	if attempt.Status != ProjectTaskAttemptStatusQueued {
		return ProjectTaskAttemptRunBindingResult{}, ErrProjectConflict
	}
	if attempt.DigitalEmployeeRunID != nil && *attempt.DigitalEmployeeRunID != req.DigitalEmployeeRunID {
		return ProjectTaskAttemptRunBindingResult{}, ErrProjectConflict
	}
	if attempt.RuntimeTaskID != nil && *attempt.RuntimeTaskID != req.RuntimeTaskID {
		return ProjectTaskAttemptRunBindingResult{}, ErrProjectConflict
	}
	if attempt.RuntimeNodeID != nil && *attempt.RuntimeNodeID != req.RuntimeNodeID {
		return ProjectTaskAttemptRunBindingResult{}, ErrProjectConflict
	}
	attempt.DigitalEmployeeRunID = &req.DigitalEmployeeRunID
	attempt.RuntimeTaskID = &req.RuntimeTaskID
	attempt.RuntimeNodeID = &req.RuntimeNodeID
	attempt.ProviderType = strPtrOrNil(req.ProviderType)
	attempt.ExecutionContextPacket = cloneMap(req.ExecutionContextPacket)
	attempt.ExecutionContextPacketVersion = nonEmptyString(req.ExecutionContextPacketVersion, "v1")
	attempt.UpdatedAt = time.Now().UTC()
	r.projectTaskAttempts[attemptIndex] = attempt
	for index, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusQueued || task.CurrentAttemptID == nil || *task.CurrentAttemptID != req.AttemptID {
			return ProjectTaskAttemptRunBindingResult{}, ErrProjectConflict
		}
		task.DigitalEmployeeRunID = &req.DigitalEmployeeRunID
		task.RuntimeTaskID = &req.RuntimeTaskID
		task.UpdatedAt = time.Now().UTC()
		r.tasks[index] = task
		return ProjectTaskAttemptRunBindingResult{Task: task, Attempt: attempt}, nil
	}
	return ProjectTaskAttemptRunBindingResult{}, ErrProjectNotFound
}

func (r *memoryRepository) FailQueuedProjectTaskAttemptDispatchStart(ctx context.Context, req FailQueuedProjectTaskAttemptDispatchStartRequest) (ProjectTaskWritebackResult, error) {
	attemptIndex := -1
	for index, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == req.TenantID && attempt.ProjectTaskID == req.ProjectTaskID && attempt.ID == req.AttemptID {
			attemptIndex = index
			break
		}
	}
	if attemptIndex == -1 {
		return ProjectTaskWritebackResult{}, ErrProjectNotFound
	}
	attempt := r.projectTaskAttempts[attemptIndex]
	if attempt.Status != ProjectTaskAttemptStatusQueued || attempt.LeaseToken != req.LeaseToken {
		return ProjectTaskWritebackResult{}, ErrProjectConflict
	}
	now := time.Now().UTC()
	attempt.Status = ProjectTaskAttemptStatusLost
	attempt.FinishedAt = &now
	attempt.Retryable = &req.Retryable
	attempt.FailureFamily = strPtrOrNil(req.FailureFamily)
	attempt.FailureMessage = strPtrOrNil(req.FailureSummary)
	attempt.TerminalEventID = req.DispatchFailureEventID
	attempt.UpdatedAt = now
	r.projectTaskAttempts[attemptIndex] = attempt
	for index, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusQueued || task.CurrentAttemptID == nil || *task.CurrentAttemptID != req.AttemptID {
			return ProjectTaskWritebackResult{}, ErrProjectConflict
		}
		task.Status = nonEmptyString(req.RestoreTaskStatus, ProjectTaskStatusPlanned)
		if req.ClearCurrentAttempt {
			task.CurrentAttemptID = nil
		}
		task.RetryNotBefore = req.RetryNotBefore
		task.UpdatedAt = now
		r.tasks[index] = task
		var event ProjectEvent
		if req.DispatchFailureEventID != nil {
			event, _ = r.GetProjectEvent(ctx, req.TenantID, req.ProjectID, *req.DispatchFailureEventID)
		}
		return ProjectTaskWritebackResult{Task: task, Event: event}, nil
	}
	return ProjectTaskWritebackResult{}, ErrProjectNotFound
}

func (r *memoryRepository) ProjectTaskEventExists(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (bool, error) {
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == eventType && event.ActorID == actorID {
			return true, nil
		}
	}
	return false, nil
}

func (r *memoryRepository) AssignProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, assignedDigitalEmployeeID, eventID *uuid.UUID) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			task.Status = status
			task.AssignedDigitalEmployeeID = assignedDigitalEmployeeID
			task.UpdatedAt = time.Now().UTC()
			r.tasks[index] = task
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) CreateExecutionSummary(ctx context.Context, req CreateExecutionSummaryRequest) (ExecutionSummary, error) {
	if r.createExecutionSummaryErr != nil {
		return ExecutionSummary{}, r.createExecutionSummaryErr
	}
	summary := ExecutionSummary{
		ID:                    uuid.New(),
		TenantID:              req.TenantID,
		ProjectID:             req.ProjectID,
		ProjectTaskID:         req.ProjectTaskID,
		DigitalEmployeeID:     req.DigitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          req.EvidenceRefs,
		ArtifactRefs:          req.ArtifactRefs,
		ConfidenceFactors:     req.ConfidenceFactors,
		Uncertainty:           strPtrOrNil(req.Uncertainty),
		MissingInformation:    req.MissingInformation,
		RecommendedNextAction: strPtrOrNil(req.RecommendedNextAction),
		RequiresHumanReview:   req.RequiresHumanReview,
		TransferRequestID:     req.TransferRequestID,
		CreatedEventID:        req.CreatedEventID,
		CreatedAt:             time.Now().UTC(),
	}
	r.executionSummaries = append(r.executionSummaries, summary)
	return summary, nil
}

func (r *memoryRepository) ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error) {
	r.lastExecutionSummariesLimit = limit
	r.lastExecutionSummariesOffset = offset
	filtered := make([]ExecutionSummary, 0, len(r.executionSummaries))
	for _, summary := range r.executionSummaries {
		if summary.TenantID == tenantID && summary.ProjectID == projectID {
			filtered = append(filtered, summary)
		}
	}
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) ListExecutionSummariesByTaskIDs(ctx context.Context, tenantID, projectID uuid.UUID, taskIDs []uuid.UUID) ([]ExecutionSummary, error) {
	taskIDSet := make(map[uuid.UUID]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		taskIDSet[taskID] = struct{}{}
	}
	filtered := make([]ExecutionSummary, 0, len(r.executionSummaries))
	for _, summary := range r.executionSummaries {
		if summary.TenantID != tenantID || summary.ProjectID != projectID {
			continue
		}
		if _, ok := taskIDSet[summary.ProjectTaskID]; ok {
			filtered = append(filtered, summary)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) ListDeclaredArtifactsByTaskIDs(ctx context.Context, tenantID, projectID uuid.UUID, taskIDs []uuid.UUID) ([]ProjectArtifactRef, error) {
	taskIDSet := make(map[uuid.UUID]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		taskIDSet[taskID] = struct{}{}
	}
	filtered := make([]ProjectArtifactRef, 0, len(r.artifactRefs))
	for _, ref := range r.artifactRefs {
		if ref.TenantID != tenantID || ref.ProjectID != projectID {
			continue
		}
		if ref.ArtifactType != "declared" || !strings.HasPrefix(ref.ObjectRef, "artifacts/") {
			continue
		}
		if ref.ProjectTaskID == nil {
			continue
		}
		if _, ok := taskIDSet[*ref.ProjectTaskID]; ok {
			filtered = append(filtered, ref)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) ListEvidenceRefsByTaskIDs(ctx context.Context, tenantID, projectID uuid.UUID, taskIDs []uuid.UUID) ([]ProjectEvidenceRef, error) {
	taskIDSet := make(map[uuid.UUID]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		taskIDSet[taskID] = struct{}{}
	}
	filtered := make([]ProjectEvidenceRef, 0, len(r.evidenceRefs))
	for _, ref := range r.evidenceRefs {
		if ref.TenantID != tenantID || ref.ProjectID != projectID || ref.ProjectTaskID == nil {
			continue
		}
		if _, ok := taskIDSet[*ref.ProjectTaskID]; ok {
			filtered = append(filtered, ref)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) ListArtifactRefsByTaskIDs(ctx context.Context, tenantID, projectID uuid.UUID, taskIDs []uuid.UUID) ([]ProjectArtifactRef, error) {
	taskIDSet := make(map[uuid.UUID]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		taskIDSet[taskID] = struct{}{}
	}
	filtered := make([]ProjectArtifactRef, 0, len(r.artifactRefs))
	for _, ref := range r.artifactRefs {
		if ref.TenantID != tenantID || ref.ProjectID != projectID || ref.ProjectTaskID == nil {
			continue
		}
		if _, ok := taskIDSet[*ref.ProjectTaskID]; ok {
			filtered = append(filtered, ref)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) ListLatestTaskResultContractsByTasks(ctx context.Context, tenantID, projectID uuid.UUID, tasks []ProjectTask) (map[uuid.UUID]*TaskResultContract, error) {
	contracts := map[uuid.UUID]*TaskResultContract{}
	for _, task := range tasks {
		for index := len(r.projectTaskResults) - 1; index >= 0; index-- {
			result := r.projectTaskResults[index]
			if result.TenantID != tenantID || result.ProjectID != projectID || result.ProjectTaskID != task.ID {
				continue
			}
			contract := result.Contract
			contracts[task.ID] = &contract
			break
		}
	}
	return contracts, nil
}

func (r *memoryRepository) CreateExecutionLedgerEvent(ctx context.Context, req CreateExecutionLedgerEventRequest) (ExecutionLedgerEvent, error) {
	if r.createExecutionLedgerEventErr != nil {
		return ExecutionLedgerEvent{}, r.createExecutionLedgerEventErr
	}
	if req.IdempotencyKey != "" {
		for _, event := range r.executionLedgerEvents {
			if event.TenantID == req.TenantID && event.IdempotencyKey == req.IdempotencyKey {
				return cloneExecutionLedgerEvent(event), nil
			}
		}
	}
	now := time.Now().UTC()
	occurredAt := now
	if req.OccurredAt != nil {
		occurredAt = *req.OccurredAt
	}
	event := ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             req.TenantID,
		TeamID:               req.TeamID,
		ProjectID:            req.ProjectID,
		ProjectTaskID:        req.ProjectTaskID,
		ProjectTaskAttemptID: req.ProjectTaskAttemptID,
		EventType:            req.EventType,
		SourceType:           req.SourceType,
		SourceID:             req.SourceID,
		ActorType:            req.ActorType,
		ActorID:              req.ActorID,
		RuntimeNodeID:        req.RuntimeNodeID,
		ProviderType:         req.ProviderType,
		ProviderSessionID:    req.ProviderSessionID,
		InputSummary:         strPtrOrNil(req.InputSummary),
		OutputSummary:        strPtrOrNil(req.OutputSummary),
		ErrorFamily:          strPtrOrNil(req.ErrorFamily),
		ErrorCode:            strPtrOrNil(req.ErrorCode),
		ErrorMessage:         strPtrOrNil(req.ErrorMessage),
		Retryable:            req.Retryable,
		ArtifactRefs:         append([]any(nil), sliceOrEmptyAny(req.ArtifactRefs)...),
		EvidenceRefs:         append([]any(nil), sliceOrEmptyAny(req.EvidenceRefs)...),
		Metadata:             cloneMap(mapOrEmptyAny(req.Metadata)),
		OccurredAt:           occurredAt,
		IdempotencyKey:       req.IdempotencyKey,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	r.executionLedgerEvents = append(r.executionLedgerEvents, cloneExecutionLedgerEvent(event))
	return event, nil
}

func (r *memoryRepository) ListProjectExecutionLedgerEvents(ctx context.Context, req GetExecutionTraceRequest) ([]ExecutionLedgerEvent, error) {
	r.executionLedgerEventListRequests = append(r.executionLedgerEventListRequests, req)
	filtered := make([]ExecutionLedgerEvent, 0, len(r.executionLedgerEvents))
	for _, event := range r.executionLedgerEvents {
		if event.TenantID != req.TenantID || event.ProjectID != req.ProjectID {
			continue
		}
		if req.ProjectTaskID != nil && (event.ProjectTaskID == nil || *event.ProjectTaskID != *req.ProjectTaskID) {
			continue
		}
		if req.ProjectTaskAttemptID != nil && (event.ProjectTaskAttemptID == nil || *event.ProjectTaskAttemptID != *req.ProjectTaskAttemptID) {
			continue
		}
		if req.EventType != nil && event.EventType != *req.EventType {
			continue
		}
		if req.ErrorFamily != nil && (event.ErrorFamily == nil || *event.ErrorFamily != *req.ErrorFamily) {
			continue
		}
		filtered = append(filtered, cloneExecutionLedgerEvent(event))
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if !filtered[i].OccurredAt.Equal(filtered[j].OccurredAt) {
			return filtered[i].OccurredAt.Before(filtered[j].OccurredAt)
		}
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, req.Limit, req.Offset), nil
}

func (r *memoryRepository) ListProjectTaskAttemptsForExecutionTrace(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectTaskAttempt, error) {
	taskProjects := make(map[uuid.UUID]uuid.UUID, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID == tenantID {
			taskProjects[task.ID] = task.ProjectID
		}
	}
	filtered := make([]ProjectTaskAttempt, 0, len(r.projectTaskAttempts))
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == tenantID && taskProjects[attempt.ProjectTaskID] == projectID {
			filtered = append(filtered, attempt)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].ProjectTaskID != filtered[j].ProjectTaskID {
			return filtered[i].ProjectTaskID.String() < filtered[j].ProjectTaskID.String()
		}
		if filtered[i].AttemptNo != filtered[j].AttemptNo {
			return filtered[i].AttemptNo < filtered[j].AttemptNo
		}
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})
	return filtered, nil
}

func (r *memoryRepository) ListCapabilityProjectionSourcesForAttempts(_ context.Context, _ uuid.UUID, attemptIDs []uuid.UUID) ([]CapabilityProjectionSourceRow, error) {
	out := make([]CapabilityProjectionSourceRow, 0, len(attemptIDs))
	for _, id := range attemptIDs {
		row := CapabilityProjectionSourceRow{AttemptID: id}
		if r.capabilityProjectionPayloads != nil {
			if payload, ok := r.capabilityProjectionPayloads[id]; ok {
				row.Payload = payload
				row.CommandID = "cmd-" + id.String()
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *memoryRepository) ListSkillNamesByIDs(_ context.Context, _ uuid.UUID, skillIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, id := range skillIDs {
		if r.skillNamesByID != nil {
			if name, ok := r.skillNamesByID[id]; ok {
				out[id] = name
			}
		}
	}
	return out, nil
}

func (r *memoryRepository) ListAttestationMetadataByAttemptIDs(_ context.Context, tenantID uuid.UUID, attemptIDs []uuid.UUID) (map[uuid.UUID][][]byte, error) {
	want := map[uuid.UUID]struct{}{}
	for _, id := range attemptIDs {
		want[id] = struct{}{}
	}
	out := map[uuid.UUID][][]byte{}
	for _, att := range r.projectTaskAttestations {
		if att.TenantID != tenantID {
			continue
		}
		if _, ok := want[att.AttemptID]; !ok {
			continue
		}
		raw, err := json.Marshal(att.Metadata)
		if err != nil {
			return nil, err
		}
		out[att.AttemptID] = append(out[att.AttemptID], raw)
	}
	return out, nil
}

func (r *memoryRepository) CreateTransferRequest(ctx context.Context, req CreateTransferRequestRequest) (TransferRequest, error) {
	if r.createTransferRequestErr != nil {
		return TransferRequest{}, r.createTransferRequestErr
	}
	transfer := TransferRequest{
		ID:                           uuid.New(),
		TenantID:                     req.TenantID,
		ProjectID:                    req.ProjectID,
		ProjectTaskID:                req.ProjectTaskID,
		RequestedByDigitalEmployeeID: req.RequestedByDigitalEmployeeID,
		Reason:                       req.Reason,
		SuggestedEmployeeType:        strPtrOrNil(req.SuggestedEmployeeType),
		SuggestedDigitalEmployeeIDs:  req.SuggestedDigitalEmployeeIDs,
		MissingContextRefs:           req.MissingContextRefs,
		Status:                       req.Status,
		CreatedEventID:               req.CreatedEventID,
		CreatedAt:                    time.Now().UTC(),
		UpdatedAt:                    time.Now().UTC(),
	}
	r.transferRequests = append(r.transferRequests, transfer)
	return transfer, nil
}

func (r *memoryRepository) CompleteProjectTaskWriteback(ctx context.Context, req CompleteProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	if _, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "completed", nil, req.AllowedCurrentStatuses); err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	summaryReq := req.Summary
	summaryReq.CreatedEventID = &event.ID
	summary, err := r.CreateExecutionSummary(ctx, summaryReq)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "completed", &event.ID, []string{"completed"})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event, Summary: summary}, nil
}

func (r *memoryRepository) FailProjectTaskWriteback(ctx context.Context, req FailProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	if _, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "failed", nil, req.AllowedCurrentStatuses); err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "failed", &event.ID, []string{"failed"})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event}, nil
}

func (r *memoryRepository) RequestProjectTaskTransferWriteback(ctx context.Context, req RequestProjectTaskTransferWritebackRequest) (ProjectTaskTransferWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	if _, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "waiting_human", nil, req.AllowedCurrentStatuses); err != nil {
		return ProjectTaskTransferWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskTransferWritebackResult{}, err
	}
	transferReq := req.Transfer
	transferReq.CreatedEventID = &event.ID
	transfer, err := r.CreateTransferRequest(ctx, transferReq)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskTransferWritebackResult{}, err
	}
	task, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "waiting_human", &event.ID, []string{"waiting_human"})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskTransferWritebackResult{}, err
	}
	return ProjectTaskTransferWritebackResult{Task: task, Event: event, Transfer: transfer}, nil
}

func (r *memoryRepository) StartProjectTaskAttemptWriteback(ctx context.Context, req StartProjectTaskAttemptRequest) (ProjectTaskAttemptWritebackResult, error) {
	for i, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != req.TenantID || attempt.ID != req.AttemptID {
			continue
		}
		if attempt.LeaseToken != req.LeaseToken {
			return ProjectTaskAttemptWritebackResult{}, ErrProjectConflict
		}
		now := time.Now().UTC()
		attempt.Status = ProjectTaskAttemptStatusRunning
		attempt.RuntimeNodeID = &req.RuntimeNodeID
		attempt.ProviderSessionID = req.ProviderSessionID
		if attempt.StartedAt == nil {
			attempt.StartedAt = &now
		}
		attempt.RenewedAt = &now
		attempt.UpdatedAt = now
		r.projectTaskAttempts[i] = attempt
		task, err := r.UpdateProjectTaskStatus(ctx, req.TenantID, req.ProjectTaskID, ProjectTaskStatusRunning, nil, []string{ProjectTaskStatusQueued, ProjectTaskStatusRunning})
		if err != nil {
			return ProjectTaskAttemptWritebackResult{}, err
		}
		return ProjectTaskAttemptWritebackResult{Task: task, Attempt: attempt}, nil
	}
	return ProjectTaskAttemptWritebackResult{}, ErrProjectNotFound
}

func (r *memoryRepository) RenewProjectTaskAttemptLeaseWriteback(ctx context.Context, req RenewProjectTaskAttemptLeaseRequest) (ProjectTaskAttempt, error) {
	for i, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != req.TenantID || attempt.ID != req.AttemptID {
			continue
		}
		if attempt.LeaseToken != req.LeaseToken {
			return ProjectTaskAttempt{}, ErrProjectConflict
		}
		now := time.Now().UTC()
		attempt.LeaseExpiresAt = req.LeaseExpiresAt
		attempt.RenewedAt = &now
		attempt.UpdatedAt = now
		r.projectTaskAttempts[i] = attempt
		return attempt, nil
	}
	return ProjectTaskAttempt{}, ErrProjectNotFound
}

func (r *memoryRepository) CompleteProjectTaskAttemptWriteback(ctx context.Context, req CompleteProjectTaskAttemptRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	task, err := r.UpdateProjectTaskStatus(ctx, req.TenantID, req.ProjectTaskID, ProjectTaskStatusCompleted, nil, []string{ProjectTaskStatusQueued, ProjectTaskStatusRunning})
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    task.ProjectID,
		EventType:    ProjectEventTaskCompleted,
		ActorType:    "digital_employee",
		ActorID:      req.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务已完成",
		Payload:      map[string]any{"project_task_id": task.ID.String(), "project_task_attempt_id": req.AttemptID.String()},
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	summary, err := r.CreateExecutionSummary(ctx, CreateExecutionSummaryRequest{
		TenantID:              req.TenantID,
		ProjectID:             task.ProjectID,
		ProjectTaskID:         task.ID,
		DigitalEmployeeID:     req.DigitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          sliceOrEmptyAny(req.EvidenceRefs),
		ArtifactRefs:          sliceOrEmptyAny(req.ArtifactRefs),
		ConfidenceFactors:     mapOrEmptyAny(req.ConfidenceFactors),
		Uncertainty:           strings.TrimSpace(req.Uncertainty),
		MissingInformation:    sliceOrEmptyAny(req.MissingInformation),
		RecommendedNextAction: strings.TrimSpace(req.RecommendedNextAction),
		RequiresHumanReview:   req.RequiresHumanReview,
		CreatedEventID:        &event.ID,
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if err := r.finishProjectTaskAttempt(req.TenantID, req.AttemptID, ProjectTaskAttemptStatusSucceeded, &event.ID, nil, nil, nil); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	for _, ledgerReq := range projectTaskAttemptCompletionLedgerEventRequests(req, task, event, summary, req.RequiresHumanReview) {
		if _, err := r.CreateExecutionLedgerEvent(ctx, ledgerReq); err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
	}
	task, err = r.UpdateProjectTaskStatus(ctx, req.TenantID, req.ProjectTaskID, ProjectTaskStatusCompleted, &event.ID, []string{ProjectTaskStatusCompleted})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event, Summary: summary}, nil
}

func (r *memoryRepository) CompleteProjectTaskAttemptResultWriteback(ctx context.Context, req CompleteProjectTaskAttemptResultWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	// Mirrors PgRepository: review_gate pending placeholders commit inside the
	// writeback, before the completion's demand-status recompute.
	for _, placeholder := range req.ReviewGatePlaceholders {
		if err := r.CreateReviewGateVerdict(ctx, placeholder); err != nil {
			return ProjectTaskWritebackResult{}, err
		}
	}
	result, err := r.CompleteProjectTaskAttemptWriteback(ctx, req.Complete)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := recordProjectTaskResultForMemoryWriteback(ctx, r, req.Result, result); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return result, nil
}

func (r *projectTaskResultMemoryRepository) CompleteProjectTaskAttemptResultWriteback(ctx context.Context, req CompleteProjectTaskAttemptResultWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	resultSnapshot := append([]ProjectTaskResult(nil), r.projectTaskResults...)
	for _, placeholder := range req.ReviewGatePlaceholders {
		if err := r.CreateReviewGateVerdict(ctx, placeholder); err != nil {
			return ProjectTaskWritebackResult{}, err
		}
	}
	result, err := r.memoryRepository.CompleteProjectTaskAttemptWriteback(ctx, req.Complete)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := recordProjectTaskResultForMemoryWriteback(ctx, r, req.Result, result); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		r.projectTaskResults = resultSnapshot
		return ProjectTaskWritebackResult{}, err
	}
	return result, nil
}

func recordProjectTaskResultForMemoryWriteback(ctx context.Context, repository interface {
	RecordProjectTaskResult(context.Context, RecordProjectTaskResultRequest) (ProjectTaskResult, error)
	LinkProjectTaskLatestResult(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (ProjectTask, error)
}, req RecordProjectTaskResultRequest, result ProjectTaskWritebackResult) (ProjectTaskResult, error) {
	req.ExecutionSummaryID = &result.Summary.ID
	req.CreatedEventID = &result.Event.ID
	taskResult, err := repository.RecordProjectTaskResult(ctx, req)
	if err != nil {
		return ProjectTaskResult{}, err
	}
	if _, err := repository.LinkProjectTaskLatestResult(ctx, req.TenantID, req.ProjectID, req.ProjectTaskID, taskResult.ID); err != nil {
		return ProjectTaskResult{}, err
	}
	return taskResult, nil
}

func (r *projectTaskResultMemoryRepository) RecordProjectTaskResult(ctx context.Context, req RecordProjectTaskResultRequest) (ProjectTaskResult, error) {
	if r.recordProjectTaskResultErr != nil {
		return ProjectTaskResult{}, r.recordProjectTaskResultErr
	}
	for _, result := range r.projectTaskResults {
		if result.TenantID == req.TenantID && result.IdempotencyKey == req.IdempotencyKey {
			return result, nil
		}
	}
	now := time.Now().UTC()
	result := ProjectTaskResult{
		ID:                 uuid.New(),
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		ProjectTaskID:      req.ProjectTaskID,
		AttemptID:          req.AttemptID,
		ExecutionSummaryID: req.ExecutionSummaryID,
		ResultStatus:       req.ResultStatus,
		ValidationStatus:   req.ValidationStatus,
		Decision:           req.Decision,
		Contract:           req.Contract,
		ValidationErrors:   append([]string(nil), req.ValidationErrors...),
		ValidationWarnings: append([]string(nil), req.ValidationWarnings...),
		IdempotencyKey:     req.IdempotencyKey,
		DecisionRequestID:  req.DecisionRequestID,
		RevisionTaskID:     req.RevisionTaskID,
		CreatedEventID:     req.CreatedEventID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	r.projectTaskResults = append(r.projectTaskResults, result)
	return result, nil
}

func (r *projectTaskResultMemoryRepository) ListProjectTaskResults(ctx context.Context, req ListProjectTaskResultsRequest) ([]ProjectTaskResult, error) {
	results := make([]ProjectTaskResult, 0, len(r.projectTaskResults))
	for _, result := range r.projectTaskResults {
		if result.TenantID == req.TenantID && result.ProjectID == req.ProjectID && result.ProjectTaskID == req.ProjectTaskID {
			results = append(results, result)
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})
	return paginateTestSlice(results, req.Limit, req.Offset), nil
}

func (r *projectTaskResultMemoryRepository) LinkProjectTaskLatestResult(ctx context.Context, tenantID, projectID, projectTaskID, resultID uuid.UUID) (ProjectTask, error) {
	r.linkProjectTaskLatestResultCalls++
	if r.linkProjectTaskLatestResultErr != nil &&
		(r.linkProjectTaskLatestResultErrAfter <= 0 || r.linkProjectTaskLatestResultCalls > r.linkProjectTaskLatestResultErrAfter) {
		return ProjectTask{}, r.linkProjectTaskLatestResultErr
	}
	for _, result := range r.projectTaskResults {
		if result.TenantID != tenantID || result.ProjectID != projectID || result.ProjectTaskID != projectTaskID || result.ID != resultID {
			continue
		}
		for i, task := range r.tasks {
			if task.TenantID == tenantID && task.ProjectID == projectID && task.ID == projectTaskID {
				task.LatestTaskResultID = &resultID
				task.UpdatedAt = time.Now().UTC()
				r.tasks[i] = task
				return task, nil
			}
		}
		return ProjectTask{}, ErrProjectNotFound
	}
	return ProjectTask{}, ErrProjectConflict
}

func (r *projectTaskResultMemoryRepository) LinkProjectTaskResultDecisionRequest(ctx context.Context, tenantID, projectID, resultID, decisionRequestID uuid.UUID) (ProjectTaskResult, error) {
	if r.linkProjectTaskResultDecisionRequestErr != nil {
		return ProjectTaskResult{}, r.linkProjectTaskResultDecisionRequestErr
	}
	decisionFound := false
	for _, decision := range r.decisionRequests {
		if decision.TenantID != tenantID || decision.ProjectID != projectID || decision.ID != decisionRequestID {
			continue
		}
		if decision.ProjectTaskID == nil {
			return ProjectTaskResult{}, ErrProjectConflict
		}
		decisionFound = true
		break
	}
	if !decisionFound {
		return ProjectTaskResult{}, ErrProjectConflict
	}
	for index, result := range r.projectTaskResults {
		if result.TenantID != tenantID || result.ProjectID != projectID || result.ID != resultID {
			continue
		}
		for _, decision := range r.decisionRequests {
			if decision.ID == decisionRequestID && decision.ProjectTaskID != nil && *decision.ProjectTaskID != result.ProjectTaskID {
				return ProjectTaskResult{}, ErrProjectConflict
			}
		}
		if result.DecisionRequestID != nil && *result.DecisionRequestID != decisionRequestID {
			return ProjectTaskResult{}, ErrProjectConflict
		}
		result.DecisionRequestID = &decisionRequestID
		result.UpdatedAt = time.Now().UTC()
		r.projectTaskResults[index] = result
		return result, nil
	}
	return ProjectTaskResult{}, ErrProjectConflict
}

func (r *memoryRepository) CreateProjectTaskAttestation(ctx context.Context, req CreateProjectTaskAttestationRequest) (ProjectTaskAttestation, error) {
	now := time.Now().UTC()
	attestation := ProjectTaskAttestation{
		ID:                        uuid.New(),
		TenantID:                  req.TenantID,
		ProjectID:                 req.ProjectID,
		ProjectTaskID:             req.ProjectTaskID,
		AttemptID:                 req.AttemptID,
		RuntimeNodeID:             req.RuntimeNodeID,
		DigitalEmployeeID:         req.DigitalEmployeeID,
		CapabilityManifestVersion: req.CapabilityManifestVersion,
		ProviderAuthMode:          req.ProviderAuthMode,
		ProviderSessionID:         req.ProviderSessionID,
		AttestationType:           req.AttestationType,
		Status:                    req.Status,
		CommandArgv:               append([]any(nil), req.CommandArgv...),
		ExitCode:                  req.ExitCode,
		DurationMs:                req.DurationMs,
		LogRef:                    req.LogRef,
		StdoutSha256:              req.StdoutSha256,
		StderrSha256:              req.StderrSha256,
		ArtifactRefs:              append([]any(nil), req.ArtifactRefs...),
		ArtifactHashes:            req.ArtifactHashes,
		GitBranch:                 req.GitBranch,
		GitBaseRef:                req.GitBaseRef,
		GitHeadSha:                req.GitHeadSha,
		GitDiffSha256:             req.GitDiffSha256,
		Metadata:                  req.Metadata,
		IdempotencyKey:            req.IdempotencyKey,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}
	r.projectTaskAttestations = append(r.projectTaskAttestations, attestation)
	return attestation, nil
}

func (r *memoryRepository) ListProjectTaskAttestations(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID, limit, offset int32) ([]ProjectTaskAttestation, error) {
	filtered := make([]ProjectTaskAttestation, 0)
	for _, item := range r.projectTaskAttestations {
		if item.TenantID == tenantID && item.ProjectID == projectID && item.ProjectTaskID == projectTaskID {
			filtered = append(filtered, item)
		}
	}
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) CompleteProjectTaskAttemptAcceptanceWriteback(ctx context.Context, req CompleteProjectTaskAttemptAcceptanceWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.Complete.TenantID,
		ProjectID:    req.Task.ProjectID,
		EventType:    ProjectEventTaskWaitingHuman,
		ActorType:    "digital_employee",
		ActorID:      req.Complete.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(req.Task.ID.String()),
		Summary:      "项目任务等待验收",
		Payload: map[string]any{
			"project_task_id":         req.Complete.ProjectTaskID.String(),
			"project_task_attempt_id": req.Complete.AttemptID.String(),
			"waiting_reason":          HumanWaitReasonAcceptanceRequired,
		},
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	summary, err := r.CreateExecutionSummary(ctx, CreateExecutionSummaryRequest{
		TenantID:              req.Complete.TenantID,
		ProjectID:             req.Task.ProjectID,
		ProjectTaskID:         req.Task.ID,
		DigitalEmployeeID:     req.Complete.DigitalEmployeeID,
		Conclusion:            req.Complete.Conclusion,
		EvidenceRefs:          sliceOrEmptyAny(req.Complete.EvidenceRefs),
		ArtifactRefs:          sliceOrEmptyAny(req.Complete.ArtifactRefs),
		ConfidenceFactors:     mapOrEmptyAny(req.Complete.ConfidenceFactors),
		Uncertainty:           strings.TrimSpace(req.Complete.Uncertainty),
		MissingInformation:    sliceOrEmptyAny(req.Complete.MissingInformation),
		RecommendedNextAction: strings.TrimSpace(req.Complete.RecommendedNextAction),
		RequiresHumanReview:   true,
		CreatedEventID:        &event.ID,
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if err := r.finishProjectTaskAttempt(req.Complete.TenantID, req.Complete.AttemptID, ProjectTaskAttemptStatusSucceeded, &event.ID, nil, nil, nil); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	for _, ledgerReq := range projectTaskAttemptCompletionLedgerEventRequests(req.Complete, req.Task, event, summary, true) {
		if _, err := r.CreateExecutionLedgerEvent(ctx, ledgerReq); err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
	}
	decisionReq := req.Decision
	decisionReq.CreatedEventID = &event.ID
	decision, err := r.CreateDecisionRequest(ctx, decisionReq)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err := r.moveProjectTaskToWaitingHumanWithRequest(req.Complete.TenantID, req.Complete.ProjectTaskID, HumanWaitReasonAcceptanceRequired, &decision.ID)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event, Summary: summary, Decision: decision}, nil
}

func (r *memoryRepository) CompleteProjectTaskAttemptAcceptanceResultWriteback(ctx context.Context, req CompleteProjectTaskAttemptAcceptanceResultWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	for _, placeholder := range req.ReviewGatePlaceholders {
		if err := r.CreateReviewGateVerdict(ctx, placeholder); err != nil {
			return ProjectTaskWritebackResult{}, err
		}
	}
	result, err := r.CompleteProjectTaskAttemptAcceptanceWriteback(ctx, req.Acceptance)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := recordProjectTaskResultForMemoryWriteback(ctx, r, req.Result, result); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return result, nil
}

func (r *projectTaskResultMemoryRepository) CompleteProjectTaskAttemptAcceptanceResultWriteback(ctx context.Context, req CompleteProjectTaskAttemptAcceptanceResultWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	resultSnapshot := append([]ProjectTaskResult(nil), r.projectTaskResults...)
	for _, placeholder := range req.ReviewGatePlaceholders {
		if err := r.CreateReviewGateVerdict(ctx, placeholder); err != nil {
			return ProjectTaskWritebackResult{}, err
		}
	}
	result, err := r.memoryRepository.CompleteProjectTaskAttemptAcceptanceWriteback(ctx, req.Acceptance)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := recordProjectTaskResultForMemoryWriteback(ctx, r, req.Result, result); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		r.projectTaskResults = resultSnapshot
		return ProjectTaskWritebackResult{}, err
	}
	return result, nil
}

func (r *memoryRepository) FailProjectTaskAttemptWriteback(ctx context.Context, req FailProjectTaskAttemptRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	task, err := r.UpdateProjectTaskStatus(ctx, req.TenantID, req.ProjectTaskID, ProjectTaskStatusFailed, nil, []string{ProjectTaskStatusQueued, ProjectTaskStatusRunning})
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    task.ProjectID,
		EventType:    ProjectEventTaskFailed,
		ActorType:    "digital_employee",
		ActorID:      req.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务执行失败",
		Payload: map[string]any{
			"project_task_id":         task.ID.String(),
			"project_task_attempt_id": req.AttemptID.String(),
			"failure_summary":         req.FailureSummary,
			"failure_family":          req.FailureFamily,
		},
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if err := r.finishProjectTaskAttempt(req.TenantID, req.AttemptID, ProjectTaskAttemptStatusFailed, &event.ID, req.Retryable, &req.FailureFamily, &req.FailureSummary); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := r.CreateExecutionLedgerEvent(ctx, projectTaskAttemptFailureLedgerEventRequest(req, task, event)); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err = r.UpdateProjectTaskStatus(ctx, req.TenantID, req.ProjectTaskID, ProjectTaskStatusFailed, &event.ID, []string{ProjectTaskStatusFailed})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event}, nil
}

func (r *memoryRepository) GetProjectTaskLatestDispatchFailureEvent(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID) (ProjectEvent, error) {
	for i := len(r.events) - 1; i >= 0; i-- {
		event := r.events[i]
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == ProjectEventTaskDispatchFailed && event.ActorID == projectTaskID.String() {
			return event, nil
		}
	}
	return ProjectEvent{}, ErrProjectNotFound
}

func (r *memoryRepository) CountProjectTaskDispatchFailureEvents(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID) (int64, error) {
	var count int64
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == ProjectEventTaskDispatchFailed && event.ActorID == projectTaskID.String() {
			count++
		}
	}
	return count, nil
}

func (r *memoryRepository) RecoverProjectTaskDispatchFailure(ctx context.Context, req RecoverProjectTaskDispatchFailureWritebackRequest) (ProjectTaskWritebackResult, error) {
	task, err := r.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	switch req.Action.Action {
	case ProjectTaskRecoveryActionFailed:
		reason := strings.TrimSpace(req.Action.TerminalReason)
		if reason == "" {
			reason = "dispatch recovery marked the project task failed"
		}
		event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:  req.TenantID,
			ProjectID: req.ProjectID,
			EventType: ProjectEventTaskFailed,
			ActorType: "project_coordinator",
			ActorID:   req.ProjectTaskID.String(),
			Summary:   reason,
			Payload: map[string]any{
				"project_task_id":          req.ProjectTaskID.String(),
				"dispatch_failed_event_id": req.FailureEventID.String(),
				"failure_family":           req.Action.FailureFamily,
				"terminal_reason":          reason,
			},
		})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		for i := range r.tasks {
			if r.tasks[i].TenantID == req.TenantID && r.tasks[i].ID == req.ProjectTaskID {
				r.tasks[i].Status = ProjectTaskStatusFailed
				r.tasks[i].WaitingReason = nil
				r.tasks[i].WaitingRequestID = nil
				r.tasks[i].RetryNotBefore = nil
				return ProjectTaskWritebackResult{Task: r.tasks[i], Event: event}, nil
			}
		}
	case ProjectTaskRecoveryActionRetryScheduled:
		event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:  req.TenantID,
			ProjectID: req.ProjectID,
			EventType: ProjectEventTaskRetryScheduled,
			ActorType: "project_coordinator",
			ActorID:   req.ProjectTaskID.String(),
			Payload: map[string]any{
				"project_task_id":          req.ProjectTaskID.String(),
				"dispatch_failed_event_id": req.FailureEventID.String(),
				"failure_family":           req.Action.FailureFamily,
				"retryable":                req.Action.Retryable,
			},
		})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		for i := range r.tasks {
			if r.tasks[i].TenantID == req.TenantID && r.tasks[i].ID == req.ProjectTaskID {
				r.tasks[i].Status = ProjectTaskStatusPlanned
				r.tasks[i].RetryNotBefore = req.Action.RetryNotBefore
				r.tasks[i].WaitingReason = nil
				r.tasks[i].WaitingRequestID = nil
				return ProjectTaskWritebackResult{Task: r.tasks[i], Event: event}, nil
			}
		}
	case ProjectTaskRecoveryActionWaitingHuman:
		event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:  req.TenantID,
			ProjectID: req.ProjectID,
			EventType: ProjectEventTaskRecoveryRequested,
			ActorType: "project_coordinator",
			ActorID:   req.ProjectTaskID.String(),
			Payload: map[string]any{
				"project_task_id":          req.ProjectTaskID.String(),
				"dispatch_failed_event_id": req.FailureEventID.String(),
				"failure_family":           req.Action.FailureFamily,
				"retryable":                req.Action.Retryable,
			},
		})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		decision, err := r.CreateDecisionRequest(ctx, CreateDecisionRequestRequest{
			TenantID:          req.TenantID,
			ProjectID:         req.ProjectID,
			ProjectTaskID:     &req.ProjectTaskID,
			TargetUserID:      r.projects[req.ProjectID].HumanOwnerUserID,
			DecisionType:      "project_task_recovery",
			TitleSnapshot:     task.Title,
			SummarySnapshot:   "项目任务分派失败，需要人工恢复决策",
			RiskLevelSnapshot: stringValue(task.RiskLevel),
			StatusSnapshot:    "pending",
			CreatedEventID:    &event.ID,
		})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		for i := range r.tasks {
			if r.tasks[i].TenantID == req.TenantID && r.tasks[i].ID == req.ProjectTaskID {
				r.tasks[i].Status = ProjectTaskStatusWaitingHuman
				r.tasks[i].WaitingReason = &req.Action.WaitingReason
				r.tasks[i].WaitingRequestID = &decision.ID
				return ProjectTaskWritebackResult{Task: r.tasks[i], Event: event, Decision: decision}, nil
			}
		}
	default:
		return ProjectTaskWritebackResult{Task: task}, nil
	}
	return ProjectTaskWritebackResult{}, ErrProjectNotFound
}

func (r *memoryRepository) ListStaleQueuedProjectTaskAttempts(ctx context.Context, tenantID uuid.UUID, startedBefore time.Time, limit int32) ([]ProjectTaskAttempt, error) {
	result := []ProjectTaskAttempt{}
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != tenantID || attempt.Status != ProjectTaskAttemptStatusQueued || attempt.StartedAt != nil {
			continue
		}
		if !attempt.CreatedAt.Before(startedBefore) {
			continue
		}
		task, err := r.GetProjectTask(ctx, tenantID, attempt.ProjectTaskID)
		if err != nil || task.Status != ProjectTaskStatusQueued {
			continue
		}
		result = append(result, attempt)
		if int32(len(result)) >= limit {
			break
		}
	}
	return result, nil
}

func (r *memoryRepository) ListExpiredRunningProjectTaskAttempts(ctx context.Context, tenantID uuid.UUID, now time.Time, limit int32) ([]ProjectTaskAttempt, error) {
	result := []ProjectTaskAttempt{}
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != tenantID || attempt.Status != ProjectTaskAttemptStatusRunning || attempt.LeaseExpiresAt == nil || !attempt.LeaseExpiresAt.Before(now) {
			continue
		}
		task, err := r.GetProjectTask(ctx, tenantID, attempt.ProjectTaskID)
		if err != nil || task.Status != ProjectTaskStatusRunning {
			continue
		}
		result = append(result, attempt)
		if int32(len(result)) >= limit {
			break
		}
	}
	return result, nil
}

func (r *memoryRepository) RecoverProjectTaskAttemptFailureWriteback(ctx context.Context, req RecoverProjectTaskAttemptFailureWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	eventType := ProjectEventTaskFailed
	summary := "项目任务执行失败"
	if req.TaskTargetStatus == ProjectTaskStatusCancelled {
		eventType = ProjectEventTaskCancelled
		summary = "项目任务已取消"
	}
	payload := map[string]any{
		"project_task_id":         req.Failure.ProjectTaskID.String(),
		"project_task_attempt_id": req.Failure.AttemptID.String(),
		"failure_summary":         req.Failure.FailureSummary,
		"failure_family":          req.Failure.FailureFamily,
		"recovery_status":         req.TaskTargetStatus,
	}
	if req.TaskTargetStatus == ProjectTaskStatusWaitingHuman {
		payload["waiting_reason"] = req.WaitingReason
	}
	if req.TaskTargetStatus == ProjectTaskStatusQueued {
		payload["retry_project_task_attempt_id"] = req.RetryAttemptID.String()
	}
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.Failure.TenantID,
		ProjectID:    req.Task.ProjectID,
		EventType:    eventType,
		ActorType:    "digital_employee",
		ActorID:      req.Failure.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(req.Task.ID.String()),
		Summary:      summary,
		Payload:      payload,
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if err := r.finishProjectTaskAttempt(req.Failure.TenantID, req.Failure.AttemptID, req.AttemptTerminalStatus, &event.ID, req.Failure.Retryable, &req.Failure.FailureFamily, &req.Failure.FailureSummary); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if ledgerReq, ok := recoveredProjectTaskAttemptLedgerEventRequest(req, event); ok {
		if _, err := r.CreateExecutionLedgerEvent(ctx, ledgerReq); err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
	}

	switch req.TaskTargetStatus {
	case ProjectTaskStatusQueued:
		task, err := r.scheduleProjectTaskRetry(req, &event.ID)
		if err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
		return ProjectTaskWritebackResult{Task: task, Event: event}, nil
	case ProjectTaskStatusWaitingHuman:
		if req.Decision == nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, fmt.Errorf("waiting_human recovery requires decision payload: %w", ErrInvalidProject)
		}
		decisionReq := *req.Decision
		decisionReq.CreatedEventID = &event.ID
		if decisionReq.ProjectTaskID == nil {
			decisionReq.ProjectTaskID = &req.Task.ID
		}
		if decisionReq.StatusSnapshot == "" {
			decisionReq.StatusSnapshot = "pending"
		}
		decision, err := r.CreateDecisionRequest(ctx, decisionReq)
		if err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
		task, err := r.moveProjectTaskToWaitingHuman(req, &event.ID, &decision.ID)
		if err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
		return ProjectTaskWritebackResult{Task: task, Event: event, Decision: decision}, nil
	case ProjectTaskStatusFailed, ProjectTaskStatusCancelled:
		task, err := r.UpdateProjectTaskStatus(ctx, req.Failure.TenantID, req.Failure.ProjectTaskID, req.TaskTargetStatus, &event.ID, []string{ProjectTaskStatusQueued, ProjectTaskStatusRunning})
		if err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
		return ProjectTaskWritebackResult{Task: task, Event: event}, nil
	default:
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, ErrInvalidProject
	}
}

func (r *memoryRepository) WaitHumanProjectTaskAttemptWriteback(ctx context.Context, req WaitHumanProjectTaskAttemptWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.Wait.TenantID,
		ProjectID:    req.Task.ProjectID,
		EventType:    ProjectEventTaskWaitingHuman,
		ActorType:    "digital_employee",
		ActorID:      req.Wait.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(req.Task.ID.String()),
		Summary:      req.Wait.Summary,
		Payload: map[string]any{
			"project_task_id":              req.Wait.ProjectTaskID.String(),
			"project_task_attempt_id":      req.Wait.AttemptID.String(),
			"reason":                       req.Wait.Reason,
			"summary":                      req.Wait.Summary,
			"missing_context_refs":         sliceOrEmptyAny(req.Wait.MissingContextRefs),
			"suggested_resolution_options": req.Wait.SuggestedResolutionOptions,
		},
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if err := r.finishProjectTaskAttempt(req.Wait.TenantID, req.Wait.AttemptID, ProjectTaskAttemptStatusWaitingHuman, &event.ID, nil, nil, &req.Wait.Summary); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	decisionReq := req.Decision
	decisionReq.CreatedEventID = &event.ID
	decision, err := r.CreateDecisionRequest(ctx, decisionReq)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := r.CreateExecutionLedgerEvent(ctx, projectTaskAttemptHumanWaitLedgerEventRequest(req, event, decision)); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err := r.moveProjectTaskToWaitingHumanWithRequest(req.Wait.TenantID, req.Wait.ProjectTaskID, req.Wait.Reason, &decision.ID)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event, Decision: decision}, nil
}

func (r *memoryRepository) moveProjectTaskToWaitingHumanWithRequest(tenantID, projectTaskID uuid.UUID, waitingReason string, waitingRequestID *uuid.UUID) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.TenantID != tenantID || task.ID != projectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusQueued && task.Status != ProjectTaskStatusRunning {
			return ProjectTask{}, ErrProjectConflict
		}
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusWaitingHuman
		task.WaitingReason = &waitingReason
		task.WaitingRequestID = waitingRequestID
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) ResolveProjectTaskHumanWaitWriteback(ctx context.Context, req ResolveProjectTaskHumanWaitWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	eventType := ProjectEventTaskCompleted
	summary := "项目任务等待已处理"
	switch req.TargetStatus {
	case ProjectTaskStatusQueued:
		eventType = ProjectEventTaskDispatched
		summary = "项目任务已恢复排队"
	case ProjectTaskStatusCancelled:
		eventType = ProjectEventTaskCancelled
		summary = "项目任务已取消"
	case ProjectTaskStatusFailed:
		eventType = ProjectEventTaskFailed
		summary = "项目任务已标记失败"
	}
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.Resolve.TenantID,
		ProjectID:    req.Resolve.ProjectID,
		EventType:    eventType,
		ActorType:    "human_user",
		ActorID:      req.Resolve.ActorUserID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(req.Resolve.ProjectTaskID.String()),
		Summary:      summary,
		Payload: map[string]any{
			"project_task_id":  req.Resolve.ProjectTaskID.String(),
			"resolution":       req.Resolve.Resolution,
			"response_summary": req.Resolve.ResponseSummary,
			"context_refs":     sliceOrEmptyAny(req.Resolve.ContextRefs),
		},
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	var task ProjectTask
	switch req.TargetStatus {
	case ProjectTaskStatusQueued:
		task, err = r.resumeProjectTaskAfterHumanWait(req, &event.ID)
	case ProjectTaskStatusCompleted, ProjectTaskStatusCancelled, ProjectTaskStatusFailed:
		task, err = r.UpdateProjectTaskStatus(ctx, req.Resolve.TenantID, req.Resolve.ProjectTaskID, req.TargetStatus, &event.ID, []string{ProjectTaskStatusWaitingHuman})
	default:
		err = ErrInvalidProject
	}
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event}, nil
}

func (r *memoryRepository) resumeProjectTaskAfterHumanWait(req ResolveProjectTaskHumanWaitWritebackRequest, eventID *uuid.UUID) (ProjectTask, error) {
	if req.CurrentAttempt.ID == uuid.Nil {
		return ProjectTask{}, ErrProjectNotFound
	}
	for index, task := range r.tasks {
		if task.TenantID != req.Resolve.TenantID || task.ID != req.Resolve.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusWaitingHuman {
			return ProjectTask{}, ErrProjectConflict
		}
		// Fresh dispatch identity on human-wait resume (same rule as automatic retry).
		attemptReq := QueueProjectTaskRequest{
			TenantID:                      task.TenantID,
			ProjectID:                     task.ProjectID,
			ProjectTaskID:                 task.ID,
			ProjectTaskAttemptID:          &req.RetryAttemptID,
			DigitalEmployeeID:             *task.AssignedDigitalEmployeeID,
			IdempotencyKey:                req.RetryIdempotencyKey,
			LeaseToken:                    req.RetryLeaseToken,
			ExecutionContextPacket:        req.CurrentAttempt.ExecutionContextPacket,
			ExecutionContextPacketVersion: req.CurrentAttempt.ExecutionContextPacketVersion,
		}
		attempt := r.createProjectTaskAttempt(attemptReq, task.AttemptCount+1, eventID)
		attempt.ID = req.RetryAttemptID
		r.projectTaskAttempts[len(r.projectTaskAttempts)-1] = attempt
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusQueued
		task.CurrentAttemptID = &attempt.ID
		task.RuntimeTaskID = nil
		task.DigitalEmployeeRunID = nil
		task.AttemptCount++
		task.WaitingReason = nil
		task.WaitingRequestID = nil
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) scheduleProjectTaskRetry(req RecoverProjectTaskAttemptFailureWritebackRequest, eventID *uuid.UUID) (ProjectTask, error) {
	retryAttemptID := req.RetryAttemptID
	if retryAttemptID == uuid.Nil {
		retryAttemptID = uuid.New()
	}
	retryLeaseToken := strings.TrimSpace(req.RetryLeaseToken)
	if retryLeaseToken == "" {
		retryLeaseToken = "retry-" + uuid.NewString()
	}
	retryIdempotencyKey := strings.TrimSpace(req.RetryIdempotencyKey)
	if retryIdempotencyKey == "" {
		retryIdempotencyKey = "project-task:" + req.Task.ID.String() + ":attempt:" + fmt.Sprint(req.Task.AttemptCount+1) + ":retry"
	}
	for index, task := range r.tasks {
		if task.TenantID != req.Failure.TenantID || task.ID != req.Failure.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusQueued && task.Status != ProjectTaskStatusRunning && task.Status != ProjectTaskStatusWaitingHuman {
			return ProjectTask{}, ErrProjectConflict
		}
		// Fresh dispatch identity on every retry (must not reuse failed attempt's run).
		attemptReq := QueueProjectTaskRequest{
			TenantID:                      task.TenantID,
			ProjectID:                     task.ProjectID,
			ProjectTaskID:                 task.ID,
			ProjectTaskAttemptID:          &retryAttemptID,
			DigitalEmployeeID:             req.Failure.DigitalEmployeeID,
			IdempotencyKey:                retryIdempotencyKey,
			LeaseToken:                    retryLeaseToken,
			ExecutionContextPacket:        req.Attempt.ExecutionContextPacket,
			ExecutionContextPacketVersion: req.Attempt.ExecutionContextPacketVersion,
		}
		attempt := r.createProjectTaskAttempt(attemptReq, task.AttemptCount+1, eventID)
		attempt.ID = retryAttemptID
		r.projectTaskAttempts[len(r.projectTaskAttempts)-1] = attempt
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusQueued
		task.CurrentAttemptID = &attempt.ID
		task.RuntimeTaskID = nil
		task.DigitalEmployeeRunID = nil
		task.AttemptCount++
		task.RetryNotBefore = req.RetryNotBefore
		task.WaitingReason = nil
		task.WaitingRequestID = nil
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) moveProjectTaskToWaitingHuman(req RecoverProjectTaskAttemptFailureWritebackRequest, eventID *uuid.UUID, waitingRequestID *uuid.UUID) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.TenantID != req.Failure.TenantID || task.ID != req.Failure.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusQueued && task.Status != ProjectTaskStatusRunning {
			return ProjectTask{}, ErrProjectConflict
		}
		waitingReason := req.WaitingReason
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusWaitingHuman
		task.WaitingReason = &waitingReason
		task.WaitingRequestID = waitingRequestID
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) finishProjectTaskAttempt(tenantID, attemptID uuid.UUID, status string, terminalEventID *uuid.UUID, retryable *bool, failureFamily, failureMessage *string) error {
	for i, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != tenantID || attempt.ID != attemptID {
			continue
		}
		now := time.Now().UTC()
		attempt.Status = status
		attempt.FinishedAt = &now
		attempt.TerminalEventID = terminalEventID
		attempt.Retryable = retryable
		attempt.FailureFamily = failureFamily
		attempt.FailureMessage = failureMessage
		attempt.UpdatedAt = now
		r.projectTaskAttempts[i] = attempt
		return nil
	}
	return ErrProjectNotFound
}

type memoryWritebackSnapshot struct {
	tasks                 []ProjectTask
	projectTaskAttempts   []ProjectTaskAttempt
	events                []ProjectEvent
	eventTypes            []ProjectEventType
	executionSummaries    []ExecutionSummary
	executionLedgerEvents []ExecutionLedgerEvent
	transferRequests      []TransferRequest
	decisionRequests      []DecisionRequest
}

func (r *memoryRepository) writebackSnapshot() memoryWritebackSnapshot {
	return memoryWritebackSnapshot{
		tasks:                 append([]ProjectTask(nil), r.tasks...),
		projectTaskAttempts:   append([]ProjectTaskAttempt(nil), r.projectTaskAttempts...),
		events:                append([]ProjectEvent(nil), r.events...),
		eventTypes:            append([]ProjectEventType(nil), r.eventTypes...),
		executionSummaries:    append([]ExecutionSummary(nil), r.executionSummaries...),
		executionLedgerEvents: append([]ExecutionLedgerEvent(nil), r.executionLedgerEvents...),
		transferRequests:      append([]TransferRequest(nil), r.transferRequests...),
		decisionRequests:      append([]DecisionRequest(nil), r.decisionRequests...),
	}
}

func (r *memoryRepository) restoreWritebackSnapshot(snapshot memoryWritebackSnapshot) {
	r.tasks = snapshot.tasks
	r.projectTaskAttempts = snapshot.projectTaskAttempts
	r.events = snapshot.events
	r.eventTypes = snapshot.eventTypes
	r.executionSummaries = snapshot.executionSummaries
	r.executionLedgerEvents = snapshot.executionLedgerEvents
	r.transferRequests = snapshot.transferRequests
	r.decisionRequests = snapshot.decisionRequests
}

func (r *memoryRepository) ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error) {
	filtered := make([]TransferRequest, 0, len(r.transferRequests))
	for _, transfer := range r.transferRequests {
		if transfer.TenantID == tenantID && transfer.ProjectID == projectID {
			filtered = append(filtered, transfer)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) ListOrphanWaitingHumanProjectTasks(ctx context.Context, limit int32) ([]ProjectTask, error) {
	out := make([]ProjectTask, 0)
	for _, task := range r.tasks {
		if task.Status != ProjectTaskStatusWaitingHuman || task.DismissedAt != nil {
			continue
		}
		if memoryTaskHasApprovedGateLinkedApproval(r, task) && memoryTaskWaitIsGateApprovalShaped(r, task) {
			continue
		}
		linkedOpen := false
		if task.WaitingRequestID != nil {
			for _, d := range r.decisionRequests {
				if d.ID == *task.WaitingRequestID && d.StatusSnapshot == "pending" {
					linkedOpen = true
					break
				}
			}
		}
		if linkedOpen {
			continue
		}
		// include if pointer missing/stale (even if another open exists — repair will bind)
		out = append(out, task)
		if limit > 0 && int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func (r *memoryRepository) ListPendingDecisionsMissingOpenInbox(ctx context.Context, limit int32) ([]DecisionRequest, error) {
	out := make([]DecisionRequest, 0)
	for _, decision := range r.decisionRequests {
		if !isPendingDecisionStatus(decision.StatusSnapshot) {
			continue
		}
		if r.openInboxDecisionIDs != nil && r.openInboxDecisionIDs[decision.ID] {
			continue
		}
		out = append(out, decision)
		if limit > 0 && int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func (r *memoryRepository) ListStrandedBlockedProjectTasks(ctx context.Context, limit int32) ([]ProjectTask, error) {
	blockerOf := map[uuid.UUID][]uuid.UUID{}
	for blockerID, dependents := range r.taskDependents {
		for _, dependentID := range dependents {
			blockerOf[dependentID] = append(blockerOf[dependentID], blockerID)
		}
	}
	statusByID := map[uuid.UUID]string{}
	for _, task := range r.tasks {
		statusByID[task.ID] = task.Status
	}
	out := make([]ProjectTask, 0)
	for _, task := range r.tasks {
		if task.Status != ProjectTaskStatusBlocked || task.DismissedAt != nil {
			continue
		}
		blockers := blockerOf[task.ID]
		if len(blockers) == 0 {
			continue
		}
		stranded := true
		for _, blockerID := range blockers {
			switch strings.ToLower(strings.TrimSpace(statusByID[blockerID])) {
			case "failed", "cancelled", "error":
			default:
				stranded = false
			}
			if !stranded {
				break
			}
		}
		if !stranded {
			continue
		}
		out = append(out, task)
		if limit > 0 && int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func memoryTaskHasApprovedGateLinkedApproval(r *memoryRepository, task ProjectTask) bool {
	for _, d := range r.decisionRequests {
		if d.TenantID != task.TenantID || d.ProjectID != task.ProjectID {
			continue
		}
		if d.ProjectTaskID == nil || *d.ProjectTaskID != task.ID {
			continue
		}
		if d.DecisionType != "project_task_approval" || !strings.EqualFold(d.StatusSnapshot, "approved") {
			continue
		}
		if d.DispatchGateResultID != nil && *d.DispatchGateResultID != uuid.Nil {
			return true
		}
	}
	return false
}

// memoryTaskWaitIsGateApprovalShaped mirrors the SQL shape predicate: pointer
// absent + approval_required reason, or pointer at a project_task_approval card.
func memoryTaskWaitIsGateApprovalShaped(r *memoryRepository, task ProjectTask) bool {
	if task.WaitingRequestID == nil || *task.WaitingRequestID == uuid.Nil {
		return task.WaitingReason != nil &&
			strings.TrimSpace(*task.WaitingReason) == HumanWaitReasonApprovalRequired
	}
	for _, d := range r.decisionRequests {
		if d.ID == *task.WaitingRequestID {
			return d.DecisionType == "project_task_approval"
		}
	}
	return false
}

func (r *memoryRepository) ListZombieGateApprovalWaitingHumanProjectTasks(ctx context.Context, limit int32) ([]ProjectTask, error) {
	out := make([]ProjectTask, 0)
	for _, task := range r.tasks {
		if task.Status != ProjectTaskStatusWaitingHuman || task.DismissedAt != nil {
			continue
		}
		if !memoryTaskHasApprovedGateLinkedApproval(r, task) {
			continue
		}
		if !memoryTaskWaitIsGateApprovalShaped(r, task) {
			continue
		}
		out = append(out, task)
		if limit > 0 && int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func (r *memoryRepository) ReleaseProjectTaskHumanWaitForRedispatch(ctx context.Context, req ReleaseProjectTaskHumanWaitRequest) (ReleaseProjectTaskHumanWaitResult, error) {
	for i, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ID != req.ProjectTaskID || task.ProjectID != req.ProjectID {
			continue
		}
		if task.Status != ProjectTaskStatusWaitingHuman {
			return ReleaseProjectTaskHumanWaitResult{}, ErrProjectConflict
		}
		now := time.Now().UTC()
		if req.MarkFailed {
			task.Status = ProjectTaskStatusFailed
			task.WaitingRequestID = nil
			task.WaitingReason = nil
			task.StatusChangedAt = now
			task.UpdatedAt = now
			r.tasks[i] = task
			return ReleaseProjectTaskHumanWaitResult{Task: task}, nil
		}
		task.Status = ProjectTaskStatusPlanned
		task.WaitingRequestID = nil
		task.WaitingReason = nil
		task.RuntimeTaskID = nil
		task.DigitalEmployeeRunID = nil
		task.CurrentAttemptID = nil
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[i] = task
		return ReleaseProjectTaskHumanWaitResult{Task: task, ReadyForDispatch: true}, nil
	}
	return ReleaseProjectTaskHumanWaitResult{}, ErrProjectNotFound
}

func (r *memoryRepository) GetOpenProjectDecisionRequestByTask(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID) (DecisionRequest, error) {
	for i := len(r.decisionRequests) - 1; i >= 0; i-- {
		d := r.decisionRequests[i]
		if d.TenantID == tenantID && d.ProjectID == projectID && d.ProjectTaskID != nil && *d.ProjectTaskID == projectTaskID && d.StatusSnapshot == "pending" {
			return d, nil
		}
	}
	return DecisionRequest{}, ErrProjectNotFound
}

func (r *memoryRepository) BindProjectTaskWaitingRequest(ctx context.Context, tenantID, projectTaskID, decisionRequestID uuid.UUID, waitingReason *string, eventID *uuid.UUID) (ProjectTask, error) {
	for i, task := range r.tasks {
		if task.TenantID != tenantID || task.ID != projectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusWaitingHuman {
			return ProjectTask{}, ErrProjectNotFound
		}
		task.WaitingRequestID = &decisionRequestID
		if waitingReason != nil {
			task.WaitingReason = waitingReason
		}
		task.UpdatedAt = time.Now().UTC()
		r.tasks[i] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) CreateDecisionRequest(ctx context.Context, req CreateDecisionRequestRequest) (DecisionRequest, error) {
	decision := DecisionRequest{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: req.ApprovalRequestID,
		CoordinationJobID: req.CoordinationJobID,
		ProjectTaskID:     req.ProjectTaskID,
		PlanRevisionID:    req.PlanRevisionID,
		TargetUserID:      req.TargetUserID,
		DecisionType:      req.DecisionType,
		TitleSnapshot:     req.TitleSnapshot,
		SummarySnapshot:   strPtrOrNil(req.SummarySnapshot),
		RiskLevelSnapshot: strPtrOrNil(req.RiskLevelSnapshot),
		StatusSnapshot:    req.StatusSnapshot,
		CreatedEventID:    req.CreatedEventID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	r.decisionRequests = append(r.decisionRequests, decision)
	return decision, nil
}

func (r *memoryRepository) GetDecisionRequest(ctx context.Context, tenantID, projectID, decisionRequestID uuid.UUID) (DecisionRequest, error) {
	for _, decision := range r.decisionRequests {
		if decision.ID == decisionRequestID && decision.TenantID == tenantID && decision.ProjectID == projectID {
			return decision, nil
		}
	}
	return DecisionRequest{}, ErrProjectNotFound
}

func (r *memoryRepository) GetDecisionRequestByPlanRevision(ctx context.Context, tenantID, projectID, planRevisionID uuid.UUID) (DecisionRequest, error) {
	for _, decision := range r.decisionRequests {
		if decision.TenantID == tenantID &&
			decision.ProjectID == projectID &&
			decision.PlanRevisionID != nil &&
			*decision.PlanRevisionID == planRevisionID &&
			decision.DecisionType == "plan_review" {
			return decision, nil
		}
	}
	return DecisionRequest{}, ErrProjectNotFound
}

func (r *memoryRepository) GetPendingDemandAcceptanceDecisionByPlanRevision(ctx context.Context, tenantID, projectID, planRevisionID uuid.UUID) (DecisionRequest, error) {
	for _, decision := range r.decisionRequests {
		if decision.TenantID == tenantID &&
			decision.ProjectID == projectID &&
			decision.PlanRevisionID != nil &&
			*decision.PlanRevisionID == planRevisionID &&
			decision.DecisionType == DecisionTypeDemandAcceptance &&
			decision.StatusSnapshot == "pending" {
			return decision, nil
		}
	}
	return DecisionRequest{}, ErrProjectNotFound
}

// RecomputeProjectDemandStatus is a test-scoped fake of
// PgRepository.RecomputeProjectDemandStatus, deliberately narrower: it only
// models the acceptance-criteria convergence gate's acceptance_pending →
// completed transition (Service.SignDemandCriterionVerdict's only caller in
// this package never invokes it from any other demand status, since that's
// a precondition it already checked). It does not replicate the real
// repository's task-status-count preamble.
func (r *memoryRepository) RecomputeProjectDemandStatus(ctx context.Context, tenantID, projectID, demandID uuid.UUID) error {
	demand, err := r.GetProjectDemand(ctx, tenantID, demandID)
	if err != nil {
		return err
	}
	if demand.Status != ProjectDemandStatusAcceptancePending {
		return nil
	}
	revisions, err := r.ListPlanRevisionsForDemand(ctx, tenantID, projectID, demandID)
	if err != nil {
		return err
	}
	revisionID := CurrentEffectivePlanRevisionID(revisions)
	if revisionID == uuid.Nil {
		return r.AdvanceProjectDemandStatus(ctx, tenantID, projectID, demandID, ProjectDemandStatusCompleted)
	}
	criteria, err := r.ListDemandAcceptanceCriteria(ctx, tenantID, demandID, revisionID)
	if err != nil {
		return err
	}
	verdicts, err := r.ListDemandCriterionVerdicts(ctx, tenantID, demandID, revisionID)
	if err != nil {
		return err
	}
	if len(ResolveUnsatisfiedBlockingCriteria(criteria, verdicts)) == 0 {
		return r.AdvanceProjectDemandStatus(ctx, tenantID, projectID, demandID, ProjectDemandStatusCompleted)
	}
	return nil
}

func (r *memoryRepository) ResolveDecisionRequest(ctx context.Context, req ResolveDecisionRequestRepositoryRequest) (DecisionRequest, error) {
	for index, decision := range r.decisionRequests {
		if decision.ID == req.ID && decision.TenantID == req.TenantID && decision.ProjectID == req.ProjectID {
			now := time.Now().UTC()
			decision.StatusSnapshot = req.StatusSnapshot
			decision.ResolvedEventID = req.ResolvedEventID
			decision.ResolvedAt = &now
			decision.UpdatedAt = now
			r.decisionRequests[index] = decision
			return decision, nil
		}
	}
	return DecisionRequest{}, ErrProjectNotFound
}

// EnsureDecisionCardsTerminal is a no-op in the in-memory repo (no feishu outbox).
// Call count is tracked so self-heal tests can assert the idempotent path fires.
func (r *memoryRepository) EnsureDecisionCardsTerminal(_ context.Context, decision DecisionRequest, resolvedBy uuid.UUID, comment string) error {
	r.ensureDecisionCardsTerminalCalls = append(r.ensureDecisionCardsTerminalCalls, ensureDecisionCardsTerminalCall{
		Decision:   decision,
		ResolvedBy: resolvedBy,
		Comment:    comment,
	})
	return nil
}

func (r *memoryRepository) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error) {
	filtered := make([]DecisionRequest, 0, len(r.decisionRequests))
	for _, decision := range r.decisionRequests {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			filtered = append(filtered, decision)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) ListDemandLaunchDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, coordinationJobIDs, projectTaskIDs []uuid.UUID, limit int32) ([]DecisionRequest, error) {
	jobIDs := map[uuid.UUID]struct{}{}
	for _, id := range coordinationJobIDs {
		jobIDs[id] = struct{}{}
	}
	taskIDs := map[uuid.UUID]struct{}{}
	for _, id := range projectTaskIDs {
		taskIDs[id] = struct{}{}
	}
	filtered := make([]DecisionRequest, 0, len(r.decisionRequests))
	for _, decision := range r.decisionRequests {
		if decision.TenantID != tenantID || decision.ProjectID != projectID {
			continue
		}
		if decision.CoordinationJobID != nil {
			if _, ok := jobIDs[*decision.CoordinationJobID]; ok {
				filtered = append(filtered, decision)
				continue
			}
		}
		if decision.ProjectTaskID != nil {
			if _, ok := taskIDs[*decision.ProjectTaskID]; ok {
				filtered = append(filtered, decision)
			}
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, 0), nil
}

func (r *memoryRepository) ListDemandLaunchEvents(ctx context.Context, tenantID, projectID, demandID uuid.UUID, createdEventID *uuid.UUID, projectTaskIDs, decisionRequestIDs []uuid.UUID, limit int32) ([]ProjectEvent, error) {
	candidates := make([]ProjectEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID {
			candidates = append(candidates, event)
		}
	}
	tasks := make([]ProjectTask, 0, len(projectTaskIDs))
	for _, id := range projectTaskIDs {
		tasks = append(tasks, ProjectTask{ID: id})
	}
	decisions := make([]DecisionRequest, 0, len(decisionRequestIDs))
	for _, id := range decisionRequestIDs {
		decisions = append(decisions, DecisionRequest{ID: id})
	}
	filtered := filterEventsForDemand(candidates, ProjectDemand{ID: demandID, CreatedEventID: createdEventID}, tasks, decisions)
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].SequenceNumber > filtered[j].SequenceNumber
	})
	return paginateTestSlice(filtered, limit, 0), nil
}

func (r *memoryRepository) CreateEvidenceRefWithEvent(ctx context.Context, req CreateEvidenceRefWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	evidenceReq := req.Evidence
	evidenceReq.CreatedEventID = &event.ID
	evidence, err := r.CreateEvidenceRef(ctx, evidenceReq)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *memoryRepository) UpdateEvidenceVerificationStatusWithEvent(ctx context.Context, req UpdateEvidenceVerificationStatusWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	evidence, err := r.UpdateEvidenceVerificationStatus(ctx, req.Evidence)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *memoryRepository) CreateAcceptanceRecordWithEvent(ctx context.Context, req CreateAcceptanceRecordWithEventRequest) (ProjectAcceptanceRecordWriteResult, error) {
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	acceptanceReq := req.Acceptance
	acceptanceReq.CreatedEventID = &event.ID
	acceptance, err := r.CreateAcceptanceRecord(ctx, acceptanceReq)
	if err != nil {
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	return ProjectAcceptanceRecordWriteResult{Event: event, Acceptance: acceptance}, nil
}

func (r *memoryRepository) CreateArchiveSnapshotWithEvent(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	snapshotReq := req.Snapshot
	snapshotReq.CreatedEventID = &event.ID
	snapshot, err := r.CreateArchiveSnapshot(ctx, snapshotReq)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return ProjectArchiveSnapshotWriteResult{Event: event, Snapshot: snapshot}, nil
}

func (r *memoryRepository) CreateArchiveSnapshotWithEventAndArchiveProject(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	eventSnapshot := append([]ProjectEvent(nil), r.events...)
	eventTypesSnapshot := append([]ProjectEventType(nil), r.eventTypes...)
	archiveSnapshotsSnapshot := append([]ProjectArchiveSnapshot(nil), r.archiveSnapshots...)
	projectsSnapshot := cloneProjects(r.projects)
	result, err := r.CreateArchiveSnapshotWithEvent(ctx, req)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	if _, err := r.ArchiveProject(ctx, req.Snapshot.TenantID, req.Snapshot.ProjectID); err != nil {
		r.events = eventSnapshot
		r.eventTypes = eventTypesSnapshot
		r.archiveSnapshots = archiveSnapshotsSnapshot
		r.projects = projectsSnapshot
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return result, nil
}

type governanceMemoryRepository struct {
	*memoryRepository
	evidenceRefs         []ProjectEvidenceRef
	artifactRefs         []ProjectArtifactRef
	reportRefs           []ProjectReportRef
	budgetLedger         []ProjectBudgetLedgerEntry
	acceptanceRecords    []ProjectAcceptanceRecord
	archiveSnapshots     []ProjectArchiveSnapshot
	createEvidenceRefErr error
	createAcceptanceErr  error
	createArchiveSnapErr error
}

func newGovernanceMemoryRepository() *governanceMemoryRepository {
	return &governanceMemoryRepository{memoryRepository: newMemoryRepository()}
}

func (r *governanceMemoryRepository) CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefRequest) (ProjectEvidenceRef, error) {
	if r.createEvidenceRefErr != nil {
		return ProjectEvidenceRef{}, r.createEvidenceRefErr
	}
	evidence := ProjectEvidenceRef{
		ID:                 uuid.New(),
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		ProjectTaskID:      req.ProjectTaskID,
		RouteDecisionID:    req.RouteDecisionID,
		ExecutionSummaryID: req.ExecutionSummaryID,
		EvidenceType:       req.EvidenceType,
		Title:              req.Title,
		Summary:            strPtrOrNil(req.Summary),
		SourceType:         req.SourceType,
		SourceRef:          req.SourceRef,
		ArtifactRefID:      req.ArtifactRefID,
		SubmittedByType:    req.SubmittedByType,
		SubmittedByID:      req.SubmittedByID,
		VerificationStatus: req.VerificationStatus,
		Metadata:           req.Metadata,
		CreatedEventID:     req.CreatedEventID,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	r.evidenceRefs = append(r.evidenceRefs, evidence)
	return evidence, nil
}

func (r *governanceMemoryRepository) CreateEvidenceRefWithEvent(ctx context.Context, req CreateEvidenceRefWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	snapshot := r.governanceSnapshot()
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	evidenceReq := req.Evidence
	evidenceReq.CreatedEventID = &event.ID
	evidence, err := r.CreateEvidenceRef(ctx, evidenceReq)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *governanceMemoryRepository) ListEvidenceRefs(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error) {
	filtered := make([]ProjectEvidenceRef, 0, len(r.evidenceRefs))
	for _, evidence := range r.evidenceRefs {
		if evidence.TenantID == tenantID && evidence.ProjectID == projectID && (status == nil || evidence.VerificationStatus == *status) {
			filtered = append(filtered, evidence)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) UpdateEvidenceVerificationStatus(ctx context.Context, req UpdateEvidenceVerificationStatusRequest) (ProjectEvidenceRef, error) {
	for index, evidence := range r.evidenceRefs {
		if evidence.ID == req.ID && evidence.TenantID == req.TenantID && evidence.ProjectID == req.ProjectID {
			evidence.VerificationStatus = req.VerificationStatus
			if req.Metadata != nil {
				evidence.Metadata = req.Metadata
			}
			evidence.UpdatedAt = time.Now().UTC()
			r.evidenceRefs[index] = evidence
			return evidence, nil
		}
	}
	return ProjectEvidenceRef{}, ErrProjectNotFound
}

func (r *governanceMemoryRepository) UpdateEvidenceVerificationStatusWithEvent(ctx context.Context, req UpdateEvidenceVerificationStatusWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	snapshot := r.governanceSnapshot()
	evidence, err := r.UpdateEvidenceVerificationStatus(ctx, req.Evidence)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *governanceMemoryRepository) CreateArtifactRef(ctx context.Context, req CreateArtifactRefRequest) (ProjectArtifactRef, error) {
	artifact := ProjectArtifactRef{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ProjectTaskID:   req.ProjectTaskID,
		ArtifactID:      req.ArtifactID,
		ArtifactType:    req.ArtifactType,
		Title:           req.Title,
		ObjectRef:       req.ObjectRef,
		ContentType:     strPtrOrNil(req.ContentType),
		SizeBytes:       req.SizeBytes,
		Checksum:        strPtrOrNil(req.Checksum),
		RetentionStatus: req.RetentionStatus,
		RetentionHoldID: req.RetentionHoldID,
		Metadata:        req.Metadata,
		CreatedEventID:  req.CreatedEventID,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	artifact.AttemptID = req.AttemptID
	artifact.DigitalEmployeeID = req.DigitalEmployeeID
	r.artifactRefs = append(r.artifactRefs, artifact)
	return artifact, nil
}

func (r *governanceMemoryRepository) GetArtifactRef(ctx context.Context, tenantID, artifactRefID uuid.UUID) (ProjectArtifactRef, error) {
	for _, artifact := range r.artifactRefs {
		if artifact.ID == artifactRefID && artifact.TenantID == tenantID {
			return artifact, nil
		}
	}
	return ProjectArtifactRef{}, ErrProjectNotFound
}

func (r *governanceMemoryRepository) ListArtifactRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error) {
	filtered := make([]ProjectArtifactRef, 0, len(r.artifactRefs))
	for _, artifact := range r.artifactRefs {
		if artifact.TenantID == tenantID && artifact.ProjectID == projectID {
			filtered = append(filtered, artifact)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) UpdateArtifactRetention(ctx context.Context, req UpdateArtifactRetentionRequest) (ProjectArtifactRef, error) {
	for index, artifact := range r.artifactRefs {
		if artifact.ID == req.ID && artifact.TenantID == req.TenantID && artifact.ProjectID == req.ProjectID {
			artifact.RetentionStatus = req.RetentionStatus
			artifact.RetentionHoldID = req.RetentionHoldID
			artifact.UpdatedAt = time.Now().UTC()
			r.artifactRefs[index] = artifact
			return artifact, nil
		}
	}
	return ProjectArtifactRef{}, ErrProjectNotFound
}

func (r *governanceMemoryRepository) CreateReportRef(ctx context.Context, req CreateReportRefRequest) (ProjectReportRef, error) {
	report := ProjectReportRef{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ReportType:      req.ReportType,
		Title:           req.Title,
		Summary:         strPtrOrNil(req.Summary),
		ObjectRef:       req.ObjectRef,
		Format:          req.Format,
		GeneratedByType: req.GeneratedByType,
		GeneratedByID:   req.GeneratedByID,
		CreatedEventID:  req.CreatedEventID,
		CreatedAt:       time.Now().UTC(),
	}
	r.reportRefs = append(r.reportRefs, report)
	return report, nil
}

func (r *governanceMemoryRepository) ListReportRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error) {
	filtered := make([]ProjectReportRef, 0, len(r.reportRefs))
	for _, report := range r.reportRefs {
		if report.TenantID == tenantID && report.ProjectID == projectID {
			filtered = append(filtered, report)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) CreateBudgetLedgerEntry(ctx context.Context, req CreateBudgetLedgerEntryRequest) (ProjectBudgetLedgerEntry, error) {
	entry := ProjectBudgetLedgerEntry{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		CoordinationJobID: req.CoordinationJobID,
		ProjectTaskID:     req.ProjectTaskID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		CostType:          req.CostType,
		EstimatedTokens:   req.EstimatedTokens,
		ActualTokens:      req.ActualTokens,
		EstimatedCost:     req.EstimatedCost,
		ActualCost:        req.ActualCost,
		Source:            req.Source,
		Reason:            strPtrOrNil(req.Reason),
		CreatedEventID:    req.CreatedEventID,
		CreatedAt:         time.Now().UTC(),
	}
	r.budgetLedger = append(r.budgetLedger, entry)
	return entry, nil
}

func (r *governanceMemoryRepository) ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error) {
	filtered := make([]ProjectBudgetLedgerEntry, 0, len(r.budgetLedger))
	for _, entry := range r.budgetLedger {
		if entry.TenantID == tenantID && entry.ProjectID == projectID {
			filtered = append(filtered, entry)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectBudgetSummary, error) {
	var summary ProjectBudgetSummary
	for _, entry := range r.budgetLedger {
		if entry.TenantID != tenantID || entry.ProjectID != projectID {
			continue
		}
		summary.LedgerCount++
		if entry.EstimatedTokens != nil {
			summary.EstimatedTokens += *entry.EstimatedTokens
		}
		if entry.ActualTokens != nil {
			summary.ActualTokens += *entry.ActualTokens
		}
		if entry.EstimatedCost != "" {
			summary.EstimatedCost = entry.EstimatedCost
		}
		if entry.ActualCost != "" {
			summary.ActualCost = entry.ActualCost
		}
	}
	return summary, nil
}

func (r *governanceMemoryRepository) CreateAcceptanceRecord(ctx context.Context, req CreateAcceptanceRecordRequest) (ProjectAcceptanceRecord, error) {
	if r.createAcceptanceErr != nil {
		return ProjectAcceptanceRecord{}, r.createAcceptanceErr
	}
	record := ProjectAcceptanceRecord{
		ID:               uuid.New(),
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		AcceptedByUserID: req.AcceptedByUserID,
		Status:           req.Status,
		Conclusion:       req.Conclusion,
		Summary:          strPtrOrNil(req.Summary),
		EvidenceRefIDs:   req.EvidenceRefIDs,
		ReportRefIDs:     req.ReportRefIDs,
		UnresolvedRisks:  req.UnresolvedRisks,
		CreatedEventID:   req.CreatedEventID,
		CreatedAt:        time.Now().UTC(),
	}
	r.acceptanceRecords = append(r.acceptanceRecords, record)
	return record, nil
}

func (r *governanceMemoryRepository) CreateAcceptanceRecordWithEvent(ctx context.Context, req CreateAcceptanceRecordWithEventRequest) (ProjectAcceptanceRecordWriteResult, error) {
	snapshot := r.governanceSnapshot()
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	acceptanceReq := req.Acceptance
	acceptanceReq.CreatedEventID = &event.ID
	acceptance, err := r.CreateAcceptanceRecord(ctx, acceptanceReq)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	return ProjectAcceptanceRecordWriteResult{Event: event, Acceptance: acceptance}, nil
}

func (r *governanceMemoryRepository) GetLatestAcceptanceRecord(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectAcceptanceRecord, error) {
	for index := len(r.acceptanceRecords) - 1; index >= 0; index-- {
		record := r.acceptanceRecords[index]
		if record.TenantID == tenantID && record.ProjectID == projectID {
			return record, nil
		}
	}
	return ProjectAcceptanceRecord{}, ErrProjectNotFound
}

func (r *governanceMemoryRepository) CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotRequest) (ProjectArchiveSnapshot, error) {
	if r.createArchiveSnapErr != nil {
		return ProjectArchiveSnapshot{}, r.createArchiveSnapErr
	}
	snapshot := ProjectArchiveSnapshot{
		ID:                   uuid.New(),
		TenantID:             req.TenantID,
		ProjectID:            req.ProjectID,
		SnapshotType:         req.SnapshotType,
		Status:               req.Status,
		ObjectRef:            strPtrOrNil(req.ObjectRef),
		Summary:              strPtrOrNil(req.Summary),
		IncludedCounts:       req.IncludedCounts,
		RetainedArtifactIDs:  req.RetainedArtifactIDs,
		RetentionLockEventID: req.RetentionLockEventID,
		CreatedByUserID:      req.CreatedByUserID,
		CreatedEventID:       req.CreatedEventID,
		CreatedAt:            time.Now().UTC(),
	}
	r.archiveSnapshots = append(r.archiveSnapshots, snapshot)
	return snapshot, nil
}

func (r *governanceMemoryRepository) CreateArchiveSnapshotWithEvent(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	snapshot := r.governanceSnapshot()
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	snapshotReq := req.Snapshot
	snapshotReq.CreatedEventID = &event.ID
	archiveSnapshot, err := r.CreateArchiveSnapshot(ctx, snapshotReq)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return ProjectArchiveSnapshotWriteResult{Event: event, Snapshot: archiveSnapshot}, nil
}

func (r *governanceMemoryRepository) CreateArchiveSnapshotWithEventAndArchiveProject(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	snapshot := r.governanceSnapshot()
	result, err := r.CreateArchiveSnapshotWithEvent(ctx, req)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	if _, err := r.ArchiveProject(ctx, req.Snapshot.TenantID, req.Snapshot.ProjectID); err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return result, nil
}

func (r *governanceMemoryRepository) ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error) {
	filtered := make([]ProjectArchiveSnapshot, 0, len(r.archiveSnapshots))
	for _, snapshot := range r.archiveSnapshots {
		if snapshot.TenantID == tenantID && snapshot.ProjectID == projectID {
			filtered = append(filtered, snapshot)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error) {
	filtered := make([]ProjectConfigRevision, 0, len(r.revisions))
	for _, revision := range r.revisions {
		if revision.TenantID == tenantID && revision.ProjectID == projectID {
			filtered = append(filtered, revision)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (ProjectConfigRevision, error) {
	for _, revision := range r.revisions {
		if revision.ID == revisionID && revision.TenantID == tenantID && revision.ProjectID == projectID {
			return revision, nil
		}
	}
	return ProjectConfigRevision{}, ErrProjectNotFound
}

type governanceMemorySnapshot struct {
	projects          map[uuid.UUID]Project
	events            []ProjectEvent
	eventTypes        []ProjectEventType
	evidenceRefs      []ProjectEvidenceRef
	acceptanceRecords []ProjectAcceptanceRecord
	archiveSnapshots  []ProjectArchiveSnapshot
}

func (r *governanceMemoryRepository) governanceSnapshot() governanceMemorySnapshot {
	return governanceMemorySnapshot{
		projects:          cloneProjects(r.projects),
		events:            append([]ProjectEvent(nil), r.events...),
		eventTypes:        append([]ProjectEventType(nil), r.eventTypes...),
		evidenceRefs:      append([]ProjectEvidenceRef(nil), r.evidenceRefs...),
		acceptanceRecords: append([]ProjectAcceptanceRecord(nil), r.acceptanceRecords...),
		archiveSnapshots:  append([]ProjectArchiveSnapshot(nil), r.archiveSnapshots...),
	}
}

func (r *governanceMemoryRepository) restoreGovernanceSnapshot(snapshot governanceMemorySnapshot) {
	r.projects = snapshot.projects
	r.events = snapshot.events
	r.eventTypes = snapshot.eventTypes
	r.evidenceRefs = snapshot.evidenceRefs
	r.acceptanceRecords = snapshot.acceptanceRecords
	r.archiveSnapshots = snapshot.archiveSnapshots
}

type fakeArchiveArtifactLocker struct {
	artifactIDs []uuid.UUID
	holdIDs     []uuid.UUID
	eventID     *uuid.UUID
	err         error
}

func (l *fakeArchiveArtifactLocker) LockProjectArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, artifactIDs []uuid.UUID) (ArchiveArtifactLockResult, error) {
	l.artifactIDs = append([]uuid.UUID(nil), artifactIDs...)
	if len(l.holdIDs) == 0 {
		l.holdIDs = make([]uuid.UUID, 0, len(artifactIDs))
		for range artifactIDs {
			l.holdIDs = append(l.holdIDs, uuid.New())
		}
	}
	return ArchiveArtifactLockResult{
		HoldIDs:     append([]uuid.UUID(nil), l.holdIDs...),
		ArtifactIDs: append([]uuid.UUID(nil), artifactIDs...),
		EventID:     l.eventID,
	}, l.err
}

type fakeCoordinatorSignalClient struct {
	ensureSignals      int
	demandSignals      int
	policySignals      int
	memberSignals      int
	completedSignals   int
	failedSignals      int
	retrySignals       int
	transferSignals    int
	decisionSignals    int
	terminateSignals   int
	lastDemand         DemandSubmittedSignal
	lastCompleted      EmployeeTaskCompletedSignal
	lastRetry          ProjectTaskRetryScheduledSignal
	lastDecision       HumanDecisionSubmittedSignal
	lastTerminate      TerminateProjectCoordinatorSignal
	demandSignalErr    error
	policySignalErr    error
	completedSignalErr error
	terminateSignalErr error
}

func (f *fakeCoordinatorSignalClient) EnsureProjectCoordinator(ctx context.Context, signal ProjectCoordinatorSignal) error {
	f.ensureSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalDemandSubmitted(ctx context.Context, signal DemandSubmittedSignal) error {
	f.demandSignals++
	f.lastDemand = signal
	return f.demandSignalErr
}

func (f *fakeCoordinatorSignalClient) SignalProjectPolicyChanged(ctx context.Context, signal ProjectPolicyChangedSignal) error {
	f.policySignals++
	return f.policySignalErr
}

func (f *fakeCoordinatorSignalClient) SignalProjectMemberChanged(ctx context.Context, signal ProjectMemberChangedSignal) error {
	f.memberSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTaskCompleted(ctx context.Context, signal EmployeeTaskCompletedSignal) error {
	f.completedSignals++
	f.lastCompleted = signal
	return f.completedSignalErr
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTaskFailed(ctx context.Context, signal EmployeeTaskFailedSignal) error {
	f.failedSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalProjectTaskRetryScheduled(ctx context.Context, signal ProjectTaskRetryScheduledSignal) error {
	f.retrySignals++
	f.lastRetry = signal
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTransferRequested(ctx context.Context, signal EmployeeTransferRequestedSignal) error {
	f.transferSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalHumanDecisionSubmitted(ctx context.Context, signal HumanDecisionSubmittedSignal) error {
	f.decisionSignals++
	f.lastDecision = signal
	return nil
}

func (f *fakeCoordinatorSignalClient) TerminateProjectCoordinator(ctx context.Context, signal TerminateProjectCoordinatorSignal) error {
	f.terminateSignals++
	f.lastTerminate = signal
	return f.terminateSignalErr
}

type fakeApprovalResolver struct {
	calls int
	last  ResolveApprovalRequest

	// contextPayloads backs GetRequestContextPayload, keyed by approval request
	// ID — mirrors the approval request's ContextPayload as ResolveDecision would
	// read it back (e.g. the planning_gap decision's structured gap).
	contextPayloads    map[uuid.UUID]map[string]any
	contextPayloadErr  error
	contextPayloadCall int

	createCalls   int
	lastCreate    CreateApprovalRequestInput
	lastCreatedID uuid.UUID
	createErr     error
}

func (f *fakeApprovalResolver) ResolveApproval(ctx context.Context, req ResolveApprovalRequest) error {
	f.calls++
	f.last = req
	return nil
}

func (f *fakeApprovalResolver) GetRequestContextPayload(ctx context.Context, tenantID, approvalRequestID uuid.UUID) (map[string]any, error) {
	f.contextPayloadCall++
	if f.contextPayloadErr != nil {
		return nil, f.contextPayloadErr
	}
	return f.contextPayloads[approvalRequestID], nil
}

func (f *fakeApprovalResolver) CreateRequest(ctx context.Context, req CreateApprovalRequestInput) (uuid.UUID, error) {
	f.createCalls++
	f.lastCreate = req
	if f.createErr != nil {
		return uuid.Nil, f.createErr
	}
	id := uuid.New()
	f.lastCreatedID = id
	return id, nil
}

type fakeDecisionInboxProjector struct {
	upserts          []DecisionRequest
	resolutions      []DecisionRequest
	upsertErr        error
	resolveErr       error
	openBySource     map[uuid.UUID]bool
}

func (f *fakeDecisionInboxProjector) UpsertProjectDecisionRequest(ctx context.Context, decision DecisionRequest) error {
	f.upserts = append(f.upserts, decision)
	if f.upsertErr != nil {
		return f.upsertErr
	}
	if f.openBySource != nil {
		if isPendingDecisionStatus(decision.StatusSnapshot) {
			f.openBySource[decision.ID] = true
		} else {
			delete(f.openBySource, decision.ID)
		}
	}
	return nil
}

func (f *fakeDecisionInboxProjector) ResolveProjectDecisionRequest(ctx context.Context, decision DecisionRequest) error {
	f.resolutions = append(f.resolutions, decision)
	if f.resolveErr != nil {
		return f.resolveErr
	}
	if f.openBySource != nil {
		delete(f.openBySource, decision.ID)
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func paginateSlice[T any](values []T, limit, offset int32) []T {
	start := int(offset)
	if start > len(values) {
		return []T{}
	}
	end := start + int(limit)
	if end > len(values) {
		end = len(values)
	}
	return values[start:end]
}

func countProjectEvents(values []ProjectEventType, target ProjectEventType) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func projectEventTypes(events []ProjectEvent) []ProjectEventType {
	values := make([]ProjectEventType, 0, len(events))
	for _, event := range events {
		values = append(values, event.EventType)
	}
	return values
}

type stubScenarioTemplateResolver struct {
	bindings     map[string]ScenarioTemplateBinding
	produceKinds map[string][]string
	kindsErr     error
}

func (r stubScenarioTemplateResolver) ResolveScenarioTemplate(_ context.Context, _ uuid.UUID, key string) (ScenarioTemplateBinding, error) {
	binding, ok := r.bindings[key]
	if !ok {
		return ScenarioTemplateBinding{}, errors.New("scenario template not found")
	}
	return binding, nil
}

func (r stubScenarioTemplateResolver) ResolveScenarioTemplateProduceKinds(_ context.Context, _ uuid.UUID, key string) ([]string, error) {
	if r.kindsErr != nil {
		return nil, r.kindsErr
	}
	kinds, ok := r.produceKinds[key]
	if !ok {
		return nil, errors.New("scenario template not found")
	}
	return kinds, nil
}

func TestCreateProjectScenarioTemplateBinding(t *testing.T) {
	newService := func(t *testing.T) (*Service, *memoryRepository, uuid.UUID, uuid.UUID, uuid.UUID) {
		t.Helper()
		repo := newMemoryRepository()
		service, err := NewService(repo)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		tenantID := uuid.New()
		ownerID := uuid.New()
		runtimeNodeID := uuid.New()
		stubProjectRuntimeNodeReader(service, tenantID, runtimeNodeID)
		service.SetScenarioTemplateResolver(stubScenarioTemplateResolver{bindings: map[string]ScenarioTemplateBinding{
			"ops_analysis": {Key: "ops_analysis", Name: "运维分析", Status: "active"},
			"retired":      {Key: "retired", Name: "退役", Status: "disabled"},
		}})
		return service, repo, tenantID, ownerID, runtimeNodeID
	}
	baseRequest := func(tenantID, ownerID, runtimeNodeID uuid.UUID) CreateProjectRequest {
		return CreateProjectRequest{
			TenantID:         tenantID,
			ActorUserID:      ownerID,
			Name:             "scenario-template-binding",
			Goal:             "验证场景模板绑定",
			HumanOwnerUserID: ownerID,
			RuntimeNodeIDs:   []uuid.UUID{runtimeNodeID},
		}
	}

	t.Run("unknown key rejected", func(t *testing.T) {
		service, _, tenantID, ownerID, nodeID := newService(t)
		req := baseRequest(tenantID, ownerID, nodeID)
		key := "nope"
		req.ScenarioTemplateKey = &key
		if _, err := service.CreateProject(context.Background(), req); !errors.Is(err, ErrInvalidProject) {
			t.Fatalf("expected ErrInvalidProject, got %v", err)
		}
	})

	t.Run("disabled key rejected", func(t *testing.T) {
		service, _, tenantID, ownerID, nodeID := newService(t)
		req := baseRequest(tenantID, ownerID, nodeID)
		key := "retired"
		req.ScenarioTemplateKey = &key
		if _, err := service.CreateProject(context.Background(), req); !errors.Is(err, ErrInvalidProject) {
			t.Fatalf("expected ErrInvalidProject, got %v", err)
		}
	})

	t.Run("active key accepted and persisted", func(t *testing.T) {
		service, _, tenantID, ownerID, nodeID := newService(t)
		req := baseRequest(tenantID, ownerID, nodeID)
		key := " ops_analysis "
		req.ScenarioTemplateKey = &key
		created, err := service.CreateProject(context.Background(), req)
		if err != nil {
			t.Fatalf("create project: %v", err)
		}
		if created.Project.ScenarioTemplateKey == nil || *created.Project.ScenarioTemplateKey != "ops_analysis" {
			t.Fatalf("expected bound key, got %#v", created.Project.ScenarioTemplateKey)
		}
	})

	t.Run("no key keeps today's behavior", func(t *testing.T) {
		service, _, tenantID, ownerID, nodeID := newService(t)
		created, err := service.CreateProject(context.Background(), baseRequest(tenantID, ownerID, nodeID))
		if err != nil {
			t.Fatalf("create project: %v", err)
		}
		if created.Project.ScenarioTemplateKey != nil {
			t.Fatalf("expected nil key, got %#v", created.Project.ScenarioTemplateKey)
		}
	})
}
