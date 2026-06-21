package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/employee"
)

func TestDigitalEmployeePlanningProfileAdapterMapsEmployeeFacts(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	reader := fakePlanningProfileEmployeeReader{
		employees: map[uuid.UUID]employee.DigitalEmployeeRecord{
			employeeID: {
				ID:               employeeID,
				TenantID:         tenantID,
				EmployeeType:     "database_admin",
				Role:             "数据库分析",
				Status:           employee.DigitalEmployeeStatusActive,
				PermissionPolicy: map[string]any{"grants": []any{"database.read:dev_database"}},
				ContextPolicy:    map[string]any{"max_context_classification": "internal"},
			},
		},
		configs: map[uuid.UUID]employee.DigitalEmployeeEffectiveConfigRecord{
			employeeID: {
				DigitalEmployeeID: employeeID,
				Status:            employee.EffectiveConfigStatusApproved,
				EffectiveConfig: map[string]any{
					"role_profile": map[string]any{"primary_role": "data_analyst"},
					"capability_selection": map[string]any{
						"enabled_skills":         []any{"sql.analysis"},
						"enabled_mcp_servers":    []any{"postgres.readonly"},
						"enabled_provider_types": []any{"codex"},
					},
				},
			},
		},
		instances: map[uuid.UUID]employee.DigitalEmployeeExecutionInstanceRecord{
			employeeID: {
				DigitalEmployeeID: employeeID,
				RuntimeNodeID:     runtimeNodeID,
				ProviderType:      "codex",
				Status:            employee.ExecutionInstanceStatusReady,
			},
		},
		signals: map[uuid.UUID]employee.OperationalSignals{
			employeeID: {
				InFlightAttemptCount:   2,
				RecentSuccessCount:     7,
				RecentFailureCount:     1,
				RecentHumanRejectCount: 0,
			},
		},
	}

	records, err := digitalEmployeePlanningProfileAdapter{reader: reader}.PlanningProfileRecords(context.Background(), tenantID, []uuid.UUID{employeeID})

	require.NoError(t, err)
	record := records[employeeID]
	require.Equal(t, "database_admin", record.EmployeeType)
	require.Equal(t, "active", record.EmployeeStatus)
	require.Equal(t, "approved", record.EffectiveConfigStatus)
	require.Equal(t, "ready", record.ExecutionStatus)
	require.Equal(t, runtimeNodeID, record.RuntimeNodeID)
	require.Equal(t, "codex", record.ProviderType)
	require.Equal(t, map[string]any{"primary_role": "data_analyst"}, record.RoleProfile)
	require.Equal(t, map[string]any{
		"in_flight_tasks": int32(2),
		"available_slots": int32(0),
		"lendable":        false,
	}, record.LoadState)
	require.Equal(t, map[string]any{
		"status":                    "healthy",
		"success_rate":              7.0 / 8.0,
		"recent_success_count":      int32(7),
		"recent_failure_count":      int32(1),
		"recent_human_reject_count": int32(0),
	}, record.ReliabilitySignals)
}

type fakePlanningProfileEmployeeReader struct {
	employees map[uuid.UUID]employee.DigitalEmployeeRecord
	configs   map[uuid.UUID]employee.DigitalEmployeeEffectiveConfigRecord
	instances map[uuid.UUID]employee.DigitalEmployeeExecutionInstanceRecord
	signals   map[uuid.UUID]employee.OperationalSignals
}

func (r fakePlanningProfileEmployeeReader) GetDigitalEmployee(_ context.Context, tenantID, employeeID uuid.UUID) (employee.DigitalEmployeeRecord, error) {
	record, ok := r.employees[employeeID]
	if !ok || record.TenantID != tenantID {
		return employee.DigitalEmployeeRecord{}, employee.ErrNotFound
	}
	return record, nil
}

func (r fakePlanningProfileEmployeeReader) GetCurrentDigitalEmployeeEffectiveConfig(_ context.Context, tenantID, employeeID uuid.UUID) (employee.DigitalEmployeeEffectiveConfigRecord, error) {
	record, ok := r.configs[employeeID]
	if !ok || tenantID == uuid.Nil {
		return employee.DigitalEmployeeEffectiveConfigRecord{}, employee.ErrNotFound
	}
	return record, nil
}

func (r fakePlanningProfileEmployeeReader) GetDigitalEmployeeExecutionInstanceByEmployeeID(_ context.Context, tenantID, employeeID uuid.UUID) (employee.DigitalEmployeeExecutionInstanceRecord, error) {
	record, ok := r.instances[employeeID]
	if !ok || tenantID == uuid.Nil {
		return employee.DigitalEmployeeExecutionInstanceRecord{}, employee.ErrNotFound
	}
	return record, nil
}

func (r fakePlanningProfileEmployeeReader) GetDigitalEmployeeOperationalSignals(_ context.Context, _ uuid.UUID, _ []uuid.UUID) (map[uuid.UUID]employee.OperationalSignals, error) {
	return r.signals, nil
}
