package employee

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateDraftDigitalEmployeeDefaultsAndTrims(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	repo.teams[teamID] = tenantID

	created, err := svc.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
		Name:     "  Finance reviewer  ",
		Role:     "  finance_reviewer  ",
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
	if created.Status != DigitalEmployeeStatusDraft {
		t.Fatalf("expected draft status, got %q", created.Status)
	}
	if created.RiskLevel != "medium" {
		t.Fatalf("expected default risk level medium, got %q", created.RiskLevel)
	}
	assertEmptyMap(t, created.PermissionPolicy, "permission policy")
	assertEmptyMap(t, created.ContextPolicy, "context policy")
	assertEmptyMap(t, created.ApprovalPolicy, "approval policy")
	assertEmptyMap(t, created.Metadata, "metadata")
}

func TestCreateDraftRequiresExistingTenantTeam(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()

	_, err = svc.CreateDraft(context.Background(), CreateDraftRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
		Name:     "Incident analyst",
		Role:     "incident_analyst",
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

type memoryRepository struct {
	teams                    map[uuid.UUID]uuid.UUID
	employees                map[uuid.UUID]DigitalEmployeeRecord
	instances                map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord
	teamConfigs              map[uuid.UUID]TeamConfigInput
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
		teamConfigs:              make(map[uuid.UUID]TeamConfigInput),
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
