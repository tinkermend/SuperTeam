package employee

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/permission"
)

// 权限审批接缝未注入时提交治理变更必须明确失败(不静默吞掉)。
func TestSubmitPermissionChangeRequiresApprovalDependencies(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	role := "backend_engineer"
	_, err = svc.SubmitPermissionChange(context.Background(), SubmitPermissionChangeRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		RequesterUserID:   uuid.New(),
		Role:              &role,
	})
	if !errors.Is(err, ErrPermissionApprovalNotConfigured) {
		t.Fatalf("expected ErrPermissionApprovalNotConfigured, got %v", err)
	}
}

// ActivateConfigRevision:批准写回 role+permission_policy(A2, 方案2: 目标值随 ContextPayload)。
func TestActivateConfigRevisionWritesBackRoleAndPolicy(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	now := time.Now().UTC()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID: employeeID, TenantID: tenantID, OwnerUserID: uuid.New(),
		EmployeeType: "backend_engineer", ProviderType: "codex", Name: "写回员工",
		Role:             "backend_engineer",
		PermissionPolicy: map[string]any{"allowed_actions": []any{"code.read"}},
		Status:           DigitalEmployeeStatusReady, CreatedAt: now, UpdatedAt: now,
	}

	err = svc.ActivateConfigRevision(context.Background(), permission.ActivateConfigRevisionInput{
		TenantID:    tenantID,
		EmployeeID:  employeeID,
		RevisionID:  uuid.New(),
		ActivatedBy: uuid.New(),
		ContextPayload: map[string]any{
			"target_role":              "qa_engineer",
			"target_permission_policy": map[string]any{"allowed_actions": []any{"code.read", "code.write"}},
		},
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	record := repo.employees[employeeID]
	if record.Role != "qa_engineer" {
		t.Fatalf("expected role written back, got %q", record.Role)
	}
	actions := stringList(record.PermissionPolicy["allowed_actions"])
	if len(actions) != 2 || actions[1] != "code.write" {
		t.Fatalf("expected permission_policy written back, got %#v", record.PermissionPolicy)
	}
}

// 只改 role 时必须保留现有 permission_policy(回填,不得覆盖为空)。
func TestActivateConfigRevisionRoleOnlyPreservesPolicy(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	now := time.Now().UTC()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID: employeeID, TenantID: tenantID, OwnerUserID: uuid.New(),
		EmployeeType: "backend_engineer", ProviderType: "codex", Name: "只改角色",
		Role:             "backend_engineer",
		PermissionPolicy: map[string]any{"grants": []any{"database.read:dev_db"}},
		Status:           DigitalEmployeeStatusReady, CreatedAt: now, UpdatedAt: now,
	}

	err = svc.ActivateConfigRevision(context.Background(), permission.ActivateConfigRevisionInput{
		TenantID:       tenantID,
		EmployeeID:     employeeID,
		RevisionID:     uuid.New(),
		ActivatedBy:    uuid.New(),
		ContextPayload: map[string]any{"target_role": "code_reviewer"},
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	record := repo.employees[employeeID]
	if record.Role != "code_reviewer" {
		t.Fatalf("expected role updated, got %q", record.Role)
	}
	grants := stringList(record.PermissionPolicy["grants"])
	if len(grants) != 1 || grants[0] != "database.read:dev_db" {
		t.Fatalf("expected existing permission_policy preserved, got %#v", record.PermissionPolicy)
	}
}

// payload 无任何目标值时必须失败,不得静默通过。
func TestActivateConfigRevisionRejectsEmptyPayload(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	err = svc.ActivateConfigRevision(context.Background(), permission.ActivateConfigRevisionInput{
		TenantID:       uuid.New(),
		EmployeeID:     uuid.New(),
		RevisionID:     uuid.New(),
		ActivatedBy:    uuid.New(),
		ContextPayload: map[string]any{},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for empty payload, got %v", err)
	}
}
