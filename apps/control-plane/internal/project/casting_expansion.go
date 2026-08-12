package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/scenariotemplate"
	"github.com/superteam/control-plane/internal/systemconfig"
)

// RequestCastingExpansionRequest opens a casting_expansion decision for a
// demand mid-execution. suggested_role_key must be an active role vocabulary
// key (or empty with needs_external_role=true for out-of-vocab natural language).
type RequestCastingExpansionRequest struct {
	TenantID   uuid.UUID
	ProjectID  uuid.UUID
	DemandID   uuid.UUID
	// SuggestedRoleKey from judge constrained output; empty if out-of-vocab.
	SuggestedRoleKey string
	// NeedsExternalRole marks that the judge could not map to vocabulary.
	NeedsExternalRole bool
	// Reason natural language from judge/coordinator.
	Reason string
	// ScenarioTemplateKey for the casting row to extend.
	ScenarioTemplateKey string
	// ActorType "system" | "coordinator" | "judge"
	ActorType string
	ActorID   string
	// ProjectTaskID 是触发本次提请的那个已完成任务。**必须带**:一单卷宗的决策
	// 读路径按 coordination_job_id / project_task_id 反查(ListDemandLaunchDecisionRequests),
	// 两者都空的决策在这一单里根本找不回来 —— 扩编卡不会出现在待办,时间线上也
	// 只剩通用「待人工决策」。人工发起(无来源任务)时才允许为空。
	ProjectTaskID *uuid.UUID
}

// RequestCastingExpansion creates a human decision (approval + decision_request
// + inbox) for mid-execution casting expansion. Does not change demand status.
func (s *Service) RequestCastingExpansion(ctx context.Context, req RequestCastingExpansionRequest) (*DecisionRequest, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.DemandID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	roleKey := strings.TrimSpace(req.SuggestedRoleKey)
	if !req.NeedsExternalRole {
		if roleKey == "" {
			return nil, fmt.Errorf("%w: suggested_role_key is required unless needs_external_role", ErrInvalidProject)
		}
		if s.roleVocabulary != nil {
			unknown, err := s.roleVocabulary.UnknownKeys(ctx, req.TenantID, []string{roleKey})
			if err != nil {
				return nil, err
			}
			if len(unknown) > 0 {
				return nil, fmt.Errorf("%w: suggested_role_key %q not in role vocabulary", ErrInvalidProject, roleKey)
			}
		}
	}
	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	demand, err := s.repository.GetProjectDemand(ctx, req.TenantID, req.DemandID)
	if err != nil {
		return nil, err
	}
	if demand.ProjectID != req.ProjectID {
		return nil, ErrInvalidProject
	}
	templateKey := strings.TrimSpace(req.ScenarioTemplateKey)
	if templateKey == "" && demand.ScenarioTemplateKey != nil {
		templateKey = strings.TrimSpace(*demand.ScenarioTemplateKey)
	}
	if templateKey == "" {
		return nil, fmt.Errorf("%w: scenario_template_key is required for casting expansion", ErrInvalidProject)
	}
	roleLabel := s.lookupRoleDisplayLabel(ctx, req.TenantID, roleKey)
	summary := strings.TrimSpace(req.Reason)
	if summary == "" {
		if roleKey != "" {
			summary = fmt.Sprintf("提请扩编角色 %s", roleLabel)
		} else {
			summary = "提请扩编（词表外角色，需人工翻译）"
		}
	}
	title := "扩编请求"
	actorType := defaultString(req.ActorType, "system")
	payload := map[string]any{
		"decision_type":         DecisionTypeCastingExpansion,
		"demand_id":             req.DemandID.String(),
		"scenario_template_key": templateKey,
		"suggested_role_key":    roleKey,
		"needs_external_role":   req.NeedsExternalRole,
		"reason":                summary,
		// actor_type distinguishes coordinator (deterministic template gap) vs
		// judge (semantic discoverer) so the inbox card can present differently.
		"actor_type": actorType,
	}
	targetUser := projectRecord.HumanOwnerUserID
	if len(projectRecord.HumanOwnerUserIDs) > 0 {
		targetUser = projectRecord.HumanOwnerUserIDs[0]
	}

	if s.approvals == nil {
		return nil, fmt.Errorf("approval resolver not configured")
	}
	approvalRequestID, err := s.approvals.CreateRequest(ctx, CreateApprovalRequestInput{
		TenantID:       req.TenantID,
		ResourceType:   "project",
		ResourceID:     req.ProjectID,
		RequesterType:  actorType,
		TargetUserID:   targetUser,
		DecisionType:   DecisionTypeCastingExpansion,
		Title:          title,
		Summary:        summary,
		RiskLevel:      "medium",
		Options:        []any{"approved", "rejected"},
		ContextPayload: payload,
	})
	if err != nil {
		return nil, err
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventDecisionRequested,
		ActorType: actorType,
		ActorID:   defaultString(req.ActorID, "coordinator"),
		Summary:   summary,
		Payload: map[string]any{
			"approval_request_id": approvalRequestID.String(),
			"decision_type":       DecisionTypeCastingExpansion,
			"demand_id":           req.DemandID.String(),
			"actor_type":          actorType,
		},
	})
	if err != nil {
		return nil, err
	}
	decision, err := s.repository.CreateDecisionRequest(ctx, CreateDecisionRequestRequest{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: approvalRequestID,
		ProjectTaskID:     req.ProjectTaskID,
		TargetUserID:      targetUser,
		DecisionType:      DecisionTypeCastingExpansion,
		TitleSnapshot:     title,
		SummarySnapshot:   summary,
		RiskLevelSnapshot: "medium",
		StatusSnapshot:    "pending",
		CreatedEventID:    &event.ID,
	})
	if err != nil {
		return nil, err
	}
	decision.InboxContext = payload
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return nil, s.compensateDecisionInboxProjectionFailure(ctx, decision, err)
		}
	}
	return &decision, nil
}

// MaybeCastingExpansionResult is the coordinator outcome after an accepted task
// completion: either an open casting_expansion decision or a structured skip.
type MaybeCastingExpansionResult struct {
	Requested        bool
	DecisionID       uuid.UUID
	DemandID         uuid.UUID
	SuggestedRoleKey string
	SkippedReason    string
}

// MaybeRequestCastingExpansionForCompletedTask is the coordinator path for
// design §7.1 / G9: after any accepted task completion, if playbook readiness
// still needs roles for a deeper exit, open casting_expansion with a
// vocabulary-constrained suggested_role_key (NextExitNeedsRoles / template
// roles — never free text). Idempotent when a pending expansion already exists
// for the demand. Does not change demand status.
func (s *Service) MaybeRequestCastingExpansionForCompletedTask(ctx context.Context, tenantID, projectID, completedTaskID uuid.UUID) (MaybeCastingExpansionResult, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil || completedTaskID == uuid.Nil {
		return MaybeCastingExpansionResult{SkippedReason: "invalid_ids"}, nil
	}
	if s.scenarioTemplateSpecs == nil || s.castingRepo == nil {
		return MaybeCastingExpansionResult{SkippedReason: "casting_deps_unwired"}, nil
	}
	task, err := s.repository.GetProjectTask(ctx, tenantID, completedTaskID)
	if err != nil {
		return MaybeCastingExpansionResult{}, err
	}
	if task.ProjectID != projectID {
		return MaybeCastingExpansionResult{SkippedReason: "task_project_mismatch"}, nil
	}
	if task.DemandID == nil || *task.DemandID == uuid.Nil {
		return MaybeCastingExpansionResult{SkippedReason: "no_demand"}, nil
	}
	demandID := *task.DemandID
	demand, err := s.repository.GetProjectDemand(ctx, tenantID, demandID)
	if err != nil {
		return MaybeCastingExpansionResult{}, err
	}
	if demand.ProjectID != projectID {
		return MaybeCastingExpansionResult{SkippedReason: "demand_project_mismatch"}, nil
	}
	switch demand.Status {
	case ProjectDemandStatusExecuting, ProjectDemandStatusAcceptancePending:
		// Mid-execution or at acceptance gate — both may still expand (§7.1).
	case ProjectDemandStatusCompleted:
		// Last-task race: writeback recompute can terminalize a shallow single-task
		// demand before the coordinator activity runs. Still allow expansion when
		// casting can deepen (design: 任一任务完成后即可提请).
	default:
		return MaybeCastingExpansionResult{
			DemandID:      demandID,
			SkippedReason: "demand_status_" + string(demand.Status),
		}, nil
	}
	templateKey := ""
	if demand.ScenarioTemplateKey != nil {
		templateKey = strings.TrimSpace(*demand.ScenarioTemplateKey)
	}
	if templateKey == "" {
		return MaybeCastingExpansionResult{DemandID: demandID, SkippedReason: "no_template"}, nil
	}

	// Idempotent: one open casting_expansion per demand.
	if open, err := s.hasOpenCastingExpansionForDemand(ctx, tenantID, projectID, demandID); err != nil {
		return MaybeCastingExpansionResult{}, err
	} else if open {
		return MaybeCastingExpansionResult{
			DemandID:      demandID,
			SkippedReason: "already_pending",
		}, nil
	}

	// Casting-only next roles (not pool/tenant feasibility from GetPlaybookReadiness).
	// Expansion is about 编制 gaps: pool may already hold reviewer/tester, but
	// without cast rows the plan cannot deepen (§7 / G9).
	//
	// Division of labour (design 2026-08-05 §3.3):
	//   - casting incomplete → deterministic coordinator proposal (batch 2)
	//   - casting complete   → semantic gap discoverer (batch 3), if wired
	roleKey, err := s.nextUncastRoleForTemplate(ctx, tenantID, projectID, templateKey)
	if err != nil {
		return MaybeCastingExpansionResult{}, err
	}
	if roleKey == "" {
		return s.maybeDiscoverCastingGap(ctx, tenantID, projectID, demandID, completedTaskID, templateKey, task)
	}

	decision, err := s.RequestCastingExpansion(ctx, RequestCastingExpansionRequest{
		TenantID:            tenantID,
		ProjectID:           projectID,
		DemandID:            demandID,
		SuggestedRoleKey:    roleKey,
		Reason:              fmt.Sprintf("协调线程：任务完成后可达收口仍缺角色 %s，提请扩编", s.lookupRoleDisplayLabel(ctx, tenantID, roleKey)),
		ScenarioTemplateKey: templateKey,
		ActorType:           "coordinator",
		ActorID:             "project-coordinator",
		ProjectTaskID:       &completedTaskID,
	})
	if err != nil {
		return MaybeCastingExpansionResult{}, err
	}
	return MaybeCastingExpansionResult{
		Requested:        true,
		DecisionID:       decision.ID,
		DemandID:         demandID,
		SuggestedRoleKey: roleKey,
	}, nil
}

// maybeDiscoverCastingGap runs the semantic discoverer when playbook casting is
// already complete. Never fails the task graph: transport/discoverer errors are
// returned so the activity can log+swallow; parse/R2 soft outcomes skip quietly.
func (s *Service) maybeDiscoverCastingGap(
	ctx context.Context,
	tenantID, projectID, demandID, completedTaskID uuid.UUID,
	templateKey string,
	task ProjectTask,
) (MaybeCastingExpansionResult, error) {
	base := MaybeCastingExpansionResult{DemandID: demandID, SkippedReason: "casting_complete"}
	if s.castingGapDiscoverer == nil {
		return MaybeCastingExpansionResult{DemandID: demandID, SkippedReason: "discoverer_unwired"}, nil
	}
	if s.roleVocabularyLister == nil {
		return MaybeCastingExpansionResult{DemandID: demandID, SkippedReason: "role_vocab_lister_unwired"}, nil
	}

	maxCalls := s.castingGapDiscoveryMaxPerDemand(ctx, tenantID)
	if maxCalls <= 0 {
		return MaybeCastingExpansionResult{DemandID: demandID, SkippedReason: "discoverer_disabled"}, nil
	}
	callCount, alreadyLimited, err := s.countCastingGapDiscoveries(ctx, tenantID, projectID, demandID)
	if err != nil {
		return MaybeCastingExpansionResult{}, err
	}
	if callCount >= maxCalls {
		if !alreadyLimited {
			// H9c: one visible timeline mark when the budget first exhausts.
			_, _ = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
				TenantID:  tenantID,
				ProjectID: projectID,
				EventType: ProjectEventCastingGapDiscovery,
				ActorType: "judge",
				ActorID:   "casting-gap-discoverer",
				Summary:   "语义扩编发现次数已达上限，本单不再自动提请",
				Payload: map[string]any{
					"demand_id":           demandID.String(),
					"completed_task_id":   completedTaskID.String(),
					"outcome":             "limit_reached",
					"call_count":          callCount,
					"max_calls":           maxCalls,
					"scenario_template_key": templateKey,
				},
			})
		}
		return MaybeCastingExpansionResult{DemandID: demandID, SkippedReason: "discoverer_limit_reached"}, nil
	}

	rows, err := s.roleVocabularyLister.ListActiveRoleRows(ctx, tenantID)
	if err != nil {
		return MaybeCastingExpansionResult{}, err
	}
	if len(rows) == 0 {
		return MaybeCastingExpansionResult{DemandID: demandID, SkippedReason: "no_active_roles"}, nil
	}
	activeRoles := make([]CastingGapRoleOption, 0, len(rows))
	for _, r := range rows {
		activeRoles = append(activeRoles, CastingGapRoleOption{
			RoleKey:     r.RoleKey,
			Title:       r.Title,
			Description: r.Description,
		})
	}

	participating, err := s.participatingRoleKeys(ctx, tenantID, projectID, templateKey)
	if err != nil {
		return MaybeCastingExpansionResult{}, err
	}

	conclusion, deliverables := s.taskOutputForGapDiscovery(ctx, tenantID, projectID, completedTaskID)

	suggestion, err := s.castingGapDiscoverer.DiscoverCastingGap(ctx, CastingGapInput{
		TaskTitle:          task.Title,
		ConclusionSummary:  conclusion,
		DeliverableNames:   deliverables,
		ActiveRoles:        activeRoles,
		ParticipatingRoles: participating,
	})
	if err != nil {
		return MaybeCastingExpansionResult{}, err
	}

	// Record every model invocation (including not-needed) for the call budget.
	outcome := "not_needed"
	if suggestion.Needed {
		if suggestion.External {
			outcome = "external"
		} else {
			outcome = "needed"
		}
	}
	_, _ = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventCastingGapDiscovery,
		ActorType: "judge",
		ActorID:   "casting-gap-discoverer",
		Summary:   castingGapDiscoverySummary(outcome, suggestion, s.lookupRoleDisplayLabel(ctx, tenantID, suggestion.RoleKey)),
		Payload: map[string]any{
			"demand_id":             demandID.String(),
			"completed_task_id":     completedTaskID.String(),
			"outcome":               outcome,
			"suggested_role_key":    suggestion.RoleKey,
			"needs_external_role":   suggestion.External,
			"reason":                suggestion.Reason,
			"scenario_template_key": templateKey,
		},
	})

	if !suggestion.Needed {
		return base, nil
	}

	reason := strings.TrimSpace(suggestion.Reason)
	if reason == "" {
		if suggestion.External {
			reason = "根据产出判断需要词表外的角色参与"
		} else {
			reason = fmt.Sprintf("根据产出判断还需要角色 %s", s.lookupRoleDisplayLabel(ctx, tenantID, suggestion.RoleKey))
		}
	}

	decision, err := s.RequestCastingExpansion(ctx, RequestCastingExpansionRequest{
		TenantID:            tenantID,
		ProjectID:           projectID,
		DemandID:            demandID,
		SuggestedRoleKey:    suggestion.RoleKey,
		NeedsExternalRole:   suggestion.External,
		Reason:              reason,
		ScenarioTemplateKey: templateKey,
		ActorType:           "judge",
		ActorID:             "casting-gap-discoverer",
		ProjectTaskID:       &completedTaskID,
	})
	if err != nil {
		// R1 demotion already applied in the discoverer; if Request still
		// rejects (e.g. race on vocab disable), surface as skip not hard fail
		// so the task graph continues.
		if strings.Contains(err.Error(), "not in role vocabulary") {
			return MaybeCastingExpansionResult{DemandID: demandID, SkippedReason: "suggested_role_rejected"}, nil
		}
		return MaybeCastingExpansionResult{}, err
	}
	return MaybeCastingExpansionResult{
		Requested:        true,
		DecisionID:       decision.ID,
		DemandID:         demandID,
		SuggestedRoleKey: suggestion.RoleKey,
	}, nil
}

func castingGapDiscoverySummary(outcome string, suggestion CastingGapSuggestion, roleLabel string) string {
	if strings.TrimSpace(roleLabel) == "" {
		roleLabel = roleKeyDisplayLabel(suggestion.RoleKey, "")
	}
	switch outcome {
	case "needed":
		return fmt.Sprintf("语义扩编发现：建议补充角色 %s", roleLabel)
	case "external":
		return "语义扩编发现：需要词表外角色"
	case "not_needed":
		return "语义扩编发现：无需补人"
	default:
		return "语义扩编发现"
	}
}

// roleKeyDisplayLabel prefers Chinese title with key in parentheses for inbox
// summaries; falls back to bare key only when title is empty.
func roleKeyDisplayLabel(roleKey, title string) string {
	key := strings.TrimSpace(roleKey)
	name := strings.TrimSpace(title)
	if name != "" && key != "" && !strings.EqualFold(name, key) {
		return fmt.Sprintf("%s（%s）", name, key)
	}
	if name != "" {
		return name
	}
	return key
}

func (s *Service) lookupRoleDisplayLabel(ctx context.Context, tenantID uuid.UUID, roleKey string) string {
	key := strings.TrimSpace(roleKey)
	if key == "" {
		return ""
	}
	if s != nil && s.roleVocabularyLister != nil {
		rows, err := s.roleVocabularyLister.ListActiveRoleRows(ctx, tenantID)
		if err == nil {
			for _, row := range rows {
				if strings.TrimSpace(row.RoleKey) == key {
					return roleKeyDisplayLabel(key, row.Title)
				}
			}
		}
	}
	return key
}

func (s *Service) castingGapDiscoveryMaxPerDemand(ctx context.Context, tenantID uuid.UUID) int {
	if s.systemConfig == nil {
		return 3
	}
	v := s.systemConfig.Int64(ctx, tenantID, systemconfig.KeyCastingGapDiscoveryMaxPerDemand)
	if v < 0 {
		return 0
	}
	if v > 20 {
		return 20
	}
	return int(v)
}

// countCastingGapDiscoveries returns (modelCallCount, alreadyHasLimitReachedMark).
// limit_reached events do not count toward the model budget.
func (s *Service) countCastingGapDiscoveries(ctx context.Context, tenantID, projectID, demandID uuid.UUID) (int, bool, error) {
	events, err := s.repository.ListProjectEvents(ctx, tenantID, projectID, 500, 0)
	if err != nil {
		return 0, false, err
	}
	demandStr := demandID.String()
	calls := 0
	limited := false
	for _, ev := range events {
		if ev.EventType != ProjectEventCastingGapDiscovery {
			continue
		}
		payload := mapOrEmptyAny(ev.Payload)
		if strings.TrimSpace(stringFromAny(payload["demand_id"])) != demandStr {
			continue
		}
		outcome := strings.TrimSpace(stringFromAny(payload["outcome"]))
		if outcome == "limit_reached" {
			limited = true
			continue
		}
		// Any non-limit event is a model invocation (needed / not_needed / external).
		calls++
	}
	return calls, limited, nil
}

func (s *Service) participatingRoleKeys(ctx context.Context, tenantID, projectID uuid.UUID, templateKey string) ([]string, error) {
	if s.castingRepo == nil {
		return nil, nil
	}
	castings, err := s.castingRepo.ListProjectCastings(ctx, tenantID, projectID, &templateKey)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(castings))
	seen := map[string]bool{}
	for _, c := range castings {
		role := strings.TrimSpace(c.RoleKey)
		if role == "" || c.DigitalEmployeeID == uuid.Nil || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out, nil
}

// taskOutputForGapDiscovery loads a short conclusion + deliverable names for the
// discoverer prompt (design open detail #3: no full artifact bodies).
func (s *Service) taskOutputForGapDiscovery(ctx context.Context, tenantID, projectID, taskID uuid.UUID) (string, []string) {
	conclusion := ""
	if summaries, err := s.repository.ListExecutionSummariesByTaskIDs(ctx, tenantID, projectID, []uuid.UUID{taskID}); err == nil {
		for _, sum := range summaries {
			if strings.TrimSpace(sum.Conclusion) != "" {
				conclusion = strings.TrimSpace(sum.Conclusion)
				break
			}
		}
	}
	var deliverables []string
	if results, err := s.repository.ListProjectTaskResults(ctx, ListProjectTaskResultsRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskID,
		Limit:         5,
		Offset:        0,
	}); err == nil {
		for _, r := range results {
			for _, d := range r.Contract.Deliverables {
				name := strings.TrimSpace(d.Name)
				if name != "" {
					deliverables = append(deliverables, name)
				}
			}
		}
	}
	// Bound prompt size.
	if len(conclusion) > 2000 {
		conclusion = conclusion[:2000] + "…"
	}
	if len(deliverables) > 20 {
		deliverables = deliverables[:20]
	}
	return conclusion, deliverables
}

func firstCastingExpansionRoleKey(candidates []string) string {
	for _, c := range candidates {
		key := strings.TrimSpace(c)
		// ValidatePlaybookCastingComplete may annotate "role (员工不可用)".
		if i := strings.Index(key, " ("); i > 0 {
			key = strings.TrimSpace(key[:i])
		}
		if key != "" {
			return key
		}
	}
	return ""
}

// nextUncastRoleForTemplate returns the first role required by the shallowest
// exit that is not fully cast, walking exits shallow→deep. Empty means every
// skeleton role for all exits is already cast (or template has no roles).
func (s *Service) nextUncastRoleForTemplate(ctx context.Context, tenantID, projectID uuid.UUID, templateKey string) (string, error) {
	if s.scenarioTemplateSpecs == nil || s.castingRepo == nil {
		return "", nil
	}
	spec, _, err := s.scenarioTemplateSpecs.GetParsedSpec(ctx, tenantID, templateKey)
	if err != nil {
		return "", err
	}
	castings, err := s.castingRepo.ListProjectCastings(ctx, tenantID, projectID, &templateKey)
	if err != nil {
		return "", err
	}
	cast := map[string]bool{}
	for _, c := range castings {
		role := strings.TrimSpace(c.RoleKey)
		if role != "" && c.DigitalEmployeeID != uuid.Nil {
			cast[role] = true
		}
	}
	// Walk exits in template order (shallow → deep).
	exits := spec.Exits
	if len(exits) == 0 {
		for _, role := range distinctRolesFromSteps(spec.Skeleton) {
			if !cast[role] {
				return role, nil
			}
		}
		return "", nil
	}
	for _, exit := range exits {
		steps, err := scenariotemplate.PruneSkeletonForExit(spec, exit.Deliverable)
		if err != nil {
			continue
		}
		for _, role := range distinctRolesFromSteps(steps) {
			if !cast[role] {
				return role, nil
			}
		}
	}
	return "", nil
}

func (s *Service) hasOpenCastingExpansionForDemand(ctx context.Context, tenantID, projectID, demandID uuid.UUID) (bool, error) {
	decisions, err := s.repository.ListDecisionRequests(ctx, tenantID, projectID, 200, 0)
	if err != nil {
		return false, err
	}
	demandStr := demandID.String()
	for _, d := range decisions {
		if d.DecisionType != DecisionTypeCastingExpansion {
			continue
		}
		if strings.TrimSpace(d.StatusSnapshot) != "pending" {
			continue
		}
		payload := map[string]any{}
		if s.approvals != nil && d.ApprovalRequestID != uuid.Nil {
			if ctxPayload, err := s.approvals.GetRequestContextPayload(ctx, tenantID, d.ApprovalRequestID); err == nil {
				payload = mapOrEmptyAny(ctxPayload)
			}
		}
		if strings.TrimSpace(stringFromAny(payload["demand_id"])) == demandStr {
			return true, nil
		}
	}
	return false, nil
}

// applyCastingExpansionApproval writes the human-selected employee into casting
// and appends an event that the coordinator can observe to replan. Demand status
// stays executing (§7.3).
func (s *Service) applyCastingExpansionApproval(ctx context.Context, req ResolveDecisionRequest, decision DecisionRequest) error {
	payload := map[string]any{}
	if s.approvals != nil && decision.ApprovalRequestID != uuid.Nil {
		if ctxPayload, err := s.approvals.GetRequestContextPayload(ctx, req.TenantID, decision.ApprovalRequestID); err == nil {
			payload = mapOrEmptyAny(ctxPayload)
		}
	}
	if len(payload) == 0 && decision.InboxContext != nil {
		payload = mapOrEmptyAny(decision.InboxContext)
	}
	// Prefer resolve-time selection over the original suggestion.
	roleKey := strings.TrimSpace(stringFromAny(req.Payload["role_key"]))
	if roleKey == "" {
		roleKey = strings.TrimSpace(stringFromAny(payload["suggested_role_key"]))
	}
	employeeIDStr := strings.TrimSpace(stringFromAny(req.Payload["digital_employee_id"]))
	if employeeIDStr == "" {
		employeeIDStr = strings.TrimSpace(stringFromAny(req.Payload["employee_id"]))
	}
	if roleKey == "" || employeeIDStr == "" {
		return fmt.Errorf("%w: casting expansion approve requires role_key and digital_employee_id", ErrInvalidProject)
	}
	employeeID, err := uuid.Parse(employeeIDStr)
	if err != nil {
		return fmt.Errorf("%w: invalid digital_employee_id", ErrInvalidProject)
	}
	templateKey := strings.TrimSpace(stringFromAny(payload["scenario_template_key"]))
	if templateKey == "" {
		return fmt.Errorf("%w: casting expansion missing scenario_template_key", ErrInvalidProject)
	}

	existing, err := s.ListCastings(ctx, req.TenantID, req.ProjectID, templateKey)
	if err != nil {
		return err
	}
	assignments := make([]CastingAssignment, 0, len(existing)+1)
	replaced := false
	for _, e := range existing {
		if e.RoleKey == roleKey {
			assignments = append(assignments, CastingAssignment{RoleKey: roleKey, DigitalEmployeeID: employeeID})
			replaced = true
			continue
		}
		assignments = append(assignments, CastingAssignment{RoleKey: e.RoleKey, DigitalEmployeeID: e.DigitalEmployeeID})
	}
	if !replaced {
		assignments = append(assignments, CastingAssignment{RoleKey: roleKey, DigitalEmployeeID: employeeID})
	}
	if _, err := s.PutCasting(ctx, PutCastingRequest{
		TenantID:            req.TenantID,
		ProjectID:           req.ProjectID,
		ActorUserID:         req.DecidedByUserID,
		ScenarioTemplateKey: templateKey,
		Assignments:         assignments,
	}); err != nil {
		return err
	}

	demandIDStr := strings.TrimSpace(stringFromAny(payload["demand_id"]))
	_, err = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventConfigChanged,
		ActorType: "human_user",
		ActorID:   req.DecidedByUserID.String(),
		Summary:   fmt.Sprintf("扩编已批准：%s", roleKey),
		Payload: map[string]any{
			"event":                 "project.casting.expansion_approved",
			"decision_request_id":   decision.ID.String(),
			"demand_id":             demandIDStr,
			"scenario_template_key": templateKey,
			"role_key":              roleKey,
			"digital_employee_id":   employeeID.String(),
			// Coordinator: replan without demand status rollback; merge completed
			// tasks by planned_task_key (investigation §7.3 / G10).
			"replan_required": true,
		},
	})
	return err
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}
