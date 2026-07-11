package employee

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	cpruntime "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/skill"
)

func TestRunServiceCreateRunRejectsActiveRun(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.activeRun = validRunServiceRun(DigitalEmployeeRunStatusRunning)
	existingKey := "existing-key"
	existingFingerprint := "existing-fingerprint"
	repo.activeRun.IdempotencyKey = &existingKey
	repo.activeRun.IdempotencyFingerprint = &existingFingerprint
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if repo.createdRunCount != 0 {
		t.Fatalf("expected active conflict not to create run, got %d creates", repo.createdRunCount)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected active conflict not to dispatch, got %d commands", len(dispatcher.commands))
	}
}

func TestRunServiceCreateRunReapsStaleDispatchingRun(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	// An abandoned dispatching run: command sent, runtime never confirmed start,
	// row untouched for far longer than staleDispatchTTL.
	staleRun := validRunServiceRun(DigitalEmployeeRunStatusDispatching)
	staleRun.UpdatedAt = time.Now().UTC().Add(-10 * time.Minute)
	repo.activeRun = staleRun
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	audit := &fakeRunServiceAuditLogger{}
	service := mustNewRunService(t, repo, dispatcher, audit)

	run, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if err != nil {
		t.Fatalf("expected stale run reaped and new run created, got error: %v", err)
	}
	if run.Status != DigitalEmployeeRunStatusDispatching {
		t.Fatalf("expected new run to reach dispatching, got %s", run.Status)
	}
	// The abandoned run must have been marked failed so it frees the active slot.
	var reaped *UpdateRunStatusRequest
	for i := range repo.statusUpdates {
		if repo.statusUpdates[i].RunID == staleRun.ID {
			reaped = &repo.statusUpdates[i]
		}
	}
	if reaped == nil {
		t.Fatalf("expected stale run to be reaped via UpdateRunStatus, got %#v", repo.statusUpdates)
	}
	if reaped.Status != DigitalEmployeeRunStatusFailed || reaped.ErrorCode == nil || *reaped.ErrorCode != "dispatch_stale" {
		t.Fatalf("expected stale run reaped as failed/dispatch_stale, got %#v", reaped)
	}
	if len(repo.events) == 0 || repo.events[0].EventType != "run_reaped_stale" {
		t.Fatalf("expected run_reaped_stale lifecycle event, got %#v", repo.events)
	}
	if repo.createdRunCount != 1 || len(dispatcher.commands) != 1 {
		t.Fatalf("expected a fresh run created and dispatched after reap, got %d creates / %d commands", repo.createdRunCount, len(dispatcher.commands))
	}
}

func TestRunServiceCreateRunDoesNotReapRecentDispatchingRun(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	// A dispatching run that is still within the staleness window — not abandoned.
	repo.activeRun = validRunServiceRun(DigitalEmployeeRunStatusDispatching)
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for a recent dispatching run, got %v", err)
	}
	for _, update := range repo.statusUpdates {
		if update.RunID == repo.activeRun.ID {
			t.Fatalf("expected recent dispatching run not to be reaped, got status update %#v", update)
		}
	}
	if repo.createdRunCount != 0 {
		t.Fatalf("expected no new run created, got %d", repo.createdRunCount)
	}
}

func TestRunServiceCreateRunDoesNotReapStaleRunningRun(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	// A running run the runtime has confirmed — never reaped by time alone,
	// even if its row is old.
	runningRun := validRunServiceRun(DigitalEmployeeRunStatusRunning)
	runningRun.UpdatedAt = time.Now().UTC().Add(-10 * time.Minute)
	repo.activeRun = runningRun
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for a stale running run, got %v", err)
	}
	for _, update := range repo.statusUpdates {
		if update.RunID == runningRun.ID {
			t.Fatalf("expected running run not to be reaped, got status update %#v", update)
		}
	}
}

func TestRunServiceCreateRunDispatchesStartSession(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	audit := &fakeRunServiceAuditLogger{}
	service := mustNewRunService(t, repo, dispatcher, audit)
	req := validCreateRunServiceRequest()

	run, err := service.CreateRun(context.Background(), req)

	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.Status != DigitalEmployeeRunStatusDispatching {
		t.Fatalf("expected dispatching run, got %s", run.Status)
	}
	if repo.createdRunCount != 1 {
		t.Fatalf("expected one created run, got %d", repo.createdRunCount)
	}
	if repo.createRunRequests[0].RunStatus != DigitalEmployeeRunStatusQueued {
		t.Fatalf("expected persisted run to start queued, got %s", repo.createRunRequests[0].RunStatus)
	}
	if len(repo.commandReceipts) != 1 || repo.commandReceipts[0].Status != "pending" {
		t.Fatalf("expected pending receipt before dispatch, got %#v", repo.commandReceipts)
	}
	if len(repo.receiptUpdates) != 1 || repo.receiptUpdates[0].Status != "dispatched" {
		t.Fatalf("expected dispatched receipt update, got %#v", repo.receiptUpdates)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched command, got %d", len(dispatcher.commands))
	}
	dispatched := dispatcher.commands[0]
	if dispatched.nodeID != repo.preflight.NodeID {
		t.Fatalf("expected dispatch to %s, got %s", repo.preflight.NodeID, dispatched.nodeID)
	}
	if dispatched.command.ID != run.CommandID || dispatched.command.Type != "start_session" {
		t.Fatalf("unexpected dispatched command: %#v", dispatched.command)
	}

	payload := commandPayload(t, dispatched.command)
	required := []string{
		"provider_run_protocol",
		"tenant_id",
		"task_id",
		"run_id",
		"command_id",
		"digital_employee_id",
		"execution_instance_id",
		"runtime_node_id",
		"node_id",
		"provider_type",
		"agent_home_dir",
		"objective",
		"prompt",
		"input",
		"context_refs",
		"artifact_refs",
		"output_schema",
		"allowed_actions",
		"forbidden_actions",
		"secret_refs",
		"timeout_sec",
		"grace_sec",
		"workspace_policy",
		"session_policy",
		"skills",
		"mcp_servers",
		"metadata",
	}
	for _, key := range required {
		if _, ok := payload[key]; !ok {
			t.Fatalf("start payload missing %s: %#v", key, payload)
		}
	}
	if payload["provider_run_protocol"] != "provider-run/v1" {
		t.Fatalf("unexpected provider_run_protocol: %#v", payload["provider_run_protocol"])
	}
	if payload["objective"] != "修复失败测试" || payload["prompt"] != "请先复现再修复" {
		t.Fatalf("expected trimmed objective and prompt, got objective=%#v prompt=%#v", payload["objective"], payload["prompt"])
	}
	if payload["input"] != "请先复现再修复" {
		t.Fatalf("expected start payload input to mirror prompt, got %#v", payload["input"])
	}
	sessionPolicy, ok := payload["session_policy"].(map[string]any)
	if !ok {
		t.Fatalf("expected session_policy object, got %#v", payload["session_policy"])
	}
	if sessionPolicy["mode"] != "new" || sessionPolicy["recoverable"] != true || sessionPolicy["resume"] != true {
		t.Fatalf("expected runtime-compatible session policy defaults, got %#v", sessionPolicy)
	}
	persistedSessionPolicy, ok := repo.createRunRequests[0].Params["session_policy"].(map[string]any)
	if !ok {
		t.Fatalf("expected persisted session_policy object, got %#v", repo.createRunRequests[0].Params["session_policy"])
	}
	if persistedSessionPolicy["mode"] != "new" || persistedSessionPolicy["recoverable"] != true || persistedSessionPolicy["resume"] != true {
		t.Fatalf("expected persisted runtime-compatible session policy defaults, got %#v", persistedSessionPolicy)
	}
	for _, key := range []string{"skills", "mcp_servers"} {
		values, ok := payload[key].([]any)
		if !ok || len(values) != 0 {
			t.Fatalf("expected start payload %s to be explicit empty array, got %#v", key, payload[key])
		}
	}
	if len(repo.events) != 1 || repo.events[0].EventType != "run_dispatched" {
		t.Fatalf("expected run_dispatched event, got %#v", repo.events)
	}
	if repo.events[0].SequenceNumber != runDispatchedLifecycleSequence {
		t.Fatalf("expected lifecycle run_dispatched sequence %d, got %d", runDispatchedLifecycleSequence, repo.events[0].SequenceNumber)
	}
	if len(audit.events) != 1 || audit.events[0].eventType != "digital_employee_run_created" || audit.events[0].action != "employee.run.create" {
		t.Fatalf("expected create audit event, got %#v", audit.events)
	}
}

func TestStartProjectTaskRunResolvesNodeThenUsesPreflightForNode(t *testing.T) {
	repo := newFakeRunServiceRepository()
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000901")
	demandID := uuid.MustParse("00000000-0000-0000-0000-000000000902")
	projectTaskID := uuid.MustParse("00000000-0000-0000-0000-000000000903")
	attemptID := uuid.MustParse("00000000-0000-0000-0000-000000000904")
	dispatchUserID := uuid.MustParse("00000000-0000-0000-0000-000000000905")
	idempotencyKey := "project-task:" + projectTaskID.String() + ":attempt:1:dispatch"
	repo.projectTaskPreflight = validProjectTaskRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.projectTaskPreflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)
	resolver := &fakeProjectTaskNodeResolver{nodeID: repo.projectTaskPreflight.RuntimeNodeID}
	service.SetProjectTaskNodeResolver(resolver)

	result, err := service.StartProjectTaskRun(context.Background(), StartProjectTaskRunRequest{
		TenantID:             runServiceTenantID,
		ProjectID:            projectID,
		DemandID:             demandID,
		ProjectTaskID:        projectTaskID,
		ProjectTaskAttemptID: attemptID,
		DigitalEmployeeID:    runServiceEmployeeID,
		DispatchUserID:       dispatchUserID,
		Objective:            "整理上线证据",
		Prompt:               "请完成项目任务",
		IdempotencyKey:       idempotencyKey,
		Metadata:             map[string]any{"source": "project_task_dispatch"},
		WorkspaceMode:        "branch",
		BaseRef:              "main",
		ProjectGit:           map[string]any{"url": "https://example.com/acme/app.git", "default_branch": "main"},
	})

	if err != nil {
		t.Fatalf("start project task run: %v", err)
	}
	if result.RunID == uuid.Nil || result.RuntimeTaskID == uuid.Nil {
		t.Fatalf("expected run ids in result, got %#v", result)
	}
	if result.RuntimeNodeID != repo.projectTaskPreflight.RuntimeNodeID || result.NodeID != repo.projectTaskPreflight.NodeID || result.ProviderType != "codex" {
		t.Fatalf("expected placement/provider result from preflight, got %#v", result)
	}
	if resolver.calls != 1 {
		t.Fatalf("expected node resolver to be called exactly once, got %d", resolver.calls)
	}
	if resolver.lastReq.TenantID != runServiceTenantID || resolver.lastReq.ProjectID != projectID ||
		resolver.lastReq.DigitalEmployeeID != runServiceEmployeeID || resolver.lastReq.ProjectTaskID != projectTaskID {
		t.Fatalf("expected node resolver called with tenant/project/employee/task ids, got %#v", resolver.lastReq)
	}
	if repo.projectTaskPreflightForNodeEmployeeID != runServiceEmployeeID || repo.projectTaskPreflightForNodeNodeID != resolver.nodeID {
		t.Fatalf("expected dispatch preflight lookup by resolved node, got employee=%s node=%s", repo.projectTaskPreflightForNodeEmployeeID, repo.projectTaskPreflightForNodeNodeID)
	}
	if len(repo.createRunRequests) != 1 {
		t.Fatalf("expected one created run, got %d", len(repo.createRunRequests))
	}
	createReq := repo.createRunRequests[0]
	if createReq.IdempotencyKey == nil || *createReq.IdempotencyKey != idempotencyKey {
		t.Fatalf("expected attempt-level idempotency key %q, got %#v", idempotencyKey, createReq.IdempotencyKey)
	}
	if createReq.ProviderType != "codex" || createReq.RuntimeNodeID != repo.projectTaskPreflight.RuntimeNodeID || createReq.NodeID != repo.projectTaskPreflight.NodeID {
		t.Fatalf("expected create run to use project placement/provider, got %#v", createReq)
	}
	if createReq.ExecutionInstanceID == uuid.Nil {
		t.Fatalf("expected compatibility execution_instance_id")
	}
	if createReq.ExecutionInstanceID == runServiceExecutionInstanceID {
		t.Fatalf("project task run must not reuse legacy execution instance binding")
	}
	if createReq.ExecutionInstanceID != projectTaskCompatibilityExecutionInstanceID(StartProjectTaskRunRequest{
		TenantID:             runServiceTenantID,
		ProjectID:            projectID,
		ProjectTaskID:        projectTaskID,
		ProjectTaskAttemptID: attemptID,
		DigitalEmployeeID:    runServiceEmployeeID,
	}) {
		t.Fatalf("expected deterministic compatibility execution_instance_id, got %s", createReq.ExecutionInstanceID)
	}
	if _, err := uuid.Parse(createReq.ExecutionInstanceID.String()); err != nil {
		t.Fatalf("expected uuid-like execution_instance_id, got %s", createReq.ExecutionInstanceID)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched command, got %d", len(dispatcher.commands))
	}
	payload := commandPayload(t, dispatcher.commands[0].command)
	if payload["provider_type"] != "codex" || payload["runtime_node_id"] != repo.projectTaskPreflight.RuntimeNodeID.String() || payload["node_id"] != repo.projectTaskPreflight.NodeID {
		t.Fatalf("expected start payload to use project placement/provider, got %#v", payload)
	}
	agentHome, _ := payload["agent_home_dir"].(string)
	for _, want := range []string{
		repo.projectTaskPreflight.WorkspaceBaseDir,
		projectID.String(),
		projectTaskID.String(),
		attemptID.String(),
		runServiceEmployeeID.String(),
	} {
		if !strings.Contains(agentHome, want) {
			t.Fatalf("expected derived agent_home_dir %q to contain %q", agentHome, want)
		}
	}
	if payload["execution_instance_id"] != createReq.ExecutionInstanceID.String() {
		t.Fatalf("expected payload execution_instance_id to match created run, got %#v want %s", payload["execution_instance_id"], createReq.ExecutionInstanceID)
	}
	metadata, ok := payload["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object, got %#v", payload["metadata"])
	}
	if metadata["source"] != "project_task_dispatch" || metadata["project_task_attempt_id"] != attemptID.String() {
		t.Fatalf("expected project task dispatch attempt metadata, got %#v", metadata)
	}
	if metadata["provider_type"] != "codex" {
		t.Fatalf("expected provider_type metadata, got %#v", metadata)
	}
}

// TestStartProjectTaskRunInjectsLineageRootSessionID exercises the dispatch
// path for a revision task: FindProviderSessionForTaskRoot is called with
// the resolved lineage root (not project_task_id), and a hit is surfaced as
// metadata.provider_session_id alongside metadata.revision_root_task_id.
func TestStartProjectTaskRunInjectsLineageRootSessionID(t *testing.T) {
	repo := newFakeRunServiceRepository()
	projectTaskID := uuid.MustParse("00000000-0000-0000-0000-000000000903")
	rootTaskID := uuid.MustParse("00000000-0000-0000-0000-000000000900")
	repo.projectTaskPreflight = validProjectTaskRunServicePreflight()
	repo.lineageRoot = rootTaskID
	repo.providerSessionForRoot = map[uuid.UUID]string{rootTaskID: "provider-session-existing"}
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.projectTaskPreflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)
	service.SetProjectTaskNodeResolver(&fakeProjectTaskNodeResolver{nodeID: repo.projectTaskPreflight.RuntimeNodeID})

	_, err := service.StartProjectTaskRun(context.Background(), StartProjectTaskRunRequest{
		TenantID:             runServiceTenantID,
		ProjectID:            uuid.New(),
		DemandID:             uuid.New(),
		ProjectTaskID:        projectTaskID,
		ProjectTaskAttemptID: uuid.New(),
		DigitalEmployeeID:    runServiceEmployeeID,
		DispatchUserID:       uuid.New(),
		Objective:            "修订上一次的任务",
	})
	if err != nil {
		t.Fatalf("start project task run: %v", err)
	}

	if len(repo.lineageRootCalls) != 1 || repo.lineageRootCalls[0].ProjectTaskID != projectTaskID || repo.lineageRootCalls[0].TenantID != runServiceTenantID {
		t.Fatalf("expected lineage root resolution for the dispatched task, got %#v", repo.lineageRootCalls)
	}
	if len(repo.providerSessionForRootCalls) != 1 {
		t.Fatalf("expected exactly one session lookup, got %#v", repo.providerSessionForRootCalls)
	}
	lookup := repo.providerSessionForRootCalls[0]
	if lookup.TaskRootID != rootTaskID || lookup.EmployeeID != runServiceEmployeeID || lookup.TenantID != runServiceTenantID {
		t.Fatalf("expected session lookup keyed by lineage root/employee/tenant, got %#v", lookup)
	}

	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched command, got %d", len(dispatcher.commands))
	}
	payload := commandPayload(t, dispatcher.commands[0].command)
	metadata, ok := payload["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object, got %#v", payload["metadata"])
	}
	if metadata["revision_root_task_id"] != rootTaskID.String() {
		t.Fatalf("expected metadata revision_root_task_id %q, got %#v", rootTaskID.String(), metadata["revision_root_task_id"])
	}
	if metadata["provider_session_id"] != "provider-session-existing" {
		t.Fatalf("expected metadata provider_session_id from lineage-root session hit, got %#v", metadata["provider_session_id"])
	}
}

// TestStartProjectTaskRunOmitsSessionIDWhenRootHasNoSession covers a fresh
// (non-revision) task dispatch for the same employee: the lineage root is
// the task's own id, no session exists yet for that root, and metadata must
// carry the root without a provider_session_id.
func TestStartProjectTaskRunOmitsSessionIDWhenRootHasNoSession(t *testing.T) {
	repo := newFakeRunServiceRepository()
	projectTaskID := uuid.MustParse("00000000-0000-0000-0000-000000000904")
	repo.projectTaskPreflight = validProjectTaskRunServicePreflight()
	// No repo.lineageRoot override: resolver echoes back projectTaskID
	// (root = self), matching a task with no revision lineage.
	// No repo.providerSessionForRoot entry: no session exists for this root.
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.projectTaskPreflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)
	service.SetProjectTaskNodeResolver(&fakeProjectTaskNodeResolver{nodeID: repo.projectTaskPreflight.RuntimeNodeID})

	_, err := service.StartProjectTaskRun(context.Background(), StartProjectTaskRunRequest{
		TenantID:             runServiceTenantID,
		ProjectID:            uuid.New(),
		DemandID:             uuid.New(),
		ProjectTaskID:        projectTaskID,
		ProjectTaskAttemptID: uuid.New(),
		DigitalEmployeeID:    runServiceEmployeeID,
		DispatchUserID:       uuid.New(),
		Objective:            "全新任务",
	})
	if err != nil {
		t.Fatalf("start project task run: %v", err)
	}

	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched command, got %d", len(dispatcher.commands))
	}
	payload := commandPayload(t, dispatcher.commands[0].command)
	metadata, ok := payload["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object, got %#v", payload["metadata"])
	}
	if metadata["revision_root_task_id"] != projectTaskID.String() {
		t.Fatalf("expected metadata revision_root_task_id to fall back to the task's own id %q, got %#v", projectTaskID.String(), metadata["revision_root_task_id"])
	}
	if _, ok := metadata["provider_session_id"]; ok {
		t.Fatalf("expected no provider_session_id when no session exists for the lineage root, got %#v", metadata["provider_session_id"])
	}
}

// TestShouldAttemptSessionResume covers the decision StartProjectTaskRun
// uses to gate the FindProviderSessionForTaskRoot lookup. Project task
// dispatch currently always normalizes session_policy to
// {"mode":"new","recoverable":true} (projectTaskRunPreflightToRunPreflight),
// so this exercises the decision directly rather than requiring an
// unreachable ephemeral preflight through the full dispatch path.
func TestShouldAttemptSessionResume(t *testing.T) {
	tests := []struct {
		name   string
		policy map[string]any
		want   bool
	}{
		{
			name:   "default normalized policy attempts resume",
			policy: runtimeSessionPolicyPayload(map[string]any{"resume": true}),
			want:   true,
		},
		{
			name:   "explicit ephemeral mode skips resume",
			policy: runtimeSessionPolicyPayload(map[string]any{"mode": "ephemeral"}),
			want:   false,
		},
		{
			name:   "ephemeral mode is case-insensitive",
			policy: runtimeSessionPolicyPayload(map[string]any{"mode": "Ephemeral"}),
			want:   false,
		},
		{
			name:   "explicit recoverable=false skips resume",
			policy: runtimeSessionPolicyPayload(map[string]any{"recoverable": false}),
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAttemptSessionResume(tc.policy); got != tc.want {
				t.Fatalf("shouldAttemptSessionResume(%#v) = %v, want %v", tc.policy, got, tc.want)
			}
		})
	}
}

func TestStartProjectTaskRunPropagatesNodeResolverError(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.projectTaskPreflight = validProjectTaskRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	service := mustNewRunService(t, repo, dispatcher)
	resolveErr := errors.New("no eligible online node")
	service.SetProjectTaskNodeResolver(&fakeProjectTaskNodeResolver{err: resolveErr})

	_, err := service.StartProjectTaskRun(context.Background(), StartProjectTaskRunRequest{
		TenantID:             runServiceTenantID,
		ProjectID:            uuid.New(),
		DemandID:             uuid.New(),
		ProjectTaskID:        uuid.New(),
		ProjectTaskAttemptID: uuid.New(),
		DigitalEmployeeID:    runServiceEmployeeID,
		DispatchUserID:       uuid.New(),
		Objective:            "整理上线证据",
	})
	if !errors.Is(err, resolveErr) {
		t.Fatalf("expected resolver error to propagate, got %v", err)
	}
	if len(repo.createRunRequests) != 0 {
		t.Fatalf("expected no run to be created when node resolution fails, got %#v", repo.createRunRequests)
	}
}

func TestStartProjectTaskRunRequiresNodeResolver(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.projectTaskPreflight = validProjectTaskRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.StartProjectTaskRun(context.Background(), StartProjectTaskRunRequest{
		TenantID:             runServiceTenantID,
		ProjectID:            uuid.New(),
		DemandID:             uuid.New(),
		ProjectTaskID:        uuid.New(),
		ProjectTaskAttemptID: uuid.New(),
		DigitalEmployeeID:    runServiceEmployeeID,
		DispatchUserID:       uuid.New(),
		Objective:            "整理上线证据",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput when no node resolver is configured, got %v", err)
	}
}

func TestValidateProjectTaskRunPreflightRejectsMissingPlacementSessionProviderAndWorkspaceBase(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*StartProjectTaskRunPreflight)
		wantError error
		contains  string
	}{
		{
			name: "missing placement",
			mutate: func(preflight *StartProjectTaskRunPreflight) {
				preflight.RuntimeNodeID = uuid.Nil
			},
			wantError: ErrInvalidInput,
			contains:  "runtime_node_id",
		},
		{
			name: "missing session",
			mutate: func(preflight *StartProjectTaskRunPreflight) {
				preflight.RuntimeSessionActive = false
			},
			wantError: ErrRuntimeUnavailable,
			contains:  "runtime session",
		},
		{
			name: "missing provider",
			mutate: func(preflight *StartProjectTaskRunPreflight) {
				preflight.ProviderType = ""
			},
			wantError: ErrInvalidInput,
			contains:  "provider_type",
		},
		{
			name: "unhealthy provider",
			mutate: func(preflight *StartProjectTaskRunPreflight) {
				preflight.ProviderHealthy = false
			},
			wantError: ErrProviderUnavailable,
			contains:  "provider capability",
		},
		{
			name: "missing workspace base",
			mutate: func(preflight *StartProjectTaskRunPreflight) {
				preflight.WorkspaceBaseDir = ""
			},
			wantError: ErrInvalidInput,
			contains:  "workspace_base_dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preflight := validProjectTaskRunServicePreflight()
			tt.mutate(&preflight)
			err := validateProjectTaskRunPreflight(preflight)
			if !errors.Is(err, tt.wantError) || !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("expected %v containing %q, got %v", tt.wantError, tt.contains, err)
			}
		})
	}
}

func TestRunServiceCreateRunRejectsSkillWithMissingToolDependency(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.runtimeSkills = []skill.SkillRuntimeRecord{{
		ID: uuid.New(), Slug: "github", ArchiveObjectRef: "s3://bucket/github.zip",
		ArchiveChecksum: strings.Repeat("a", 64), ArchiveSizeBytes: 10, ArchiveFileCount: 1,
		RuntimeDependencies: skill.SkillRuntimeDependencies{Tools: []string{"gh"}},
	}}
	repo.runtimeCapabilities = []cpruntime.RuntimeCapability{{
		CapabilityType: "tool",
		CapabilityKey:  "git",
		Available:      true,
	}}
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := newRunServiceWithListers(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if err == nil || !strings.Contains(err.Error(), "gh") {
		t.Fatalf("expected missing gh dependency, got %v", err)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected missing dependency not to dispatch, got %#v", dispatcher.commands)
	}
}

func TestRunServiceStartSessionPayloadIncludesLoadableSkillsAndEnvironment(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.runtimeCapabilities = []cpruntime.RuntimeCapability{{
		CapabilityType: "tool",
		CapabilityKey:  "gh",
		Available:      true,
	}}
	repo.runtimeSkills = []skill.SkillRuntimeRecord{{
		ID: uuid.New(), Slug: "github", ArchiveObjectRef: "s3://bucket/github.zip",
		ArchiveChecksum: strings.Repeat("b", 64), ArchiveSizeBytes: 10, ArchiveFileCount: 1,
		RuntimeDependencies: skill.SkillRuntimeDependencies{Tools: []string{"gh"}, Env: []string{"GH_TOKEN"}},
	}}
	repo.runtimeEnv = []RuntimeEnvironmentVariablePayload{{
		Name:      "GH_TOKEN",
		Value:     "plain-token",
		Sensitive: true,
	}}
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := newRunServiceWithListers(t, repo, dispatcher)

	run, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched command, got %#v", dispatcher.commands)
	}
	payload := commandPayload(t, dispatcher.commands[0].command)
	environment, ok := payload["environment"].([]any)
	if !ok || len(environment) != 1 {
		t.Fatalf("expected one env payload, got %#v", payload["environment"])
	}
	env := environment[0].(map[string]any)
	if env["name"] != "GH_TOKEN" || env["value"] != "plain-token" || env["sensitive"] != true {
		t.Fatalf("unexpected env payload: %#v", env)
	}
	if got := payload["skills"].([]any); len(got) != 1 {
		t.Fatalf("expected one skill payload, got %#v", payload["skills"])
	}
	if run.CommandID == "" {
		t.Fatal("expected command id")
	}
}

func TestRunServiceCreateRunReportsPendingRuntimeWhenNodeHasNotReported(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.runtimeSkills = []skill.SkillRuntimeRecord{{
		ID: uuid.New(), Slug: "github", ArchiveObjectRef: "s3://bucket/github.zip",
		ArchiveChecksum: strings.Repeat("c", 64), ArchiveSizeBytes: 10, ArchiveFileCount: 1,
		RuntimeDependencies: skill.SkillRuntimeDependencies{Tools: []string{"gh"}},
	}}
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := newRunServiceWithListers(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if err == nil || !strings.Contains(err.Error(), "pending_runtime") {
		t.Fatalf("expected pending_runtime failure, got %v", err)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected pending runtime not to dispatch, got %#v", dispatcher.commands)
	}
}

func TestRunServiceRuntimeEventPayloadRedactsEnvironmentValues(t *testing.T) {
	redacted := redactRuntimeEventPayload(map[string]any{
		"environment": []any{
			map[string]any{"name": "GH_TOKEN", "value": "plain-token", "sensitive": true},
		},
	})

	environment, ok := redacted["environment"].([]any)
	if !ok || len(environment) != 1 {
		t.Fatalf("expected one redacted environment entry, got %#v", redacted["environment"])
	}
	entry := environment[0].(map[string]any)
	if entry["name"] != "GH_TOKEN" || entry["sensitive"] != true {
		t.Fatalf("expected name and sensitive preserved, got %#v", entry)
	}
	if entry["value"] != "[redacted]" {
		t.Fatalf("expected environment value redacted, got %#v", entry)
	}
}

func TestRunServiceRuntimeEventPayloadForPersistenceOmitsRuntimeLocalPaths(t *testing.T) {
	redacted := redactRuntimeEventPayloadForPersistence(map[string]any{
		"agent_home_dir": "/runtime/employees/employee-1",
		"nested": map[string]any{
			"workspace_path": "/runtime/workspaces/project",
			"keep":           "value",
		},
	})

	if _, ok := redacted["agent_home_dir"]; ok {
		t.Fatalf("expected agent_home_dir to be omitted, got %#v", redacted)
	}
	nested := redacted["nested"].(map[string]any)
	if _, ok := nested["workspace_path"]; ok {
		t.Fatalf("expected workspace_path to be omitted, got %#v", nested)
	}
	if nested["keep"] != "value" {
		t.Fatalf("expected non-path value preserved, got %#v", nested)
	}
}

func TestRunServiceCreateRunEnrichesProjectTaskAttemptMetadata(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)
	req := validCreateRunServiceRequest()
	req.Metadata = map[string]any{
		"source":                   "project_task_dispatch",
		"project_task_id":          "55555555-5555-4555-8555-555555555555",
		"project_task_attempt_id":  "66666666-6666-4666-8666-666666666666",
		"project_task_lease_token": "lease-token-1",
		"runtime_node_id":          "stale-runtime-node",
		"handoff_contract":         map[string]any{"completion_path": "project_task_attempt_writeback"},
	}

	_, err := service.CreateRun(context.Background(), req)

	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched command, got %d", len(dispatcher.commands))
	}
	payload := commandPayload(t, dispatcher.commands[0].command)
	metadata, ok := payload["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object, got %#v", payload["metadata"])
	}
	if metadata["runtime_node_id"] != repo.preflight.RuntimeNodeID.String() {
		t.Fatalf("expected runtime_node_id from preflight, got %#v", metadata["runtime_node_id"])
	}
	if metadata["execution_context_packet_version"] != "v1" {
		t.Fatalf("expected default execution_context_packet_version, got %#v", metadata["execution_context_packet_version"])
	}
	if metadata["project_task_attempt_id"] != "66666666-6666-4666-8666-666666666666" ||
		metadata["project_task_lease_token"] != "lease-token-1" {
		t.Fatalf("expected attempt metadata to be preserved, got %#v", metadata)
	}
}

func TestRunServiceStartSessionPayloadAndIdempotencyFingerprintIncludeWorkspaceMetadata(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)
	req := validCreateRunServiceRequest()
	req.Metadata = map[string]any{
		"source":         "project_task_dispatch",
		"workspace_mode": "branch",
		"base_ref":       "main",
		"project_git": map[string]any{
			"url":            "https://github.com/acme/app.git",
			"default_branch": "main",
			"scope":          []any{"apps/web", "packages/shared"},
		},
	}

	_, err := service.CreateRun(context.Background(), req)

	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if len(repo.createRunRequests) != 1 {
		t.Fatalf("expected one create run request, got %d", len(repo.createRunRequests))
	}
	paramsMetadata, ok := repo.createRunRequests[0].Params["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected params metadata object, got %#v", repo.createRunRequests[0].Params["metadata"])
	}
	requireWorkspaceMetadata(t, paramsMetadata)
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched command, got %d", len(dispatcher.commands))
	}
	payload := commandPayload(t, dispatcher.commands[0].command)
	payloadMetadata, ok := payload["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload metadata object, got %#v", payload["metadata"])
	}
	requireWorkspaceMetadata(t, payloadMetadata)

	sameFingerprint, err := computeRunIdempotencyFingerprint(req, strings.TrimSpace(req.Objective), strings.TrimSpace(req.Prompt), repo.preflight)
	if err != nil {
		t.Fatalf("compute fingerprint: %v", err)
	}
	changedReq := req
	changedReq.Metadata = cloneMap(req.Metadata)
	changedReq.Metadata["workspace_mode"] = "readonly"
	changedFingerprint, err := computeRunIdempotencyFingerprint(changedReq, strings.TrimSpace(changedReq.Objective), strings.TrimSpace(changedReq.Prompt), repo.preflight)
	if err != nil {
		t.Fatalf("compute changed fingerprint: %v", err)
	}
	if repo.createRunRequests[0].IdempotencyFingerprint == nil || *repo.createRunRequests[0].IdempotencyFingerprint != sameFingerprint {
		t.Fatalf("expected persisted fingerprint to include workspace metadata")
	}
	if sameFingerprint == changedFingerprint {
		t.Fatalf("expected workspace metadata change to alter idempotency fingerprint")
	}
}

func TestRunServiceCreateRunDispatchesStartSessionWithoutEmployeeWorkspaceFiles(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	req := validCreateRunServiceRequest()
	req.Metadata = map[string]any{
		"source":          "project_task_dispatch",
		"project_id":      "33333333-3333-4333-8333-333333333333",
		"project_task_id": "44444444-4444-4444-8444-444444444444",
	}

	run, err := service.CreateRun(context.Background(), req)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched command, got %d", len(dispatcher.commands))
	}

	payload := commandPayload(t, dispatcher.commands[0].command)
	if payload["run_id"] != run.ID.String() {
		t.Fatalf("run id mismatch in payload: %#v", payload)
	}
	if payload["agent_home_dir"] != repo.preflight.AgentHomeDir {
		t.Fatalf("expected preflight employee home %q, got %#v", repo.preflight.AgentHomeDir, payload["agent_home_dir"])
	}
	if _, ok := payload["workspace_files"]; ok {
		t.Fatalf("start payload must not include employee workspace files: %#v", payload["workspace_files"])
	}
	metadata := payload["metadata"].(map[string]any)
	if metadata["project_id"] != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("project metadata missing from start payload: %#v", metadata)
	}
}

func TestCreateRunRejectsWhenDailyTokenBudgetExceeded(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.preflight.BudgetPolicy = map[string]any{"daily_token_limit": float64(1000)}
	repo.preflight.TodayTokenUsage = 1000
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "employee daily token budget exceeded") {
		t.Fatalf("expected budget exceeded error, got %v", err)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("budget exceeded run must not dispatch command")
	}
	if repo.createdRun != nil {
		t.Fatalf("budget exceeded run must not create run record")
	}
}

func TestCreateRunAllowsWhenDailyTokenBudgetUnset(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.preflight.BudgetPolicy = map[string]any{}
	repo.preflight.TodayTokenUsage = 999999
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if err != nil {
		t.Fatalf("expected run allowed without budget limit, got %v", err)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected command dispatch")
	}
}

func TestRunServiceListRunEventsReturnsPersistedEvents(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.run = validRunServiceRun(DigitalEmployeeRunStatusRunning)
	repo.runEvents = []RuntimeCommandEventWriteback{
		{
			EventType:      "text_delta",
			SequenceNumber: 2,
			Payload:        map[string]any{"text": "hello"},
			Metadata:       map[string]any{"provider": "codex"},
		},
	}
	dispatcher := newFakeRunServiceDispatcher()
	service := mustNewRunService(t, repo, dispatcher)

	events, err := service.ListRunEvents(context.Background(), repo.run.TenantID, repo.run.DigitalEmployeeID, repo.run.ID, 50, 0)

	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one run event, got %#v", events)
	}
	if events[0].EventType != "text_delta" || events[0].Payload["text"] != "hello" {
		t.Fatalf("unexpected run event: %#v", events[0])
	}
	if repo.listRunEventsTaskID != repo.run.TaskID || repo.listRunEventsRunID != repo.run.ID {
		t.Fatalf("expected service to list events by run task/run ids, got task=%s run=%s", repo.listRunEventsTaskID, repo.listRunEventsRunID)
	}
}

func TestRunServiceListRunsReconcilesTerminalReceiptForActiveRun(t *testing.T) {
	repo := newFakeRunServiceRepository()
	staleRun := validRunServiceRun(DigitalEmployeeRunStatusDispatching)
	staleRun.CommandID = "cmd-list-terminal-receipt"
	repo.runs = []*DigitalEmployeeRun{staleRun}
	repo.commandReceipt = &RuntimeCommandReceipt{
		TenantID:  staleRun.TenantID,
		CommandID: staleRun.CommandID,
		Status:    "cancelled",
	}
	service := mustNewRunService(t, repo, newFakeRunServiceDispatcher())

	result, err := service.ListRunsDetailed(context.Background(), staleRun.TenantID, staleRun.DigitalEmployeeID, DigitalEmployeeRunListFilter{Limit: 10})

	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Run.Status != DigitalEmployeeRunStatusCancelled {
		t.Fatalf("expected stale run returned as cancelled, got %#v", result.Items)
	}
	if len(repo.statusUpdates) != 1 || repo.statusUpdates[0].Status != DigitalEmployeeRunStatusCancelled {
		t.Fatalf("expected stale listed run reconciled to cancelled, got %#v", repo.statusUpdates)
	}
}

func TestRunServiceCreateRunReconcilesIdempotentQueuedRunWithoutReceipt(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	audit := &fakeRunServiceAuditLogger{}
	service := mustNewRunService(t, repo, dispatcher, audit)
	req := validCreateRunServiceRequest()
	idempotencyKey := "idem-1"
	req.IdempotencyKey = &idempotencyKey
	fingerprint, err := computeRunIdempotencyFingerprint(req, strings.TrimSpace(req.Objective), strings.TrimSpace(req.Prompt), repo.preflight)
	if err != nil {
		t.Fatalf("compute fingerprint: %v", err)
	}
	repo.activeRun = validRunServiceRun(DigitalEmployeeRunStatusQueued)
	repo.activeRun.IdempotencyKey = &idempotencyKey
	repo.activeRun.IdempotencyFingerprint = &fingerprint

	run, err := service.CreateRun(context.Background(), req)

	if err != nil {
		t.Fatalf("create run retry: %v", err)
	}
	if run.Status != DigitalEmployeeRunStatusDispatching {
		t.Fatalf("expected reconciled run dispatching, got %s", run.Status)
	}
	if repo.createdRunCount != 0 {
		t.Fatalf("expected idempotent retry not to create another run, got %d creates", repo.createdRunCount)
	}
	if len(repo.commandReceipts) != 1 || repo.commandReceipts[0].CommandID != repo.activeRun.CommandID {
		t.Fatalf("expected missing receipt to be recreated for existing command, got %#v", repo.commandReceipts)
	}
	if len(dispatcher.commands) != 1 || dispatcher.commands[0].command.ID != repo.activeRun.CommandID {
		t.Fatalf("expected existing command to be dispatched, got %#v", dispatcher.commands)
	}
	if len(repo.events) != 1 || repo.events[0].SequenceNumber != runDispatchedLifecycleSequence {
		t.Fatalf("expected run_dispatched lifecycle event, got %#v", repo.events)
	}
	if len(audit.events) != 1 || audit.events[0].action != "employee.run.create" {
		t.Fatalf("expected create audit for repaired dispatch, got %#v", audit.events)
	}
}

func TestRunServiceCreateRunReconcilesDispatchedReceiptForQueuedRun(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	audit := &fakeRunServiceAuditLogger{}
	service := mustNewRunService(t, repo, dispatcher, audit)
	req := validCreateRunServiceRequest()
	idempotencyKey := "idem-dispatched"
	req.IdempotencyKey = &idempotencyKey
	fingerprint, err := computeRunIdempotencyFingerprint(req, strings.TrimSpace(req.Objective), strings.TrimSpace(req.Prompt), repo.preflight)
	if err != nil {
		t.Fatalf("compute fingerprint: %v", err)
	}
	repo.activeRun = validRunServiceRun(DigitalEmployeeRunStatusQueued)
	repo.activeRun.IdempotencyKey = &idempotencyKey
	repo.activeRun.IdempotencyFingerprint = &fingerprint
	repo.commandReceipt = &RuntimeCommandReceipt{
		TenantID:  repo.activeRun.TenantID,
		CommandID: repo.activeRun.CommandID,
		Status:    "dispatched",
	}

	run, err := service.CreateRun(context.Background(), req)

	if err != nil {
		t.Fatalf("create run retry: %v", err)
	}
	if run.Status != DigitalEmployeeRunStatusDispatching {
		t.Fatalf("expected queued run with dispatched receipt to be marked dispatching, got %s", run.Status)
	}
	if len(dispatcher.commands) != 0 || len(repo.commandReceipts) != 0 {
		t.Fatalf("expected dispatched receipt retry not to redispatch/create receipt, commands=%#v receipts=%#v", dispatcher.commands, repo.commandReceipts)
	}
	if len(repo.events) != 1 || repo.events[0].SequenceNumber != runDispatchedLifecycleSequence {
		t.Fatalf("expected run_dispatched lifecycle event, got %#v", repo.events)
	}
	if len(audit.events) != 1 || audit.events[0].action != "employee.run.create" {
		t.Fatalf("expected create audit for dispatched receipt reconciliation, got %#v", audit.events)
	}
}

func TestRunServiceCreateRunMarksFailedWhenReceiptFailed(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)
	req := validCreateRunServiceRequest()
	idempotencyKey := "idem-failed"
	req.IdempotencyKey = &idempotencyKey
	fingerprint, err := computeRunIdempotencyFingerprint(req, strings.TrimSpace(req.Objective), strings.TrimSpace(req.Prompt), repo.preflight)
	if err != nil {
		t.Fatalf("compute fingerprint: %v", err)
	}
	errorMessage := "ws write failed"
	repo.activeRun = validRunServiceRun(DigitalEmployeeRunStatusQueued)
	repo.activeRun.IdempotencyKey = &idempotencyKey
	repo.activeRun.IdempotencyFingerprint = &fingerprint
	repo.commandReceipt = &RuntimeCommandReceipt{
		TenantID:     repo.activeRun.TenantID,
		CommandID:    repo.activeRun.CommandID,
		Status:       "failed",
		ErrorMessage: &errorMessage,
	}

	run, err := service.CreateRun(context.Background(), req)

	if err != nil {
		t.Fatalf("create run retry: %v", err)
	}
	if run.Status != DigitalEmployeeRunStatusFailed {
		t.Fatalf("expected failed run from failed receipt, got %s", run.Status)
	}
	if len(dispatcher.commands) != 0 || len(repo.events) != 0 {
		t.Fatalf("expected failed receipt retry not to dispatch/events, commands=%#v events=%#v", dispatcher.commands, repo.events)
	}
	if len(repo.statusUpdates) != 1 || repo.statusUpdates[0].Status != DigitalEmployeeRunStatusFailed {
		t.Fatalf("expected failed run status update, got %#v", repo.statusUpdates)
	}
}

func TestRunServiceCreateRunDoesNotRestartTerminalReceipt(t *testing.T) {
	for _, tc := range []struct {
		receiptStatus string
		expectedRun   DigitalEmployeeRunStatus
	}{
		{receiptStatus: "completed", expectedRun: DigitalEmployeeRunStatusCompleted},
		{receiptStatus: "cancelled", expectedRun: DigitalEmployeeRunStatusCancelled},
		{receiptStatus: "timed_out", expectedRun: DigitalEmployeeRunStatusTimedOut},
	} {
		t.Run(tc.receiptStatus, func(t *testing.T) {
			repo := newFakeRunServiceRepository()
			repo.preflight = validRunServicePreflight()
			dispatcher := newFakeRunServiceDispatcher()
			dispatcher.connected[repo.preflight.NodeID] = true
			service := mustNewRunService(t, repo, dispatcher)
			req := validCreateRunServiceRequest()
			idempotencyKey := "idem-receipt-" + tc.receiptStatus
			req.IdempotencyKey = &idempotencyKey
			fingerprint, err := computeRunIdempotencyFingerprint(req, strings.TrimSpace(req.Objective), strings.TrimSpace(req.Prompt), repo.preflight)
			if err != nil {
				t.Fatalf("compute fingerprint: %v", err)
			}
			repo.activeRun = validRunServiceRun(DigitalEmployeeRunStatusQueued)
			repo.activeRun.IdempotencyKey = &idempotencyKey
			repo.activeRun.IdempotencyFingerprint = &fingerprint
			repo.commandReceipt = &RuntimeCommandReceipt{
				TenantID:  repo.activeRun.TenantID,
				CommandID: repo.activeRun.CommandID,
				Status:    tc.receiptStatus,
			}

			run, err := service.CreateRun(context.Background(), req)

			if err != nil {
				t.Fatalf("create run retry: %v", err)
			}
			if run.Status != tc.expectedRun {
				t.Fatalf("expected existing run reconciled to %s, got %s", tc.expectedRun, run.Status)
			}
			if len(dispatcher.commands) != 0 || len(repo.commandReceipts) != 0 || len(repo.statusUpdates) != 1 || len(repo.events) != 0 {
				t.Fatalf("expected terminal receipt not to restart/write, commands=%#v receipts=%#v status=%#v events=%#v", dispatcher.commands, repo.commandReceipts, repo.statusUpdates, repo.events)
			}
		})
	}
}

func TestRunServiceCreateRunReconcilesTerminalReceiptBeforeActiveConflict(t *testing.T) {
	for _, tc := range []struct {
		receiptStatus string
		expectedRun   DigitalEmployeeRunStatus
	}{
		{receiptStatus: "completed", expectedRun: DigitalEmployeeRunStatusCompleted},
		{receiptStatus: "cancelled", expectedRun: DigitalEmployeeRunStatusCancelled},
		{receiptStatus: "timed_out", expectedRun: DigitalEmployeeRunStatusTimedOut},
	} {
		t.Run(tc.receiptStatus, func(t *testing.T) {
			repo := newFakeRunServiceRepository()
			repo.preflight = validRunServicePreflight()
			dispatcher := newFakeRunServiceDispatcher()
			dispatcher.connected[repo.preflight.NodeID] = true
			service := mustNewRunService(t, repo, dispatcher)
			req := validCreateRunServiceRequest()
			repo.activeRun = validRunServiceRun(DigitalEmployeeRunStatusQueued)
			repo.activeRun.CommandID = "cmd-stale-" + tc.receiptStatus
			repo.commandReceipt = &RuntimeCommandReceipt{
				TenantID:  repo.activeRun.TenantID,
				CommandID: repo.activeRun.CommandID,
				Status:    tc.receiptStatus,
			}

			run, err := service.CreateRun(context.Background(), req)

			if err != nil {
				t.Fatalf("create run after stale active reconciliation: %v", err)
			}
			if len(repo.statusUpdates) < 1 || repo.statusUpdates[0].Status != tc.expectedRun {
				t.Fatalf("expected stale active run reconciled to %s, got %#v", tc.expectedRun, repo.statusUpdates)
			}
			if run.Status != DigitalEmployeeRunStatusDispatching || repo.createdRunCount != 1 || len(dispatcher.commands) != 1 {
				t.Fatalf("expected new run dispatched after reconciliation, run=%#v creates=%d commands=%#v", run, repo.createdRunCount, dispatcher.commands)
			}
		})
	}
}

func TestRunServiceCreateRunDoesNotRestartRunningOrTerminalIdempotentRun(t *testing.T) {
	for _, status := range []DigitalEmployeeRunStatus{
		DigitalEmployeeRunStatusRunning,
		DigitalEmployeeRunStatusCompleted,
		DigitalEmployeeRunStatusFailed,
		DigitalEmployeeRunStatusCancelled,
		DigitalEmployeeRunStatusTimedOut,
	} {
		t.Run(string(status), func(t *testing.T) {
			repo := newFakeRunServiceRepository()
			repo.preflight = validRunServicePreflight()
			dispatcher := newFakeRunServiceDispatcher()
			dispatcher.connected[repo.preflight.NodeID] = true
			service := mustNewRunService(t, repo, dispatcher)
			req := validCreateRunServiceRequest()
			idempotencyKey := "idem-" + string(status)
			req.IdempotencyKey = &idempotencyKey
			fingerprint, err := computeRunIdempotencyFingerprint(req, strings.TrimSpace(req.Objective), strings.TrimSpace(req.Prompt), repo.preflight)
			if err != nil {
				t.Fatalf("compute fingerprint: %v", err)
			}
			repo.activeRun = validRunServiceRun(status)
			repo.activeRun.IdempotencyKey = &idempotencyKey
			repo.activeRun.IdempotencyFingerprint = &fingerprint

			run, err := service.CreateRun(context.Background(), req)

			if err != nil {
				t.Fatalf("create run retry: %v", err)
			}
			if run.Status != status {
				t.Fatalf("expected existing %s run returned, got %s", status, run.Status)
			}
			if len(dispatcher.commands) != 0 || len(repo.commandReceipts) != 0 || len(repo.statusUpdates) != 0 {
				t.Fatalf("expected no restart writes, commands=%#v receipts=%#v status=%#v", dispatcher.commands, repo.commandReceipts, repo.statusUpdates)
			}
		})
	}
}

func TestRunServiceStopRunMovesToCancellingAndDispatchesStop(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.run = validRunServiceRun(DigitalEmployeeRunStatusRunning)
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.run.NodeID] = true
	audit := &fakeRunServiceAuditLogger{}
	service := mustNewRunService(t, repo, dispatcher, audit)

	run, err := service.StopRun(context.Background(), StopDigitalEmployeeRunRequest{
		TenantID:          repo.run.TenantID,
		UserID:            uuid.New(),
		DigitalEmployeeID: repo.run.DigitalEmployeeID,
		RunID:             repo.run.ID,
		Reason:            "  用户要求停止  ",
	})

	if err != nil {
		t.Fatalf("stop run: %v", err)
	}
	if run.Status != DigitalEmployeeRunStatusCancelling {
		t.Fatalf("expected cancelling run, got %s", run.Status)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one dispatched stop command, got %d", len(dispatcher.commands))
	}
	dispatched := dispatcher.commands[0]
	if dispatched.nodeID != repo.run.NodeID || dispatched.command.Type != "stop_session" {
		t.Fatalf("unexpected stop dispatch: %#v", dispatched)
	}
	payload := commandPayload(t, dispatched.command)
	if payload["provider_run_protocol"] != "provider-run/v1" {
		t.Fatalf("unexpected provider_run_protocol: %#v", payload["provider_run_protocol"])
	}
	if payload["run_id"] != repo.run.ID.String() || payload["task_id"] != repo.run.TaskID.String() {
		t.Fatalf("unexpected stop payload ids: %#v", payload)
	}
	if payload["start_command_id"] != repo.run.CommandID {
		t.Fatalf("unexpected start command id: %#v", payload["start_command_id"])
	}
	if payload["reason"] != "用户要求停止" {
		t.Fatalf("expected trimmed reason, got %#v", payload["reason"])
	}
	if len(repo.events) != 1 || repo.events[0].EventType != "stop_requested" {
		t.Fatalf("expected stop_requested event, got %#v", repo.events)
	}
	if repo.events[0].SequenceNumber != stopRequestedLifecycleSequence {
		t.Fatalf("expected lifecycle stop_requested sequence %d, got %d", stopRequestedLifecycleSequence, repo.events[0].SequenceNumber)
	}
	if len(audit.events) != 1 || audit.events[0].eventType != "digital_employee_run_stop_requested" || audit.events[0].action != "employee.run.stop" {
		t.Fatalf("expected stop audit event, got %#v", audit.events)
	}
}

func TestRunServiceStopRunRejectsBlankReason(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.run = validRunServiceRun(DigitalEmployeeRunStatusRunning)
	dispatcher := newFakeRunServiceDispatcher()
	service := mustNewRunService(t, repo, dispatcher, &fakeRunServiceAuditLogger{})

	_, err := service.StopRun(context.Background(), StopDigitalEmployeeRunRequest{
		TenantID:          repo.run.TenantID,
		UserID:            uuid.New(),
		DigitalEmployeeID: repo.run.DigitalEmployeeID,
		RunID:             repo.run.ID,
		Reason:            "  ",
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for blank stop reason, got %v", err)
	}
	if len(repo.statusUpdates) != 0 || len(dispatcher.commands) != 0 {
		t.Fatalf("expected blank stop reason not to mutate run or dispatch, updates=%#v commands=%#v", repo.statusUpdates, dispatcher.commands)
	}
}

func TestRunServiceStopRunRejectsAlreadyCancelling(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.run = validRunServiceRun(DigitalEmployeeRunStatusCancelling)
	dispatcher := newFakeRunServiceDispatcher()
	service := mustNewRunService(t, repo, dispatcher, &fakeRunServiceAuditLogger{})

	_, err := service.StopRun(context.Background(), StopDigitalEmployeeRunRequest{
		TenantID:          repo.run.TenantID,
		UserID:            uuid.New(),
		DigitalEmployeeID: repo.run.DigitalEmployeeID,
		RunID:             repo.run.ID,
		Reason:            "human stop",
	})

	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if len(repo.statusUpdates) != 0 || len(repo.events) != 0 || len(dispatcher.commands) != 0 {
		t.Fatalf("expected cancelling run rejection before writes, status=%#v events=%#v commands=%#v", repo.statusUpdates, repo.events, dispatcher.commands)
	}
}

func TestRunServiceStopRunRecordsStopRequestedBeforeDispatchFailure(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.run = validRunServiceRun(DigitalEmployeeRunStatusRunning)
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.dispatchErr = errors.New("ws write failed")
	service := mustNewRunService(t, repo, dispatcher, &fakeRunServiceAuditLogger{})

	_, err := service.StopRun(context.Background(), StopDigitalEmployeeRunRequest{
		TenantID:          repo.run.TenantID,
		UserID:            uuid.New(),
		DigitalEmployeeID: repo.run.DigitalEmployeeID,
		RunID:             repo.run.ID,
		Reason:            "human stop",
	})

	if err == nil {
		t.Fatalf("expected stop dispatch error")
	}
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected ErrRuntimeUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "ws write failed") {
		t.Fatalf("expected original dispatch error context, got %v", err)
	}
	if len(repo.statusUpdates) != 1 || repo.statusUpdates[0].Status != DigitalEmployeeRunStatusCancelling {
		t.Fatalf("expected run to move to cancelling, got %#v", repo.statusUpdates)
	}
	if len(repo.events) != 1 || repo.events[0].EventType != "stop_requested" {
		t.Fatalf("expected stop_requested event before dispatch failure, got %#v", repo.events)
	}
	if repo.events[0].SequenceNumber != stopRequestedLifecycleSequence {
		t.Fatalf("expected lifecycle stop_requested sequence %d, got %d", stopRequestedLifecycleSequence, repo.events[0].SequenceNumber)
	}
	if len(repo.receiptUpdates) != 1 || repo.receiptUpdates[0].Status != "failed" {
		t.Fatalf("expected failed stop receipt update, got %#v", repo.receiptUpdates)
	}
}

func TestRunServiceCreateRunAllowsPreflightWithoutApprovedEffectiveConfig(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if err != nil {
		t.Fatalf("expected run creation to allow missing approved effective config, got %v", err)
	}
	if repo.createdRunCount != 1 || len(dispatcher.commands) != 1 {
		t.Fatalf("expected run creation and dispatch to proceed, got created=%d dispatched=%d", repo.createdRunCount, len(dispatcher.commands))
	}
}

func TestRunServiceCreateRunRejectsDisconnectedRuntime(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected ErrRuntimeUnavailable, got %v", err)
	}
	if repo.createdRunCount != 0 || len(dispatcher.commands) != 0 {
		t.Fatalf("expected runtime connection rejection before create/dispatch")
	}
}

func TestRunServiceCreateRunDispatchFailureMarksRunFailed(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	dispatcher.dispatchErr = errors.New("ws write failed")
	audit := &fakeRunServiceAuditLogger{}
	service := mustNewRunService(t, repo, dispatcher, audit)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if err == nil {
		t.Fatalf("expected dispatch error")
	}
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected ErrRuntimeUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "ws write failed") {
		t.Fatalf("expected original dispatch error context, got %v", err)
	}
	if len(repo.receiptUpdates) != 1 || repo.receiptUpdates[0].Status != "failed" {
		t.Fatalf("expected failed receipt update, got %#v", repo.receiptUpdates)
	}
	if len(repo.statusUpdates) != 1 || repo.statusUpdates[0].Status != DigitalEmployeeRunStatusFailed {
		t.Fatalf("expected failed run status update, got %#v", repo.statusUpdates)
	}
	if repo.statusUpdates[0].ErrorCode == nil || *repo.statusUpdates[0].ErrorCode != "dispatch_failed" {
		t.Fatalf("expected dispatch_failed error code, got %#v", repo.statusUpdates[0].ErrorCode)
	}
	if len(audit.events) != 1 || audit.events[0].eventType != "digital_employee_run_dispatch_failed" {
		t.Fatalf("expected dispatch failure audit, got %#v", audit.events)
	}
}

func TestRunServiceCreateRunDispatchRuntimeNotConnectedMapsRuntimeUnavailable(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	dispatcher.dispatchErr = cpruntime.ErrRuntimeNotConnected
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected ErrRuntimeUnavailable, got %v", err)
	}
	if !errors.Is(err, cpruntime.ErrRuntimeNotConnected) {
		t.Fatalf("expected original runtime error to be preserved, got %v", err)
	}
}

func mustNewRunService(t *testing.T, repo DigitalEmployeeRunRepository, dispatcher RuntimeCommandDispatcher, audit ...AuditLogger) *DigitalEmployeeRunService {
	t.Helper()
	var logger AuditLogger
	if len(audit) > 0 {
		logger = audit[0]
	}
	service, err := NewDigitalEmployeeRunService(repo, dispatcher, logger)
	if err != nil {
		t.Fatalf("new run service: %v", err)
	}
	return service
}

func newRunServiceWithListers(t *testing.T, repo *fakeRunServiceRepository, dispatcher RuntimeCommandDispatcher) *DigitalEmployeeRunService {
	t.Helper()
	service := mustNewRunService(t, repo, dispatcher)
	service.SetSkillLister(repo)
	service.SetRuntimeCapabilityLister(repo)
	service.SetEnvironmentLister(repo)
	return service
}

func validCreateRunServiceRequest() CreateDigitalEmployeeRunRequest {
	timeoutSec := int32(120)
	graceSec := int32(15)
	return CreateDigitalEmployeeRunRequest{
		TenantID:          runServiceTenantID,
		UserID:            uuid.New(),
		DigitalEmployeeID: runServiceEmployeeID,
		Objective:         "  修复失败测试  ",
		Prompt:            "  请先复现再修复  ",
		ContextRefs:       []map[string]any{{"type": "doc", "ref": "ctx-1"}},
		ArtifactRefs:      []map[string]any{{"type": "log", "ref": "artifact-1"}},
		OutputSchema:      map[string]any{"type": "object"},
		AllowedActions:    []string{"test.run"},
		ForbiddenActions:  []string{"deploy.prod"},
		SecretRefs:        []string{"secret://github-token"},
		TimeoutSec:        &timeoutSec,
		GraceSec:          &graceSec,
		Metadata:          map[string]any{"source": "test"},
	}
}

func TestBuildStartSessionPayloadIncludesEffectiveMCPServers(t *testing.T) {
	run := &DigitalEmployeeRun{ID: uuid.New(), TaskID: uuid.New(), CommandID: uuid.NewString(), DigitalEmployeeID: uuid.New()}
	payload := buildStartSessionPayload(
		CreateDigitalEmployeeRunRequest{TenantID: uuid.New(), DigitalEmployeeID: run.DigitalEmployeeID},
		"Inspect repo",
		"Inspect repo",
		RunPreflight{ProviderType: "codex"},
		run,
		EmployeeConfigInput{
			PersonaMemoryMarkdown: "# 人格画像\n证据优先",
			CapabilityBindings: map[string]any{
				"skills":                []any{"repo-inspection"},
				"mcp_servers":           []any{"github"},
				"external_capabilities": []any{},
			},
		},
		nil,
		nil,
		[]RuntimeMCPServerPayload{{
			ServerID:         "mcp-1",
			ServerKey:        "github",
			Name:             "GitHub MCP",
			Transport:        "streamable_http",
			URL:              "https://api.githubcopilot.com/mcp/",
			AuthStrategy:     "bearer_env",
			CredentialEnvVar: "GITHUB_TOKEN",
			RequiredEnvVars:  []string{"GITHUB_TOKEN"},
			SourceScope:      "employee",
		}},
	)

	servers, ok := payload["mcp_servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one mcp server payload, got %#v", payload["mcp_servers"])
	}
	if payload["persona_memory_markdown"] != "# 人格画像\n证据优先" {
		t.Fatalf("expected persona memory in payload, got %#v", payload["persona_memory_markdown"])
	}
	bindings, ok := payload["capability_bindings"].(map[string]any)
	if !ok {
		t.Fatalf("expected capability_bindings object, got %#v", payload["capability_bindings"])
	}
	if got := bindings["mcp_servers"]; !reflect.DeepEqual(got, []any{"github"}) {
		t.Fatalf("expected capability_bindings.mcp_servers [github], got %#v", got)
	}
	if servers[0]["server_key"] != "github" || servers[0]["credential_env_var"] != "GITHUB_TOKEN" {
		t.Fatalf("unexpected mcp payload: %#v", servers[0])
	}
	required, ok := servers[0]["required_env_vars"].([]string)
	if !ok || len(required) != 1 || required[0] != "GITHUB_TOKEN" {
		t.Fatalf("expected required_env_vars [GITHUB_TOKEN], got %#v", servers[0]["required_env_vars"])
	}
}

func validRunServicePreflight() RunPreflight {
	return RunPreflight{
		TenantID:              runServiceTenantID,
		TeamID:                uuid.New(),
		DigitalEmployeeID:     runServiceEmployeeID,
		DigitalEmployeeStatus: DigitalEmployeeStatusReady,
		ExecutionInstanceID:   runServiceExecutionInstanceID,
		ExecutionStatus:       ExecutionInstanceStatusReady,
		RuntimeNodeID:         runServiceRuntimeNodeID,
		NodeID:                "runtime-authoritative",
		ProviderType:          "codex",
		AgentHomeDir:          "/var/lib/superteam/agents/employee",
		RuntimeSelector:       map[string]any{"node_id": "runtime-authoritative"},
		SessionPolicy:         map[string]any{"resume": true},
		WorkspacePolicy:       map[string]any{"workspace": "isolated"},
		ProviderHealthy:       true,
	}
}

func validProjectTaskRunServicePreflight() StartProjectTaskRunPreflight {
	return StartProjectTaskRunPreflight{
		TenantID:              runServiceTenantID,
		TeamID:                uuid.New(),
		DigitalEmployeeID:     runServiceEmployeeID,
		DigitalEmployeeStatus: DigitalEmployeeStatusReady,
		RuntimeNodeID:         runServiceRuntimeNodeID,
		NodeID:                "runtime-project-placement",
		ProviderType:          "codex",
		WorkspaceBaseDir:      "/var/lib/superteam/workspaces",
		BudgetPolicy:          map[string]any{"daily_token_limit": float64(100000)},
		BusinessTimezone:      "Asia/Shanghai",
		RuntimeSessionActive:  true,
		ProviderHealthy:       true,
	}
}

func validRunServiceRun(status DigitalEmployeeRunStatus) *DigitalEmployeeRun {
	timeoutSec := int32(120)
	graceSec := int32(15)
	return &DigitalEmployeeRun{
		ID:                  uuid.New(),
		TenantID:            runServiceTenantID,
		TaskID:              uuid.New(),
		DigitalEmployeeID:   runServiceEmployeeID,
		ExecutionInstanceID: runServiceExecutionInstanceID,
		RuntimeNodeID:       runServiceRuntimeNodeID,
		NodeID:              "runtime-authoritative",
		CommandID:           "cmd-start-existing",
		ProviderType:        "codex",
		Status:              status,
		TimeoutSec:          &timeoutSec,
		GraceSec:            &graceSec,
		StartedAt:           time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}
}

func commandPayload(t *testing.T, command cpruntime.RuntimeCommand) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		t.Fatalf("decode command payload: %v", err)
	}
	return payload
}

func requireWorkspaceMetadata(t *testing.T, metadata map[string]any) {
	t.Helper()
	if metadata["workspace_mode"] != "branch" || metadata["base_ref"] != "main" {
		t.Fatalf("expected workspace metadata, got %#v", metadata)
	}
	projectGit, ok := metadata["project_git"].(map[string]any)
	if !ok {
		t.Fatalf("expected project_git metadata object, got %#v", metadata["project_git"])
	}
	if projectGit["url"] != "https://github.com/acme/app.git" || projectGit["default_branch"] != "main" {
		t.Fatalf("expected project git url/default branch, got %#v", projectGit)
	}
	scope, ok := projectGit["scope"].([]any)
	if !ok || len(scope) != 2 || scope[0] != "apps/web" || scope[1] != "packages/shared" {
		t.Fatalf("expected project git scope, got %#v", projectGit["scope"])
	}
}

var (
	runServiceTenantID            = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	runServiceEmployeeID          = uuid.MustParse("00000000-0000-0000-0000-000000000201")
	runServiceExecutionInstanceID = uuid.MustParse("00000000-0000-0000-0000-000000000301")
	runServiceRuntimeNodeID       = uuid.MustParse("00000000-0000-0000-0000-000000000401")
)

type fakeProjectTaskNodeResolver struct {
	nodeID  uuid.UUID
	err     error
	lastReq ResolveProjectTaskNodeRequest
	calls   int
}

func (f *fakeProjectTaskNodeResolver) ResolveProjectTaskNode(_ context.Context, req ResolveProjectTaskNodeRequest) (uuid.UUID, error) {
	f.calls++
	f.lastReq = req
	if f.err != nil {
		return uuid.Nil, f.err
	}
	return f.nodeID, nil
}

type fakeRunServiceRepository struct {
	preflight                             RunPreflight
	projectTaskPreflight                  StartProjectTaskRunPreflight
	projectTaskPreflightProjectID         uuid.UUID
	projectTaskPreflightEmployeeID        uuid.UUID
	projectTaskPreflightForNodeEmployeeID uuid.UUID
	projectTaskPreflightForNodeNodeID     uuid.UUID
	activeRun                             *DigitalEmployeeRun
	run                                   *DigitalEmployeeRun
	runs                                  []*DigitalEmployeeRun
	createdRun                            *DigitalEmployeeRun
	createdRunCount                       int
	createRunRequests                     []CreateRunRecordRequest
	statusUpdates                         []UpdateRunStatusRequest
	events                                []CreateRunEventRecordRequest
	runEvents                             []RuntimeCommandEventWriteback
	workspaceFilesForSync                 []WorkspaceFileForSyncRecord
	listRunEventsTaskID                   uuid.UUID
	listRunEventsRunID                    uuid.UUID
	commandReceipt                        *RuntimeCommandReceipt
	commandReceipts                       []CreateRuntimeCommandReceiptRequest
	receiptUpdates                        []UpdateRuntimeCommandReceiptRequest
	runtimeSkills                         []skill.SkillRuntimeRecord
	runtimeCapabilities                   []cpruntime.RuntimeCapability
	runtimeEnv                            []RuntimeEnvironmentVariablePayload
	latestConfigInput                     EmployeeConfigInput
	runStats                              DigitalEmployeeRunStats

	// lineageRoot is the value ResolveProjectTaskLineageRoot returns; when
	// left uuid.Nil it echoes back the requested project task id (root =
	// self), matching the production default when no revision lineage
	// exists.
	lineageRoot      uuid.UUID
	lineageRootErr   error
	lineageRootCalls []resolveLineageRootCall

	// providerSessionForRoot maps a task-lineage root id to the session id
	// FindProviderSessionForTaskRoot should return for it (empty string /
	// absent means "no active session for this root").
	providerSessionForRoot      map[uuid.UUID]string
	providerSessionForRootCalls []findProviderSessionForRootCall
}

type resolveLineageRootCall struct {
	TenantID      uuid.UUID
	ProjectTaskID uuid.UUID
}

type findProviderSessionForRootCall struct {
	TenantID   uuid.UUID
	EmployeeID uuid.UUID
	TaskRootID uuid.UUID
}

func newFakeRunServiceRepository() *fakeRunServiceRepository {
	return &fakeRunServiceRepository{}
}

func (f *fakeRunServiceRepository) GetRunPreflight(context.Context, uuid.UUID, uuid.UUID) (RunPreflight, error) {
	return f.preflight, nil
}

func (f *fakeRunServiceRepository) GetProjectTaskRunPreflight(_ context.Context, tenantID, projectID, employeeID uuid.UUID) (StartProjectTaskRunPreflight, error) {
	f.projectTaskPreflightProjectID = projectID
	f.projectTaskPreflightEmployeeID = employeeID
	return f.projectTaskPreflight, nil
}

func (f *fakeRunServiceRepository) GetProjectTaskRunPreflightForNode(_ context.Context, tenantID, employeeID, resolvedNodeID uuid.UUID) (StartProjectTaskRunPreflight, error) {
	f.projectTaskPreflightForNodeEmployeeID = employeeID
	f.projectTaskPreflightForNodeNodeID = resolvedNodeID
	return f.projectTaskPreflight, nil
}

func (f *fakeRunServiceRepository) ResolveProjectTaskLineageRoot(_ context.Context, tenantID, projectTaskID uuid.UUID) (uuid.UUID, error) {
	f.lineageRootCalls = append(f.lineageRootCalls, resolveLineageRootCall{TenantID: tenantID, ProjectTaskID: projectTaskID})
	if f.lineageRootErr != nil {
		return uuid.Nil, f.lineageRootErr
	}
	if f.lineageRoot != uuid.Nil {
		return f.lineageRoot, nil
	}
	return projectTaskID, nil
}

func (f *fakeRunServiceRepository) WithTransaction(ctx context.Context, fn func(DigitalEmployeeRunRepository) error) error {
	return fn(f)
}

func (f *fakeRunServiceRepository) GetActiveRun(context.Context, uuid.UUID, uuid.UUID) (*DigitalEmployeeRun, error) {
	return f.activeRun, nil
}

func (f *fakeRunServiceRepository) GetDigitalEmployeeRunStats(context.Context, uuid.UUID, uuid.UUID) (DigitalEmployeeRunStats, error) {
	return f.runStats, nil
}

func (f *fakeRunServiceRepository) GetRun(_ context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error) {
	if f.run == nil || f.run.TenantID != tenantID || f.run.DigitalEmployeeID != employeeID || f.run.ID != runID {
		return nil, ErrNotFound
	}
	return cloneRun(f.run), nil
}

func (f *fakeRunServiceRepository) GetRunByID(_ context.Context, tenantID, runID uuid.UUID) (*DigitalEmployeeRun, error) {
	if f.run != nil && f.run.TenantID == tenantID && f.run.ID == runID {
		return cloneRun(f.run), nil
	}
	if f.activeRun != nil && f.activeRun.TenantID == tenantID && f.activeRun.ID == runID {
		return cloneRun(f.activeRun), nil
	}
	if f.createdRun != nil && f.createdRun.TenantID == tenantID && f.createdRun.ID == runID {
		return cloneRun(f.createdRun), nil
	}
	return nil, ErrNotFound
}

func (f *fakeRunServiceRepository) GetRunByCommandID(context.Context, uuid.UUID, string) (*DigitalEmployeeRun, error) {
	return nil, ErrNotFound
}

func (f *fakeRunServiceRepository) ListRunsDetailed(_ context.Context, tenantID, employeeID uuid.UUID, _ DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error) {
	items := make([]DigitalEmployeeRunListItem, 0, len(f.runs))
	for _, run := range f.runs {
		if run.TenantID == tenantID && run.DigitalEmployeeID == employeeID {
			items = append(items, DigitalEmployeeRunListItem{Run: cloneRun(run)})
		}
	}
	return &DigitalEmployeeRunListResult{Items: items, TotalCount: int64(len(items))}, nil
}

func (f *fakeRunServiceRepository) ListRunEvents(_ context.Context, _ uuid.UUID, taskID, runID uuid.UUID, _ int32, _ int32) ([]RuntimeCommandEventWriteback, error) {
	f.listRunEventsTaskID = taskID
	f.listRunEventsRunID = runID
	return f.runEvents, nil
}

func (f *fakeRunServiceRepository) ListWorkspaceFilesForSync(_ context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]WorkspaceFileForSyncRecord, error) {
	out := make([]WorkspaceFileForSyncRecord, 0, len(f.workspaceFilesForSync))
	for _, file := range f.workspaceFilesForSync {
		if file.TenantID == tenantID && file.DigitalEmployeeID == digitalEmployeeID {
			out = append(out, file)
		}
	}
	return out, nil
}

func (f *fakeRunServiceRepository) GetLatestDigitalEmployeeConfigRevision(context.Context, uuid.UUID, uuid.UUID) (EmployeeConfigInput, error) {
	return f.latestConfigInput, nil
}

func (f *fakeRunServiceRepository) ListSkillsForRuntime(context.Context, uuid.UUID, uuid.UUID) ([]skill.SkillRuntimeRecord, error) {
	return f.runtimeSkills, nil
}

func (f *fakeRunServiceRepository) ListRuntimeCapabilitiesForNode(context.Context, uuid.UUID, string) ([]cpruntime.RuntimeCapability, error) {
	return f.runtimeCapabilities, nil
}

func (f *fakeRunServiceRepository) ListRuntimeEnvironmentVariablesForRuntime(context.Context, uuid.UUID, uuid.UUID) ([]RuntimeEnvironmentVariablePayload, error) {
	return f.runtimeEnv, nil
}

func (f *fakeRunServiceRepository) UpsertWorkspaceFileSync(context.Context, UpsertWorkspaceFileSyncParams) error {
	return nil
}

func (f *fakeRunServiceRepository) CreateRun(_ context.Context, req CreateRunRecordRequest) (*DigitalEmployeeRun, error) {
	f.createdRunCount++
	f.createRunRequests = append(f.createRunRequests, req)
	run := validRunServiceRun(req.RunStatus)
	run.TenantID = req.TenantID
	run.DigitalEmployeeID = req.DigitalEmployeeID
	run.ExecutionInstanceID = req.ExecutionInstanceID
	run.RuntimeNodeID = req.RuntimeNodeID
	run.NodeID = req.NodeID
	run.CommandID = req.CommandID
	run.ProviderType = req.ProviderType
	run.IdempotencyKey = req.IdempotencyKey
	run.IdempotencyFingerprint = req.IdempotencyFingerprint
	run.TimeoutSec = req.TimeoutSec
	run.GraceSec = req.GraceSec
	f.createdRun = run
	return cloneRun(run), nil
}

func (f *fakeRunServiceRepository) UpdateRunStatus(_ context.Context, req UpdateRunStatusRequest) (*DigitalEmployeeRun, error) {
	f.statusUpdates = append(f.statusUpdates, req)
	var run *DigitalEmployeeRun
	if f.createdRun != nil && f.createdRun.ID == req.RunID {
		run = f.createdRun
	} else if f.activeRun != nil && f.activeRun.ID == req.RunID {
		run = f.activeRun
	} else if f.run != nil && f.run.ID == req.RunID {
		run = f.run
	} else {
		for _, listedRun := range f.runs {
			if listedRun.ID == req.RunID {
				run = listedRun
				break
			}
		}
	}
	if run == nil {
		return nil, ErrNotFound
	}
	run.Status = req.Status
	run.ErrorMessage = req.ErrorMessage
	run.ErrorCode = req.ErrorCode
	run.ErrorFamily = req.ErrorFamily
	return cloneRun(run), nil
}

func (f *fakeRunServiceRepository) HasRunEventSequence(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int32) (bool, error) {
	return false, nil
}

func (f *fakeRunServiceRepository) CreateTaskEventIfAbsent(_ context.Context, req CreateRunEventRecordRequest) (bool, error) {
	f.events = append(f.events, req)
	return true, nil
}

func (f *fakeRunServiceRepository) UpsertProviderSession(context.Context, UpsertProviderSessionRequest) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (f *fakeRunServiceRepository) FindProviderSessionForTaskRoot(_ context.Context, tenantID, employeeID, taskRootID uuid.UUID) (string, error) {
	f.providerSessionForRootCalls = append(f.providerSessionForRootCalls, findProviderSessionForRootCall{
		TenantID:   tenantID,
		EmployeeID: employeeID,
		TaskRootID: taskRootID,
	})
	return f.providerSessionForRoot[taskRootID], nil
}

func (f *fakeRunServiceRepository) CreateProviderSessionEventIfAbsent(context.Context, CreateProviderSessionEventRecordRequest) (uuid.UUID, error) {
	return uuid.MustParse("00000000-0000-0000-0000-000000000701"), nil
}

func (f *fakeRunServiceRepository) CreateCommandReceipt(_ context.Context, req CreateRuntimeCommandReceiptRequest) error {
	f.commandReceipts = append(f.commandReceipts, req)
	return nil
}

func (f *fakeRunServiceRepository) GetCommandReceipt(_ context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error) {
	if f.commandReceipt != nil && f.commandReceipt.TenantID == tenantID && f.commandReceipt.CommandID == commandID {
		copied := *f.commandReceipt
		return &copied, nil
	}
	return nil, ErrNotFound
}

func (f *fakeRunServiceRepository) GetCommandReceiptForUpdate(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error) {
	return f.GetCommandReceipt(ctx, tenantID, commandID)
}

func (f *fakeRunServiceRepository) UpdateCommandReceipt(_ context.Context, req UpdateRuntimeCommandReceiptRequest) (*RuntimeCommandReceipt, error) {
	f.receiptUpdates = append(f.receiptUpdates, req)
	return &RuntimeCommandReceipt{
		ID:        uuid.New(),
		TenantID:  req.TenantID,
		CommandID: req.CommandID,
		Status:    req.Status,
	}, nil
}

func (f *fakeRunServiceRepository) UpdateExecutionInstanceStatus(context.Context, uuid.UUID, uuid.UUID, ExecutionInstanceStatus, *string) (DigitalEmployeeExecutionInstanceRecord, error) {
	return DigitalEmployeeExecutionInstanceRecord{}, ErrInvalidInput
}

func (f *fakeRunServiceRepository) UpdateDigitalEmployeeStatus(context.Context, uuid.UUID, uuid.UUID, DigitalEmployeeStatus) (DigitalEmployeeRecord, error) {
	return DigitalEmployeeRecord{}, ErrInvalidInput
}

func (f *fakeRunServiceRepository) DeleteExecutionInstance(context.Context, uuid.UUID, uuid.UUID) error {
	return ErrInvalidInput
}

func (f *fakeRunServiceRepository) DeleteDigitalEmployee(context.Context, uuid.UUID, uuid.UUID) error {
	return ErrInvalidInput
}

type fakeRunServiceDispatcher struct {
	connected   map[string]bool
	dispatchErr error
	commands    []fakeRunServiceDispatchedCommand
}

type fakeRunServiceDispatchedCommand struct {
	nodeID  string
	command cpruntime.RuntimeCommand
}

func newFakeRunServiceDispatcher() *fakeRunServiceDispatcher {
	return &fakeRunServiceDispatcher{connected: map[string]bool{}}
}

func (f *fakeRunServiceDispatcher) IsConnected(nodeID string) bool {
	return f.connected[nodeID]
}

func (f *fakeRunServiceDispatcher) Dispatch(_ context.Context, nodeID string, command cpruntime.RuntimeCommand) error {
	f.commands = append(f.commands, fakeRunServiceDispatchedCommand{nodeID: nodeID, command: command})
	return f.dispatchErr
}

type fakeRunServiceAuditLogger struct {
	events []fakeRunServiceAuditEvent
}

type fakeRunServiceAuditEvent struct {
	eventType    string
	actorType    string
	actorID      string
	resourceType string
	resourceID   string
	action       string
}

func (f *fakeRunServiceAuditLogger) LogEvent(_ context.Context, eventType, actorType, actorID, resourceType, resourceID, action string) error {
	f.events = append(f.events, fakeRunServiceAuditEvent{
		eventType:    eventType,
		actorType:    actorType,
		actorID:      actorID,
		resourceType: resourceType,
		resourceID:   resourceID,
		action:       action,
	})
	return nil
}

func cloneRun(run *DigitalEmployeeRun) *DigitalEmployeeRun {
	if run == nil {
		return nil
	}
	copied := *run
	return &copied
}
