package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/scenariotemplate"
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
	summary := strings.TrimSpace(req.Reason)
	if summary == "" {
		if roleKey != "" {
			summary = fmt.Sprintf("提请扩编角色 %s", roleKey)
		} else {
			summary = "提请扩编（词表外角色，需人工翻译）"
		}
	}
	title := "扩编请求"
	payload := map[string]any{
		"decision_type":         DecisionTypeCastingExpansion,
		"demand_id":             req.DemandID.String(),
		"scenario_template_key": templateKey,
		"suggested_role_key":    roleKey,
		"needs_external_role":   req.NeedsExternalRole,
		"reason":                summary,
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
		RequesterType:  defaultString(req.ActorType, "system"),
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
		ActorType: defaultString(req.ActorType, "system"),
		ActorID:   defaultString(req.ActorID, "coordinator"),
		Summary:   summary,
		Payload: map[string]any{
			"approval_request_id": approvalRequestID.String(),
			"decision_type":       DecisionTypeCastingExpansion,
			"demand_id":           req.DemandID.String(),
		},
	})
	if err != nil {
		return nil, err
	}
	decision, err := s.repository.CreateDecisionRequest(ctx, CreateDecisionRequestRequest{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: approvalRequestID,
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
			return nil, err
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
	roleKey, err := s.nextUncastRoleForTemplate(ctx, tenantID, projectID, templateKey)
	if err != nil {
		return MaybeCastingExpansionResult{}, err
	}
	if roleKey == "" {
		return MaybeCastingExpansionResult{
			DemandID:      demandID,
			SkippedReason: "casting_complete",
		}, nil
	}

	decision, err := s.RequestCastingExpansion(ctx, RequestCastingExpansionRequest{
		TenantID:            tenantID,
		ProjectID:           projectID,
		DemandID:            demandID,
		SuggestedRoleKey:    roleKey,
		Reason:              fmt.Sprintf("协调线程：任务完成后可达收口仍缺角色 %s，提请扩编", roleKey),
		ScenarioTemplateKey: templateKey,
		ActorType:           "coordinator",
		ActorID:             "project-coordinator",
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
