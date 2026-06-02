package employee

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateDraftDigitalEmployeeDefaultsAndTrims(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()

	created, err := svc.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID: tenantID,
		Name:     "  Finance reviewer  ",
		Role:     "  finance_reviewer  ",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	if created.TenantID != tenantID {
		t.Fatalf("expected tenant id %s, got %s", tenantID, created.TenantID)
	}
	if created.Name != "Finance reviewer" {
		t.Fatalf("expected trimmed name, got %q", created.Name)
	}
	if created.Role != "finance_reviewer" {
		t.Fatalf("expected trimmed role, got %q", created.Role)
	}
	if created.Status != DigitalEmployeeStatusDraft {
		t.Fatalf("expected draft status, got %q", created.Status)
	}
	if created.RiskLevel != "medium" {
		t.Fatalf("expected default risk level medium, got %q", created.RiskLevel)
	}
	assertEmptyMap(t, created.PermissionPolicy, "permission policy")
	assertEmptyMap(t, created.ContextPolicy, "context policy")
	assertEmptyMap(t, created.ApprovalPolicy, "approval policy")
	assertEmptyMap(t, created.Metadata, "metadata")
}

func TestBindExecutionInstanceReplacesExistingForEmployee(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	firstRuntimeID := uuid.New()
	secondRuntimeID := uuid.New()

	first, err := svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RuntimeNodeID:     firstRuntimeID,
		ProviderType:      "codex",
		AgentHomeDir:      "/srv/superteam/employees/finance",
		SessionPolicy:     map[string]any{"max_turns": float64(5)},
	})
	if err != nil {
		t.Fatalf("first bind: %v", err)
	}
	second, err := svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RuntimeNodeID:     secondRuntimeID,
		ProviderType:      "opencode",
		AgentHomeDir:      "/srv/superteam/employees/finance-v2",
		SessionPolicy:     map[string]any{"max_turns": float64(12)},
	})
	if err != nil {
		t.Fatalf("second bind: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected bind to replace same execution instance row id, got %s then %s", first.ID, second.ID)
	}
	if second.RuntimeNodeID != secondRuntimeID {
		t.Fatalf("expected updated runtime node id %s, got %s", secondRuntimeID, second.RuntimeNodeID)
	}
	if second.ProviderType != "opencode" {
		t.Fatalf("expected updated provider type, got %q", second.ProviderType)
	}
	if second.AgentHomeDir != "/srv/superteam/employees/finance-v2" {
		t.Fatalf("expected updated agent home dir, got %q", second.AgentHomeDir)
	}
	if second.SessionPolicy["max_turns"] != float64(12) {
		t.Fatalf("expected updated session policy, got %#v", second.SessionPolicy)
	}
	if second.Status != ExecutionInstanceStatusReady {
		t.Fatalf("expected ready status, got %q", second.Status)
	}
}

func TestServiceValidation(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "create requires tenant",
			run: func() error {
				_, err := svc.CreateDraft(context.Background(), CreateDraftRequest{Name: "employee", Role: "reviewer"})
				return err
			},
		},
		{
			name: "create requires name",
			run: func() error {
				_, err := svc.CreateDraft(context.Background(), CreateDraftRequest{TenantID: tenantID, Name: " ", Role: "reviewer"})
				return err
			},
		},
		{
			name: "create requires role",
			run: func() error {
				_, err := svc.CreateDraft(context.Background(), CreateDraftRequest{TenantID: tenantID, Name: "employee", Role: " "})
				return err
			},
		},
		{
			name: "bind requires provider",
			run: func() error {
				_, err := svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
					TenantID:          tenantID,
					DigitalEmployeeID: employeeID,
					RuntimeNodeID:     runtimeNodeID,
					AgentHomeDir:      "/tmp/agent",
				})
				return err
			},
		},
		{
			name: "bind requires agent home dir",
			run: func() error {
				_, err := svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
					TenantID:          tenantID,
					DigitalEmployeeID: employeeID,
					RuntimeNodeID:     runtimeNodeID,
					ProviderType:      "codex",
					AgentHomeDir:      " ",
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func assertEmptyMap(t *testing.T, value map[string]any, label string) {
	t.Helper()
	if value == nil {
		t.Fatalf("expected %s to default to empty map, got nil", label)
	}
	if len(value) != 0 {
		t.Fatalf("expected empty %s, got %#v", label, value)
	}
}

type memoryRepository struct {
	employees map[uuid.UUID]DigitalEmployeeRecord
	instances map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		employees: make(map[uuid.UUID]DigitalEmployeeRecord),
		instances: make(map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord),
	}
}

func (r *memoryRepository) CreateDigitalEmployee(_ context.Context, params CreateDigitalEmployeeParams) (DigitalEmployeeRecord, error) {
	now := time.Now().UTC()
	record := DigitalEmployeeRecord{
		ID:               uuid.New(),
		TenantID:         params.TenantID,
		TeamID:           params.TeamID,
		Name:             params.Name,
		Role:             params.Role,
		Description:      params.Description,
		Status:           params.Status,
		PermissionPolicy: cloneMap(params.PermissionPolicy),
		ContextPolicy:    cloneMap(params.ContextPolicy),
		ApprovalPolicy:   cloneMap(params.ApprovalPolicy),
		RiskLevel:        params.RiskLevel,
		Metadata:         cloneMap(params.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	r.employees[record.ID] = record
	return record, nil
}

func (r *memoryRepository) ListDigitalEmployees(_ context.Context, params ListDigitalEmployeesParams) ([]DigitalEmployeeRecord, error) {
	records := make([]DigitalEmployeeRecord, 0, len(r.employees))
	for _, record := range r.employees {
		if record.TenantID != params.TenantID {
			continue
		}
		if params.Status != "" && record.Status != params.Status {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *memoryRepository) GetDigitalEmployee(_ context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error) {
	record, ok := r.employees[employeeID]
	if !ok || record.TenantID != tenantID {
		return DigitalEmployeeRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) UpdateDigitalEmployeeStatus(_ context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error) {
	record, ok := r.employees[employeeID]
	if !ok || record.TenantID != tenantID {
		return DigitalEmployeeRecord{}, ErrNotFound
	}
	record.Status = status
	record.UpdatedAt = time.Now().UTC()
	r.employees[employeeID] = record
	return record, nil
}

func (r *memoryRepository) UpsertDigitalEmployeeExecutionInstance(_ context.Context, params UpsertExecutionInstanceParams) (DigitalEmployeeExecutionInstanceRecord, error) {
	if params.TenantID == uuid.Nil || params.DigitalEmployeeID == uuid.Nil {
		return DigitalEmployeeExecutionInstanceRecord{}, errors.New("tenant and employee are required")
	}
	now := time.Now().UTC()
	record, ok := r.instances[params.DigitalEmployeeID]
	if !ok {
		record.ID = uuid.New()
		record.CreatedAt = now
	}
	record.TenantID = params.TenantID
	record.DigitalEmployeeID = params.DigitalEmployeeID
	record.RuntimeNodeID = params.RuntimeNodeID
	record.ProviderType = params.ProviderType
	record.AgentHomeDir = params.AgentHomeDir
	record.WorkspacePolicy = cloneMap(params.WorkspacePolicy)
	record.SessionPolicy = cloneMap(params.SessionPolicy)
	record.RuntimeSelector = cloneMap(params.RuntimeSelector)
	record.CapacityRequirements = cloneMap(params.CapacityRequirements)
	record.FallbackPolicy = cloneMap(params.FallbackPolicy)
	record.Status = params.Status
	record.Metadata = cloneMap(params.Metadata)
	record.UpdatedAt = now
	r.instances[params.DigitalEmployeeID] = record
	return record, nil
}

func (r *memoryRepository) GetDigitalEmployeeExecutionInstanceByEmployeeID(_ context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeExecutionInstanceRecord, error) {
	record, ok := r.instances[employeeID]
	if !ok || record.TenantID != tenantID {
		return DigitalEmployeeExecutionInstanceRecord{}, ErrNotFound
	}
	return record, nil
}
