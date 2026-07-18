package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/systemconfig"
)

// 技能包上传上限默认值在 systemconfig 注册表,可被系统配置中心覆盖。

type HandlerService interface {
	ListSkills(ctx context.Context, req ListSkillsRequest) ([]*Skill, error)
	GetSkill(ctx context.Context, req GetSkillRequest) (*Skill, error)
	UploadSkill(ctx context.Context, req UploadSkillRequest) (*Skill, error)
	DeleteSkill(ctx context.Context, req DeleteSkillRequest) error
	BindSkillToTeam(ctx context.Context, req BindTeamSkillRequest) (*Skill, error)
	UnbindSkillFromTeam(ctx context.Context, req BindTeamSkillRequest) error
	ListTeamSkills(ctx context.Context, req ListTeamSkillsRequest) ([]*Skill, error)
	BindSkillToEmployee(ctx context.Context, req BindEmployeeSkillRequest) (*Skill, error)
	UnbindSkillFromEmployee(ctx context.Context, req BindEmployeeSkillRequest) error
	ListEffectiveEmployeeSkills(ctx context.Context, req ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error)
	InstallSkill(ctx context.Context, req InstallSkillRequest) (InstallSkillResult, error)
}

type HTTPHandler struct {
	service      HandlerService
	authorizer   authz.Authorizer
	systemConfig systemconfig.Reader
}

// SetSystemConfigReader 注入配置中心读取器;未注入(测试)时使用注册表默认值。
func (h *HTTPHandler) SetSystemConfigReader(reader systemconfig.Reader) {
	h.systemConfig = reader
}

func (h *HTTPHandler) uploadMaxBytes(r *http.Request, tenantID uuid.UUID) int64 {
	if h.systemConfig == nil {
		return systemconfig.DefaultFor(systemconfig.KeySkillUploadMaxBytes)
	}
	return h.systemConfig.Int64(r.Context(), tenantID, systemconfig.KeySkillUploadMaxBytes)
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
	maxUploadBytes := h.uploadMaxBytes(r, tenantID)
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
	if int64(len(archive)) > maxUploadBytes {
		http.Error(w, fmt.Sprintf("uploaded skill zip exceeds %d bytes", maxUploadBytes), http.StatusBadRequest)
		return
	}
	runtimeDependencies, err := parseRuntimeDependenciesForm(r)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	skill, err := service.UploadSkill(r.Context(), UploadSkillRequest{
		TenantID:            tenantID,
		ActorUserID:         middleware.GetUserID(r.Context()),
		Name:                r.FormValue("name"),
		Description:         r.FormValue("description"),
		Tags:                splitFormList(r.MultipartForm.Value["tags"]),
		TeamIDs:             parseUUIDList(r.MultipartForm.Value["team_ids"]),
		RiskLevel:           r.FormValue("risk_level"),
		RuntimeDependencies: runtimeDependencies,
		Archive:             archive,
		Filename:            header.Filename,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, skillResponseFromDomain(skill))
}

func (h *HTTPHandler) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	skillID, ok := skillIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionSkillDelete, authz.ResourceRef{Type: authz.ResourceSkill, ID: skillID.String()}, "skill delete")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.DeleteSkill(r.Context(), DeleteSkillRequest{
		TenantID: tenantID,
		SkillID:  skillID,
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) InstallSkill(w http.ResponseWriter, r *http.Request) {
	skillID, ok := skillIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionSkillInstall, authz.ResourceRef{Type: authz.ResourceSkill, ID: skillID.String()}, "skill install")
	if !ok {
		return
	}
	req, ok := installSkillRequestFromJSONBody(w, r)
	if !ok {
		return
	}
	req.TenantID = tenantID
	req.SkillID = skillID
	req.ActorUserID = middleware.GetUserID(r.Context())
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	result, err := service.InstallSkill(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, installSkillResponseFromDomain(result))
}

func (h *HTTPHandler) ListTeamSkills(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionTeamRead, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team skill read", &teamID)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	skills, err := service.ListTeamSkills(r.Context(), ListTeamSkillsRequest{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillResponses(skills))
}

func (h *HTTPHandler) BindTeamSkill(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionTeamCapabilityBind, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team skill bind", &teamID)
	if !ok {
		return
	}
	skillID, ok := skillIDFromJSONBody(w, r)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	item, err := service.BindSkillToTeam(r.Context(), BindTeamSkillRequest{
		TenantID: tenantID,
		TeamID:   teamID,
		SkillID:  skillID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, skillResponseFromDomain(item))
}

func (h *HTTPHandler) UnbindTeamSkill(w http.ResponseWriter, r *http.Request) {
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return
	}
	skillID, ok := skillIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionTeamCapabilityUnbind, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team skill unbind", &teamID)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.UnbindSkillFromTeam(r.Context(), BindTeamSkillRequest{
		TenantID: tenantID,
		TeamID:   teamID,
		SkillID:  skillID,
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) ListEffectiveEmployeeSkills(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionEmployeeRead, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "effective employee skill read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	skills, err := service.ListEffectiveEmployeeSkills(r.Context(), ListEffectiveEmployeeSkillsRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, effectiveEmployeeSkillResponses(skills))
}

func (h *HTTPHandler) BindEmployeeSkill(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionEmployeeConfigCreate, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee skill bind")
	if !ok {
		return
	}
	skillID, ok := skillIDFromJSONBody(w, r)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	item, err := service.BindSkillToEmployee(r.Context(), BindEmployeeSkillRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		SkillID:           skillID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, skillResponseFromDomain(item))
}

func (h *HTTPHandler) UnbindEmployeeSkill(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	skillID, ok := skillIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionEmployeeConfigCreate, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee skill unbind")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.UnbindSkillFromEmployee(r.Context(), BindEmployeeSkillRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		SkillID:           skillID,
	}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "skill service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

func (h *HTTPHandler) authorizeSkillAction(w http.ResponseWriter, r *http.Request, action string, resource authz.ResourceRef, auditReason string, teamID ...*uuid.UUID) (uuid.UUID, bool) {
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
		TeamID:      firstTeamID(teamID),
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
	ID                  string                      `json:"id"`
	TenantID            string                      `json:"tenant_id"`
	Slug                string                      `json:"slug"`
	Name                string                      `json:"name"`
	Description         string                      `json:"description"`
	Version             string                      `json:"version"`
	Source              string                      `json:"source"`
	RiskLevel           string                      `json:"risk_level"`
	IconKey             string                      `json:"icon_key"`
	ColorToken          string                      `json:"color_token"`
	Tags                []string                    `json:"tags"`
	ArchiveObjectRef    string                      `json:"archive_object_ref"`
	ArchiveFilename     string                      `json:"archive_filename"`
	ArchiveSizeBytes    int64                       `json:"archive_size_bytes"`
	ArchiveChecksum     string                      `json:"archive_checksum_sha256"`
	ArchiveFileCount    int                         `json:"archive_file_count"`
	RuntimeDependencies SkillRuntimeDependencies    `json:"runtime_dependencies"`
	CreatedBy           string                      `json:"created_by"`
	CreatedByName       string                      `json:"created_by_name"`
	TeamBindings        []skillTeamBindingResponse  `json:"team_bindings"`
	AgentBindings       []skillAgentBindingResponse `json:"agent_bindings"`
	CreatedAt           string                      `json:"created_at,omitempty"`
	UpdatedAt           string                      `json:"updated_at,omitempty"`
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

type effectiveEmployeeSkillResponse struct {
	Skill       skillResponse `json:"skill"`
	SourceScope string        `json:"source_scope"`
	Inherited   bool          `json:"inherited"`
	ReadOnly    bool          `json:"read_only"`
}

type installSkillResponse struct {
	SkillID           string `json:"skill_id"`
	TargetScope       string `json:"target_scope"`
	TeamID            string `json:"team_id,omitempty"`
	DigitalEmployeeID string `json:"digital_employee_id,omitempty"`
	AlreadyBound      bool   `json:"already_bound"`
	BoundAt           string `json:"bound_at"`
}

func skillResponses(skills []*Skill) []skillResponse {
	responses := make([]skillResponse, 0, len(skills))
	for _, item := range skills {
		responses = append(responses, skillResponseFromDomain(item))
	}
	return responses
}

func effectiveEmployeeSkillResponses(skills []EffectiveEmployeeSkill) []effectiveEmployeeSkillResponse {
	responses := make([]effectiveEmployeeSkillResponse, 0, len(skills))
	for _, item := range skills {
		skillItem := item.Skill
		responses = append(responses, effectiveEmployeeSkillResponse{
			Skill:       skillResponseFromDomain(&skillItem),
			SourceScope: item.SourceScope,
			Inherited:   item.Inherited,
			ReadOnly:    item.ReadOnly,
		})
	}
	return responses
}

func installSkillResponseFromDomain(result InstallSkillResult) installSkillResponse {
	return installSkillResponse{
		SkillID:           result.SkillID.String(),
		TargetScope:       string(result.TargetScope),
		TeamID:            uuidStringOrEmpty(result.TeamID),
		DigitalEmployeeID: uuidStringOrEmpty(result.DigitalEmployeeID),
		AlreadyBound:      result.AlreadyBound,
		BoundAt:           formatTime(result.BoundAt),
	}
}

func skillResponseFromDomain(item *Skill) skillResponse {
	if item == nil {
		return skillResponse{}
	}
	return skillResponse{
		ID:                  item.ID.String(),
		TenantID:            item.TenantID.String(),
		Slug:                item.Slug,
		Name:                item.Name,
		Description:         item.Description,
		Version:             item.Version,
		Source:              item.Source,
		RiskLevel:           item.RiskLevel,
		IconKey:             item.IconKey,
		ColorToken:          item.ColorToken,
		Tags:                item.Tags,
		ArchiveObjectRef:    item.ArchiveObjectRef,
		ArchiveFilename:     item.ArchiveFilename,
		ArchiveSizeBytes:    item.ArchiveSizeBytes,
		ArchiveChecksum:     item.ArchiveChecksum,
		ArchiveFileCount:    item.ArchiveFileCount,
		RuntimeDependencies: runtimeDependenciesForResponse(item.RuntimeDependencies),
		CreatedBy:           item.CreatedBy.String(),
		CreatedByName:       item.CreatedByName,
		TeamBindings:        skillTeamBindingResponses(item.TeamBindings),
		AgentBindings:       skillAgentBindingResponses(item.AgentBindings),
		CreatedAt:           formatTime(item.CreatedAt),
		UpdatedAt:           formatTime(item.UpdatedAt),
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

func runtimeDependenciesForResponse(deps SkillRuntimeDependencies) SkillRuntimeDependencies {
	if deps.Tools == nil {
		deps.Tools = []string{}
	}
	if deps.Env == nil {
		deps.Env = []string{}
	}
	return deps
}

func skillIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "skillId"))
	if err != nil || id == uuid.Nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func teamIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "teamId"))
	if err != nil || id == uuid.Nil {
		http.Error(w, "invalid team id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func employeeIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "employeeId"))
	if err != nil || id == uuid.Nil {
		http.Error(w, "invalid employee id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func skillIDFromJSONBody(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	var req struct {
		SkillID uuid.UUID `json:"skill_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return uuid.Nil, false
	}
	if req.SkillID == uuid.Nil {
		http.Error(w, "skill_id is required", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return req.SkillID, true
}

func installSkillRequestFromJSONBody(w http.ResponseWriter, r *http.Request) (InstallSkillRequest, bool) {
	var body struct {
		TargetScope       SkillInstallTargetScope `json:"target_scope"`
		TeamID            *uuid.UUID              `json:"team_id"`
		DigitalEmployeeID *uuid.UUID              `json:"digital_employee_id"`
		// timeout_sec is deprecated: install is a synchronous logical bind
		// with no runtime wait. Accepted and ignored for old clients.
		TimeoutSec int `json:"timeout_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return InstallSkillRequest{}, false
	}
	req := InstallSkillRequest{TargetScope: body.TargetScope}
	if body.TeamID != nil {
		req.TeamID = *body.TeamID
		if req.TeamID == uuid.Nil {
			http.Error(w, "team_id must be a valid uuid", http.StatusBadRequest)
			return InstallSkillRequest{}, false
		}
	}
	if body.DigitalEmployeeID != nil {
		req.DigitalEmployeeID = *body.DigitalEmployeeID
		if req.DigitalEmployeeID == uuid.Nil {
			http.Error(w, "digital_employee_id must be a valid uuid", http.StatusBadRequest)
			return InstallSkillRequest{}, false
		}
	}
	return req, true
}

func uuidStringOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func firstTeamID(values []*uuid.UUID) *uuid.UUID {
	if len(values) == 0 {
		return nil
	}
	return values[0]
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

func parseRuntimeDependenciesForm(r *http.Request) (SkillRuntimeDependencies, error) {
	if raw := strings.TrimSpace(r.FormValue("runtime_dependencies")); raw != "" {
		var deps SkillRuntimeDependencies
		if err := json.Unmarshal([]byte(raw), &deps); err != nil {
			return SkillRuntimeDependencies{}, fmt.Errorf("%w: invalid runtime_dependencies", ErrInvalidInput)
		}
		return deps, nil
	}
	return SkillRuntimeDependencies{
		Tools: splitFormList(r.MultipartForm.Value["runtime_tools"]),
		Env:   splitFormList(r.MultipartForm.Value["runtime_env"]),
	}, nil
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
	case errors.Is(err, ErrTeamAlreadyInherited):
		http.Error(w, err.Error(), http.StatusConflict)
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
