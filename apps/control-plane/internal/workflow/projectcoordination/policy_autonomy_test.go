package projectcoordination

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
)

func TestPolicyAutoResolvablePredispatchAction(t *testing.T) {
	tokenExhausted := project.PreDispatchGateResult{
		Blockers: []project.PreDispatchGateBlocker{{Key: "budget.token_exhausted"}},
	}
	budgetMissing := project.PreDispatchGateResult{
		Blockers: []project.PreDispatchGateBlocker{{Key: "budget.task_budget_missing"}},
	}
	require.False(t, policyAutoResolvablePredispatchAction(project.PreDispatchHumanActionRiskApproval, tokenExhausted))
	require.True(t, policyAutoResolvablePredispatchAction(project.PreDispatchHumanActionBudgetApproval, tokenExhausted))
	require.False(t, policyAutoResolvablePredispatchAction(project.PreDispatchHumanActionBudgetApproval, budgetMissing))
	require.False(t, policyAutoResolvablePredispatchAction(project.PreDispatchHumanActionBudgetApproval, project.PreDispatchGateResult{}))
	require.False(t, policyAutoResolvablePredispatchAction(project.PreDispatchHumanActionMissingContext, tokenExhausted))
	require.False(t, policyAutoResolvablePredispatchAction(project.PreDispatchHumanActionRuntimeRecovery, tokenExhausted))
}

func tokenExhaustedBudgetGate(decisionID uuid.UUID) project.PreDispatchGateResult {
	return project.PreDispatchGateResult{
		DecisionRequestID: &decisionID,
		Blockers:          []project.PreDispatchGateBlocker{{Key: "budget.token_exhausted"}},
	}
}

func TestAutomationRuleIDFromDemand(t *testing.T) {
	ruleID := uuid.New()
	id, ok := automationRuleIDFromDemand(project.ProjectDemand{
		SourceType: project.DemandSourceAutomation,
		SourceRefs: map[string]any{"automation_rule_id": ruleID.String()},
	})
	require.True(t, ok)
	require.Equal(t, ruleID, id)

	_, ok = automationRuleIDFromDemand(project.ProjectDemand{SourceType: project.DemandSourceManual})
	require.False(t, ok)
}

type stubAutonomyLookup struct {
	tier   string
	actor  uuid.UUID
	err    error
	calls  int
	ruleID uuid.UUID
}

func (s *stubAutonomyLookup) LookupAutomationAutonomy(ctx context.Context, tenantID, ruleID uuid.UUID) (string, uuid.UUID, error) {
	s.calls++
	s.ruleID = ruleID
	return s.tier, s.actor, s.err
}

type stubGateResolver struct {
	reqs []project.ResolveDecisionRequest
	err  error
}

func (s *stubGateResolver) ResolveDecision(ctx context.Context, req project.ResolveDecisionRequest) (*project.DecisionRequest, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return &project.DecisionRequest{ID: req.DecisionRequestID, StatusSnapshot: req.Decision}, nil
}

func TestMaybePolicyAutoResolvePredispatchGateFullAuto(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	ruleID := uuid.New()
	actorID := uuid.New()
	decisionID := uuid.New()
	taskID := uuid.New()

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:         demandID,
			TenantID:   tenantID,
			ProjectID:  projectID,
			SourceType: project.DemandSourceAutomation,
			SourceRefs: map[string]any{"automation_rule_id": ruleID.String()},
		},
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, CoordinationPolicy: map[string]any{}},
	}
	lookup := &stubAutonomyLookup{tier: autonomyTierFullAuto, actor: actorID}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithAutomationAutonomyLookup(lookup).
		WithGateDecisionResolver(resolver)

	err := store.maybePolicyAutoResolvePredispatchGate(
		context.Background(),
		DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID},
		project.ProjectTask{ID: taskID, ProjectID: projectID, DemandID: &demandID},
		tokenExhaustedBudgetGate(decisionID),
		&project.PreDispatchHumanActionRequest{Type: project.PreDispatchHumanActionBudgetApproval},
	)
	require.NoError(t, err)
	require.Equal(t, 1, lookup.calls)
	require.Equal(t, ruleID, lookup.ruleID)
	require.Len(t, resolver.reqs, 1)
	require.Equal(t, "approved", resolver.reqs[0].Decision)
	require.Equal(t, actorID, resolver.reqs[0].DecidedByUserID)
	require.Equal(t, "policy:"+ruleID.String(), resolver.reqs[0].Comment)
	require.Equal(t, "policy:"+ruleID.String(), resolver.reqs[0].Payload["resolved_by"])
}

func TestMaybePolicyAutoResolvePredispatchGateProjectCeilingTightens(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	ruleID := uuid.New()
	decisionID := uuid.New()

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:         demandID,
			TenantID:   tenantID,
			ProjectID:  projectID,
			SourceType: project.DemandSourceAutomation,
			SourceRefs: map[string]any{"automation_rule_id": ruleID.String()},
		},
		projectRecord: project.Project{
			ID:       projectID,
			TenantID: tenantID,
			CoordinationPolicy: map[string]any{
				"autonomy_ceiling": "pause_at_gate",
			},
		},
	}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithAutomationAutonomyLookup(&stubAutonomyLookup{tier: autonomyTierFullAuto, actor: uuid.New()}).
		WithGateDecisionResolver(resolver)

	err := store.maybePolicyAutoResolvePredispatchGate(
		context.Background(),
		DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID},
		project.ProjectTask{DemandID: &demandID, ProjectID: projectID},
		tokenExhaustedBudgetGate(decisionID),
		&project.PreDispatchHumanActionRequest{Type: project.PreDispatchHumanActionBudgetApproval},
	)
	require.NoError(t, err)
	require.Empty(t, resolver.reqs)
}

func TestMaybePolicyAutoResolvePredispatchGatePauseSkips(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	ruleID := uuid.New()
	decisionID := uuid.New()

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:         demandID,
			SourceType: project.DemandSourceAutomation,
			SourceRefs: map[string]any{"automation_rule_id": ruleID.String()},
		},
	}
	lookup := &stubAutonomyLookup{tier: "pause_at_gate", actor: uuid.New()}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithAutomationAutonomyLookup(lookup).
		WithGateDecisionResolver(resolver)

	err := store.maybePolicyAutoResolvePredispatchGate(
		context.Background(),
		DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID},
		project.ProjectTask{DemandID: &demandID, ProjectID: projectID},
		tokenExhaustedBudgetGate(decisionID),
		&project.PreDispatchHumanActionRequest{Type: project.PreDispatchHumanActionBudgetApproval},
	)
	require.NoError(t, err)
	require.Equal(t, 1, lookup.calls)
	require.Empty(t, resolver.reqs)
}

func TestMaybePolicyAutoResolvePredispatchGateRiskNeverAuto(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	ruleID := uuid.New()
	decisionID := uuid.New()

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:         demandID,
			TenantID:   tenantID,
			ProjectID:  projectID,
			SourceType: project.DemandSourceAutomation,
			SourceRefs: map[string]any{"automation_rule_id": ruleID.String()},
		},
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, CoordinationPolicy: map[string]any{}},
	}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithAutomationAutonomyLookup(&stubAutonomyLookup{tier: autonomyTierFullAuto, actor: uuid.New()}).
		WithGateDecisionResolver(resolver)

	err := store.maybePolicyAutoResolvePredispatchGate(
		context.Background(),
		DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID},
		project.ProjectTask{DemandID: &demandID, ProjectID: projectID},
		project.PreDispatchGateResult{DecisionRequestID: &decisionID},
		&project.PreDispatchHumanActionRequest{Type: project.PreDispatchHumanActionRiskApproval},
	)
	require.NoError(t, err)
	require.Empty(t, resolver.reqs, "playbook/platform risk_approval must park under full_auto")
}

func TestMaybePolicyAutoResolvePredispatchGateBudgetMissingNeverAuto(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	ruleID := uuid.New()
	decisionID := uuid.New()

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:         demandID,
			TenantID:   tenantID,
			ProjectID:  projectID,
			SourceType: project.DemandSourceAutomation,
			SourceRefs: map[string]any{"automation_rule_id": ruleID.String()},
		},
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, CoordinationPolicy: map[string]any{}},
	}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithAutomationAutonomyLookup(&stubAutonomyLookup{tier: autonomyTierFullAuto, actor: uuid.New()}).
		WithGateDecisionResolver(resolver)

	err := store.maybePolicyAutoResolvePredispatchGate(
		context.Background(),
		DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID},
		project.ProjectTask{DemandID: &demandID, ProjectID: projectID},
		project.PreDispatchGateResult{
			DecisionRequestID: &decisionID,
			Blockers:          []project.PreDispatchGateBlocker{{Key: "budget.task_budget_missing"}},
		},
		&project.PreDispatchHumanActionRequest{Type: project.PreDispatchHumanActionBudgetApproval},
	)
	require.NoError(t, err)
	require.Empty(t, resolver.reqs)
}

func TestMaybePolicyAutoResolvePredispatchGatePlaybookCeilingTightens(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	ruleID := uuid.New()
	decisionID := uuid.New()
	key := "ops_patrol"

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:                  demandID,
			TenantID:            tenantID,
			ProjectID:           projectID,
			SourceType:          project.DemandSourceAutomation,
			SourceRefs:          map[string]any{"automation_rule_id": ruleID.String()},
			ScenarioTemplateKey: &key,
		},
	}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithAutomationAutonomyLookup(&stubAutonomyLookup{tier: autonomyTierFullAuto, actor: uuid.New()}).
		WithGateDecisionResolver(resolver).
		WithScenarioTemplateSource(fakeScenarioTemplateSource{templates: map[string]ScenarioTemplateSnapshot{
			key: {
				Key: key,
				Spec: map[string]any{
					"spec_version":     2,
					"autonomy_ceiling": "pause_at_gate",
					"roles":            []any{map[string]any{"key": "executor", "title": "执行者"}},
					"skeleton":         []any{map[string]any{"step": "execute", "role": "executor"}},
					"exits":            []any{map[string]any{"deliverable": "outcome", "label": "完成"}},
				},
			},
		}})

	err := store.maybePolicyAutoResolvePredispatchGate(
		context.Background(),
		DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID},
		project.ProjectTask{DemandID: &demandID, ProjectID: projectID},
		tokenExhaustedBudgetGate(decisionID),
		&project.PreDispatchHumanActionRequest{Type: project.PreDispatchHumanActionBudgetApproval},
	)
	require.NoError(t, err)
	require.Empty(t, resolver.reqs)
}

type stubExternalAutonomyLookup struct {
	tier          string
	actor         uuid.UUID
	calls         int
	integrationID uuid.UUID
}

func (s *stubExternalAutonomyLookup) LookupIntegrationAutonomy(ctx context.Context, tenantID, integrationID uuid.UUID) (string, uuid.UUID, error) {
	s.calls++
	s.integrationID = integrationID
	return s.tier, s.actor, nil
}

func TestMaybePolicyAutoResolvePredispatchGateExternalIntegrationFullAuto(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	integrationID := uuid.New()
	actorID := uuid.New()
	decisionID := uuid.New()

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:         demandID,
			TenantID:   tenantID,
			ProjectID:  projectID,
			SourceType: project.DemandSourceExternal,
			SourceRefs: map[string]any{"external_integration_id": integrationID.String()},
		},
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, CoordinationPolicy: map[string]any{}},
	}
	lookup := &stubExternalAutonomyLookup{tier: autonomyTierFullAuto, actor: actorID}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithExternalIntegrationAutonomyLookup(lookup).
		WithGateDecisionResolver(resolver)

	err := store.maybePolicyAutoResolvePredispatchGate(
		context.Background(),
		DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID},
		project.ProjectTask{DemandID: &demandID, ProjectID: projectID},
		tokenExhaustedBudgetGate(decisionID),
		&project.PreDispatchHumanActionRequest{Type: project.PreDispatchHumanActionBudgetApproval},
	)
	require.NoError(t, err)
	require.Equal(t, 1, lookup.calls)
	require.Equal(t, integrationID, lookup.integrationID)
	require.Len(t, resolver.reqs, 1)
	require.Equal(t, "policy:"+integrationID.String(), resolver.reqs[0].Comment)
	require.Equal(t, actorID, resolver.reqs[0].DecidedByUserID)
}

func TestMaybePolicyAutoResolvePredispatchGateExternalCeilingTightens(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	integrationID := uuid.New()
	decisionID := uuid.New()
	actorID := uuid.New()

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:         demandID,
			TenantID:   tenantID,
			ProjectID:  projectID,
			SourceType: project.DemandSourceExternal,
			SourceRefs: map[string]any{"external_integration_id": integrationID.String()},
		},
		projectRecord: project.Project{
			ID:                 projectID,
			TenantID:           tenantID,
			CoordinationPolicy: map[string]any{"autonomy_ceiling": "pause_at_gate"},
		},
	}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithExternalIntegrationAutonomyLookup(&stubExternalAutonomyLookup{tier: autonomyTierFullAuto, actor: actorID}).
		WithGateDecisionResolver(resolver)

	err := store.maybePolicyAutoResolvePredispatchGate(
		context.Background(),
		DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID},
		project.ProjectTask{DemandID: &demandID, ProjectID: projectID},
		tokenExhaustedBudgetGate(decisionID),
		&project.PreDispatchHumanActionRequest{Type: project.PreDispatchHumanActionBudgetApproval},
	)
	require.NoError(t, err)
	// Live ceiling tighten → sync policy reject (never park external into inbox).
	require.Len(t, resolver.reqs, 1)
	require.Equal(t, project.PlanReviewDecisionReject, resolver.reqs[0].Decision)
	require.Equal(t, actorID, resolver.reqs[0].DecidedByUserID)
	require.Equal(t, "pause_at_gate", resolver.reqs[0].Payload["autonomy_tier"])
}

func TestMaybePolicyAutoResolvePredispatchGateExternalRiskRejects(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	integrationID := uuid.New()
	decisionID := uuid.New()
	actorID := uuid.New()

	repo := &policyAutonomyRepoStub{
		demand: project.ProjectDemand{
			ID:         demandID,
			TenantID:   tenantID,
			ProjectID:  projectID,
			SourceType: project.DemandSourceExternal,
			SourceRefs: map[string]any{"external_integration_id": integrationID.String()},
		},
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, CoordinationPolicy: map[string]any{}},
	}
	resolver := &stubGateResolver{}
	store := NewProjectStore(repo).
		WithExternalIntegrationAutonomyLookup(&stubExternalAutonomyLookup{tier: autonomyTierFullAuto, actor: actorID}).
		WithGateDecisionResolver(resolver)

	err := store.maybePolicyAutoResolvePredispatchGate(
		context.Background(),
		DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID},
		project.ProjectTask{DemandID: &demandID, ProjectID: projectID},
		project.PreDispatchGateResult{DecisionRequestID: &decisionID},
		&project.PreDispatchHumanActionRequest{Type: project.PreDispatchHumanActionRiskApproval},
	)
	require.NoError(t, err)
	require.Len(t, resolver.reqs, 1)
	require.Equal(t, project.PlanReviewDecisionReject, resolver.reqs[0].Decision)
}

func TestMaybePolicyAutoResolvePredispatchGateMissingContextNeverAuto(t *testing.T) {
	decisionID := uuid.New()
	resolver := &stubGateResolver{}
	store := NewProjectStore(&policyAutonomyRepoStub{}).
		WithAutomationAutonomyLookup(&stubAutonomyLookup{tier: autonomyTierFullAuto, actor: uuid.New()}).
		WithGateDecisionResolver(resolver)

	err := store.maybePolicyAutoResolvePredispatchGate(
		context.Background(),
		DispatchProjectTaskInput{},
		project.ProjectTask{},
		project.PreDispatchGateResult{DecisionRequestID: &decisionID},
		&project.PreDispatchHumanActionRequest{Type: project.PreDispatchHumanActionMissingContext},
	)
	require.NoError(t, err)
	require.Empty(t, resolver.reqs)
}

// policyAutonomyRepoStub only implements GetProjectDemand / GetProject for these unit tests.
type policyAutonomyRepoStub struct {
	project.Repository
	demand          project.ProjectDemand
	projectRecord   project.Project
	task            project.ProjectTask
	pendingRecovery []project.DecisionRequest
	err             error
}

func (r *policyAutonomyRepoStub) GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (project.ProjectDemand, error) {
	if r.err != nil {
		return project.ProjectDemand{}, r.err
	}
	return r.demand, nil
}

func (r *policyAutonomyRepoStub) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (project.Project, error) {
	if r.projectRecord.ID != uuid.Nil {
		return r.projectRecord, nil
	}
	return project.Project{ID: projectID, TenantID: tenantID, CoordinationPolicy: map[string]any{}}, nil
}

func (r *policyAutonomyRepoStub) GetProjectTask(ctx context.Context, tenantID, taskID uuid.UUID) (project.ProjectTask, error) {
	if r.task.ID != uuid.Nil {
		return r.task, nil
	}
	return project.ProjectTask{}, project.ErrProjectNotFound
}

func (r *policyAutonomyRepoStub) ListPendingRecoveryDecisionsForAutoRecheck(ctx context.Context, limit int32) ([]project.DecisionRequest, error) {
	return append([]project.DecisionRequest(nil), r.pendingRecovery...), nil
}

func (r *policyAutonomyRepoStub) SumProjectConsumedTokens(ctx context.Context, tenantID, projectID uuid.UUID) (int64, error) {
	return 0, nil
}
