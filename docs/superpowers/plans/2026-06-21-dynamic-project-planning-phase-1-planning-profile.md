# Dynamic Project Planning Phase 1 Planning Profile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add explainable digital-employee planning profiles to the current project coordinator planner so each planned task records why an employee was selected and the Control Plane can reject or review weak selections.

**Architecture:** Keep Phase 1 inside the existing route-decision and ProjectTask metadata path. Add profile types, scoring, optional ProjectStore profile-source injection, planner prompt/JSON fields, and validation that uses the snapshot profile facts; do not add new database tables or OpenAPI schemas in this phase.

**Tech Stack:** Go Control Plane, Temporal activity package `projectcoordination`, existing `project_tasks.planner_metadata`, existing `project_route_decisions.input_requirements`, OpenAI-compatible planner JSON mode, Go unit tests.

---

## Execution Preflight

This root checkout is dirty with unrelated work. Start implementation in an isolated worktree before touching code:

```bash
git status --short
git worktree add ../SuperTeam-planning-profile -b codex/dynamic-planning-phase-1
cd ../SuperTeam-planning-profile
git status --short
```

Expected:

- the root checkout may show unrelated changes
- the implementation worktree should start clean
- branch should be `codex/dynamic-planning-phase-1`

## File Structure

- Create `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go`: profile data model, source-record model, snapshot builder helpers, scoring, hash helpers, and metadata helpers.
- Create `apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go`: profile builder, unknown fallback, hard failure, scoring, hash, and metadata tests.
- Modify `apps/control-plane/internal/workflow/projectcoordination/planner.go`: add `PlanningProfile` to `ProjectMemberSnapshot`, add selection-evidence fields to `PlannedTask`.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`: optional profile-source injection, profile attachment during snapshot loading, and per-task profile metadata persistence.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`: snapshot profile source tests and metadata persistence tests.
- Modify `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go`: prompt text, prompt payload, planner JSON decoding, and profile-aware validation call.
- Modify `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go`: prompt, decode, validation, and fixture updates.
- Modify `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`: add profile-aware plan validation while preserving current graph validation helper.
- Modify `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`: selection evidence, missing capability, and review-required validation tests.
- Modify `apps/control-plane/internal/workflow/projectcoordination/heuristic_planner_test.go`: test-only planner emits default selection metadata so tests remain representative.
- Create `apps/control-plane/internal/app/planning_profile_adapter.go`: app-layer adapter that reads existing employee records, effective config, and execution instance facts.
- Create `apps/control-plane/internal/app/planning_profile_adapter_test.go`: adapter mapping and missing-source behavior tests.
- Modify `apps/control-plane/internal/app/app.go`: wire the adapter into `ProjectStore`.

No migration, sqlc, contract, or frontend file should change in Phase 1.

### Task 1: Planning Profile Model, Builder, Scoring, And Hash

**Files:**
- Create: `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go`

- [ ] **Step 1: Write failing profile tests**

Create `apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go` with:

```go
package projectcoordination

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
)

func TestBuildDigitalEmployeePlanningProfileFallsBackToUnknownFacts(t *testing.T) {
	employeeID := uuid.New()
	member := project.ProjectMember{
		PrincipalID:         employeeID,
		ProjectRole:         project.ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("数据分析员工"),
		Settings:            map[string]any{"planning_role": "data_analyst"},
	}

	profile := BuildDigitalEmployeePlanningProfile(member, DigitalEmployeePlanningProfileSourceRecord{}, true)

	require.Equal(t, employeeID, profile.DigitalEmployeeID)
	require.Equal(t, "数据分析员工", profile.DisplayName)
	require.Equal(t, "data_analyst", profile.RoleProfile.PrimaryRole)
	require.Equal(t, "unknown", profile.RuntimeRequirements.ProviderStatus)
	require.Equal(t, "unknown", profile.ProfileFreshness.SourceState)
	require.Contains(t, profile.SelectionWarnings, "profile_source_missing")
	require.Empty(t, profile.HardFailures)
}

func TestBuildDigitalEmployeePlanningProfileUsesSourceFacts(t *testing.T) {
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	member := project.ProjectMember{
		PrincipalID:         employeeID,
		ProjectRole:         project.ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("执行员工"),
	}

	profile := BuildDigitalEmployeePlanningProfile(member, DigitalEmployeePlanningProfileSourceRecord{
		DigitalEmployeeID: employeeID,
		EmployeeType:      "database_admin",
		Role:              "数据库分析",
		EmployeeStatus:    "active",
		RoleProfile:       map[string]any{"primary_role": "data_analyst", "description": "数据库分析"},
		CapabilitySelection: map[string]any{
			"enabled_skills":                []any{"sql.analysis", "data.quality.check"},
			"enabled_mcp_servers":           []any{"postgres.readonly"},
			"enabled_external_capabilities": []any{"database.read"},
			"enabled_provider_types":        []any{"codex"},
		},
		PermissionPolicy: map[string]any{
			"grants": []any{"database.read:dev_database"},
		},
		ContextPolicy:         map[string]any{"max_context_classification": "internal"},
		RuntimeNodeID:         runtimeNodeID,
		ProviderType:          "codex",
		ExecutionStatus:       "ready",
		EffectiveConfigStatus: "approved",
		FetchedAt:             now,
	}, true)

	require.Equal(t, "data_analyst", profile.RoleProfile.PrimaryRole)
	require.Equal(t, []PlanningCapability{{Key: "database.read", Level: "strong", Source: "capability_selection.enabled_external_capabilities", Confidence: 0.9}}, profile.Capabilities)
	require.Equal(t, []PlanningSkill{{Key: "sql.analysis", Source: "capability_selection.enabled_skills"}, {Key: "data.quality.check", Source: "capability_selection.enabled_skills"}}, profile.Skills)
	require.Equal(t, []PlanningToolBinding{{Type: "mcp", Key: "postgres.readonly", Status: "available"}}, profile.ToolBindings)
	require.Equal(t, []string{"codex"}, profile.RuntimeRequirements.ProviderTypes)
	require.Equal(t, "ready", profile.RuntimeRequirements.ProviderStatus)
	require.Equal(t, runtimeNodeID.String(), profile.RuntimeRequirements.RuntimeNodeID)
	require.Equal(t, []PlanningPermission{{Scope: "database.read", Resource: "dev_database", Status: "granted"}}, profile.Permissions)
	require.Equal(t, "ready", profile.ProfileFreshness.SourceState)
}

func TestScorePlanningProfileRecordsHardFailures(t *testing.T) {
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID: uuid.New(),
		RoleProfile:      PlanningRoleProfile{PrimaryRole: "data_analyst"},
		Capabilities:     []PlanningCapability{{Key: "database.read", Level: "strong", Source: "test", Confidence: 1}, {Key: "sql.analysis", Level: "strong", Source: "test", Confidence: 1}},
		Skills:           []PlanningSkill{{Key: "sql.analysis", Source: "test"}},
		ToolBindings:     []PlanningToolBinding{{Type: "mcp", Key: "postgres.readonly", Status: "available"}},
		RuntimeRequirements: PlanningRuntimeRequirements{
			ProviderTypes:  []string{"codex"},
			ProviderStatus: "ready",
		},
		Permissions: []PlanningPermission{{Scope: "database.read", Resource: "dev_database", Status: "granted"}},
		LoadState:   PlanningLoadState{AvailableSlots: 1, Lendable: true},
	}

	result := ScorePlanningProfile(profile, PlanningTaskRequirements{
		TaskType:               "database_analysis",
		RequiredCapabilities:   []string{"database.read", "sql.analysis"},
		PermissionRequirements: []string{"database.read:dev_database"},
		ToolRequirements:       []string{"mcp:postgres.readonly"},
		RuntimeRequirements:    []string{"provider:codex"},
	})

	require.Equal(t, 100, result.Score)
	require.Equal(t, []string{"database.read", "sql.analysis"}, result.MatchedCapabilities)
	require.Empty(t, result.MissingCapabilities)
	require.Empty(t, result.HardFailures)

	result = ScorePlanningProfile(profile, PlanningTaskRequirements{
		TaskType:             "database_analysis",
		RequiredCapabilities: []string{"database.write"},
	})

	require.Equal(t, []string{"database.write"}, result.MissingCapabilities)
	require.Contains(t, result.HardFailures, "missing_capability:database.write")
}

func TestPlanningProfileSnapshotHashIsStable(t *testing.T) {
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		DisplayName:       "A",
		RoleProfile:       PlanningRoleProfile{PrimaryRole: "executor"},
		Skills:            []PlanningSkill{{Key: "sql.analysis", Source: "test"}},
	}

	first := PlanningProfileSnapshotHash(profile)
	second := PlanningProfileSnapshotHash(profile)

	require.Len(t, first, 64)
	require.Equal(t, first, second)
}
```

- [ ] **Step 2: Run profile tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestBuildDigitalEmployeePlanningProfile|TestScorePlanningProfile|TestPlanningProfileSnapshotHash' -count=1
```

Expected: FAIL with undefined symbols including `BuildDigitalEmployeePlanningProfile`, `DigitalEmployeePlanningProfileSourceRecord`, `ScorePlanningProfile`, and `PlanningProfileSnapshotHash`.

- [ ] **Step 3: Add planning profile implementation**

Create `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go`:

```go
package projectcoordination

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

type DigitalEmployeePlanningProfile struct {
	DigitalEmployeeID    uuid.UUID                       `json:"digital_employee_id"`
	DisplayName          string                          `json:"display_name,omitempty"`
	RoleProfile          PlanningRoleProfile             `json:"role_profile"`
	Capabilities         []PlanningCapability            `json:"capabilities,omitempty"`
	Skills               []PlanningSkill                 `json:"skills,omitempty"`
	ToolBindings         []PlanningToolBinding           `json:"tool_bindings,omitempty"`
	RuntimeRequirements  PlanningRuntimeRequirements     `json:"runtime_requirements"`
	Permissions          []PlanningPermission            `json:"permissions,omitempty"`
	ContextPolicy        PlanningContextPolicy           `json:"context_policy"`
	LoadState            PlanningLoadState               `json:"load_state"`
	ReliabilitySignals   PlanningReliabilitySignals      `json:"reliability_signals"`
	ProfileFreshness     PlanningProfileFreshness        `json:"profile_freshness"`
	SelectionScore       int                             `json:"selection_score,omitempty"`
	MatchedCapabilities  []string                        `json:"matched_capabilities,omitempty"`
	MissingCapabilities  []string                        `json:"missing_capabilities,omitempty"`
	SelectionWarnings    []string                        `json:"selection_warnings,omitempty"`
	HardFailures         []string                        `json:"hard_failures,omitempty"`
	SourceMetadata       map[string]any                  `json:"source_metadata,omitempty"`
}

type PlanningRoleProfile struct {
	PrimaryRole    string   `json:"primary_role,omitempty"`
	SecondaryRoles []string `json:"secondary_roles,omitempty"`
	Description    string   `json:"description,omitempty"`
	ProjectRole    string   `json:"project_role,omitempty"`
	EmployeeType   string   `json:"employee_type,omitempty"`
}

type PlanningCapability struct {
	Key        string  `json:"key"`
	Level      string  `json:"level"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

type PlanningSkill struct {
	Key    string `json:"key"`
	Source string `json:"source"`
}

type PlanningToolBinding struct {
	Type   string `json:"type"`
	Key    string `json:"key"`
	Status string `json:"status"`
}

type PlanningRuntimeRequirements struct {
	ProviderTypes  []string `json:"provider_types,omitempty"`
	ProviderStatus string   `json:"provider_status"`
	RuntimeNodeID  string   `json:"runtime_node_id,omitempty"`
	WorkspaceScope string   `json:"workspace_scope,omitempty"`
}

type PlanningPermission struct {
	Scope    string `json:"scope"`
	Resource string `json:"resource,omitempty"`
	Status   string `json:"status"`
}

type PlanningContextPolicy struct {
	MaxContextClassification string `json:"max_context_classification,omitempty"`
	AllowsSensitiveContext   bool   `json:"allows_sensitive_context"`
	SourceState              string `json:"source_state"`
}

type PlanningLoadState struct {
	RunningTasks   int  `json:"running_tasks"`
	AvailableSlots int  `json:"available_slots"`
	Lendable       bool `json:"lendable"`
}

type PlanningReliabilitySignals struct {
	RecentSuccessCount     int `json:"recent_success_count"`
	RecentFailureCount     int `json:"recent_failure_count"`
	RecentHumanRejectCount int `json:"recent_human_reject_count"`
}

type PlanningProfileFreshness struct {
	GeneratedAt    time.Time      `json:"generated_at"`
	SourceState    string         `json:"source_state"`
	SourceVersions map[string]any `json:"source_versions,omitempty"`
}

type DigitalEmployeePlanningProfileSourceRecord struct {
	DigitalEmployeeID     uuid.UUID
	EmployeeType          string
	Role                  string
	EmployeeStatus        string
	RoleProfile           map[string]any
	CapabilitySelection   map[string]any
	PermissionPolicy      map[string]any
	ContextPolicy         map[string]any
	RuntimeNodeID         uuid.UUID
	ProviderType          string
	ExecutionStatus       string
	EffectiveConfigStatus string
	LoadState             map[string]any
	ReliabilitySignals    map[string]any
	FetchedAt             time.Time
}

type PlanningTaskRequirements struct {
	TaskType               string
	RequiredCapabilities   []string
	PermissionRequirements []string
	ToolRequirements       []string
	RuntimeRequirements    []string
}

type PlanningProfileScore struct {
	Score               int
	MatchedCapabilities []string
	MissingCapabilities []string
	HardFailures        []string
	Warnings            []string
}

func BuildDigitalEmployeePlanningProfile(member project.ProjectMember, source DigitalEmployeePlanningProfileSourceRecord, runtimeReady bool) DigitalEmployeePlanningProfile {
	displayName := ""
	if member.DisplayNameSnapshot != nil {
		displayName = strings.TrimSpace(*member.DisplayNameSnapshot)
	}
	sourceState := "ready"
	warnings := []string{}
	if source.DigitalEmployeeID == uuid.Nil {
		sourceState = "unknown"
		warnings = append(warnings, "profile_source_missing")
	}
	generatedAt := source.FetchedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID: member.PrincipalID,
		DisplayName:       displayName,
		RoleProfile: PlanningRoleProfile{
			PrimaryRole:  firstString(source.RoleProfile, "primary_role", firstString(member.Settings, "planning_role", source.Role)),
			Description:  firstString(source.RoleProfile, "description", ""),
			ProjectRole:  string(member.ProjectRole),
			EmployeeType: source.EmployeeType,
		},
		Capabilities:        capabilitiesFromSelection(source.CapabilitySelection),
		Skills:              skillsFromSelection(source.CapabilitySelection),
		ToolBindings:        toolBindingsFromSelection(source.CapabilitySelection),
		RuntimeRequirements: runtimeRequirementsFromSource(source, runtimeReady),
		Permissions:         permissionsFromPolicy(source.PermissionPolicy),
		ContextPolicy: PlanningContextPolicy{
			MaxContextClassification: firstString(source.ContextPolicy, "max_context_classification", ""),
			AllowsSensitiveContext:   firstBool(source.ContextPolicy, "allows_sensitive_context", false),
			SourceState:              sourceState,
		},
		LoadState:          loadStateFromSource(source.LoadState),
		ReliabilitySignals: reliabilitySignalsFromSource(source.ReliabilitySignals),
		ProfileFreshness: PlanningProfileFreshness{
			GeneratedAt: generatedAt,
			SourceState: sourceState,
			SourceVersions: map[string]any{
				"effective_config_status": source.EffectiveConfigStatus,
				"employee_status":         source.EmployeeStatus,
				"execution_status":        source.ExecutionStatus,
			},
		},
		SelectionWarnings: warnings,
	}
	return profile
}

func ScorePlanningProfile(profile DigitalEmployeePlanningProfile, req PlanningTaskRequirements) PlanningProfileScore {
	score := 0
	matched := []string{}
	missing := []string{}
	hardFailures := []string{}
	capabilitySet := planningCapabilitySet(profile)
	for _, capability := range normalizeStringSet(req.RequiredCapabilities) {
		if capabilitySet[capability] {
			matched = append(matched, capability)
			continue
		}
		missing = append(missing, capability)
		hardFailures = append(hardFailures, "missing_capability:"+capability)
	}
	if len(req.RequiredCapabilities) == 0 || len(missing) == 0 {
		score += 40
	}
	if roleMatchesTaskType(profile.RoleProfile, req.TaskType) {
		score += 20
	}
	if runtimeRequirementsSatisfied(profile.RuntimeRequirements, req.RuntimeRequirements) {
		score += 15
	} else {
		hardFailures = append(hardFailures, "runtime_requirement_unsatisfied")
	}
	if permissionAndToolSatisfied(profile, req) {
		score += 10
	} else {
		hardFailures = append(hardFailures, "permission_or_tool_requirement_unsatisfied")
	}
	if profile.LoadState.AvailableSlots > 0 && profile.LoadState.Lendable {
		score += 10
	}
	if profile.ReliabilitySignals.RecentFailureCount == 0 && profile.ReliabilitySignals.RecentHumanRejectCount == 0 {
		score += 5
	}
	if len(hardFailures) > 0 && score > 80 {
		score = 80
	}
	return PlanningProfileScore{
		Score:               score,
		MatchedCapabilities: matched,
		MissingCapabilities: missing,
		HardFailures:        hardFailures,
	}
}

func PlanningProfileSnapshotHash(profile DigitalEmployeePlanningProfile) string {
	copy := profile
	copy.ProfileFreshness.GeneratedAt = time.Time{}
	body, err := json.Marshal(copy)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func capabilitiesFromSelection(selection map[string]any) []PlanningCapability {
	values := stringListAny(selection, "enabled_external_capabilities")
	out := make([]PlanningCapability, 0, len(values))
	for _, value := range values {
		out = append(out, PlanningCapability{Key: value, Level: "strong", Source: "capability_selection.enabled_external_capabilities", Confidence: 0.9})
	}
	return out
}

func skillsFromSelection(selection map[string]any) []PlanningSkill {
	values := stringListAny(selection, "enabled_skills")
	out := make([]PlanningSkill, 0, len(values))
	for _, value := range values {
		out = append(out, PlanningSkill{Key: value, Source: "capability_selection.enabled_skills"})
	}
	return out
}

func toolBindingsFromSelection(selection map[string]any) []PlanningToolBinding {
	servers := stringListAny(selection, "enabled_mcp_servers")
	out := make([]PlanningToolBinding, 0, len(servers))
	for _, server := range servers {
		out = append(out, PlanningToolBinding{Type: "mcp", Key: server, Status: "available"})
	}
	return out
}

func runtimeRequirementsFromSource(source DigitalEmployeePlanningProfileSourceRecord, runtimeReady bool) PlanningRuntimeRequirements {
	status := source.ExecutionStatus
	if strings.TrimSpace(status) == "" {
		status = "unknown"
	}
	if !runtimeReady && status == "ready" {
		status = "unknown"
	}
	providerTypes := stringListAny(source.CapabilitySelection, "enabled_provider_types")
	if strings.TrimSpace(source.ProviderType) != "" && !containsString(providerTypes, source.ProviderType) {
		providerTypes = append(providerTypes, source.ProviderType)
	}
	sort.Strings(providerTypes)
	runtimeNodeID := ""
	if source.RuntimeNodeID != uuid.Nil {
		runtimeNodeID = source.RuntimeNodeID.String()
	}
	return PlanningRuntimeRequirements{ProviderTypes: providerTypes, ProviderStatus: status, RuntimeNodeID: runtimeNodeID}
}

func permissionsFromPolicy(policy map[string]any) []PlanningPermission {
	grants := stringListAny(policy, "grants")
	out := make([]PlanningPermission, 0, len(grants))
	for _, grant := range grants {
		scope, resource, _ := strings.Cut(grant, ":")
		out = append(out, PlanningPermission{Scope: scope, Resource: resource, Status: "granted"})
	}
	return out
}

func loadStateFromSource(values map[string]any) PlanningLoadState {
	return PlanningLoadState{
		RunningTasks:   intFromAny(values["running_tasks"]),
		AvailableSlots: intFromAny(values["available_slots"]),
		Lendable:       firstBool(values, "lendable", true),
	}
}

func reliabilitySignalsFromSource(values map[string]any) PlanningReliabilitySignals {
	return PlanningReliabilitySignals{
		RecentSuccessCount:     intFromAny(values["recent_success_count"]),
		RecentFailureCount:     intFromAny(values["recent_failure_count"]),
		RecentHumanRejectCount: intFromAny(values["recent_human_reject_count"]),
	}
}

func planningCapabilitySet(profile DigitalEmployeePlanningProfile) map[string]bool {
	set := map[string]bool{}
	for _, capability := range profile.Capabilities {
		set[capability.Key] = true
	}
	for _, skill := range profile.Skills {
		set[skill.Key] = true
	}
	return set
}

func roleMatchesTaskType(role PlanningRoleProfile, taskType string) bool {
	switch taskType {
	case "", "execution":
		return true
	case "database_analysis":
		return role.PrimaryRole == "data_analyst" || role.EmployeeType == "database_admin"
	case "incident_diagnosis":
		return role.PrimaryRole == "incident_investigator"
	case "feature_development":
		return role.PrimaryRole == "backend_engineer" || role.PrimaryRole == "frontend_engineer" || role.PrimaryRole == "fullstack_engineer"
	default:
		return role.PrimaryRole == taskType || containsString(role.SecondaryRoles, taskType)
	}
}

func runtimeRequirementsSatisfied(runtime PlanningRuntimeRequirements, requirements []string) bool {
	if len(requirements) == 0 {
		return runtime.ProviderStatus == "ready" || runtime.ProviderStatus == "active" || runtime.ProviderStatus == "unknown"
	}
	for _, requirement := range normalizeStringSet(requirements) {
		if strings.HasPrefix(requirement, "provider:") {
			provider := strings.TrimPrefix(requirement, "provider:")
			if !containsString(runtime.ProviderTypes, provider) {
				return false
			}
		}
	}
	return runtime.ProviderStatus == "ready" || runtime.ProviderStatus == "active"
}

func permissionAndToolSatisfied(profile DigitalEmployeePlanningProfile, req PlanningTaskRequirements) bool {
	for _, requirement := range normalizeStringSet(req.PermissionRequirements) {
		scope, resource, _ := strings.Cut(requirement, ":")
		found := false
		for _, permission := range profile.Permissions {
			if permission.Scope == scope && (resource == "" || permission.Resource == resource) && permission.Status == "granted" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, requirement := range normalizeStringSet(req.ToolRequirements) {
		toolType, key, ok := strings.Cut(requirement, ":")
		if !ok {
			toolType = "mcp"
			key = requirement
		}
		found := false
		for _, binding := range profile.ToolBindings {
			if binding.Type == toolType && binding.Key == key && binding.Status == "available" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func stringListAny(values map[string]any, key string) []string {
	raw, ok := values[key]
	if !ok {
		return nil
	}
	out := []string{}
	switch typed := raw.(type) {
	case []string:
		out = append(out, typed...)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
	case string:
		out = append(out, typed)
	}
	return normalizeStringSet(out)
}

func normalizeStringSet(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func firstString(values map[string]any, key string, fallback string) string {
	if values == nil {
		return strings.TrimSpace(fallback)
	}
	if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

func firstBool(values map[string]any, key string, fallback bool) bool {
	if values == nil {
		return fallback
	}
	if value, ok := values[key].(bool); ok {
		return value
	}
	return fallback
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run profile tests and verify they pass**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestBuildDigitalEmployeePlanningProfile|TestScorePlanningProfile|TestPlanningProfileSnapshotHash' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 1**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/planning_profile.go apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go
git commit -m "feat: add planning profile scoring"
```

### Task 2: Attach Planning Profiles To Coordination Snapshots

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Create: `apps/control-plane/internal/app/planning_profile_adapter.go`
- Create: `apps/control-plane/internal/app/planning_profile_adapter_test.go`
- Modify: `apps/control-plane/internal/app/app.go`

- [ ] **Step 1: Write ProjectStore snapshot tests**

Append to `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`:

```go
func TestProjectStoreSnapshotAttachesPlanningProfilesFromSource(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID},
		demand: project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "分析数据库", Content: strPtr("检查慢查询")},
		members: []project.ProjectMember{{
			ID:                  uuid.New(),
			TenantID:            tenantID,
			ProjectID:           projectID,
			PrincipalType:       project.PrincipalTypeDigitalEmployee,
			PrincipalID:         employeeID,
			ProjectRole:         project.ProjectRoleExecutor,
			Status:              "active",
			DisplayNameSnapshot: strPtr("数据库员工"),
		}},
	}
	source := fakePlanningProfileSource{
		records: map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{
			employeeID: {
				DigitalEmployeeID: employeeID,
				EmployeeType:      "database_admin",
				RoleProfile:       map[string]any{"primary_role": "data_analyst"},
				CapabilitySelection: map[string]any{
					"enabled_external_capabilities": []any{"database.read"},
					"enabled_skills":                []any{"sql.analysis"},
					"enabled_provider_types":        []any{"codex"},
				},
				ExecutionStatus:       "ready",
				EffectiveConfigStatus: "approved",
			},
		},
	}

	snapshot, err := NewProjectStore(repo).WithDigitalEmployeePlanningProfiles(source).LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	})

	require.NoError(t, err)
	require.Len(t, snapshot.DigitalEmployeePool, 1)
	profile := snapshot.DigitalEmployeePool[0].PlanningProfile
	require.NotNil(t, profile)
	require.Equal(t, employeeID, profile.DigitalEmployeeID)
	require.Equal(t, "data_analyst", profile.RoleProfile.PrimaryRole)
	require.Equal(t, []PlanningCapability{{Key: "database.read", Level: "strong", Source: "capability_selection.enabled_external_capabilities", Confidence: 0.9}}, profile.Capabilities)
}

func TestProjectStoreSnapshotKeepsUnknownProfileWhenProfileSourceFails(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID},
		demand: project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "分析数据库"},
		members: []project.ProjectMember{{
			ID:            uuid.New(),
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   employeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
	}

	snapshot, err := NewProjectStore(repo).WithDigitalEmployeePlanningProfiles(fakePlanningProfileSource{err: errors.New("source down")}).LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	})

	require.NoError(t, err)
	require.Len(t, snapshot.DigitalEmployeePool, 1)
	require.NotNil(t, snapshot.DigitalEmployeePool[0].PlanningProfile)
	require.Equal(t, "unknown", snapshot.DigitalEmployeePool[0].PlanningProfile.ProfileFreshness.SourceState)
	require.Contains(t, snapshot.DigitalEmployeePool[0].PlanningProfile.SelectionWarnings, "profile_source_missing")
}

type fakePlanningProfileSource struct {
	records map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord
	err     error
}

func (s fakePlanningProfileSource) PlanningProfileRecords(_ context.Context, _ uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord{}
	for _, id := range employeeIDs {
		if record, ok := s.records[id]; ok {
			out[id] = record
		}
	}
	return out, nil
}
```

- [ ] **Step 2: Run snapshot tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreSnapshotAttachesPlanningProfilesFromSource|TestProjectStoreSnapshotKeepsUnknownProfileWhenProfileSourceFails' -count=1
```

Expected: FAIL with undefined `WithDigitalEmployeePlanningProfiles` or missing `PlanningProfile` field.

- [ ] **Step 3: Add snapshot fields and profile source interface**

Modify `apps/control-plane/internal/workflow/projectcoordination/planner.go`:

```go
type ProjectMemberSnapshot struct {
	PrincipalID     uuid.UUID                       `json:"principal_id"`
	ProjectRole     string                          `json:"project_role"`
	Status          string                          `json:"status"`
	DisplayName     string                          `json:"display_name,omitempty"`
	PlanningProfile *DigitalEmployeePlanningProfile `json:"planning_profile,omitempty"`
}
```

Add to `apps/control-plane/internal/workflow/projectcoordination/project_store.go` near the other optional dependencies:

```go
type DigitalEmployeePlanningProfileSource interface {
	PlanningProfileRecords(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord, error)
}
```

Modify the `ProjectStore` struct:

```go
type ProjectStore struct {
	repository    project.Repository
	approvals     ApprovalCreator
	inbox         project.DecisionInboxProjector
	runStarter    ProjectTaskRunStarter
	readiness     DigitalEmployeeReadinessChecker
	lending       LendingGatekeeper
	profileSource DigitalEmployeePlanningProfileSource
}
```

Add this method:

```go
func (s *ProjectStore) WithDigitalEmployeePlanningProfiles(source DigitalEmployeePlanningProfileSource) *ProjectStore {
	s.profileSource = source
	return s
}
```

Add this helper:

```go
func (s *ProjectStore) planningProfileSourceRecords(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) map[uuid.UUID]DigitalEmployeePlanningProfileSourceRecord {
	if s.profileSource == nil || len(employeeIDs) == 0 {
		return nil
	}
	records, err := s.profileSource.PlanningProfileRecords(ctx, tenantID, employeeIDs)
	if err != nil {
		return nil
	}
	return records
}
```

In `LoadProjectCoordinationSnapshot`, after lending filtering and before building `pool`, build source records:

```go
profileEmployeeIDs := make([]uuid.UUID, 0, len(candidates))
for _, member := range candidates {
	if lendingEligible != nil && !lendingEligible[member.PrincipalID] {
		continue
	}
	profileEmployeeIDs = append(profileEmployeeIDs, member.PrincipalID)
}
profileRecords := s.planningProfileSourceRecords(ctx, input.TenantID, profileEmployeeIDs)
```

Then set each `ProjectMemberSnapshot.PlanningProfile`:

```go
sourceRecord := profileRecords[member.PrincipalID]
runtimeReady := readyEmployees == nil || readyEmployees[member.PrincipalID]
profile := BuildDigitalEmployeePlanningProfile(member, sourceRecord, runtimeReady)
pool = append(pool, ProjectMemberSnapshot{
	PrincipalID:     member.PrincipalID,
	ProjectRole:     string(member.ProjectRole),
	Status:          member.Status,
	DisplayName:     displayName,
	PlanningProfile: &profile,
})
```

- [ ] **Step 4: Run snapshot tests and verify they pass**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreSnapshotAttachesPlanningProfilesFromSource|TestProjectStoreSnapshotKeepsUnknownProfileWhenProfileSourceFails|TestProjectStoreSnapshotIncludesOnlyActiveDigitalExecutorsAndReviewers' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write app adapter tests**

Create `apps/control-plane/internal/app/planning_profile_adapter_test.go`:

```go
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
```

- [ ] **Step 6: Run app adapter test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/app -run TestDigitalEmployeePlanningProfileAdapterMapsEmployeeFacts -count=1
```

Expected: FAIL with undefined `digitalEmployeePlanningProfileAdapter`.

- [ ] **Step 7: Implement app adapter and wire it**

Create `apps/control-plane/internal/app/planning_profile_adapter.go`:

```go
package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/workflow/projectcoordination"
)

type planningProfileEmployeeReader interface {
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (employee.DigitalEmployeeRecord, error)
	GetCurrentDigitalEmployeeEffectiveConfig(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (employee.DigitalEmployeeEffectiveConfigRecord, error)
	GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (employee.DigitalEmployeeExecutionInstanceRecord, error)
}

type digitalEmployeePlanningProfileAdapter struct {
	reader planningProfileEmployeeReader
}

func (a digitalEmployeePlanningProfileAdapter) PlanningProfileRecords(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]projectcoordination.DigitalEmployeePlanningProfileSourceRecord, error) {
	out := make(map[uuid.UUID]projectcoordination.DigitalEmployeePlanningProfileSourceRecord, len(employeeIDs))
	for _, employeeID := range employeeIDs {
		if employeeID == uuid.Nil {
			continue
		}
		employeeRecord, err := a.reader.GetDigitalEmployee(ctx, tenantID, employeeID)
		if err != nil {
			return nil, err
		}
		record := projectcoordination.DigitalEmployeePlanningProfileSourceRecord{
			DigitalEmployeeID: employeeID,
			EmployeeType:      employeeRecord.EmployeeType,
			Role:              employeeRecord.Role,
			EmployeeStatus:    string(employeeRecord.Status),
			PermissionPolicy:  clonePlanningProfileMap(employeeRecord.PermissionPolicy),
			ContextPolicy:     clonePlanningProfileMap(employeeRecord.ContextPolicy),
			FetchedAt:         time.Now().UTC(),
		}
		effectiveConfig, err := a.reader.GetCurrentDigitalEmployeeEffectiveConfig(ctx, tenantID, employeeID)
		if err == nil {
			record.EffectiveConfigStatus = string(effectiveConfig.Status)
			record.RoleProfile = cloneNestedMap(effectiveConfig.EffectiveConfig, "role_profile")
			record.CapabilitySelection = cloneNestedMap(effectiveConfig.EffectiveConfig, "capability_selection")
		} else if !errors.Is(err, employee.ErrNotFound) {
			return nil, err
		}
		instance, err := a.reader.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, tenantID, employeeID)
		if err == nil {
			record.RuntimeNodeID = instance.RuntimeNodeID
			record.ProviderType = instance.ProviderType
			record.ExecutionStatus = string(instance.Status)
		} else if !errors.Is(err, employee.ErrNotFound) {
			return nil, err
		}
		out[employeeID] = record
	}
	return out, nil
}

func cloneNestedMap(parent map[string]any, key string) map[string]any {
	raw, ok := parent[key]
	if !ok {
		return map[string]any{}
	}
	if typed, ok := raw.(map[string]any); ok {
		return clonePlanningProfileMap(typed)
	}
	return map[string]any{}
}

func clonePlanningProfileMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
```

Modify `apps/control-plane/internal/app/app.go` coordination store wiring:

```go
coordinationStore := projectcoordination.NewProjectStoreWithApprovalsInboxAndRunStarter(
	projectRepository,
	approvalService,
	decisionProjector,
	projectTaskRunStarterAdapter{runService: runService},
).WithDigitalEmployeeReadiness(digitalEmployeeReadinessAdapter{repository: employeeRepository}).
	WithLendingGatekeeper(lendingGatekeeperAdapter{employees: employeeRepository, lending: teamLendingRepository}).
	WithDigitalEmployeePlanningProfiles(digitalEmployeePlanningProfileAdapter{reader: employeeRepository})
```

- [ ] **Step 8: Run Task 2 tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreSnapshotAttachesPlanningProfilesFromSource|TestProjectStoreSnapshotKeepsUnknownProfileWhenProfileSourceFails|TestProjectStoreSnapshotIncludesOnlyActiveDigitalExecutorsAndReviewers' -count=1
go test ./apps/control-plane/internal/app -run TestDigitalEmployeePlanningProfileAdapterMapsEmployeeFacts -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit Task 2**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/planner.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go apps/control-plane/internal/app/planning_profile_adapter.go apps/control-plane/internal/app/planning_profile_adapter_test.go apps/control-plane/internal/app/app.go
git commit -m "feat: attach planning profiles to coordination snapshots"
```

### Task 3: Extend Planner JSON And Prompt Selection Evidence

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/heuristic_planner_test.go`

- [ ] **Step 1: Write planner JSON and prompt tests**

Add tests to `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go`:

```go
func TestOpenAICompatiblePlannerPromptIncludesPlanningProfiles(t *testing.T) {
	employeeID := uuid.New()
	client := &recordingChatCompletionClient{
		response: fmt.Sprintf(`{
			"reason":"按能力选择数据库分析员工",
			"requires_human_review":false,
			"tasks":[{
				"key":"analyze-db",
				"title":"分析数据库",
				"summary":"检查慢查询",
				"selected_employee_id":%q,
				"employee_selection_reason":"具备 database.read 和 sql.analysis",
				"required_capabilities":["database.read","sql.analysis"],
				"matched_capabilities":["database.read","sql.analysis"],
				"missing_capabilities":[],
				"permission_requirements":["database.read:dev_database"],
				"tool_requirements":["mcp:postgres.readonly"],
				"runtime_requirements":["provider:codex"],
				"verification_requirements":["只读查询成功"],
				"selection_score":100,
				"expected_outputs":["execution_summary"],
				"input_requirements":{},
				"handoff_contract":{},
				"blocked_by_keys":[],
				"risk_level":"medium",
				"task_kind":"database_analysis"
			}],
			"budget_estimate":{},
			"template_key":"database_analysis",
			"planner_metadata":{"provider":"openai-compatible"}
		}`, employeeID.String()),
	}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{APIKey: "test", BaseURL: "https://planner.example", Model: "planner-model"}, client)

	_, err := planner.Plan(context.Background(), CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand: DemandSnapshot{ID: uuid.New(), Title: "分析数据库", Content: "检查慢查询"},
		DigitalEmployeePool: []ProjectMemberSnapshot{{
			PrincipalID: employeeID,
			ProjectRole: "executor",
			Status: "active",
			DisplayName: "数据库员工",
			PlanningProfile: &DigitalEmployeePlanningProfile{
				DigitalEmployeeID: employeeID,
				RoleProfile: PlanningRoleProfile{PrimaryRole: "data_analyst"},
				Capabilities: []PlanningCapability{{Key: "database.read", Level: "strong", Source: "test", Confidence: 1}},
				Skills: []PlanningSkill{{Key: "sql.analysis", Source: "test"}},
				ToolBindings: []PlanningToolBinding{{Type: "mcp", Key: "postgres.readonly", Status: "available"}},
				RuntimeRequirements: PlanningRuntimeRequirements{ProviderTypes: []string{"codex"}, ProviderStatus: "ready"},
				Permissions: []PlanningPermission{{Scope: "database.read", Resource: "dev_database", Status: "granted"}},
				LoadState: PlanningLoadState{AvailableSlots: 1, Lendable: true},
				ProfileFreshness: PlanningProfileFreshness{SourceState: "ready"},
			},
		}},
	})

	require.NoError(t, err)
	require.Contains(t, client.lastRequest.User, `"planning_profile"`)
	require.Contains(t, client.lastRequest.User, `"database.read"`)
	require.Contains(t, client.lastRequest.System, "employee_selection_reason")
	require.Contains(t, client.lastRequest.System, "required_capabilities")
}

func TestOpenAICompatiblePlannerDecodesSelectionEvidence(t *testing.T) {
	employeeID := uuid.New()
	content := fmt.Sprintf(`{
		"reason":"按能力选择",
		"requires_human_review":false,
		"tasks":[{
			"key":"analyze-db",
			"title":"分析数据库",
			"summary":"检查慢查询",
			"selected_employee_id":%q,
			"employee_selection_reason":"具备 database.read 和 sql.analysis",
			"required_capabilities":["database.read","sql.analysis"],
			"matched_capabilities":["database.read","sql.analysis"],
			"missing_capabilities":[],
			"permission_requirements":["database.read:dev_database"],
			"tool_requirements":["mcp:postgres.readonly"],
			"runtime_requirements":["provider:codex"],
			"verification_requirements":["只读查询成功"],
			"selection_score":100,
			"expected_outputs":["execution_summary"],
			"input_requirements":{},
			"handoff_contract":{},
			"blocked_by_keys":[],
			"risk_level":"medium",
			"task_kind":"database_analysis"
		}],
		"budget_estimate":{},
		"template_key":"database_analysis",
		"planner_metadata":{}
	}`, employeeID.String())

	plan, err := decodePlannerJSON(content)

	require.NoError(t, err)
	task := plan.Tasks[0]
	require.Equal(t, "具备 database.read 和 sql.analysis", task.EmployeeSelectionReason)
	require.Equal(t, []string{"database.read", "sql.analysis"}, task.RequiredCapabilities)
	require.Equal(t, []string{"database.read", "sql.analysis"}, task.MatchedCapabilities)
	require.Empty(t, task.MissingCapabilities)
	require.Equal(t, []string{"database.read:dev_database"}, task.PermissionRequirements)
	require.Equal(t, []string{"mcp:postgres.readonly"}, task.ToolRequirements)
	require.Equal(t, []string{"provider:codex"}, task.RuntimeRequirements)
	require.Equal(t, []string{"只读查询成功"}, task.VerificationRequirements)
	require.Equal(t, 100, task.SelectionScore)
}
```

- [ ] **Step 2: Run planner tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestOpenAICompatiblePlannerPromptIncludesPlanningProfiles|TestOpenAICompatiblePlannerDecodesSelectionEvidence' -count=1
```

Expected: FAIL because prompt and decoded task fields are missing.

- [ ] **Step 3: Add task selection fields**

Modify `apps/control-plane/internal/workflow/projectcoordination/planner.go` `PlannedTask`:

```go
type PlannedTask struct {
	Key                     string
	Title                   string
	Summary                 string
	SelectedEmployeeID      uuid.UUID
	EmployeeSelectionReason string
	RequiredCapabilities    []string
	MatchedCapabilities     []string
	MissingCapabilities     []string
	PermissionRequirements  []string
	ToolRequirements        []string
	RuntimeRequirements     []string
	VerificationRequirements []string
	SelectionScore          int
	TaskKind                string
	StageIndex              *int32
	RiskLevel               string
	RequiresHumanApproval   bool
	ExpectedOutputs         []string
	InputRequirements       map[string]any
	HandoffContract         map[string]any
	BlockedByKeys           []string
}
```

Run `gofmt` after adding the field; the long field name alignment should be handled by the formatter.

- [ ] **Step 4: Update planner prompt and JSON decoder**

Modify `buildPlannerSystemPrompt()` in `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go` so the schema lines read:

```go
"Each task JSON object must include key, title, summary, selected_employee_id as a UUID string, employee_selection_reason, required_capabilities, matched_capabilities, missing_capabilities, permission_requirements, tool_requirements, runtime_requirements, verification_requirements, selection_score, expected_outputs, input_requirements, handoff_contract, blocked_by_keys, risk_level, and task_kind.",
"For every task, choose selected_employee_id by comparing planning_profile facts; explain the choice in employee_selection_reason and copy the required, matched, and missing capability arrays.",
"A task with missing_capabilities must set requires_human_approval or make the whole route requires_human_review true.",
```

Modify `plannerTask` and `plannerTaskJSON`:

```go
type plannerTask struct {
	Key                      string         `json:"key"`
	Title                    string         `json:"title"`
	Summary                  string         `json:"summary"`
	SelectedEmployeeID       uuid.UUID      `json:"selected_employee_id"`
	EmployeeSelectionReason  string         `json:"employee_selection_reason"`
	RequiredCapabilities     []string       `json:"required_capabilities"`
	MatchedCapabilities      []string       `json:"matched_capabilities"`
	MissingCapabilities      []string       `json:"missing_capabilities"`
	PermissionRequirements   []string       `json:"permission_requirements"`
	ToolRequirements         []string       `json:"tool_requirements"`
	RuntimeRequirements      []string       `json:"runtime_requirements"`
	VerificationRequirements []string       `json:"verification_requirements"`
	SelectionScore           int            `json:"selection_score"`
	TaskKind                 string         `json:"task_kind"`
	StageIndex               *int32         `json:"stage_index"`
	RiskLevel                string         `json:"risk_level"`
	RequiresHumanApproval    bool           `json:"requires_human_approval"`
	ExpectedOutputs          []string       `json:"expected_outputs"`
	InputRequirements        map[string]any `json:"input_requirements"`
	HandoffContract          map[string]any `json:"handoff_contract"`
	BlockedByKeys            []string       `json:"blocked_by_keys"`
}
```

Inside `decodePlannerJSON`, copy those fields into `PlannedTask`:

```go
EmployeeSelectionReason:  task.EmployeeSelectionReason,
RequiredCapabilities:     nonNilStrings(task.RequiredCapabilities),
MatchedCapabilities:      nonNilStrings(task.MatchedCapabilities),
MissingCapabilities:      nonNilStrings(task.MissingCapabilities),
PermissionRequirements:   nonNilStrings(task.PermissionRequirements),
ToolRequirements:         nonNilStrings(task.ToolRequirements),
RuntimeRequirements:      nonNilStrings(task.RuntimeRequirements),
VerificationRequirements: nonNilStrings(task.VerificationRequirements),
SelectionScore:           task.SelectionScore,
```

Inside `plannerTask.UnmarshalJSON`, decode each new array with `decodePlannerStringArray` and assign the fields.

- [ ] **Step 5: Update test-only heuristic planner**

Modify both heuristic task constructors in `apps/control-plane/internal/workflow/projectcoordination/heuristic_planner_test.go` to set:

```go
EmployeeSelectionReason:  "测试 planner 选择 active executor",
RequiredCapabilities:     []string{"execution"},
MatchedCapabilities:      []string{"execution"},
PermissionRequirements:   []string{},
ToolRequirements:         []string{},
RuntimeRequirements:      []string{},
VerificationRequirements: []string{"写回 project task attempt 结果"},
SelectionScore:           80,
```

- [ ] **Step 6: Run planner tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestOpenAICompatiblePlannerPromptIncludesPlanningProfiles|TestOpenAICompatiblePlannerDecodesSelectionEvidence|TestPlanDemandRouteSelectsOnlyActiveExecutorPoolMembers' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 3**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/planner.go apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go apps/control-plane/internal/workflow/projectcoordination/heuristic_planner_test.go
git commit -m "feat: require planner selection evidence"
```

### Task 4: Profile-Aware Validation And Metadata Persistence

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Write profile-aware validation tests**

Append to `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`:

```go
func TestValidateRouteDecisionPlanRejectsMissingSelectionReason(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].TaskKind = "database_analysis"
	plan.Tasks[0].RequiredCapabilities = []string{"database.read"}
	plan.Tasks[0].MatchedCapabilities = []string{"database.read"}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateRouteDecisionPlanRejectsHardMissingCapabilityWithoutReview(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].TaskKind = "database_analysis"
	plan.Tasks[0].EmployeeSelectionReason = "选择员工"
	plan.Tasks[0].RequiredCapabilities = []string{"database.write"}
	plan.Tasks[0].MissingCapabilities = []string{"database.write"}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateRouteDecisionPlanAllowsMissingCapabilityWithHumanReview(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validGraphPlan(employeeID)
	plan.RequiresHumanReview = true
	plan.Tasks[0].RequiresHumanApproval = true
	plan.Tasks[0].TaskKind = "database_analysis"
	plan.Tasks[0].EmployeeSelectionReason = "缺 database.write，等待人工确认"
	plan.Tasks[0].RequiredCapabilities = []string{"database.write"}
	plan.Tasks[0].MissingCapabilities = []string{"database.write"}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.NoError(t, err)
}

func validationSnapshotWithProfile(employeeID uuid.UUID) CoordinationSnapshot {
	return CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand: DemandSnapshot{ID: uuid.New(), Title: "分析数据库"},
		DigitalEmployeePool: []ProjectMemberSnapshot{{
			PrincipalID: employeeID,
			ProjectRole: "executor",
			Status: "active",
			PlanningProfile: &DigitalEmployeePlanningProfile{
				DigitalEmployeeID: employeeID,
				RoleProfile: PlanningRoleProfile{PrimaryRole: "data_analyst"},
				Capabilities: []PlanningCapability{{Key: "database.read", Level: "strong", Source: "test", Confidence: 1}},
				Skills: []PlanningSkill{{Key: "sql.analysis", Source: "test"}},
				RuntimeRequirements: PlanningRuntimeRequirements{ProviderTypes: []string{"codex"}, ProviderStatus: "ready"},
				Permissions: []PlanningPermission{{Scope: "database.read", Resource: "dev_database", Status: "granted"}},
				LoadState: PlanningLoadState{AvailableSlots: 1, Lendable: true},
				ProfileFreshness: PlanningProfileFreshness{SourceState: "ready"},
			},
		}},
	}
}
```

- [ ] **Step 2: Run validation tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestValidateRouteDecisionPlanRejectsMissingSelectionReason|TestValidateRouteDecisionPlanRejectsHardMissingCapabilityWithoutReview|TestValidateRouteDecisionPlanAllowsMissingCapabilityWithHumanReview' -count=1
```

Expected: FAIL with undefined `ValidateRouteDecisionPlan`.

- [ ] **Step 3: Implement profile-aware validation**

Add to `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`:

```go
func ValidateRouteDecisionPlan(snapshot CoordinationSnapshot, plan RouteDecisionPlan, policy GraphValidationPolicy) error {
	poolIDs := activeExecutorIDs(snapshot.DigitalEmployeePool)
	if err := ValidateRouteDecisionGraph(plan, poolIDs, policy); err != nil {
		return err
	}
	profiles := planningProfilesByEmployeeID(snapshot.DigitalEmployeePool)
	for _, task := range plan.Tasks {
		if strings.TrimSpace(task.EmployeeSelectionReason) == "" {
			return ErrInvalidRouteDecision
		}
		profile, ok := profiles[task.SelectedEmployeeID]
		if !ok || profile.DigitalEmployeeID == uuid.Nil {
			return ErrInvalidRouteDecision
		}
		score := ScorePlanningProfile(profile, PlanningTaskRequirements{
			TaskType:               task.TaskKind,
			RequiredCapabilities:   task.RequiredCapabilities,
			PermissionRequirements: task.PermissionRequirements,
			ToolRequirements:       task.ToolRequirements,
			RuntimeRequirements:    task.RuntimeRequirements,
		})
		if len(score.HardFailures) > 0 && !plan.RequiresHumanReview && !task.RequiresHumanApproval {
			return ErrInvalidRouteDecision
		}
		for _, missing := range task.MissingCapabilities {
			if strings.TrimSpace(missing) == "" || missing != strings.TrimSpace(missing) {
				return ErrInvalidRouteDecision
			}
		}
		for _, required := range task.RequiredCapabilities {
			if strings.TrimSpace(required) == "" || required != strings.TrimSpace(required) {
				return ErrInvalidRouteDecision
			}
		}
	}
	return nil
}

func planningProfilesByEmployeeID(members []ProjectMemberSnapshot) map[uuid.UUID]DigitalEmployeePlanningProfile {
	out := make(map[uuid.UUID]DigitalEmployeePlanningProfile, len(members))
	for _, member := range members {
		if member.PlanningProfile != nil {
			out[member.PrincipalID] = *member.PlanningProfile
		}
	}
	return out
}
```

Modify `OpenAICompatibleRoutePlanner.Plan` to use:

```go
if err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}); err != nil {
```

and the repair branch:

```go
if repairErr := ValidateRouteDecisionPlan(snapshot, repaired, GraphValidationPolicy{MaxTasks: 12}); repairErr == nil {
```

- [ ] **Step 4: Run validation tests**

Before running the full openai planner test set, update every planner JSON fixture in `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go` that represents a valid task to include this selection-evidence fragment inside each task object:

```json
"employee_selection_reason":"具备 execution 能力",
"required_capabilities":["execution"],
"matched_capabilities":["execution"],
"missing_capabilities":[],
"permission_requirements":[],
"tool_requirements":[],
"runtime_requirements":[],
"verification_requirements":["写回 project task attempt 结果"],
"selection_score":80,
```

For fixtures whose selected employee has a database-analysis profile, use this fragment instead:

```json
"employee_selection_reason":"具备 database.read 和 sql.analysis",
"required_capabilities":["database.read","sql.analysis"],
"matched_capabilities":["database.read","sql.analysis"],
"missing_capabilities":[],
"permission_requirements":["database.read:dev_database"],
"tool_requirements":["mcp:postgres.readonly"],
"runtime_requirements":["provider:codex"],
"verification_requirements":["只读查询成功"],
"selection_score":100,
```

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestValidateRouteDecisionPlan|TestOpenAICompatiblePlanner' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write metadata persistence tests**

Add assertions to `TestProjectStoreCreateProjectTasksCreatesOneTaskPerPlannedTaskWithGraphMetadata` in `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`:

```go
selection, ok := firstTask.PlannerMetadata["employee_selection"].(map[string]any)
require.True(t, ok, "expected employee_selection metadata, got %#v", firstTask.PlannerMetadata)
require.Equal(t, employeeID.String(), selection["selected_employee_id"])
require.Equal(t, "具备 execution 能力", selection["employee_selection_reason"])
require.Equal(t, []any{"execution"}, selection["required_capabilities"])
require.Equal(t, "profile-hash-for-test", selection["profile_snapshot_hash"])
```

Update the planned task in that test fixture to include:

```go
EmployeeSelectionReason:  "具备 execution 能力",
RequiredCapabilities:     []string{"execution"},
MatchedCapabilities:      []string{"execution"},
SelectionScore:           80,
VerificationRequirements: []string{"写回 project task attempt 结果"},
PlanningProfileSnapshotHash: "profile-hash-for-test",
```

- [ ] **Step 6: Run metadata test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestProjectStoreCreateProjectTasksCreatesOneTaskPerPlannedTaskWithGraphMetadata -count=1
```

Expected: FAIL because `employee_selection` metadata is absent.

- [ ] **Step 7: Persist selection evidence into task metadata and route aggregates**

Add this helper to `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go`:

```go
func PlanningSelectionMetadata(task PlannedTask) map[string]any {
	return map[string]any{
		"selected_employee_id":        task.SelectedEmployeeID.String(),
		"employee_selection_reason":   task.EmployeeSelectionReason,
		"profile_snapshot_hash":       task.PlanningProfileSnapshotHash,
		"selection_score":             task.SelectionScore,
		"required_capabilities":       stringsToAny(task.RequiredCapabilities),
		"matched_capabilities":        stringsToAny(task.MatchedCapabilities),
		"missing_capabilities":        stringsToAny(task.MissingCapabilities),
		"permission_requirements":     stringsToAny(task.PermissionRequirements),
		"tool_requirements":           stringsToAny(task.ToolRequirements),
		"runtime_requirements":        stringsToAny(task.RuntimeRequirements),
		"verification_requirements":   stringsToAny(task.VerificationRequirements),
	}
}
```

Modify `CreateProjectTasks` in `apps/control-plane/internal/workflow/projectcoordination/project_store.go` before appending `ProjectTaskGraphCreateTask`:

```go
if plannerMetadata == nil {
	plannerMetadata = map[string]any{}
}
if plannedTask.PlanningProfileSnapshotHash != "" || plannedTask.EmployeeSelectionReason != "" {
	plannerMetadata["employee_selection"] = PlanningSelectionMetadata(plannedTask)
} else {
	plannerMetadata["employee_selection"] = map[string]any{
		"selected_employee_id":      plannedTask.SelectedEmployeeID.String(),
		"employee_selection_reason": "",
		"selection_score":           0,
		"required_capabilities":     []any{},
		"matched_capabilities":      []any{},
		"missing_capabilities":      []any{},
	}
}
```

Because `CreateProjectTasksInput` does not carry the snapshot, add `PlanningProfileSnapshotHash` directly to `PlannedTask` if the selected profile is available during validation. Add this field to `PlannedTask`:

```go
PlanningProfileSnapshotHash string
```

In `ValidateRouteDecisionPlan`, when profile validation succeeds, set the hash by normalizing plan tasks through a new helper:

```go
func ApplyPlanningProfileScores(snapshot CoordinationSnapshot, plan *RouteDecisionPlan) {
	profiles := planningProfilesByEmployeeID(snapshot.DigitalEmployeePool)
	for index := range plan.Tasks {
		task := &plan.Tasks[index]
		profile := profiles[task.SelectedEmployeeID]
		score := ScorePlanningProfile(profile, PlanningTaskRequirements{
			TaskType:               task.TaskKind,
			RequiredCapabilities:   task.RequiredCapabilities,
			PermissionRequirements: task.PermissionRequirements,
			ToolRequirements:       task.ToolRequirements,
			RuntimeRequirements:    task.RuntimeRequirements,
		})
		task.SelectionScore = score.Score
		task.MatchedCapabilities = score.MatchedCapabilities
		if len(task.MissingCapabilities) == 0 {
			task.MissingCapabilities = score.MissingCapabilities
		}
		task.PlanningProfileSnapshotHash = PlanningProfileSnapshotHash(profile)
	}
}
```

Call it in `OpenAICompatibleRoutePlanner.Plan` after validation and before `return plan, nil`:

```go
ApplyPlanningProfileScores(snapshot, &plan)
return plan, nil
```

Modify `aggregateTaskInputSummary` to include:

```go
if task.EmployeeSelectionReason != "" {
	summary["employee_selection_reason"] = task.EmployeeSelectionReason
}
if len(task.RequiredCapabilities) > 0 {
	summary["required_capabilities"] = stringsToAny(task.RequiredCapabilities)
}
if len(task.MatchedCapabilities) > 0 {
	summary["matched_capabilities"] = stringsToAny(task.MatchedCapabilities)
}
if len(task.MissingCapabilities) > 0 {
	summary["missing_capabilities"] = stringsToAny(task.MissingCapabilities)
}
```

- [ ] **Step 8: Run validation and persistence tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestValidateRouteDecisionPlan|TestProjectStoreCreateProjectTasksCreatesOneTaskPerPlannedTaskWithGraphMetadata|TestProjectStorePersistRouteDecisionAggregatesGraphFields' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit Task 4**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/graph_validation.go apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go apps/control-plane/internal/workflow/projectcoordination/planner.go apps/control-plane/internal/workflow/projectcoordination/planning_profile.go
git commit -m "feat: validate planner employee selection evidence"
```

### Task 5: Phase 1 Integration Verification And Scope Guard

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add a database-analysis route integration test**

Add to `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go`:

```go
func TestOpenAICompatiblePlannerDatabaseAnalysisRequiresDatabaseProfile(t *testing.T) {
	dbEmployeeID := uuid.New()
	genericEmployeeID := uuid.New()
	client := &recordingChatCompletionClient{
		response: fmt.Sprintf(`{
			"reason":"数据库分析需要具备 database.read 的员工",
			"requires_human_review":false,
			"tasks":[{
				"key":"analyze-db",
				"title":"分析数据库异常",
				"summary":"检查慢查询和异常状态",
				"selected_employee_id":%q,
				"employee_selection_reason":"具备 database.read、sql.analysis 和 postgres.readonly",
				"required_capabilities":["database.read","sql.analysis"],
				"matched_capabilities":["database.read","sql.analysis"],
				"missing_capabilities":[],
				"permission_requirements":["database.read:dev_database"],
				"tool_requirements":["mcp:postgres.readonly"],
				"runtime_requirements":["provider:codex"],
				"verification_requirements":["只读查询成功","结果包含证据引用"],
				"selection_score":100,
				"expected_outputs":["execution_summary","evidence_refs"],
				"input_requirements":{"scope":"database_analysis"},
				"handoff_contract":{"completion_path":"project_task_attempt_writeback"},
				"blocked_by_keys":[],
				"risk_level":"medium",
				"task_kind":"database_analysis"
			}],
			"budget_estimate":{"mode":"planner"},
			"template_key":"database_analysis",
			"planner_metadata":{"provider":"openai-compatible"}
		}`, dbEmployeeID.String()),
	}
	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{APIKey: "test", BaseURL: "https://planner.example", Model: "planner-model"}, client)

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand: DemandSnapshot{ID: uuid.New(), Title: "分析数据库异常", Content: "找出订单状态异常原因"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			databaseAnalysisMemberSnapshot(dbEmployeeID),
			genericExecutionMemberSnapshot(genericEmployeeID),
		},
	})

	require.NoError(t, err)
	require.Len(t, plan.Tasks, 1)
	require.Equal(t, dbEmployeeID, plan.Tasks[0].SelectedEmployeeID)
	require.Equal(t, []string{"database.read", "sql.analysis"}, plan.Tasks[0].MatchedCapabilities)
	require.Empty(t, plan.Tasks[0].MissingCapabilities)
	require.NotEmpty(t, plan.Tasks[0].PlanningProfileSnapshotHash)
}

func databaseAnalysisMemberSnapshot(employeeID uuid.UUID) ProjectMemberSnapshot {
	return ProjectMemberSnapshot{
		PrincipalID: employeeID,
		ProjectRole: "executor",
		Status: "active",
		DisplayName: "数据库员工",
		PlanningProfile: &DigitalEmployeePlanningProfile{
			DigitalEmployeeID: employeeID,
			RoleProfile: PlanningRoleProfile{PrimaryRole: "data_analyst"},
			Capabilities: []PlanningCapability{{Key: "database.read", Level: "strong", Source: "test", Confidence: 1}},
			Skills: []PlanningSkill{{Key: "sql.analysis", Source: "test"}},
			ToolBindings: []PlanningToolBinding{{Type: "mcp", Key: "postgres.readonly", Status: "available"}},
			RuntimeRequirements: PlanningRuntimeRequirements{ProviderTypes: []string{"codex"}, ProviderStatus: "ready"},
			Permissions: []PlanningPermission{{Scope: "database.read", Resource: "dev_database", Status: "granted"}},
			LoadState: PlanningLoadState{AvailableSlots: 1, Lendable: true},
			ProfileFreshness: PlanningProfileFreshness{SourceState: "ready"},
		},
	}
}

func genericExecutionMemberSnapshot(employeeID uuid.UUID) ProjectMemberSnapshot {
	return ProjectMemberSnapshot{
		PrincipalID: employeeID,
		ProjectRole: "executor",
		Status: "active",
		DisplayName: "通用执行员工",
		PlanningProfile: &DigitalEmployeePlanningProfile{
			DigitalEmployeeID: employeeID,
			RoleProfile: PlanningRoleProfile{PrimaryRole: "general_executor"},
			RuntimeRequirements: PlanningRuntimeRequirements{ProviderTypes: []string{"codex"}, ProviderStatus: "ready"},
			LoadState: PlanningLoadState{AvailableSlots: 1, Lendable: true},
			ProfileFreshness: PlanningProfileFreshness{SourceState: "ready"},
		},
	}
}
```

- [ ] **Step 2: Run integration test**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestOpenAICompatiblePlannerDatabaseAnalysisRequiresDatabaseProfile -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full projectcoordination package tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -count=1
```

Expected: PASS.

- [ ] **Step 4: Run app package adapter tests**

Run:

```bash
go test ./apps/control-plane/internal/app -run TestDigitalEmployeePlanningProfileAdapter -count=1
```

Expected: PASS.

- [ ] **Step 5: Run repository contract-adjacent tests that read planner metadata**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestCreateProjectTaskGraph|TestProjectHandlerListsRouteDecisionsAndResolvesDecision|TestProjectHandlerGetsOperationalDetail' -count=1
```

Expected: PASS. These tests prove existing ProjectTask and route-decision read models still tolerate enriched metadata without contract changes.

- [ ] **Step 6: Add CHANGELOG entry**

Get the timestamp and render the exact entry text:

```bash
ts=$(TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M')
printf -- "- %s: Control Plane project coordinator Phase 1 planning profile now injects digital-employee capability, runtime, permission, and selection evidence into planner snapshots and task metadata.\n" "$ts"
```

Add the printed line near the top of `CHANGELOG.md`.

- [ ] **Step 7: Run hygiene checks**

Run:

```bash
gofmt -w apps/control-plane/internal/workflow/projectcoordination/planner.go apps/control-plane/internal/workflow/projectcoordination/planning_profile.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go apps/control-plane/internal/workflow/projectcoordination/graph_validation.go apps/control-plane/internal/workflow/projectcoordination/*_test.go apps/control-plane/internal/app/planning_profile_adapter.go apps/control-plane/internal/app/planning_profile_adapter_test.go apps/control-plane/internal/app/app.go
go test ./apps/control-plane/internal/workflow/projectcoordination -count=1
go test ./apps/control-plane/internal/app -run TestDigitalEmployeePlanningProfileAdapter -count=1
go test ./apps/control-plane/internal/project -run 'TestCreateProjectTaskGraph|TestProjectHandlerListsRouteDecisionsAndResolvesDecision|TestProjectHandlerGetsOperationalDetail' -count=1
git diff --check
```

Expected: all tests PASS and `git diff --check` prints no errors.

- [ ] **Step 8: Confirm scope guard**

Run:

```bash
git status --short
git diff --name-only
```

Expected changed paths are limited to:

```text
CHANGELOG.md
apps/control-plane/internal/app/app.go
apps/control-plane/internal/app/planning_profile_adapter.go
apps/control-plane/internal/app/planning_profile_adapter_test.go
apps/control-plane/internal/workflow/projectcoordination/graph_validation.go
apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go
apps/control-plane/internal/workflow/projectcoordination/heuristic_planner_test.go
apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go
apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go
apps/control-plane/internal/workflow/projectcoordination/planner.go
apps/control-plane/internal/workflow/projectcoordination/planning_profile.go
apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go
apps/control-plane/internal/workflow/projectcoordination/project_store.go
apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
```

- [ ] **Step 9: Commit Task 5**

```bash
git add CHANGELOG.md apps/control-plane/internal/app/app.go apps/control-plane/internal/app/planning_profile_adapter.go apps/control-plane/internal/app/planning_profile_adapter_test.go apps/control-plane/internal/workflow/projectcoordination/graph_validation.go apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go apps/control-plane/internal/workflow/projectcoordination/heuristic_planner_test.go apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner_test.go apps/control-plane/internal/workflow/projectcoordination/planner.go apps/control-plane/internal/workflow/projectcoordination/planning_profile.go apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "feat: complete planning profile phase one"
```

## Completion Gate

Before claiming Phase 1 implementation complete, run the project completion skill and report one of these shapes:

```text
局部验证：go test ./apps/control-plane/internal/workflow/projectcoordination -count=1；go test ./apps/control-plane/internal/app -run TestDigitalEmployeePlanningProfileAdapter -count=1；go test ./apps/control-plane/internal/project -run 'TestCreateProjectTaskGraph|TestProjectHandlerListsRouteDecisionsAndResolvesDecision|TestProjectHandlerGetsOperationalDetail' -count=1；未做真实链路验证，因为 Phase 1 只改变 planner snapshot/validation/metadata，未启动 Runtime/Provider 执行链路。
```

If the implementation is paired with a running Temporal/Control Plane stack and a real planner API key, add a real smoke:

```bash
scripts/dev-services.sh status
```

Then submit a real demand and verify the generated route decision contains `employee_selection_reason`, `required_capabilities`, `matched_capabilities`, `profile_snapshot_hash`, and ProjectTask `planner_metadata.employee_selection`.

Do not claim the full dynamic planning loop is usable after Phase 1. Phase 1 only proves explainable employee selection and validation.
