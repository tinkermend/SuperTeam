package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/capability"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/project"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
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
		configs: map[uuid.UUID]employee.EmployeeConfigInput{
			employeeID: {
				TenantID:              tenantID,
				DigitalEmployeeID:     employeeID,
				PersonaMemoryMarkdown: "# 人格画像\n证据优先",
				CapabilityBindings: map[string]any{
					"external_capabilities": []any{"text_generation", "codebase.analysis"},
					"skills":                []any{"backend-implementation"},
					"mcp_servers":           []any{"postgres-readonly"},
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

	records, err := digitalEmployeePlanningProfileAdapter{reader: reader}.PlanningProfileRecords(context.Background(), tenantID, uuid.Nil, []uuid.UUID{employeeID})

	require.NoError(t, err)
	record := records[employeeID]
	require.Equal(t, "database_admin", record.EmployeeType)
	require.Equal(t, "active", record.EmployeeStatus)
	require.Equal(t, "ready", record.ExecutionStatus)
	require.Equal(t, runtimeNodeID, record.RuntimeNodeID)
	require.Equal(t, "codex", record.ProviderType)
	require.Equal(t, "# 人格画像\n证据优先", record.PersonaMemoryMarkdown)
	require.Equal(t, map[string]any{
		"external_capabilities": []any{"text_generation", "codebase.analysis"},
		"skills":                []any{"backend-implementation"},
		"mcp_servers":           []any{"postgres-readonly"},
	}, record.CapabilityBindings)
	require.Equal(t, map[string]any{"grants": []any{"database.read:dev_database"}}, record.PermissionPolicy)
	require.Equal(t, map[string]any{"max_context_classification": "internal"}, record.ContextPolicy)
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

func TestDigitalEmployeePlanningProfileAdapterUsesProjectTaskPreflightWithoutExecutionInstance(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	reader := fakePlanningProfileEmployeeReader{
		employees: map[uuid.UUID]employee.DigitalEmployeeRecord{
			employeeID: {
				ID:           employeeID,
				TenantID:     tenantID,
				EmployeeType: "implementation",
				Role:         "项目执行",
				Status:       employee.DigitalEmployeeStatusReady,
				ProviderType: "codex",
			},
		},
	}
	preflightReader := fakeProjectTaskRunPreflightReader{
		preflight: employee.StartProjectTaskRunPreflight{
			TenantID:              tenantID,
			DigitalEmployeeID:     employeeID,
			DigitalEmployeeStatus: employee.DigitalEmployeeStatusReady,
			RuntimeNodeID:         runtimeNodeID,
			NodeID:                "provider-runtime-smoke-node",
			ProviderType:          "codex",
			WorkspaceBaseDir:      "/var/superteam/projects",
			RuntimeSessionActive:  true,
			ProviderHealthy:       true,
		},
	}

	records, err := digitalEmployeePlanningProfileAdapter{reader: reader, projectTaskRuns: preflightReader}.PlanningProfileRecords(context.Background(), tenantID, projectID, []uuid.UUID{employeeID})

	require.NoError(t, err)
	record := records[employeeID]
	require.Equal(t, "codex", record.ProviderType)
	require.Equal(t, runtimeNodeID, record.RuntimeNodeID)
	require.Equal(t, "ready", record.ExecutionStatus)
}

func TestPreDispatchGateAdapterMapsEmployeeRuntimeFacts(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	nodeID := "runtime-node-gate-1"
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	reader := fakePlanningProfileEmployeeReader{
		employees: map[uuid.UUID]employee.DigitalEmployeeRecord{
			employeeID: {
				ID:       employeeID,
				TenantID: tenantID,
				Name:     "数据库员工",
				Status:   employee.DigitalEmployeeStatusActive,
			},
		},
		instances: map[uuid.UUID]employee.DigitalEmployeeExecutionInstanceRecord{
			employeeID: {
				DigitalEmployeeID: employeeID,
				RuntimeNodeID:     runtimeNodeID,
				ProviderType:      "codex",
				AgentHomeDir:      "/var/superteam/agents/db",
				RuntimeSelector:   map[string]any{"runtime_node_id": runtimeNodeID.String(), "node_id": nodeID},
				Status:            employee.ExecutionInstanceStatusReady,
			},
		},
	}
	runtimeReader := &fakeGateRuntimeNodeReader{
		nodes: map[string]runtimepkg.NodeRecord{
			nodeID: {
				ID:              runtimeNodeID,
				TenantID:        tenantID,
				NodeID:          nodeID,
				MaxSlots:        4,
				CurrentLoad:     2,
				Status:          "online",
				LastHeartbeatAt: timestamptz(now.Add(-30 * time.Second)),
			},
		},
	}
	adapter := preDispatchGateAdapter{
		employees:    reader,
		runtimeNodes: runtimeReader,
		now:          func() time.Time { return now },
	}

	employeeSnapshot, runtimeSnapshot, err := adapter.GetEmployeeRuntimeSnapshot(context.Background(), tenantID, projectID, employeeID)

	require.NoError(t, err)
	require.Equal(t, employeeID, employeeSnapshot.ID)
	require.Equal(t, "active", employeeSnapshot.Status)
	require.True(t, employeeSnapshot.PolicyAllowed)
	require.Equal(t, int32(1), employeeSnapshot.RequiredLoadSlots)
	require.Equal(t, int32(2), employeeSnapshot.AvailableLoadSlots)
	require.True(t, runtimeSnapshot.NodeOnline)
	require.True(t, runtimeSnapshot.ProviderAvailable)
	require.True(t, runtimeSnapshot.WorkspaceReady)
	require.True(t, runtimeSnapshot.SlotAvailable)
	require.True(t, runtimeSnapshot.ContractVersionAccepted)
}

func TestPreDispatchGateAdapterUsesProjectTaskPlacementForRuntimeLessReadyEmployee(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	nodeID := "runtime-project-placement-1"
	now := time.Date(2026, 7, 3, 11, 0, 0, 0, time.UTC)
	reader := fakePlanningProfileEmployeeReader{
		employees: map[uuid.UUID]employee.DigitalEmployeeRecord{
			employeeID: {
				ID:           employeeID,
				TenantID:     tenantID,
				Name:         "项目执行员工",
				Status:       employee.DigitalEmployeeStatusReady,
				ProviderType: "codex",
			},
		},
	}
	runtimeReader := &fakeGateRuntimeNodeReader{
		nodesByID: map[uuid.UUID]runtimepkg.NodeRecord{
			runtimeNodeID: {
				ID:                 runtimeNodeID,
				TenantID:           tenantID,
				NodeID:             nodeID,
				SupportedProviders: []byte(`["codex"]`),
				MaxSlots:           4,
				CurrentLoad:        1,
				Status:             "online",
				LastHeartbeatAt:    timestamptz(now.Add(-20 * time.Second)),
			},
		},
	}
	projectTaskPreflight := fakeProjectTaskRunPreflightReader{
		preflight: employee.StartProjectTaskRunPreflight{
			TenantID:              tenantID,
			DigitalEmployeeID:     employeeID,
			DigitalEmployeeStatus: employee.DigitalEmployeeStatusReady,
			RuntimeNodeID:         runtimeNodeID,
			NodeID:                nodeID,
			ProviderType:          "codex",
			WorkspaceBaseDir:      "/var/superteam/projects",
			RuntimeSessionActive:  true,
			ProviderHealthy:       true,
		},
	}
	adapter := preDispatchGateAdapter{
		employees:       reader,
		projectTaskRuns: projectTaskPreflight,
		runtimeNodes:    runtimeReader,
		now:             func() time.Time { return now },
	}

	employeeSnapshot, runtimeSnapshot, err := adapter.GetEmployeeRuntimeSnapshot(context.Background(), tenantID, projectID, employeeID)

	require.NoError(t, err)
	require.Equal(t, employeeID, employeeSnapshot.ID)
	require.Equal(t, "ready", employeeSnapshot.Status)
	require.True(t, employeeSnapshot.PolicyAllowed)
	require.Equal(t, int32(3), employeeSnapshot.AvailableLoadSlots)
	require.True(t, runtimeSnapshot.NodeOnline)
	require.True(t, runtimeSnapshot.ProviderAvailable)
	require.True(t, runtimeSnapshot.WorkspaceReady)
	require.True(t, runtimeSnapshot.SlotAvailable)
	require.Equal(t, 1, runtimeReader.getNodeByIDCalls)
	require.Zero(t, runtimeReader.getNodeCalls)
}

func TestPreDispatchGateAdapterMapsRuntimeFactsWithoutRuntimeSelector(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	reader := fakePlanningProfileEmployeeReader{
		employees: map[uuid.UUID]employee.DigitalEmployeeRecord{
			employeeID: {
				ID:       employeeID,
				TenantID: tenantID,
				Status:   employee.DigitalEmployeeStatusActive,
			},
		},
		instances: map[uuid.UUID]employee.DigitalEmployeeExecutionInstanceRecord{
			employeeID: {
				DigitalEmployeeID: employeeID,
				RuntimeNodeID:     runtimeNodeID,
				ProviderType:      "codex",
				AgentHomeDir:      "/var/superteam/agents/db",
				Status:            employee.ExecutionInstanceStatusReady,
			},
		},
	}
	runtimeReader := &fakeGateRuntimeNodeReader{
		nodesByID: map[uuid.UUID]runtimepkg.NodeRecord{
			runtimeNodeID: {
				ID:                 runtimeNodeID,
				TenantID:           tenantID,
				NodeID:             "runtime-node-empty-selector",
				SupportedProviders: []byte(`["codex"]`),
				MaxSlots:           3,
				CurrentLoad:        1,
				Status:             "online",
				LastHeartbeatAt:    timestamptz(now.Add(-30 * time.Second)),
			},
		},
	}
	adapter := preDispatchGateAdapter{
		employees:    reader,
		runtimeNodes: runtimeReader,
		now:          func() time.Time { return now },
	}

	employeeSnapshot, runtimeSnapshot, err := adapter.GetEmployeeRuntimeSnapshot(context.Background(), tenantID, projectID, employeeID)

	require.NoError(t, err)
	require.Equal(t, int32(2), employeeSnapshot.AvailableLoadSlots)
	require.True(t, runtimeSnapshot.NodeOnline)
	require.True(t, runtimeSnapshot.ProviderAvailable)
	require.True(t, runtimeSnapshot.SlotAvailable)
	require.Equal(t, 1, runtimeReader.getNodeByIDCalls)
	require.Zero(t, runtimeReader.getNodeCalls)
}

func TestPreDispatchGateAdapterReportsMissingMCPBinding(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	adapter := preDispatchGateAdapter{
		capabilities: fakeGateCapabilityReader{
			servers: []capability.MCPServer{
				{Name: "postgres.reporting", Status: "active"},
			},
		},
	}
	task := project.ProjectTask{
		InputRequirements: map[string]any{
			"required_capabilities": []any{"database.read"},
			"tool_requirements":     []any{"mcp:postgres.readonly", "mcp:postgres.reporting", "external:deploy", "malformed"},
		},
	}

	capabilitySnapshot, toolSnapshot, err := adapter.GetEmployeeCapabilitySnapshot(context.Background(), tenantID, employeeID, task)

	require.NoError(t, err)
	require.Empty(t, capabilitySnapshot.Required)
	require.Empty(t, capabilitySnapshot.Matched)
	require.Equal(t, []string{"mcp:postgres.readonly"}, toolSnapshot.MissingBindings)
	require.Equal(t, []string{"external:deploy", "malformed"}, toolSnapshot.RetryableUnavailable)
}

func TestPreDispatchGateAdapterMarksRequiredToolsRetryableOnCapabilityError(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	adapter := preDispatchGateAdapter{
		capabilities: fakeGateCapabilityReader{err: errors.New("capability store unavailable")},
	}
	task := project.ProjectTask{
		InputRequirements: map[string]any{
			"tool_requirements": []any{"mcp:postgres.readonly", "external:deploy"},
		},
	}

	capabilitySnapshot, toolSnapshot, err := adapter.GetEmployeeCapabilitySnapshot(context.Background(), tenantID, employeeID, task)

	require.NoError(t, err)
	require.Empty(t, capabilitySnapshot.Required)
	require.Equal(t, []string{"mcp:postgres.readonly", "external:deploy"}, toolSnapshot.RetryableUnavailable)
	require.Empty(t, toolSnapshot.MissingBindings)
}

func TestPreDispatchGateAdapterTreatsStaleRuntimeHeartbeatAsOffline(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	reader := fakePlanningProfileEmployeeReader{
		employees: map[uuid.UUID]employee.DigitalEmployeeRecord{
			employeeID: {
				ID:       employeeID,
				TenantID: tenantID,
				Status:   employee.DigitalEmployeeStatusActive,
			},
		},
		instances: map[uuid.UUID]employee.DigitalEmployeeExecutionInstanceRecord{
			employeeID: {
				DigitalEmployeeID: employeeID,
				RuntimeNodeID:     runtimeNodeID,
				ProviderType:      "codex",
				AgentHomeDir:      "/var/superteam/agents/db",
				Status:            employee.ExecutionInstanceStatusActive,
			},
		},
	}
	adapter := preDispatchGateAdapter{
		employees: reader,
		runtimeNodes: &fakeGateRuntimeNodeReader{
			nodesByID: map[uuid.UUID]runtimepkg.NodeRecord{
				runtimeNodeID: {
					ID:              runtimeNodeID,
					TenantID:        tenantID,
					NodeID:          "runtime-node-stale",
					MaxSlots:        2,
					CurrentLoad:     0,
					Status:          "online",
					LastHeartbeatAt: timestamptz(now.Add(-runtimeNodeHeartbeatTTL - time.Second)),
				},
			},
		},
		now: func() time.Time { return now },
	}

	employeeSnapshot, runtimeSnapshot, err := adapter.GetEmployeeRuntimeSnapshot(context.Background(), tenantID, projectID, employeeID)

	require.NoError(t, err)
	require.Equal(t, employeeID, employeeSnapshot.ID)
	require.False(t, runtimeSnapshot.NodeOnline)
	require.True(t, runtimeSnapshot.SlotAvailable)
	require.True(t, runtimeSnapshot.ContractVersionAccepted)
}

func TestPreDispatchGateAdapterDoesNotDeriveCapabilitiesFromPlannerOutput(t *testing.T) {
	adapter := preDispatchGateAdapter{}
	task := project.ProjectTask{
		// The planner is free to write anything into this map; none of it may
		// reach the gate.
		InputRequirements: map[string]any{
			"required_capabilities": []any{"database.write"},
			"missing_capabilities":  []any{"codebase.analysis"},
			"matched_capabilities":  []any{"bash_execution"},
		},
	}

	capabilitySnapshot, _, err := adapter.GetEmployeeCapabilitySnapshot(
		context.Background(), uuid.New(), uuid.New(), task,
	)

	require.NoError(t, err)
	require.Empty(t, capabilitySnapshot.Required)
	require.Empty(t, capabilitySnapshot.Matched)
}

type fakePlanningProfileEmployeeReader struct {
	employees map[uuid.UUID]employee.DigitalEmployeeRecord
	configs   map[uuid.UUID]employee.EmployeeConfigInput
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

func (r fakePlanningProfileEmployeeReader) GetLatestDigitalEmployeeConfigRevision(_ context.Context, tenantID, employeeID uuid.UUID) (employee.EmployeeConfigInput, error) {
	record, ok := r.configs[employeeID]
	if !ok || record.TenantID != tenantID {
		return employee.EmployeeConfigInput{}, employee.ErrNotFound
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

type fakeGateRuntimeNodeReader struct {
	nodes            map[string]runtimepkg.NodeRecord
	nodesByID        map[uuid.UUID]runtimepkg.NodeRecord
	getNodeCalls     int
	getNodeByIDCalls int
	err              error
}

func (r *fakeGateRuntimeNodeReader) GetNode(_ context.Context, nodeID string) (runtimepkg.NodeRecord, error) {
	r.getNodeCalls++
	if r.err != nil {
		return runtimepkg.NodeRecord{}, r.err
	}
	node, ok := r.nodes[nodeID]
	if !ok {
		return runtimepkg.NodeRecord{}, pgx.ErrNoRows
	}
	return node, nil
}

func (r *fakeGateRuntimeNodeReader) GetNodeByID(_ context.Context, id uuid.UUID) (runtimepkg.NodeRecord, error) {
	r.getNodeByIDCalls++
	if r.err != nil {
		return runtimepkg.NodeRecord{}, r.err
	}
	node, ok := r.nodesByID[id]
	if !ok {
		return runtimepkg.NodeRecord{}, pgx.ErrNoRows
	}
	return node, nil
}

type fakeGateCapabilityReader struct {
	servers []capability.MCPServer
	err     error
}

func (r fakeGateCapabilityReader) ListEffectiveMCPServers(_ context.Context, req capability.EmployeeScopedRequest) ([]capability.MCPServer, error) {
	if r.err != nil {
		return nil, r.err
	}
	if req.TenantID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return nil, errors.New("invalid request")
	}
	return append([]capability.MCPServer(nil), r.servers...), nil
}

type fakeProjectTaskRunPreflightReader struct {
	preflight employee.StartProjectTaskRunPreflight
	err       error
}

func (r fakeProjectTaskRunPreflightReader) GetProjectTaskRunPreflight(_ context.Context, tenantID, projectID, employeeID uuid.UUID) (employee.StartProjectTaskRunPreflight, error) {
	if r.err != nil {
		return employee.StartProjectTaskRunPreflight{}, r.err
	}
	if tenantID == uuid.Nil || projectID == uuid.Nil || employeeID == uuid.Nil {
		return employee.StartProjectTaskRunPreflight{}, employee.ErrNotFound
	}
	return r.preflight, nil
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
