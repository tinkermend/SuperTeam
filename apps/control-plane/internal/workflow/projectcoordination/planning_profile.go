package projectcoordination

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

type DigitalEmployeePlanningProfile struct {
	DigitalEmployeeID   uuid.UUID                   `json:"digital_employee_id"`
	DisplayName         string                      `json:"display_name,omitempty"`
	RoleProfile         PlanningRoleProfile         `json:"employee_profile"`
	Capabilities        []PlanningCapability        `json:"capabilities,omitempty"`
	Skills              []PlanningSkill             `json:"skills,omitempty"`
	ToolBindings        []PlanningToolBinding       `json:"tool_bindings,omitempty"`
	RuntimeRequirements PlanningRuntimeRequirements `json:"runtime_requirements"`
	Permissions         []PlanningPermission        `json:"permissions,omitempty"`
	LoadState           PlanningLoadState           `json:"load_state"`
	ReliabilitySignals  PlanningReliabilitySignals  `json:"reliability_signals"`
	ProfileFreshness    PlanningProfileFreshness    `json:"profile_freshness"`
	SelectionWarnings   []string                    `json:"selection_warnings,omitempty"`
	HardFailures        []string                    `json:"hard_failures,omitempty"`
	GeneratedAt         time.Time                   `json:"generated_at,omitempty"`
}

type PlanningRoleProfile struct {
	PrimaryRole    string `json:"primary_role,omitempty"`
	Description    string `json:"description,omitempty"`
	EmployeeType   string `json:"employee_type,omitempty"`
	SourceRole     string `json:"source_role,omitempty"`
	PersonaSummary string `json:"persona_summary,omitempty"`
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
	ProviderTypes           []string `json:"provider_types,omitempty"`
	ProviderStatus          string   `json:"provider_status,omitempty"`
	RuntimeNodeID           string   `json:"runtime_node_id,omitempty"`
	DispatchReadinessStatus string   `json:"dispatch_readiness_status,omitempty"`
	DispatchBlockingReasons []string `json:"dispatch_blocking_reasons,omitempty"`
}

type PlanningPermission struct {
	Scope    string `json:"scope"`
	Resource string `json:"resource,omitempty"`
	Status   string `json:"status"`
}

type PlanningLoadState struct {
	AvailableSlots int  `json:"available_slots,omitempty"`
	InFlightTasks  int  `json:"in_flight_tasks,omitempty"`
	Lendable       bool `json:"lendable,omitempty"`
}

type PlanningReliabilitySignals struct {
	Status                 string  `json:"status,omitempty"`
	SuccessRate            float64 `json:"success_rate,omitempty"`
	RecentFailureCount     int     `json:"recent_failure_count,omitempty"`
	RecentSuccessCount     int     `json:"recent_success_count,omitempty"`
	RecentHumanRejectCount int     `json:"recent_human_reject_count,omitempty"`
}

type PlanningProfileFreshness struct {
	SourceState    string            `json:"source_state"`
	FetchedAt      time.Time         `json:"fetched_at,omitempty"`
	GeneratedAt    time.Time         `json:"generated_at,omitempty"`
	SourceVersions map[string]string `json:"source_versions,omitempty"`
}

type DigitalEmployeePlanningProfileSourceRecord struct {
	DigitalEmployeeID     uuid.UUID      `json:"digital_employee_id"`
	EmployeeType          string         `json:"employee_type,omitempty"`
	Role                  string         `json:"role,omitempty"`
	Description           string         `json:"description,omitempty"`
	PersonaMemoryMarkdown string         `json:"persona_memory_markdown,omitempty"`
	EmployeeStatus        string         `json:"employee_status,omitempty"`
	CapabilityBindings    map[string]any `json:"capability_bindings,omitempty"`
	PermissionPolicy      map[string]any `json:"permission_policy,omitempty"`
	RuntimeNodeID         uuid.UUID      `json:"runtime_node_id,omitempty"`
	ProviderType          string         `json:"provider_type,omitempty"`
	ExecutionStatus       string         `json:"execution_status,omitempty"`
	LoadState             map[string]any `json:"load_state,omitempty"`
	ReliabilitySignals    map[string]any `json:"reliability_signals,omitempty"`
	FetchedAt             time.Time      `json:"fetched_at,omitempty"`
}

type PlanningTaskRequirements struct {
	TaskType               string   `json:"task_type,omitempty"`
	RequiredCapabilities   []string `json:"required_capabilities,omitempty"`
	PermissionRequirements []string `json:"permission_requirements,omitempty"`
	ToolRequirements       []string `json:"tool_requirements,omitempty"`
	RuntimeRequirements    []string `json:"runtime_requirements,omitempty"`
}

type PlanningProfileScore struct {
	Score                      int      `json:"score"`
	MatchedCapabilities        []string `json:"matched_capabilities,omitempty"`
	MissingCapabilities        []string `json:"missing_capabilities,omitempty"`
	MatchedRuntimeRequirements []string `json:"matched_runtime_requirements,omitempty"`
	MissingRuntimeRequirements []string `json:"missing_runtime_requirements,omitempty"`
	// UnrecognizedRuntimeRequirements are requirement strings whose kind the
	// platform does not know how to evaluate. They are a planner syntax problem,
	// not a fact about the employee, so they never hard-fail and never dilute the
	// score. Display only.
	UnrecognizedRuntimeRequirements []string `json:"unrecognized_runtime_requirements,omitempty"`
	MatchedPermissions              []string `json:"matched_permissions,omitempty"`
	MissingPermissions              []string `json:"missing_permissions,omitempty"`
	MatchedTools                    []string `json:"matched_tools,omitempty"`
	MissingTools                    []string `json:"missing_tools,omitempty"`
	HardFailures                    []string `json:"hard_failures,omitempty"`
}

func BuildDigitalEmployeePlanningProfile(member project.ProjectMember, source DigitalEmployeePlanningProfileSourceRecord, runtimeReady bool) DigitalEmployeePlanningProfile {
	sourceMissing := planningProfileSourceMissing(source)
	generatedAt := time.Now().UTC()
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID:   member.PrincipalID,
		DisplayName:         displayNameFromMember(member),
		RoleProfile:         buildPlanningRoleProfile(member, source),
		Capabilities:        buildPlanningCapabilities(source.CapabilityBindings),
		Skills:              buildPlanningSkills(source.CapabilityBindings),
		ToolBindings:        buildPlanningToolBindings(source.CapabilityBindings),
		RuntimeRequirements: buildPlanningRuntimeRequirements(source, runtimeReady, sourceMissing),
		Permissions:         buildPlanningPermissions(source.PermissionPolicy),
		LoadState:           buildPlanningLoadState(source.LoadState),
		ReliabilitySignals:  buildPlanningReliabilitySignals(source.ReliabilitySignals),
		ProfileFreshness:    buildPlanningProfileFreshness(source, sourceMissing),
		GeneratedAt:         generatedAt,
	}
	profile.ProfileFreshness.GeneratedAt = generatedAt
	if source.DigitalEmployeeID != uuid.Nil {
		profile.DigitalEmployeeID = source.DigitalEmployeeID
	}
	if sourceMissing {
		profile.SelectionWarnings = append(profile.SelectionWarnings, "profile_source_missing")
	}
	return profile
}

func ScorePlanningProfile(profile DigitalEmployeePlanningProfile, req PlanningTaskRequirements) PlanningProfileScore {
	result := PlanningProfileScore{
		HardFailures: append([]string(nil), profile.HardFailures...),
	}

	result.Score += scoreCapabilities(profile, req, &result)
	result.Score += scoreRole(profile, req)
	result.Score += scoreRuntime(profile, req, &result)
	result.Score += scorePermissionsAndTools(profile, req, &result)
	result.Score += scoreLoad(profile)
	result.Score += scoreReliability(profile)

	if len(result.HardFailures) > 0 {
		result.Score = 0
		return result
	}
	if result.Score > 100 {
		result.Score = 100
	}
	if result.Score < 0 {
		result.Score = 0
	}
	return result
}

func PlanningProfileSnapshotHash(profile DigitalEmployeePlanningProfile) string {
	profile.GeneratedAt = time.Time{}
	profile.ProfileFreshness.GeneratedAt = time.Time{}
	payload, err := json.Marshal(profile)
	if err != nil {
		payload = []byte(fmt.Sprintf("%#v", profile))
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func PlanningSelectionMetadata(task PlannedTask) map[string]any {
	return map[string]any{
		"selected_employee_id":      task.SelectedEmployeeID.String(),
		"employee_selection_reason": task.EmployeeSelectionReason,
		"profile_snapshot_hash":     task.PlanningProfileSnapshotHash,
		"selection_score":           task.SelectionScore,
		"required_capabilities":     stringsToAny(task.RequiredCapabilities),
		"matched_capabilities":      stringsToAny(task.MatchedCapabilities),
		"missing_capabilities":      stringsToAny(task.MissingCapabilities),
		"permission_requirements":   stringsToAny(task.PermissionRequirements),
		"tool_requirements":         stringsToAny(task.ToolRequirements),
		"runtime_requirements":      stringsToAny(task.RuntimeRequirements),
		"verification_requirements": stringsToAny(task.VerificationRequirements),
	}
}

func hasPlanningSelectionEvidence(task PlannedTask) bool {
	return strings.TrimSpace(task.EmployeeSelectionReason) != "" ||
		task.PlanningProfileSnapshotHash != "" ||
		len(task.RequiredCapabilities) > 0 ||
		len(task.MatchedCapabilities) > 0 ||
		len(task.MissingCapabilities) > 0 ||
		len(task.PermissionRequirements) > 0 ||
		len(task.ToolRequirements) > 0 ||
		len(task.RuntimeRequirements) > 0 ||
		len(task.VerificationRequirements) > 0 ||
		task.SelectionScore > 0
}

func buildPlanningRoleProfile(member project.ProjectMember, source DigitalEmployeePlanningProfileSourceRecord) PlanningRoleProfile {
	roleProfile := PlanningRoleProfile{
		PrimaryRole:    normalizePlanningString(source.Role),
		Description:    strings.TrimSpace(source.Description),
		EmployeeType:   normalizePlanningString(source.EmployeeType),
		SourceRole:     strings.TrimSpace(source.Role),
		PersonaSummary: firstNonEmptyLine(source.PersonaMemoryMarkdown),
	}
	if roleProfile.PrimaryRole == "" {
		roleProfile.PrimaryRole = normalizePlanningString(stringFromMap(member.Settings, "planning_role"))
	}
	if roleProfile.PrimaryRole == "" {
		roleProfile.PrimaryRole = normalizePlanningString(source.EmployeeType)
	}
	if roleProfile.PrimaryRole == "" {
		roleProfile.PrimaryRole = normalizePlanningString(source.Role)
	}
	return roleProfile
}

func buildPlanningCapabilities(capabilityBindings map[string]any) []PlanningCapability {
	keys := stringSliceFromMap(capabilityBindings, "external_capabilities")
	capabilities := make([]PlanningCapability, 0, len(keys))
	for _, key := range keys {
		capabilities = append(capabilities, PlanningCapability{
			Key:        key,
			Level:      "strong",
			Source:     "capability_bindings.external_capabilities",
			Confidence: 0.9,
		})
	}
	return capabilities
}

func buildPlanningSkills(capabilityBindings map[string]any) []PlanningSkill {
	keys := stringSliceFromMap(capabilityBindings, "skills")
	skills := make([]PlanningSkill, 0, len(keys))
	for _, key := range keys {
		skills = append(skills, PlanningSkill{
			Key:    key,
			Source: "capability_bindings.skills",
		})
	}
	return skills
}

func buildPlanningToolBindings(capabilityBindings map[string]any) []PlanningToolBinding {
	keys := stringSliceFromMap(capabilityBindings, "mcp_servers")
	tools := make([]PlanningToolBinding, 0, len(keys))
	for _, key := range keys {
		tools = append(tools, PlanningToolBinding{
			Type:   "mcp",
			Key:    key,
			Status: "available",
		})
	}
	return tools
}

func buildPlanningRuntimeRequirements(source DigitalEmployeePlanningProfileSourceRecord, runtimeReady bool, sourceMissing bool) PlanningRuntimeRequirements {
	requirements := PlanningRuntimeRequirements{}
	if !runtimeReady {
		requirements.DispatchReadinessStatus = "not_ready"
		requirements.DispatchBlockingReasons = []string{"runtime_not_ready"}
	}
	requirements.ProviderTypes = appendUniqueString(requirements.ProviderTypes, normalizePlanningString(source.ProviderType))
	if source.RuntimeNodeID != uuid.Nil {
		requirements.RuntimeNodeID = source.RuntimeNodeID.String()
	}
	if sourceMissing {
		requirements.ProviderStatus = "unknown"
		return requirements
	}
	executionStatus := normalizePlanningString(source.ExecutionStatus)
	if executionStatus == "ready" || executionStatus == "active" {
		requirements.ProviderStatus = executionStatus
		return requirements
	}
	if !runtimeReady {
		requirements.ProviderStatus = "unavailable"
		return requirements
	}
	if executionStatus != "" {
		requirements.ProviderStatus = executionStatus
		return requirements
	}
	requirements.ProviderStatus = "unknown"
	return requirements
}

func buildPlanningPermissions(permissionPolicy map[string]any) []PlanningPermission {
	grants := stringSliceFromMap(permissionPolicy, "grants")
	permissions := make([]PlanningPermission, 0, len(grants))
	for _, grant := range grants {
		scope, resource := splitRequirement(grant)
		if scope == "" {
			continue
		}
		permissions = append(permissions, PlanningPermission{
			Scope:    scope,
			Resource: resource,
			Status:   "granted",
		})
	}
	return permissions
}

func buildPlanningLoadState(loadState map[string]any) PlanningLoadState {
	inFlightTasks := intFromMap(loadState, "in_flight_tasks")
	if inFlightTasks == 0 {
		inFlightTasks = intFromMap(loadState, "running_tasks")
	}
	return PlanningLoadState{
		AvailableSlots: intFromMap(loadState, "available_slots"),
		InFlightTasks:  inFlightTasks,
		Lendable:       boolFromMap(loadState, "lendable"),
	}
}

func buildPlanningReliabilitySignals(reliabilitySignals map[string]any) PlanningReliabilitySignals {
	return PlanningReliabilitySignals{
		Status:                 normalizePlanningString(stringFromMap(reliabilitySignals, "status")),
		SuccessRate:            floatFromMap(reliabilitySignals, "success_rate"),
		RecentFailureCount:     intFromMap(reliabilitySignals, "recent_failure_count"),
		RecentSuccessCount:     intFromMap(reliabilitySignals, "recent_success_count"),
		RecentHumanRejectCount: intFromMap(reliabilitySignals, "recent_human_reject_count"),
	}
}

func buildPlanningProfileFreshness(source DigitalEmployeePlanningProfileSourceRecord, sourceMissing bool) PlanningProfileFreshness {
	if sourceMissing {
		return PlanningProfileFreshness{SourceState: "unknown"}
	}
	state := normalizePlanningString(source.ExecutionStatus)
	if state == "" {
		state = normalizePlanningString(source.EmployeeStatus)
	}
	if state == "" {
		state = "ready"
	}
	return PlanningProfileFreshness{
		SourceState:    state,
		FetchedAt:      source.FetchedAt,
		SourceVersions: buildPlanningSourceVersions(source),
	}
}

func buildPlanningSourceVersions(source DigitalEmployeePlanningProfileSourceRecord) map[string]string {
	return nil
}

// scoreCapabilities records the capability diff for display but contributes a
// constant to the score.
//
// 历史原因：两侧曾都是无注册表的自由文本，让该 diff 归零评分（经 HardFailures）
// 会因为一个杜撰的名字就废掉一个完全胜任的员工，故此处只记录不扣分。
// 见 2026-07-10 plan 阶段重构 spec §1.6。
//
// 现状已部分改变（2026-08-04）：能力词表 capability_vocabulary 已存在，模板
// required_capabilities 与员工 external_capabilities **两侧均已过词表校验**。
// 仍未对齐的最后一环是 planner 提示里不注入词表，required_capabilities 因此
// 仍由模型自行合成；在那一环补齐之前，这里维持"只记录不扣分"。
func scoreCapabilities(profile DigitalEmployeePlanningProfile, req PlanningTaskRequirements, result *PlanningProfileScore) int {
	available := map[string]struct{}{}
	for _, capability := range profile.Capabilities {
		key := normalizePlanningString(capability.Key)
		if key != "" {
			available[key] = struct{}{}
		}
	}
	for _, required := range req.RequiredCapabilities {
		key := normalizePlanningString(required)
		if key == "" {
			continue
		}
		if _, ok := available[key]; ok {
			result.MatchedCapabilities = append(result.MatchedCapabilities, key)
			continue
		}
		result.MissingCapabilities = append(result.MissingCapabilities, key)
	}
	return 40
}

func scoreRole(profile DigitalEmployeePlanningProfile, req PlanningTaskRequirements) int {
	primaryRole := normalizePlanningString(profile.RoleProfile.PrimaryRole)
	if primaryRole == "" {
		return 0
	}
	if req.TaskType == "" || planningRoleMatchesTask(primaryRole, req.TaskType) {
		return 20
	}
	return 10
}

func scoreRuntime(profile DigitalEmployeePlanningProfile, req PlanningTaskRequirements, result *PlanningProfileScore) int {
	if len(req.RuntimeRequirements) == 0 {
		if runtimeProviderReady(profile.RuntimeRequirements.ProviderStatus) {
			return 15
		}
		result.HardFailures = appendUniqueString(result.HardFailures, "runtime_requirement_unsatisfied")
		result.HardFailures = appendUniqueString(result.HardFailures, "unsatisfied_runtime:provider_status")
		return 0
	}
	for _, requirement := range req.RuntimeRequirements {
		kind, value := splitRequirement(requirement)
		if kind == "" {
			continue
		}
		if !knownRuntimeRequirementKind(kind) {
			// splitRequirement("codex") yields kind="codex", value="". The planner
			// named the right thing in the wrong syntax; that is not evidence the
			// employee lacks anything.
			result.UnrecognizedRuntimeRequirements = append(result.UnrecognizedRuntimeRequirements, normalizeRequirement(requirement))
			continue
		}
		if runtimeRequirementSatisfied(profile.RuntimeRequirements, kind, value) {
			result.MatchedRuntimeRequirements = append(result.MatchedRuntimeRequirements, normalizeRequirement(requirement))
			continue
		}
		missing := normalizeRequirement(requirement)
		result.MissingRuntimeRequirements = append(result.MissingRuntimeRequirements, missing)
		result.HardFailures = appendUniqueString(result.HardFailures, "runtime_requirement_unsatisfied")
		result.HardFailures = append(result.HardFailures, "unsatisfied_runtime:"+missing)
	}
	if !runtimeProviderReady(profile.RuntimeRequirements.ProviderStatus) {
		result.HardFailures = appendUniqueString(result.HardFailures, "runtime_requirement_unsatisfied")
		result.HardFailures = appendUniqueString(result.HardFailures, "unsatisfied_runtime:provider_status")
	}
	return proportionalScore(15, len(result.MatchedRuntimeRequirements), len(result.MatchedRuntimeRequirements)+len(result.MissingRuntimeRequirements))
}

// scorePermissionsAndTools records the permission and tool diffs for display but
// never hard-fails on them. permission_policy has no consumer: employee CRUD
// writes it, the runtime never reads it, and the authz decision point never reads
// it. A digital employee's real boundary is the provider's own sandbox plus the
// action-level risk.approval gate. tool_requirements are likewise advisory — MCP
// is materialized into the workspace by the Runtime, not selected by matching
// planner-invented names. Turning either miss into a HardFailure zeroed
// SelectionScore and forced human approval for fields nothing honours. See the
// 2026-07-10 plan-phase refactor spec §1.6.
func scorePermissionsAndTools(profile DigitalEmployeePlanningProfile, req PlanningTaskRequirements, result *PlanningProfileScore) int {
	total := 0
	matched := 0
	for _, requirement := range req.PermissionRequirements {
		normalized := normalizeRequirement(requirement)
		if normalized == "" {
			continue
		}
		total++
		if permissionRequirementSatisfied(profile.Permissions, normalized) {
			matched++
			result.MatchedPermissions = append(result.MatchedPermissions, normalized)
			continue
		}
		result.MissingPermissions = append(result.MissingPermissions, normalized)
	}
	for _, requirement := range req.ToolRequirements {
		normalized := normalizeRequirement(requirement)
		if normalized == "" {
			continue
		}
		total++
		if toolRequirementSatisfied(profile.ToolBindings, normalized) {
			matched++
			result.MatchedTools = append(result.MatchedTools, normalized)
			continue
		}
		result.MissingTools = append(result.MissingTools, normalized)
	}
	if total == 0 {
		return 10
	}
	return proportionalScore(10, matched, total)
}

func scoreLoad(profile DigitalEmployeePlanningProfile) int {
	if profile.LoadState.Lendable || profile.LoadState.AvailableSlots > 0 {
		return 10
	}
	return 0
}

func scoreReliability(profile DigitalEmployeePlanningProfile) int {
	status := normalizePlanningString(profile.ReliabilitySignals.Status)
	if status == "blocked" || status == "unhealthy" || status == "failing" {
		return 0
	}
	if profile.ReliabilitySignals.SuccessRate > 0 && profile.ReliabilitySignals.SuccessRate < 0.5 {
		return 0
	}
	if profile.ReliabilitySignals.RecentFailureCount > 0 && profile.ReliabilitySignals.RecentSuccessCount == 0 {
		return 0
	}
	return 5
}

func planningProfileSourceMissing(source DigitalEmployeePlanningProfileSourceRecord) bool {
	return !planningProfileSourceHasFacts(source)
}

func planningProfileSourceHasFacts(source DigitalEmployeePlanningProfileSourceRecord) bool {
	if normalizePlanningString(source.EmployeeType) != "" ||
		normalizePlanningString(source.Role) != "" ||
		strings.TrimSpace(source.Description) != "" ||
		firstNonEmptyLine(source.PersonaMemoryMarkdown) != "" ||
		normalizePlanningString(source.EmployeeStatus) != "" ||
		normalizePlanningString(source.ProviderType) != "" ||
		normalizePlanningString(source.ExecutionStatus) != "" ||
		source.RuntimeNodeID != uuid.Nil {
		return true
	}
	if len(buildPlanningCapabilities(source.CapabilityBindings)) > 0 ||
		len(buildPlanningSkills(source.CapabilityBindings)) > 0 ||
		len(buildPlanningToolBindings(source.CapabilityBindings)) > 0 {
		return true
	}
	if len(buildPlanningPermissions(source.PermissionPolicy)) > 0 {
		return true
	}
	loadState := buildPlanningLoadState(source.LoadState)
	if loadState.AvailableSlots != 0 || loadState.InFlightTasks != 0 || loadState.Lendable {
		return true
	}
	reliabilitySignals := buildPlanningReliabilitySignals(source.ReliabilitySignals)
	return reliabilitySignals.Status != "" ||
		reliabilitySignals.SuccessRate != 0 ||
		reliabilitySignals.RecentFailureCount != 0 ||
		reliabilitySignals.RecentSuccessCount != 0 ||
		reliabilitySignals.RecentHumanRejectCount != 0
}

func displayNameFromMember(member project.ProjectMember) string {
	if member.DisplayNameSnapshot == nil {
		return ""
	}
	return strings.TrimSpace(*member.DisplayNameSnapshot)
}

func normalizePlanningString(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmptyLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	switch value := values[key].(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return ""
	}
}

func stringSliceFromMap(values map[string]any, key string) []string {
	if values == nil {
		return nil
	}
	var raw []string
	switch typed := values[key].(type) {
	case []string:
		raw = typed
	case []any:
		raw = make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				raw = append(raw, value)
			}
		}
	case string:
		raw = []string{typed}
	default:
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, value := range raw {
		result = appendUniqueString(result, normalizePlanningString(value))
	}
	return result
}

func intFromMap(values map[string]any, key string) int {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	default:
		return 0
	}
}

func floatFromMap(values map[string]any, key string) float64 {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		parsed, _ := value.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return parsed
	default:
		return 0
	}
}

func boolFromMap(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	switch value := values[key].(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(value))
		return parsed
	default:
		return false
	}
}

func appendUniqueString(values []string, value string) []string {
	value = normalizePlanningString(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func splitRequirement(requirement string) (string, string) {
	parts := strings.SplitN(normalizeRequirement(requirement), ":", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func normalizeRequirement(requirement string) string {
	return normalizePlanningString(requirement)
}

func proportionalScore(weight int, matched int, total int) int {
	if total == 0 {
		return weight
	}
	return weight * matched / total
}

func planningRoleMatchesTask(primaryRole string, taskType string) bool {
	role := normalizePlanningString(primaryRole)
	task := normalizePlanningString(taskType)
	if role == "" || task == "" {
		return false
	}
	if role == task || strings.Contains(task, role) || strings.Contains(role, task) {
		return true
	}
	roleTokens := tokenSet(role)
	taskTokens := tokenSet(task)
	for roleToken := range roleTokens {
		if _, ok := taskTokens[roleToken]; ok {
			return true
		}
		if roleToken == "data" {
			if _, ok := taskTokens["database"]; ok {
				return true
			}
		}
		if roleToken == "analyst" {
			if _, ok := taskTokens["analysis"]; ok {
				return true
			}
		}
	}
	return false
}

func tokenSet(value string) map[string]struct{} {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/' || r == ' '
	})
	tokens := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		token := normalizePlanningString(field)
		if token != "" {
			tokens[token] = struct{}{}
		}
	}
	return tokens
}

// knownRuntimeRequirementKind reports whether the platform can evaluate this
// requirement kind at all. It must stay in sync with runtimeRequirementSatisfied.
func knownRuntimeRequirementKind(kind string) bool {
	switch kind {
	case "provider", "runtime_node":
		return true
	default:
		return false
	}
}

func runtimeRequirementSatisfied(runtime PlanningRuntimeRequirements, kind string, value string) bool {
	switch kind {
	case "provider":
		return runtimeProviderReady(runtime.ProviderStatus) && containsString(runtime.ProviderTypes, value)
	case "runtime_node":
		return runtime.RuntimeNodeID != "" && normalizePlanningString(runtime.RuntimeNodeID) == value
	default:
		return false
	}
}

func runtimeProviderReady(providerStatus string) bool {
	status := normalizePlanningString(providerStatus)
	return status == "" || status == "ready" || status == "active" || status == "available"
}

func permissionRequirementSatisfied(permissions []PlanningPermission, requirement string) bool {
	requiredScope, requiredResource := splitRequirement(requirement)
	for _, permission := range permissions {
		if normalizePlanningString(permission.Status) != "granted" {
			continue
		}
		if normalizePlanningString(permission.Scope) != requiredScope {
			continue
		}
		if requiredResource == "" || normalizePlanningString(permission.Resource) == requiredResource {
			return true
		}
	}
	return false
}

func toolRequirementSatisfied(tools []PlanningToolBinding, requirement string) bool {
	requiredType, requiredKey := splitRequirement(requirement)
	for _, tool := range tools {
		if normalizePlanningString(tool.Status) != "available" {
			continue
		}
		if normalizePlanningString(tool.Type) != requiredType {
			continue
		}
		if requiredKey == "" || normalizePlanningString(tool.Key) == requiredKey {
			return true
		}
	}
	return false
}

func containsString(values []string, value string) bool {
	normalized := normalizePlanningString(value)
	for _, candidate := range values {
		if normalizePlanningString(candidate) == normalized {
			return true
		}
	}
	return false
}
