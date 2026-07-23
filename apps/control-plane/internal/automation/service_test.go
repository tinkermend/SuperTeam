package automation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRenderTemplateUsesTimezone(t *testing.T) {
	at := time.Date(2026, 7, 22, 1, 30, 0, 0, time.UTC) // 09:30 Asia/Shanghai
	got, err := RenderTemplate("日报 {{date}} {{datetime}} {{rule_name}} / {{project_name}}", at, "Asia/Shanghai", "晨检", "示例项目")
	if err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}
	want := "日报 2026-07-22 2026-07-22 09:30:00 晨检 / 示例项目"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRenderTemplateInvalidTimezone(t *testing.T) {
	_, err := RenderTemplate("x", time.Now(), "Not/AZone", "r", "p")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestFireIdempotent(t *testing.T) {
	tenantID := uuid.New()
	ruleID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo := newFakeRepo()
	repo.rules[ruleID] = Rule{
		ID:                  ruleID,
		TenantID:            tenantID,
		TeamID:              uuid.New(),
		ProjectID:           projectID,
		Name:                "晨检",
		Enabled:             true,
		CoordinationMode:    ModeLoop,
		DemandTitleTemplate: strPtr("title {{date}}"),
		DemandBodyTemplate:  strPtr("body"),
		ScheduleKind:        ScheduleInterval,
		IntervalSeconds:     int32Ptr(3600),
		Timezone:            DefaultTimezone,
		OverlapPolicy:       OverlapSkip,
		ActorUserID:         actorID,
	}
	demands := &fakeDemands{demandID: uuid.New()}
	svc := NewService(repo, &fakeProjects{eligible: true, name: "P"}, demands, nil, nil)
	at := time.Date(2026, 7, 22, 1, 0, 0, 0, time.UTC)

	first, err := svc.Fire(context.Background(), tenantID, ruleID, at)
	if err != nil {
		t.Fatalf("first fire: %v", err)
	}
	if first.Status != FireStatusSucceeded {
		t.Fatalf("status=%s", first.Status)
	}
	if demands.calls != 1 {
		t.Fatalf("demand calls=%d", demands.calls)
	}

	second, err := svc.Fire(context.Background(), tenantID, ruleID, at)
	if err != nil {
		t.Fatalf("second fire: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected same fire id")
	}
	if demands.calls != 1 {
		t.Fatalf("expected idempotent demand call, got %d", demands.calls)
	}
}

func TestFireConsecutiveFailuresDisable(t *testing.T) {
	tenantID := uuid.New()
	ruleID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo := newFakeRepo()
	scheduleID := "automation-rule:" + ruleID.String()
	repo.rules[ruleID] = Rule{
		ID:                  ruleID,
		TenantID:            tenantID,
		TeamID:              uuid.New(),
		ProjectID:           projectID,
		Name:                "晨检",
		Enabled:             true,
		CoordinationMode:    ModeLoop,
		DemandTitleTemplate: strPtr("title"),
		DemandBodyTemplate:  strPtr("body"),
		ScheduleKind:        ScheduleInterval,
		IntervalSeconds:     int32Ptr(3600),
		Timezone:            DefaultTimezone,
		OverlapPolicy:       OverlapSkip,
		ActorUserID:         actorID,
		TemporalScheduleID:  &scheduleID,
	}
	demands := &fakeDemands{err: errors.New("boom")}
	schedules := &fakeSchedules{}
	svc := NewService(repo, &fakeProjects{eligible: true, name: "P"}, demands, nil, schedules)

	for i := 0; i < MaxConsecutiveFailures; i++ {
		at := time.Date(2026, 7, 22, 1, 0, i, 0, time.UTC)
		fire, err := svc.Fire(context.Background(), tenantID, ruleID, at)
		if err != nil {
			t.Fatalf("fire %d: %v", i, err)
		}
		if fire.Status != FireStatusFailed {
			t.Fatalf("fire %d status=%s", i, fire.Status)
		}
	}
	rule := repo.rules[ruleID]
	if rule.Enabled {
		t.Fatalf("expected rule disabled after consecutive failures")
	}
	if rule.DisabledReason == nil || *rule.DisabledReason != DisabledReasonConsecutiveFireFailures {
		t.Fatalf("disabled_reason=%v", rule.DisabledReason)
	}
	if schedules.pauseCalls != 1 {
		t.Fatalf("expected schedule pause once, got %d", schedules.pauseCalls)
	}
}

func TestCascadeForProjectDeletedRemovesRulesAndSchedules(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ruleID := uuid.New()
	scheduleID := "automation-rule:" + ruleID.String()
	repo := newFakeRepo()
	repo.rules[ruleID] = Rule{
		ID:                 ruleID,
		TenantID:           tenantID,
		ProjectID:          projectID,
		Enabled:            true,
		TemporalScheduleID: &scheduleID,
	}
	schedules := &fakeSchedules{}
	svc := NewService(repo, &fakeProjects{eligible: true, name: "P"}, nil, nil, schedules)
	if err := svc.CascadeForProjectDeleted(context.Background(), tenantID, projectID); err != nil {
		t.Fatalf("cascade: %v", err)
	}
	if _, ok := repo.rules[ruleID]; ok {
		t.Fatal("expected rule deleted")
	}
	if schedules.deleteCalls != 1 {
		t.Fatalf("expected schedule delete once, got %d", schedules.deleteCalls)
	}
}

func TestFireSkipsOverlap(t *testing.T) {
	tenantID := uuid.New()
	ruleID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo := newFakeRepo()
	repo.rules[ruleID] = Rule{
		ID:                  ruleID,
		TenantID:            tenantID,
		TeamID:              uuid.New(),
		ProjectID:           projectID,
		Name:                "晨检",
		Enabled:             true,
		CoordinationMode:    ModeLoop,
		DemandTitleTemplate: strPtr("title"),
		DemandBodyTemplate:  strPtr("body"),
		ScheduleKind:        ScheduleInterval,
		IntervalSeconds:     int32Ptr(3600),
		Timezone:            DefaultTimezone,
		OverlapPolicy:       OverlapSkip,
		ActorUserID:         actorID,
	}
	demandID := uuid.New()
	repo.nonTerminal[ruleID] = Fire{
		ID:       uuid.New(),
		TenantID: tenantID,
		RuleID:   ruleID,
		Status:   FireStatusSucceeded,
		DemandID: &demandID,
	}
	demands := &fakeDemands{demandID: uuid.New()}
	svc := NewService(repo, &fakeProjects{eligible: true, name: "P"}, demands, nil, nil)
	fire, err := svc.Fire(context.Background(), tenantID, ruleID, time.Date(2026, 7, 22, 2, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("fire: %v", err)
	}
	if fire.Status != FireStatusSkippedOverlap {
		t.Fatalf("status=%s", fire.Status)
	}
	if demands.calls != 0 {
		t.Fatalf("expected no demand submit on overlap")
	}
	if repo.rules[ruleID].ConsecutiveFailureCount != 0 {
		t.Fatalf("overlap must not increment failure count")
	}
}

type fakeRepo struct {
	mu          sync.Mutex
	rules       map[uuid.UUID]Rule
	fires       map[string]Fire
	firesByID   map[uuid.UUID]Fire
	nonTerminal map[uuid.UUID]Fire
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		rules:       map[uuid.UUID]Rule{},
		fires:       map[string]Fire{},
		firesByID:   map[uuid.UUID]Fire{},
		nonTerminal: map[uuid.UUID]Fire{},
	}
}

func (r *fakeRepo) CreateRule(ctx context.Context, rule Rule) (Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	r.rules[rule.ID] = rule
	return rule, nil
}

func (r *fakeRepo) GetRule(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule, ok := r.rules[ruleID]
	if !ok || rule.TenantID != tenantID {
		return Rule{}, ErrNotFound
	}
	return rule, nil
}

func (r *fakeRepo) ListRules(ctx context.Context, req ListRulesRequest) ([]Rule, error) {
	return nil, nil
}
func (r *fakeRepo) UpdateRule(ctx context.Context, rule Rule) (Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules[rule.ID] = rule
	return rule, nil
}
func (r *fakeRepo) SetRuleEnabled(ctx context.Context, tenantID, ruleID uuid.UUID, enabled bool, disabledReason *string) (Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule, ok := r.rules[ruleID]
	if !ok {
		return Rule{}, ErrNotFound
	}
	rule.Enabled = enabled
	rule.DisabledReason = disabledReason
	if enabled {
		rule.ConsecutiveFailureCount = 0
		rule.DisabledReason = nil
	}
	r.rules[ruleID] = rule
	return rule, nil
}
func (r *fakeRepo) SetRuleScheduleID(ctx context.Context, tenantID, ruleID uuid.UUID, scheduleID *string) (Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule := r.rules[ruleID]
	rule.TemporalScheduleID = scheduleID
	r.rules[ruleID] = rule
	return rule, nil
}
func (r *fakeRepo) IncrementFailureCount(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule := r.rules[ruleID]
	rule.ConsecutiveFailureCount++
	r.rules[ruleID] = rule
	return rule, nil
}
func (r *fakeRepo) ResetFailureCount(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule := r.rules[ruleID]
	rule.ConsecutiveFailureCount = 0
	r.rules[ruleID] = rule
	return rule, nil
}
func (r *fakeRepo) DisableRuleSystem(ctx context.Context, tenantID, ruleID uuid.UUID, reason string) (Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule := r.rules[ruleID]
	rule.Enabled = false
	rule.DisabledReason = &reason
	r.rules[ruleID] = rule
	return rule, nil
}
func (r *fakeRepo) DeleteRule(ctx context.Context, tenantID, ruleID uuid.UUID) error {
	return nil
}
func (r *fakeRepo) ListRulesByProject(ctx context.Context, tenantID, projectID uuid.UUID) ([]Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Rule, 0)
	for _, rule := range r.rules {
		if rule.TenantID == tenantID && rule.ProjectID == projectID {
			out = append(out, rule)
		}
	}
	return out, nil
}
func (r *fakeRepo) DeleteRulesForProject(ctx context.Context, tenantID, projectID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, rule := range r.rules {
		if rule.TenantID == tenantID && rule.ProjectID == projectID {
			delete(r.rules, id)
		}
	}
	return nil
}
func (r *fakeRepo) ListEnabledRulesByActor(ctx context.Context, tenantID, actorUserID uuid.UUID) ([]Rule, error) {
	return nil, nil
}
func (r *fakeRepo) ListEnabledRulesByActorOnProject(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) ([]Rule, error) {
	return nil, nil
}
func (r *fakeRepo) CreateFire(ctx context.Context, fire Fire) (Fire, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.fires[fire.IdempotencyKey]; ok {
		return Fire{}, errors.New("unique violation")
	}
	fire.ID = uuid.New()
	fire.CreatedAt = time.Now().UTC()
	r.fires[fire.IdempotencyKey] = fire
	r.firesByID[fire.ID] = fire
	return fire, nil
}
func (r *fakeRepo) GetFireByIdempotency(ctx context.Context, idempotencyKey string) (Fire, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fire, ok := r.fires[idempotencyKey]
	if !ok {
		return Fire{}, ErrNotFound
	}
	return fire, nil
}
func (r *fakeRepo) UpdateFire(ctx context.Context, fire Fire) (Fire, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fires[fire.IdempotencyKey] = fire
	r.firesByID[fire.ID] = fire
	return fire, nil
}
func (r *fakeRepo) ListFires(ctx context.Context, req ListFiresRequest) ([]Fire, error) {
	return nil, nil
}
func (r *fakeRepo) GetLatestNonTerminalFire(ctx context.Context, tenantID, ruleID uuid.UUID) (Fire, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fire, ok := r.nonTerminal[ruleID]
	if !ok {
		return Fire{}, ErrNotFound
	}
	return fire, nil
}

type fakeProjects struct {
	eligible bool
	name     string
}

func (f *fakeProjects) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectInfo, error) {
	return ProjectInfo{ID: projectID, TeamID: uuid.New(), Name: f.name}, nil
}
func (f *fakeProjects) IsEligibleInitiator(ctx context.Context, tenantID, projectID, userID uuid.UUID) (bool, error) {
	return f.eligible, nil
}

type fakeDemands struct {
	demandID uuid.UUID
	err      error
	calls    int
}

func (f *fakeDemands) SubmitDemand(ctx context.Context, req DemandSubmitRequest) (DemandSubmitResult, error) {
	f.calls++
	if f.err != nil {
		return DemandSubmitResult{}, f.err
	}
	return DemandSubmitResult{DemandID: f.demandID}, nil
}

type fakeSchedules struct {
	pauseCalls  int
	deleteCalls int
}

func (f *fakeSchedules) Create(ctx context.Context, rule Rule) (string, error) { return "", nil }
func (f *fakeSchedules) Update(ctx context.Context, rule Rule) error          { return nil }
func (f *fakeSchedules) Pause(ctx context.Context, scheduleID string, note string) error {
	f.pauseCalls++
	return nil
}
func (f *fakeSchedules) Unpause(ctx context.Context, scheduleID string, note string) error {
	return nil
}
func (f *fakeSchedules) Delete(ctx context.Context, scheduleID string) error {
	f.deleteCalls++
	return nil
}

func strPtr(v string) *string { return &v }
func int32Ptr(v int32) *int32 { return &v }
