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
}

type fakePlanningProfileEmployeeReader struct {
	employees map[uuid.UUID]employee.DigitalEmployeeRecord
	configs   map[uuid.UUID]employee.DigitalEmployeeEffectiveConfigRecord
	instances map[uuid.UUID]employee.DigitalEmployeeExecutionInstanceRecord
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
