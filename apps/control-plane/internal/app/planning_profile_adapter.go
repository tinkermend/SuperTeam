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
	GetDigitalEmployeeOperationalSignals(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]employee.OperationalSignals, error)
}

type digitalEmployeePlanningProfileAdapter struct {
	reader digitalEmployeePlanningProfileReader
}

func (a digitalEmployeePlanningProfileAdapter) PlanningProfileRecords(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]projectcoordination.DigitalEmployeePlanningProfileSourceRecord, error) {
	if a.reader == nil {
		return nil, errors.New("digital employee planning profile reader is required")
	}
	signals, err := a.reader.GetDigitalEmployeeOperationalSignals(ctx, tenantID, employeeIDs)
	if err != nil {
		return nil, err
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
			DigitalEmployeeID:     employeeRecord.ID,
			EmployeeType:          employeeRecord.EmployeeType,
			Role:                  employeeRecord.Role,
			EmployeeStatus:        string(employeeRecord.Status),
			PermissionPolicy:      clonePlanningProfileMap(employeeRecord.PermissionPolicy),
			ContextPolicy:         clonePlanningProfileMap(employeeRecord.ContextPolicy),
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
		if signal, ok := signals[employeeID]; ok {
			record.LoadState = planningLoadStateMap(signal)
			record.ReliabilitySignals = planningReliabilitySignalsMap(signal)
		}
		records[employeeID] = record
	}
	return records, nil
}

// planningLoadStateMap translates raw operational counts into the map shape expected by
// DigitalEmployeePlanningProfileSourceRecord.LoadState. An employee with no in-flight
// attempts is treated as lendable with one available slot; once actively working they
// are neither lendable nor available until their current attempts finish.
func planningLoadStateMap(signal employee.OperationalSignals) map[string]any {
	lendable := signal.InFlightAttemptCount == 0
	availableSlots := int32(0)
	if lendable {
		availableSlots = 1
	}
	return map[string]any{
		"in_flight_tasks":  signal.InFlightAttemptCount,
		"available_slots":  availableSlots,
		"lendable":         lendable,
	}
}

// planningReliabilitySignalsMap translates raw operational counts into the map shape
// expected by DigitalEmployeePlanningProfileSourceRecord.ReliabilitySignals. The status
// is derived so the scoring hard-fail path can flag employees whose recent track record
// is dominated by failures.
func planningReliabilitySignalsMap(signal employee.OperationalSignals) map[string]any {
	total := signal.RecentSuccessCount + signal.RecentFailureCount
	var successRate float64
	if total > 0 {
		successRate = float64(signal.RecentSuccessCount) / float64(total)
	}
	status := "healthy"
	if total > 0 && successRate < 0.5 {
		status = "unhealthy"
	}
	return map[string]any{
		"status":                    status,
		"success_rate":              successRate,
		"recent_success_count":      signal.RecentSuccessCount,
		"recent_failure_count":      signal.RecentFailureCount,
		"recent_human_reject_count": signal.RecentHumanRejectCount,
	}
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
