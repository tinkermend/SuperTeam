package employee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateDigitalEmployee(ctx context.Context, params CreateDigitalEmployeeParams) (DigitalEmployeeRecord, error) {
	permissionPolicy, err := jsonbFromMap(params.PermissionPolicy, "permission_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	contextPolicy, err := jsonbFromMap(params.ContextPolicy, "context_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	approvalPolicy, err := jsonbFromMap(params.ApprovalPolicy, "approval_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}

	employee, err := r.q.CreateDigitalEmployee(ctx, queries.CreateDigitalEmployeeParams{
		TenantID:         params.TenantID,
		TeamID:           nullUUIDFromPtr(params.TeamID),
		Name:             params.Name,
		Role:             params.Role,
		Description:      textFromPtr(params.Description),
		Status:           string(params.Status),
		PermissionPolicy: permissionPolicy,
		ContextPolicy:    contextPolicy,
		ApprovalPolicy:   approvalPolicy,
		RiskLevel:        params.RiskLevel,
		Metadata:         metadata,
	})
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	return digitalEmployeeRecordFromQuery(employee)
}

func (r *PgRepository) ListDigitalEmployees(ctx context.Context, params ListDigitalEmployeesParams) ([]DigitalEmployeeRecord, error) {
	employees, err := r.q.ListDigitalEmployees(ctx, queries.ListDigitalEmployeesParams{
		TenantID: params.TenantID,
		TeamID:   nullUUIDFromPtr(params.TeamID),
		Status:   textFromStatus(params.Status),
		Offset:   params.Offset,
		Limit:    params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]DigitalEmployeeRecord, 0, len(employees))
	for _, employee := range employees {
		record, err := digitalEmployeeRecordFromQuery(employee)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *PgRepository) GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error) {
	employee, err := r.q.GetDigitalEmployee(ctx, queries.GetDigitalEmployeeParams{
		ID:       employeeID,
		TenantID: tenantID,
	})
	if err != nil {
		return DigitalEmployeeRecord{}, mapNoRows(err)
	}
	return digitalEmployeeRecordFromQuery(employee)
}

func (r *PgRepository) UpdateDigitalEmployeeStatus(ctx context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error) {
	employee, err := r.q.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		Status:   string(status),
		ID:       employeeID,
		TenantID: tenantID,
	})
	if err != nil {
		return DigitalEmployeeRecord{}, mapNoRows(err)
	}
	return digitalEmployeeRecordFromQuery(employee)
}

func (r *PgRepository) UpsertDigitalEmployeeExecutionInstance(ctx context.Context, params UpsertExecutionInstanceParams) (DigitalEmployeeExecutionInstanceRecord, error) {
	workspacePolicy, err := jsonbFromMap(params.WorkspacePolicy, "workspace_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	sessionPolicy, err := jsonbFromMap(params.SessionPolicy, "session_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	runtimeSelector, err := jsonbFromMap(params.RuntimeSelector, "runtime_selector")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	capacityRequirements, err := jsonbFromMap(params.CapacityRequirements, "capacity_requirements")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	fallbackPolicy, err := jsonbFromMap(params.FallbackPolicy, "fallback_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}

	instance, err := r.q.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		ProviderType:         params.ProviderType,
		AgentHomeDir:         params.AgentHomeDir,
		WorkspacePolicy:      workspacePolicy,
		SessionPolicy:        sessionPolicy,
		RuntimeSelector:      runtimeSelector,
		CapacityRequirements: capacityRequirements,
		FallbackPolicy:       fallbackPolicy,
		Status:               string(params.Status),
		Metadata:             metadata,
		RuntimeNodeID:        params.RuntimeNodeID,
		DigitalEmployeeID:    params.DigitalEmployeeID,
		TenantID:             params.TenantID,
	})
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, mapNoRows(err)
	}
	return executionInstanceRecordFromQuery(instance)
}

func (r *PgRepository) GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeExecutionInstanceRecord, error) {
	instance, err := r.q.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, queries.GetDigitalEmployeeExecutionInstanceByEmployeeIDParams{
		DigitalEmployeeID: employeeID,
		TenantID:          tenantID,
	})
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, mapNoRows(err)
	}
	return executionInstanceRecordFromQuery(instance)
}

func digitalEmployeeRecordFromQuery(employee queries.DigitalEmployee) (DigitalEmployeeRecord, error) {
	permissionPolicy, err := mapFromJSONB(employee.PermissionPolicy, "permission_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	contextPolicy, err := mapFromJSONB(employee.ContextPolicy, "context_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	approvalPolicy, err := mapFromJSONB(employee.ApprovalPolicy, "approval_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	metadata, err := mapFromJSONB(employee.Metadata, "metadata")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	return DigitalEmployeeRecord{
		ID:               employee.ID,
		TenantID:         employee.TenantID,
		TeamID:           uuidPtrFromNull(employee.TeamID),
		Name:             employee.Name,
		Role:             employee.Role,
		Description:      stringPtrFromText(employee.Description),
		Status:           DigitalEmployeeStatus(employee.Status),
		PermissionPolicy: permissionPolicy,
		ContextPolicy:    contextPolicy,
		ApprovalPolicy:   approvalPolicy,
		RiskLevel:        employee.RiskLevel,
		Metadata:         metadata,
		DisabledAt:       timePtrFromTimestamptz(employee.DisabledAt),
		ArchivedAt:       timePtrFromTimestamptz(employee.ArchivedAt),
		CreatedAt:        timeFromTimestamptz(employee.CreatedAt),
		UpdatedAt:        timeFromTimestamptz(employee.UpdatedAt),
	}, nil
}

func executionInstanceRecordFromQuery(instance queries.DigitalEmployeeExecutionInstance) (DigitalEmployeeExecutionInstanceRecord, error) {
	workspacePolicy, err := mapFromJSONB(instance.WorkspacePolicy, "workspace_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	sessionPolicy, err := mapFromJSONB(instance.SessionPolicy, "session_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	runtimeSelector, err := mapFromJSONB(instance.RuntimeSelector, "runtime_selector")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	capacityRequirements, err := mapFromJSONB(instance.CapacityRequirements, "capacity_requirements")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	fallbackPolicy, err := mapFromJSONB(instance.FallbackPolicy, "fallback_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	metadata, err := mapFromJSONB(instance.Metadata, "metadata")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	return DigitalEmployeeExecutionInstanceRecord{
		ID:                   instance.ID,
		TenantID:             instance.TenantID,
		DigitalEmployeeID:    instance.DigitalEmployeeID,
		RuntimeNodeID:        instance.RuntimeNodeID,
		ProviderType:         instance.ProviderType,
		AgentHomeDir:         instance.AgentHomeDir,
		WorkspacePolicy:      workspacePolicy,
		SessionPolicy:        sessionPolicy,
		RuntimeSelector:      runtimeSelector,
		CapacityRequirements: capacityRequirements,
		FallbackPolicy:       fallbackPolicy,
		Status:               ExecutionInstanceStatus(instance.Status),
		ReadyAt:              timePtrFromTimestamptz(instance.ReadyAt),
		DisabledAt:           timePtrFromTimestamptz(instance.DisabledAt),
		ErrorAt:              timePtrFromTimestamptz(instance.ErrorAt),
		ErrorMessage:         stringPtrFromText(instance.ErrorMessage),
		Metadata:             metadata,
		CreatedAt:            timeFromTimestamptz(instance.CreatedAt),
		UpdatedAt:            timeFromTimestamptz(instance.UpdatedAt),
	}, nil
}

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nullUUIDFromPtr(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid || value.UUID == uuid.Nil {
		return nil
	}
	copied := value.UUID
	return &copied
}

func textFromPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func textFromStatus(status DigitalEmployeeStatus) pgtype.Text {
	if status == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(status), Valid: true}
}

func stringPtrFromText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func timePtrFromTimestamptz(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func jsonbFromMap(value map[string]any, field string) ([]byte, error) {
	encoded, err := json.Marshal(cloneMap(value))
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return encoded, nil
}

func mapFromJSONB(raw []byte, field string) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	return decoded, nil
}
