package employee

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/authz"
)

type employeeTemplateResponse struct {
	ID                       string         `json:"id"`
	TenantID                 string         `json:"tenant_id"`
	Type                     string         `json:"type"`
	Label                    string         `json:"label"`
	Description              string         `json:"description"`
	DefaultRole              string         `json:"default_role"`
	RecommendedSkills        []string       `json:"recommended_skills"`
	RecommendedMCPServers    []string       `json:"recommended_mcp_servers"`
	RecommendedProviderTypes []string       `json:"recommended_provider_types"`
	PersonaMemoryMarkdown    string         `json:"persona_memory_markdown"`
	CapabilityBindings       map[string]any `json:"capability_bindings"`
	BudgetPolicy             map[string]any `json:"budget_policy"`
	Metadata                 map[string]any `json:"metadata"`
	Status                   string         `json:"status"`
	IsSystem                 bool           `json:"is_system"`
	CreatedAt                string         `json:"created_at"`
	UpdatedAt                string         `json:"updated_at"`
}

type createEmployeeTemplateRequest struct {
	Type                     string         `json:"type"`
	Label                    string         `json:"label"`
	Description              string         `json:"description"`
	DefaultRole              string         `json:"default_role"`
	RecommendedSkills        []string       `json:"recommended_skills"`
	RecommendedMCPServers    []string       `json:"recommended_mcp_servers"`
	RecommendedProviderTypes []string       `json:"recommended_provider_types"`
	PersonaMemoryMarkdown    string         `json:"persona_memory_markdown"`
	CapabilityBindings       map[string]any `json:"capability_bindings"`
	BudgetPolicy             map[string]any `json:"budget_policy"`
	Metadata                 map[string]any `json:"metadata"`
}

type updateEmployeeTemplateRequest struct {
	Label                    string         `json:"label"`
	Description              string         `json:"description"`
	DefaultRole              string         `json:"default_role"`
	RecommendedSkills        []string       `json:"recommended_skills"`
	RecommendedMCPServers    []string       `json:"recommended_mcp_servers"`
	RecommendedProviderTypes []string       `json:"recommended_provider_types"`
	PersonaMemoryMarkdown    string         `json:"persona_memory_markdown"`
	CapabilityBindings       map[string]any `json:"capability_bindings"`
	BudgetPolicy             map[string]any `json:"budget_policy"`
	Metadata                 map[string]any `json:"metadata"`
}

type setEmployeeTemplateStatusRequest struct {
	Status string `json:"status"`
}

func (h *HTTPHandler) ListEmployeeTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template list")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	templates, err := service.ListEmployeeTemplates(r.Context(), tenantID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeTemplateResponses(templates))
}

func (h *HTTPHandler) GetEmployeeTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		http.Error(w, "invalid templateId", http.StatusBadRequest)
		return
	}
	template, err := service.GetEmployeeTemplate(r.Context(), tenantID, templateID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeTemplateResponseFromDomain(template))
}

func (h *HTTPHandler) CreateEmployeeTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template create")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if field, ok := firstLegacyEmployeeTemplateField(raw); ok {
		http.Error(w, field+" is no longer supported", http.StatusBadRequest)
		return
	}
	var req createEmployeeTemplateRequest
	payload, _ := json.Marshal(raw)
	if err := json.Unmarshal(payload, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	template, err := service.CreateEmployeeTemplate(r.Context(), CreateEmployeeTemplateParams{
		TenantID:                 tenantID,
		Type:                     req.Type,
		Label:                    req.Label,
		Description:              req.Description,
		DefaultRole:              req.DefaultRole,
		RecommendedSkills:        req.RecommendedSkills,
		RecommendedMCPServers:    req.RecommendedMCPServers,
		RecommendedProviderTypes: req.RecommendedProviderTypes,
		PersonaMemoryMarkdown:    req.PersonaMemoryMarkdown,
		CapabilityBindings:       req.CapabilityBindings,
		BudgetPolicy:             req.BudgetPolicy,
		Metadata:                 req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, employeeTemplateResponseFromDomain(template))
}

func (h *HTTPHandler) UpdateEmployeeTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		http.Error(w, "invalid templateId", http.StatusBadRequest)
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if field, ok := firstLegacyEmployeeTemplateField(raw); ok {
		http.Error(w, field+" is no longer supported", http.StatusBadRequest)
		return
	}
	var req updateEmployeeTemplateRequest
	payload, _ := json.Marshal(raw)
	if err := json.Unmarshal(payload, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	template, err := service.UpdateEmployeeTemplate(r.Context(), UpdateEmployeeTemplateParams{
		TenantID:                 tenantID,
		ID:                       templateID,
		Label:                    req.Label,
		Description:              req.Description,
		DefaultRole:              req.DefaultRole,
		RecommendedSkills:        req.RecommendedSkills,
		RecommendedMCPServers:    req.RecommendedMCPServers,
		RecommendedProviderTypes: req.RecommendedProviderTypes,
		PersonaMemoryMarkdown:    req.PersonaMemoryMarkdown,
		CapabilityBindings:       req.CapabilityBindings,
		BudgetPolicy:             req.BudgetPolicy,
		Metadata:                 req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeTemplateResponseFromDomain(template))
}

func (h *HTTPHandler) SetEmployeeTemplateStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template status update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		http.Error(w, "invalid templateId", http.StatusBadRequest)
		return
	}
	var req setEmployeeTemplateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	template, err := service.SetEmployeeTemplateStatus(r.Context(), tenantID, templateID, req.Status)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeTemplateResponseFromDomain(template))
}

func (h *HTTPHandler) DeleteEmployeeTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template delete")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		http.Error(w, "invalid templateId", http.StatusBadRequest)
		return
	}
	if err := service.DeleteEmployeeTemplate(r.Context(), tenantID, templateID); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func employeeTemplateResponses(templates []EmployeeTemplateRecord) []employeeTemplateResponse {
	responses := make([]employeeTemplateResponse, 0, len(templates))
	for _, template := range templates {
		responses = append(responses, employeeTemplateResponseFromDomain(template))
	}
	return responses
}

func employeeTemplateResponseFromDomain(t EmployeeTemplateRecord) employeeTemplateResponse {
	return employeeTemplateResponse{
		ID:                       t.ID.String(),
		TenantID:                 t.TenantID.String(),
		Type:                     t.Type,
		Label:                    t.Label,
		Description:              t.Description,
		DefaultRole:              t.DefaultRole,
		RecommendedSkills:        stringSliceForJSON(t.RecommendedSkills),
		RecommendedMCPServers:    stringSliceForJSON(t.RecommendedMCPServers),
		RecommendedProviderTypes: stringSliceForJSON(t.RecommendedProviderTypes),
		PersonaMemoryMarkdown:    t.PersonaMemoryMarkdown,
		CapabilityBindings:       cloneMap(t.CapabilityBindings),
		BudgetPolicy:             cloneMap(t.BudgetPolicy),
		Metadata:                 cloneMap(t.Metadata),
		Status:                   t.Status,
		IsSystem:                 t.IsSystem,
		CreatedAt:                timeString(t.CreatedAt),
		UpdatedAt:                timeString(t.UpdatedAt),
	}
}

func firstLegacyEmployeeTemplateField(raw map[string]json.RawMessage) (string, bool) {
	for _, field := range []string{
		"default_capability_selection",
		"default_context_policy_override",
		"default_approval_policy",
	} {
		if _, exists := raw[field]; exists {
			return field, true
		}
	}
	return "", false
}
