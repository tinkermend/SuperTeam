package employee

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	return &Service{repository: repository}, nil
}

func (s *Service) CreateDraft(ctx context.Context, req CreateDraftRequest) (*DigitalEmployee, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		return nil, fmt.Errorf("%w: role is required", ErrInvalidInput)
	}
	description := trimOptionalString(req.Description)
	riskLevel := strings.TrimSpace(req.RiskLevel)
	if riskLevel == "" {
		riskLevel = "medium"
	}

	record, err := s.repository.CreateDigitalEmployee(ctx, CreateDigitalEmployeeParams{
		TenantID:         req.TenantID,
		TeamID:           validUUIDPtr(req.TeamID),
		Name:             name,
		Role:             role,
		Description:      description,
		Status:           DigitalEmployeeStatusDraft,
		PermissionPolicy: cloneMap(req.PermissionPolicy),
		ContextPolicy:    cloneMap(req.ContextPolicy),
		ApprovalPolicy:   cloneMap(req.ApprovalPolicy),
		RiskLevel:        riskLevel,
		Metadata:         cloneMap(req.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("create digital employee: %w", err)
	}
	return employeeFromRecord(record), nil
}

func (s *Service) ListDigitalEmployees(ctx context.Context, req ListDigitalEmployeesRequest) ([]*DigitalEmployee, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.Status != "" && !req.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	records, err := s.repository.ListDigitalEmployees(ctx, ListDigitalEmployeesParams{
		TenantID: req.TenantID,
		TeamID:   validUUIDPtr(req.TeamID),
		Status:   req.Status,
		Offset:   req.Offset,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list digital employees: %w", err)
	}
	employees := make([]*DigitalEmployee, 0, len(records))
	for _, record := range records {
		employees = append(employees, employeeFromRecord(record))
	}
	return employees, nil
}

func (s *Service) GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployee, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetDigitalEmployee(ctx, tenantID, employeeID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee: %w", err)
	}
	return employeeFromRecord(record), nil
}

func (s *Service) UpdateStatus(ctx context.Context, req UpdateStatusRequest) (*DigitalEmployee, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	if !req.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}
	record, err := s.repository.UpdateDigitalEmployeeStatus(ctx, req.TenantID, req.DigitalEmployeeID, req.Status)
	if err != nil {
		return nil, fmt.Errorf("update digital employee status: %w", err)
	}
	return employeeFromRecord(record), nil
}

func (s *Service) BindExecutionInstance(ctx context.Context, req BindExecutionInstanceRequest) (*DigitalEmployeeExecutionInstance, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	if req.RuntimeNodeID == uuid.Nil {
		return nil, fmt.Errorf("%w: runtime_node_id is required", ErrInvalidInput)
	}
	providerType := strings.TrimSpace(req.ProviderType)
	if providerType == "" {
		return nil, fmt.Errorf("%w: provider_type is required", ErrInvalidInput)
	}
	agentHomeDir := strings.TrimSpace(req.AgentHomeDir)
	if agentHomeDir == "" {
		return nil, fmt.Errorf("%w: agent_home_dir is required", ErrInvalidInput)
	}

	record, err := s.repository.UpsertDigitalEmployeeExecutionInstance(ctx, UpsertExecutionInstanceParams{
		TenantID:             req.TenantID,
		DigitalEmployeeID:    req.DigitalEmployeeID,
		RuntimeNodeID:        req.RuntimeNodeID,
		ProviderType:         providerType,
		AgentHomeDir:         agentHomeDir,
		WorkspacePolicy:      cloneMap(req.WorkspacePolicy),
		SessionPolicy:        cloneMap(req.SessionPolicy),
		RuntimeSelector:      cloneMap(req.RuntimeSelector),
		CapacityRequirements: cloneMap(req.CapacityRequirements),
		FallbackPolicy:       cloneMap(req.FallbackPolicy),
		Status:               ExecutionInstanceStatusReady,
		Metadata:             cloneMap(req.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("upsert digital employee execution instance: %w", err)
	}
	return executionInstanceFromRecord(record), nil
}

func (s *Service) GetExecutionInstance(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeExecutionInstance, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, tenantID, employeeID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee execution instance: %w", err)
	}
	return executionInstanceFromRecord(record), nil
}

func employeeFromRecord(record DigitalEmployeeRecord) *DigitalEmployee {
	return &DigitalEmployee{
		ID:               record.ID,
		TenantID:         record.TenantID,
		TeamID:           validUUIDPtr(record.TeamID),
		Name:             record.Name,
		Role:             record.Role,
		Description:      trimOptionalString(record.Description),
		Status:           record.Status,
		PermissionPolicy: cloneMap(record.PermissionPolicy),
		ContextPolicy:    cloneMap(record.ContextPolicy),
		ApprovalPolicy:   cloneMap(record.ApprovalPolicy),
		RiskLevel:        record.RiskLevel,
		Metadata:         cloneMap(record.Metadata),
		DisabledAt:       cloneTimePtr(record.DisabledAt),
		ArchivedAt:       cloneTimePtr(record.ArchivedAt),
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}
}

func executionInstanceFromRecord(record DigitalEmployeeExecutionInstanceRecord) *DigitalEmployeeExecutionInstance {
	return &DigitalEmployeeExecutionInstance{
		ID:                   record.ID,
		TenantID:             record.TenantID,
		DigitalEmployeeID:    record.DigitalEmployeeID,
		RuntimeNodeID:        record.RuntimeNodeID,
		ProviderType:         record.ProviderType,
		AgentHomeDir:         record.AgentHomeDir,
		WorkspacePolicy:      cloneMap(record.WorkspacePolicy),
		SessionPolicy:        cloneMap(record.SessionPolicy),
		RuntimeSelector:      cloneMap(record.RuntimeSelector),
		CapacityRequirements: cloneMap(record.CapacityRequirements),
		FallbackPolicy:       cloneMap(record.FallbackPolicy),
		Status:               record.Status,
		ReadyAt:              cloneTimePtr(record.ReadyAt),
		DisabledAt:           cloneTimePtr(record.DisabledAt),
		ErrorAt:              cloneTimePtr(record.ErrorAt),
		ErrorMessage:         trimOptionalString(record.ErrorMessage),
		Metadata:             cloneMap(record.Metadata),
		CreatedAt:            record.CreatedAt,
		UpdatedAt:            record.UpdatedAt,
	}
}

func validUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	copied := *value
	return &copied
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any)
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
