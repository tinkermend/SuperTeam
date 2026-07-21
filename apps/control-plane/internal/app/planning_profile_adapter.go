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
	GetLatestDigitalEmployeeConfigRevision(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (employee.EmployeeConfigInput, error)
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

type digitalEmployeePlanningProfileAdapter struct {
	reader          digitalEmployeePlanningProfileReader
	projectTaskRuns gateProjectTaskRunPreflightReader
	// effectiveSkillSlugs / effectiveMCPServerKeys feed the planning profile's
	// capability view from the authoritative binding tables (team inheritance
	// included). The config revision JSON no longer carries skills/mcp_servers.
	effectiveSkillSlugs    func(ctx context.Context, tenantID, employeeID uuid.UUID) ([]string, error)
	effectiveMCPServerKeys func(ctx context.Context, tenantID, employeeID uuid.UUID) ([]string, error)
}

func planningProfileSourceWithPreflights(source digitalEmployeePlanningProfileAdapter, preflights gateProjectTaskRunPreflightReader) digitalEmployeePlanningProfileAdapter {
	source.projectTaskRuns = preflights
	return source
}

type preDispatchGateAdapter struct {
	employees       digitalEmployeePlanningProfileReader
	projectTaskRuns gateProjectTaskRunPreflightReader
	runtimeNodes    gateRuntimeNodeReader
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
			DigitalEmployeeID: record.DigitalEmployeeID,
			ProviderType:      record.ProviderType,
			ExecutionStatus:   record.ExecutionStatus,
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

// gateProjectTaskNodeResolverAdapter implements
// projectcoordination.GateProjectTaskNodeResolver by delegating to the same
// three-layer resolver dispatch uses (Service.ResolveProjectTaskNode), always
// in DryRun mode so gate evaluation — which can run repeatedly, e.g. on retry
// polling — never mutates employee affinity as a side effect of merely
// checking readiness.
type gateProjectTaskNodeResolverAdapter struct {
	service *project.Service
}

func (a gateProjectTaskNodeResolverAdapter) ResolveProjectTaskNodeForGate(ctx context.Context, tenantID, projectID, employeeID, projectTaskID uuid.UUID) (project.NodeResolution, error) {
	return a.service.ResolveProjectTaskNode(ctx, project.ResolveProjectTaskNodeInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: employeeID,
		ProjectTaskID:     projectTaskID,
		DryRun:            true,
	})
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
			Description:       strings.TrimSpace(stringPointerValue(employeeRecord.Description)),
			EmployeeStatus:    string(employeeRecord.Status),
			ProviderType:      employeeRecord.ProviderType,
			PermissionPolicy:  clonePlanningProfileMap(employeeRecord.PermissionPolicy),
		}
		config, err := a.reader.GetLatestDigitalEmployeeConfigRevision(ctx, tenantID, employeeID)
		if err != nil {
			if !normalGateAbsence(err) {
				return nil, err
			}
		} else {
			record.PersonaMemoryMarkdown = strings.TrimSpace(config.PersonaMemoryMarkdown)
			record.CapabilityBindings = clonePlanningProfileMap(config.CapabilityBindings)
		}
		if err := a.applyEffectiveCapabilityBindings(ctx, tenantID, employeeID, &record); err != nil {
			return nil, err
		}
		if a.projectTaskRuns != nil && projectID != uuid.Nil {
			preflight, err := a.projectTaskRuns.GetProjectTaskRunPreflight(ctx, tenantID, projectID, employeeID)
			if err != nil {
				if !normalGateAbsence(err) {
					return nil, err
				}
			} else {
				applyProjectTaskPreflightPlanningFacts(&record, preflight)
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

// applyEffectiveCapabilityBindings overlays the authoritative skill/MCP
// bindings (binding tables, team inheritance included) onto the planning
// profile's capability_bindings map, upgrading the planner's view from the
// retired config-revision declaration to real effective bindings.
func (a digitalEmployeePlanningProfileAdapter) applyEffectiveCapabilityBindings(ctx context.Context, tenantID, employeeID uuid.UUID, record *projectcoordination.DigitalEmployeePlanningProfileSourceRecord) error {
	if a.effectiveSkillSlugs == nil && a.effectiveMCPServerKeys == nil {
		return nil
	}
	bindings := record.CapabilityBindings
	if bindings == nil {
		bindings = map[string]any{}
	}
	if a.effectiveSkillSlugs != nil {
		slugs, err := a.effectiveSkillSlugs(ctx, tenantID, employeeID)
		if err != nil {
			if !normalGateAbsence(err) {
				return err
			}
		} else {
			bindings["skills"] = stringsToAnyList(slugs)
		}
	}
	if a.effectiveMCPServerKeys != nil {
		keys, err := a.effectiveMCPServerKeys(ctx, tenantID, employeeID)
		if err != nil {
			if !normalGateAbsence(err) {
				return err
			}
		} else {
			bindings["mcp_servers"] = stringsToAnyList(keys)
		}
	}
	record.CapabilityBindings = bindings
	return nil
}

func stringsToAnyList(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
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
	record.ExecutionStatus = projectTaskPreflightExecutionStatus(preflight)
}

func projectTaskPreflightExecutionStatus(preflight employee.StartProjectTaskRunPreflight) string {
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
			return employeeSnapshot, runtimeSnapshot, nil
		}
		return a.runtimeSnapshotFromProjectTaskPreflight(ctx, tenantID, employeeSnapshot, preflight)
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

func (a preDispatchGateAdapter) GetEmployeeCapabilitySnapshot(ctx context.Context, tenantID, employeeID uuid.UUID, task project.ProjectTask) (project.PreDispatchCapabilitySnapshot, error) {
	// Tool/MCP availability is the provider concern (§1.7); capability keys are
	// advisory (§1.6). Only the capability diff is returned, for display.
	return project.PreDispatchCapabilitySnapshot{}, nil
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

func runtimeNodeOnline(node runtimepkg.NodeRecord, now time.Time) bool {
	if strings.TrimSpace(node.Status) != "online" || !node.LastHeartbeatAt.Valid {
		return false
	}
	heartbeatAt := node.LastHeartbeatAt.Time.UTC()
	return !heartbeatAt.After(now) && now.Sub(heartbeatAt) <= runtimeNodeHeartbeatTTL
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
