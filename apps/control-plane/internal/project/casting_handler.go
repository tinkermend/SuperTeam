package project

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (h *HTTPHandler) ListProjectCastings(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	templateKey := strings.TrimSpace(r.URL.Query().Get("template_key"))
	entries, err := service.ListCastings(r.Context(), tenantID, projectID, templateKey)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	out := make([]castingResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, castingResponseFrom(e))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *HTTPHandler) PutProjectCastings(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	var body struct {
		ScenarioTemplateKey string `json:"scenario_template_key"`
		Assignments         []struct {
			RoleKey           string    `json:"role_key"`
			DigitalEmployeeID uuid.UUID `json:"digital_employee_id"`
		} `json:"assignments"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	assignments := make([]CastingAssignment, 0, len(body.Assignments))
	for _, a := range body.Assignments {
		assignments = append(assignments, CastingAssignment{
			RoleKey:           a.RoleKey,
			DigitalEmployeeID: a.DigitalEmployeeID,
		})
	}
	entries, err := service.PutCasting(r.Context(), PutCastingRequest{
		TenantID:            tenantID,
		ProjectID:           projectID,
		ActorUserID:         actorID,
		ScenarioTemplateKey: body.ScenarioTemplateKey,
		Assignments:         assignments,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	out := make([]castingResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, castingResponseFrom(e))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *HTTPHandler) ListRoleCandidates(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	roleKey := strings.TrimSpace(r.URL.Query().Get("role_key"))
	required := r.URL.Query()["required_capability"]
	// also accept comma-separated required_capabilities
	if raw := strings.TrimSpace(r.URL.Query().Get("required_capabilities")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(part); t != "" {
				required = append(required, t)
			}
		}
	}
	candidates, err := service.ListRoleCandidates(r.Context(), tenantID, projectID, roleKey, required)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	out := make([]roleCandidateResponse, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, roleCandidateResponseFrom(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *HTTPHandler) GetPlaybookReadiness(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	templateKey := strings.TrimSpace(r.URL.Query().Get("template_key"))
	items, err := service.GetPlaybookReadiness(r.Context(), tenantID, projectID, templateKey)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	out := make([]playbookReadinessResponse, 0, len(items))
	for _, item := range items {
		out = append(out, playbookReadinessResponseFrom(item))
	}
	writeJSON(w, http.StatusOK, out)
}

// RequestCastingExpansion opens a casting_expansion human decision for a demand
// mid-execution (§7). Used by console/E2E and later by judge/coordinator adapters.
func (h *HTTPHandler) RequestCastingExpansion(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	var body struct {
		DemandID            uuid.UUID `json:"demand_id"`
		SuggestedRoleKey    string    `json:"suggested_role_key"`
		NeedsExternalRole   bool      `json:"needs_external_role"`
		Reason              string    `json:"reason"`
		ScenarioTemplateKey string    `json:"scenario_template_key"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	decision, err := service.RequestCastingExpansion(r.Context(), RequestCastingExpansionRequest{
		TenantID:            tenantID,
		ProjectID:           projectID,
		DemandID:            body.DemandID,
		SuggestedRoleKey:    body.SuggestedRoleKey,
		NeedsExternalRole:   body.NeedsExternalRole,
		Reason:              body.Reason,
		ScenarioTemplateKey: body.ScenarioTemplateKey,
		ActorType:           "human_user",
		ActorID:             actorID.String(),
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, decisionRequestResponseFromDomain(*decision))
}

type castingResponse struct {
	ID                  uuid.UUID `json:"id"`
	TenantID            uuid.UUID `json:"tenant_id"`
	ProjectID           uuid.UUID `json:"project_id"`
	ScenarioTemplateKey string    `json:"scenario_template_key"`
	RoleKey             string    `json:"role_key"`
	DigitalEmployeeID   uuid.UUID `json:"digital_employee_id"`
	CastByUserID        uuid.UUID `json:"cast_by_user_id"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func castingResponseFrom(e CastingEntry) castingResponse {
	return castingResponse{
		ID:                  e.ID,
		TenantID:            e.TenantID,
		ProjectID:           e.ProjectID,
		ScenarioTemplateKey: e.ScenarioTemplateKey,
		RoleKey:             e.RoleKey,
		DigitalEmployeeID:   e.DigitalEmployeeID,
		CastByUserID:        e.CastByUserID,
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
	}
}

type roleCandidateResponse struct {
	DigitalEmployeeID   uuid.UUID  `json:"digital_employee_id"`
	Name                string     `json:"name"`
	TeamID              *uuid.UUID `json:"team_id,omitempty"`
	TeamName            string     `json:"team_name"`
	RoleKeys            []string   `json:"role_keys"`
	MatchedCapabilities []string   `json:"matched_capabilities"`
	MissingCapabilities []string   `json:"missing_capabilities"`
	CapabilityFit       string     `json:"capability_fit"`
}

func roleCandidateResponseFrom(c RoleCandidate) roleCandidateResponse {
	keys := c.RoleKeys
	if keys == nil {
		keys = []string{}
	}
	matched := c.MatchedCapabilities
	if matched == nil {
		matched = []string{}
	}
	missing := c.MissingCapabilities
	if missing == nil {
		missing = []string{}
	}
	return roleCandidateResponse{
		DigitalEmployeeID:   c.DigitalEmployeeID,
		Name:                c.Name,
		TeamID:              c.TeamID,
		TeamName:            c.TeamName,
		RoleKeys:            keys,
		MatchedCapabilities: matched,
		MissingCapabilities: missing,
		CapabilityFit:       c.CapabilityFit,
	}
}

type playbookExitResponse struct {
	Deliverable   string   `json:"deliverable"`
	Label         string   `json:"label"`
	Reachable     bool     `json:"reachable"`
	RequiredRoles []string `json:"required_roles"`
	MissingRoles  []string `json:"missing_roles"`
}

type playbookReadinessResponse struct {
	ScenarioTemplateKey string                 `json:"scenario_template_key"`
	TemplateName        string                 `json:"template_name"`
	Runnable            bool                   `json:"runnable"`
	DeepestExit         *playbookExitResponse  `json:"deepest_exit"`
	NextExitNeedsRoles  []string               `json:"next_exit_needs_roles"`
	MissingRolesForAny  []string               `json:"missing_roles_for_any"`
	Exits               []playbookExitResponse `json:"exits"`
}

func playbookReadinessResponseFrom(item PlaybookReadiness) playbookReadinessResponse {
	exits := make([]playbookExitResponse, 0, len(item.Exits))
	for _, e := range item.Exits {
		exits = append(exits, playbookExitResponse{
			Deliverable:   e.Deliverable,
			Label:         e.Label,
			Reachable:     e.Reachable,
			RequiredRoles: nilToEmpty(e.RequiredRoles),
			MissingRoles:  nilToEmpty(e.MissingRoles),
		})
	}
	var deepest *playbookExitResponse
	if item.DeepestExit != nil {
		d := playbookExitResponse{
			Deliverable:   item.DeepestExit.Deliverable,
			Label:         item.DeepestExit.Label,
			Reachable:     item.DeepestExit.Reachable,
			RequiredRoles: nilToEmpty(item.DeepestExit.RequiredRoles),
			MissingRoles:  nilToEmpty(item.DeepestExit.MissingRoles),
		}
		deepest = &d
	}
	return playbookReadinessResponse{
		ScenarioTemplateKey: item.ScenarioTemplateKey,
		TemplateName:        item.TemplateName,
		Runnable:            item.Runnable,
		DeepestExit:         deepest,
		NextExitNeedsRoles:  nilToEmpty(item.NextExitNeedsRoles),
		MissingRolesForAny:  nilToEmpty(item.MissingRolesForAny),
		Exits:               exits,
	}
}

func nilToEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
