package employee

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	cpruntime "github.com/superteam/control-plane/internal/runtime"
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
	if len(repo.events) != 1 || repo.events[0].EventType != "run_dispatched" {
		t.Fatalf("expected run_dispatched event, got %#v", repo.events)
	}
	if len(audit.events) != 1 || audit.events[0].eventType != "digital_employee_run_created" || audit.events[0].action != "employee.run.create" {
		t.Fatalf("expected create audit event, got %#v", audit.events)
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
	if len(audit.events) != 1 || audit.events[0].eventType != "digital_employee_run_stop_requested" || audit.events[0].action != "employee.run.stop" {
		t.Fatalf("expected stop audit event, got %#v", audit.events)
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
	if len(repo.statusUpdates) != 1 || repo.statusUpdates[0].Status != DigitalEmployeeRunStatusCancelling {
		t.Fatalf("expected run to move to cancelling, got %#v", repo.statusUpdates)
	}
	if len(repo.events) != 1 || repo.events[0].EventType != "stop_requested" {
		t.Fatalf("expected stop_requested event before dispatch failure, got %#v", repo.events)
	}
	if len(repo.receiptUpdates) != 1 || repo.receiptUpdates[0].Status != "failed" {
		t.Fatalf("expected failed stop receipt update, got %#v", repo.receiptUpdates)
	}
}

func TestRunServiceCreateRunRejectsPreflightWithoutApprovedEffectiveConfig(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.preflight.HasApprovedEffectiveConfig = false
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if repo.createdRunCount != 0 || len(dispatcher.commands) != 0 {
		t.Fatalf("expected preflight rejection before create/dispatch")
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

func validRunServicePreflight() RunPreflight {
	return RunPreflight{
		TenantID:                   runServiceTenantID,
		TeamID:                     uuid.New(),
		DigitalEmployeeID:          runServiceEmployeeID,
		DigitalEmployeeStatus:      DigitalEmployeeStatusReady,
		ExecutionInstanceID:        runServiceExecutionInstanceID,
		ExecutionStatus:            ExecutionInstanceStatusReady,
		RuntimeNodeID:              runServiceRuntimeNodeID,
		NodeID:                     "runtime-authoritative",
		ProviderType:               "codex",
		AgentHomeDir:               "/var/lib/superteam/agents/employee",
		RuntimeSelector:            map[string]any{"node_id": "runtime-authoritative"},
		SessionPolicy:              map[string]any{"resume": true},
		WorkspacePolicy:            map[string]any{"workspace": "isolated"},
		HasApprovedEffectiveConfig: true,
		ProviderHealthy:            true,
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

var (
	runServiceTenantID            = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	runServiceEmployeeID          = uuid.MustParse("00000000-0000-0000-0000-000000000201")
	runServiceExecutionInstanceID = uuid.MustParse("00000000-0000-0000-0000-000000000301")
	runServiceRuntimeNodeID       = uuid.MustParse("00000000-0000-0000-0000-000000000401")
)

type fakeRunServiceRepository struct {
	preflight         RunPreflight
	activeRun         *DigitalEmployeeRun
	run               *DigitalEmployeeRun
	createdRun        *DigitalEmployeeRun
	createdRunCount   int
	createRunRequests []CreateRunRecordRequest
	statusUpdates     []UpdateRunStatusRequest
	events            []CreateRunEventRecordRequest
	commandReceipts   []CreateRuntimeCommandReceiptRequest
	receiptUpdates    []UpdateRuntimeCommandReceiptRequest
}

func newFakeRunServiceRepository() *fakeRunServiceRepository {
	return &fakeRunServiceRepository{}
}

func (f *fakeRunServiceRepository) GetRunPreflight(context.Context, uuid.UUID, uuid.UUID) (RunPreflight, error) {
	return f.preflight, nil
}

func (f *fakeRunServiceRepository) GetActiveRun(context.Context, uuid.UUID, uuid.UUID) (*DigitalEmployeeRun, error) {
	return f.activeRun, nil
}

func (f *fakeRunServiceRepository) GetRun(_ context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error) {
	if f.run == nil || f.run.TenantID != tenantID || f.run.DigitalEmployeeID != employeeID || f.run.ID != runID {
		return nil, ErrNotFound
	}
	return cloneRun(f.run), nil
}

func (f *fakeRunServiceRepository) GetRunByCommandID(context.Context, uuid.UUID, string) (*DigitalEmployeeRun, error) {
	return nil, ErrNotFound
}

func (f *fakeRunServiceRepository) ListRuns(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]*DigitalEmployeeRun, error) {
	return nil, nil
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
	} else if f.run != nil && f.run.ID == req.RunID {
		run = f.run
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

func (f *fakeRunServiceRepository) CreateTaskEventIfAbsent(_ context.Context, req CreateRunEventRecordRequest) error {
	f.events = append(f.events, req)
	return nil
}

func (f *fakeRunServiceRepository) UpsertProviderSession(context.Context, UpsertProviderSessionRequest) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (f *fakeRunServiceRepository) CreateProviderSessionEventIfAbsent(context.Context, CreateProviderSessionEventRecordRequest) error {
	return nil
}

func (f *fakeRunServiceRepository) CreateCommandReceipt(_ context.Context, req CreateRuntimeCommandReceiptRequest) error {
	f.commandReceipts = append(f.commandReceipts, req)
	return nil
}

func (f *fakeRunServiceRepository) GetCommandReceipt(context.Context, uuid.UUID, string) (*RuntimeCommandReceipt, error) {
	return nil, ErrNotFound
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
