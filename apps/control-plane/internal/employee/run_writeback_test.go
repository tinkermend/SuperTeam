package employee

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWritebackEventCreatesTaskAndProviderSessionEventsIdempotently(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-1")
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})
	providerSessionExternalID := "provider-session-1"
	event := RuntimeCommandEventWriteback{
		EventType:                 "text_delta",
		SequenceNumber:            7,
		Payload:                   map[string]any{"text": "hello", "token": "secret"},
		ProviderSessionExternalID: &providerSessionExternalID,
		SessionStatePatch:         map[string]any{"cursor": "seq-7"},
		Metadata:                  map[string]any{"stream": "stdout", "token": "metadata-secret"},
	}

	identity := validWritebackIdentity(run)
	if err := service.RecordEvent(context.Background(), identity, "cmd-1", event); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if err := service.RecordEvent(context.Background(), identity, "cmd-1", event); err != nil {
		t.Fatalf("record duplicate event: %v", err)
	}

	if repo.taskEventInsertCount != 1 {
		t.Fatalf("expected exactly one effective task event insert, got %d", repo.taskEventInsertCount)
	}
	if repo.providerSessionEventInsertCount != 1 {
		t.Fatalf("expected exactly one effective provider session event insert, got %d", repo.providerSessionEventInsertCount)
	}
	insertedTaskEvent := repo.taskEvents[0]
	if insertedTaskEvent.EventType != "text_delta" || insertedTaskEvent.SequenceNumber != 7 {
		t.Fatalf("unexpected task event: %#v", insertedTaskEvent)
	}
	if insertedTaskEvent.CommandID == nil || *insertedTaskEvent.CommandID != "cmd-1" {
		t.Fatalf("expected task event command_id cmd-1, got %#v", insertedTaskEvent.CommandID)
	}
	if insertedTaskEvent.Payload["token"] != "[redacted]" {
		t.Fatalf("expected fake repository to redact task event token, got %#v", insertedTaskEvent.Payload)
	}
	if insertedTaskEvent.Metadata["token"] != "[redacted]" {
		t.Fatalf("expected task event metadata token redacted, got %#v", insertedTaskEvent.Metadata)
	}
	if event.Payload["token"] != "secret" {
		t.Fatalf("expected service/repository not to mutate caller payload, got %#v", event.Payload)
	}
	if len(repo.providerSessionUpserts) != 2 {
		t.Fatalf("expected duplicate writeback to still upsert session state twice, got %d", len(repo.providerSessionUpserts))
	}
	firstUpsert := repo.providerSessionUpserts[0]
	if firstUpsert.ProviderSessionID != providerSessionExternalID || firstUpsert.Status != "active" || !firstUpsert.Recoverable {
		t.Fatalf("unexpected provider session upsert: %#v", firstUpsert)
	}
	if firstUpsert.LastSequenceNumber != 7 || firstUpsert.LastCommandID == nil || *firstUpsert.LastCommandID != "cmd-1" {
		t.Fatalf("unexpected provider session last command/sequence: %#v", firstUpsert)
	}
	if firstUpsert.Metadata["token"] != "[redacted]" {
		t.Fatalf("expected provider session metadata token redacted, got %#v", firstUpsert.Metadata)
	}
	if repo.providerSessionEvents[0].Metadata["token"] != "[redacted]" {
		t.Fatalf("expected provider session event metadata token redacted, got %#v", repo.providerSessionEvents[0].Metadata)
	}
}

func TestWritebackTerminalDoesNotChangeExistingTerminalRun(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-1")
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})

	identity := validWritebackIdentity(run)
	if err := service.Complete(context.Background(), identity, "cmd-1", RuntimeCommandTerminalWriteback{
		Status:  DigitalEmployeeRunStatusCompleted,
		Summary: "done",
		Result:  map[string]any{"summary": "done"},
	}); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	lateFailure := "late failure"
	err := service.Fail(context.Background(), identity, "cmd-1", RuntimeCommandTerminalWriteback{
		Status:       DigitalEmployeeRunStatusFailed,
		ErrorMessage: &lateFailure,
	})

	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for late terminal mutation, got %v", err)
	}
	current := repo.runsByCommand["cmd-1"]
	if current.Status != DigitalEmployeeRunStatusCompleted {
		t.Fatalf("expected run to remain completed, got %s", current.Status)
	}
	if current.ErrorMessage != nil {
		t.Fatalf("expected late failure not to mutate terminal run error, got %#v", current.ErrorMessage)
	}
	if len(repo.runUpdates) != 1 {
		t.Fatalf("expected only initial complete update, got %#v", repo.runUpdates)
	}
	if len(repo.receiptUpdates) != 1 || repo.receiptUpdates[0].Status != "completed" {
		t.Fatalf("expected only completed receipt update, got %#v", repo.receiptUpdates)
	}
}

func TestWritebackCancelledAndTimedOutSetRunAndReceiptStatus(t *testing.T) {
	tests := []struct {
		name          string
		status        DigitalEmployeeRunStatus
		writeback     func(context.Context, *DigitalEmployeeRunWritebackService, RuntimeCommandWritebackIdentity, string, RuntimeCommandTerminalWriteback) error
		wantTimedOut  bool
		wantResultKey string
	}{
		{
			name:   "cancelled",
			status: DigitalEmployeeRunStatusCancelled,
			writeback: func(ctx context.Context, service *DigitalEmployeeRunWritebackService, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback) error {
				return service.Cancel(ctx, identity, commandID, terminal)
			},
		},
		{
			name:   "timed_out",
			status: DigitalEmployeeRunStatusTimedOut,
			writeback: func(ctx context.Context, service *DigitalEmployeeRunWritebackService, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback) error {
				return service.TimedOut(ctx, identity, commandID, terminal)
			},
			wantTimedOut:  true,
			wantResultKey: "timed_out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRunWritebackRepository()
			run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-"+tt.name)
			repo.putRun(run)
			repo.putReceipt(validWritebackReceipt(run))
			service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})

			if err := tt.writeback(context.Background(), service, validWritebackIdentity(run), run.CommandID, RuntimeCommandTerminalWriteback{
				Status: tt.status,
			}); err != nil {
				t.Fatalf("%s writeback: %v", tt.name, err)
			}

			if repo.runsByCommand[run.CommandID].Status != tt.status {
				t.Fatalf("expected run status %s, got %s", tt.status, repo.runsByCommand[run.CommandID].Status)
			}
			if len(repo.receiptUpdates) != 1 || repo.receiptUpdates[0].Status != string(tt.status) {
				t.Fatalf("expected receipt status %s, got %#v", tt.status, repo.receiptUpdates)
			}
			if len(repo.runUpdates) != 1 || repo.runUpdates[0].TimedOut != tt.wantTimedOut {
				t.Fatalf("expected TimedOut=%v on update, got %#v", tt.wantTimedOut, repo.runUpdates)
			}
			if tt.wantResultKey != "" && repo.receiptUpdates[0].Result[tt.wantResultKey] != true {
				t.Fatalf("expected receipt result %s=true, got %#v", tt.wantResultKey, repo.receiptUpdates[0].Result)
			}
		})
	}
}

func TestWritebackMissingCommandReturnsNotFound(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})

	err := service.RecordEvent(context.Background(), RuntimeCommandWritebackIdentity{
		TenantID:      runWritebackTenantID,
		RuntimeNodeID: runWritebackRuntimeNodeID,
		NodeID:        "runtime-node-1",
	}, "cmd-missing", RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 1,
		Payload:        map[string]any{"text": "hello"},
	})

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestWritebackWrongRuntimeIdentityRejected(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-1")
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})
	identity := validWritebackIdentity(run)
	identity.NodeID = "runtime-node-other"

	err := service.RecordEvent(context.Background(), identity, "cmd-1", RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 1,
		Payload:        map[string]any{"text": "hello"},
	})

	if !errors.Is(err, ErrRuntimeIdentityMismatch) {
		t.Fatalf("expected ErrRuntimeIdentityMismatch, got %v", err)
	}
	if repo.taskEventInsertCount != 0 || len(repo.providerSessionUpserts) != 0 {
		t.Fatalf("expected wrong runtime identity to reject before writes, events=%d upserts=%#v", repo.taskEventInsertCount, repo.providerSessionUpserts)
	}
}

func TestWritebackTerminalStatusMismatchRejected(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-1")
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})

	err := service.Complete(context.Background(), validWritebackIdentity(run), "cmd-1", RuntimeCommandTerminalWriteback{
		Status: DigitalEmployeeRunStatusFailed,
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if len(repo.runUpdates) != 0 || len(repo.receiptUpdates) != 0 || repo.taskEventInsertCount != 0 {
		t.Fatalf("expected mismatch rejection before writes, run=%#v receipt=%#v events=%d", repo.runUpdates, repo.receiptUpdates, repo.taskEventInsertCount)
	}
}

func TestWritebackTerminalReplayRepairsReceiptAndEventProjection(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusCompleted, "cmd-1")
	providerSessionExternalID := "provider-session-1"
	run.ProviderSessionExternalID = &providerSessionExternalID
	run.Result = map[string]any{"summary": "done", "status": string(DigitalEmployeeRunStatusCompleted), "details": "persisted"}
	run.Diagnostic = map[string]any{"exit_code": float64(0)}
	run.SessionState = map[string]any{"cursor": "seq-terminal", "token": "[redacted]"}
	run.WorkProducts = []WorkProduct{{
		Type:     "report",
		Title:    "报告",
		Metadata: map[string]any{"path": "s3://safe/ref"},
	}}
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})

	if err := service.Complete(context.Background(), validWritebackIdentity(run), "cmd-1", RuntimeCommandTerminalWriteback{
		Status:  DigitalEmployeeRunStatusCompleted,
		Summary: "done",
	}); err != nil {
		t.Fatalf("replay completed terminal writeback: %v", err)
	}

	if len(repo.runUpdates) != 0 {
		t.Fatalf("expected terminal replay not to mutate run status, got %#v", repo.runUpdates)
	}
	if repo.taskEventInsertCount != 1 {
		t.Fatalf("expected terminal replay to repair missing task event, got %d", repo.taskEventInsertCount)
	}
	if len(repo.receiptUpdates) != 1 || repo.receiptUpdates[0].Status != string(DigitalEmployeeRunStatusCompleted) {
		t.Fatalf("expected terminal replay to repair receipt status, got %#v", repo.receiptUpdates)
	}
	if repo.receiptUpdates[0].Result["details"] != "persisted" {
		t.Fatalf("expected receipt replay to use persisted run result, got %#v", repo.receiptUpdates[0].Result)
	}
	if repo.taskEvents[0].Payload["details"] != "persisted" {
		t.Fatalf("expected task event replay to use persisted run result, got %#v", repo.taskEvents[0].Payload)
	}
	if len(repo.providerSessionUpserts) != 1 || repo.providerSessionUpserts[0].SessionState["cursor"] != "seq-terminal" {
		t.Fatalf("expected provider session replay to use persisted run session state, got %#v", repo.providerSessionUpserts)
	}
	if repo.transactionCount != 1 || repo.lockedReceiptReadCount != 1 {
		t.Fatalf("expected terminal replay to lock receipt in transaction, tx=%d locks=%d", repo.transactionCount, repo.lockedReceiptReadCount)
	}
}

func TestWritebackTerminalReplayRejectsConflictingProjection(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusCompleted, "cmd-1")
	run.Result = map[string]any{"summary": "done", "status": string(DigitalEmployeeRunStatusCompleted), "details": "persisted"}
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})

	err := service.Complete(context.Background(), validWritebackIdentity(run), "cmd-1", RuntimeCommandTerminalWriteback{
		Status:  DigitalEmployeeRunStatusCompleted,
		Summary: "done",
		Result:  map[string]any{"details": "different"},
	})

	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for conflicting terminal replay, got %v", err)
	}
	if len(repo.receiptUpdates) != 0 || repo.taskEventInsertCount != 0 || len(repo.providerSessionUpserts) != 0 {
		t.Fatalf("expected conflicting replay to reject before writes, receipts=%#v events=%d upserts=%#v", repo.receiptUpdates, repo.taskEventInsertCount, repo.providerSessionUpserts)
	}
}

func TestWritebackTerminalRedactsRuntimeOriginatedJSON(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-1")
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})
	providerSessionExternalID := "provider-session-1"

	if err := service.Complete(context.Background(), validWritebackIdentity(run), "cmd-1", RuntimeCommandTerminalWriteback{
		Status:                    DigitalEmployeeRunStatusCompleted,
		Result:                    map[string]any{"token": "secret", "nested": map[string]any{"api_key": "key"}},
		Diagnostic:                map[string]any{"access_token": "access"},
		SessionStatePatch:         map[string]any{"refresh_token": "refresh", "cursor": "seq-10"},
		ProviderSessionExternalID: &providerSessionExternalID,
		WorkProducts: []WorkProduct{{
			Type:     "report",
			Title:    "报告",
			Metadata: map[string]any{"secret": "hidden", "path": "s3://safe/ref"},
		}},
	}); err != nil {
		t.Fatalf("complete run: %v", err)
	}

	if len(repo.runUpdates) != 1 {
		t.Fatalf("expected one run update, got %#v", repo.runUpdates)
	}
	update := repo.runUpdates[0]
	if update.Result["token"] != "[redacted]" {
		t.Fatalf("expected result token redacted, got %#v", update.Result)
	}
	nested, ok := update.Result["nested"].(map[string]any)
	if !ok || nested["api_key"] != "[redacted]" {
		t.Fatalf("expected nested api_key redacted, got %#v", update.Result)
	}
	if update.Diagnostic["access_token"] != "[redacted]" {
		t.Fatalf("expected diagnostic access_token redacted, got %#v", update.Diagnostic)
	}
	if update.SessionState["refresh_token"] != "[redacted]" || update.SessionState["cursor"] != "seq-10" {
		t.Fatalf("expected session state patch redacted and safe fields preserved, got %#v", update.SessionState)
	}
	if len(update.WorkProducts) != 1 || update.WorkProducts[0].Metadata["secret"] != "[redacted]" {
		t.Fatalf("expected work product metadata redacted, got %#v", update.WorkProducts)
	}
	if len(repo.receiptUpdates) != 1 || repo.receiptUpdates[0].Result["token"] != "[redacted]" {
		t.Fatalf("expected receipt result redacted, got %#v", repo.receiptUpdates)
	}
	if len(repo.providerSessionUpserts) != 1 || repo.providerSessionUpserts[0].SessionState["refresh_token"] != "[redacted]" {
		t.Fatalf("expected provider session state redacted, got %#v", repo.providerSessionUpserts)
	}
	if len(repo.providerSessionEvents) != 1 || repo.providerSessionEvents[0].SessionStatePatch["refresh_token"] != "[redacted]" {
		t.Fatalf("expected provider session event patch redacted, got %#v", repo.providerSessionEvents)
	}
}

func TestWritebackEventWithoutProviderSessionStillWritesTaskEvent(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-1")
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})

	if err := service.RecordEvent(context.Background(), validWritebackIdentity(run), "cmd-1", RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 2,
		Payload:        map[string]any{"text": "hello"},
	}); err != nil {
		t.Fatalf("record event: %v", err)
	}

	if repo.taskEventInsertCount != 1 {
		t.Fatalf("expected one task event, got %d", repo.taskEventInsertCount)
	}
	if repo.providerSessionEventInsertCount != 0 || len(repo.providerSessionUpserts) != 0 {
		t.Fatalf("expected no provider session writes, events=%d upserts=%#v", repo.providerSessionEventInsertCount, repo.providerSessionUpserts)
	}
}

func TestWritebackDuplicateEventAfterTerminalRunIsAcceptedIdempotently(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-1")
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})
	event := RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 7,
		Payload:        map[string]any{"text": "hello"},
	}

	identity := validWritebackIdentity(run)
	if err := service.RecordEvent(context.Background(), identity, "cmd-1", event); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if err := service.Complete(context.Background(), identity, "cmd-1", RuntimeCommandTerminalWriteback{
		Status:  DigitalEmployeeRunStatusCompleted,
		Summary: "done",
	}); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	if err := service.RecordEvent(context.Background(), identity, "cmd-1", event); err != nil {
		t.Fatalf("duplicate terminal event should be idempotent: %v", err)
	}
	err := service.RecordEvent(context.Background(), identity, "cmd-1", RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 8,
		Payload:        map[string]any{"text": "late"},
	})

	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for new terminal event, got %v", err)
	}
	if repo.taskEventInsertCount != 2 {
		t.Fatalf("expected original event plus terminal event only, got %d", repo.taskEventInsertCount)
	}
}

func TestWritebackCompleteProvisioningMarksInstanceAndEmployeeReady(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	receipt := validProvisioningReceipt("provision-cmd-1")
	repo.putReceipt(receipt)
	repo.putExecutionInstance(validProvisioningInstance(receipt.ResourceID))
	audit := &fakeWritebackAuditLogger{}
	service := mustNewRunWritebackService(t, repo, audit)

	if err := service.Complete(context.Background(), validProvisioningIdentity(receipt), receipt.CommandID, RuntimeCommandTerminalWriteback{
		Status: DigitalEmployeeRunStatusCompleted,
		Result: map[string]any{"agent_home_dir": "/srv/agents/code"},
	}); err != nil {
		t.Fatalf("complete provisioning: %v", err)
	}

	if len(repo.executionInstanceStatusUpdates) != 1 || repo.executionInstanceStatusUpdates[0].status != ExecutionInstanceStatusReady {
		t.Fatalf("expected execution instance ready update, got %#v", repo.executionInstanceStatusUpdates)
	}
	if len(repo.employeeStatusUpdates) != 1 || repo.employeeStatusUpdates[0].status != DigitalEmployeeStatusReady {
		t.Fatalf("expected employee ready update, got %#v", repo.employeeStatusUpdates)
	}
	if len(repo.receiptUpdates) != 1 || repo.receiptUpdates[0].Status != string(DigitalEmployeeRunStatusCompleted) {
		t.Fatalf("expected completed receipt update, got %#v", repo.receiptUpdates)
	}
	if len(audit.events) != 1 || audit.events[0].eventType != "digital_employee_instance_provisioned" {
		t.Fatalf("expected provisioning audit event, got %#v", audit.events)
	}
}

func TestWritebackFailProvisioningDeletesEmployeeAndInstance(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	receipt := validProvisioningReceipt("provision-cmd-1")
	repo.putReceipt(receipt)
	instance := validProvisioningInstance(receipt.ResourceID)
	repo.putExecutionInstance(instance)
	audit := &fakeWritebackAuditLogger{}
	service := mustNewRunWritebackService(t, repo, audit)
	errorMessage := "runtime failed to create workspace"

	if err := service.Fail(context.Background(), validProvisioningIdentity(receipt), receipt.CommandID, RuntimeCommandTerminalWriteback{
		Status:       DigitalEmployeeRunStatusFailed,
		ErrorMessage: &errorMessage,
	}); err != nil {
		t.Fatalf("fail provisioning: %v", err)
	}

	if len(repo.executionInstanceStatusUpdates) != 1 || repo.executionInstanceStatusUpdates[0].status != ExecutionInstanceStatusError {
		t.Fatalf("expected execution instance error update, got %#v", repo.executionInstanceStatusUpdates)
	}
	if repo.executionInstanceStatusUpdates[0].errorMessage == nil || *repo.executionInstanceStatusUpdates[0].errorMessage != errorMessage {
		t.Fatalf("expected execution instance error message, got %#v", repo.executionInstanceStatusUpdates[0].errorMessage)
	}
	if len(repo.deletedExecutionInstances) != 1 || repo.deletedExecutionInstances[0] != instance.ID {
		t.Fatalf("expected execution instance deletion, got %#v", repo.deletedExecutionInstances)
	}
	if len(repo.deletedEmployees) != 1 || repo.deletedEmployees[0] != instance.DigitalEmployeeID {
		t.Fatalf("expected employee deletion, got %#v", repo.deletedEmployees)
	}
	if len(repo.receiptUpdates) != 1 || repo.receiptUpdates[0].Status != string(DigitalEmployeeRunStatusFailed) {
		t.Fatalf("expected failed receipt update, got %#v", repo.receiptUpdates)
	}
	if len(audit.events) != 1 || audit.events[0].eventType != "digital_employee_instance_provision_failed" {
		t.Fatalf("expected provisioning failure audit event, got %#v", audit.events)
	}
}

func mustNewRunWritebackService(t *testing.T, repo DigitalEmployeeRunRepository, audit AuditLogger) *DigitalEmployeeRunWritebackService {
	t.Helper()
	service, err := NewDigitalEmployeeRunWritebackService(repo, audit)
	if err != nil {
		t.Fatalf("new writeback service: %v", err)
	}
	return service
}

func validWritebackRun(status DigitalEmployeeRunStatus, commandID string) *DigitalEmployeeRun {
	return &DigitalEmployeeRun{
		ID:                  uuid.New(),
		TenantID:            runWritebackTenantID,
		TaskID:              uuid.New(),
		DigitalEmployeeID:   runWritebackEmployeeID,
		ExecutionInstanceID: runWritebackExecutionInstanceID,
		RuntimeNodeID:       runWritebackRuntimeNodeID,
		NodeID:              "runtime-node-1",
		CommandID:           commandID,
		ProviderType:        "codex",
		Status:              status,
		Result:              map[string]any{},
		Diagnostic:          map[string]any{},
		SessionState:        map[string]any{},
		StartedAt:           time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}
}

func validWritebackReceipt(run *DigitalEmployeeRun) *RuntimeCommandReceipt {
	return &RuntimeCommandReceipt{
		ID:            uuid.New(),
		TenantID:      run.TenantID,
		CommandID:     run.CommandID,
		CommandType:   "start_session",
		RuntimeNodeID: run.RuntimeNodeID,
		NodeID:        run.NodeID,
		ResourceType:  "digital_employee_run",
		ResourceID:    run.ID,
		Status:        "dispatched",
		Payload:       map[string]any{"command_id": run.CommandID},
		Result:        map[string]any{},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
}

func validWritebackIdentity(run *DigitalEmployeeRun) RuntimeCommandWritebackIdentity {
	return RuntimeCommandWritebackIdentity{
		TenantID:      run.TenantID,
		RuntimeNodeID: run.RuntimeNodeID,
		NodeID:        run.NodeID,
	}
}

func validProvisioningIdentity(receipt *RuntimeCommandReceipt) RuntimeCommandWritebackIdentity {
	return RuntimeCommandWritebackIdentity{
		TenantID:      receipt.TenantID,
		RuntimeNodeID: receipt.RuntimeNodeID,
		NodeID:        receipt.NodeID,
	}
}

func validProvisioningReceipt(commandID string) *RuntimeCommandReceipt {
	return &RuntimeCommandReceipt{
		ID:            uuid.New(),
		TenantID:      runWritebackTenantID,
		CommandID:     commandID,
		CommandType:   "provision_instance",
		RuntimeNodeID: runWritebackRuntimeNodeID,
		NodeID:        "runtime-node-1",
		ResourceType:  "digital_employee_execution_instance",
		ResourceID:    runWritebackExecutionInstanceID,
		Status:        "dispatched",
		Payload:       map[string]any{"command_id": commandID},
		Result:        map[string]any{},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
}

func validProvisioningInstance(instanceID uuid.UUID) DigitalEmployeeExecutionInstanceRecord {
	return DigitalEmployeeExecutionInstanceRecord{
		ID:                instanceID,
		TenantID:          runWritebackTenantID,
		DigitalEmployeeID: runWritebackEmployeeID,
		RuntimeNodeID:     runWritebackRuntimeNodeID,
		ProviderType:      "codex",
		AgentHomeDir:      "/srv/agents/code",
		Status:            ExecutionInstanceStatusProvisioning,
		Metadata:          map[string]any{},
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
}

var (
	runWritebackTenantID            = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	runWritebackEmployeeID          = uuid.MustParse("00000000-0000-0000-0000-000000000501")
	runWritebackExecutionInstanceID = uuid.MustParse("00000000-0000-0000-0000-000000000502")
	runWritebackRuntimeNodeID       = uuid.MustParse("00000000-0000-0000-0000-000000000503")
)

type fakeRunWritebackRepository struct {
	runsByCommand                   map[string]*DigitalEmployeeRun
	runsByID                        map[uuid.UUID]*DigitalEmployeeRun
	receipts                        map[string]*RuntimeCommandReceipt
	executionInstances              map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord
	taskEventKeys                   map[string]struct{}
	providerSessionEventKeys        map[string]struct{}
	providerSessionIDs              map[string]uuid.UUID
	taskEvents                      []CreateRunEventRecordRequest
	providerSessionEvents           []CreateProviderSessionEventRecordRequest
	providerSessionUpserts          []UpsertProviderSessionRequest
	runUpdates                      []UpdateRunStatusRequest
	receiptUpdates                  []UpdateRuntimeCommandReceiptRequest
	executionInstanceStatusUpdates  []fakeExecutionInstanceStatusUpdate
	employeeStatusUpdates           []fakeEmployeeStatusUpdate
	deletedExecutionInstances       []uuid.UUID
	deletedEmployees                []uuid.UUID
	taskEventInsertCount            int
	providerSessionEventInsertCount int
	transactionCount                int
	lockedReceiptReadCount          int
}

type fakeExecutionInstanceStatusUpdate struct {
	tenantID            uuid.UUID
	executionInstanceID uuid.UUID
	status              ExecutionInstanceStatus
	errorMessage        *string
}

type fakeEmployeeStatusUpdate struct {
	tenantID   uuid.UUID
	employeeID uuid.UUID
	status     DigitalEmployeeStatus
}

func newFakeRunWritebackRepository() *fakeRunWritebackRepository {
	return &fakeRunWritebackRepository{
		runsByCommand:            map[string]*DigitalEmployeeRun{},
		runsByID:                 map[uuid.UUID]*DigitalEmployeeRun{},
		receipts:                 map[string]*RuntimeCommandReceipt{},
		executionInstances:       map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord{},
		taskEventKeys:            map[string]struct{}{},
		providerSessionEventKeys: map[string]struct{}{},
		providerSessionIDs:       map[string]uuid.UUID{},
	}
}

func (f *fakeRunWritebackRepository) putRun(run *DigitalEmployeeRun) {
	f.runsByCommand[run.CommandID] = cloneWritebackRun(run)
	f.runsByID[run.ID] = cloneWritebackRun(run)
}

func (f *fakeRunWritebackRepository) putReceipt(receipt *RuntimeCommandReceipt) {
	copied := *receipt
	f.receipts[receipt.CommandID] = &copied
}

func (f *fakeRunWritebackRepository) putExecutionInstance(instance DigitalEmployeeExecutionInstanceRecord) {
	f.executionInstances[instance.ID] = cloneExecutionInstanceRecord(instance)
}

func (f *fakeRunWritebackRepository) GetRunPreflight(context.Context, uuid.UUID, uuid.UUID) (RunPreflight, error) {
	return RunPreflight{}, ErrNotFound
}

func (f *fakeRunWritebackRepository) WithTransaction(ctx context.Context, fn func(DigitalEmployeeRunRepository) error) error {
	f.transactionCount++
	return fn(f)
}

func (f *fakeRunWritebackRepository) GetActiveRun(context.Context, uuid.UUID, uuid.UUID) (*DigitalEmployeeRun, error) {
	return nil, nil
}

func (f *fakeRunWritebackRepository) GetRun(_ context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error) {
	run, ok := f.runsByID[runID]
	if !ok || run.TenantID != tenantID || run.DigitalEmployeeID != employeeID {
		return nil, ErrNotFound
	}
	return cloneWritebackRun(run), nil
}

func (f *fakeRunWritebackRepository) GetRunByID(_ context.Context, tenantID, runID uuid.UUID) (*DigitalEmployeeRun, error) {
	run, ok := f.runsByID[runID]
	if !ok || run.TenantID != tenantID {
		return nil, ErrNotFound
	}
	return cloneWritebackRun(run), nil
}

func (f *fakeRunWritebackRepository) GetRunByCommandID(_ context.Context, tenantID uuid.UUID, commandID string) (*DigitalEmployeeRun, error) {
	run, ok := f.runsByCommand[commandID]
	if !ok || run.TenantID != tenantID {
		return nil, ErrNotFound
	}
	return cloneWritebackRun(run), nil
}

func (f *fakeRunWritebackRepository) ListRuns(context.Context, uuid.UUID, uuid.UUID, int32, int32) ([]*DigitalEmployeeRun, error) {
	return nil, nil
}

func (f *fakeRunWritebackRepository) CreateRun(context.Context, CreateRunRecordRequest) (*DigitalEmployeeRun, error) {
	return nil, ErrInvalidInput
}

func (f *fakeRunWritebackRepository) UpdateRunStatus(_ context.Context, req UpdateRunStatusRequest) (*DigitalEmployeeRun, error) {
	f.runUpdates = append(f.runUpdates, req)
	run, ok := f.runsByID[req.RunID]
	if !ok || run.TenantID != req.TenantID {
		return nil, ErrNotFound
	}
	run.Status = req.Status
	if req.Result != nil {
		run.Result = cloneMap(req.Result)
	}
	run.ErrorMessage = req.ErrorMessage
	if req.Diagnostic != nil {
		run.Diagnostic = cloneMap(req.Diagnostic)
	}
	run.LogRef = req.LogRef
	run.RawResultRef = req.RawResultRef
	if req.WorkProducts != nil {
		run.WorkProducts = append([]WorkProduct(nil), req.WorkProducts...)
	}
	if req.SessionState != nil {
		run.SessionState = cloneMap(req.SessionState)
	}
	run.ErrorCode = req.ErrorCode
	run.ErrorFamily = req.ErrorFamily
	run.ExitCode = req.ExitCode
	run.Signal = req.Signal
	run.ProviderSessionExternalID = req.ProviderSessionExternalID
	run.TimedOut = req.TimedOut
	f.runsByCommand[run.CommandID] = cloneWritebackRun(run)
	return cloneWritebackRun(run), nil
}

func (f *fakeRunWritebackRepository) HasRunEventSequence(_ context.Context, tenantID, _ uuid.UUID, runID uuid.UUID, sequenceNumber int32) (bool, error) {
	key := fmt.Sprintf("%s:%s:%d", tenantID, runID, sequenceNumber)
	_, exists := f.taskEventKeys[key]
	return exists, nil
}

func (f *fakeRunWritebackRepository) CreateTaskEventIfAbsent(_ context.Context, req CreateRunEventRecordRequest) error {
	key := fmt.Sprintf("%s:%s:%d", req.TenantID, req.RunID, req.SequenceNumber)
	if _, exists := f.taskEventKeys[key]; exists {
		return nil
	}
	f.taskEventKeys[key] = struct{}{}
	req.Payload = redactRuntimeEventPayload(req.Payload)
	req.Metadata = redactRuntimeEventPayload(req.Metadata)
	f.taskEvents = append(f.taskEvents, req)
	f.taskEventInsertCount++
	return nil
}

func (f *fakeRunWritebackRepository) UpsertProviderSession(_ context.Context, req UpsertProviderSessionRequest) (uuid.UUID, error) {
	req.SessionState = redactRuntimeEventPayload(req.SessionState)
	req.Metadata = redactRuntimeEventPayload(req.Metadata)
	f.providerSessionUpserts = append(f.providerSessionUpserts, req)
	key := req.TenantID.String() + ":" + req.ProviderType + ":" + req.ProviderSessionID
	id, ok := f.providerSessionIDs[key]
	if !ok {
		id = uuid.New()
		f.providerSessionIDs[key] = id
	}
	return id, nil
}

func (f *fakeRunWritebackRepository) CreateProviderSessionEventIfAbsent(_ context.Context, req CreateProviderSessionEventRecordRequest) error {
	commandID := ""
	if req.CommandID != nil {
		commandID = *req.CommandID
	}
	key := fmt.Sprintf("%s:%s:%d", req.TenantID, commandID, req.SequenceNumber)
	if _, exists := f.providerSessionEventKeys[key]; exists {
		return nil
	}
	f.providerSessionEventKeys[key] = struct{}{}
	req.Payload = redactRuntimeEventPayload(req.Payload)
	req.SessionStatePatch = redactRuntimeEventPayload(req.SessionStatePatch)
	req.Metadata = redactRuntimeEventPayload(req.Metadata)
	f.providerSessionEvents = append(f.providerSessionEvents, req)
	f.providerSessionEventInsertCount++
	return nil
}

func (f *fakeRunWritebackRepository) CreateCommandReceipt(context.Context, CreateRuntimeCommandReceiptRequest) error {
	return ErrInvalidInput
}

func (f *fakeRunWritebackRepository) GetCommandReceipt(_ context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error) {
	receipt, ok := f.receipts[commandID]
	if !ok || receipt.TenantID != tenantID {
		return nil, ErrNotFound
	}
	copied := *receipt
	return &copied, nil
}

func (f *fakeRunWritebackRepository) GetCommandReceiptForUpdate(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error) {
	f.lockedReceiptReadCount++
	return f.GetCommandReceipt(ctx, tenantID, commandID)
}

func (f *fakeRunWritebackRepository) UpdateCommandReceipt(_ context.Context, req UpdateRuntimeCommandReceiptRequest) (*RuntimeCommandReceipt, error) {
	f.receiptUpdates = append(f.receiptUpdates, req)
	receipt, ok := f.receipts[req.CommandID]
	if !ok || receipt.TenantID != req.TenantID {
		return nil, ErrNotFound
	}
	receipt.Status = req.Status
	if req.Result != nil {
		receipt.Result = cloneMap(req.Result)
	}
	receipt.ErrorMessage = req.ErrorMessage
	copied := *receipt
	return &copied, nil
}

func (f *fakeRunWritebackRepository) UpdateExecutionInstanceStatus(_ context.Context, tenantID, executionInstanceID uuid.UUID, status ExecutionInstanceStatus, errorMessage *string) (DigitalEmployeeExecutionInstanceRecord, error) {
	f.executionInstanceStatusUpdates = append(f.executionInstanceStatusUpdates, fakeExecutionInstanceStatusUpdate{
		tenantID:            tenantID,
		executionInstanceID: executionInstanceID,
		status:              status,
		errorMessage:        errorMessage,
	})
	instance, ok := f.executionInstances[executionInstanceID]
	if !ok || instance.TenantID != tenantID {
		return DigitalEmployeeExecutionInstanceRecord{}, ErrNotFound
	}
	instance.Status = status
	instance.ErrorMessage = errorMessage
	f.executionInstances[executionInstanceID] = instance
	return cloneExecutionInstanceRecord(instance), nil
}

func (f *fakeRunWritebackRepository) UpdateDigitalEmployeeStatus(_ context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error) {
	f.employeeStatusUpdates = append(f.employeeStatusUpdates, fakeEmployeeStatusUpdate{
		tenantID:   tenantID,
		employeeID: employeeID,
		status:     status,
	})
	return DigitalEmployeeRecord{
		ID:        employeeID,
		TenantID:  tenantID,
		Status:    status,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (f *fakeRunWritebackRepository) DeleteExecutionInstance(_ context.Context, tenantID, executionInstanceID uuid.UUID) error {
	instance, ok := f.executionInstances[executionInstanceID]
	if !ok || instance.TenantID != tenantID {
		return ErrNotFound
	}
	f.deletedExecutionInstances = append(f.deletedExecutionInstances, executionInstanceID)
	delete(f.executionInstances, executionInstanceID)
	return nil
}

func (f *fakeRunWritebackRepository) DeleteDigitalEmployee(_ context.Context, tenantID, employeeID uuid.UUID) error {
	f.deletedEmployees = append(f.deletedEmployees, employeeID)
	return nil
}

type fakeWritebackAuditLogger struct {
	events []fakeRunServiceAuditEvent
}

func (f *fakeWritebackAuditLogger) LogEvent(_ context.Context, eventType, actorType, actorID, resourceType, resourceID, action string) error {
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

func cloneWritebackRun(run *DigitalEmployeeRun) *DigitalEmployeeRun {
	if run == nil {
		return nil
	}
	copied := *run
	copied.Result = cloneMap(run.Result)
	copied.Diagnostic = cloneMap(run.Diagnostic)
	copied.SessionState = cloneMap(run.SessionState)
	copied.WorkProducts = append([]WorkProduct(nil), run.WorkProducts...)
	return &copied
}

func cloneExecutionInstanceRecord(instance DigitalEmployeeExecutionInstanceRecord) DigitalEmployeeExecutionInstanceRecord {
	instance.WorkspacePolicy = cloneMap(instance.WorkspacePolicy)
	instance.SessionPolicy = cloneMap(instance.SessionPolicy)
	instance.RuntimeSelector = cloneMap(instance.RuntimeSelector)
	instance.CapacityRequirements = cloneMap(instance.CapacityRequirements)
	instance.FallbackPolicy = cloneMap(instance.FallbackPolicy)
	instance.Metadata = cloneMap(instance.Metadata)
	return instance
}
