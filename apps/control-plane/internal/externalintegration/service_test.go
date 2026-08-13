package externalintegration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/autonomypolicy"
)

type memoryRepo struct {
	integrations map[uuid.UUID]Integration
	tokens       map[uuid.UUID]Token
	tokenSHAs    map[string]uuid.UUID
	budgetLeft   map[uuid.UUID]int
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		integrations: map[uuid.UUID]Integration{},
		tokens:       map[uuid.UUID]Token{},
		tokenSHAs:    map[string]uuid.UUID{},
		budgetLeft:   map[uuid.UUID]int{},
	}
}

func (m *memoryRepo) CreateIntegration(_ context.Context, integration Integration) (Integration, error) {
	integration.ID = uuid.New()
	m.integrations[integration.ID] = integration
	m.budgetLeft[integration.ID] = int(integration.MaxCallsPerHour)
	return integration, nil
}

func (m *memoryRepo) GetIntegration(_ context.Context, tenantID, id uuid.UUID) (Integration, error) {
	integration, ok := m.integrations[id]
	if !ok || integration.TenantID != tenantID {
		return Integration{}, ErrNotFound
	}
	return integration, nil
}

func (m *memoryRepo) ListIntegrations(_ context.Context, tenantID uuid.UUID, projectID *uuid.UUID) ([]Integration, error) {
	out := []Integration{}
	for _, integration := range m.integrations {
		if integration.TenantID != tenantID {
			continue
		}
		if projectID != nil && integration.ProjectID != *projectID {
			continue
		}
		out = append(out, integration)
	}
	return out, nil
}

func (m *memoryRepo) UpdateIntegration(_ context.Context, integration Integration) (Integration, error) {
	if _, ok := m.integrations[integration.ID]; !ok {
		return Integration{}, ErrNotFound
	}
	m.integrations[integration.ID] = integration
	return integration, nil
}

func (m *memoryRepo) ConsumeBudget(_ context.Context, tenantID, id uuid.UUID) (bool, error) {
	integration, ok := m.integrations[id]
	if !ok || integration.TenantID != tenantID || integration.Status != StatusActive {
		return false, nil
	}
	if m.budgetLeft[id] <= 0 {
		return false, nil
	}
	m.budgetLeft[id]--
	return true, nil
}

func (m *memoryRepo) CreateToken(_ context.Context, tenantID, integrationID uuid.UUID, sha string) (Token, error) {
	token := Token{ID: uuid.New(), TenantID: tenantID, IntegrationID: integrationID, Status: TokenStatusActive}
	m.tokens[token.ID] = token
	m.tokenSHAs[sha] = token.ID
	return token, nil
}

func (m *memoryRepo) GetActiveTokenBySHA(_ context.Context, sha string) (Token, error) {
	id, ok := m.tokenSHAs[sha]
	if !ok {
		return Token{}, ErrUnauthorized
	}
	token := m.tokens[id]
	if token.Status != TokenStatusActive {
		return Token{}, ErrUnauthorized
	}
	return token, nil
}

func (m *memoryRepo) ListTokens(_ context.Context, tenantID, integrationID uuid.UUID) ([]Token, error) {
	out := []Token{}
	for _, token := range m.tokens {
		if token.TenantID == tenantID && token.IntegrationID == integrationID {
			out = append(out, token)
		}
	}
	return out, nil
}

func (m *memoryRepo) TouchTokenLastUsed(_ context.Context, _ uuid.UUID) error { return nil }

func (m *memoryRepo) RevokeToken(_ context.Context, tenantID, integrationID, tokenID uuid.UUID) (Token, error) {
	token, ok := m.tokens[tokenID]
	if !ok || token.TenantID != tenantID || token.IntegrationID != integrationID || token.Status != TokenStatusActive {
		return Token{}, ErrNotFound
	}
	token.Status = TokenStatusRevoked
	m.tokens[tokenID] = token
	return token, nil
}

type fakeProjects struct {
	policy        map[string]any
	eligible      bool
	skillIDs      []uuid.UUID
	employeeOK    bool
	getProjectErr error
}

func (f fakeProjects) GetProject(_ context.Context, _, projectID uuid.UUID) (ProjectInfo, error) {
	if f.getProjectErr != nil {
		return ProjectInfo{}, f.getProjectErr
	}
	return ProjectInfo{ID: projectID, Name: "P", CoordinationPolicy: f.policy}, nil
}

func (f fakeProjects) IsEligibleInitiator(_ context.Context, _, _, _ uuid.UUID) (bool, error) {
	return f.eligible, nil
}

func (f fakeProjects) ListProjectSkillIDs(_ context.Context, _, _, _ uuid.UUID) ([]uuid.UUID, error) {
	return append([]uuid.UUID(nil), f.skillIDs...), nil
}

func (f fakeProjects) IsDigitalEmployeeMember(_ context.Context, _, _, _ uuid.UUID) (bool, error) {
	return f.employeeOK, nil
}

type fakeChats struct {
	lastReq ChatRunGatewayRequest
	err     error
}

func (f *fakeChats) CreateExternalChatRun(_ context.Context, req ChatRunGatewayRequest) (uuid.UUID, string, error) {
	f.lastReq = req
	if f.err != nil {
		return uuid.Nil, "", f.err
	}
	return uuid.New(), "queued", nil
}

type fakeDemands struct {
	lastReq DemandGatewayRequest
}

func (f *fakeDemands) SubmitExternalDemand(_ context.Context, req DemandGatewayRequest) (uuid.UUID, string, error) {
	f.lastReq = req
	return uuid.New(), "pending_review", nil
}

type fakePlaybooks struct{ ceiling string }

func (f fakePlaybooks) PlaybookAutonomyCeiling(_ context.Context, _ uuid.UUID, _ string) (string, error) {
	return f.ceiling, nil
}

func newTestService(repo Repository, projects ProjectGateway) (*Service, *fakeChats, *fakeDemands) {
	chats := &fakeChats{}
	demands := &fakeDemands{}
	return NewService(repo, projects, chats, demands), chats, demands
}

func baseCreateRequest() CreateIntegrationRequest {
	skillID := uuid.New()
	return CreateIntegrationRequest{
		TenantID:          uuid.New(),
		CreatedByUserID:   uuid.New(),
		ProjectID:         uuid.New(),
		DigitalEmployeeID: uuid.New(),
		Name:              "外部工单系统",
		AllowChatRun:      true,
		AllowDemandSubmit: true,
		SkillIDs:          []uuid.UUID{skillID},
	}
}

func eligibleProjects(skillIDs ...uuid.UUID) fakeProjects {
	return fakeProjects{eligible: true, employeeOK: true, skillIDs: skillIDs}
}

func TestCreateIntegrationDefaultsAndValidation(t *testing.T) {
	repo := newMemoryRepo()
	req := baseCreateRequest()
	svc, _, _ := newTestService(repo, eligibleProjects(req.SkillIDs...))

	created, err := svc.CreateIntegration(context.Background(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.AutonomyTier != autonomypolicy.TierPauseAtGate {
		t.Fatalf("default tier = %s, want pause_at_gate", created.AutonomyTier)
	}
	if created.MaxCallsPerHour != DefaultMaxCallsPerHour {
		t.Fatalf("default budget = %d", created.MaxCallsPerHour)
	}
	if created.Status != StatusActive {
		t.Fatalf("status = %s", created.Status)
	}

	noVerbs := baseCreateRequest()
	noVerbs.AllowChatRun = false
	noVerbs.AllowDemandSubmit = false
	if _, err := svc.CreateIntegration(context.Background(), noVerbs); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("no-verb create = %v, want invalid input", err)
	}
}

func TestCreateIntegrationRequiresSkillEnvelopeForChat(t *testing.T) {
	svc, _, _ := newTestService(newMemoryRepo(), eligibleProjects())
	req := baseCreateRequest()
	req.SkillIDs = nil
	if _, err := svc.CreateIntegration(context.Background(), req); !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "skill_ids") {
		t.Fatalf("empty chat envelope = %v, want skill_ids required", err)
	}

	// Demand-only may omit skills.
	req.AllowChatRun = false
	req.AllowDemandSubmit = true
	if _, err := svc.CreateIntegration(context.Background(), req); err != nil {
		t.Fatalf("demand-only without skills: %v", err)
	}
}

func TestCreateIntegrationRejectsSkillOutsideProjectSurface(t *testing.T) {
	svc, _, _ := newTestService(newMemoryRepo(), eligibleProjects(uuid.New()))
	req := baseCreateRequest()
	if _, err := svc.CreateIntegration(context.Background(), req); !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "outside the project") {
		t.Fatalf("out-of-surface skill = %v", err)
	}
}

func TestCreateIntegrationRejectsNonMemberEmployee(t *testing.T) {
	req := baseCreateRequest()
	projects := eligibleProjects(req.SkillIDs...)
	projects.employeeOK = false
	svc, _, _ := newTestService(newMemoryRepo(), projects)
	if _, err := svc.CreateIntegration(context.Background(), req); !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "digital employee") {
		t.Fatalf("non-member employee = %v", err)
	}
}

func TestCreateIntegrationRejectsIneligibleCreator(t *testing.T) {
	req := baseCreateRequest()
	svc, _, _ := newTestService(newMemoryRepo(), fakeProjects{eligible: false, employeeOK: true, skillIDs: req.SkillIDs})
	if _, err := svc.CreateIntegration(context.Background(), req); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ineligible creator = %v, want forbidden", err)
	}
}

func TestCreateIntegrationTierExceedsProjectCeiling(t *testing.T) {
	req := baseCreateRequest()
	projects := eligibleProjects(req.SkillIDs...)
	projects.policy = map[string]any{"autonomy_ceiling": "pause_at_gate"}
	svc, _, _ := newTestService(newMemoryRepo(), projects)
	req.AutonomyTier = autonomypolicy.TierFullAuto
	_, err := svc.CreateIntegration(context.Background(), req)
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "autonomy_ceiling") {
		t.Fatalf("wide tier vs ceiling = %v, want ceiling rejection", err)
	}
}

func TestCreateIntegrationTierExceedsPlaybookCeiling(t *testing.T) {
	req := baseCreateRequest()
	svc, _, _ := newTestService(newMemoryRepo(), eligibleProjects(req.SkillIDs...))
	svc.SetPlaybookAutonomySource(fakePlaybooks{ceiling: autonomypolicy.TierPauseAtGate})
	req.AutonomyTier = autonomypolicy.TierFullAuto
	key := "cautious-playbook"
	req.ScenarioTemplateKey = &key
	_, err := svc.CreateIntegration(context.Background(), req)
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "playbook") {
		t.Fatalf("wide tier vs playbook = %v, want playbook rejection", err)
	}
}

func TestUpdateIntegrationRequiresEligibleActorAndPropagatesProjectErrors(t *testing.T) {
	req := baseCreateRequest()
	repo := newMemoryRepo()
	svc, _, _ := newTestService(repo, eligibleProjects(req.SkillIDs...))
	created, err := svc.CreateIntegration(context.Background(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ineligible := eligibleProjects(req.SkillIDs...)
	ineligible.eligible = false
	svc.projects = ineligible
	name := "x"
	if _, err := svc.UpdateIntegration(context.Background(), UpdateIntegrationRequest{
		TenantID: created.TenantID, IntegrationID: created.ID, ActorUserID: created.CreatedByUserID, Name: &name,
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ineligible update = %v", err)
	}

	broken := eligibleProjects(req.SkillIDs...)
	broken.getProjectErr = errors.New("project gone")
	svc.projects = broken
	if _, err := svc.UpdateIntegration(context.Background(), UpdateIntegrationRequest{
		TenantID: created.TenantID, IntegrationID: created.ID, ActorUserID: created.CreatedByUserID, Name: &name,
	}); err == nil || !strings.Contains(err.Error(), "project gone") {
		t.Fatalf("project error swallowed: %v", err)
	}
}

func TestTokenIssueAuthorizeRevokeRoundtrip(t *testing.T) {
	repo := newMemoryRepo()
	req := baseCreateRequest()
	svc, _, _ := newTestService(repo, eligibleProjects(req.SkillIDs...))
	created, err := svc.CreateIntegration(context.Background(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	plaintext, token, err := svc.IssueToken(context.Background(), created.TenantID, created.ID, created.CreatedByUserID)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !strings.HasPrefix(plaintext, TokenPrefix) {
		t.Fatalf("token plaintext %q missing prefix", plaintext)
	}

	resolved, err := svc.AuthorizeToken(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if resolved.ID != created.ID {
		t.Fatalf("authorized integration = %s, want %s", resolved.ID, created.ID)
	}

	if _, err := svc.AuthorizeToken(context.Background(), TokenPrefix+strings.Repeat("0", 64)); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bogus token = %v, want unauthorized", err)
	}

	if err := svc.RevokeToken(context.Background(), created.TenantID, created.ID, token.ID, created.CreatedByUserID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.AuthorizeToken(context.Background(), plaintext); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked token authorize = %v, want unauthorized", err)
	}
}

func TestAuthorizeTokenRejectsDisabledIntegration(t *testing.T) {
	repo := newMemoryRepo()
	req := baseCreateRequest()
	svc, _, _ := newTestService(repo, eligibleProjects(req.SkillIDs...))
	created, _ := svc.CreateIntegration(context.Background(), req)
	plaintext, _, _ := svc.IssueToken(context.Background(), created.TenantID, created.ID, created.CreatedByUserID)

	disabled := StatusDisabled
	if _, err := svc.UpdateIntegration(context.Background(), UpdateIntegrationRequest{
		TenantID:      created.TenantID,
		IntegrationID: created.ID,
		ActorUserID:   created.CreatedByUserID,
		Status:        &disabled,
	}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := svc.AuthorizeToken(context.Background(), plaintext); !errors.Is(err, ErrForbidden) {
		t.Fatalf("disabled integration authorize = %v, want forbidden", err)
	}
	if _, _, err := svc.IssueToken(context.Background(), created.TenantID, created.ID, created.CreatedByUserID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("issue on disabled = %v, want forbidden", err)
	}
}

func TestExecuteChatRunEnforcesVerbEnvelopeAndBudget(t *testing.T) {
	repo := newMemoryRepo()
	req := baseCreateRequest()
	skillID := req.SkillIDs[0]
	svc, chats, _ := newTestService(repo, eligibleProjects(skillID))
	two := int32(2)
	req.MaxCallsPerHour = &two
	created, err := svc.CreateIntegration(context.Background(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	result, err := svc.ExecuteChatRun(context.Background(), created, ExternalChatRunRequest{Objective: "查询工单"})
	if err != nil {
		t.Fatalf("chat run: %v", err)
	}
	if result.Status != "queued" {
		t.Fatalf("status = %s", result.Status)
	}
	if len(chats.lastReq.SkillIDs) != 1 || chats.lastReq.SkillIDs[0] != skillID {
		t.Fatalf("envelope skills not forwarded: %v", chats.lastReq.SkillIDs)
	}
	if chats.lastReq.Metadata["source_type"] != SourceType {
		t.Fatalf("metadata source_type = %v", chats.lastReq.Metadata["source_type"])
	}
	if chats.lastReq.Metadata["external_integration_id"] != created.ID.String() {
		t.Fatalf("metadata integration id = %v", chats.lastReq.Metadata["external_integration_id"])
	}
	if chats.lastReq.ActorUserID != created.CreatedByUserID {
		t.Fatalf("actor = %s, want grantor %s", chats.lastReq.ActorUserID, created.CreatedByUserID)
	}

	// Second call exhausts the 2-call budget; third must be 429-shaped.
	if _, err := svc.ExecuteChatRun(context.Background(), created, ExternalChatRunRequest{Objective: "再问一次"}); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if _, err := svc.ExecuteChatRun(context.Background(), created, ExternalChatRunRequest{Objective: "超预算"}); !errors.Is(err, ErrOverBudget) {
		t.Fatalf("over budget = %v, want ErrOverBudget", err)
	}

	// Verb off → forbidden without consuming budget.
	noChat := created
	noChat.AllowChatRun = false
	if _, err := svc.ExecuteChatRun(context.Background(), noChat, ExternalChatRunRequest{Objective: "x"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("verb off = %v, want forbidden", err)
	}
}

func TestExecuteChatRunLiveSkillIntersection(t *testing.T) {
	repo := newMemoryRepo()
	req := baseCreateRequest()
	kept := req.SkillIDs[0]
	dropped := uuid.New()
	req.SkillIDs = []uuid.UUID{kept, dropped}
	projects := eligibleProjects(kept, dropped)
	svc, chats, _ := newTestService(repo, projects)
	created, err := svc.CreateIntegration(context.Background(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Project surface tightens: only `kept` remains.
	projects.skillIDs = []uuid.UUID{kept}
	svc.projects = projects
	if _, err := svc.ExecuteChatRun(context.Background(), created, ExternalChatRunRequest{Objective: "收紧后"}); err != nil {
		t.Fatalf("intersect: %v", err)
	}
	if len(chats.lastReq.SkillIDs) != 1 || chats.lastReq.SkillIDs[0] != kept {
		t.Fatalf("live skills = %v, want [%s]", chats.lastReq.SkillIDs, kept)
	}

	// All skills removed from project → reject, do not expand to default surface.
	projects.skillIDs = nil
	svc.projects = projects
	if _, err := svc.ExecuteChatRun(context.Background(), created, ExternalChatRunRequest{Objective: "空面"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty live envelope = %v, want invalid input", err)
	}
}

func TestExecuteDemandSubmitRequiresFullAutoEffective(t *testing.T) {
	repo := newMemoryRepo()
	req := baseCreateRequest()
	req.AllowChatRun = false
	req.SkillIDs = nil
	req.AutonomyTier = autonomypolicy.TierPauseAtGate
	svc, _, demands := newTestService(repo, eligibleProjects())
	created, err := svc.CreateIntegration(context.Background(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := svc.ExecuteDemandSubmit(context.Background(), created, ExternalDemandRequest{
		Title: "外部工单同步", Content: "从工单系统触发",
	}); !errors.Is(err, ErrPolicyReject) {
		t.Fatalf("pause_at_gate demand = %v, want ErrPolicyReject", err)
	}
	if demands.lastReq.Title != "" {
		t.Fatal("demand must not be submitted when policy rejects")
	}

	// full_auto binding + project ceiling pause → still reject (live Effective).
	req.AutonomyTier = autonomypolicy.TierFullAuto
	wide, err := svc.CreateIntegration(context.Background(), req)
	if err != nil {
		t.Fatalf("create full_auto: %v", err)
	}
	svc.projects = fakeProjects{
		eligible: true, employeeOK: true,
		policy: map[string]any{"autonomy_ceiling": "pause_at_gate"},
	}
	if _, err := svc.ExecuteDemandSubmit(context.Background(), wide, ExternalDemandRequest{
		Title: "t", Content: "c",
	}); !errors.Is(err, ErrPolicyReject) {
		t.Fatalf("ceiling-tightened demand = %v, want ErrPolicyReject", err)
	}
}

func TestExecuteDemandSubmitStampsBindingFactsWhenFullAuto(t *testing.T) {
	repo := newMemoryRepo()
	req := baseCreateRequest()
	req.AllowChatRun = false
	req.SkillIDs = nil
	req.AutonomyTier = autonomypolicy.TierFullAuto
	key := "ops-playbook"
	req.ScenarioTemplateKey = &key
	svc, _, demands := newTestService(repo, eligibleProjects())
	created, err := svc.CreateIntegration(context.Background(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	result, err := svc.ExecuteDemandSubmit(context.Background(), created, ExternalDemandRequest{
		Title:   "外部工单同步",
		Content: "从工单系统触发",
	})
	if err != nil {
		t.Fatalf("demand: %v", err)
	}
	if result.DemandID == uuid.Nil {
		t.Fatal("demand id empty")
	}
	if demands.lastReq.CoordinationMode != "plan" {
		t.Fatalf("default mode = %s, want plan", demands.lastReq.CoordinationMode)
	}
	if demands.lastReq.ScenarioTemplateKey == nil || *demands.lastReq.ScenarioTemplateKey != key {
		t.Fatalf("scenario key not stamped from binding: %v", demands.lastReq.ScenarioTemplateKey)
	}
	if demands.lastReq.SourceRefs["external_integration_id"] != created.ID.String() {
		t.Fatalf("source_refs missing integration id: %v", demands.lastReq.SourceRefs)
	}
	if demands.lastReq.SubmittedByUserID != created.CreatedByUserID {
		t.Fatalf("submitter = %s, want grantor", demands.lastReq.SubmittedByUserID)
	}

	if _, err := svc.ExecuteDemandSubmit(context.Background(), created, ExternalDemandRequest{Title: "t", Content: "c", CoordinationMode: "chat"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("chat mode = %v, want invalid input", err)
	}
}

func TestLookupIntegrationAutonomyDisabledFallsToPause(t *testing.T) {
	repo := newMemoryRepo()
	req := baseCreateRequest()
	req.AutonomyTier = autonomypolicy.TierFullAuto
	svc, _, _ := newTestService(repo, eligibleProjects(req.SkillIDs...))
	created, err := svc.CreateIntegration(context.Background(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	tier, actor, err := svc.LookupIntegrationAutonomy(context.Background(), created.TenantID, created.ID)
	if err != nil || tier != autonomypolicy.TierFullAuto || actor != created.CreatedByUserID {
		t.Fatalf("lookup = %s/%s/%v", tier, actor, err)
	}

	disabled := StatusDisabled
	if _, err := svc.UpdateIntegration(context.Background(), UpdateIntegrationRequest{
		TenantID:      created.TenantID,
		IntegrationID: created.ID,
		ActorUserID:   created.CreatedByUserID,
		Status:        &disabled,
	}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	tier, actor, err = svc.LookupIntegrationAutonomy(context.Background(), created.TenantID, created.ID)
	if err != nil {
		t.Fatalf("lookup disabled: %v", err)
	}
	if tier != autonomypolicy.TierPauseAtGate || actor != created.CreatedByUserID {
		t.Fatalf("disabled lookup = %s/%s, want pause + grantor", tier, actor)
	}
}
