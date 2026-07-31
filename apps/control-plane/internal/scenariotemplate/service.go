package scenariotemplate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/audit"
)

// AuditRecorder records domain audit events for scenario template writes.
// Left unset via SetAuditRecorder, writes proceed without emitting audit
// events (nil-safe, same optional-injection convention as
// VocabularyRepository).
type AuditRecorder interface {
	RecordEvent(ctx context.Context, event *audit.Event) error
}

var knownScenarioTemplateStatuses = map[string]bool{
	"active":   true,
	"disabled": true,
}

type Service struct {
	repository           Repository
	vocabularyRepository VocabularyRepository
	audit                AuditRecorder
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// SetAuditRecorder injects the audit event recorder. Left unset, writes
// proceed without emitting audit events — same nil-optional convention as
// SetVocabularyRepository.
func (s *Service) SetAuditRecorder(recorder AuditRecorder) {
	s.audit = recorder
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]ScenarioTemplate, error) {
	return s.repository.ListScenarioTemplates(ctx, tenantID)
}

func (s *Service) GetByKey(ctx context.Context, tenantID uuid.UUID, key string) (ScenarioTemplate, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return ScenarioTemplate{}, ErrScenarioTemplateNotFound
	}
	return s.repository.GetScenarioTemplateByKey(ctx, tenantID, key)
}

// ProduceKinds 返回模板活跃 spec 的骨架产出 kind 序列,去重保序(骨架顺序即
// 呈现顺序)。一单卷宗右轨用它定槽位次序;解析逻辑留在本包,调用方不得自行
// 解析 spec map。模板不存在或 spec 不可解析时返回错误,由调用方决定降级。
func (s *Service) ProduceKinds(ctx context.Context, tenantID uuid.UUID, key string) ([]string, error) {
	template, err := s.GetByKey(ctx, tenantID, key)
	if err != nil {
		return nil, err
	}
	spec, err := ParseSpec(template.Spec)
	if err != nil {
		return nil, err
	}
	kinds := make([]string, 0, len(spec.Skeleton))
	seen := map[string]bool{}
	for _, step := range spec.Skeleton {
		for _, produce := range step.ProducesDefaults {
			kind := strings.TrimSpace(produce.Kind)
			if kind == "" || seen[kind] {
				continue
			}
			seen[kind] = true
			kinds = append(kinds, kind)
		}
	}
	return kinds, nil
}

// CreateScenarioTemplateRequest builds a template's v1: main row + version
// row 1, mirrored.
type CreateScenarioTemplateRequest struct {
	TenantID    uuid.UUID
	ActorUserID uuid.UUID
	Key         string
	Name        string
	Description string
	Spec        map[string]any
}

// Create validates the spec (structure via ParseSpec, capability vocabulary
// via ValidateCapabilityKeys), rejects a duplicate template_key, then
// inserts the main row (v1, active) and version row 1, and records an
// audit event.
func (s *Service) Create(ctx context.Context, req CreateScenarioTemplateRequest) (ScenarioTemplate, error) {
	if req.TenantID == uuid.Nil {
		return ScenarioTemplate{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		return ScenarioTemplate{}, fmt.Errorf("%w: template key is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return ScenarioTemplate{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	spec := req.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	if err := rejectVersionlessV2Spec(spec); err != nil {
		return ScenarioTemplate{}, err
	}
	parsedSpec, err := ParseSpec(spec)
	if err != nil {
		return ScenarioTemplate{}, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}
	if err := s.validateSpecVocabulary(ctx, req.TenantID, parsedSpec); err != nil {
		return ScenarioTemplate{}, err
	}

	if _, err := s.repository.GetScenarioTemplateByKey(ctx, req.TenantID, key); err == nil {
		return ScenarioTemplate{}, fmt.Errorf("%w: template key %q already exists", ErrConflict, key)
	} else if !errors.Is(err, ErrScenarioTemplateNotFound) {
		return ScenarioTemplate{}, err
	}

	createdBy := actorPtr(req.ActorUserID)

	created, err := s.repository.CreateScenarioTemplate(ctx, CreateScenarioTemplateParams{
		TenantID:    req.TenantID,
		Key:         key,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Spec:        spec,
		CreatedBy:   createdBy,
	})
	if err != nil {
		return ScenarioTemplate{}, err
	}

	if _, err := s.repository.CreateScenarioTemplateVersion(ctx, CreateScenarioTemplateVersionParams{
		TenantID:   req.TenantID,
		TemplateID: created.ID,
		Version:    1,
		Spec:       spec,
		CreatedBy:  createdBy,
	}); err != nil {
		return ScenarioTemplate{}, err
	}

	s.recordAudit(ctx, req.TenantID, created.Key, "create", req.ActorUserID, map[string]any{
		"template_key": created.Key,
		"version":      1,
	})

	return created, nil
}

// CreateScenarioTemplateVersionRequest bumps a template to a new spec
// version.
type CreateScenarioTemplateVersionRequest struct {
	TenantID    uuid.UUID
	ActorUserID uuid.UUID
	Key         string
	Spec        map[string]any
}

// CreateVersion validates the new spec the same way Create does, then
// inserts version row (active_version+1) and mirrors it onto the main row.
func (s *Service) CreateVersion(ctx context.Context, req CreateScenarioTemplateVersionRequest) (ScenarioTemplate, error) {
	if req.TenantID == uuid.Nil {
		return ScenarioTemplate{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		return ScenarioTemplate{}, fmt.Errorf("%w: template key is required", ErrInvalidInput)
	}
	spec := req.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	if err := rejectVersionlessV2Spec(spec); err != nil {
		return ScenarioTemplate{}, err
	}
	parsedSpec, err := ParseSpec(spec)
	if err != nil {
		return ScenarioTemplate{}, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}
	if err := s.validateSpecVocabulary(ctx, req.TenantID, parsedSpec); err != nil {
		return ScenarioTemplate{}, err
	}

	existing, err := s.repository.GetScenarioTemplateByKey(ctx, req.TenantID, key)
	if err != nil {
		return ScenarioTemplate{}, err
	}

	// Derive from the version table's MAX, not the main row's active_version:
	// if a previous bump crashed between version-insert and mirror-update,
	// active_version is stale and active_version+1 would collide with the
	// orphan row forever. MAX+1 makes recovery from that partial write
	// automatic. A residual duplicate (two bumps racing this read) is caught
	// by the version table's unique constraint and surfaces as ErrConflict.
	maxVersion, err := s.repository.GetScenarioTemplateMaxVersion(ctx, req.TenantID, existing.ID)
	if err != nil {
		return ScenarioTemplate{}, err
	}
	nextVersion := maxVersion + 1
	createdBy := actorPtr(req.ActorUserID)

	if _, err := s.repository.CreateScenarioTemplateVersion(ctx, CreateScenarioTemplateVersionParams{
		TenantID:   req.TenantID,
		TemplateID: existing.ID,
		Version:    nextVersion,
		Spec:       spec,
		CreatedBy:  createdBy,
	}); err != nil {
		return ScenarioTemplate{}, err
	}

	updated, err := s.repository.UpdateScenarioTemplateActiveSpec(ctx, UpdateScenarioTemplateActiveSpecParams{
		TenantID:      req.TenantID,
		TemplateID:    existing.ID,
		Spec:          spec,
		ActiveVersion: nextVersion,
	})
	if err != nil {
		return ScenarioTemplate{}, err
	}

	s.recordAudit(ctx, req.TenantID, updated.Key, "version", req.ActorUserID, map[string]any{
		"template_key": updated.Key,
		"version":      nextVersion,
	})

	return updated, nil
}

// ListVersions returns the version history (newest first) for a template,
// resolved by key.
func (s *Service) ListVersions(ctx context.Context, tenantID uuid.UUID, key string) ([]ScenarioTemplateVersion, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrScenarioTemplateNotFound
	}
	template, err := s.repository.GetScenarioTemplateByKey(ctx, tenantID, key)
	if err != nil {
		return nil, err
	}
	return s.repository.ListScenarioTemplateVersions(ctx, tenantID, template.ID)
}

// PatchScenarioTemplateRequest partially updates status and/or name and/or
// description. Unset (nil) fields keep their current value.
type PatchScenarioTemplateRequest struct {
	TenantID    uuid.UUID
	ActorUserID uuid.UUID
	Key         string
	Status      *string
	Name        *string
	Description *string
}

// Patch applies a partial update to status/name/description and records an
// audit event. Setting Status to a non-"active" value (e.g. "disabled")
// makes the template unusable for new planning: see
// app.go's scenarioTemplateSourceAdapter, which treats any Status != "active"
// as an error and falls back to generic planning behavior.
func (s *Service) Patch(ctx context.Context, req PatchScenarioTemplateRequest) (ScenarioTemplate, error) {
	if req.TenantID == uuid.Nil {
		return ScenarioTemplate{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		return ScenarioTemplate{}, fmt.Errorf("%w: template key is required", ErrInvalidInput)
	}

	existing, err := s.repository.GetScenarioTemplateByKey(ctx, req.TenantID, key)
	if err != nil {
		return ScenarioTemplate{}, err
	}

	status := existing.Status
	if req.Status != nil {
		status = strings.TrimSpace(*req.Status)
		if !knownScenarioTemplateStatuses[status] {
			return ScenarioTemplate{}, fmt.Errorf("%w: unsupported status %q", ErrInvalidInput, status)
		}
	}
	name := existing.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" {
			return ScenarioTemplate{}, fmt.Errorf("%w: name cannot be blank", ErrInvalidInput)
		}
	}
	description := existing.Description
	if req.Description != nil {
		description = strings.TrimSpace(*req.Description)
	}

	updated, err := s.repository.UpdateScenarioTemplateStatus(ctx, UpdateScenarioTemplateStatusParams{
		TenantID:    req.TenantID,
		TemplateID:  existing.ID,
		Status:      status,
		Name:        name,
		Description: description,
	})
	if err != nil {
		return ScenarioTemplate{}, err
	}

	// Details carry the old→new diff for keys that actually changed, e.g.
	// {"status": ["active", "disabled"]}.
	details := map[string]any{"template_key": updated.Key}
	if status != existing.Status {
		details["status"] = []string{existing.Status, status}
	}
	if name != existing.Name {
		details["name"] = []string{existing.Name, name}
	}
	if description != existing.Description {
		details["description"] = []string{existing.Description, description}
	}
	// Audit action mirrors what actually changed: only a status transition is a
	// "status" action; name/description-only patches are a plain "update".
	action := "update"
	if status != existing.Status {
		action = "status"
	}
	s.recordAudit(ctx, req.TenantID, updated.Key, action, req.ActorUserID, details)

	return updated, nil
}

// rejectVersionlessV2Spec is the spec_version guardrail (spec
// 2026-07-18-scenario-template-spec-version-guardrail): a v2-shaped spec that
// forgot "spec_version": 2 would be v1-normalized with its governance fields
// silently dropped — reject it at write time with an actionable message
// instead of registering a template whose constraints don't exist at runtime.
func rejectVersionlessV2Spec(spec map[string]any) error {
	missing, offending := MissingSpecVersionForV2Shape(spec)
	if !missing {
		return nil
	}
	return fmt.Errorf(
		`%w: spec 包含 v2 字段（%s）但未声明 "spec_version": 2；v1 归一化会静默丢弃这些字段导致治理约束失效。请补 "spec_version": 2 后重试`,
		ErrInvalidInput, strings.Join(offending, "、"))
}

// validateSpecVocabulary collects every role's required_capabilities and
// rejects the spec (ErrInvalidInput, key names in the message) if any are
// not registered/active in the tenant's capability vocabulary. With no
// vocabulary repository injected, ValidateCapabilityKeys passes through.
func (s *Service) validateSpecVocabulary(ctx context.Context, tenantID uuid.UUID, spec SpecV2) error {
	var keys []string
	for _, role := range spec.Roles {
		keys = append(keys, role.RequiredCapabilities...)
	}
	unknown, err := s.ValidateCapabilityKeys(ctx, tenantID, keys)
	if err != nil {
		return err
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%w: unknown capability keys: %s", ErrInvalidInput, strings.Join(unknown, ", "))
	}
	return nil
}

func (s *Service) recordAudit(ctx context.Context, tenantID uuid.UUID, templateKey, action string, actorID uuid.UUID, details map[string]any) {
	if s.audit == nil {
		return
	}
	actor := ""
	if actorID != uuid.Nil {
		actor = actorID.String()
	}
	event := &audit.Event{
		TenantID:     tenantID,
		EventType:    "scenario_template",
		ActorType:    "user",
		ActorID:      actor,
		ResourceType: "scenario_template",
		ResourceID:   templateKey,
		Action:       action,
		Details:      details,
		CreatedAt:    time.Now().UTC(),
	}
	_ = s.audit.RecordEvent(ctx, event)
}

func actorPtr(actorID uuid.UUID) *uuid.UUID {
	if actorID == uuid.Nil {
		return nil
	}
	return &actorID
}
