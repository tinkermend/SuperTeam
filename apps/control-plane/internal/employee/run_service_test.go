package employee

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

	runs, err := service.ListRuns(context.Background(), staleRun.TenantID, staleRun.DigitalEmployeeID, 10, 0)

	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != DigitalEmployeeRunStatusCancelled {
		t.Fatalf("expected stale run returned as cancelled, got %#v", runs)
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

func TestRunServiceCreateRunRejectsPreflightWithoutApprovedEffectiveConfig(t *testing.T) {
	repo := newFakeRunServiceRepository()
	repo.preflight = validRunServicePreflight()
	repo.preflight.HasApprovedEffectiveConfig = false
	dispatcher := newFakeRunServiceDispatcher()
	dispatcher.connected[repo.preflight.NodeID] = true
	service := mustNewRunService(t, repo, dispatcher)

	_, err := service.CreateRun(context.Background(), validCreateRunServiceRequest())

	if !errors.Is(err, ErrEffectiveConfigRequired) {
		t.Fatalf("expected ErrEffectiveConfigRequired, got %v", err)
	}
	if repo.createdRunCount != 0 || len(dispatcher.commands) != 0 {
		t.Fatalf("expected preflight rejection before create/dispatch")
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
	preflight           RunPreflight
	activeRun           *DigitalEmployeeRun
	run                 *DigitalEmployeeRun
	runs                []*DigitalEmployeeRun
	createdRun          *DigitalEmployeeRun
	createdRunCount     int
	createRunRequests   []CreateRunRecordRequest
	statusUpdates       []UpdateRunStatusRequest
	events              []CreateRunEventRecordRequest
	runEvents           []RuntimeCommandEventWriteback
	listRunEventsTaskID uuid.UUID
	listRunEventsRunID  uuid.UUID
	commandReceipt      *RuntimeCommandReceipt
	commandReceipts     []CreateRuntimeCommandReceiptRequest
	receiptUpdates      []UpdateRuntimeCommandReceiptRequest
}

func newFakeRunServiceRepository() *fakeRunServiceRepository {
	return &fakeRunServiceRepository{}
}

func (f *fakeRunServiceRepository) GetRunPreflight(context.Context, uuid.UUID, uuid.UUID) (RunPreflight, error) {
	return f.preflight, nil
}

func (f *fakeRunServiceRepository) WithTransaction(ctx context.Context, fn func(DigitalEmployeeRunRepository) error) error {
	return fn(f)
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

func (f *fakeRunServiceRepository) ListRuns(_ context.Context, tenantID, employeeID uuid.UUID, _ int32, _ int32) ([]*DigitalEmployeeRun, error) {
	out := make([]*DigitalEmployeeRun, 0, len(f.runs))
	for _, run := range f.runs {
		if run.TenantID == tenantID && run.DigitalEmployeeID == employeeID {
			out = append(out, cloneRun(run))
		}
	}
	return out, nil
}

func (f *fakeRunServiceRepository) ListRunEvents(_ context.Context, _ uuid.UUID, taskID, runID uuid.UUID, _ int32, _ int32) ([]RuntimeCommandEventWriteback, error) {
	f.listRunEventsTaskID = taskID
	f.listRunEventsRunID = runID
	return f.runEvents, nil
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

func (f *fakeRunServiceRepository) CreateProviderSessionEventIfAbsent(context.Context, CreateProviderSessionEventRecordRequest) error {
	return nil
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
