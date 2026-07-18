package systemconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/audit"
)

type fakeRepo struct {
	overrides map[string]int64
	failReads bool
	getCalls  int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{overrides: map[string]int64{}}
}

func (r *fakeRepo) ListOverrides(_ context.Context, _ uuid.UUID) ([]Override, error) {
	if r.failReads {
		return nil, errors.New("db down")
	}
	out := []Override{}
	for key, value := range r.overrides {
		out = append(out, Override{ConfigKey: key, Value: value, UpdatedAt: time.Now()})
	}
	return out, nil
}

func (r *fakeRepo) GetOverride(_ context.Context, _ uuid.UUID, key string) (*Override, error) {
	r.getCalls++
	if r.failReads {
		return nil, errors.New("db down")
	}
	value, ok := r.overrides[key]
	if !ok {
		return nil, nil
	}
	return &Override{ConfigKey: key, Value: value, UpdatedAt: time.Now()}, nil
}

func (r *fakeRepo) UpsertOverride(_ context.Context, _ uuid.UUID, key string, value int64, _ uuid.UUID) error {
	r.overrides[key] = value
	return nil
}

func (r *fakeRepo) DeleteOverride(_ context.Context, _ uuid.UUID, key string) (bool, error) {
	_, ok := r.overrides[key]
	delete(r.overrides, key)
	return ok, nil
}

type fakeAudit struct {
	events []*audit.Event
}

func (a *fakeAudit) RecordEvent(_ context.Context, event *audit.Event) error {
	a.events = append(a.events, event)
	return nil
}

func TestRegistryDefaultsWithinBounds(t *testing.T) {
	for _, def := range Definitions() {
		if def.DefaultValue < def.MinValue || def.DefaultValue > def.MaxValue {
			t.Fatalf("definition %s default %d outside [%d, %d]", def.Key, def.DefaultValue, def.MinValue, def.MaxValue)
		}
	}
}

func TestSetRejectsUnknownKeyAndOutOfBounds(t *testing.T) {
	service := NewService(newFakeRepo())
	tenant := uuid.New()
	actor := uuid.New()

	if _, err := service.Set(context.Background(), tenant, "nope.unknown", 1, actor); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("expected ErrUnknownKey, got %v", err)
	}
	def, _ := LookupDefinition(KeySkillUploadMaxBytes)
	if _, err := service.Set(context.Background(), tenant, def.Key, def.MaxValue+1, actor); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("expected ErrInvalidValue above max, got %v", err)
	}
	if _, err := service.Set(context.Background(), tenant, def.Key, def.MinValue-1, actor); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("expected ErrInvalidValue below min, got %v", err)
	}
}

func TestSetResetRoundTripWithAuditAndCacheInvalidation(t *testing.T) {
	repo := newFakeRepo()
	recorder := &fakeAudit{}
	service := NewService(repo)
	service.SetAuditRecorder(recorder)
	tenant := uuid.New()
	actor := uuid.New()
	ctx := context.Background()

	if got := service.Int64(ctx, tenant, KeySkillUploadMaxBytes); got != DefaultFor(KeySkillUploadMaxBytes) {
		t.Fatalf("expected default before override, got %d", got)
	}
	target := int64(2 * 1024 * 1024)
	if _, err := service.Set(ctx, tenant, KeySkillUploadMaxBytes, target, actor); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	// 写后缓存失效,立即读到新值(同进程写后即时可见)。
	if got := service.Int64(ctx, tenant, KeySkillUploadMaxBytes); got != target {
		t.Fatalf("expected %d after set, got %d", target, got)
	}
	if _, err := service.Reset(ctx, tenant, KeySkillUploadMaxBytes, actor); err != nil {
		t.Fatalf("reset failed: %v", err)
	}
	if got := service.Int64(ctx, tenant, KeySkillUploadMaxBytes); got != DefaultFor(KeySkillUploadMaxBytes) {
		t.Fatalf("expected default after reset, got %d", got)
	}
	if len(recorder.events) != 2 {
		t.Fatalf("expected 2 audit events (update+reset), got %d", len(recorder.events))
	}
	if recorder.events[0].Action != "update" || recorder.events[1].Action != "reset" {
		t.Fatalf("unexpected audit actions: %s, %s", recorder.events[0].Action, recorder.events[1].Action)
	}
	// 幂等 reset:无覆盖时成功且不再写审计。
	if _, err := service.Reset(ctx, tenant, KeySkillUploadMaxBytes, actor); err != nil {
		t.Fatalf("idempotent reset failed: %v", err)
	}
	if len(recorder.events) != 2 {
		t.Fatalf("idempotent reset must not add audit events, got %d", len(recorder.events))
	}
}

func TestReadCachesWithinTTL(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo)
	tenant := uuid.New()
	ctx := context.Background()

	service.Int64(ctx, tenant, KeyArtifactMaxFileSizeBytes)
	service.Int64(ctx, tenant, KeyArtifactMaxFileSizeBytes)
	if repo.getCalls != 1 {
		t.Fatalf("expected single repo read within cache TTL, got %d", repo.getCalls)
	}
}

func TestReadFallsBackToDefaultOnRepoFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.failReads = true
	service := NewService(repo)
	tenant := uuid.New()

	if got := service.Int64(context.Background(), tenant, KeyAuthSessionTTLSeconds); got != DefaultFor(KeyAuthSessionTTLSeconds) {
		t.Fatalf("expected registry default on repo failure, got %d", got)
	}
}

func TestListProjectsOverrides(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo)
	tenant := uuid.New()
	actor := uuid.New()
	ctx := context.Background()

	if _, err := service.Set(ctx, tenant, KeyArtifactContentGetTTL, 120, actor); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	items, err := service.List(ctx, tenant)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(items) != len(Definitions()) {
		t.Fatalf("expected %d items, got %d", len(Definitions()), len(items))
	}
	for _, item := range items {
		if item.Key == KeyArtifactContentGetTTL {
			if !item.IsOverridden || item.EffectiveValue != 120 {
				t.Fatalf("expected overridden 120, got overridden=%v value=%d", item.IsOverridden, item.EffectiveValue)
			}
		} else if item.IsOverridden {
			t.Fatalf("unexpected overridden flag on %s", item.Key)
		}
	}
}
