package externalintegration

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/httpx"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/employee"
)

type HTTPHandler struct {
	service    *Service
	authorizer authz.Authorizer
}

func NewHandler(service *Service) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

// --- Admin surface (console auth) --------------------------------------------

func (h *HTTPHandler) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorize(w, r, authz.ActionProjectDemandRead, "external integration list")
	if !ok {
		return
	}
	var projectID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("project_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid project_id", http.StatusBadRequest)
			return
		}
		projectID = &id
	}
	integrations, err := h.service.ListIntegrations(r.Context(), tenantID, projectID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	items := make([]integrationResponse, 0, len(integrations))
	for _, integration := range integrations {
		items = append(items, integrationResponseFrom(integration))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"integrations": items})
}

func (h *HTTPHandler) CreateIntegration(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionProjectDemandSubmit, "external integration create")
	if !ok {
		return
	}
	var body createIntegrationBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	integration, err := h.service.CreateIntegration(r.Context(), CreateIntegrationRequest{
		TenantID:            tenantID,
		CreatedByUserID:     userID,
		ProjectID:           body.ProjectID,
		DigitalEmployeeID:   body.DigitalEmployeeID,
		Name:                body.Name,
		Description:         body.Description,
		AllowChatRun:        body.AllowChatRun,
		AllowDemandSubmit:   body.AllowDemandSubmit,
		SkillIDs:            body.SkillIDs,
		ScenarioTemplateKey: body.ScenarioTemplateKey,
		AutonomyTier:        body.AutonomyTier,
		PinnedExitDeliverable: body.PinnedExitDeliverable,
		AcknowledgeExitTierSemantics: body.AcknowledgeExitTierSemantics,
		MaxCallsPerHour:     body.MaxCallsPerHour,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, integrationResponseFrom(integration))
}

func (h *HTTPHandler) UpdateIntegration(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionProjectDemandSubmit, "external integration update")
	if !ok {
		return
	}
	integrationID, ok := integrationIDFromRequest(w, r)
	if !ok {
		return
	}
	var body updateIntegrationBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req := UpdateIntegrationRequest{
		TenantID:          tenantID,
		IntegrationID:     integrationID,
		ActorUserID:       userID,
		Name:              body.Name,
		Description:       body.Description,
		AllowChatRun:      body.AllowChatRun,
		AllowDemandSubmit: body.AllowDemandSubmit,
		SkillIDs:          body.SkillIDs,
		AutonomyTier:      body.AutonomyTier,
		MaxCallsPerHour:   body.MaxCallsPerHour,
		Status:            body.Status,
	}
	if body.ScenarioTemplateKey.set {
		req.ScenarioTemplateKeySet = true
		req.ScenarioTemplateKey = body.ScenarioTemplateKey.value
	}
	integration, err := h.service.UpdateIntegration(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, integrationResponseFrom(integration))
}

func (h *HTTPHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorizeCredential(w, r, authz.ActionCredentialRead)
	if !ok {
		return
	}
	integrationID, ok := integrationIDFromRequest(w, r)
	if !ok {
		return
	}
	tokens, err := h.service.ListTokens(r.Context(), tenantID, integrationID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	items := make([]tokenResponse, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, tokenResponseFrom(token))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tokens": items})
}

func (h *HTTPHandler) IssueToken(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorizeCredential(w, r, authz.ActionCredentialCreate)
	if !ok {
		return
	}
	integrationID, ok := integrationIDFromRequest(w, r)
	if !ok {
		return
	}
	plaintext, token, err := h.service.IssueToken(r.Context(), tenantID, integrationID, userID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":             token.ID,
		"integration_id": token.IntegrationID,
		"token":          plaintext,
	})
}

func (h *HTTPHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorizeCredential(w, r, authz.ActionCredentialDelete)
	if !ok {
		return
	}
	integrationID, ok := integrationIDFromRequest(w, r)
	if !ok {
		return
	}
	tokenID, err := uuid.Parse(chi.URLParam(r, "tokenId"))
	if err != nil {
		http.Error(w, "invalid token id", http.StatusBadRequest)
		return
	}
	if err := h.service.RevokeToken(r.Context(), tenantID, integrationID, tokenID, userID); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- External surface (integration token auth) -------------------------------

func (h *HTTPHandler) ExternalChatRun(w http.ResponseWriter, r *http.Request) {
	integration, ok := h.authenticateIntegration(w, r)
	if !ok {
		return
	}
	var body externalChatRunBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.service.ExecuteChatRun(r.Context(), integration, ExternalChatRunRequest{
		Objective:     body.Objective,
		ResumeOfRunID: body.ResumeOfRunID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"run_id": result.RunID,
		"status": result.Status,
	})
}

func (h *HTTPHandler) ExternalSubmitDemand(w http.ResponseWriter, r *http.Request) {
	integration, ok := h.authenticateIntegration(w, r)
	if !ok {
		return
	}
	var body externalDemandBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.service.ExecuteDemandSubmit(r.Context(), integration, ExternalDemandRequest{
		Title:            body.Title,
		Content:          body.Content,
		CoordinationMode: body.CoordinationMode,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"demand_id": result.DemandID,
		"status":    result.Status,
	})
}

func (h *HTTPHandler) authenticateIntegration(w http.ResponseWriter, r *http.Request) (Integration, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(header, bearerPrefix) {
		http.Error(w, "missing bearer integration token", http.StatusUnauthorized)
		return Integration{}, false
	}
	integration, err := h.service.AuthorizeToken(r.Context(), strings.TrimPrefix(header, bearerPrefix))
	if err != nil {
		writeHandlerError(w, err)
		return Integration{}, false
	}
	return integration, true
}

func (h *HTTPHandler) authorize(w http.ResponseWriter, r *http.Request, action string, auditReason string) (uuid.UUID, uuid.UUID, bool) {
	return httpx.AuthorizeConsoleAction(w, r, h.authorizer, "external integration", action, auditReason)
}

// authorizeCredential mirrors serviceauth: credential.* actions are checked
// against the credential(self) resource, not the tenant resource.
func (h *HTTPHandler) authorizeCredential(w http.ResponseWriter, r *http.Request, action string) (uuid.UUID, uuid.UUID, bool) {
	return httpx.CheckConsole(w, r, h.authorizer, func(tenantID, userID uuid.UUID) authz.CheckRequest {
		return authz.CheckRequest{
			Actor:    authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
			Action:   action,
			Resource: authz.ResourceRef{Type: authz.ResourceCredential, ID: userID.String()},
			TenantID: tenantID,
		}
	})
}

func integrationIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	return httpx.URLParamUUID(w, r, "integrationId", "integration id")
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrUnauthorized):
		http.Error(w, err.Error(), http.StatusUnauthorized)
	case errors.Is(err, ErrForbidden):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, ErrPolicyReject):
		// Sync reject when effective ≠ full_auto (spec §4.4 / §10).
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrOverBudget):
		http.Error(w, err.Error(), http.StatusTooManyRequests)
	case errors.Is(err, employee.ErrConflict):
		// e.g. an active run already exists for the employee — caller should retry later.
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		// Gateway errors (employee/project validation) surface as 400: the
		// envelope re-check happens there (out-of-surface skill etc.).
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// --- wire types ---------------------------------------------------------------

type createIntegrationBody struct {
	ProjectID           uuid.UUID   `json:"project_id"`
	DigitalEmployeeID   uuid.UUID   `json:"digital_employee_id"`
	Name                string      `json:"name"`
	Description         string      `json:"description"`
	AllowChatRun        bool        `json:"allow_chat_run"`
	AllowDemandSubmit   bool        `json:"allow_demand_submit"`
	SkillIDs            []uuid.UUID `json:"skill_ids"`
	ScenarioTemplateKey *string     `json:"scenario_template_key"`
	AutonomyTier        string      `json:"autonomy_tier"`
	PinnedExitDeliverable *string   `json:"pinned_exit_deliverable"`
	AcknowledgeExitTierSemantics bool `json:"acknowledge_exit_tier_semantics"`
	MaxCallsPerHour     *int32      `json:"max_calls_per_hour"`
}

// optionalString distinguishes "field absent" from "field set to null" so PATCH
// can clear scenario_template_key with an explicit null.
type optionalString struct {
	set   bool
	value *string
}

func (o *optionalString) UnmarshalJSON(data []byte) error {
	o.set = true
	if string(data) == "null" {
		o.value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	o.value = &s
	return nil
}

type updateIntegrationBody struct {
	Name                *string        `json:"name"`
	Description         *string        `json:"description"`
	AllowChatRun        *bool          `json:"allow_chat_run"`
	AllowDemandSubmit   *bool          `json:"allow_demand_submit"`
	SkillIDs            *[]uuid.UUID   `json:"skill_ids"`
	ScenarioTemplateKey optionalString `json:"scenario_template_key"`
	AutonomyTier        *string        `json:"autonomy_tier"`
	MaxCallsPerHour     *int32         `json:"max_calls_per_hour"`
	Status              *string        `json:"status"`
}

type externalChatRunBody struct {
	Objective     string     `json:"objective"`
	ResumeOfRunID *uuid.UUID `json:"resume_of_run_id"`
}

type externalDemandBody struct {
	Title            string `json:"title"`
	Content          string `json:"content"`
	CoordinationMode string `json:"coordination_mode"`
}

type integrationResponse struct {
	ID                  uuid.UUID   `json:"id"`
	TenantID            uuid.UUID   `json:"tenant_id"`
	ProjectID           uuid.UUID   `json:"project_id"`
	DigitalEmployeeID   uuid.UUID   `json:"digital_employee_id"`
	Name                string      `json:"name"`
	Description         string      `json:"description"`
	AllowChatRun        bool        `json:"allow_chat_run"`
	AllowDemandSubmit   bool        `json:"allow_demand_submit"`
	SkillIDs            []uuid.UUID `json:"skill_ids"`
	ScenarioTemplateKey *string     `json:"scenario_template_key"`
	AutonomyTier        string      `json:"autonomy_tier"`
	PinnedExitDeliverable *string   `json:"pinned_exit_deliverable,omitempty"`
	AcknowledgeExitTierSemantics bool `json:"acknowledge_exit_tier_semantics"`
	MaxCallsPerHour     int32       `json:"max_calls_per_hour"`
	Status              string      `json:"status"`
	CreatedByUserID     uuid.UUID   `json:"created_by_user_id"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
}

func integrationResponseFrom(integration Integration) integrationResponse {
	skillIDs := integration.SkillIDs
	if skillIDs == nil {
		skillIDs = []uuid.UUID{}
	}
	return integrationResponse{
		ID:                  integration.ID,
		TenantID:            integration.TenantID,
		ProjectID:           integration.ProjectID,
		DigitalEmployeeID:   integration.DigitalEmployeeID,
		Name:                integration.Name,
		Description:         integration.Description,
		AllowChatRun:        integration.AllowChatRun,
		AllowDemandSubmit:   integration.AllowDemandSubmit,
		SkillIDs:            skillIDs,
		ScenarioTemplateKey: integration.ScenarioTemplateKey,
		AutonomyTier:        integration.AutonomyTier,
		PinnedExitDeliverable: integration.PinnedExitDeliverable,
		AcknowledgeExitTierSemantics: integration.AcknowledgeExitTierSemantics,
		MaxCallsPerHour:     integration.MaxCallsPerHour,
		Status:              integration.Status,
		CreatedByUserID:     integration.CreatedByUserID,
		CreatedAt:           integration.CreatedAt,
		UpdatedAt:           integration.UpdatedAt,
	}
}

type tokenResponse struct {
	ID            uuid.UUID  `json:"id"`
	IntegrationID uuid.UUID  `json:"integration_id"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

func tokenResponseFrom(token Token) tokenResponse {
	return tokenResponse{
		ID:            token.ID,
		IntegrationID: token.IntegrationID,
		Status:        token.Status,
		CreatedAt:     token.CreatedAt,
		LastUsedAt:    token.LastUsedAt,
		RevokedAt:     token.RevokedAt,
	}
}
