package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	return &Service{repository: repository}, nil
}

func (s *Service) CreateTeam(ctx context.Context, req CreateTeamRequest) (*Team, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if req.HumanOwnerUserID == nil || *req.HumanOwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: human_owner_user_id is required", ErrInvalidInput)
	}
	status := req.Status
	if status == "" {
		status = TeamStatusActive
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}

	record, err := s.repository.CreateTeam(ctx, CreateTeamParams{
		TenantID:         req.TenantID,
		Slug:             slug,
		Name:             name,
		Status:           status,
		HumanOwnerUserID: validUUIDPtr(req.HumanOwnerUserID),
		Metadata:         cloneMap(req.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) ListTeams(ctx context.Context, req ListTeamsRequest) ([]*Team, error) {
	req, err := normalizeListTeamsRequest(req)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListTeams(ctx, ListTeamsParams{
		TenantID: req.TenantID,
		Status:   req.Status,
		Q:        req.Q,
		Offset:   req.Offset,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	teams := make([]*Team, 0, len(records))
	for _, record := range records {
		teams = append(teams, teamFromRecord(record))
	}
	return teams, nil
}

func (s *Service) ListTeamSummaries(ctx context.Context, req ListTeamsRequest) ([]*TeamListItem, error) {
	req, err := normalizeListTeamsRequest(req)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListTeamSummaries(ctx, ListTeamSummariesParams{
		TenantID: req.TenantID,
		Status:   req.Status,
		Q:        req.Q,
		Offset:   req.Offset,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list team summaries: %w", err)
	}
	items := make([]*TeamListItem, 0, len(records))
	for _, record := range records {
		items = append(items, teamListItemFromRecord(record))
	}
	return items, nil
}

func (s *Service) GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (*Team, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetTeam(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) UpdateTeam(ctx context.Context, req UpdateTeamRequest) (*Team, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	humanOwnerUserID := validUUIDPtr(req.HumanOwnerUserID)
	metadata := cloneMap(req.Metadata)
	if req.HumanOwnerUserID == nil || req.Metadata == nil {
		existing, err := s.repository.GetTeam(ctx, req.TenantID, req.TeamID)
		if err != nil {
			return nil, fmt.Errorf("get team: %w", err)
		}
		if req.HumanOwnerUserID == nil {
			humanOwnerUserID = validUUIDPtr(existing.HumanOwnerUserID)
		}
		if req.Metadata == nil {
			metadata = cloneMap(existing.Metadata)
		}
	}
	record, err := s.repository.UpdateTeam(ctx, UpdateTeamParams{
		TenantID:         req.TenantID,
		TeamID:           req.TeamID,
		Slug:             slug,
		Name:             name,
		HumanOwnerUserID: humanOwnerUserID,
		Metadata:         metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("update team: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) ChangeTeamStatus(ctx context.Context, req ChangeTeamStatusRequest) (*Team, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if !req.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}
	record, err := s.repository.SetTeamStatus(ctx, SetTeamStatusParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
		Status:   req.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("set team status: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) GetOverview(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamOverview, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	summary, err := s.repository.GetTeamSummary(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get team summary: %w", err)
	}
	item := teamListItemFromRecord(summary)
	overview := &TeamOverview{
		Team:                 teamFromRecord(summary.Team),
		MemberCount:          item.MemberCount,
		DigitalEmployeeCount: item.DigitalEmployeeCount,
		CapabilityCount:      item.CapabilityCount,
		PendingDraftCount:    item.PendingDraftCount,
		PendingItemCount:     item.PendingDraftCount,
	}
	if revision, err := s.GetCurrentConfigRevision(ctx, tenantID, teamID); err == nil {
		overview.CurrentRevision = revision
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return overview, nil
}

func (s *Service) CreateConfigRevision(ctx context.Context, req CreateTeamConfigRevisionRequest) (*TeamConfigRevision, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.HumanOwnerUserID == nil || *req.HumanOwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: human_owner_user_id is required", ErrInvalidInput)
	}
	status := req.Status
	if status == "" {
		status = TeamConfigRevisionStatusActive
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid config revision status", ErrInvalidInput)
	}
	if status != TeamConfigRevisionStatusDraft && status != TeamConfigRevisionStatusActive {
		return nil, fmt.Errorf("%w: config revision status must be draft or active", ErrInvalidInput)
	}

	if _, err := s.repository.GetTeam(ctx, req.TenantID, req.TeamID); err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	if status == TeamConfigRevisionStatusActive {
		if _, err := s.repository.GetCurrentTeamConfigRevision(ctx, req.TenantID, req.TeamID); err == nil {
			return nil, fmt.Errorf("%w: active config revision already exists", ErrInvalidInput)
		} else if !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("get current team config revision: %w", err)
		}
	}

	nextRevision, err := s.repository.GetNextTeamConfigRevisionNumber(ctx, req.TenantID, req.TeamID)
	if err != nil {
		return nil, fmt.Errorf("get next team config revision number: %w", err)
	}
	approvedBy := validUUIDPtr(req.ApprovedBy)
	var approvedAt *time.Time
	if status == TeamConfigRevisionStatusActive {
		now := time.Now().UTC()
		approvedAt = &now
	} else {
		approvedBy = nil
	}
	record, err := s.repository.CreateTeamConfigRevision(ctx, CreateTeamConfigRevisionParams{
		TenantID:                    req.TenantID,
		TeamID:                      req.TeamID,
		RevisionNumber:              nextRevision,
		Constitution:                cloneMap(req.Constitution),
		CapabilityPolicy:            cloneMap(req.CapabilityPolicy),
		ContextPolicy:               cloneMap(req.ContextPolicy),
		ApprovalPolicy:              cloneMap(req.ApprovalPolicy),
		ArtifactContract:            cloneMap(req.ArtifactContract),
		InternalCollaborationPolicy: cloneMap(req.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneMap(req.RuntimeScopePolicy),
		HumanOwnerUserID:            validUUIDPtr(req.HumanOwnerUserID),
		Status:                      status,
		ApprovedBy:                  approvedBy,
		ApprovedAt:                  approvedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create team config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) GetConfigRevision(ctx context.Context, tenantID, revisionID uuid.UUID) (*TeamConfigRevision, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if revisionID == uuid.Nil {
		return nil, fmt.Errorf("%w: config_revision_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetTeamConfigRevision(ctx, tenantID, revisionID)
	if err != nil {
		return nil, fmt.Errorf("get team config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) GetCurrentConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamConfigRevision, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetCurrentTeamConfigRevision(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get current team config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) ListGovernanceDrafts(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*TeamConfigRevision, error) {
	params, err := normalizeListTeamConfigDraftsRequest(tenantID, teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListTeamConfigDrafts(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list team config drafts: %w", err)
	}
	drafts := make([]*TeamConfigRevision, 0, len(records))
	for _, record := range records {
		drafts = append(drafts, configRevisionFromRecord(record))
	}
	return drafts, nil
}

func (s *Service) CreateGovernanceDraft(ctx context.Context, req CreateTeamConfigRevisionRequest) (*TeamConfigRevision, error) {
	req.Status = TeamConfigRevisionStatusDraft
	req.ApprovedBy = nil
	return s.CreateConfigRevision(ctx, req)
}

func (s *Service) UpdateGovernanceDraft(ctx context.Context, tenantID, teamID, draftID uuid.UUID, input GovernanceDraftInput) (*TeamConfigRevision, error) {
	if err := validateGovernanceRevisionIDs(tenantID, teamID, draftID); err != nil {
		return nil, err
	}
	if issues := validateCapabilityBindingArrays(input.CapabilityPolicy); len(issues) > 0 {
		return nil, fmt.Errorf("%w: capability policy has invalid binding arrays", ErrInvalidInput)
	}
	record, err := s.repository.UpdateTeamConfigRevisionDraft(ctx, UpdateTeamConfigRevisionDraftParams{
		TenantID:                    tenantID,
		TeamID:                      teamID,
		RevisionID:                  draftID,
		Constitution:                cloneOptionalMap(input.Constitution),
		CapabilityPolicy:            cloneOptionalMap(input.CapabilityPolicy),
		ContextPolicy:               cloneOptionalMap(input.ContextPolicy),
		ApprovalPolicy:              cloneOptionalMap(input.ApprovalPolicy),
		ArtifactContract:            cloneOptionalMap(input.ArtifactContract),
		InternalCollaborationPolicy: cloneOptionalMap(input.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneOptionalMap(input.RuntimeScopePolicy),
		HumanOwnerUserID:            validUUIDPtr(input.HumanOwnerUserID),
	})
	if err != nil {
		return nil, fmt.Errorf("update governance draft: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) ApproveGovernanceDraft(ctx context.Context, tenantID, teamID, draftID, approvedBy uuid.UUID) (*TeamConfigRevision, error) {
	if err := validateGovernanceRevisionIDs(tenantID, teamID, draftID); err != nil {
		return nil, err
	}
	if approvedBy == uuid.Nil {
		return nil, fmt.Errorf("%w: approved_by is required", ErrInvalidInput)
	}
	team, err := s.repository.GetTeam(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	if team.Status == TeamStatusDisabled || team.Status == TeamStatusArchived {
		return nil, fmt.Errorf("%w: disabled or archived team cannot approve governance drafts", ErrInvalidInput)
	}
	draft, err := s.repository.GetTeamConfigRevision(ctx, tenantID, draftID)
	if err != nil {
		return nil, fmt.Errorf("get governance draft: %w", err)
	}
	if draft.TeamID != teamID {
		return nil, fmt.Errorf("%w: governance draft does not belong to team", ErrInvalidInput)
	}
	if draft.Status != TeamConfigRevisionStatusDraft {
		return nil, fmt.Errorf("%w: governance revision must be draft", ErrInvalidInput)
	}
	_, blockingErrors := validateGovernancePolicies(draft.Constitution, draft.CapabilityPolicy, true)
	if len(blockingErrors) > 0 {
		return nil, fmt.Errorf("%w: governance draft has blocking validation errors", ErrInvalidInput)
	}
	record, err := s.repository.ApproveTeamConfigRevision(ctx, ActivateTeamConfigRevisionParams{
		TenantID:   tenantID,
		TeamID:     teamID,
		RevisionID: draftID,
		ApprovedBy: approvedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("approve governance draft: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) RejectGovernanceDraft(ctx context.Context, tenantID, teamID, draftID uuid.UUID) (*TeamConfigRevision, error) {
	if err := validateGovernanceRevisionIDs(tenantID, teamID, draftID); err != nil {
		return nil, err
	}
	record, err := s.repository.RejectTeamConfigRevision(ctx, tenantID, teamID, draftID)
	if err != nil {
		return nil, fmt.Errorf("reject governance draft: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) PreviewGovernanceDiff(ctx context.Context, tenantID, teamID, draftID uuid.UUID) (*GovernanceDiffSummary, error) {
	if err := validateGovernanceRevisionIDs(tenantID, teamID, draftID); err != nil {
		return nil, err
	}
	draft, err := s.repository.GetTeamConfigRevision(ctx, tenantID, draftID)
	if err != nil {
		return nil, fmt.Errorf("get governance draft: %w", err)
	}
	if draft.TeamID != teamID {
		return nil, fmt.Errorf("%w: governance draft does not belong to team", ErrInvalidInput)
	}
	if draft.Status != TeamConfigRevisionStatusDraft {
		return nil, fmt.Errorf("%w: governance revision must be draft", ErrInvalidInput)
	}
	active, err := s.repository.GetCurrentTeamConfigRevision(ctx, tenantID, teamID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("get current team config revision: %w", err)
	}
	warnings, blockingErrors := validateGovernancePolicies(draft.Constitution, draft.CapabilityPolicy, true)
	summary := &GovernanceDiffSummary{
		AddedHardRules: countAddedHardRules(active.Constitution, draft.Constitution),
		Warnings:       warnings,
		BlockingErrors: blockingErrors,
	}
	if jsonMapChanged(active.CapabilityPolicy, draft.CapabilityPolicy) {
		summary.ChangedCapabilities = 1
	}
	if jsonMapChanged(active.ApprovalPolicy, draft.ApprovalPolicy) {
		summary.ChangedApprovalRules = 1
	}
	return summary, nil
}

func normalizeListTeamsRequest(req ListTeamsRequest) (ListTeamsRequest, error) {
	if req.TenantID == uuid.Nil {
		return req, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.Status != "" && !req.Status.IsValid() {
		return req, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}
	if req.Offset < 0 {
		return req, fmt.Errorf("%w: offset must be non-negative", ErrInvalidInput)
	}
	if req.Limit < 0 {
		return req, fmt.Errorf("%w: limit must be non-negative", ErrInvalidInput)
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	req.Q = strings.TrimSpace(req.Q)
	return req, nil
}

func normalizeListTeamConfigDraftsRequest(tenantID, teamID uuid.UUID, limit, offset int32) (ListTeamConfigDraftsParams, error) {
	if tenantID == uuid.Nil {
		return ListTeamConfigDraftsParams{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return ListTeamConfigDraftsParams{}, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if offset < 0 {
		return ListTeamConfigDraftsParams{}, fmt.Errorf("%w: offset must be non-negative", ErrInvalidInput)
	}
	if limit < 0 {
		return ListTeamConfigDraftsParams{}, fmt.Errorf("%w: limit must be non-negative", ErrInvalidInput)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	return ListTeamConfigDraftsParams{
		TenantID: tenantID,
		TeamID:   teamID,
		Offset:   offset,
		Limit:    limit,
	}, nil
}

func validateGovernanceRevisionIDs(tenantID, teamID, revisionID uuid.UUID) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if revisionID == uuid.Nil {
		return fmt.Errorf("%w: config_revision_id is required", ErrInvalidInput)
	}
	return nil
}

func teamFromRecord(record TeamRecord) *Team {
	return &Team{
		ID:               record.ID,
		TenantID:         record.TenantID,
		Slug:             record.Slug,
		Name:             record.Name,
		Status:           record.Status,
		HumanOwnerUserID: validUUIDPtr(record.HumanOwnerUserID),
		Metadata:         cloneMap(record.Metadata),
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}
}

func teamListItemFromRecord(record TeamListItemRecord) *TeamListItem {
	team := teamFromRecord(record.Team)
	return &TeamListItem{
		Team:                 *team,
		MemberCount:          record.MemberCount,
		DigitalEmployeeCount: record.DigitalEmployeeCount,
		CapabilityCount:      record.CapabilityCount,
		GovernanceStatus:     record.GovernanceStatus,
		CurrentRevision:      cloneInt32Ptr(record.CurrentRevision),
		PendingDraftCount:    record.PendingDraftCount,
		RiskSummary:          record.RiskSummary,
	}
}

func configRevisionFromRecord(record TeamConfigRevisionRecord) *TeamConfigRevision {
	return &TeamConfigRevision{
		ID:                          record.ID,
		TenantID:                    record.TenantID,
		TeamID:                      record.TeamID,
		RevisionNumber:              record.RevisionNumber,
		Constitution:                cloneMap(record.Constitution),
		CapabilityPolicy:            cloneMap(record.CapabilityPolicy),
		ContextPolicy:               cloneMap(record.ContextPolicy),
		ApprovalPolicy:              cloneMap(record.ApprovalPolicy),
		ArtifactContract:            cloneMap(record.ArtifactContract),
		InternalCollaborationPolicy: cloneMap(record.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneMap(record.RuntimeScopePolicy),
		HumanOwnerUserID:            validUUIDPtr(record.HumanOwnerUserID),
		Status:                      record.Status,
		ApprovedBy:                  validUUIDPtr(record.ApprovedBy),
		ApprovedAt:                  cloneTimePtr(record.ApprovedAt),
		CreatedAt:                   record.CreatedAt,
		UpdatedAt:                   record.UpdatedAt,
	}
}

func validateGovernancePolicies(constitution, capabilityPolicy map[string]any, requireHardRules bool) ([]ValidationIssue, []ValidationIssue) {
	warnings := []ValidationIssue{}
	blockingErrors := []ValidationIssue{}
	if requireHardRules {
		rules, ok, hardRuleIssues := hardRulesFromConstitution(constitution)
		blockingErrors = append(blockingErrors, hardRuleIssues...)
		if len(hardRuleIssues) == 0 && (!ok || len(rules) == 0) {
			blockingErrors = append(blockingErrors, ValidationIssue{
				Field:    "constitution.hard_rules",
				Message:  "hard_rules must be an array with at least one non-empty string",
				Severity: "error",
			})
		}
	}
	blockingErrors = append(blockingErrors, validateCapabilityBindingArrays(capabilityPolicy)...)
	return warnings, blockingErrors
}

func validateCapabilityBindingArrays(capabilityPolicy map[string]any) []ValidationIssue {
	keys := []string{
		"skill_bindings",
		"mcp_bindings",
		"knowledge_base_bindings",
		"external_capability_bindings",
		"allowed_skills",
		"allowed_mcp_servers",
		"allowed_plugins",
		"allowed_provider_types",
	}
	issues := []ValidationIssue{}
	for _, key := range keys {
		value, ok := capabilityPolicy[key]
		if !ok {
			continue
		}
		path := fmt.Sprintf("capability_policy.%s", key)
		switch typed := value.(type) {
		case []string:
			continue
		case []any:
			for index, item := range typed {
				if _, ok := item.(string); !ok {
					issues = append(issues, invalidGovernanceIssue(path, fmt.Sprintf("binding item %d must be a string", index)))
				}
			}
		default:
			issues = append(issues, invalidGovernanceIssue(path, "binding value must be an array of strings"))
		}
	}
	return issues
}

func invalidGovernanceIssue(field, message string) ValidationIssue {
	return ValidationIssue{
		Field:    field,
		Message:  message,
		Severity: "error",
	}
}

func countAddedHardRules(activeConstitution, draftConstitution map[string]any) int32 {
	activeRules, _, _ := hardRulesFromConstitution(activeConstitution)
	draftRules, _, _ := hardRulesFromConstitution(draftConstitution)
	activeSet := map[string]bool{}
	for _, rule := range activeRules {
		activeSet[rule] = true
	}
	seenDraft := map[string]bool{}
	var added int32
	for _, rule := range draftRules {
		if activeSet[rule] || seenDraft[rule] {
			continue
		}
		seenDraft[rule] = true
		added++
	}
	return added
}

func hardRulesFromConstitution(constitution map[string]any) ([]string, bool, []ValidationIssue) {
	value, ok := constitution["hard_rules"]
	if !ok {
		return nil, false, nil
	}
	switch typed := value.(type) {
	case []string:
		return normalizedStringList(typed), true, nil
	case []any:
		rules := make([]string, 0, len(typed))
		issues := []ValidationIssue{}
		for index, item := range typed {
			text, ok := item.(string)
			if !ok {
				issues = append(issues, invalidGovernanceIssue("constitution.hard_rules", fmt.Sprintf("hard_rules item %d must be a string", index)))
				continue
			}
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				rules = append(rules, trimmed)
			}
		}
		return rules, true, issues
	default:
		return nil, false, []ValidationIssue{invalidGovernanceIssue("constitution.hard_rules", "hard_rules must be an array of strings")}
	}
}

func normalizedStringList(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func jsonMapChanged(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(cloneMap(left))
	rightJSON, rightErr := json.Marshal(cloneMap(right))
	if leftErr != nil || rightErr != nil {
		return true
	}
	return string(leftJSON) != string(rightJSON)
}

func cloneInt32Ptr(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func validUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any)
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneOptionalMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	return cloneMap(value)
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
