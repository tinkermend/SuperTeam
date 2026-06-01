package authzcenter

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/authz"
)

type Service struct {
	repo       Repository
	authorizer authz.Authorizer
	now        func() time.Time
}

func NewService(repo Repository, authorizer authz.Authorizer) *Service {
	return &Service{
		repo:       repo,
		authorizer: authorizer,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) GetOverview(ctx context.Context) (Overview, error) {
	if s == nil || s.repo == nil {
		return Overview{}, ErrInvalidInput
	}
	since := s.now().Add(-24 * time.Hour)
	totals, err := s.repo.CountDecisionsSince(ctx, since)
	if err != nil {
		return Overview{}, err
	}
	topDenied, err := s.repo.ListTopDeniedActionsSince(ctx, since, 5)
	if err != nil {
		return Overview{}, err
	}
	recent, err := s.repo.ListDecisions(ctx, DecisionFilter{Limit: 10, Offset: 0})
	if err != nil {
		return Overview{}, err
	}
	return Overview{
		Engine: EngineStatus{
			Engine:        "db",
			Status:        "healthy",
			EngineVersion: "db-authorizer-v1",
		},
		Totals:           totals,
		TopDeniedActions: topDenied,
		RecentEvents:     recent,
	}, nil
}

func (s *Service) ListDecisions(ctx context.Context, filter DecisionFilter) ([]DecisionRecord, error) {
	if s == nil || s.repo == nil {
		return nil, ErrInvalidInput
	}
	filter.Limit, filter.Offset = normalizePagination(filter.Limit, filter.Offset)
	return s.repo.ListDecisions(ctx, filter)
}

func (s *Service) ListRuntimeScopes(ctx context.Context) ([]RuntimeScopeNodeRecord, error) {
	if s == nil || s.repo == nil {
		return nil, ErrInvalidInput
	}
	return s.repo.ListRuntimeScopeNodes(ctx)
}

func (s *Service) CreateRuntimeScope(ctx context.Context, actor Actor, input RuntimeScopeInput) (RuntimeScopeRecord, error) {
	if s == nil || s.repo == nil {
		return RuntimeScopeRecord{}, ErrInvalidInput
	}
	input.ScopeType = strings.TrimSpace(input.ScopeType)
	input.ScopeValue = strings.TrimSpace(input.ScopeValue)
	if err := validateRuntimeScopeInput(input); err != nil {
		return RuntimeScopeRecord{}, err
	}

	decision, err := s.authorizeRuntimeScopeManage(ctx, actor, input.TenantID)
	if err != nil {
		s.recordRuntimeScopeOperation(ctx, actor, input.TenantID, "", OperationActionRuntimeScopeCreate, OperationResultFailed, input, map[string]any{"error": err.Error()})
		return RuntimeScopeRecord{}, err
	}
	if !decision.Allowed {
		s.recordRuntimeScopeOperation(ctx, actor, input.TenantID, "", OperationActionRuntimeScopeCreate, OperationResultFailed, input, map[string]any{"reason": decision.Reason})
		return RuntimeScopeRecord{}, ErrForbidden
	}

	scope, err := s.repo.CreateRuntimeScope(ctx, input)
	if err != nil {
		s.recordRuntimeScopeOperation(ctx, actor, input.TenantID, "", OperationActionRuntimeScopeCreate, OperationResultFailed, input, map[string]any{"error": err.Error()})
		return RuntimeScopeRecord{}, err
	}
	s.recordRuntimeScopeOperation(ctx, actor, input.TenantID, scope.ID.String(), OperationActionRuntimeScopeCreate, OperationResultSucceeded, input, map[string]any{"matched_rule": decision.MatchedRule})
	return scope, nil
}

func (s *Service) UpdateRuntimeScopeStatus(ctx context.Context, actor Actor, scopeID uuid.UUID, status string) (RuntimeScopeRecord, error) {
	if s == nil || s.repo == nil {
		return RuntimeScopeRecord{}, ErrInvalidInput
	}
	if scopeID == uuid.Nil {
		return RuntimeScopeRecord{}, ErrInvalidInput
	}
	if status != string(RuntimeScopeStatusActive) && status != string(RuntimeScopeStatusDisabled) {
		return RuntimeScopeRecord{}, ErrInvalidInput
	}

	decision, err := s.authorizeRuntimeScopeManage(ctx, actor, actor.TenantID)
	if err != nil {
		s.recordRuntimeScopeStatusOperation(ctx, actor, scopeID, status, OperationResultFailed, map[string]any{"error": err.Error()})
		return RuntimeScopeRecord{}, err
	}
	if !decision.Allowed {
		s.recordRuntimeScopeStatusOperation(ctx, actor, scopeID, status, OperationResultFailed, map[string]any{"reason": decision.Reason})
		return RuntimeScopeRecord{}, ErrForbidden
	}

	scope, err := s.repo.UpdateRuntimeScopeStatus(ctx, scopeID, status)
	if err != nil {
		s.recordRuntimeScopeStatusOperation(ctx, actor, scopeID, status, OperationResultFailed, map[string]any{"error": err.Error()})
		return RuntimeScopeRecord{}, err
	}
	s.recordRuntimeScopeStatusOperation(ctx, actor, scopeID, status, OperationResultSucceeded, map[string]any{"matched_rule": decision.MatchedRule})
	return scope, nil
}

func (s *Service) ListMembers(ctx context.Context, filter MemberFilter) ([]MemberRecord, error) {
	if s == nil || s.repo == nil {
		return nil, ErrInvalidInput
	}
	filter.Limit, filter.Offset = normalizePagination(filter.Limit, filter.Offset)
	return s.repo.ListMembers(ctx, filter)
}

func (s *Service) CheckPermission(ctx context.Context, input CheckPermissionInput) (authz.Decision, error) {
	if s == nil || s.authorizer == nil {
		return authz.Decision{}, ErrForbidden
	}
	return s.authorizer.Check(ctx, authz.CheckRequest{
		Actor:       input.Actor,
		Action:      input.Action,
		Resource:    input.Resource,
		TenantID:    input.TenantID,
		TeamID:      input.TeamID,
		AuditReason: "authz center dry-run",
	})
}

func (s *Service) authorizeRuntimeScopeManage(ctx context.Context, actor Actor, tenantID uuid.UUID) (authz.Decision, error) {
	if s.authorizer == nil {
		return authz.Decision{}, ErrForbidden
	}
	return s.authorizer.Check(ctx, authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   actor.UserID.String(),
		},
		Action: ActionRuntimeScopeManage,
		Resource: authz.ResourceRef{
			Type: authz.ResourceTenant,
			ID:   tenantID.String(),
		},
		TenantID:    tenantID,
		TeamID:      actor.TeamID,
		AuditReason: "authz center runtime scope manage",
	})
}

func (s *Service) recordRuntimeScopeOperation(ctx context.Context, actor Actor, tenantID uuid.UUID, scopeID, action, result string, input RuntimeScopeInput, extra map[string]any) {
	details := map[string]any{
		"runtime_node_id": input.RuntimeNodeID.String(),
		"scope_type":      input.ScopeType,
		"scope_value":     input.ScopeValue,
	}
	if input.TeamID != nil {
		details["team_id"] = input.TeamID.String()
	}
	for key, value := range extra {
		details[key] = value
	}
	_ = s.repo.RecordOperationLog(ctx, OperationLogInput{
		Actor:        actor,
		TenantID:     tenantID,
		Module:       OperationModuleAuthz,
		ResourceType: OperationResourceRuntimeNodeScope,
		ResourceID:   scopeID,
		Action:       action,
		Result:       result,
		Details:      details,
	})
}

func (s *Service) recordRuntimeScopeStatusOperation(ctx context.Context, actor Actor, scopeID uuid.UUID, status, result string, extra map[string]any) {
	details := map[string]any{"status": status}
	for key, value := range extra {
		details[key] = value
	}
	_ = s.repo.RecordOperationLog(ctx, OperationLogInput{
		Actor:        actor,
		TenantID:     actor.TenantID,
		Module:       OperationModuleAuthz,
		ResourceType: OperationResourceRuntimeNodeScope,
		ResourceID:   scopeID.String(),
		Action:       OperationActionRuntimeScopeUpdate,
		Result:       result,
		Details:      details,
	})
}

func validateRuntimeScopeInput(input RuntimeScopeInput) error {
	if input.TenantID == uuid.Nil || input.RuntimeNodeID == uuid.Nil {
		return ErrInvalidInput
	}
	if input.ScopeValue == "" {
		return ErrInvalidInput
	}
	switch input.ScopeType {
	case string(RuntimeScopeScopeTypeTenant):
		if input.TeamID != nil || input.ScopeValue != input.TenantID.String() {
			return ErrInvalidInput
		}
	case string(RuntimeScopeScopeTypeTeam):
		if input.TeamID == nil || input.ScopeValue != input.TeamID.String() {
			return ErrInvalidInput
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func normalizePagination(limit, offset int32) (int32, int32) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
