package projectcoordination

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/autonomypolicy"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// Wire-literal shared with automation.AutonomyTierFullAuto (avoid importing
// automation into coordination and risking package cycles).
const autonomyTierFullAuto = "full_auto"

// AutomationAutonomyLookup resolves a rule's autonomy tier + signing actor.
// Optional: nil disables policy auto-resolve (all gates park as today).
type AutomationAutonomyLookup interface {
	LookupAutomationAutonomy(ctx context.Context, tenantID, ruleID uuid.UUID) (tier string, actorUserID uuid.UUID, err error)
}

// ExternalIntegrationAutonomyLookup resolves an external API integration
// binding's tier + grantor (autonomy P5). Same live-reference semantics:
// ceilings are applied here at gate time via effectiveAutomationAutonomy.
type ExternalIntegrationAutonomyLookup interface {
	LookupIntegrationAutonomy(ctx context.Context, tenantID, integrationID uuid.UUID) (tier string, actorUserID uuid.UUID, err error)
}

// GateDecisionResolver auto-resolves minted decisions under full_auto policy.
// Typically project.Service.ResolveDecision (audit + inbox + coordinator signal).
type GateDecisionResolver interface {
	ResolveDecision(ctx context.Context, req project.ResolveDecisionRequest) (*project.DecisionRequest, error)
}

func (s *ProjectStore) WithAutomationAutonomyLookup(lookup AutomationAutonomyLookup) *ProjectStore {
	s.autonomyLookup = lookup
	return s
}

func (s *ProjectStore) WithExternalIntegrationAutonomyLookup(lookup ExternalIntegrationAutonomyLookup) *ProjectStore {
	s.externalAutonomyLookup = lookup
	return s
}

func (s *ProjectStore) WithGateDecisionResolver(resolver GateDecisionResolver) *ProjectStore {
	s.gateDecisionResolver = resolver
	return s
}

// policyAutoResolvablePredispatchAction reports whether a minted predispatch
// human action may be signed by a pre-authorized full_auto rule.
//
// F6 / §4.2 fifth step + §2.2 budget split:
//   - risk_approval: never (only fires from platform/playbook declarations)
//   - budget_approval: only sub-route ② token exhaustion (server fact);
//     ① task_budget_missing / ③ needs_budget_approval are planner metadata
//   - missing_context: never (human facts)
//   - runtime_recovery: F7 auto_recheck, not blind policy sign
func policyAutoResolvablePredispatchAction(actionType string, gate project.PreDispatchGateResult) bool {
	switch strings.TrimSpace(actionType) {
	case project.PreDispatchHumanActionBudgetApproval:
		return predispatchGateHasBlockerKey(gate, "budget.token_exhausted")
	default:
		return false
	}
}

func predispatchGateHasBlockerKey(gate project.PreDispatchGateResult, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, blocker := range gate.Blockers {
		if strings.TrimSpace(blocker.Key) == key {
			return true
		}
	}
	return false
}

func automationRuleIDFromDemand(demand project.ProjectDemand) (uuid.UUID, bool) {
	if demand.SourceType != project.DemandSourceAutomation {
		return uuid.Nil, false
	}
	return sourceRefUUID(demand, "automation_rule_id")
}

func externalIntegrationIDFromDemand(demand project.ProjectDemand) (uuid.UUID, bool) {
	if demand.SourceType != project.DemandSourceExternal {
		return uuid.Nil, false
	}
	return sourceRefUUID(demand, "external_integration_id")
}

func sourceRefUUID(demand project.ProjectDemand, key string) (uuid.UUID, bool) {
	raw, _ := demand.SourceRefs[key].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, false
	}
	return id, true
}

// policyAutoResolveConfigured gates the whole policy-resolve feature: without a
// resolver plus at least one lookup, gates park as before and the demand is
// never loaded (keeps fakes/tests without demand access working).
func (s *ProjectStore) policyAutoResolveConfigured() bool {
	return s.gateDecisionResolver != nil && (s.autonomyLookup != nil || s.externalAutonomyLookup != nil)
}

// lookupFullAutoPolicy resolves the demand's pre-authorized invoker (automation
// rule or external integration binding) and reports whether the effective tier
// — after live project/playbook ceilings — signs gates as full_auto.
func (s *ProjectStore) lookupFullAutoPolicy(ctx context.Context, tenantID uuid.UUID, demand project.ProjectDemand) (policyID uuid.UUID, actorUserID uuid.UUID, ok bool, err error) {
	if s.gateDecisionResolver == nil {
		return uuid.Nil, uuid.Nil, false, nil
	}
	var tier string
	var actor uuid.UUID
	switch {
	case s.autonomyLookup != nil && demand.SourceType == project.DemandSourceAutomation:
		ruleID, found := automationRuleIDFromDemand(demand)
		if !found {
			return uuid.Nil, uuid.Nil, false, nil
		}
		policyID = ruleID
		tier, actor, err = s.autonomyLookup.LookupAutomationAutonomy(ctx, tenantID, ruleID)
	case s.externalAutonomyLookup != nil && demand.SourceType == project.DemandSourceExternal:
		integrationID, found := externalIntegrationIDFromDemand(demand)
		if !found {
			return uuid.Nil, uuid.Nil, false, nil
		}
		policyID = integrationID
		tier, actor, err = s.externalAutonomyLookup.LookupIntegrationAutonomy(ctx, tenantID, integrationID)
	default:
		return uuid.Nil, uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, false, err
	}
	if actor == uuid.Nil {
		return policyID, actor, false, nil
	}
	effective := s.effectiveAutomationAutonomy(ctx, tenantID, demand, tier)
	if effective != autonomyTierFullAuto {
		return policyID, actor, false, nil
	}
	return policyID, actor, true, nil
}

// effectiveAutomationAutonomy applies playbook + project ceilings to the rule tier
// at fire/gate time (P3 unidirectional tighten; stock wide tiers degrade).
func (s *ProjectStore) effectiveAutomationAutonomy(ctx context.Context, tenantID uuid.UUID, demand project.ProjectDemand, invokerTier string) string {
	projectCeiling := ""
	if s.repository != nil {
		if proj, err := s.repository.GetProject(ctx, tenantID, demand.ProjectID); err == nil {
			projectCeiling = autonomypolicy.CoordinationPolicyCeiling(proj.CoordinationPolicy)
		}
	}
	playbookCeiling := ""
	if key := strings.TrimSpace(ptrString(demand.ScenarioTemplateKey)); key != "" && s.scenarioTemplates != nil {
		if snap, err := s.scenarioTemplates.GetScenarioTemplateSnapshot(ctx, tenantID, key); err == nil {
			if spec, err := scenariotemplate.ParseSpec(snap.Spec); err == nil {
				playbookCeiling = strings.TrimSpace(spec.AutonomyCeiling)
			}
		}
	}
	return autonomypolicy.Effective(playbookCeiling, projectCeiling, invokerTier)
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *ProjectStore) resolveDecisionAsPolicy(ctx context.Context, tenantID, projectID, decisionID, ruleID, actorUserID uuid.UUID, decision, autonomyTier string) error {
	resolvedBy := fmt.Sprintf("policy:%s", ruleID.String())
	if strings.TrimSpace(autonomyTier) == "" {
		autonomyTier = autonomyTierFullAuto
	}
	_, err := s.gateDecisionResolver.ResolveDecision(ctx, project.ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorUserID,
		Decision:          decision,
		Comment:           resolvedBy,
		Payload: map[string]any{
			"resolved_by":   resolvedBy,
			"autonomy_tier": autonomyTier,
		},
		Channel: "policy",
	})
	return err
}

// maybePolicyAutoResolvePredispatchGate mints-then-releases machine-decidable
// predispatch waits when the demand's automation rule / external integration is
// full_auto. Non-auto-resolvable gates park for automation; external demands
// are always policy-rejected instead of parking (spec §4.4 / §10).
func (s *ProjectStore) maybePolicyAutoResolvePredispatchGate(
	ctx context.Context,
	input DispatchProjectTaskInput,
	task project.ProjectTask,
	gate project.PreDispatchGateResult,
	action *project.PreDispatchHumanActionRequest,
) error {
	if !s.policyAutoResolveConfigured() {
		return nil
	}
	if action == nil {
		return nil
	}
	if gate.DecisionRequestID == nil || *gate.DecisionRequestID == uuid.Nil {
		return nil
	}
	if task.DemandID == nil || *task.DemandID == uuid.Nil {
		return nil
	}
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, *task.DemandID)
	if err != nil {
		return fmt.Errorf("load demand for policy autonomy: %w", err)
	}
	ruleID, actorID, ok, err := s.lookupFullAutoPolicy(ctx, input.TenantID, demand)
	if err != nil {
		return err
	}
	if ok && policyAutoResolvablePredispatchAction(action.Type, gate) {
		return s.resolveDecisionAsPolicy(ctx, input.TenantID, input.ProjectID, *gate.DecisionRequestID, ruleID, actorID, "approved", autonomyTierFullAuto)
	}
	return s.maybeRejectExternalHumanGate(ctx, input.TenantID, input.ProjectID, *gate.DecisionRequestID, demand, ruleID, actorID)
}

// maybePolicyAutoResolvePlanReview auto-approves plan_review when the demand's
// invoker is full_auto. External demands that are not full_auto are
// policy-rejected instead of parking (spec §4.4 / §10).
func (s *ProjectStore) maybePolicyAutoResolvePlanReview(ctx context.Context, input RequestPlanRevisionReviewInput, decisionID uuid.UUID) error {
	if !s.policyAutoResolveConfigured() {
		return nil
	}
	if decisionID == uuid.Nil {
		return nil
	}
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, input.DemandID)
	if err != nil {
		return fmt.Errorf("load demand for plan_review policy autonomy: %w", err)
	}
	ruleID, actorID, ok, err := s.lookupFullAutoPolicy(ctx, input.TenantID, demand)
	if err != nil {
		return err
	}
	if ok {
		return s.resolveDecisionAsPolicy(ctx, input.TenantID, input.ProjectID, decisionID, ruleID, actorID, project.PlanReviewDecisionAccept, autonomyTierFullAuto)
	}
	return s.maybeRejectExternalHumanGate(ctx, input.TenantID, input.ProjectID, decisionID, demand, ruleID, actorID)
}

// maybePolicyAutoResolveDemandAcceptance auto-approves a freshly minted
// demand_acceptance decision when the demand is full_auto AND the acceptance
// evidence gate (E1–E6) passes. pause_at_gate / failed evidence parks for humans.
func (s *ProjectStore) maybePolicyAutoResolveDemandAcceptance(ctx context.Context, tenantID, projectID, demandID, decisionID uuid.UUID) error {
	if !s.policyAutoResolveConfigured() {
		return nil
	}
	if decisionID == uuid.Nil {
		return nil
	}
	demand, err := s.repository.GetProjectDemand(ctx, tenantID, demandID)
	if err != nil {
		return fmt.Errorf("load demand for demand_acceptance policy autonomy: %w", err)
	}
	// Prefer create-time snapshot; fall back to live lookup for in-flight demands
	// created before F4 snapshotting.
	snapshot := project.DemandAutonomyTierSnapshot(demand)
	ruleID, actorID, ok, err := s.lookupFullAutoPolicy(ctx, tenantID, demand)
	if err != nil {
		return err
	}
	if snapshot != "" && snapshot != autonomyTierFullAuto {
		ok = false
	}
	if !ok {
		return s.maybeRejectExternalHumanGate(ctx, tenantID, projectID, decisionID, demand, ruleID, actorID)
	}
	revisions, err := s.repository.ListPlanRevisionsForDemand(ctx, tenantID, projectID, demandID)
	if err != nil {
		return err
	}
	revisionID := project.CurrentEffectivePlanRevisionID(revisions)
	if revisionID == uuid.Nil {
		return nil
	}
	criteria, err := s.repository.ListDemandAcceptanceCriteria(ctx, tenantID, demandID, revisionID)
	if err != nil {
		return err
	}
	verdicts, err := s.repository.ListDemandCriterionVerdicts(ctx, tenantID, demandID, revisionID)
	if err != nil {
		return err
	}
	if reason := project.EvaluateAcceptanceEvidenceGate(criteria, verdicts); reason != project.AcceptanceEvidencePass {
		return nil
	}
	return s.resolveDecisionAsPolicy(ctx, tenantID, projectID, decisionID, ruleID, actorID, "approved", autonomyTierFullAuto)
}

// maybeRejectExternalHumanGate closes a minted gate for external API demands
// when Effective ≠ full_auto. Automation pause_at_gate still parks as before.
func (s *ProjectStore) maybeRejectExternalHumanGate(
	ctx context.Context,
	tenantID, projectID, decisionID uuid.UUID,
	demand project.ProjectDemand,
	policyID, actorUserID uuid.UUID,
) error {
	if demand.SourceType != project.DemandSourceExternal {
		return nil
	}
	if actorUserID == uuid.Nil || policyID == uuid.Nil || decisionID == uuid.Nil {
		return nil
	}
	return s.resolveDecisionAsPolicy(
		ctx, tenantID, projectID, decisionID, policyID, actorUserID,
		project.PlanReviewDecisionReject,
		"pause_at_gate",
	)
}
