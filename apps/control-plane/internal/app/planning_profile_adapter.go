package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/superteam/control-plane/internal/capability"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/project"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/workflow/projectcoordination"
)

const runtimeNodeHeartbeatTTL = 2 * time.Minute

type digitalEmployeePlanningProfileReader interface {
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (employee.DigitalEmployeeRecord, error)
	GetCurrentDigitalEmployeeEffectiveConfig(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (employee.DigitalEmployeeEffectiveConfigRecord, error)
	GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (employee.DigitalEmployeeExecutionInstanceRecord, error)
	GetDigitalEmployeeOperationalSignals(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]employee.OperationalSignals, error)
}

type gateRuntimeNodeReader interface {
	GetNode(ctx context.Context, nodeID string) (runtimepkg.NodeRecord, error)
}

type gateRuntimeNodeIDReader interface {
	GetNodeByID(ctx context.Context, id uuid.UUID) (runtimepkg.NodeRecord, error)
}

type gateProjectTaskRunPreflightReader interface {
	GetProjectTaskRunPreflight(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (employee.StartProjectTaskRunPreflight, error)
}

type gateCapabilityReader interface {
	ListEffectiveMCPServers(ctx context.Context, req capability.EmployeeScopedRequest) ([]capability.MCPServer, error)
}

type digitalEmployeePlanningProfileAdapter struct {
	reader          digitalEmployeePlanningProfileReader
	projectTaskRuns gateProjectTaskRunPreflightReader
}

type preDispatchGateAdapter struct {
	employees       digitalEmployeePlanningProfileReader
	projectTaskRuns gateProjectTaskRunPreflightReader
	runtimeNodes    gateRuntimeNodeReader
	capabilities    gateCapabilityReader
	now             func() time.Time
}

type projectPlanningProfileAdapter struct {
	source digitalEmployeePlanningProfileAdapter
}

type projectRuntimeNodeReader struct {
	runtimeNodes        gateRuntimePlacementNodeReader
	runtimeCapabilities interface {
		ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]runtimepkg.RuntimeCapability, error)
	}
	connections *runtimepkg.ConnectionRegistry
}

type gateRuntimePlacementNodeReader interface {
	ListRuntimeNodesForTenant(ctx context.Context, params runtimepkg.ListRuntimeNodesForTenantParams) ([]runtimepkg.NodeRecord, error)
}

func (a projectPlanningProfileAdapter) PlanningProfileRecords(ctx context.Context, tenantID, projectID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]project.DigitalEmployeePlanningProfileSourceRecord, error) {
	records, err := a.source.PlanningProfileRecords(ctx, tenantID, projectID, employeeIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]project.DigitalEmployeePlanningProfileSourceRecord, len(records))
	for employeeID, record := range records {
		out[employeeID] = project.DigitalEmployeePlanningProfileSourceRecord{
			DigitalEmployeeID:     record.DigitalEmployeeID,
			ProviderType:          record.ProviderType,
			EffectiveConfigStatus: record.EffectiveConfigStatus,
			ExecutionStatus:       record.ExecutionStatus,
		}
	}
	return out, nil
}

func (r projectRuntimeNodeReader) ListRuntimeNodesForTenant(ctx context.Context, params runtimepkg.ListRuntimeNodesForTenantParams) ([]runtimepkg.NodeRecord, error) {
	return r.runtimeNodes.ListRuntimeNodesForTenant(ctx, params)
}

func (r projectRuntimeNodeReader) ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]runtimepkg.RuntimeCapability, error) {
	return r.runtimeCapabilities.ListRuntimeCapabilitiesForNode(ctx, tenantID, nodeID)
}

func (r projectRuntimeNodeReader) IsConnected(nodeID string) bool {
	return r.connections != nil && r.connections.IsConnected(nodeID)
}

func (a digitalEmployeePlanningProfileAdapter) PlanningProfileRecords(ctx context.Context, tenantID, projectID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]projectcoordination.DigitalEmployeePlanningProfileSourceRecord, error) {
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
			DigitalEmployeeID: employeeRecord.ID,
			EmployeeType:      employeeRecord.EmployeeType,
			Role:              employeeRecord.Role,
			EmployeeStatus:    string(employeeRecord.Status),
			ProviderType:      employeeRecord.ProviderType,
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
		preflightApplied := false
		if a.projectTaskRuns != nil && projectID != uuid.Nil {
			preflight, err := a.projectTaskRuns.GetProjectTaskRunPreflight(ctx, tenantID, projectID, employeeID)
			if err != nil {
				if !normalGateAbsence(err) {
					return nil, err
				}
			} else {
				applyProjectTaskPreflightPlanningFacts(&record, preflight)
				preflightApplied = true
			}
		}
		if !preflightApplied {
			instance, err := a.reader.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, tenantID, employeeID)
			if err != nil {
				if !errors.Is(err, employee.ErrNotFound) {
					return nil, err
				}
			} else {
				record.RuntimeNodeID = instance.RuntimeNodeID
				if strings.TrimSpace(instance.ProviderType) != "" {
					record.ProviderType = instance.ProviderType
				}
				record.ExecutionStatus = string(instance.Status)
			}
		}
		if signal, ok := signals[employeeID]; ok {
			record.LoadState = planningLoadStateMap(signal)
			record.ReliabilitySignals = planningReliabilitySignalsMap(signal)
		}
		records[employeeID] = record
	}
	return records, nil
}

func applyProjectTaskPreflightPlanningFacts(record *projectcoordination.DigitalEmployeePlanningProfileSourceRecord, preflight employee.StartProjectTaskRunPreflight) {
	if record == nil {
		return
	}
	if preflight.RuntimeNodeID != uuid.Nil {
		record.RuntimeNodeID = preflight.RuntimeNodeID
	}
	if providerType := strings.TrimSpace(preflight.ProviderType); providerType != "" {
		record.ProviderType = providerType
	}
	if preflight.HasApprovedEffectiveConfig && strings.TrimSpace(record.EffectiveConfigStatus) == "" {
		record.EffectiveConfigStatus = string(employee.EffectiveConfigStatusApproved)
	}
	record.ExecutionStatus = projectTaskPreflightExecutionStatus(preflight)
}

func projectTaskPreflightExecutionStatus(preflight employee.StartProjectTaskRunPreflight) string {
	if !preflight.HasApprovedEffectiveConfig {
		return "pending_configuration"
	}
	if !preflight.RuntimeSessionActive || !preflight.ProviderHealthy || strings.TrimSpace(preflight.WorkspaceBaseDir) == "" {
		return "unavailable"
	}
	return "ready"
}

func (a preDispatchGateAdapter) GetEmployeeRuntimeSnapshot(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (project.PreDispatchEmployeeSnapshot, project.PreDispatchRuntimeSnapshot, error) {
	employeeSnapshot := project.PreDispatchEmployeeSnapshot{
		ID:                employeeID,
		RequiredLoadSlots: 1,
	}
	runtimeSnapshot := unavailableRuntimeSnapshot(a.clock().Add(runtimeNodeHeartbeatTTL))
	if a.employees == nil {
		employeeSnapshot.Status = "missing"
		return employeeSnapshot, runtimeSnapshot, nil
	}

	employeeRecord, err := a.employees.GetDigitalEmployee(ctx, tenantID, employeeID)
	if err != nil {
		if normalGateAbsence(err) {
			employeeSnapshot.Status = "missing"
			return employeeSnapshot, runtimeSnapshot, nil
		}
		return project.PreDispatchEmployeeSnapshot{}, project.PreDispatchRuntimeSnapshot{}, err
	}
	employeeSnapshot.ID = employeeRecord.ID
	employeeSnapshot.Status = string(employeeRecord.Status)
	employeeSnapshot.PolicyAllowed = employeeRecord.Status == employee.DigitalEmployeeStatusReady || employeeRecord.Status == employee.DigitalEmployeeStatusActive

	if a.projectTaskRuns != nil && projectID != uuid.Nil {
		preflight, err := a.projectTaskRuns.GetProjectTaskRunPreflight(ctx, tenantID, projectID, employeeID)
		if err != nil {
			if !normalGateAbsence(err) {
				return project.PreDispatchEmployeeSnapshot{}, project.PreDispatchRuntimeSnapshot{}, err
			}
		} else {
			return a.runtimeSnapshotFromProjectTaskPreflight(ctx, tenantID, employeeSnapshot, preflight)
		}
	}

	instance, err := a.employees.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, tenantID, employeeID)
	if err != nil {
		if normalGateAbsence(err) {
			return employeeSnapshot, runtimeSnapshot, nil
		}
		return project.PreDispatchEmployeeSnapshot{}, project.PreDispatchRuntimeSnapshot{}, err
	}
	runtimeSnapshot.ProviderAvailable = instance.ProviderType != "" && executionInstanceRunnable(instance.Status)
	runtimeSnapshot.WorkspaceReady = strings.TrimSpace(instance.AgentHomeDir) != ""
	if instance.RuntimeNodeID == uuid.Nil || a.runtimeNodes == nil {
		return employeeSnapshot, runtimeSnapshot, nil
	}

	node, err := a.runtimeNodeForInstance(ctx, instance)
	if err != nil {
		if normalGateAbsence(err) {
			return employeeSnapshot, runtimeSnapshot, nil
		}
		return project.PreDispatchEmployeeSnapshot{}, project.PreDispatchRuntimeSnapshot{}, err
	}
	if node.TenantID != uuid.Nil && node.TenantID != tenantID {
		return employeeSnapshot, runtimeSnapshot, nil
	}
	availableSlots := node.MaxSlots - node.CurrentLoad
	if availableSlots < 0 {
		availableSlots = 0
	}
	employeeSnapshot.AvailableLoadSlots = availableSlots
	runtimeSnapshot.NodeOnline = runtimeNodeOnline(node, a.clock())
	runtimeSnapshot.SlotAvailable = availableSlots >= employeeSnapshot.RequiredLoadSlots
	if !runtimeProviderSupported(node, instance.ProviderType) {
		runtimeSnapshot.ProviderAvailable = false
	}
	return employeeSnapshot, runtimeSnapshot, nil
}

func (a preDispatchGateAdapter) runtimeSnapshotFromProjectTaskPreflight(ctx context.Context, tenantID uuid.UUID, employeeSnapshot project.PreDispatchEmployeeSnapshot, preflight employee.StartProjectTaskRunPreflight) (project.PreDispatchEmployeeSnapshot, project.PreDispatchRuntimeSnapshot, error) {
	runtimeSnapshot := unavailableRuntimeSnapshot(a.clock().Add(runtimeNodeHeartbeatTTL))
	runtimeSnapshot.NodeOnline = preflight.RuntimeSessionActive
	runtimeSnapshot.ProviderAvailable = strings.TrimSpace(preflight.ProviderType) != "" && preflight.ProviderHealthy
	runtimeSnapshot.WorkspaceReady = strings.TrimSpace(preflight.WorkspaceBaseDir) != ""
	if preflight.RuntimeNodeID == uuid.Nil || a.runtimeNodes == nil {
		return employeeSnapshot, runtimeSnapshot, nil
	}

	node, err := a.runtimeNodeForProjectTaskPreflight(ctx, preflight)
	if err != nil {
		if normalGateAbsence(err) {
			return employeeSnapshot, runtimeSnapshot, nil
		}
		return project.PreDispatchEmployeeSnapshot{}, project.PreDispatchRuntimeSnapshot{}, err
	}
	if node.TenantID != uuid.Nil && node.TenantID != tenantID {
		return employeeSnapshot, runtimeSnapshot, nil
	}
	availableSlots := node.MaxSlots - node.CurrentLoad
	if availableSlots < 0 {
		availableSlots = 0
	}
	employeeSnapshot.AvailableLoadSlots = availableSlots
	runtimeSnapshot.NodeOnline = runtimeSnapshot.NodeOnline && runtimeNodeOnline(node, a.clock())
	runtimeSnapshot.SlotAvailable = availableSlots >= employeeSnapshot.RequiredLoadSlots
	if !runtimeProviderSupported(node, preflight.ProviderType) {
		runtimeSnapshot.ProviderAvailable = false
	}
	return employeeSnapshot, runtimeSnapshot, nil
}

func (a preDispatchGateAdapter) GetEmployeeCapabilitySnapshot(ctx context.Context, tenantID, employeeID uuid.UUID, task project.ProjectTask) (project.PreDispatchCapabilitySnapshot, project.PreDispatchToolSnapshot, error) {
	requiredCapabilities := gateStringList(task.InputRequirements["required_capabilities"])
	hardMissing := gateStringList(task.InputRequirements["missing_capabilities"])
	matched := gateStringList(task.InputRequirements["matched_capabilities"])
	capabilitySnapshot := project.PreDispatchCapabilitySnapshot{
		Required:    requiredCapabilities,
		Matched:     matched,
		HardMissing: hardMissing,
	}

	requiredTools := gateStringList(task.InputRequirements["tool_requirements"])
	toolSnapshot := project.PreDispatchToolSnapshot{}
	if len(requiredTools) == 0 {
		return capabilitySnapshot, toolSnapshot, nil
	}
	if a.capabilities == nil {
		toolSnapshot.RetryableUnavailable = append([]string(nil), requiredTools...)
		return capabilitySnapshot, toolSnapshot, nil
	}
	servers, err := a.capabilities.ListEffectiveMCPServers(ctx, capability.EmployeeScopedRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		toolSnapshot.RetryableUnavailable = append([]string(nil), requiredTools...)
		return capabilitySnapshot, toolSnapshot, nil
	}
	availableMCP := effectiveMCPServerNames(servers)
	for _, requirement := range requiredTools {
		toolType, toolKey, ok := strings.Cut(requirement, ":")
		if !ok || strings.TrimSpace(toolType) == "" || strings.TrimSpace(toolKey) == "" {
			toolSnapshot.RetryableUnavailable = append(toolSnapshot.RetryableUnavailable, requirement)
			continue
		}
		if strings.TrimSpace(toolType) != "mcp" {
			toolSnapshot.RetryableUnavailable = append(toolSnapshot.RetryableUnavailable, requirement)
			continue
		}
		if !availableMCP[strings.TrimSpace(toolKey)] {
			toolSnapshot.MissingBindings = append(toolSnapshot.MissingBindings, requirement)
		}
	}
	return capabilitySnapshot, toolSnapshot, nil
}

func unavailableRuntimeSnapshot(retryAfter time.Time) project.PreDispatchRuntimeSnapshot {
	return project.PreDispatchRuntimeSnapshot{
		ContractVersionAccepted: true,
		RetryAfter:              retryAfter,
	}
}

func (a preDispatchGateAdapter) clock() time.Time {
	if a.now != nil {
		return a.now().UTC()
	}
	return time.Now().UTC()
}

func (a preDispatchGateAdapter) runtimeNodeForInstance(ctx context.Context, instance employee.DigitalEmployeeExecutionInstanceRecord) (runtimepkg.NodeRecord, error) {
	if nodeID := runtimeNodeSelectorID(instance); nodeID != "" {
		return a.runtimeNodes.GetNode(ctx, nodeID)
	}
	if reader, ok := a.runtimeNodes.(gateRuntimeNodeIDReader); ok {
		return reader.GetNodeByID(ctx, instance.RuntimeNodeID)
	}
	return runtimepkg.NodeRecord{}, pgx.ErrNoRows
}

func (a preDispatchGateAdapter) runtimeNodeForProjectTaskPreflight(ctx context.Context, preflight employee.StartProjectTaskRunPreflight) (runtimepkg.NodeRecord, error) {
	if reader, ok := a.runtimeNodes.(gateRuntimeNodeIDReader); ok {
		return reader.GetNodeByID(ctx, preflight.RuntimeNodeID)
	}
	if nodeID := strings.TrimSpace(preflight.NodeID); nodeID != "" {
		return a.runtimeNodes.GetNode(ctx, nodeID)
	}
	return runtimepkg.NodeRecord{}, pgx.ErrNoRows
}

func normalGateAbsence(err error) bool {
	return errors.Is(err, employee.ErrNotFound) ||
		errors.Is(err, capability.ErrNotFound) ||
		errors.Is(err, pgx.ErrNoRows)
}

func executionInstanceRunnable(status employee.ExecutionInstanceStatus) bool {
	return status == employee.ExecutionInstanceStatusReady || status == employee.ExecutionInstanceStatusActive
}

func runtimeNodeSelectorID(instance employee.DigitalEmployeeExecutionInstanceRecord) string {
	if instance.RuntimeSelector != nil {
		if nodeID := strings.TrimSpace(stringFromAny(instance.RuntimeSelector["node_id"])); nodeID != "" {
			return nodeID
		}
	}
	return ""
}

func runtimeNodeOnline(node runtimepkg.NodeRecord, now time.Time) bool {
	if strings.TrimSpace(node.Status) != "online" || !node.LastHeartbeatAt.Valid {
		return false
	}
	heartbeatAt := node.LastHeartbeatAt.Time.UTC()
	return !heartbeatAt.After(now) && now.Sub(heartbeatAt) <= runtimeNodeHeartbeatTTL
}

func runtimeProviderSupported(node runtimepkg.NodeRecord, providerType string) bool {
	providerType = strings.TrimSpace(providerType)
	if providerType == "" || len(node.SupportedProviders) == 0 {
		return true
	}
	var raw any
	if err := json.Unmarshal(node.SupportedProviders, &raw); err != nil {
		return true
	}
	return providerInSupportedProviderValue(raw, providerType)
}

func providerInSupportedProviderValue(value any, providerType string) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if strings.TrimSpace(stringFromAny(item)) == providerType {
				return true
			}
		}
	case map[string]any:
		for _, key := range []string{"providers", "provider_types", "supported_providers"} {
			if providerInSupportedProviderValue(typed[key], providerType) {
				return true
			}
		}
	case string:
		return strings.TrimSpace(typed) == providerType
	}
	return false
}

func gateStringList(value any) []string {
	values := make([]string, 0)
	seen := map[string]struct{}{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		values = append(values, raw)
	}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			add(item)
		}
	case []any:
		for _, item := range typed {
			add(stringFromAny(item))
		}
	case string:
		add(typed)
	}
	return values
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func effectiveMCPServerNames(servers []capability.MCPServer) map[string]bool {
	names := make(map[string]bool, len(servers))
	for _, server := range servers {
		if !server.DisabledAt.IsZero() || !server.DeletedAt.IsZero() {
			continue
		}
		if server.Status != "" && server.Status != "active" {
			continue
		}
		name := strings.TrimSpace(server.Name)
		if name == "" {
			continue
		}
		names[name] = true
		names["mcp:"+name] = true
	}
	return names
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
		"in_flight_tasks": signal.InFlightAttemptCount,
		"available_slots": availableSlots,
		"lendable":        lendable,
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
