package teamlending

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/audit"
)

// AuditRecorder 记录借调审计事件（resource_type=team，action 以 team. 开头以进入团队审计流）。
type AuditRecorder interface {
	RecordEvent(ctx context.Context, event *audit.Event) error
}

// InboxProjector 把待裁决的借调请求投影到团队负责人 inbox（D3）。可为空（nil-safe）。
type InboxProjector interface {
	UpsertLendingRequest(ctx context.Context, item LendingInboxItem) error
	ResolveLendingRequest(ctx context.Context, item LendingInboxItem) error
}

// LendingInboxItem 借调 inbox 投影所需的最小信息。
type LendingInboxItem struct {
	TenantID     uuid.UUID
	TeamID       uuid.UUID
	RequestID    uuid.UUID
	ProjectID    uuid.UUID
	OwnerUserIDs []uuid.UUID
	Title        string
	Summary      string
	RiskLevel    string
}

// Service 团队借调业务服务：策略管理 + 请求生命周期（auto/manual + 超纲转人工）+ 审计/inbox。
type Service struct {
	repository Repository
	audit      AuditRecorder
	inbox      InboxProjector
}

// NewService 构造借调服务。audit 与 inbox 可为空（仅退化为不记审计/inbox，核心链路仍可用）。
func NewService(repository Repository, auditRecorder AuditRecorder, inboxProjector InboxProjector) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	return &Service{repository: repository, audit: auditRecorder, inbox: inboxProjector}, nil
}

// NewServiceWithoutAuditForTest 仅供测试：不记审计/inbox。
func NewServiceWithoutAuditForTest(repository Repository) (*Service, error) {
	return NewService(repository, nil, nil)
}

// GetPolicy 读取团队当前生效的借调策略。未配置返回 ErrPolicyNotFound。
func (s *Service) GetPolicy(ctx context.Context, tenantID, teamID uuid.UUID) (*Policy, error) {
	if tenantID == uuid.Nil || teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and team_id are required", ErrInvalidInput)
	}
	policy, err := s.repository.GetPolicy(ctx, tenantID, teamID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrPolicyNotFound
		}
		return nil, fmt.Errorf("get team lending policy: %w", err)
	}
	return &policy, nil
}

// UpsertPolicy 设置或更新团队借调策略。
func (s *Service) UpsertPolicy(ctx context.Context, input UpsertPolicyInput) (*Policy, error) {
	if input.TenantID == uuid.Nil || input.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and team_id are required", ErrInvalidInput)
	}
	if input.ActorUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: actor_user_id is required", ErrInvalidInput)
	}
	if !input.ApprovalMode.IsValid() {
		return nil, fmt.Errorf("%w: invalid approval_mode", ErrInvalidInput)
	}
	if input.BudgetCeiling != "" {
		if _, err := parseDecimal(input.BudgetCeiling); err != nil {
			return nil, fmt.Errorf("%w: budget_ceiling must be a decimal string", ErrInvalidInput)
		}
	}
	policy, err := s.repository.UpsertPolicy(ctx, UpsertPolicyParams{
		TenantID:          input.TenantID,
		TeamID:            input.TeamID,
		ActorUserID:       input.ActorUserID,
		AllowLending:      input.AllowLending,
		ApprovalMode:      input.ApprovalMode,
		BudgetCeiling:     strings.TrimSpace(input.BudgetCeiling),
		CapabilityCeiling: input.CapabilityCeiling,
		ProjectMatch:      input.ProjectMatch,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert team lending policy: %w", err)
	}
	s.recordAudit(ctx, input.TenantID, input.TeamID, "team.lending.policy.update", "user", input.ActorUserID.String(), map[string]any{
		"policy_id":     policy.ID.String(),
		"allow_lending": policy.AllowLending,
		"approval_mode": string(policy.ApprovalMode),
	})
	return &policy, nil
}

// CreateRequest 项目侧发起借调请求；按策略自动裁决（auto 放行 / manual 或超纲转人工 pending）。
func (s *Service) CreateRequest(ctx context.Context, input CreateRequestInput) (*Request, error) {
	if input.TenantID == uuid.Nil || input.TeamID == uuid.Nil || input.ProjectID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, team_id and project_id are required", ErrInvalidInput)
	}
	if input.RequestedByUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: requested_by_user_id is required", ErrInvalidInput)
	}
	if input.RequestedBudget != "" {
		if _, err := parseDecimal(input.RequestedBudget); err != nil {
			return nil, fmt.Errorf("%w: requested_budget must be a decimal string", ErrInvalidInput)
		}
	}

	policy, policyErr := s.repository.GetPolicy(ctx, input.TenantID, input.TeamID)
	status, isException, grantedBudget, grantedCapability := evaluateRequest(policy, policyErr, input)

	request, err := s.repository.CreateRequest(ctx, CreateRequestParams{
		TenantID:            input.TenantID,
		TeamID:              input.TeamID,
		ProjectID:           input.ProjectID,
		RequestedByUserID:   input.RequestedByUserID,
		RequestReason:       strings.TrimSpace(input.RequestReason),
		RequestedBudget:     strings.TrimSpace(input.RequestedBudget),
		RequestedCapability: input.RequestedCapability,
		Status:              status,
		GrantedBudget:       grantedBudget,
		GrantedCapability:   grantedCapability,
		IsException:         isException,
	})
	if err != nil {
		return nil, fmt.Errorf("create team lending request: %w", err)
	}

	s.recordAudit(ctx, input.TenantID, input.TeamID, "team.lending.request.create", "user", input.RequestedByUserID.String(), map[string]any{
		"request_id":   request.ID.String(),
		"project_id":   request.ProjectID.String(),
		"status":       string(request.Status),
		"is_exception": request.IsException,
	})

	// 仅 pending（需人工裁决）才进团队负责人 inbox；auto_approved 直接生效，不打扰。
	if request.Status == RequestStatusPending {
		s.projectPendingInbox(ctx, request)
	}
	return &request, nil
}

// ListRequestsByTeam 列举某团队的借调请求（团队负责人审批视角）。
func (s *Service) ListRequestsByTeam(ctx context.Context, tenantID, teamID uuid.UUID, status RequestStatus, limit, offset int32) ([]*Request, error) {
	if tenantID == uuid.Nil || teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and team_id are required", ErrInvalidInput)
	}
	if status != "" && !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}
	limit, offset, err := normalizeLimitOffset(limit, offset)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListRequestsByTeam(ctx, ListTeamRequestsParams{
		TenantID: tenantID,
		TeamID:   teamID,
		Status:   status,
		Offset:   offset,
		Limit:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list team lending requests: %w", err)
	}
	return requestPtrs(records), nil
}

// ListRequestsByProject 列举某项目发起的借调请求（项目负责人视角）。
func (s *Service) ListRequestsByProject(ctx context.Context, tenantID, projectID uuid.UUID, status RequestStatus, limit, offset int32) ([]*Request, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and project_id are required", ErrInvalidInput)
	}
	if status != "" && !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}
	limit, offset, err := normalizeLimitOffset(limit, offset)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListRequestsByProject(ctx, ListProjectRequestsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		Status:    status,
		Offset:    offset,
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list project lending requests: %w", err)
	}
	return requestPtrs(records), nil
}

// EffectiveLendingTeams 返回项目当前持有有效（approved/auto_approved）借调授权的团队集合，
// 供协调线程挑数字员工前的借调闸门批量判断（key 为团队 ID）。
func (s *Service) EffectiveLendingTeams(ctx context.Context, tenantID, projectID uuid.UUID) (map[uuid.UUID]bool, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and project_id are required", ErrInvalidInput)
	}
	teams, err := s.repository.ListEffectiveLendingTeams(ctx, tenantID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list effective lending teams: %w", err)
	}
	granted := make(map[uuid.UUID]bool, len(teams))
	for _, teamID := range teams {
		if teamID != uuid.Nil {
			granted[teamID] = true
		}
	}
	return granted, nil
}

// ApproveRequest 团队负责人通过一个 pending 请求（含超纲转人工的例外）。
func (s *Service) ApproveRequest(ctx context.Context, input DecideRequestInput) (*Request, error) {
	return s.decideRequest(ctx, input, RequestStatusApproved, "team.lending.request.approve")
}

// RejectRequest 团队负责人驳回一个 pending 请求。
func (s *Service) RejectRequest(ctx context.Context, input DecideRequestInput) (*Request, error) {
	return s.decideRequest(ctx, input, RequestStatusRejected, "team.lending.request.reject")
}

// RevokeRequest 团队负责人撤销一个已生效（approved/auto_approved）的借调。
func (s *Service) RevokeRequest(ctx context.Context, input DecideRequestInput) (*Request, error) {
	if err := validateDecideInput(input); err != nil {
		return nil, err
	}
	request, err := s.repository.RevokeRequest(ctx, DecideRequestParams{
		TenantID:          input.TenantID,
		TeamID:            input.TeamID,
		RequestID:         input.RequestID,
		DecidedByUserID:   input.DecidedByUserID,
		DecisionReason:    strings.TrimSpace(input.DecisionReason),
	})
	if err != nil {
		return nil, mapDecisionServiceError(err)
	}
	s.recordAudit(ctx, input.TenantID, input.TeamID, "team.lending.request.revoke", "user", input.DecidedByUserID.String(), map[string]any{
		"request_id": request.ID.String(),
		"project_id": request.ProjectID.String(),
	})
	s.resolveInbox(ctx, request)
	return &request, nil
}

func (s *Service) decideRequest(ctx context.Context, input DecideRequestInput, status RequestStatus, auditAction string) (*Request, error) {
	if err := validateDecideInput(input); err != nil {
		return nil, err
	}
	var request Request
	var err error
	decideParams := DecideRequestParams{
		TenantID:          input.TenantID,
		TeamID:            input.TeamID,
		RequestID:         input.RequestID,
		DecidedByUserID:   input.DecidedByUserID,
		DecisionReason:    strings.TrimSpace(input.DecisionReason),
		GrantedBudget:     strings.TrimSpace(input.GrantedBudget),
		GrantedCapability: input.GrantedCapability,
	}
	if status == RequestStatusApproved {
		if decideParams.GrantedBudget != "" {
			if _, err := parseDecimal(decideParams.GrantedBudget); err != nil {
				return nil, fmt.Errorf("%w: granted_budget must be a decimal string", ErrInvalidInput)
			}
		}
		request, err = s.repository.ApproveRequest(ctx, decideParams)
	} else {
		request, err = s.repository.RejectRequest(ctx, decideParams)
	}
	if err != nil {
		return nil, mapDecisionServiceError(err)
	}
	s.recordAudit(ctx, input.TenantID, input.TeamID, auditAction, "user", input.DecidedByUserID.String(), map[string]any{
		"request_id":     request.ID.String(),
		"project_id":     request.ProjectID.String(),
		"granted_budget": request.GrantedBudget,
	})
	s.resolveInbox(ctx, request)
	return &request, nil
}

// ---- 裁决评估（D2：超纲强制转人工）----

// evaluateRequest 依据策略计算请求初始状态与授予额度。
func evaluateRequest(policy Policy, policyErr error, input CreateRequestInput) (status RequestStatus, isException bool, grantedBudget string, grantedCapability map[string]any) {
	autoCapable := policyErr == nil && policy.AllowLending && policy.ApprovalMode == ApprovalModeAuto
	if !autoCapable {
		// 未配置策略 / 明确关闭借调 / manual 模式：走人工审批（常规 pending，非例外）。
		return RequestStatusPending, false, "", nil
	}
	if requestExceedsCeiling(input, policy) {
		// auto 模式但超预算/能力天花板：强制转人工（例外）。
		return RequestStatusPending, true, "", nil
	}
	// auto 且未超纲：自动放行，授予即所申请（已在天花板内）。
	return RequestStatusAutoApproved, false, strings.TrimSpace(input.RequestedBudget), cloneMap(input.RequestedCapability)
}

// requestExceedsCeiling 判断申请是否超出策略天花板（预算数值比较 + 能力键存在性保守判断）。
func requestExceedsCeiling(input CreateRequestInput, policy Policy) bool {
	if policy.BudgetCeiling != "" && input.RequestedBudget != "" {
		if exceeds, err := decimalGreaterThan(input.RequestedBudget, policy.BudgetCeiling); err == nil && exceeds {
			return true
		}
	}
	if len(policy.CapabilityCeiling) > 0 && len(input.RequestedCapability) > 0 {
		for key := range input.RequestedCapability {
			if _, allowed := policy.CapabilityCeiling[key]; !allowed {
				return true
			}
		}
	}
	return false
}

// ---- inbox 投影（nil-safe）----

func (s *Service) projectPendingInbox(ctx context.Context, request Request) {
	if s.inbox == nil {
		return
	}
	owners, err := s.repository.GetTeamOwnerUserIDs(ctx, request.TenantID, request.TeamID)
	if err != nil || len(owners) == 0 {
		return
	}
	riskLevel := "medium"
	if request.IsException {
		riskLevel = "high"
	}
	_ = s.inbox.UpsertLendingRequest(ctx, LendingInboxItem{
		TenantID:     request.TenantID,
		TeamID:       request.TeamID,
		RequestID:    request.ID,
		ProjectID:    request.ProjectID,
		OwnerUserIDs: owners,
		Title:        "团队借调请求待审批",
		Summary:      lendingSummary(request),
		RiskLevel:    riskLevel,
	})
}

func (s *Service) resolveInbox(ctx context.Context, request Request) {
	if s.inbox == nil {
		return
	}
	owners, err := s.repository.GetTeamOwnerUserIDs(ctx, request.TenantID, request.TeamID)
	if err != nil || len(owners) == 0 {
		return
	}
	_ = s.inbox.ResolveLendingRequest(ctx, LendingInboxItem{
		TenantID:     request.TenantID,
		TeamID:       request.TeamID,
		RequestID:    request.ID,
		ProjectID:    request.ProjectID,
		OwnerUserIDs: owners,
	})
}

func lendingSummary(request Request) string {
	if strings.TrimSpace(request.RequestReason) != "" {
		return request.RequestReason
	}
	return fmt.Sprintf("项目 %s 申请借调本团队数字员工", request.ProjectID)
}

// ---- 审计（nil-safe）----

func (s *Service) recordAudit(ctx context.Context, tenantID, teamID uuid.UUID, action, actorType, actorID string, details map[string]any) {
	if s.audit == nil {
		return
	}
	event := &audit.Event{
		TenantID:     tenantID,
		EventType:    "team_lending",
		ActorType:    actorType,
		ActorID:      actorID,
		ResourceType: "team",
		ResourceID:   teamID.String(),
		Action:       action,
		Details:      details,
		CreatedAt:    time.Now().UTC(),
	}
	_ = s.audit.RecordEvent(ctx, event)
}

// ---- 校验/工具 ----

func validateDecideInput(input DecideRequestInput) error {
	if input.TenantID == uuid.Nil || input.TeamID == uuid.Nil || input.RequestID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id, team_id and request_id are required", ErrInvalidInput)
	}
	if input.DecidedByUserID == uuid.Nil {
		return fmt.Errorf("%w: decided_by_user_id is required", ErrInvalidInput)
	}
	return nil
}

func normalizeLimitOffset(limit, offset int32) (int32, int32, error) {
	if offset < 0 {
		return 0, 0, fmt.Errorf("%w: offset must be non-negative", ErrInvalidInput)
	}
	if limit < 0 {
		return 0, 0, fmt.Errorf("%w: limit must be non-negative", ErrInvalidInput)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	return limit, offset, nil
}

func requestPtrs(records []Request) []*Request {
	out := make([]*Request, 0, len(records))
	for i := range records {
		out = append(out, &records[i])
	}
	return out
}

// mapDecisionServiceError 把仓储裁决错误映射为服务层语义错误。
func mapDecisionServiceError(err error) error {
	if errors.Is(err, ErrInvalidTransition) {
		return err
	}
	return fmt.Errorf("decide team lending request: %w", err)
}

// parseDecimal 解析十进制字符串为 big.Float（校验金额格式）。
func parseDecimal(value string) (*big.Float, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("empty decimal")
	}
	parsed, _, err := big.ParseFloat(trimmed, 10, 64, big.ToNearestEven)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

// decimalGreaterThan 返回 a > b（非负金额比较）。
func decimalGreaterThan(a, b string) (bool, error) {
	af, err := parseDecimal(a)
	if err != nil {
		return false, err
	}
	bf, err := parseDecimal(b)
	if err != nil {
		return false, err
	}
	return af.Cmp(bf) > 0, nil
}
