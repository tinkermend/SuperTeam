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

var ErrScenarioTemplateNotFound = errors.New("scenario template not found")

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

type Repository interface {
	ListScenarioTemplates(ctx context.Context, tenantID uuid.UUID) ([]ScenarioTemplate, error)
	GetScenarioTemplateByKey(ctx context.Context, tenantID uuid.UUID, key string) (ScenarioTemplate, error)
}
