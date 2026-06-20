package app

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/workflow/projectcoordination"
)

type digitalEmployeePlanningProfileReader interface {
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (employee.DigitalEmployeeRecord, error)
	GetCurrentDigitalEmployeeEffectiveConfig(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (employee.DigitalEmployeeEffectiveConfigRecord, error)
	GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (employee.DigitalEmployeeExecutionInstanceRecord, error)
}

type digitalEmployeePlanningProfileAdapter struct {
	reader digitalEmployeePlanningProfileReader
}

func (a digitalEmployeePlanningProfileAdapter) PlanningProfileRecords(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]projectcoordination.DigitalEmployeePlanningProfileSourceRecord, error) {
	if a.reader == nil {
		return nil, errors.New("digital employee planning profile reader is required")
	}
	records := make(map[uuid.UUID]projectcoordination.DigitalEmployeePlanningProfileSourceRecord, len(employeeIDs))
	for _, employeeID := range employeeIDs {
		if employeeID == uuid.Nil {
			continue
		}
		employeeRecord, err := a.reader.GetDigitalEmployee(ctx, tenantID, employeeID)
		if err != nil {
			return nil, err
		}
		record := projectcoordination.DigitalEmployeePlanningProfileSourceRecord{
			DigitalEmployeeID: employeeRecord.ID,
			EmployeeType:      employeeRecord.EmployeeType,
			Role:              employeeRecord.Role,
			EmployeeStatus:    string(employeeRecord.Status),
			PermissionPolicy:  clonePlanningProfileMap(employeeRecord.PermissionPolicy),
			ContextPolicy:     clonePlanningProfileMap(employeeRecord.ContextPolicy),
		}
		effectiveConfig, err := a.reader.GetCurrentDigitalEmployeeEffectiveConfig(ctx, tenantID, employeeID)
		if err != nil {
			if !errors.Is(err, employee.ErrNotFound) {
				return nil, err
			}
		} else {
			record.EffectiveConfigStatus = string(effectiveConfig.Status)
			record.RoleProfile = cloneNestedMap(effectiveConfig.EffectiveConfig, "role_profile")
			record.CapabilitySelection = cloneNestedMap(effectiveConfig.EffectiveConfig, "capability_selection")
		}
		instance, err := a.reader.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, tenantID, employeeID)
		if err != nil {
			if !errors.Is(err, employee.ErrNotFound) {
				return nil, err
			}
		} else {
			record.RuntimeNodeID = instance.RuntimeNodeID
			record.ProviderType = instance.ProviderType
			record.ExecutionStatus = string(instance.Status)
		}
		records[employeeID] = record
	}
	return records, nil
}

func clonePlanningProfileMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneNestedMap(values map[string]any, key string) map[string]any {
	nested, ok := values[key].(map[string]any)
	if !ok {
		return nil
	}
	return clonePlanningProfileMap(nested)
}
