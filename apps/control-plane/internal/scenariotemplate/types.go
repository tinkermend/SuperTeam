// Package scenariotemplate is the tenant-scoped registry of scenario templates:
// the sedimentation layer for "how this kind of work gets planned" (role
// contracts, decomposition skeleton, default handoff contracts). Content is
// finite, the mechanism is open — adding a scenario is a data row, never a
// code enum.
package scenariotemplate

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrScenarioTemplateNotFound = errors.New("scenario template not found")
	// ErrInvalidInput covers missing required fields, structurally invalid
	// specs (ParseSpec failures) and specs referencing capability vocabulary
	// keys that are not registered/active for the tenant.
	ErrInvalidInput = errors.New("invalid scenario template input")
	// ErrConflict is returned when a template_key already exists (active)
	// for the tenant.
	ErrConflict = errors.New("scenario template key already exists")
)

type ScenarioTemplate struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	Key           string
	Name          string
	Description   string
	Spec          map[string]any
	Status        string
	ActiveVersion int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ScenarioTemplateVersion is one immutable spec snapshot in a template's
// version history (scenario_template_versions). The main ScenarioTemplate
// row's Spec/ActiveVersion always mirror the version whose Version equals
// ActiveVersion.
type ScenarioTemplateVersion struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	TemplateID uuid.UUID
	Version    int
	Spec       map[string]any
	CreatedBy  *uuid.UUID
	CreatedAt  time.Time
}

type CreateScenarioTemplateParams struct {
	TenantID    uuid.UUID
	Key         string
	Name        string
	Description string
	Spec        map[string]any
	CreatedBy   *uuid.UUID
}

type CreateScenarioTemplateVersionParams struct {
	TenantID   uuid.UUID
	TemplateID uuid.UUID
	Version    int
	Spec       map[string]any
	CreatedBy  *uuid.UUID
}

type UpdateScenarioTemplateActiveSpecParams struct {
	TenantID      uuid.UUID
	TemplateID    uuid.UUID
	Spec          map[string]any
	ActiveVersion int
}

type UpdateScenarioTemplateStatusParams struct {
	TenantID    uuid.UUID
	TemplateID  uuid.UUID
	Status      string
	Name        string
	Description string
}

type Repository interface {
	ListScenarioTemplates(ctx context.Context, tenantID uuid.UUID) ([]ScenarioTemplate, error)
	GetScenarioTemplateByKey(ctx context.Context, tenantID uuid.UUID, key string) (ScenarioTemplate, error)
	CreateScenarioTemplate(ctx context.Context, params CreateScenarioTemplateParams) (ScenarioTemplate, error)
	CreateScenarioTemplateVersion(ctx context.Context, params CreateScenarioTemplateVersionParams) (ScenarioTemplateVersion, error)
	UpdateScenarioTemplateActiveSpec(ctx context.Context, params UpdateScenarioTemplateActiveSpecParams) (ScenarioTemplate, error)
	UpdateScenarioTemplateStatus(ctx context.Context, params UpdateScenarioTemplateStatusParams) (ScenarioTemplate, error)
	ListScenarioTemplateVersions(ctx context.Context, tenantID, templateID uuid.UUID) ([]ScenarioTemplateVersion, error)
}
