package projectcoordination

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

// RecoveryAutoRecheckLister lists pending recovery-family decisions for F7.
type RecoveryAutoRecheckLister interface {
	ListPendingRecoveryDecisionsForAutoRecheck(ctx context.Context, limit int32) ([]project.DecisionRequest, error)
}

// SweepAutoRecheckRecoveryGates probes pending runtime/dispatch recovery (and
// budget token-exhaustion) decisions for full_auto demands. When the server-
// side fact has healed, it resolves with resolved_by=policy:{id} and
// auto_recheck=true — no human click, no blind sign while still broken.
func (s *ProjectStore) SweepAutoRecheckRecoveryGates(ctx context.Context, limit int32) (int, error) {
	if s == nil || s.repository == nil || !s.policyAutoResolveConfigured() {
		return 0, nil
	}
	lister, ok := s.repository.(RecoveryAutoRecheckLister)
	if !ok {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	pending, err := lister.ListPendingRecoveryDecisionsForAutoRecheck(ctx, limit)
	if err != nil {
		return 0, err
	}
	released := 0
	for _, decision := range pending {
		ok, err := s.maybeAutoRecheckOneRecoveryDecision(ctx, decision)
		if err != nil {
			slog.Warn("auto_recheck recovery gate failed",
				"decision_id", decision.ID.String(),
				"decision_type", decision.DecisionType,
				"error", err)
			continue
		}
		if ok {
			released++
		}
	}
	return released, nil
}

func (s *ProjectStore) maybeAutoRecheckOneRecoveryDecision(ctx context.Context, decision project.DecisionRequest) (bool, error) {
	if decision.ProjectTaskID == nil || *decision.ProjectTaskID == uuid.Nil {
		return false, nil
	}
	task, err := s.repository.GetProjectTask(ctx, decision.TenantID, *decision.ProjectTaskID)
	if err != nil {
		return false, err
	}
	if task.DemandID == nil || *task.DemandID == uuid.Nil {
		return false, nil
	}
	demand, err := s.repository.GetProjectDemand(ctx, decision.TenantID, *task.DemandID)
	if err != nil {
		return false, err
	}
	ruleID, actorID, fullAuto, err := s.lookupFullAutoPolicy(ctx, decision.TenantID, demand)
	if err != nil {
		return false, err
	}
	snapshot := project.DemandAutonomyTierSnapshot(demand)
	if snapshot != "" && snapshot != autonomyTierFullAuto {
		return false, nil
	}
	if !fullAuto {
		if snapshot != autonomyTierFullAuto {
			return false, nil
		}
		if actorID == uuid.Nil {
			actorID = demand.SubmittedByUserID
		}
		if ruleID == uuid.Nil {
			ruleID, _ = automationRuleIDFromDemand(demand)
			if ruleID == uuid.Nil {
				ruleID, _ = externalIntegrationIDFromDemand(demand)
			}
		}
	}

	healed, err := s.probeRecoveryGateHealed(ctx, decision)
	if err != nil || !healed {
		return false, err
	}
	if ruleID == uuid.Nil || actorID == uuid.Nil {
		return false, nil
	}
	err = s.resolveDecisionAsPolicyWithRecheck(ctx, decision.TenantID, decision.ProjectID, decision.ID, ruleID, actorID)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *ProjectStore) probeRecoveryGateHealed(ctx context.Context, decision project.DecisionRequest) (bool, error) {
	switch strings.TrimSpace(decision.DecisionType) {
	case "project_task_runtime_recovery", "project_task_recovery":
		projectRecord, err := s.repository.GetProject(ctx, decision.TenantID, decision.ProjectID)
		if err != nil {
			return false, err
		}
		return projectRecord.WorkspaceReadyStatus == project.WorkspaceReadyStatusReady, nil
	case "project_task_budget_approval":
		title := strings.TrimSpace(decision.TitleSnapshot)
		summary := ""
		if decision.SummarySnapshot != nil {
			summary = strings.TrimSpace(*decision.SummarySnapshot)
		}
		blob := strings.ToLower(title + " " + summary)
		if !strings.Contains(blob, "token") {
			return false, nil
		}
		projectRecord, err := s.repository.GetProject(ctx, decision.TenantID, decision.ProjectID)
		if err != nil {
			return false, err
		}
		if projectRecord.BudgetTokenLimit == nil || *projectRecord.BudgetTokenLimit <= 0 {
			return true, nil
		}
		consumed, err := s.repository.SumProjectConsumedTokens(ctx, decision.TenantID, decision.ProjectID)
		if err != nil {
			return false, err
		}
		return consumed < *projectRecord.BudgetTokenLimit, nil
	default:
		return false, nil
	}
}

func (s *ProjectStore) resolveDecisionAsPolicyWithRecheck(
	ctx context.Context,
	tenantID, projectID, decisionID, policyID, actorUserID uuid.UUID,
) error {
	resolvedBy := fmt.Sprintf("policy:%s", policyID.String())
	_, err := s.gateDecisionResolver.ResolveDecision(ctx, project.ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorUserID,
		Decision:          "approved",
		Comment:           resolvedBy,
		Payload: map[string]any{
			"resolved_by":   resolvedBy,
			"autonomy_tier": autonomyTierFullAuto,
			"auto_recheck":  true,
		},
		Channel: "policy",
	})
	return err
}
