package runtimecommand

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
)

func TestWritebackServiceMarksInstallSkillsFailReceiptFailed(t *testing.T) {
	repo := newFakeRepository()
	receipt := installSkillsReceipt("cmd-install-1")
	repo.receipts[receipt.CommandID] = receipt
	service := NewWritebackService(repo)

	err := service.Fail(context.Background(), validIdentity(receipt), receipt.CommandID, employee.RuntimeCommandTerminalWriteback{
		Status:       employee.DigitalEmployeeRunStatusFailed,
		ErrorMessage: stringPtr("install failed"),
		ErrorCode:    stringPtr("archive_missing"),
		Result:       map[string]any{"rolled_back": true},
	})

	if err != nil {
		t.Fatalf("expected fail writeback to succeed, got %v", err)
	}
	if len(repo.updates) != 1 {
		t.Fatalf("expected one receipt update, got %#v", repo.updates)
	}
	update := repo.updates[0]
	if update.Status != "failed" || update.ErrorMessage == nil || *update.ErrorMessage != "install failed" {
		t.Fatalf("unexpected update: %#v", update)
	}
	if update.Result["rolled_back"] != true {
		t.Fatalf("expected terminal result to be preserved, got %#v", update.Result)
	}
}

func TestWritebackServiceMarksInstallSkillsCompleteReceiptCompleted(t *testing.T) {
	repo := newFakeRepository()
	receipt := installSkillsReceipt("cmd-install-2")
	repo.receipts[receipt.CommandID] = receipt
	service := NewWritebackService(repo)

	err := service.Complete(context.Background(), validIdentity(receipt), receipt.CommandID, employee.RuntimeCommandTerminalWriteback{
		Status:  employee.DigitalEmployeeRunStatusCompleted,
		Summary: "installed",
		Result:  map[string]any{"installed_count": float64(2)},
	})

	if err != nil {
		t.Fatalf("expected complete writeback to succeed, got %v", err)
	}
	if len(repo.updates) != 1 {
		t.Fatalf("expected one receipt update, got %#v", repo.updates)
	}
	update := repo.updates[0]
	if update.Status != "completed" {
		t.Fatalf("expected completed update, got %#v", update)
	}
	if update.Result["installed_count"] != float64(2) || update.Result["summary"] != "installed" {
		t.Fatalf("expected terminal result and summary, got %#v", update.Result)
	}
}

func TestWritebackServiceRejectsInstallSkillsRuntimeIdentityMismatch(t *testing.T) {
	repo := newFakeRepository()
	receipt := installSkillsReceipt("cmd-install-3")
	repo.receipts[receipt.CommandID] = receipt
	service := NewWritebackService(repo)

	identity := validIdentity(receipt)
	identity.RuntimeNodeID = uuid.MustParse("00000000-0000-0000-0000-000000000999")
	err := service.Fail(context.Background(), identity, receipt.CommandID, employee.RuntimeCommandTerminalWriteback{
		Status: employee.DigitalEmployeeRunStatusFailed,
	})

	if !errors.Is(err, employee.ErrRuntimeIdentityMismatch) {
		t.Fatalf("expected identity mismatch, got %v", err)
	}
	if len(repo.updates) != 0 {
		t.Fatalf("expected no updates on identity mismatch, got %#v", repo.updates)
	}
}

func installSkillsReceipt(commandID string) *employee.RuntimeCommandReceipt {
	now := time.Now().UTC()
	return &employee.RuntimeCommandReceipt{
		ID:            uuid.MustParse("00000000-0000-0000-0000-000000000010"),
		TenantID:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		CommandID:     commandID,
		CommandType:   "install_skills",
		RuntimeNodeID: uuid.MustParse("00000000-0000-0000-0000-000000000444"),
		NodeID:        "runtime-node-1",
		ResourceType:  "skill",
		ResourceID:    uuid.MustParse("00000000-0000-0000-0000-000000000333"),
		Status:        "pending",
		Payload:       map[string]any{"command_id": commandID},
		DispatchedAt:  &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func validIdentity(receipt *employee.RuntimeCommandReceipt) employee.RuntimeCommandWritebackIdentity {
	return employee.RuntimeCommandWritebackIdentity{
		TenantID:      receipt.TenantID,
		RuntimeNodeID: receipt.RuntimeNodeID,
		NodeID:        receipt.NodeID,
	}
}

func stringPtr(value string) *string {
	return &value
}

type fakeRepository struct {
	receipts map[string]*employee.RuntimeCommandReceipt
	updates  []employee.UpdateRuntimeCommandReceiptRequest
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{receipts: map[string]*employee.RuntimeCommandReceipt{}}
}

func (f *fakeRepository) WithTransaction(ctx context.Context, fn func(employee.DigitalEmployeeRunRepository) error) error {
	return fn(fakeRunRepository{f})
}

func (f *fakeRepository) GetCommandReceiptForUpdate(_ context.Context, tenantID uuid.UUID, commandID string) (*employee.RuntimeCommandReceipt, error) {
	receipt := f.receipts[commandID]
	if receipt == nil || receipt.TenantID != tenantID {
		return nil, employee.ErrNotFound
	}
	return receipt, nil
}

func (f *fakeRepository) UpdateCommandReceipt(_ context.Context, req employee.UpdateRuntimeCommandReceiptRequest) (*employee.RuntimeCommandReceipt, error) {
	receipt := f.receipts[req.CommandID]
	if receipt == nil || receipt.TenantID != req.TenantID {
		return nil, employee.ErrNotFound
	}
	f.updates = append(f.updates, req)
	updated := *receipt
	updated.Status = req.Status
	updated.Result = req.Result
	updated.ErrorMessage = req.ErrorMessage
	f.receipts[req.CommandID] = &updated
	return &updated, nil
}

type fakeRunRepository struct {
	*fakeRepository
}

func (f fakeRunRepository) GetRunPreflight(context.Context, uuid.UUID, uuid.UUID) (employee.RunPreflight, error) {
	return employee.RunPreflight{}, employee.ErrInvalidInput
}

func (f fakeRunRepository) GetActiveRun(context.Context, uuid.UUID, uuid.UUID) (*employee.DigitalEmployeeRun, error) {
	return nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) GetRun(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*employee.DigitalEmployeeRun, error) {
	return nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) GetRunByID(context.Context, uuid.UUID, uuid.UUID) (*employee.DigitalEmployeeRun, error) {
	return nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) GetRunByCommandID(context.Context, uuid.UUID, string) (*employee.DigitalEmployeeRun, error) {
	return nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) ListRunsDetailed(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ employee.DigitalEmployeeRunListFilter) (*employee.DigitalEmployeeRunListResult, error) {
	return nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) ListRunEvents(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int32, int32) ([]employee.RuntimeCommandEventWriteback, error) {
	return nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) GetLatestDigitalEmployeeConfigRevision(context.Context, uuid.UUID, uuid.UUID) (employee.EmployeeConfigInput, error) {
	return employee.EmployeeConfigInput{}, employee.ErrInvalidInput
}

func (f fakeRunRepository) CreateRun(context.Context, employee.CreateRunRecordRequest) (*employee.DigitalEmployeeRun, error) {
	return nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) UpdateRunStatus(context.Context, employee.UpdateRunStatusRequest) (*employee.DigitalEmployeeRun, error) {
	return nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) HasRunEventSequence(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int32) (bool, error) {
	return false, employee.ErrInvalidInput
}

func (f fakeRunRepository) CreateTaskEventIfAbsent(context.Context, employee.CreateRunEventRecordRequest) (bool, error) {
	return false, employee.ErrInvalidInput
}

func (f fakeRunRepository) UpsertProviderSession(context.Context, employee.UpsertProviderSessionRequest) (uuid.UUID, error) {
	return uuid.Nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) FindProviderSessionForTaskRoot(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (string, error) {
	return "", employee.ErrInvalidInput
}

func (f fakeRunRepository) GetRunTaskMetadata(context.Context, uuid.UUID, uuid.UUID) (map[string]any, error) {
	return nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) CreateProviderSessionEventIfAbsent(context.Context, employee.CreateProviderSessionEventRecordRequest) (uuid.UUID, error) {
	return uuid.Nil, employee.ErrInvalidInput
}

func (f fakeRunRepository) CreateCommandReceipt(context.Context, employee.CreateRuntimeCommandReceiptRequest) error {
	return employee.ErrInvalidInput
}

func (f fakeRunRepository) GetCommandReceipt(ctx context.Context, tenantID uuid.UUID, commandID string) (*employee.RuntimeCommandReceipt, error) {
	return f.fakeRepository.GetCommandReceiptForUpdate(ctx, tenantID, commandID)
}

func (f fakeRunRepository) UpdateExecutionInstanceStatus(context.Context, uuid.UUID, uuid.UUID, employee.ExecutionInstanceStatus, *string) (employee.DigitalEmployeeExecutionInstanceRecord, error) {
	return employee.DigitalEmployeeExecutionInstanceRecord{}, employee.ErrInvalidInput
}

func (f fakeRunRepository) UpdateDigitalEmployeeStatus(context.Context, uuid.UUID, uuid.UUID, employee.DigitalEmployeeStatus) (employee.DigitalEmployeeRecord, error) {
	return employee.DigitalEmployeeRecord{}, employee.ErrInvalidInput
}

func (f fakeRunRepository) DeleteExecutionInstance(context.Context, uuid.UUID, uuid.UUID) error {
	return employee.ErrInvalidInput
}

func (f fakeRunRepository) DeleteDigitalEmployee(context.Context, uuid.UUID, uuid.UUID) error {
	return employee.ErrInvalidInput
}

func (f fakeRunRepository) GetDigitalEmployeeRunStats(context.Context, uuid.UUID, uuid.UUID) (employee.DigitalEmployeeRunStats, error) {
	return employee.DigitalEmployeeRunStats{}, employee.ErrInvalidInput
}
