package project

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestEvaluatePreDispatchGatePassesReadyTask(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 30, 0, 0, time.UTC)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	taskID := uuid.MustParse("00000000-0000-0000-0000-000000000102")
	employeeID := uuid.MustParse("00000000-0000-0000-0000-000000000103")
	revisionID := uuid.MustParse("00000000-0000-0000-0000-000000000104")
	key := "inspect-db"

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:              projectID,
		ProjectTaskID:          taskID,
		AcceptedPlanRevisionID: &revisionID,
		PlannedTaskKey:         &key,
		SelectedEmployeeID:     employeeID,
		AttemptNo:              1,
		DispatchReason:         DispatchReasonDependencyUnlocked,
	}, PreDispatchGateSnapshot{
		Task: ProjectTask{
			ID:                        taskID,
			ProjectID:                 projectID,
			Status:                    ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			AcceptedPlanRevisionID:    &revisionID,
			PlannedTaskKey:            &key,
			MaxAttempts:               int32Ptr(3),
			AttemptCount:              0,
		},
		Employee: PreDispatchEmployeeSnapshot{
			ID:                  employeeID,
			IsProjectExecutor:   true,
			Status:              "active",
			PolicyAllowed:       true,
			RequiredLoadSlots:   1,
			AvailableLoadSlots:  1,
			ProfileSnapshotHash: "profile-hash",
		},
		Capabilities: PreDispatchCapabilitySnapshot{
			Required: []string{"database.read", "sql.analysis"},
			Matched:  []string{"database.read", "sql.analysis"},
		},
		Runtime: PreDispatchRuntimeSnapshot{
			PlacementPresent:        true,
			NodeOnline:              true,
			ProviderAvailable:       true,
			WorkspaceReady:          true,
			SlotAvailable:           true,
			ContractVersionAccepted: true,
		},
		Budget:  PreDispatchBudgetSnapshot{ProjectBudgetAllowed: true, TaskBudgetPresent: true},
		Context: PreDispatchContextSnapshot{RequiredRefsResolved: true, InjectionAllowed: true},
	}, now)

	require.Equal(t, PreDispatchGateStatusPassed, result.Status)
	require.Empty(t, result.Blockers)
	require.Nil(t, result.HumanActionRequest)
	require.Equal(t, "project-task:00000000-0000-0000-0000-000000000102:reason:dependency_unlocked:attempt:1:employee:00000000-0000-0000-0000-000000000103", result.IdempotencyKey)
	require.Len(t, result.DispatchToken, 64)
	require.Contains(t, result.CheckKeys(), "task.dispatchable")
	require.Contains(t, result.CheckKeys(), "runtime.ready")
}

func TestEvaluatePreDispatchGatePassesReadyEmployeeForProjectTask(t *testing.T) {
	now := time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Employee.Status = "ready"
	snapshot.Employee.PolicyAllowed = true

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusPassed, result.Status)
	require.Empty(t, result.Blockers)
	require.True(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateBlocksNoEligibleOnlineNode(t *testing.T) {
	now := time.Date(2026, 7, 8, 9, 30, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Runtime.Pinned = false
	snapshot.Runtime.NodeOnline = false

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusRetryLater, result.Status)
	require.Len(t, result.Blockers, 1)
	require.Equal(t, "runtime.no_eligible_online_node", result.Blockers[0].Key)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateBlocksPinnedNodeOffline(t *testing.T) {
	now := time.Date(2026, 7, 8, 9, 31, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Runtime.Pinned = true
	snapshot.Runtime.NodeOnline = false

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusBlocked, result.Status)
	require.Len(t, result.Blockers, 1)
	require.Equal(t, "runtime.pinned_node_offline", result.Blockers[0].Key)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateBlocksActiveAttempt(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 31, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	activeAttemptID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Task.CurrentAttemptID = &activeAttemptID
	snapshot.Task.AttemptCount = 1
	snapshot.ActiveAttempt = &PreDispatchAttemptSnapshot{ID: activeAttemptID, Status: ProjectTaskAttemptStatusRunning}

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          2,
		DispatchReason:     DispatchReasonRetry,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusBlocked, result.Status)
	require.Equal(t, "task.active_attempt_exists", result.Blockers[0].Key)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateWaitsForHumanWhenRiskApprovalMissing(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 32, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	risk := "high"
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Task.RiskLevel = &risk
	snapshot.Task.MaxAttempts = int32Ptr(1)
	snapshot.Risk = PreDispatchRiskSnapshot{HumanApprovalRequired: true, HumanApprovalGranted: false, Reason: "database.write"}

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusWaitingHuman, result.Status)
	require.NotNil(t, result.HumanActionRequest)
	require.Equal(t, PreDispatchHumanActionRiskApproval, result.HumanActionRequest.Type)
	require.Equal(t, HumanWaitReasonApprovalRequired, result.HumanActionRequest.WaitingReason)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateReturnsRetryLaterForRuntimeSlot(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 33, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Runtime.SlotAvailable = false
	snapshot.Runtime.RetryAfter = now.Add(2 * time.Minute)

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusRetryLater, result.Status)
	require.NotNil(t, result.RetryAfter)
	require.Equal(t, now.Add(2*time.Minute), *result.RetryAfter)
	require.Equal(t, "runtime.slot_unavailable", result.Blockers[0].Key)
}

func TestEvaluatePreDispatchGateIgnoresPlannerCapabilityClaims(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 34, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	// The planner used to be able to kill a task by naming a capability it
	// invented. Capability state is no longer a gate input at all.
	snapshot.Capabilities = PreDispatchCapabilitySnapshot{
		Required: []string{"database.write"},
	}

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusPassed, result.Status)
	require.Empty(t, result.Blockers)
	for _, check := range result.Checks {
		require.NotEqual(t, "capability.match", check.Key)
	}
}

func TestEvaluatePreDispatchGateBlocksDependencyNotReady(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 35, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	dependencyID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Dependencies = []PreDispatchDependencySnapshot{{
		TaskID:              dependencyID,
		Status:              ProjectTaskStatusPlanned,
		AcceptanceSatisfied: false,
	}}

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusBlocked, result.Status)
	require.Equal(t, "dependency.not_ready", result.Blockers[0].Key)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateWaitsForHumanWhenContextMissing(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 36, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Context = PreDispatchContextSnapshot{
		RequiredRefsResolved: false,
		InjectionAllowed:     true,
		MissingRefs:          []string{"prd", "customer-thread"},
	}

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusWaitingHuman, result.Status)
	require.Equal(t, "context.missing_required_refs", result.Blockers[0].Key)
	require.NotNil(t, result.HumanActionRequest)
	require.Equal(t, PreDispatchHumanActionMissingContext, result.HumanActionRequest.Type)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateRequiresReplanForEmployeeSnapshotMismatch(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 37, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Employee.ID = uuid.New()

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusReplanRequired, result.Status)
	require.Equal(t, "employee.snapshot_mismatch", result.Blockers[0].Key)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateRequiresReplanForAcceptedPlanRevisionDrift(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 38, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	inputRevisionID := uuid.New()
	currentRevisionID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Task.AcceptedPlanRevisionID = &currentRevisionID

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:              projectID,
		ProjectTaskID:          taskID,
		AcceptedPlanRevisionID: &inputRevisionID,
		SelectedEmployeeID:     employeeID,
		AttemptNo:              1,
		DispatchReason:         DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusReplanRequired, result.Status)
	require.Equal(t, "task.accepted_plan_revision_changed", result.Blockers[0].Key)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateRequiresReplanForPlannedTaskKeyDrift(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 39, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	inputKey := "inspect-db"
	currentKey := "inspect-runtime"
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Task.PlannedTaskKey = &currentKey

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		PlannedTaskKey:     &inputKey,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusReplanRequired, result.Status)
	require.Equal(t, "task.planned_task_key_changed", result.Blockers[0].Key)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateNormalizesReplanOverWaitingHuman(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 40, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Tools.MissingBindings = []string{"jira"}
	snapshot.Runtime.ContractVersionAccepted = false

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusReplanRequired, result.Status)
	require.Equal(t, "runtime.contract_version_unsupported", result.Blockers[0].Key)
	require.Nil(t, result.HumanActionRequest)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateNormalizesBlockedOverWaitingHuman(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 41, 0, 0, time.UTC)
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	snapshot := readyPreDispatchGateSnapshot(projectID, taskID, employeeID)
	snapshot.Risk = PreDispatchRiskSnapshot{HumanApprovalRequired: true, HumanApprovalGranted: false, Reason: "database.write"}
	snapshot.Context.InjectionAllowed = false

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, snapshot, now)

	require.Equal(t, PreDispatchGateStatusBlocked, result.Status)
	require.Equal(t, "context.injection_denied", result.Blockers[0].Key)
	require.Nil(t, result.HumanActionRequest)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateSanitizesNestedDetailsAndHumanContext(t *testing.T) {
	details := sanitizeGateDetails(map[string]any{
		"safe": "kept",
		"nested": map[string]any{
			"api_token": "secret-token",
			"items": []any{
				map[string]any{
					"db_password": "secret-password",
					"label":       "primary",
				},
			},
		},
		"connection_string": "postgres://user:password@localhost/db",
	})

	require.Equal(t, "kept", details["safe"])
	require.Equal(t, "[redacted]", details["connection_string"])
	nested := details["nested"].(map[string]any)
	require.Equal(t, "[redacted]", nested["api_token"])
	items := nested["items"].([]any)
	item := items[0].(map[string]any)
	require.Equal(t, "[redacted]", item["db_password"])
	require.Equal(t, "primary", item["label"])

	request := &PreDispatchHumanActionRequest{Context: map[string]any{
		"source": "predispatch_gate",
		"nested": map[string]any{
			"refresh_token": "secret-token",
			"labels": []any{
				map[string]any{
					"password": "secret-password",
					"name":     "context",
				},
			},
		},
	}}

	sanitizeHumanActionRequest(request)

	require.Equal(t, "predispatch_gate", request.Context["source"])
	requestNested := request.Context["nested"].(map[string]any)
	require.Equal(t, "[redacted]", requestNested["refresh_token"])
	labels := requestNested["labels"].([]any)
	label := labels[0].(map[string]any)
	require.Equal(t, "[redacted]", label["password"])
	require.Equal(t, "context", label["name"])
}

func readyPreDispatchGateSnapshot(projectID, taskID, employeeID uuid.UUID) PreDispatchGateSnapshot {
	return PreDispatchGateSnapshot{
		Task: ProjectTask{
			ID:                        taskID,
			ProjectID:                 projectID,
			Status:                    ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			MaxAttempts:               int32Ptr(3),
		},
		Employee: PreDispatchEmployeeSnapshot{
			ID:                 employeeID,
			IsProjectExecutor:  true,
			Status:             "active",
			PolicyAllowed:      true,
			RequiredLoadSlots:  1,
			AvailableLoadSlots: 1,
		},
		Runtime: PreDispatchRuntimeSnapshot{
			PlacementPresent:        true,
			NodeOnline:              true,
			ProviderAvailable:       true,
			WorkspaceReady:          true,
			SlotAvailable:           true,
			ContractVersionAccepted: true,
		},
		Budget:  PreDispatchBudgetSnapshot{ProjectBudgetAllowed: true, TaskBudgetPresent: true},
		Context: PreDispatchContextSnapshot{RequiredRefsResolved: true, InjectionAllowed: true},
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}
