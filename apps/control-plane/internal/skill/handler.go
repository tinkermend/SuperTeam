package skill

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

const maxUploadBytes = 50 << 20

type HandlerService interface {
	ListSkills(ctx context.Context, req ListSkillsRequest) ([]*Skill, error)
	GetSkill(ctx context.Context, req GetSkillRequest) (*Skill, error)
	UploadSkill(ctx context.Context, req UploadSkillRequest) (*Skill, error)
	UpdateSkillFile(ctx context.Context, req UpdateSkillFileRequest) (*SkillFile, error)
}

type HTTPHandler struct {
	service    HandlerService
	authorizer authz.Authorizer
}

func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *HTTPHandler) ListSkills(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionSkillRead, authz.ResourceRef{Type: authz.ResourceTenant, ID: middleware.GetTenantID(r.Context()).String()}, "skill read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	skills, err := service.ListSkills(r.Context(), ListSkillsRequest{
		TenantID: tenantID,
		Status:   SkillStatus(r.URL.Query().Get("status")),
		Q:        r.URL.Query().Get("q"),
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillResponses(skills))
}

func (h *HTTPHandler) GetSkill(w http.ResponseWriter, r *http.Request) {
	skillID, ok := skillIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionSkillRead, authz.ResourceRef{Type: authz.ResourceSkill, ID: skillID.String()}, "skill detail read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	skill, err := service.GetSkill(r.Context(), GetSkillRequest{TenantID: tenantID, SkillID: skillID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillResponseFromDomain(skill))
}

func (h *HTTPHandler) UploadSkill(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionSkillUpload, authz.ResourceRef{Type: authz.ResourceTenant, ID: middleware.GetTenantID(r.Context()).String()}, "skill upload")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	archive, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		http.Error(w, "cannot read uploaded file", http.StatusBadRequest)
		return
	}
	if len(archive) > maxUploadBytes {
		http.Error(w, "uploaded skill zip exceeds 50MB", http.StatusBadRequest)
		return
	}
	skill, err := service.UploadSkill(r.Context(), UploadSkillRequest{
		TenantID:    tenantID,
		ActorUserID: middleware.GetUserID(r.Context()),
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		Tags:        splitFormList(r.MultipartForm.Value["tags"]),
		TeamIDs:     parseUUIDList(r.MultipartForm.Value["team_ids"]),
		RiskLevel:   r.FormValue("risk_level"),
		Archive:     archive,
		Filename:    header.Filename,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, skillResponseFromDomain(skill))
}

func (h *HTTPHandler) UpdateSkillFile(w http.ResponseWriter, r *http.Request) {
	skillID, ok := skillIDFromRequest(w, r)
	if !ok {
		return
	}
	path := chi.URLParam(r, "*")
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionSkillUpdate, authz.ResourceRef{Type: authz.ResourceSkill, ID: skillID.String()}, "skill file update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, err := service.UpdateSkillFile(r.Context(), UpdateSkillFileRequest{
		TenantID: tenantID,
		SkillID:  skillID,
		Path:     path,
		Content:  req.Content,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillFileResponseFromDomain(file))
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "skill service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

func (h *HTTPHandler) authorizeSkillAction(w http.ResponseWriter, r *http.Request, action string, resource authz.ResourceRef, auditReason string) (uuid.UUID, bool) {
	if h == nil || h.authorizer == nil {
		http.Error(w, "skill authorization is not configured", http.StatusForbidden)
		return uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, false
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   userID.String(),
		},
		Action:      action,
		Resource:    resource,
		TenantID:    tenantID,
		AuditReason: auditReason,
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return uuid.Nil, false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.Nil, false
	}
	return tenantID, true
}

type skillResponse struct {
	ID            string                      `json:"id"`
	TenantID      string                      `json:"tenant_id"`
	Slug          string                      `json:"slug"`
	Name          string                      `json:"name"`
	Description   string                      `json:"description"`
	Version       string                      `json:"version"`
	Source        string                      `json:"source"`
	RiskLevel     string                      `json:"risk_level"`
	Status        SkillStatus                 `json:"status"`
	IconKey       string                      `json:"icon_key"`
	ColorToken    string                      `json:"color_token"`
	Tags          []string                    `json:"tags"`
	Files         []skillFileResponse         `json:"files"`
	TeamBindings  []skillTeamBindingResponse  `json:"team_bindings"`
	AgentBindings []skillAgentBindingResponse `json:"agent_bindings"`
	CreatedAt     string                      `json:"created_at,omitempty"`
	UpdatedAt     string                      `json:"updated_at,omitempty"`
}

type skillFileResponse struct {
	ID             string        `json:"id,omitempty"`
	Path           string        `json:"path"`
	FileType       SkillFileType `json:"file_type"`
	Content        string        `json:"content,omitempty"`
	SizeBytes      int64         `json:"size_bytes"`
	ChecksumSHA256 string        `json:"checksum_sha256,omitempty"`
	UpdatedAt      string        `json:"updated_at,omitempty"`
}

type skillTeamBindingResponse struct {
	TeamID   string `json:"team_id"`
	TeamName string `json:"team_name"`
}

type skillAgentBindingResponse struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	TeamID    string `json:"team_id,omitempty"`
	TeamName  string `json:"team_name,omitempty"`
	Status    string `json:"status"`
}

func skillResponses(skills []*Skill) []skillResponse {
	responses := make([]skillResponse, 0, len(skills))
	for _, item := range skills {
		responses = append(responses, skillResponseFromDomain(item))
	}
	return responses
}

func skillResponseFromDomain(item *Skill) skillResponse {
	if item == nil {
		return skillResponse{}
	}
	return skillResponse{
		ID:            item.ID.String(),
		TenantID:      item.TenantID.String(),
		Slug:          item.Slug,
		Name:          item.Name,
		Description:   item.Description,
		Version:       item.Version,
		Source:        item.Source,
		RiskLevel:     item.RiskLevel,
		Status:        item.Status,
		IconKey:       item.IconKey,
		ColorToken:    item.ColorToken,
		Tags:          item.Tags,
		Files:         skillFileResponses(item.Files),
		TeamBindings:  skillTeamBindingResponses(item.TeamBindings),
		AgentBindings: skillAgentBindingResponses(item.AgentBindings),
		CreatedAt:     formatTime(item.CreatedAt),
		UpdatedAt:     formatTime(item.UpdatedAt),
	}
}

func skillFileResponses(files []*SkillFile) []skillFileResponse {
	responses := make([]skillFileResponse, 0, len(files))
	for _, file := range files {
		responses = append(responses, skillFileResponseFromDomain(file))
	}
	return responses
}

func skillFileResponseFromDomain(file *SkillFile) skillFileResponse {
	if file == nil {
		return skillFileResponse{}
	}
	id := ""
	if file.ID != uuid.Nil {
		id = file.ID.String()
	}
	return skillFileResponse{
		ID:             id,
		Path:           file.Path,
		FileType:       file.FileType,
		Content:        file.Content,
		SizeBytes:      file.SizeBytes,
		ChecksumSHA256: file.ChecksumSHA256,
		UpdatedAt:      formatTime(file.UpdatedAt),
	}
}

func skillTeamBindingResponses(bindings []*SkillTeamBinding) []skillTeamBindingResponse {
	responses := make([]skillTeamBindingResponse, 0, len(bindings))
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		responses = append(responses, skillTeamBindingResponse{
			TeamID:   binding.TeamID.String(),
			TeamName: binding.TeamName,
		})
	}
	return responses
}

func skillAgentBindingResponses(bindings []*SkillAgentBinding) []skillAgentBindingResponse {
	responses := make([]skillAgentBindingResponse, 0, len(bindings))
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		teamID := ""
		if binding.TeamID != nil {
			teamID = binding.TeamID.String()
		}
		responses = append(responses, skillAgentBindingResponse{
			AgentID:   binding.AgentID.String(),
			AgentName: binding.AgentName,
			TeamID:    teamID,
			TeamName:  binding.TeamName,
			Status:    binding.Status,
		})
	}
	return responses
}

func skillIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "skillId"))
	if err != nil || id == uuid.Nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func splitFormList(values []string) []string {
	var result []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}

func parseUUIDList(values []string) []uuid.UUID {
	var result []uuid.UUID
	for _, value := range splitFormList(values) {
		parsed, err := uuid.Parse(value)
		if err == nil && parsed != uuid.Nil {
			result = append(result, parsed)
		}
	}
	return result
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
