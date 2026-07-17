package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
}

var (
	ErrGatewayForbidden = errors.New("actor is not an eligible decider")
	ErrGatewayBadInput  = errors.New("invalid connector business input")
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
	if err != nil || projectID == uuid.Nil || strings.TrimSpace(req.Decision) == "" {
		http.Error(w, "project_id and decision are required", http.StatusBadRequest)
		return
	}
	conflict, err := h.projects.ResolveDecision(r.Context(), tenantID, projectID, decisionID, userID, req.Decision, req.Comment)
	if err != nil {
		writeConnectorBusinessError(w, err)
		return
	}
	if conflict {
		http.Error(w, "decision already resolved by someone else", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

func writeConnectorBusinessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrGatewayForbidden):
		http.Error(w, "forbidden: not an eligible decider", http.StatusForbidden)
	case errors.Is(err, ErrGatewayBadInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
