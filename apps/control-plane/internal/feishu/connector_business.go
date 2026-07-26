package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/gen"
	"github.com/superteam/control-plane/internal/api/middleware"
)

// ProjectRef 是 my-projects 的最小投影。
type ProjectRef struct {
	ID   uuid.UUID
	Name string
}

// ProjectGateway 把 connector 业务动作接到项目域(app 层用 project.Service 适配)。
// 判权发生在项目域内部(any-of-N 合格处理人),此处只透传行为人。
type ProjectGateway interface {
	ListProjectsForHumanMember(ctx context.Context, tenantID, userID uuid.UUID) ([]ProjectRef, error)
	SubmitDemand(ctx context.Context, tenantID, projectID, userID uuid.UUID, title, content, mode string) (demandID uuid.UUID, status string, err error)
	// ResolveDecision 返回 conflict=true 表示决策已由他人处理(异值终态)。
	ResolveDecision(ctx context.Context, tenantID, projectID, decisionID, userID uuid.UUID, decision, comment string) (conflict bool, err error)
	// DecisionCardSnapshot 返回决策卡投影 payload(与 outbox decision_card 同源),
	// 供飞书端 resolve 后即时渲染保留详情的终态卡。仅在 resolve 出结果后内部调用,
	// 不单独暴露为路由。
	DecisionCardSnapshot(ctx context.Context, tenantID, projectID, decisionID uuid.UUID) (map[string]any, error)
	// SignDemandCriterion on-behalf-of 逐条签署验收判据(卡内签署),返回签署进度、
	// 全量判据 verdict 覆盖与刷新后的决策卡快照,供飞书端整卡重渲染。
	SignDemandCriterion(ctx context.Context, req SignCriterionGatewayRequest) (*SignCriterionOutcome, error)
}

// SignCriterionGatewayRequest 是卡内签署的网关入参。
type SignCriterionGatewayRequest struct {
	TenantID    uuid.UUID
	ProjectID   uuid.UUID
	DemandID    uuid.UUID
	DecisionID  uuid.UUID
	ActorUserID uuid.UUID
	CriterionID string
	Verdict     string
	Reason      string
}

// SignCriterionOutcome 是卡内签署的结果:进度 + verdict 覆盖 + 刷新后的卡快照。
type SignCriterionOutcome struct {
	DemandStatus      string
	Signed            int32
	Total             int32
	Remaining         int32
	CriterionVerdicts map[string]string
	CardPayload       map[string]any
}

var (
	ErrGatewayForbidden = errors.New("actor is not an eligible decider")
	ErrGatewayBadInput  = errors.New("invalid connector business input")
	ErrGatewayConflict  = errors.New("resource is not in a signable state")
)

func (h *ConnectorHTTPHandler) SetProjectGateway(gateway ProjectGateway) {
	h.projects = gateway
}

// actingUser 读取 on-behalf-of 注入的行为人;connector 业务端点必须带。
func actingUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, "service tenant not found in context", http.StatusUnauthorized)
		return uuid.Nil, uuid.Nil, false
	}
	if userID == uuid.Nil {
		http.Error(w, "on-behalf-of user is required for this endpoint", http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

// MyProjects 行为人可发起需求的项目(owner 或 active 人类成员)。
func (h *ConnectorHTTPHandler) MyProjects(w http.ResponseWriter, r *http.Request) {
	if h.projects == nil {
		http.Error(w, "project gateway is not configured", http.StatusServiceUnavailable)
		return
	}
	tenantID, userID, ok := actingUser(w, r)
	if !ok {
		return
	}
	projects, err := h.projects.ListProjectsForHumanMember(r.Context(), tenantID, userID)
	if err != nil {
		writeFeishuError(w, err)
		return
	}
	out := make([]map[string]string, 0, len(projects))
	for _, project := range projects {
		out = append(out, map[string]string{"id": project.ID.String(), "name": project.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

type connectorDemandRequest struct {
	ProjectID        string `json:"project_id"`
	Title            string `json:"title"`
	Content          string `json:"content"`
	CoordinationMode string `json:"coordination_mode"`
}

// SubmitDemand on-behalf-of 包装:SourceType 固定 feishu,收敛可传字段。
func (h *ConnectorHTTPHandler) SubmitDemand(w http.ResponseWriter, r *http.Request) {
	if h.projects == nil {
		http.Error(w, "project gateway is not configured", http.StatusServiceUnavailable)
		return
	}
	tenantID, userID, ok := actingUser(w, r)
	if !ok {
		return
	}
	var req connectorDemandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	projectID, err := uuid.Parse(strings.TrimSpace(req.ProjectID))
	if err != nil || projectID == uuid.Nil || strings.TrimSpace(req.Title) == "" {
		http.Error(w, "project_id and title are required", http.StatusBadRequest)
		return
	}
	demandID, status, err := h.projects.SubmitDemand(r.Context(), tenantID, projectID, userID, req.Title, req.Content, req.CoordinationMode)
	if err != nil {
		writeConnectorBusinessError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"demand_id": demandID.String(),
		"title":     strings.TrimSpace(req.Title),
		"status":    status,
	})
}

type connectorResolveRequest struct {
	ProjectID string `json:"project_id"`
	Decision  string `json:"decision"`
	Comment   string `json:"comment,omitempty"`
}

// ResolveDecision on-behalf-of 决策回传;已由他人处理返回 409。
func (h *ConnectorHTTPHandler) ResolveDecision(w http.ResponseWriter, r *http.Request) {
	if h.projects == nil {
		http.Error(w, "project gateway is not configured", http.StatusServiceUnavailable)
		return
	}
	tenantID, userID, ok := actingUser(w, r)
	if !ok {
		return
	}
	decisionID, err := uuid.Parse(chi.URLParam(r, "decisionId"))
	if err != nil || decisionID == uuid.Nil {
		http.Error(w, "invalid decision id", http.StatusBadRequest)
		return
	}
	var req connectorResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	projectID, err := uuid.Parse(strings.TrimSpace(req.ProjectID))
	decisionValue := strings.TrimSpace(req.Decision)
	if err != nil || projectID == uuid.Nil || decisionValue == "" {
		http.Error(w, "project_id and decision are required", http.StatusBadRequest)
		return
	}
	// 与 Web resolve 腿同一道契约枚举门（ResolveDecisionValue，两条腿共用值域）；
	// 契约外值在进入业务校验前拒绝，业务层 validHumanDecision 仍按 decision_type 把关。
	if !gen.ResolveDecisionValue(decisionValue).Valid() {
		http.Error(w, "invalid decision", http.StatusBadRequest)
		return
	}
	conflict, err := h.projects.ResolveDecision(r.Context(), tenantID, projectID, decisionID, userID, decisionValue, req.Comment)
	if err != nil {
		writeConnectorBusinessError(w, err)
		return
	}
	// 终态快照 best-effort:取不到不影响 resolve 结果,connector 侧降级为薄终态卡。
	card, cardErr := h.projects.DecisionCardSnapshot(r.Context(), tenantID, projectID, decisionID)
	if conflict {
		resp := map[string]any{"error": "decision already resolved by someone else"}
		if cardErr == nil && card != nil {
			resp["card_payload"] = card
		}
		writeJSON(w, http.StatusConflict, resp)
		return
	}
	resp := map[string]any{"status": "resolved"}
	if cardErr == nil && card != nil {
		resp["card_payload"] = card
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeConnectorBusinessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrGatewayForbidden):
		http.Error(w, "forbidden: not an eligible decider", http.StatusForbidden)
	case errors.Is(err, ErrGatewayBadInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrGatewayConflict):
		http.Error(w, "demand is not awaiting acceptance sign-off", http.StatusConflict)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

type connectorSignCriterionRequest struct {
	ProjectID   string `json:"project_id"`
	DecisionID  string `json:"decision_id"`
	CriterionID string `json:"criterion_id"`
	Verdict     string `json:"verdict"`
	Reason      string `json:"reason,omitempty"`
}

// SignDemandCriterion on-behalf-of 卡内逐条签署验收判据。
// 响应带签署进度、全量判据 verdict 与刷新后的决策卡快照,供飞书端整卡重渲染——
// 签署闭环在手机上完成,Console 深链只作证据血缘兜底。
func (h *ConnectorHTTPHandler) SignDemandCriterion(w http.ResponseWriter, r *http.Request) {
	if h.projects == nil {
		http.Error(w, "project gateway is not configured", http.StatusServiceUnavailable)
		return
	}
	tenantID, userID, ok := actingUser(w, r)
	if !ok {
		return
	}
	demandID, err := uuid.Parse(chi.URLParam(r, "demandId"))
	if err != nil || demandID == uuid.Nil {
		http.Error(w, "invalid demand id", http.StatusBadRequest)
		return
	}
	var req connectorSignCriterionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	projectID, err := uuid.Parse(strings.TrimSpace(req.ProjectID))
	if err != nil || projectID == uuid.Nil {
		http.Error(w, "project_id is required", http.StatusBadRequest)
		return
	}
	decisionID, _ := uuid.Parse(strings.TrimSpace(req.DecisionID))
	criterionID := strings.TrimSpace(req.CriterionID)
	verdict := strings.TrimSpace(req.Verdict)
	if criterionID == "" || (verdict != "satisfied" && verdict != "unsatisfied") {
		http.Error(w, "criterion_id and verdict(satisfied|unsatisfied) are required", http.StatusBadRequest)
		return
	}
	outcome, err := h.projects.SignDemandCriterion(r.Context(), SignCriterionGatewayRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		DemandID:    demandID,
		DecisionID:  decisionID,
		ActorUserID: userID,
		CriterionID: criterionID,
		Verdict:     verdict,
		Reason:      strings.TrimSpace(req.Reason),
	})
	if err != nil {
		writeConnectorBusinessError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"demand_status":      outcome.DemandStatus,
		"signed":             outcome.Signed,
		"total":              outcome.Total,
		"remaining":          outcome.Remaining,
		"criterion_verdicts": outcome.CriterionVerdicts,
		"card_payload":       outcome.CardPayload,
	})
}
