package externalintegration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/autonomypolicy"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// ProjectGateway exposes the project facts the two verbs need (live reference).
type ProjectGateway interface {
	GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectInfo, error)
	IsEligibleInitiator(ctx context.Context, tenantID, projectID, userID uuid.UUID) (bool, error)
	ListProjectSkillIDs(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) ([]uuid.UUID, error)
	IsDigitalEmployeeMember(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (bool, error)
}

type ProjectInfo struct {
	ID                 uuid.UUID
	Name               string
	CoordinationPolicy map[string]any
}

// PlaybookAutonomySource resolves a scenario template's autonomy_ceiling and
// parsed spec for full_auto exit-pin prechecks (F6).
type PlaybookAutonomySource interface {
	PlaybookAutonomyCeiling(ctx context.Context, tenantID uuid.UUID, templateKey string) (string, error)
	PlaybookSpec(ctx context.Context, tenantID uuid.UUID, templateKey string) (scenariotemplate.SpecV2, error)
}

// ChatRunner creates the envelope chat run (verb 1). The employee service
// enforces the live skill surface (out-of-envelope skill → invalid input).
type ChatRunner interface {
	CreateExternalChatRun(ctx context.Context, req ChatRunGatewayRequest) (runID uuid.UUID, status string, err error)
}

type ChatRunGatewayRequest struct {
	TenantID      uuid.UUID
	EmployeeID    uuid.UUID
	ProjectID     uuid.UUID
	ActorUserID   uuid.UUID
	Objective     string
	SkillIDs      []uuid.UUID
	ResumeOfRunID *uuid.UUID
	Metadata      map[string]any
}

// DemandSubmitter submits plan/loop demands into the task hub (verb 2).
type DemandSubmitter interface {
	SubmitExternalDemand(ctx context.Context, req DemandGatewayRequest) (demandID uuid.UUID, status string, err error)
}

type DemandGatewayRequest struct {
	TenantID            uuid.UUID
	ProjectID           uuid.UUID
	SubmittedByUserID   uuid.UUID
	Title               string
	Content             string
	CoordinationMode    string
	ScenarioTemplateKey *string
	SourceRefs          map[string]any
}

// AuditRecorder logs external calls (optional).
type AuditRecorder interface {
	LogEvent(ctx context.Context, eventType, actorType, actorID, resourceType, resourceID, action string) error
}

type Service struct {
	repo      Repository
	projects  ProjectGateway
	chats     ChatRunner
	demands   DemandSubmitter
	playbooks PlaybookAutonomySource
	audit     AuditRecorder
}

func NewService(repo Repository, projects ProjectGateway, chats ChatRunner, demands DemandSubmitter) *Service {
	return &Service{repo: repo, projects: projects, chats: chats, demands: demands}
}

func (s *Service) SetPlaybookAutonomySource(src PlaybookAutonomySource) {
	if s != nil {
		s.playbooks = src
	}
}

func (s *Service) SetAuditRecorder(recorder AuditRecorder) {
	if s != nil {
		s.audit = recorder
	}
}

// --- Admin CRUD -------------------------------------------------------------

func (s *Service) CreateIntegration(ctx context.Context, req CreateIntegrationRequest) (Integration, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Integration{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if req.ProjectID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return Integration{}, fmt.Errorf("%w: project_id and digital_employee_id are required", ErrInvalidInput)
	}
	if !req.AllowChatRun && !req.AllowDemandSubmit {
		return Integration{}, fmt.Errorf("%w: at least one verb (allow_chat_run / allow_demand_submit) must be enabled", ErrInvalidInput)
	}
	projectInfo, err := s.projects.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return Integration{}, err
	}
	// Grantor must hold launch rights on the project — the integration acts as
	// this user on every call (same model as automation actor_user_id).
	eligible, err := s.projects.IsEligibleInitiator(ctx, req.TenantID, req.ProjectID, req.CreatedByUserID)
	if err != nil {
		return Integration{}, err
	}
	if !eligible {
		return Integration{}, fmt.Errorf("%w: creator has no launch rights on the project", ErrForbidden)
	}
	member, err := s.projects.IsDigitalEmployeeMember(ctx, req.TenantID, req.ProjectID, req.DigitalEmployeeID)
	if err != nil {
		return Integration{}, err
	}
	if !member {
		return Integration{}, fmt.Errorf("%w: digital employee is not an active member of the project", ErrInvalidInput)
	}
	skillIDs := dedupeUUIDs(req.SkillIDs)
	if req.AllowChatRun && len(skillIDs) == 0 {
		// Empty skill_ids must NOT mean "full project Chat surface" (§3 / §8.1).
		// Chat verb requires an explicit non-empty envelope at grant time.
		return Integration{}, fmt.Errorf("%w: skill_ids is required when allow_chat_run is enabled (empty envelope is not the project default surface)", ErrInvalidInput)
	}
	if err := s.validateSkillEnvelope(ctx, req.TenantID, req.ProjectID, req.CreatedByUserID, skillIDs); err != nil {
		return Integration{}, err
	}
	tier, err := autonomypolicy.Normalize(req.AutonomyTier)
	if err != nil {
		return Integration{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	scenarioKey := normalizeScenarioKey(req.ScenarioTemplateKey)
	if err := s.validateTierAgainstCeilings(ctx, req.TenantID, tier, projectInfo.CoordinationPolicy, scenarioKey); err != nil {
		return Integration{}, err
	}
	pinnedExit := normalizeScenarioKey(req.PinnedExitDeliverable) // reuse trim helper for optional string
	ackExit := req.AcknowledgeExitTierSemantics
	if err := s.validateFullAutoExitPin(ctx, req.TenantID, tier, scenarioKey, pinnedExit, ackExit); err != nil {
		return Integration{}, err
	}
	maxCalls := int32(DefaultMaxCallsPerHour)
	if req.MaxCallsPerHour != nil {
		if *req.MaxCallsPerHour <= 0 {
			return Integration{}, fmt.Errorf("%w: max_calls_per_hour must be positive", ErrInvalidInput)
		}
		maxCalls = *req.MaxCallsPerHour
	}
	integration := Integration{
		TenantID:            req.TenantID,
		ProjectID:           req.ProjectID,
		DigitalEmployeeID:   req.DigitalEmployeeID,
		Name:                name,
		Description:         strings.TrimSpace(req.Description),
		AllowChatRun:        req.AllowChatRun,
		AllowDemandSubmit:   req.AllowDemandSubmit,
		SkillIDs:            skillIDs,
		ScenarioTemplateKey: scenarioKey,
		AutonomyTier:        tier,
		PinnedExitDeliverable: pinnedExit,
		AcknowledgeExitTierSemantics: ackExit,
		MaxCallsPerHour:     maxCalls,
		Status:              StatusActive,
		CreatedByUserID:     req.CreatedByUserID,
	}
	created, err := s.repo.CreateIntegration(ctx, integration)
	if err != nil {
		return Integration{}, err
	}
	s.logAudit(ctx, "external_integration.created", "user", req.CreatedByUserID.String(), created.ID.String(), "create")
	return created, nil
}

func (s *Service) GetIntegration(ctx context.Context, tenantID, integrationID uuid.UUID) (Integration, error) {
	return s.repo.GetIntegration(ctx, tenantID, integrationID)
}

func (s *Service) ListIntegrations(ctx context.Context, tenantID uuid.UUID, projectID *uuid.UUID) ([]Integration, error) {
	return s.repo.ListIntegrations(ctx, tenantID, projectID)
}

func (s *Service) UpdateIntegration(ctx context.Context, req UpdateIntegrationRequest) (Integration, error) {
	integration, err := s.repo.GetIntegration(ctx, req.TenantID, req.IntegrationID)
	if err != nil {
		return Integration{}, err
	}
	eligible, err := s.projects.IsEligibleInitiator(ctx, req.TenantID, integration.ProjectID, req.ActorUserID)
	if err != nil {
		return Integration{}, err
	}
	if !eligible {
		return Integration{}, fmt.Errorf("%w: actor has no launch rights on the project", ErrForbidden)
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return Integration{}, fmt.Errorf("%w: name cannot be empty", ErrInvalidInput)
		}
		integration.Name = name
	}
	if req.Description != nil {
		integration.Description = strings.TrimSpace(*req.Description)
	}
	if req.AllowChatRun != nil {
		integration.AllowChatRun = *req.AllowChatRun
	}
	if req.AllowDemandSubmit != nil {
		integration.AllowDemandSubmit = *req.AllowDemandSubmit
	}
	if !integration.AllowChatRun && !integration.AllowDemandSubmit {
		return Integration{}, fmt.Errorf("%w: at least one verb (allow_chat_run / allow_demand_submit) must be enabled", ErrInvalidInput)
	}
	if req.SkillIDs != nil {
		integration.SkillIDs = dedupeUUIDs(*req.SkillIDs)
	}
	if integration.AllowChatRun && len(integration.SkillIDs) == 0 {
		return Integration{}, fmt.Errorf("%w: skill_ids is required when allow_chat_run is enabled (empty envelope is not the project default surface)", ErrInvalidInput)
	}
	if req.ScenarioTemplateKeySet {
		integration.ScenarioTemplateKey = normalizeScenarioKey(req.ScenarioTemplateKey)
	}
	if req.AutonomyTier != nil {
		tier, err := autonomypolicy.Normalize(*req.AutonomyTier)
		if err != nil {
			return Integration{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		integration.AutonomyTier = tier
	}
	if req.PinnedExitDeliverable != nil {
		integration.PinnedExitDeliverable = normalizeScenarioKey(req.PinnedExitDeliverable)
	}
	if req.AcknowledgeExitTierSemanticsSet {
		integration.AcknowledgeExitTierSemantics = req.AcknowledgeExitTierSemantics
	}
	if req.MaxCallsPerHour != nil {
		if *req.MaxCallsPerHour <= 0 {
			return Integration{}, fmt.Errorf("%w: max_calls_per_hour must be positive", ErrInvalidInput)
		}
		integration.MaxCallsPerHour = *req.MaxCallsPerHour
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if status != StatusActive && status != StatusDisabled {
			return Integration{}, fmt.Errorf("%w: status must be active or disabled", ErrInvalidInput)
		}
		integration.Status = status
	}
	projectInfo, err := s.projects.GetProject(ctx, req.TenantID, integration.ProjectID)
	if err != nil {
		return Integration{}, err
	}
	if err := s.validateSkillEnvelope(ctx, req.TenantID, integration.ProjectID, req.ActorUserID, integration.SkillIDs); err != nil {
		return Integration{}, err
	}
	if err := s.validateTierAgainstCeilings(ctx, req.TenantID, integration.AutonomyTier, projectInfo.CoordinationPolicy, integration.ScenarioTemplateKey); err != nil {
		return Integration{}, err
	}
	if err := s.validateFullAutoExitPin(ctx, req.TenantID, integration.AutonomyTier, integration.ScenarioTemplateKey, integration.PinnedExitDeliverable, integration.AcknowledgeExitTierSemantics); err != nil {
		return Integration{}, err
	}
	updated, err := s.repo.UpdateIntegration(ctx, integration)
	if err != nil {
		return Integration{}, err
	}
	s.logAudit(ctx, "external_integration.updated", "user", req.ActorUserID.String(), updated.ID.String(), "update")
	return updated, nil
}

// --- Tokens -----------------------------------------------------------------

// IssueToken mints a dedicated integration token: `xi_` + 64 hex chars.
// Plaintext is returned exactly once; only the SHA-256 is stored (deterministic
// indexed lookup — safe for high-entropy random tokens, unlike passwords).
func (s *Service) IssueToken(ctx context.Context, tenantID, integrationID, actorUserID uuid.UUID) (string, Token, error) {
	integration, err := s.repo.GetIntegration(ctx, tenantID, integrationID)
	if err != nil {
		return "", Token{}, err
	}
	if integration.Status != StatusActive {
		return "", Token{}, fmt.Errorf("%w: cannot issue token for a disabled integration", ErrForbidden)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", Token{}, fmt.Errorf("generate integration token: %w", err)
	}
	plaintext := TokenPrefix + hex.EncodeToString(buf)
	token, err := s.repo.CreateToken(ctx, tenantID, integrationID, hashToken(plaintext))
	if err != nil {
		return "", Token{}, err
	}
	s.logAudit(ctx, "external_integration.token_issued", "user", actorUserID.String(), integrationID.String(), "issue_token")
	return plaintext, token, nil
}

func (s *Service) ListTokens(ctx context.Context, tenantID, integrationID uuid.UUID) ([]Token, error) {
	return s.repo.ListTokens(ctx, tenantID, integrationID)
}

func (s *Service) RevokeToken(ctx context.Context, tenantID, integrationID, tokenID, actorUserID uuid.UUID) error {
	if _, err := s.repo.RevokeToken(ctx, tenantID, integrationID, tokenID); err != nil {
		return err
	}
	s.logAudit(ctx, "external_integration.token_revoked", "user", actorUserID.String(), integrationID.String(), "revoke_token")
	return nil
}

// AuthorizeToken resolves a bearer plaintext to its active integration.
func (s *Service) AuthorizeToken(ctx context.Context, plaintext string) (Integration, error) {
	plaintext = strings.TrimSpace(plaintext)
	if !strings.HasPrefix(plaintext, TokenPrefix) {
		return Integration{}, ErrUnauthorized
	}
	token, err := s.repo.GetActiveTokenBySHA(ctx, hashToken(plaintext))
	if err != nil {
		return Integration{}, err
	}
	integration, err := s.repo.GetIntegration(ctx, token.TenantID, token.IntegrationID)
	if err != nil {
		return Integration{}, ErrUnauthorized
	}
	if integration.Status != StatusActive {
		return Integration{}, fmt.Errorf("%w: integration is disabled", ErrForbidden)
	}
	_ = s.repo.TouchTokenLastUsed(ctx, token.ID)
	return integration, nil
}

// --- The two verbs ----------------------------------------------------------

// ExecuteChatRun is verb 1: an envelope chat run. Output stays quarantined
// (run_kind=chat bypass effectiveness). Live skill envelope = binding ∩ project
// Chat surface; empty binding never expands to the project default surface.
func (s *Service) ExecuteChatRun(ctx context.Context, integration Integration, req ExternalChatRunRequest) (ExternalChatRunResult, error) {
	if !integration.AllowChatRun {
		return ExternalChatRunResult{}, fmt.Errorf("%w: chat_run verb is not enabled for this integration", ErrForbidden)
	}
	if integration.Status != StatusActive {
		return ExternalChatRunResult{}, fmt.Errorf("%w: integration is disabled", ErrForbidden)
	}
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return ExternalChatRunResult{}, fmt.Errorf("%w: objective is required", ErrInvalidInput)
	}
	// Chat has no human-gate collision; still recompute Effective for live
	// reference / audit (ceilings apply if a future chat gate appears).
	if _, err := s.resolveEffectiveAutonomy(ctx, integration); err != nil {
		return ExternalChatRunResult{}, err
	}
	liveSkills, err := s.resolveLiveSkillEnvelope(ctx, integration)
	if err != nil {
		return ExternalChatRunResult{}, err
	}
	if err := s.consumeBudget(ctx, integration); err != nil {
		return ExternalChatRunResult{}, err
	}
	runID, status, err := s.chats.CreateExternalChatRun(ctx, ChatRunGatewayRequest{
		TenantID:      integration.TenantID,
		EmployeeID:    integration.DigitalEmployeeID,
		ProjectID:     integration.ProjectID,
		ActorUserID:   integration.CreatedByUserID,
		Objective:     objective,
		SkillIDs:      liveSkills,
		ResumeOfRunID: req.ResumeOfRunID,
		Metadata: map[string]any{
			"source_type":             SourceType,
			"external_integration_id": integration.ID.String(),
		},
	})
	if err != nil {
		return ExternalChatRunResult{}, err
	}
	s.logAudit(ctx, "external_integration.chat_run", "integration", integration.ID.String(), runID.String(), "create_chat_run")
	return ExternalChatRunResult{RunID: runID, Status: status}, nil
}

// ExecuteDemandSubmit is verb 2: a plan/loop demand through the binding.
// Effective autonomy is recomputed on every call; anything other than full_auto
// is rejected synchronously (spec §2.2 / §4.4 / §10 — no inbox park).
func (s *Service) ExecuteDemandSubmit(ctx context.Context, integration Integration, req ExternalDemandRequest) (ExternalDemandResult, error) {
	if !integration.AllowDemandSubmit {
		return ExternalDemandResult{}, fmt.Errorf("%w: demand_submit verb is not enabled for this integration", ErrForbidden)
	}
	if integration.Status != StatusActive {
		return ExternalDemandResult{}, fmt.Errorf("%w: integration is disabled", ErrForbidden)
	}
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	if title == "" || content == "" {
		return ExternalDemandResult{}, fmt.Errorf("%w: title and content are required", ErrInvalidInput)
	}
	mode := strings.TrimSpace(req.CoordinationMode)
	if mode == "" {
		mode = "plan"
	}
	if mode != "plan" && mode != "loop" {
		return ExternalDemandResult{}, fmt.Errorf("%w: coordination_mode must be plan or loop", ErrInvalidInput)
	}
	effective, err := s.resolveEffectiveAutonomy(ctx, integration)
	if err != nil {
		return ExternalDemandResult{}, err
	}
	if effective != autonomypolicy.TierFullAuto {
		s.logAudit(ctx, "external_integration.policy_reject", "integration", integration.ID.String(), integration.ID.String(), "reject_pause_at_gate")
		return ExternalDemandResult{}, fmt.Errorf("%w: effective autonomy is %s (need full_auto); external API does not park into inbox", ErrPolicyReject, effective)
	}
	if err := s.consumeBudget(ctx, integration); err != nil {
		return ExternalDemandResult{}, err
	}
	sourceRefs := map[string]any{
		"external_integration_id":                integration.ID.String(),
		project.AutonomyTierSnapshotSourceRefKey: effective,
	}
	if integration.PinnedExitDeliverable != nil {
		if pin := strings.TrimSpace(*integration.PinnedExitDeliverable); pin != "" {
			sourceRefs[project.PinnedExitDeliverableSourceRefKey] = pin
		}
	}
	demandID, status, err := s.demands.SubmitExternalDemand(ctx, DemandGatewayRequest{
		TenantID:            integration.TenantID,
		ProjectID:           integration.ProjectID,
		SubmittedByUserID:   integration.CreatedByUserID,
		Title:               title,
		Content:             content,
		CoordinationMode:    mode,
		ScenarioTemplateKey: integration.ScenarioTemplateKey,
		SourceRefs:          sourceRefs,
	})
	if err != nil {
		return ExternalDemandResult{}, err
	}
	s.logAudit(ctx, "external_integration.demand_submitted", "integration", integration.ID.String(), demandID.String(), "submit_demand")
	return ExternalDemandResult{DemandID: demandID, Status: status}, nil
}

// LookupIntegrationAutonomy feeds coordination policy auto-resolve: the binding
// tier + signing actor. Ceilings are applied by the coordination side at gate
// time (live reference). Disabled bindings keep the grantor as actor so mid-flight
// gates can be policy-rejected (never left parked for an external demand).
func (s *Service) LookupIntegrationAutonomy(ctx context.Context, tenantID, integrationID uuid.UUID) (string, uuid.UUID, error) {
	integration, err := s.repo.GetIntegration(ctx, tenantID, integrationID)
	if err != nil {
		return "", uuid.Nil, err
	}
	if integration.Status != StatusActive {
		return autonomypolicy.TierPauseAtGate, integration.CreatedByUserID, nil
	}
	return integration.AutonomyTier, integration.CreatedByUserID, nil
}

// --- helpers ----------------------------------------------------------------

func (s *Service) consumeBudget(ctx context.Context, integration Integration) error {
	ok, err := s.repo.ConsumeBudget(ctx, integration.TenantID, integration.ID)
	if err != nil {
		return err
	}
	if !ok {
		// Distinguish disabled / missing from true over-budget (ConsumeBudget
		// also returns false when status≠active).
		current, getErr := s.repo.GetIntegration(ctx, integration.TenantID, integration.ID)
		if getErr == nil && current.Status != StatusActive {
			return fmt.Errorf("%w: integration is disabled", ErrForbidden)
		}
		s.logAudit(ctx, "external_integration.over_budget", "integration", integration.ID.String(), integration.ID.String(), "reject_over_budget")
		return fmt.Errorf("%w: max %d calls per hour", ErrOverBudget, integration.MaxCallsPerHour)
	}
	return nil
}

func (s *Service) resolveEffectiveAutonomy(ctx context.Context, integration Integration) (string, error) {
	projectInfo, err := s.projects.GetProject(ctx, integration.TenantID, integration.ProjectID)
	if err != nil {
		return "", err
	}
	projectCeiling := autonomypolicy.CoordinationPolicyCeiling(projectInfo.CoordinationPolicy)
	playbookCeiling := ""
	if integration.ScenarioTemplateKey != nil && s.playbooks != nil {
		key := strings.TrimSpace(*integration.ScenarioTemplateKey)
		if key != "" {
			ceiling, err := s.playbooks.PlaybookAutonomyCeiling(ctx, integration.TenantID, key)
			if err != nil {
				return "", fmt.Errorf("%w: scenario template %s: %v", ErrInvalidInput, key, err)
			}
			playbookCeiling = strings.TrimSpace(ceiling)
		}
	}
	return autonomypolicy.Effective(playbookCeiling, projectCeiling, integration.AutonomyTier), nil
}

// resolveLiveSkillEnvelope recomputes binding ∩ project Chat surface.
// Empty binding → empty envelope (never expands to project default).
// Skills no longer on the project surface are dropped; if the live set is empty
// while chat is enabled, the call is rejected (envelope collapsed).
func (s *Service) resolveLiveSkillEnvelope(ctx context.Context, integration Integration) ([]uuid.UUID, error) {
	bound := dedupeUUIDs(integration.SkillIDs)
	if len(bound) == 0 {
		return nil, fmt.Errorf("%w: skill envelope is empty", ErrInvalidInput)
	}
	projectSkills, err := s.projects.ListProjectSkillIDs(ctx, integration.TenantID, integration.ProjectID, integration.CreatedByUserID)
	if err != nil {
		return nil, err
	}
	allow := make(map[uuid.UUID]struct{}, len(projectSkills))
	for _, id := range projectSkills {
		allow[id] = struct{}{}
	}
	live := make([]uuid.UUID, 0, len(bound))
	for _, id := range bound {
		if _, ok := allow[id]; ok {
			live = append(live, id)
		}
	}
	if len(live) == 0 {
		return nil, fmt.Errorf("%w: live skill envelope is empty after intersecting with the project Chat surface", ErrInvalidInput)
	}
	return live, nil
}

func (s *Service) validateSkillEnvelope(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, skillIDs []uuid.UUID) error {
	if len(skillIDs) == 0 {
		return nil
	}
	projectSkills, err := s.projects.ListProjectSkillIDs(ctx, tenantID, projectID, actorUserID)
	if err != nil {
		return err
	}
	allow := make(map[uuid.UUID]struct{}, len(projectSkills))
	for _, id := range projectSkills {
		allow[id] = struct{}{}
	}
	for _, id := range skillIDs {
		if _, ok := allow[id]; !ok {
			return fmt.Errorf("%w: skill_id %s is outside the project Chat skill surface", ErrInvalidInput, id)
		}
	}
	return nil
}

func (s *Service) validateTierAgainstCeilings(ctx context.Context, tenantID uuid.UUID, tier string, policy map[string]any, scenarioTemplateKey *string) error {
	projectCeiling := autonomypolicy.CoordinationPolicyCeiling(policy)
	playbookCeiling := ""
	if scenarioTemplateKey != nil && s.playbooks != nil {
		key := strings.TrimSpace(*scenarioTemplateKey)
		if key != "" {
			ceiling, err := s.playbooks.PlaybookAutonomyCeiling(ctx, tenantID, key)
			if err == nil {
				playbookCeiling = strings.TrimSpace(ceiling)
			}
		}
	}
	effective := autonomypolicy.Effective(playbookCeiling, projectCeiling, tier)
	if effective == tier {
		return nil
	}
	switch {
	case playbookCeiling != "" && autonomypolicy.Effective(playbookCeiling, "", tier) != tier:
		return fmt.Errorf("%w: autonomy_tier %q exceeds playbook autonomy_ceiling %q", ErrInvalidInput, tier, playbookCeiling)
	case projectCeiling != "":
		return fmt.Errorf("%w: autonomy_tier %q exceeds project coordination_policy.autonomy_ceiling %q", ErrInvalidInput, tier, projectCeiling)
	default:
		return fmt.Errorf("%w: autonomy_tier %q exceeds autonomy ceiling", ErrInvalidInput, tier)
	}
}

func (s *Service) validateFullAutoExitPin(
	ctx context.Context,
	tenantID uuid.UUID,
	tier string,
	scenarioTemplateKey *string,
	pinned *string,
	acknowledge bool,
) error {
	if strings.TrimSpace(tier) != autonomypolicy.TierFullAuto {
		return nil
	}
	if scenarioTemplateKey == nil || strings.TrimSpace(*scenarioTemplateKey) == "" {
		return nil
	}
	if s.playbooks == nil {
		return nil
	}
	spec, err := s.playbooks.PlaybookSpec(ctx, tenantID, strings.TrimSpace(*scenarioTemplateKey))
	if err != nil {
		return fmt.Errorf("%w: load playbook for exit pin check: %v", ErrInvalidInput, err)
	}
	if !scenariotemplate.RequiresFullAutoExitPin(spec) {
		return nil
	}
	pin := ""
	if pinned != nil {
		pin = strings.TrimSpace(*pinned)
	}
	if err := scenariotemplate.ValidatePinnedExitDeliverable(spec, pin); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if pin == "" && !acknowledge {
		preview := scenariotemplate.PreviewExitAutonomy(spec)
		stopping := make([]string, 0)
		for _, exit := range preview {
			if exit.StopsHuman {
				stopping = append(stopping, exit.Deliverable)
			}
		}
		return fmt.Errorf(
			"%w: full_auto integrations bound to multi-exit playbooks require pinned_exit_deliverable or acknowledge_exit_tier_semantics=true (exits that stop humans: %s)",
			ErrInvalidInput,
			strings.Join(stopping, ","),
		)
	}
	return nil
}

func (s *Service) logAudit(ctx context.Context, eventType, actorType, actorID, resourceID, action string) {
	if s == nil || s.audit == nil {
		return
	}
	_ = s.audit.LogEvent(ctx, eventType, actorType, actorID, "external_integration", resourceID, action)
}

func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func normalizeScenarioKey(key *string) *string {
	if key == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*key)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
