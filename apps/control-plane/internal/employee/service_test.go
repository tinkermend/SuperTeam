package employee

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	cpruntime "github.com/superteam/control-plane/internal/runtime"
)

func TestEmployeeTypeRegistryExcludesProjectCoordinator(t *testing.T) {
	types := DefaultEmployeeTypeDefinitions()
	if len(types) < 6 {
		t.Fatalf("expected professional engineer types, got %#v", types)
	}
	for _, item := range types {
		if strings.Contains(item.Type, "coordinator") || strings.Contains(item.Label, "协调") {
			t.Fatalf("project coordinator must not be a reusable employee type: %#v", item)
		}
	}
	if _, ok := EmployeeTypeDefinitionByType("database_admin"); !ok {
		t.Fatalf("expected database_admin type")
	}
	if _, ok := EmployeeTypeDefinitionByType("devops_engineer"); !ok {
		t.Fatalf("expected devops_engineer type")
	}
}

func TestEmployeeTypeRegistryReturnsClonedDefinitions(t *testing.T) {
	types := DefaultEmployeeTypeDefinitions()
	if len(types) == 0 {
		t.Fatalf("expected employee type definitions")
	}
	originalSkill := types[0].RecommendedSkills[0]
	types[0].RecommendedSkills[0] = "mutated-skill"
	enabledSkills, ok := types[0].DefaultCapabilitySelection["enabled_skills"].([]string)
	if !ok || len(enabledSkills) == 0 {
		t.Fatalf("expected enabled_skills default selection, got %#v", types[0].DefaultCapabilitySelection)
	}
	enabledSkills[0] = "mutated-enabled-skill"

	fresh := DefaultEmployeeTypeDefinitions()
	if fresh[0].RecommendedSkills[0] != originalSkill {
		t.Fatalf("expected recommended skills to be cloned, got %#v", fresh[0].RecommendedSkills)
	}
	freshEnabledSkills, ok := fresh[0].DefaultCapabilitySelection["enabled_skills"].([]string)
	if !ok || len(freshEnabledSkills) == 0 {
		t.Fatalf("expected fresh enabled_skills default selection, got %#v", fresh[0].DefaultCapabilitySelection)
	}
	if freshEnabledSkills[0] == "mutated-enabled-skill" {
		t.Fatalf("expected default capability selection to be cloned, got %#v", fresh[0].DefaultCapabilitySelection)
	}
}

func TestGetCreateOptionsReturnsTeamPolicyAndRuntimeCandidates(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	teamConfigID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.teamConfigs[teamConfigID] = TeamConfigInput{
		ID:             teamConfigID,
		TenantID:       tenantID,
		TeamID:         teamID,
		RevisionNumber: 4,
		Status:         TeamConfigRevisionStatusActive,
		CapabilityPolicy: map[string]any{
			"allowed_skills":         []any{"database-troubleshooting", "incident-diagnosis"},
			"allowed_mcp_servers":    []any{"postgres-readonly"},
			"allowed_provider_types": []any{"codex"},
			"allowed_employee_types": []any{"database_admin"},
		},
		ContextPolicy:  map[string]any{"sources": []any{"runbook", "monitoring"}},
		ApprovalPolicy: map[string]any{"min_risk_for_human": "high"},
	}
	repo.currentTeamConfigByTeam[teamID] = teamConfigID
	repo.runtimeProviderOptions = []RuntimeProviderOption{{
		RuntimeNodeID:         runtimeNodeID,
		NodeID:                "node-ops-01",
		RuntimeName:           "运维节点 01",
		ProviderType:          "codex",
		RuntimeStatus:         "online",
		ProviderStatus:        "healthy",
		HealthStatus:          "healthy",
		CurrentLoad:           1,
		MaxSlots:              4,
		AgentHomeDir:          "/srv/superteam/agents",
		AgentHomeDirAvailable: true,
		Available:             true,
		DisabledReason:        "",
	}}

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		t.Fatalf("get create options: %v", err)
	}

	if options.TeamConfig.ID != teamConfigID || options.TeamConfig.RevisionNumber != 4 {
		t.Fatalf("unexpected team config option: %#v", options.TeamConfig)
	}
	if got := options.TeamConfig.AllowedEmployeeTypes; len(got) != 1 || got[0] != "database_admin" {
		t.Fatalf("expected allowed employee types from policy, got %#v", got)
	}
	if len(options.EmployeeTypes) != 1 || options.EmployeeTypes[0].Type != "database_admin" {
		t.Fatalf("expected filtered employee type database_admin, got %#v", options.EmployeeTypes)
	}
	if len(options.RuntimeProviderOptions) != 1 || !options.RuntimeProviderOptions[0].Available {
		t.Fatalf("expected available runtime provider option, got %#v", options.RuntimeProviderOptions)
	}
	if got := options.CapabilityOptions.ProviderTypes; len(got) != 1 || got[0] != "codex" {
		t.Fatalf("expected provider type from team policy, got %#v", got)
	}
}

func TestGetCreateOptionsRejectsEmptyAllowedEmployeeTypes(t *testing.T) {
	svc, _, tenantID, teamID := newCreateOptionsTestService(t, map[string]any{
		"allowed_employee_types": []any{},
	}, nil)

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   teamID,
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for empty allowed_employee_types, got options=%#v err=%v", options, err)
	}
}

func TestGetCreateOptionsRejectsMalformedAllowedEmployeeTypes(t *testing.T) {
	svc, _, tenantID, teamID := newCreateOptionsTestService(t, map[string]any{
		"allowed_employee_types": []any{"database_admin", 42},
	}, nil)

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   teamID,
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for malformed allowed_employee_types, got options=%#v err=%v", options, err)
	}
}

func TestCreateDigitalEmployeeParamsAndDomainMappingKeepOwnerAndType(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	ownerUserID := uuid.New()

	record, err := repo.CreateDigitalEmployee(context.Background(), CreateDigitalEmployeeParams{
		TenantID:     tenantID,
		OwnerUserID:  ownerUserID,
		EmployeeType: "database_admin",
		Name:         "Database maintainer",
		Role:         "database_admin",
		Status:       DigitalEmployeeStatusDraft,
	})
	if err != nil {
		t.Fatalf("create digital employee: %v", err)
	}

	if record.OwnerUserID != ownerUserID {
		t.Fatalf("expected owner_user_id %s, got %s", ownerUserID, record.OwnerUserID)
	}
	if record.EmployeeType != "database_admin" {
		t.Fatalf("expected employee_type database_admin, got %q", record.EmployeeType)
	}
	employee := employeeFromRecord(record)
	if employee.OwnerUserID != ownerUserID {
		t.Fatalf("expected domain owner_user_id %s, got %s", ownerUserID, employee.OwnerUserID)
	}
	if employee.EmployeeType != "database_admin" {
		t.Fatalf("expected domain employee_type database_admin, got %q", employee.EmployeeType)
	}
}

func TestCreateDraftProvisioningDispatchesRuntimeCommandAndReturnsReadyEmployee(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewServiceWithProvisioning(repo, dispatcher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.preflight = RuntimeProvisioningPreflight{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "runtime-node-1",
		AgentHomeDir:  "/runtime/reported/agent-home",
		GovernanceSnapshot: map[string]any{
			"team_config_revision_id": uuid.NewString(),
			"authorization":           "Bearer raw-token",
			"capability_policy":       map[string]any{"api_key": "raw-key"},
			"approval_policy":         map[string]any{"min_risk_for_human": "high"},
		},
		HasActiveTeamConfig:   true,
		RuntimeOnline:         true,
		EnrollmentApproved:    true,
		RuntimeSessionActive:  true,
		ProviderAvailable:     true,
		ProviderPolicyAllowed: true,
		RuntimePolicyAllowed:  true,
	}
	repo.waitStatus = string(DigitalEmployeeRunStatusCompleted)
	dispatcher.connected["runtime-node-1"] = true
	sessionPolicy := map[string]any{"mode": "reuse_latest", "token": "raw-session-token"}
	workspacePolicy := map[string]any{"labels": map[string]any{"tier": "standard"}, "secret": "raw-workspace-secret"}

	created, err := svc.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID:        tenantID,
		TeamID:          &teamID,
		OwnerUserID:     ownerUserID,
		Name:            "  Finance reviewer  ",
		Role:            "  finance_reviewer  ",
		RuntimeNodeID:   runtimeNodeID,
		ProviderType:    "  codex  ",
		SessionPolicy:   sessionPolicy,
		WorkspacePolicy: workspacePolicy,
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	if created.TenantID != tenantID {
		t.Fatalf("expected tenant id %s, got %s", tenantID, created.TenantID)
	}
	if created.TeamID == nil || *created.TeamID != teamID {
		t.Fatalf("expected team id %s, got %#v", teamID, created.TeamID)
	}
	if created.Name != "Finance reviewer" {
		t.Fatalf("expected trimmed name, got %q", created.Name)
	}
	if created.Role != "finance_reviewer" {
		t.Fatalf("expected trimmed role, got %q", created.Role)
	}
	if created.Status != DigitalEmployeeStatusReady {
		t.Fatalf("expected ready status after provisioning completion, got %q", created.Status)
	}
	if created.RiskLevel != "medium" {
		t.Fatalf("expected default risk level medium, got %q", created.RiskLevel)
	}
	assertEmptyMap(t, created.PermissionPolicy, "permission policy")
	assertEmptyMap(t, created.ContextPolicy, "context policy")
	assertEmptyMap(t, created.ApprovalPolicy, "approval policy")
	assertEmptyMap(t, created.Metadata, "metadata")
	if repo.createdEmployeeCount != 1 {
		t.Fatalf("expected one employee to be created, got %d", repo.createdEmployeeCount)
	}
	if len(repo.instances) != 1 {
		t.Fatalf("expected one execution instance, got %#v", repo.instances)
	}
	var instance DigitalEmployeeExecutionInstanceRecord
	for _, record := range repo.instances {
		instance = record
	}
	if instance.Status != ExecutionInstanceStatusReady {
		t.Fatalf("expected ready execution instance after completion, got %q", instance.Status)
	}
	if instance.AgentHomeDir != "/runtime/reported/agent-home" {
		t.Fatalf("expected agent home dir from runtime preflight, got %q", instance.AgentHomeDir)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one runtime command, got %#v", dispatcher.commands)
	}
	command := dispatcher.commands[0]
	if command.Type != "provision_instance" {
		t.Fatalf("expected provision_instance command, got %q", command.Type)
	}
	var payload map[string]any
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		t.Fatalf("decode runtime command payload: %v", err)
	}
	if payload["command_id"] != command.ID {
		t.Fatalf("expected payload command_id %q, got %#v", command.ID, payload["command_id"])
	}
	if payload["digital_employee_id"] != created.ID.String() || payload["execution_instance_id"] != instance.ID.String() {
		t.Fatalf("unexpected provisioning ids in payload: %#v", payload)
	}
	if payload["provider_type"] != "codex" || payload["provider_run_protocol"] != providerRunProtocol {
		t.Fatalf("unexpected provider payload fields: %#v", payload)
	}
	gotSessionPolicy, ok := payload["session_policy"].(map[string]any)
	if !ok || gotSessionPolicy["mode"] != "reuse_latest" {
		t.Fatalf("expected session policy in payload, got %#v", payload["session_policy"])
	}
	if gotSessionPolicy["token"] != "[redacted]" {
		t.Fatalf("expected session policy token redacted in payload, got %#v", gotSessionPolicy)
	}
	gotWorkspacePolicy, ok := payload["workspace_policy"].(map[string]any)
	if !ok {
		t.Fatalf("expected workspace policy in payload, got %#v", payload["workspace_policy"])
	}
	if gotWorkspacePolicy["secret"] != "[redacted]" {
		t.Fatalf("expected workspace policy secret redacted in payload, got %#v", gotWorkspacePolicy)
	}
	governanceSnapshot, ok := payload["governance_snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("expected governance snapshot object in payload, got %#v", payload["governance_snapshot"])
	}
	if governanceSnapshot["authorization"] != "[redacted]" {
		t.Fatalf("expected governance authorization redacted in payload, got %#v", governanceSnapshot)
	}
	if gotCapabilityPolicy, ok := governanceSnapshot["capability_policy"].(map[string]any); !ok || gotCapabilityPolicy["api_key"] != "[redacted]" {
		t.Fatalf("expected governance capability policy api_key redacted in payload, got %#v", governanceSnapshot["capability_policy"])
	}
	if gotApprovalPolicy, ok := governanceSnapshot["approval_policy"].(map[string]any); !ok || gotApprovalPolicy["min_risk_for_human"] != "high" {
		t.Fatalf("expected governance approval policy in payload, got %#v", governanceSnapshot["approval_policy"])
	}
	if len(repo.commandReceipts) != 1 {
		t.Fatalf("expected one command receipt, got %#v", repo.commandReceipts)
	}
	for _, receipt := range repo.commandReceipts {
		gotSessionPolicy, ok := receipt.Payload["session_policy"].(map[string]any)
		if !ok || gotSessionPolicy["token"] != "[redacted]" {
			t.Fatalf("expected command receipt session policy token redacted, got %#v", receipt.Payload["session_policy"])
		}
		gotGovernanceSnapshot, ok := receipt.Payload["governance_snapshot"].(map[string]any)
		if !ok || gotGovernanceSnapshot["authorization"] != "[redacted]" {
			t.Fatalf("expected command receipt governance authorization redacted, got %#v", receipt.Payload["governance_snapshot"])
		}
	}
}

func TestCreateDraftRequiresRuntimeConnectionBeforeCreatingEmployee(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewServiceWithProvisioning(repo, dispatcher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.preflight = RuntimeProvisioningPreflight{
		TenantID:              tenantID,
		TeamID:                teamID,
		RuntimeNodeID:         runtimeNodeID,
		NodeID:                "runtime-node-offline",
		AgentHomeDir:          "/runtime/reported/agent-home",
		GovernanceSnapshot:    map[string]any{"team_config_revision_id": uuid.NewString()},
		HasActiveTeamConfig:   true,
		RuntimeOnline:         true,
		EnrollmentApproved:    true,
		RuntimeSessionActive:  true,
		ProviderAvailable:     true,
		ProviderPolicyAllowed: true,
		RuntimePolicyAllowed:  true,
	}

	_, err = svc.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		OwnerUserID:   ownerUserID,
		Name:          "Incident analyst",
		Role:          "incident_analyst",
		RuntimeNodeID: runtimeNodeID,
		ProviderType:  "codex",
	})

	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected runtime unavailable for disconnected runtime, got %v", err)
	}
	if repo.createdEmployeeCount != 0 {
		t.Fatalf("expected disconnected runtime not to create employee, got %d", repo.createdEmployeeCount)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected disconnected runtime not to dispatch command, got %#v", dispatcher.commands)
	}
}

func TestCreateDraftProvisioningFailureCleansUpEmployeeAndInstance(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewServiceWithProvisioning(repo, dispatcher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.preflight = RuntimeProvisioningPreflight{
		TenantID:              tenantID,
		TeamID:                teamID,
		RuntimeNodeID:         runtimeNodeID,
		NodeID:                "runtime-node-1",
		AgentHomeDir:          "/runtime/reported/agent-home",
		GovernanceSnapshot:    map[string]any{"team_config_revision_id": uuid.NewString()},
		HasActiveTeamConfig:   true,
		RuntimeOnline:         true,
		EnrollmentApproved:    true,
		RuntimeSessionActive:  true,
		ProviderAvailable:     true,
		ProviderPolicyAllowed: true,
		RuntimePolicyAllowed:  true,
	}
	repo.waitStatus = string(DigitalEmployeeRunStatusFailed)
	dispatcher.connected["runtime-node-1"] = true

	_, err = svc.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		OwnerUserID:   ownerUserID,
		Name:          "Incident analyst",
		Role:          "incident_analyst",
		RuntimeNodeID: runtimeNodeID,
		ProviderType:  "codex",
	})

	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected runtime unavailable for failed provisioning, got %v", err)
	}
	if len(repo.abortReasons) != 1 {
		t.Fatalf("expected provisioning abort, got %#v", repo.abortReasons)
	}
	for _, record := range repo.employees {
		if record.DeletedAt == nil || record.Status != DigitalEmployeeStatusError {
			t.Fatalf("expected failed employee to be error and deleted, got %#v", record)
		}
	}
	for _, record := range repo.instances {
		if record.DeletedAt == nil || record.Status != ExecutionInstanceStatusError {
			t.Fatalf("expected failed instance to be error and deleted, got %#v", record)
		}
	}
}

func TestCreateDraftRejectsProviderOutsideTeamPolicyBeforeCreatingEmployee(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewServiceWithProvisioning(repo, dispatcher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.preflight = validRuntimeProvisioningPreflight(tenantID, teamID, runtimeNodeID)
	repo.preflight.ProviderPolicyAllowed = false
	dispatcher.connected["runtime-node-1"] = true

	_, err = svc.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		OwnerUserID:   ownerUserID,
		Name:          "Incident analyst",
		Role:          "incident_analyst",
		RuntimeNodeID: runtimeNodeID,
		ProviderType:  "opencode",
	})

	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected provider unavailable for provider outside team policy, got %v", err)
	}
	if repo.createdEmployeeCount != 0 {
		t.Fatalf("expected policy rejection not to create employee, got %d", repo.createdEmployeeCount)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected policy rejection not to dispatch command, got %#v", dispatcher.commands)
	}
}

func TestCreateDraftRejectsRuntimeOutsideTeamPolicyBeforeCreatingEmployee(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewServiceWithProvisioning(repo, dispatcher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.preflight = validRuntimeProvisioningPreflight(tenantID, teamID, runtimeNodeID)
	repo.preflight.RuntimePolicyAllowed = false
	dispatcher.connected["runtime-node-1"] = true

	_, err = svc.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		OwnerUserID:   ownerUserID,
		Name:          "Incident analyst",
		Role:          "incident_analyst",
		RuntimeNodeID: runtimeNodeID,
		ProviderType:  "codex",
	})

	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected runtime unavailable for runtime outside team policy, got %v", err)
	}
	if repo.createdEmployeeCount != 0 {
		t.Fatalf("expected policy rejection not to create employee, got %d", repo.createdEmployeeCount)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected policy rejection not to dispatch command, got %#v", dispatcher.commands)
	}
}

func TestCreateDraftProvisioningWaitCancellationUsesIndependentCleanupContext(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewServiceWithProvisioning(repo, dispatcher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.preflight = validRuntimeProvisioningPreflight(tenantID, teamID, runtimeNodeID)
	dispatcher.connected["runtime-node-1"] = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = svc.CreateDraft(ctx, CreateDraftRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		OwnerUserID:   ownerUserID,
		Name:          "Incident analyst",
		Role:          "incident_analyst",
		RuntimeNodeID: runtimeNodeID,
		ProviderType:  "codex",
	})

	if !errors.Is(err, ErrRuntimeUnavailable) || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("expected runtime unavailable wrapping context cancellation, got %v", err)
	}
	if len(repo.abortReasons) != 1 {
		t.Fatalf("expected provisioning abort after wait cancellation, got %#v", repo.abortReasons)
	}
	if len(repo.abortContextErrors) != 1 || repo.abortContextErrors[0] != nil {
		t.Fatalf("expected abort cleanup to use independent live context, got %#v", repo.abortContextErrors)
	}
	for _, record := range repo.employees {
		if record.DeletedAt == nil || record.Status != DigitalEmployeeStatusError {
			t.Fatalf("expected cancelled provisioning employee to be error and deleted, got %#v", record)
		}
	}
	for _, record := range repo.instances {
		if record.DeletedAt == nil || record.Status != ExecutionInstanceStatusError {
			t.Fatalf("expected cancelled provisioning instance to be error and deleted, got %#v", record)
		}
	}
}

func TestCreateDraftRequiresOwnerBeforeCreatingEmployee(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewServiceWithProvisioning(repo, dispatcher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.preflight = validRuntimeProvisioningPreflight(tenantID, teamID, runtimeNodeID)
	repo.waitStatus = string(DigitalEmployeeRunStatusCompleted)
	dispatcher.connected["runtime-node-1"] = true

	_, err = svc.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		Name:          "Incident analyst",
		Role:          "incident_analyst",
		RuntimeNodeID: runtimeNodeID,
		ProviderType:  "codex",
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for missing owner_user_id, got %v", err)
	}
	if repo.createdEmployeeCount != 0 {
		t.Fatalf("expected missing owner not to create employee, got %d", repo.createdEmployeeCount)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected missing owner not to dispatch command, got %#v", dispatcher.commands)
	}
}

func TestCreateDraftRequiresExistingTenantTeam(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithProvisioning(repo, newFakeRuntimeCommandDispatcher())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()

	_, err = svc.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		OwnerUserID:   ownerUserID,
		Name:          "Incident analyst",
		Role:          "incident_analyst",
		RuntimeNodeID: uuid.New(),
		ProviderType:  "codex",
	})

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found for missing tenant team, got %v", err)
	}
	if len(repo.employees) != 0 {
		t.Fatalf("expected missing tenant team not to create employee, got %#v", repo.employees)
	}
}

func TestBindExecutionInstanceReplacesExistingForEmployee(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	firstRuntimeID := uuid.New()
	secondRuntimeID := uuid.New()

	first, err := svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RuntimeNodeID:     firstRuntimeID,
		ProviderType:      "codex",
		AgentHomeDir:      "/srv/superteam/employees/finance",
		SessionPolicy:     map[string]any{"max_turns": float64(5)},
	})
	if err != nil {
		t.Fatalf("first bind: %v", err)
	}
	second, err := svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RuntimeNodeID:     secondRuntimeID,
		ProviderType:      "opencode",
		AgentHomeDir:      "/srv/superteam/employees/finance-v2",
		SessionPolicy:     map[string]any{"max_turns": float64(12)},
	})
	if err != nil {
		t.Fatalf("second bind: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected bind to replace same execution instance row id, got %s then %s", first.ID, second.ID)
	}
	if second.RuntimeNodeID != secondRuntimeID {
		t.Fatalf("expected updated runtime node id %s, got %s", secondRuntimeID, second.RuntimeNodeID)
	}
	if second.ProviderType != "opencode" {
		t.Fatalf("expected updated provider type, got %q", second.ProviderType)
	}
	if second.AgentHomeDir != "/srv/superteam/employees/finance-v2" {
		t.Fatalf("expected updated agent home dir, got %q", second.AgentHomeDir)
	}
	if second.SessionPolicy["max_turns"] != float64(12) {
		t.Fatalf("expected updated session policy, got %#v", second.SessionPolicy)
	}
	if second.Status != ExecutionInstanceStatusReady {
		t.Fatalf("expected ready status, got %q", second.Status)
	}
}

func TestServiceValidation(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "create requires tenant",
			run: func() error {
				_, err := svc.CreateDraft(context.Background(), CreateDraftRequest{TeamID: &teamID, Name: "employee", Role: "reviewer"})
				return err
			},
		},
		{
			name: "create requires team",
			run: func() error {
				_, err := svc.CreateDraft(context.Background(), CreateDraftRequest{TenantID: tenantID, Name: "employee", Role: "reviewer"})
				return err
			},
		},
		{
			name: "create requires name",
			run: func() error {
				_, err := svc.CreateDraft(context.Background(), CreateDraftRequest{TenantID: tenantID, TeamID: &teamID, Name: " ", Role: "reviewer"})
				return err
			},
		},
		{
			name: "create requires role",
			run: func() error {
				_, err := svc.CreateDraft(context.Background(), CreateDraftRequest{TenantID: tenantID, TeamID: &teamID, Name: "employee", Role: " "})
				return err
			},
		},
		{
			name: "bind requires provider",
			run: func() error {
				_, err := svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
					TenantID:          tenantID,
					DigitalEmployeeID: employeeID,
					RuntimeNodeID:     runtimeNodeID,
					AgentHomeDir:      "/tmp/agent",
				})
				return err
			},
		},
		{
			name: "bind requires agent home dir",
			run: func() error {
				_, err := svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
					TenantID:          tenantID,
					DigitalEmployeeID: employeeID,
					RuntimeNodeID:     runtimeNodeID,
					ProviderType:      "codex",
					AgentHomeDir:      " ",
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestCreateConfigRevisionDefaultsDraftAndRevisionNumber(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	teamID := uuid.New()
	spoofedApproverID := uuid.New()
	repo.nextConfigRevisionNumber = 3
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:       employeeID,
		TenantID: tenantID,
		TeamID:   &teamID,
		Name:     "Finance reviewer",
		Role:     "finance_reviewer",
		Status:   DigitalEmployeeStatusDraft,
	}

	revision, err := svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RoleProfile:       map[string]any{"title": "finance reviewer"},
		ApprovedBy:        &spoofedApproverID,
	})
	if err != nil {
		t.Fatalf("create config revision: %v", err)
	}

	if revision.RevisionNumber != 3 {
		t.Fatalf("expected revision number 3, got %d", revision.RevisionNumber)
	}
	if revision.Status != ConfigRevisionStatusDraft {
		t.Fatalf("expected draft status, got %q", revision.Status)
	}
	if revision.ApprovedAt != nil {
		t.Fatalf("expected draft revision approved_at to be nil, got %v", revision.ApprovedAt)
	}
	if revision.ApprovedBy != nil {
		t.Fatalf("expected draft revision approved_by to be nil, got %#v", revision.ApprovedBy)
	}
	if repo.createdConfigRevision.Status != ConfigRevisionStatusDraft {
		t.Fatalf("expected repository draft status, got %q", repo.createdConfigRevision.Status)
	}
	if repo.createdConfigRevision.ApprovedBy != nil || repo.createdConfigRevision.ApprovedAt != nil {
		t.Fatalf("expected repository draft approval metadata to be cleared, got %#v/%#v", repo.createdConfigRevision.ApprovedBy, repo.createdConfigRevision.ApprovedAt)
	}
}

func TestCreateConfigRevisionRequiresExistingEmployee(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	employeeID := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:       employeeID,
		TenantID: uuid.New(),
		Name:     "Wrong tenant employee",
		Role:     "reviewer",
		Status:   DigitalEmployeeStatusDraft,
	}

	_, err = svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: employeeID,
		RoleProfile:       map[string]any{"title": "finance reviewer"},
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found for wrong-tenant employee, got %v", err)
	}
	if len(repo.employeeConfigs) != 0 {
		t.Fatalf("expected missing employee not to insert config revision, got %#v", repo.employeeConfigs)
	}
}

func TestPreviewEffectiveConfigBlocksCapabilityOutsideTeamAllowlist(t *testing.T) {
	svc := newTestService(t)
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID:               uuid.New(),
			CapabilityPolicy: map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID:                  uuid.New(),
			CapabilitySelection: map[string]any{"enabled_skills": []any{"database-troubleshooting"}},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}

	assertBlockingIssue(t, preview.Validation, "capability_outside_team_allowlist")
}

func TestPreviewEffectiveConfigBlocksContextOutsideTeamScope(t *testing.T) {
	svc := newTestService(t)
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID:            uuid.New(),
			ContextPolicy: map[string]any{"sources": []any{"monitoring", "logs"}},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID:                    uuid.New(),
			ContextPolicyOverride: map[string]any{"sources": []any{"monitoring", "customer_profile"}},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}

	assertBlockingIssue(t, preview.Validation, "context_outside_team_scope")
}

func TestPreviewEffectiveConfigBlocksApprovalPolicyDowngrade(t *testing.T) {
	svc := newTestService(t)
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID: uuid.New(),
			ApprovalPolicy: map[string]any{
				"min_risk_for_human":          "high",
				"write_actions_require_human": true,
			},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID: uuid.New(),
			ApprovalPolicyOverride: map[string]any{
				"min_risk_for_human":          "critical",
				"write_actions_require_human": false,
			},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}

	if len(preview.Validation.BlockingErrors) != 2 {
		t.Fatalf("expected two approval downgrade blocking errors, got %#v", preview.Validation.BlockingErrors)
	}
	assertBlockingIssuePath(t, preview.Validation, "approval_policy_downgrade", "approval_policy_override.min_risk_for_human")
	assertBlockingIssuePath(t, preview.Validation, "approval_policy_downgrade", "approval_policy_override.write_actions_require_human")
}

func TestPreviewEffectiveConfigAllowsTeamInternalCollaborationPolicy(t *testing.T) {
	svc := newTestService(t)
	policy := map[string]any{
		"mode":                       "team_internal",
		"allow_same_team_handoffs":   true,
		"requires_external_approval": false,
	}
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID:                          uuid.New(),
			CapabilityPolicy:            map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
			ContextPolicy:               map[string]any{"sources": []any{"monitoring"}},
			ApprovalPolicy:              map[string]any{"min_risk_for_human": "high"},
			InternalCollaborationPolicy: policy,
		},
		EmployeeConfig: EmployeeConfigInput{
			ID:                    uuid.New(),
			CapabilitySelection:   map[string]any{"enabled_skills": []any{"incident-diagnosis"}},
			ContextPolicyOverride: map[string]any{"sources": []any{"monitoring"}},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}
	if len(preview.Validation.BlockingErrors) != 0 {
		t.Fatalf("expected no blocking errors, got %#v", preview.Validation.BlockingErrors)
	}
	got, ok := preview.EffectiveConfig["internal_collaboration_policy"].(map[string]any)
	if !ok {
		t.Fatalf("expected internal collaboration policy in effective config, got %#v", preview.EffectiveConfig["internal_collaboration_policy"])
	}
	if got["mode"] != "team_internal" || got["allow_same_team_handoffs"] != true {
		t.Fatalf("expected team internal collaboration policy, got %#v", got)
	}
}

func TestPreviewEffectiveConfigReportsMalformedPolicyValues(t *testing.T) {
	svc := newTestService(t)
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID:               uuid.New(),
			CapabilityPolicy: map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID:                  uuid.New(),
			CapabilitySelection: map[string]any{"enabled_skills": []any{"incident-diagnosis", 42}},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}

	assertBlockingIssuePath(t, preview.Validation, "invalid_policy_value", "capability_selection.enabled_skills")
}

func TestPreviewEffectiveConfigReportsUnknownApprovalRisk(t *testing.T) {
	svc := newTestService(t)
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID:             uuid.New(),
			ApprovalPolicy: map[string]any{"min_risk_for_human": "high"},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID:                     uuid.New(),
			ApprovalPolicyOverride: map[string]any{"min_risk_for_human": "severe"},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}

	assertBlockingIssuePath(t, preview.Validation, "invalid_policy_value", "approval_policy_override.min_risk_for_human")
}

func TestApproveEffectiveConfigBlocksValidationErrors(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	teamID := uuid.New()
	teamConfigRevisionID := uuid.New()
	employeeConfigRevisionID := uuid.New()
	approvedBy := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:       employeeID,
		TenantID: tenantID,
		TeamID:   &teamID,
		Name:     "Incident analyst",
		Role:     "incident_analyst",
		Status:   DigitalEmployeeStatusDraft,
	}
	repo.teamConfigs[teamConfigRevisionID] = TeamConfigInput{
		ID:               teamConfigRevisionID,
		TenantID:         tenantID,
		TeamID:           teamID,
		Status:           TeamConfigRevisionStatusActive,
		CapabilityPolicy: map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
	}
	repo.employeeConfigs[employeeConfigRevisionID] = EmployeeConfigInput{
		ID:                  employeeConfigRevisionID,
		TenantID:            tenantID,
		DigitalEmployeeID:   employeeID,
		CapabilitySelection: map[string]any{"enabled_skills": []any{"database-troubleshooting"}},
	}
	repo.instances[employeeID] = DigitalEmployeeExecutionInstanceRecord{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Status:            ExecutionInstanceStatusReady,
	}

	_, err = svc.ApproveEffectiveConfig(context.Background(), ApproveEffectiveConfigRequest{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     teamConfigRevisionID,
		EmployeeConfigRevisionID: employeeConfigRevisionID,
		ApprovedBy:               approvedBy,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for blocking validation errors, got %v", err)
	}
	if len(repo.effectiveConfigs) != 0 {
		t.Fatalf("expected no effective config to be created, got %#v", repo.effectiveConfigs)
	}
}

func TestApproveEffectiveConfigRejectsDuplicateApprovedConfig(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	teamID := uuid.New()
	teamConfigRevisionID := uuid.New()
	employeeConfigRevisionID := uuid.New()
	approvedBy := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:       employeeID,
		TenantID: tenantID,
		TeamID:   &teamID,
		Name:     "Incident analyst",
		Role:     "incident_analyst",
		Status:   DigitalEmployeeStatusDraft,
	}
	repo.teamConfigs[teamConfigRevisionID] = TeamConfigInput{
		ID:               teamConfigRevisionID,
		TenantID:         tenantID,
		TeamID:           teamID,
		Status:           TeamConfigRevisionStatusActive,
		CapabilityPolicy: map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
	}
	repo.employeeConfigs[employeeConfigRevisionID] = EmployeeConfigInput{
		ID:                  employeeConfigRevisionID,
		TenantID:            tenantID,
		DigitalEmployeeID:   employeeID,
		CapabilitySelection: map[string]any{"enabled_skills": []any{"incident-diagnosis"}},
	}
	repo.instances[employeeID] = DigitalEmployeeExecutionInstanceRecord{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Status:            ExecutionInstanceStatusReady,
	}
	existingID := uuid.New()
	repo.effectiveConfigs[existingID] = DigitalEmployeeEffectiveConfigRecord{
		ID:                       existingID,
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     teamConfigRevisionID,
		EmployeeConfigRevisionID: employeeConfigRevisionID,
		Status:                   EffectiveConfigStatusApproved,
	}

	_, err = svc.ApproveEffectiveConfig(context.Background(), ApproveEffectiveConfigRequest{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     teamConfigRevisionID,
		EmployeeConfigRevisionID: employeeConfigRevisionID,
		ApprovedBy:               approvedBy,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict for duplicate approved effective config, got %v", err)
	}
	if len(repo.effectiveConfigs) != 1 {
		t.Fatalf("expected duplicate approval not to create another effective config, got %#v", repo.effectiveConfigs)
	}
}

func TestPreviewEffectiveConfigByRevisionIDsRejectsWrongTeamConfig(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	employeeTeamID := uuid.New()
	otherTeamID := uuid.New()
	teamConfigRevisionID := uuid.New()
	employeeConfigRevisionID := uuid.New()
	approvedBy := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:       employeeID,
		TenantID: tenantID,
		TeamID:   &employeeTeamID,
		Name:     "Incident analyst",
		Role:     "incident_analyst",
		Status:   DigitalEmployeeStatusDraft,
	}
	repo.teamConfigs[teamConfigRevisionID] = TeamConfigInput{
		ID:               teamConfigRevisionID,
		TenantID:         tenantID,
		TeamID:           otherTeamID,
		Status:           TeamConfigRevisionStatusActive,
		CapabilityPolicy: map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
	}
	repo.employeeConfigs[employeeConfigRevisionID] = EmployeeConfigInput{
		ID:                  employeeConfigRevisionID,
		TenantID:            tenantID,
		DigitalEmployeeID:   employeeID,
		CapabilitySelection: map[string]any{"enabled_skills": []any{"incident-diagnosis"}},
	}
	repo.instances[employeeID] = DigitalEmployeeExecutionInstanceRecord{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Status:            ExecutionInstanceStatusReady,
	}

	_, err = svc.PreviewEffectiveConfigByRevisionIDs(context.Background(), PreviewEffectiveConfigByRevisionIDsRequest{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     teamConfigRevisionID,
		EmployeeConfigRevisionID: employeeConfigRevisionID,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for mismatched team config, got %v", err)
	}

	_, err = svc.ApproveEffectiveConfig(context.Background(), ApproveEffectiveConfigRequest{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     teamConfigRevisionID,
		EmployeeConfigRevisionID: employeeConfigRevisionID,
		ApprovedBy:               approvedBy,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected approval to reject mismatched team config, got %v", err)
	}
	if len(repo.effectiveConfigs) != 0 {
		t.Fatalf("expected mismatched team config not to persist effective config, got %#v", repo.effectiveConfigs)
	}
}

func TestPreviewEffectiveConfigByRevisionIDsRejectsDraftTeamConfig(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	teamID := uuid.New()
	teamConfigRevisionID := uuid.New()
	employeeConfigRevisionID := uuid.New()
	approvedBy := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:       employeeID,
		TenantID: tenantID,
		TeamID:   &teamID,
		Name:     "Incident analyst",
		Role:     "incident_analyst",
		Status:   DigitalEmployeeStatusDraft,
	}
	repo.teamConfigs[teamConfigRevisionID] = TeamConfigInput{
		ID:               teamConfigRevisionID,
		TenantID:         tenantID,
		TeamID:           teamID,
		Status:           TeamConfigRevisionStatusDraft,
		CapabilityPolicy: map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
	}
	repo.employeeConfigs[employeeConfigRevisionID] = EmployeeConfigInput{
		ID:                  employeeConfigRevisionID,
		TenantID:            tenantID,
		DigitalEmployeeID:   employeeID,
		CapabilitySelection: map[string]any{"enabled_skills": []any{"incident-diagnosis"}},
	}
	repo.instances[employeeID] = DigitalEmployeeExecutionInstanceRecord{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Status:            ExecutionInstanceStatusReady,
	}

	_, err = svc.PreviewEffectiveConfigByRevisionIDs(context.Background(), PreviewEffectiveConfigByRevisionIDsRequest{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     teamConfigRevisionID,
		EmployeeConfigRevisionID: employeeConfigRevisionID,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for draft team config preview, got %v", err)
	}

	_, err = svc.ApproveEffectiveConfig(context.Background(), ApproveEffectiveConfigRequest{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     teamConfigRevisionID,
		EmployeeConfigRevisionID: employeeConfigRevisionID,
		ApprovedBy:               approvedBy,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected approval to reject draft team config, got %v", err)
	}
	if len(repo.effectiveConfigs) != 0 {
		t.Fatalf("expected draft team config not to persist effective config, got %#v", repo.effectiveConfigs)
	}
}

func TestApproveEffectiveConfigRequiresReadyOrActiveExecutionInstance(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	teamID := uuid.New()
	teamConfigRevisionID := uuid.New()
	employeeConfigRevisionID := uuid.New()
	approvedBy := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:       employeeID,
		TenantID: tenantID,
		TeamID:   &teamID,
		Name:     "Incident analyst",
		Role:     "incident_analyst",
		Status:   DigitalEmployeeStatusDraft,
	}
	repo.teamConfigs[teamConfigRevisionID] = TeamConfigInput{
		ID:               teamConfigRevisionID,
		TenantID:         tenantID,
		TeamID:           teamID,
		Status:           TeamConfigRevisionStatusActive,
		CapabilityPolicy: map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
	}
	repo.employeeConfigs[employeeConfigRevisionID] = EmployeeConfigInput{
		ID:                  employeeConfigRevisionID,
		TenantID:            tenantID,
		DigitalEmployeeID:   employeeID,
		CapabilitySelection: map[string]any{"enabled_skills": []any{"incident-diagnosis"}},
	}
	repo.instances[employeeID] = DigitalEmployeeExecutionInstanceRecord{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Status:            ExecutionInstanceStatusDisabled,
	}

	_, err = svc.ApproveEffectiveConfig(context.Background(), ApproveEffectiveConfigRequest{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     teamConfigRevisionID,
		EmployeeConfigRevisionID: employeeConfigRevisionID,
		ApprovedBy:               approvedBy,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for disabled execution instance, got %v", err)
	}

	repo.instances[employeeID] = DigitalEmployeeExecutionInstanceRecord{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Status:            ExecutionInstanceStatusReady,
	}
	effectiveConfig, err := svc.ApproveEffectiveConfig(context.Background(), ApproveEffectiveConfigRequest{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     teamConfigRevisionID,
		EmployeeConfigRevisionID: employeeConfigRevisionID,
		ApprovedBy:               approvedBy,
	})
	if err != nil {
		t.Fatalf("approve effective config: %v", err)
	}
	if effectiveConfig.Status != EffectiveConfigStatusApproved {
		t.Fatalf("expected approved effective config, got %q", effectiveConfig.Status)
	}
	if effectiveConfig.ApprovedBy == nil || *effectiveConfig.ApprovedBy != approvedBy {
		t.Fatalf("expected approved_by %s, got %#v", approvedBy, effectiveConfig.ApprovedBy)
	}
	if repo.createdEffectiveConfig.ValidationResult["blocking_errors"] == nil {
		t.Fatalf("expected validation result to be persisted, got %#v", repo.createdEffectiveConfig.ValidationResult)
	}
}

func TestJSONBFromMapRejectsUnsupportedValues(t *testing.T) {
	_, err := jsonbFromMap(map[string]any{"bad": func() {}}, "metadata")
	if err == nil {
		t.Fatalf("expected JSONB encoding error")
	}
	if !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("expected field name in error, got %v", err)
	}
}

func assertEmptyMap(t *testing.T, value map[string]any, label string) {
	t.Helper()
	if value == nil {
		t.Fatalf("expected %s to default to empty map, got nil", label)
	}
	if len(value) != 0 {
		t.Fatalf("expected empty %s, got %#v", label, value)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func newCreateOptionsTestService(t *testing.T, capabilityPolicy, runtimeScopePolicy map[string]any) (*Service, *memoryRepository, uuid.UUID, uuid.UUID) {
	t.Helper()
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	teamConfigID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.teamConfigs[teamConfigID] = TeamConfigInput{
		ID:                 teamConfigID,
		TenantID:           tenantID,
		TeamID:             teamID,
		RevisionNumber:     1,
		Status:             TeamConfigRevisionStatusActive,
		CapabilityPolicy:   cloneMap(capabilityPolicy),
		RuntimeScopePolicy: cloneMap(runtimeScopePolicy),
	}
	repo.currentTeamConfigByTeam[teamID] = teamConfigID
	return svc, repo, tenantID, teamID
}

func assertBlockingIssue(t *testing.T, validation EffectiveConfigValidation, code string) {
	t.Helper()
	for _, issue := range validation.BlockingErrors {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("expected blocking issue %q, got %#v", code, validation.BlockingErrors)
}

func assertBlockingIssuePath(t *testing.T, validation EffectiveConfigValidation, code, path string) {
	t.Helper()
	for _, issue := range validation.BlockingErrors {
		if issue.Code == code && issue.Path == path {
			return
		}
	}
	t.Fatalf("expected blocking issue %q at %q, got %#v", code, path, validation.BlockingErrors)
}

func validRuntimeProvisioningPreflight(tenantID, teamID, runtimeNodeID uuid.UUID) RuntimeProvisioningPreflight {
	return RuntimeProvisioningPreflight{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        "runtime-node-1",
		AgentHomeDir:  "/runtime/reported/agent-home",
		GovernanceSnapshot: map[string]any{
			"team_config_revision_id": uuid.NewString(),
		},
		HasActiveTeamConfig:   true,
		RuntimeOnline:         true,
		EnrollmentApproved:    true,
		RuntimeSessionActive:  true,
		ProviderAvailable:     true,
		ProviderPolicyAllowed: true,
		RuntimePolicyAllowed:  true,
	}
}

type memoryRepository struct {
	teams                    map[uuid.UUID]uuid.UUID
	employees                map[uuid.UUID]DigitalEmployeeRecord
	instances                map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord
	preflight                RuntimeProvisioningPreflight
	preflightErr             error
	commandReceipts          map[string]*RuntimeCommandReceipt
	waitStatus               string
	waitErr                  error
	abortReasons             []string
	abortContextErrors       []error
	createdEmployeeCount     int
	teamConfigs              map[uuid.UUID]TeamConfigInput
	currentTeamConfigByTeam  map[uuid.UUID]uuid.UUID
	runtimeProviderOptions   []RuntimeProviderOption
	employeeConfigs          map[uuid.UUID]EmployeeConfigInput
	effectiveConfigs         map[uuid.UUID]DigitalEmployeeEffectiveConfigRecord
	nextConfigRevisionNumber int32
	createdConfigRevision    CreateConfigRevisionParams
	createdEffectiveConfig   CreateEffectiveConfigParams
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		teams:                    make(map[uuid.UUID]uuid.UUID),
		employees:                make(map[uuid.UUID]DigitalEmployeeRecord),
		instances:                make(map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord),
		commandReceipts:          make(map[string]*RuntimeCommandReceipt),
		teamConfigs:              make(map[uuid.UUID]TeamConfigInput),
		currentTeamConfigByTeam:  make(map[uuid.UUID]uuid.UUID),
		employeeConfigs:          make(map[uuid.UUID]EmployeeConfigInput),
		effectiveConfigs:         make(map[uuid.UUID]DigitalEmployeeEffectiveConfigRecord),
		nextConfigRevisionNumber: 1,
	}
}

func (r *memoryRepository) CreateDigitalEmployee(_ context.Context, params CreateDigitalEmployeeParams) (DigitalEmployeeRecord, error) {
	now := time.Now().UTC()
	record := DigitalEmployeeRecord{
		ID:               uuid.New(),
		TenantID:         params.TenantID,
		TeamID:           params.TeamID,
		OwnerUserID:      params.OwnerUserID,
		EmployeeType:     params.EmployeeType,
		Name:             params.Name,
		Role:             params.Role,
		Description:      params.Description,
		Status:           params.Status,
		PermissionPolicy: cloneMap(params.PermissionPolicy),
		ContextPolicy:    cloneMap(params.ContextPolicy),
		ApprovalPolicy:   cloneMap(params.ApprovalPolicy),
		RiskLevel:        params.RiskLevel,
		Metadata:         cloneMap(params.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	r.employees[record.ID] = record
	r.createdEmployeeCount++
	return record, nil
}

func (r *memoryRepository) ListDigitalEmployees(_ context.Context, params ListDigitalEmployeesParams) ([]DigitalEmployeeRecord, error) {
	records := make([]DigitalEmployeeRecord, 0, len(r.employees))
	for _, record := range r.employees {
		if record.TenantID != params.TenantID {
			continue
		}
		if params.Status != "" && record.Status != params.Status {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *memoryRepository) GetDigitalEmployee(_ context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error) {
	record, ok := r.employees[employeeID]
	if !ok || record.TenantID != tenantID {
		return DigitalEmployeeRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) EnsureTeamExists(_ context.Context, tenantID, teamID uuid.UUID) error {
	teamTenantID, ok := r.teams[teamID]
	if !ok || teamTenantID != tenantID {
		return ErrNotFound
	}
	return nil
}

func (r *memoryRepository) GetCurrentTeamConfigRevision(_ context.Context, tenantID, teamID uuid.UUID) (TeamConfigInput, error) {
	teamConfigID, ok := r.currentTeamConfigByTeam[teamID]
	if !ok {
		return TeamConfigInput{}, ErrNotFound
	}
	record, ok := r.teamConfigs[teamConfigID]
	if !ok || record.TenantID != tenantID || record.TeamID != teamID || record.Status != TeamConfigRevisionStatusActive {
		return TeamConfigInput{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) ListRuntimeProviderOptionsForCreate(_ context.Context, tenantID, teamID uuid.UUID) ([]RuntimeProviderOption, error) {
	if err := r.EnsureTeamExists(context.Background(), tenantID, teamID); err != nil {
		return nil, err
	}
	return append([]RuntimeProviderOption(nil), r.runtimeProviderOptions...), nil
}

func (r *memoryRepository) UpdateDigitalEmployeeStatus(_ context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error) {
	record, ok := r.employees[employeeID]
	if !ok || record.TenantID != tenantID {
		return DigitalEmployeeRecord{}, ErrNotFound
	}
	record.Status = status
	record.UpdatedAt = time.Now().UTC()
	r.employees[employeeID] = record
	return record, nil
}

func (r *memoryRepository) UpsertDigitalEmployeeExecutionInstance(_ context.Context, params UpsertExecutionInstanceParams) (DigitalEmployeeExecutionInstanceRecord, error) {
	if params.TenantID == uuid.Nil || params.DigitalEmployeeID == uuid.Nil {
		return DigitalEmployeeExecutionInstanceRecord{}, errors.New("tenant and employee are required")
	}
	now := time.Now().UTC()
	record, ok := r.instances[params.DigitalEmployeeID]
	if !ok {
		record.ID = uuid.New()
		record.CreatedAt = now
	}
	record.TenantID = params.TenantID
	record.DigitalEmployeeID = params.DigitalEmployeeID
	record.RuntimeNodeID = params.RuntimeNodeID
	record.ProviderType = params.ProviderType
	record.AgentHomeDir = params.AgentHomeDir
	record.WorkspacePolicy = cloneMap(params.WorkspacePolicy)
	record.SessionPolicy = cloneMap(params.SessionPolicy)
	record.RuntimeSelector = cloneMap(params.RuntimeSelector)
	record.CapacityRequirements = cloneMap(params.CapacityRequirements)
	record.FallbackPolicy = cloneMap(params.FallbackPolicy)
	record.Status = params.Status
	record.Metadata = cloneMap(params.Metadata)
	record.UpdatedAt = now
	r.instances[params.DigitalEmployeeID] = record
	return record, nil
}

func (r *memoryRepository) GetDigitalEmployeeExecutionInstanceByEmployeeID(_ context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeExecutionInstanceRecord, error) {
	record, ok := r.instances[employeeID]
	if !ok || record.TenantID != tenantID {
		return DigitalEmployeeExecutionInstanceRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) CreateDigitalEmployeeConfigRevision(_ context.Context, params CreateConfigRevisionParams) (DigitalEmployeeConfigRevisionRecord, error) {
	r.createdConfigRevision = params
	now := time.Now().UTC()
	approvedAt := params.ApprovedAt
	record := DigitalEmployeeConfigRevisionRecord{
		ID:                     uuid.New(),
		TenantID:               params.TenantID,
		DigitalEmployeeID:      params.DigitalEmployeeID,
		RevisionNumber:         params.RevisionNumber,
		RoleProfile:            cloneMap(params.RoleProfile),
		ConstitutionAddendum:   cloneMap(params.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(params.CapabilitySelection),
		ContextPolicyOverride:  cloneMap(params.ContextPolicyOverride),
		ApprovalPolicyOverride: cloneMap(params.ApprovalPolicyOverride),
		OutputContractAddendum: cloneMap(params.OutputContractAddendum),
		Status:                 params.Status,
		ApprovedBy:             validUUIDPtr(params.ApprovedBy),
		ApprovedAt:             cloneTimePtr(approvedAt),
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	r.employeeConfigs[record.ID] = EmployeeConfigInput{
		ID:                     record.ID,
		TenantID:               record.TenantID,
		DigitalEmployeeID:      record.DigitalEmployeeID,
		RevisionNumber:         record.RevisionNumber,
		RoleProfile:            cloneMap(record.RoleProfile),
		ConstitutionAddendum:   cloneMap(record.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(record.CapabilitySelection),
		ContextPolicyOverride:  cloneMap(record.ContextPolicyOverride),
		ApprovalPolicyOverride: cloneMap(record.ApprovalPolicyOverride),
		OutputContractAddendum: cloneMap(record.OutputContractAddendum),
	}
	return record, nil
}

func (r *memoryRepository) GetTeamConfigRevision(_ context.Context, tenantID, teamConfigRevisionID uuid.UUID) (TeamConfigInput, error) {
	record, ok := r.teamConfigs[teamConfigRevisionID]
	if !ok || record.TenantID != tenantID {
		return TeamConfigInput{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) GetDigitalEmployeeConfigRevision(_ context.Context, tenantID, digitalEmployeeID, employeeConfigRevisionID uuid.UUID) (EmployeeConfigInput, error) {
	record, ok := r.employeeConfigs[employeeConfigRevisionID]
	if !ok || record.TenantID != tenantID || record.DigitalEmployeeID != digitalEmployeeID {
		return EmployeeConfigInput{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) GetNextDigitalEmployeeConfigRevisionNumber(_ context.Context, tenantID, digitalEmployeeID uuid.UUID) (int32, error) {
	if tenantID == uuid.Nil || digitalEmployeeID == uuid.Nil {
		return 0, errors.New("tenant and employee are required")
	}
	return r.nextConfigRevisionNumber, nil
}

func (r *memoryRepository) GetCurrentDigitalEmployeeEffectiveConfig(_ context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeEffectiveConfigRecord, error) {
	for _, record := range r.effectiveConfigs {
		if record.TenantID != tenantID || record.DigitalEmployeeID != digitalEmployeeID {
			continue
		}
		if record.Status != EffectiveConfigStatusApproved || record.RevokedAt != nil {
			continue
		}
		return record, nil
	}
	return DigitalEmployeeEffectiveConfigRecord{}, ErrNotFound
}

func (r *memoryRepository) CreateDigitalEmployeeEffectiveConfig(_ context.Context, params CreateEffectiveConfigParams) (DigitalEmployeeEffectiveConfigRecord, error) {
	r.createdEffectiveConfig = params
	now := time.Now().UTC()
	record := DigitalEmployeeEffectiveConfigRecord{
		ID:                       uuid.New(),
		TenantID:                 params.TenantID,
		DigitalEmployeeID:        params.DigitalEmployeeID,
		TeamConfigRevisionID:     params.TeamConfigRevisionID,
		EmployeeConfigRevisionID: params.EmployeeConfigRevisionID,
		EffectiveConfig:          cloneMap(params.EffectiveConfig),
		ValidationResult:         cloneMap(params.ValidationResult),
		Status:                   params.Status,
		ApprovedBy:               validUUIDPtr(params.ApprovedBy),
		ApprovedAt:               cloneTimePtr(params.ApprovedAt),
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	r.effectiveConfigs[record.ID] = record
	return record, nil
}

func (r *memoryRepository) GetRuntimeProvisioningPreflight(_ context.Context, tenantID, teamID, runtimeNodeID uuid.UUID, providerType string) (RuntimeProvisioningPreflight, error) {
	if r.preflightErr != nil {
		return RuntimeProvisioningPreflight{}, r.preflightErr
	}
	if r.preflight.TenantID != tenantID || r.preflight.TeamID != teamID || r.preflight.RuntimeNodeID != runtimeNodeID || providerType == "" {
		return RuntimeProvisioningPreflight{}, ErrNotFound
	}
	return r.preflight, nil
}

func (r *memoryRepository) CreateRuntimeCommandReceipt(_ context.Context, req CreateRuntimeCommandReceiptRequest) error {
	r.commandReceipts[req.CommandID] = &RuntimeCommandReceipt{
		ID:            uuid.New(),
		TenantID:      req.TenantID,
		CommandID:     req.CommandID,
		CommandType:   req.CommandType,
		RuntimeNodeID: req.RuntimeNodeID,
		NodeID:        req.NodeID,
		ResourceType:  req.ResourceType,
		ResourceID:    req.ResourceID,
		Status:        req.Status,
		Payload:       redactRuntimeEventPayload(req.Payload),
		DispatchedAt:  cloneTimePtr(req.DispatchedAt),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	return nil
}

func (r *memoryRepository) WaitForRuntimeCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeCommandReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.waitErr != nil {
		return nil, r.waitErr
	}
	receipt, ok := r.commandReceipts[commandID]
	if !ok || receipt.TenantID != tenantID {
		return nil, ErrNotFound
	}
	if r.waitStatus != "" {
		now := time.Now().UTC()
		receipt.Status = r.waitStatus
		receipt.CompletedAt = &now
	}
	if receipt.Status == string(DigitalEmployeeRunStatusCompleted) {
		instance, ok := r.instances[receipt.ResourceID]
		if !ok {
			for _, record := range r.instances {
				if record.ID == receipt.ResourceID {
					instance = record
					ok = true
					break
				}
			}
		}
		if ok {
			now := time.Now().UTC()
			instance.Status = ExecutionInstanceStatusReady
			instance.ReadyAt = &now
			r.instances[instance.DigitalEmployeeID] = instance
			employeeRecord := r.employees[instance.DigitalEmployeeID]
			employeeRecord.Status = DigitalEmployeeStatusReady
			employeeRecord.UpdatedAt = now
			r.employees[instance.DigitalEmployeeID] = employeeRecord
		}
	}
	return receipt, nil
}

func (r *memoryRepository) AbortProvisionedDigitalEmployee(ctx context.Context, tenantID, employeeID, executionInstanceID uuid.UUID, reason string) error {
	r.abortReasons = append(r.abortReasons, reason)
	r.abortContextErrors = append(r.abortContextErrors, ctx.Err())
	now := time.Now().UTC()
	employeeRecord, ok := r.employees[employeeID]
	if ok && employeeRecord.TenantID == tenantID {
		employeeRecord.Status = DigitalEmployeeStatusError
		employeeRecord.DeletedAt = &now
		employeeRecord.UpdatedAt = now
		r.employees[employeeID] = employeeRecord
	}
	instance, ok := r.instances[employeeID]
	if ok && instance.TenantID == tenantID && (executionInstanceID == uuid.Nil || instance.ID == executionInstanceID) {
		instance.Status = ExecutionInstanceStatusError
		instance.ErrorAt = &now
		instance.ErrorMessage = &reason
		instance.DeletedAt = &now
		instance.UpdatedAt = now
		r.instances[employeeID] = instance
	}
	for _, receipt := range r.commandReceipts {
		if receipt.TenantID == tenantID && (executionInstanceID == uuid.Nil || receipt.ResourceID == executionInstanceID) {
			receipt.Status = string(DigitalEmployeeRunStatusFailed)
			receipt.ErrorMessage = &reason
			receipt.CompletedAt = &now
			receipt.UpdatedAt = now
		}
	}
	return nil
}

type fakeRuntimeCommandDispatcher struct {
	connected map[string]bool
	commands  []cpruntime.RuntimeCommand
	err       error
}

func newFakeRuntimeCommandDispatcher() *fakeRuntimeCommandDispatcher {
	return &fakeRuntimeCommandDispatcher{connected: make(map[string]bool)}
}

func (f *fakeRuntimeCommandDispatcher) IsConnected(nodeID string) bool {
	return f.connected[nodeID]
}

func (f *fakeRuntimeCommandDispatcher) Dispatch(_ context.Context, nodeID string, command cpruntime.RuntimeCommand) error {
	if f.err != nil {
		return f.err
	}
	if !f.IsConnected(nodeID) {
		return cpruntime.ErrRuntimeNotConnected
	}
	f.commands = append(f.commands, command)
	return nil
}
