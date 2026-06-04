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
		Metadata:                  map[string]any{"stream": "stdout"},
	}

	if err := service.RecordEvent(context.Background(), run.TenantID, "cmd-1", event); err != nil {
		t.Fatalf("record event: %v", err)
	}
	if err := service.RecordEvent(context.Background(), run.TenantID, "cmd-1", event); err != nil {
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
}

func TestWritebackTerminalDoesNotChangeExistingTerminalRun(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-1")
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})

	if err := service.Complete(context.Background(), run.TenantID, "cmd-1", RuntimeCommandTerminalWriteback{
		Status:  DigitalEmployeeRunStatusCompleted,
		Summary: "done",
		Result:  map[string]any{"summary": "done"},
	}); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	lateFailure := "late failure"
	err := service.Fail(context.Background(), run.TenantID, "cmd-1", RuntimeCommandTerminalWriteback{
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
		writeback     func(context.Context, *DigitalEmployeeRunWritebackService, uuid.UUID, string, RuntimeCommandTerminalWriteback) error
		wantTimedOut  bool
		wantResultKey string
	}{
		{
			name:   "cancelled",
			status: DigitalEmployeeRunStatusCancelled,
			writeback: func(ctx context.Context, service *DigitalEmployeeRunWritebackService, tenantID uuid.UUID, commandID string, terminal RuntimeCommandTerminalWriteback) error {
				return service.Cancel(ctx, tenantID, commandID, terminal)
			},
		},
		{
			name:   "timed_out",
			status: DigitalEmployeeRunStatusTimedOut,
			writeback: func(ctx context.Context, service *DigitalEmployeeRunWritebackService, tenantID uuid.UUID, commandID string, terminal RuntimeCommandTerminalWriteback) error {
				return service.TimedOut(ctx, tenantID, commandID, terminal)
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

			if err := tt.writeback(context.Background(), service, run.TenantID, run.CommandID, RuntimeCommandTerminalWriteback{
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

	err := service.RecordEvent(context.Background(), runWritebackTenantID, "cmd-missing", RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 1,
		Payload:        map[string]any{"text": "hello"},
	})

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestWritebackTerminalStatusMismatchRejected(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-1")
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})

	err := service.Complete(context.Background(), run.TenantID, "cmd-1", RuntimeCommandTerminalWriteback{
		Status: DigitalEmployeeRunStatusFailed,
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if len(repo.runUpdates) != 0 || len(repo.receiptUpdates) != 0 || repo.taskEventInsertCount != 0 {
		t.Fatalf("expected mismatch rejection before writes, run=%#v receipt=%#v events=%d", repo.runUpdates, repo.receiptUpdates, repo.taskEventInsertCount)
	}
}

func TestWritebackEventWithoutProviderSessionStillWritesTaskEvent(t *testing.T) {
	repo := newFakeRunWritebackRepository()
	run := validWritebackRun(DigitalEmployeeRunStatusRunning, "cmd-1")
	repo.putRun(run)
	repo.putReceipt(validWritebackReceipt(run))
	service := mustNewRunWritebackService(t, repo, &fakeWritebackAuditLogger{})

	if err := service.RecordEvent(context.Background(), run.TenantID, "cmd-1", RuntimeCommandEventWriteback{
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
	taskEventKeys                   map[string]struct{}
	providerSessionEventKeys        map[string]struct{}
	providerSessionIDs              map[string]uuid.UUID
	taskEvents                      []CreateRunEventRecordRequest
	providerSessionEvents           []CreateProviderSessionEventRecordRequest
	providerSessionUpserts          []UpsertProviderSessionRequest
	runUpdates                      []UpdateRunStatusRequest
	receiptUpdates                  []UpdateRuntimeCommandReceiptRequest
	taskEventInsertCount            int
	providerSessionEventInsertCount int
}

func newFakeRunWritebackRepository() *fakeRunWritebackRepository {
	return &fakeRunWritebackRepository{
		runsByCommand:            map[string]*DigitalEmployeeRun{},
		runsByID:                 map[uuid.UUID]*DigitalEmployeeRun{},
		receipts:                 map[string]*RuntimeCommandReceipt{},
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

func (f *fakeRunWritebackRepository) GetRunPreflight(context.Context, uuid.UUID, uuid.UUID) (RunPreflight, error) {
	return RunPreflight{}, ErrNotFound
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

func (f *fakeRunWritebackRepository) CreateTaskEventIfAbsent(_ context.Context, req CreateRunEventRecordRequest) error {
	key := fmt.Sprintf("%s:%s:%d", req.TenantID, req.RunID, req.SequenceNumber)
	if _, exists := f.taskEventKeys[key]; exists {
		return nil
	}
	f.taskEventKeys[key] = struct{}{}
	req.Payload = redactRuntimeEventPayload(req.Payload)
	f.taskEvents = append(f.taskEvents, req)
	f.taskEventInsertCount++
	return nil
}

func (f *fakeRunWritebackRepository) UpsertProviderSession(_ context.Context, req UpsertProviderSessionRequest) (uuid.UUID, error) {
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
