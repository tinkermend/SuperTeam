package scenariotemplate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/audit"
)

// fakeRepository is an in-memory Repository fake keyed by template_key,
// mirroring the package convention (see fakeVocabularyRepository in
// vocabulary_test.go) of testing services against hand-rolled fakes rather
// than a real Postgres connection.
type fakeRepository struct {
	templates map[string]ScenarioTemplate
	versions  map[uuid.UUID][]ScenarioTemplateVersion
	// maxVersionOverride, when set for a template, makes
	// GetScenarioTemplateMaxVersion report a stale MAX — used to simulate a
	// concurrent bump racing past the service's derivation.
	maxVersionOverride map[uuid.UUID]int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		templates: map[string]ScenarioTemplate{},
		versions:  map[uuid.UUID][]ScenarioTemplateVersion{},
	}
}

func (f *fakeRepository) GetScenarioTemplateMaxVersion(_ context.Context, _ uuid.UUID, templateID uuid.UUID) (int, error) {
	if override, ok := f.maxVersionOverride[templateID]; ok {
		return override, nil
	}
	max := 0
	for _, version := range f.versions[templateID] {
		if version.Version > max {
			max = version.Version
		}
	}
	return max, nil
}

func (f *fakeRepository) ListScenarioTemplates(_ context.Context, _ uuid.UUID) ([]ScenarioTemplate, error) {
	out := make([]ScenarioTemplate, 0, len(f.templates))
	for _, t := range f.templates {
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeRepository) GetScenarioTemplateByKey(_ context.Context, _ uuid.UUID, key string) (ScenarioTemplate, error) {
	t, ok := f.templates[key]
	if !ok {
		return ScenarioTemplate{}, ErrScenarioTemplateNotFound
	}
	return t, nil
}

func (f *fakeRepository) CreateScenarioTemplate(_ context.Context, params CreateScenarioTemplateParams) (ScenarioTemplate, error) {
	if _, ok := f.templates[params.Key]; ok {
		return ScenarioTemplate{}, ErrConflict
	}
	template := ScenarioTemplate{
		ID:            uuid.New(),
		TenantID:      params.TenantID,
		Key:           params.Key,
		Name:          params.Name,
		Description:   params.Description,
		Spec:          params.Spec,
		Status:        "active",
		ActiveVersion: 1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	f.templates[params.Key] = template
	return template, nil
}

func (f *fakeRepository) CreateScenarioTemplateVersion(_ context.Context, params CreateScenarioTemplateVersionParams) (ScenarioTemplateVersion, error) {
	for _, existing := range f.versions[params.TemplateID] {
		if existing.Version == params.Version {
			// Mirrors the pg layer's 23505-on-uq_scenario_template_versions
			// mapping (pg_repository.go mapConstraintError).
			return ScenarioTemplateVersion{}, ErrConflict
		}
	}
	version := ScenarioTemplateVersion{
		ID:         uuid.New(),
		TenantID:   params.TenantID,
		TemplateID: params.TemplateID,
		Version:    params.Version,
		Spec:       params.Spec,
		CreatedBy:  params.CreatedBy,
		CreatedAt:  time.Now(),
	}
	f.versions[params.TemplateID] = append(f.versions[params.TemplateID], version)
	return version, nil
}

func (f *fakeRepository) UpdateScenarioTemplateActiveSpec(_ context.Context, params UpdateScenarioTemplateActiveSpecParams) (ScenarioTemplate, error) {
	for key, t := range f.templates {
		if t.ID != params.TemplateID {
			continue
		}
		t.Spec = params.Spec
		t.ActiveVersion = params.ActiveVersion
		t.UpdatedAt = time.Now()
		f.templates[key] = t
		return t, nil
	}
	return ScenarioTemplate{}, ErrScenarioTemplateNotFound
}

func (f *fakeRepository) UpdateScenarioTemplateStatus(_ context.Context, params UpdateScenarioTemplateStatusParams) (ScenarioTemplate, error) {
	for key, t := range f.templates {
		if t.ID != params.TemplateID {
			continue
		}
		t.Status = params.Status
		t.Name = params.Name
		t.Description = params.Description
		t.UpdatedAt = time.Now()
		f.templates[key] = t
		return t, nil
	}
	return ScenarioTemplate{}, ErrScenarioTemplateNotFound
}

func (f *fakeRepository) ListScenarioTemplateVersions(_ context.Context, _ uuid.UUID, templateID uuid.UUID) ([]ScenarioTemplateVersion, error) {
	return f.versions[templateID], nil
}

type fakeAuditRecorder struct {
	events []*audit.Event
}

func (f *fakeAuditRecorder) RecordEvent(_ context.Context, event *audit.Event) error {
	f.events = append(f.events, event)
	return nil
}

func goodSpec(requiredCapability string) map[string]any {
	roles := []any{}
	if requiredCapability != "" {
		roles = append(roles, map[string]any{
			"key":                   "analyst",
			"title":                 "分析",
			"required_capabilities": []any{requiredCapability},
		})
	}
	return map[string]any{
		"spec_version": 2,
		"roles":        roles,
		"skeleton":     []any{},
		"exits":        []any{},
		"constraints":  []any{},
	}
}

func TestCreateScenarioTemplateValidatesSpecAndVocabulary(t *testing.T) {
	t.Run("bad spec rejected with ErrInvalidInput", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)

		_, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
			TenantID: uuid.New(),
			Key:      "bad_one",
			Name:     "坏模板",
			Spec: map[string]any{
				"spec_version": 2,
				"constraints":  []any{map[string]any{"kind": "unknown_kind"}},
			},
		})
		if err == nil {
			t.Fatal("expected error for invalid spec")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
		if len(repo.templates) != 0 {
			t.Fatalf("expected no template row on invalid spec, got %#v", repo.templates)
		}
	})

	t.Run("ghost capability rejected with key named in error", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)
		svc.SetVocabularyRepository(&fakeVocabularyRepository{active: map[string]bool{"code_review": true}})

		_, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
			TenantID: uuid.New(),
			Key:      "ghost_one",
			Name:     "幽灵模板",
			Spec:     goodSpec("ghost"),
		})
		if err == nil {
			t.Fatal("expected error for unknown capability key")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
		if !strings.Contains(err.Error(), "ghost") {
			t.Fatalf("expected error to mention the missing key %q, got %v", "ghost", err)
		}
		if len(repo.templates) != 0 {
			t.Fatalf("expected no template row on vocabulary rejection, got %#v", repo.templates)
		}
	})

	t.Run("good spec creates main row, version row 1, and an audit event", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)
		svc.SetVocabularyRepository(&fakeVocabularyRepository{active: map[string]bool{"code_review": true}})
		auditRecorder := &fakeAuditRecorder{}
		svc.SetAuditRecorder(auditRecorder)

		tenantID := uuid.New()
		actorID := uuid.New()
		created, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
			TenantID:    tenantID,
			ActorUserID: actorID,
			Key:         "ops_review",
			Name:        "运维评审",
			Description: "评审场景",
			Spec:        goodSpec("code_review"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if created.Key != "ops_review" || created.ActiveVersion != 1 || created.Status != "active" {
			t.Fatalf("unexpected created template: %#v", created)
		}
		if _, ok := repo.templates["ops_review"]; !ok {
			t.Fatal("expected main row to be created")
		}
		versions := repo.versions[created.ID]
		if len(versions) != 1 || versions[0].Version != 1 {
			t.Fatalf("expected exactly one version-1 row, got %#v", versions)
		}
		if len(auditRecorder.events) != 1 {
			t.Fatalf("expected one audit event, got %#v", auditRecorder.events)
		}
		if auditRecorder.events[0].Action != "create" || auditRecorder.events[0].ResourceType != "scenario_template" {
			t.Fatalf("unexpected audit event: %#v", auditRecorder.events[0])
		}
	})

	t.Run("duplicate key rejected with ErrConflict", func(t *testing.T) {
		repo := newFakeRepository()
		svc := NewService(repo)
		tenantID := uuid.New()

		if _, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
			TenantID: tenantID,
			Key:      "dup_key",
			Name:     "重复模板",
			Spec:     goodSpec(""),
		}); err != nil {
			t.Fatalf("setup create failed: %v", err)
		}

		_, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
			TenantID: tenantID,
			Key:      "dup_key",
			Name:     "重复模板2",
			Spec:     goodSpec(""),
		})
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("expected ErrConflict, got %v", err)
		}
	})
}

func TestCreateVersionBumpsActiveAndMirrorsSpec(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	auditRecorder := &fakeAuditRecorder{}
	svc.SetAuditRecorder(auditRecorder)

	tenantID := uuid.New()
	created, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
		TenantID: tenantID,
		Key:      "ops_review",
		Name:     "运维评审",
		Spec:     goodSpec("code_review"),
	})
	if err != nil {
		t.Fatalf("setup create failed: %v", err)
	}

	newSpec := goodSpec("code_review_v2")
	updated, err := svc.CreateVersion(context.Background(), CreateScenarioTemplateVersionRequest{
		TenantID: tenantID,
		Key:      "ops_review",
		Spec:     newSpec,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.ActiveVersion != 2 {
		t.Fatalf("expected active_version 2, got %d", updated.ActiveVersion)
	}

	stored := repo.templates["ops_review"]
	if stored.ActiveVersion != 2 {
		t.Fatalf("expected main row active_version mirrored to 2, got %d", stored.ActiveVersion)
	}
	storedRoles, _ := stored.Spec["roles"].([]any)
	if len(storedRoles) != 1 {
		t.Fatalf("expected main row spec mirrored to the new version spec, got %#v", stored.Spec)
	}

	versions := repo.versions[created.ID]
	if len(versions) != 2 || versions[0].Version != 1 || versions[1].Version != 2 {
		t.Fatalf("expected version rows [1,2], got %#v", versions)
	}

	if len(auditRecorder.events) != 2 {
		t.Fatalf("expected 2 audit events (create + version), got %d", len(auditRecorder.events))
	}
	if auditRecorder.events[1].Action != "version" {
		t.Fatalf("expected second audit event action=version, got %#v", auditRecorder.events[1])
	}
}

func TestCreateVersionRecoversFromPartialWrite(t *testing.T) {
	// Simulates a crash between version-insert and mirror-update: the version
	// table already holds a row ahead of the main row's active_version. The
	// next bump must derive from MAX(version)+1, not active_version+1, so
	// recovery is automatic instead of a permanent unique-violation wedge.
	repo := newFakeRepository()
	svc := NewService(repo)

	tenantID := uuid.New()
	created, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
		TenantID: tenantID,
		Key:      "ops_review",
		Name:     "运维评审",
		Spec:     goodSpec(""),
	})
	if err != nil {
		t.Fatalf("setup create failed: %v", err)
	}

	// Orphan version row 2 without the mirror update (main stays at v1).
	if _, err := repo.CreateScenarioTemplateVersion(context.Background(), CreateScenarioTemplateVersionParams{
		TenantID:   tenantID,
		TemplateID: created.ID,
		Version:    2,
		Spec:       goodSpec(""),
	}); err != nil {
		t.Fatalf("setup orphan version failed: %v", err)
	}
	if repo.templates["ops_review"].ActiveVersion != 1 {
		t.Fatalf("setup expected main row still at v1, got %d", repo.templates["ops_review"].ActiveVersion)
	}

	updated, err := svc.CreateVersion(context.Background(), CreateScenarioTemplateVersionRequest{
		TenantID: tenantID,
		Key:      "ops_review",
		Spec:     goodSpec(""),
	})
	if err != nil {
		t.Fatalf("expected bump to recover past the orphan row, got %v", err)
	}
	if updated.ActiveVersion != 3 {
		t.Fatalf("expected active_version 3 (MAX(2)+1), got %d", updated.ActiveVersion)
	}
	versions := repo.versions[created.ID]
	if len(versions) != 3 || versions[2].Version != 3 {
		t.Fatalf("expected version rows [1,2,3], got %#v", versions)
	}
}

func TestCreateVersionUniqueViolationMapsToConflict(t *testing.T) {
	// A residual duplicate (e.g. two concurrent bumps racing past MAX+1
	// derivation) must surface as ErrConflict (409), not a raw 500. The fake
	// repo returns ErrConflict on a duplicate version, mirroring the pg
	// layer's mapConstraintError on 23505.
	repo := newFakeRepository()
	svc := NewService(repo)

	tenantID := uuid.New()
	created, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
		TenantID: tenantID,
		Key:      "ops_review",
		Name:     "运维评审",
		Spec:     goodSpec(""),
	})
	if err != nil {
		t.Fatalf("setup create failed: %v", err)
	}

	// Force the next derived version (MAX+1 = 2) to already exist while
	// keeping MAX itself stale from the service's perspective.
	repo.maxVersionOverride = map[uuid.UUID]int{created.ID: 1}
	if _, err := repo.CreateScenarioTemplateVersion(context.Background(), CreateScenarioTemplateVersionParams{
		TenantID:   tenantID,
		TemplateID: created.ID,
		Version:    2,
		Spec:       goodSpec(""),
	}); err != nil {
		t.Fatalf("setup racing version failed: %v", err)
	}

	_, err = svc.CreateVersion(context.Background(), CreateScenarioTemplateVersionRequest{
		TenantID: tenantID,
		Key:      "ops_review",
		Spec:     goodSpec(""),
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict on duplicate version, got %v", err)
	}
}

func TestPatchStatusDisabled(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	auditRecorder := &fakeAuditRecorder{}
	svc.SetAuditRecorder(auditRecorder)

	tenantID := uuid.New()
	if _, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
		TenantID: tenantID,
		Key:      "ops_review",
		Name:     "运维评审",
		Spec:     goodSpec(""),
	}); err != nil {
		t.Fatalf("setup create failed: %v", err)
	}

	disabled := "disabled"
	updated, err := svc.Patch(context.Background(), PatchScenarioTemplateRequest{
		TenantID: tenantID,
		Key:      "ops_review",
		Status:   &disabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != "disabled" {
		t.Fatalf("expected status disabled, got %s", updated.Status)
	}

	// Existing adapter contract (app.go scenarioTemplateSourceAdapter.
	// GetScenarioTemplateSnapshot) treats Status != "active" as unusable and
	// falls back to generic planning. We only assert the persisted state the
	// adapter reads, not the adapter itself.
	refetched, err := svc.GetByKey(context.Background(), tenantID, "ops_review")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refetched.Status == "active" {
		t.Fatalf("expected disabled status to persist, got %q", refetched.Status)
	}

	if len(auditRecorder.events) != 2 {
		t.Fatalf("expected 2 audit events (create + status), got %d", len(auditRecorder.events))
	}
	patchEvent := auditRecorder.events[1]
	if patchEvent.Action != "status" {
		t.Fatalf("expected second audit event action=status, got %#v", patchEvent)
	}
	// Details must carry the old→new diff for keys that actually changed.
	statusDiff, ok := patchEvent.Details["status"].([]string)
	if !ok || len(statusDiff) != 2 || statusDiff[0] != "active" || statusDiff[1] != "disabled" {
		t.Fatalf(`expected Details["status"] = ["active","disabled"], got %#v`, patchEvent.Details)
	}
	if _, ok := patchEvent.Details["name"]; ok {
		t.Fatalf("expected unchanged name to be absent from Details diff, got %#v", patchEvent.Details)
	}
	if _, ok := patchEvent.Details["description"]; ok {
		t.Fatalf("expected unchanged description to be absent from Details diff, got %#v", patchEvent.Details)
	}
}

func TestPatchRejectsUnsupportedStatus(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	tenantID := uuid.New()
	if _, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
		TenantID: tenantID,
		Key:      "ops_review",
		Name:     "运维评审",
		Spec:     goodSpec(""),
	}); err != nil {
		t.Fatalf("setup create failed: %v", err)
	}

	bogus := "archived"
	_, err := svc.Patch(context.Background(), PatchScenarioTemplateRequest{
		TenantID: tenantID,
		Key:      "ops_review",
		Status:   &bogus,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

// --- spec_version 缺失护栏 (spec 2026-07-18-scenario-template-spec-version-guardrail) ---

// versionlessV2Spec is a v2-shaped spec whose author forgot "spec_version": 2 —
// pre-guardrail it would be v1-normalized with constraints/exits silently dropped.
func versionlessV2Spec() map[string]any {
	return map[string]any{
		"roles": []any{
			map[string]any{"key": "requester", "required_capabilities": []any{"code_review"}},
			map[string]any{"key": "approver", "required_capabilities": []any{"code_review"}},
		},
		"skeleton": []any{
			map[string]any{"step": "request", "role": "requester"},
			map[string]any{"step": "approve", "role": "approver", "depends_on": []any{"request"}},
		},
		"exits":       []any{map[string]any{"deliverable": "approval_record", "label": "审批通过"}},
		"constraints": []any{map[string]any{"kind": "role_independence", "roles": []any{"requester", "approver"}}},
	}
}

func TestCreateRejectsVersionlessV2Spec(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
		TenantID: uuid.New(),
		Key:      "versionless_v2",
		Name:     "无版本号v2模板",
		Spec:     versionlessV2Spec(),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for versionless v2 spec, got %v", err)
	}
	if !strings.Contains(err.Error(), "spec_version") {
		t.Fatalf("expected actionable spec_version message, got %v", err)
	}
	if !strings.Contains(err.Error(), "constraints") {
		t.Fatalf("expected offending keys named in error, got %v", err)
	}
	if len(repo.templates) != 0 {
		t.Fatalf("expected no template row, got %#v", repo.templates)
	}
}

func TestCreateVersionRejectsVersionlessV2Spec(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	svc.SetVocabularyRepository(&fakeVocabularyRepository{active: map[string]bool{"code_review": true}})
	tenantID := uuid.New()
	created, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
		TenantID: tenantID,
		Key:      "guardrail_bump",
		Name:     "护栏版本模板",
		Spec:     goodSpec("code_review"),
	})
	if err != nil {
		t.Fatalf("create baseline template: %v", err)
	}

	_, err = svc.CreateVersion(context.Background(), CreateScenarioTemplateVersionRequest{
		TenantID: tenantID,
		Key:      created.Key,
		Spec:     versionlessV2Spec(),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for versionless v2 spec on version bump, got %v", err)
	}
	if !strings.Contains(err.Error(), "spec_version") {
		t.Fatalf("expected actionable spec_version message, got %v", err)
	}
}

func TestCreateAcceptsGenuineV1SpecUnchanged(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	svc.SetVocabularyRepository(&fakeVocabularyRepository{active: map[string]bool{"code_review": true}})

	// 真 v1 形态：只有 v1 字段（independent_from），无任何 v2 专属键。
	created, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
		TenantID: uuid.New(),
		Key:      "genuine_v1",
		Name:     "真v1模板",
		Spec: map[string]any{
			"roles": []any{
				map[string]any{"key": "developer", "required_capabilities": []any{"code_review"}},
				map[string]any{"key": "reviewer", "required_capabilities": []any{"code_review"}, "independent_from": []any{"developer"}},
			},
			"skeleton": []any{
				map[string]any{"step": "develop", "role": "developer"},
				map[string]any{"step": "review", "role": "reviewer", "depends_on": []any{"develop"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("genuine v1 spec must stay accepted: %v", err)
	}
	// v1 归一化回归断言：independent_from 仍被合成为 role_independence 约束。
	parsed, err := ParseSpec(created.Spec)
	if err != nil {
		t.Fatalf("parse stored v1 spec: %v", err)
	}
	foundIndependence := false
	for _, constraint := range parsed.Constraints {
		if constraint.Kind == "role_independence" {
			foundIndependence = true
		}
	}
	if !foundIndependence {
		t.Fatalf("expected v1 independent_from to normalize into role_independence, got %#v", parsed.Constraints)
	}
}

func TestCreateRejectsVersionlessObjectAcceptanceCriteria(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), CreateScenarioTemplateRequest{
		TenantID: uuid.New(),
		Key:      "versionless_criteria",
		Name:     "无版本号对象判据模板",
		Spec: map[string]any{
			"roles": []any{map[string]any{"key": "developer"}},
			"default_acceptance_criteria": []any{
				map[string]any{"statement": "对象形态判据", "applies_from_exit": "branch_ref"},
			},
		},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for versionless object criteria, got %v", err)
	}
	if !strings.Contains(err.Error(), "default_acceptance_criteria") {
		t.Fatalf("expected offending field named in error, got %v", err)
	}
}
